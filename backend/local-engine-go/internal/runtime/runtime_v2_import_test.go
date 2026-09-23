package runtime_test

import (
	"testing"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
)

// TestRuntimeV2ImportBoundary proves that local-engine consumes the semantic
// core only through its public module API, without a reverse dependency.
func TestRuntimeV2ImportBoundary(t *testing.T) {
	identity, err := runtimev2.NewFactoryIdentity(runtimev2.FactoryIdentityInput{
		SchemaVersion: runtimev2.SchemaVersion,
		Provider:      "sqlrs",
		Kind:          "postgres",
		IdentitySchema: "factory.v1",
		Fields:         []runtimev2.ResolvedField{},
	})
	if err != nil {
		t.Fatal(err)
	}
	factory, err := runtimev2.NewFactoryProvenance(runtimev2.FactoryProvenanceInput{Identity: identity})
	if err != nil {
		t.Fatal(err)
	}
	recipe, err := runtimev2.NewRecipe(factory, nil)
	if err != nil {
		t.Fatal(err)
	}
	lineage, err := runtimev2.Build(recipe)
	if err != nil {
		t.Fatal(err)
	}
	if lineage.Endpoint().ID() == "" {
		t.Fatal("factory-only lineage has no endpoint")
	}
}
