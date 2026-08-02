package api

import (
	"context"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/certvault/certvault/config"
	"github.com/certvault/certvault/service"
	"github.com/certvault/certvault/store"
)

func TestHealthAndScopedCertificateList(t *testing.T) {
	dir := t.TempDir()
	cfg := &config.Config{
		DataDir:   dir,
		MasterKey: make([]byte, 32),
		ACME: config.ACME{
			Email:       "test@example.com",
			AcceptTerms: true,
		},
		Certificates: []config.Certificate{
			{
				Name:    "home",
				Domains: []string{"example.com"},
				KeyType: "ec256",
			},
		},
	}
	db, err := store.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	if err = db.Reconcile(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	testLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	manager, err := service.NewManager(cfg, db, testLogger)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := New(cfg, db, manager, slog.Default())
	if err != nil {
		t.Fatal(err)
	}

	health := httptest.NewRecorder()
	healthRequest := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		"/api/v1/health",
		nil,
	)
	handler.ServeHTTP(health, healthRequest)
	if health.Code != http.StatusOK {
		t.Fatalf("health returned %d", health.Code)
	}

	_, token, err := db.CreateAPIKey(context.Background(), "node", []string{"certificates:read"}, []string{"home"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	req := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		"/api/v1/certificates",
		nil,
	)
	req.Header.Set("Authorization", "Bearer "+token)
	list := httptest.NewRecorder()
	handler.ServeHTTP(list, req)
	if list.Code != http.StatusOK {
		t.Fatalf("list returned %d: %s", list.Code, list.Body.String())
	}
}
