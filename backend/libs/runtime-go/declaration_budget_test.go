package runtimev2

import (
	"encoding/json"
	"strings"
	"testing"
)

// DL08: checked arithmetic and the production traversal primitive provide
// deterministic early-exit evidence without timing or allocation thresholds.
func TestDeclarationBudgetArithmeticAndEarlyExit(t *testing.T) {
	b := declarationJSONBudget{limit: 4}
	if err := b.add(4); err != nil || b.used != 4 {
		t.Fatal("inclusive bound", err)
	}
	if err := b.add(1); err == nil || b.used != 4 {
		t.Fatal("overflow changed budget")
	}
	b = declarationJSONBudget{used: 1, limit: 4}
	if err := b.add(int(^uint(0) >> 1)); err == nil || b.used != 1 {
		t.Fatal("arithmetic wrapped")
	}
	for _, limit := range []int{0, 2, 3, 4, 5} {
		b = declarationJSONBudget{limit: limit}
		visits := 0
		err := b.collection(3, func(i int) error { visits++; return b.add(1) })
		if err == nil || visits > 2 || b.used > limit {
			t.Fatalf("unbounded traversal at %d: %d, %v", limit, visits, err)
		}
	}
	b = declarationJSONBudget{limit: 7}
	visits := 0
	if err := b.collection(3, func(i int) error { visits++; return b.add(1) }); err != nil || visits != 3 || b.used != 7 {
		t.Fatal("exact collection bound", visits, b.used, err)
	}
	childErr := invalid(CodeTooLarge, "child")
	b = declarationJSONBudget{limit: 100}
	visits = 0
	if err := b.collection(3, func(i int) error { visits++; return childErr }); err != childErr || visits != 1 {
		t.Fatal("visited rejected tail")
	}
}

func TestDeclarationBudgetEscaping(t *testing.T) {
	for _, value := range []string{"", "simple", "q\"\\<>&\n\t\u2028\u2029", strings.Repeat("<", 256<<10)} {
		raw, err := json.Marshal(value)
		if err != nil {
			t.Fatal(err)
		}
		b := declarationJSONBudget{limit: len(raw)}
		if err := b.string(value); err != nil || b.used != len(raw) {
			t.Fatalf("escaped size: %d/%d %v", b.used, len(raw), err)
		}
		b = declarationJSONBudget{limit: len(raw) - 1}
		if err := b.string(value); err == nil || b.used > b.limit {
			t.Fatal("escaped overflow admitted")
		}
	}
	b := declarationJSONBudget{limit: 1}
	if err := b.string(strings.Repeat("x", 256<<10)); err == nil || b.used != 0 {
		t.Fatal("raw lower bound was not rejected")
	}
}

func TestDeclarationBudgetObjectEarlyExit(t *testing.T) {
	for _, limit := range []int{0, 2, 4, 5, 6, 7} {
		b := declarationJSONBudget{limit: limit}
		visits := 0
		members := []declarationJSONMember{
			{name: "a", value: func() error { visits++; return b.add(1) }},
			{name: "b", value: func() error { visits++; return b.add(1) }},
		}
		if err := b.object(members); err == nil || visits > 1 || b.used > limit {
			t.Fatalf("object tail visited at %d", limit)
		}
	}
	b := declarationJSONBudget{limit: 7}
	if err := b.object([]declarationJSONMember{{name: "a", value: func() error { return b.add(1) }}}); err != nil || b.used != 7 {
		t.Fatal("exact object budget", err)
	}
}
