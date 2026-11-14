package prepare

import (
	"context"
	"errors"
	"testing"

	"github.com/sqlrs/engine-local/internal/prepare/queue"
	"github.com/sqlrs/engine-local/internal/preparecost"
)

type costBoundaryJournal struct {
	normalLiquibaseJournal
	cause error
	calls int
}

func (q *costBoundaryJournal) GetLiquibaseExecutionForJob(context.Context, string) (queue.LiquibaseExecutionRecord, bool, error) {
	q.calls++
	return queue.LiquibaseExecutionRecord{}, false, q.cause
}

func (q *costBoundaryJournal) ReadLiquibaseSession(context.Context, string) (queue.LiquibaseSessionRecord, bool, error) {
	q.calls++
	return queue.LiquibaseSessionRecord{}, false, q.cause
}

func (q *costBoundaryJournal) NextCloneGeneration(context.Context, string) (int, error) {
	q.calls++
	return 0, q.cause
}

// CT08: the actual boundaries retain their reached failure and call count.
// They must not green on an earlier cost-only model or run extra SQL/runtime.
func TestCostNamedHostBoundariesRetainFailures(t *testing.T) {
	primary := errors.New("private-journal-failure")
	for _, operation := range []preparecost.Operation{preparecost.Planning, preparecost.SessionStartup, preparecost.StepExecution} {
		t.Run(string(operation), func(t *testing.T) {
			m := &PrepareService{}
			q := &costBoundaryJournal{cause: primary}
			r := preparecost.NewRecorder(true, 1)
			ctx, end := preparecost.Start(preparecost.WithRecorder(t.Context(), r), preparecost.Worker)
			phase := preparecost.PhasePlanning
			ordinal := 0
			if operation == preparecost.SessionStartup {
				phase = preparecost.PhaseStartup
			}
			if operation == preparecost.StepExecution {
				phase, ordinal = preparecost.PhaseStep, 2
			}
			ctx = preparecost.WithPhase(ctx, phase, ordinal)
			var err error
			switch operation {
			case preparecost.Planning:
				_, err = m.planNormalContainerLiquibase(ctx, q, nil, "private-job", preparedRequest{})
			case preparecost.SessionStartup:
				_, err = m.startNormalLiquibaseSession(ctx, q, nil, "private-job", preparedRequest{}, taskState{}, 0, queue.ExecutionBinding{})
			case preparecost.StepExecution:
				err = m.executeNormalLiquibaseStep(ctx, q, nil, preparedRequest{}, taskState{}, queue.ExecutionBinding{}, &normalLiquibaseWorking{})
			}
			if q.calls != 1 || err == nil || operation != preparecost.Planning && !errors.Is(err, primary) {
				t.Fatal("observation changed/repeated the actual failed operation")
			}
			end(preparecost.Failed)
			report := r.Finish()
			if !report.Complete || len(report.Spans) != 2 {
				t.Fatal("actual named boundary was not observed")
			}
			got := report.Spans[1]
			if got.Operation != operation || got.Parent != 1 || got.Phase != phase || got.Step != ordinal || got.Outcome != preparecost.Failed {
				t.Fatal("source/phase/step/failed outcome differs")
			}
		})
	}
}
