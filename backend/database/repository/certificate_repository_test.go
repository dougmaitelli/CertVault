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

func TestCertificateDisableLifecycle(t *testing.T) {
	for _, transition := range []string{"initially disabled", "disabled after issuance", "removed", "empty configuration"} {
		t.Run(transition, func(t *testing.T) {
			ctx := context.Background()

			db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
			if err != nil {
				t.Fatal(err)
			}

			t.Cleanup(func() { _ = db.Close() })

			repos := New(db)
			cfg := &config.Config{Certificates: []config.Certificate{{Name: "home", Domains: []string{"example.com"}}, {Name: "other", Domains: []string{"other.example.com"}}}}

			disabled := false
			if transition == "initially disabled" {
				cfg.Certificates[0].Enabled = &disabled
			}

			if err = repos.Certificates.Reconcile(ctx, cfg); err != nil {
				t.Fatal(err)
			}

			if transition != "initially disabled" {
				if err = repos.Certificates.AddVersion(ctx, Version{CertificateName: "home", Domains: []string{"example.com"}, Path: "retained"}); err != nil {
					t.Fatal(err)
				}
			}

			switch transition {
			case "disabled after issuance":
				cfg.Certificates[0].Enabled = &disabled
			case "removed":
				cfg.Certificates = cfg.Certificates[1:]
			case "empty configuration":
				cfg.Certificates = nil
			}

			if err = repos.Certificates.Reconcile(ctx, cfg); err != nil {
				t.Fatal(err)
			}

			var stored database.Certificate
			if err = db.ORM().WithContext(ctx).Where("name = ?", "home").First(&stored).Error; err != nil {
				t.Fatal(err)
			}

			if stored.Enabled {
				t.Fatal("disabled certificate remained enabled")
			}

			certificates, err := repos.Certificates.List(ctx)

			want := 1
			if transition == "empty configuration" {
				want = 0
			}

			if err != nil || len(certificates) != want {
				t.Fatalf("list = %#v, %v", certificates, err)
			}

			if _, err = repos.Certificates.Get(ctx, "home"); !NotFound(err) {
				t.Fatalf("get: %v", err)
			}

			if _, err = repos.Certificates.CurrentVersion(ctx, "home"); !NotFound(err) {
				t.Fatalf("current version: %v", err)
			}

			if _, err = repos.Certificates.Versions(ctx, "home"); !NotFound(err) {
				t.Fatalf("versions: %v", err)
			}

			if _, err = repos.Jobs.Start(ctx, "home", "scheduled"); !NotFound(err) {
				t.Fatalf("start job: %v", err)
			}

			if err = repos.Certificates.AddVersion(ctx, Version{CertificateName: "home"}); !NotFound(err) {
				t.Fatalf("add version: %v", err)
			}

			cfg.Certificates = []config.Certificate{{Name: "home", Domains: []string{"example.com"}}}
			if err = repos.Certificates.Reconcile(ctx, cfg); err != nil {
				t.Fatal(err)
			}

			certificate, err := repos.Certificates.Get(ctx, "home")
			if err != nil {
				t.Fatal(err)
			}

			if transition == "initially disabled" {
				if certificate.Status != "pending" {
					t.Fatalf("initial status = %q", certificate.Status)
				}
			} else if certificate.CurrentVersion == nil || certificate.CurrentVersion.Path != "retained" {
				t.Fatal("re-enable lost version")
			}

			var restored database.Certificate
			if err = db.ORM().WithContext(ctx).Where("name = ?", "home").First(&restored).Error; err != nil {
				t.Fatal(err)
			}

			if restored.ID != stored.ID {
				t.Fatal("reconciliation changed certificate ID")
			}
		})
	}
}
