package service

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"io"
	"log/slog"
	"path/filepath"
	"testing"

	"github.com/certvault/certvault/config"
	"github.com/certvault/certvault/database"
	"github.com/certvault/certvault/database/repository"
)

func TestReconcileIssuesConfigurationDrift(t *testing.T) {
	for _, tc := range []struct {
		name      string
		domains   []string
		key       config.KeyType
		legacy    bool
		automatic bool
		want      bool
	}{
		{"SAN addition", []string{"example.com", "www.example.com", "new.example.com"}, config.KeyTypeEC256, false, true, true},
		{"SAN removal", []string{"example.com"}, config.KeyTypeEC256, false, true, true},
		{"key curve", []string{"example.com", "www.example.com"}, config.KeyTypeEC384, false, true, true},
		{"key algorithm", []string{"example.com", "www.example.com"}, config.KeyTypeRSA2048, false, true, true},
		{"order and case", []string{"WWW.EXAMPLE.COM", "example.com"}, config.KeyTypeEC256, false, true, false},
		{"legacy same key", []string{"example.com", "www.example.com"}, config.KeyTypeEC256, true, true, false},
		{"legacy changed key", []string{"example.com", "www.example.com"}, config.KeyTypeEC384, true, true, true},
		{"automatic disabled", []string{"example.com"}, config.KeyTypeEC384, false, false, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := context.Background()
			dir := t.TempDir()
			cfg := &config.Config{DataDir: dir, MasterKey: make([]byte, 32), ACME: config.ACME{Mock: true}, Certificates: []config.Certificate{{Name: "home", Domains: []string{"example.com", "www.example.com"}, KeyType: config.KeyTypeEC256}}}

			db, err := database.Open(filepath.Join(dir, "test.db"))
			if err != nil {
				t.Fatal(err)
			}

			defer func() { _ = db.Close() }()

			repos := repository.New(db)
			if err = repos.Certificates.Reconcile(ctx, cfg); err != nil {
				t.Fatal(err)
			}

			m, err := NewManager(cfg, repos, slog.New(slog.NewTextHandler(io.Discard, nil)))
			if err != nil {
				t.Fatal(err)
			}

			if err = m.Issue(ctx, "home", IssueKindInitial); err != nil {
				t.Fatal(err)
			}

			previous, err := repos.Certificates.CurrentVersion(ctx, "home")
			if err != nil {
				t.Fatal(err)
			}

			if previous.KeyType != config.KeyTypeEC256 {
				t.Fatalf("issued key metadata: %q", previous.KeyType)
			}

			if tc.legacy {
				if err = db.ORM().Model(&database.CertificateVersion{}).Where("id = ?", previous.ID).Update("key_type", "").Error; err != nil {
					t.Fatal(err)
				}
			}

			cfg.Certificates[0].Domains = tc.domains
			cfg.Certificates[0].KeyType = tc.key

			cfg.ACME.AutomaticIssuance = tc.automatic
			if err = repos.Certificates.Reconcile(ctx, cfg); err != nil {
				t.Fatal(err)
			}

			m.reconcile(ctx)
			m.reconcile(ctx)

			jobs, err := repos.Jobs.List(ctx, 10)
			if err != nil {
				t.Fatal(err)
			}

			count := 1
			if tc.want {
				count++
			}

			if len(jobs) != count {
				t.Fatalf("job count = %d, want %d: %#v", len(jobs), count, jobs)
			}

			current, err := repos.Certificates.CurrentVersion(ctx, "home")
			if err != nil || current.ID != previous.ID || !sameDomains(current.Domains, previous.Domains) {
				t.Fatalf("reconciliation changed current certificate: %#v %v", current, err)
			}

			if !tc.want {
				return
			}

			job, err := repos.Jobs.Claim(ctx)
			if err != nil || job == nil {
				t.Fatalf("claim: %#v %v", job, err)
			}

			if err = m.issue(ctx, job.CertificateName, IssueKind(job.Kind), job.ID); err != nil {
				t.Fatal(err)
			}

			current, err = repos.Certificates.CurrentVersion(ctx, "home")
			if err != nil || current.ID == previous.ID || current.KeyType != tc.key || !sameDomains(current.Domains, tc.domains) {
				t.Fatalf("replacement mismatch: %#v %v", current, err)
			}

			bytes, err := m.ReadFile(current, "certificate.crt")
			if err != nil || len(bytes) == 0 {
				t.Fatalf("replacement download: %v", err)
			}

			block, _ := pem.Decode(bytes)
			if block == nil {
				t.Fatal("replacement download contains no certificate")
			}

			issued, err := x509.ParseCertificate(block.Bytes)
			if err != nil {
				t.Fatal(err)
			}

			if issuedKeyType(issued) != tc.key || !sameDomains(issued.DNSNames, tc.domains) {
				t.Fatal("download does not match configured SANs and key type")
			}

			m.reconcile(ctx)

			jobs, err = repos.Jobs.List(ctx, 10)
			if err != nil || len(jobs) != count {
				t.Fatalf("replacement keeps queuing: %#v %v", jobs, err)
			}
		})
	}
}
