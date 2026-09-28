package composition

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
)

// D01, D06: every document variant round-trips through the canonical wire form.
func TestAliasDocumentVariantsAndCanonicalTransport(t *testing.T) {
	transform := testTransform("migrations/main.sql")
	document := mustDocument(t,
		NamedAliasInput{Name: "z-transform", Transform: &transform},
		NamedAliasInput{Name: "a-factory", Recipe: factoryRecipe("postgres:17")},
		NamedAliasInput{Name: "b-prefix", Recipe: prefixRecipe("a-factory", namedTransformStep("z-transform"), inlineTransformStep("inline.sql"))},
	)
	raw := mustJSON(t, document)
	if bytes.Index(raw, []byte(`"a-factory"`)) > bytes.Index(raw, []byte(`"z-transform"`)) {
		t.Fatalf("aliases are not emitted in bytewise name order: %s", raw)
	}
	var decoded AliasDocument
	if err := DecodeAliasDocumentJSON(raw, &decoded); err != nil {
		t.Fatalf("DecodeAliasDocumentJSON: %v", err)
	}
	if !bytes.Equal(raw, mustJSON(t, decoded)) {
		t.Fatalf("round-trip changed canonical bytes:\n%s\n%s", raw, mustJSON(t, decoded))
	}

	empty := mustDocument(t)
	if got := string(mustJSON(t, empty)); got != `{"schema_version":"sqlrs.runtime.v2.aliases.v1","aliases":{}}` {
		t.Fatalf("empty document JSON = %s", got)
	}
}

// D02, D05: constructor unions and duplicate names are closed and atomic.
func TestAliasDocumentRejectsInvalidUnionsAndDuplicateNames(t *testing.T) {
	transform := testTransform("one.sql")
	factory := testFactory("postgres:17")
	cases := []DocumentInput{
		{Aliases: []NamedAliasInput{{Name: "missing"}}},
		{Aliases: []NamedAliasInput{{Name: "both", Transform: &transform, Recipe: factoryRecipe("postgres:17")}}},
		{Aliases: []NamedAliasInput{{Name: "recipe", Recipe: &RecipeAliasInput{Base: RecipeBaseInput{}, Steps: []RecipeStepInput{}}}}},
		{Aliases: []NamedAliasInput{{Name: "recipe", Recipe: &RecipeAliasInput{Base: RecipeBaseInput{Factory: &factory, Recipe: "other"}}}}},
		{Aliases: []NamedAliasInput{{Name: "recipe", Recipe: factoryRecipe("postgres:17", RecipeStepInput{})}}},
		{Aliases: []NamedAliasInput{{Name: "recipe", Recipe: factoryRecipe("postgres:17", RecipeStepInput{Use: "other", Transform: &transform})}}},
		{Aliases: []NamedAliasInput{{Name: "same", Transform: &transform}, {Name: "same", Transform: &transform}}},
	}
	for index, input := range cases {
		if _, err := NewAliasDocument(input); err == nil {
			t.Fatalf("case %d unexpectedly succeeded", index)
		} else {
			assertCompositionError(t, err, CodeInvalidDocument)
		}
	}
}

// D03: strict decoders reject malformed shapes and leave receivers unchanged.
func TestAliasDocumentStrictJSONIsAtomic(t *testing.T) {
	valid := mustDocument(t, NamedAliasInput{Name: "seed", Transform: ptrTransform(testTransform("seed.sql"))})
	before := append([]byte(nil), mustJSON(t, valid)...)
	invalidUTF8 := append([]byte(`{"schema_version":"sqlrs.runtime.v2.aliases.v1","aliases":{"`), 0xff)
	cases := [][]byte{
		[]byte(`{"schema_version":"wrong","aliases":{}}`),
		[]byte(`{"schema_version":"sqlrs.runtime.v2.aliases.v1","aliases":{},"extra":true}`),
		[]byte(`{"schema_version":"sqlrs.runtime.v2.aliases.v1","schema_version":"sqlrs.runtime.v2.aliases.v1","aliases":{}}`),
		[]byte(`{"schema_version":"sqlrs.runtime.v2.aliases.v1","aliases":null}`),
		[]byte(`{"schema_version":"sqlrs.runtime.v2.aliases.v1","aliases":{}} {}`),
		invalidUTF8,
	}
	for index, raw := range cases {
		if err := DecodeAliasDocumentJSON(raw, &valid); err == nil {
			t.Fatalf("case %d unexpectedly succeeded", index)
		}
		if !bytes.Equal(before, mustJSON(t, valid)) {
			t.Fatalf("case %d mutated receiver", index)
		}
	}
	if err := DecodeAliasDocumentJSON(before, nil); err == nil {
		t.Fatal("nil target unexpectedly succeeded")
	}
}

