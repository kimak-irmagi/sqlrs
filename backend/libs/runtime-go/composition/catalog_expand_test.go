package composition

import (
	"bytes"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
)

// C01, C02: empty catalogs and bounded opaque source identifiers have explicit behavior.
func TestCatalogEmptyAndSourceIDValidation(t *testing.T) {
	empty, err := NewCatalog(nil)
	if err != nil {
		t.Fatalf("NewCatalog(nil): %v", err)
	}
	assertCompositionError(t, expansionError(empty.ExpandRecipe("missing")), CodeMissingReference)

	document := mustDocument(t, NamedAliasInput{Name: "seed", Transform: ptrTransform(testTransform("seed.sql"))})
	validIDs := []string{"aliases.yaml", "каталог/алиасы.yaml", strings.Repeat("x", MaxSourceIDBytes)}
	for _, id := range validIDs {
		if _, err := NewCatalog([]SourceDocument{{SourceID: id, Document: document}}); err != nil {
			t.Fatalf("valid source ID %q rejected: %v", id, err)
		}
	}
	invalidIDs := []string{"", string([]byte{0xff}), strings.Repeat("x", MaxSourceIDBytes+1)}
	for _, id := range invalidIDs {
		if _, err := NewCatalog([]SourceDocument{{SourceID: id, Document: document}}); err == nil {
			t.Fatalf("invalid source ID %q accepted", id)
		} else {
			assertCompositionError(t, err, CodeInvalidDocument)
		}
	}
	if _, err := NewCatalog([]SourceDocument{{SourceID: "same", Document: document}, {SourceID: "same", Document: document}}); err == nil {
		t.Fatal("duplicate source IDs accepted")
	}
}

// C03-C05: catalog failures and results are independent of caller slice order.
func TestCatalogPermutationDeterminismAndAmbiguity(t *testing.T) {
	factory := mustDocument(t, NamedAliasInput{Name: "base", Recipe: factoryRecipe("postgres:17")})
	transform := mustDocument(t, NamedAliasInput{Name: "seed", Transform: ptrTransform(testTransform("seed.sql"))})
	target := mustDocument(t, NamedAliasInput{Name: "target", Recipe: prefixRecipe("base", namedTransformStep("seed"))})
	sources := []SourceDocument{{SourceID: "z.yaml", Document: target}, {SourceID: "a.yaml", Document: factory}, {SourceID: "m.yaml", Document: transform}}
	var baselineRecipe, baselineTrace []byte
	for _, permutation := range permutations(sources) {
		catalog := mustCatalog(t, permutation...)
		expanded, err := catalog.ExpandRecipe("target")
		if err != nil {
			t.Fatalf("ExpandRecipe: %v", err)
		}
		recipe, trace := mustJSON(t, expanded.Declaration()), mustJSON(t, expanded.Trace())
		if baselineRecipe == nil {
			baselineRecipe, baselineTrace = recipe, trace
			continue
		}
		if !bytes.Equal(recipe, baselineRecipe) || !bytes.Equal(trace, baselineTrace) {
			t.Fatal("source permutation changed expansion")
		}
	}

	duplicateA := mustDocument(t, NamedAliasInput{Name: "same", Transform: ptrTransform(testTransform("a.sql"))})
	duplicateB := mustDocument(t, NamedAliasInput{Name: "same", Transform: ptrTransform(testTransform("b.sql"))})
	for _, order := range [][]SourceDocument{
		{{SourceID: "z.yaml", Document: duplicateA}, {SourceID: "a.yaml", Document: duplicateB}},
		{{SourceID: "a.yaml", Document: duplicateB}, {SourceID: "z.yaml", Document: duplicateA}},
	} {
		_, err := NewCatalog(order)
		problem := assertCompositionError(t, err, CodeAmbiguousReference)
		candidates := problem.Candidates()
		if len(candidates) != 2 || candidates[0].SourceID != "a.yaml" || candidates[1].SourceID != "z.yaml" {
			t.Fatalf("candidates = %#v", candidates)
		}
		candidates[0].SourceID = "changed"
		if problem.Candidates()[0].SourceID != "a.yaml" {
			t.Fatal("candidate accessor retained caller mutation")
		}
	}
}

