package service

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"

	"github.com/certvault/certvault/vault"
)

const encryptedAccountFileSuffix = ".json.enc"

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
	return &acmeUser{wire.Email, wire.Registration, key}, err
}

func (m *Manager) saveUser(user *acmeUser) error {
	key, err := x509.MarshalECPrivateKey(user.Key)
	if err != nil {
		return err
	}
	plain, err := json.Marshal(acmeUserWire{user.Email, key, user.Registration})
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
	directoryURL := strings.TrimRight(m.cfg.ACME.DirectoryURL, "/")
	digest := sha256.Sum256([]byte(directoryURL))
	filename := hex.EncodeToString(digest[:]) + encryptedAccountFileSuffix
	return filepath.Join(m.cfg.DataDir, "accounts", filename)
}
