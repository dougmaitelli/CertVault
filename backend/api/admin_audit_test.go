package api

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/certvault/certvault/api/auth"
	"github.com/certvault/certvault/config"
	"github.com/certvault/certvault/database"
	"github.com/certvault/certvault/database/repository"
	certnetwork "github.com/certvault/certvault/network"
	"github.com/certvault/certvault/service"
	"github.com/certvault/certvault/vault"
	"gorm.io/gorm"
)

func TestAPIKeyAdministrationAuditsAuthenticatedIdentity(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	repos := repository.New(db)

	resolver, err := certnetwork.NewClientIPResolver(nil)
	if err != nil {
		t.Fatal(err)
	}

	a := &API{repos: repos, clientIPs: resolver}

	for _, actor := range []string{"alice@example.com", "bob@example.com"} {
		request := func(method string, body string, id int64) *http.Request {
			r := httptest.NewRequestWithContext(context.Background(), method, "/api/v1/api-keys", strings.NewReader(body))
			r.Header.Set("Content-Type", "application/json")
			r.SetPathValue("id", fmt.Sprint(id))

			return auth.WithIdentity(r, auth.Identity{Admin: true, Name: actor, AuthenticationMethod: "oidc"})
		}
		created := httptest.NewRecorder()
		a.createAPIKey(created, request(http.MethodPost, `{"name":"deploy","scopes":["certificates:read"],"certificates":["*"]}`, 0))

		if created.Code != http.StatusCreated {
			t.Fatalf("create: %d %s", created.Code, created.Body.String())
		}

		keys, err := repos.APIKeys.List(context.Background())
		if err != nil || len(keys) != 1 {
			t.Fatalf("created keys: %#v %v", keys, err)
		}

		id := keys[0].ID
		revoked := httptest.NewRecorder()
		a.revokeAPIKey(revoked, request(http.MethodPost, "", id))

		if revoked.Code != http.StatusNoContent {
			t.Fatalf("revoke: %d %s", revoked.Code, revoked.Body.String())
		}

		deleted := httptest.NewRecorder()
		a.deleteAPIKey(deleted, request(http.MethodDelete, "", id))

		if deleted.Code != http.StatusNoContent {
			t.Fatalf("delete: %d %s", deleted.Code, deleted.Body.String())
		}

		events, err := repos.Audits.Search(context.Background(), repository.AuditFilter{Actors: []string{actor}, Page: 1, PerPage: 10})
		if err != nil || events.Total != 3 {
			t.Fatalf("administrator attribution lost: %#v %v", events, err)
		}

		for i, action := range []string{"api_key.delete", "api_key.revoke", "api_key.create"} {
			if events.Items[i].Action != action || events.Items[i].Actor != actor || events.Items[i].Resource != "deploy" || events.Items[i].IP == "" {
				t.Fatalf("incorrect admin audit: %#v", events.Items[i])
			}
		}
	}
}

func TestACMEAccountDeletionAuditIdentityAndFailurePolicy(t *testing.T) {
	for _, tc := range []struct {
		actor string
		fail  bool
	}{
		{"alice@example.com", false}, {"bob@example.com", false}, {"operator@example.com", true},
	} {
		t.Run(tc.actor, func(t *testing.T) {
			dir := t.TempDir()
			cfg := &config.Config{DataDir: dir, MasterKey: make([]byte, 32), ACME: config.ACME{DirectoryURL: "https://current.example/directory"}}

			db, err := database.Open(filepath.Join(dir, "test.db"))
			if err != nil {
				t.Fatal(err)
			}

			defer func() { _ = db.Close() }()

			repos := repository.New(db)

			manager, err := service.NewManager(cfg, repos, slog.New(slog.NewTextHandler(io.Discard, nil)))
			if err != nil {
				t.Fatal(err)
			}

			path := filepath.Join(dir, "accounts", "account.json.enc")
			// A legacy account is deletable regardless of the active directory URL.
			encrypted, err := vault.Encrypt(cfg.MasterKey, []byte(`{"DirectoryURL":"https://old.example/directory","Email":"account@example.com"}`))
			if err != nil {
				t.Fatal(err)
			}

			if err = os.MkdirAll(filepath.Dir(path), 0700); err != nil {
				t.Fatal(err)
			}

			if err = os.WriteFile(path, encrypted, 0600); err != nil {
				t.Fatal(err)
			}

			if tc.fail {
				if err = db.ORM().Callback().Create().Before("gorm:create").Register("test:fail_audit", func(tx *gorm.DB) {
					if tx.Statement.Table == "audit_events" {
						_ = tx.AddError(errors.New("injected audit failure"))
					}
				}); err != nil {
					t.Fatal(err)
				}
			}

			resolver, err := certnetwork.NewClientIPResolver(nil)
			if err != nil {
				t.Fatal(err)
			}

			a := &API{repos: repos, manager: manager, clientIPs: resolver}
			r := httptest.NewRequestWithContext(context.Background(), http.MethodDelete, "/api/v1/acme/accounts/account", nil)
			r.SetPathValue("id", "account")
			r = auth.WithIdentity(r, auth.Identity{Admin: true, Name: tc.actor, AuthenticationMethod: "oidc"})
			response := httptest.NewRecorder()
			a.deleteACMEAccount(response, r)

			if response.Code != http.StatusNoContent {
				t.Fatalf("delete: %d %s", response.Code, response.Body.String())
			}

			if _, err = os.Stat(path); !os.IsNotExist(err) {
				t.Fatalf("account file not deleted: %v", err)
			}

			events, err := repos.Audits.List(context.Background(), 10)
			if err != nil {
				t.Fatal(err)
			}

			if tc.fail {
				if len(events) != 0 {
					t.Fatal("injected audit write unexpectedly succeeded")
				}
			} else if len(events) != 1 || events[0].Actor != tc.actor || events[0].Action != "acme_account.delete" || events[0].Resource != "https://old.example/directory" {
				t.Fatalf("account deletion attribution lost: %#v", events)
			}
		})
	}
}
