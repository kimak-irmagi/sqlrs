package sqlite

import "context"

// StateProtectedByPrepare exposes the existing immutable clone-source guard
// before physical removal. Counting clone rows gives one physical owner across
// planning/session handoff; released history adds no permanent source pin.
// Requirements: issue89-planning-db-handoff-schema.md §§4–5, PG07.
func (s *Store) StateProtectedByPrepare(ctx context.Context, stateID string) (bool, error) {
	var installed int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master WHERE type='table' AND name='prepare_clones'").Scan(&installed); err != nil {
		return false, err
	}
	// Standalone legacy stores do not own a prepare namespace. Managed startup
	// independently validates the complete installed catalog before admission.
	if installed == 0 {
		return false, nil
	}
	var protected bool
	err := s.db.QueryRowContext(ctx, "SELECT EXISTS(SELECT 1 FROM prepare_clones WHERE source_state_id=? AND phase<>'released')", stateID).Scan(&protected)
	return protected, err
}
