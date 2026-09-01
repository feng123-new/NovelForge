package idempotency

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	serverrepository "github.com/voocel/ainovel-cli/internal/server/repository"
)

func TestStoreReplaysCompletedResponse(t *testing.T) {
	t.Parallel()
	databasePath := initializeControlDatabase(t)
	now := time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)
	store := Store{
		DatabasePath: databasePath,
		TTL:          time.Hour,
		Now:          func() time.Time { return now },
	}
	hash := RequestHash("POST", "/api/projects", []byte(`{"title":"A"}`))
	first, err := store.Begin(context.Background(), "create-a", "project.create", "", hash)
	if err != nil {
		t.Fatalf("Begin: %v", err)
	}
	if !first.Execute {
		t.Fatal("first request should execute")
	}
	body := []byte(`{"id":"opaque"}`)
	if err := store.Complete(context.Background(), "create-a", hash, 201, body); err != nil {
		t.Fatalf("Complete: %v", err)
	}
	replay, err := store.Begin(context.Background(), "create-a", "project.create", "", hash)
	if err != nil {
		t.Fatalf("replay Begin: %v", err)
	}
	if replay.Execute || replay.ResponseStatus != 201 || string(replay.ResponseBody) != string(body) {
		t.Fatalf("replay = %#v", replay)
	}
}

func TestStoreRejectsKeyReuseWithDifferentRequest(t *testing.T) {
	t.Parallel()
	databasePath := initializeControlDatabase(t)
	store := Store{DatabasePath: databasePath}
	hashA := RequestHash("POST", "/api/projects", []byte(`{"title":"A"}`))
	hashB := RequestHash("POST", "/api/projects", []byte(`{"title":"B"}`))
	if _, err := store.Begin(context.Background(), "same-key", "project.create", "", hashA); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Begin(
		context.Background(),
		"same-key",
		"project.create",
		"",
		hashB,
	); !errors.Is(err, ErrConflict) {
		t.Fatalf("error = %v, want ErrConflict", err)
	}
}

func TestStoreReportsInProgress(t *testing.T) {
	t.Parallel()
	databasePath := initializeControlDatabase(t)
	store := Store{DatabasePath: databasePath}
	hash := RequestHash("PATCH", "/api/projects/id", []byte(`{"title":"A"}`))
	if _, err := store.Begin(context.Background(), "update", "project.update", "id", hash); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Begin(
		context.Background(),
		"update",
		"project.update",
		"id",
		hash,
	); !errors.Is(err, ErrInProgress) {
		t.Fatalf("error = %v, want ErrInProgress", err)
	}
}

func TestExpiredKeyCanBeReservedAgain(t *testing.T) {
	t.Parallel()
	databasePath := initializeControlDatabase(t)
	now := time.Date(2026, time.September, 1, 0, 0, 0, 0, time.UTC)
	store := Store{
		DatabasePath: databasePath,
		TTL:          time.Minute,
		Now:          func() time.Time { return now },
	}
	hashA := RequestHash("POST", "/api/projects", []byte(`{"title":"A"}`))
	if _, err := store.Begin(context.Background(), "expired", "project.create", "", hashA); err != nil {
		t.Fatal(err)
	}
	now = now.Add(2 * time.Minute)
	hashB := RequestHash("POST", "/api/projects", []byte(`{"title":"B"}`))
	result, err := store.Begin(context.Background(), "expired", "project.create", "", hashB)
	if err != nil {
		t.Fatalf("Begin after expiry: %v", err)
	}
	if !result.Execute {
		t.Fatal("expired key should execute again")
	}
}

func initializeControlDatabase(t *testing.T) string {
	t.Helper()
	workspace := t.TempDir()
	paths, err := serverrepository.Initialize(context.Background(), workspace)
	if err != nil {
		t.Fatalf("Initialize: %v", err)
	}
	if !filepath.IsAbs(paths.Database) {
		t.Fatalf("database path = %q", paths.Database)
	}
	return paths.Database
}
