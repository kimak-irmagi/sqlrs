package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "modernc.org/sqlite"

	"github.com/sqlrs/engine-local/internal/store"
)

type Store struct {
	db *sql.DB
}

func Open(path string) (*Store, error) {
	if path == "" {
		return nil, fmt.Errorf("sqlite path is empty")
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	if err := initDB(db); err != nil {
		_ = db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func New(db *sql.DB) (*Store, error) {
	if db == nil {
		return nil, fmt.Errorf("db is required")
	}
	if err := initDB(db); err != nil {
		return nil, err
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error {
	if s == nil || s.db == nil {
		return nil
	}
	return s.db.Close()
}

func (s *Store) ListNames(ctx context.Context, filters store.NameFilters) ([]store.NameEntry, error) {
	query := strings.Builder{}
	query.WriteString(`
SELECT n.name, n.instance_id, n.image_id, n.state_id, n.state_fingerprint, n.last_used_at, i.expires_at, i.instance_id
FROM names n
LEFT JOIN instances i ON i.instance_id = n.instance_id
WHERE 1=1`)
	args := []any{}
	addFilter(&query, &args, "n.instance_id", filters.InstanceID)
	addFilter(&query, &args, "n.state_id", filters.StateID)
	addFilter(&query, &args, "n.image_id", filters.ImageID)
	rows, err := s.db.QueryContext(ctx, query.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	now := time.Now().UTC()
	var out []store.NameEntry
	for rows.Next() {
		var name string
		var instanceID sql.NullString
		var imageID string
		var stateID sql.NullString
		var stateFingerprint string
		var lastUsedAt sql.NullString
		var expiresAt sql.NullString
		var joinedInstanceID sql.NullString
		if err := rows.Scan(&name, &instanceID, &imageID, &stateID, &stateFingerprint, &lastUsedAt, &expiresAt, &joinedInstanceID); err != nil {
			return nil, err
		}
		entry := store.NameEntry{
			Name:             name,
			ImageID:          imageID,
			StateFingerprint: stateFingerprint,
			Status:           store.NameStatusActive,
		}
		if instanceID.Valid {
			entry.InstanceID = strPtr(instanceID.String)
		}
		if lastUsedAt.Valid {
			entry.LastUsedAt = strPtr(lastUsedAt.String)
		}
		entry.StateID = stateFingerprint
		if stateID.Valid && stateID.String != "" {
			entry.StateID = stateID.String
		}
		if instanceID.Valid && joinedInstanceID.Valid {
			if isExpired(expiresAt, now) {
				entry.Status = store.NameStatusExpired
			}
		} else {
			entry.Status = store.NameStatusMissing
		}
		out = append(out, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) GetName(ctx context.Context, name string) (store.NameEntry, bool, error) {
	query := `
SELECT n.name, n.instance_id, n.image_id, n.state_id, n.state_fingerprint, n.last_used_at, i.expires_at, i.instance_id
FROM names n
LEFT JOIN instances i ON i.instance_id = n.instance_id
WHERE n.name = ?`
	row := s.db.QueryRowContext(ctx, query, name)
	now := time.Now().UTC()
	entry, err := scanName(row, now)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return store.NameEntry{}, false, nil
		}
		return store.NameEntry{}, false, err
	}
	return entry, true, nil
}

func (s *Store) ListInstances(ctx context.Context, filters store.InstanceFilters) ([]store.InstanceEntry, error) {
	query := strings.Builder{}
	query.WriteString(`
SELECT i.instance_id, i.image_id, i.state_id, i.created_at, i.expires_at, i.runtime_id, i.runtime_dir,
       pn.name,
       (SELECT COUNT(1) FROM names n WHERE n.instance_id = i.instance_id) as name_count
FROM instances i
LEFT JOIN names pn ON pn.instance_id = i.instance_id AND pn.is_primary = 1
WHERE 1=1`)
	args := []any{}
	addFilter(&query, &args, "i.state_id", filters.StateID)
	addFilter(&query, &args, "i.image_id", filters.ImageID)
	addPrefixFilter(&query, &args, "i.instance_id", filters.IDPrefix)
	rows, err := s.db.QueryContext(ctx, query.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	now := time.Now().UTC()
	var out []store.InstanceEntry
	for rows.Next() {
		entry, err := scanInstance(rows, now)
		if err != nil {
			return nil, err
		}
		out = append(out, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) GetInstance(ctx context.Context, instanceID string) (store.InstanceEntry, bool, error) {
	query := `
SELECT i.instance_id, i.image_id, i.state_id, i.created_at, i.expires_at, i.runtime_id, i.runtime_dir,
       pn.name,
       (SELECT COUNT(1) FROM names n WHERE n.instance_id = i.instance_id) as name_count
FROM instances i
LEFT JOIN names pn ON pn.instance_id = i.instance_id AND pn.is_primary = 1
WHERE i.instance_id = ?`
	row := s.db.QueryRowContext(ctx, query, instanceID)
	entry, err := scanInstance(row, time.Now().UTC())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return store.InstanceEntry{}, false, nil
		}
		return store.InstanceEntry{}, false, err
	}
	return entry, true, nil
}

func (s *Store) ListStates(ctx context.Context, filters store.StateFilters) ([]store.StateEntry, error) {
	query := strings.Builder{}
	query.WriteString(`
SELECT s.state_id, s.parent_state_id, s.image_id, s.prepare_kind, s.prepare_args_normalized, s.created_at, s.size_bytes,
       s.last_used_at, s.use_count, s.min_retention_until, coalesce(s.lineage_ref,''), coalesce(s.identity_digest,''),
       (SELECT COUNT(1) FROM instances i WHERE i.state_id = s.state_id) as refcount
FROM states s
WHERE 1=1`)
	args := []any{}
	addFilter(&query, &args, "s.prepare_kind", filters.Kind)
	addFilter(&query, &args, "s.image_id", filters.ImageID)
	addFilter(&query, &args, "s.parent_state_id", filters.ParentID)
	addPrefixFilter(&query, &args, "s.state_id", filters.IDPrefix)
	rows, err := s.db.QueryContext(ctx, query.String(), args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []store.StateEntry
	for rows.Next() {
		var entry store.StateEntry
		var parent sql.NullString
		var lastUsedAt sql.NullString
		var useCount sql.NullInt64
		var minRetentionUntil sql.NullString
		if err := rows.Scan(
			&entry.StateID,
			&parent,
			&entry.ImageID,
			&entry.PrepareKind,
			&entry.PrepareArgs,
			&entry.CreatedAt,
			&entry.SizeBytes,
			&lastUsedAt,
			&useCount,
			&minRetentionUntil,
			&entry.LineageRef,
			&entry.IdentityDigest,
			&entry.RefCount,
		); err != nil {
			return nil, err
		}
		if parent.Valid && strings.TrimSpace(parent.String) != "" {
			entry.ParentStateID = strPtr(parent.String)
		}
		if lastUsedAt.Valid && strings.TrimSpace(lastUsedAt.String) != "" {
			entry.LastUsedAt = strPtr(lastUsedAt.String)
		}
		if useCount.Valid {
			entry.UseCount = int64Ptr(useCount.Int64)
		}
		if minRetentionUntil.Valid && strings.TrimSpace(minRetentionUntil.String) != "" {
			entry.MinRetentionUntil = strPtr(minRetentionUntil.String)
		}
		out = append(out, entry)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return out, nil
}

func (s *Store) GetState(ctx context.Context, stateID string) (store.StateEntry, bool, error) {
	query := `
SELECT s.state_id, s.parent_state_id, s.image_id, s.prepare_kind, s.prepare_args_normalized, s.created_at, s.size_bytes,
       s.last_used_at, s.use_count, s.min_retention_until, coalesce(s.lineage_ref,''), coalesce(s.identity_digest,''),
       (SELECT COUNT(1) FROM instances i WHERE i.state_id = s.state_id) as refcount
FROM states s
WHERE s.state_id = ?`
	row := s.db.QueryRowContext(ctx, query, stateID)
	var entry store.StateEntry
	var parent sql.NullString
	var lastUsedAt sql.NullString
	var useCount sql.NullInt64
	var minRetentionUntil sql.NullString
	if err := row.Scan(
		&entry.StateID,
		&parent,
		&entry.ImageID,
		&entry.PrepareKind,
		&entry.PrepareArgs,
		&entry.CreatedAt,
		&entry.SizeBytes,
		&lastUsedAt,
		&useCount,
		&minRetentionUntil,
		&entry.LineageRef,
		&entry.IdentityDigest,
		&entry.RefCount,
	); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return store.StateEntry{}, false, nil
		}
		return store.StateEntry{}, false, err
	}
	if parent.Valid && strings.TrimSpace(parent.String) != "" {
		entry.ParentStateID = strPtr(parent.String)
	}
	if lastUsedAt.Valid && strings.TrimSpace(lastUsedAt.String) != "" {
		entry.LastUsedAt = strPtr(lastUsedAt.String)
	}
	if useCount.Valid {
		entry.UseCount = int64Ptr(useCount.Int64)
	}
	if minRetentionUntil.Valid && strings.TrimSpace(minRetentionUntil.String) != "" {
		entry.MinRetentionUntil = strPtr(minRetentionUntil.String)
	}
	return entry, true, nil
}

func (s *Store) CreateState(ctx context.Context, entry store.StateCreate) error {
	query := `
INSERT OR IGNORE INTO states (
	state_id, parent_state_id, state_fingerprint, image_id, prepare_kind, prepare_args_normalized, created_at,
	size_bytes, last_used_at, use_count, status, lineage_ref, identity_digest
)
VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`
	_, err := s.db.ExecContext(ctx, query,
		entry.StateID,
		entry.ParentStateID,
		entry.StateFingerprint,
		entry.ImageID,
		entry.PrepareKind,
		entry.PrepareArgsNormalized,
		entry.CreatedAt,
		entry.SizeBytes,
		entry.CreatedAt,
		0,
		entry.Status,
		entry.LineageRef,
		entry.IdentityDigest,
	)
	return err
}

func (s *Store) UpdateStateSize(ctx context.Context, stateID string, sizeBytes int64) error {
	_, err := s.db.ExecContext(ctx,
		`UPDATE states
		 SET size_bytes = ?
		 WHERE state_id = ? AND (size_bytes IS NULL OR size_bytes <= 0)`,
		sizeBytes,
		stateID,
	)
	return err
}

func (s *Store) CreateInstance(ctx context.Context, entry store.InstanceCreate) error {
	insertQuery := `
INSERT INTO instances (instance_id, state_id, image_id, created_at, expires_at, runtime_id, runtime_dir, status)
VALUES (?, ?, ?, ?, ?, ?, ?, ?)`
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, insertQuery,
		entry.InstanceID,
		entry.StateID,
		entry.ImageID,
		entry.CreatedAt,
		entry.ExpiresAt,
		entry.RuntimeID,
		entry.RuntimeDir,
		entry.Status,
	); err != nil {
		_ = tx.Rollback()
		return err
	}
	if _, err := tx.ExecContext(ctx,
		`UPDATE states
		 SET last_used_at = ?, use_count = COALESCE(use_count, 0) + 1
		 WHERE state_id = ?`,
		entry.CreatedAt,
		entry.StateID,
	); err != nil {
		_ = tx.Rollback()
		return err
	}
	return tx.Commit()
}

func (s *Store) DeleteInstance(ctx context.Context, instanceID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM instances WHERE instance_id = ?`, instanceID)
	return err
}

func (s *Store) DeleteState(ctx context.Context, stateID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM states WHERE state_id = ?`, stateID)
	return err
}

func (s *Store) UpdateInstanceRuntime(ctx context.Context, instanceID string, runtimeID *string) error {
	var value any
	if runtimeID != nil {
		trimmed := strings.TrimSpace(*runtimeID)
		if trimmed != "" {
			value = trimmed
		}
	}
	_, err := s.db.ExecContext(ctx, `UPDATE instances SET runtime_id = ? WHERE instance_id = ?`, value, instanceID)
	return err
}

func initDB(db *sql.DB) error {
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	if _, err := db.Exec("PRAGMA foreign_keys = ON"); err != nil {
		return err
	}
	if err := ensureParentStateColumn(db); err != nil {
		return err
	}
	if err := ensureRuntimeIDColumn(db); err != nil {
		return err
	}
	if err := ensureRuntimeDirColumn(db); err != nil {
		return err
	}
	if err := ensureStateLastUsedAtColumn(db); err != nil {
		return err
	}
	if err := ensureStateUseCountColumn(db); err != nil {
		return err
	}
	if err := ensureStateMinRetentionUntilColumn(db); err != nil {
		return err
	}
	if err := ensureStateEvictedAtColumn(db); err != nil {
		return err
	}
	if err := ensureStateEvictionReasonColumn(db); err != nil {
		return err
	}
	if _, err := db.Exec(SchemaSQL()); err != nil {
		return err
	}
	// Transitional constructor support; production cutover installs the guarded
	// schema in its preflight transaction before constructing the adapter.
	for _, column := range []string{"lineage_ref", "identity_digest"} {
		if _, err := db.Exec("ALTER TABLE states ADD COLUMN " + column + " TEXT"); err != nil && !strings.Contains(err.Error(), "duplicate column name") {
			return err
		}
	}
	return nil
}

func ensureParentStateColumn(db *sql.DB) error {
	if _, err := db.Exec("ALTER TABLE states ADD COLUMN parent_state_id TEXT"); err != nil {
		if strings.Contains(err.Error(), "duplicate column name") {
			// column already exists, continue to index creation
		} else if strings.Contains(err.Error(), "no such table") {
			// fresh database; schema creation will add the column and index
			return nil
		} else {
			return err
		}
	}
	_, err := db.Exec("CREATE INDEX IF NOT EXISTS idx_states_parent ON states(parent_state_id)")
	return err
}

func ensureRuntimeIDColumn(db *sql.DB) error {
	if _, err := db.Exec("ALTER TABLE instances ADD COLUMN runtime_id TEXT"); err != nil {
		if strings.Contains(err.Error(), "duplicate column name") {
			return nil
		} else if strings.Contains(err.Error(), "no such table") {
			return nil
		} else {
			return err
		}
	}
	return nil
}

func ensureRuntimeDirColumn(db *sql.DB) error {
	if _, err := db.Exec("ALTER TABLE instances ADD COLUMN runtime_dir TEXT"); err != nil {
		if strings.Contains(err.Error(), "duplicate column name") {
			return nil
		} else if strings.Contains(err.Error(), "no such table") {
			return nil
		} else {
			return err
		}
	}
	return nil
}

func ensureStateLastUsedAtColumn(db *sql.DB) error {
	if _, err := db.Exec("ALTER TABLE states ADD COLUMN last_used_at TEXT"); err != nil {
		if strings.Contains(err.Error(), "duplicate column name") {
			return nil
		} else if strings.Contains(err.Error(), "no such table") {
			return nil
		} else {
			return err
		}
	}
	return nil
}

func ensureStateUseCountColumn(db *sql.DB) error {
	if _, err := db.Exec("ALTER TABLE states ADD COLUMN use_count INTEGER"); err != nil {
		if strings.Contains(err.Error(), "duplicate column name") {
			return nil
		} else if strings.Contains(err.Error(), "no such table") {
			return nil
		} else {
			return err
		}
	}
	return nil
}

func ensureStateMinRetentionUntilColumn(db *sql.DB) error {
	if _, err := db.Exec("ALTER TABLE states ADD COLUMN min_retention_until TEXT"); err != nil {
		if strings.Contains(err.Error(), "duplicate column name") {
			return nil
		} else if strings.Contains(err.Error(), "no such table") {
			return nil
		} else {
			return err
		}
	}
	return nil
}

func ensureStateEvictedAtColumn(db *sql.DB) error {
	if _, err := db.Exec("ALTER TABLE states ADD COLUMN evicted_at TEXT"); err != nil {
		if strings.Contains(err.Error(), "duplicate column name") {
			return nil
		} else if strings.Contains(err.Error(), "no such table") {
			return nil
		} else {
			return err
		}
	}
	return nil
}

func ensureStateEvictionReasonColumn(db *sql.DB) error {
	if _, err := db.Exec("ALTER TABLE states ADD COLUMN eviction_reason TEXT"); err != nil {
		if strings.Contains(err.Error(), "duplicate column name") {
			return nil
		} else if strings.Contains(err.Error(), "no such table") {
			return nil
		} else {
			return err
		}
	}
	return nil
}

func addFilter(query *strings.Builder, args *[]any, column, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	query.WriteString(" AND ")
	query.WriteString(column)
	query.WriteString(" = ?")
	*args = append(*args, value)
}

func addPrefixFilter(query *strings.Builder, args *[]any, column, value string) {
	value = strings.TrimSpace(value)
	if value == "" {
		return
	}
	query.WriteString(" AND lower(")
	query.WriteString(column)
	query.WriteString(") LIKE ?")
	*args = append(*args, strings.ToLower(value)+"%")
}

func scanName(scanner interface {
	Scan(dest ...any) error
}, now time.Time) (store.NameEntry, error) {
	var name string
	var instanceID sql.NullString
	var imageID string
	var stateID sql.NullString
	var stateFingerprint string
	var lastUsedAt sql.NullString
	var expiresAt sql.NullString
	var joinedInstanceID sql.NullString
	if err := scanner.Scan(&name, &instanceID, &imageID, &stateID, &stateFingerprint, &lastUsedAt, &expiresAt, &joinedInstanceID); err != nil {
		return store.NameEntry{}, err
	}
	entry := store.NameEntry{
		Name:             name,
		ImageID:          imageID,
		StateFingerprint: stateFingerprint,
		Status:           store.NameStatusActive,
		StateID:          stateFingerprint,
	}
	if instanceID.Valid {
		entry.InstanceID = strPtr(instanceID.String)
	}
	if lastUsedAt.Valid {
		entry.LastUsedAt = strPtr(lastUsedAt.String)
	}
	if stateID.Valid && stateID.String != "" {
		entry.StateID = stateID.String
	}
	if instanceID.Valid && joinedInstanceID.Valid {
		if isExpired(expiresAt, now) {
			entry.Status = store.NameStatusExpired
		}
	} else {
		entry.Status = store.NameStatusMissing
	}
	return entry, nil
}

func scanInstance(scanner interface {
	Scan(dest ...any) error
}, now time.Time) (store.InstanceEntry, error) {
	var entry store.InstanceEntry
	var expiresAt sql.NullString
	var runtimeID sql.NullString
	var runtimeDir sql.NullString
	var name sql.NullString
	var nameCount int
	if err := scanner.Scan(&entry.InstanceID, &entry.ImageID, &entry.StateID, &entry.CreatedAt, &expiresAt, &runtimeID, &runtimeDir, &name, &nameCount); err != nil {
		return store.InstanceEntry{}, err
	}
	entry.Status = store.InstanceStatusActive
	if expiresAt.Valid {
		entry.ExpiresAt = strPtr(expiresAt.String)
		if isExpired(expiresAt, now) {
			entry.Status = store.InstanceStatusExpired
		}
	}
	if runtimeID.Valid {
		entry.RuntimeID = strPtr(runtimeID.String)
	}
	if runtimeDir.Valid {
		entry.RuntimeDir = strPtr(runtimeDir.String)
	}
	if name.Valid {
		entry.Name = strPtr(name.String)
	}
	if entry.Status != store.InstanceStatusExpired && nameCount == 0 {
		entry.Status = store.InstanceStatusOrphaned
	}
	return entry, nil
}

func isExpired(expiresAt sql.NullString, now time.Time) bool {
	if !expiresAt.Valid || expiresAt.String == "" {
		return false
	}
	parsed, ok := parseTime(expiresAt.String)
	if !ok {
		return false
	}
	return parsed.Before(now)
}

func parseTime(value string) (time.Time, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return time.Time{}, false
	}
	if ts, err := time.Parse(time.RFC3339, value); err == nil {
		return ts, true
	}
	if ts, err := time.Parse(time.RFC3339Nano, value); err == nil {
		return ts, true
	}
	return time.Time{}, false
}

func strPtr(value string) *string {
	return &value
}

func int64Ptr(value int64) *int64 {
	return &value
}
