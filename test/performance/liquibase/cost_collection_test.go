package benchmark

import (
	"reflect"
	"strings"
	"testing"
)

// CT06/CT13: before/after and off/on all use the same collector, but only the
// explicit new-v2 measured member can enable engine recording. Child/image
// environment is not a measurement authority.
func TestCostCollectionSelectionAndEnvironment(t *testing.T) {
	for _, row := range []struct {
		route, revision            string
		enabled, diagnostic, valid bool
	}{
		{"v2", strings.Repeat("a", 40), true, false, true},
		{"v2", "668d5050ad154c1967bea4c6e40b91f3a703d5ba", true, false, false},
		{"v1", strings.Repeat("a", 40), true, false, false},
		{"v2", strings.Repeat("a", 40), true, true, false},
		{"v1", strings.Repeat("a", 40), false, true, true},
		{"v2", "668d5050ad154c1967bea4c6e40b91f3a703d5ba", false, false, true},
	} {
		if err := validateCostSelection(row.route, row.revision, row.enabled, row.diagnostic); (err == nil) != row.valid {
			t.Fatal("cost selection crossed route/source/trace boundary", row)
		}
	}
	input := []string{"SQLRS_PREPARE_COST=1", "sqlrs_prepare_cost=1", "PATH=original", "JAVA_HOME=original"}
	before := append([]string(nil), input...)
	for _, enabled := range []bool{false, true} {
		got := costEngineEnvironment(input, enabled)
		want := []string{"PATH=original", "JAVA_HOME=original"}
		if enabled {
			want = append(want, "SQLRS_PREPARE_COST=1")
		}
		if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(input, before) {
			t.Fatal("measurement leaked into another route or mutated caller environment", got)
		}
	}
}

func TestCostCollectionQualifiesActualWorkerAndAPIWindow(t *testing.T) {
	stream := newCostStream()
	if err := stream.consume(strings.NewReader(costStreamExample(1))); err != nil {
		t.Fatal(err)
	}
	root, err := stream.await(t.Context(), 1)
	if err != nil {
		t.Fatal(err)
	}
	if err := qualifyCostPass(root, 1, 100, 1); err != nil {
		t.Fatal("valid worker evidence rejected", err)
	}
	for _, variant := range []string{"root", "incomplete", "unended", "empty", "worker", "parent", "phase", "step", "failed", "elapsed", "missing-parent", "foreign-step", "denominator", "changes"} {
		t.Run(variant, func(t *testing.T) {
			copy := root
			copy.Spans = append([]costSpan(nil), root.Spans...)
			elapsed, changes := int64(100), 1
			switch variant {
			case "root":
				copy.Root = 2
			case "incomplete":
				copy.Complete = false
			case "unended":
				copy.Ended = false
			case "empty":
				copy.Spans = nil
			case "worker":
				copy.Spans[0].Operation = "capture.read"
			case "parent":
				copy.Spans[0].Parent = 1
			case "phase":
				copy.Spans[0].Phase = "step"
			case "step":
				copy.Spans[0].Step = 1
			case "failed":
				copy.Spans[0].Outcome = "failed"
			case "elapsed":
				copy.ElapsedNS = 101
			case "missing-parent":
				copy.Spans[1].Parent = 0
			case "foreign-step":
				copy.Spans[1].Step = 2
			case "denominator":
				elapsed = 0
			case "changes":
				changes = 0
			}
			if err := qualifyCostPass(copy, 1, elapsed, changes); err == nil {
				t.Fatal("wrong worker/pass/step qualified", variant)
			}
		})
	}
}
