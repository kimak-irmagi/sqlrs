package benchmark

import (
	"context"
	"strings"
	"testing"
)

func timelineStreamExample() string {
	return "sqlrs-prepare-cost-v1 begin 1 3 100 1\n" +
		"sqlrs-prepare-cost-v1 clock 1 linux-monotonic-relative-v1 1000\n" +
		"sqlrs-prepare-cost-v1 span 1 1 0 prepare.request worker 0 0 100 succeeded\n" +
		"sqlrs-prepare-cost-v1 span 1 2 1 prepare.admission admission 0 0 20 succeeded\n" +
		"sqlrs-prepare-cost-v1 span 1 3 1 prepare.worker worker 0 10 100 succeeded\n" +
		"sqlrs-prepare-cost-v1 end 1 3\n" +
		"sqlrs-prepare-cost-v1 observer 1 2 3 4\n" +
		"sqlrs-prepare-cost-v1 observer-window 1 1101 1110\n"
}

// TL02: literal frames exercise the independent collector, not the engine's
// serializer. Legacy roots remain qualified only by their original contract.
func TestTimelineStreamClockObserverAndOwnership(t *testing.T) {
	s := newCostStream()
	if err := s.consume(strings.NewReader(timelineStreamExample())); err != nil {
		t.Fatal(err)
	}
	root, err := s.awaitTimeline(t.Context(), 1)
	if err != nil || root.Clock == nil || root.Clock.OriginNS != 1000 || root.ObserverWindow == nil ||
		root.ObserverWindow.StartNS != 1101 || root.ObserverWindow.EndNS != 1110 {
		t.Fatal("closed timeline evidence differs from literal oracle", err)
	}
	root.Clock.OriginNS, root.ObserverWindow.StartNS = 0, 0
	again, err := s.awaitTimeline(t.Context(), 1)
	if err != nil || again.Clock.OriginNS != 1000 || again.ObserverWindow.StartNS != 1101 {
		t.Fatal("timeline snapshots alias")
	}
	if qualifyCostPass(again, 1, 200, 1) == nil {
		t.Fatal("request-root report masqueraded as legacy worker evidence")
	}
	legacy := newCostStream()
	if err := legacy.consume(strings.NewReader(costDetailStreamExample(1))); err != nil {
		t.Fatal(err)
	}
	if _, err := legacy.awaitTimeline(t.Context(), 1); err == nil {
		t.Fatal("legacy evidence became a complete timeline")
	}
	if root, err := legacy.awaitMeasured(t.Context(), 1); err != nil || qualifyCostPass(root, 1, 100, 1) != nil {
		t.Fatal("legacy behavior changed", err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := s.awaitTimeline(ctx, 1); err != context.Canceled {
		t.Fatal("waiting changed caller cancellation", err)
	}
}

func TestTimelineStreamRejectsBrokenClockFrames(t *testing.T) {
	base := timelineStreamExample()
	clock := "sqlrs-prepare-cost-v1 clock 1 linux-monotonic-relative-v1 1000\n"
	observer := "sqlrs-prepare-cost-v1 observer-window 1 1101 1110\n"
	for name, wire := range map[string]string{
		"missing-clock":             strings.Replace(base, clock, "", 1),
		"missing-observer-window":   strings.Replace(base, observer, "", 1),
		"duplicate-clock":           strings.Replace(base, clock, clock+clock, 1),
		"duplicate-observer-window": base + observer,
		"wrong-domain":              strings.Replace(base, "linux-monotonic-relative-v1", "private-sentinel", 1),
		"negative-origin":           strings.Replace(base, "v1 1000\n", "v1 -1\n", 1),
		"noncanonical-origin":       strings.Replace(base, "v1 1000\n", "v1 01000\n", 1),
		"unsafe-origin":             strings.Replace(base, "v1 1000\n", "v1 9007199254740991\n", 1),
		"late-clock":                strings.Replace(base, clock, "", 1) + clock,
		"old-root":                  strings.Replace(base, "prepare.request worker", "prepare.worker worker", 1),
		"early-observer":            strings.Replace(base, "window 1 1101 1110", "window 1 1099 1110", 1),
		"reversed-observer":         strings.Replace(base, "window 1 1101 1110", "window 1 1110 1101", 1),
		"unsafe-observer":           strings.Replace(base, "window 1 1101 1110", "window 1 1101 9007199254740992", 1),
		"unended-observer":          strings.Replace(base, "sqlrs-prepare-cost-v1 end 1 3\n", "", 1),
		"unknown-field":             strings.Replace(base, observer, strings.TrimSuffix(observer, "\n")+" private-sentinel\n", 1),
	} {
		t.Run(name, func(t *testing.T) {
			s := newCostStream()
			err := s.consume(strings.NewReader(wire))
			if err == nil {
				_, err = s.awaitTimeline(t.Context(), 1)
			}
			if err == nil || strings.Contains(err.Error(), "private-sentinel") {
				t.Fatal("incomplete/private evidence qualified", err)
			}
		})
	}
}
