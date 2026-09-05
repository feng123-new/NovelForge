package project

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/voocel/ainovel-cli/internal/db/migrate"
	"github.com/voocel/ainovel-cli/internal/lifecycle"
)

func TestLifecycleMigrationKeepsPreimage(t *testing.T) {
	ctx := context.Background()
	workspace := t.TempDir()
	r, err := NewRepository(workspace)
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(workspace, "old-book")
	if err = os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	if err = initializeLayout(root); err != nil {
		t.Fatal(err)
	}
	meta := Metadata{ID: "0123456789abcdef0123456789abcdef", Title: "旧项目", FormatVersion: 1, Status: StatusActive, CreatedAt: time.Now().UTC(), UpdatedAt: time.Now().UTC(), Language: "zh"}
	if err = writeJSONAtomic(filepath.Join(root, projectMetadataRelative), meta, 0600); err != nil {
		t.Fatal(err)
	}
	var migrations []migrate.Migration
	for _, m := range projectMigrations {
		if m.Version <= 9 {
			migrations = append(migrations, m)
		}
	}
	if err = (migrate.Runner{Path: filepath.Join(root, projectDatabaseRelative), Migrations: migrations}).Run(ctx); err != nil {
		t.Fatal(err)
	}
	if err = writeDatabaseMetadata(ctx, root, meta); err != nil {
		t.Fatal(err)
	}
	config := []byte(`{"api_key":"local-test-credential"}`)
	if err = os.WriteFile(filepath.Join(root, projectConfigRelative), config, 0600); err != nil {
		t.Fatal(err)
	}
	lease, err := r.AcquireExecution(ctx, meta.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	got, err := r.MigrateLifecycle(ctx, meta.ID, "migrate-old", 1)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Changed || got.From != 1 || got.To != 2 || got.Schema != 10 || got.BackupID == "" {
		t.Fatalf("migration result: %+v", got)
	}
	backup, err := r.ReadLifecycleBackup(ctx, meta.ID, got.BackupID)
	if err != nil {
		t.Fatal(err)
	}
	m, files, err := lifecycle.Unpack(backup)
	if err != nil {
		t.Fatal(err)
	}
	if m.Format != 1 || m.Schema != 9 {
		t.Fatal("backup is not pre-migration", m)
	}
	var prior Metadata
	if err = json.Unmarshal(files[projectMetadataRelative], &prior); err != nil || prior.FormatVersion != 1 {
		t.Fatal("old metadata missing", err)
	}
	current, err := r.Get(ctx, meta.ID)
	if err != nil || current.FormatVersion != 2 {
		t.Fatal("format was not upgraded", err)
	}
	if b, err := os.ReadFile(filepath.Join(root, projectConfigRelative)); err != nil || string(b) != string(config) {
		t.Fatal("local credentials changed")
	}
	store, err := r.OpenLifecycle(ctx, meta.ID)
	if err != nil {
		t.Fatal(err)
	}
	var schema int
	err = store.DB.QueryRow(`SELECT max(version) FROM schema_migrations`).Scan(&schema)
	store.DB.Close()
	if err != nil || schema != 10 {
		t.Fatal("schema upgrade missing", err)
	}
	second, err := r.MigrateLifecycle(ctx, meta.ID, "migrate-noop", 2)
	if err != nil || second.Changed {
		t.Fatal("non-idempotent migration", err)
	}
	restored, err := NewRepository(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	result, err := restored.RestoreLifecycle(ctx, backup)
	if err != nil {
		t.Fatal("restore known old schema", err)
	}
	if result.Project.ID != meta.ID || result.JobsResumed {
		t.Fatal("restore identity/task contract")
	}
}

func TestLifecycleRejectsChangedSchemaAndSymlinks(t *testing.T) {
	ctx := context.Background()
	r, err := NewRepository(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	p, err := r.Create(ctx, CreateInput{Title: "Snapshot"})
	if err != nil {
		t.Fatal(err)
	}
	lease, err := r.AcquireExecution(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	defer lease.Close()
	backup, err := r.BackupLifecycle(ctx, p.ID)
	if err != nil {
		t.Fatal(err)
	}
	m, files, err := lifecycle.Unpack(backup)
	if err != nil {
		t.Fatal(err)
	}
	poison := filepath.Join(t.TempDir(), "snapshot.db")
	if err = os.WriteFile(poison, files[projectDatabaseRelative], 0600); err != nil {
		t.Fatal(err)
	}
	db, err := migrate.Open(poison, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	// Prefix resembles an internal name but must not escape schema comparison.
	if _, err = db.Exec(`CREATE VIEW sqliteXunexpected AS SELECT 1 AS value`); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		t.Fatal(err)
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	files[projectDatabaseRelative], err = os.ReadFile(poison)
	if err != nil {
		t.Fatal(err)
	}
	changed, err := lifecycle.Pack(m, files)
	if err != nil {
		t.Fatal(err)
	} // Recompute file hashes: schema validation must still reject.
	other, err := NewRepository(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if _, err = other.RestoreLifecycle(ctx, changed); !errors.Is(err, lifecycle.ErrInvalid) {
		t.Fatalf("changed schema accepted: %v", err)
	}
	page, err := other.List(ctx, ListOptions{})
	if err != nil || page.Total != 0 {
		t.Fatal("failed restore published a project", err)
	}
	root, err := r.find(p.ID)
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "private.txt")
	if err = os.WriteFile(outside, []byte("do not read"), 0600); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root.Root, "references", "outside.txt")
	if err = os.Symlink(outside, link); err != nil {
		t.Fatal(err)
	}
	if _, err = r.BackupLifecycle(ctx, p.ID); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("symlink reference accepted: %v", err)
	}
}
