package database

import (
	"path/filepath"
	"testing"
)

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
		&AuditEvent{},
		&Setting{},
	}
	for _, model := range models {
		if !db.ORM().Migrator().HasTable(model) {
			t.Fatalf("table for %T was not created", model)
		}
	}
}
