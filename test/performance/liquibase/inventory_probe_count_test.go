package benchmark

import "testing"

// IC07: expected values are chosen before execution; the Go collector must
// reject incorrect observed counts, not merely produce the expected policy.
func TestInventoryProbeCountRejectsWrongObservation(t *testing.T) {
	for _, expectations := range [][2]int{{0, 0}, {1, 1}, {1, 0}} {
		for index, pass := range []string{"uncached", "cached"} {
			want := expectations[index]
			if !inventoryProbeCountMatches(want, expectations, pass) {
				t.Fatal("declared observation rejected", expectations, pass)
			}
			for _, wrong := range []int{-1, want + 1, want - 1} {
				if inventoryProbeCountMatches(wrong, expectations, pass) {
					t.Fatal("wrong observation accepted", wrong, expectations, pass)
				}
			}
		}
		if inventoryProbeCountMatches(0, expectations, "unknown") {
			t.Fatal("unknown pass accepted", expectations)
		}
	}
}
