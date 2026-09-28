package composition

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
)

// D08, E14, T07, T09: zero values expose no partial state, and every stable
// error-location combination remains useful without optional context.
func TestZeroValueAccessorsAndErrorRendering(t *testing.T) {
	var nilProblem *Error
	if nilProblem.Error() != ErrInvalid.Error() || len(nilProblem.Unwrap()) != 1 || nilProblem.Cycle() != nil || nilProblem.Candidates() != nil {
		t.Fatal("nil composition error did not expose its stable empty contract")
	}
	for _, test := range []struct {
		problem *Error
		want    string
	}{
		{&Error{Code: CodeInvalidDocument}, "runtime v2 composition invalid: invalid_document"},
		{&Error{Code: CodeInvalidDocument, SourceID: "source.json"}, "runtime v2 composition invalid: invalid_document at source.json"},
		{&Error{Code: CodeInvalidDocument, Pointer: "/aliases/a"}, "runtime v2 composition invalid: invalid_document at /aliases/a"},
		{&Error{Code: CodeInvalidDocument, SourceID: "source.json", Pointer: "/aliases/a"}, "runtime v2 composition invalid: invalid_document at source.json:/aliases/a"},
	} {
		if got := test.problem.Error(); got != test.want {
			t.Fatalf("Error() = %q, want %q", got, test.want)
		}
		if !errors.Is(test.problem, ErrInvalid) || test.problem.Cycle() != nil || test.problem.Candidates() != nil {
			t.Fatalf("incomplete error envelope: %#v", test.problem)
		}
	}

	var trace ExpansionTrace
	if trace.TargetNodeID() != 0 || trace.Nodes() != nil || trace.Transforms() != nil {
		t.Fatal("zero trace exposed nodes")
	}
	if _, ok := trace.Factory(); ok {
		t.Fatal("zero trace exposed a factory")
	}
	var node TraceNode
	if node.ID() != 0 || node.ReferencePointer() != "" || node.Kind() != "" || node.Alias() != "" || node.SourceID() != "" || node.Pointer() != "" {
		t.Fatal("zero trace node exposed data")
	}
	if parent, ok := node.ParentID(); parent != 0 || ok {
		t.Fatal("zero trace node exposed a parent")
	}
	var factory FactoryOrigin
	if factory.NodeID() != 0 || factory.Pointer() != "" {
		t.Fatal("zero factory origin exposed data")
	}
	var transform TransformOrigin
	if transform.OutputIndex() != 0 || transform.NodeID() != 0 || transform.Pointer() != "" {
		t.Fatal("zero transform origin exposed data")
	}
	var recipe ExpandedRecipe
	if recipe.Declaration().Factory().Kind != "" || recipe.Trace().Nodes() != nil {
		t.Fatal("zero expanded recipe exposed partial data")
	}
	var standalone ExpandedTransform
	if standalone.Declaration().Declaration().Kind != "" || standalone.Trace().Nodes() != nil {
		t.Fatal("zero expanded transform exposed partial data")
	}
}

