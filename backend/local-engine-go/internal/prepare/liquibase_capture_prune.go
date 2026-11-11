package prepare

import (
	"context"
	"fmt"

	"github.com/sqlrs/engine-local/internal/prepare/queue"
	engineRuntime "github.com/sqlrs/engine-local/internal/runtime"
)

// capturePruneJournal is the narrow capability for the already approved §9
// coordinator. Historical shared interfaces acquire no new methods. MP01–MP04.
type capturePruneJournal interface {
	ReadLiquibaseCapturePrunePlan(context.Context, queue.LiquibaseSessionRecord, queue.LiquibaseInvocationRecord) (*queue.CapturePrunePlan, error)
	ConfirmLiquibaseCapturePruned(context.Context, queue.LiquibaseSessionRecord, queue.LiquibaseInvocationRecord, queue.LiquibaseCapturePruneVerifier, string) error
}

type capturePruneRuntime interface {
	VerifyLiquibaseCapturePruned(context.Context, string, engineRuntime.StepCaptureIdentity) error
}

// sessionCapturePruneObservation binds a queue callback to the same exact
// current completion that supplied the saved authority to the helper. Native
// observation has already finished outside SQLite; this callback only matches
// the opaque runtime receipt. Issue89-cost-attribution-and-trimming.md §6, CPR05/06.
type sessionCapturePruneObservation struct {
	identity engineRuntime.StepCaptureIdentity
	capture  engineRuntime.CompleteStepCapture
}

func (v sessionCapturePruneObservation) VerifyLiquibaseCapturePruned(ctx context.Context, plan queue.CapturePrunePlan) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !v.capture.MatchesPruneObservation(v.identity, projectCapturePrune(plan)) {
		return fmt.Errorf("saved Liquibase prune observation differs")
	}
	return nil
}

// projectCapturePrune copies only the saved public identities; raw capture bytes
// and client input never supply a missing link. MP03.
func projectCapturePrune(plan queue.CapturePrunePlan) engineRuntime.CapturePruneAuthorization {
	return engineRuntime.CapturePruneAuthorization{RetirementRef: plan.RetirementRef, SessionID: plan.SessionID,
		WorkerGeneration: plan.WorkerGeneration, RunnerID: plan.RunnerID, HelperDigest: plan.HelperDigest,
		PriorInvocationRef: plan.PriorInvocationRef, PriorManifestDigest: plan.PriorManifestDigest, NextInvocationRef: plan.NextInvocationRef}
}
