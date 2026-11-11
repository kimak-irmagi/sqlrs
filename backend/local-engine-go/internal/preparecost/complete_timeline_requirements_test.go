package preparecost

import (
	"math"
	"reflect"
	"testing"
)

// TL09–TL11 use handwritten nanosecond oracles, not the implementation's own
// inclusive totals. See issue89-complete-timeline-tests.md.
func timelineVector() ([]Span, []TimelineAuxSpan, TimelineWindow) {
	return []Span{
			{ID: 1, Operation: Worker, Phase: PhaseWorker, StartNS: 110, EndNS: 190, Outcome: Succeeded},
			{ID: 2, Parent: 1, Operation: CaptureRead, Phase: PhaseStep, Step: 1, StartNS: 120, EndNS: 150, Outcome: Succeeded},
			{ID: 3, Parent: 1, Operation: ImageInventory, Phase: PhaseStartup, StartNS: 150, EndNS: 180, Outcome: Succeeded},
		}, []TimelineAuxSpan{
			{Kind: "client.post", StartNS: 100, EndNS: 125},
			{Kind: "client.poll", StartNS: 125, EndNS: 195},
			{Kind: "client.status", StartNS: 195, EndNS: 200},
		}, TimelineWindow{StartNS: 100, EndNS: 200}
}

func timelineAmounts(result Timeline) map[string]int64 {
	amounts := map[string]int64{}
	for _, segment := range result.Segments {
		amounts[segment.Kind] += segment.EndNS - segment.StartNS
	}
	return amounts
}

func assertTimelineConserves(t *testing.T, got Timeline) {
	t.Helper()
	var cursor, total int64
	for _, segment := range got.Segments {
		if segment.StartNS != cursor || segment.EndNS <= segment.StartNS {
			t.Fatalf("noncontiguous/empty ledger: %+v", got.Segments)
		}
		cursor = segment.EndNS
		total += segment.EndNS - segment.StartNS
	}
	if cursor != got.ElapsedNS || total != got.ElapsedNS {
		t.Fatalf("ledger total=%d end=%d elapsed=%d", total, cursor, got.ElapsedNS)
	}
}

func TestCompleteTimelineHandwrittenPartition(t *testing.T) {
	spans, auxiliary, window := timelineVector()
	before := append([]Span(nil), spans...)
	got, err := BuildTimeline(spans, auxiliary, window)
	if err != nil {
		t.Fatal(err)
	}
	assertTimelineConserves(t, got)
	want := map[string]int64{"client.post": 10, "unresolved": 20, "engine": 60, "client.poll": 5, "client.status": 5}
	if !reflect.DeepEqual(timelineAmounts(got), want) || got.UnresolvedNS != 20 || got.OverlapNS != 0 || got.ElapsedNS != 100 {
		t.Fatalf("handwritten partition differs: %+v", got)
	}
	var first, second int64
	for _, segment := range got.Segments {
		if segment.Kind == "engine" && reflect.DeepEqual(segment.Owners, []int{2}) {
			first += segment.EndNS - segment.StartNS
		}
		if segment.Kind == "engine" && reflect.DeepEqual(segment.Owners, []int{3}) {
			second += segment.EndNS - segment.StartNS
		}
	}
	if first != 30 || second != 30 || !reflect.DeepEqual(spans, before) {
		t.Fatal("child identity/duration or caller ownership changed")
	}
	spans[0], spans[2] = spans[2], spans[0]
	reordered, err := BuildTimeline(spans, auxiliary, window)
	if err != nil || !reflect.DeepEqual(got, reordered) {
		t.Fatal("input enumeration changed the timeline", err)
	}
}

func TestCompleteTimelineOverlapAndObserver(t *testing.T) {
	spans, auxiliary, window := timelineVector()
	spans = append(spans, Span{ID: 4, Parent: 1, Operation: CaptureRead, Phase: PhaseStep, Step: 2,
		StartNS: 135, EndNS: 145, Outcome: Failed})
	got, err := BuildTimeline(spans, auxiliary, window)
	if err != nil {
		t.Fatal(err)
	}
	assertTimelineConserves(t, got)
	if got.OverlapNS != 10 || timelineAmounts(got)["engine"] != 50 {
		t.Fatal("incomparable calls counted twice", got)
	}
	for _, segment := range got.Segments {
		if segment.Kind == "overlap" && (!reflect.DeepEqual(segment.Owners, []int{2, 4}) || segment.Observer) {
			t.Fatal("overlap lost its exact owners", segment)
		}
	}
	auxiliary = append(auxiliary, TimelineAuxSpan{Kind: "observer", StartNS: 175, EndNS: 205})
	got, err = BuildTimeline(spans, auxiliary, window)
	if err != nil {
		t.Fatal(err)
	}
	assertTimelineConserves(t, got)
	if got.OverlapNS != 25 || timelineAmounts(got)["observer"] != 10 {
		t.Fatal("observer/engine/client overlap or tail was double counted", got)
	}
	if !reflect.DeepEqual(got.Outside, []TimelineOutside{{Kind: "observer", StartNS: 200, EndNS: 205}}) {
		t.Fatal("observer tail was lost", got.Outside)
	}
}

