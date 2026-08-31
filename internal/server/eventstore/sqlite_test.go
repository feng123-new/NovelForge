package eventstore

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	serverrepository "github.com/voocel/ainovel-cli/internal/server/repository"
)

func TestSQLiteRepositoryPersistsAndFiltersReplay(t *testing.T) {
	t.Parallel()
	workspace := t.TempDir()
	paths, err := serverrepository.Initialize(context.Background(), workspace)
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	now := time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)
	firstProcess := SQLiteRepository{
		DatabasePath: paths.Database,
		Now:          func() time.Time { return now },
	}
	first, err := firstProcess.Append(
		context.Background(),
		"project.created",
		"project-a",
		map[string]any{"title": "A"},
	)
	if err != nil {
		t.Fatalf("Append first: %v", err)
	}
	now = now.Add(time.Second)
	second, err := firstProcess.Append(
		context.Background(),
		"project.created",
		"project-b",
		map[string]any{"title": "B"},
	)
	if err != nil {
		t.Fatalf("Append second: %v", err)
	}
	if first.ID == 0 || second.ID != first.ID+1 {
		t.Fatalf("ids = %d, %d", first.ID, second.ID)
	}

	// A new repository instance simulates a process restart.
	secondProcess := SQLiteRepository{DatabasePath: paths.Database}
	replayed, err := secondProcess.Replay(context.Background(), 0, "project-a", 100)
	if err != nil {
		t.Fatalf("Replay: %v", err)
	}
	if len(replayed) != 1 || replayed[0].ID != first.ID || replayed[0].Project != "project-a" {
		t.Fatalf("replayed = %#v", replayed)
	}
	var payload map[string]any
	if err := json.Unmarshal(replayed[0].Data, &payload); err != nil {
		t.Fatal(err)
	}
	if payload["title"] != "A" {
		t.Fatalf("payload = %#v", payload)
	}

	afterFirst, err := secondProcess.Replay(context.Background(), first.ID, "", 100)
	if err != nil {
		t.Fatal(err)
	}
	if len(afterFirst) != 1 || afterFirst[0].ID != second.ID {
		t.Fatalf("after first = %#v", afterFirst)
	}
}

func TestReplayLimitIsBounded(t *testing.T) {
	t.Parallel()
	workspace := t.TempDir()
	paths, err := serverrepository.Initialize(context.Background(), workspace)
	if err != nil {
		t.Fatal(err)
	}
	repository := SQLiteRepository{DatabasePath: paths.Database}
	for index := 0; index < 3; index++ {
		if _, err := repository.Append(
			context.Background(),
			"test",
			"",
			map[string]int{"index": index},
		); err != nil {
			t.Fatal(err)
		}
	}
	records, err := repository.Replay(context.Background(), 0, "", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("records = %d", len(records))
	}
}
