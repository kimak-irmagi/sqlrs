package prepare

import (
	"context"
	"errors"
	"testing"

	"github.com/sqlrs/engine-local/internal/preparecost"
	engineRuntime "github.com/sqlrs/engine-local/internal/runtime"
)

type timelineBindingRuntime struct{ *costTrimImageRuntime }

func (r timelineBindingRuntime) EnsureRunnerImage(ctx context.Context, b engineRuntime.RunnerImageBinding) (string, error) {
	return preparecost.Observe(ctx, preparecost.DockerCommand, func(child context.Context) (string, error) {
		return r.costTrimImageRuntime.EnsureRunnerImage(child, b)
	})
}

// TL04 observes the real binding call, including an actual refusal and a child
// marker emitted at its runtime boundary. The marker is not native timing proof.
func TestTimelineBindingActualBoundaryAndFailure(t *testing.T) {
	for _, refused := range []bool{false, true} {
		m, p, job, resolver, rt := costTrimBindingFixture(t)
		denied := errors.New("timeline ensure refused")
		if refused {
			rt.ensureErr = denied
		}
		m.containerLiquibase.Runtime = timelineBindingRuntime{rt}
		recorder := preparecost.NewRecorder(true, 1)
		ctx, finish := preparecost.Start(preparecost.WithRecorder(t.Context(), recorder), preparecost.Worker)
		_, cleanup, err := m.bindContainerLiquibase(ctx, job.JobID, p)
		costTrimBindingCleanup(t, cleanup)
		finish(preparecost.Succeeded)
		report := recorder.Finish()
		if !report.Complete || refused && !errors.Is(err, denied) || !refused && err != nil || resolver.calls != 1 {
			t.Fatal("instrumentation changed work", err)
		}
		counts := map[preparecost.Operation]int{}
		for _, span := range report.Spans {
			counts[span.Operation]++
			if span.Operation == preparecost.DockerCommand && report.Spans[span.Parent-1].Operation != preparecost.RunnerEnsure {
				t.Fatal("runtime not inside exact ensure boundary")
			}
			if span.Operation == preparecost.RunnerEnsure && (span.Outcome == preparecost.Failed) != refused {
				t.Fatal("failure not preserved")
			}
		}
		for _, key := range []preparecost.Operation{preparecost.Binding, preparecost.ProjectionWait, preparecost.InputMaterialize, preparecost.RunnerResolve, preparecost.RunnerEnsure, preparecost.DockerCommand} {
			if counts[key] != 1 {
				t.Fatal("actual binding boundary missing", key, counts)
			}
		}
	}
}

// TL05: the source/base failure is observed at its real managed-runtime call,
// before a clone or container start. It must not be converted to successful work.
func TestTimelineManagedStartupActualBaseFailure(t *testing.T) {
	m, p, _, _, rt := managedExecutionFixture(t)
	rt.initErr = errors.New("timeline base denied")
	recorder := preparecost.NewRecorder(true, 1)
	ctx, finish := preparecost.Start(preparecost.WithRecorder(context.Background(), recorder), preparecost.Worker)
	_, problem := (&taskExecutor{m: m}).startManagedRuntimeWithPlan(ctx, "timeline-base", p, &TaskInput{Kind: "image", ID: p.managed.Key}, nil)
	finish(preparecost.Succeeded)
	report := recorder.Finish()
	if problem == nil || !report.Complete {
		t.Fatal("base refusal lost", problem, report.Complete)
	}
	counts := map[preparecost.Operation]int{}
	for _, span := range report.Spans {
		counts[span.Operation]++
		if (span.Operation == preparecost.DBStartup || span.Operation == preparecost.DBBaseInit) && span.Outcome != preparecost.Failed {
			t.Fatal("failed startup labelled successful")
		}
	}
	if counts[preparecost.DBStartup] != 1 || counts[preparecost.DBLockWait] != 1 || counts[preparecost.DBBaseInit] != 1 || counts[preparecost.DBClone] != 0 || counts[preparecost.DBStart] != 0 {
		t.Fatal("real startup phase order lost", counts)
	}
}
