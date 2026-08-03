package repository

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/certvault/certvault/config"
	"github.com/certvault/certvault/database"
)

func TestCertificateReconcile(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	repository := New(db).Certificates
	cfg := &config.Config{
		Certificates: []config.Certificate{
			{
				Name:    "home",
				Domains: []string{"example.com"},
				KeyType: config.KeyTypeEC256,
			},
		},
	}
	if err = repository.Reconcile(context.Background(), cfg); err != nil {
		t.Fatal(err)
	}
	certificates, err := repository.List(context.Background())
	if err != nil || len(certificates) != 1 || certificates[0].Name != "home" {
		t.Fatalf("unexpected certificates: %#v %v", certificates, err)
	}
}
