package store

import "context"

// PrepareStateProtection reads durable source ownership before deletion or
// eviction touches files. Implementations must return an error for an unreadable
// installed owner namespace. Historical released clones do not pin sources.
// Requirements: issue89-planning-db-handoff-schema.md §5, PG07.
type PrepareStateProtection interface {
	StateProtectedByPrepare(context.Context, string) (bool, error)
}

// PrepareProtectsState preserves compatibility with stores that do not host the
// prepare journal. The SQLite store hosting O5 always implements this reader;
// its read failures are propagated, never interpreted as absence of ownership.
func PrepareProtectsState(ctx context.Context, source Store, stateID string) (bool, error) {
	if reader, ok := source.(PrepareStateProtection); ok {
		return reader.StateProtectedByPrepare(ctx, stateID)
	}
	return false, nil
}
