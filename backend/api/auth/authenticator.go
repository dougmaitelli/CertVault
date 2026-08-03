package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/certvault/certvault/config"
	"github.com/certvault/certvault/database/repository"
	certnetwork "github.com/certvault/certvault/network"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

const (
	identityKey         contextKey = "identity"
	sessionCookie                  = "cv_session"
	sessionDuration                = 12 * time.Hour
	oidcStateLifetime              = 10 * time.Minute
	authMethodBootstrap            = "bootstrap"
	authMethodOIDC                 = "oidc"
)

func New(
	cfg *config.Config,
	repos *repository.Repositories,
	clientIPs *certnetwork.ClientIPResolver,
) (*Authenticator, error) {
	authenticator := &Authenticator{
		config:    cfg,
		repos:     repos,
		bootstrap: cfg.Auth.BootstrapToken,
		clientIPs: clientIPs,
	}
	if cfg.Auth.BootstrapTokenFile != "" {
		contents, err := os.ReadFile(cfg.Auth.BootstrapTokenFile)
		if err != nil {
			return nil, err
		}
		authenticator.bootstrap = strings.TrimSpace(string(contents))
	}
	if cfg.Auth.OIDC != nil {
		provider, err := oidc.NewProvider(context.Background(), cfg.Auth.OIDC.IssuerURL)
		if err != nil {
			return nil, fmt.Errorf("OIDC discovery: %w", err)
		}
		secret := cfg.Auth.OIDC.ClientSecret
		if cfg.Auth.OIDC.ClientSecretFile != "" {
			contents, readErr := os.ReadFile(cfg.Auth.OIDC.ClientSecretFile)
			if readErr != nil {
				return nil, readErr
			}
			secret = strings.TrimSpace(string(contents))
		}
		authenticator.oidc = provider
		authenticator.oauth = &oauth2.Config{
			ClientID:     cfg.Auth.OIDC.ClientID,
			ClientSecret: secret,
			Endpoint:     provider.Endpoint(),
			RedirectURL:  cfg.OIDCRedirectURL(),
			Scopes:       []string{oidc.ScopeOpenID, "profile", "email", "groups"},
		}
	}
	return authenticator, nil
}

func (a *Authenticator) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if cookie, err := r.Cookie(sessionCookie); err == nil {
			if name, ok := a.verifySession(cookie.Value); ok {
				next.ServeHTTP(w, withIdentity(r, Identity{Admin: true, Name: name}))
				return
			}
		}
		token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
		if token != "" {
			principal, err := a.repos.APIKeys.Authenticate(r.Context(), token, a.remoteIP(r))
			if err == nil {
				next.ServeHTTP(w, withIdentity(r, Identity{Name: principal.Name, Principal: principal}))
				return
			}
		}
		problem(w, http.StatusUnauthorized, "unauthorized", "Authentication is required")
	})
}

func (a *Authenticator) BootstrapLogin(w http.ResponseWriter, r *http.Request) {
	var input bootstrapLoginRequest
	if decode(r, &input) != nil || a.bootstrap == "" || !hmac.Equal([]byte(input.Token), []byte(a.bootstrap)) {
		problem(w, http.StatusUnauthorized, "unauthorized", "Invalid bootstrap token")
		return
	}
	a.setSession(w, "bootstrap-admin")
	a.repos.Audits.Record(
		r.Context(),
		"bootstrap-admin",
		"auth.login",
		"ui",
		authMethodBootstrap,
		a.remoteIP(r),
	)
	w.WriteHeader(http.StatusNoContent)
}

func (a *Authenticator) Login(w http.ResponseWriter, r *http.Request) {
	if a.oauth == nil {
		problem(w, http.StatusNotFound, "oidc_disabled", "OIDC is not configured")
		return
	}
	state := randomToken()
	nonce := randomToken()
	verifier := oauth2.GenerateVerifier()
	a.states.Store(state, oidcState{nonce: nonce, verifier: verifier, at: time.Now()})
	http.Redirect(
		w,
		r,
		a.oauth.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier)),
		http.StatusFound,
	)
}

