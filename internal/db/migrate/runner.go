package migrate

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

const (
	defaultBusyTimeout = 5 * time.Second
	schemaTableSQL     = `CREATE TABLE IF NOT EXISTS schema_migrations (
		version INTEGER PRIMARY KEY,
		name TEXT NOT NULL,
		checksum TEXT NOT NULL,
		applied_at TEXT NOT NULL
	)`
)

// Migration is an immutable, ordered database change.
type Migration struct {
	Version int
	Name    string
	SQL     string
}

// BackupFunc is called before the first pending migration is applied to an
// existing database. The returned label is diagnostic only and must not expose
// credentials.
type BackupFunc func(ctx context.Context, databasePath string) (string, error)

// Runner applies versioned SQLite migrations transactionally.
type Runner struct {
	Path        string
	Migrations  []Migration
	BusyTimeout time.Duration
	Backup      BackupFunc
	Now         func() time.Time
}

// ChecksumError reports a changed migration that was already applied.
type ChecksumError struct {
	Version  int
	Expected string
	Actual   string
}

func (e *ChecksumError) Error() string {
	return fmt.Sprintf("migration %d checksum mismatch", e.Version)
}

// Run validates checksums, creates a pre-migration backup when required and
// applies every pending migration in a single transaction.
func (r Runner) Run(ctx context.Context) error {
	if strings.TrimSpace(r.Path) == "" {
		return errors.New("database path is required")
	}
	migrations, err := normalizeMigrations(r.Migrations)
	if err != nil {
		return err
	}
	if r.BusyTimeout <= 0 {
		r.BusyTimeout = defaultBusyTimeout
	}
	if r.Now == nil {
		r.Now = time.Now
	}
	if err := os.MkdirAll(filepath.Dir(r.Path), 0o700); err != nil {
		return fmt.Errorf("prepare database directory: %w", err)
	}

	info, statErr := os.Stat(r.Path)
	existed := statErr == nil && info.Mode().IsRegular() && info.Size() > 0
	if statErr != nil && !errors.Is(statErr, os.ErrNotExist) {
		return fmt.Errorf("stat database: %w", statErr)
	}

	db, err := Open(r.Path, r.BusyTimeout)
	if err != nil {
		return err
	}
	hasSchemaTable, err := schemaTableExists(ctx, db)
	if err != nil {
		_ = db.Close()
		return err
	}
	applied := make(map[int]appliedMigration)
	if hasSchemaTable {
		applied, err = loadApplied(ctx, db)
		if err != nil {
			_ = db.Close()
			return err
		}
	}
	if err := validateApplied(migrations, applied); err != nil {
		_ = db.Close()
		return err
	}

	pending := pendingMigrations(migrations, applied)
	if hasSchemaTable && len(pending) == 0 {
		return db.Close()
	}
	// Force any committed WAL frames into the main database before copying it.
	// No schema change has been made at this point, so the backup represents the
	// exact pre-migration state.
	_, _ = db.ExecContext(ctx, "PRAGMA wal_checkpoint(TRUNCATE)")
	if err := db.Close(); err != nil {
		return fmt.Errorf("close database before migration: %w", err)
	}

	if existed {
		backup := r.Backup
		if backup == nil {
			backup = TimestampedCopyBackup(filepath.Join(filepath.Dir(r.Path), "backups"), r.Now)
		}
		if _, err := backup(ctx, r.Path); err != nil {
			return fmt.Errorf("backup database: %w", err)
		}
	}

	db, err = Open(r.Path, r.BusyTimeout)
	if err != nil {
		return err
	}
	defer db.Close()

	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin migration transaction: %w", err)
	}
	committed := false
	defer func() {
		if !committed {
			_ = tx.Rollback()
		}
	}()

	if _, err := tx.ExecContext(ctx, schemaTableSQL); err != nil {
		return fmt.Errorf("initialize migration table: %w", err)
	}
	for _, migration := range pending {
		if _, err := tx.ExecContext(ctx, migration.SQL); err != nil {
			return fmt.Errorf("apply migration %d (%s): %w", migration.Version, migration.Name, err)
		}
		if _, err := tx.ExecContext(
			ctx,
			`INSERT INTO schema_migrations(version, name, checksum, applied_at) VALUES (?, ?, ?, ?)`,
			migration.Version,
			migration.Name,
			migrationChecksum(migration),
			r.Now().UTC().Format(time.RFC3339Nano),
		); err != nil {
			return fmt.Errorf("record migration %d: %w", migration.Version, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit migrations: %w", err)
	}
	committed = true
	return nil
}

// Open opens a SQLite database with foreign keys and a bounded busy timeout.
// Callers must close the returned handle.
func Open(path string, busyTimeout time.Duration) (*sql.DB, error) {
	if busyTimeout <= 0 {
		busyTimeout = defaultBusyTimeout
	}
	dsn, err := DSN(path, busyTimeout)
	if err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, fmt.Errorf("open database: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if err := db.Ping(); err != nil {
		_ = db.Close()
		return nil, fmt.Errorf("ping database: %w", err)
	}
	for _, pragma := range []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA busy_timeout = " + strconv.FormatInt(busyTimeout.Milliseconds(), 10),
		"PRAGMA journal_mode = WAL",
	} {
		if _, err := db.Exec(pragma); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("configure database: %w", err)
		}
	}
	return db, nil
}

