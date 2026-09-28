package consumertest

import (
	"bytes"
	"encoding/json"
	"errors"
	"testing"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
	"github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/composition"
	"github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/resolver"
)

// X09: a clean external consumer exercises every supported recipe composition
// shape through exported APIs only. Requirements:
// docs/architecture/runtime-v2-composition-tests.md.
func TestCompositionReleaseConsumerSuccessMatrix(t *testing.T) {
	seed := consumerTransform("seed.sql")
	migrate := consumerTransform("migrate.sql")
	factoryOnly := consumerRecipe("postgres:16")
	base := consumerRecipe("postgres:17", composition.RecipeStepInput{Use: "seed"})
	middle := composition.RecipeAliasInput{
		Base:  composition.RecipeBaseInput{Recipe: "base"},
		Steps: []composition.RecipeStepInput{{Transform: transformPointer(consumerTransform("middle.sql"))}},
	}
	target := composition.RecipeAliasInput{
		Base: composition.RecipeBaseInput{Recipe: "middle"},
		Steps: []composition.RecipeStepInput{
			{Use: "migrate"},
			{Use: "seed"},
		},
	}
	document := mustConsumerDocument(t,
		composition.NamedAliasInput{Name: "seed", Transform: &seed},
		composition.NamedAliasInput{Name: "migrate", Transform: &migrate},
		composition.NamedAliasInput{Name: "factory-only", Recipe: &factoryOnly},
		composition.NamedAliasInput{Name: "base", Recipe: &base},
		composition.NamedAliasInput{Name: "middle", Recipe: &middle},
		composition.NamedAliasInput{Name: "target", Recipe: &target},
	)
	catalog := mustConsumerCatalog(t, composition.SourceDocument{SourceID: "aliases.json", Document: document})

	assertConsumerRecipe(t, mustExpandConsumerRecipe(t, catalog, "factory-only"), "postgres:16")
	assertConsumerRecipe(t, mustExpandConsumerRecipe(t, catalog, "base"), "postgres:17", "seed.sql")
	expanded := mustExpandConsumerRecipe(t, catalog, "target")
	assertConsumerRecipe(t, expanded, "postgres:17", "seed.sql", "middle.sql", "migrate.sql", "seed.sql")
	if composition.AliasSchemaVersion != "sqlrs.runtime.v2.aliases.v1" ||
		composition.ExpansionTraceSchemaVersion != "sqlrs.runtime.v2.alias-expansion-trace.v1" {
		t.Fatalf("unexpected composition schemas: %q, %q", composition.AliasSchemaVersion, composition.ExpansionTraceSchemaVersion)
	}
	traceJSON, err := json.Marshal(expanded.Trace())
	if err != nil {
		t.Fatal(err)
	}
	var trace composition.ExpansionTrace
	if err := composition.DecodeExpansionTraceJSON(traceJSON, &trace); err != nil {
		t.Fatal(err)
	}
	if nodes := trace.Nodes(); len(nodes) == 0 || nodes[0].Alias() != "target" {
		t.Fatalf("unexpected expansion trace: %s", traceJSON)
	}
}

