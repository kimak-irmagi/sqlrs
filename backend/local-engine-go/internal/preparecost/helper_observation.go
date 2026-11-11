package preparecost

import "context"

// HelperObservation binds local coarse-clock detail to a host-owned invocation.
// Unavailable is explicit, never a measured zero. No raw bytes are retained.
// DT01/DT03: issue89-cost-attribution-and-trimming.md §3.
type HelperObservation struct {
	Owner         int
	HostElapsedNS int64
	Available     bool
	Detail        HelperDetail
}

// RecordHelper observes protected-helper stdout without changing work outcomes.
// ctx must be the current open planning/step invocation. The disabled path has
// no allocation, clock read or decoder work. The host supplies elapsed time.
func RecordHelper(ctx context.Context, data []byte, hostElapsedNS int64) {
	current, ok := ctx.Value(scopeKey{}).(scope)
	if !ok || current.recorder == nil {
		return
	}
	r := current.recorder
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.finished || current.parent <= 0 || current.parent > len(r.spans) ||
		r.ended[current.parent-1] || !helperOwner(r.spans[current.parent-1].Operation) {
		r.invalid = true
		return
	}
	observation := HelperObservation{Owner: current.parent, HostElapsedNS: max(0, hostElapsedNS)}
	if _, duplicate := r.helpers[current.parent]; !duplicate {
		if detail, err := ParseHelperDetail(data, hostElapsedNS); err == nil {
			observation.Available, observation.Detail = true, detail
		}
	}
	if r.helpers == nil {
		r.helpers = make(map[int]HelperObservation)
	}
	r.helpers[current.parent] = observation
}

func helperOwner(operation Operation) bool {
	return operation == Invocation || operation == PlanningInvocation
}

// validHelper checks caller-owned snapshots again before serialization. Derived
// totals are verified from the fixed local intervals, not trusted as input.
func validHelper(observation HelperObservation, span Span) bool {
	if observation.Owner != span.ID || !helperOwner(span.Operation) || observation.HostElapsedNS < 0 ||
		observation.HostElapsedNS > span.EndNS-span.StartNS {
		return false
	}
	detail := observation.Detail
	if !observation.Available {
		return len(detail.Regions) == 0 && detail.TotalNS == 0 && detail.ResolutionNS == 0 && detail.CumulativeUncertaintyNS == 0
	}
	phases := [...]HelperPhase{HelperPreflight, HelperCLI, HelperDrain, HelperStreamFlush,
		HelperHashRename, HelperRenamedFlush, HelperManifest, HelperFinalFlush}
	if len(detail.Regions) != len(phases) || detail.ResolutionNS != HelperResolutionNS ||
		detail.CumulativeUncertaintyNS != int64(len(phases))*HelperResolutionNS {
		return false
	}
	var previous int64
	for index, region := range detail.Regions {
		if region.Phase != phases[index] || region.StartCS != previous || region.EndCS < region.StartCS ||
			region.EndCS > int64(^uint64(0)>>1)/HelperResolutionNS || region.DurationNS != (region.EndCS-region.StartCS)*HelperResolutionNS {
			return false
		}
		previous = region.EndCS
	}
	total := previous * HelperResolutionNS
	return detail.TotalNS == total && (total <= observation.HostElapsedNS || total-observation.HostElapsedNS <= HelperResolutionNS)
}
