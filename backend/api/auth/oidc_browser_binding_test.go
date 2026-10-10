package auth

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/certvault/certvault/config"
	"github.com/certvault/certvault/database"
	"github.com/certvault/certvault/database/repository"
	"github.com/coreos/go-oidc/v3/oidc"
	"golang.org/x/oauth2"
)

func TestOIDCBindingCookieOptions(t *testing.T) {
	for _, secure := range []bool{false, true} {
		t.Run(fmt.Sprintf("secure=%t", secure), func(t *testing.T) {
			publicURL := "http://certvault.example"
			if secure {
				publicURL = "https://certvault.example"
			}

			a := NewBrowserAuthenticator(&config.Config{Server: config.Server{PublicURL: publicURL}, Auth: config.Auth{OIDC: &config.OIDC{}}}, nil, nil)
			t.Cleanup(a.states.close)
			a.oidc = &oidc.Provider{}
			a.oauth = &oauth2.Config{Endpoint: oauth2.Endpoint{AuthURL: "https://id.example/authorize"}}
			response := httptest.NewRecorder()
			a.Login(response, httptest.NewRequestWithContext(context.Background(), http.MethodGet, publicURL+"/auth/login", nil))

			if response.Code != http.StatusFound {
				t.Fatal("login did not redirect")
			}

			cookies := response.Result().Cookies()
			if len(cookies) != 1 {
				t.Fatal("missing browser binding cookie")
			}

			cookie := cookies[0]
			if !cookie.HttpOnly || cookie.Secure != secure || cookie.Path != "/" || cookie.Domain != "" || cookie.SameSite != http.SameSiteLaxMode || cookie.MaxAge != int(oidcStateLifetime.Seconds()) {
				t.Fatalf("incorrect cookie options: %#v", cookie)
			}

			if strings.HasPrefix(cookie.Name, "__Host-") != secure {
				t.Fatal("incorrect host cookie prefix")
			}

			if cookie.Expires.Before(time.Now().Add(oidcStateLifetime - time.Second)) {
				t.Fatal("incorrect cookie expiry")
			}
		})
	}
}

