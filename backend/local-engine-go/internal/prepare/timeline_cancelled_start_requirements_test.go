package prepare

import (
	"context"
	"github.com/sqlrs/engine-local/internal/preparecost"
	"testing"
)

// TL07: the existing immediate cancellation response is not relabelled as a
// successful/failed DB start and must not enter the managed lock or runtime.
func TestTimelineManagedStartupPreservesImmediateCancellation(t *testing.T) {
	m, p, _, _, _ := managedExecutionFixture(t)
	recorder := preparecost.NewRecorder(true, 1)
	ctx, finish := preparecost.Start(preparecost.WithRecorder(t.Context(), recorder), preparecost.Worker)
	ctx, cancel := context.WithCancel(ctx)
	cancel()
	_, problem := (&taskExecutor{m: m}).startManagedRuntimeWithPlan(ctx, "cancelled", p, &TaskInput{Kind: "image", ID: p.managed.Key}, nil)
	finish(preparecost.Cancelled)
	report := recorder.Finish()
	if problem == nil || problem.Code != "cancelled" || !report.Complete || len(report.Spans) != 2 || report.Spans[1].Operation != preparecost.DBStartup || report.Spans[1].Outcome != preparecost.Cancelled {
		t.Fatal("cancelled startup response/boundary changed")
	}
}
