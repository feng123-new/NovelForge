package compat

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const (
	MigrationManifestVersion = 1
	MigrationManifestName    = "migration-manifest.json"
)

type MigrationOptions struct {
	Home           string
	ProjectRoot    string
	IncludeGlobal  bool
	IncludeProject bool
	DryRun         bool
	Now            func() time.Time
}

type MigrationReport struct {
	Version   int               `json:"version"`
	DryRun    bool              `json:"dry_run"`
	CreatedAt time.Time         `json:"created_at"`
	Actions   []MigrationAction `json:"actions"`
}

type MigrationAction struct {
	Scope       Scope  `json:"scope"`
	Source      string `json:"source"`
	Destination string `json:"destination"`
	Backup      string `json:"backup,omitempty"`
	Status      string `json:"status"`
	Message     string `json:"message,omitempty"`
}

type MigrationManifest struct {
	Version     int                 `json:"version"`
	Scope       Scope               `json:"scope"`
	Source      string              `json:"source"`
	Destination string              `json:"destination"`
	CreatedAt   time.Time           `json:"created_at"`
	Files       []MigrationFileInfo `json:"files"`
}

type MigrationFileInfo struct {
	Path   string `json:"path"`
	Size   int64  `json:"size"`
	SHA256 string `json:"sha256"`
}

type migrationSpec struct {
	scope       Scope
	source      string
	destination string
	backupRoot  string
}

func Migrate(ctx context.Context, opts MigrationOptions) (MigrationReport, error) {
	now := time.Now().UTC()
	if opts.Now != nil {
		now = opts.Now().UTC()
	}
	report := MigrationReport{
		Version:   MigrationManifestVersion,
		DryRun:    opts.DryRun,
		CreatedAt: now,
	}

	paths, err := ResolvePaths(opts.Home, opts.ProjectRoot)
	if err != nil {
		return report, err
	}
	includeGlobal, includeProject := opts.IncludeGlobal, opts.IncludeProject
	if !includeGlobal && !includeProject {
		includeGlobal, includeProject = true, true
	}

	var specs []migrationSpec
	if includeGlobal {
		specs = append(specs, migrationSpec{
			scope:       ScopeGlobal,
			source:      paths.GlobalLegacyDir,
			destination: paths.GlobalNovelForgeDir,
			backupRoot:  filepath.Join(paths.Home, ProductDirName+"-migration-backups"),
		})
	}
	if includeProject {
		specs = append(specs, migrationSpec{
			scope:       ScopeProject,
			source:      paths.ProjectLegacyDir,
			destination: paths.ProjectNovelForgeDir,
			backupRoot:  filepath.Join(paths.ProjectRoot, ProductDirName+"-migration-backups"),
		})
	}

	for _, spec := range specs {
		action, err := migrateOne(ctx, spec, opts.DryRun, now)
		report.Actions = append(report.Actions, action)
		if err != nil {
			return report, err
		}
	}
	return report, nil
}

