package benchmark

import "fmt"

// stepResultCostSelection is a pre-observation artifact policy, never a product
// control. Reuse the same closed identity constraints as the new cache policy.
// IR06/IR10: issue89-same-exec-result-tests.md.
func stepResultCostSelection(selector, variant, route, revision, engineHash string) (bool, error) {
	if selector == "" && variant == "" {
		return false, nil
	}
	if selector != "same-exec-v1" {
		return false, fmt.Errorf("invalid step result policy")
	}
	if _, err := inventoryProbeExpectations("per-process-v1", variant, route, revision, engineHash); err != nil {
		return false, fmt.Errorf("invalid step result policy")
	}
	return true, nil
}
