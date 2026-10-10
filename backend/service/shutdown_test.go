package service

import (
	"context"
	"encoding/json"
	"encoding/pem"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/certvault/certvault/config"
	"github.com/certvault/certvault/database"
	"github.com/certvault/certvault/database/repository"
	"gorm.io/gorm"
)

func TestWorkerShutdownFinalizesInterruptedIssuance(t *testing.T) {
	for _, phase := range []string{"ACME registration", "version persistence"} {
		t.Run(phase, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()

			entered := make(chan struct{})

			release := make(chan struct{})

			dir := t.TempDir()
			cfg := &config.Config{DataDir: dir, MasterKey: make([]byte, 32), ACME: config.ACME{Mock: true}, Certificates: []config.Certificate{{Name: "home", Domains: []string{"example.com"}}}}

			if phase == "ACME registration" {
				server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					base := "https://" + r.Host
					switch r.URL.Path {
					case "/directory":
						_ = json.NewEncoder(w).Encode(map[string]string{"newNonce": base + "/nonce", "newAccount": base + "/account", "newOrder": base + "/order"})
					case "/nonce":
						w.Header().Set("Replay-Nonce", "test-nonce")
						w.WriteHeader(http.StatusNoContent)
					case "/account":
						close(entered)

						select {
						case <-r.Context().Done():
						case <-release:
						}
					default:
						http.NotFound(w, r)
					}
				}))

				defer func() { close(release); server.Close() }()

				caFile := filepath.Join(dir, "acme-ca.pem")
				if err := os.WriteFile(caFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: server.Certificate().Raw}), 0600); err != nil {
					t.Fatal(err)
				}

				t.Setenv("LEGO_CA_CERTIFICATES", caFile)

				cfg.ACME = config.ACME{Email: "admin@example.com", DirectoryURL: server.URL + "/directory", AcceptTerms: true}
			}

			db, err := database.Open(filepath.Join(dir, "test.db"))
			if err != nil {
				t.Fatal(err)
			}

			defer func() { _ = db.Close() }()

			repos := repository.New(db)
			if err = repos.Certificates.Reconcile(ctx, cfg); err != nil {
				t.Fatal(err)
			}

			if phase == "version persistence" {
				if err = db.ORM().Callback().Create().Before("gorm:create").Register("test:cancel_persistence", func(tx *gorm.DB) {
					if tx.Statement.Table == "certificate_versions" {
						// Artifacts have been saved; cancel before the database transaction commits.
						close(entered)
						cancel()
					}
				}); err != nil {
					t.Fatal(err)
				}
			}

			manager, err := NewManager(cfg, repos, slog.New(slog.NewTextHandler(io.Discard, nil)))
			if err != nil {
				t.Fatal(err)
			}

			job, _, err := manager.Enqueue(ctx, "home", IssueKindManual)
			if err != nil {
				t.Fatal(err)
			}

			done := make(chan struct{})
			go func() { defer close(done); manager.Run(ctx) }()

			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				cancel()
				t.Fatal("issuance did not reach interruption point")
			}

			cancel()

			select {
			case <-done:
			case <-time.After(5 * time.Second):
				t.Fatal("worker did not finish after cancellation")
			}
			// Finalization must survive cancellation and happen before Run returns,
			// while the database is still available to the server shutdown lifecycle.
			finished, err := repos.Jobs.Get(context.Background(), job.ID)
			if err != nil {
				t.Fatal(err)
			}

			if finished.Status != "failed" || finished.FinishedAt == nil || !strings.HasPrefix(finished.Error, repository.InterruptedIssuanceError) {
				t.Fatalf("interrupted job was not finalized: %#v", finished)
			}

			if _, err = repos.Certificates.CurrentVersion(context.Background(), "home"); !repository.NotFound(err) {
				t.Fatalf("interrupted issuance published a version: %v", err)
			}

			if phase == "version persistence" {
				artifacts, err := filepath.Glob(filepath.Join(dir, "certificates", "home", "versions", "*", "private.key.enc"))
				if err != nil || len(artifacts) != 1 {
					t.Fatalf("did not interrupt after artifact persistence: %v %v", artifacts, err)
				}

				if _, err = os.Stat(artifacts[0]); err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}
