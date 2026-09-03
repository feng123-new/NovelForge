package chapterversion_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/voocel/ainovel-cli/internal/chapterversion"
	"github.com/voocel/ainovel-cli/internal/project"
)

func newStore(t *testing.T) (*chapterversion.Store, string) {
	t.Helper()
	workspace := t.TempDir()
	repository, err := project.NewRepository(workspace)
	if err != nil {
		t.Fatal(err)
	}
	created, err := repository.Create(context.Background(), project.CreateInput{Title: "Phase 8"})
	if err != nil {
		t.Fatal(err)
	}
	store, err := repository.OpenChapterVersionStore(context.Background(), created.ID)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = store.Close() })
	return store, filepath.Join(workspace, filepath.FromSlash(created.Path))
}

func TestMigration6CreatesChapterVersionProductionSchema(t *testing.T) {
	store, _ := newStore(t)
	for _, name := range []string{
		"chapter_versions", "chapter_version_events", "chapter_active_finals", "chapter_revision_operations",
		"chapter_external_state", "derived_state_rebuilds", "chapter_plan_impacts", "chapter_finalize_sagas",
	} {
		var got string
		if err := store.Database().QueryRow(`SELECT name FROM sqlite_master WHERE type='table' AND name=?`, name).Scan(&got); err != nil {
			t.Fatalf("missing migration 6 table %s: %v", name, err)
		}
	}
	for _, name := range []string{
		"idx_chapter_versions_number", "idx_chapter_versions_created", "idx_chapter_versions_parent",
		"idx_chapter_versions_sha", "idx_chapter_external_pending", "idx_derived_rebuild_boundary",
	} {
		var got string
		if err := store.Database().QueryRow(`SELECT name FROM sqlite_master WHERE type='index' AND name=?`, name).Scan(&got); err != nil {
			t.Fatalf("missing migration 6 index %s: %v", name, err)
		}
	}
}

