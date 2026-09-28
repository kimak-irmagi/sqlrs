package composition_test

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"testing"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
	"github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/composition"
)

// X01: an external consumer can use the complete documented public API.
func TestExternalConsumerSurface(t *testing.T) {
	factory := runtimev2.FactoryDeclaration{Kind: "postgres", Reference: "postgres:17", Arguments: []string{}, Attributes: map[string]string{}}
	transform := runtimev2.TransformDeclaration{Kind: "psql", Reference: "seed.sql", Arguments: []string{}, Attributes: map[string]string{}}
	document, err := composition.NewAliasDocument(composition.DocumentInput{Aliases: []composition.NamedAliasInput{
		{Name: "seed", Transform: &transform},
		{Name: "database", Recipe: &composition.RecipeAliasInput{
			Base:  composition.RecipeBaseInput{Factory: &factory},
			Steps: []composition.RecipeStepInput{{Use: "seed"}},
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	var decoded composition.AliasDocument
	if err := composition.DecodeAliasDocumentJSON(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	catalog, err := composition.NewCatalog([]composition.SourceDocument{{SourceID: "aliases.json", Document: decoded}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := catalog.ExpandRecipe("database")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Declaration().Transforms()) != 1 || len(result.Trace().Nodes()) != 2 {
		t.Fatalf("unexpected expansion: %s / %s", mustMarshal(t, result.Declaration()), mustMarshal(t, result.Trace()))
	}
	traceRaw := mustMarshal(t, result.Trace())
	var trace composition.ExpansionTrace
	if err := composition.DecodeExpansionTraceJSON(traceRaw, &trace); err != nil {
		t.Fatal(err)
	}
	if trace.TargetNodeID() != 0 || trace.Nodes()[0].Alias() != "database" {
		t.Fatalf("unexpected trace accessors: %s", traceRaw)
	}
}

// X02: diagnostic renames are identity-neutral; child and order changes are identity-bearing.
func TestExpandedDeclarationsDriveDownstreamIdentity(t *testing.T) {
	base := expandedRecipe(t, "recipe-a", "first-a", "second-a", "one.sql", "two.sql", false)
	renamed := expandedRecipe(t, "recipe-b", "first-b", "second-b", "one.sql", "two.sql", false)
	childChanged := expandedRecipe(t, "recipe-a", "first-a", "second-a", "changed.sql", "two.sql", false)
	reordered := expandedRecipe(t, "recipe-a", "first-a", "second-a", "one.sql", "two.sql", true)

	baseID := resolveDeclaration(t, base.Declaration())
	if got := resolveDeclaration(t, renamed.Declaration()); got != baseID {
		t.Fatalf("rename changed StateID: %s != %s", got, baseID)
	}
	if got := resolveDeclaration(t, childChanged.Declaration()); got == baseID {
		t.Fatal("identity-bearing child change did not change StateID")
	}
	if got := resolveDeclaration(t, reordered.Declaration()); got == baseID {
		t.Fatal("step reordering did not change StateID")
	}
}

func expandedRecipe(t *testing.T, recipeName, firstName, secondName, firstRef, secondRef string, reverse bool) composition.ExpandedRecipe {
	t.Helper()
	factory := runtimev2.FactoryDeclaration{Kind: "postgres", Reference: "postgres:17", Arguments: []string{}, Attributes: map[string]string{}}
	first := runtimev2.TransformDeclaration{Kind: "psql", Reference: firstRef, Arguments: []string{}, Attributes: map[string]string{}}
	second := runtimev2.TransformDeclaration{Kind: "psql", Reference: secondRef, Arguments: []string{}, Attributes: map[string]string{}}
	steps := []composition.RecipeStepInput{{Use: firstName}, {Use: secondName}}
	if reverse {
		steps[0], steps[1] = steps[1], steps[0]
	}
	document, err := composition.NewAliasDocument(composition.DocumentInput{Aliases: []composition.NamedAliasInput{
		{Name: firstName, Transform: &first},
		{Name: secondName, Transform: &second},
		{Name: recipeName, Recipe: &composition.RecipeAliasInput{Base: composition.RecipeBaseInput{Factory: &factory}, Steps: steps}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	catalog, err := composition.NewCatalog([]composition.SourceDocument{{SourceID: "diagnostic.yaml", Document: document}})
	if err != nil {
		t.Fatal(err)
	}
	value, err := catalog.ExpandRecipe(recipeName)
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func resolveDeclaration(t *testing.T, declaration runtimev2.RecipeDeclaration) runtimev2.StateID {
	t.Helper()
	factoryDeclaration := declaration.Factory()
	factoryIdentity, err := runtimev2.NewFactoryIdentity(runtimev2.FactoryIdentityInput{
		SchemaVersion: runtimev2.SchemaVersion, Provider: "test", Kind: factoryDeclaration.Kind,
		IdentitySchema: "test.factory.v1", Fields: []runtimev2.ResolvedField{{Name: "reference.digest", Value: digest(factoryDeclaration.Reference)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	factory, err := runtimev2.NewFactoryProvenance(runtimev2.FactoryProvenanceInput{Identity: factoryIdentity, Declaration: &factoryDeclaration})
	if err != nil {
		t.Fatal(err)
	}
	declarations := declaration.Transforms()
	transforms := make([]runtimev2.TransformProvenance, len(declarations))
	for index := range declarations {
		identity, identityErr := runtimev2.NewTransformIdentity(runtimev2.TransformIdentityInput{
			SchemaVersion: runtimev2.SchemaVersion, Provider: "test", Kind: declarations[index].Kind,
			IdentitySchema: "test.transform.v1", Fields: []runtimev2.ResolvedField{{Name: "reference.digest", Value: digest(declarations[index].Reference)}},
		})
		if identityErr != nil {
			t.Fatal(identityErr)
		}
		transforms[index], err = runtimev2.NewTransformProvenance(runtimev2.TransformProvenanceInput{Identity: identity, Declaration: &declarations[index]})
		if err != nil {
			t.Fatal(err)
		}
	}
	recipe, err := runtimev2.NewRecipe(factory, transforms)
	if err != nil {
		t.Fatal(err)
	}
	lineage, err := runtimev2.Build(recipe)
	if err != nil {
		t.Fatal(err)
	}
	return lineage.Endpoint().ID()
}

func digest(value string) string {
	sum := sha256.Sum256([]byte(value))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func mustMarshal(t testing.TB, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return raw
}
