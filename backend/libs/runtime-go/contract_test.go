package runtimev2_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
)

func TestFactoryOnlyAndOrderedLineage(t *testing.T) {
	factory := testFactory(t, "a")
	recipe, err := runtimev2.NewRecipe(factory, nil)
	if err != nil {
		t.Fatal(err)
	}
	lineage, err := runtimev2.Build(recipe)
	if err != nil {
		t.Fatal(err)
	}
	if len(lineage.Steps()) != 0 || lineage.Endpoint().ID() != lineage.Root().ID() {
		t.Fatalf("factory-only lineage invented a step: %+v", lineage)
	}

	a := testTransform(t, "a", "one.sql")
	b := testTransform(t, "b", "two.sql")
	ab := buildRecipe(t, factory, a, b)
	ba := buildRecipe(t, factory, b, a)
	if ab.Endpoint().ID() == ba.Endpoint().ID() {
		t.Fatal("transform order did not affect endpoint")
	}
	steps := ab.Steps()
	if len(steps) != 2 || steps[0].State().ParentID() != ab.Root().ID() ||
		steps[1].State().ParentID() != steps[0].State().ID() {
		t.Fatalf("broken parent chain: %+v", steps)
	}
}

func TestParentAndIdentitySensitivity(t *testing.T) {
	transform := testTransform(t, "a", "one.sql")
	parentA := runtimev2.StateID("sha256:" + strings.Repeat("1", 64))
	parentB := runtimev2.StateID("sha256:" + strings.Repeat("2", 64))
	stepA, err := runtimev2.Derive(parentA, transform)
	if err != nil {
		t.Fatal(err)
	}
	stepB, err := runtimev2.Derive(parentB, transform)
	if err != nil {
		t.Fatal(err)
	}
	if stepA.State().ID() == stepB.State().ID() {
		t.Fatal("different parents produced the same derived state")
	}

	base := identityInput("a")
	original, err := runtimev2.NewTransformIdentity(base)
	if err != nil {
		t.Fatal(err)
	}
	originalFingerprint, err := runtimev2.TransformFingerprint(original)
	if err != nil {
		t.Fatal(err)
	}
	mutations := map[string]func(*runtimev2.TransformIdentityInput){
		"provider":        func(v *runtimev2.TransformIdentityInput) { v.Provider = "other" },
		"kind":            func(v *runtimev2.TransformIdentityInput) { v.Kind = "liquibase" },
		"identity schema": func(v *runtimev2.TransformIdentityInput) { v.IdentitySchema = "sqlrs.transform.psql.v2" },
		"field name":      func(v *runtimev2.TransformIdentityInput) { v.Fields[0].Name = "source.digest" },
		"field value":     func(v *runtimev2.TransformIdentityInput) { v.Fields[0].Value = "sha256:" + strings.Repeat("b", 64) },
		"add field": func(v *runtimev2.TransformIdentityInput) {
			v.Fields = append(v.Fields, runtimev2.ResolvedField{Name: "mode", Value: "strict"})
		},
		"remove field": func(v *runtimev2.TransformIdentityInput) { v.Fields = nil },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			changed := identityInput("a")
			mutate(&changed)
			identity, err := runtimev2.NewTransformIdentity(changed)
			if err != nil {
				t.Fatal(err)
			}
			fingerprint, err := runtimev2.TransformFingerprint(identity)
			if err != nil || fingerprint == originalFingerprint {
				t.Fatalf("mutation did not change fingerprint: %s %v", fingerprint, err)
			}
		})
	}
}

func TestDiagnosticChangesDoNotAffectIdentity(t *testing.T) {
	identity, err := runtimev2.NewTransformIdentity(identityInput("a"))
	if err != nil {
		t.Fatal(err)
	}
	first, err := runtimev2.NewTransformProvenance(runtimev2.TransformProvenanceInput{
		Identity:    identity,
		Declaration: &runtimev2.TransformDeclaration{Kind: "psql", Reference: "one.sql", Arguments: []string{"-v", "x=1"}, Attributes: map[string]string{"note": "first"}},
		Resolver:    &runtimev2.ResolverObservation{Implementation: "resolver", Version: "1"},
	})
	if err != nil {
		t.Fatal(err)
	}
	second, err := runtimev2.NewTransformProvenance(runtimev2.TransformProvenanceInput{
		Identity:    identity,
		Declaration: &runtimev2.TransformDeclaration{Kind: "psql", Reference: "./one.sql", Arguments: []string{"x=1", "-v"}, Attributes: map[string]string{"note": "second"}},
		Resolver:    &runtimev2.ResolverObservation{Implementation: "resolver-next", Version: "2"},
	})
	if err != nil {
		t.Fatal(err)
	}
	a, _ := runtimev2.Derive(runtimev2.StateID("sha256:"+strings.Repeat("1", 64)), first)
	b, _ := runtimev2.Derive(runtimev2.StateID("sha256:"+strings.Repeat("1", 64)), second)
	if a.Fingerprint() != b.Fingerprint() || a.State().ID() != b.State().ID() {
		t.Fatal("diagnostics affected logical identity")
	}
	ja, _ := json.Marshal(first)
	jb, _ := json.Marshal(second)
	if string(ja) == string(jb) {
		t.Fatal("diagnostic provenance was not serialized")
	}
}

