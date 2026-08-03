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

func New(c *config.Config, db *store.Store, m *service.Manager, log *slog.Logger) (http.Handler, error) {
	a := &API{cfg: c, db: db, manager: m, log: log}
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

func (a *API) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/v1/health", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("GET /api/v1/ready", func(w http.ResponseWriter, _ *http.Request) {
		jsonResponse(w, http.StatusOK, map[string]string{"status": "ready"})
	})
	mux.HandleFunc("GET /auth/login", a.login)
	mux.HandleFunc("GET /auth/callback", a.callback)
	mux.HandleFunc("POST /auth/bootstrap", a.bootstrapLogin)
	mux.HandleFunc("POST /auth/logout", a.logout)
	mux.Handle("/api/", a.auth(http.HandlerFunc(a.api)))
	mux.HandleFunc("/", a.frontend)
	return a.security(mux)
}

func (a *API) security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy", contentSecurityPolicy)
		next.ServeHTTP(w, r)
	})
}

func (a *API) auth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if c, e := r.Cookie("cv_session"); e == nil {
			if name, ok := a.verifySession(c.Value); ok {
				next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey, identity{admin: true, name: name})))
				return
			}
		}
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token != "" {
			p, e := a.db.Authenticate(r.Context(), token, remoteIP(r))
			if e == nil {
				next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), principalKey, identity{name: p.Name, principal: p})))
				return
			}
		}
		problem(w, 401, "unauthorized", "Authentication is required")
	})
}

func (a *API) api(w http.ResponseWriter, r *http.Request) {
	id, ok := r.Context().Value(principalKey).(identity)
	if !ok {
		problem(w, http.StatusInternalServerError, "missing_identity", "Authenticated identity is unavailable")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/api/v1/")
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if path == "session" && r.Method == "GET" {
		jsonResponse(w, 200, map[string]any{"name": id.name, "admin": id.admin})
		return
	}
	if parts[0] == "certificates" {
		a.certificates(w, r, id, parts)
		return
	}
	if parts[0] == "jobs" && r.Method == "GET" {
		if !a.allow(id, "certificates:read", "") {
			problem(w, 403, "forbidden", "Missing scope")
			return
		}
		v, e := a.db.Jobs(r.Context(), 100)
		respond(w, v, e)
		return
	}
	if parts[0] == "api-keys" {
		if !id.admin {
			problem(w, 403, "forbidden", "Administrator access required")
			return
		}
		a.keys(w, r, parts)
		return
	}
	if parts[0] == "audit" && r.Method == "GET" {
		if !id.admin {
			problem(w, 403, "forbidden", "Administrator access required")
			return
		}
		v, e := a.db.Audits(r.Context(), 200)
		respond(w, v, e)
		return
	}
	problem(w, 404, "not_found", "Resource not found")
}

func (a *API) certificates(w http.ResponseWriter, r *http.Request, id identity, p []string) {
	if len(p) == 1 && r.Method == "GET" {
		if !a.allow(id, "certificates:read", "") {
			problem(w, 403, "forbidden", "Missing scope")
			return
		}
		v, e := a.db.ListCertificates(r.Context())
		if !id.admin {
			filtered := v[:0]
			for _, c := range v {
				if id.principal.Allows("certificates:read", c.Name) {
					filtered = append(filtered, c)
				}
			}
			v = filtered
		}
		respond(w, v, e)
		return
	}
	if len(p) < 2 {
		problem(w, 404, "not_found", "Resource not found")
		return
	}
	name := p[1]
	if len(p) == 2 && r.Method == "GET" {
		if !a.allow(id, "certificates:read", name) {
			problem(w, 403, "forbidden", "Missing scope")
			return
		}
		v, e := a.db.Certificate(r.Context(), name)
		respond(w, v, e)
		return
	}
	if len(p) == 3 && p[2] == "versions" && r.Method == "GET" {
		if !a.allow(id, "certificates:read", name) {
			problem(w, 403, "forbidden", "Missing scope")
			return
		}
		v, e := a.db.Versions(r.Context(), name)
		respond(w, v, e)
		return
	}
	if len(p) == 3 && p[2] == "renew" && r.Method == "POST" {
		if !a.allow(id, "renewals:trigger", name) {
			problem(w, 403, "forbidden", "Missing scope")
			return
		}
		go func() { _ = a.manager.Issue(context.Background(), name, "manual") }()
		a.db.Audit(r.Context(), id.name, "renewal.trigger", name, "", remoteIP(r))
		jsonResponse(w, 202, map[string]string{"status": "queued"})
		return
	}
	if len(p) == 3 && r.Method == "GET" {
		file := p[2]
		if file != "certificate.pem" && file != "chain.pem" && file != "fullchain.pem" && file != "private-key.pem" {
			problem(w, 404, "not_found", "Resource not found")
			return
		}
		scope := "certificates:read"
		if file == "private-key.pem" {
			scope = "private_keys:read"
		}
		if !a.allow(id, scope, name) {
			problem(w, 403, "forbidden", "Missing scope")
			return
		}
		v, e := a.db.CurrentVersion(r.Context(), name)
		if e != nil {
			respond(w, nil, e)
			return
		}
		b, e := a.manager.ReadFile(v, file)
		if e != nil {
			problem(w, 500, "storage_error", e.Error())
			return
		}
		sum := sha256.Sum256(b)
		etag := fmt.Sprintf("\"%x\"", sum)
		if r.Header.Get("If-None-Match") == etag {
			w.WriteHeader(304)
			return
		}
		w.Header().Set("ETag", etag)
		w.Header().Set("Content-Type", "application/x-pem-file")
		w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name+"-"+file))
		_, _ = w.Write(b)
		a.db.Audit(r.Context(), id.name, "certificate.download", name, file, remoteIP(r))
		return
	}
	problem(w, 404, "not_found", "Resource not found")
}

