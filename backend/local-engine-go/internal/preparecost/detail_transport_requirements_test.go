package preparecost

import (
	"bytes"
	"context"
	"errors"
	"io"
	"math"
	"reflect"
	"strconv"
	"strings"
	"testing"
)

// DT01: only the host's own invocation scope can own helper-local evidence.
// Zero is a real quantised observation, not an unavailable detail.
func TestHelperObservationOwnershipAndCopies(t *testing.T) {
	r := NewRecorder(true, 7)
	root, endRoot := Start(WithRecorder(t.Context(), r), Worker)
	for step, operation := range []Operation{PlanningInvocation, Invocation} {
		ctx := WithPhase(root, PhasePlanning, 0)
		if step > 0 {
			ctx = WithPhase(root, PhaseStep, step)
		}
		owner, end := Start(ctx, operation)
		input := []byte(helperCostFixture(make([]int64, 9)))
		RecordHelper(owner, input, 0)
		input[0] = '!'
		end(Succeeded)
	}
	endRoot(Succeeded)
	first := r.Finish()
	if !first.Complete || len(first.Helpers) != 2 {
		t.Fatal("host-owned helper detail missing")
	}
	for index, detail := range first.Helpers {
		if detail.Owner != index+2 || !detail.Available || detail.HostElapsedNS != 0 || len(detail.Detail.Regions) != 8 || detail.Detail.TotalNS != 0 {
			t.Fatal("zero/local helper observation lost its owner or completeness")
		}
	}
	before := r.Finish()
	first.Helpers[0].Detail.Regions[0].Phase = "private-mutation"
	first.Helpers[1].Owner = 99
	if !reflect.DeepEqual(before, r.Finish()) {
		t.Fatal("helper snapshot aliases caller storage")
	}
	if n := testing.AllocsPerRun(100, func() { RecordHelper(context.Background(), nil, 0) }); n != 0 {
		t.Fatal("disabled helper observation allocated", n)
	}
}

func TestHelperObservationRefusesInvalidEvidence(t *testing.T) {
	for _, variant := range []string{"missing", "malformed", "oversized", "negative", "duration", "duplicate", "late", "wrong-kind", "missing-owner", "foreign-owner"} {
		t.Run(variant, func(t *testing.T) {
			r := NewRecorder(true, 1)
			root, endRoot := Start(WithRecorder(t.Context(), r), Worker)
			owner, end := Start(WithPhase(root, PhaseStep, 1), Invocation)
			data, elapsed := []byte(helperCostFixture(make([]int64, 9))), int64(0)
			switch variant {
			case "missing":
				data = nil
			case "malformed":
				data = []byte("private-marker\n")
			case "oversized":
				data = bytes.Repeat([]byte("x"), 4097)
			case "negative":
				elapsed = -1
			case "duration":
				elapsed = math.MaxInt64
			case "duplicate":
				RecordHelper(owner, data, elapsed)
			case "late":
				end(Succeeded)
				endRoot(Succeeded)
				r.Finish()
			case "wrong-kind":
				owner = root
			case "missing-owner":
				owner = WithRecorder(t.Context(), r)
			case "foreign-owner":
				owner = context.WithValue(owner, scopeKey{}, scope{recorder: r, parent: MaxSpans + 1})
			}
			RecordHelper(owner, data, elapsed)
			if variant != "late" {
				end(Succeeded)
				endRoot(Succeeded)
			}
			report := r.Finish()
			if variant == "late" || variant == "wrong-kind" || variant == "missing-owner" || variant == "foreign-owner" {
				if report.Complete {
					t.Fatal("invalid owner/late write qualified")
				}
			} else if !report.Complete || len(report.Helpers) != 1 || report.Helpers[0].Available {
				t.Fatal("invalid detail changed host evidence or became measured zero")
			}
			wire, err := EncodeReport(report)
			if err != nil || bytes.Contains(wire, []byte("private-marker")) {
				t.Fatal("rejected detail leaked into host transport", err)
			}
		})
	}
}

func detailWireReport(t *testing.T) Report {
	t.Helper()
	report := wireCostReport()
	report.Spans[1].Operation = Invocation
	detail, err := ParseHelperDetail([]byte(helperCostFixture(make([]int64, 9))), 0)
	if err != nil {
		t.Fatal(err)
	}
	report.Helpers = []HelperObservation{{Owner: 2, Available: true, Detail: detail}}
	return report
}

