package service

import (
	"crypto/sha256"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/certvault/certvault/config"
	"github.com/go-acme/lego/v5/acme"
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

func TestListAccountsIncludesEachACMEDirectory(t *testing.T) {
	manager := &Manager{cfg: &config.Config{
		DataDir:   t.TempDir(),
		MasterKey: make([]byte, 32),
		ACME: config.ACME{
			Email:        "admin@example.com",
			DirectoryURL: "https://acme-staging.example.com/directory",
		},
	}}

	stagingUser, err := manager.loadUser()
	if err != nil {
		t.Fatal(err)
	}

	stagingUser.Registration = &acme.ExtendedAccount{
		Account:  acme.Account{Status: "valid"},
		Location: "https://acme-staging.example.com/account/1",
	}
	if err = manager.saveUser(stagingUser); err != nil {
		t.Fatal(err)
	}

	manager.cfg.ACME.DirectoryURL = "https://acme.example.com/directory"

	productionUser, err := manager.loadUser()
	if err != nil {
		t.Fatal(err)
	}

	productionUser.Registration = &acme.ExtendedAccount{
		Account:  acme.Account{Status: "valid"},
		Location: "https://acme.example.com/account/2",
	}
	if err = manager.saveUser(productionUser); err != nil {
		t.Fatal(err)
	}

	accounts, err := manager.ListAccounts()
	if err != nil {
		t.Fatal(err)
	}

	if len(accounts) != 2 {
		t.Fatalf("ACME accounts = %#v", accounts)
	}

	if !accounts[0].Current || accounts[0].DirectoryURL != manager.cfg.ACME.DirectoryURL {
		t.Fatalf("current ACME account = %#v", accounts[0])
	}

	if accounts[1].Current || accounts[1].DirectoryURL != "https://acme-staging.example.com/directory" {
		t.Fatalf("staging ACME account = %#v", accounts[1])
	}

	if _, err = manager.DeleteAccount(accounts[0].ID); !errors.Is(err, ErrCurrentACMEAccount) {
		t.Fatalf("delete current ACME account error = %v", err)
	}

	deleted, err := manager.DeleteAccount(accounts[1].ID)
	if err != nil {
		t.Fatal(err)
	}

	if deleted.DirectoryURL != "https://acme-staging.example.com/directory" {
		t.Fatalf("deleted ACME account = %#v", deleted)
	}

	accounts, err = manager.ListAccounts()
	if err != nil {
		t.Fatal(err)
	}

	if len(accounts) != 1 || !accounts[0].Current {
		t.Fatalf("ACME accounts after deletion = %#v", accounts)
	}
}

func TestDeleteAccountRejectsInvalidAndUnknownIDs(t *testing.T) {
	manager := &Manager{cfg: &config.Config{
		DataDir:   t.TempDir(),
		MasterKey: make([]byte, 32),
		ACME: config.ACME{
			DirectoryURL: "https://acme.example.com/directory",
		},
	}}

	if _, err := manager.DeleteAccount("../account"); !errors.Is(err, ErrInvalidACMEAccountID) {
		t.Fatalf("invalid ACME account ID error = %v", err)
	}

	unknownID := strings.Repeat("0", sha256.Size*2)
	if _, err := manager.DeleteAccount(unknownID); !errors.Is(err, ErrACMEAccountNotFound) {
		t.Fatalf("unknown ACME account error = %v", err)
	}
}

func TestDeleteLegacyAccount(t *testing.T) {
	manager := &Manager{cfg: &config.Config{
		DataDir:   t.TempDir(),
		MasterKey: make([]byte, 32),
		ACME: config.ACME{
			Email:        "admin@example.com",
			DirectoryURL: "https://acme.example.com/directory",
		},
	}}

	user, err := manager.loadUser()
	if err != nil {
		t.Fatal(err)
	}

	if err = manager.saveUser(user); err != nil {
		t.Fatal(err)
	}

	legacyPath := filepath.Join(manager.cfg.DataDir, "accounts", legacyAccountID+encryptedAccountFileSuffix)
	if err = os.Rename(manager.accountPath(), legacyPath); err != nil {
		t.Fatal(err)
	}

	deleted, err := manager.DeleteAccount(legacyAccountID)
	if err != nil {
		t.Fatal(err)
	}

	if deleted.Email != manager.cfg.ACME.Email {
		t.Fatalf("deleted legacy account = %#v", deleted)
	}

	if _, err = os.Stat(legacyPath); !os.IsNotExist(err) {
		t.Fatalf("legacy account still exists: %v", err)
	}
}
