package project

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/voocel/ainovel-cli/internal/truthstore"
)

func TestProjectDatabaseMigratesAndPersistsTruth(t *testing.T) {
	ctx := context.Background()
	repository, err := NewRepository(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	created, err := repository.Create(ctx, CreateInput{Title: "Temporal Project"})
	if err != nil {
		t.Fatal(err)
	}
	store, err := repository.OpenTruthStore(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	input := truthstore.AppendInput{IdempotencyKey: "project-truth", Kind: truthstore.EventAssert,
		SubjectType: "character", SubjectID: "hero", Predicate: "status.alive",
		Value: json.RawMessage(`true`), ValidFromChapter: 1, KnownFromChapter: 1,
		Authority: truthstore.AuthorityHumanFinal, Confidence: 1,
		Source: truthstore.Source{Type: "human_final", ID: "author", Chapter: 1, Version: "human-v1"}}
	if _, err := store.Append(ctx, input); err != nil {
		t.Fatal(err)
	}
	_ = store.Close()
	reopened, err := repository.OpenTruthStore(ctx, created.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	state, err := reopened.State(ctx, truthstore.StateQuery{Chapter: 1, SubjectID: "hero"})
	if err != nil || state.Total != 1 {
		t.Fatalf("reopened truth = %#v, %v", state, err)
	}
}
