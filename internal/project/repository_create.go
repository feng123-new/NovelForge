package project

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (r *Repository) Create(ctx context.Context, input CreateInput) (Project, error) {
	if err := validateCreateInput(input); err != nil {
		return Project{}, err
	}
	id, err := r.newID()
	if err != nil {
		return Project{}, fmt.Errorf("generate project id: %w", err)
	}
	now := r.now().UTC()

	var (
		root              string
		createdRoot       bool
		novelForgeExisted bool
		importing         = strings.TrimSpace(input.ImportPath) != ""
	)
	if importing {
		root, err = r.resolveImportPath(input.ImportPath)
		if err != nil {
			return Project{}, err
		}
		if info, statErr := os.Lstat(filepath.Join(root, ".novelforge")); statErr == nil {
			if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
				return Project{}, newError(
					"PROJECT_IMPORT_PATH_INVALID",
					"existing project metadata directory is unsafe",
					ErrUnsafePath,
				)
			}
			novelForgeExisted = true
		} else if !errors.Is(statErr, os.ErrNotExist) {
			return Project{}, newError(
				"PROJECT_STORAGE_ERROR",
				"project metadata directory cannot be inspected",
				statErr,
			)
		}
		if _, err := os.Stat(filepath.Join(root, projectMetadataRelative)); err == nil {
			return Project{}, newError(
				"PROJECT_ALREADY_INITIALIZED",
				"project is already initialized",
				ErrConflict,
			)
		} else if !errors.Is(err, os.ErrNotExist) {
			return Project{}, newError(
				"PROJECT_STORAGE_ERROR",
				"project metadata cannot be inspected",
				err,
			)
		}
		if strings.TrimSpace(input.Title) == "" {
			legacy, readErr := readLegacyProject(r.workspace, root)
			if readErr != nil {
				return Project{}, readErr
			}
			input.Title = legacy.Title
			if input.Synopsis == "" {
				input.Synopsis = legacy.Synopsis
			}
		}
	} else {
		slug := normalizeSlug(input.Slug, input.Title, id)
		root = filepath.Join(r.workspace, slug)
		if err := ensureChildPath(r.workspace, root); err != nil {
			return Project{}, err
		}
		if _, err := os.Lstat(root); err == nil {
			return Project{}, newError(
				"PROJECT_PATH_CONFLICT",
				"a project directory with this name already exists",
				ErrConflict,
			)
		} else if !errors.Is(err, os.ErrNotExist) {
			return Project{}, newError(
				"PROJECT_STORAGE_ERROR",
				"project directory cannot be inspected",
				err,
			)
		}
		if err := os.Mkdir(root, 0o755); err != nil {
			return Project{}, newError(
				"PROJECT_STORAGE_ERROR",
				"project directory could not be created",
				err,
			)
		}
		createdRoot = true
	}
	rollback := true
	defer func() {
		if !rollback {
			return
		}
		if createdRoot {
			_ = os.RemoveAll(root)
			return
		}
		if importing {
			if !novelForgeExisted {
				_ = os.RemoveAll(filepath.Join(root, ".novelforge"))
				return
			}
			_ = os.Remove(filepath.Join(root, projectMetadataRelative))
			_ = os.Remove(filepath.Join(root, projectDatabaseRelative))
			_ = os.Remove(filepath.Join(root, projectDatabaseRelative+"-wal"))
			_ = os.Remove(filepath.Join(root, projectDatabaseRelative+"-shm"))
		}
	}()

	if err := initializeLayout(root); err != nil {
		return Project{}, newError(
			"PROJECT_STORAGE_ERROR",
			"project layout could not be created",
			err,
		)
	}
	metadata := Metadata{
		FormatVersion:   CurrentFormatVersion,
		ID:              id,
		Title:           strings.TrimSpace(input.Title),
		Synopsis:        strings.TrimSpace(input.Synopsis),
		Genre:           strings.TrimSpace(input.Genre),
		Language:        strings.TrimSpace(input.Language),
		TargetWords:     input.TargetWords,
		TargetChapters:  input.TargetChapters,
		WordsPerChapter: input.WordsPerChapter,
		Status:          StatusActive,
		CreatedAt:       now,
		UpdatedAt:       now,
		SourceFormat:    sourceFormat(importing),
	}
	if err := writeJSONAtomic(filepath.Join(root, projectMetadataRelative), metadata, 0o600); err != nil {
		return Project{}, newError(
			"PROJECT_STORAGE_ERROR",
			"project metadata could not be written",
			err,
		)
	}
	configPath := filepath.Join(root, projectConfigRelative)
	if _, err := os.Stat(configPath); errors.Is(err, os.ErrNotExist) {
		if err := writeJSONAtomic(configPath, defaultProjectConfig(), 0o600); err != nil {
			return Project{}, newError(
				"PROJECT_STORAGE_ERROR",
				"project configuration could not be written",
				err,
			)
		}
	} else if err != nil {
		return Project{}, newError(
			"PROJECT_STORAGE_ERROR",
			"project configuration could not be inspected",
			err,
		)
	}
	if err := r.initializeProjectDatabase(ctx, root); err != nil {
		return Project{}, err
	}
	if err := writeDatabaseMetadata(ctx, root, metadata); err != nil {
		return Project{}, err
	}
	rollback = false
	entry, err := r.read(root)
	if err != nil {
		return Project{}, err
	}
	return entry.Project, nil
}