// DSN returns a file URI suitable for modernc.org/sqlite on Unix and Windows.
func DSN(path string, busyTimeout time.Duration) (string, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolve database path: %w", err)
	}
	timeoutMS := busyTimeout.Milliseconds()
	if timeoutMS <= 0 {
		timeoutMS = defaultBusyTimeout.Milliseconds()
	}
	uriPath := filepath.ToSlash(absolute)
	// A Windows drive path such as C:/data/project.db must be represented as
	// file:///C:/data/project.db. Without the leading slash, SQLite interprets
	// C: as the URI authority and rejects the DSN.
	if filepath.VolumeName(absolute) != "" && !strings.HasPrefix(uriPath, "/") {
		uriPath = "/" + uriPath
	}
	u := &url.URL{
		Scheme: "file",
		Path:   uriPath,
	}
	query := u.Query()
	query.Add("_pragma", "foreign_keys(1)")
	query.Add("_pragma", "busy_timeout("+strconv.FormatInt(timeoutMS, 10)+")")
	u.RawQuery = query.Encode()
	return u.String(), nil
}

// TimestampedCopyBackup returns a backup function that creates an atomic
// owner-readable copy in backupDir.
func TimestampedCopyBackup(backupDir string, now func() time.Time) BackupFunc {
	if now == nil {
		now = time.Now
	}
	return func(ctx context.Context, databasePath string) (string, error) {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
		if err := os.MkdirAll(backupDir, 0o700); err != nil {
			return "", err
		}
		source, err := os.Open(databasePath)
		if err != nil {
			return "", err
		}
		defer source.Close()

		name := strings.TrimSuffix(filepath.Base(databasePath), filepath.Ext(databasePath))
		target := filepath.Join(
			backupDir,
			name+"-"+now().UTC().Format("20060102T150405.000000000Z")+filepath.Ext(databasePath),
		)
		temp := target + ".tmp"
		destination, err := os.OpenFile(temp, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return "", err
		}
		copied := false
		defer func() {
			_ = destination.Close()
			if !copied {
				_ = os.Remove(temp)
			}
		}()
		if _, err := io.Copy(destination, source); err != nil {
			return "", err
		}
		if err := destination.Sync(); err != nil {
			return "", err
		}
		if err := destination.Close(); err != nil {
			return "", err
		}
		if err := os.Rename(temp, target); err != nil {
			return "", err
		}
		copied = true
		return filepath.Base(target), nil
	}
}

func schemaTableExists(ctx context.Context, db *sql.DB) (bool, error) {
	var count int
	if err := db.QueryRowContext(
		ctx,
		`SELECT COUNT(*) FROM sqlite_master WHERE type = 'table' AND name = 'schema_migrations'`,
	).Scan(&count); err != nil {
		return false, fmt.Errorf("inspect migration table: %w", err)
	}
	return count == 1, nil
}

type appliedMigration struct {
	Name     string
	Checksum string
}

func loadApplied(ctx context.Context, db *sql.DB) (map[int]appliedMigration, error) {
	rows, err := db.QueryContext(ctx, `SELECT version, name, checksum FROM schema_migrations ORDER BY version`)
	if err != nil {
		return nil, fmt.Errorf("read migrations: %w", err)
	}
	defer rows.Close()
	applied := make(map[int]appliedMigration)
	for rows.Next() {
		var version int
		var record appliedMigration
		if err := rows.Scan(&version, &record.Name, &record.Checksum); err != nil {
			return nil, fmt.Errorf("scan migration: %w", err)
		}
		applied[version] = record
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate migrations: %w", err)
	}
	return applied, nil
}

func validateApplied(migrations []Migration, applied map[int]appliedMigration) error {
	known := make(map[int]Migration, len(migrations))
	for _, migration := range migrations {
		known[migration.Version] = migration
	}
	for version, record := range applied {
		migration, ok := known[version]
		if !ok {
			return fmt.Errorf("database contains unknown migration version %d", version)
		}
		actual := migrationChecksum(migration)
		if record.Checksum != actual {
			return &ChecksumError{Version: version, Expected: record.Checksum, Actual: actual}
		}
	}
	return nil
}

func pendingMigrations(migrations []Migration, applied map[int]appliedMigration) []Migration {
	pending := make([]Migration, 0, len(migrations))
	for _, migration := range migrations {
		if _, ok := applied[migration.Version]; !ok {
			pending = append(pending, migration)
		}
	}
	return pending
}

func normalizeMigrations(input []Migration) ([]Migration, error) {
	migrations := append([]Migration(nil), input...)
	sort.Slice(migrations, func(i, j int) bool { return migrations[i].Version < migrations[j].Version })
	seen := make(map[int]struct{}, len(migrations))
	for _, migration := range migrations {
		if migration.Version <= 0 {
			return nil, errors.New("migration version must be positive")
		}
		if strings.TrimSpace(migration.Name) == "" {
			return nil, fmt.Errorf("migration %d name is required", migration.Version)
		}
		if strings.TrimSpace(migration.SQL) == "" {
			return nil, fmt.Errorf("migration %d SQL is required", migration.Version)
		}
		if _, ok := seen[migration.Version]; ok {
			return nil, fmt.Errorf("duplicate migration version %d", migration.Version)
		}
		seen[migration.Version] = struct{}{}
	}
	return migrations, nil
}

func migrationChecksum(migration Migration) string {
	sum := sha256.Sum256([]byte(
		strconv.Itoa(migration.Version) + "\x00" + migration.Name + "\x00" + strings.TrimSpace(migration.SQL),
	))
	return hex.EncodeToString(sum[:])
}
