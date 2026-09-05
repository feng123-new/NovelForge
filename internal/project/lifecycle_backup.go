package project

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/voocel/ainovel-cli/internal/db/migrate"
	"github.com/voocel/ainovel-cli/internal/lifecycle"
)

func readLifecycleFile(p string, limit int64) ([]byte, error) {
	st, err := os.Lstat(p)
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, ErrUnsafePath
	}
	if st.Size() > limit {
		return nil, lifecycle.ErrLimit
	}
	f, err := os.Open(p)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, limit+1))
	if len(b) > int(limit) {
		return nil, lifecycle.ErrLimit
	}
	return b, err
}
func safeLifecycleDirectory(root string, parts ...string) (string, error) {
	p := root
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || strings.ContainsAny(part, "/\\") {
			return "", ErrUnsafePath
		}
		p = filepath.Join(p, part)
		st, err := os.Lstat(p)
		if errors.Is(err, os.ErrNotExist) {
			if err = os.Mkdir(p, 0700); err != nil {
				return "", err
			}
			continue
		}
		if err != nil {
			return "", err
		}
		if st.Mode()&os.ModeSymlink != 0 || !st.IsDir() {
			return "", ErrUnsafePath
		}
	}
	return p, nil
}

// Caller holds the project OS lease. VACUUM INTO captures committed WAL frames
// into a standalone database; copying the live .db file is not sufficient.
func (r *Repository) BackupLifecycle(ctx context.Context, id string) ([]byte, error) {
	e, err := r.lifecycleRoot(id)
	if err != nil {
		return nil, err
	}
	dir, err := os.MkdirTemp("", "novelforge-snapshot-")
	if err != nil {
		return nil, err
	}
	defer os.RemoveAll(dir)
	db, err := migrate.Open(filepath.Join(e.Root, projectDatabaseRelative), 5*time.Second)
	if err != nil {
		return nil, err
	}
	var schema int
	if err = db.QueryRowContext(ctx, `SELECT coalesce(max(version),0) FROM schema_migrations`).Scan(&schema); err != nil {
		db.Close()
		return nil, err
	}
	snapshot := filepath.Join(dir, "project.db")
	_, err = db.ExecContext(ctx, `VACUUM INTO ?`, snapshot)
	closeErr := db.Close()
	if err != nil {
		return nil, err
	}
	if closeErr != nil {
		return nil, closeErr
	}
	m := lifecycle.Manifest{ProjectID: id, Title: e.Metadata.Title, Format: e.Metadata.FormatVersion, Schema: schema, Created: r.now().UTC()}
	if err = verifyLifecycleDB(ctx, snapshot, m); err != nil {
		return nil, err
	}
	files := map[string][]byte{}
	files[projectDatabaseRelative], err = readLifecycleFile(snapshot, lifecycle.MaxExpanded)
	if err != nil {
		return nil, err
	}
	files[projectMetadataRelative], err = json.Marshal(e.Metadata)
	if err != nil {
		return nil, err
	}
	total := len(files[projectDatabaseRelative]) + len(files[projectMetadataRelative])
	add := func(name, p string) error {
		if !lifecycle.BackupPath(name) {
			return nil
		}
		b, err := readLifecycleFile(p, int64(lifecycle.MaxExpanded-total))
		if err != nil {
			return err
		}
		total += len(b)
		if len(files) >= lifecycle.MaxFiles-2 || total > lifecycle.MaxExpanded {
			return lifecycle.ErrLimit
		}
		files[name] = b
		return nil
	}
	for _, name := range []string{foundationRequestRelative, ".novelforge/foundation-output.json"} {
		p := filepath.Join(e.Root, filepath.FromSlash(name))
		if _, err = os.Lstat(p); errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		if err = add(name, p); err != nil {
			return nil, err
		}
	}
	for _, name := range []string{"chapters", "references", ".novelforge/skills", ".novelforge/style", ".novelforge/rules"} {
		base := filepath.Join(e.Root, filepath.FromSlash(name))
		if _, err = os.Lstat(base); errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return nil, err
		}
		err = filepath.WalkDir(base, func(p string, d fs.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if d.Type()&os.ModeSymlink != 0 {
				return ErrUnsafePath
			}
			if d.IsDir() {
				return nil
			}
			relative, err := filepath.Rel(e.Root, p)
			if err != nil {
				return err
			}
			return add(filepath.ToSlash(relative), p)
		})
		if err != nil {
			return nil, err
		}
	}
	if err = verifyLifecycleFiles(ctx, snapshot, m, files); err != nil {
		return nil, err
	}
	return lifecycle.Pack(m, files)
}

