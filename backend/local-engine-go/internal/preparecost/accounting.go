package preparecost

import (
	"errors"
	"math"
	"sort"
)

const MaxSpans = 32768

// Phase separates per-step work from work performed once for the prepare.
type Phase string

const (
	PhaseAdmission   Phase = "admission"
	PhaseWorker      Phase = "worker"
	PhaseStartup     Phase = "startup"
	PhasePlanning    Phase = "planning"
	PhaseStep        Phase = "step"
	PhasePublication Phase = "publication"
	PhaseCleanup     Phase = "cleanup"
)

// Outcome deliberately carries no arbitrary error text.
type Outcome string

const (
	Succeeded Outcome = "succeeded"
	Failed    Outcome = "failed"
	Cancelled Outcome = "cancelled"
	Panicked  Outcome = "panicked"
)

// Span is one complete host observation on a single prepare-relative monotonic
// axis. Helper-local coarse-clock regions must never be inserted into this tree.
type Span struct {
	ID, Parent     int
	Operation      Operation
	Phase          Phase
	Step           int
	StartNS, EndNS int64
	Outcome        Outcome
}

// Row keeps inclusive union and caller durations distinct. SelfNS is the union
// of this key's own work, excluding its children; rows remain non-additive.
// ExclusiveNS alone is additive with other rows, SharedNS and UnassignedNS.
type Row struct {
	Operation                           Operation
	Phase                               Phase
	Step, Count                         int
	CallNS                              []int64
	SumNS, UnionNS, SelfNS, ExclusiveNS int64
	MeanPerCallMS, RatioPercent         float64
}

// Summary conserves the original API elapsed time without inventing a cause
// for unobserved work. AssignedNS includes the shared-overlap bucket.
type Summary struct {
	PrepareElapsedNS, AssignedNS, SharedNS, UnassignedNS int64
	Rows                                                 []Row
}

var errEvidence = errors.New("invalid prepare cost evidence")

type interval struct{ start, end int64 }
type rowKey struct {
	op    Operation
	phase Phase
	step  int
}

// Summarize implements CT11's union/self/exclusive accounting in O(n log n),
// bounded by MaxSpans. It does not mutate observations or infer missing timings.
func Summarize(spans []Span, elapsedNS int64) (Summary, error) {
	if elapsedNS <= 0 || len(spans) > MaxSpans {
		return Summary{}, errEvidence
	}
	byID := make(map[int]Span, len(spans))
	for _, span := range spans {
		if !validSpan(span, elapsedNS) {
			return Summary{}, errEvidence
		}
		if _, exists := byID[span.ID]; exists {
			return Summary{}, errEvidence
		}
		byID[span.ID] = span
	}
	children := make(map[int][]interval)
	for _, span := range spans {
		if span.Parent == 0 {
			continue
		}
		parent, found := byID[span.Parent]
		if !found || span.StartNS < parent.StartNS || span.EndNS > parent.EndNS ||
			parent.Step > 0 && (span.Step != parent.Step || span.Phase != parent.Phase) {
			return Summary{}, errEvidence
		}
		children[span.Parent] = append(children[span.Parent], interval{span.StartNS, span.EndNS})
	}
	state := make(map[int]byte, len(spans))
	var acyclic func(int) bool
	acyclic = func(id int) bool {
		if id == 0 || state[id] == 2 {
			return true
		}
		if state[id] == 1 {
			return false
		}
		state[id] = 1
		if !acyclic(byID[id].Parent) {
			return false
		}
		state[id] = 2
		return true
	}
	for _, span := range spans {
		if !acyclic(span.ID) {
			return Summary{}, errEvidence
		}
	}
	ordered := append([]Span(nil), spans...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].ID < ordered[j].ID })
	result := Summary{PrepareElapsedNS: elapsedNS}
	rowIndex, spanRow := map[rowKey]int{}, map[int]int{}
	var inclusive, own [][]interval
	for _, span := range ordered {
		key := rowKey{span.Operation, span.Phase, span.Step}
		index, found := rowIndex[key]
		if !found {
			index = len(result.Rows)
			rowIndex[key] = index
			result.Rows = append(result.Rows, Row{Operation: key.op, Phase: key.phase, Step: key.step})
			inclusive, own = append(inclusive, nil), append(own, nil)
		}
		spanRow[span.ID] = index
		row := &result.Rows[index]
		duration := span.EndNS - span.StartNS
		if duration > math.MaxInt64-row.SumNS {
			return Summary{}, errEvidence
		}
		row.Count++
		row.SumNS += duration
		row.CallNS = append(row.CallNS, duration)
		inclusive[index] = append(inclusive[index], interval{span.StartNS, span.EndNS})
		cursor := span.StartNS
		for _, child := range mergeIntervals(children[span.ID]) {
			if cursor < child.start {
				own[index] = append(own[index], interval{cursor, child.start})
			}
			cursor = max(cursor, child.end)
		}
		if cursor < span.EndNS {
			own[index] = append(own[index], interval{cursor, span.EndNS})
		}
	}
	for index := range result.Rows {
		row := &result.Rows[index]
		row.UnionNS = unionNS(inclusive[index])
		row.SelfNS = unionNS(own[index])
		row.MeanPerCallMS = float64(row.SumNS) / float64(row.Count) / 1e6
		row.RatioPercent = float64(row.UnionNS) / float64(elapsedNS) * 100
	}
	attributeExclusive(ordered, spanRow, &result)
	result.UnassignedNS = elapsedNS - result.AssignedNS
	sort.Slice(result.Rows, func(i, j int) bool {
		a, b := result.Rows[i], result.Rows[j]
		if a.Operation != b.Operation {
			return a.Operation < b.Operation
		}
		if a.Phase != b.Phase {
			return a.Phase < b.Phase
		}
		return a.Step < b.Step
	})
	return result, nil
}

