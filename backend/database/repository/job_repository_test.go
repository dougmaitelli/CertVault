package repository

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/certvault/certvault/database"
)

func TestJobSearchFiltersAndPaginates(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	home := database.Certificate{Name: "home", Domains: "home.example", KeyType: "ec256", UpdatedAt: time.Now()}
	work := database.Certificate{Name: "work", Domains: "work.example", KeyType: "ec256", UpdatedAt: time.Now()}

	if err = db.ORM().Create(&home).Error; err != nil {
		t.Fatal(err)
	}

	if err = db.ORM().Create(&work).Error; err != nil {
		t.Fatal(err)
	}

	jobs := []database.Job{
		{CertificateID: home.ID, Kind: "initial", Status: "succeeded", StartedAt: time.Now()},
		{CertificateID: home.ID, Kind: "renewal", Status: "failed", StartedAt: time.Now()},
		{CertificateID: work.ID, Kind: "renewal", Status: "succeeded", StartedAt: time.Now()},
	}
	if err = db.ORM().Create(&jobs).Error; err != nil {
		t.Fatal(err)
	}

	repository := New(db).Jobs

	result, err := repository.Search(context.Background(), JobFilter{Certificates: []string{"home"}, Kinds: []string{"renewal"}, Page: 1, PerPage: 1})
	if err != nil || result.Total != 1 || len(result.Items) != 1 || result.Items[0].Status != "failed" {
		t.Fatalf("unexpected filtered jobs: %#v %v", result, err)
	}

	result, err = repository.Search(context.Background(), JobFilter{Statuses: []string{"succeeded"}, Page: 1, PerPage: 1})
	if err != nil || result.Total != 2 || len(result.Items) != 1 || result.Items[0].CertificateName != "work" {
		t.Fatalf("unexpected paginated jobs: %#v %v", result, err)
	}

	certificates, operations, statuses, err := repository.FilterOptions(context.Background())
	if err != nil || len(certificates) != 2 || len(operations) != 2 || len(statuses) != 2 {
		t.Fatalf("unexpected options: certificates=%#v operations=%#v statuses=%#v err=%v", certificates, operations, statuses, err)
	}
}
