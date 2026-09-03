package chapterversion_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/voocel/ainovel-cli/internal/chapterversion"
	"github.com/voocel/ainovel-cli/internal/domain"
	"github.com/voocel/ainovel-cli/internal/narrativeledger"
	"github.com/voocel/ainovel-cli/internal/truthstore"
)

type oneShotFinalizeFault struct {
	point string
	fired bool
}

func (f *oneShotFinalizeFault) Fail(point string) error {
	if point == f.point && !f.fired {
		f.fired = true
		return errors.New("injected Phase 8 finalize fault at " + point)
	}
	return nil
}

func TestFinalizeRecoveryFaultMatrixConvergesWithoutDuplicateAuthority(t *testing.T) {
	faultPoints := []string{
		"after_version_ready",
		"after_truth_commit",
		"after_ledger_commit",
		"after_chapter_file_write",
		"after_active_final_switch",
		"before_checkpoint",
		"after_checkpoint",
	}
	for _, point := range faultPoints {
		point := point
		t.Run(point, func(t *testing.T) {
			ctx := context.Background()
			store, root := newStore(t)
			dbPath := filepath.Join(root, ".novelforge", "project.db")
			truth, err := truthstore.OpenExisting(dbPath, 5*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = truth.Close() })
			ledger, err := narrativeledger.OpenExisting(dbPath, 5*time.Second)
			if err != nil {
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = ledger.Close() })

			baseline, err := truth.Append(ctx, truthstore.AppendInput{
				IdempotencyKey:   "recovery-baseline-" + point,
				Kind:             truthstore.EventAssert,
				SubjectType:      "character",
				SubjectID:        "A",
				Predicate:        "alive",
				Value:            json.RawMessage(`false`),
				ValidFromChapter: 50,
				KnownFromChapter: 50,
				Authority:        truthstore.AuthorityGeneratedFinal,
				Confidence:       1,
				Source: truthstore.Source{
					Type:    "chapter_final",
					ID:      "generated-50",
					Chapter: 50,
					Version: "generated-50",
				},
			})
			if err != nil {
				t.Fatal(err)
			}

			oldFinal, err := store.Create(ctx, 50, chapterversion.CreateInput{
				Content:    "Character A died.\n",
				Type:       chapterversion.TypeFinal,
				AuthorType: chapterversion.AuthorSystem,
				Provenance: json.RawMessage(`{"source":"generated_final"}`),
			})
			if err != nil {
				t.Fatal(err)
			}
			if err := store.SwitchActiveFinal(ctx, 50, oldFinal.ID, chapterversion.AuthorityGeneratedFinal); err != nil {
				t.Fatal(err)
			}
			chapterPath := filepath.Join(root, "chapters", "050.md")
			if err := os.WriteFile(chapterPath, []byte(domain.NormalizeChapterContent(oldFinal.Content)), 0o600); err != nil {
				t.Fatal(err)
			}

			service := chapterversion.Service{Store: store}
			human, err := service.SaveHuman(ctx, 50, "recovery-save-"+point, "Character A is severely injured. Character A escaped and remains alive.\n")
			if err != nil {
				t.Fatal(err)
			}
			continuity := &scenarioBContinuity{}
			fault := &oneShotFinalizeFault{point: point}
			coordinator := chapterversion.Coordinator{
				Store:       store,
				Truth:       truth,
				Ledger:      ledger,
				Librarian:   scenarioBLibrarian{},
				Continuity:  continuity,
				FinalWriter: scenarioBWriter{root: root},
				Faults:      fault,
			}
			accepted, err := coordinator.Accept(ctx, 50, "recovery-accept-"+point, human.ID, "accept recovery candidate")
			if err != nil || !accepted.Accepted {
				t.Fatalf("accept = %#v err=%v", accepted, err)
			}

			key := "recovery-finalize-" + point
			if _, err := coordinator.Finalize(ctx, 50, key, accepted.ID); err == nil || !fault.fired {
				t.Fatalf("expected one-shot fault at %s, fired=%v err=%v", point, fault.fired, err)
			}
			result, err := coordinator.Finalize(ctx, 50, key, accepted.ID)
			if err != nil {
				t.Fatalf("retry after %s: %v", point, err)
			}
			if result.OperationID == "" || result.TruthEvents != 3 {
				t.Fatalf("recovered result = %#v", result)
			}
			if !result.ActiveFinal.ActiveFinal || result.ActiveFinal.Authority != chapterversion.AuthorityHumanFinal || result.ActiveFinal.ParentVersionID != accepted.ID {
				t.Fatalf("recovered Active Final = %#v", result.ActiveFinal)
			}

			replay, err := coordinator.Finalize(ctx, 50, key, accepted.ID)
			if err != nil {
				t.Fatalf("completed replay after %s: %v", point, err)
			}
			if replay.OperationID != result.OperationID || replay.TruthEvents != result.TruthEvents || replay.ActiveFinal.ID != result.ActiveFinal.ID || replay.Version.ID != result.Version.ID {
				t.Fatalf("completed replay drift: first=%#v replay=%#v", result, replay)
			}

			counts, err := store.DebugCounts(ctx, 50)
			if err != nil {
				t.Fatal(err)
			}
			if counts["versions"] != 3 || counts["active_final"] != 1 {
				t.Fatalf("version/final duplication after %s: %#v", point, counts)
			}
			var checkpoints int
			if err := store.Database().QueryRow(`SELECT COUNT(*) FROM chapter_version_checkpoints WHERE operation_id=?`, result.OperationID).Scan(&checkpoints); err != nil {
				t.Fatal(err)
			}
			if checkpoints != 1 {
				t.Fatalf("checkpoint count after %s = %d", point, checkpoints)
			}
			var sagaState string
			if err := store.Database().QueryRow(`SELECT state FROM chapter_finalize_sagas WHERE operation_id=?`, result.OperationID).Scan(&sagaState); err != nil {
				t.Fatal(err)
			}
			if sagaState != "completed" {
				t.Fatalf("saga state after %s = %q", point, sagaState)
			}

			events, err := truth.Events(ctx, truthstore.EventQuery{SubjectType: "character", SubjectID: "A", Limit: 100})
			if err != nil {
				t.Fatal(err)
			}
			if len(events.Events) != 4 {
				t.Fatalf("Truth event duplication after %s: %#v", point, events.Events)
			}
			foundBaseline := false
			foundSupersede := false
			for _, event := range events.Events {
				if event.ID == baseline.Event.ID {
					foundBaseline = true
				}
				if event.Predicate == "alive" && event.Kind == truthstore.EventSupersede && event.SupersedesEventID == baseline.Event.ID && event.Authority == truthstore.AuthorityHumanFinal {
					foundSupersede = true
				}
			}
			if !foundBaseline || !foundSupersede {
				t.Fatalf("Truth authority history after %s = %#v", point, events.Events)
			}

			fileBytes, err := os.ReadFile(chapterPath)
			if err != nil {
				t.Fatal(err)
			}
			if domain.ChapterContentSHA256(domain.NormalizeChapterContent(string(fileBytes))) != result.ActiveFinal.ContentSHA {
				t.Fatalf("chapter file did not converge after %s", point)
			}
			rebuild, ok, err := store.LatestRebuild(ctx, 50)
			if err != nil || !ok || rebuild.State != "completed" || rebuild.BoundaryChapter != 50 {
				t.Fatalf("rebuild after %s = %#v ok=%v err=%v", point, rebuild, ok, err)
			}
		})
	}
}