// X10: alias/source names remain diagnostic while child declarations and step
// order remain semantic; stable error classes cross the module boundary.
func TestCompositionReleaseConsumerIdentityAndErrors(t *testing.T) {
	first := mustExpandedConsumerJSON(t, "recipe-a", "first-a", "second-a", false)
	renamed := mustExpandedConsumerJSON(t, "recipe-b", "first-b", "second-b", false)
	reordered := mustExpandedConsumerJSON(t, "recipe-a", "first-a", "second-a", true)
	if !bytes.Equal(first, renamed) {
		t.Fatalf("diagnostic rename changed expanded declaration:\n%s\n%s", first, renamed)
	}
	if bytes.Equal(first, reordered) {
		t.Fatal("step reordering did not change expanded declaration")
	}

	missing := consumerRecipe("postgres:17", composition.RecipeStepInput{Use: "absent"})
	missingCatalog := mustConsumerCatalog(t, composition.SourceDocument{
		SourceID: "missing.json",
		Document: mustConsumerDocument(t, composition.NamedAliasInput{Name: "target", Recipe: &missing}),
	})
	_, err := missingCatalog.ExpandRecipe("target")
	assertConsumerCompositionError(t, err, composition.CodeMissingReference)

	a := composition.RecipeAliasInput{Base: composition.RecipeBaseInput{Recipe: "b"}}
	b := composition.RecipeAliasInput{Base: composition.RecipeBaseInput{Recipe: "a"}}
	cycleCatalog := mustConsumerCatalog(t, composition.SourceDocument{
		SourceID: "cycle.json",
		Document: mustConsumerDocument(t,
			composition.NamedAliasInput{Name: "a", Recipe: &a},
			composition.NamedAliasInput{Name: "b", Recipe: &b},
		),
	})
	_, err = cycleCatalog.ExpandRecipe("a")
	problem := assertConsumerCompositionError(t, err, composition.CodeCycle)
	if cycle := problem.Cycle(); len(cycle) != 3 || cycle[0].Alias != "a" || cycle[2].Alias != "a" {
		t.Fatalf("unexpected cycle: %#v", cycle)
	}

	duplicateA := consumerTransform("a.sql")
	duplicateB := consumerTransform("b.sql")
	_, err = composition.NewCatalog([]composition.SourceDocument{
		{SourceID: "z.json", Document: mustConsumerDocument(t, composition.NamedAliasInput{Name: "same", Transform: &duplicateA})},
		{SourceID: "a.json", Document: mustConsumerDocument(t, composition.NamedAliasInput{Name: "same", Transform: &duplicateB})},
	})
	problem = assertConsumerCompositionError(t, err, composition.CodeAmbiguousReference)
	if candidates := problem.Candidates(); len(candidates) != 2 || candidates[0].SourceID != "a.json" || candidates[1].SourceID != "z.json" {
		t.Fatalf("unexpected ambiguity candidates: %#v", candidates)
	}
}

