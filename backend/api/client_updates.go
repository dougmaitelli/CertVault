package api

import (
	"archive/tar"
	"bytes"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/certvault/certvault/config"
	"github.com/certvault/certvault/vault"
)

const maxClientArchiveSize = 2 * 1024 * 1024

type clientUpdateStore struct {
	once      sync.Once
	archive   []byte
	manifest  []byte
	publicKey []byte
	revision  string
	err       error
}

func (s *clientUpdateStore) load(cfg *config.Config) error {
	s.once.Do(func() { s.err = s.initialize(cfg) })
	return s.err
}

func (s *clientUpdateStore) initialize(cfg *config.Config) error {
	directory := os.Getenv(config.EnvUIDir)
	if directory == "" {
		directory = "/app/ui"
	}

	root, err := os.OpenRoot(directory)
	if err != nil {
		return err
	}

	defer func() { _ = root.Close() }()

	var buffer bytes.Buffer

	writer := tar.NewWriter(&buffer)

	for _, file := range []string{"launcher.sh", "sync.sh", "update.sh"} {
		asset, err := root.Open("client/" + file)
		if err != nil {
			return err
		}

		content, err := io.ReadAll(io.LimitReader(asset, maxClientArchiveSize/3+1))
		closeErr := asset.Close()

		if err != nil {
			return err
		}

		if closeErr != nil {
			return closeErr
		}

		if len(content) > maxClientArchiveSize/3 || !bytes.Contains(content, []byte("# certvault-client-protocol: 1\n")) {
			return fmt.Errorf("invalid client asset %q", file)
		}

		if err = writer.WriteHeader(&tar.Header{Name: file, Mode: 0700, Size: int64(len(content)), ModTime: time.Unix(0, 0), Format: tar.FormatUSTAR}); err != nil {
			return err
		}

		if _, err = writer.Write(content); err != nil {
			return err
		}
	}

	if err = writer.Close(); err != nil {
		return err
	}

	if buffer.Len() > maxClientArchiveSize {
		return errors.New("client archive exceeds size limit")
	}

	key, err := loadClientSigningKey(cfg)
	if err != nil {
		return err
	}

	public, err := x509.MarshalPKIXPublicKey(&key.PublicKey)
	if err != nil {
		return err
	}

	s.archive = buffer.Bytes()
	sum := sha256.Sum256(s.archive)
	s.revision = hex.EncodeToString(sum[:])
	payload := []byte(fmt.Sprintf("protocol 1\nsha256 %s\nsize %d\n", s.revision, len(s.archive)))
	digest := sha256.Sum256(payload)

	signature, err := ecdsa.SignASN1(rand.Reader, key, digest[:])
	if err != nil {
		return err
	}

	s.manifest = append(payload, []byte("signature "+base64.StdEncoding.EncodeToString(signature)+"\n")...)
	s.publicKey = pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: public})

	return nil
}

func loadClientSigningKey(cfg *config.Config) (*ecdsa.PrivateKey, error) {
	if cfg.DataDir == "" {
		return nil, errors.New("client signing key requires a data directory")
	}

	path := filepath.Join(cfg.DataDir, "client-update-signing-key.enc")
	read := func() (*ecdsa.PrivateKey, error) {
		encrypted, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}

		der, err := vault.Decrypt(cfg.MasterKey, encrypted)
		if err != nil {
			return nil, err
		}

		return x509.ParseECPrivateKey(der)
	}

	key, err := read()
	if !errors.Is(err, os.ErrNotExist) {
		return key, err
	}

	key, err = ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, err
	}

	der, err := x509.MarshalECPrivateKey(key)
	if err != nil {
		return nil, err
	}

	encrypted, err := vault.Encrypt(cfg.MasterKey, der)
	if err != nil {
		return nil, err
	}

	if err = os.MkdirAll(cfg.DataDir, 0700); err != nil {
		return nil, err
	}

	file, err := os.CreateTemp(cfg.DataDir, ".client-update-key-*")
	if err != nil {
		return nil, err
	}

	defer func() { _ = os.Remove(file.Name()) }()
	defer func() { _ = file.Close() }()

	if _, err = file.Write(encrypted); err == nil {
		err = file.Sync()
	}

	if err != nil {
		return nil, err
	}

	if err = file.Close(); err != nil {
		return nil, err
	}
	// Publish a complete file without replacing another process's existing key.
	if err = os.Link(file.Name(), path); errors.Is(err, os.ErrExist) {
		return read()
	}

	if err != nil {
		return nil, err
	}

	return key, nil
}

func (a *API) clientUpdate(w http.ResponseWriter, r *http.Request) {
	if err := a.clientUpdates.load(a.cfg); err != nil {
		slog.Error("initialize client updates", "error", err)
		problem(w, http.StatusServiceUnavailable, "client_updates_unavailable", "Client updates are unavailable")

		return
	}

	var body []byte

	switch r.PathValue("asset") {
	case "key.pem":
		w.Header().Set("Content-Type", "application/x-pem-file")

		body = a.clientUpdates.publicKey
	case "manifest":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")

		body = a.clientUpdates.manifest
	case a.clientUpdates.revision + ".tar":
		w.Header().Set("Content-Type", "application/x-tar")
		w.Header().Set("ETag", `"`+a.clientUpdates.revision+`"`)
		body = a.clientUpdates.archive
	default:
		http.NotFound(w, r)
		return
	}

	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("Content-Length", fmt.Sprint(len(body)))
	_, _ = w.Write(body)
}
