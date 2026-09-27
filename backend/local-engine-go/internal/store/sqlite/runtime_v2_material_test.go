package sqlite

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
	"github.com/sqlrs/engine-local/internal/runtimev2store"
)

func materializationForState(t *testing.T, stateID string, id string, created time.Time) runtimev2store.Materialization {
	t.Helper()
	size := int64(10)
	value, err := runtimev2store.NewMaterialization(runtimev2store.MaterializationInput{
		StateID: runtimeV2StateID(stateID), MaterializationID: id, Backend: "local.snapshot", CreatedAt: created,
		Metadata: json.RawMessage(`{"purpose":"test"}`), Components: []runtimev2store.MaterializationComponentInput{
			{Name: "data", Kind: "directory", Locator: "snapshot://" + id, SizeBytes: &size},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func runtimeV2StateID(value string) runtimev2.StateID { return runtimev2.StateID(value) }

func TestRuntimeV2MaterializationsPersistPageAndRestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.db")
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	lineage := runtimeV2Recipe(t, "1", 0)
	if err := store.PutRecipeLineage(context.Background(), lineage); err != nil {
		t.Fatal(err)
	}
	stateID := lineage.Root().ID()
	empty, err := store.ListMaterializations(context.Background(), stateID, runtimev2store.MaterializationPageRequest{Limit: 2})
	if err != nil || len(empty.Values()) != 0 {
		t.Fatalf("empty = %+v %v", empty.Values(), err)
	}
	for i, id := range []string{"one", "two", "three"} {
		if err := store.PutMaterialization(context.Background(), materializationForState(t, string(stateID), id, time.Date(2026, 1, 1, 0, 0, i, 0, time.UTC))); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	first, err := store.ListMaterializations(context.Background(), stateID, runtimev2store.MaterializationPageRequest{Limit: 2})
	if err != nil || len(first.Values()) != 2 || first.Next() == nil {
		t.Fatalf("first = %+v %v", first.Values(), err)
	}
	if first.Values()[0].MaterializationID() != "three" || first.Values()[1].MaterializationID() != "two" {
		t.Fatalf("order = %s %s", first.Values()[0].MaterializationID(), first.Values()[1].MaterializationID())
	}
	if err := store.PutMaterialization(context.Background(), materializationForState(t, string(stateID), "four", time.Date(2026, 1, 1, 0, 0, 4, 0, time.UTC))); err != nil {
		t.Fatal(err)
	}
	second, err := store.ListMaterializations(context.Background(), stateID, runtimev2store.MaterializationPageRequest{Limit: 2, Cursor: first.Next()})
	if err != nil || len(second.Values()) != 1 || second.Next() != nil || second.Values()[0].MaterializationID() != "one" {
		t.Fatalf("second = %+v %v", second.Values(), err)
	}
	if components := second.Values()[0].Components(); len(components) != 1 || components[0].Name() != "data" {
		t.Fatalf("components = %+v", components)
	}
	fresh, err := store.ListMaterializations(context.Background(), stateID, runtimev2store.MaterializationPageRequest{Limit: 10})
	if err != nil || len(fresh.Values()) != 4 || fresh.Values()[0].MaterializationID() != "four" {
		t.Fatalf("fresh snapshot = %+v %v", fresh.Values(), err)
	}
}

func TestRuntimeV2MaterializationIdempotenceConflictAndStateBoundary(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	lineage := runtimeV2Recipe(t, "1", 0)
	if err := store.PutRecipeLineage(context.Background(), lineage); err != nil {
		t.Fatal(err)
	}
	value := materializationForState(t, string(lineage.Root().ID()), "one", time.Now())
	if err := store.PutMaterialization(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	if err := store.PutMaterialization(context.Background(), value); err != nil {
		t.Fatal(err)
	}
	conflict := materializationForState(t, string(lineage.Root().ID()), "one", time.Now().Add(time.Hour))
	if err := store.PutMaterialization(context.Background(), conflict); err == nil {
		t.Fatal("conflicting duplicate accepted")
	}
	unknown := materializationForState(t, "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb", "unknown", time.Now())
	if err := store.PutMaterialization(context.Background(), unknown); err == nil {
		t.Fatal("unknown state accepted")
	}
}

func TestRuntimeV2StateClassificationKeepsLegacySeparate(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	lineage := runtimeV2Recipe(t, "1", 0)
	if err := store.PutRecipeLineage(context.Background(), lineage); err != nil {
		t.Fatal(err)
	}
	if _, err := store.db.Exec(`INSERT INTO states(state_id,image_id,prepare_kind,prepare_args_normalized,created_at) VALUES('legacy id','image','kind','{}','now')`); err != nil {
		t.Fatal(err)
	}
	cases := []struct {
		id         string
		legacy, v2 bool
	}{{"missing", false, false}, {"legacy id", true, false}, {string(lineage.Root().ID()), false, true}}
	for _, test := range cases {
		got, err := store.ClassifyState(context.Background(), test.id)
		if err != nil || got.LegacyPresent != test.legacy || got.RuntimeV2Present != test.v2 {
			t.Fatalf("classify %q = %+v %v", test.id, got, err)
		}
		if test.v2 && got.RuntimeV2RecordVersion != RuntimeV2RecordVersion {
			t.Fatalf("version=%q", got.RuntimeV2RecordVersion)
		}
	}
}
