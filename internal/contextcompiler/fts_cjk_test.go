package contextcompiler

import (
	"context"
	"testing"
)

func TestCJKFTSBackfillBoundaryAndUpdate(t *testing.T) {
	ctx := context.Background()
	db := openFTSTestDB(t)
	store := NewFTSStore(db)
	fixture := Document{ID: "old", ProjectID: "p1", Kind: "chapter", Content: "张三在青云宗获得玄铁剑", SourceChapter: 4, SourceVersion: "v1"}
	if err := store.Upsert(ctx, fixture); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(CJKMigration().SQL); err != nil {
		t.Fatal(err)
	}
	for _, query := range []string{"张三", "青云宗", "玄铁剑", "张三 玄铁剑"} {
		items, err := store.Collect(ctx, Request{ProjectID: "p1", Chapter: 4, Query: query})
		if err != nil || len(items) != 1 || items[0].Content != fixture.Content {
			t.Fatalf("query=%q items=%v error=%v", query, items, err)
		}
	}
	for _, request := range []Request{{ProjectID: "p1", Chapter: 3, Query: "张三"}, {ProjectID: "p2", Chapter: 4, Query: "张三"}, {ProjectID: "p1", Chapter: 4, Query: "三张"}} {
		items, err := store.Collect(ctx, request)
		if err != nil || len(items) != 0 {
			t.Fatalf("boundary/phrase leak: %v %v", items, err)
		}
	}
	fixture.Content = "李四回到山门"
	fixture.SourceVersion = "v2"
	if err := store.Upsert(ctx, fixture); err != nil {
		t.Fatal(err)
	}
	items, err := store.Collect(ctx, Request{ProjectID: "p1", Chapter: 4, Query: "张三"})
	if err != nil || len(items) != 0 {
		t.Fatalf("stale index after update: %v %v", items, err)
	}
	if err := store.Delete(ctx, "p1", fixture.ID); err != nil {
		t.Fatal(err)
	}
	items, err = store.Collect(ctx, Request{ProjectID: "p1", Chapter: 4, Query: "李四"})
	if err != nil || len(items) != 0 {
		t.Fatalf("stale index after delete: %v %v", items, err)
	}
}
