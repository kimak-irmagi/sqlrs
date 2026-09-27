package runtimev2store

import (
	"errors"
	"strings"
	"testing"
	"time"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
)

func TestClosedDTOZeroValuesAreSafe(t *testing.T) {
	var state StateRecord
	if state.RecordVersion() != "" || state.State().ID() != "" {
		t.Fatal("zero state record exposed data")
	}
	if _, ok := state.FactoryIdentity(); ok {
		t.Fatal("zero state has factory identity")
	}
	if _, ok := state.TransformIdentity(); ok {
		t.Fatal("zero state has transform identity")
	}
	var provenance ProvenanceRecord
	if provenance.RecordVersion() != "" || provenance.ObservationDigest() != "" || provenance.ObservedAt() != "" {
		t.Fatal("zero provenance exposed data")
	}
	if _, ok := provenance.Factory(); ok {
		t.Fatal("zero provenance has factory")
	}
	if _, ok := provenance.Transform(); ok {
		t.Fatal("zero provenance has transform")
	}
	var materialization Materialization
	if materialization.RecordVersion() != "" || materialization.StateID() != "" || materialization.MaterializationID() != "" || materialization.Backend() != "" || materialization.RuntimeID() != nil || materialization.JobID() != nil || materialization.CreatedAt() != "" || materialization.SizeBytes() != nil || materialization.Metadata() != nil || materialization.Components() != nil {
		t.Fatal("zero materialization exposed data")
	}
	if !materialization.Equal(Materialization{}) || materialization.Equal(validMaterialization(t)) {
		t.Fatal("zero equality is inconsistent")
	}
}

func validMaterialization(t *testing.T) Materialization {
	t.Helper()
	value, err := NewMaterialization(validMaterializationInput())
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestStateAndProvenanceConstructorsRejectInvalidValues(t *testing.T) {
	factoryIdentity, err := runtimev2.NewFactoryIdentity(identityInput("factory"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewFactoryStateRecord(runtimev2.State{}, runtimev2.ResolvedFactoryIdentity{}); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("zero factory identity=%v", err)
	}
	if _, err := NewFactoryStateRecord(runtimev2.State{}, factoryIdentity); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("mismatched factory state=%v", err)
	}
	transformIdentity, err := runtimev2.NewTransformIdentity(identityInput("transform"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewTransformStateRecord(runtimev2.State{}, runtimev2.ResolvedTransformIdentity{}); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("zero transform identity=%v", err)
	}
	if _, err := NewTransformStateRecord(runtimev2.State{}, transformIdentity); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("mismatched transform state=%v", err)
	}
	for _, test := range []struct {
		name, digest string
		when         time.Time
		zero         bool
	}{{"digest", "bad", time.Now(), false}, {"time", "sha256:" + strings.Repeat("a", 64), time.Time{}, false}, {"value", "sha256:" + strings.Repeat("a", 64), time.Now(), true}} {
		t.Run(test.name, func(t *testing.T) {
			value := runtimev2.FactoryProvenance{}
			if !test.zero {
				value, _ = runtimev2.NewFactoryProvenance(runtimev2.FactoryProvenanceInput{Identity: factoryIdentity})
			}
			if _, err := NewFactoryProvenanceRecord(test.digest, test.when, value); !errors.Is(err, ErrInvalid) {
				t.Fatalf("error=%v", err)
			}
		})
	}
}

func TestTransformProvenanceRecordAndPageCopies(t *testing.T) {
	identity, err := runtimev2.NewTransformIdentity(identityInput("transform"))
	if err != nil {
		t.Fatal(err)
	}
	value, err := runtimev2.NewTransformProvenance(runtimev2.TransformProvenanceInput{Identity: identity})
	if err != nil {
		t.Fatal(err)
	}
	digest := "sha256:" + strings.Repeat("d", 64)
	record, err := NewTransformProvenanceRecord(digest, time.Date(2026, 1, 1, 0, 0, 0, 1, time.UTC), value)
	if err != nil {
		t.Fatal(err)
	}
	if record.RecordVersion() != RecordVersion || record.ObservationDigest() != digest || record.ObservedAt() != "2026-01-01T00:00:00.000000001Z" {
		t.Fatal("transform provenance accessors lost data")
	}
	if _, ok := record.Factory(); ok {
		t.Fatal("transform record exposed factory")
	}
	if _, ok := record.Transform(); !ok {
		t.Fatal("transform record missing value")
	}
	state := testStateID()
	pc, _ := NewProvenanceCursor(state, 4, 2)
	page := NewProvenancePage([]ProvenanceRecord{record}, &pc)
	values := page.Values()
	values[0] = ProvenanceRecord{}
	next := page.Next()
	*next = ProvenanceCursor{}
	if len(page.Values()) != 1 || page.Values()[0].ObservationDigest() != digest {
		t.Fatal("provenance page slice was mutable")
	}
	if _, _, err := page.Next().Continuation(state); err != nil {
		t.Fatal("provenance cursor was mutable")
	}
	mc, _ := NewMaterializationCursor(state, 4, 2)
	material := validMaterialization(t)
	materialPage := NewMaterializationPage([]Materialization{material}, &mc)
	materials := materialPage.Values()
	materials[0] = Materialization{}
	materialNext := materialPage.Next()
	*materialNext = MaterializationCursor{}
	if len(materialPage.Values()) != 1 || materialPage.Values()[0].MaterializationID() == "" {
		t.Fatal("materialization page slice was mutable")
	}
	if _, _, err := materialPage.Next().Continuation(state); err != nil {
		t.Fatal("materialization cursor was mutable")
	}
	if NewProvenancePage(nil, nil).Next() != nil || NewMaterializationPage(nil, nil).Next() != nil {
		t.Fatal("nil cursors became non-nil")
	}
}

func TestCursorAndMetadataAdditionalInvalidBranches(t *testing.T) {
	state := testStateID()
	for _, args := range [][2]int64{{-1, 0}, {1, -1}, {1, 2}} {
		if _, err := NewMaterializationCursor(state, args[0], args[1]); !errors.Is(err, ErrInvalid) {
			t.Fatalf("cursor %v accepted", args)
		}
	}
	if _, _, err := (MaterializationCursor{}).Continuation(state); !errors.Is(err, ErrInvalid) {
		t.Fatalf("zero cursor=%v", err)
	}
	if validStateID(runtimev2.StateID("sha256:" + strings.Repeat("A", 64))) {
		t.Fatal("uppercase state id accepted")
	}
	if _, err := canonicalMetadata([]byte(`{"a":`)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("truncated metadata=%v", err)
	}
	if _, err := canonicalMetadata([]byte(`{"a":[{"b":1,"b":2}]}`)); !errors.Is(err, ErrInvalid) {
		t.Fatalf("nested duplicate=%v", err)
	}
	expanding := []byte(`{"a":"` + strings.Repeat("<", 11000) + `"}`)
	if _, err := canonicalMetadata(expanding); !errors.Is(err, ErrInvalid) {
		t.Fatalf("expanded metadata=%v", err)
	}
}
