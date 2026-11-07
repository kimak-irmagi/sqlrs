package prepare

import (
	"context"
	"fmt"

	"github.com/sqlrs/engine-local/internal/prepare/queue"
)

type planningCloneJournal interface {
	PlanBoundPlanningAttempt(context.Context, queue.PlanningLaunch) (queue.AttemptRecord, error)
	ActivatePlanningClone(context.Context, queue.AttemptRecord, int, string, string) error
}

// launchBoundPlanningClone enforces the same journal-before-materialization
// boundary as state execution, but owns the stable plan task and protected
// capture refs. Any external failure leaves the exact rows for recovery.
// See P01/R01 in the planning recovery test design.
func launchBoundPlanningClone(ctx context.Context, journal planningCloneJournal, materializer boundCloneMaterializer, plan queue.PlanningLaunch) (queue.AttemptRecord, error) {
	if journal == nil || materializer == nil || plan.Clone.ID == "" || plan.Clone.PlannedRuntimeName == "" || plan.Attempt.CloneID != plan.Clone.ID {
		return queue.AttemptRecord{}, fmt.Errorf("bound planning clone and materializer are required")
	}
	attempt, err := journal.PlanBoundPlanningAttempt(ctx, plan)
	if err != nil {
		return queue.AttemptRecord{}, err
	}
	if attempt.AttemptPlan != plan.Attempt || attempt.AttemptNo != 1 || attempt.PlannedExecutorName == "" || attempt.Phase != "planned" || attempt.LegacyUnbound {
		return attempt, fmt.Errorf("journal returned mismatched planning attempt")
	}
	runtimeID, err := materializer.MaterializeBoundClone(ctx, plan.Clone)
	if err != nil {
		return attempt, err
	}
	if err := journal.ActivatePlanningClone(ctx, attempt, attempt.WorkerGeneration, plan.Clone.PlannedRuntimeName, runtimeID); err != nil {
		return attempt, err
	}
	return attempt, nil
}
