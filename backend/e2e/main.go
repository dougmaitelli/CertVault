//go:build e2e

// This standalone harness is excluded from production builds. It uses the real
// HTTP handler and repositories with disposable data and no external services.
package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/certvault/certvault/api"
	"github.com/certvault/certvault/audit"
	"github.com/certvault/certvault/config"
	"github.com/certvault/certvault/database"
	"github.com/certvault/certvault/database/repository"
	"github.com/certvault/certvault/service"
	"github.com/certvault/certvault/vault"
)

var fixed = time.Date(2026, 8, 16, 12, 0, 0, 0, time.UTC)

func must(err error) {
	if err != nil {
		panic(err)
	}
}

func seed(db *database.Database, cfg *config.Config, repos *repository.Repositories) {
	ctx := context.Background()
	must(repos.Certificates.Reconcile(ctx, cfg))
	for i, name := range []string{"homelab-wildcard", "monitoring-services"} {
		for v := 0; v < 2; v++ {
			created := fixed.AddDate(0, 0, -98+v*90)
			must(repos.Certificates.AddVersion(ctx, repository.Version{
				CertificateName: name, Path: "visual-fixture", Domains: cfg.Certificates[i].Domains,
				Serial: fmt.Sprintf("%d%d048AB270", i, v), Issuer: "CN=CertVault example CA",
				FingerprintSHA256: "7a2b9403c81e6f39ae049bec9589407d1520f18c43deaa7d309fefc612a8e754",
				NotBefore:         created, NotAfter: created.AddDate(0, 0, 90), CreatedAt: created,
			}))
		}
	}
	// Enough rows to exercise real pagination and filter metadata.
	for i := 1; i <= 12; i++ {
		at := fixed.Add(-time.Duration(i) * 24 * time.Hour)
		status, detail := "succeeded", ""
		if i == 2 {
			status, detail = "failed", "DNS challenge timed out"
		}
		must(db.ORM().Create(&database.Job{ID: int64(i), CertificateID: 1, Kind: "manual", Status: status, Error: detail, StartedAt: at, FinishedAt: &at}).Error)
	}

	events := []database.AuditEvent{
		{Actor: "bootstrap-admin", Action: string(audit.ActionAuthLogin), Resource: audit.ResourceUI, Detail: audit.DetailAuthBootstrap, IP: "192.0.2.10"},
		{Actor: "admin@example.com", Action: string(audit.ActionAuthLogin), Resource: audit.ResourceUI, Detail: audit.DetailAuthOIDC, IP: "192.0.2.24"},
		{Actor: "admin", Action: string(audit.ActionAPIKeyCreate), Resource: "Traefik deployment", IP: "192.0.2.24"},
		{Actor: "system", Action: string(audit.ActionCertificateInitial), Resource: "monitoring-services"},
		{Actor: "Traefik deployment", Action: string(audit.ActionCertificateDownload), Resource: "homelab-wildcard", Detail: "fullchain.crt", IP: "192.0.2.80"},
		{Actor: "Traefik deployment", Action: string(audit.ActionCertificateDownload), Resource: "homelab-wildcard", Detail: "private.key", IP: "192.0.2.80"},
		{Actor: "admin@example.com", Action: string(audit.ActionRenewalTrigger), Resource: "homelab-wildcard", IP: "192.0.2.24"},
		{Actor: "system", Action: string(audit.ActionCertificateManual), Resource: "homelab-wildcard"},
		{Actor: "system", Action: string(audit.ActionCertificateScheduled), Resource: "monitoring-services"},
		{Actor: "admin", Action: string(audit.ActionAPIKeyRevoke), Resource: "Legacy NAS", IP: "192.0.2.24"},
		{Actor: "admin", Action: string(audit.ActionAPIKeyDelete), Resource: "Retired backup client", IP: "192.0.2.24"},
		{Actor: "admin", Action: string(audit.ActionACMEAccountDelete), Resource: "https://acme-staging.example.com/directory", IP: "192.0.2.24"},
	}
	for i := range events {
		events[i].ID = int64(i + 1)
		events[i].At = fixed.Add(-time.Duration(len(events)-i) * 24 * time.Hour)
		must(db.ORM().Create(&events[i]).Error)
	}
	must(db.ORM().Create(&database.APIKey{ID: 1, Name: "Traefik deployment", Prefix: "cv_demo_3fa1", SecretHash: "synthetic-non-authenticating-fixture", Scopes: `["certificates:read","private_keys:read"]`, AllCertificates: true, CreatedAt: fixed.AddDate(0, 0, -41)}).Error)
	must(db.ORM().Create(&database.APIKey{ID: 2, Name: "Legacy NAS", Prefix: "cv_demo_90bd", SecretHash: "synthetic-non-authenticating-fixture", Scopes: `["certificates:read"]`, AllCertificates: true, CreatedAt: fixed.AddDate(0, 0, -120), Revoked: true}).Error)
	must(os.MkdirAll(filepath.Join(cfg.DataDir, "accounts"), 0700))
	for _, host := range []string{"acme.example.com", "acme-staging.example.com"} {
		directory := "https://" + host + "/directory"
		plain, err := json.Marshal(map[string]any{"DirectoryURL": directory, "Email": "admin@example.com", "Registration": map[string]any{"status": "valid", "accountURL": "https://" + host + "/account/demo"}})
		must(err)
		encrypted, err := vault.Encrypt(cfg.MasterKey, plain)
		must(err)
		sum := sha256.Sum256([]byte(directory))
		must(os.WriteFile(filepath.Join(cfg.DataDir, "accounts", hex.EncodeToString(sum[:])+".json.enc"), encrypted, 0600))
	}
}

