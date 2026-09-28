package runtimev2

import (
	"encoding/json"
	"errors"
	"testing"
)

// These cases preserve the explicit declaration boundary used by CP03
// re-resolution into a selected canonical-v1 schema.
func TestRecipeDeclarationFailureAndAtomicDecodeMatrix(t *testing.T) {
	validFactory := FactoryDeclaration{Kind: "docker", Reference: "postgres:17", Arguments: []string{}, Attributes: map[string]string{}}
	if _, err := NewRecipeDeclaration(FactoryDeclaration{}, nil); err == nil {
		t.Fatal("invalid factory accepted")
	}
	tooMany := make([]TransformDeclaration, MaxTransforms+1)
	if _, err := NewRecipeDeclaration(validFactory, tooMany); err == nil {
		t.Fatal("too many transforms accepted")
	}
	if _, err := NewRecipeDeclaration(validFactory, []TransformDeclaration{{}}); err == nil {
		t.Fatal("invalid transform accepted")
	}
	if _, err := json.Marshal(RecipeDeclaration{}); err == nil {
		t.Fatal("zero recipe declaration marshaled")
	}
	var nilTarget *RecipeDeclaration
	if err := nilTarget.UnmarshalJSON([]byte(`{}`)); err == nil {
		t.Fatal("nil receiver accepted")
	}

	version := SchemaVersion
	wrong := CanonicalSchemaVersion
	cases := []recipeDeclarationWire{
		{},
		{SchemaVersion: &wrong},
		{SchemaVersion: &version},
		{SchemaVersion: &version, Factory: &validFactory},
	}
	for index, wire := range cases {
		raw, _ := json.Marshal(wire)
		var target RecipeDeclaration
		if err := json.Unmarshal(raw, &target); err == nil {
			t.Fatalf("invalid wire %d accepted", index)
		}
	}
}

func TestStrictJSONErrorNormalizationPreservesContract(t *testing.T) {
	structured := invalid(CodeInvalidShape, "field")
	if normalizeJSONError(structured) != structured {
		t.Fatal("structured validation error was replaced")
	}
	var validation *ValidationError
	if err := normalizeJSONError(errors.New("synthetic syntax failure")); !errors.As(err, &validation) || validation.Code != CodeInvalidShape {
		t.Fatalf("unexpected fallback: %v", err)
	}
}
