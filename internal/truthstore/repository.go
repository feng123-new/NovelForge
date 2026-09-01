package truthstore

import "context"

// Repository is the storage boundary consumed by project and transport code.
// SQLite is the V1 implementation; a future PostgreSQL adapter can implement
// the same contract without changing Truth semantics.
type Repository interface {
	Append(context.Context, AppendInput) (AppendResult, error)
	Events(context.Context, EventQuery) (EventPage, error)
	State(context.Context, StateQuery) (StatePage, error)
	StateMany(context.Context, []StateQuery) ([]StatePage, error)
	Conflicts(context.Context, ConflictQuery) (ConflictPage, error)
	Rebuild(context.Context, int) (RebuildResult, error)
	Verify(context.Context) (VerifyResult, error)
	Close() error
}

var _ Repository = (*Store)(nil)
