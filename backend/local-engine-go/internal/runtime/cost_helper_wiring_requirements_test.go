package runtime

import (
	"errors"
	"strings"
	"testing"

	"github.com/sqlrs/engine-local/internal/preparecost"
)

func zeroHelperCostOutput() string {
	var output strings.Builder
	for _, phase := range []string{"preflight", "cli", "drain", "stream_flush", "hash_rename", "renamed_flush", "manifest_flush", "final_flush"} {
		output.WriteString("sqlrs-lb-cost-v1 " + phase + " 0 0\n")
	}
	return output.String()
}

// DT02/DT06: malformed diagnostics cannot replace the protected capture result.
func TestStepHelperCostOwnsActualInvocationAndPreservesResult(t *testing.T) {
	for _, mode := range []string{"off", "on", "missing", "private", "oversized", "child-failed"} {
		t.Run(mode, func(t *testing.T) {
			dir, identity, manifest := stepCaptureFixture(t)
			response := runResponse{output: zeroHelperCostOutput()}
			if mode == "missing" {
				response.output = ""
			}
			if mode == "private" {
				response.output = "private-timer-marker\n"
			}
			if mode == "oversized" {
				response.output = strings.Repeat("x", 4097)
			}
			if mode == "child-failed" {
				response.err = errors.New("private-exec-marker")
				manifest["child_exit_code"] = 7
			}
			manifest["stdout_bytes"] = len("private-stream")
			manifest["stdout_sha256"] = captureHex([]byte("private-stream"))
			writeStepCaptureFixture(t, dir, manifest, []byte("private-stream"), nil)
			name := "sqlrs-lb-cost-helper"
			runner := &captureFilesRequirementsRunner{archives: sessionBundleFixture(t, dir, identity.RunnerID, identity.InvocationRef)}
			runner.responses = presentInspection(identity.RunnerID, name, "running", 0)
			runner.responses = append(runner.responses, response)
			runner.responses = append(runner.responses, presentInspection(identity.RunnerID, name, "running", 0)...)
			runner.responses = append(runner.responses, runResponse{output: "sqlrs-step-idle-v1\n"})
			runner.responses = append(runner.responses, presentInspection(identity.RunnerID, name, "running", 0)...)
			recorder := preparecost.NewRecorder(mode != "off", 1)
			ctx := preparecost.WithPhase(preparecost.WithRecorder(t.Context(), recorder), preparecost.PhaseStep, 2)
			result, err := NewDocker(Options{Runner: runner}).InvokeTrackedLiquibaseStep(ctx, name, "", identity, []string{"update-count", "--count", "1"}, 1024)
			wantExit := 0
			if mode == "child-failed" {
				wantExit = 7
			}
			if err != nil || result.ExitCode != wantExit || string(result.Stdout) != "private-stream" || len(runner.calls) != 5 || len(runner.copies) != 1 {
				t.Fatal("diagnostics changed original capture/outcome/counts")
			}
			flag := "0"
			if mode != "off" {
				flag = "1"
			}
			if !containsArg(runner.calls[1].args, "-e", "SQLRS_CAPTURE_COST="+flag) {
				t.Fatal("actual invocation lacks explicit cost override")
			}
			report := recorder.Finish()
			if mode == "off" {
				if len(report.Helpers) != 0 || len(report.Spans) != 0 {
					t.Fatal("off path recorded helper data")
				}
				return
			}
			available := mode == "on" || mode == "child-failed"
			if !report.Complete || len(report.Helpers) != 1 || report.Helpers[0].Owner != 1 || report.Helpers[0].Available != available || report.Spans[0].Operation != preparecost.Invocation || report.Spans[0].Step != 2 {
				t.Fatal("helper detail lost the actual host invocation/step")
			}
			wire, err := preparecost.EncodeReport(report)
			if err != nil || strings.Contains(string(wire), "private-") {
				t.Fatal("helper diagnostics leaked", err)
			}
		})
	}
}

// DT02: source/order guards supplement (not replace) native execution. The
// timer functions must have no command substitution or execution-failure path.
func TestHelperTimerClosedBoundaries(t *testing.T) {
	for _, helper := range [][]byte{liquibaseCaptureHelper, liquibaseStepCaptureHelper} {
		source := string(helper)
		_, timer, ok := strings.Cut(source, "# cost-clock-begin\n")
		if !ok {
			t.Fatal("actual helper has no bounded timer")
		}
		timer, _, ok = strings.Cut(timer, "# cost-clock-end\n")
		// Arithmetic expansion is a builtin, unlike $(command) substitution.
		withoutArithmetic := strings.ReplaceAll(timer, "$"+"((", "")
		if !ok || strings.Contains(withoutArithmetic, "$"+"(") || strings.Contains(timer, "die") || strings.Contains(timer, "exit ") || !strings.Contains(timer, "/proc/uptime") {
			t.Fatal("timer gained a subprocess or execution authority")
		}
		phases := []string{"preflight", "cli", "drain", "stream_flush", "hash_rename", "renamed_flush", "manifest_flush", "final_flush"}
		remaining := source
		for _, phase := range phases {
			_, next, found := strings.Cut(remaining, "\ncost_mark "+phase+"\n")
			if !found {
				t.Fatal("actual helper phase/order missing", phase)
			}
			remaining = next
		}
		if !strings.Contains(remaining, "cost_emit\nexit \"$child_exit\"") {
			t.Fatal("timer changes final CLI exit")
		}
	}
}

// DT06: the existing runtime log sink must never carry diagnostic helper
// stdout. This reaches real runObservedCommand, not just a serializer check.
func TestStepHelperCostNeverEntersLogSink(t *testing.T) {
	dir, identity, _ := stepCaptureFixture(t)
	name := "sqlrs-lb-cost-private"
	runner := &captureFilesRequirementsRunner{archives: sessionBundleFixture(t, dir, identity.RunnerID, identity.InvocationRef)}
	runner.responses = presentInspection(identity.RunnerID, name, "running", 0)
	runner.responses = append(runner.responses, runResponse{output: "private-helper-marker\n"})
	runner.responses = append(runner.responses, presentInspection(identity.RunnerID, name, "running", 0)...)
	runner.responses = append(runner.responses, runResponse{output: "sqlrs-step-idle-v1\n"})
	runner.responses = append(runner.responses, presentInspection(identity.RunnerID, name, "running", 0)...)
	var logged []string
	recorder := preparecost.NewRecorder(true, 1)
	ctx := WithLogSink(preparecost.WithPhase(preparecost.WithRecorder(t.Context(), recorder), preparecost.PhaseStep, 1), func(line string) { logged = append(logged, line) })
	result, err := NewDocker(Options{Runner: runner}).InvokeTrackedLiquibaseStep(ctx, name, "", identity, []string{"update-count", "--count", "1"}, 1024)
	if err != nil || result.ExitCode != 0 {
		t.Fatal("diagnostic privacy changed capture outcome")
	}
	if strings.Contains(strings.Join(logged, "\n"), "private-helper-marker") {
		t.Fatal("helper detail escaped through runtime log sink")
	}
	if report := recorder.Finish(); !report.Complete || len(report.Helpers) != 1 || report.Helpers[0].Available {
		t.Fatal("invalid helper detail was not marked unavailable")
	}
}
