package runtimev2store

import (
	"context"
	"errors"
	"time"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
	"github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/resolver"
)

const (
	MaxPageRows  = 100
	MaxPageBytes = 32 << 20
)

var ErrCorrupt = errors.New("runtime v2 store record corrupt")

// Store is the closed Runtime v2 persistence boundary approved for issue #110.
type Store interface {
	PutRecipeLineage(context.Context, runtimev2.RecipeLineage) error
	PutRelativeLineage(context.Context, runtimev2.RelativeLineage) error
	GetLogicalState(context.Context, runtimev2.StateID) (StateRecord, bool, error)
	TraceLineage(context.Context, runtimev2.StateID) ([]StateRecord, error)
	ListProvenance(context.Context, runtimev2.StateID, ProvenancePageRequest) (ProvenancePage, error)
	PutMaterialization(context.Context, Materialization) error
	ListMaterializations(context.Context, runtimev2.StateID, MaterializationPageRequest) (MaterializationPage, error)
	ClassifyState(context.Context, string) (StateClassification, error)
	resolver.Cache
}

type stateRecordData struct {
	state             runtimev2.State
	factoryIdentity   runtimev2.ResolvedFactoryIdentity
	transformIdentity runtimev2.ResolvedTransformIdentity
	factory           bool
}

// StateRecord is a validated state plus exactly one matching resolved identity.
type StateRecord struct{ data *stateRecordData }

func NewFactoryStateRecord(state runtimev2.State, identity runtimev2.ResolvedFactoryIdentity) (StateRecord, error) {
	provenance, err := runtimev2.NewFactoryProvenance(runtimev2.FactoryProvenanceInput{Identity: identity})
	if err != nil {
		return StateRecord{}, ErrCorrupt
	}
	expected, err := runtimev2.FactoryState(provenance)
	if err != nil || expected.ID() != state.ID() {
		return StateRecord{}, ErrCorrupt
	}
	if state.ParentID() != "" {
		return StateRecord{}, ErrCorrupt
	}
	return StateRecord{data: &stateRecordData{state: state, factoryIdentity: identity, factory: true}}, nil
}

func NewTransformStateRecord(state runtimev2.State, identity runtimev2.ResolvedTransformIdentity) (StateRecord, error) {
	provenance, err := runtimev2.NewTransformProvenance(runtimev2.TransformProvenanceInput{Identity: identity})
	if err != nil {
		return StateRecord{}, ErrCorrupt
	}
	expected, err := runtimev2.Derive(state.ParentID(), provenance)
	if err != nil || expected.State().ID() != state.ID() {
		return StateRecord{}, ErrCorrupt
	}
	return StateRecord{data: &stateRecordData{state: state, transformIdentity: identity}}, nil
}

func (r StateRecord) RecordVersion() string {
	if r.data == nil {
		return ""
	}
	return RecordVersion
}
func (r StateRecord) State() runtimev2.State {
	if r.data == nil {
		return runtimev2.State{}
	}
	return r.data.state
}
func (r StateRecord) FactoryIdentity() (runtimev2.ResolvedFactoryIdentity, bool) {
	if r.data == nil || !r.data.factory {
		return runtimev2.ResolvedFactoryIdentity{}, false
	}
	return r.data.factoryIdentity, true
}
func (r StateRecord) TransformIdentity() (runtimev2.ResolvedTransformIdentity, bool) {
	if r.data == nil || r.data.factory {
		return runtimev2.ResolvedTransformIdentity{}, false
	}
	return r.data.transformIdentity, true
}

type provenanceRecordData struct {
	digest, observedAt string
	factory            *runtimev2.FactoryProvenance
	transform          *runtimev2.TransformProvenance
}
type ProvenanceRecord struct{ data *provenanceRecordData }