// C05: catalog validation precedence is canonical rather than input-order based.
func TestCatalogValidationPrecedence(t *testing.T) {
	valid := mustDocument(t)
	tooMany := make([]SourceDocument, MaxSourceDocuments+1)
	for index := range tooMany {
		tooMany[index] = SourceDocument{SourceID: indexedName(index), Document: valid}
	}
	tooMany[0].SourceID = ""
	if _, err := NewCatalog(tooMany); err == nil {
		t.Fatal("over-count catalog accepted")
	} else {
		assertCompositionError(t, err, CodeExpansionTooLarge)
	}

	_, err := NewCatalog([]SourceDocument{
		{SourceID: "z", Document: AliasDocument{}},
		{SourceID: "a", Document: valid},
		{SourceID: "", Document: valid},
	})
	problem := assertCompositionError(t, err, CodeInvalidDocument)
	if problem.SourceID != "" {
		t.Fatalf("primary source = %q", problem.SourceID)
	}
}

// C06: source count, definition count, and aggregate byte limits reject atomically.
func TestCatalogAggregateBounds(t *testing.T) {
	empty := mustDocument(t)
	sources := make([]SourceDocument, MaxSourceDocuments)
	for index := range sources {
		sources[index] = SourceDocument{SourceID: indexedName(index), Document: empty}
	}
	if _, err := NewCatalog(sources); err != nil {
		t.Fatalf("MaxSourceDocuments rejected: %v", err)
	}
	sources = append(sources, SourceDocument{SourceID: "overflow", Document: empty})
	if _, err := NewCatalog(sources); err == nil {
		t.Fatal("MaxSourceDocuments+1 accepted")
	}

	largeTransform := testTransform(strings.Repeat("x", 4096))
	largeTransform.Attributes = map[string]string{}
	for index := 0; index < 8; index++ {
		largeTransform.Attributes[indexedName(index)] = strings.Repeat("v", 4096)
	}
	largeSources := make([]SourceDocument, 0, MaxSourceDocuments)
	for index := 0; index < MaxSourceDocuments; index++ {
		largeDocument := mustDocument(t, NamedAliasInput{Name: indexedName(index), Transform: &largeTransform})
		largeSources = append(largeSources, SourceDocument{SourceID: indexedName(index) + strings.Repeat("s", 120), Document: largeDocument})
	}
	if _, err := NewCatalog(largeSources); err == nil {
		t.Fatal("fixture did not exceed MaxCatalogBytes")
	} else if assertCompositionError(t, err, CodeExpansionTooLarge).Pointer != "" {
		t.Fatal("aggregate error must use root pointer")
	}
}

// C07, C08: catalogs retain immutable data only and have no hidden lookup state.
func TestCatalogSnapshotAndConcurrentExpansion(t *testing.T) {
	document := mustDocument(t,
		NamedAliasInput{Name: "base", Recipe: factoryRecipe("postgres:17", inlineTransformStep("one.sql"))},
	)
	sources := []SourceDocument{{SourceID: "aliases.yaml", Document: document}}
	catalog := mustCatalog(t, sources...)
	sources[0] = SourceDocument{}

	const readers = 16
	var wg sync.WaitGroup
	errorsFound := make(chan error, readers)
	for range readers {
		wg.Add(1)
		go func() {
			defer wg.Done()
			result, err := catalog.ExpandRecipe("base")
			if err == nil && result.Declaration().Factory().Reference != "postgres:17" {
				err = errors.New("snapshot changed")
			}
			errorsFound <- err
		}()
	}
	wg.Wait()
	close(errorsFound)
	for err := range errorsFound {
		if err != nil {
			t.Fatal(err)
		}
	}
}

// E01-E05: all successful expansion forms preserve their literal semantic order.
func TestExpansionSuccessMatrix(t *testing.T) {
	seed := testTransform("seed.sql")
	migrate := testTransform("migrate.sql")
	document := mustDocument(t,
		NamedAliasInput{Name: "seed", Transform: &seed},
		NamedAliasInput{Name: "migrate", Transform: &migrate},
		NamedAliasInput{Name: "base", Recipe: factoryRecipe("postgres:17", namedTransformStep("seed"))},
		NamedAliasInput{Name: "middle", Recipe: prefixRecipe("base", inlineTransformStep("middle.sql"))},
		NamedAliasInput{Name: "target", Recipe: prefixRecipe("middle", namedTransformStep("migrate"), namedTransformStep("seed"))},
		NamedAliasInput{Name: "factory-only", Recipe: factoryRecipe("postgres:16")},
	)
	catalog := mustCatalog(t, SourceDocument{SourceID: "aliases.yaml", Document: document})

	factoryOnly, err := catalog.ExpandRecipe("factory-only")
	if err != nil {
		t.Fatal(err)
	}
	assertRecipe(t, factoryOnly, "postgres:16")

	target, err := catalog.ExpandRecipe("target")
	if err != nil {
		t.Fatal(err)
	}
	assertRecipe(t, target, "postgres:17", "seed.sql", "middle.sql", "migrate.sql", "seed.sql")

	standalone, err := catalog.ExpandTransform("seed")
	if err != nil {
		t.Fatal(err)
	}
	if got := standalone.Declaration().Declaration().Reference; got != "seed.sql" {
		t.Fatalf("standalone reference = %q", got)
	}
}

