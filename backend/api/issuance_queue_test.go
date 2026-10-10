package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"

	"github.com/certvault/certvault/config"
	"github.com/certvault/certvault/database"
	"github.com/certvault/certvault/database/repository"
	"github.com/certvault/certvault/service"
)

func TestRenewalAdmissionReturnsDurableScopedJob(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	disabled := false

	cfg := &config.Config{DataDir: dir, MasterKey: make([]byte, 32), ACME: config.ACME{Mock: true}}
	for i := range repository.MaxPendingIssuanceJobs + 1 {
		cfg.Certificates = append(cfg.Certificates, config.Certificate{Name: fmt.Sprintf("cert-%d", i), Domains: []string{"example.com"}})
	}

	cfg.Certificates = append(cfg.Certificates, config.Certificate{Name: "disabled", Enabled: &disabled, Domains: []string{"example.com"}})

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

	handler, err := New(cfg, db, repos, manager)
	if err != nil {
		t.Fatal(err)
	}

	_, token, err := repos.APIKeys.Create(ctx, "renew", []string{scopeRenewalsTrigger}, []string{"*"}, nil)
	if err != nil {
		t.Fatal(err)
	}

	_, otherToken, err := repos.APIKeys.Create(ctx, "other", []string{scopeRenewalsTrigger}, []string{"cert-1"}, nil)
	if err != nil {
		t.Fatal(err)
	}

	request := func(method, path, key string) *httptest.ResponseRecorder {
		r := httptest.NewRequestWithContext(ctx, method, "/api/v1/"+path, nil)
		r.Header.Set("Authorization", "Bearer "+key)

		w := httptest.NewRecorder()
		handler.ServeHTTP(w, r)

		return w
	}

	var wg sync.WaitGroup

	ids := make(chan int64, 50)

	for range 50 {
		wg.Go(func() {
			w := request(http.MethodPost, "certificates/cert-0/renew", token)
			if w.Code != http.StatusAccepted {
				t.Errorf("renew: %d %s", w.Code, w.Body.String())
				return
			}

			var response struct {
				ID     int64  `json:"job_id"`
				Status string `json:"status"`
			}
			if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
				t.Error(err)
				return
			}

			if response.ID == 0 || response.Status != "queued" || w.Header().Get("Location") != fmt.Sprintf("/api/v1/jobs/%d", response.ID) {
				t.Errorf("invalid admission: %s", w.Body.String())
			}

			ids <- response.ID
		})
	}

	wg.Wait()
	close(ids)

	var id int64
	for actual := range ids {
		if id == 0 {
			id = actual
		}

		if actual != id {
			t.Fatal("duplicate renewal queued another job")
		}
	}

	if id == 0 {
		t.Fatal("burst returned no durable job")
	}

	path := fmt.Sprintf("jobs/%d", id)
	if w := request(http.MethodGet, path, token); w.Code != http.StatusOK {
		t.Fatalf("job status: %d %s", w.Code, w.Body.String())
	}

	if w := request(http.MethodGet, path, otherToken); w.Code != http.StatusForbidden {
		t.Fatal("job crossed certificate allowlist")
	}

	for _, name := range []string{"missing", "disabled"} {
		if w := request(http.MethodPost, "certificates/"+name+"/renew", token); w.Code != http.StatusNotFound {
			t.Fatalf("%s admission: %d", name, w.Code)
		}
	}

	for i := 1; i < repository.MaxPendingIssuanceJobs; i++ {
		if w := request(http.MethodPost, fmt.Sprintf("certificates/cert-%d/renew", i), token); w.Code != http.StatusAccepted {
			t.Fatalf("admit %d: %d", i, w.Code)
		}
	}

	if w := request(http.MethodPost, fmt.Sprintf("certificates/cert-%d/renew", repository.MaxPendingIssuanceJobs), token); w.Code != http.StatusTooManyRequests || w.Header().Get("Retry-After") == "" {
		t.Fatalf("full queue: %d %s", w.Code, w.Body.String())
	}

	if w := request(http.MethodPost, "certificates/cert-0/renew", token); w.Code != http.StatusAccepted {
		t.Fatal("duplicate rejected at capacity")
	}

	jobs, err := repos.Jobs.List(ctx, 100)
	if err != nil || len(jobs) != repository.MaxPendingIssuanceJobs {
		t.Fatalf("unbounded queue: %d %v", len(jobs), err)
	}
}
