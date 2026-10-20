package composition

import (
	"bytes"
	"encoding/json"
	"fmt"
	"reflect"
	"strings"
	"testing"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
)

// DL04: repeated references may exceed the recipe budget even though the
// source catalog and every independently admitted transform fit comfortably.
func TestLargeDeclarationExpansionBudget(t *testing.T) {
	input, err := runtimev2.NewInputDeclaration(runtimev2.ExtensionSpecificationInput{
		SchemaVersion: runtimev2.SchemaVersion, Owner: "example", Kind: "file", SpecificationSchema: "example.file.v1",
		Fields: []runtimev2.DeclarationField{{Name: "payload", Value: strings.Repeat("x", 256<<10)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	transform := testTransform("large.sql")
	transform.Inputs = []runtimev2.InputDeclaration{input}
	steps := make([]RecipeStepInput, 20)
	for i := range steps {
		steps[i] = namedTransformStep("large")
	}
	document := mustDocument(t,
		NamedAliasInput{Name: "large", Transform: &transform},
		NamedAliasInput{Name: "fits", Recipe: factoryRecipe("base", steps[:15]...)},
		NamedAliasInput{Name: "overflow", Recipe: factoryRecipe("base", steps...)},
	)
	catalog := mustCatalog(t, SourceDocument{SourceID: "source.json", Document: document})
	result, err := catalog.ExpandRecipe("overflow")
	problem := assertCompositionError(t, err, CodeExpansionTooLarge)
	if problem.SourceID != "source.json" || problem.Pointer != "/aliases/overflow" {
		t.Fatalf("wrong location: %v", err)
	}
	if !reflect.DeepEqual(result, ExpandedRecipe{}) || result.Trace().Nodes() != nil {
		t.Fatal("partial expansion or trace escaped")
	}
	fit, err := catalog.ExpandRecipe("fits")
	if err != nil {
		t.Fatal(err)
	}
	declaration := fit.Declaration()
	if len(declaration.Transforms()) != 15 || len(mustJSON(t, declaration)) > runtimev2.MaxJSONBytes {
		t.Fatal("valid expansion lost")
	}
	copy := declaration.Transforms()
	copy[0].Inputs = nil
	if len(fit.Declaration().Transforms()[0].Inputs[0].Fields()[0].Value) != 256<<10 {
		t.Fatal("defensive copy lost payload")
	}
	standalone, err := catalog.ExpandTransform("large")
	if err != nil || len(standalone.Declaration().Declaration().Inputs[0].Fields()[0].Value) != 256<<10 {
		t.Fatalf("standalone copy: %v", err)
	}
}

// DL04: aggregate failures are not leaf validation errors. Compact untrusted
// JSON can fit the raw limit while its normalized declaration does not.
func TestLargeDeclarationSourceErrorClassification(t *testing.T) {
	inputs := make([]runtimev2.InputDeclaration, 16)
	for i := range inputs {
		var err error
		inputs[i], err = runtimev2.NewInputDeclaration(runtimev2.ExtensionSpecificationInput{
			SchemaVersion: runtimev2.SchemaVersion, Owner: "example", Kind: "file", SpecificationSchema: "example.file.v1",
			Fields: []runtimev2.DeclarationField{{Name: "payload", Value: strings.Repeat("<", 48<<10)}},
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	transform := testTransform("large.sql")
	transform.Inputs = inputs
	value, err := NewAliasDocument(DocumentInput{Aliases: []NamedAliasInput{{Name: "large", Transform: &transform}}})
	problem := assertCompositionError(t, err, CodeExpansionTooLarge)
	if problem.Pointer != "" || !reflect.DeepEqual(value, AliasDocument{}) {
		t.Fatal("aggregate failure must be at source root")
	}
	// This independent wire DTO deliberately bypasses the constructor being tested.
	wireInputs := make([]map[string]any, len(inputs))
	for i, input := range inputs {
		wireInputs[i] = map[string]any{"schema_version": input.SchemaVersion(), "owner": input.Owner(), "kind": input.Kind(), "specification_schema": input.SpecificationSchema(), "fields": input.Fields()}
	}
	wire := map[string]any{"schema_version": AliasSchemaVersion, "aliases": map[string]any{"large": map[string]any{"type": "transform", "declaration": map[string]any{"kind": "psql", "reference": "large.sql", "arguments": []string{}, "attributes": map[string]string{}, "inputs": wireInputs}}}}
	raw := bytes.ReplaceAll(mustJSON(t, wire), []byte(`\u003c`), []byte("<"))
	if len(raw) >= runtimev2.MaxJSONBytes {
		t.Fatal("fixture must fit raw limit")
	}
	seed := mustDocument(t, NamedAliasInput{Name: "seed", Transform: &runtimev2.TransformDeclaration{Kind: "psql", Reference: "seed"}})
	before := mustJSON(t, seed)
	problem = assertCompositionError(t, DecodeAliasDocumentJSON(raw, &seed), CodeExpansionTooLarge)
	if problem.Pointer != "" || !bytes.Equal(before, mustJSON(t, seed)) {
		t.Fatal("decode changed receiver or aggregate location")
	}
	// A sole bad field still has the old invalid-document classification.
	leaf := fmt.Sprintf(`{"schema_version":%q,"aliases":{"bad":{"type":"transform","declaration":{"kind":"psql","reference":"x","arguments":[],"attributes":{},"inputs":[{"schema_version":%q,"owner":"example","kind":"file","specification_schema":"example.file.v1","fields":[{"name":"payload","value":%q}]}]}}}}`, AliasSchemaVersion, runtimev2.SchemaVersion, strings.Repeat("x", (256<<10)+1))
	problem = assertCompositionError(t, json.Unmarshal([]byte(leaf), &seed), CodeInvalidDocument)
	if problem.Pointer == "" || !bytes.Equal(before, mustJSON(t, seed)) {
		t.Fatal("leaf location or atomicity lost")
	}
}

func TestNearBoundaryStandaloneSnapshot(t *testing.T) {
	fields := make([]runtimev2.DeclarationField, 16)
	for i := range fields {
		fields[i] = runtimev2.DeclarationField{Name: fmt.Sprintf("f%03d", i), Value: strings.Repeat("x", 256<<10)}
	}
	inputWire := map[string]any{"schema_version": runtimev2.SchemaVersion, "owner": "example", "kind": "file", "specification_schema": "example.file.v1", "fields": fields}
	transformWire := map[string]any{"kind": "psql", "reference": "near.sql", "arguments": []string{}, "attributes": map[string]string{}, "inputs": []any{inputWire}}
	wire := map[string]any{"schema_version": AliasSchemaVersion, "aliases": map[string]any{"near": map[string]any{"type": "transform", "declaration": transformWire}}}
	raw := mustJSON(t, wire)
	fields[15].Value = fields[15].Value[:len(fields[15].Value)-(len(raw)-(runtimev2.MaxJSONBytes-1))]
	raw = mustJSON(t, wire)
	if len(raw) != runtimev2.MaxJSONBytes-1 {
		t.Fatal("wrong near-boundary oracle")
	}
	var document AliasDocument
	if err := DecodeAliasDocumentJSON(raw, &document); err != nil {
		t.Fatal(err)
	}
	catalog := mustCatalog(t, SourceDocument{SourceID: "near.json", Document: document})
	value, err := catalog.ExpandTransform("near")
	if err != nil {
		t.Fatal(err)
	}
	copy := value.Declaration()
	if got := copy.Declaration().Inputs[0].Fields(); len(got) != 16 || got[15].Value != fields[15].Value {
		t.Fatal("near-boundary snapshot became empty")
	}
	if _, err := json.Marshal(copy); err != nil {
		t.Fatal(err)
	}
}
