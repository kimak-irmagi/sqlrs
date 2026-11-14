package preparecost

import "sort"

const (
	// MaxTimelineAuxSpans bounds client polling and the separate observer scope.
	MaxTimelineAuxSpans  = 8193
	maxTimelineNumber    = int64(1<<53 - 1)
	maxTimelineOwnerRefs = 4 * MaxSpans
)

// TimelineWindow is the actual API workload window on the selected shared
// monotonic axis, not a duration inferred from the worker. Requirements: §4 of
// docs/architecture/issue89-complete-timeline.md.
type TimelineWindow struct{ StartNS, EndNS int64 }

// TimelineAuxSpan contains observed client or emitter boundaries, never a
// guessed network/SQL cause. Only the four closed Kind values are accepted.
type TimelineAuxSpan struct {
	Kind           string
	StartNS, EndNS int64
}

// TimelineSegment owns wall time once. Owners identify deepest engine spans;
// Observer distinguishes emitter overlap without fabricating an engine span.
// Endpoints are relative to the beginning of the selected window.
type TimelineSegment struct {
	StartNS, EndNS int64
	Kind           string
	Owners         []int
	Observer       bool
}

// TimelineOutside retains observed intervals outside the workload window.
// Its endpoints remain on the input reference axis, not the window-local axis.
type TimelineOutside struct {
	Kind           string
	Owner          int
	StartNS, EndNS int64
}

// Timeline conserves the selected wall duration; a balanced sum is not proof
// that source-local unresolved work or concurrent costs have been explained.
type Timeline struct {
	ElapsedNS, UnresolvedNS, OverlapNS int64
	Segments                           []TimelineSegment
	Outside                            []TimelineOutside
}

type timelineEvent struct {
	at         int64
	id, parent int
	kind       string
	start      bool
}

