package prepare

import (
	"errors"
	"testing"

	"github.com/sqlrs/engine-local/internal/prepare/queue"
	"github.com/sqlrs/engine-local/internal/preparecost"
)

// CT08: three separately named real journal boundaries surround the existing
// runtime call. A refused admission cannot become a runtime/SQL invocation.
func TestCostCoordinatorJournalBoundaries(t *testing.T) {
	for _, mode := range []string{"control", "plan-refused", "admission-refused"} {
		t.Run(mode, func(t *testing.T) {
			c, base, runtime := sessionCoordinatorFixture()
			base.session.RunnerID, base.session.RunnerPhase = runtime.id, "running"
			runtime.status = "running"
			journal := &mainPathDispatchJournal{sessionCoordinatorJournal: base}
			c.journal = journal
			primary := errors.New("private-admission-refusal")
			if mode == "plan-refused" {
				base.pending = true
			}
			if mode == "admission-refused" {
				journal.requestErr = primary
			}
			r := preparecost.NewRecorder(true, 1)
			ctx, end := preparecost.Start(preparecost.WithRecorder(t.Context(), r), preparecost.Worker)
			ctx = preparecost.WithPhase(ctx, preparecost.PhaseStep, 2)
			plan := queue.LiquibaseInvocationPlan{InvocationRef: "next", StdoutRef: "next-out", StderrRef: "next-err", ManifestRef: "next-manifest", ProtocolRevision: "lb-step-capture-v2"}
			_, _, _, err := c.invokeStep(ctx, base.session, "execute-2", "binding", plan, "", []string{"update-count", "--count", "1"})
			want := []preparecost.Operation{preparecost.InvocationPlan, preparecost.InvocationAdmit, preparecost.InvocationComplete}
			if mode != "control" {
				if err == nil || runtime.calls != 0 || base.completed != 0 {
					t.Fatal("refused journal boundary started/completed SQL")
				}
				if mode == "admission-refused" {
					want = want[:2]
					if !errors.Is(err, primary) {
						t.Fatal("recording replaced the original admission refusal")
					}
				} else {
					want = want[:1]
					if !errors.Is(err, queue.ErrCommonAttempt) {
						t.Fatal("plan refusal did not reach its intended boundary")
					}
				}
			} else if err != nil || runtime.calls != 1 || base.completed != 1 {
				t.Fatal("recording changed actual successful protocol")
			}
			end(preparecost.Succeeded)
			report := r.Finish()
			if !report.Complete || len(report.Spans) != len(want)+1 {
				t.Fatal("journal source intervals missing")
			}
			for i, key := range want {
				span := report.Spans[i+1]
				outcome := preparecost.Succeeded
				if mode != "control" && i == len(want)-1 {
					outcome = preparecost.Failed
				}
				if span.Operation != key || span.Parent != 1 || span.Step != 2 || span.Phase != preparecost.PhaseStep || span.Outcome != outcome {
					t.Fatal("journal call lost exact key/caller/step/outcome")
				}
			}
		})
	}
}
