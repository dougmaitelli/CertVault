package api

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/certvault/certvault/config"
	"github.com/certvault/certvault/service"
	"github.com/certvault/certvault/store"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

const principalKey contextKey = "principal"

const contentSecurityPolicy = "default-src 'self'; " +
	"style-src 'self' 'unsafe-inline'; " +
	"img-src 'self' data:; " +
	"connect-src 'self'"

func New(c *config.Config, repos *store.Repositories, m *service.Manager, log *slog.Logger) (http.Handler, error) {
	a := &API{cfg: c, repos: repos, manager: m, log: log}
	if c.Auth.BootstrapTokenFile != "" {
		b, e := os.ReadFile(c.Auth.BootstrapTokenFile)
		if e != nil {
			return nil, e
		}
		a.bootstrap = strings.TrimSpace(string(b))
	}
	if c.Auth.OIDC != nil {
		p, e := oidc.NewProvider(context.Background(), c.Auth.OIDC.IssuerURL)
		if e != nil {
			return nil, fmt.Errorf("OIDC discovery: %w", e)
		}
		secret, e := os.ReadFile(c.Auth.OIDC.ClientSecretFile)
		if e != nil {
			return nil, e
		}
		a.oidc = p
		a.oauth = &oauth2.Config{
			ClientID:     c.Auth.OIDC.ClientID,
			ClientSecret: strings.TrimSpace(string(secret)),
			Endpoint:     p.Endpoint(),
			RedirectURL:  c.Auth.OIDC.RedirectURL,
			Scopes:       []string{oidc.ScopeOpenID, "profile", "email", "groups"},
		}
	}
	return a.routes(), nil
}

func (a *API) bootstrapLogin(w http.ResponseWriter, r *http.Request) {
	var in bootstrapLoginRequest
	if decode(r, &in) != nil || a.bootstrap == "" || !hmac.Equal([]byte(in.Token), []byte(a.bootstrap)) {
		problem(w, 401, "unauthorized", "Invalid bootstrap token")
		return
	}
	a.setSession(w, "bootstrap-admin")
	a.repos.Audits.Record(r.Context(), "bootstrap-admin", "auth.login", "ui", "", remoteIP(r))
	w.WriteHeader(204)
}

func (a *API) login(w http.ResponseWriter, r *http.Request) {
	if a.oauth == nil {
		problem(w, 404, "oidc_disabled", "OIDC is not configured")
		return
	}
	state := randomToken()
	nonce := randomToken()
	verifier := oauth2.GenerateVerifier()
	a.states.Store(state, oidcState{Nonce: nonce, Verifier: verifier, At: time.Now()})
	http.Redirect(w, r, a.oauth.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier)), http.StatusFound)
}

func (a *API) callback(w http.ResponseWriter, r *http.Request) {
	value, ok := a.states.LoadAndDelete(r.URL.Query().Get("state"))
	if !ok {
		problem(w, 400, "invalid_state", "OIDC state is invalid")
		return
	}
	st, ok := value.(oidcState)
	if !ok {
		problem(w, http.StatusBadRequest, "invalid_state", "OIDC state is invalid")
		return
	}
	if time.Since(st.At) > 10*time.Minute {
		problem(w, 400, "expired_state", "OIDC state expired")
		return
	}
	token, e := a.oauth.Exchange(r.Context(), r.URL.Query().Get("code"), oauth2.VerifierOption(st.Verifier))
	if e != nil {
		problem(w, 401, "oidc_error", e.Error())
		return
	}
	raw, _ := token.Extra("id_token").(string)
	verified, e := a.oidc.Verifier(&oidc.Config{ClientID: a.oauth.ClientID}).Verify(r.Context(), raw)
	if e != nil || verified.Nonce != st.Nonce {
		problem(w, 401, "oidc_error", "ID token verification failed")
		return
	}
	var claims oidcClaims
	if verified.Claims(&claims) != nil || !groupAllowed(claims.Groups, a.cfg.Auth.OIDC.AllowedGroups) {
		problem(w, 403, "forbidden", "OIDC group is not allowed")
		return
	}
	name := claims.Email
	if name == "" {
		name = claims.Sub
	}
	a.setSession(w, name)
	a.repos.Audits.Record(r.Context(), name, "auth.login", "ui", "oidc", remoteIP(r))
	http.Redirect(w, r, "/", http.StatusFound)
}

func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     "cv_session",
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   strings.HasPrefix(a.cfg.Server.PublicURL, "https://"),
		SameSite: http.SameSiteStrictMode,
	})
	w.WriteHeader(204)
}

func (a *API) setSession(w http.ResponseWriter, name string) {
	payload := base64.RawURLEncoding.EncodeToString([]byte(name + "\n" + strconv.FormatInt(time.Now().Add(12*time.Hour).Unix(), 10)))
	mac := hmac.New(sha256.New, a.cfg.MasterKey)
	mac.Write([]byte(payload))
	value := payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	http.SetCookie(w, &http.Cookie{
		Name:     "cv_session",
		Value:    value,
		Path:     "/",
		MaxAge:   43200,
		HttpOnly: true,
		Secure:   strings.HasPrefix(a.cfg.Server.PublicURL, "https://"),
		SameSite: http.SameSiteStrictMode,
	})
}

func (a *API) verifySession(value string) (string, bool) {
	p := strings.Split(value, ".")
	if len(p) != 2 {
		return "", false
	}
	mac := hmac.New(sha256.New, a.cfg.MasterKey)
	mac.Write([]byte(p[0]))
	sig, e := base64.RawURLEncoding.DecodeString(p[1])
	if e != nil || !hmac.Equal(sig, mac.Sum(nil)) {
		return "", false
	}
	raw, e := base64.RawURLEncoding.DecodeString(p[0])
	fields := strings.Split(string(raw), "\n")
	if e != nil || len(fields) != 2 {
		return "", false
	}
	expiry, e := strconv.ParseInt(fields[1], 10, 64)
	return fields[0], e == nil && time.Now().Unix() < expiry
}

func (a *API) frontend(w http.ResponseWriter, r *http.Request) {
	path := strings.TrimPrefix(filepath.Clean(r.URL.Path), "/")
	if path == "." || path == "" {
		path = "index.html"
	}
	root := os.Getenv(config.EnvUIDir)
	if root == "" {
		root = "/app/ui"
	}
	full := filepath.Join(root, path)
	if _, e := os.Stat(full); e != nil {
		full = filepath.Join(root, "index.html")
	}
	if t := mime.TypeByExtension(filepath.Ext(full)); t != "" {
		w.Header().Set("Content-Type", t)
	}
	http.ServeFile(w, r, full)
}

func decode(r *http.Request, v any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	return d.Decode(v)
}

func jsonResponse(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

func problem(w http.ResponseWriter, status int, code, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	jsonResponse(w, status, map[string]any{"status": status, "code": code, "detail": detail})
}

func respond(w http.ResponseWriter, v any, e error) {
	if e == nil {
		jsonResponse(w, 200, v)
		return
	}
	if store.NotFound(e) {
		problem(w, 404, "not_found", "Resource not found")
		return
	}
	problem(w, 500, "internal_error", e.Error())
}

func remoteIP(r *http.Request) string {
	h, _, e := net.SplitHostPort(r.RemoteAddr)
	if e == nil {
		return h
	}
	return r.RemoteAddr
}

func randomToken() string {
	b := make([]byte, 24)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

func groupAllowed(got, want []string) bool {
	if len(want) == 0 {
		return true
	}
	for _, a := range got {
		for _, b := range want {
			if a == b {
				return true
			}
		}
	}
	return false
}
