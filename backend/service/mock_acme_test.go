package service

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"io"
	"log/slog"
	"path/filepath"
	"testing"
	"time"

	"github.com/certvault/certvault/config"
	"github.com/certvault/certvault/database"
	"github.com/certvault/certvault/database/repository"
)

func TestMockACMEIssuanceUsesRealStorageWorkflow(t *testing.T) {
	ctx := context.Background()
	dataDir := t.TempDir()
	cfg := &config.Config{
		DataDir:   dataDir,
		MasterKey: make([]byte, 32),
		ACME: config.ACME{
			Email: "dev@example.com",
			Mock:  true,
		},
		Certificates: []config.Certificate{
			{
				Name:    "development",
				Domains: []string{"example.test", "*.example.test"},
				KeyType: config.KeyTypeEC256,
			},
		},
	}

	db, err := database.Open(filepath.Join(dataDir, "certvault.db"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	repositories := repository.New(db)
	if err = repositories.Certificates.Reconcile(ctx, cfg); err != nil {
		t.Fatal(err)
	}

	manager, err := NewManager(
		cfg,
		repositories,
		slog.New(slog.NewTextHandler(io.Discard, nil)),
	)
	if err != nil {
		t.Fatal(err)
	}

	notifications := make(chan sentNotification, 1)
	manager.notify = channelNotifier{notifications}

	if err = manager.Issue(ctx, "development", IssueKindManual); err != nil {
		t.Fatal(err)
	}

	select {
	case notification := <-notifications:
		if notification.title != "Certificate renewed" || notification.typeName != NotificationSuccess {
			t.Fatalf("notification = %#v", notification)
		}
		if notification.body != `Certificate "development" was renewed successfully.` {
			t.Fatalf("notification body = %q", notification.body)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for issuance notification")
	}

	version, err := repositories.Certificates.CurrentVersion(ctx, "development")
	if err != nil {
		t.Fatal(err)
	}

	if version.Issuer != "CN=CertVault Development CA" {
		t.Fatalf("issuer = %q", version.Issuer)
	}

	if len(version.Domains) != 2 {
		t.Fatalf("domains = %#v", version.Domains)
	}

	certificatePEM, err := manager.ReadFile(version, "certificate.crt")
	if err != nil {
		t.Fatal(err)
	}

	block, _ := pem.Decode(certificatePEM)
	if block == nil {
		t.Fatal("mock certificate is not PEM encoded")
	}

	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}

	if err = certificate.VerifyHostname("service.example.test"); err != nil {
		t.Fatal(err)
	}

	if _, err = manager.ReadFile(version, "private.key"); err != nil {
		t.Fatal(err)
	}

	jobs, err := repositories.Jobs.List(ctx, 10)
	if err != nil {
		t.Fatal(err)
	}

	if len(jobs) != 1 || jobs[0].Status != "succeeded" {
		t.Fatalf("jobs = %#v", jobs)
	}
}

type sentNotification struct {
	title    string
	body     string
	typeName NotificationType
}

type channelNotifier struct {
	notifications chan<- sentNotification
}

func (channelNotifier) Configured() bool { return true }

func (n channelNotifier) Notify(
	_ context.Context,
	title string,
	body string,
	typeName NotificationType,
) error {
	n.notifications <- sentNotification{title: title, body: body, typeName: typeName}
	return nil
}