func validSpan(span Span, elapsedNS int64) bool {
	if _, found := Describe(span.Operation); !found || span.ID <= 0 || span.Parent < 0 ||
		span.StartNS < 0 || span.EndNS < span.StartNS || span.EndNS > elapsedNS {
		return false
	}
	return validOutcome(span.Outcome) && validPhase(span.Phase, span.Step)
}

func validOutcome(outcome Outcome) bool {
	switch outcome {
	case Succeeded, Failed, Cancelled, Panicked:
		return true
	default:
		return false
	}
}

func validPhase(phase Phase, step int) bool {
	switch phase {
	case PhaseStep:
		return step > 0
	case PhaseAdmission, PhaseWorker, PhaseStartup, PhasePlanning, PhasePublication, PhaseCleanup:
		return step == 0
	default:
		return false
	}
}

// mergeIntervals returns a new, disjoint list. Adjacent intervals share no gap;
// zero duration still counts as a call but owns no elapsed-time segment.
func mergeIntervals(input []interval) []interval {
	ordered := append([]interval(nil), input...)
	sort.Slice(ordered, func(i, j int) bool {
		if ordered[i].start == ordered[j].start {
			return ordered[i].end < ordered[j].end
		}
		return ordered[i].start < ordered[j].start
	})
	var merged []interval
	for _, item := range ordered {
		if item.start == item.end {
			continue
		}
		if len(merged) == 0 || merged[len(merged)-1].end < item.start {
			merged = append(merged, item)
		} else {
			merged[len(merged)-1].end = max(merged[len(merged)-1].end, item.end)
		}
	}
	return merged
}

func unionNS(input []interval) int64 {
	var duration int64
	for _, item := range mergeIntervals(input) {
		duration += item.end - item.start
	}
	return duration
}

// attributeExclusive sweeps host endpoints. Active ancestors do not duplicate
// their descendants; simultaneously active leaves go into one shared bucket,
// even if their operation key is identical. No per-tick/per-pair scan is used.
func attributeExclusive(spans []Span, rowIndex map[int]int, summary *Summary) {
	type endpoint struct {
		time   int64
		id     int
		parent int
		start  bool
	}
	var events []endpoint
	for _, span := range spans {
		if span.StartNS != span.EndNS {
			events = append(events, endpoint{span.StartNS, span.ID, span.Parent, true}, endpoint{span.EndNS, span.ID, span.Parent, false})
		}
	}
	sort.Slice(events, func(i, j int) bool { return events[i].time < events[j].time })
	active, leaves, childCount := map[int]bool{}, map[int]bool{}, map[int]int{}
	previous := int64(0)
	for index := 0; index < len(events); {
		at := events[index].time
		if len(leaves) > 0 {
			duration := at - previous
			summary.AssignedNS += duration
			if len(leaves) > 1 {
				summary.SharedNS += duration
			} else {
				for id := range leaves {
					summary.Rows[rowIndex[id]].ExclusiveNS += duration
				}
			}
		}
		touched := map[int]bool{}
		for index < len(events) && events[index].time == at {
			event := events[index]
			active[event.id] = event.start
			touched[event.id] = true
			if event.parent != 0 {
				if event.start {
					childCount[event.parent]++
				} else {
					childCount[event.parent]--
				}
				touched[event.parent] = true
			}
			index++
		}
		for id := range touched {
			if active[id] && childCount[id] == 0 {
				leaves[id] = true
			} else {
				delete(leaves, id)
			}
		}
		previous = at
	}
}
