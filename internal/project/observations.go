package project

import (
	"context"
	"github.com/voocel/ainovel-cli/internal/observability"
)

func init() { projectMigrations = append(projectMigrations, observability.Migration()) }
func CurrentDatabaseSchema() int {
	n := 0
	for _, m := range projectMigrations {
		if m.Version > n {
			n = m.Version
		}
	}
	return n
}
func (r *Repository) OpenObservations(ctx context.Context, id string) (*observability.Store, func(), error) {
	store, err := r.OpenChapterVersionStore(ctx, id)
	if err != nil {
		return nil, func() {}, err
	}
	return &observability.Store{DB: store.Database(), ProjectID: id}, func() { _ = store.Close() }, nil
}
