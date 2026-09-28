package composition

import (
	"bytes"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"
)

// T01-T03: generated traces use the exact occurrence-based canonical wire shape.
func TestExpansionTraceGoldensAndOccurrenceModel(t *testing.T) {
	transform := testTransform("main.sql")
	document := mustDocument(t,
		NamedAliasInput{Name: "main", Transform: &transform},
		NamedAliasInput{Name: "base", Recipe: factoryRecipe("postgres:17")},
		NamedAliasInput{Name: "target", Recipe: prefixRecipe("base", namedTransformStep("main"), inlineTransformStep("inline.sql"), namedTransformStep("main"))},
	)
	catalog := mustCatalog(t, SourceDocument{SourceID: "db/aliases.yaml", Document: document})
	result, err := catalog.ExpandRecipe("target")
	if err != nil {
		t.Fatal(err)
	}
	trace := result.Trace()
	nodes := trace.Nodes()
	if trace.TargetNodeID() != 0 || len(nodes) != 4 {
		t.Fatalf("target/nodes = %d/%d", trace.TargetNodeID(), len(nodes))
	}
	if nodes[0].ID() != 0 || nodes[0].Alias() != "target" || nodes[0].Pointer() != "/aliases/target" {
		t.Fatalf("target node = %#v", nodes[0])
	}
	if parent, ok := nodes[1].ParentID(); !ok || parent != 0 || nodes[1].ReferencePointer() != "/aliases/target/base/recipe" {
		t.Fatalf("base node = %#v", nodes[1])
	}
	if parent, ok := nodes[2].ParentID(); !ok || parent != 0 || nodes[2].ReferencePointer() != "/aliases/target/steps/0/use" {
		t.Fatalf("first named node = %#v", nodes[2])
	}
	if nodes[3].ReferencePointer() != "/aliases/target/steps/2/use" {
		t.Fatalf("repeated occurrence pointer = %q", nodes[3].ReferencePointer())
	}
	factory, ok := trace.Factory()
	if !ok || factory.NodeID() != 1 || factory.Pointer() != "/aliases/base/base/factory" {
		t.Fatalf("factory origin = %#v, %v", factory, ok)
	}
	origins := trace.Transforms()
	if len(origins) != 3 || origins[0].NodeID() != 2 || origins[1].NodeID() != 0 || origins[2].NodeID() != 3 {
		t.Fatalf("origins = %#v", origins)
	}
	for index := range origins {
		if origins[index].OutputIndex() != index {
			t.Fatalf("origin[%d] output index = %d", index, origins[index].OutputIndex())
		}
	}

	standalone, err := catalog.ExpandTransform("main")
	if err != nil {
		t.Fatal(err)
	}
	standaloneTrace := standalone.Trace()
	if len(standaloneTrace.Nodes()) != 1 || len(standaloneTrace.Transforms()) != 1 {
		t.Fatalf("standalone trace = %s", mustJSON(t, standaloneTrace))
	}
	if _, ok := standaloneTrace.Factory(); ok {
		t.Fatal("standalone trace has factory")
	}
}

