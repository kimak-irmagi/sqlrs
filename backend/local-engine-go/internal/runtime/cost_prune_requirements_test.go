package runtime

import (
	"errors"
	"strings"
	"testing"

	"github.com/sqlrs/engine-local/internal/preparecost"
)

// CT08: observe the real five-command prune proof, including bounded commands.
// Failed proof retains its exact reached boundary, never mutation authority.
func TestCostPruneProofCallerAndCommandBoundaries(t *testing.T) {
	for _, mode := range []string{"off", "control", "probe-refused"} {
		t.Run(mode, func(t *testing.T) {
			_, identity, _ := stepCaptureFixture(t)
			ref := "retirement"
			identity.PriorRetirementRef = &ref
			identity.Prune = &CapturePruneAuthorization{RetirementRef: ref, SessionID: identity.SessionID,
				WorkerGeneration: identity.WorkerGeneration, RunnerID: identity.RunnerID,
				PriorInvocationRef: "prior-call", PriorManifestDigest: "sha256:" + strings.Repeat("e", 64),
				NextInvocationRef: identity.InvocationRef, HelperDigest: identity.HelperDigest}
			name := "sqlrs-lb-cost-prune"
			runner := &mainPathPruneProbeRunner{outputs: [][]byte{[]byte("sqlrs-step-idle-v1\n"), []byte("sqlrs-capture-prune-absent-v1\n"), []byte("sqlrs-step-idle-v1\n")}}
			runner.responses = append(presentInspection(identity.RunnerID, name, "running", 0), presentInspection(identity.RunnerID, name, "running", 0)...)
			if mode == "probe-refused" {
				runner.probeErr = errors.New("private-probe-failure")
			}
			r := preparecost.NewRecorder(mode != "off", 1)
			ctx := preparecost.WithPhase(preparecost.WithRecorder(t.Context(), r), preparecost.PhaseStep, 2)
			err := NewDocker(Options{Runner: runner}).VerifyLiquibaseCapturePruned(ctx, name, identity)
			failed := mode == "probe-refused"
			if (err != nil) != failed || !failed && (len(runner.calls) != 2 || len(runner.bounded) != 3) || failed && (len(runner.calls) != 1 || len(runner.bounded) != 2) {
				t.Fatal("measurement changed actual proof or reached refusal")
			}
			report := r.Finish()
			if mode == "off" {
				if len(report.Spans) != 0 {
					t.Fatal("disabled prune proof recorded work")
				}
				return
			}
			want := 6
			outcome := preparecost.Succeeded
			if failed {
				want, outcome = 4, preparecost.Failed
			}
			if !report.Complete || len(report.Spans) != want || report.Spans[0].Operation != preparecost.PruneObserve || report.Spans[0].Outcome != outcome {
				t.Fatal("actual prune boundary/call counts were not recorded")
			}
			for _, span := range report.Spans[1:] {
				if span.Operation != preparecost.DockerCommand || span.Parent != 1 || span.Step != 2 || span.Phase != preparecost.PhaseStep {
					t.Fatal("bounded command lost caller/step association")
				}
			}
		})
	}
}
