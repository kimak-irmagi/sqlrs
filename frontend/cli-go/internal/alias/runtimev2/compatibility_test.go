package aliasruntimev2

import (
	"errors"
	"path/filepath"
	"strings"
	"testing"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
	legacyalias "github.com/sqlrs/cli/internal/alias"
)

type testAdapter struct {
	kind   string
	result Result
	err    error
	calls  int
	input  TranslationInput
}

type valueAdapter struct{ result Result }

func (a valueAdapter) Kind() string                               { return "psql" }
func (a valueAdapter) Translate(TranslationInput) (Result, error) { return a.result, nil }

func (a *testAdapter) Kind() string { return a.kind }
func (a *testAdapter) Translate(input TranslationInput) (Result, error) {
	a.calls++
	a.input = input
	return a.result, a.err
}

func testRecipe(t *testing.T) runtimev2.RecipeDeclaration {
	t.Helper()
	recipe, err := runtimev2.NewRecipeDeclaration(runtimev2.FactoryDeclaration{
		Kind: "postgres", Reference: "postgres:17", Arguments: []string{}, Attributes: map[string]string{},
	}, []runtimev2.TransformDeclaration{{
		Kind: "psql", Reference: "db/schema.sql", Arguments: []string{"-f", "db/schema.sql"}, Attributes: map[string]string{},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return recipe
}

func validTranslationInput(t *testing.T) TranslationInput {
	t.Helper()
	root := filepath.Clean(t.TempDir())
	aliasPath := filepath.Join(root, "db", "schema.prep.s9s.yaml")
	return TranslationInput{
		Definition:    legacyalias.Definition{Class: legacyalias.ClassPrepare, Kind: "psql", Image: "postgres:17", Args: []string{"-f", "schema.sql"}},
		WorkspaceRoot: root, AliasPath: aliasPath, SourceID: "db/schema.prep.s9s.yaml",
		EffectiveImage: "postgres:17", EffectiveImageSource: ImageSourceAlias,
	}
}

func TestResultConstructorsAreExclusiveAndImmutable(t *testing.T) {
	recipe := testRecipe(t)
	translated, err := NewTranslatedResult(recipe)
	if err != nil {
		t.Fatal(err)
	}
	if translated.Status() != StatusTranslated || translated.Reason() != "" || translated.Message() != "" {
		t.Fatalf("unexpected translated metadata: %#v", translated)
	}
	declaration, ok := translated.Declaration()
	if !ok || declaration.Factory().Reference != "postgres:17" || len(declaration.Transforms()) != 1 {
		t.Fatalf("unexpected translated declaration: %#v, %v", declaration, ok)
	}
	mutated := declaration.Factory()
	mutated.Reference = "changed"
	if again, _ := translated.Declaration(); again.Factory().Reference != "postgres:17" {
		t.Fatal("result leaked mutable declaration state")
	}

	legacyOnly, err := NewLegacyOnlyResult(ReasonUnsupported, "use the existing legacy executor")
	if err != nil {
		t.Fatal(err)
	}
	if legacyOnly.Status() != StatusLegacyOnly || legacyOnly.Reason() != ReasonUnsupported || legacyOnly.Message() == "" {
		t.Fatalf("unexpected legacy-only metadata: %#v", legacyOnly)
	}
	if _, ok := legacyOnly.Declaration(); ok {
		t.Fatal("legacy-only result exposed a declaration")
	}

	for _, invalid := range []struct {
		reason  ReasonCode
		message string
	}{
		{"unknown", "message"},
		{ReasonUnsupported, ""},
		{ReasonUnsupported, strings.Repeat("x", MaxCompatibilityMessageBytes+1)},
		{ReasonUnsupported, string([]byte{0xff})},
	} {
		if _, err := NewLegacyOnlyResult(invalid.reason, invalid.message); err == nil {
			t.Fatalf("invalid legacy-only result accepted: %#v", invalid)
		}
	}
	for _, size := range []int{MaxCompatibilityMessageBytes - 1, MaxCompatibilityMessageBytes} {
		if _, err := NewLegacyOnlyResult(ReasonUnsupported, strings.Repeat("x", size)); err != nil {
			t.Fatalf("valid message size %d rejected: %v", size, err)
		}
	}
	if _, err := NewTranslatedResult(runtimev2.RecipeDeclaration{}); err == nil {
		t.Fatal("zero declaration accepted")
	}
}

func TestTranslateLegacyReturnsCompleteProviderResult(t *testing.T) {
	want, err := NewTranslatedResult(testRecipe(t))
	if err != nil {
		t.Fatal(err)
	}
	adapter := &testAdapter{kind: "psql", result: want}
	input := validTranslationInput(t)
	got, err := TranslateLegacy(input, adapter)
	if err != nil {
		t.Fatal(err)
	}
	if got.Status() != StatusTranslated || adapter.calls != 1 || adapter.input.SourceID != input.SourceID {
		t.Fatalf("translation did not preserve the fully bound input: got=%#v adapter=%#v", got, adapter)
	}
	adapter.input.Definition.Args[0] = "changed"
	if input.Definition.Args[0] != "-f" {
		t.Fatal("provider adapter mutated caller-owned legacy arguments")
	}
	declaration, ok := got.Declaration()
	if !ok || declaration.Factory().Reference != "postgres:17" || declaration.Transforms()[0].Reference != "db/schema.sql" {
		t.Fatalf("incomplete translation: %#v", declaration)
	}
}

func TestTranslateLegacyClassificationPrecedence(t *testing.T) {
	translated, _ := NewTranslatedResult(testRecipe(t))

	run := validTranslationInput(t)
	run.Definition.Class = legacyalias.ClassRun
	run.Definition.Image = ""
	run.EffectiveImage = ""
	run.EffectiveImageSource = ""
	adapter := &testAdapter{kind: "psql", result: translated}
	result, err := TranslateLegacy(run, adapter)
	if err != nil || result.Reason() != ReasonUnsupported || adapter.calls != 0 {
		t.Fatalf("run precedence = %#v, %v, calls=%d", result, err, adapter.calls)
	}

	input := validTranslationInput(t)
	result, err = TranslateLegacy(input, nil)
	if err != nil || result.Reason() != ReasonProviderUnavailable {
		t.Fatalf("nil provider classification = %#v, %v", result, err)
	}
	var typedNil *testAdapter
	result, err = TranslateLegacy(input, typedNil)
	if err != nil || result.Reason() != ReasonProviderUnavailable {
		t.Fatalf("typed-nil provider classification = %#v, %v", result, err)
	}

	input.Definition.Image = ""
	input.EffectiveImage = ""
	input.EffectiveImageSource = ""
	result, err = TranslateLegacy(input, &testAdapter{kind: "wrong", result: translated})
	if err != nil || result.Reason() != ReasonDefaultUnavailable {
		t.Fatalf("default precedence = %#v, %v", result, err)
	}

	input = validTranslationInput(t)
	if _, err := TranslateLegacy(input, &testAdapter{kind: "lb", result: translated}); err == nil {
		t.Fatal("kind mismatch unexpectedly succeeded")
	}
	if result, err := TranslateLegacy(input, valueAdapter{result: translated}); err != nil || result.Status() != StatusTranslated {
		t.Fatalf("non-pointer adapter rejected: %#v, %v", result, err)
	}
}

func TestTranslateLegacyValidatesPathsSourceAndImageBeforeAdapter(t *testing.T) {
	translated, _ := NewTranslatedResult(testRecipe(t))
	cases := map[string]func(*TranslationInput){
		"invalid class":        func(v *TranslationInput) { v.Definition.Class = "other" },
		"empty kind":           func(v *TranslationInput) { v.Definition.Kind = "" },
		"upper-case kind":      func(v *TranslationInput) { v.Definition.Kind = "PSQL" },
		"relative root":        func(v *TranslationInput) { v.WorkspaceRoot = "relative" },
		"relative alias":       func(v *TranslationInput) { v.AliasPath = "relative" },
		"equal root":           func(v *TranslationInput) { v.AliasPath = v.WorkspaceRoot; v.SourceID = "." },
		"outside alias":        func(v *TranslationInput) { v.AliasPath = filepath.Join(filepath.Dir(v.WorkspaceRoot), "outside.yaml") },
		"absolute source":      func(v *TranslationInput) { v.SourceID = filepath.ToSlash(v.AliasPath) },
		"mismatched source":    func(v *TranslationInput) { v.SourceID = "other.yaml" },
		"backslash source":     func(v *TranslationInput) { v.SourceID = `db\schema.prep.s9s.yaml` },
		"dot source":           func(v *TranslationInput) { v.SourceID = "db/../schema.prep.s9s.yaml" },
		"oversized source":     func(v *TranslationInput) { v.SourceID = strings.Repeat("x", MaxSourceIDBytes+1) },
		"invalid UTF-8 source": func(v *TranslationInput) { v.SourceID = string([]byte{0xff}) },
		"explicit mismatch":    func(v *TranslationInput) { v.EffectiveImage = "postgres:16" },
		"explicit provenance":  func(v *TranslationInput) { v.EffectiveImageSource = ImageSourceWorkspaceConfig },
		"image whitespace":     func(v *TranslationInput) { v.Definition.Image = " postgres:17" },
		"unknown provenance":   func(v *TranslationInput) { v.Definition.Image = ""; v.EffectiveImageSource = "unknown" },
		"empty provenance": func(v *TranslationInput) {
			v.Definition.Image = ""
			v.EffectiveImage = ""
			v.EffectiveImageSource = ImageSourceAlias
		},
		"run image": func(v *TranslationInput) {
			v.Definition.Class = legacyalias.ClassRun
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			input := validTranslationInput(t)
			mutate(&input)
			adapter := &testAdapter{kind: "psql", result: translated}
			if _, err := TranslateLegacy(input, adapter); err == nil {
				t.Fatal("invalid input unexpectedly succeeded")
			}
			if adapter.calls != 0 {
				t.Fatal("adapter called before input validation")
			}
		})
	}

	for _, source := range []ImageSource{ImageSourceWorkspaceConfig, ImageSourceGlobalConfig} {
		input := validTranslationInput(t)
		input.Definition.Image = ""
		input.EffectiveImageSource = source
		adapter := &testAdapter{kind: "psql", result: translated}
		if _, err := TranslateLegacy(input, adapter); err != nil || adapter.calls != 1 {
			t.Fatalf("valid inherited image source %q rejected: %v", source, err)
		}
	}
}

func TestSourceIDRejectsDotSegments(t *testing.T) {
	for _, value := range []string{".", "..", "../alias.yaml", "db//alias.yaml", "db/./alias.yaml", "db/../alias.yaml"} {
		if validSourceID(value) {
			t.Fatalf("invalid source ID %q accepted", value)
		}
	}
}

func TestTranslateLegacyRejectsHostileAdapterOutcomesAtomically(t *testing.T) {
	input := validTranslationInput(t)
	translated, _ := NewTranslatedResult(testRecipe(t))
	sentinel := errors.New("adapter failed")
	cases := []struct {
		name    string
		adapter *testAdapter
	}{
		{"zero result", &testAdapter{kind: "psql"}},
		{"result plus error", &testAdapter{kind: "psql", result: translated, err: sentinel}},
		{"invalid result", &testAdapter{kind: "psql", result: Result{status: StatusLegacyOnly, reason: ReasonUnsupported}}},
		{"mixed translated result", &testAdapter{kind: "psql", result: Result{status: StatusTranslated, declaration: testRecipe(t), reason: ReasonUnsupported, message: "mixed"}}},
		{"mixed legacy result", &testAdapter{kind: "psql", result: Result{status: StatusLegacyOnly, declaration: testRecipe(t), reason: ReasonUnsupported, message: "mixed"}}},
		{"unknown status", &testAdapter{kind: "psql", result: Result{status: "unknown"}}},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			got, err := TranslateLegacy(input, test.adapter)
			if err == nil {
				t.Fatal("hostile adapter outcome unexpectedly succeeded")
			}
			if got.Status() != "" {
				t.Fatalf("partial result leaked: %#v", got)
			}
		})
	}
}

func TestTranslateLegacyPreservesProviderLegacyOnlyClassification(t *testing.T) {
	input := validTranslationInput(t)
	want, err := NewLegacyOnlyResult(ReasonUnsupported, "remove the unsupported provider option")
	if err != nil {
		t.Fatal(err)
	}
	got, err := TranslateLegacy(input, &testAdapter{kind: "psql", result: want})
	if err != nil {
		t.Fatal(err)
	}
	if got.Status() != StatusLegacyOnly || got.Reason() != ReasonUnsupported || got.Message() != want.Message() {
		t.Fatalf("legacy-only provider result changed: %#v", got)
	}
}
