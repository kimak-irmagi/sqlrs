package preparecost

import (
	"strings"
	"testing"
)

func timelineWireReport() Report {
	return Report{Version: 1, Root: 7, ElapsedNS: 100, Complete: true,
		Clock: &TimelineClockFrame{Domain: "linux-monotonic-relative-v1", OriginNS: 1000},
		Spans: []Span{
			{ID: 1, Operation: Request, Phase: PhaseWorker, EndNS: 100, Outcome: Succeeded},
			{ID: 2, Parent: 1, Operation: Worker, Phase: PhaseWorker, StartNS: 10, EndNS: 90, Outcome: Succeeded},
		}}
}

// TL02 keeps the original wire version/legacy frames; the selected complete
// mode adds a closed relative clock frame and a different explicitly qualified root.
func TestTimelineWireClosedClockAndRoot(t *testing.T) {
	data, err := EncodeReport(timelineWireReport())
	want := "sqlrs-prepare-cost-v1 begin 7 2 100 1\n" +
		"sqlrs-prepare-cost-v1 clock 7 linux-monotonic-relative-v1 1000\n" +
		"sqlrs-prepare-cost-v1 span 7 1 0 prepare.request worker 0 0 100 succeeded\n" +
		"sqlrs-prepare-cost-v1 span 7 2 1 prepare.worker worker 0 10 90 succeeded\n" +
		"sqlrs-prepare-cost-v1 end 7 2\n"
	if err != nil || string(data) != want {
		t.Fatal("complete wire differs from literal oracle", err)
	}
	for _, mutate := range []func(*Report){
		func(r *Report) { r.Clock.Domain = "private-sentinel" },
		func(r *Report) { r.Clock.OriginNS = -1 },
		func(r *Report) { r.Clock.OriginNS = 9007199254740991 },
		func(r *Report) { r.Spans[0].Operation = Worker },
		func(r *Report) { r.Spans[0].Parent = 2 },
		func(r *Report) { r.Spans = nil },
	} {
		report := timelineWireReport()
		mutate(&report)
		data, err := EncodeReport(report)
		if err == nil || data != nil || strings.Contains(err.Error(), "private-sentinel") {
			t.Fatal("invalid clock/root escaped", err)
		}
	}
	report := timelineWireReport()
	report.Complete, report.Clock.Domain = false, "private-sentinel"
	data, err = EncodeReport(report)
	if err != nil || string(data) != "sqlrs-prepare-cost-v1 begin 7 0 100 0\nsqlrs-prepare-cost-v1 end 7 0\n" {
		t.Fatal("incomplete timeline leaked a partial clock/root", err)
	}
	data, err = EncodeReport(wireCostReport())
	if err != nil || strings.Contains(string(data), " clock ") {
		t.Fatal("legacy report acquired a timeline frame", err)
	}
}

func TestTimelineRecorderMissingClockIsOnlyIncompleteEvidence(t *testing.T) {
	r := NewTimelineRecorder(nil, 8)
	ctx, end := Start(WithRecorder(t.Context(), r), Request)
	_, child := Start(ctx, Worker)
	child(Failed)
	end(Failed)
	report := r.Finish()
	if report.Complete || report.Root != 8 {
		t.Fatal("absent shared clock became qualified")
	}
	data, err := EncodeReport(report)
	if err != nil || strings.Contains(string(data), " span ") {
		t.Fatal("incomplete measurement lost its safe framing", err)
	}
}
