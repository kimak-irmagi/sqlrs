package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"time"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
	"github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/resolver"
)

// Load implements resolver.Cache over the isolated Runtime v2 resolution
// namespace. Requirements: runtime-v2-persistence-structure.md.
func (s *Store) Load(ctx context.Context, key resolver.CacheKey, schema runtimev2.ExtensionIdentitySchema) (resolver.CacheLoad, error) {
	if err := ctx.Err(); err != nil {
		return resolver.CacheLoad{}, err
	}
	var version, storedAt string
	var raw []byte
	err := s.db.QueryRowContext(ctx, `SELECT record_version,cache_record_json,stored_at FROM runtime_v2_resolutions WHERE cache_key=?`, key.String()).Scan(&version, &raw, &storedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return resolver.CacheLoad{}, nil
	}
	if err != nil {
		return resolver.CacheLoad{}, err
	}
	if version != RuntimeV2RecordVersion {
		return resolver.CacheLoad{}, resolver.ErrIncompatibleCache
	}
	if !validRuntimeV2Timestamp(storedAt) {
		return resolver.CacheLoad{}, resolver.ErrCorruptCache
	}
	record, err := resolver.DecodeCacheRecordJSON(raw, schema)
	if err != nil {
		return resolver.CacheLoad{}, err
	}
	if !record.Matches(key) {
		return resolver.CacheLoad{}, resolver.ErrCorruptCache
	}
	return resolver.CacheLoad{Hit: true, Resolution: record.Resolution()}, nil
}

// Store atomically replaces evidence for one exact, versioned cache key.
func (s *Store) Store(ctx context.Context, key resolver.CacheKey, schema runtimev2.ExtensionIdentitySchema, resolution resolver.Resolution) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	record, err := resolver.NewCacheRecord(key, schema, resolution)
	if err != nil {
		return err
	}
	// NewCacheRecord has already validated the closed envelope.
	raw, _ := record.MarshalJSON()
	now := time.Now
	if s.now != nil {
		now = s.now
	}
	_, err = s.db.ExecContext(ctx, `
INSERT INTO runtime_v2_resolutions(cache_key,record_version,cache_record_json,stored_at)
VALUES(?,?,?,?)
ON CONFLICT(cache_key) DO UPDATE SET
  record_version=excluded.record_version,
  cache_record_json=excluded.cache_record_json,
  stored_at=excluded.stored_at`,
		key.String(), RuntimeV2RecordVersion, raw, now().UTC().Format("2006-01-02T15:04:05.000000000Z"))
	return err
}
