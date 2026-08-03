package service

import (
	"path/filepath"
	"testing"

	"github.com/certvault/certvault/config"
)

func TestAccountPathIsSpecificToACMEDirectory(t *testing.T) {
	dataDir := t.TempDir()
	staging := &Manager{cfg: &config.Config{
		DataDir: dataDir,
		ACME: config.ACME{
			DirectoryURL: "https://acme-staging.example.com/directory",
		},
	}}
	production := &Manager{cfg: &config.Config{
		DataDir: dataDir,
		ACME: config.ACME{
			DirectoryURL: "https://acme.example.com/directory",
		},
	}}

	if staging.accountPath() == production.accountPath() {
		t.Fatalf("staging and production account paths both equal %q", staging.accountPath())
	}
	if filepath.Dir(staging.accountPath()) != filepath.Join(dataDir, "accounts") {
		t.Fatalf("account path = %q", staging.accountPath())
	}
}

func TestAccountPathIgnoresTrailingDirectorySlash(t *testing.T) {
	manager := &Manager{cfg: &config.Config{
		DataDir: "/data",
		ACME: config.ACME{
			DirectoryURL: "https://acme.example.com/directory",
		},
	}}
	withoutSlash := manager.accountPath()
	manager.cfg.ACME.DirectoryURL += "///"

	if manager.accountPath() != withoutSlash {
		t.Fatalf("account paths differ: %q and %q", withoutSlash, manager.accountPath())
	}
}