// D02, D03, D05, D09, D10: direct encoding/json use has the same strict,
// atomic union validation as the explicit decoder and constructor.
func TestAliasDocumentDirectUnmarshalAndRejectionMatrix(t *testing.T) {
	valid := []byte(`{"schema_version":"sqlrs.runtime.v2.aliases.v1","aliases":{"seed":{"type":"transform","declaration":{"kind":"psql","reference":"seed.sql","arguments":[],"attributes":{}}}}}`)
	var direct AliasDocument
	if err := json.Unmarshal(valid, &direct); err != nil {
		t.Fatalf("direct Unmarshal: %v", err)
	}
	before := string(mustJSON(t, direct))
	factory := `{"kind":"postgres","reference":"postgres:17","arguments":[],"attributes":{}}`
	transform := `{"kind":"psql","reference":"seed.sql","arguments":[],"attributes":{}}`
	cases := []string{
		`{}`,
		`{"schema_version":"sqlrs.runtime.v2.aliases.v1"}`,
		`{"schema_version":"sqlrs.runtime.v2.aliases.v1","aliases":{"seed":null}}`,
		`{"schema_version":"sqlrs.runtime.v2.aliases.v1","aliases":{"seed":[]}}`,
		`{"schema_version":"sqlrs.runtime.v2.aliases.v1","aliases":{"seed":{}}}`,
		`{"schema_version":"sqlrs.runtime.v2.aliases.v1","aliases":{"seed":{"type":"unknown"}}}`,
		`{"schema_version":"sqlrs.runtime.v2.aliases.v1","aliases":{"seed":{"type":"transform"}}}`,
		`{"schema_version":"sqlrs.runtime.v2.aliases.v1","aliases":{"seed":{"type":"transform","declaration":null}}}`,
		`{"schema_version":"sqlrs.runtime.v2.aliases.v1","aliases":{"seed":{"type":"transform","declaration":` + transform + `,"extra":true}}}`,
		`{"schema_version":"sqlrs.runtime.v2.aliases.v1","aliases":{"base":{"type":"recipe","steps":[]}}}`,
		`{"schema_version":"sqlrs.runtime.v2.aliases.v1","aliases":{"base":{"type":"recipe","base":{"factory":` + factory + `}}}}`,
		`{"schema_version":"sqlrs.runtime.v2.aliases.v1","aliases":{"base":{"type":"recipe","base":{"factory":` + factory + `,"recipe":"other"},"steps":[]}}}`,
		`{"schema_version":"sqlrs.runtime.v2.aliases.v1","aliases":{"base":{"type":"recipe","base":{"factory":` + factory + `},"steps":[{}]}}}`,
		`{"schema_version":"sqlrs.runtime.v2.aliases.v1","aliases":{"base":{"type":"recipe","base":{"factory":` + factory + `},"steps":[{"use":"seed","transform":` + transform + `}]}}}`,
		`{"schema_version":"sqlrs.runtime.v2.aliases.v1","aliases":{"base":{"type":"recipe","base":{"factory":{"kind":"BAD","reference":"x","arguments":[],"attributes":{}}},"steps":[]}}}`,
		`{"schema_version":"sqlrs.runtime.v2.aliases.v1","aliases":{"base":{"type":"recipe","base":{"factory":` + factory + `},"steps":[{"transform":{"kind":"BAD","reference":"x","arguments":[],"attributes":{}}}]}}}`,
	}
	for index, raw := range cases {
		if err := json.Unmarshal([]byte(raw), &direct); err == nil {
			t.Fatalf("case %d unexpectedly succeeded: %s", index, raw)
		}
		if got := string(mustJSON(t, direct)); got != before {
			t.Fatalf("case %d changed receiver", index)
		}
	}

	badFactory := testFactory("postgres:17")
	badFactory.Kind = "BAD"
	if _, err := NewAliasDocument(DocumentInput{Aliases: []NamedAliasInput{{Name: "base", Recipe: &RecipeAliasInput{Base: RecipeBaseInput{Factory: &badFactory}}}}}); err == nil {
		t.Fatal("invalid inline factory accepted")
	}
	badTransform := testTransform("bad.sql")
	badTransform.Kind = "BAD"
	if _, err := NewAliasDocument(DocumentInput{Aliases: []NamedAliasInput{{Name: "base", Recipe: factoryRecipe("postgres:17", RecipeStepInput{Transform: &badTransform})}}}); err == nil {
		t.Fatal("invalid inline transform accepted")
	}
	tooManySteps := make([]RecipeStepInput, runtimev2.MaxTransforms+1)
	for index := range tooManySteps {
		tooManySteps[index] = namedTransformStep("seed")
	}
	if _, err := NewAliasDocument(DocumentInput{Aliases: []NamedAliasInput{{Name: "base", Recipe: factoryRecipe("postgres:17", tooManySteps...)}}}); err == nil {
		t.Fatal("too many recipe steps accepted")
	} else {
		assertCompositionError(t, err, CodeExpansionTooLarge)
	}
}

