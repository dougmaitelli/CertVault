package database

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"
)

func TestOpenAddsAuditIndexesToExistingDatabase(t *testing.T) {
	path := filepath.Join(t.TempDir(), "legacy.db")
	ctx := context.Background()

	legacy, err := sql.Open("sqlite3", path)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = legacy.Close() })

	if _, err = legacy.ExecContext(ctx, `CREATE TABLE audit_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		at DATETIME NOT NULL,
		actor TEXT NOT NULL,
		action TEXT NOT NULL,
		resource TEXT NOT NULL,
		detail TEXT NOT NULL DEFAULT '',
		ip TEXT NOT NULL DEFAULT ''
	)`); err != nil {
		t.Fatal(err)
	}

	if _, err = legacy.ExecContext(ctx, `INSERT INTO audit_events (at, actor, action, resource)
		VALUES (?, 'admin', 'auth.login', 'ui')`, time.Now().UTC()); err != nil {
		t.Fatal(err)
	}

	if err = legacy.Close(); err != nil {
		t.Fatal(err)
	}

	// Reopening also verifies that repeated migrations keep indexes and data intact.
	for range 2 {
		db, err := Open(path)
		if err != nil {
			t.Fatal(err)
		}

		t.Cleanup(func() { _ = db.Close() })

		for _, index := range []string{"idx_audit_at", "idx_audit_actor", "idx_audit_action", "idx_audit_resource"} {
			if !db.ORM().Migrator().HasIndex(&AuditEvent{}, index) {
				t.Errorf("missing audit index %s", index)
			}
		}

		var event AuditEvent
		if err = db.ORM().First(&event).Error; err != nil {
			t.Fatal(err)
		}

		if event.ID != 1 || event.Actor != "admin" || event.Action != "auth.login" {
			t.Fatalf("migration changed the audit event: %#v", event)
		}

		if err = db.Close(); err != nil {
			t.Fatal(err)
		}
	}
}

func TestOpenMigratesSchema(t *testing.T) {
	databasePath := filepath.Join(t.TempDir(), "nested", "data", "test.db")

	db, err := Open(databasePath)
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = db.Close() }()

	models := []any{
		&Certificate{},
		&CertificateVersion{},
		&Job{},
		&APIKey{},
		&APIKeyCertificate{},
		&AuditEvent{},
		&Setting{},
	}
	for _, model := range models {
		if !db.ORM().Migrator().HasTable(model) {
			t.Fatalf("table for %T was not created", model)
		}
	}
}

func TestAPIKeyCertificateForeignKeys(t *testing.T) {
	db, err := Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	certificate := Certificate{
		Name:               "home",
		Domains:            `["example.com"]`,
		KeyType:            "ec256",
		RenewBeforeSeconds: 2592000,
		Enabled:            true,
		Status:             "valid",
		UpdatedAt:          time.Now().UTC(),
	}
	if err = db.ORM().Create(&certificate).Error; err != nil {
		t.Fatal(err)
	}

	key := APIKey{
		Name:       "node",
		Prefix:     "cv_live_node",
		SecretHash: "hash",
		Scopes:     `[]`,
		CreatedAt:  time.Now().UTC(),
	}
	if err = db.ORM().Create(&key).Error; err != nil {
		t.Fatal(err)
	}

	access := APIKeyCertificate{APIKeyID: key.ID, CertificateID: certificate.ID}
	if err = db.ORM().Create(&access).Error; err != nil {
		t.Fatal(err)
	}

	version := CertificateVersion{
		CertificateID: certificate.ID,
		Path:          "versions/1",
		Domains:       certificate.Domains,
		Serial:        "serial",
		Issuer:        "issuer",
		NotBefore:     time.Now().UTC(),
		NotAfter:      time.Now().UTC().Add(time.Hour),
		CreatedAt:     time.Now().UTC(),
	}
	if err = db.ORM().Create(&version).Error; err != nil {
		t.Fatal(err)
	}

	job := Job{
		CertificateID: certificate.ID,
		Kind:          "manual",
		Status:        "succeeded",
		StartedAt:     time.Now().UTC(),
	}
	if err = db.ORM().Create(&job).Error; err != nil {
		t.Fatal(err)
	}

	if err = db.ORM().Delete(&certificate).Error; err != nil {
		t.Fatal(err)
	}

	var count int64
	if err = db.ORM().Model(&APIKeyCertificate{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}

	if count != 0 {
		t.Fatalf("certificate access rows after certificate deletion = %d, want 0", count)
	}

	for _, model := range []any{&CertificateVersion{}, &Job{}} {
		if err = db.ORM().Model(model).Count(&count).Error; err != nil {
			t.Fatal(err)
		}

		if count != 0 {
			t.Fatalf("%T rows after certificate deletion = %d, want 0", model, count)
		}
	}
}
