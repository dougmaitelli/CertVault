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
