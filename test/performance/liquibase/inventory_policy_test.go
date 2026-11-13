package benchmark

import (
	"strings"
	"testing"
)

// IC07: expectations precede samples and belong to exact selected artifacts,
// not to whichever observed count makes a run green.
func TestInventoryProbeExpectations(t *testing.T) {
	revision, hash := strings.Repeat("a", 40), strings.Repeat("b", 64)
	for _, route := range []string{"v1", "v2"} {
		got, err := inventoryProbeExpectations("", "", route, revision, hash)
		want := [2]int{1, 1}
		if route == "v1" {
			want = [2]int{0, 0}
		}
		if err != nil || got != want {
			t.Fatal("legacy policy changed", route, got, err)
		}
	}
	for _, variant := range []string{"new_off", "new_on"} {
		got, err := inventoryProbeExpectations("per-process-v1", variant, "v2", revision, hash)
		if err != nil || got != [2]int{1, 0} {
			t.Fatal("new cold/hit policy differs", got, err)
		}
	}
	for _, fields := range [][5]string{
		{"unknown", "new_on", "v2", revision, hash}, {"", "new_on", "v2", revision, hash},
		{"per-process-v1", "old_v2", "v2", revision, hash}, {"per-process-v1", "", "v2", revision, hash},
		{"per-process-v1", "new_on", "v1", revision, hash}, {"", "", "unknown", revision, hash},
		{"per-process-v1", "new_on", "v2", "main", hash}, {"per-process-v1", "new_on", "v2", revision, "bad"},
		{"per-process-v1", "new_on", "v2", "668d5050ad154c1967bea4c6e40b91f3a703d5ba", hash},
		{"per-process-v1", "new_on", "v2", "cab99c4c7ced5b18f4dc43a65b002d84d8fef02d", hash},
		{"per-process-v1", "new_on", "v2", revision, "1367bce9ddaa44c0aa378b07c015dfb20a4c0d29f00d2f5a893905686fae14b0"},
		{"per-process-v1", "new_on", "v2", revision, "b34c358d2d1e6e969f17d4e393641dd7cb53167870b8e4674db1381bf410bbca"},
	} {
		if _, err := inventoryProbeExpectations(fields[0], fields[1], fields[2], fields[3], fields[4]); err == nil {
			t.Fatal("misplaced policy accepted", fields[:3])
		}
	}
}
