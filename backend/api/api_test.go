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
	"github.com/certvault/certvault/database"
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
				KeyType: config.KeyTypeEC256,
			},
		},
	}
	db, err := database.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()
	repositories := store.New(db)
	if err = repositories.Certificates.Reconcile(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	testLogger := slog.New(slog.NewTextHandler(io.Discard, nil))
	manager, err := service.NewManager(cfg, repositories, testLogger)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := New(cfg, repositories, manager)
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

	_, token, err := repositories.APIKeys.Create(context.Background(), "node", []string{"certificates:read"}, []string{"home"}, nil)
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

	wrongMethodRequest := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodPost,
		"/api/v1/certificates",
		nil,
	)
	wrongMethodRequest.Header.Set("Authorization", "Bearer "+token)
	wrongMethod := httptest.NewRecorder()
	handler.ServeHTTP(wrongMethod, wrongMethodRequest)
	if wrongMethod.Code != http.StatusMethodNotAllowed {
		t.Fatalf("wrong method returned %d", wrongMethod.Code)
	}

	unknownRequest := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		"/api/v1/unknown",
		nil,
	)
	unknownRequest.Header.Set("Authorization", "Bearer "+token)
	unknown := httptest.NewRecorder()
	handler.ServeHTTP(unknown, unknownRequest)
	if unknown.Code != http.StatusNotFound {
		t.Fatalf("unknown API route returned %d", unknown.Code)
	}

	privateKeyRequest := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		"/api/v1/certificates/home/private-key.pem",
		nil,
	)
	privateKeyRequest.Header.Set("Authorization", "Bearer "+token)
	privateKey := httptest.NewRecorder()
	handler.ServeHTTP(privateKey, privateKeyRequest)
	if privateKey.Code != http.StatusForbidden {
		t.Fatalf("private key without scope returned %d", privateKey.Code)
	}

	adminRequest := httptest.NewRequestWithContext(
		context.Background(),
		http.MethodGet,
		"/api/v1/api-keys",
		nil,
	)
	adminRequest.Header.Set("Authorization", "Bearer "+token)
	admin := httptest.NewRecorder()
	handler.ServeHTTP(admin, adminRequest)
	if admin.Code != http.StatusForbidden {
		t.Fatalf("admin route with machine key returned %d", admin.Code)
	}
}
