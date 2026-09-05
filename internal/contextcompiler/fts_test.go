package contextcompiler

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/voocel/ainovel-cli/internal/db/migrate"
)

func openFTSTestDB(t *testing.T) *sql.DB {
	t.Helper()
	path := filepath.Join(t.TempDir(), "context.db")
	runner := migrate.Runner{Path: path, Migrations: []migrate.Migration{Migration(), CharacterSearchMigration()}, BusyTimeout: time.Second}
	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	db, err := migrate.Open(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return db
}

func TestFTS5ChapterBoundaryStableOrderAndUpsert(t *testing.T) {
	ctx := context.Background()
	store := NewFTSStore(openFTSTestDB(t))
	now := time.Date(2026, 9, 2, 0, 0, 0, 0, time.UTC)
	fixtures := []Document{
		{ID: "past-b", ProjectID: "p1", Kind: "chapter_summary", Title: "sealed gate", Content: "the sealed gate remains closed", SourceChapter: 40, SourceVersion: "v1", Priority: 2, CreatedAt: now},
		{ID: "past-a", ProjectID: "p1", Kind: "chapter_summary", Title: "sealed gate", Content: "the sealed gate was found", SourceChapter: 40, SourceVersion: "v1", Priority: 1, CreatedAt: now},
		{ID: "future", ProjectID: "p1", Kind: "chapter_summary", Title: "sealed gate", Content: "the sealed gate opens", SourceChapter: 60, SourceVersion: "v1", Priority: 9, CreatedAt: now},
		{ID: "other", ProjectID: "p2", Kind: "chapter_summary", Title: "sealed gate", Content: "other project", SourceChapter: 10, SourceVersion: "v1", CreatedAt: now},
	}
	for _, fixture := range fixtures {
		if err := store.Upsert(ctx, fixture); err != nil {
			t.Fatal(err)
		}
	}
	items, err := store.Collect(ctx, Request{ProjectID: "p1", Chapter: 50, Query: "sealed gate"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 2 {
		t.Fatalf("items=%d want=2: %+v", len(items), items)
	}
	for _, item := range items {
		if item.ID == "future" || item.ID == "other" {
			t.Fatalf("boundary leak: %+v", items)
		}
		if item.Stage != StageFTS5 || item.Layer != LayerHistorical {
			t.Fatalf("bad classification: %+v", item)
		}
	}
	if err := store.Upsert(ctx, Document{ID: "past-a", ProjectID: "p1", Kind: "chapter_summary", Title: "different", Content: "no matching phrase", SourceChapter: 40, SourceVersion: "v2", CreatedAt: now}); err != nil {
		t.Fatal(err)
	}
	items, err = store.Collect(ctx, Request{ProjectID: "p1", Chapter: 50, Query: "sealed gate"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].ID != "past-b" {
		t.Fatalf("upsert index stale: %+v", items)
	}
}

func TestFTSDeleteAndQueryEscaping(t *testing.T) {
	ctx := context.Background()
	store := NewFTSStore(openFTSTestDB(t))
	if err := store.Upsert(ctx, Document{ID: "d1", ProjectID: "p", Kind: "summary", Title: "a+b", Content: "quoted phrase", SourceVersion: "v1"}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Collect(ctx, Request{ProjectID: "p", Chapter: 0, Query: `quoted "phrase" +`}); err != nil {
		t.Fatalf("escaped search: %v", err)
	}
	if err := store.Delete(ctx, "p", "d1"); err != nil {
		t.Fatal(err)
	}
	items, err := store.Collect(ctx, Request{ProjectID: "p", Chapter: 0, Query: "quoted"})
	if err != nil {
		t.Fatal(err)
	}
	if len(items) != 0 {
		t.Fatalf("deleted item returned: %+v", items)
	}
}

func TestMigrationUsesProjectChapterIndex(t *testing.T) {
	db := openFTSTestDB(t)
	rows, err := db.Query(`EXPLAIN QUERY PLAN SELECT id FROM context_documents WHERE project_id=? AND source_chapter<=? ORDER BY source_chapter DESC, kind, id LIMIT 20`, "p", 50)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	found := false
	for rows.Next() {
		var id, parent, unused int
		var detail string
		if err := rows.Scan(&id, &parent, &unused, &detail); err != nil {
			t.Fatal(err)
		}
		if contains(detail, "idx_context_documents_project_chapter") {
			found = true
		}
	}
	if !found {
		t.Fatal("query plan did not use idx_context_documents_project_chapter")
	}
}

func contains(value, part string) bool {
	for i := 0; i+len(part) <= len(value); i++ {
		if value[i:i+len(part)] == part {
			return true
		}
	}
	return false
}
