package project

import (
	"context"
	"path/filepath"
	"time"

	"github.com/voocel/ainovel-cli/internal/truthstore"
)

func (r *Repository) OpenTruthStore(ctx context.Context, id string) (*truthstore.Store, error) {
	entry, err := r.find(id)
	if err != nil {
		return nil, err
	}
	root := entry.Root
	if err := r.initializeProjectDatabase(ctx, root); err != nil {
		return nil, err
	}
	store, err := truthstore.OpenExisting(filepath.Join(root, projectDatabaseRelative), 5*time.Second)
	if err != nil {
		return nil, newError("PROJECT_TRUTH_STORE_ERROR", "project truth store could not be opened", err)
	}
	return store, nil
}
