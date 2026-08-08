package auth

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/certvault/certvault/config"
	"github.com/certvault/certvault/database"
	"github.com/certvault/certvault/database/repository"
	certnetwork "github.com/certvault/certvault/network"
	"github.com/coreos/go-oidc/v3/oidc"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func TestSessionRoundTrip(t *testing.T) {
	authenticator := &BrowserAuthenticator{config: &config.Config{MasterKey: make([]byte, 32)}}
	recorder := httptest.NewRecorder()
	authenticator.setSession(recorder, sessionPayload{
		Name:                 "admin@example.com",
		DisplayName:          "Certificate Admin",
		Email:                "admin@example.com",
		Picture:              "https://id.example.com/avatar.png",
		AuthenticationMethod: authMethodOIDC,
	})

	response := recorder.Result()
	defer func() { _ = response.Body.Close() }()
	cookies := response.Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d, want 1", len(cookies))
	}
	if cookies[0].MaxAge != int(config.DefaultSessionDuration.Seconds()) {
		t.Fatalf("cookie max age = %d, want %d", cookies[0].MaxAge, int(config.DefaultSessionDuration.Seconds()))
	}
	request := httptest.NewRequest(http.MethodGet, "/api/v1/session", nil)
	request.AddCookie(cookies[0])
	identity, ok := authenticator.AuthenticateSession(request)
	if !ok || identity.Name != "admin@example.com" ||
		identity.DisplayName != "Certificate Admin" ||
		identity.Picture != "https://id.example.com/avatar.png" ||
		identity.AuthenticationMethod != authMethodOIDC {
		t.Fatalf("verified session = %#v, %v", identity, ok)
	}
}

func TestSessionUsesConfiguredDuration(t *testing.T) {
	authenticator := &BrowserAuthenticator{config: &config.Config{
		MasterKey: make([]byte, 32),
		Auth:      config.Auth{SessionDuration: config.Duration{Duration: 2 * time.Hour}},
	}}
	recorder := httptest.NewRecorder()
	authenticator.setSession(recorder, sessionPayload{Name: "admin@example.com"})

	cookies := recorder.Result().Cookies()
	if len(cookies) != 1 || cookies[0].MaxAge != int((2*time.Hour).Seconds()) {
		t.Fatalf("cookies = %#v, want a two-hour max age", cookies)
	}
}

func TestGroupAllowed(t *testing.T) {
	if !groupAllowed([]string{"operators"}, nil) {
		t.Fatal("empty allowlist rejected a group")
	}
	if !groupAllowed([]string{"users", "operators"}, []string{"operators"}) {
		t.Fatal("matching group was rejected")
	}
	if groupAllowed([]string{"users"}, []string{"operators"}) {
		t.Fatal("non-matching group was accepted")
	}
}

func TestOIDCDiscoveryIsLazyAndRetryable(t *testing.T) {
	var available atomic.Bool
	var discoveries atomic.Int32
	issuer := "https://id.example.com"
	client := &http.Client{Transport: roundTripFunc(func(_ *http.Request) (*http.Response, error) {
		discoveries.Add(1)
		if !available.Load() {
			return &http.Response{
				StatusCode: http.StatusServiceUnavailable,
				Body:       io.NopCloser(strings.NewReader("unavailable")),
				Header:     make(http.Header),
			}, nil
		}
		contents, _ := json.Marshal(map[string]any{
			"issuer":                 issuer,
			"authorization_endpoint": issuer + "/authorize",
			"token_endpoint":         issuer + "/token",
			"jwks_uri":               issuer + "/keys",
		})
		return &http.Response{
			StatusCode: http.StatusOK,
			Body:       io.NopCloser(strings.NewReader(string(contents))),
			Header:     make(http.Header),
		}, nil
	})}

	authenticator, err := NewBrowserAuthenticator(
		&config.Config{
			Server: config.Server{PublicURL: "https://certvault.example.com"},
			Auth: config.Auth{OIDC: &config.OIDC{
				IssuerURL:    issuer,
				ClientID:     "certvault",
				ClientSecret: "secret",
			}},
		},
		nil,
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if discoveries.Load() != 0 {
		t.Fatal("OIDC discovery ran during authenticator initialization")
	}

	loginRequest := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/auth/login", nil)
	loginRequest = loginRequest.WithContext(oidc.ClientContext(loginRequest.Context(), client))
	response := httptest.NewRecorder()
	authenticator.Login(response, loginRequest)
	if response.Code != http.StatusServiceUnavailable {
		t.Fatalf("unavailable OIDC login returned %d", response.Code)
	}

	available.Store(true)
	response = httptest.NewRecorder()
	authenticator.Login(response, loginRequest)
	if response.Code != http.StatusFound {
		t.Fatalf("recovered OIDC login returned %d: %s", response.Code, response.Body.String())
	}
}

func TestBootstrapLoginRecordsAuthenticationMethod(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	repositories := repository.New(db)
	clientIPs, err := certnetwork.NewClientIPResolver(nil)
	if err != nil {
		t.Fatal(err)
	}
	authenticator, err := NewBrowserAuthenticator(
		&config.Config{
			MasterKey: make([]byte, 32),
			Auth:      config.Auth{BootstrapToken: "secret"},
		},
		repositories,
		clientIPs,
	)
	if err != nil {
		t.Fatal(err)
	}

	request := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		"/auth/bootstrap",
		strings.NewReader(`{"token":"secret"}`),
	)
	response := httptest.NewRecorder()
	authenticator.BootstrapLogin(response, request)
	if response.Code != http.StatusNoContent {
		t.Fatalf("bootstrap login returned %d", response.Code)
	}

	audits, err := repositories.Audits.List(context.Background(), 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(audits) != 1 || audits[0].Detail != authMethodBootstrap {
		t.Fatalf("bootstrap audit events = %#v", audits)
	}
}