// E06, E07: top-level and nested reference failures carry exact stable locations.
func TestExpansionReferenceErrors(t *testing.T) {
	transform := testTransform("seed.sql")
	document := mustDocument(t,
		NamedAliasInput{Name: "seed", Transform: &transform},
		NamedAliasInput{Name: "bad-base", Recipe: prefixRecipe("missing")},
		NamedAliasInput{Name: "wrong-base", Recipe: prefixRecipe("seed")},
		NamedAliasInput{Name: "bad-step", Recipe: factoryRecipe("postgres:17", namedTransformStep("missing"))},
	)
	catalog := mustCatalog(t, SourceDocument{SourceID: "aliases.yaml", Document: document})

	tests := []struct {
		name, target string
		transform    bool
		code         ErrorCode
		pointer      string
	}{
		{"invalid target", "BAD", false, CodeInvalidDocument, ""},
		{"missing target", "missing", false, CodeMissingReference, ""},
		{"wrong top kind", "seed", false, CodeWrongReferenceKind, "/aliases/seed"},
		{"missing base", "bad-base", false, CodeMissingReference, "/aliases/bad-base/base/recipe"},
		{"wrong base", "wrong-base", false, CodeWrongReferenceKind, "/aliases/wrong-base/base/recipe"},
		{"missing step", "bad-step", false, CodeMissingReference, "/aliases/bad-step/steps/0/use"},
		{"wrong transform target", "bad-base", true, CodeWrongReferenceKind, "/aliases/bad-base"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var err error
			if test.transform {
				_, err = catalog.ExpandTransform(test.target)
			} else {
				_, err = catalog.ExpandRecipe(test.target)
			}
			problem := assertCompositionError(t, err, test.code)
			if problem.Pointer != test.pointer {
				t.Fatalf("pointer = %q, want %q", problem.Pointer, test.pointer)
			}
		})
	}
}

// E08: active recipe revisits report the exact closed cycle and closing edge.
func TestExpansionCycleDiagnostics(t *testing.T) {
	document := mustDocument(t,
		NamedAliasInput{Name: "a", Recipe: prefixRecipe("b")},
		NamedAliasInput{Name: "b", Recipe: prefixRecipe("c")},
		NamedAliasInput{Name: "c", Recipe: prefixRecipe("b")},
	)
	catalog := mustCatalog(t, SourceDocument{SourceID: "aliases.yaml", Document: document})
	_, err := catalog.ExpandRecipe("a")
	problem := assertCompositionError(t, err, CodeCycle)
	if problem.Pointer != "/aliases/c/base/recipe" {
		t.Fatalf("cycle pointer = %q", problem.Pointer)
	}
	cycle := problem.Cycle()
	aliases := make([]string, len(cycle))
	for index := range cycle {
		aliases[index] = cycle[index].Alias
	}
	if !reflect.DeepEqual(aliases, []string{"b", "c", "b"}) {
		t.Fatalf("cycle = %#v", aliases)
	}
	cycle[0].Alias = "changed"
	if problem.Cycle()[0].Alias != "b" {
		t.Fatal("cycle accessor retained mutation")
	}
}

// E09: a maximal parent chain is iterative and cannot overflow the Go stack.
func TestExpansionDeepChainIsIterative(t *testing.T) {
	aliases := make([]NamedAliasInput, MaxAliases)
	aliases[MaxAliases-1] = NamedAliasInput{Name: indexedName(MaxAliases - 1), Recipe: factoryRecipe("postgres:17")}
	for index := MaxAliases - 2; index >= 0; index-- {
		aliases[index] = NamedAliasInput{Name: indexedName(index), Recipe: prefixRecipe(indexedName(index + 1))}
	}
	catalog := mustCatalog(t, SourceDocument{SourceID: "deep.json", Document: mustDocument(t, aliases...)})
	result, err := catalog.ExpandRecipe(indexedName(0))
	if err != nil {
		t.Fatalf("deep expansion: %v", err)
	}
	assertRecipe(t, result, "postgres:17")
}

