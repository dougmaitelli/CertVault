package service

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/certvault/certvault/vault"
)

const (
	encryptedAccountFileSuffix = ".json.enc"
	legacyAccountID            = "account"
)

var (
	ErrACMEAccountNotFound  = errors.New("ACME account not found")
	ErrCurrentACMEAccount   = errors.New("current ACME account cannot be deleted")
	ErrInvalidACMEAccountID = errors.New("invalid ACME account ID")
)

func (m *Manager) loadUser() (*acmeUser, error) {
	dir := filepath.Join(m.cfg.DataDir, "accounts")
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	b, err := os.ReadFile(m.accountPath())
	if os.IsNotExist(err) {
		key, keyErr := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
		return &acmeUser{Email: m.cfg.ACME.Email, Key: key}, keyErr
	}
	if err != nil {
		return nil, err
	}
	plain, err := vault.Decrypt(m.cfg.MasterKey, b)
	if err != nil {
		return nil, err
	}
	var wire acmeUserWire
	if err = json.Unmarshal(plain, &wire); err != nil {
		return nil, err
	}
	key, err := x509.ParseECPrivateKey(wire.Key)
	if err != nil {
		return nil, err
	}
	user := &acmeUser{wire.Email, wire.Registration, key}
	if wire.DirectoryURL == "" {
		if err = m.saveUser(user); err != nil {
			return nil, err
		}
	}
	return user, nil
}

func (m *Manager) saveUser(user *acmeUser) error {
	key, err := x509.MarshalECPrivateKey(user.Key)
	if err != nil {
		return err
	}
	plain, err := json.Marshal(acmeUserWire{
		DirectoryURL: normalizedDirectoryURL(m.cfg.ACME.DirectoryURL),
		Email:        user.Email,
		Key:          key,
		Registration: user.Registration,
	})
	if err != nil {
		return err
	}
	encrypted, err := vault.Encrypt(m.cfg.MasterKey, plain)
	if err != nil {
		return err
	}
	return atomicWrite(m.accountPath(), encrypted, 0600)
}

func (m *Manager) accountPath() string {
	directoryURL := normalizedDirectoryURL(m.cfg.ACME.DirectoryURL)
	digest := sha256.Sum256([]byte(directoryURL))
	filename := hex.EncodeToString(digest[:]) + encryptedAccountFileSuffix
	return filepath.Join(m.cfg.DataDir, "accounts", filename)
}

func (m *Manager) ListAccounts() ([]ACMEAccount, error) {
	directory := filepath.Join(m.cfg.DataDir, "accounts")
	entries, err := os.ReadDir(directory)
	if os.IsNotExist(err) {
		return []ACMEAccount{}, nil
	}
	if err != nil {
		return nil, err
	}

	currentFilename := filepath.Base(m.accountPath())
	accounts := make([]ACMEAccount, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), encryptedAccountFileSuffix) {
			continue
		}
		account, readErr := m.readAccount(filepath.Join(directory, entry.Name()))
		if readErr != nil {
			return nil, fmt.Errorf("read ACME account %q: %w", entry.Name(), readErr)
		}
		account.ID = strings.TrimSuffix(entry.Name(), encryptedAccountFileSuffix)
		account.Current = entry.Name() == currentFilename
		if account.DirectoryURL == "" && account.Current {
			account.DirectoryURL = normalizedDirectoryURL(m.cfg.ACME.DirectoryURL)
		}
		accounts = append(accounts, account)
	}
	sort.Slice(accounts, func(i, j int) bool {
		if accounts[i].Current != accounts[j].Current {
			return accounts[i].Current
		}
		return accounts[i].DirectoryURL < accounts[j].DirectoryURL
	})
	return accounts, nil
}

func (m *Manager) DeleteAccount(id string) (ACMEAccount, error) {
	if !validAccountID(id) {
		return ACMEAccount{}, ErrInvalidACMEAccountID
	}
	if id == strings.TrimSuffix(filepath.Base(m.accountPath()), encryptedAccountFileSuffix) {
		return ACMEAccount{}, ErrCurrentACMEAccount
	}

	path := filepath.Join(m.cfg.DataDir, "accounts", id+encryptedAccountFileSuffix)
	account, err := m.readAccount(path)
	if os.IsNotExist(err) {
		return ACMEAccount{}, ErrACMEAccountNotFound
	}
	if err != nil {
		return ACMEAccount{}, err
	}
	account.ID = id
	if err = os.Remove(path); err != nil {
		if os.IsNotExist(err) {
			return ACMEAccount{}, ErrACMEAccountNotFound
		}
		return ACMEAccount{}, err
	}
	return account, nil
}

func validAccountID(id string) bool {
	if id == legacyAccountID {
		return true
	}
	digest, err := hex.DecodeString(id)
	return err == nil && len(digest) == sha256.Size
}

func (m *Manager) readAccount(path string) (ACMEAccount, error) {
	encrypted, err := os.ReadFile(path)
	if err != nil {
		return ACMEAccount{}, err
	}
	plain, err := vault.Decrypt(m.cfg.MasterKey, encrypted)
	if err != nil {
		return ACMEAccount{}, err
	}
	var wire acmeUserWire
	if err = json.Unmarshal(plain, &wire); err != nil {
		return ACMEAccount{}, err
	}
	account := ACMEAccount{
		DirectoryURL: wire.DirectoryURL,
		Email:        wire.Email,
		Status:       "unregistered",
	}
	if wire.Registration != nil {
		account.Status = wire.Registration.Status
		if account.Status == "" {
			account.Status = "registered"
		}
		account.RegistrationURL = wire.Registration.Location
	}
	return account, nil
}

func normalizedDirectoryURL(directoryURL string) string {
	return strings.TrimRight(directoryURL, "/")
}