// D09 and C06: serialized documents, aggregate definitions, and checked
// arithmetic reject their first value above the documented bounds.
func TestDocumentAndCatalogRemainingBounds(t *testing.T) {
	large := maximalTransform("large.sql")
	aliases := make([]NamedAliasInput, 5)
	for index := range aliases {
		aliases[index] = NamedAliasInput{Name: fmt.Sprintf("large-%d", index), Transform: &large}
	}
	if _, err := NewAliasDocument(DocumentInput{Aliases: aliases}); err == nil {
		t.Fatal("oversized canonical document accepted")
	} else {
		assertCompositionError(t, err, CodeExpansionTooLarge)
	}

	valid := mustDocument(t)
	if _, err := NewCatalog([]SourceDocument{{SourceID: "zero.json", Document: AliasDocument{}}, {SourceID: "valid.json", Document: valid}}); err == nil {
		t.Fatal("zero document accepted by catalog")
	}

	declaration := testTransform("x")
	leftAliases := make([]NamedAliasInput, MaxAliases/2+1)
	rightAliases := make([]NamedAliasInput, MaxAliases/2)
	for index := range leftAliases {
		leftAliases[index] = NamedAliasInput{Name: "l" + indexedName(index), Transform: &declaration}
	}
	for index := range rightAliases {
		rightAliases[index] = NamedAliasInput{Name: "r" + indexedName(index), Transform: &declaration}
	}
	left := mustDocument(t, leftAliases...)
	right := mustDocument(t, rightAliases...)
	if _, err := NewCatalog([]SourceDocument{{SourceID: "left.json", Document: left}, {SourceID: "right.json", Document: right}}); err == nil {
		t.Fatal("aggregate definition overflow accepted")
	} else {
		assertCompositionError(t, err, CodeExpansionTooLarge)
	}

	if _, ok := checkedAdd(-1, 0, 1); ok {
		t.Fatal("negative checked-add operand accepted")
	}
	if _, ok := checkedAdd(0, -1, 1); ok {
		t.Fatal("negative checked-add operand accepted")
	}
}

// E01, E06, E07, E10: transform expansion mirrors recipe target handling,
// wrong step kinds fail at the edge, and both semantic output limits apply.
func TestExpansionRemainingErrorsAndLimits(t *testing.T) {
	transform := testTransform("seed.sql")
	document := mustDocument(t,
		NamedAliasInput{Name: "seed", Transform: &transform},
		NamedAliasInput{Name: "base", Recipe: factoryRecipe("postgres:17")},
		NamedAliasInput{Name: "wrong-step", Recipe: factoryRecipe("postgres:17", namedTransformStep("base"))},
	)
	catalog := mustCatalog(t, SourceDocument{SourceID: "aliases.json", Document: document})
	for _, test := range []struct {
		name string
		from Catalog
		code ErrorCode
	}{
		{"zero catalog", Catalog{}, CodeInvalidDocument},
		{"invalid name", catalog, CodeInvalidDocument},
		{"missing", catalog, CodeMissingReference},
	} {
		target := map[string]string{"zero catalog": "seed", "invalid name": "BAD", "missing": "missing"}[test.name]
		if _, err := test.from.ExpandTransform(target); err == nil {
			t.Fatalf("%s unexpectedly succeeded", test.name)
		} else {
			assertCompositionError(t, err, test.code)
		}
	}
	if _, err := catalog.ExpandTransform("base"); err == nil {
		t.Fatal("recipe expanded as a transform")
	} else {
		assertCompositionError(t, err, CodeWrongReferenceKind)
	}
	if _, err := catalog.ExpandRecipe("wrong-step"); err == nil {
		t.Fatal("recipe step accepted a recipe alias")
	} else if problem := assertCompositionError(t, err, CodeWrongReferenceKind); problem.Pointer != "/aliases/wrong-step/steps/0/use" {
		t.Fatalf("wrong-step pointer = %q", problem.Pointer)
	}

	namedBase := make([]RecipeStepInput, runtimev2.MaxTransforms/2+1)
	namedTarget := make([]RecipeStepInput, runtimev2.MaxTransforms/2)
	for index := range namedBase {
		namedBase[index] = namedTransformStep("seed")
	}
	for index := range namedTarget {
		namedTarget[index] = namedTransformStep("seed")
	}
	countDocument := mustDocument(t,
		NamedAliasInput{Name: "seed", Transform: &transform},
		NamedAliasInput{Name: "large-base", Recipe: factoryRecipe("postgres:17", namedBase...)},
		NamedAliasInput{Name: "large-target", Recipe: prefixRecipe("large-base", namedTarget...)},
	)
	if _, err := mustCatalog(t, SourceDocument{SourceID: "count.json", Document: countDocument}).ExpandRecipe("large-target"); err == nil {
		t.Fatal("named transform output overflow accepted")
	} else {
		assertCompositionError(t, err, CodeExpansionTooLarge)
	}

	large := maximalTransform("large.sql")
	base := mustDocument(t, NamedAliasInput{Name: "large-base", Recipe: factoryRecipe("postgres:17",
		RecipeStepInput{Transform: &large}, RecipeStepInput{Transform: &large}, RecipeStepInput{Transform: &large})})
	target := mustDocument(t, NamedAliasInput{Name: "large-target", Recipe: prefixRecipe("large-base",
		RecipeStepInput{Transform: &large}, RecipeStepInput{Transform: &large})})
	largeCatalog := mustCatalog(t,
		SourceDocument{SourceID: "base.json", Document: base},
		SourceDocument{SourceID: "target.json", Document: target},
	)
	if _, err := largeCatalog.ExpandRecipe("large-target"); err == nil {
		t.Fatal("oversized expanded declaration accepted")
	} else {
		assertCompositionError(t, err, CodeExpansionTooLarge)
	}
}

