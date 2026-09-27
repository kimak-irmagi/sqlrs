package runtimev2store

import (
	"errors"
	"strings"
	"testing"
	"time"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
)

func identityInput(kind string) runtimev2.FactoryIdentityInput {
	return runtimev2.FactoryIdentityInput{SchemaVersion: runtimev2.SchemaVersion, Provider: "test", Kind: kind, IdentitySchema: "test." + kind + ".v1", Fields: []runtimev2.ResolvedField{}}
}

func TestStateRecordAcceptsOnlyMatchingIdentityKind(t *testing.T) {
	factoryIdentity, err := runtimev2.NewFactoryIdentity(identityInput("factory"))
	if err != nil {
		t.Fatal(err)
	}
	factoryProvenance, _ := runtimev2.NewFactoryProvenance(runtimev2.FactoryProvenanceInput{Identity: factoryIdentity})
	factoryState, _ := runtimev2.FactoryState(factoryProvenance)
	factoryRecord, err := NewFactoryStateRecord(factoryState, factoryIdentity)
	if err != nil {
		t.Fatal(err)
	}
	if factoryRecord.RecordVersion() != RecordVersion || factoryRecord.State().ID() != factoryState.ID() {
		t.Fatal("factory record lost data")
	}
	if _, ok := factoryRecord.FactoryIdentity(); !ok {
		t.Fatal("factory identity missing")
	}
	if _, ok := factoryRecord.TransformIdentity(); ok {
		t.Fatal("factory exposed transform identity")
	}

	transformIdentity, _ := runtimev2.NewTransformIdentity(identityInput("transform"))
	transformProvenance, _ := runtimev2.NewTransformProvenance(runtimev2.TransformProvenanceInput{Identity: transformIdentity})
	step, _ := runtimev2.Derive(factoryState.ID(), transformProvenance)
	transformRecord, err := NewTransformStateRecord(step.State(), transformIdentity)
	if err != nil {
		t.Fatal(err)
	}
	if _, ok := transformRecord.TransformIdentity(); !ok {
		t.Fatal("transform identity missing")
	}
	if _, ok := transformRecord.FactoryIdentity(); ok {
		t.Fatal("transform exposed factory identity")
	}

	other, _ := runtimev2.NewTransformIdentity(identityInput("other"))
	if _, err := NewTransformStateRecord(step.State(), other); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("mismatch = %v", err)
	}
	if _, err := NewFactoryStateRecord(step.State(), factoryIdentity); !errors.Is(err, ErrCorrupt) {
		t.Fatalf("wrong kind = %v", err)
	}
}

func TestOpaqueCursorsBindStateAndValidateBounds(t *testing.T) {
	state := testStateID()
	other := runtimev2.StateID("sha256:" + strings.Repeat("b", 64))
	provenance, err := NewProvenanceCursor(state, 20, 10)
	if err != nil {
		t.Fatal(err)
	}
	if high, last, err := provenance.Continuation(state); err != nil || high != 20 || last != 10 {
		t.Fatalf("continuation = %d %d %v", high, last, err)
	}
	if _, _, err := provenance.Continuation(other); !errors.Is(err, ErrInvalid) {
		t.Fatalf("cross-state cursor = %v", err)
	}
	materialization, err := NewMaterializationCursor(state, 20, 10)
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := materialization.Continuation(other); !errors.Is(err, ErrInvalid) {
		t.Fatalf("cross-state cursor = %v", err)
	}
	for _, limit := range []int{0, -1, 101} {
		if ValidPageLimit(limit) {
			t.Fatalf("accepted limit %d", limit)
		}
	}
	for _, limit := range []int{1, 50, 100} {
		if !ValidPageLimit(limit) {
			t.Fatalf("rejected limit %d", limit)
		}
	}
	if _, err := NewProvenanceCursor(state, 1, 2); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid watermark = %v", err)
	}
}

func TestProvenanceRecordNormalizesTimestamp(t *testing.T) {
	identity, _ := runtimev2.NewFactoryIdentity(identityInput("factory"))
	provenance, _ := runtimev2.NewFactoryProvenance(runtimev2.FactoryProvenanceInput{Identity: identity})
	record, err := NewFactoryProvenanceRecord("sha256:"+strings.Repeat("c", 64), time.Date(2026, 1, 2, 3, 4, 5, 6, time.FixedZone("offset", 3600)), provenance)
	if err != nil {
		t.Fatal(err)
	}
	if record.ObservedAt() != "2026-01-02T02:04:05.000000006Z" {
		t.Fatalf("observed_at = %s", record.ObservedAt())
	}
	if _, ok := record.Factory(); !ok {
		t.Fatal("factory provenance missing")
	}
	if _, ok := record.Transform(); ok {
		t.Fatal("factory record exposed transform provenance")
	}
}
