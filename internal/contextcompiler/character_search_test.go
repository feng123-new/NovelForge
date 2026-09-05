package contextcompiler

import (
	"context"
	"testing"
)

func TestCharacterSearchNamesBoundaryAndRefresh(t *testing.T) {
	ctx := context.Background()
	s := NewFTSStore(openFTSTestDB(t))
	for _, d := range []Document{
		{ID: "past", ProjectID: "p1", Kind: "chapter_final", Content: "张三在青云宗获得玄铁剑", SourceChapter: 1, SourceVersion: "v1"},
		{ID: "future", ProjectID: "p1", Kind: "chapter_final", Content: "张三在青云宗获得玄铁剑", SourceChapter: 3, SourceVersion: "v1"},
		{ID: "other", ProjectID: "p2", Kind: "chapter_final", Content: "张三在青云宗获得玄铁剑", SourceChapter: 1, SourceVersion: "v1"},
	} {
		if err := s.Upsert(ctx, d); err != nil { t.Fatal(err) }
	}
	for _, q := range []string{"张三", "青云宗", "玄铁剑", "张三 青云宗"} {
		items, err := s.Collect(ctx, Request{ProjectID: "p1", Chapter: 2, Query: q})
		if err != nil || len(items) != 1 || items[0].ID != "past" { t.Fatalf("query %q: items=%v err=%v", q, items, err) }
		if items[0].Content != "张三在青云宗获得玄铁剑" { t.Fatal("raw content changed") }
	}
	if err := s.Upsert(ctx, Document{ID: "past", ProjectID: "p1", Kind: "chapter_final", Content: "李四返回长安", SourceChapter: 1, SourceVersion: "v2"}); err != nil { t.Fatal(err) }
	items, err := s.Collect(ctx, Request{ProjectID: "p1", Chapter: 2, Query: "张三"})
	if err != nil || len(items) != 0 { t.Fatalf("stale index: %v %v", items, err) }
	if err := s.Delete(ctx, "p1", "past"); err != nil { t.Fatal(err) }
	items, err = s.Collect(ctx, Request{ProjectID: "p1", Chapter: 2, Query: "李四"})
	if err != nil || len(items) != 0 { t.Fatalf("deleted row: %v %v", items, err) }
}
