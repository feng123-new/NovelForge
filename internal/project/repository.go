package project

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/voocel/ainovel-cli/internal/db/migrate"
)

const (
	projectMetadataRelative = ".novelforge/project.json"
	projectDatabaseRelative = ".novelforge/project.db"
	projectConfigRelative   = ".novelforge/config.json"
)

var projectMigrations = []migrate.Migration{
	{
		Version: 1,
		Name:    "project_metadata",
		SQL: `CREATE TABLE project_metadata (
			id TEXT PRIMARY KEY,
			title TEXT NOT NULL,
			synopsis TEXT NOT NULL DEFAULT '',
			genre TEXT NOT NULL DEFAULT '',
			language TEXT NOT NULL DEFAULT '',
			target_words INTEGER NOT NULL DEFAULT 0,
			target_chapters INTEGER NOT NULL DEFAULT 0,
			words_per_chapter INTEGER NOT NULL DEFAULT 0,
			status TEXT NOT NULL,
			created_at TEXT NOT NULL,
			updated_at TEXT NOT NULL,
			archived_at TEXT,
			source_format TEXT NOT NULL DEFAULT ''
		);`,
	},
}

// Repository owns project filesystem lifecycle inside one workspace.
type Repository struct {
	workspace         string
	resolvedWorkspace string
	now               func() time.Time
	random            io.Reader
}

// NewRepository resolves a workspace once so every destructive operation can
// enforce the same boundary.
func NewRepository(workspace string) (*Repository, error) {
	absolute, err := filepath.Abs(workspace)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace: %w", err)
	}
	if err := os.MkdirAll(absolute, 0o755); err != nil {
		return nil, fmt.Errorf("prepare workspace: %w", err)
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return nil, fmt.Errorf("stat workspace: %w", err)
	}
	if !info.IsDir() {
		return nil, errors.New("workspace is not a directory")
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return nil, fmt.Errorf("resolve workspace links: %w", err)
	}
	return &Repository{
		workspace:         filepath.Clean(absolute),
		resolvedWorkspace: filepath.Clean(resolved),
		now:               time.Now,
		random:            rand.Reader,
	}, nil
}

// Workspace returns the internal absolute root. Transport code must never
// serialize it.
func (r *Repository) Workspace() string { return r.workspace }

// List returns a deterministic project page.
func (r *Repository) List(_ context.Context, options ListOptions) (ListResult, error) {
	if options.Limit <= 0 {
		options.Limit = 50
	}
	if options.Limit > 100 {
		options.Limit = 100
	}
	if options.Offset < 0 {
		return ListResult{}, newError(
			"PROJECT_VALIDATION_FAILED",
			"offset must not be negative",
			ErrValidation,
		)
	}
	entries, err := r.scan()
	if err != nil {
		return ListResult{}, err
	}
	query := strings.ToLower(strings.TrimSpace(options.Query))
	filtered := make([]Summary, 0, len(entries))
	for _, entry := range entries {
		if options.Archived != nil && entry.Project.Archived != *options.Archived {
			continue
		}
		if query != "" &&
			!strings.Contains(strings.ToLower(entry.Project.Title), query) &&
			!strings.Contains(strings.ToLower(entry.Project.Synopsis), query) {
			continue
		}
		filtered = append(filtered, entry.Project.Summary)
	}
	result := ListResult{
		Projects: []Summary{},
		Total:    len(filtered),
		Limit:    options.Limit,
		Offset:   options.Offset,
	}
	if options.Offset >= len(filtered) {
		return result, nil
	}
	end := options.Offset + options.Limit
	if end > len(filtered) {
		end = len(filtered)
	}
	result.Projects = append(result.Projects, filtered[options.Offset:end]...)
	if end < len(filtered) {
		next := end
		result.NextOffset = &next
	}
	return result, nil
}

// Get returns one project by opaque ID.
func (r *Repository) Get(_ context.Context, id string) (Project, error) {
	entry, err := r.find(id)
	if err != nil {
		return Project{}, err
	}
	return entry.Project, nil
}
