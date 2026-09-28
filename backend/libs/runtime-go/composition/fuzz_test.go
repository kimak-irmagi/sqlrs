package composition

import (
	"bytes"
	"encoding/json"
	"testing"
)

// X03: accepted document JSON is strict, atomic, and immediately revalidatable.
func FuzzAliasDocumentJSON(f *testing.F) {
	seed := mustDocument(f,
		NamedAliasInput{Name: "seed", Transform: ptrTransform(testTransform("seed.sql"))},
		NamedAliasInput{Name: "database", Recipe: factoryRecipe("postgres:17", namedTransformStep("seed"))},
	)
	f.Add(mustJSON(f, seed))
	f.Add([]byte(`{"schema_version":"sqlrs.runtime.v2.aliases.v1","aliases":{}}`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		initial := mustDocument(t)
		before := append([]byte(nil), mustJSON(t, initial)...)
		err := DecodeAliasDocumentJSON(raw, &initial)
		if err != nil {
			if !bytes.Equal(before, mustJSON(t, initial)) {
				t.Fatal("failed decode mutated receiver")
			}
			return
		}
		encoded := mustJSON(t, initial)
		var roundTrip AliasDocument
		if err := DecodeAliasDocumentJSON(encoded, &roundTrip); err != nil {
			t.Fatalf("accepted value failed revalidation: %v", err)
		}
	})
}

// X04: every accepted trace satisfies the canonical generated wire grammar.
func FuzzExpansionTraceJSON(f *testing.F) {
	f.Add([]byte(recipeTraceJSON("source.yaml")))
	f.Fuzz(func(t *testing.T, raw []byte) {
		var trace ExpansionTrace
		if err := DecodeExpansionTraceJSON(raw, &trace); err != nil {
			return
		}
		encoded := mustJSON(t, trace)
		var roundTrip ExpansionTrace
		if err := DecodeExpansionTraceJSON(encoded, &roundTrip); err != nil {
			t.Fatalf("accepted trace failed revalidation: %v", err)
		}
		if trace.TargetNodeID() != 0 || len(trace.Nodes()) == 0 {
			t.Fatalf("accepted invalid trace: %s", encoded)
		}
	})
}

// X05: bounded production expansion agrees with an independent recursive flattener.
func FuzzGraphExpansion(f *testing.F) {
	f.Add(uint8(1), uint8(2), uint8(3))
	f.Fuzz(func(t *testing.T, depthRaw, baseStepsRaw, targetStepsRaw uint8) {
		depth := int(depthRaw%8) + 1
		baseSteps := int(baseStepsRaw % 8)
		targetSteps := int(targetStepsRaw % 8)
		aliases := make([]NamedAliasInput, 0, depth+baseSteps+targetSteps)
		expected := make([]string, 0, baseSteps+targetSteps)
		for index := 0; index < depth; index++ {
			name := indexedName(index)
			steps := []RecipeStepInput{}
			count := 0
			if index == depth-1 {
				count = baseSteps
			} else if index == 0 {
				count = targetSteps
			}
			for step := 0; step < count; step++ {
				reference := indexedName(index) + "-" + indexedName(step) + ".sql"
				steps = append(steps, inlineTransformStep(reference))
				if index == depth-1 {
					expected = append(expected, reference)
				}
			}
			if index == depth-1 {
				aliases = append(aliases, NamedAliasInput{Name: name, Recipe: factoryRecipe("postgres:17", steps...)})
			} else {
				aliases = append(aliases, NamedAliasInput{Name: name, Recipe: prefixRecipe(indexedName(index+1), steps...)})
			}
		}
		if depth > 1 {
			for step := 0; step < targetSteps; step++ {
				expected = append(expected, indexedName(0)+"-"+indexedName(step)+".sql")
			}
		}
		catalog := mustCatalog(t, SourceDocument{SourceID: "fuzz.yaml", Document: mustDocument(t, aliases...)})
		result, err := catalog.ExpandRecipe(indexedName(0))
		if err != nil {
			t.Fatal(err)
		}
		got := result.Declaration().Transforms()
		if len(got) != len(expected) {
			t.Fatalf("transform count = %d, want %d", len(got), len(expected))
		}
		for index := range got {
			if got[index].Reference != expected[index] {
				t.Fatalf("transform[%d] = %q, want %q", index, got[index].Reference, expected[index])
			}
		}
		if _, err := json.Marshal(result.Trace()); err != nil {
			t.Fatal(err)
		}
	})
}