// DT03: expected bytes are literal grammar, not another encoder's output.
func TestHelperWireLiteralGrammarAndRefusals(t *testing.T) {
	report := detailWireReport(t)
	wire, err := EncodeReport(report)
	want := ""
	for _, phase := range []string{"preflight", "cli", "drain", "stream_flush", "hash_rename", "renamed_flush", "manifest_flush", "final_flush"} {
		want += "sqlrs-prepare-cost-v1 helper 7 2 " + phase + " 0 0 0\n"
	}
	if err != nil || !strings.Contains(string(wire), want+"sqlrs-prepare-cost-v1 end 7 2\n") {
		t.Fatal("helper literal wire differs", err)
	}
	for _, variant := range []string{"owner", "kind", "duplicate", "negative", "containment", "phases", "phase", "start", "duration", "resolution", "uncertainty", "total", "unavailable-payload"} {
		t.Run(variant, func(t *testing.T) {
			copy := detailWireReport(t)
			observation := &copy.Helpers[0]
			switch variant {
			case "owner":
				observation.Owner = 0
			case "kind":
				observation.Owner = 1
			case "duplicate":
				copy.Helpers = append(copy.Helpers, *observation)
			case "negative":
				observation.HostElapsedNS = -1
			case "containment":
				observation.HostElapsedNS = 21
			case "phases":
				observation.Detail.Regions = observation.Detail.Regions[:7]
			case "phase":
				observation.Detail.Regions[0].Phase = "private-marker"
			case "start":
				observation.Detail.Regions[0].StartCS = 1
			case "duration":
				observation.Detail.Regions[0].DurationNS = 1
			case "resolution":
				observation.Detail.ResolutionNS++
			case "uncertainty":
				observation.Detail.CumulativeUncertaintyNS++
			case "total":
				observation.Detail.TotalNS++
			case "unavailable-payload":
				observation.Available = false
			}
			if data, err := EncodeReport(copy); err == nil || data != nil || strings.Contains(err.Error(), "private-marker") {
				t.Fatal("invalid detail produced partial/private bytes")
			}
		})
	}
	report.Helpers[0] = HelperObservation{Owner: 2}
	if data, err := EncodeReport(report); err != nil || !strings.Contains(string(data), "helper 7 2 unavailable 0\n") {
		t.Fatal("unavailable observation lost explicit marker", err)
	}
	report.Complete = false
	report.Helpers[0].Owner = -1
	if data, err := EncodeReport(report); err != nil || strings.Contains(string(data), "helper") {
		t.Fatal("incomplete host report exposed helper payload", err)
	}
}

type detailFaultWriter struct {
	calls, failAt int
	short         bool
	chunks        [][]byte
}

func (w *detailFaultWriter) Write(data []byte) (int, error) {
	w.calls++
	w.chunks = append(w.chunks, bytes.Clone(data))
	if w.calls == w.failAt {
		if w.short {
			return len(data) - 1, nil
		}
		return 0, errors.New("private-writer-marker")
	}
	return len(data), nil
}

func TestObserverFollowsSuccessfulFramedWrite(t *testing.T) {
	for _, position := range []int{0, 1, 2} {
		for _, short := range []bool{false, true} {
			writer := &detailFaultWriter{failAt: position, short: short}
			err := NewEmitter(writer).Emit(wireCostReport())
			if (err == nil) != (position == 0) || writer.calls != map[bool]int{true: 1, false: 2}[position == 1] {
				t.Fatal("emitter confirmed failed delivery or emitted after first refusal")
			}
			if err != nil && strings.Contains(err.Error(), "private-writer-marker") {
				t.Fatal("writer error leaked")
			}
			if writer.calls == 2 {
				fields := strings.Fields(string(writer.chunks[1]))
				if len(fields) != 6 || strings.Join(fields[:3], " ") != "sqlrs-prepare-cost-v1 observer 7" {
					t.Fatal("observer is not the second closed write")
				}
				for _, value := range fields[3:] {
					number, err := strconv.ParseInt(value, 10, 64)
					if err != nil || number < 0 {
						t.Fatal("invalid observer duration")
					}
				}
			}
		}
	}
	var writer io.Writer = &detailFaultWriter{}
	if err := NewEmitter(writer).Emit(detailWireReport(t)); err != nil {
		t.Fatal(err)
	}
}