func TestRelativeLineageAndDefensiveCopies(t *testing.T) {
	transforms := []runtimev2.TransformProvenance{testTransform(t, "a", "one.sql")}
	anchor := runtimev2.StateID("sha256:" + strings.Repeat("1", 64))
	lineage, err := runtimev2.Extend(anchor, transforms)
	if err != nil {
		t.Fatal(err)
	}
	transforms[0] = testTransform(t, "b", "two.sql")
	steps := lineage.Steps()
	expected := steps[0].State().ID()
	steps[0] = runtimev2.LineageStep{}
	if lineage.EndpointID() != expected || lineage.Steps()[0].State().ID() != expected {
		t.Fatal("lineage exposed mutable storage")
	}
	empty, err := runtimev2.Extend(anchor, nil)
	if err != nil || empty.EndpointID() != anchor || len(empty.Steps()) != 0 {
		t.Fatalf("empty relative suffix changed anchor: %+v %v", empty, err)
	}
}

func TestValidationErrorContract(t *testing.T) {
	input := identityInput("a")
	input.Provider = "INVALID"
	got, err := runtimev2.NewTransformIdentity(input)
	if err == nil || got != (runtimev2.ResolvedTransformIdentity{}) || !errors.Is(err, runtimev2.ErrInvalid) {
		t.Fatalf("invalid result contract: %+v %v", got, err)
	}
	var validation *runtimev2.ValidationError
	if !errors.As(err, &validation) || validation.Code != runtimev2.CodeInvalidValue || validation.Path != "provider" {
		t.Fatalf("unexpected validation detail: %+v", validation)
	}
	if strings.Contains(err.Error(), "INVALID") {
		t.Fatal("validation error leaked rejected value")
	}
}

func buildRecipe(t *testing.T, factory runtimev2.FactoryProvenance, transforms ...runtimev2.TransformProvenance) runtimev2.RecipeLineage {
	t.Helper()
	recipe, err := runtimev2.NewRecipe(factory, transforms)
	if err != nil {
		t.Fatal(err)
	}
	lineage, err := runtimev2.Build(recipe)
	if err != nil {
		t.Fatal(err)
	}
	return lineage
}

func testFactory(t *testing.T, seed string) runtimev2.FactoryProvenance {
	t.Helper()
	identity, err := runtimev2.NewFactoryIdentity(runtimev2.FactoryIdentityInput{
		SchemaVersion: runtimev2.SchemaVersion,
		Provider:      "sqlrs", Kind: "postgres", IdentitySchema: "sqlrs.factory.postgres.v1",
		Fields: []runtimev2.ResolvedField{{Name: "image.digest", Value: "sha256:" + strings.Repeat(seed, 64)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	got, err := runtimev2.NewFactoryProvenance(runtimev2.FactoryProvenanceInput{Identity: identity})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func testTransform(t *testing.T, seed, reference string) runtimev2.TransformProvenance {
	t.Helper()
	identity, err := runtimev2.NewTransformIdentity(identityInput(seed))
	if err != nil {
		t.Fatal(err)
	}
	got, err := runtimev2.NewTransformProvenance(runtimev2.TransformProvenanceInput{
		Identity:    identity,
		Declaration: &runtimev2.TransformDeclaration{Kind: "psql", Reference: reference, Arguments: []string{}, Attributes: map[string]string{}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return got
}

func identityInput(seed string) runtimev2.TransformIdentityInput {
	return runtimev2.TransformIdentityInput{
		SchemaVersion: runtimev2.SchemaVersion,
		Provider:      "sqlrs", Kind: "psql", IdentitySchema: "sqlrs.transform.psql.v1",
		Fields: []runtimev2.ResolvedField{{Name: "content.digest", Value: "sha256:" + strings.Repeat(seed, 64)}},
	}
}
