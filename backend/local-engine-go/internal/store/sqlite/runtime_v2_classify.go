package sqlite

import (
	"context"
	"database/sql"

	"github.com/sqlrs/engine-local/internal/runtimev2store"
)

// ClassifyState reports namespace presence without decoding, adopting, or
// mutating legacy records.
func (s *Store) ClassifyState(ctx context.Context, rawID string) (runtimev2store.StateClassification, error) {
	var legacy int
	var version sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM states WHERE state_id=?), (SELECT record_version FROM runtime_v2_states WHERE state_id=? LIMIT 1)`, rawID, rawID).Scan(&legacy, &version)
	if err != nil {
		return runtimev2store.StateClassification{}, err
	}
	if version.Valid && version.String != RuntimeV2RecordVersion {
		return runtimev2store.StateClassification{}, runtimev2store.ErrCorrupt
	}
	return runtimev2store.StateClassification{LegacyPresent: legacy > 0, RuntimeV2Present: version.Valid, RuntimeV2RecordVersion: version.String}, nil
}
