package sqlite

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
	"github.com/sqlrs/engine-local/internal/runtimev2store"
)

const runtimeV2MaxSemanticBytes = 4 << 20

var _ runtimev2store.Store = (*Store)(nil)

func (s *Store) PutRecipeLineage(ctx context.Context, lineage runtimev2.RecipeLineage) error {
	root := lineage.Root()
	factory := lineage.Factory()
	if root.ID() == "" || factory.Identity().SchemaVersion() == "" {
		return runtimev2store.ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := s.timestamp()
	if err := putRuntimeV2State(ctx, tx, root, factory.Identity(), factory, now); err != nil {
		return err
	}
	for _, step := range lineage.Steps() {
		if err := putRuntimeV2State(ctx, tx, step.State(), step.Transform().Identity(), step.Transform(), now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) PutRelativeLineage(ctx context.Context, lineage runtimev2.RelativeLineage) error {
	if lineage.Anchor() == "" {
		return runtimev2store.ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, found, err := getRuntimeV2State(ctx, tx, lineage.Anchor())
	if err != nil {
		return err
	}
	if !found {
		return runtimev2store.ErrInvalid
	}
	now := s.timestamp()
	for _, step := range lineage.Steps() {
		if err := putRuntimeV2State(ctx, tx, step.State(), step.Transform().Identity(), step.Transform(), now); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) timestamp() string {
	now := time.Now
	if s.now != nil {
		now = s.now
	}
	return now().UTC().Format("2006-01-02T15:04:05.000000000Z")
}

func putRuntimeV2State(ctx context.Context, tx *sql.Tx, state runtimev2.State, identity any, provenance any, timestamp string) error {
	// The closed public semantic values were validated by their constructors;
	// their MarshalJSON methods cannot fail for a non-zero value.
	stateJSON, _ := json.Marshal(state)
	identityJSON, _ := json.Marshal(identity)
	provenanceJSON, _ := json.Marshal(provenance)
	if len(stateJSON) > runtimeV2MaxSemanticBytes || len(identityJSON) > runtimeV2MaxSemanticBytes || len(provenanceJSON) > runtimeV2MaxSemanticBytes {
		return runtimev2store.ErrInvalid
	}
	kind, parent := "factory", any(nil)
	if state.ParentID() != "" {
		kind, parent = "derived", state.ParentID()
	}
	result, err := tx.ExecContext(ctx, `INSERT INTO runtime_v2_states(state_id,record_version,state_kind,parent_state_id,state_json,resolved_identity_json,stored_at) VALUES(?,?,?,?,?,?,?) ON CONFLICT(state_id) DO NOTHING`, state.ID(), RuntimeV2RecordVersion, kind, parent, stateJSON, identityJSON, timestamp)
	if err != nil {
		return err
	}
	inserted, _ := result.RowsAffected()
	if inserted == 0 {
		var storedState, storedIdentity []byte
		var storedKind string
		var storedParent sql.NullString
		if err := tx.QueryRowContext(ctx, `SELECT state_kind,parent_state_id,state_json,resolved_identity_json FROM runtime_v2_states WHERE state_id=? AND record_version=?`, state.ID(), RuntimeV2RecordVersion).Scan(&storedKind, &storedParent, &storedState, &storedIdentity); err != nil {
			return runtimev2store.ErrCorrupt
		}
		parentMatches := (parent == nil && !storedParent.Valid) || (parent != nil && storedParent.Valid && storedParent.String == string(state.ParentID()))
		if storedKind != kind || !parentMatches || !bytes.Equal(storedState, stateJSON) || !bytes.Equal(storedIdentity, identityJSON) {
			return runtimev2store.ErrCorrupt
		}
	}
	digestBytes := sha256.Sum256(provenanceJSON)
	digest := "sha256:" + hex.EncodeToString(digestBytes[:])
	result, err = tx.ExecContext(ctx, `INSERT INTO runtime_v2_provenance_observations(state_id,observation_digest,record_version,provenance_json,observed_at) VALUES(?,?,?,?,?) ON CONFLICT(state_id,observation_digest) DO NOTHING`, state.ID(), digest, RuntimeV2RecordVersion, provenanceJSON, timestamp)
	if err != nil {
		return err
	}
	inserted, _ = result.RowsAffected()
	if inserted == 0 {
		var stored []byte
		if err := tx.QueryRowContext(ctx, `SELECT provenance_json FROM runtime_v2_provenance_observations WHERE state_id=? AND observation_digest=? AND record_version=?`, state.ID(), digest, RuntimeV2RecordVersion).Scan(&stored); err != nil || !bytes.Equal(stored, provenanceJSON) {
			return runtimev2store.ErrCorrupt
		}
	}
	return nil
}

func (s *Store) GetLogicalState(ctx context.Context, stateID runtimev2.StateID) (runtimev2store.StateRecord, bool, error) {
	if !runtimeV2ValidStateID(stateID) {
		return runtimev2store.StateRecord{}, false, runtimev2store.ErrInvalid
	}
	return getRuntimeV2State(ctx, s.db, stateID)
}

type runtimeV2RowQuerier interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func getRuntimeV2State(ctx context.Context, q runtimeV2RowQuerier, stateID runtimev2.StateID) (runtimev2store.StateRecord, bool, error) {
	var version, kind, storedAt string
	var parent sql.NullString
	var stateJSON, identityJSON []byte
	err := q.QueryRowContext(ctx, `SELECT record_version,state_kind,parent_state_id,state_json,resolved_identity_json,stored_at FROM runtime_v2_states WHERE state_id=? AND (record_version=? OR record_version<>?)`, stateID, RuntimeV2RecordVersion, RuntimeV2RecordVersion).Scan(&version, &kind, &parent, &stateJSON, &identityJSON, &storedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return runtimev2store.StateRecord{}, false, nil
	}
	if err != nil {
		return runtimev2store.StateRecord{}, false, err
	}
	if version != RuntimeV2RecordVersion || !validRuntimeV2Timestamp(storedAt) {
		return runtimev2store.StateRecord{}, false, runtimev2store.ErrCorrupt
	}
	var state runtimev2.State
	if json.Unmarshal(stateJSON, &state) != nil || state.ID() != stateID {
		return runtimev2store.StateRecord{}, false, runtimev2store.ErrCorrupt
	}
	canonicalState, _ := json.Marshal(state)
	if !bytes.Equal(canonicalState, stateJSON) {
		return runtimev2store.StateRecord{}, false, runtimev2store.ErrCorrupt
	}
	if kind == "factory" {
		if parent.Valid || state.ParentID() != "" {
			return runtimev2store.StateRecord{}, false, runtimev2store.ErrCorrupt
		}
		var identity runtimev2.ResolvedFactoryIdentity
		if json.Unmarshal(identityJSON, &identity) != nil {
			return runtimev2store.StateRecord{}, false, runtimev2store.ErrCorrupt
		}
		canonicalIdentity, _ := json.Marshal(identity)
		if !bytes.Equal(canonicalIdentity, identityJSON) {
			return runtimev2store.StateRecord{}, false, runtimev2store.ErrCorrupt
		}
		record, e := runtimev2store.NewFactoryStateRecord(state, identity)
		return record, e == nil, e
	}
	if kind == "derived" {
		if !parent.Valid || state.ParentID() != runtimev2.StateID(parent.String) {
			return runtimev2store.StateRecord{}, false, runtimev2store.ErrCorrupt
		}
		var identity runtimev2.ResolvedTransformIdentity
		if json.Unmarshal(identityJSON, &identity) != nil {
			return runtimev2store.StateRecord{}, false, runtimev2store.ErrCorrupt
		}
		canonicalIdentity, _ := json.Marshal(identity)
		if !bytes.Equal(canonicalIdentity, identityJSON) {
			return runtimev2store.StateRecord{}, false, runtimev2store.ErrCorrupt
		}
		record, e := runtimev2store.NewTransformStateRecord(state, identity)
		return record, e == nil, e
	}
	return runtimev2store.StateRecord{}, false, runtimev2store.ErrCorrupt
}

func (s *Store) TraceLineage(ctx context.Context, endpoint runtimev2.StateID) ([]runtimev2store.StateRecord, error) {
	seen := map[runtimev2.StateID]struct{}{}
	reverse := make([]runtimev2store.StateRecord, 0)
	current := endpoint
	bytesUsed := 0
	for len(reverse) <= runtimev2.MaxTransforms {
		if _, exists := seen[current]; exists {
			return nil, runtimev2store.ErrCorrupt
		}
		seen[current] = struct{}{}
		record, found, err := s.GetLogicalState(ctx, current)
		if err != nil {
			return nil, err
		}
		if !found {
			return nil, runtimev2store.ErrCorrupt
		}
		stateRaw, _ := json.Marshal(record.State())
		var identityRaw []byte
		if identity, ok := record.FactoryIdentity(); ok {
			identityRaw, _ = json.Marshal(identity)
		} else {
			identity, _ := record.TransformIdentity()
			identityRaw, _ = json.Marshal(identity)
		}
		bytesUsed += len(stateRaw) + len(identityRaw)
		if bytesUsed > runtimev2store.MaxPageBytes {
			return nil, runtimev2store.ErrCorrupt
		}
		reverse = append(reverse, record)
		if record.State().ParentID() == "" {
			break
		}
		current = record.State().ParentID()
	}
	if len(reverse) > runtimev2.MaxTransforms+1 || reverse[len(reverse)-1].State().ParentID() != "" {
		return nil, runtimev2store.ErrCorrupt
	}
	result := make([]runtimev2store.StateRecord, len(reverse))
	for i := range reverse {
		result[len(reverse)-1-i] = reverse[i]
	}
	return result, nil
}

func (s *Store) ListProvenance(ctx context.Context, stateID runtimev2.StateID, request runtimev2store.ProvenancePageRequest) (runtimev2store.ProvenancePage, error) {
	if !runtimeV2ValidStateID(stateID) || !runtimev2store.ValidPageLimit(request.Limit) {
		return runtimev2store.ProvenancePage{}, runtimev2store.ErrInvalid
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return runtimev2store.ProvenancePage{}, err
	}
	defer tx.Rollback()
	highWater, last := int64(0), int64(0)
	if request.Cursor == nil {
		err = tx.QueryRowContext(ctx, `SELECT coalesce((SELECT seq FROM sqlite_sequence WHERE name='runtime_v2_provenance_observations'),0)`).Scan(&highWater)
	} else {
		highWater, last, err = request.Cursor.Continuation(stateID)
	}
	if err != nil {
		return runtimev2store.ProvenancePage{}, err
	}
	stateRecord, found, err := getRuntimeV2State(ctx, tx, stateID)
	if err != nil || !found {
		if err == nil {
			err = runtimev2store.ErrInvalid
		}
		return runtimev2store.ProvenancePage{}, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT insertion_seq,record_version,observation_digest,provenance_json,observed_at FROM runtime_v2_provenance_observations WHERE state_id=? AND (record_version=? OR record_version<>?) AND insertion_seq>? AND insertion_seq<=? ORDER BY insertion_seq ASC LIMIT ?`, stateID, RuntimeV2RecordVersion, RuntimeV2RecordVersion, last, highWater, request.Limit+1)
	if err != nil {
		return runtimev2store.ProvenancePage{}, err
	}
	defer rows.Close()
	values := make([]runtimev2store.ProvenanceRecord, 0, request.Limit)
	bytesUsed := 0
	var lastReturned int64
	more := false
	for rows.Next() {
		var seq int64
		var version, digest, observed string
		var raw []byte
		if err := rows.Scan(&seq, &version, &digest, &raw, &observed); err != nil {
			return runtimev2store.ProvenancePage{}, err
		}
		if len(values) == request.Limit {
			more = true
			break
		}
		if version != RuntimeV2RecordVersion {
			return runtimev2store.ProvenancePage{}, runtimev2store.ErrCorrupt
		}
		cost := len(RuntimeV2RecordVersion) + len(digest) + len(observed) + len(raw)
		if bytesUsed+cost > runtimev2store.MaxPageBytes {
			more = true
			break
		}
		record, err := decodeRuntimeV2Provenance(digest, observed, raw, stateRecord)
		if err != nil {
			return runtimev2store.ProvenancePage{}, err
		}
		values = append(values, record)
		bytesUsed += cost
		lastReturned = seq
	}
	if err := rows.Err(); err != nil {
		return runtimev2store.ProvenancePage{}, err
	}
	if err := tx.Commit(); err != nil {
		return runtimev2store.ProvenancePage{}, err
	}
	var next *runtimev2store.ProvenanceCursor
	if more {
		// Bounds and state identity were validated before the query.
		cursor, _ := runtimev2store.NewProvenanceCursor(stateID, highWater, lastReturned)
		next = &cursor
	}
	return runtimev2store.NewProvenancePage(values, next), nil
}

func decodeRuntimeV2Provenance(digest, observed string, raw []byte, state runtimev2store.StateRecord) (runtimev2store.ProvenanceRecord, error) {
	sum := sha256.Sum256(raw)
	if digest != "sha256:"+hex.EncodeToString(sum[:]) || !validRuntimeV2Timestamp(observed) {
		return runtimev2store.ProvenanceRecord{}, runtimev2store.ErrCorrupt
	}
	when, _ := time.Parse("2006-01-02T15:04:05.000000000Z", observed)
	if identity, ok := state.FactoryIdentity(); ok {
		var value runtimev2.FactoryProvenance
		if json.Unmarshal(raw, &value) != nil {
			return runtimev2store.ProvenanceRecord{}, runtimev2store.ErrCorrupt
		}
		canonical, _ := json.Marshal(value)
		if !bytes.Equal(canonical, raw) {
			return runtimev2store.ProvenanceRecord{}, runtimev2store.ErrCorrupt
		}
		a, _ := json.Marshal(identity)
		b, _ := json.Marshal(value.Identity())
		if !bytes.Equal(a, b) {
			return runtimev2store.ProvenanceRecord{}, runtimev2store.ErrCorrupt
		}
		return runtimev2store.NewFactoryProvenanceRecord(digest, when, value)
	}
	identity, _ := state.TransformIdentity()
	var value runtimev2.TransformProvenance
	if json.Unmarshal(raw, &value) != nil {
		return runtimev2store.ProvenanceRecord{}, runtimev2store.ErrCorrupt
	}
	canonical, _ := json.Marshal(value)
	if !bytes.Equal(canonical, raw) {
		return runtimev2store.ProvenanceRecord{}, runtimev2store.ErrCorrupt
	}
	a, _ := json.Marshal(identity)
	b, _ := json.Marshal(value.Identity())
	if !bytes.Equal(a, b) {
		return runtimev2store.ProvenanceRecord{}, runtimev2store.ErrCorrupt
	}
	return runtimev2store.NewTransformProvenanceRecord(digest, when, value)
}

func runtimeV2ValidStateID(value runtimev2.StateID) bool {
	s := string(value)
	if len(s) != 71 || s[:7] != "sha256:" {
		return false
	}
	_, err := hex.DecodeString(s[7:])
	return err == nil && bytes.ToLower([]byte(s)) != nil && s == string(bytes.ToLower([]byte(s)))
}
func validRuntimeV2Timestamp(value string) bool {
	parsed, err := time.Parse("2006-01-02T15:04:05.000000000Z", value)
	return err == nil && parsed.Format("2006-01-02T15:04:05.000000000Z") == value
}
