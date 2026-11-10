package preparecost

import (
	"math"
	"strconv"
	"strings"
)

// HelperPhase names a region of the trusted helper, not a JVM internal phase.
type HelperPhase string

const (
	HelperPreflight    HelperPhase = "preflight"
	HelperCLI          HelperPhase = "cli"
	HelperDrain        HelperPhase = "drain"
	HelperStreamFlush  HelperPhase = "stream_flush"
	HelperHashRename   HelperPhase = "hash_rename"
	HelperRenamedFlush HelperPhase = "renamed_flush"
	HelperManifest     HelperPhase = "manifest_flush"
	HelperFinalFlush   HelperPhase = "final_flush"
	HelperResolutionNS             = int64(10_000_000)
)

// HelperRegion retains the helper's own centisecond offsets. DurationNS is a
// unit conversion, never a claim of nanosecond precision or host placement.
type HelperRegion struct {
	Phase          HelperPhase
	StartCS, EndCS int64
	DurationNS     int64
}

// HelperDetail is nested, non-additive evidence inside one host invocation.
// Each region has <10 ms quantization uncertainty; the reported cumulative
// bound deliberately does not assume independent errors or a common host clock.
type HelperDetail struct {
	ResolutionNS, TotalNS, CumulativeUncertaintyNS int64
	Regions                                        []HelperRegion
}

// ParseHelperDetail accepts only the eight ordered, contiguous fixed regions
// emitted on trusted helper stdout (never captured Liquibase output). The host
// supplies the current invocation/duration; no child-supplied ID can bind it.
// Missing/invalid detail yields no partial evidence and no private error text.
// CT09/CT11: issue89-cost-attribution-and-trimming.md §3.
func ParseHelperDetail(data []byte, hostElapsedNS int64) (HelperDetail, error) {
	if len(data) == 0 || len(data) > 4096 || data[len(data)-1] != '\n' || hostElapsedNS < 0 {
		return HelperDetail{}, errEvidence
	}
	phases := [...]HelperPhase{HelperPreflight, HelperCLI, HelperDrain, HelperStreamFlush,
		HelperHashRename, HelperRenamedFlush, HelperManifest, HelperFinalFlush}
	lines := strings.Split(string(data[:len(data)-1]), "\n")
	if len(lines) != len(phases) {
		return HelperDetail{}, errEvidence
	}
	result := HelperDetail{ResolutionNS: HelperResolutionNS,
		CumulativeUncertaintyNS: int64(len(phases)) * HelperResolutionNS,
		Regions:                 make([]HelperRegion, 0, len(phases))}
	var previous int64
	for index, line := range lines {
		fields := strings.Split(line, " ")
		if len(fields) != 4 || fields[0] != "sqlrs-lb-cost-v1" || fields[1] != string(phases[index]) {
			return HelperDetail{}, errEvidence
		}
		start, okStart := helperCentiseconds(fields[2])
		end, okEnd := helperCentiseconds(fields[3])
		if !okStart || !okEnd || start != previous || end < start {
			return HelperDetail{}, errEvidence
		}
		result.Regions = append(result.Regions, HelperRegion{Phase: phases[index], StartCS: start,
			EndCS: end, DurationNS: (end - start) * HelperResolutionNS})
		previous = end
	}
	result.TotalNS = previous * HelperResolutionNS
	// Differencing two downward-quantized helper readings can exceed the real
	// containing duration by less than one clock quantum. Avoid host+quantum
	// overflow at MaxInt64; larger mismatch is unavailable, not zero detail.
	if result.TotalNS > hostElapsedNS && result.TotalNS-hostElapsedNS > HelperResolutionNS {
		return HelperDetail{}, errEvidence
	}
	return result, nil
}

func helperCentiseconds(value string) (int64, bool) {
	number, err := strconv.ParseInt(value, 10, 64)
	return number, err == nil && number >= 0 && number <= math.MaxInt64/HelperResolutionNS && strconv.FormatInt(number, 10) == value
}
