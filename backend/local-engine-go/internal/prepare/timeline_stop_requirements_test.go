package prepare

import (
	"errors"
	"github.com/sqlrs/engine-local/internal/preparecost"
	engineRuntime "github.com/sqlrs/engine-local/internal/runtime"
	"testing"
)

// TL07: observe the real stop/confirmation sequence without removing retained
// output or inventing acknowledgement after the runtime's actual refusal.
func TestTimelineRunnerStopActualBoundaries(t *testing.T) {
	for _, refused := range []bool{false, true} {
		c, j, rt := sessionCoordinatorFixture()
		session, err := c.launchRunner(t.Context(), j.session, engineRuntime.RunRequest{ImageID: "image"})
		if err != nil {
			t.Fatal(err)
		}
		if refused {
			rt.signalErr = errors.New("stop refused")
		}
		recorder := preparecost.NewRecorder(true, 1)
		ctx, end := preparecost.Start(preparecost.WithRecorder(t.Context(), recorder), preparecost.Worker)
		err = c.stopRunner(ctx, session)
		end(preparecost.Succeeded)
		report := recorder.Finish()
		if !report.Complete || (err != nil) != refused || len(report.Spans) != 2 || report.Spans[1].Operation != preparecost.RunnerStop || (report.Spans[1].Outcome == preparecost.Failed) != refused || rt.removes != 0 {
			t.Fatal("stop observation changed work or missed real call")
		}
		if refused && j.session.RunnerPhase != "stop_requested" || !refused && j.session.RunnerPhase != "stopped" {
			t.Fatal("stop acknowledgement invented or lost")
		}
	}
}
