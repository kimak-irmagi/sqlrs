package sqlite

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
	"github.com/sqlrs/engine-local/internal/runtimev2store"
)

func runtimeV2Recipe(t *testing.T, resolverVersion string, transforms int) runtimev2.RecipeLineage {
	t.Helper()
	factoryIdentity, err := runtimev2.NewFactoryIdentity(runtimev2.FactoryIdentityInput{
		SchemaVersion: runtimev2.SchemaVersion, Provider: "test", Kind: "database",
		IdentitySchema: "test.database.v1", Fields: []runtimev2.ResolvedField{{Name: "image", Value: "postgres"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	factory, err := runtimev2.NewFactoryProvenance(runtimev2.FactoryProvenanceInput{
		Identity: factoryIdentity, Resolver: &runtimev2.ResolverObservation{Implementation: "test", Version: resolverVersion},
	})
	if err != nil {
		t.Fatal(err)
	}
	values := make([]runtimev2.TransformProvenance, transforms)
	for i := range values {
		identity, identityErr := runtimev2.NewTransformIdentity(runtimev2.TransformIdentityInput{
			SchemaVersion: runtimev2.SchemaVersion, Provider: "test", Kind: "migration",
			IdentitySchema: "test.migration.v1", Fields: []runtimev2.ResolvedField{{Name: "step", Value: string(rune('a' + i))}},
		})
		if identityErr != nil {
			t.Fatal(identityErr)
		}
		values[i], err = runtimev2.NewTransformProvenance(runtimev2.TransformProvenanceInput{Identity: identity, Resolver: &runtimev2.ResolverObservation{Implementation: "test", Version: resolverVersion}})
		if err != nil {
			t.Fatal(err)
		}
	}
	recipe, err := runtimev2.NewRecipe(factory, values)
	if err != nil {
		t.Fatal(err)
	}
	lineage, err := runtimev2.Build(recipe)
	if err != nil {
		t.Fatal(err)
	}
	return lineage
}

func TestRuntimeV2RecipeLineageRestartRoundTripAndIdempotence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store.db")
	lineage := runtimeV2Recipe(t, "1", 2)
	store, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	store.now = func() time.Time { return time.Date(2026, 1, 2, 3, 4, 5, 6, time.UTC) }
	if err := store.PutRecipeLineage(context.Background(), lineage); err != nil {
		t.Fatal(err)
	}
	if err := store.PutRecipeLineage(context.Background(), lineage); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(); err != nil {
		t.Fatal(err)
	}
	store, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	record, found, err := store.GetLogicalState(context.Background(), lineage.Endpoint().ID())
	if err != nil || !found || record.State().ID() != lineage.Endpoint().ID() {
		t.Fatalf("endpoint = %+v %v %v", record, found, err)
	}
	trace, err := store.TraceLineage(context.Background(), lineage.Endpoint().ID())
	if err != nil || len(trace) != 3 {
		t.Fatalf("trace len=%d err=%v", len(trace), err)
	}
	if trace[0].State().ID() != lineage.Root().ID() || trace[2].State().ID() != lineage.Endpoint().ID() {
		t.Fatal("trace order is not root-to-endpoint")
	}
	var states, observations int
	if err := store.db.QueryRow(`SELECT count(*) FROM runtime_v2_states`).Scan(&states); err != nil {
		t.Fatal(err)
	}
	if err := store.db.QueryRow(`SELECT count(*) FROM runtime_v2_provenance_observations`).Scan(&observations); err != nil {
		t.Fatal(err)
	}
	if states != 3 || observations != 3 {
		t.Fatalf("counts states=%d observations=%d", states, observations)
	}
}

func TestRuntimeV2RelativeLineageAndMissingAnchor(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	base := runtimeV2Recipe(t, "1", 0)
	if err := store.PutRecipeLineage(context.Background(), base); err != nil {
		t.Fatal(err)
	}
	full := runtimeV2Recipe(t, "1", 1)
	relative, err := runtimev2.Extend(base.Root().ID(), []runtimev2.TransformProvenance{full.Steps()[0].Transform()})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PutRelativeLineage(context.Background(), relative); err != nil {
		t.Fatal(err)
	}
	trace, err := store.TraceLineage(context.Background(), relative.EndpointID())
	if err != nil || len(trace) != 2 {
		t.Fatalf("relative trace len=%d err=%v", len(trace), err)
	}
	missing, _ := runtimev2.Extend(runtimev2.StateID("sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"), []runtimev2.TransformProvenance{full.Steps()[0].Transform()})
	if err := store.PutRelativeLineage(context.Background(), missing); err == nil {
		t.Fatal("missing anchor accepted")
	}
}

func TestRuntimeV2ProvenanceKeepsDistinctDiagnosticsAndSnapshotPages(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "store.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	first := runtimeV2Recipe(t, "1", 0)
	second := runtimeV2Recipe(t, "2", 0)
	if first.Root().ID() != second.Root().ID() {
		t.Fatal("diagnostics changed state identity")
	}
	if err := store.PutRecipeLineage(context.Background(), first); err != nil {
		t.Fatal(err)
	}
	if err := store.PutRecipeLineage(context.Background(), second); err != nil {
		t.Fatal(err)
	}
	page, err := store.ListProvenance(context.Background(), first.Root().ID(), runtimev2store.ProvenancePageRequest{Limit: 1})
	if err != nil || len(page.Values()) != 1 || page.Next() == nil {
		t.Fatalf("first page = %+v %v", page.Values(), err)
	}
	third := runtimeV2Recipe(t, "3", 0)
	if err := store.PutRecipeLineage(context.Background(), third); err != nil {
		t.Fatal(err)
	}
	continuation, err := store.ListProvenance(context.Background(), first.Root().ID(), runtimev2store.ProvenancePageRequest{Limit: 1, Cursor: page.Next()})
	if err != nil || len(continuation.Values()) != 1 || continuation.Next() != nil {
		t.Fatalf("continuation = %+v %v", continuation.Values(), err)
	}
	fresh, err := store.ListProvenance(context.Background(), first.Root().ID(), runtimev2store.ProvenancePageRequest{Limit: 10})
	if err != nil || len(fresh.Values()) != 3 {
		t.Fatalf("fresh snapshot = %+v %v", fresh.Values(), err)
	}
}
