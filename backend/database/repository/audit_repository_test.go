package repository

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/certvault/certvault/database"
)

func TestAuditSearchFiltersAndPaginates(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = db.Close() }()

	audits := New(db).Audits
	ctx := context.Background()
	audits.Record(ctx, "admin", "api_key.create", "deploy", "created key", "192.0.2.1")
	audits.Record(ctx, "node", "certificate.download", "example.com", "fullchain.pem", "192.0.2.2")
	audits.Record(ctx, "admin", "certificate.renew", "example.com", "", "192.0.2.1")

	page, err := audits.Search(ctx, AuditFilter{Actors: []string{"admin"}, Page: 1, PerPage: 1})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || len(page.Items) != 1 || page.Items[0].Action != "certificate.renew" {
		t.Fatalf("unexpected first page: %#v", page)
	}
	page, err = audits.Search(ctx, AuditFilter{Actions: []string{"api_key.create", "certificate.download"}, Page: 1, PerPage: 25})
	if err != nil || page.Total != 2 {
		t.Fatalf("unexpected multi-action filter: %#v %v", page, err)
	}
	page, err = audits.Search(ctx, AuditFilter{Resources: []string{"deploy", "example.com"}, Page: 1, PerPage: 25})
	if err != nil || page.Total != 3 {
		t.Fatalf("unexpected multi-resource filter: %#v %v", page, err)
	}
	actors, actions, resources, err := audits.FilterOptions(ctx)
	if err != nil || len(actors) != 2 || len(actions) != 3 || len(resources) != 2 {
		t.Fatalf("unexpected filter options: actors=%#v actions=%#v resources=%#v err=%v", actors, actions, resources, err)
	}

	page, err = audits.Search(ctx, AuditFilter{Query: "fullchain", Page: 1, PerPage: 25})
	if err != nil || page.Total != 1 || page.Items[0].Actor != "node" {
		t.Fatalf("unexpected text search: %#v %v", page, err)
	}
	page, err = audits.Search(ctx, AuditFilter{Query: "certificate.download", Page: 1, PerPage: 25})
	if err != nil || page.Total != 1 || page.Items[0].Actor != "node" {
		t.Fatalf("action was not included in text search: %#v %v", page, err)
	}
	page, err = audits.Search(ctx, AuditFilter{Query: "node", Page: 1, PerPage: 25})
	if err != nil || page.Total != 1 || page.Items[0].Action != "certificate.download" {
		t.Fatalf("actor was not included in text search: %#v %v", page, err)
	}

	page, err = audits.Search(ctx, AuditFilter{Query: "%", Page: 1, PerPage: 25})
	if err != nil || page.Total != 0 {
		t.Fatalf("wildcard was not escaped: %#v %v", page, err)
	}

	old := database.AuditEvent{At: time.Now().Add(-48 * time.Hour), Actor: "system", Action: "old", Resource: "test"}
	if err = db.ORM().Create(&old).Error; err != nil {
		t.Fatal(err)
	}
	deleted, err := audits.DeleteBefore(ctx, time.Now().Add(-24*time.Hour))
	if err != nil || deleted != 1 {
		t.Fatalf("unexpected audit cleanup: deleted=%d err=%v", deleted, err)
	}
	page, err = audits.Search(ctx, AuditFilter{Page: 1, PerPage: 25})
	if err != nil || page.Total != 3 {
		t.Fatalf("cleanup removed retained events: %#v %v", page, err)
	}
}