// T04-T08: every required trace field and graph relationship is validated,
// including direct encoding/json decoding and canonical step indices.
func TestTraceHostileWireMatrix(t *testing.T) {
	valid := []byte(recipeTraceJSON("source.json"))
	var direct ExpansionTrace
	if err := json.Unmarshal(valid, &direct); err != nil {
		t.Fatalf("direct trace Unmarshal: %v", err)
	}
	before := string(mustJSON(t, direct))
	if err := json.Unmarshal([]byte(`{}`), &direct); err == nil {
		t.Fatal("empty trace accepted")
	}
	if got := string(mustJSON(t, direct)); got != before {
		t.Fatal("failed direct trace decode changed receiver")
	}

	mutations := []func(*traceWire){
		func(w *traceWire) { w.SchemaVersion = nil },
		func(w *traceWire) { w.Nodes = pointerTo(make([]traceNodeWire, MaxTraceNodes+1)) },
		func(w *traceWire) { w.Transforms = pointerTo(make([]transformOriginWire, runtimev2.MaxTransforms+1)) },
		func(w *traceWire) { (*w.Nodes)[0].NodeID = nil },
		func(w *traceWire) { *(*w.Nodes)[0].NodeID = 4 },
		func(w *traceWire) { *(*w.Nodes)[0].Alias = "BAD" },
		func(w *traceWire) { *(*w.Nodes)[0].SourceID = "" },
		func(w *traceWire) { *(*w.Nodes)[0].Pointer = string([]byte{0xff}) },
		func(w *traceWire) { (*w.Nodes)[1].ParentNodeID = nil },
		func(w *traceWire) { *(*w.Nodes)[1].ReferencePointer = string([]byte{0xff}) },
		func(w *traceWire) { w.Factory.NodeID = nil },
		func(w *traceWire) { *w.Factory.Pointer = string([]byte{0xff}) },
		func(w *traceWire) { (*w.Transforms)[0].OutputIndex = nil },
		func(w *traceWire) { *(*w.Transforms)[0].OutputIndex = 2 },
		func(w *traceWire) { *(*w.Transforms)[0].Pointer = string([]byte{0xff}) },
	}
	for index, mutate := range mutations {
		wire := validTraceWire(t)
		mutate(&wire)
		if _, err := traceFromWire(wire); err == nil {
			t.Fatalf("hostile wire %d accepted", index)
		}
	}

	invalidData := []*traceData{
		{},
		{nodes: []traceNodeData{{id: 0, kind: AliasKindRecipe, alias: "a", sourceID: "s", pointer: "/aliases/wrong"}}},
		{nodes: []traceNodeData{{id: 0, hasParent: true, kind: AliasKindRecipe, alias: "a", sourceID: "s", pointer: "/aliases/a"}}},
		{nodes: []traceNodeData{{id: 0, kind: AliasKindRecipe, alias: "a", sourceID: "s", pointer: "/aliases/a"}, {id: 1, kind: AliasKindRecipe, alias: "b", sourceID: "s", pointer: "/aliases/b"}}},
		{nodes: []traceNodeData{{id: 0, kind: AliasKind("unknown"), alias: "a", sourceID: "s", pointer: "/aliases/a"}}},
		{nodes: []traceNodeData{{id: 0, kind: AliasKindTransform, alias: "a", sourceID: "s", pointer: "/aliases/a"}}},
		{nodes: []traceNodeData{{id: 0, kind: AliasKindTransform, alias: "a", sourceID: "s", pointer: "/aliases/a"}}, transforms: []originData{{nodeID: 0, pointer: "/bad"}}},
		{nodes: []traceNodeData{{id: 0, kind: AliasKindRecipe, alias: "a", sourceID: "s", pointer: "/aliases/a"}, {id: 1, hasParent: true, parentID: 0, referencePointer: "/bad", kind: AliasKindRecipe, alias: "b", sourceID: "s", pointer: "/aliases/b"}}, factory: &originData{nodeID: 1, pointer: "/aliases/b/base/factory"}},
		{nodes: []traceNodeData{{id: 0, kind: AliasKindRecipe, alias: "a", sourceID: "s", pointer: "/aliases/a"}}, factory: &originData{nodeID: 1, pointer: "/aliases/a/base/factory"}},
		{nodes: []traceNodeData{{id: 0, kind: AliasKindRecipe, alias: "a", sourceID: "s", pointer: "/aliases/a"}, {id: 1, hasParent: true, parentID: 0, referencePointer: "/aliases/a/steps/0/use", kind: AliasKindTransform, alias: "x", sourceID: "s", pointer: "/aliases/x"}}, factory: &originData{nodeID: 0, pointer: "/aliases/a/base/factory"}},
		{nodes: []traceNodeData{{id: 0, kind: AliasKindRecipe, alias: "a", sourceID: "s", pointer: "/aliases/a"}}, factory: &originData{nodeID: 0, pointer: "/aliases/a/base/factory"}, transforms: []originData{{nodeID: 3, pointer: "/bad"}}},
		{nodes: []traceNodeData{{id: 0, kind: AliasKindRecipe, alias: "a", sourceID: "s", pointer: "/aliases/a"}, {id: 1, hasParent: true, parentID: 0, referencePointer: "/aliases/a/steps/01/use", kind: AliasKindTransform, alias: "x", sourceID: "s", pointer: "/aliases/x"}}, factory: &originData{nodeID: 0, pointer: "/aliases/a/base/factory"}, transforms: []originData{{nodeID: 1, pointer: "/aliases/x/declaration"}}},
		{nodes: []traceNodeData{{id: 0, kind: AliasKindRecipe, alias: "a", sourceID: "s", pointer: "/aliases/a"}}, factory: &originData{nodeID: 0, pointer: "/aliases/a/base/factory"}, transforms: []originData{{nodeID: 0, pointer: "/aliases/a/steps/2/transform"}}},
	}
	for index, data := range invalidData {
		if err := validateTraceData(data); err == nil {
			t.Fatalf("hostile graph %d accepted", index)
		}
	}
	if _, err := newTrace(traceData{nodes: make([]traceNodeData, MaxTraceNodes+1)}); err == nil {
		t.Fatal("newTrace accepted too many nodes")
	}

	for _, value := range []string{
		"/other/a/steps/0/use",
		"/aliases/a/steps//use",
		"/aliases/a/steps/01/use",
		"/aliases/a/steps/x/use",
		fmt.Sprintf("/aliases/a/steps/%d/use", runtimev2.MaxTransforms+1),
	} {
		if _, ok := parseStepPointer(value, "a", "use"); ok {
			t.Fatalf("invalid step pointer accepted: %q", value)
		}
	}
}

// D03 and T04: malformed or incomplete JSON is rejected at every scanner
// boundary before a partially decoded public value can escape.
func TestStrictJSONScannerMalformedInputs(t *testing.T) {
	for _, raw := range [][]byte{
		nil,
		[]byte(`{`),
		[]byte(`{"a":[`),
		[]byte(`{"a":1`),
		[]byte(`1 2`),
	} {
		if err := rejectDuplicateMembers(raw); err == nil {
			t.Fatalf("malformed JSON accepted: %q", raw)
		}
	}
}

func maximalTransform(reference string) runtimev2.TransformDeclaration {
	value := testTransform(reference)
	value.Attributes = make(map[string]string, runtimev2.MaxAttributes)
	for index := 0; index < runtimev2.MaxAttributes; index++ {
		value.Attributes[indexedName(index)] = strings.Repeat("v", runtimev2.MaxResolvedValueBytes)
	}
	return value
}

func validTraceWire(t testing.TB) traceWire {
	t.Helper()
	var wire traceWire
	if err := json.Unmarshal([]byte(recipeTraceJSON("source.json")), &wire); err != nil {
		t.Fatalf("trace fixture: %v", err)
	}
	return wire
}

func pointerTo[T any](value T) *T { return &value }
