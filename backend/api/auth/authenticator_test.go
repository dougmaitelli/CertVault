package auth

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/certvault/certvault/config"
	"github.com/certvault/certvault/database"
	"github.com/certvault/certvault/database/repository"
	certnetwork "github.com/certvault/certvault/network"
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
	authenticator, err := New(
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
