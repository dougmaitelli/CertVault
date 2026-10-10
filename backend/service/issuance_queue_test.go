package service

import (
	"context"
	"io"
	"log/slog"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/certvault/certvault/config"
	"github.com/certvault/certvault/database"
	"github.com/certvault/certvault/database/repository"
)

func waitFinishedJob(t *testing.T, repos *repository.Repositories, id int64) repository.Job {
	t.Helper()

	deadline := time.NewTimer(5 * time.Second)
	defer deadline.Stop()

	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()

	for {
		job, err := repos.Jobs.Get(context.Background(), id)
		if err != nil {
			t.Fatal(err)
		}

		if job.FinishedAt != nil {
			return job
		}

		select {
		case <-deadline.C:
			t.Fatalf("job %d never finished", id)
		case <-tick.C:
		}
	}
}

func TestWorkerResumesQueuedJobsAfterDatabaseReopen(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	cfg := &config.Config{DataDir: dir, MasterKey: make([]byte, 32), ACME: config.ACME{Mock: true}, Certificates: []config.Certificate{
		{Name: "queued", Domains: []string{"example.com"}}, {Name: "interrupted", Domains: []string{"other.example.com"}},
	}}

	db, err := database.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}

	repos := repository.New(db)
	if err = repos.Certificates.Reconcile(ctx, cfg); err != nil {
		t.Fatal(err)
	}

	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	manager, err := NewManager(cfg, repos, logger)
	if err != nil {
		t.Fatal(err)
	}

	queued, _, err := manager.Enqueue(ctx, "queued", IssueKindManual)
	if err != nil {
		t.Fatal(err)
	}

	interrupted, err := repos.Jobs.Start(ctx, "interrupted", "manual")
	if err != nil {
		t.Fatal(err)
	}

	if err = db.Close(); err != nil {
		t.Fatal(err)
	}

	db, err = database.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	repos = repository.New(db)

	manager, err = NewManager(cfg, repos, logger)
	if err != nil {
		t.Fatal(err)
	}

	runCtx, cancel := context.WithCancel(ctx)

	done := make(chan struct{})
	go func() { defer close(done); manager.Run(runCtx) }()

	t.Cleanup(func() { cancel(); <-done })

	job := waitFinishedJob(t, repos, queued.ID)
	if job.Status != "succeeded" {
		t.Fatalf("resumed job: %#v", job)
	}

	failed := waitFinishedJob(t, repos, interrupted)
	if failed.Status != "failed" || !strings.Contains(failed.Error, "external ACME outcome") {
		t.Fatalf("interrupted job: %#v", failed)
	}

	if _, err = repos.Certificates.CurrentVersion(ctx, "queued"); err != nil {
		t.Fatal(err)
	}

	if _, err = repos.Certificates.CurrentVersion(ctx, "interrupted"); !repository.NotFound(err) {
		t.Fatal("interrupted issuance was repeated")
	}

	jobs, err := repos.Jobs.List(ctx, 10)
	if err != nil || len(jobs) != 2 {
		t.Fatalf("recovery created extra jobs: %#v %v", jobs, err)
	}
}

func TestScheduledIssuanceDeduplicatesAndSkipsInterrupted(t *testing.T) {
	ctx := context.Background()

	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	repos := repository.New(db)

	cfg := &config.Config{ACME: config.ACME{AutomaticIssuance: true}, Certificates: []config.Certificate{
		{Name: "pending", Domains: []string{"example.com"}}, {Name: "interrupted", Domains: []string{"other.example.com"}},
	}}
	if err = repos.Certificates.Reconcile(ctx, cfg); err != nil {
		t.Fatal(err)
	}

	manager, err := NewManager(cfg, repos, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}

	queued, _, err := manager.Enqueue(ctx, "pending", IssueKindManual)
	if err != nil {
		t.Fatal(err)
	}

	if _, err = repos.Jobs.Start(ctx, "interrupted", "scheduled"); err != nil {
		t.Fatal(err)
	}

	if err = repos.Jobs.RecoverInterrupted(ctx); err != nil {
		t.Fatal(err)
	}

	manager.reconcile(ctx)
	manager.reconcile(ctx)

	jobs, err := repos.Jobs.List(ctx, 10)
	if err != nil || len(jobs) != 2 {
		t.Fatalf("scheduler queued duplicates: %#v %v", jobs, err)
	}

	job, err := repos.Jobs.Get(ctx, queued.ID)
	if err != nil || job.Kind != "manual" || job.Status != "queued" {
		t.Fatalf("scheduler replaced manual request: %#v %v", job, err)
	}
}

func TestWorkerRejectsCertificateDisabledWhileQueued(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	cfg := &config.Config{DataDir: dir, MasterKey: make([]byte, 32), ACME: config.ACME{Mock: true}, Certificates: []config.Certificate{{Name: "home", Domains: []string{"example.com"}}}}

	db, err := database.Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })

	repos := repository.New(db)
	if err = repos.Certificates.Reconcile(ctx, cfg); err != nil {
		t.Fatal(err)
	}

	manager, err := NewManager(cfg, repos, slog.New(slog.NewTextHandler(io.Discard, nil)))
	if err != nil {
		t.Fatal(err)
	}

	queued, _, err := manager.Enqueue(ctx, "home", IssueKindManual)
	if err != nil {
		t.Fatal(err)
	}

	disabled := false

	cfg.Certificates[0].Enabled = &disabled
	if err = repos.Certificates.Reconcile(ctx, cfg); err != nil {
		t.Fatal(err)
	}

	runCtx, cancel := context.WithCancel(ctx)

	done := make(chan struct{})
	go func() { defer close(done); manager.Run(runCtx) }()

	t.Cleanup(func() { cancel(); <-done })

	job := waitFinishedJob(t, repos, queued.ID)
	if job.Status != "failed" {
		t.Fatalf("disabled job ran: %#v", job)
	}

	var versions int64
	if err = db.ORM().WithContext(ctx).Model(&database.CertificateVersion{}).Count(&versions).Error; err != nil || versions != 0 {
		t.Fatalf("disabled queued certificate issued: %d %v", versions, err)
	}
}
