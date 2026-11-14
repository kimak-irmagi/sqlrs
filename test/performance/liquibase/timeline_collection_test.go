package benchmark

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// TL01/TL08 and OT-TL01: inherited selectors are not authority to instrument
// an old/off member, and failed reads must not manufacture zero-cost spans.
func TestTimelineCollectionSelectionEnvironmentAndIntervals(t *testing.T) {
	for _, row := range []struct{ cost, timeline, valid bool }{
		{false, false, true}, {true, false, true}, {true, true, true}, {false, true, false},
	} {
		if (validateTimelineSelection(row.cost, row.timeline) == nil) != row.valid {
			t.Fatal("timeline selection crossed explicit cost authority")
		}
	}
	input := []string{"PATH=keep", "SQLRS_PREPARE_COST=1", "sqlrs_prepare_timeline=1", "SQLRS_PREPARE_TIMELINE_REFERENCE=123"}
	before := append([]string(nil), input...)
	for _, selected := range []bool{false, true} {
		got := costTimelineEnvironment(input, selected, 456)
		want := []string{"PATH=keep"}
		if selected {
			want = append(want, "SQLRS_PREPARE_COST=1", "SQLRS_PREPARE_TIMELINE=1", "SQLRS_PREPARE_TIMELINE_REFERENCE=456")
		}
		if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(input, before) {
			t.Fatal("inherited measurement escaped or caller environment changed", got)
		}
	}
	var absent *costTimelineCapture
	absent.begin("client.post")()
	clock := int64(10)
	capture := &costTimelineCapture{read: func() (int64, error) { clock += 5; return clock, nil }}
	capture.begin("client.post")()
	capture.begin("client.poll")()
	if capture.err != nil || !reflect.DeepEqual(capture.intervals, []costTimelineAux{{"client.post", 15, 20}, {"client.poll", 25, 30}}) {
		t.Fatal("actual interval endpoints not retained", capture)
	}
	capture.begin("raw-private-path")()
	if !errors.Is(capture.err, errCostStream) || len(capture.intervals) != 2 {
		t.Fatal("arbitrary label exported")
	}
	for _, failAt := range []int{1, 2} {
		reads := 0
		failed := &costTimelineCapture{read: func() (int64, error) {
			reads++
			if reads == failAt {
				return 0, errors.New("private clock failure")
			}
			return 100, nil
		}}
		failed.begin("client.status")()
		if !errors.Is(failed.err, errCostStream) || len(failed.intervals) != 0 || strings.Contains(failed.err.Error(), "private") {
			t.Fatal("clock failure qualified or leaked")
		}
	}
}

// TL02/TL03: qualification needs a real request, admission and worker on the
// selected axis; a worker-only report remains valid only in its old mode.
func TestTimelineCollectionQualifiesRequestAndClientEvidence(t *testing.T) {
	s := newCostStream()
	if err := s.consume(strings.NewReader(costDetailStreamExample(1))); err != nil {
		t.Fatal(err)
	}
	root, err := s.await(t.Context(), 1)
	if err != nil {
		t.Fatal(err)
	}
	// Retain the real detail grammar while adding the request/admission tree.
	for i := range root.Spans {
		root.Spans[i].ID += 2
		if root.Spans[i].Parent != 0 {
			root.Spans[i].Parent += 2
		} else {
			root.Spans[i].Parent = 1
		}
	}
	for i := range root.Helpers {
		root.Helpers[i].Owner += 2
	}
	root.Spans = append([]costSpan{
		{ID: 1, Operation: "prepare.request", Phase: "worker", Outcome: "succeeded", EndNS: root.ElapsedNS},
		{ID: 2, Parent: 1, Operation: "prepare.admission", Phase: "admission", Outcome: "succeeded", EndNS: 1},
	}, root.Spans...)
	root.Declared = len(root.Spans)
	root.Clock = &costTimelineClock{Domain: "linux-monotonic-relative-v1", OriginNS: 101}
	root.ObserverWindow = &costTimelineWindow{201, 205}
	timer := costTimelineTimer{Window: costTimelineWindow{100, 202}, StartBracket: costTimelineWindow{99, 101}, EndBracket: costTimelineWindow{201, 203}, APIElapsedNS: 102}
	aux := []costTimelineAux{{"client.post", 100, 120}, {"client.status", 120, 202}}
	if err := qualifyTimelinePass(root, 1, timer, aux, 1); err != nil {
		t.Fatal("complete request rejected", err)
	}
	for _, mutate := range []func(*costRoot){
		func(r *costRoot) { r.Clock = nil },
		func(r *costRoot) { r.Clock.Domain = "calendar" },
		func(r *costRoot) { r.ObserverWindow = nil },
		func(r *costRoot) { r.Spans[0].Operation = "prepare.worker" },
		func(r *costRoot) { r.Spans[1].Operation = "prepare.worker" },
		func(r *costRoot) { r.Spans[2].Outcome = "failed" },
		func(r *costRoot) { r.Spans[2].Parent = 2 },
	} {
		copy := root
		clock := *root.Clock
		copy.Clock = &clock
		copy.Spans = append([]costSpan(nil), root.Spans...)
		mutate(&copy)
		if qualifyTimelinePass(copy, 1, timer, aux, 1) == nil {
			t.Fatal("partial or wrong execution qualified")
		}
	}
	for _, bad := range [][]costTimelineAux{nil, {{"client.post", 100, 203}}, {{"client.post", 100, 120}, {"client.status", 119, 202}}, {{"private", 100, 202}}} {
		if qualifyTimelinePass(root, 1, timer, bad, 1) == nil {
			t.Fatal("invalid client window qualified")
		}
	}
}
