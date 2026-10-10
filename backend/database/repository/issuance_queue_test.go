package repository

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/certvault/certvault/config"
	"github.com/certvault/certvault/database"
)

func TestIssuanceAdmissionBoundsAndDeduplicates(t *testing.T) {
	ctx := context.Background()

	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	repos := New(db)

	cfg := &config.Config{}
	for i := range MaxPendingIssuanceJobs + 1 {
		cfg.Certificates = append(cfg.Certificates, config.Certificate{Name: fmt.Sprintf("cert-%d", i), Domains: []string{"example.com"}})
	}

	if err = repos.Certificates.Reconcile(ctx, cfg); err != nil {
		t.Fatal(err)
	}

	var (
		created atomic.Int64
		wg      sync.WaitGroup
	)

	ids := make(chan int64, 100)

	for range 100 {
		wg.Go(func() {
			job, fresh, admitErr := repos.Jobs.Admit(ctx, "cert-0", "manual")
			if admitErr != nil {
				t.Errorf("admission: %v", admitErr)
				return
			}

			if fresh {
				created.Add(1)
			}

			ids <- job.ID
		})
	}

	wg.Wait()
	close(ids)

	var expected int64
	for id := range ids {
		if expected == 0 {
			expected = id
		}

		if id != expected {
			t.Fatal("burst created different job IDs")
		}
	}

	if created.Load() != 1 {
		t.Fatalf("created %d jobs", created.Load())
	}

	claimed, err := repos.Jobs.Claim(ctx)
	if err != nil || claimed == nil || claimed.ID != expected || claimed.Status != "running" {
		t.Fatalf("claim: %#v %v", claimed, err)
	}

	for i := 1; i < MaxPendingIssuanceJobs; i++ {
		if _, fresh, admitErr := repos.Jobs.Admit(ctx, fmt.Sprintf("cert-%d", i), "scheduled"); admitErr != nil || !fresh {
			t.Fatalf("admit %d: %v", i, admitErr)
		}
	}

	if _, _, err = repos.Jobs.Admit(ctx, fmt.Sprintf("cert-%d", MaxPendingIssuanceJobs), "manual"); !errors.Is(err, ErrIssuanceQueueFull) {
		t.Fatalf("queue limit: %v", err)
	}

	duplicate, fresh, err := repos.Jobs.Admit(ctx, "cert-0", "scheduled")
	if err != nil || fresh || duplicate.ID != expected || duplicate.Status != "running" {
		t.Fatalf("running dedup at capacity: %#v %v", duplicate, err)
	}

	if err = repos.Jobs.Finish(ctx, expected, nil); err != nil {
		t.Fatal(err)
	}

	if _, fresh, err = repos.Jobs.Admit(ctx, fmt.Sprintf("cert-%d", MaxPendingIssuanceJobs), "manual"); err != nil || !fresh {
		t.Fatalf("slot not released: %v", err)
	}

	if _, _, err = repos.Jobs.Admit(ctx, "missing", "manual"); !NotFound(err) {
		t.Fatalf("missing accepted: %v", err)
	}
}
