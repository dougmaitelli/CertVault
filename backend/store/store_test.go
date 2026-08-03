package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/certvault/certvault/config"
	"github.com/certvault/certvault/database"
)

func TestAPIKeyAuthenticationAndRevocation(t *testing.T) {
	db, e := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = db.Close() }()
	s := New(db)
	ctx := context.Background()
	_, token, e := s.CreateAPIKey(ctx, "node", []string{"private_keys:read"}, []string{"home"}, nil)
	if e != nil {
		t.Fatal(e)
	}
	p, e := s.Authenticate(ctx, token, "127.0.0.1")
	if e != nil {
		t.Fatal(e)
	}
	if !p.Allows("private_keys:read", "home") || p.Allows("private_keys:read", "other") {
		t.Fatal("scope enforcement failed")
	}
	keys, _ := s.ListAPIKeys(ctx)
	if len(keys) != 1 {
		t.Fatal("key not listed")
	}
	if e = s.RevokeAPIKey(ctx, keys[0].ID); e != nil {
		t.Fatal(e)
	}
	if _, e = s.Authenticate(ctx, token, "127.0.0.1"); e == nil {
		t.Fatal("revoked key authenticated")
	}
}

func TestReconcile(t *testing.T) {
	db, e := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if e != nil {
		t.Fatal(e)
	}
	defer func() { _ = db.Close() }()
	s := New(db)
	c := &config.Config{
		Certificates: []config.Certificate{
			{
				Name:    "home",
				Domains: []string{"example.com"},
				KeyType: config.KeyTypeEC256,
			},
		},
	}
	if e = s.Reconcile(context.Background(), c); e != nil {
		t.Fatal(e)
	}
	got, e := s.ListCertificates(context.Background())
	if e != nil || len(got) != 1 || got[0].Name != "home" {
		t.Fatalf("unexpected certificates: %#v %v", got, e)
	}
}
