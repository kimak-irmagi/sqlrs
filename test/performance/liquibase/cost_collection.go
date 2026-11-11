package benchmark

import "strings"

// validateCostSelection follows (never replaces) exact clean artifact/hash
// validation. Only the explicit new-v2, untraced member can enable recording.
func validateCostSelection(route, revision string, enabled, diagnostic bool) error {
	if enabled && (route != "v2" || diagnostic || revision == "668d5050ad154c1967bea4c6e40b91f3a703d5ba") {
		return errCostStream
	}
	return nil
}

// costEngineEnvironment copies the caller's environment while stripping any
// inherited engine measurement selector on all routes. Only this collector's
// explicit selection may add it back. Unrelated environment remains unchanged.
func costEngineEnvironment(input []string, enabled bool) []string {
	var result []string
	for _, entry := range input {
		name, _, _ := strings.Cut(entry, "=")
		if !strings.EqualFold(name, "SQLRS_PREPARE_COST") {
			result = append(result, entry)
		}
	}
	if enabled {
		result = append(result, "SQLRS_PREPARE_COST=1")
	}
	return result
}

// qualifyCostPass links parsed evidence to this serial public API pass. The
// unchanged API elapsed is the denominator; uncovered admission/response time
// is not fabricated SQLite work. Only the actual successful worker may root
// a successful measured sample; step ordinals belong to this workload's plan.
func qualifyCostPass(root costRoot, expectedRoot uint64, apiElapsedNS int64, changes int) error {
	if root.Root != expectedRoot || root.Clock != nil || !root.Complete || !root.Ended || apiElapsedNS <= 0 || changes <= 0 ||
		root.ElapsedNS > apiElapsedNS || len(root.Spans) == 0 || len(root.Spans) != root.Declared {
		return errCostStream
	}
	worker := root.Spans[0]
	if worker.ID != 1 || worker.Parent != 0 || worker.Operation != "prepare.worker" || worker.Phase != "worker" || worker.Step != 0 ||
		worker.Outcome != "succeeded" || worker.StartNS < 0 || worker.EndNS < worker.StartNS || worker.EndNS != root.ElapsedNS {
		return errCostStream
	}
	for _, span := range root.Spans[1:] {
		if span.Parent == 0 || span.Step > changes {
			return errCostStream
		}
	}
	return qualifyDetails(root)
}
