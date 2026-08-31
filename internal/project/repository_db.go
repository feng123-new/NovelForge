package project

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/voocel/ainovel-cli/internal/db/migrate"
)

func (r *Repository) persistMetadata(
	ctx context.Context,
	root string,
	metadata Metadata,
) error {
	if err := initializeLayout(root); err != nil {
		return newError(
			"PROJECT_STORAGE_ERROR",
			"project layout could not be initialized",
			err,
		)
	}
	if err := writeJSONAtomic(filepath.Join(root, projectMetadataRelative), metadata, 0o600); err != nil {
		return newError(
			"PROJECT_STORAGE_ERROR",
			"project metadata could not be written",
			err,
		)
	}
	if err := r.initializeProjectDatabase(ctx, root); err != nil {
		return err
	}
	return writeDatabaseMetadata(ctx, root, metadata)
}

func (r *Repository) initializeProjectDatabase(ctx context.Context, root string) error {
	path := filepath.Join(root, projectDatabaseRelative)
	runner := migrate.Runner{
		Path:        path,
		Migrations:  projectMigrations,
		BusyTimeout: 5 * time.Second,
		Backup: migrate.TimestampedCopyBackup(
			filepath.Join(root, ".novelforge", "backups"),
			r.now,
		),
	}
	if err := runner.Run(ctx); err != nil {
		return newError(
			"PROJECT_DATABASE_MIGRATION_FAILED",
			"project database migration failed",
			err,
		)
	}
	return nil
}

func writeDatabaseMetadata(ctx context.Context, root string, metadata Metadata) error {
	db, err := migrate.Open(filepath.Join(root, projectDatabaseRelative), 5*time.Second)
	if err != nil {
		return newError(
			"PROJECT_DATABASE_OPEN_FAILED",
			"project database could not be opened",
			err,
		)
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return newError(
			"PROJECT_DATABASE_WRITE_FAILED",
			"project database transaction could not start",
			err,
		)
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `DELETE FROM project_metadata`); err != nil {
		return databaseWriteError(err)
	}
	if _, err := tx.ExecContext(
		ctx,
		`INSERT INTO project_metadata(
			id, title, synopsis, genre, language, target_words, target_chapters,
			words_per_chapter, status, created_at, updated_at, archived_at,
			source_format
		) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		metadata.ID,
		metadata.Title,
		metadata.Synopsis,
		metadata.Genre,
		metadata.Language,
		metadata.TargetWords,
		metadata.TargetChapters,
		metadata.WordsPerChapter,
		metadata.Status,
		metadata.CreatedAt.UTC().Format(time.RFC3339Nano),
		metadata.UpdatedAt.UTC().Format(time.RFC3339Nano),
		nullableTime(metadata.ArchivedAt),
		metadata.SourceFormat,
	); err != nil {
		return databaseWriteError(err)
	}
	if err := tx.Commit(); err != nil {
		return databaseWriteError(err)
	}
	return nil
}

func checkpointProjectDatabase(ctx context.Context, root string) error {
	path := filepath.Join(root, projectDatabaseRelative)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil
	} else if err != nil {
		return newError(
			"PROJECT_DATABASE_OPEN_FAILED",
			"project database could not be inspected",
			err,
		)
	}
	db, err := migrate.Open(path, 5*time.Second)
	if err != nil {
		return newError(
			"PROJECT_DATABASE_OPEN_FAILED",
			"project database could not be opened",
			err,
		)
	}
	defer db.Close()
	if _, err := db.ExecContext(ctx, `PRAGMA wal_checkpoint(TRUNCATE)`); err != nil {
		return newError(
			"PROJECT_DATABASE_CHECKPOINT_FAILED",
			"project database could not be checkpointed",
			err,
		)
	}
	return nil
}

func databaseWriteError(err error) error {
	return newError(
		"PROJECT_DATABASE_WRITE_FAILED",
		"project database metadata could not be written",
		err,
	)
}

func nullableTime(value *time.Time) any {
	if value == nil {
		return nil
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func validateCreateInput(input CreateInput) error {
	if strings.TrimSpace(input.Title) == "" && strings.TrimSpace(input.ImportPath) == "" {
		return newError(
			"PROJECT_VALIDATION_FAILED",
			"title is required",
			ErrValidation,
		)
	}
	for name, value := range map[string]int{
		"target_words":      input.TargetWords,
		"target_chapters":   input.TargetChapters,
		"words_per_chapter": input.WordsPerChapter,
	} {
		if err := validateNonNegative(name, value); err != nil {
			return err
		}
	}
	if len([]rune(strings.TrimSpace(input.Title))) > 200 {
		return newError(
			"PROJECT_VALIDATION_FAILED",
			"title must be at most 200 characters",
			ErrValidation,
		)
	}
	return nil
}

func metadataFromProject(project Project, now time.Time) Metadata {
	created := project.CreatedAt
	if created.IsZero() {
		created = now
	}
	status := project.Status
	if status == "" {
		status = StatusActive
	}
	return Metadata{
		FormatVersion:   CurrentFormatVersion,
		ID:              project.ID,
		Title:           project.Title,
		Synopsis:        project.Synopsis,
		Genre:           project.Genre,
		Language:        project.Language,
		TargetWords:     project.TargetWords,
		TargetChapters:  project.TotalChapters,
		WordsPerChapter: project.WordsPerChapter,
		Status:          status,
		CreatedAt:       created.UTC(),
		UpdatedAt:       now.UTC(),
		ArchivedAt:      project.ArchivedAt,
		SourceFormat:    project.SourceFormat,
	}
}

func sourceFormat(importing bool) string {
	if importing {
		return "imported-skeleton"
	}
	return "novelforge"
}

func defaultProjectConfig() map[string]any {
	return map[string]any{
		"version":       1,
		"model_profile": "",
		"automation":    "copilot",
	}
}

func (r *Repository) newID() (string, error) {
	return randomHex(r.random, 16)
}
