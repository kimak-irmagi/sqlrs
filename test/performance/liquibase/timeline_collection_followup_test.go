package benchmark

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

// LC02 uses complete helper evidence as a positive control before independently
// corrupting timer, tree, or client evidence (complete-timeline-tests §LC01–LC03).
func TestTimelineCollectionRejectsIndependentPartialEvidence(t *testing.T) {
	fixture := func(t *testing.T) (costRoot, costTimelineTimer, []costTimelineAux) {
		t.Helper()
		stream := newCostStream()
		if err := stream.consume(strings.NewReader(costDetailStreamExample(1))); err != nil {
			t.Fatal(err)
		}
		root, err := stream.await(t.Context(), 1)
		if err != nil {
			t.Fatal(err)
		}
		for i := range root.Spans {
			root.Spans[i].ID += 2
			if root.Spans[i].Parent == 0 {
				root.Spans[i].Parent = 1
			} else {
				root.Spans[i].Parent += 2
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
		return root, timer, []costTimelineAux{{"client.post", 100, 120}, {"client.status", 120, 202}}
	}
	for _, row := range []struct {
		name   string
		mutate func(*costRoot, *costTimelineTimer, *[]costTimelineAux)
	}{
		{"timer", func(_ *costRoot, timer *costTimelineTimer, _ *[]costTimelineAux) {
			timer.Window.EndNS = timer.Window.StartNS - 1
		}},
		{"span-range", func(root *costRoot, _ *costTimelineTimer, _ *[]costTimelineAux) { root.Spans[3].StartNS = -1 }},
		{"parent-containment", func(root *costRoot, _ *costTimelineTimer, _ *[]costTimelineAux) {
			root.Spans[2].StartNS = 1
			root.Spans[3].StartNS = 0
		}},
		{"admission-phase", func(root *costRoot, _ *costTimelineTimer, _ *[]costTimelineAux) { root.Spans[1].Phase = "worker" }},
		{"duplicate-post", func(_ *costRoot, _ *costTimelineTimer, aux *[]costTimelineAux) {
			*aux = []costTimelineAux{{"client.post", 100, 110}, {"client.post", 110, 120}, {"client.status", 120, 202}}
		}},
		{"missing-status", func(_ *costRoot, _ *costTimelineTimer, aux *[]costTimelineAux) { (*aux)[1].Kind = "client.poll" }},
	} {
		t.Run(row.name, func(t *testing.T) {
			root, timer, aux := fixture(t)
			if err := qualifyTimelinePass(root, 1, timer, aux, 1); err != nil {
				t.Fatal("complete control rejected", err)
			}
			originalTimer := timer
			row.mutate(&root, &timer, &aux)
			if err := qualifyTimelinePass(root, 1, timer, aux, 1); !errors.Is(err, errCostStream) || err.Error() != errCostStream.Error() {
				t.Fatal("partial evidence did not return the closed refusal", err)
			}
			if row.name != "timer" && timer != originalTimer {
				t.Fatal("evidence validation changed the original API timer")
			}
		})
	}
}

// LC01: poisoned evidence and the finite client quota stop observation without
// reading another clock or appending a misleading successful interval.
func TestTimelineCollectionPoisonAndQuotaStopObservation(t *testing.T) {
	for _, quota := range []bool{false, true} {
		reads := 0
		capture := &costTimelineCapture{read: func() (int64, error) { reads++; return 100, nil }}
		if quota {
			capture.intervals = make([]costTimelineAux, 8192)
		} else {
			capture.err = errCostStream
			capture.intervals = []costTimelineAux{{"client.post", 1, 2}}
		}
		before := append([]costTimelineAux(nil), capture.intervals...)
		capture.begin("client.status")()
		capture.begin("client.poll")()
		if reads != 0 || !errors.Is(capture.err, errCostStream) || !reflect.DeepEqual(capture.intervals, before) {
			t.Fatal("poisoned/quota evidence continued observation", quota, reads)
		}
	}
}
