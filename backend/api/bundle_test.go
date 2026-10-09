package api

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/certvault/certvault/config"
	"github.com/certvault/certvault/database"
	"github.com/certvault/certvault/database/repository"
	"github.com/certvault/certvault/service"
)

func TestBundleDownloads(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	cfg := &config.Config{DataDir: dir, MasterKey: make([]byte, 32), ACME: config.ACME{Mock: true}, Certificates: []config.Certificate{{Name: "home", Domains: []string{"example.com"}}, {Name: "other", Domains: []string{"other.example.com"}}}}

	db, err := database.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	repos := repository.New(db)
	if err = repos.Certificates.Reconcile(ctx, cfg); err != nil {
		t.Fatal(err)
	}

	manager, err := service.NewManager(cfg, repos, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}

	if err = manager.Issue(ctx, "home", service.IssueKindInitial); err != nil {
		t.Fatal(err)
	}

	handler, err := New(cfg, db, repos, manager)
	if err != nil {
		t.Fatal(err)
	}

	_, fullToken, err := repos.APIKeys.Create(ctx, "full", []string{scopeCertificatesRead, scopePrivateKeysRead}, []string{"home"}, nil)
	if err != nil {
		t.Fatal(err)
	}

	_, publicToken, err := repos.APIKeys.Create(ctx, "public", []string{scopeCertificatesRead}, []string{"home"}, nil)
	if err != nil {
		t.Fatal(err)
	}

	request := func(token, name, files, etag string) *httptest.ResponseRecorder {
		r := httptest.NewRequestWithContext(ctx, http.MethodGet, "/api/v1/certificates/"+name+"/bundle.tar?files="+files, nil)
		r.Header.Set("Authorization", "Bearer "+token)
		r.Header.Set("If-None-Match", etag)

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)

		return w
	}

	first := request(fullToken, "home", "fullchain.crt,private.key", "")
	if first.Code != http.StatusOK {
		t.Fatalf("bundle: %d %s", first.Code, first.Body.String())
	}

	old, err := repos.Certificates.CurrentVersion(ctx, "home")
	if err != nil {
		t.Fatal(err)
	}

	expected := map[string][]byte{}
	for _, file := range []string{fullChainArtifact, privateKeyArtifact} {
		expected[file], err = manager.ReadFile(old, file)
		if err != nil {
			t.Fatal(err)
		}
	}

	assertBundle := func(contents []byte, expected map[string][]byte) {
		t.Helper()

		reader := tar.NewReader(bytes.NewReader(contents))
		count := 0

		for {
			header, readErr := reader.Next()
			if errors.Is(readErr, io.EOF) {
				break
			}

			if readErr != nil {
				t.Fatal(readErr)
			}

			content, readErr := io.ReadAll(reader)
			if readErr != nil {
				t.Fatal(readErr)
			}

			want, exists := expected[header.Name]
			if !exists || !bytes.Equal(content, want) {
				t.Fatalf("unexpected bundle entry %s", header.Name)
			}

			if header.Name == privateKeyArtifact && header.Mode != 0600 {
				t.Fatal("private key archive permissions")
			}

			count++
		}

		if count != len(expected) {
			t.Fatal("bundle missing entries")
		}
	}
	assertBundle(first.Body.Bytes(), expected)

	reordered := request(fullToken, "home", "private.key,fullchain.crt", "")
	if reordered.Header().Get("ETag") != first.Header().Get("ETag") || !bytes.Equal(reordered.Body.Bytes(), first.Body.Bytes()) {
		t.Fatal("canonical bundle cache key changed")
	}

	unchanged := request(fullToken, "home", "fullchain.crt,private.key", first.Header().Get("ETag"))
	if unchanged.Code != http.StatusNotModified || unchanged.Body.Len() != 0 {
		t.Fatal("unchanged bundle downloaded")
	}

	for _, tc := range []struct {
		token, name, files string
		status             int
	}{
		{publicToken, "home", "fullchain.crt,private.key", http.StatusForbidden},
		{publicToken, "home", "fullchain.crt", http.StatusOK},
		{fullToken, "other", "fullchain.crt", http.StatusForbidden},
		{fullToken, "home", "private.key,private.key", http.StatusBadRequest},
		{fullToken, "home", "../private.key", http.StatusBadRequest},
		{fullToken, "home", "", http.StatusBadRequest},
	} {
		if w := request(tc.token, tc.name, tc.files, ""); w.Code != tc.status {
			t.Fatalf("%s %s: got %d, want %d", tc.name, tc.files, w.Code, tc.status)
		}
	}

	if err = manager.Issue(ctx, "home", service.IssueKindManual); err != nil {
		t.Fatal(err)
	}
	// Renewal between resolving a version and reading entries must not change it.
	api := &API{manager: manager}

	pinned, err := api.buildBundle(old, []string{fullChainArtifact, privateKeyArtifact})
	if err != nil {
		t.Fatal(err)
	}

	assertBundle(pinned, expected)

	renewed := request(fullToken, "home", "fullchain.crt,private.key", first.Header().Get("ETag"))
	if renewed.Code != http.StatusOK || renewed.Header().Get("ETag") == first.Header().Get("ETag") {
		t.Fatal("renewal did not invalidate bundle validator")
	}

	current, err := repos.Certificates.CurrentVersion(ctx, "home")
	if err != nil {
		t.Fatal(err)
	}

	for file := range expected {
		expected[file], err = manager.ReadFile(current, file)
		if err != nil {
			t.Fatal(err)
		}
	}

	assertBundle(renewed.Body.Bytes(), expected)

	disabled := false

	cfg.Certificates[0].Enabled = &disabled
	if err = repos.Certificates.Reconcile(ctx, cfg); err != nil {
		t.Fatal(err)
	}

	if w := request(fullToken, "home", "fullchain.crt,private.key", renewed.Header().Get("ETag")); w.Code != http.StatusNotFound {
		t.Fatal("cached disabled bundle accessible")
	}
}

func TestBundleCacheBuildsOnceAndEvicts(t *testing.T) {
	var cache bundleCache

	builds := 0

	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			_, _ = cache.get("same", func() ([]byte, error) { builds++; return []byte("bundle"), nil })
		})
	}

	wg.Wait()

	if builds != 1 {
		t.Fatalf("bundle built %d times", builds)
	}

	for _, key := range []string{"one", "two", "three"} {
		if _, err := cache.get(key, func() ([]byte, error) { return []byte(strings.Repeat("x", bundleCacheLimit/2)), nil }); err != nil {
			t.Fatal(err)
		}
	}

	if cache.size > bundleCacheLimit || len(cache.entries) != 2 {
		t.Fatal("bundle cache not bounded")
	}
}