// T04-T06: decoding validates the complete graph, origin, and canonical pointer grammar.
func TestExpansionTraceStrictDecodeAndGraphIntegrity(t *testing.T) {
	valid := recipeTraceJSON("source.yaml")
	var target ExpansionTrace
	if err := DecodeExpansionTraceJSON([]byte(valid), &target); err != nil {
		t.Fatalf("valid trace rejected: %v", err)
	}
	before := append([]byte(nil), mustJSON(t, target)...)

	mutations := []string{
		strings.Replace(valid, `"target_node":0`, `"target_node":1`, 1),
		strings.Replace(valid, `"node_id":1`, `"node_id":2`, 1),
		strings.Replace(valid, `,"parent_node_id":0`, ``, 1),
		strings.Replace(valid, `"parent_node_id":0`, `"parent_node_id":9`, 1),
		strings.Replace(valid, `"kind":"recipe"`, `"kind":"transform"`, 1),
		strings.Replace(valid, `"node_id":1,"pointer":"/aliases/base/base/factory"`, `"node_id":0,"pointer":"/aliases/base/base/factory"`, 1),
		strings.Replace(valid, `"output_index":0`, `"output_index":1`, 1),
		strings.Replace(valid, `/aliases/target/steps/0/use`, `/aliases/target/steps/1/use`, 1),
		strings.Replace(valid, `/aliases/main/declaration`, `/aliases/main/~1declaration`, 1),
		strings.Replace(valid, `"alias":"main"`, `"alias":"BAD"`, 1),
		strings.Replace(valid, `"source_id":"source.yaml"`, `"source_id":""`, 1),
		strings.Replace(valid, `}]}`, `}],"extra":true}`, 1),
		valid + `{}`,
	}
	for index, raw := range mutations {
		if raw == valid {
			t.Fatalf("mutation %d did not modify fixture", index)
		}
		if err := DecodeExpansionTraceJSON([]byte(raw), &target); err == nil {
			t.Fatalf("mutation %d accepted: %s", index, raw)
		}
		if !bytes.Equal(before, mustJSON(t, target)) {
			t.Fatalf("mutation %d changed receiver", index)
		}
	}
	if err := DecodeExpansionTraceJSON([]byte(valid), nil); err == nil {
		t.Fatal("nil target accepted")
	}
}

// T07: accessors are defensive and absence is represented explicitly.
func TestExpansionTraceAccessorsAreDefensive(t *testing.T) {
	var trace ExpansionTrace
	if err := DecodeExpansionTraceJSON([]byte(recipeTraceJSON("source.yaml")), &trace); err != nil {
		t.Fatal(err)
	}
	nodes := trace.Nodes()
	nodes[0] = TraceNode{}
	if trace.Nodes()[0].Alias() != "target" {
		t.Fatal("node accessor retained mutation")
	}
	origins := trace.Transforms()
	origins[0] = TransformOrigin{}
	if trace.Transforms()[0].Pointer() != "/aliases/main/declaration" {
		t.Fatal("origin accessor retained mutation")
	}
	if parent, ok := trace.Nodes()[0].ParentID(); ok || parent != 0 {
		t.Fatalf("target parent = %d, %v", parent, ok)
	}
}

// T08: count and raw transport limits are enforced before accepting a trace.
func TestExpansionTraceBounds(t *testing.T) {
	oversized := append([]byte(recipeTraceJSON("source.yaml")), bytes.Repeat([]byte(" "), MaxTraceJSONBytes)...)
	var trace ExpansionTrace
	if err := DecodeExpansionTraceJSON(oversized, &trace); err == nil {
		t.Fatal("oversized trace accepted")
	} else {
		assertCompositionError(t, err, CodeExpansionTooLarge)
	}

	tooManyNodes := strings.Replace(recipeTraceJSON("source.yaml"), `"nodes":[`, `"nodes":[`+strings.Repeat(`{"node_id":0},`, MaxTraceNodes+1), 1)
	if err := DecodeExpansionTraceJSON([]byte(tooManyNodes), &trace); err == nil {
		t.Fatal("node count overflow accepted")
	}
}

// T09, T10: the error envelope is stable and diagnostic contexts are bounded copies.
func TestCompositionErrorEnvelopeAndContextBounds(t *testing.T) {
	document := mustDocument(t,
		NamedAliasInput{Name: "a", Recipe: prefixRecipe("b")},
		NamedAliasInput{Name: "b", Recipe: prefixRecipe("a")},
	)
	catalog := mustCatalog(t, SourceDocument{SourceID: "aliases.yaml", Document: document})
	_, err := catalog.ExpandRecipe("a")
	problem := assertCompositionError(t, err, CodeCycle)
	if len(problem.Cycle()) != 3 || problem.Candidates() != nil {
		t.Fatalf("error contexts = cycle %#v, candidates %#v", problem.Cycle(), problem.Candidates())
	}
	if !errors.Is(problem, ErrInvalid) {
		t.Fatal("*Error does not unwrap ErrInvalid")
	}
}

