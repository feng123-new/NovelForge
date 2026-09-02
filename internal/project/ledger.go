package project

import (
	"context"
	"path/filepath"
	"time"

	"github.com/voocel/ainovel-cli/internal/narrativeledger"
)

func (r *Repository) OpenNarrativeLedger(ctx context.Context, id string) (*narrativeledger.Store, error) {
	entry, err := r.find(id)
	if err != nil {
		return nil, err
	}
	if err := r.initializeProjectDatabase(ctx, entry.Root); err != nil {
		return nil, err
	}
	store, err := narrativeledger.OpenExisting(filepath.Join(entry.Root, projectDatabaseRelative), 5*time.Second)
	if err != nil {
		return nil, newError("PROJECT_LEDGER_ERROR", "project narrative ledger could not be opened", err)
	}
	return store, nil
}
