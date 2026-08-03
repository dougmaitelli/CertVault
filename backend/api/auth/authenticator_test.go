package auth

import (
	"net/http/httptest"
	"testing"

	"github.com/certvault/certvault/config"
)

func TestSessionRoundTrip(t *testing.T) {
	authenticator := &Authenticator{config: &config.Config{MasterKey: make([]byte, 32)}}
	recorder := httptest.NewRecorder()
	authenticator.setSession(recorder, "admin@example.com")

	response := recorder.Result()
	defer func() { _ = response.Body.Close() }()
	cookies := response.Cookies()
	if len(cookies) != 1 {
		t.Fatalf("cookies = %d, want 1", len(cookies))
	}
	name, ok := authenticator.verifySession(cookies[0].Value)
	if !ok || name != "admin@example.com" {
		t.Fatalf("verified session = %q, %v", name, ok)
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
