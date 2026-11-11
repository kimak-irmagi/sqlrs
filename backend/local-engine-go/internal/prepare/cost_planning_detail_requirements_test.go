package prepare

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"github.com/sqlrs/engine-local/internal/prepare/queue"
	"github.com/sqlrs/engine-local/internal/preparecost"
	engineRuntime "github.com/sqlrs/engine-local/internal/runtime"
)

type costPlanningEffects struct {
	calls         []string
	output        string
	fail, panicAt string
	failure       error
}

func (f *costPlanningEffects) call(name string) error {
	f.calls = append(f.calls, name)
	if f.panicAt == name {
		panic(f.failure)
	}
	if f.fail == name {
		return f.failure
	}
	return nil
}
func (f *costPlanningEffects) RequestAttemptCreate(context.Context, queue.AttemptRecord, int) error {
	return f.call("request-create")
}
func (f *costPlanningEffects) AttachAttemptExecutor(context.Context, queue.AttemptRecord, int, string) error {
	return f.call("attach")
}
func (f *costPlanningEffects) RequestAttemptStart(context.Context, queue.AttemptRecord, int, string) error {
	return f.call("request-start")
}
func (f *costPlanningEffects) ObserveAttemptExit(context.Context, queue.AttemptRecord, int, string, int, string) (int64, error) {
	return 9, f.call("exit-fact")
}
func (f *costPlanningEffects) CreateContainer(context.Context, engineRuntime.RunRequest) (string, error) {
	return strings.Repeat("a", 64), f.call("create")
}
func (f *costPlanningEffects) StartContainer(context.Context, string) error { return f.call("start") }
func (f *costPlanningEffects) WaitContainer(context.Context, string) (string, int, error) {
	return f.output, 7, f.call("wait")
}

// DT06: exercise the real tracked planner with the same effect/journal order,
// preserving natural nonzero exits, failures and the exact panic object.
func TestPlanningHelperCostUsesNarrowActualCall(t *testing.T) {
	for _, mode := range []string{"off", "on", "missing", "start-failure", "wait-failure", "panic"} {
		t.Run(mode, func(t *testing.T) {
			f := &costPlanningEffects{failure: errors.New("private-planning-marker")}
			for _, phase := range []string{"preflight", "cli", "drain", "stream_flush", "hash_rename", "renamed_flush", "manifest_flush", "final_flush"} {
				f.output += "sqlrs-lb-cost-v1 " + phase + " 0 0\n"
			}
			if mode == "missing" {
				f.output = ""
			}
			if mode == "start-failure" {
				f.fail = "start"
			}
			if mode == "wait-failure" {
				f.fail = "wait"
			}
			if mode == "panic" {
				f.panicAt = "wait"
			}
			recorder := preparecost.NewRecorder(mode != "off", 1)
			root, end := preparecost.Start(preparecost.WithRecorder(t.Context(), recorder), preparecost.Worker)
			ctx := preparecost.WithPhase(root, preparecost.PhasePlanning, 0)
			attempt := queue.AttemptRecord{AttemptPlan: queue.AttemptPlan{JobID: "cost-job", TaskID: "plan", CloneID: "clone", ExecutionDigest: "binding", WorkerGeneration: 1}, AttemptNo: 1, PlannedExecutorName: "sqlrs-plan-cost", Phase: "planned"}
			var result trackedExecutorResult
			var err error
			var caught any
			func() {
				defer func() { caught = recover() }()
				result, err = runTrackedExecutor(ctx, f, f, attempt, engineRuntime.RunRequest{ImageID: "image"}, func() string { return "2026-11-11T00:00:00Z" })
			}()
			want := []string{"request-create", "create", "attach", "request-start", "start", "wait", "exit-fact"}
			if mode == "start-failure" {
				want = want[:5]
			}
			if mode == "wait-failure" || mode == "panic" {
				want = want[:6]
			}
			if !reflect.DeepEqual(f.calls, want) {
				t.Fatal("observation changed planner ordering or repeated work")
			}
			if mode == "panic" {
				if caught != f.failure {
					t.Fatal("observation changed original panic")
				}
				end(preparecost.Panicked)
			} else {
				if caught != nil || (err != nil) != (f.fail != "") || err != nil && !errors.Is(err, f.failure) {
					t.Fatal("planner outcome changed")
				}
				if err == nil && (result.ExitCode != 7 || result.ExitEvidenceID != 9 || result.Output != f.output) {
					t.Fatal("natural nonzero exit/capture changed")
				}
				end(preparecost.Succeeded)
			}
			report := recorder.Finish()
			if mode == "off" {
				if len(report.Spans) != 0 || len(report.Helpers) != 0 {
					t.Fatal("off planner recorded details")
				}
				return
			}
			if !report.Complete || len(report.Spans) != 2 || report.Spans[1].Operation != preparecost.PlanningInvocation || report.Spans[1].Phase != preparecost.PhasePlanning || report.Spans[1].Parent != 1 {
				t.Fatal("planner is not one narrow source-linked call")
			}
			if mode == "panic" {
				if report.Spans[1].Outcome != preparecost.Panicked {
					t.Fatal("panic outcome not observed")
				}
				return
			}
			if len(report.Helpers) != 1 || report.Helpers[0].Owner != 2 || report.Helpers[0].Available != (mode != "missing" && mode != "start-failure") {
				t.Fatal("planner helper detail lost actual owner")
			}
			wire, wireErr := preparecost.EncodeReport(report)
			if wireErr != nil || strings.Contains(string(wire), "private-") {
				t.Fatal("planner detail leaked", wireErr)
			}
		})
	}
}