func TestOIDCCallbackRequiresInitiatingBrowser(t *testing.T) {
	issuer := "https://id.example.com"

	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}

	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	nonceByCode := map[string]string{}
	verifierByCode := map[string]string{}

	var exchanges atomic.Int64

	client := &http.Client{Transport: roundTripFunc(func(r *http.Request) (*http.Response, error) {
		var payload any

		switch r.URL.Path {
		case "/.well-known/openid-configuration":
			payload = map[string]any{"issuer": issuer, "authorization_endpoint": issuer + "/authorize", "token_endpoint": issuer + "/token", "jwks_uri": issuer + "/keys", "id_token_signing_alg_values_supported": []string{"RS256"}}
		case "/keys":
			payload = map[string]any{"keys": []any{map[string]any{"kty": "RSA", "kid": "test", "alg": "RS256", "use": "sig", "n": base64.RawURLEncoding.EncodeToString(key.N.Bytes()), "e": base64.RawURLEncoding.EncodeToString(big.NewInt(int64(key.E)).Bytes())}}}
		case "/token":
			exchanges.Add(1)

			if err := r.ParseForm(); err != nil {
				return nil, err
			}

			code := r.Form.Get("code")
			if r.Form.Get("code_verifier") != verifierByCode[code] || verifierByCode[code] == "" {
				return nil, fmt.Errorf("PKCE verifier mismatch")
			}

			header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"RS256","kid":"test"}`))

			claims, err := json.Marshal(map[string]any{"iss": issuer, "aud": "client", "exp": time.Now().Add(time.Hour).Unix(), "sub": "allowed-account", "email": "operator@example.com", "nonce": nonceByCode[code], "groups": []string{"operators"}})
			if err != nil {
				return nil, err
			}

			signingInput := header + "." + base64.RawURLEncoding.EncodeToString(claims)
			sum := sha256.Sum256([]byte(signingInput))

			signature, err := rsa.SignPKCS1v15(rand.Reader, key, crypto.SHA256, sum[:])
			if err != nil {
				return nil, err
			}

			payload = map[string]any{"access_token": "test-token", "token_type": "Bearer", "id_token": signingInput + "." + base64.RawURLEncoding.EncodeToString(signature)}
		default:
			return nil, fmt.Errorf("unexpected OIDC request: %s", r.URL.Path)
		}

		body, err := json.Marshal(payload)
		if err != nil {
			return nil, err
		}

		return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(string(body)))}, nil
	})}
	ctx := oidc.ClientContext(context.Background(), client)
	a := NewBrowserAuthenticator(&config.Config{MasterKey: make([]byte, 32), Server: config.Server{PublicURL: "https://certvault.example"}, Auth: config.Auth{OIDC: &config.OIDC{IssuerURL: issuer, ClientID: "client", Scopes: []string{oidc.ScopeOpenID}, AllowedGroups: []string{"operators"}}}}, repository.New(db), nil)
	t.Cleanup(a.states.close)

	login := func() (string, *http.Cookie) {
		t.Helper()

		response := httptest.NewRecorder()
		a.Login(response, httptest.NewRequestWithContext(ctx, http.MethodGet, "https://certvault.example/auth/login", nil))

		if response.Code != http.StatusFound {
			t.Fatalf("login: %d %s", response.Code, response.Body.String())
		}

		location, err := url.Parse(response.Header().Get("Location"))
		if err != nil {
			t.Fatal(err)
		}

		state := location.Query().Get("state")
		nonceByCode[state] = location.Query().Get("nonce")

		a.states.mu.Lock()
		verifierByCode[state] = a.states.flows[state].state.verifier
		a.states.mu.Unlock()

		return state, response.Result().Cookies()[0]
	}
	state, cookie := login()

	otherState, otherCookie := login()
	if cookie.Name == otherCookie.Name || cookie.Value == otherCookie.Value || cookie.Value == state {
		t.Fatal("flows did not receive independent browser secrets")
	}

	callback := func(state string, cookie *http.Cookie) *httptest.ResponseRecorder {
		r := httptest.NewRequestWithContext(ctx, http.MethodGet, "https://certvault.example/auth/callback?state="+state+"&code="+state, nil)
		if cookie != nil {
			r.AddCookie(cookie)
		}

		response := httptest.NewRecorder()
		a.Callback(response, r)

		return response
	}
	for _, wrongCookie := range []*http.Cookie{nil, otherCookie, {Name: cookie.Name, Value: otherCookie.Value}, {Name: cookie.Name, Value: state}, {Name: cookie.Name, Value: ""}} {
		response := callback(state, wrongCookie)
		if response.Code != http.StatusBadRequest {
			t.Fatalf("cross-browser callback: %d", response.Code)
		}

		if len(response.Result().Cookies()) != 0 {
			t.Fatal("rejected browser received cookies")
		}
	}

	if exchanges.Load() != 0 {
		t.Fatal("cross-browser callback reached token endpoint")
	}

	for _, flow := range []struct {
		state  string
		cookie *http.Cookie
	}{{state, cookie}, {otherState, otherCookie}} {
		response := callback(flow.state, flow.cookie)
		if response.Code != http.StatusFound {
			t.Fatalf("bound callback: %d %s", response.Code, response.Body.String())
		}

		var session *http.Cookie

		cleared := false

		for _, value := range response.Result().Cookies() {
			if value.Name == sessionCookie {
				session = value
			}

			if value.Name == flow.cookie.Name && value.MaxAge == -1 {
				cleared = true
			}
		}

		if session == nil || !cleared {
			t.Fatal("successful callback did not create session and clear flow cookie")
		}

		request := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/session", nil)
		request.AddCookie(session)

		identity, ok := a.AuthenticateSession(request)
		if !ok || identity.Name != "operator@example.com" {
			t.Fatal("OIDC session identity incorrect")
		}

		if response = callback(flow.state, flow.cookie); response.Code != http.StatusBadRequest {
			t.Fatal("flow replay accepted")
		}
	}

	if exchanges.Load() != 2 {
		t.Fatalf("exchanges: %d", exchanges.Load())
	}
}
