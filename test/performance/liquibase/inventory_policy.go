package benchmark

import (
	"fmt"
	"regexp"
)

// inventoryProbeExpectations is an observer policy, never a product setting.
// The caller has already verified these artifact bytes/build provenance. An
// explicit new off/on member may expect a cold probe followed by a hit; absent
// selection preserves the original independent legacy counts. IC07.
func inventoryProbeExpectations(selector, variant, route, revision, engineHash string) ([2]int, error) {
	invalid := func() ([2]int, error) { return [2]int{}, fmt.Errorf("invalid inventory probe policy") }
	if route != "v1" && route != "v2" {
		return invalid()
	}
	if selector == "" {
		if variant != "" {
			return invalid()
		}
		if route == "v1" {
			return [2]int{0, 0}, nil
		}
		return [2]int{1, 1}, nil
	}
	if selector != "per-process-v1" || route != "v2" || (variant != "new_off" && variant != "new_on") ||
		!regexp.MustCompile(`^[a-f0-9]{40}$`).MatchString(revision) || !regexp.MustCompile(`^[a-f0-9]{64}$`).MatchString(engineHash) ||
		revision == "668d5050ad154c1967bea4c6e40b91f3a703d5ba" || revision == "cab99c4c7ced5b18f4dc43a65b002d84d8fef02d" ||
		engineHash == "1367bce9ddaa44c0aa378b07c015dfb20a4c0d29f00d2f5a893905686fae14b0" || engineHash == "b34c358d2d1e6e969f17d4e393641dd7cb53167870b8e4674db1381bf410bbca" {
		return invalid()
	}
	return [2]int{1, 0}, nil
}

// inventoryProbeCountMatches checks real Docker observations against the
// artifact-bound cold/cache expectation. Unknown passes cannot qualify (IC07).
func inventoryProbeCountMatches(observed int, expected [2]int, pass string) bool {
	switch pass {
	case "uncached":
		return observed == expected[0]
	case "cached":
		return observed == expected[1]
	default:
		return false
	}
}
