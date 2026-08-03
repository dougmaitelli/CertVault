package store

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/certvault/certvault/database"
)

func TestAPIKeyAuthenticationAndRevocation(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	repository := New(db).APIKeys
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
