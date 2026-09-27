package sqlite

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
	"github.com/sqlrs/engine-local/internal/runtimev2store"
)

// PutMaterialization atomically stores one immutable physical association and
// all of its components.
func (s *Store) PutMaterialization(ctx context.Context, value runtimev2store.Materialization) error {
	if value.RecordVersion() != runtimev2store.RecordVersion {
		return runtimev2store.ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var stateExists int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM runtime_v2_states WHERE state_id=? AND record_version=?`, value.StateID(), RuntimeV2RecordVersion).Scan(&stateExists); err != nil {
		return err
	}
	if stateExists != 1 {
		return runtimev2store.ErrInvalid
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO runtime_v2_materializations(materialization_id,record_version,state_id,backend,runtime_id,job_id,created_at,size_bytes,metadata_json) VALUES(?,?,?,?,?,?,?,?,?) ON CONFLICT(materialization_id) DO NOTHING`,
		value.MaterializationID(), RuntimeV2RecordVersion, value.StateID(), value.Backend(), nullableString(value.RuntimeID()), nullableString(value.JobID()), value.CreatedAt(), nullableInt64(value.SizeBytes()), value.Metadata())
	if err != nil {
		return err
	}
	inserted, _ := result.RowsAffected()
	if inserted == 0 {
		stored, found, loadErr := loadRuntimeV2Materialization(ctx, tx, value.MaterializationID())
		if loadErr != nil {
			return loadErr
		}
		if !found || !stored.Equal(value) {
			return runtimev2store.ErrCorrupt
		}
		return tx.Commit()
	}
	for _, component := range value.Components() {
		if _, err := tx.ExecContext(ctx, `INSERT INTO runtime_v2_materialization_components(materialization_id,record_version,name,kind,locator,size_bytes,metadata_json) VALUES(?,?,?,?,?,?,?)`, value.MaterializationID(), RuntimeV2RecordVersion, component.Name(), component.Kind(), component.Locator(), nullableInt64(component.SizeBytes()), component.Metadata()); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type runtimeV2MaterialQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func loadRuntimeV2Materialization(ctx context.Context, q runtimeV2MaterialQuerier, id string) (runtimev2store.Materialization, bool, error) {
	var version, stateID, backend, createdAt string
	var runtimeID, jobID sql.NullString
	var size sql.NullInt64
	var metadata []byte
	err := q.QueryRowContext(ctx, `SELECT record_version,state_id,backend,runtime_id,job_id,created_at,size_bytes,metadata_json FROM runtime_v2_materializations WHERE materialization_id=? AND (record_version=? OR record_version<>?)`, id, RuntimeV2RecordVersion, RuntimeV2RecordVersion).Scan(&version, &stateID, &backend, &runtimeID, &jobID, &createdAt, &size, &metadata)
	if errors.Is(err, sql.ErrNoRows) {
		return runtimev2store.Materialization{}, false, nil
	}
	if err != nil {
		return runtimev2store.Materialization{}, false, err
	}
	if version != RuntimeV2RecordVersion {
		return runtimev2store.Materialization{}, false, runtimev2store.ErrCorrupt
	}
	created, err := time.Parse("2006-01-02T15:04:05.000000000Z", createdAt)
	if err != nil {
		return runtimev2store.Materialization{}, false, runtimev2store.ErrCorrupt
	}
	rows, err := q.QueryContext(ctx, `SELECT record_version,name,kind,locator,size_bytes,metadata_json FROM runtime_v2_materialization_components WHERE materialization_id=? AND (record_version=? OR record_version<>?) ORDER BY name`, id, RuntimeV2RecordVersion, RuntimeV2RecordVersion)
	if err != nil {
		return runtimev2store.Materialization{}, false, err
	}
	defer rows.Close()
	components := []runtimev2store.MaterializationComponentInput{}
	for rows.Next() {
		var componentVersion, name, kind, locator string
		var componentSize sql.NullInt64
		var componentMetadata []byte
		if err := rows.Scan(&componentVersion, &name, &kind, &locator, &componentSize, &componentMetadata); err != nil {
			return runtimev2store.Materialization{}, false, err
		}
		if componentVersion != RuntimeV2RecordVersion {
			return runtimev2store.Materialization{}, false, runtimev2store.ErrCorrupt
		}
		components = append(components, runtimev2store.MaterializationComponentInput{Name: name, Kind: kind, Locator: locator, SizeBytes: nullInt64Pointer(componentSize), Metadata: json.RawMessage(componentMetadata)})
	}
	if err := rows.Err(); err != nil {
		return runtimev2store.Materialization{}, false, err
	}
	value, err := runtimev2store.NewMaterialization(runtimev2store.MaterializationInput{StateID: runtimev2.StateID(stateID), MaterializationID: id, Backend: backend, RuntimeID: nullStringPointer(runtimeID), JobID: nullStringPointer(jobID), CreatedAt: created, SizeBytes: nullInt64Pointer(size), Metadata: json.RawMessage(metadata), Components: components})
	if err != nil {
		return runtimev2store.Materialization{}, false, runtimev2store.ErrCorrupt
	}
	if !bytes.Equal(value.Metadata(), metadata) {
		return runtimev2store.Materialization{}, false, runtimev2store.ErrCorrupt
	}
	canonicalComponents := value.Components()
	for index := range canonicalComponents {
		if !bytes.Equal(canonicalComponents[index].Metadata(), components[index].Metadata) {
			return runtimev2store.Materialization{}, false, runtimev2store.ErrCorrupt
		}
	}
	return value, true, nil
}

func (s *Store) ListMaterializations(ctx context.Context, stateID runtimev2.StateID, request runtimev2store.MaterializationPageRequest) (runtimev2store.MaterializationPage, error) {
	if !runtimeV2ValidStateID(stateID) || !runtimev2store.ValidPageLimit(request.Limit) {
		return runtimev2store.MaterializationPage{}, runtimev2store.ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return runtimev2store.MaterializationPage{}, err
	}
	defer tx.Rollback()
	var exists int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM runtime_v2_states WHERE state_id=? AND record_version=?`, stateID, RuntimeV2RecordVersion).Scan(&exists); err != nil {
		return runtimev2store.MaterializationPage{}, err
	}
	if exists != 1 {
		return runtimev2store.MaterializationPage{}, runtimev2store.ErrInvalid
	}
	high, last := int64(0), int64(0)
	if request.Cursor == nil {
		err = tx.QueryRowContext(ctx, `SELECT coalesce((SELECT seq FROM sqlite_sequence WHERE name='runtime_v2_materializations'),0)`).Scan(&high)
	} else {
		high, last, err = request.Cursor.Continuation(stateID)
	}
	if err != nil {
		return runtimev2store.MaterializationPage{}, err
	}
	query := `SELECT insertion_seq,record_version,materialization_id FROM runtime_v2_materializations WHERE state_id=? AND (record_version=? OR record_version<>?) AND insertion_seq<=? ORDER BY insertion_seq DESC LIMIT ?`
	args := []any{stateID, RuntimeV2RecordVersion, RuntimeV2RecordVersion, high, request.Limit + 1}
	if request.Cursor != nil {
		query = `SELECT insertion_seq,record_version,materialization_id FROM runtime_v2_materializations WHERE state_id=? AND (record_version=? OR record_version<>?) AND insertion_seq<=? AND insertion_seq<? ORDER BY insertion_seq DESC LIMIT ?`
		args = []any{stateID, RuntimeV2RecordVersion, RuntimeV2RecordVersion, high, last, request.Limit + 1}
	}
	rows, err := tx.QueryContext(ctx, query, args...)
	if err != nil {
		return runtimev2store.MaterializationPage{}, err
	}
	type item struct {
		seq     int64
		version string
		id      string
	}
	items := []item{}
	for rows.Next() {
		var value item
		if err := rows.Scan(&value.seq, &value.version, &value.id); err != nil {
			rows.Close()
			return runtimev2store.MaterializationPage{}, err
		}
		items = append(items, value)
	}
	if err := rows.Close(); err != nil {
		return runtimev2store.MaterializationPage{}, err
	}
	values := make([]runtimev2store.Materialization, 0, request.Limit)
	bytesUsed := 0
	lastReturned := int64(0)
	more := false
	for _, item := range items {
		if item.version != RuntimeV2RecordVersion {
			return runtimev2store.MaterializationPage{}, runtimev2store.ErrCorrupt
		}
		if len(values) == request.Limit {
			more = true
			break
		}
		value, found, loadErr := loadRuntimeV2Materialization(ctx, tx, item.id)
		if loadErr != nil {
			return runtimev2store.MaterializationPage{}, loadErr
		}
		if !found {
			return runtimev2store.MaterializationPage{}, runtimev2store.ErrCorrupt
		}
		cost := runtimeV2MaterializationCost(value)
		if bytesUsed+cost > runtimev2store.MaxPageBytes {
			more = true
			break
		}
		values = append(values, value)
		bytesUsed += cost
		lastReturned = item.seq
	}
	if err := tx.Commit(); err != nil {
		return runtimev2store.MaterializationPage{}, err
	}
	var next *runtimev2store.MaterializationCursor
	if more {
		// Bounds and state identity were validated before the query.
		cursor, _ := runtimev2store.NewMaterializationCursor(stateID, high, lastReturned)
		next = &cursor
	}
	return runtimev2store.NewMaterializationPage(values, next), nil
}

func runtimeV2MaterializationCost(value runtimev2store.Materialization) int {
	cost := len(value.RecordVersion()) + len(value.StateID()) + len(value.MaterializationID()) + len(value.Backend()) + len(value.CreatedAt()) + len(value.Metadata())
	if v := value.RuntimeID(); v != nil {
		cost += len(*v)
	}
	if v := value.JobID(); v != nil {
		cost += len(*v)
	}
	if value.SizeBytes() != nil {
		cost += 8
	}
	for _, component := range value.Components() {
		cost += len(component.Name()) + len(component.Kind()) + len(component.Locator()) + len(component.Metadata())
		if component.SizeBytes() != nil {
			cost += 8
		}
	}
	return cost
}
func nullableString(value *string) any {
	if value == nil {
		return nil
	}
	return *value
}
func nullableInt64(value *int64) any {
	if value == nil {
		return nil
	}
	return *value
}
func nullStringPointer(value sql.NullString) *string {
	if !value.Valid {
		return nil
	}
	result := value.String
	return &result
}
func nullInt64Pointer(value sql.NullInt64) *int64 {
	if !value.Valid {
		return nil
	}
	result := value.Int64
	return &result
}