// E10, E14: oversized output and all other failures return zero atomic results.
func TestExpansionOutputBoundAndAtomicFailure(t *testing.T) {
	baseSteps := make([]RecipeStepInput, MaxAliases/2+1)
	targetSteps := make([]RecipeStepInput, MaxAliases/2)
	for index := range baseSteps {
		baseSteps[index] = inlineTransformStep("x")
	}
	for index := range targetSteps {
		targetSteps[index] = inlineTransformStep("x")
	}
	document := mustDocument(t,
		NamedAliasInput{Name: "large-base", Recipe: factoryRecipe("postgres:17", baseSteps...)},
		NamedAliasInput{Name: "too-large", Recipe: prefixRecipe("large-base", targetSteps...)},
		NamedAliasInput{Name: "valid", Recipe: factoryRecipe("postgres:17")},
	)
	catalog := mustCatalog(t, SourceDocument{SourceID: "aliases.yaml", Document: document})
	result, err := catalog.ExpandRecipe("too-large")
	assertCompositionError(t, err, CodeExpansionTooLarge)
	if result.Declaration().Factory().Kind != "" || result.Trace().Nodes() != nil {
		t.Fatal("failure returned partial expansion")
	}
	valid, err := catalog.ExpandRecipe("valid")
	if err != nil {
		t.Fatalf("later valid expansion failed: %v", err)
	}
	assertRecipe(t, valid, "postgres:17")
}

// E11-E13: diagnostic changes are excluded while semantic values remain sensitive.
func TestExpansionSemanticExclusionAndSensitivity(t *testing.T) {
	makeCatalog := func(source, baseName, stepName, transformRef string, reversed bool) Catalog {
		transformA := testTransform(transformRef)
		transformB := testTransform("other.sql")
		steps := []RecipeStepInput{namedTransformStep(stepName), inlineTransformStep("other.sql")}
		if reversed {
			steps[0], steps[1] = steps[1], steps[0]
		}
		document := mustDocument(t,
			NamedAliasInput{Name: stepName, Transform: &transformA},
			NamedAliasInput{Name: "unused", Transform: &transformB},
			NamedAliasInput{Name: baseName, Recipe: factoryRecipe("postgres:17", steps...)},
		)
		return mustCatalog(t, SourceDocument{SourceID: source, Document: document})
	}
	left := makeCatalog("one.yaml", "recipe-a", "step-a", "seed.sql", false)
	right := makeCatalog("two.yaml", "recipe-b", "step-b", "seed.sql", false)
	changed := makeCatalog("one.yaml", "recipe-a", "step-a", "changed.sql", false)
	reordered := makeCatalog("one.yaml", "recipe-a", "step-a", "seed.sql", true)

	leftValue, _ := left.ExpandRecipe("recipe-a")
	rightValue, _ := right.ExpandRecipe("recipe-b")
	changedValue, _ := changed.ExpandRecipe("recipe-a")
	reorderedValue, _ := reordered.ExpandRecipe("recipe-a")
	if !bytes.Equal(mustJSON(t, leftValue.Declaration()), mustJSON(t, rightValue.Declaration())) {
		t.Fatal("alias/source renaming changed declaration")
	}
	if bytes.Equal(mustJSON(t, leftValue.Trace()), mustJSON(t, rightValue.Trace())) {
		t.Fatal("diagnostic rename did not change trace")
	}
	if bytes.Equal(mustJSON(t, leftValue.Declaration()), mustJSON(t, changedValue.Declaration())) {
		t.Fatal("child declaration change did not propagate")
	}
	if bytes.Equal(mustJSON(t, leftValue.Declaration()), mustJSON(t, reorderedValue.Declaration())) {
		t.Fatal("step reordering did not propagate")
	}
}

func expansionError[T any](_ T, err error) error { return err }

func permutations[T any](values []T) [][]T {
	result := make([][]T, 0)
	var visit func(int)
	copyValues := append([]T(nil), values...)
	visit = func(index int) {
		if index == len(copyValues) {
			result = append(result, append([]T(nil), copyValues...))
			return
		}
		for candidate := index; candidate < len(copyValues); candidate++ {
			copyValues[index], copyValues[candidate] = copyValues[candidate], copyValues[index]
			visit(index + 1)
			copyValues[index], copyValues[candidate] = copyValues[candidate], copyValues[index]
		}
	}
	visit(0)
	return result
}
