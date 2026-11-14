package preparecost

import (
	"math"
	"testing"
)

// CT07/CT11: quotas bound the algorithm, not just a nominal record format.
// Maximal valid evidence and maximal arithmetic must not overflow or disappear.
func TestAccountingExactQuotaAndIntegerLimit(t *testing.T) {
	spans := make([]Span, MaxSpans)
	for index := range spans {
		spans[index] = costSpan(index+1, 0, CaptureRead, 0, 1)
	}
	got, err := Summarize(spans, 2)
	if err != nil || got.AssignedNS != 1 || got.SharedNS != 1 || got.UnassignedNS != 1 ||
		len(got.Rows) != 1 || got.Rows[0].Count != MaxSpans || got.Rows[0].SumNS != MaxSpans ||
		got.Rows[0].UnionNS != 1 || got.Rows[0].ExclusiveNS != 0 {
		t.Fatal("exact quota rejected or counted overlapping calls as elapsed", err)
	}
	got, err = Summarize([]Span{costSpan(1, 0, Worker, 0, math.MaxInt64)}, math.MaxInt64)
	if err != nil || got.AssignedNS != math.MaxInt64 || got.UnassignedNS != 0 || got.Rows[0].RatioPercent != 100 ||
		math.IsInf(got.Rows[0].MeanPerCallMS, 0) || math.IsNaN(got.Rows[0].MeanPerCallMS) {
		t.Fatal("maximal valid interval overflowed", err)
	}
}

func TestAccountingSeparatedPhasesAndGaps(t *testing.T) {
	startup := costSpan(2, 0, CaptureRead, 10, 20)
	startup.Phase, startup.Step = PhaseStartup, 0
	cleanup := costSpan(1, 0, CaptureRead, 40, 50)
	cleanup.Phase, cleanup.Step = PhaseCleanup, 0
	got, err := Summarize([]Span{startup, cleanup}, 100)
	if err != nil || got.AssignedNS != 20 || got.UnassignedNS != 80 || got.SharedNS != 0 || len(got.Rows) != 2 {
		t.Fatal("gaps/once-per-job phases were attributed to steps", err)
	}
	if got.Rows[0].Phase != PhaseCleanup || got.Rows[1].Phase != PhaseStartup {
		t.Fatal("phase ordering not deterministic")
	}
	for _, row := range got.Rows {
		if row.Count != 1 || row.UnionNS != 10 || row.SelfNS != 10 || row.ExclusiveNS != 10 || row.RatioPercent != 10 {
			t.Fatal("separate phases were combined", row)
		}
	}
}

func TestAccountingRepeatedNestedKeySelfTime(t *testing.T) {
	spans := []Span{costSpan(1, 0, Worker, 0, 100), costSpan(2, 1, CaptureRead, 10, 80),
		costSpan(3, 2, CaptureRead, 20, 30), costSpan(4, 2, PruneObserve, 30, 40)}
	got, err := Summarize(spans, 100)
	if err != nil {
		t.Fatal(err)
	}
	for _, row := range got.Rows {
		if row.Operation == CaptureRead && (row.Count != 2 || row.SumNS != 80 || row.UnionNS != 70 || row.SelfNS != 60 || row.ExclusiveNS != 60) {
			t.Fatal("same-key nesting double-counted parent/child time", row)
		}
	}
}