type LifecycleMigrationResult struct {
	From     int    `json:"from_format"`
	To       int    `json:"to_format"`
	Schema   int    `json:"schema_version"`
	BackupID string `json:"backup_id"`
	Changed  bool   `json:"changed"`
}

func (r *Repository) MigrateLifecycle(ctx context.Context, id, key string, expected int) (LifecycleMigrationResult, error) {
	e, err := r.lifecycleRoot(id)
	if err != nil {
		return LifecycleMigrationResult{}, err
	}
	result := LifecycleMigrationResult{From: e.Metadata.FormatVersion, To: CurrentFormatVersion, Schema: CurrentDatabaseSchema()}
	if expected != e.Metadata.FormatVersion {
		return result, lifecycle.ErrConflict
	}
	db, err := lifecycleReadDB(filepath.Join(e.Root, projectDatabaseRelative))
	if err != nil {
		return result, err
	}
	var schema int
	err = db.QueryRowContext(ctx, `SELECT coalesce(max(version),0) FROM schema_migrations`).Scan(&schema)
	db.Close()
	if err != nil {
		return result, err
	}
	if schema == CurrentDatabaseSchema() && expected == CurrentFormatVersion {
		return result, nil
	}
	archive, err := r.BackupLifecycle(ctx, id)
	if err != nil {
		return result, err
	}
	dir, err := safeLifecycleDirectory(e.Root, ".novelforge", "backups")
	if err != nil {
		return result, err
	}
	result.BackupID = "lifecycle-migrate-" + lifecycle.SHA([]byte(id+":"+key)) + ".zip"
	file, err := os.OpenFile(filepath.Join(dir, result.BackupID), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err == nil {
		_, err = file.Write(archive)
		if err == nil {
			err = file.Sync()
		}
		ce := file.Close()
		if err == nil {
			err = ce
		}
		if err != nil {
			return result, err
		}
	} else if !errors.Is(err, os.ErrExist) {
		return result, err
	} else {
		prior, readErr := readLifecycleFile(filepath.Join(dir, result.BackupID), lifecycle.MaxArchive)
		if readErr != nil {
			return result, readErr
		}
		manifest, _, readErr := lifecycle.Unpack(prior)
		if readErr != nil || manifest.ProjectID != id || manifest.Format != expected {
			return result, lifecycle.ErrConflict
		}
	}
	if err = r.initializeProjectDatabase(ctx, e.Root); err != nil {
		return result, err
	}
	meta := e.Metadata
	meta.FormatVersion = CurrentFormatVersion
	meta.UpdatedAt = r.now().UTC()
	if err = r.persistMetadata(ctx, e.Root, meta); err != nil {
		return result, err
	}
	result.Changed = true
	return result, nil
}
func (r *Repository) ReadLifecycleBackup(ctx context.Context, id, name string) ([]byte, error) {
	if len(name) != len("lifecycle-migrate-")+64+4 || !strings.HasPrefix(name, "lifecycle-migrate-") || !strings.HasSuffix(name, ".zip") || !lifecycle.SafeName(name) || strings.Contains(name, "/") {
		return nil, lifecycle.ErrInvalid
	}
	e, err := r.lifecycleRoot(id)
	if err != nil {
		return nil, err
	}
	dir, err := safeLifecycleDirectory(e.Root, ".novelforge", "backups")
	if err != nil {
		return nil, err
	}
	return readLifecycleFile(filepath.Join(dir, name), lifecycle.MaxArchive)
}
