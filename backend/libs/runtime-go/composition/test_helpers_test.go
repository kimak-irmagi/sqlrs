package composition

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
)

func testFactory(reference string) runtimev2.FactoryDeclaration {
	return runtimev2.FactoryDeclaration{
		Kind:       "postgres",
		Reference:  reference,
		Arguments:  []string{},
		Attributes: map[string]string{},
	}
}

func testTransform(reference string) runtimev2.TransformDeclaration {
	return runtimev2.TransformDeclaration{
		Kind:       "psql",
		Reference:  reference,
		Arguments:  []string{},
		Attributes: map[string]string{},
	}
}

func factoryRecipe(reference string, steps ...RecipeStepInput) *RecipeAliasInput {
	factory := testFactory(reference)
	return &RecipeAliasInput{Base: RecipeBaseInput{Factory: &factory}, Steps: steps}
}

func prefixRecipe(name string, steps ...RecipeStepInput) *RecipeAliasInput {
	return &RecipeAliasInput{Base: RecipeBaseInput{Recipe: name}, Steps: steps}
}

func namedTransformStep(name string) RecipeStepInput { return RecipeStepInput{Use: name} }

func inlineTransformStep(reference string) RecipeStepInput {
	value := testTransform(reference)
	return RecipeStepInput{Transform: &value}
}

func mustDocument(t testing.TB, aliases ...NamedAliasInput) AliasDocument {
	t.Helper()
	document, err := NewAliasDocument(DocumentInput{Aliases: aliases})
	if err != nil {
		t.Fatalf("NewAliasDocument: %v", err)
	}
	return document
}

func mustCatalog(t testing.TB, sources ...SourceDocument) Catalog {
	t.Helper()
	catalog, err := NewCatalog(sources)
	if err != nil {
		t.Fatalf("NewCatalog: %v", err)
	}
	return catalog
}

func mustJSON(t testing.TB, value any) []byte {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return raw
}

func assertCompositionError(t testing.TB, err error, code ErrorCode) *Error {
	t.Helper()
	if !errors.Is(err, ErrInvalid) {
		t.Fatalf("error %v does not match ErrInvalid", err)
	}
	var target *Error
	if !errors.As(err, &target) {
		t.Fatalf("error %T does not expose *Error: %v", err, err)
	}
	if target.Code != code {
		t.Fatalf("error code = %q, want %q", target.Code, code)
	}
	return target
}

func assertRecipe(t testing.TB, got ExpandedRecipe, factory string, transforms ...string) {
	t.Helper()
	declaration := got.Declaration()
	if declaration.Factory().Reference != factory {
		t.Fatalf("factory reference = %q, want %q", declaration.Factory().Reference, factory)
	}
	values := declaration.Transforms()
	if len(values) != len(transforms) {
		t.Fatalf("transform count = %d, want %d", len(values), len(transforms))
	}
	for index, want := range transforms {
		if values[index].Reference != want {
			t.Fatalf("transform[%d] = %q, want %q", index, values[index].Reference, want)
		}
	}
}

func assertJSONEqual(t testing.TB, left, right any) {
	t.Helper()
	var leftValue, rightValue any
	if err := json.Unmarshal(mustJSON(t, left), &leftValue); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(mustJSON(t, right), &rightValue); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(leftValue, rightValue) {
		t.Fatalf("JSON values differ:\nleft:  %s\nright: %s", mustJSON(t, left), mustJSON(t, right))
	}
}

func repeated(value string, bytes int) string {
	if bytes <= 0 {
		return ""
	}
	return strings.Repeat(value, bytes/len(value)) + value[:bytes%len(value)]
}