func migrateOne(ctx context.Context, spec migrationSpec, dryRun bool, now time.Time) (MigrationAction, error) {
	action := MigrationAction{
		Scope:       spec.scope,
		Source:      spec.source,
		Destination: spec.destination,
	}

	sourceInfo, err := os.Lstat(spec.source)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			action.Status = "source_missing"
			action.Message = "legacy directory does not exist"
			return action, nil
		}
		action.Status = "failed"
		return action, fmt.Errorf("stat %s source: %w", spec.scope, err)
	}
	if sourceInfo.Mode()&os.ModeSymlink != 0 || !sourceInfo.IsDir() {
		action.Status = "failed"
		return action, fmt.Errorf("%s source must be a real directory: %s", spec.scope, spec.source)
	}

	if destinationInfo, err := os.Lstat(spec.destination); err == nil {
		if destinationInfo.Mode()&os.ModeSymlink != 0 || !destinationInfo.IsDir() {
			action.Status = "failed"
			return action, fmt.Errorf("%s destination exists but is not a real directory: %s", spec.scope, spec.destination)
		}
		action.Status = "destination_exists"
		action.Message = "destination already exists; no files were overwritten"
		return action, nil
	} else if !errors.Is(err, os.ErrNotExist) {
		action.Status = "failed"
		return action, fmt.Errorf("stat %s destination: %w", spec.scope, err)
	}

	stamp := now.Format("20060102T150405Z")
	backup := filepath.Join(spec.backupRoot, stamp+"-"+string(spec.scope)+"-ainovel")
	action.Backup = backup
	if dryRun {
		action.Status = "planned"
		action.Message = "would back up and copy the legacy directory"
		return action, nil
	}

	if err := ctx.Err(); err != nil {
		action.Status = "failed"
		return action, err
	}
	if err := os.MkdirAll(spec.backupRoot, 0o700); err != nil {
		action.Status = "failed"
		return action, fmt.Errorf("create %s backup root: %w", spec.scope, err)
	}
	if _, err := os.Lstat(backup); err == nil {
		backup = filepath.Join(spec.backupRoot, stamp+"-"+string(spec.scope)+"-ainovel-"+shortDigest(spec.source))
		action.Backup = backup
	}

	backupFiles, err := copyDirectory(ctx, spec.source, backup)
	if err != nil {
		action.Status = "failed"
		_ = os.RemoveAll(backup)
		return action, fmt.Errorf("back up %s configuration: %w", spec.scope, err)
	}
	manifest := MigrationManifest{
		Version:     MigrationManifestVersion,
		Scope:       spec.scope,
		Source:      spec.source,
		Destination: spec.destination,
		CreatedAt:   now,
		Files:       backupFiles,
	}
	if err := writeManifest(filepath.Join(backup, MigrationManifestName), manifest); err != nil {
		action.Status = "failed"
		_ = os.RemoveAll(backup)
		return action, fmt.Errorf("write %s backup manifest: %w", spec.scope, err)
	}

	parent := filepath.Dir(spec.destination)
	if err := os.MkdirAll(parent, 0o755); err != nil {
		action.Status = "failed"
		return action, fmt.Errorf("prepare %s destination parent: %w", spec.scope, err)
	}
	stage, err := os.MkdirTemp(parent, ".novelforge-migrate-*")
	if err != nil {
		action.Status = "failed"
		return action, fmt.Errorf("create %s migration stage: %w", spec.scope, err)
	}
	stageCommitted := false
	defer func() {
		if !stageCommitted {
			_ = os.RemoveAll(stage)
		}
	}()

	stageFiles, err := copyDirectoryContents(ctx, spec.source, stage)
	if err != nil {
		action.Status = "failed"
		return action, fmt.Errorf("stage %s configuration: %w", spec.scope, err)
	}
	manifest.Files = stageFiles
	if err := writeManifest(filepath.Join(stage, MigrationManifestName), manifest); err != nil {
		action.Status = "failed"
		return action, fmt.Errorf("write %s destination manifest: %w", spec.scope, err)
	}
	if err := os.Rename(stage, spec.destination); err != nil {
		action.Status = "failed"
		return action, fmt.Errorf("commit %s migration: %w", spec.scope, err)
	}
	stageCommitted = true
	action.Status = "migrated"
	action.Message = "legacy directory retained; destination created atomically"
	return action, nil
}

func copyDirectory(ctx context.Context, source, destination string) ([]MigrationFileInfo, error) {
	if err := os.Mkdir(destination, 0o700); err != nil {
		return nil, err
	}
	return copyDirectoryContents(ctx, source, destination)
}

func copyDirectoryContents(ctx context.Context, source, destination string) ([]MigrationFileInfo, error) {
	var files []MigrationFileInfo
	err := filepath.WalkDir(source, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if path == source {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symbolic links are not migrated: %s", path)
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if relative == "." || relative == "" || strings.HasPrefix(relative, ".."+string(filepath.Separator)) || relative == ".." {
			return fmt.Errorf("invalid migration path %q", relative)
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, info.Mode().Perm())
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("unsupported file type: %s", path)
		}
		digest, size, err := copyRegularFile(path, target, info.Mode().Perm())
		if err != nil {
			return err
		}
		files = append(files, MigrationFileInfo{
			Path:   filepath.ToSlash(relative),
			Size:   size,
			SHA256: digest,
		})
		return nil
	})
	sort.Slice(files, func(i, j int) bool { return files[i].Path < files[j].Path })
	return files, err
}

func copyRegularFile(source, destination string, mode fs.FileMode) (string, int64, error) {
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return "", 0, err
	}
	input, err := os.Open(source)
	if err != nil {
		return "", 0, err
	}
	defer input.Close()
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_EXCL|os.O_WRONLY, mode)
	if err != nil {
		return "", 0, err
	}
	committed := false
	defer func() {
		_ = output.Close()
		if !committed {
			_ = os.Remove(destination)
		}
	}()
	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(output, hash), input)
	if err != nil {
		return "", 0, err
	}
	if err := output.Sync(); err != nil {
		return "", 0, err
	}
	if err := output.Close(); err != nil {
		return "", 0, err
	}
	committed = true
	return hex.EncodeToString(hash.Sum(nil)), size, nil
}

func writeManifest(path string, manifest MigrationManifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp, err := os.CreateTemp(filepath.Dir(path), ".migration-manifest-*.tmp")
	if err != nil {
		return err
	}
	tmpPath := tmp.Name()
	committed := false
	defer func() {
		_ = tmp.Close()
		if !committed {
			_ = os.Remove(tmpPath)
		}
	}()
	if err := tmp.Chmod(0o600); err != nil {
		return err
	}
	if _, err := tmp.Write(data); err != nil {
		return err
	}
	if err := tmp.Sync(); err != nil {
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpPath, path); err != nil {
		return err
	}
	committed = true
	return nil
}

func shortDigest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return hex.EncodeToString(sum[:4])
}
