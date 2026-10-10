package api

import (
	"archive/tar"
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/hex"
	"encoding/pem"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/certvault/certvault/config"
)

func clientUpdateFixture(t *testing.T) *config.Config {
	t.Helper()

	ui := t.TempDir()
	if err := os.Mkdir(filepath.Join(ui, "client"), 0700); err != nil {
		t.Fatal(err)
	}

	for _, file := range []string{"launcher.sh", "sync.sh", "update.sh"} {
		source, err := os.ReadFile(filepath.Join("..", "..", "web", "public", "client", file))
		if err != nil {
			t.Fatal(err)
		}

		if err = os.WriteFile(filepath.Join(ui, "client", file), source, 0600); err != nil {
			t.Fatal(err)
		}
	}

	t.Setenv(config.EnvUIDir, ui)

	return &config.Config{DataDir: t.TempDir(), MasterKey: bytes.Repeat([]byte{7}, 32)}
}

func TestClientUpdateEndpointsSignExactArchiveAndPersistKey(t *testing.T) {
	cfg := clientUpdateFixture(t)
	a := &API{cfg: cfg}
	handler := http.NewServeMux()
	handler.HandleFunc("GET /client/update/{asset}", a.clientUpdate)

	get := func(asset string) *httptest.ResponseRecorder {
		r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/client/update/"+asset, nil)
		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)

		if w.Code != http.StatusOK {
			t.Fatalf("%s returned %d: %s", asset, w.Code, w.Body.String())
		}

		if w.Header().Get("Cache-Control") != "no-store" {
			t.Fatal("update response can serve a stale manifest")
		}

		return w
	}
	keyPEM := get("key.pem").Body.Bytes()

	block, _ := pem.Decode(keyPEM)
	if block == nil {
		t.Fatal("public key is not PEM")
	}

	public, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		t.Fatal(err)
	}

	key, ok := public.(*ecdsa.PublicKey)
	if !ok {
		t.Fatal("unexpected signing key type")
	}

	manifest := get("manifest").Body.String()

	lines := strings.Split(strings.TrimSuffix(manifest, "\n"), "\n")
	if len(lines) != 4 || lines[0] != "protocol 1" {
		t.Fatalf("invalid manifest: %q", manifest)
	}

	signature, err := base64.StdEncoding.DecodeString(strings.TrimPrefix(lines[3], "signature "))
	if err != nil {
		t.Fatal(err)
	}

	payload := []byte(strings.Join(lines[:3], "\n") + "\n")

	digest := sha256.Sum256(payload)
	if !ecdsa.VerifyASN1(key, digest[:], signature) {
		t.Fatal("manifest signature invalid")
	}

	tampered := sha256.Sum256(append(bytes.Clone(payload), 'x'))
	if ecdsa.VerifyASN1(key, tampered[:], signature) {
		t.Fatal("tampered manifest accepted")
	}

	revision := strings.TrimPrefix(lines[1], "sha256 ")
	archive := get(revision + ".tar").Body.Bytes()

	sum := sha256.Sum256(archive)
	if revision != hex.EncodeToString(sum[:]) {
		t.Fatal("manifest does not hash the served archive")
	}

	size, err := strconv.Atoi(strings.TrimPrefix(lines[2], "size "))
	if err != nil || size != len(archive) {
		t.Fatal("manifest size does not match archive")
	}

	reader := tar.NewReader(bytes.NewReader(archive))
	for _, file := range []string{"launcher.sh", "sync.sh", "update.sh"} {
		header, err := reader.Next()
		if err != nil || header.Name != file || header.Typeflag != tar.TypeReg || header.Mode != 0700 {
			t.Fatalf("invalid archive entry: %#v %v", header, err)
		}

		content, err := io.ReadAll(reader)
		if err != nil {
			t.Fatal(err)
		}

		source, err := os.ReadFile(filepath.Join(os.Getenv(config.EnvUIDir), "client", file))
		if err != nil || !bytes.Equal(content, source) {
			t.Fatalf("archive source mismatch: %s %v", file, err)
		}
	}

	if _, err = reader.Next(); !errors.Is(err, io.EOF) {
		t.Fatalf("unexpected archive contents: %v", err)
	}

	restarted := clientUpdateStore{}
	if err = restarted.load(cfg); err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(keyPEM, restarted.publicKey) || !bytes.Equal(archive, restarted.archive) {
		t.Fatal("restart changed trust key or immutable archive")
	}

	encrypted, err := os.ReadFile(filepath.Join(cfg.DataDir, "client-update-signing-key.enc"))
	if err != nil || bytes.Contains(encrypted, []byte("PRIVATE KEY")) {
		t.Fatal("signing key not stored encrypted")
	}

	info, err := os.Stat(filepath.Join(cfg.DataDir, "client-update-signing-key.enc"))
	if err != nil || info.Mode().Perm() != 0600 {
		t.Fatal("signing key permissions are not 0600")
	}

	wrong := &config.Config{DataDir: cfg.DataDir, MasterKey: bytes.Repeat([]byte{8}, 32)}
	if _, err = loadClientSigningKey(wrong); err == nil {
		t.Fatal("incorrect master key decrypted the signing key")
	}
}

func TestClientUpdateKeyCreationIsAtomicAcrossInstances(t *testing.T) {
	cfg := clientUpdateFixture(t)

	const count = 10

	results := make(chan []byte, count)

	var workers sync.WaitGroup
	for range count {
		workers.Go(func() {
			store := clientUpdateStore{}
			if err := store.load(cfg); err != nil {
				t.Error(err)
				return
			}

			results <- store.publicKey
		})
	}

	workers.Wait()
	close(results)

	var expected []byte
	for key := range results {
		if expected == nil {
			expected = key
		}

		if !bytes.Equal(key, expected) {
			t.Fatal("concurrent instances published different signing keys")
		}
	}

	if len(expected) == 0 {
		t.Fatal("no signing key was created")
	}
}

func TestClientUpdatesRemainAvailableWhenUIIsDisabled(t *testing.T) {
	cfg := clientUpdateFixture(t)
	disabled := false
	cfg.Server.UIEnabled = &disabled
	a := &API{cfg: cfg}
	handler := a.routes()
	r := httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/client/update/manifest", nil)
	w := httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusOK || !strings.HasPrefix(w.Body.String(), "protocol 1\n") {
		t.Fatalf("public update unavailable without UI: %d %s", w.Code, w.Body.String())
	}

	r = httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/client/update/"+strings.Repeat("0", 64)+".tar", nil)
	w = httptest.NewRecorder()
	handler.ServeHTTP(w, r)

	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown revision did not return 404: %d", w.Code)
	}
}

func TestClientUpdatesRejectAssetsOutsideUIRoot(t *testing.T) {
	cfg := clientUpdateFixture(t)
	directory := os.Getenv(config.EnvUIDir)

	file := filepath.Join(directory, "client", "sync.sh")
	if err := os.Remove(file); err != nil {
		t.Fatal(err)
	}

	outside := filepath.Join(t.TempDir(), "sync.sh")
	if err := os.WriteFile(outside, []byte("#!/bin/sh\n# certvault-client-protocol: 1\n"), 0600); err != nil {
		t.Fatal(err)
	}

	if err := os.Symlink(outside, file); err != nil {
		t.Fatal(err)
	}

	store := clientUpdateStore{}
	if err := store.load(cfg); err == nil {
		t.Fatal("update signer read a script outside the UI root")
	}

	if _, err := os.Stat(filepath.Join(cfg.DataDir, "client-update-signing-key.enc")); !os.IsNotExist(err) {
		t.Fatal("invalid assets created a signing key")
	}
}