func (a *API) keys(w http.ResponseWriter, r *http.Request, p []string) {
	if len(p) == 1 && r.Method == "GET" {
		v, e := a.db.ListAPIKeys(r.Context())
		respond(w, v, e)
		return
	}
	if len(p) == 1 && r.Method == "POST" {
		var in createAPIKeyRequest
		if e := decode(r, &in); e != nil {
			problem(w, 400, "invalid_request", e.Error())
			return
		}
		if in.Name == "" || len(in.Scopes) == 0 || len(in.Certificates) == 0 {
			problem(w, 400, "invalid_request", "name, scopes, and certificates are required")
			return
		}
		key, token, e := a.db.CreateAPIKey(r.Context(), in.Name, in.Scopes, in.Certificates, in.ExpiresAt)
		if e != nil {
			problem(w, 500, "database_error", e.Error())
			return
		}
		a.db.Audit(r.Context(), "admin", "api_key.create", strconv.FormatInt(key.ID, 10), in.Name, remoteIP(r))
		jsonResponse(w, 201, map[string]any{"api_key": key, "token": token})
		return
	}
	if len(p) == 2 && r.Method == "DELETE" {
		id, e := strconv.ParseInt(p[1], 10, 64)
		if e == nil {
			e = a.db.RevokeAPIKey(r.Context(), id)
		}
		if e != nil {
			respond(w, nil, e)
			return
		}
		a.db.Audit(r.Context(), "admin", "api_key.revoke", p[1], "", remoteIP(r))
		w.WriteHeader(204)
		return
	}
	problem(w, 404, "not_found", "Resource not found")
}

func (a *API) allow(id identity, scope, cert string) bool {
	return id.admin || id.principal.Allows(scope, cert)
}

func (a *API) bootstrapLogin(w http.ResponseWriter, r *http.Request) {
	var in bootstrapLoginRequest
	if decode(r, &in) != nil || a.bootstrap == "" || !hmac.Equal([]byte(in.Token), []byte(a.bootstrap)) {
		problem(w, 401, "unauthorized", "Invalid bootstrap token")
		return
	}
	a.setSession(w, "bootstrap-admin")
	a.db.Audit(r.Context(), "bootstrap-admin", "auth.login", "ui", "", remoteIP(r))
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
	a.db.Audit(r.Context(), name, "auth.login", "ui", "oidc", remoteIP(r))
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
