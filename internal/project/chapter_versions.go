package project

import (
	"context"
	"path/filepath"

	"github.com/voocel/ainovel-cli/internal/chapterversion"
)

func init() {
	projectMigrations = append(projectMigrations, chapterversion.Migration(), chapterversion.CounterMigration())
}

// OpenChapterVersionStore resolves an opaque project ID, applies all registered
// project migrations (including Phase 8) through the existing safe runner, and
// returns a store that owns only that project's database and chapter root.
func (r *Repository) OpenChapterVersionStore(ctx context.Context, id string) (*chapterversion.Store, error) {
	entry, err := r.find(id)
	if err != nil {
		return nil, err
	}
	if err := r.initializeProjectDatabase(ctx, entry.Root); err != nil {
		return nil, err
	}
	store, err := chapterversion.OpenExisting(filepath.Join(entry.Root, projectDatabaseRelative), entry.Root, entry.Project.ID)
	if err != nil {
		return nil, newError("PROJECT_CHAPTER_VERSION_STORE_ERROR", "project chapter version store could not be opened", err)
	}
	return store, nil
}
