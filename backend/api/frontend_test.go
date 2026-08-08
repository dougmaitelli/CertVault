package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/certvault/certvault/config"
)

func TestFrontendServesIndexForApplicationRoute(t *testing.T) {
	uiDir := t.TempDir()

	index := []byte("<!doctype html><title>CertVault</title>")
	if err := os.WriteFile(filepath.Join(uiDir, "index.html"), index, 0o600); err != nil {
		t.Fatal(err)
	}

	t.Setenv(config.EnvUIDir, uiDir)

	request := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		"/certificates",
		nil,
	)
	response := httptest.NewRecorder()

	(&API{}).frontend(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("frontend route returned %d", response.Code)
	}

	if response.Body.String() != string(index) {
		t.Fatalf("frontend route returned %q", response.Body.String())
	}
}

func TestFrontendDoesNotFollowSymlinksOutsideUIRoot(t *testing.T) {
	parent := t.TempDir()

	uiDir := filepath.Join(parent, "ui")
	if err := os.Mkdir(uiDir, 0o700); err != nil {
		t.Fatal(err)
	}

	index := []byte("safe index")
	if err := os.WriteFile(filepath.Join(uiDir, "index.html"), index, 0o600); err != nil {
		t.Fatal(err)
	}

	secretPath := filepath.Join(parent, "secret.txt")
	if err := os.WriteFile(secretPath, []byte("outside root"), 0o600); err != nil {
		t.Fatal(err)
	}

	if err := os.Symlink(secretPath, filepath.Join(uiDir, "escape.txt")); err != nil {
		t.Fatal(err)
	}

	t.Setenv(config.EnvUIDir, uiDir)

	request := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		"/escape.txt",
		nil,
	)
	response := httptest.NewRecorder()
	(&API{}).frontend(response, request)

	if response.Code != http.StatusOK || response.Body.String() != string(index) {
		t.Fatalf("escaped frontend response = %d %q", response.Code, response.Body.String())
	}
}

func TestHeadlessModeDoesNotRegisterUIOrBrowserAuthenticationRoutes(t *testing.T) {
	disabled := false
	handler := (&API{cfg: &config.Config{
		Server: config.Server{UIEnabled: &disabled},
	}}).routes()

	for _, route := range []string{"/", "/certificates", "/auth/methods", "/auth/login"} {
		request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, route, nil)
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, request)

		if response.Code != http.StatusNotFound {
			t.Errorf("headless route %s returned %d, want 404", route, response.Code)
		}
	}

	request := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/api/v1/health", nil)
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)

	if response.Code != http.StatusOK {
		t.Fatalf("headless health route returned %d", response.Code)
	}
}