func TestImmutableVersionNumbersParentsAndRejectedHistory(t *testing.T) {
	ctx := context.Background()
	store, _ := newStore(t)
	first, err := store.Create(ctx, 7, chapterversion.CreateInput{Content: "alpha\n", Type: chapterversion.TypeDraft, AuthorType: chapterversion.AuthorWriter})
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Create(ctx, 7, chapterversion.CreateInput{Content: "alpha\nbeta\n", Type: chapterversion.TypeEditorRevision, ParentVersionID: first.ID, AuthorType: chapterversion.AuthorEditor})
	if err != nil {
		t.Fatal(err)
	}
	if first.VersionNumber != 1 || second.VersionNumber != 2 || second.ParentVersionID != first.ID {
		t.Fatalf("version chain = %#v -> %#v", first, second)
	}
	if _, err := store.Database().Exec(`UPDATE chapter_versions SET content='mutated' WHERE id=?`, first.ID); err == nil {
		t.Fatal("immutable chapter content update unexpectedly succeeded")
	}
	if _, err := store.Database().Exec(`DELETE FROM chapter_versions WHERE id=?`, first.ID); err == nil {
		t.Fatal("immutable chapter version delete unexpectedly succeeded")
	}
	service := chapterversion.Service{Store: store}
	rejected, err := service.Reject(ctx, 7, "reject-1", second.ID, "continuity mismatch")
	if err != nil {
		t.Fatal(err)
	}
	if !rejected.Rejected {
		t.Fatal("reject event was not projected")
	}
	page, err := store.List(ctx, 7, chapterversion.ListOptions{Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if page.Total != 2 || page.Versions[0].ID != second.ID || !page.Versions[0].Rejected {
		t.Fatalf("history = %#v", page)
	}
}

func TestHumanSaveAndRestoreAreIdempotentAndAppendOnly(t *testing.T) {
	ctx := context.Background()
	store, _ := newStore(t)
	final, err := store.Create(ctx, 50, chapterversion.CreateInput{Content: "Character A died.\n", Type: chapterversion.TypeFinal, AuthorType: chapterversion.AuthorSystem})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SwitchActiveFinal(ctx, 50, final.ID, chapterversion.AuthorityGeneratedFinal); err != nil {
		t.Fatal(err)
	}
	service := chapterversion.Service{Store: store}
	human1, err := service.SaveHuman(ctx, 50, "save-human-1", "Character A escaped alive.\n")
	if err != nil {
		t.Fatal(err)
	}
	human2, err := service.SaveHuman(ctx, 50, "save-human-1", "Character A escaped alive.\n")
	if err != nil {
		t.Fatal(err)
	}
	if human1.ID != human2.ID || human1.ParentVersionID != final.ID || human1.Type != chapterversion.TypeHumanRevision || human1.ActiveFinal {
		t.Fatalf("human revision replay = %#v / %#v", human1, human2)
	}
	if _, err := service.SaveHuman(ctx, 50, "save-human-1", "different content"); err == nil || !strings.Contains(err.Error(), "Idempotency-Key") {
		t.Fatalf("idempotency conflict = %v", err)
	}
	restored1, err := service.Restore(ctx, 50, "restore-1", final.ID)
	if err != nil {
		t.Fatal(err)
	}
	restored2, err := service.Restore(ctx, 50, "restore-1", final.ID)
	if err != nil {
		t.Fatal(err)
	}
	if restored1.ID != restored2.ID || restored1.ID == final.ID || restored1.ParentVersionID != final.ID || restored1.AuthorType != chapterversion.AuthorRestore || restored1.ActiveFinal {
		t.Fatalf("restore replay = %#v / %#v", restored1, restored2)
	}
	active, err := store.ActiveFinal(ctx, 50, true)
	if err != nil || active == nil || active.ID != final.ID {
		t.Fatalf("active final changed after save/restore: %#v %v", active, err)
	}
}

func TestUniqueActiveFinalAndHumanAuthorityCannotDowngrade(t *testing.T) {
	ctx := context.Background()
	store, _ := newStore(t)
	generated, err := store.Create(ctx, 9, chapterversion.CreateInput{Content: "generated\n", Type: chapterversion.TypeFinal, AuthorType: chapterversion.AuthorSystem})
	if err != nil {
		t.Fatal(err)
	}
	human, err := store.Create(ctx, 9, chapterversion.CreateInput{Content: "human\n", Type: chapterversion.TypeFinal, ParentVersionID: generated.ID, AuthorType: chapterversion.AuthorHuman})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SwitchActiveFinal(ctx, 9, generated.ID, chapterversion.AuthorityGeneratedFinal); err != nil {
		t.Fatal(err)
	}
	if err := store.SwitchActiveFinal(ctx, 9, human.ID, chapterversion.AuthorityHumanFinal); err != nil {
		t.Fatal(err)
	}
	if err := store.SwitchActiveFinal(ctx, 9, generated.ID, chapterversion.AuthorityGeneratedFinal); err == nil {
		t.Fatal("generated final downgraded Accepted Human Final authority")
	}
	var count int
	if err := store.Database().QueryRow(`SELECT COUNT(*) FROM chapter_active_finals WHERE project_id=? AND chapter=?`, store.ProjectID(), 9).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("active final count = %d", count)
	}
	active, err := store.ActiveFinal(ctx, 9, false)
	if err != nil || active == nil || active.ID != human.ID || active.Authority != chapterversion.AuthorityHumanFinal {
		t.Fatalf("active human final = %#v %v", active, err)
	}
}

func TestExternalSHAChangeIsDetectedWithoutOverwritingFinal(t *testing.T) {
	ctx := context.Background()
	store, root := newStore(t)
	final, err := store.Create(ctx, 50, chapterversion.CreateInput{Content: "Character A died.\n", Type: chapterversion.TypeFinal, AuthorType: chapterversion.AuthorSystem})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SwitchActiveFinal(ctx, 50, final.ID, chapterversion.AuthorityGeneratedFinal); err != nil {
		t.Fatal(err)
	}
	chapterPath := filepath.Join(root, "chapters", "050.md")
	if err := os.WriteFile(chapterPath, []byte("Character A died.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	status, err := store.DetectExternal(ctx, 50)
	if err != nil {
		t.Fatal(err)
	}
	if status.SyncRequired {
		t.Fatalf("matching file marked stale: %#v", status)
	}
	if err := os.WriteFile(chapterPath, []byte("Character A is severely injured and escaped alive.\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	status, err = store.DetectExternal(ctx, 50)
	if err != nil {
		t.Fatal(err)
	}
	if !status.SyncRequired || status.ExpectedSHA == status.ObservedSHA {
		t.Fatalf("external change not detected: %#v", status)
	}
	active, err := store.ActiveFinal(ctx, 50, true)
	if err != nil || active == nil || active.ID != final.ID || active.Content != final.Content {
		t.Fatalf("external detection overwrote final: %#v %v", active, err)
	}
}

func TestDiffIsDeterministicBoundedAndSupportsBothModes(t *testing.T) {
	ctx := context.Background()
	store, _ := newStore(t)
	from, err := store.Create(ctx, 3, chapterversion.CreateInput{Content: "one\ntwo\nthree\n", Type: chapterversion.TypeDraft, AuthorType: chapterversion.AuthorWriter})
	if err != nil {
		t.Fatal(err)
	}
	to, err := store.Create(ctx, 3, chapterversion.CreateInput{Content: "one\nTWO\nthree\nfour\n", Type: chapterversion.TypeEditorRevision, ParentVersionID: from.ID, AuthorType: chapterversion.AuthorEditor})
	if err != nil {
		t.Fatal(err)
	}
	inline1, err := store.Diff(ctx, 3, from.ID, to.ID, chapterversion.DiffInline, "", 2)
	if err != nil {
		t.Fatal(err)
	}
	inline2, err := store.Diff(ctx, 3, from.ID, to.ID, chapterversion.DiffInline, "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if inline1.FromSHA != from.ContentSHA || inline1.ToSHA != to.ContentSHA || inline1.Additions == 0 || inline1.Deletions == 0 || inline1.Truncated != inline2.Truncated || inline1.NextCursor != inline2.NextCursor {
		t.Fatalf("inline diff not deterministic/bounded: %#v %#v", inline1, inline2)
	}
	side, err := store.Diff(ctx, 3, from.ID, to.ID, chapterversion.DiffSideBySide, "", 200)
	if err != nil {
		t.Fatal(err)
	}
	if side.Mode != chapterversion.DiffSideBySide || side.Additions == 0 || side.Deletions == 0 {
		t.Fatalf("side diff = %#v", side)
	}
}

func TestParentVersionCannotCrossChapterBoundary(t *testing.T) {
	ctx := context.Background()
	store, _ := newStore(t)
	parent, err := store.Create(ctx, 1, chapterversion.CreateInput{Content: "chapter one", Type: chapterversion.TypeDraft, AuthorType: chapterversion.AuthorWriter})
	if err != nil {
		t.Fatal(err)
	}
	_, err = store.Create(ctx, 2, chapterversion.CreateInput{Content: "chapter two", Type: chapterversion.TypeDraft, ParentVersionID: parent.ID, AuthorType: chapterversion.AuthorWriter})
	if err == nil || errors.Is(err, context.Canceled) {
		t.Fatalf("cross-chapter parent error = %v", err)
	}
}
