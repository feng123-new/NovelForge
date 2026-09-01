package migrate

import (
	"context"
	"database/sql"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestRunnerAppliesMigrationsAndPragmas(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "project.db")
	runner := Runner{
		Path: path,
		Migrations: []Migration{
			{
				Version: 1,
				Name:    "widgets",
				SQL: `CREATE TABLE widgets (
					id TEXT PRIMARY KEY,
					parent_id TEXT,
					FOREIGN KEY(parent_id) REFERENCES widgets(id)
				)`,
			},
		},
	}
	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}
	db, err := Open(path, time.Second)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()

	var foreignKeys int
	if err := db.QueryRow(`PRAGMA foreign_keys`).Scan(&foreignKeys); err != nil {
		t.Fatalf("foreign_keys: %v", err)
	}
	if foreignKeys != 1 {
		t.Fatalf("foreign_keys = %d", foreignKeys)
	}
	var count int
	if err := db.QueryRow(`SELECT COUNT(*) FROM schema_migrations`).Scan(&count); err != nil {
		t.Fatalf("migration count: %v", err)
	}
	if count != 1 {
		t.Fatalf("migration count = %d", count)
	}
}

func TestRunnerRollbackAndBackup(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "project.db")
	initial := Runner{
		Path: path,
		Migrations: []Migration{{
			Version: 1,
			Name:    "base",
			SQL:     `CREATE TABLE stable(id INTEGER PRIMARY KEY)`,
		}},
	}
	if err := initial.Run(context.Background()); err != nil {
		t.Fatalf("initial Run: %v", err)
	}

	backupCalls := 0
	failing := Runner{
		Path: path,
		Migrations: []Migration{
			{Version: 1, Name: "base", SQL: `CREATE TABLE stable(id INTEGER PRIMARY KEY)`},
			{Version: 2, Name: "rollback", SQL: `CREATE TABLE transient(id INTEGER); THIS IS NOT SQL`},
		},
		Backup: func(_ context.Context, databasePath string) (string, error) {
			backupCalls++
			if databasePath != path {
				t.Fatalf("backup path = %q", databasePath)
			}
			return "backup.db", nil
		},
	}
	if err := failing.Run(context.Background()); err == nil {
		t.Fatal("expected migration failure")
	}
	if backupCalls != 1 {
		t.Fatalf("backup calls = %d", backupCalls)
	}

	db, err := Open(path, time.Second)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer db.Close()
	var count int
	err = db.QueryRow(`SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='transient'`).Scan(&count)
	if err != nil {
		t.Fatalf("table lookup: %v", err)
	}
	if count != 0 {
		t.Fatalf("transient table survived rollback")
	}
}

func TestRunnerRejectsChangedAppliedMigration(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "project.db")
	if err := (Runner{
		Path: path,
		Migrations: []Migration{{
			Version: 1,
			Name:    "base",
			SQL:     `CREATE TABLE stable(id INTEGER PRIMARY KEY)`,
		}},
	}).Run(context.Background()); err != nil {
		t.Fatalf("initial Run: %v", err)
	}
	err := (Runner{
		Path: path,
		Migrations: []Migration{{
			Version: 1,
			Name:    "base",
			SQL:     `CREATE TABLE stable(id TEXT PRIMARY KEY)`,
		}},
	}).Run(context.Background())
	var checksumErr *ChecksumError
	if !errors.As(err, &checksumErr) {
		t.Fatalf("error = %v, want ChecksumError", err)
	}
}

func TestTimestampedCopyBackup(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	source := filepath.Join(dir, "project.db")
	if err := os.WriteFile(source, []byte("sqlite bytes"), 0o600); err != nil {
		t.Fatal(err)
	}
	now := func() time.Time {
		return time.Date(2026, time.September, 1, 0, 0, 0, 123, time.UTC)
	}
	name, err := TimestampedCopyBackup(filepath.Join(dir, "backups"), now)(context.Background(), source)
	if err != nil {
		t.Fatalf("backup: %v", err)
	}
	if !strings.HasPrefix(name, "project-20260901T000000.") {
		t.Fatalf("backup name = %q", name)
	}
	data, err := os.ReadFile(filepath.Join(dir, "backups", name))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "sqlite bytes" {
		t.Fatalf("backup data = %q", data)
	}
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(dir, "backups", name))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("backup mode = %#o", info.Mode().Perm())
		}
	}
}

func TestDSNDoesNotExposeQueryCharacters(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "project ? #.db")
	dsn, err := DSN(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(dsn, "project ? #") {
		t.Fatalf("DSN is not escaped: %q", dsn)
	}
}

func queryInt(t *testing.T, db *sql.DB, query string) int {
	t.Helper()
	var value int
	if err := db.QueryRow(query).Scan(&value); err != nil {
		t.Fatal(err)
	}
	return value
}

func TestRunnerBacksUpBeforeCreatingMigrationTable(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	path := filepath.Join(dir, "legacy.db")
	db, err := Open(path, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`CREATE TABLE legacy_value(id INTEGER PRIMARY KEY, value TEXT NOT NULL); INSERT INTO legacy_value(value) VALUES ('kept')`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if _, err := db.Exec(`PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}

	backupPath := filepath.Join(dir, "captured.db")
	runner := Runner{
		Path: path,
		Migrations: []Migration{{
			Version: 1,
			Name:    "new_table",
			SQL:     `CREATE TABLE new_value(id INTEGER PRIMARY KEY)`,
		}},
		Backup: func(_ context.Context, databasePath string) (string, error) {
			input, err := os.Open(databasePath)
			if err != nil {
				return "", err
			}
			defer input.Close()
			output, err := os.Create(backupPath)
			if err != nil {
				return "", err
			}
			if _, err := io.Copy(output, input); err != nil {
				output.Close()
				return "", err
			}
			if err := output.Close(); err != nil {
				return "", err
			}
			return filepath.Base(backupPath), nil
		},
	}
	if err := runner.Run(context.Background()); err != nil {
		t.Fatalf("Run: %v", err)
	}

	backupDB, err := Open(backupPath, time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer backupDB.Close()
	if got := queryInt(t, backupDB, `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='schema_migrations'`); got != 0 {
		t.Fatalf("pre-migration backup contains schema_migrations: %d", got)
	}
	if got := queryInt(t, backupDB, `SELECT COUNT(*) FROM legacy_value WHERE value='kept'`); got != 1 {
		t.Fatalf("legacy row count = %d", got)
	}
}
