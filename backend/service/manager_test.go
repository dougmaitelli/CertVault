package service

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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

func TestIssueRejectsUnknownCertificate(t *testing.T) {
	manager, err := NewManager(&config.Config{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	if err = manager.Issue(context.Background(), "unknown", IssueKindManual); err == nil {
		t.Fatal("expected unknown certificate error")
	}
}

func TestIssueCompletionReportsPersistenceFailures(t *testing.T) {
	versionErr := errors.New("injected version insert failure")
	finishErr := errors.New("injected job finalization failure")

	for _, tc := range []struct {
		name           string
		kind           IssueKind
		acmeFailure    bool
		storageFailure bool
		versionFailure bool
		finishFailure  bool
	}{
		{name: "initial success", kind: IssueKindInitial},
		{name: "manual success", kind: IssueKindManual},
		{name: "scheduled success", kind: IssueKindScheduled},
		{name: "ACME failure", kind: IssueKindManual, acmeFailure: true},
		{name: "storage failure", kind: IssueKindManual, storageFailure: true},
		{name: "version insert failure", kind: IssueKindManual, versionFailure: true},
		{name: "job finalization failure", kind: IssueKindManual, finishFailure: true},
		{name: "version and finalization failures", kind: IssueKindManual, versionFailure: true, finishFailure: true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			dataDir := t.TempDir()

			db, err := database.Open(filepath.Join(dataDir, "certvault.db"))
			if err != nil {
				t.Fatal(err)
			}

			t.Cleanup(func() { _ = db.Close() })

			repos := repository.New(db)

			cfg := &config.Config{
				DataDir:   dataDir,
				MasterKey: bytes.Repeat([]byte{1}, 32),
				ACME:      config.ACME{Mock: true},
				Certificates: []config.Certificate{{
					Name: "home", Domains: []string{"example.com"}, KeyType: config.KeyTypeEC256,
				}},
			}
			if err = repos.Certificates.Reconcile(ctx, cfg); err != nil {
				t.Fatal(err)
			}

			var logs bytes.Buffer

			manager, err := NewManager(cfg, repos, slog.New(slog.NewJSONHandler(&logs, nil)))
			if err != nil {
				t.Fatal(err)
			}

			if err = manager.Issue(ctx, "home", IssueKindInitial); err != nil {
				t.Fatal(err)
			}

			previous, err := repos.Certificates.CurrentVersion(ctx, "home")
			if err != nil {
				t.Fatal(err)
			}

			type hookPayload struct {
				Event   string              `json:"event"`
				Error   string              `json:"error"`
				Version *repository.Version `json:"version"`
			}

			hooks := make(chan hookPayload, 4)
			notifications := make(chan apprisePayload, 2)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/hook" {
					var payload hookPayload
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						t.Errorf("decode hook: %v", err)
					}

					hooks <- payload
				} else {
					var payload apprisePayload
					if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
						t.Errorf("decode notification: %v", err)
					}

					notifications <- payload
				}

				w.WriteHeader(http.StatusNoContent)
			}))
			t.Cleanup(server.Close)
			cfg.Hooks = []config.Hook{{
				Name: "completion", Type: "webhook", URL: server.URL + "/hook",
				Events: []string{"certificate.issued", "certificate.renewed", "certificate.failed"},
			}}
			manager.notify = newAppriseNotifier(config.Notifications{AppriseURL: server.URL})

			if tc.acmeFailure {
				cfg.Certificates[0].Domains = nil
			}

			if tc.storageFailure {
				blockedPath := filepath.Join(dataDir, "blocked")
				if err = os.WriteFile(blockedPath, []byte("not a directory"), 0600); err != nil {
					t.Fatal(err)
				}

				cfg.DataDir = blockedPath
			}

			if tc.versionFailure {
				if err = db.ORM().Callback().Create().Before("gorm:create").Register("test:fail_version", func(tx *gorm.DB) {
					if tx.Statement.Table == "certificate_versions" {
						_ = tx.AddError(versionErr)
					}
				}); err != nil {
					t.Fatal(err)
				}
			}

			if tc.finishFailure {
				if err = db.ORM().Callback().Update().Before("gorm:update").Register("test:fail_finish", func(tx *gorm.DB) {
					if tx.Statement.Table == "jobs" {
						_ = tx.AddError(finishErr)
					}
				}); err != nil {
					t.Fatal(err)
				}
			}

			issueErr := manager.Issue(ctx, "home", tc.kind)

			failed := tc.acmeFailure || tc.storageFailure || tc.versionFailure || tc.finishFailure
			if (issueErr != nil) != failed {
				t.Fatalf("Issue error = %v, want failure %v", issueErr, failed)
			}

			if tc.versionFailure && !errors.Is(issueErr, versionErr) {
				t.Fatalf("version error not preserved: %v", issueErr)
			}

			if tc.finishFailure && (!errors.Is(issueErr, finishErr) || !strings.Contains(logs.String(), "finish certificate issuance job")) {
				t.Fatalf("finalization error not returned and logged: %v, logs %s", issueErr, logs.String())
			}

			expectedEvents := map[string]bool{"certificate.issued": true}
			if failed {
				expectedEvents = map[string]bool{"certificate.failed": true}
			} else if tc.kind != IssueKindInitial {
				expectedEvents["certificate.renewed"] = true
			}

			for len(expectedEvents) > 0 {
				select {
				case payload := <-hooks:
					if !expectedEvents[payload.Event] {
						t.Fatalf("unexpected or duplicate event: %s", payload.Event)
					}

					delete(expectedEvents, payload.Event)

					if failed && payload.Error != issueErr.Error() {
						t.Fatalf("hook error = %q, want %q", payload.Error, issueErr)
					}

					persisted := !tc.acmeFailure && !tc.storageFailure && !tc.versionFailure
					if (payload.Version != nil) != persisted {
						t.Fatalf("hook version = %#v, persisted = %v", payload.Version, persisted)
					}
				case <-time.After(5 * time.Second):
					t.Fatalf("missing events: %v", expectedEvents)
				}
			}

			select {
			case payload := <-notifications:
				wantType := NotificationSuccess
				if failed {
					wantType = NotificationFailure
				}

				if payload.Type != wantType || failed && !strings.Contains(payload.Body, issueErr.Error()) {
					t.Fatalf("notification = %#v, want type %s and error %v", payload, wantType, issueErr)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("missing completion notification")
			}

			jobs, err := repos.Jobs.List(ctx, 1)
			if err != nil || len(jobs) != 1 {
				t.Fatalf("jobs = %#v, error = %v", jobs, err)
			}

			wantStatus := "succeeded"
			if tc.finishFailure {
				wantStatus = "running"
			} else if failed {
				wantStatus = "failed"
			}

			if jobs[0].Status != wantStatus {
				t.Fatalf("job status = %s, want %s", jobs[0].Status, wantStatus)
			}

			current, err := repos.Certificates.CurrentVersion(ctx, "home")
			if err != nil {
				t.Fatal(err)
			}

			if tc.acmeFailure || tc.storageFailure || tc.versionFailure {
				if current.ID != previous.ID {
					t.Fatal("failed issuance replaced the previous version")
				}
			} else if current.ID == previous.ID {
				t.Fatal("persisted issuance did not update the current version")
			}
		})
	}
}

func TestIssueRejectsUnknownKind(t *testing.T) {
	manager, err := NewManager(&config.Config{}, nil, nil)
	if err != nil {
		t.Fatal(err)
	}

	if err = manager.Issue(context.Background(), "home", IssueKind("unexpected")); err == nil {
		t.Fatal("expected unsupported issuance kind error")
	}
}

func TestHookMatchesCertificateAndEvent(t *testing.T) {
	hook := config.Hook{Events: []string{"certificate.renewed"}}
	if !hookMatches(hook, "certificate.renewed", "home") {
		t.Fatal("hook without certificate filter did not match")
	}

	hook.Certificates = []string{"home", "proxy"}
	if !hookMatches(hook, "certificate.renewed", "home") {
		t.Fatal("hook did not match selected certificate")
	}

	if hookMatches(hook, "certificate.renewed", "other") {
		t.Fatal("hook matched unselected certificate")
	}

	if hookMatches(hook, "certificate.failed", "home") {
		t.Fatal("hook matched unselected event")
	}
}
