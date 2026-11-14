package preparecost

import (
	"context"
	"math"
	"strings"
	"testing"
)

func stepResultCostLiteral() []byte {
	return []byte("sqlrs-lb-cost-v1 preflight 0 0\n" +
		"sqlrs-lb-cost-v1 cli 0 0\n" +
		"sqlrs-lb-cost-v1 drain 0 0\n" +
		"sqlrs-lb-cost-v1 stream_flush 0 0\n" +
		"sqlrs-lb-cost-v1 hash_rename 0 0\n" +
		"sqlrs-lb-cost-v1 renamed_flush 0 0\n" +
		"sqlrs-lb-cost-v1 manifest_flush 0 0\n" +
		"sqlrs-lb-cost-v1 final_flush 0 0\n" +
		"sqlrs-lb-cost-v1 result_prepare 0 0\n" +
		"sqlrs-lb-cost-v1 result_emit 0 0\n")
}

// IR06: ten phases are explicitly selected, never guessed by a legacy parser.
func TestStepResultHelperDetailIsExplicitAndBounded(t *testing.T) {
	data := stepResultCostLiteral()
	detail, err := ParseStepResultHelperDetail(data, 0)
	if err != nil || len(detail.Regions) != 10 || detail.TotalNS != 0 || detail.CumulativeUncertaintyNS != 100_000_000 || detail.ResolutionNS != 10_000_000 {
		t.Fatal("result helper regions changed", detail, err)
	}
	if _, err := ParseHelperDetail(data, 0); err == nil {
		t.Fatal("legacy parser accepted extended phases")
	}
	legacy := []byte(helperCostFixture(make([]int64, 9)))
	if _, err := ParseStepResultHelperDetail(legacy, 0); err == nil {
		t.Fatal("result parser accepted incomplete phases")
	}
	for _, bad := range [][]byte{nil, []byte("private-detail-marker"), data[:len(data)-1],
		[]byte(strings.Replace(string(data), "result_prepare 0 0", "result_prepare 1 1", 1)),
		[]byte(strings.Replace(string(data), "result_emit 0 0", "result_prepare 0 0", 1)),
		[]byte(strings.Replace(string(data), "result_emit 0 0", "result_emit 0 922337203686", 1)),
		[]byte(strings.Repeat("x", 4097))} {
		if _, err := ParseStepResultHelperDetail(bad, math.MaxInt64); err == nil {
			t.Fatal("invalid detail accepted")
		}
	}
	long := []byte(strings.Replace(string(data), "result_emit 0 0", "result_emit 0 2", 1))
	if _, err := ParseStepResultHelperDetail(long, 10_000_000); err != nil {
		t.Fatal("quantisation boundary refused", err)
	}
	if _, err := ParseStepResultHelperDetail(long, 9_999_999); err == nil {
		t.Fatal("host bound ignored")
	}
}

func TestStepResultHelperOwnerAndPrivacy(t *testing.T) {
	for _, operation := range []Operation{Invocation, PlanningInvocation, Worker} {
		r := NewRecorder(true, 1)
		owner, end := Start(WithRecorder(t.Context(), r), operation)
		RecordStepResultHelper(owner, stepResultCostLiteral(), 0)
		end(Succeeded)
		report := r.Finish()
		if operation != Invocation {
			if report.Complete {
				t.Fatal("result helper bound to a non-step owner")
			}
			continue
		}
		if !report.Complete || len(report.Helpers) != 1 || !report.Helpers[0].Available || len(report.Helpers[0].Detail.Regions) != 10 {
			t.Fatal("step result owner missing")
		}
		wire, err := EncodeReport(report)
		if err != nil || !strings.Contains(string(wire), "result_emit") || strings.Contains(string(wire), "private-") {
			t.Fatal("extended numeric report refused/leaked", err)
		}
	}
	for _, input := range [][]byte{nil, []byte("private-marker"), []byte(strings.Repeat("x", 4097))} {
		r := NewRecorder(true, 1)
		owner, end := Start(WithRecorder(t.Context(), r), Invocation)
		RecordStepResultHelper(owner, input, 0)
		end(Succeeded)
		report := r.Finish()
		if !report.Complete || len(report.Helpers) != 1 || report.Helpers[0].Available {
			t.Fatal("diagnostics changed work/availability")
		}
		wire, err := EncodeReport(report)
		if err != nil || strings.Contains(string(wire), "private-marker") {
			t.Fatal("invalid diagnostics leaked", err)
		}
	}
	if n := testing.AllocsPerRun(50, func() { RecordStepResultHelper(context.Background(), nil, 0) }); n != 0 {
		t.Fatal("disabled result recorder allocated", n)
	}
}
