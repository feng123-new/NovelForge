package chapterversion_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/voocel/ainovel-cli/internal/chapterversion"
	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/narrativeledger"
	"github.com/voocel/ainovel-cli/internal/qualitygate"
	"github.com/voocel/ainovel-cli/internal/truthstore"
)

type scenarioBLibrarian struct{}

func (scenarioBLibrarian) Propose(_ context.Context, request qualitygate.LibrarianRequest) (qualitygate.FactProposal, error) {
	change := func(predicate string, object any) qualitygate.FactChange {
		value, _ := json.Marshal(object)
		return qualitygate.FactChange{Subject: "character:A", Predicate: predicate, Object: value, SourceChapter: request.Chapter, SourceVersion: request.Candidate.SourceVersion, SourceSHA: request.Candidate.TextSHA, Extractor: "scenario-b-fake-librarian", Confidence: 1, ProposedAuthority: string(truthstore.AuthorityLLMSuggestion), ValidFromChapter: request.Chapter, KnownFromChapter: request.Chapter, Reason: "deterministic Scenario B extraction"}
	}
	return qualitygate.FactProposal{
		ProposalID: "proposal-scenario-b-" + request.Candidate.ID, ProjectID: request.ProjectID, Chapter: request.Chapter,
		SourceVersion: request.Candidate.SourceVersion, SourceSHA: request.Candidate.TextSHA, Extractor: "scenario-b-fake-librarian", Authority: string(truthstore.AuthorityLLMSuggestion),
		CharacterChanges: []qualitygate.FactChange{change("alive", true), change("escaped", true)},
		Injuries:         []qualitygate.FactChange{change("injury", "severe")},
	}, nil
}

type scenarioBContinuity struct{ calls int }

func (c *scenarioBContinuity) Check(_ context.Context, _ qualitygate.ContinuityRequest) (qualitygate.ContinuityResult, error) {
	c.calls++
	return qualitygate.ContinuityResult{Status: qualitygate.ContinuityPass, Issues: []qualitygate.ContinuityIssue{}}, nil
}

type scenarioBWriter struct{ root string }

func (w scenarioBWriter) WriteFinalChapter(_ context.Context, _ string, chapter int, content, sha string) error {
	if domain.ChapterContentSHA256(domain.NormalizeChapterContent(content)) != sha {
		return os.ErrInvalid
	}
	if chapter != 50 {
		return os.ErrInvalid
	}
	return os.WriteFile(filepath.Join(w.root, "chapters", "050.md"), []byte(domain.NormalizeChapterContent(content)), 0o600)
}

