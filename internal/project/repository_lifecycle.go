package project

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func (r *Repository) Update(ctx context.Context, id string, input UpdateInput) (Project, error) {
	entry, err := r.find(id)
	if err != nil {
		return Project{}, err
	}
	metadata := entry.Metadata
	if metadata.ID == "" {
		metadata = metadataFromProject(entry.Project, r.now().UTC())
	}
	if input.Title != nil {
		title := strings.TrimSpace(*input.Title)
		if title == "" {
			return Project{}, newError(
				"PROJECT_VALIDATION_FAILED",
				"title is required",
				ErrValidation,
			)
		}
		metadata.Title = title
	}
	if input.Synopsis != nil {
		metadata.Synopsis = strings.TrimSpace(*input.Synopsis)
	}
	if input.Genre != nil {
		metadata.Genre = strings.TrimSpace(*input.Genre)
	}
	if input.Language != nil {
		metadata.Language = strings.TrimSpace(*input.Language)
	}
	if input.TargetWords != nil {
		if err := validateNonNegative("target_words", *input.TargetWords); err != nil {
			return Project{}, err
		}
		metadata.TargetWords = *input.TargetWords
	}
	if input.TargetChapters != nil {
		if err := validateNonNegative("target_chapters", *input.TargetChapters); err != nil {
			return Project{}, err
		}
		metadata.TargetChapters = *input.TargetChapters
	}
	if input.WordsPerChapter != nil {
		if err := validateNonNegative("words_per_chapter", *input.WordsPerChapter); err != nil {
			return Project{}, err
		}
		metadata.WordsPerChapter = *input.WordsPerChapter
	}
	metadata.UpdatedAt = r.now().UTC()
	if err := r.persistMetadata(ctx, entry.Root, metadata); err != nil {
		return Project{}, err
	}
	updated, err := r.read(entry.Root)
	if err != nil {
		return Project{}, err
	}
	return updated.Project, nil
}

// SetArchived changes project visibility without moving or deleting files.
func (r *Repository) SetArchived(ctx context.Context, id string, archived bool) (Project, error) {
	entry, err := r.find(id)
	if err != nil {
		return Project{}, err
	}
	metadata := entry.Metadata
	if metadata.ID == "" {
		metadata = metadataFromProject(entry.Project, r.now().UTC())
	}
	now := r.now().UTC()
	if archived {
		metadata.Status = StatusArchived
		metadata.ArchivedAt = &now
	} else {
		metadata.Status = StatusActive
		metadata.ArchivedAt = nil
	}
	metadata.UpdatedAt = now
	if err := r.persistMetadata(ctx, entry.Root, metadata); err != nil {
		return Project{}, err
	}
	updated, err := r.read(entry.Root)
	if err != nil {
		return Project{}, err
	}
	return updated.Project, nil
}

