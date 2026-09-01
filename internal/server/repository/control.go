package repository

import (
	"context"
	"database/sql"
	"path/filepath"
	"time"

	"github.com/voocel/ainovel-cli/internal/db/migrate"
)

const busyTimeout = 5 * time.Second

// ControlMigrations owns workspace-scoped API durability. Project truth remains
// isolated in each project's project.db.
var ControlMigrations = []migrate.Migration{
	{
		Version: 1,
		Name:    "durable_events",
		SQL: `CREATE TABLE events (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			event_type TEXT NOT NULL,
			project_id TEXT NOT NULL DEFAULT '',
			payload_json BLOB NOT NULL,
			created_at TEXT NOT NULL
		);
		CREATE INDEX events_project_id_id_idx ON events(project_id, id);`,
	},
	{
		Version: 2,
		Name:    "idempotency_records",
		SQL: `CREATE TABLE idempotency_records (
			key TEXT PRIMARY KEY,
			operation TEXT NOT NULL,
			project_id TEXT NOT NULL DEFAULT '',
			request_hash TEXT NOT NULL,
			status TEXT NOT NULL,
			response_status INTEGER,
			response_body BLOB,
			created_at TEXT NOT NULL,
			expires_at TEXT NOT NULL
		);
		CREATE INDEX idempotency_expires_at_idx ON idempotency_records(expires_at);`,
	},
}

// Paths returns workspace-private control locations.
type Paths struct {
	Root     string
	Database string
	Backups  string
	Trash    string
	Archive  string
}

func WorkspacePaths(workspace string) Paths {
	root := filepath.Join(workspace, ".novelforge")
	return Paths{
		Root:     root,
		Database: filepath.Join(root, "server.db"),
		Backups:  filepath.Join(root, "backups"),
		Trash:    filepath.Join(root, "trash"),
		Archive:  filepath.Join(root, "archive"),
	}
}

// Initialize creates and migrates the workspace-scoped control database.
func Initialize(ctx context.Context, workspace string) (Paths, error) {
	paths := WorkspacePaths(workspace)
	runner := migrate.Runner{
		Path:        paths.Database,
		Migrations:  ControlMigrations,
		BusyTimeout: busyTimeout,
		Backup:      migrate.TimestampedCopyBackup(paths.Backups, time.Now),
	}
	if err := runner.Run(ctx); err != nil {
		return Paths{}, err
	}
	return paths, nil
}

// OpenControl opens the initialized workspace control database.
func OpenControl(path string) (*sql.DB, error) {
	return migrate.Open(path, busyTimeout)
}
