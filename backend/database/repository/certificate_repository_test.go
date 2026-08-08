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

	jobID, err := New(db).Jobs.Start(context.Background(), "home", "renewal")
	if err != nil {
		t.Fatal(err)
	}

	certificates, err = repository.List(context.Background())
	if err != nil || certificates[0].LatestJob == nil || certificates[0].LatestJob.ID != jobID || certificates[0].LatestJob.Status != "running" {
		t.Fatalf("latest running job missing from certificate: %#v %v", certificates, err)
	}

	if err = New(db).Jobs.Finish(context.Background(), jobID, nil); err != nil {
		t.Fatal(err)
	}

	certificate, err := repository.Get(context.Background(), "home")
	if err != nil || certificate.LatestJob == nil || certificate.LatestJob.FinishedAt == nil {
		t.Fatalf("latest finished job missing from certificate: %#v %v", certificate, err)
	}
}
