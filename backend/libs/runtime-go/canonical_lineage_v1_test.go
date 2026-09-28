package runtimev2_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
	"github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/schemaauthor"
)

func testLineageTransform(t *testing.T, script string) runtimev2.FingerprintEnvelope {
	t.Helper()
	schema, err := schemaauthor.NewTransformSchema(schemaauthor.SchemaInput{
		Provider:       "acme",
		SemanticKind:   "migration",
		IdentitySchema: "acme.migration.v1",
		Fields: []schemaauthor.FieldDefinition{{
			Name: "script", Role: schemaauthor.FieldRoleSemantic,
			Kind: runtimev2.IdentityFieldText, Required: true,
			Disclosure: schemaauthor.DisclosurePublic,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	builder, err := runtimev2.NewTransformIdentityBuilder(schema, runtimev2.TransformDeclaration{
		Kind: "migration", Reference: script, Arguments: []string{}, Attributes: map[string]string{},
	})
	if err != nil {
		t.Fatal(err)
	}
	field, err := runtimev2.NewTextIdentityField("script", script)
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.AddIdentityField(field); err != nil {
		t.Fatal(err)
	}
	composition, err := builder.Build()
	if err != nil {
		t.Fatal(err)
	}
	result, err := runtimev2.NewTransformEnvelope(composition.Identity())
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func requireLineageCode(t *testing.T, err error, code runtimev2.ValidationCode) {
	t.Helper()
	var validation *runtimev2.ValidationError
	if !errors.As(err, &validation) || validation.Code != code {
		t.Fatalf("error = %v, want validation code %q", err, code)
	}
}

func mutateLineageJSON(t *testing.T, raw []byte, mutate func(map[string]any)) []byte {
	t.Helper()
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}
	mutate(document)
	result, err := json.Marshal(document)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestCanonicalRecipeAndRelativeLineageStrictRoundTrip(t *testing.T) {
	root, _ := runtimev2.NewFactoryEnvelope(testEnvelopeFactoryIdentity(t))
	first, second := testLineageTransform(t, "one"), testLineageTransform(t, "two")
	recipe, err := runtimev2.NewCanonicalRecipeEnvelope(root, []runtimev2.FingerprintEnvelope{first, second})
	if err != nil {
		t.Fatal(err)
	}
	if len(recipe.Steps()) != 2 || recipe.Endpoint().Descriptor().Digest() == root.Descriptor().Digest() {
		t.Fatal("recipe lineage not derived")
	}
	raw, _ := json.Marshal(recipe)
	var decoded runtimev2.CanonicalRecipeEnvelope
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Endpoint().Descriptor().Digest() != recipe.Endpoint().Descriptor().Digest() {
		t.Fatal("endpoint changed")
	}
	relative, err := runtimev2.NewCanonicalRelativeEnvelope(recipe.Endpoint(), []runtimev2.FingerprintEnvelope{first})
	if err != nil {
		t.Fatal(err)
	}
	relativeRaw, _ := json.Marshal(relative)
	var decodedRelative runtimev2.CanonicalRelativeEnvelope
	if err := json.Unmarshal(relativeRaw, &decodedRelative); err != nil {
		t.Fatal(err)
	}
	tampered := strings.Replace(string(raw), string(recipe.Endpoint().Descriptor().Digest()), "sha256:"+strings.Repeat("0", 64), 1)
	if err := json.Unmarshal([]byte(tampered), &decoded); err == nil {
		t.Fatal("tampered endpoint accepted")
	}
	if _, err := runtimev2.NewCanonicalRelativeEnvelope(first, nil); err == nil {
		t.Fatal("bare transform accepted as anchor")
	}
}

func TestCanonicalLineageConstructorsAndAccessors(t *testing.T) {
	root, err := runtimev2.NewFactoryEnvelope(testEnvelopeFactoryIdentity(t))
	if err != nil {
		t.Fatal(err)
	}
	transform := testLineageTransform(t, "one")

	emptyRecipe, err := runtimev2.NewCanonicalRecipeEnvelope(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !emptyRecipe.Root().Valid() || len(emptyRecipe.Steps()) != 0 || emptyRecipe.Endpoint().Descriptor().Digest() != root.Descriptor().Digest() {
		t.Fatal("empty recipe does not preserve its root as endpoint")
	}
	emptyRelative, err := runtimev2.NewCanonicalRelativeEnvelope(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !emptyRelative.Anchor().Valid() || len(emptyRelative.Steps()) != 0 || emptyRelative.Endpoint().Descriptor().Digest() != root.Descriptor().Digest() {
		t.Fatal("empty relative lineage does not preserve its anchor as endpoint")
	}

	derived, err := runtimev2.NewDerivedStateEnvelope(runtimev2.CanonicalStateID(root.Descriptor().Digest()), transform)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtimev2.NewCanonicalRelativeEnvelope(derived, []runtimev2.FingerprintEnvelope{transform}); err != nil {
		t.Fatalf("derived state rejected as relative anchor: %v", err)
	}
	if _, err := runtimev2.NewCanonicalRecipeEnvelope(derived, nil); err == nil {
		t.Fatal("derived state accepted as a recipe root")
	} else {
		requireLineageCode(t, err, runtimev2.CodeDescriptorInvalid)
	}
	value := runtimev2.CanonicalNull()
	valueEnvelope, err := runtimev2.NewCanonicalValueEnvelope(value)
	if err != nil {
		t.Fatal(err)
	}
	for name, anchor := range map[string]runtimev2.FingerprintEnvelope{
		"zero": {}, "canonical value": valueEnvelope, "transform": transform,
	} {
		t.Run("invalid relative anchor "+name, func(t *testing.T) {
			if _, err := runtimev2.NewCanonicalRelativeEnvelope(anchor, nil); err == nil {
				t.Fatal("invalid anchor accepted")
			}
		})
	}
	for name, badTransform := range map[string]runtimev2.FingerprintEnvelope{"zero": {}, "factory": root} {
		t.Run("invalid transform "+name, func(t *testing.T) {
			if _, err := runtimev2.NewCanonicalRecipeEnvelope(root, []runtimev2.FingerprintEnvelope{badTransform}); err == nil {
				t.Fatal("invalid transform accepted")
			} else {
				requireLineageCode(t, err, runtimev2.CodeDescriptorInvalid)
			}
		})
	}

	var zeroRecipe runtimev2.CanonicalRecipeEnvelope
	if zeroRecipe.Root().Valid() || zeroRecipe.Steps() != nil || zeroRecipe.Endpoint().Valid() {
		t.Fatal("zero recipe accessors must return zero values")
	}
	if _, err := json.Marshal(zeroRecipe); err == nil {
		t.Fatal("zero recipe marshaled")
	}
	var zeroRelative runtimev2.CanonicalRelativeEnvelope
	if zeroRelative.Anchor().Valid() || zeroRelative.Steps() != nil || zeroRelative.Endpoint().Valid() {
		t.Fatal("zero relative accessors must return zero values")
	}
	if _, err := json.Marshal(zeroRelative); err == nil {
		t.Fatal("zero relative lineage marshaled")
	}
	var nilRecipe *runtimev2.CanonicalRecipeEnvelope
	requireLineageCode(t, nilRecipe.UnmarshalJSON([]byte(`{}`)), runtimev2.CodeShapeInvalid)
	var nilRelative *runtimev2.CanonicalRelativeEnvelope
	requireLineageCode(t, nilRelative.UnmarshalJSON([]byte(`{}`)), runtimev2.CodeShapeInvalid)
}

func TestCanonicalRecipeRejectsLineageFaultMatrixAtomically(t *testing.T) {
	root, err := runtimev2.NewFactoryEnvelope(testEnvelopeFactoryIdentity(t))
	if err != nil {
		t.Fatal(err)
	}
	first := testLineageTransform(t, "one")
	second := testLineageTransform(t, "two")
	recipe, err := runtimev2.NewCanonicalRecipeEnvelope(root, []runtimev2.FingerprintEnvelope{first, second})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(recipe)
	if err != nil {
		t.Fatal(err)
	}
	originalEndpoint := recipe.Endpoint().Descriptor().Digest()

	tests := []struct {
		name   string
		code   runtimev2.ValidationCode
		mutate func(map[string]any)
	}{
		{"wrong schema", runtimev2.CodeRevisionMismatch, func(value map[string]any) { value["schema_version"] = "future" }},
		{"missing schema", runtimev2.CodeRevisionMismatch, func(value map[string]any) { delete(value, "schema_version") }},
		{"missing anchor", runtimev2.CodeShapeInvalid, func(value map[string]any) { delete(value, "anchor") }},
		{"null anchor", runtimev2.CodeShapeInvalid, func(value map[string]any) { value["anchor"] = nil }},
		{"invalid anchor", runtimev2.CodeShapeInvalid, func(value map[string]any) { value["anchor"] = "invalid" }},
		{"invalid step", runtimev2.CodeShapeInvalid, func(value map[string]any) { value["steps"].([]any)[0] = "invalid" }},
		{"non-derived step", runtimev2.CodeLineageMismatch, func(value map[string]any) { value["steps"].([]any)[0] = value["anchor"] }},
		{"reordered steps", runtimev2.CodeLineageMismatch, func(value map[string]any) {
			steps := value["steps"].([]any)
			steps[0], steps[1] = steps[1], steps[0]
		}},
		{"wrong endpoint", runtimev2.CodeEndpointMismatch, func(value map[string]any) { value["endpoint"] = "sha256:" + strings.Repeat("0", 64) }},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			candidate := recipe
			err := json.Unmarshal(mutateLineageJSON(t, raw, test.mutate), &candidate)
			requireLineageCode(t, err, test.code)
			if candidate.Endpoint().Descriptor().Digest() != originalEndpoint {
				t.Fatal("failed decode mutated the receiver")
			}
		})
	}
}

func TestCanonicalLineageRequiresExplicitStepsMember(t *testing.T) {
	root, err := runtimev2.NewFactoryEnvelope(testEnvelopeFactoryIdentity(t))
	if err != nil {
		t.Fatal(err)
	}
	recipe, err := runtimev2.NewCanonicalRecipeEnvelope(root, nil)
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(recipe)
	if err != nil {
		t.Fatal(err)
	}
	withoutSteps := mutateLineageJSON(t, raw, func(value map[string]any) { delete(value, "steps") })
	var decoded runtimev2.CanonicalRecipeEnvelope
	requireLineageCode(t, json.Unmarshal(withoutSteps, &decoded), runtimev2.CodeShapeInvalid)
}

func TestCanonicalRelativeRejectsRecipeRootAndParentMismatch(t *testing.T) {
	root, err := runtimev2.NewFactoryEnvelope(testEnvelopeFactoryIdentity(t))
	if err != nil {
		t.Fatal(err)
	}
	transform := testLineageTransform(t, "one")
	recipe, err := runtimev2.NewCanonicalRecipeEnvelope(root, []runtimev2.FingerprintEnvelope{transform})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(recipe)
	if err != nil {
		t.Fatal(err)
	}

	var relative runtimev2.CanonicalRelativeEnvelope
	if err := json.Unmarshal(raw, &relative); err != nil {
		t.Fatalf("factory-root lineage is also a valid relative lineage: %v", err)
	}
	if relative.Anchor().Descriptor().Digest() != root.Descriptor().Digest() || len(relative.Steps()) != 1 {
		t.Fatal("relative lineage accessors changed decoded content")
	}

	wrongStep, err := runtimev2.NewDerivedStateEnvelope(runtimev2.CanonicalStateID(transform.Descriptor().Digest()), transform)
	if err != nil {
		t.Fatal(err)
	}
	var wrongStepJSON any
	encodedStep, _ := json.Marshal(wrongStep)
	if err := json.Unmarshal(encodedStep, &wrongStepJSON); err != nil {
		t.Fatal(err)
	}
	bad := mutateLineageJSON(t, raw, func(value map[string]any) { value["steps"].([]any)[0] = wrongStepJSON })
	requireLineageCode(t, json.Unmarshal(bad, &relative), runtimev2.CodeLineageMismatch)
}