func NewFactoryProvenanceRecord(digest string, observedAt time.Time, value runtimev2.FactoryProvenance) (ProvenanceRecord, error) {
	if !validStateID(runtimev2.StateID(digest)) || observedAt.IsZero() || value.Identity().SchemaVersion() == "" {
		return ProvenanceRecord{}, ErrInvalid
	}
	copy := value
	return ProvenanceRecord{data: &provenanceRecordData{digest: digest, observedAt: formatTimestamp(observedAt), factory: &copy}}, nil
}
func NewTransformProvenanceRecord(digest string, observedAt time.Time, value runtimev2.TransformProvenance) (ProvenanceRecord, error) {
	if !validStateID(runtimev2.StateID(digest)) || observedAt.IsZero() || value.Identity().SchemaVersion() == "" {
		return ProvenanceRecord{}, ErrInvalid
	}
	copy := value
	return ProvenanceRecord{data: &provenanceRecordData{digest: digest, observedAt: formatTimestamp(observedAt), transform: &copy}}, nil
}
func (r ProvenanceRecord) RecordVersion() string {
	if r.data == nil {
		return ""
	}
	return RecordVersion
}
func (r ProvenanceRecord) ObservationDigest() string {
	if r.data == nil {
		return ""
	}
	return r.data.digest
}
func (r ProvenanceRecord) ObservedAt() string {
	if r.data == nil {
		return ""
	}
	return r.data.observedAt
}
func (r ProvenanceRecord) Factory() (runtimev2.FactoryProvenance, bool) {
	if r.data == nil || r.data.factory == nil {
		return runtimev2.FactoryProvenance{}, false
	}
	return *r.data.factory, true
}
func (r ProvenanceRecord) Transform() (runtimev2.TransformProvenance, bool) {
	if r.data == nil || r.data.transform == nil {
		return runtimev2.TransformProvenance{}, false
	}
	return *r.data.transform, true
}

type ProvenanceCursor struct {
	version         uint8
	stateID         runtimev2.StateID
	highWater, last int64
}
type MaterializationCursor struct {
	version         uint8
	stateID         runtimev2.StateID
	highWater, last int64
}

type ProvenancePageRequest struct {
	Limit  int
	Cursor *ProvenanceCursor
}
type MaterializationPageRequest struct {
	Limit  int
	Cursor *MaterializationCursor
}

type ProvenancePage struct {
	values []ProvenanceRecord
	next   *ProvenanceCursor
}
type MaterializationPage struct {
	values []Materialization
	next   *MaterializationCursor
}

func NewProvenanceCursor(stateID runtimev2.StateID, highWater, last int64) (ProvenanceCursor, error) {
	if !validStateID(stateID) || highWater < 0 || last < 0 || last > highWater {
		return ProvenanceCursor{}, ErrInvalid
	}
	return ProvenanceCursor{1, stateID, highWater, last}, nil
}
func NewMaterializationCursor(stateID runtimev2.StateID, highWater, last int64) (MaterializationCursor, error) {
	if !validStateID(stateID) || highWater < 0 || last < 0 || last > highWater {
		return MaterializationCursor{}, ErrInvalid
	}
	return MaterializationCursor{1, stateID, highWater, last}, nil
}
func (c ProvenanceCursor) Continuation(stateID runtimev2.StateID) (int64, int64, error) {
	if c.version != 1 || c.stateID != stateID {
		return 0, 0, ErrInvalid
	}
	return c.highWater, c.last, nil
}
func (c MaterializationCursor) Continuation(stateID runtimev2.StateID) (int64, int64, error) {
	if c.version != 1 || c.stateID != stateID {
		return 0, 0, ErrInvalid
	}
	return c.highWater, c.last, nil
}
func ValidPageLimit(limit int) bool { return limit >= 1 && limit <= MaxPageRows }

func NewProvenancePage(values []ProvenanceRecord, next *ProvenanceCursor) ProvenancePage {
	return ProvenancePage{append([]ProvenanceRecord(nil), values...), copyProvenanceCursor(next)}
}
func NewMaterializationPage(values []Materialization, next *MaterializationCursor) MaterializationPage {
	return MaterializationPage{append([]Materialization(nil), values...), copyMaterializationCursor(next)}
}
func (p ProvenancePage) Values() []ProvenanceRecord {
	return append([]ProvenanceRecord(nil), p.values...)
}
func (p ProvenancePage) Next() *ProvenanceCursor { return copyProvenanceCursor(p.next) }
func (p MaterializationPage) Values() []Materialization {
	return append([]Materialization(nil), p.values...)
}
func (p MaterializationPage) Next() *MaterializationCursor { return copyMaterializationCursor(p.next) }
func copyProvenanceCursor(value *ProvenanceCursor) *ProvenanceCursor {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}
func copyMaterializationCursor(value *MaterializationCursor) *MaterializationCursor {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

type StateClassification struct {
	LegacyPresent          bool
	RuntimeV2Present       bool
	RuntimeV2RecordVersion string
}

func formatTimestamp(value time.Time) string {
	return value.UTC().Format("2006-01-02T15:04:05.000000000Z")
}
