package benchmark

import (
	"strings"
	"testing"
)

// IR06/IR10: selection precedes observation, defaults remain strictly legacy.
func TestStepResultCostSelection(t *testing.T) {
	revision, hash := strings.Repeat("a", 40), strings.Repeat("b", 64)
	for _, variant := range []string{"new_off", "new_on"} {
		if enabled, err := stepResultCostSelection("same-exec-v1", variant, "v2", revision, hash); err != nil || !enabled {
			t.Fatal(err)
		}
	}
	if enabled, err := stepResultCostSelection("", "", "v1", "", ""); err != nil || enabled {
		t.Fatal(err)
	}
	for _, args := range [][5]string{
		{"unknown", "new_on", "v2", revision, hash}, {"same-exec-v1", "old_v2", "v2", revision, hash},
		{"same-exec-v1", "new_on", "v1", revision, hash}, {"same-exec-v1", "new_on", "v2", "", hash},
		{"", "new_on", "v2", revision, hash},
	} {
		if _, err := stepResultCostSelection(args[0], args[1], args[2], args[3], args[4]); err == nil {
			t.Fatal("invalid result policy")
		}
	}
}

func TestStepResultCostStreamExplicitPhases(t *testing.T) {
	legacy := costDetailStreamExample(1)
	step := strings.Replace(legacy, "liquibase.plan_invoke planning 0", "liquibase.invoke step 1", 1)
	step = strings.Replace(step, "sqlrs-prepare-cost-v1 end", "sqlrs-prepare-cost-v1 helper 1 3 result_prepare 0 0 0\nsqlrs-prepare-cost-v1 helper 1 3 result_emit 0 0 0\nsqlrs-prepare-cost-v1 end", 1)
	for _, tc := range []struct {
		wire            string
		selected, valid bool
	}{
		{step, true, true}, {step, false, false}, {legacy, true, true}, {legacy, false, true},
		{strings.Replace(step, "liquibase.invoke step 1", "liquibase.plan_invoke planning 0", 1), true, false},
		{strings.Replace(step, "sqlrs-prepare-cost-v1 helper 1 3 result_emit 0 0 0\n", "", 1), true, false},
		{strings.Replace(legacy, "liquibase.plan_invoke planning 0", "liquibase.invoke step 1", 1), true, false},
	} {
		stream := newCostStream()
		if tc.selected {
			stream = newStepResultCostStream()
		}
		err := stream.consume(strings.NewReader(tc.wire))
		if err == nil {
			root, loadErr := stream.awaitMeasured(t.Context(), 1)
			err = loadErr
			if err == nil {
				err = qualifyDetails(root)
			}
		}
		if (err == nil) != tc.valid {
			t.Fatal("selected phase grammar not enforced", tc.selected, tc.valid, err)
		}
	}
}