// X11: a clean consumer can construct and round-trip CacheRecord while the
// published v0.2.0 wire representation remains byte-for-byte stable.
func TestCacheRecordReleaseConsumer(t *testing.T) {
	declaration, err := runtimev2.NewInputDeclaration(runtimev2.ExtensionSpecificationInput{
		SchemaVersion:       runtimev2.SchemaVersion,
		Owner:               "owner",
		Kind:                "kind",
		SpecificationSchema: "owner.kind.v1",
		Fields:              []runtimev2.DeclarationField{},
	})
	if err != nil {
		t.Fatal(err)
	}
	descriptor := resolver.Descriptor{
		Role: "input", Owner: "owner", Kind: "kind",
		SpecificationSchema: "owner.kind.v1", SemanticVersion: "1",
	}
	key, err := resolver.NewCacheKey(
		resolver.Workspace{Root: t.TempDir()},
		descriptor,
		resolver.NormalizedDeclaration{Declaration: declaration},
	)
	if err != nil {
		t.Fatal(err)
	}
	identity, err := runtimev2.NewResolvedExtensionIdentity(runtimev2.ResolvedExtensionIdentityInput{
		SchemaVersion: runtimev2.SchemaVersion, Owner: "owner", Kind: "kind",
		IdentitySchema: "owner.kind.v1", Fields: []runtimev2.ResolvedField{},
	})
	if err != nil {
		t.Fatal(err)
	}
	record, err := resolver.NewCacheRecord(key, resolver.Resolution{Identity: identity, Evidence: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := resolver.DecodeCacheRecordJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	if !decoded.Matches(key) || decoded.Resolution().Identity.IdentitySchema() != "owner.kind.v1" {
		t.Fatal("public CacheRecord round trip lost key or resolution")
	}

	legacy := []byte(`{"schema_version":"sqlrs.resolution-cache.v1","key":"54f09db3aa36879ab19fa6b7e8aa3844929ff88859a1e104e10b4cbad64c0fdb","workspace_scope":"9400f1b21cb527d7fa3d3eabba93557d81f3918c6eabd22fc6bd8ef8bedc2722","resolver":{"role":"input","owner":"owner","kind":"kind","specification_schema":"owner.kind.v1","semantic_version":"1"},"normalized_declaration":{"schema_version":"sqlrs.runtime.v2","owner":"owner","kind":"kind","specification_schema":"owner.kind.v1","fields":[]},"resolution":{"Identity":{"schema_version":"sqlrs.runtime.v2","owner":"owner","kind":"kind","identity_schema":"owner.kind.v1","fields":[]},"Evidence":{}},"checksum":"sha256:133036d75465b288ddde5655d40bab0ffce6c1825488815758be938c7ae8bb0d"}`)
	legacyRecord, err := resolver.DecodeCacheRecordJSON(legacy)
	if err != nil {
		t.Fatal(err)
	}
	legacyRoundTrip, err := json.Marshal(legacyRecord)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(legacyRoundTrip, legacy) {
		t.Fatalf("v0.2.0 CacheRecord wire changed:\n%s\n%s", legacy, legacyRoundTrip)
	}
}

func consumerFactory(reference string) runtimev2.FactoryDeclaration {
	return runtimev2.FactoryDeclaration{Kind: "postgres", Reference: reference, Arguments: []string{}, Attributes: map[string]string{}}
}

func consumerTransform(reference string) runtimev2.TransformDeclaration {
	return runtimev2.TransformDeclaration{Kind: "psql", Reference: reference, Arguments: []string{}, Attributes: map[string]string{}}
}

func transformPointer(value runtimev2.TransformDeclaration) *runtimev2.TransformDeclaration {
	return &value
}

func consumerRecipe(reference string, steps ...composition.RecipeStepInput) composition.RecipeAliasInput {
	factory := consumerFactory(reference)
	return composition.RecipeAliasInput{Base: composition.RecipeBaseInput{Factory: &factory}, Steps: steps}
}

func mustConsumerDocument(t testing.TB, aliases ...composition.NamedAliasInput) composition.AliasDocument {
	t.Helper()
	document, err := composition.NewAliasDocument(composition.DocumentInput{Aliases: aliases})
	if err != nil {
		t.Fatal(err)
	}
	return document
}

func mustConsumerCatalog(t testing.TB, sources ...composition.SourceDocument) composition.Catalog {
	t.Helper()
	catalog, err := composition.NewCatalog(sources)
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func mustExpandConsumerRecipe(t testing.TB, catalog composition.Catalog, name string) composition.ExpandedRecipe {
	t.Helper()
	expanded, err := catalog.ExpandRecipe(name)
	if err != nil {
		t.Fatal(err)
	}
	return expanded
}

func assertConsumerRecipe(t testing.TB, expanded composition.ExpandedRecipe, factory string, transforms ...string) {
	t.Helper()
	declaration := expanded.Declaration()
	if got := declaration.Factory().Reference; got != factory {
		t.Fatalf("factory reference = %q, want %q", got, factory)
	}
	actual := declaration.Transforms()
	if len(actual) != len(transforms) {
		t.Fatalf("transform count = %d, want %d", len(actual), len(transforms))
	}
	for index := range transforms {
		if actual[index].Reference != transforms[index] {
			t.Fatalf("transform[%d] = %q, want %q", index, actual[index].Reference, transforms[index])
		}
	}
}

func mustExpandedConsumerJSON(t testing.TB, recipeName, firstName, secondName string, reverse bool) []byte {
	t.Helper()
	first := consumerTransform("one.sql")
	second := consumerTransform("two.sql")
	steps := []composition.RecipeStepInput{{Use: firstName}, {Use: secondName}}
	if reverse {
		steps[0], steps[1] = steps[1], steps[0]
	}
	recipe := consumerRecipe("postgres:17", steps...)
	catalog := mustConsumerCatalog(t, composition.SourceDocument{
		SourceID: "diagnostics.json",
		Document: mustConsumerDocument(t,
			composition.NamedAliasInput{Name: firstName, Transform: &first},
			composition.NamedAliasInput{Name: secondName, Transform: &second},
			composition.NamedAliasInput{Name: recipeName, Recipe: &recipe},
		),
	})
	raw, err := json.Marshal(mustExpandConsumerRecipe(t, catalog, recipeName).Declaration())
	if err != nil {
		t.Fatal(err)
	}
	return raw
}

func assertConsumerCompositionError(t testing.TB, err error, code composition.ErrorCode) *composition.Error {
	t.Helper()
	if !errors.Is(err, composition.ErrInvalid) {
		t.Fatalf("error %v does not match ErrInvalid", err)
	}
	var problem *composition.Error
	if !errors.As(err, &problem) || problem.Code != code {
		t.Fatalf("composition error = %#v, want code %q", problem, code)
	}
	return problem
}
