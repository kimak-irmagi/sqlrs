package preparecost

import (
	"math"
	"reflect"
	"testing"
)

func costSpan(id, parent int, op Operation, start, end int64) Span {
	return Span{ID: id, Parent: parent, Operation: op, Phase: PhaseStep, Step: 1,
		StartNS: start, EndNS: end, Outcome: Succeeded}
}

// CT11: these expected values are arithmetic, not another invocation of the
// product aggregator. Overlapping calls cannot inflate elapsed-time attribution.
// docs/architecture/issue89-cost-attribution-and-trimming.md §4.
func TestAccountingIndependentOverlapOracle(t *testing.T) {
	const ms = int64(1_000_000)
	spans := []Span{costSpan(1, 0, Worker, 0, 80*ms), costSpan(2, 1, CaptureRead, 10*ms, 30*ms), costSpan(3, 1, CaptureRead, 20*ms, 40*ms)}
	before := append([]Span(nil), spans...)
	got, err := Summarize(spans, 100*ms)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(spans, before) {
		t.Fatal("accounting mutated raw observations")
	}
	if got.AssignedNS != 80*ms || got.SharedNS != 10*ms || got.UnassignedNS != 20*ms || len(got.Rows) != 2 {
		t.Fatalf("bad conserved overview: %+v", got)
	}
	var capture, worker Row
	for _, row := range got.Rows {
		if row.Operation == CaptureRead {
			capture = row
		} else if row.Operation == Worker {
			worker = row
		}
	}
	if capture.Count != 2 || capture.SumNS != 40*ms || capture.UnionNS != 30*ms || capture.SelfNS != 30*ms ||
		capture.ExclusiveNS != 20*ms || capture.MeanPerCallMS != 20 || capture.RatioPercent != 30 ||
		!reflect.DeepEqual(capture.CallNS, []int64{20 * ms, 20 * ms}) {
		t.Fatalf("raw call sum confused with prepare union: %+v", capture)
	}
	if worker.SelfNS != 50*ms || worker.ExclusiveNS != 50*ms || worker.UnionNS != 80*ms || worker.RatioPercent != 80 {
		t.Fatalf("parent counted its children twice: %+v", worker)
	}
	if capture.ExclusiveNS+worker.ExclusiveNS+got.SharedNS+got.UnassignedNS != 100*ms {
		t.Fatal("additive overview is not 100 ms")
	}
}

func TestAccountingStepAndOncePerJobWork(t *testing.T) {
	spans := []Span{costSpan(1, 0, Worker, 0, 80), costSpan(2, 1, ImageInventory, 0, 10), costSpan(3, 1, CaptureRead, 10, 30), costSpan(4, 1, CaptureRead, 30, 50)}
	spans[0].Phase, spans[0].Step = PhaseWorker, 0
	spans[1].Phase, spans[1].Step = PhaseStartup, 0
	spans[3].Step = 2
	got, err := Summarize(spans, 100)
	if err != nil {
		t.Fatal(err)
	}
	if got.SharedNS != 0 || got.AssignedNS != 80 || got.UnassignedNS != 20 || len(got.Rows) != 4 {
		t.Fatalf("adjacent steps/startup overlap or disappeared: %+v", got)
	}
	for _, row := range got.Rows {
		if row.Operation == ImageInventory && (row.Count != 1 || row.Step != 0 || row.Phase != PhaseStartup) {
			t.Fatal("once-per-job work spread across steps")
		}
		if row.Operation == CaptureRead && (row.Count != 1 || row.UnionNS != 20 || row.Step < 1 || row.Step > 2) {
			t.Fatal("different steps combined")
		}
	}
	for _, input := range [][]Span{nil, {costSpan(1, 0, CaptureRead, 5, 5)}} {
		got, err := Summarize(input, 10)
		if err != nil || got.AssignedNS != 0 || got.SharedNS != 0 || got.UnassignedNS != 10 {
			t.Fatal("empty/cached/zero-duration time was invented", got, err)
		}
		if len(input) != 0 && (got.Rows[0].Count != 1 || got.Rows[0].UnionNS != 0) {
			t.Fatal("a measured zero was treated as an absent call")
		}
	}
}

func TestAccountingNestedSiblingAndUnorderedInput(t *testing.T) {
	spans := []Span{costSpan(1, 0, Worker, 0, 100), costSpan(2, 1, CaptureRead, 10, 80), costSpan(3, 2, CaptureRead, 20, 30), costSpan(4, 2, PruneObserve, 30, 40)}
	spans[1].Outcome = Failed
	spans[2].Outcome = Cancelled
	spans[3].Outcome = Panicked
	a, err := Summarize(spans, 100)
	if err != nil {
		t.Fatal(err)
	}
	spans[0], spans[3] = spans[3], spans[0]
	b, err := Summarize(spans, 100)
	if err != nil || !reflect.DeepEqual(a, b) {
		t.Fatal("raw iteration order changed accounting", err)
	}
	if a.SharedNS != 0 || a.UnassignedNS != 0 {
		t.Fatal("ancestor/descendant work counted as concurrent", a)
	}
	var exclusive int64
	for _, row := range a.Rows {
		exclusive += row.ExclusiveNS
	}
	if exclusive != 100 {
		t.Fatal("nested self-time does not conserve prepare time", a)
	}
}

func TestAccountingRefusesInvalidEvidence(t *testing.T) {
	for _, variant := range []string{"api-zero", "api-negative", "id-zero", "duplicate", "missing-parent", "cycle", "outside-parent", "negative-start", "reversed", "outside-api", "operation", "phase", "negative-step", "step-without-ordinal", "startup-with-step", "foreign-step", "outcome", "sum-overflow", "too-many"} {
		t.Run(variant, func(t *testing.T) {
			api := int64(100)
			spans := []Span{costSpan(1, 0, Worker, 0, 80), costSpan(2, 1, CaptureRead, 10, 30)}
			switch variant {
			case "api-zero":
				api = 0
			case "api-negative":
				api = -1
			case "id-zero":
				spans[1].ID = 0
			case "duplicate":
				spans[1].ID = 1
			case "missing-parent":
				spans[1].Parent = 99
			case "cycle":
				spans[0].Parent = 2
				spans[1].StartNS, spans[1].EndNS = 0, 80
			case "outside-parent":
				spans[1].EndNS = 90
			case "negative-start":
				spans[0].StartNS = -1
			case "reversed":
				spans[1].StartNS = 40
			case "outside-api":
				spans[0].EndNS = 101
			case "operation":
				spans[1].Operation = "private-unknown-operation"
			case "phase":
				spans[1].Phase = "private-unknown-phase"
			case "negative-step":
				spans[1].Step = -1
			case "step-without-ordinal":
				spans[1].Step = 0
			case "startup-with-step":
				spans[1].Phase = PhaseStartup
			case "foreign-step":
				spans[1].Step = 2
			case "outcome":
				spans[1].Outcome = "private-error-details"
			case "sum-overflow":
				api = math.MaxInt64
				spans = []Span{costSpan(1, 0, CaptureRead, 0, api), costSpan(2, 0, CaptureRead, 0, api)}
			case "too-many":
				spans = make([]Span, MaxSpans+1)
			}
			if got, err := Summarize(spans, api); err == nil || !reflect.DeepEqual(got, Summary{}) {
				t.Fatalf("invalid evidence returned attribution: %+v err=%v", got, err)
			}
		})
	}
}
