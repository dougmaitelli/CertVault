package repository

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/certvault/certvault/config"
	"github.com/certvault/certvault/database"
)

func TestAPIKeyAuthenticationAndRevocation(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	repositories := New(db)
	if err = repositories.Certificates.Reconcile(context.Background(), &config.Config{
		Certificates: []config.Certificate{
			{Name: "home", Domains: []string{"example.com"}},
		},
	}); err != nil {
		t.Fatal(err)
	}
	repository := repositories.APIKeys
	ctx := context.Background()
	_, token, err := repository.Create(
		ctx,
		"node",
		[]string{"private_keys:read"},
		[]string{"home"},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	principal, err := repository.Authenticate(ctx, token, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if !principal.Allows("private_keys:read", "home") || principal.Allows("private_keys:read", "other") {
		t.Fatal("scope enforcement failed")
	}
	keys, err := repository.List(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(keys) != 1 {
		t.Fatal("key not listed")
	}
	if err = repository.Revoke(ctx, keys[0].ID); err != nil {
		t.Fatal(err)
	}
	if _, err = repository.Authenticate(ctx, token, "127.0.0.1"); err == nil {
		t.Fatal("revoked key authenticated")
	}
}

func TestAPIKeyRejectsUnknownCertificate(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	_, _, err = New(db).APIKeys.Create(
		context.Background(),
		"node",
		[]string{"certificates:read"},
		[]string{"missing"},
		nil,
	)
	if err == nil {
		t.Fatal("API key accepted an unknown certificate")
	}

	var count int64
	if err = db.ORM().Model(&database.APIKey{}).Count(&count).Error; err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatalf("rolled-back API keys = %d, want 0", count)
	}
}

func TestAPIKeyWildcardUsesAllCertificatesFlag(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	repository := New(db).APIKeys
	key, token, err := repository.Create(
		context.Background(),
		"all-nodes",
		[]string{"certificates:read"},
		[]string{"*"},
		nil,
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(key.Certificates) != 1 || key.Certificates[0] != "*" {
		t.Fatalf("certificates = %#v", key.Certificates)
	}
	principal, err := repository.Authenticate(context.Background(), token, "127.0.0.1")
	if err != nil {
		t.Fatal(err)
	}
	if !principal.Allows("certificates:read", "any-certificate") {
		t.Fatal("wildcard API key did not allow an arbitrary certificate")
	}

	var accessCount int64
	if err = db.ORM().Model(&database.APIKeyCertificate{}).Count(&accessCount).Error; err != nil {
		t.Fatal(err)
	}
	if accessCount != 0 {
		t.Fatalf("wildcard join rows = %d, want 0", accessCount)
	}
}
