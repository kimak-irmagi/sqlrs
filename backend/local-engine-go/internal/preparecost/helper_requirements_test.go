package preparecost

import (
	"fmt"
	"math"
	"reflect"
	"strings"
	"testing"
)

func helperCostFixture(ticks []int64) string {
	var data strings.Builder
	phases := []string{"preflight", "cli", "drain", "stream_flush", "hash_rename", "renamed_flush", "manifest_flush", "final_flush"}
	for i, phase := range phases {
		fmt.Fprintf(&data, "sqlrs-lb-cost-v1 %s %d %d\n", phase, ticks[i], ticks[i+1])
	}
	return data.String()
}

// CT09/CT11: this is local 10 ms detail of an already observed host call,
// never a fabricated placement on the host clock or an additive host cost.
func TestHelperCostDetailResolutionAndOwnership(t *testing.T) {
	data := helperCostFixture([]int64{0, 1, 31, 31, 33, 34, 36, 37, 38})
	got, err := ParseHelperDetail([]byte(data), 385_000_000)
	if err != nil || got.ResolutionNS != 10_000_000 || got.TotalNS != 380_000_000 ||
		got.CumulativeUncertaintyNS != 80_000_000 || len(got.Regions) != 8 {
		t.Fatal("helper-local costs lost their coarse resolution", got, err)
	}
	if got.Regions[1].Phase != HelperCLI || got.Regions[1].DurationNS != 300_000_000 || got.Regions[2].DurationNS != 0 {
		t.Fatal("CLI/quantized zero duration differ from independent oracle")
	}
	if got.Regions[2].StartCS != 31 || got.Regions[2].EndCS != 31 {
		t.Fatal("local offsets replaced with invented host offsets")
	}
	got.Regions[0].Phase = "caller-mutation"
	second, err := ParseHelperDetail([]byte(data), 385_000_000)
	if err != nil || second.Regions[0].Phase != HelperPreflight {
		t.Fatal("helper detail shares mutable report storage", err)
	}
	zero, err := ParseHelperDetail([]byte(helperCostFixture(make([]int64, 9))), 0)
	if err != nil || len(zero.Regions) != 8 || zero.TotalNS != 0 {
		t.Fatal("present quantized zero confused with unavailable detail", err)
	}
	if _, err := ParseHelperDetail([]byte(data), 370_000_000); err != nil {
		t.Fatal("one-resolution host/quantization boundary rejected", err)
	}
	if _, err := ParseHelperDetail([]byte(data), 369_999_999); err == nil {
		t.Fatal("helper duration incompatible with containing host call accepted")
	}
	maximum := helperCostFixture([]int64{0, 1, 2, 3, 4, 5, 6, 7, 922337203685})
	if got, err := ParseHelperDetail([]byte(maximum), math.MaxInt64); err != nil || got.TotalNS != 9223372036850000000 {
		t.Fatal("exact representable helper maximum rejected", err)
	}
}

func TestHelperCostDetailRejectsIncompleteOrPrivateInput(t *testing.T) {
	base := helperCostFixture([]int64{0, 1, 2, 3, 4, 5, 6, 7, 8})
	for _, variant := range []string{"missing", "empty", "unknown-version", "unknown-phase", "duplicate", "reordered", "gap", "reversed",
		"negative", "plus", "leading-zero", "fraction", "overflow", "extra-field", "extra-record", "unterminated", "crlf", "oversize", "negative-host", "nonzero-origin"} {
		t.Run(variant, func(t *testing.T) {
			data, host := base, int64(math.MaxInt64)
			switch variant {
			case "missing":
				data = strings.ReplaceAll(data, "sqlrs-lb-cost-v1 drain 2 3\n", "")
			case "empty":
				data = ""
			case "unknown-version":
				data = strings.Replace(data, "cost-v1", "cost-v2", 1)
			case "unknown-phase":
				data = strings.Replace(data, "preflight", "private-input-marker", 1)
			case "duplicate":
				data = strings.Replace(data, "cli 1 2", "preflight 1 2", 1)
			case "reordered":
				data = strings.Replace(data, "preflight", "cli", 1)
			case "gap":
				data = strings.Replace(data, "drain 2 3", "drain 3 3", 1)
			case "reversed":
				data = strings.Replace(data, "drain 2 3", "drain 2 1", 1)
			case "negative":
				data = strings.Replace(data, "preflight 0 1", "preflight -1 1", 1)
			case "plus":
				data = strings.Replace(data, "preflight 0 1", "preflight +0 1", 1)
			case "leading-zero":
				data = strings.Replace(data, "preflight 0 1", "preflight 00 1", 1)
			case "fraction":
				data = strings.Replace(data, "preflight 0 1", "preflight 0 1.5", 1)
			case "overflow":
				data = strings.Replace(data, "final_flush 7 8", "final_flush 7 922337203686", 1)
			case "extra-field":
				data = strings.Replace(data, "preflight 0 1", "preflight 0 1 private-input-marker", 1)
			case "extra-record":
				data += "private-input-marker\n"
			case "unterminated":
				data = strings.TrimSuffix(data, "\n")
			case "crlf":
				data = strings.ReplaceAll(data, "\n", "\r\n")
			case "oversize":
				data += strings.Repeat("private-input-marker", 300)
			case "negative-host":
				host = -1
			case "nonzero-origin":
				data = strings.Replace(data, "preflight 0 1", "preflight 1 1", 1)
			}
			got, err := ParseHelperDetail([]byte(data), host)
			if err == nil || !reflect.DeepEqual(got, HelperDetail{}) || strings.Contains(err.Error(), "private-input-marker") {
				t.Fatal("unqualified/private helper detail escaped its containing call", variant)
			}
		})
	}
}
