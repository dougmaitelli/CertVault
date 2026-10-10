package service

import (
	"bytes"
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

	block, rest := pem.Decode(certificatePEM)
	if block == nil {
		t.Fatal("mock certificate is not PEM encoded")
	}

	if len(bytes.TrimSpace(rest)) != 0 {
		t.Fatal("certificate.crt contains more than the leaf")
	}

	certificate, err := x509.ParseCertificate(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}

	if err = certificate.VerifyHostname("service.example.test"); err != nil {
		t.Fatal(err)
	}

	chainPEM, err := manager.ReadFile(version, "chain.crt")
	if err != nil {
		t.Fatal(err)
	}

	issuerBlock, rest := pem.Decode(chainPEM)
	if issuerBlock == nil || len(bytes.TrimSpace(rest)) != 0 {
		t.Fatal("mock chain must contain exactly one issuer")
	}

	issuer, err := x509.ParseCertificate(issuerBlock.Bytes)
	if err != nil {
		t.Fatal(err)
	}

	if err = certificate.CheckSignatureFrom(issuer); err != nil {
		t.Fatal(err)
	}

	fullChain, err := manager.ReadFile(version, "fullchain.crt")
	if err != nil || !bytes.Equal(fullChain, append(bytes.Clone(certificatePEM), chainPEM...)) {
		t.Fatalf("mock full chain is duplicated or inconsistent: %v", err)
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

func TestMockACMEResponseMatchesLegoBundleFormat(t *testing.T) {
	resource, err := mockCertificate(config.Certificate{Name: "mock", Domains: []string{"example.test"}, KeyType: config.KeyTypeEC256})
	if err != nil {
		t.Fatal(err)
	}

	block, remaining := pem.Decode(resource.Certificate)
	if block == nil || !bytes.Equal(remaining, resource.IssuerCertificate) || len(remaining) == 0 {
		t.Fatal("mock does not return leaf plus bundled issuer and a separate issuer field")
	}
}
