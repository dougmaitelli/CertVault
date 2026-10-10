package repository

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/certvault/certvault/database"
	"gorm.io/gorm"
)

func seedCertificateLists(tb testing.TB, db *database.Database, count int) {
	tb.Helper()

	now := time.Now().UTC()

	err := db.ORM().Transaction(func(tx *gorm.DB) error {
		for i := range count {
			cert := database.Certificate{Name: fmt.Sprintf("cert-%04d", i), Domains: `["desired.example.com"]`, KeyType: "ec384", Enabled: true, UpdatedAt: now}
			if err := tx.Create(&cert).Error; err != nil {
				return err
			}

			for j := range 3 {
				// Equal timestamps exercise deterministic ID ordering.
				v := database.CertificateVersion{CertificateID: cert.ID, Domains: `["issued.example.com"]`, KeyType: "ec256", CreatedAt: now, NotAfter: now.Add(90 * 24 * time.Hour), Path: fmt.Sprintf("version-%d", j)}
				if err := tx.Create(&v).Error; err != nil {
					return err
				}

				job := database.Job{CertificateID: cert.ID, Kind: "manual", Status: "succeeded", StartedAt: now}
				if err := tx.Create(&job).Error; err != nil {
					return err
				}
			}
		}

		return nil
	})
	if err != nil {
		tb.Fatal(err)
	}
}

func TestCertificateListBatchesVersionsAndJobs(t *testing.T) {
	ctx := context.Background()

	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = db.Close() }()

	seedCertificateLists(t, db, 100)

	var queries atomic.Int64

	if err = db.ORM().Callback().Query().After("gorm:query").Register("test:count_queries", func(tx *gorm.DB) {
		if !tx.DryRun {
			queries.Add(1)
		}
	}); err != nil {
		t.Fatal(err)
	}

	certificates, err := New(db).Certificates.List(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if len(certificates) != 100 || queries.Load() != 3 {
		t.Fatalf("list performed %d queries for %d certificates", queries.Load(), len(certificates))
	}

	for _, cert := range certificates {
		v := cert.CurrentVersion
		if v == nil || v.Path != "version-2" || v.CertificateName != cert.Name || v.KeyType != "ec256" || v.Domains[0] != "issued.example.com" {
			t.Fatalf("incorrect current version for %q: %#v", cert.Name, v)
		}

		if cert.LatestJob == nil || cert.LatestJob.CertificateName != cert.Name || cert.LatestJob.ID != v.ID {
			t.Fatalf("incorrect latest job for %q: %#v", cert.Name, cert.LatestJob)
		}
		// Match the single-certificate lookup, including timestamp ties.
		single, err := New(db).Certificates.CurrentVersion(ctx, cert.Name)
		if err != nil || single.ID != v.ID {
			t.Fatalf("batch and download disagree: %#v %v", single, err)
		}
	}
}

func BenchmarkCertificateList(b *testing.B) {
	for _, count := range []int{10, 100, 1000} {
		b.Run(fmt.Sprintf("certificates=%d", count), func(b *testing.B) {
			db, err := database.Open(filepath.Join(b.TempDir(), "test.db"))
			if err != nil {
				b.Fatal(err)
			}
			defer func() { _ = db.Close() }()

			seedCertificateLists(b, db, count)
			repos := New(db)

			for _, clients := range []int{1, 8} {
				b.Run(fmt.Sprintf("clients=%d", clients), func(b *testing.B) {
					b.ReportAllocs()

					if clients == 1 {
						for b.Loop() {
							if _, err := repos.Certificates.List(context.Background()); err != nil {
								b.Fatal(err)
							}
						}
					} else {
						var (
							next    atomic.Int64
							workers sync.WaitGroup
						)
						for range clients {
							workers.Go(func() {
								for next.Add(1) <= int64(b.N) {
									if _, err := repos.Certificates.List(context.Background()); err != nil {
										b.Error(err)
									}
								}
							})
						}

						workers.Wait()
					}
				})
			}
		})
	}
}

func TestAPIKeyUsageIsSampledWithoutCachingAuthorization(t *testing.T) {
	ctx := context.Background()

	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}

	defer func() { _ = db.Close() }()

	repos := New(db)

	key, token, err := repos.APIKeys.Create(ctx, "client", []string{"*"}, []string{"*"}, nil)
	if err != nil {
		t.Fatal(err)
	}

	var writes int

	if err = db.ORM().Callback().Update().After("gorm:update").Register("test:count_usage", func(tx *gorm.DB) {
		if tx.Statement.Table == "api_keys" {
			writes++
		}
	}); err != nil {
		t.Fatal(err)
	}

	for range 25 {
		if _, err = repos.APIKeys.Authenticate(ctx, token, "192.0.2.1"); err != nil {
			t.Fatal(err)
		}
	}

	if writes != 1 {
		t.Fatalf("usage writes = %d, want 1", writes)
	}

	old := time.Now().Add(-2 * time.Minute)
	if err = db.ORM().Model(&database.APIKey{}).Where("id = ?", key.ID).Update("last_used_at", old).Error; err != nil {
		t.Fatal(err)
	}

	before := writes

	if _, err = repos.APIKeys.Authenticate(ctx, token, "192.0.2.2"); err != nil {
		t.Fatal(err)
	}

	keys, err := repos.APIKeys.List(ctx)
	if err != nil || writes != before+1 || keys[0].LastUsedIP != "192.0.2.2" || !keys[0].LastUsedAt.After(old) {
		t.Fatalf("usage did not refresh: %#v %v", keys, err)
	}

	if _, err = repos.APIKeys.Revoke(ctx, key.ID); err != nil {
		t.Fatal(err)
	}

	if _, err = repos.APIKeys.Authenticate(ctx, token, "192.0.2.2"); err == nil {
		t.Fatal("usage sampling cached authorization after revocation")
	}
}
