package auth

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
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
	oidcStateLifetime              = 10 * time.Minute
	authMethodBootstrap            = "bootstrap"
	authMethodOIDC                 = "oidc"
)

func NewBrowserAuthenticator(
	cfg *config.Config,
	repos *repository.Repositories,
	clientIPs *certnetwork.ClientIPResolver,
) (*BrowserAuthenticator, error) {
	browserAuthenticator := &BrowserAuthenticator{
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

		browserAuthenticator.bootstrap = strings.TrimSpace(string(contents))
	}

	if cfg.Auth.OIDC != nil {
		secret := cfg.Auth.OIDC.ClientSecret
		if cfg.Auth.OIDC.ClientSecretFile != "" {
			contents, readErr := os.ReadFile(cfg.Auth.OIDC.ClientSecretFile)
			if readErr != nil {
				return nil, readErr
			}

			secret = strings.TrimSpace(string(contents))
		}

		browserAuthenticator.oidcSecret = secret
	}

	return browserAuthenticator, nil
}

func (a *BrowserAuthenticator) oidcClient(ctx context.Context) (*oidc.Provider, *oauth2.Config, error) {
	if a.config.Auth.OIDC == nil {
		return nil, nil, errors.New("OIDC is not configured")
	}

	a.oidcMu.Lock()
	defer a.oidcMu.Unlock()

	if a.oidc != nil && a.oauth != nil {
		return a.oidc, a.oauth, nil
	}

	provider, err := oidc.NewProvider(ctx, a.config.Auth.OIDC.IssuerURL)
	if err != nil {
		return nil, nil, fmt.Errorf("OIDC discovery: %w", err)
	}

	a.oidc = provider
	a.oauth = &oauth2.Config{
		ClientID:     a.config.Auth.OIDC.ClientID,
		ClientSecret: a.oidcSecret,
		Endpoint:     provider.Endpoint(),
		RedirectURL:  a.config.OIDCRedirectURL(),
		Scopes:       []string{oidc.ScopeOpenID, "profile", "email", "groups"},
	}

	return a.oidc, a.oauth, nil
}

func (a *BrowserAuthenticator) AuthenticateSession(r *http.Request) (Identity, bool) {
	cookie, err := r.Cookie(sessionCookie)
	if err != nil {
		return Identity{}, false
	}

	return a.verifySession(cookie.Value)
}

func (a *BrowserAuthenticator) BootstrapLogin(w http.ResponseWriter, r *http.Request) {
	var input bootstrapLoginRequest
	if decode(r, &input) != nil || a.bootstrap == "" || !hmac.Equal([]byte(input.Token), []byte(a.bootstrap)) {
		problem(w, http.StatusUnauthorized, "unauthorized", "Invalid bootstrap token")
		return
	}

	a.setSession(w, sessionPayload{
		Name:                 "bootstrap-admin",
		DisplayName:          "Bootstrap administrator",
		AuthenticationMethod: authMethodBootstrap,
	})
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

func (a *BrowserAuthenticator) Login(w http.ResponseWriter, r *http.Request) {
	if a.config.Auth.OIDC == nil {
		problem(w, http.StatusNotFound, "oidc_disabled", "OIDC is not configured")
		return
	}

	_, oauth, err := a.oidcClient(r.Context())
	if err != nil {
		problem(w, http.StatusServiceUnavailable, "oidc_unavailable", err.Error())
		return
	}

	state := randomToken()
	nonce := randomToken()
	verifier := oauth2.GenerateVerifier()
	a.states.Store(state, oidcState{nonce: nonce, verifier: verifier, at: time.Now()})
	http.Redirect(
		w,
		r,
		oauth.AuthCodeURL(state, oidc.Nonce(nonce), oauth2.S256ChallengeOption(verifier)),
		http.StatusFound,
	)
}

func (a *BrowserAuthenticator) Callback(w http.ResponseWriter, r *http.Request) {
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

	provider, oauth, err := a.oidcClient(r.Context())
	if err != nil {
		problem(w, http.StatusServiceUnavailable, "oidc_unavailable", err.Error())
		return
	}

	token, err := oauth.Exchange(r.Context(), r.URL.Query().Get("code"), oauth2.VerifierOption(state.verifier))
	if err != nil {
		problem(w, http.StatusUnauthorized, "oidc_error", err.Error())
		return
	}

	rawIDToken, _ := token.Extra("id_token").(string)

	verified, err := provider.Verifier(&oidc.Config{ClientID: oauth.ClientID}).Verify(r.Context(), rawIDToken)
	if err != nil || verified.Nonce != state.nonce {
		problem(w, http.StatusUnauthorized, "oidc_error", "ID token verification failed")
		return
	}

	var claims oidcClaims
	if verified.Claims(&claims) != nil || !groupAllowed(claims.Groups, a.config.Auth.OIDC.AllowedGroups) {
		problem(w, http.StatusForbidden, "forbidden", "OIDC group is not allowed")
		return
	}

	actor := claims.Email
	if actor == "" {
		actor = claims.Sub
	}

	displayName := claims.Name
	if displayName == "" {
		displayName = actor
	}

	a.setSession(w, sessionPayload{
		Name:                 actor,
		DisplayName:          displayName,
		Email:                claims.Email,
		Picture:              claims.Picture,
		AuthenticationMethod: authMethodOIDC,
	})
	a.repos.Audits.Record(r.Context(), actor, "auth.login", "ui", authMethodOIDC, a.remoteIP(r))
	http.Redirect(w, r, "/", http.StatusFound)
}

func (a *BrowserAuthenticator) Logout(w http.ResponseWriter, _ *http.Request) {
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

func WithIdentity(r *http.Request, identity Identity) *http.Request {
	return r.WithContext(context.WithValue(r.Context(), identityKey, identity))
}

func (a *BrowserAuthenticator) setSession(w http.ResponseWriter, session sessionPayload) {
	sessionDuration := a.config.SessionDuration()
	session.ExpiresAt = time.Now().Add(sessionDuration).Unix()
	contents, _ := json.Marshal(session)
	payload := base64.RawURLEncoding.EncodeToString(contents)
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

func (a *BrowserAuthenticator) verifySession(value string) (Identity, bool) {
	parts := strings.Split(value, ".")
	if len(parts) != 2 {
		return Identity{}, false
	}

	mac := hmac.New(sha256.New, a.config.MasterKey)
	_, _ = mac.Write([]byte(parts[0]))

	signature, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil || !hmac.Equal(signature, mac.Sum(nil)) {
		return Identity{}, false
	}

	raw, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return Identity{}, false
	}

	var session sessionPayload
	if err = json.Unmarshal(raw, &session); err != nil || time.Now().Unix() >= session.ExpiresAt {
		return Identity{}, false
	}

	return Identity{
		Admin:                true,
		Name:                 session.Name,
		DisplayName:          session.DisplayName,
		Email:                session.Email,
		Picture:              session.Picture,
		AuthenticationMethod: session.AuthenticationMethod,
	}, true
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

func (a *BrowserAuthenticator) remoteIP(r *http.Request) string {
	return a.clientIPs.ClientIP(r)
}
