package benchmark

import "math"

const costHelperResolutionNS = int64(10_000_000)

var costHelperPhases = [...]string{"preflight", "cli", "drain", "stream_flush", "hash_rename", "renamed_flush", "manifest_flush", "final_flush"}

func (root *costRoot) helperPhases(owner int) []string {
	phases := costHelperPhases[:]
	if root.stepResult && root.Spans[owner-1].Operation == "liquibase.invoke" {
		phases = append(append([]string(nil), phases...), "result_prepare", "result_emit")
	}
	return phases
}

// Closed numeric evidence; no helper text/identity/error survives parsing.
// DT03/DT05/DT07: issue89-cost-attribution-and-trimming-tests.md.
type costHelperRegion struct {
	Phase                      string
	StartCS, EndCS, DurationNS int64
}

type costHelperDetail struct {
	ResolutionNS, TotalNS, CumulativeUncertaintyNS int64
	Regions                                        []costHelperRegion
}

type costHelperObservation struct {
	Owner         int
	HostElapsedNS int64
	Available     bool
	Detail        costHelperDetail
}

type costObserver struct{ EncodeNS, LockWaitNS, WriteNS int64 }

func (root *costRoot) acceptHelper(fields []string) error {
	if len(fields) != 5 && len(fields) != 7 {
		return errCostStream
	}
	owner, valid := costUnsigned(fields[2], 63)
	if !valid || owner == 0 || owner > uint64(len(root.Spans)) {
		return errCostStream
	}
	span := root.Spans[owner-1]
	phases := root.helperPhases(int(owner))
	if span.Operation != "liquibase.invoke" && span.Operation != "liquibase.plan_invoke" {
		return errCostStream
	}
	host, valid := costUnsigned(fields[len(fields)-1], 63)
	if !valid || host > uint64(span.EndNS-span.StartNS) {
		return errCostStream
	}
	index, exists := root.helperIndex[int(owner)]
	if len(fields) == 5 {
		if fields[3] != "unavailable" || exists {
			return errCostStream
		}
		root.addHelper(costHelperObservation{Owner: int(owner), HostElapsedNS: int64(host)})
		return nil
	}
	start, startOK := costUnsigned(fields[4], 63)
	end, endOK := costUnsigned(fields[5], 63)
	if !startOK || !endOK || end < start || end > uint64(math.MaxInt64/costHelperResolutionNS) {
		return errCostStream
	}
	if !exists {
		root.addHelper(costHelperObservation{Owner: int(owner), HostElapsedNS: int64(host), Available: true,
			Detail: costHelperDetail{ResolutionNS: costHelperResolutionNS, CumulativeUncertaintyNS: int64(len(phases)) * costHelperResolutionNS}})
		index = len(root.Helpers) - 1
	}
	helper := &root.Helpers[index]
	phase := len(helper.Detail.Regions)
	previous := int64(0)
	if phase > 0 {
		previous = helper.Detail.Regions[phase-1].EndCS
	}
	if !helper.Available || helper.HostElapsedNS != int64(host) || phase >= len(phases) || fields[3] != phases[phase] || int64(start) != previous {
		return errCostStream
	}
	total := int64(end) * costHelperResolutionNS
	if total > int64(host) && total-int64(host) > costHelperResolutionNS {
		return errCostStream
	}
	helper.Detail.Regions = append(helper.Detail.Regions, costHelperRegion{Phase: fields[3], StartCS: int64(start), EndCS: int64(end), DurationNS: int64(end-start) * costHelperResolutionNS})
	helper.Detail.TotalNS = total
	return nil
}

func (root *costRoot) addHelper(helper costHelperObservation) {
	if root.helperIndex == nil {
		root.helperIndex = make(map[int]int)
	}
	root.helperIndex[helper.Owner] = len(root.Helpers)
	root.Helpers = append(root.Helpers, helper)
}

// qualifyDetails requires exactly one complete observation for every invocation.
// Observation costs may occur outside the API window and are never subtracted.
func qualifyDetails(root costRoot) error {
	if root.Observer == nil || root.Observer.EncodeNS < 0 || root.Observer.LockWaitNS < 0 || root.Observer.WriteNS < 0 {
		return errCostStream
	}
	owners := make(map[int]bool)
	for _, span := range root.Spans {
		if span.Operation == "liquibase.invoke" || span.Operation == "liquibase.plan_invoke" {
			owners[span.ID] = true
		}
	}
	if len(owners) == 0 || len(owners) != len(root.Helpers) {
		return errCostStream
	}
	for _, helper := range root.Helpers {
		if !owners[helper.Owner] || !helper.Available || len(helper.Detail.Regions) != len(root.helperPhases(helper.Owner)) {
			return errCostStream
		}
		delete(owners, helper.Owner)
	}
	return nil
}