// BuildTimeline independently partitions host and client evidence. It accepts
// empty engine evidence for arithmetic tests only; the collector separately
// requires the actual request/worker/boundary proof. Inputs are never mutated.
// Invalid or over-budget evidence returns no partial result or private text.
func BuildTimeline(spans []Span, auxiliary []TimelineAuxSpan, window TimelineWindow) (Timeline, error) {
	if !timelineInterval(window.StartNS, window.EndNS) || window.StartNS == window.EndNS ||
		len(spans) > MaxSpans || len(auxiliary) > MaxTimelineAuxSpans {
		return Timeline{}, errEvidence
	}
	elapsed := window.EndNS
	byID := make(map[int]Span, len(spans))
	for _, span := range spans {
		if !timelineInterval(span.StartNS, span.EndNS) {
			return Timeline{}, errEvidence
		}
		elapsed = max(elapsed, span.EndNS)
		byID[span.ID] = span
	}
	if _, err := Summarize(spans, elapsed); err != nil {
		return Timeline{}, errEvidence
	}
	for _, observer := range []bool{false, true} {
		var intervals []interval
		for _, item := range auxiliary {
			if !timelineInterval(item.StartNS, item.EndNS) || !timelineAuxKind(item.Kind) {
				return Timeline{}, errEvidence
			}
			if (item.Kind == "observer") == observer && item.EndNS > item.StartNS {
				intervals = append(intervals, interval{item.StartNS, item.EndNS})
			}
		}
		sort.Slice(intervals, func(i, j int) bool { return intervals[i].start < intervals[j].start })
		for i := 1; i < len(intervals); i++ {
			if intervals[i].start < intervals[i-1].end {
				return Timeline{}, errEvidence
			}
		}
	}
	result := Timeline{ElapsedNS: window.EndNS - window.StartNS}
	events := []timelineEvent{{at: window.StartNS}, {at: window.EndNS}}
	add := func(start, end int64, id, parent int, kind string) {
		if start < window.StartNS && start < end {
			result.Outside = append(result.Outside, TimelineOutside{kind, id, start, min(end, window.StartNS)})
		}
		if end > window.EndNS && start < end {
			result.Outside = append(result.Outside, TimelineOutside{kind, id, max(start, window.EndNS), end})
		}
		start, end = max(start, window.StartNS), min(end, window.EndNS)
		if start < end {
			events = append(events, timelineEvent{start, id, parent, kind, true}, timelineEvent{end, id, parent, kind, false})
		}
	}
	for _, span := range spans {
		add(span.StartNS, span.EndNS, span.ID, span.Parent, "engine")
	}
	for _, item := range auxiliary {
		add(item.StartNS, item.EndNS, 0, 0, item.Kind)
	}
	sort.Slice(events, func(i, j int) bool {
		if events[i].at != events[j].at {
			return events[i].at < events[j].at
		}
		return !events[i].start && events[j].start
	})
	active, children, leaves := map[int]bool{}, map[int]int{}, map[int]bool{}
	client, observer := "", false
	previous, references := window.StartNS, 0
	for i := 0; i < len(events); {
		at := events[i].at
		if at > previous {
			references += len(leaves)
			if references > maxTimelineOwnerRefs {
				return Timeline{}, errEvidence
			}
			segment := TimelineSegment{StartNS: previous - window.StartNS, EndNS: at - window.StartNS, Kind: "unresolved", Observer: observer}
			for id := range leaves {
				segment.Owners = append(segment.Owners, id)
			}
			sort.Ints(segment.Owners)
			switch {
			case len(leaves) > 1 || len(leaves) == 1 && observer:
				segment.Kind = "overlap"
				result.OverlapNS += at - previous
			case len(leaves) == 1:
				if !timelineStructural(byID[segment.Owners[0]].Operation) {
					segment.Kind = "engine"
				}
			case observer:
				segment.Kind = "observer"
			case client != "":
				segment.Kind = client
			}
			if segment.Kind == "unresolved" {
				result.UnresolvedNS += at - previous
			}
			result.Segments = append(result.Segments, segment)
		}
		for i < len(events) && events[i].at == at {
			event := events[i]
			i++
			if event.kind == "" {
				continue
			}
			if event.kind == "observer" {
				observer = event.start
				continue
			}
			if event.kind != "engine" {
				client = ""
				if event.start {
					client = event.kind
				}
				continue
			}
			if event.start {
				active[event.id] = true
				if children[event.id] == 0 {
					leaves[event.id] = true
				}
				if event.parent != 0 {
					children[event.parent]++
					delete(leaves, event.parent)
				}
			} else {
				delete(active, event.id)
				delete(leaves, event.id)
				if event.parent != 0 {
					children[event.parent]--
					if children[event.parent] == 0 && active[event.parent] {
						leaves[event.parent] = true
					}
				}
			}
		}
		previous = at
	}
	sort.Slice(result.Outside, func(i, j int) bool {
		a, b := result.Outside[i], result.Outside[j]
		if a.StartNS != b.StartNS {
			return a.StartNS < b.StartNS
		}
		if a.EndNS != b.EndNS {
			return a.EndNS < b.EndNS
		}
		if a.Kind != b.Kind {
			return a.Kind < b.Kind
		}
		return a.Owner < b.Owner
	})
	return result, nil
}

// timelineInterval enforces interoperable integer nanoseconds before arithmetic.
func timelineInterval(start, end int64) bool {
	return start >= 0 && end >= start && end <= maxTimelineNumber
}

func timelineAuxKind(kind string) bool {
	switch kind {
	case "client.post", "client.status", "client.poll", "observer":
		return true
	}
	return false
}

// Structural self time names its source but deliberately does not invent the
// underlying cause. Finer observed leaf calls can classify it in later reports.
func timelineStructural(operation Operation) bool {
	switch operation {
	case Request, Admission, Worker, Binding, DBStartup, Planning, SessionStartup, StepExecution, Publication, FinalCleanup:
		return true
	}
	return false
}