func main() {
	root, err := os.MkdirTemp("", "certvault-e2e-")
	must(err)
	defer func() { must(os.RemoveAll(root)) }()
	var mu sync.RWMutex
	var handler http.Handler
	var databases []*database.Database
	defer func() {
		for _, db := range databases {
			must(db.Close())
		}
	}()
	reset := func(empty bool) {
		// Keep old databases alive until exit: an asynchronous renewal may still be
		// finishing after its browser test. Each reset gets a separate data directory.
		dir := filepath.Join(root, fmt.Sprint(len(databases)))
		cfg := &config.Config{AppVersion: "e2e", DataDir: dir, MasterKey: make([]byte, 32),
			Server: config.Server{PublicURL: "http://127.0.0.1:8099"},
			Auth:   config.Auth{BootstrapToken: "certvault-e2e-admin"},
			ACME:   config.ACME{Email: "admin@example.com", DirectoryURL: "https://acme.example.com/directory", AcceptTerms: true, Mock: true},
		}
		if !empty {
			cfg.Certificates = []config.Certificate{
				{Name: "homelab-wildcard", Domains: []string{"example.com", "*.example.com"}, KeyType: config.KeyTypeEC256},
				{Name: "monitoring-services", Domains: []string{"grafana.example.com", "prometheus.example.com"}, KeyType: config.KeyTypeEC384},
				{Name: "internal-gateway", Domains: []string{"gateway.example.com"}, KeyType: config.KeyTypeEC256},
			}
		}
		db, err := database.Open(filepath.Join(dir, "test.db"))
		must(err)
		databases = append(databases, db)
		repos := repository.New(db)
		if !empty {
			seed(db, cfg, repos)
		}
		manager, err := service.NewManager(cfg, repos, slog.New(slog.NewTextHandler(io.Discard, nil)))
		must(err)
		handler, err = api.New(cfg, db, repos, manager)
		must(err)
	}
	reset(false)
	mux := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/__test/reset" {
			if r.Method != http.MethodPost {
				w.WriteHeader(http.StatusMethodNotAllowed)
				return
			}
			mu.Lock()
			defer mu.Unlock()
			reset(r.URL.Query().Get("scenario") == "empty")
			w.WriteHeader(http.StatusNoContent)
			return
		}
		mu.RLock()
		defer mu.RUnlock()
		handler.ServeHTTP(w, r)
	})
	listener, err := net.Listen("tcp", "127.0.0.1:8099")
	must(err)
	server := &http.Server{Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	done := make(chan os.Signal, 1)
	signal.Notify(done, syscall.SIGTERM, syscall.SIGINT)
	go func() {
		<-done
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_ = server.Shutdown(ctx)
	}()
	fmt.Println("CertVault E2E listening on 127.0.0.1:8099")
	if err := server.Serve(listener); err != http.ErrServerClosed {
		must(err)
	}
}