// Duplicate copies project data while excluding credentials, transient locks,
// backups and trash.
func (r *Repository) Duplicate(
	ctx context.Context,
	id string,
	input DuplicateInput,
) (Project, error) {
	entry, err := r.find(id)
	if err != nil {
		return Project{}, err
	}
	newID, err := r.newID()
	if err != nil {
		return Project{}, fmt.Errorf("generate project id: %w", err)
	}
	title := strings.TrimSpace(input.Title)
	if title == "" {
		title = entry.Project.Title + " Copy"
	}
	slug := normalizeSlug(input.Slug, title, newID)
	destination := filepath.Join(r.workspace, slug)
	if err := ensureChildPath(r.workspace, destination); err != nil {
		return Project{}, err
	}
	if _, err := os.Lstat(destination); err == nil {
		return Project{}, newError(
			"PROJECT_PATH_CONFLICT",
			"a project directory with this name already exists",
			ErrConflict,
		)
	} else if !errors.Is(err, os.ErrNotExist) {
		return Project{}, newError(
			"PROJECT_STORAGE_ERROR",
			"duplicate destination cannot be inspected",
			err,
		)
	}
	if err := checkpointProjectDatabase(ctx, entry.Root); err != nil {
		return Project{}, err
	}
	if err := copyProjectTree(entry.Root, destination); err != nil {
		_ = os.RemoveAll(destination)
		return Project{}, err
	}
	complete := false
	defer func() {
		if !complete {
			_ = os.RemoveAll(destination)
		}
	}()

	now := r.now().UTC()
	metadata := entry.Metadata
	if metadata.ID == "" {
		metadata = metadataFromProject(entry.Project, now)
	}
	metadata.ID = newID
	metadata.Title = title
	metadata.Status = StatusActive
	metadata.ArchivedAt = nil
	metadata.CreatedAt = now
	metadata.UpdatedAt = now
	metadata.SourceFormat = "duplicate"
	metadata.FormatVersion = CurrentFormatVersion

	if err := initializeLayout(destination); err != nil {
		return Project{}, newError(
			"PROJECT_STORAGE_ERROR",
			"duplicate layout could not be initialized",
			err,
		)
	}
	if err := writeJSONAtomic(filepath.Join(destination, projectMetadataRelative), metadata, 0o600); err != nil {
		return Project{}, newError(
			"PROJECT_STORAGE_ERROR",
			"duplicate metadata could not be written",
			err,
		)
	}
	if err := sanitizeProjectConfig(entry.Root, destination); err != nil {
		return Project{}, err
	}
	if err := r.initializeProjectDatabase(ctx, destination); err != nil {
		return Project{}, err
	}
	if err := writeDatabaseMetadata(ctx, destination, metadata); err != nil {
		return Project{}, err
	}
	complete = true
	duplicated, err := r.read(destination)
	if err != nil {
		return Project{}, err
	}
	return duplicated.Project, nil
}

// Delete moves a project to workspace trash by default. Permanent deletion is
// available only through an explicit input flag and the same confirmation.
func (r *Repository) Delete(
	_ context.Context,
	id string,
	input DeleteInput,
) (DeleteResult, error) {
	entry, err := r.find(id)
	if err != nil {
		return DeleteResult{}, err
	}
	confirmation := strings.TrimSpace(input.Confirm)
	if confirmation != entry.Project.ID && confirmation != entry.Project.Title {
		return DeleteResult{}, newError(
			"PROJECT_CONFIRMATION_MISMATCH",
			"confirmation must match the project ID or title",
			ErrConfirmation,
		)
	}
	if err := r.validateDestructiveRoot(entry.Root); err != nil {
		return DeleteResult{}, err
	}

	if input.Permanent {
		if err := os.RemoveAll(entry.Root); err != nil {
			return DeleteResult{}, newError(
				"PROJECT_DELETE_FAILED",
				"project could not be permanently deleted",
				err,
			)
		}
		return DeleteResult{ID: id, Deleted: true, Permanent: true}, nil
	}

	trashRoot := filepath.Join(r.workspace, ".novelforge", "trash")
	if err := os.MkdirAll(trashRoot, 0o700); err != nil {
		return DeleteResult{}, newError(
			"PROJECT_DELETE_FAILED",
			"workspace trash could not be prepared",
			err,
		)
	}
	suffix, err := r.newID()
	if err != nil {
		return DeleteResult{}, fmt.Errorf("generate trash id: %w", err)
	}
	destination := filepath.Join(
		trashRoot,
		id+"-"+r.now().UTC().Format("20060102T150405.000000000Z")+"-"+suffix[:8],
	)
	if err := ensureChildPath(trashRoot, destination); err != nil {
		return DeleteResult{}, err
	}
	if err := os.Rename(entry.Root, destination); err != nil {
		return DeleteResult{}, newError(
			"PROJECT_DELETE_FAILED",
			"project could not be moved to workspace trash",
			err,
		)
	}
	tombstone := map[string]any{
		"id":         id,
		"title":      entry.Project.Title,
		"deleted_at": r.now().UTC(),
		"permanent":  false,
	}
	if err := writeJSONAtomic(
		filepath.Join(destination, ".novelforge", "deleted.json"),
		tombstone,
		0o600,
	); err != nil {
		// The safe move already completed. Keep the project in trash and return
		// success; the durable API event remains the authoritative audit record.
	}
	return DeleteResult{ID: id, Deleted: true, Permanent: false}, nil
}
