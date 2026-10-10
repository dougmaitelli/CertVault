package api

import (
	"archive/tar"
	"bytes"
	"crypto/sha256"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"

	"github.com/certvault/certvault/audit"
	"github.com/certvault/certvault/database/repository"
)

const bundleCacheLimit = 8 << 20

type cachedBundle struct {
	key  string
	data []byte
}

// Keep decrypted bundles only in bounded process memory, never on disk.
type bundleCache struct {
	mu      sync.Mutex
	entries []cachedBundle
	size    int
}

func (c *bundleCache) get(key string, build func() ([]byte, error)) ([]byte, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	for i, entry := range c.entries {
		if entry.key == key {
			copy(c.entries[i:], c.entries[i+1:])
			c.entries[len(c.entries)-1] = entry

			return entry.data, nil
		}
	}

	data, err := build()
	if err != nil {
		return nil, err
	}

	if len(data) > bundleCacheLimit {
		return data, nil
	}

	for c.size+len(data) > bundleCacheLimit {
		c.size -= len(c.entries[0].data)
		c.entries[0] = cachedBundle{}
		c.entries = c.entries[1:]
	}

	c.entries = append(c.entries, cachedBundle{key: key, data: data})
	c.size += len(data)

	return data, nil
}

func (a *API) downloadBundle(w http.ResponseWriter, r *http.Request) {
	id, ok := requestIdentity(w, r)
	if !ok {
		return
	}

	files := strings.Split(r.URL.Query().Get("files"), ",")
	sort.Strings(files)

	name := r.PathValue("name")

	for i, file := range files {
		scope, valid := certificateArtifacts[file]
		if !valid || (i > 0 && files[i-1] == file) {
			problem(w, http.StatusBadRequest, "invalid_files", "Select unique supported artifacts using files")
			return
		}

		if !id.Admin && !id.Principal.Allows(scope, name) {
			problem(w, http.StatusForbidden, "forbidden", "Missing scope")
			return
		}
	}

	version, err := a.repos.Certificates.CurrentVersion(r.Context(), name)
	if err != nil {
		respond(w, nil, err)
		return
	}
	// Stored versions are immutable. Resolve once and use this version for every entry.
	key := fmt.Sprintf("bundle-v1:%d:%s:%s:%s", version.ID, version.Path, version.FingerprintSHA256, strings.Join(files, ","))
	sum := sha256.Sum256([]byte(key))
	etag := fmt.Sprintf("\"%x\"", sum)
	w.Header().Set("ETag", etag)
	w.Header().Set("Cache-Control", "private, no-cache")

	if r.Header.Get("If-None-Match") == etag {
		w.WriteHeader(http.StatusNotModified)
		return
	}

	contents, err := a.bundles.get(key, func() ([]byte, error) { return a.buildBundle(version, files) })
	if err != nil {
		problem(w, http.StatusInternalServerError, "storage_error", "Unable to read certificate bundle")
		return
	}

	w.Header().Set("Content-Type", "application/x-tar")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", name+"-bundle.tar"))
	_, _ = w.Write(contents)

	_ = a.repos.Audits.Record(r.Context(), audit.Actor(id.Name), audit.ActionCertificateDownload, name, strings.Join(files, ","), a.remoteIP(r))
}

func (a *API) buildBundle(version *repository.Version, files []string) ([]byte, error) {
	var buffer bytes.Buffer

	writer := tar.NewWriter(&buffer)

	for _, file := range files {
		contents, err := a.manager.ReadFile(version, file)
		if err != nil {
			return nil, err
		}

		mode := int64(0644)
		if file == privateKeyArtifact {
			mode = 0600
		}

		if err = writer.WriteHeader(&tar.Header{Name: file, Mode: mode, Size: int64(len(contents))}); err != nil {
			return nil, err
		}

		if _, err = writer.Write(contents); err != nil {
			return nil, err
		}
	}

	if err := writer.Close(); err != nil {
		return nil, err
	}

	return buffer.Bytes(), nil
}