// T11: the first error follows target/base/deepest-to-target traversal.
func TestExpansionErrorPrecedence(t *testing.T) {
	document := mustDocument(t,
		NamedAliasInput{Name: "cycle-a", Recipe: prefixRecipe("cycle-b", namedTransformStep("missing-late"))},
		NamedAliasInput{Name: "cycle-b", Recipe: prefixRecipe("cycle-a")},
		NamedAliasInput{Name: "deep", Recipe: factoryRecipe("postgres:17", namedTransformStep("missing-deep"))},
		NamedAliasInput{Name: "target", Recipe: prefixRecipe("deep", namedTransformStep("missing-target"))},
	)
	catalog := mustCatalog(t, SourceDocument{SourceID: "aliases.yaml", Document: document})
	if _, err := catalog.ExpandRecipe("cycle-a"); err == nil {
		t.Fatal("cycle expansion succeeded")
	} else {
		assertCompositionError(t, err, CodeCycle)
	}
	if _, err := catalog.ExpandRecipe("target"); err == nil {
		t.Fatal("missing-reference expansion succeeded")
	} else if got := assertCompositionError(t, err, CodeMissingReference).Pointer; got != "/aliases/deep/steps/0/use" {
		t.Fatalf("first missing pointer = %q", got)
	}
}

// T12, T13: rendered errors disclose no declaration payload and remain bounded UTF-8.
func TestCompositionErrorDisclosureAndTextBounds(t *testing.T) {
	secret := "secret-declaration-payload"
	invalid := testTransform(secret)
	invalid.Kind = "BAD"
	_, err := NewAliasDocument(DocumentInput{Aliases: []NamedAliasInput{{Name: "seed", Transform: &invalid}}})
	problem := assertCompositionError(t, err, CodeInvalidDocument)
	text := problem.Error()
	if strings.Contains(text, secret) || len(text) > MaxErrorTextBytes || !utf8.ValidString(text) {
		t.Fatalf("unsafe error text: %q", text)
	}
	if !strings.Contains(text, string(CodeInvalidDocument)) || !strings.Contains(text, "/aliases/seed/declaration") {
		t.Fatalf("error lacks stable location: %q", text)
	}

	overlongPointer := strings.Replace(recipeTraceJSON("source.yaml"), "/aliases/main", "/aliases/"+strings.Repeat("x", MaxPointerBytes), 1)
	var trace ExpansionTrace
	if err := DecodeExpansionTraceJSON([]byte(overlongPointer), &trace); err == nil {
		t.Fatal("overlong pointer accepted")
	}
}

func recipeTraceJSON(sourceID string) string {
	value := map[string]any{
		"schema_version": ExpansionTraceSchemaVersion,
		"target_node":    0,
		"nodes": []any{
			map[string]any{"node_id": 0, "kind": "recipe", "alias": "target", "source_id": sourceID, "pointer": "/aliases/target"},
			map[string]any{"node_id": 1, "parent_node_id": 0, "reference_pointer": "/aliases/target/base/recipe", "kind": "recipe", "alias": "base", "source_id": sourceID, "pointer": "/aliases/base"},
			map[string]any{"node_id": 2, "parent_node_id": 0, "reference_pointer": "/aliases/target/steps/0/use", "kind": "transform", "alias": "main", "source_id": sourceID, "pointer": "/aliases/main"},
		},
		"factory": map[string]any{"node_id": 1, "pointer": "/aliases/base/base/factory"},
		"transforms": []any{
			map[string]any{"output_index": 0, "node_id": 2, "pointer": "/aliases/main/declaration"},
		},
	}
	raw, err := json.Marshal(value)
	if err != nil {
		panic(err)
	}
	return string(raw)
}
