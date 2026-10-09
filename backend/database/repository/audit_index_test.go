package repository

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/certvault/certvault/database"
	"gorm.io/gorm"
)

type auditQuery struct {
	statement string
	values    []any
}

func TestAuditQueriesUseIndexes(t *testing.T) {
	db, err := database.Open(filepath.Join(t.TempDir(), "audit.db"))
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() { _ = db.Close() })
	seedAuditVolume(t, db, 1_000)
	audits := New(db).Audits
	ctx := context.Background()

	var statements []auditQuery

	capture := func(tx *gorm.DB) {
		statements = append(statements, auditQuery{
			statement: tx.Statement.SQL.String(),
			values:    append([]any(nil), tx.Statement.Vars...),
		})
	}
	if err = db.ORM().Callback().Query().After("gorm:query").Register("test:capture_audit_query", capture); err != nil {
		t.Fatal(err)
	}

	if err = db.ORM().Callback().Delete().After("gorm:delete").Register("test:capture_audit_delete", capture); err != nil {
		t.Fatal(err)
	}

	checkPlan := func(t *testing.T, query auditQuery, index string) {
		t.Helper()

		var plan []struct{ Detail string }
		if err := db.ORM().Raw("EXPLAIN QUERY PLAN "+query.statement, query.values...).Scan(&plan).Error; err != nil {
			t.Fatal(err)
		}

		details := make([]string, 0, len(plan))
		for _, row := range plan {
			details = append(details, row.Detail)
		}

		detail := strings.Join(details, "; ")
		t.Logf("%s: %s", query.statement, detail)

		if !strings.Contains(detail, index) || strings.Contains(detail, "TEMP B-TREE") {
			t.Fatalf("query did not use %s without temporary sorting: %s", index, detail)
		}
	}

	for _, tc := range []struct {
		name   string
		filter AuditFilter
		index  string
	}{
		{name: "actor", filter: AuditFilter{Actors: []string{"actor-42"}}, index: "idx_audit_actor"},
		{name: "action", filter: AuditFilter{Actions: []string{"action-2"}}, index: "idx_audit_action"},
		{name: "resource", filter: AuditFilter{Resources: []string{"cert-42"}}, index: "idx_audit_resource"},
		{name: "substring with exact actor", filter: AuditFilter{Query: "fullchain", Actors: []string{"actor-42"}}, index: "idx_audit_actor"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			statements = nil
			tc.filter.Page = 1
			tc.filter.PerPage = 25

			result, err := audits.Search(ctx, tc.filter)
			if err != nil || result.Total == 0 {
				t.Fatalf("search result = %#v, error = %v", result, err)
			}

			if len(statements) != 2 {
				t.Fatalf("expected count and page queries, got %v", statements)
			}

			for _, statement := range statements {
				checkPlan(t, statement, tc.index)
			}
		})
	}

	statements = nil

	if _, _, _, err = audits.FilterOptions(ctx); err != nil {
		t.Fatal(err)
	}

	indexes := []string{"idx_audit_actor", "idx_audit_action", "idx_audit_resource"}
	if len(statements) != len(indexes) {
		t.Fatalf("expected three filter-option queries, got %v", statements)
	}

	for i, statement := range statements {
		checkPlan(t, statement, "COVERING INDEX "+indexes[i])
	}

	statements = nil

	if _, err = audits.DeleteBefore(ctx, time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}

	if len(statements) != 1 {
		t.Fatalf("expected one retention query, got %v", statements)
	}

	checkPlan(t, statements[0], "idx_audit_at")
}

func seedAuditVolume(tb testing.TB, db *database.Database, count int) {
	tb.Helper()

	events := make([]database.AuditEvent, count)
	for i := range events {
		events[i] = database.AuditEvent{
			At:       time.Date(2026, 1, 1+i%28, 0, 0, 0, 0, time.UTC),
			Actor:    fmt.Sprintf("actor-%d", i%1_000),
			Action:   fmt.Sprintf("action-%d", i%10),
			Resource: fmt.Sprintf("cert-%d", i%100),
			Detail:   "downloaded fullchain.crt",
			IP:       "192.0.2.1",
		}
	}

	if err := db.ORM().CreateInBatches(events, 100).Error; err != nil {
		tb.Fatal(err)
	}
}

func BenchmarkAuditQueries100K(b *testing.B) {
	for _, indexed := range []bool{false, true} {
		name := "unindexed"
		if indexed {
			name = "indexed"
		}

		b.Run(name, func(b *testing.B) {
			db, err := database.Open(filepath.Join(b.TempDir(), "audit.db"))
			if err != nil {
				b.Fatal(err)
			}

			b.Cleanup(func() { _ = db.Close() })

			if !indexed {
				for _, column := range []string{"at", "actor", "action", "resource"} {
					if err = db.ORM().Migrator().DropIndex(&database.AuditEvent{}, "idx_audit_"+column); err != nil {
						b.Fatal(err)
					}
				}
			}

			seedAuditVolume(b, db, 100_000)
			audits := New(db).Audits
			ctx := context.Background()

			for _, tc := range []struct {
				name   string
				filter AuditFilter
			}{
				{name: "actor", filter: AuditFilter{Actors: []string{"actor-42"}}},
				{name: "action", filter: AuditFilter{Actions: []string{"action-2"}}},
				{name: "resource", filter: AuditFilter{Resources: []string{"cert-42"}}},
				{name: "substring", filter: AuditFilter{Query: "fullchain"}},
				{name: "substring_with_actor", filter: AuditFilter{Query: "fullchain", Actors: []string{"actor-42"}}},
			} {
				b.Run(tc.name, func(b *testing.B) {
					tc.filter.Page = 1
					tc.filter.PerPage = 25

					for b.Loop() {
						if _, err := audits.Search(ctx, tc.filter); err != nil {
							b.Fatal(err)
						}
					}
				})
			}

			b.Run("filter_options", func(b *testing.B) {
				for b.Loop() {
					if _, _, _, err := audits.FilterOptions(ctx); err != nil {
						b.Fatal(err)
					}
				}
			})
		})
	}
}