func (a *Authenticator) Callback(w http.ResponseWriter, r *http.Request) {
	value, ok := a.states.LoadAndDelete(r.URL.Query().Get("state"))
	if !ok {
		problem(w, http.StatusBadRequest, "invalid_state", "OIDC state is invalid")
		return
	}
	state, ok := value.(oidcState)
	if !ok {
		problem(w, http.StatusBadRequest, "invalid_state", "OIDC state is invalid")
		return
	}
	if time.Since(state.at) > oidcStateLifetime {
		problem(w, http.StatusBadRequest, "expired_state", "OIDC state expired")
		return
	}
	token, err := a.oauth.Exchange(r.Context(), r.URL.Query().Get("code"), oauth2.VerifierOption(state.verifier))
	if err != nil {
		problem(w, http.StatusUnauthorized, "oidc_error", err.Error())
		return
	}
	rawIDToken, _ := token.Extra("id_token").(string)
	verified, err := a.oidc.Verifier(&oidc.Config{ClientID: a.oauth.ClientID}).Verify(r.Context(), rawIDToken)
	if err != nil || verified.Nonce != state.nonce {
		problem(w, http.StatusUnauthorized, "oidc_error", "ID token verification failed")
		return
	}
	var claims oidcClaims
	if verified.Claims(&claims) != nil || !groupAllowed(claims.Groups, a.config.Auth.OIDC.AllowedGroups) {
		problem(w, http.StatusForbidden, "forbidden", "OIDC group is not allowed")
		return
	}
	name := claims.Email
	if name == "" {
		name = claims.Sub
	}
	a.setSession(w, name)
	a.repos.Audits.Record(r.Context(), name, "auth.login", "ui", authMethodOIDC, a.remoteIP(r))
	http.Redirect(w, r, "/", http.StatusFound)
}

func (a *Authenticator) Logout(w http.ResponseWriter, _ *http.Request) {
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   strings.HasPrefix(a.config.Server.PublicURL, "https://"),
		SameSite: http.SameSiteStrictMode,
	})
	w.WriteHeader(http.StatusNoContent)
}

func IdentityFromContext(ctx context.Context) (Identity, bool) {
	identity, ok := ctx.Value(identityKey).(Identity)
	return identity, ok
}

func withIdentity(r *http.Request, identity Identity) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), identityKey, identity))
}

func (a *Authenticator) setSession(w http.ResponseWriter, name string) {
	expiresAt := time.Now().Add(sessionDuration)
	payload := base64.RawURLEncoding.EncodeToString(
		[]byte(name + "\n" + strconv.FormatInt(expiresAt.Unix(), 10)),
	)
	mac := hmac.New(sha256.New, a.config.MasterKey)
	_, _ = mac.Write([]byte(payload))
	value := payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	http.SetCookie(w, &http.Cookie{
		Name:     sessionCookie,
		Value:    value,
		Path:     "/",
		MaxAge:   int(sessionDuration.Seconds()),
		HttpOnly: true,
		Secure:   strings.HasPrefix(a.config.Server.PublicURL, "https://"),
		SameSite: http.SameSiteStrictMode,
	})
}

func (a *Authenticator) verifySession(value string) (string, bool) {
	parts := strings.Split(value, ".")
	if len(parts) != 2 {
		return "", false
	}
	mac := hmac.New(sha256.New, a.config.MasterKey)
	_, _ = mac.Write([]byte(parts[0]))
	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(signature, mac.Sum(nil)) {
		return "", false
	}
	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	fields := strings.Split(string(raw), "\n")
	if err != nil || len(fields) != 2 {
		return "", false
	}
	expiry, err := strconv.ParseInt(fields[1], 10, 64)
	return fields[0], err == nil && time.Now().Unix() < expiry
}

func decode(r *http.Request, value any) error {
	r.Body = http.MaxBytesReader(nil, r.Body, 1<<20)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	return decoder.Decode(value)
}

func problem(w http.ResponseWriter, status int, code, detail string) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{"status": status, "code": code, "detail": detail})
}

func randomToken() string {
	value := make([]byte, 24)
	_, _ = rand.Read(value)
	return base64.RawURLEncoding.EncodeToString(value)
}

func groupAllowed(got, wanted []string) bool {
	if len(wanted) == 0 {
		return true
	}
	for _, actual := range got {
		for _, allowed := range wanted {
			if actual == allowed {
				return true
			}
		}
	}
	return false
}

func (a *Authenticator) remoteIP(r *http.Request) string {
	return a.clientIPs.ClientIP(r)
}