func TestScenarioBHumanEditSyncSupersedesDeathAndRebuildsFromChapter50(t *testing.T) {
	ctx := context.Background()
	store, root := newStore(t)
	dbPath := filepath.Join(root, ".novelforge", "project.db")
	truth, err := truthstore.OpenExisting(dbPath, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer truth.Close()
	ledger, err := narrativeledger.OpenExisting(dbPath, 5*time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer ledger.Close()

	alive49, err := truth.Append(ctx, truthstore.AppendInput{
		IdempotencyKey: "scenario-b-alive-49", Kind: truthstore.EventAssert, SubjectType: "character", SubjectID: "A", Predicate: "alive", Value: json.RawMessage(`true`),
		ValidFromChapter: 1, KnownFromChapter: 1, Authority: truthstore.AuthorityGeneratedFinal, Confidence: 1,
		Source: truthstore.Source{Type: "chapter_final", ID: "chapter-49", Chapter: 49, Version: "v49"},
	})
	if err != nil {
		t.Fatal(err)
	}
	death50, err := truth.Append(ctx, truthstore.AppendInput{
		IdempotencyKey: "scenario-b-death-50", Kind: truthstore.EventSupersede, SubjectType: "character", SubjectID: "A", Predicate: "alive", Value: json.RawMessage(`false`),
		ValidFromChapter: 50, KnownFromChapter: 50, Authority: truthstore.AuthorityGeneratedFinal, Confidence: 1,
		Source: truthstore.Source{Type: "chapter_final", ID: "chapter-50", Chapter: 50, Version: "generated-50"}, SupersedesEventID: alive49.Event.ID,
	})
	if err != nil {
		t.Fatal(err)
	}

	before49, err := truth.State(ctx, truthstore.StateQuery{Chapter: 49, SubjectType: "character", SubjectID: "A", Predicate: "alive", Limit: 10})
	if err != nil || len(before49.Facts) != 1 || string(before49.Facts[0].Value) != "true" {
		t.Fatalf("Chapter 49 baseline = %#v, err=%v", before49, err)
	}
	beforeDigest49, err := store.ProjectionBoundaryDigest(ctx, 49)
	if err != nil {
		t.Fatal(err)
	}
	beforeDigest50, err := store.ProjectionBoundaryDigest(ctx, 50)
	if err != nil {
		t.Fatal(err)
	}

	oldFinal, err := store.Create(ctx, 50, chapterversion.CreateInput{Content: "Character A died.\n", Type: chapterversion.TypeFinal, AuthorType: chapterversion.AuthorSystem, Provenance: json.RawMessage(`{"source":"generated_final"}`)})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.SwitchActiveFinal(ctx, 50, oldFinal.ID, chapterversion.AuthorityGeneratedFinal); err != nil {
		t.Fatal(err)
	}
	chapterFile := filepath.Join(root, "chapters", "050.md")
	if err := os.WriteFile(chapterFile, []byte(domain.NormalizeChapterContent(oldFinal.Content)), 0o600); err != nil {
		t.Fatal(err)
	}
	status, err := store.DetectExternal(ctx, 50)
	if err != nil || status.SyncRequired {
		t.Fatalf("matching final should not require sync: %#v err=%v", status, err)
	}

	humanText := "Character A is severely injured. Character A escaped and remains alive.\n"
	if err := os.WriteFile(chapterFile, []byte(humanText), 0o600); err != nil {
		t.Fatal(err)
	}
	status, err = store.DetectExternal(ctx, 50)
	if err != nil || !status.SyncRequired || status.ExpectedSHA == status.ObservedSHA {
		t.Fatalf("external edit was not detected: %#v err=%v", status, err)
	}
	active, err := store.ActiveFinal(ctx, 50, true)
	if err != nil || active == nil || active.ID != oldFinal.ID {
		t.Fatalf("external detection changed Active Final: %#v err=%v", active, err)
	}

	continuity := &scenarioBContinuity{}
	coordinator := chapterversion.Coordinator{Store: store, Truth: truth, Ledger: ledger, Librarian: scenarioBLibrarian{}, Continuity: continuity, FinalWriter: scenarioBWriter{root: root}}
	synced, err := coordinator.SyncExternal(ctx, 50, "scenario-b-sync", status.ObservedSHA)
	if err != nil {
		t.Fatal(err)
	}
	if synced.Version.Type != chapterversion.TypeHumanRevision || synced.Version.AuthorType != chapterversion.AuthorHuman || synced.Version.ParentVersionID != oldFinal.ID {
		t.Fatalf("sync version = %#v", synced.Version)
	}
	if synced.Conflicts != 1 || continuity.calls != 1 {
		t.Fatalf("sync conflicts=%d continuity calls=%d", synced.Conflicts, continuity.calls)
	}
	if replay, err := coordinator.SyncExternal(ctx, 50, "scenario-b-sync", status.ObservedSHA); err != nil || replay.Version.ID != synced.Version.ID {
		t.Fatalf("sync replay = %#v err=%v", replay, err)
	}
	var versionCount int
	if err := store.Database().QueryRowContext(ctx, `SELECT COUNT(*) FROM chapter_versions WHERE chapter=?`, 50).Scan(&versionCount); err != nil || versionCount != 2 {
		t.Fatalf("repeated sync duplicated version: count=%d err=%v", versionCount, err)
	}

	accepted, err := coordinator.Accept(ctx, 50, "scenario-b-accept", synced.Version.ID, "accept corrected human edit")
	if err != nil || !accepted.Accepted {
		t.Fatalf("accept = %#v err=%v", accepted, err)
	}
	finalized, err := coordinator.Finalize(ctx, 50, "scenario-b-finalize", accepted.ID)
	if err != nil {
		t.Fatal(err)
	}
	if !finalized.ActiveFinal.ActiveFinal || finalized.ActiveFinal.Authority != chapterversion.AuthorityHumanFinal || finalized.ActiveFinal.Type != chapterversion.TypeFinal || finalized.ActiveFinal.ID == oldFinal.ID {
		t.Fatalf("human final = %#v", finalized.ActiveFinal)
	}
	if replay, err := coordinator.Finalize(ctx, 50, "scenario-b-finalize", accepted.ID); err != nil || replay.ActiveFinal.ID != finalized.ActiveFinal.ID {
		t.Fatalf("finalize replay = %#v err=%v", replay, err)
	}

	after49, err := truth.State(ctx, truthstore.StateQuery{Chapter: 49, SubjectType: "character", SubjectID: "A", Predicate: "alive", Limit: 10})
	if err != nil || len(after49.Facts) != 1 || string(after49.Facts[0].Value) != "true" || after49.Facts[0].ID != before49.Facts[0].ID {
		t.Fatalf("Chapter 49 changed across boundary rebuild: before=%#v after=%#v err=%v", before49, after49, err)
	}
	afterDigest49, err := store.ProjectionBoundaryDigest(ctx, 49)
	if err != nil || afterDigest49 != beforeDigest49 {
		t.Fatalf("Chapter 49 digest changed: before=%s after=%s err=%v", beforeDigest49, afterDigest49, err)
	}
	afterDigest50, err := store.ProjectionBoundaryDigest(ctx, 50)
	if err != nil || afterDigest50 == beforeDigest50 {
		t.Fatalf("Chapter 50 digest did not change: before=%s after=%s err=%v", beforeDigest50, afterDigest50, err)
	}
	after50, err := truth.State(ctx, truthstore.StateQuery{Chapter: 50, SubjectType: "character", SubjectID: "A", Predicate: "alive", Limit: 10})
	if err != nil || len(after50.Facts) != 1 || string(after50.Facts[0].Value) != "true" || after50.Facts[0].Authority != truthstore.AuthorityHumanFinal {
		t.Fatalf("Chapter 50 corrected Truth = %#v err=%v", after50, err)
	}
	events, err := truth.Events(ctx, truthstore.EventQuery{SubjectType: "character", SubjectID: "A", Predicate: "alive", Limit: 20})
	if err != nil {
		t.Fatal(err)
	}
	foundOldDeath, foundHumanSupersede := false, false
	for _, event := range events.Events {
		if event.ID == death50.Event.ID {
			foundOldDeath = true
		}
		if event.Kind == truthstore.EventSupersede && event.SupersedesEventID == death50.Event.ID && event.Authority == truthstore.AuthorityHumanFinal {
			foundHumanSupersede = true
		}
	}
	if !foundOldDeath || !foundHumanSupersede {
		t.Fatalf("Truth audit missing old death or Human Final supersede: %#v", events.Events)
	}
	for predicate, expected := range map[string]string{"escaped": "true", "injury": `"severe"`} {
		page, err := truth.State(ctx, truthstore.StateQuery{Chapter: 50, SubjectType: "character", SubjectID: "A", Predicate: predicate, Limit: 10})
		if err != nil || len(page.Facts) != 1 || string(page.Facts[0].Value) != expected || page.Facts[0].Authority != truthstore.AuthorityHumanFinal {
			t.Fatalf("%s projection = %#v err=%v", predicate, page, err)
		}
	}

	rebuild, ok, err := store.LatestRebuild(ctx, 50)
	if err != nil || !ok || rebuild.State != "completed" || rebuild.BoundaryChapter != 50 {
		t.Fatalf("boundary rebuild = %#v ok=%v err=%v", rebuild, ok, err)
	}
	impacts, total, err := store.ListPlanImpacts(ctx, 51, 100, 0)
	if err != nil || total < 3 || len(impacts) < 3 {
		t.Fatalf("Chapter 51+ plan impacts = %#v total=%d err=%v", impacts, total, err)
	}
	for _, impact := range impacts {
		if impact.Chapter != 51 || impact.SourceVersion != finalized.ActiveFinal.ID {
			t.Fatalf("unexpected plan impact = %#v", impact)
		}
	}
	fileBytes, err := os.ReadFile(chapterFile)
	if err != nil {
		t.Fatal(err)
	}
	if domain.ChapterContentSHA256(domain.NormalizeChapterContent(string(fileBytes))) != finalized.ActiveFinal.ContentSHA {
		t.Fatal("chapter file SHA diverged from Active Human Final")
	}
	if syncStatus, err := store.SyncStatus(ctx, 50, false); err != nil || syncStatus.SyncRequired {
		t.Fatalf("sync_required not cleared after finalization: %#v err=%v", syncStatus, err)
	}
}
