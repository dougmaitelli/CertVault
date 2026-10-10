package repository

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"

	"github.com/certvault/certvault/audit"
	"github.com/certvault/certvault/database"
	"gorm.io/gorm"
)

func TestAuditWriteFailureReturnsErrorAndLogsIdentity(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	injected := errors.New("injected audit failure")

	if err = db.ORM().Callback().Create().Before("gorm:create").Register("test:fail_audit", func(tx *gorm.DB) {
		if tx.Statement.Table == "audit_events" {
			_ = tx.AddError(injected)
		}
	}); err != nil {
		t.Fatal(err)
	}

	var logs bytes.Buffer

	previous := slog.Default()

	slog.SetDefault(slog.New(slog.NewJSONHandler(&logs, nil)))
	defer slog.SetDefault(previous)

	err = New(db).Audits.Record(context.Background(), "alice@example.com", audit.ActionACMEAccountDelete, "old-directory", "", "192.0.2.1")
	if !errors.Is(err, injected) {
		t.Fatalf("audit error discarded: %v", err)
	}

	for _, value := range []string{"persist audit event", "alice@example.com", "acme_account.delete", "old-directory", injected.Error()} {
		if !strings.Contains(logs.String(), value) {
			t.Fatalf("failure log missing %q: %s", value, logs.String())
		}
	}
}

func TestAPIKeyMutationsRollbackWhenAuditingFails(t *testing.T) {
	for _, operation := range []string{"create", "revoke", "delete"} {
		t.Run(operation, func(t *testing.T) {
			ctx := context.Background()

			db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
			if err != nil {
				t.Fatal(err)
			}

			defer func() { _ = db.Close() }()

			repos := New(db)

			key, _, err := repos.APIKeys.Create(ctx, "deploy", []string{"certificates:read"}, []string{"*"}, nil)
			if err != nil {
				t.Fatal(err)
			}

			if operation == "delete" {
				if _, err = repos.APIKeys.Revoke(ctx, key.ID); err != nil {
					t.Fatal(err)
				}
			}

			injected := errors.New("injected audit failure")

			if err = db.ORM().Callback().Create().Before("gorm:create").Register("test:fail_audit", func(tx *gorm.DB) {
				if tx.Statement.Table == "audit_events" {
					_ = tx.AddError(injected)
				}
			}); err != nil {
				t.Fatal(err)
			}

			metadata := AuditMetadata{Actor: "operator@example.com", IP: "192.0.2.1"}

			switch operation {
			case "create":
				created, token, createErr := repos.APIKeys.Create(ctx, "new", []string{"*"}, []string{"*"}, nil, metadata)
				err = createErr

				if token != "" || created.ID != 0 {
					t.Fatal("failed audited creation returned usable credentials")
				}
			case "revoke":
				_, err = repos.APIKeys.Revoke(ctx, key.ID, metadata)
			case "delete":
				_, err = repos.APIKeys.Delete(ctx, key.ID, metadata)
			}

			if !errors.Is(err, injected) {
				t.Fatalf("mutation discarded audit failure: %v", err)
			}

			keys, err := repos.APIKeys.List(ctx)
			if err != nil || len(keys) != 1 || keys[0].ID != key.ID || keys[0].Revoked != (operation == "delete") {
				t.Fatalf("mutation was not rolled back: %#v %v", keys, err)
			}

			events, err := repos.Audits.List(ctx, 10)
			if err != nil || len(events) != 0 {
				t.Fatalf("failed mutation has audit entries: %#v %v", events, err)
			}
		})
	}
}