// D04: definitions and references share the exact lower-case identifier grammar.
func TestAliasDocumentNameGrammar(t *testing.T) {
	transform := testTransform("seed.sql")
	validNames := []string{"a", "a0", "a.b-c_d", "a" + strings.Repeat("x", 127)}
	for _, name := range validNames {
		if _, err := NewAliasDocument(DocumentInput{Aliases: []NamedAliasInput{{Name: name, Transform: &transform}}}); err != nil {
			t.Fatalf("valid name %q rejected: %v", name, err)
		}
	}
	invalidNames := []string{"", "A", "0a", "a/b", "a~b", "é", "a\n", "a" + strings.Repeat("x", 128)}
	for _, name := range invalidNames {
		if _, err := NewAliasDocument(DocumentInput{Aliases: []NamedAliasInput{{Name: name, Transform: &transform}}}); err == nil {
			t.Fatalf("invalid name %q accepted", name)
		}
		if _, err := NewAliasDocument(DocumentInput{Aliases: []NamedAliasInput{{Name: "recipe", Recipe: prefixRecipe(name)}}}); err == nil {
			t.Fatalf("invalid reference %q accepted", name)
		}
	}
}

// D07: documents own all caller-provided declaration data.
func TestAliasDocumentOwnsImmutableSnapshot(t *testing.T) {
	transform := testTransform("seed.sql")
	transform.Arguments = []string{"-f", "seed.sql"}
	transform.Attributes = map[string]string{"mode": "strict"}
	aliases := []NamedAliasInput{{Name: "seed", Transform: &transform}}
	document := mustDocument(t, aliases...)
	before := append([]byte(nil), mustJSON(t, document)...)
	transform.Reference = "changed.sql"
	transform.Arguments[0] = "changed"
	transform.Attributes["mode"] = "changed"
	aliases[0].Name = "changed"
	if !bytes.Equal(before, mustJSON(t, document)) {
		t.Fatal("caller mutation changed document")
	}
}

// D08: constructed empty values are valid while zero values are never transports.
func TestAliasDocumentZeroAndEmptyValues(t *testing.T) {
	if _, err := NewAliasDocument(DocumentInput{}); err != nil {
		t.Fatalf("empty constructor: %v", err)
	}
	if _, err := json.Marshal(AliasDocument{}); err == nil {
		t.Fatal("zero document marshalled")
	}
	if _, err := json.Marshal(ExpansionTrace{}); err == nil {
		t.Fatal("zero trace marshalled")
	}
	if _, err := (Catalog{}).ExpandRecipe("missing"); err == nil {
		t.Fatal("zero catalog expanded")
	}
}

// D09: count limits apply equally to constructor and decoder inputs.
func TestAliasDocumentCountAndTransportBounds(t *testing.T) {
	transform := testTransform("x")
	aliases := make([]NamedAliasInput, MaxAliases+1)
	for index := range aliases {
		aliases[index] = NamedAliasInput{Name: indexedName(index), Transform: &transform}
	}
	if _, err := NewAliasDocument(DocumentInput{Aliases: aliases[:MaxAliases]}); err != nil {
		t.Fatalf("MaxAliases rejected: %v", err)
	}
	if _, err := NewAliasDocument(DocumentInput{Aliases: aliases}); err == nil {
		t.Fatal("MaxAliases+1 accepted")
	} else {
		assertCompositionError(t, err, CodeExpansionTooLarge)
	}

	oversized := []byte(`{"schema_version":"sqlrs.runtime.v2.aliases.v1","aliases":{},"padding":"` + strings.Repeat("x", runtimev2.MaxJSONBytes) + `"}`)
	var target AliasDocument
	if err := DecodeAliasDocumentJSON(oversized, &target); err == nil {
		t.Fatal("oversized JSON accepted")
	} else {
		assertCompositionError(t, err, CodeExpansionTooLarge)
	}
}

// D10: nested Runtime failures retain their original stable error contract.
func TestAliasDocumentPreservesNestedRuntimeValidation(t *testing.T) {
	invalid := testTransform("seed.sql")
	invalid.Kind = "BAD"
	_, err := NewAliasDocument(DocumentInput{Aliases: []NamedAliasInput{{Name: "seed", Transform: &invalid}}})
	compositionError := assertCompositionError(t, err, CodeInvalidDocument)
	if compositionError.Pointer != "/aliases/seed/declaration" {
		t.Fatalf("pointer = %q", compositionError.Pointer)
	}
	var validation *runtimev2.ValidationError
	if !errors.As(err, &validation) || validation.Code != runtimev2.CodeInvalidValue || validation.Path != "kind" {
		t.Fatalf("nested validation = %#v (%v)", validation, err)
	}
	if !utf8.ValidString(err.Error()) {
		t.Fatal("error is not valid UTF-8")
	}
}

func ptrTransform(value runtimev2.TransformDeclaration) *runtimev2.TransformDeclaration {
	return &value
}

func indexedName(index int) string {
	const digits = "0123456789abcdefghijklmnopqrstuvwxyz"
	if index == 0 {
		return "a0"
	}
	var suffix [16]byte
	position := len(suffix)
	for index > 0 {
		position--
		suffix[position] = digits[index%len(digits)]
		index /= len(digits)
	}
	return "a" + string(suffix[position:])
}