func TestCompleteTimelineTailsUnknownAndBoundaryCases(t *testing.T) {
	spans, auxiliary, window := timelineVector()
	window = TimelineWindow{StartNS: 125, EndNS: 175}
	got, err := BuildTimeline(spans, auxiliary, window)
	if err != nil {
		t.Fatal(err)
	}
	assertTimelineConserves(t, got)
	wantOutside := []TimelineOutside{
		{Kind: "client.post", StartNS: 100, EndNS: 125},
		{Kind: "engine", Owner: 1, StartNS: 110, EndNS: 125},
		{Kind: "engine", Owner: 2, StartNS: 120, EndNS: 125},
		{Kind: "engine", Owner: 3, StartNS: 175, EndNS: 180},
		{Kind: "engine", Owner: 1, StartNS: 175, EndNS: 190},
		{Kind: "client.poll", StartNS: 175, EndNS: 195},
		{Kind: "client.status", StartNS: 195, EndNS: 200},
	}
	if got.ElapsedNS != 50 || timelineAmounts(got)["engine"] != 50 || !reflect.DeepEqual(got.Outside, wantOutside) {
		t.Fatal("intersection accounting changed source intervals", got)
	}
	for _, window := range []TimelineWindow{{StartNS: 0, EndNS: 1}, {StartNS: 9007199254740990, EndNS: 9007199254740991}} {
		got, err := BuildTimeline(nil, nil, window)
		if err != nil || got.UnresolvedNS != 1 || len(got.Segments) != 1 {
			t.Fatal("empty arithmetic vector/one-nanosecond boundary", got, err)
		}
		assertTimelineConserves(t, got)
	}
	spans, _, window = timelineVector()
	spans[0].Operation = Planning
	spans = append(spans, Span{ID: 4, Parent: 1, Operation: CaptureRead, Phase: PhaseWorker,
		StartNS: 150, EndNS: 150, Outcome: Succeeded})
	got, err = BuildTimeline(spans, nil, window)
	if err != nil || got.UnresolvedNS != 40 {
		t.Fatal("unobserved parent self/gaps were declared classified work", got, err)
	}
	assertTimelineConserves(t, got)
}

func TestCompleteTimelineRejectsInvalidEvidence(t *testing.T) {
	cases := map[string]func(*[]Span, *[]TimelineAuxSpan, *TimelineWindow){
		"empty-window":    func(_ *[]Span, _ *[]TimelineAuxSpan, w *TimelineWindow) { w.EndNS = w.StartNS },
		"negative-window": func(_ *[]Span, _ *[]TimelineAuxSpan, w *TimelineWindow) { w.StartNS = -1 },
		"unsafe-window":   func(_ *[]Span, _ *[]TimelineAuxSpan, w *TimelineWindow) { w.EndNS = math.MaxInt64 },
		"unknown-key":     func(s *[]Span, _ *[]TimelineAuxSpan, _ *TimelineWindow) { (*s)[1].Operation = "private-sentinel" },
		"duplicate-id":    func(s *[]Span, _ *[]TimelineAuxSpan, _ *TimelineWindow) { (*s)[1].ID = 1 },
		"missing-parent":  func(s *[]Span, _ *[]TimelineAuxSpan, _ *TimelineWindow) { (*s)[1].Parent = 99 },
		"cycle":           func(s *[]Span, _ *[]TimelineAuxSpan, _ *TimelineWindow) { (*s)[0].Parent = 2 },
		"bad-containment": func(s *[]Span, _ *[]TimelineAuxSpan, _ *TimelineWindow) { (*s)[1].EndNS = 195 },
		"negative-span":   func(s *[]Span, _ *[]TimelineAuxSpan, _ *TimelineWindow) { (*s)[0].StartNS = -1 },
		"unsafe-span":     func(s *[]Span, _ *[]TimelineAuxSpan, _ *TimelineWindow) { (*s)[0].EndNS = math.MaxInt64 },
		"unknown-phase":   func(s *[]Span, _ *[]TimelineAuxSpan, _ *TimelineWindow) { (*s)[1].Phase = "private-sentinel" },
		"unknown-outcome": func(s *[]Span, _ *[]TimelineAuxSpan, _ *TimelineWindow) { (*s)[1].Outcome = "private-sentinel" },
		"client-overlap":  func(_ *[]Span, a *[]TimelineAuxSpan, _ *TimelineWindow) { (*a)[1].StartNS = 124 },
		"unknown-client":  func(_ *[]Span, a *[]TimelineAuxSpan, _ *TimelineWindow) { (*a)[0].Kind = "private-sentinel" },
		"negative-client": func(_ *[]Span, a *[]TimelineAuxSpan, _ *TimelineWindow) { (*a)[0].StartNS = -1 },
		"reversed-client": func(_ *[]Span, a *[]TimelineAuxSpan, _ *TimelineWindow) { (*a)[0].EndNS = 99 },
		"unsafe-client":   func(_ *[]Span, a *[]TimelineAuxSpan, _ *TimelineWindow) { (*a)[2].EndNS = math.MaxInt64 },
		"duplicate-observer": func(_ *[]Span, a *[]TimelineAuxSpan, _ *TimelineWindow) {
			*a = append(*a, TimelineAuxSpan{Kind: "observer", StartNS: 180, EndNS: 195}, TimelineAuxSpan{Kind: "observer", StartNS: 190, EndNS: 200})
		},
		"span-quota": func(s *[]Span, _ *[]TimelineAuxSpan, _ *TimelineWindow) { *s = make([]Span, MaxSpans+1) },
		"aux-quota": func(_ *[]Span, a *[]TimelineAuxSpan, _ *TimelineWindow) {
			*a = make([]TimelineAuxSpan, MaxTimelineAuxSpans+1)
		},
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			spans, auxiliary, window := timelineVector()
			mutate(&spans, &auxiliary, &window)
			got, err := BuildTimeline(spans, auxiliary, window)
			if err == nil || err.Error() != "invalid prepare cost evidence" || !reflect.DeepEqual(got, Timeline{}) {
				t.Fatal("invalid/private/partial evidence escaped", got, err)
			}
		})
	}
}
