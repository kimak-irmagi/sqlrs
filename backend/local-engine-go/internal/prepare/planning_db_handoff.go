package prepare

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/sqlrs/engine-local/internal/instanceaccess"
	"github.com/sqlrs/engine-local/internal/managedidentity"
	"github.com/sqlrs/engine-local/internal/prepare/finalizationcodec"
	"github.com/sqlrs/engine-local/internal/prepare/planningcodec"
	"github.com/sqlrs/engine-local/internal/prepare/queue"
	"github.com/sqlrs/engine-local/internal/preparecost"
	engineRuntime "github.com/sqlrs/engine-local/internal/runtime"
)

// normalPlanningResult carries the fresh worker's exact allocation privately.
// Its pointer is not durable authority; every transfer/disposal uses queue CAS.
// Requirements: issue89-measured-overhead-reduction.md §2; handoff schema §§3–5.
type normalPlanningResult struct {
	queue.PlanningResult
	retained *normalPlanningDB
}

type normalPlanningDB struct {
	runtime *jobRuntime
	preview queue.PlanningRecoverySnapshot
}

type planningDBJournal interface {
	RecordPlanningDBRetention(context.Context, queue.AttemptRecord, int, instanceaccess.Operation, func(context.Context, queue.PlanningRecoverySnapshot, instanceaccess.Operation) error, string) (int64, error)
	CompleteRetainedPlanning(context.Context, queue.AttemptRecord, int, int64, int64, int64, string) error
	TransferPlanningDBToFirstSession(context.Context, queue.LiquibaseSessionLaunch) (queue.LiquibaseSessionRecord, error)
	RequestPlanningDBDisposal(context.Context, queue.AttemptRecord, int, string, string) (int64, error)
	CompletePlanningDBDisposal(context.Context, queue.AttemptRecord, int, instanceaccess.Operation, queue.CloneAbsenceVerifier, string) error
}

func planningCloneSource(clone queue.ClonePlan) string {
	if clone.SourceKind == "state" {
		return clone.SourceStateID
	}
	return clone.SourceImageID
}

// observePlanningDBDisposal times the real physical/metadata disposal branch;
// retained/transferred paths do not emit a placeholder for removed work.
func observePlanningDBDisposal(ctx context.Context, action func(context.Context) error) error {
	return preparecost.ObserveCall(ctx, preparecost.PlanDispose, action)
}

func (m *PrepareService) finishDisposedPlanningCancellation(ctx context.Context, q normalLiquibaseJournal, jobID string) error {
	intent, err := q.BeginFinalization(ctx, jobID, finalizationcodec.Outcome{Kind: finalizationcodec.Cancelled, EventProfile: finalizationcodec.StatusError, Problem: &finalizationcodec.PublicError{Code: "cancelled", Message: "job cancelled"}}, m.normalTimestamp())
	if err != nil {
		return err
	}
	return m.finishNormalLiquibase(ctx, q, intent)
}

func normalPlanningOperation(rt *jobRuntime, stage string) instanceaccess.Operation {
	op := rt.operation
	op.RuntimeRef, op.Stage = rt.instance.ID, stage
	return op
}

// observeRetainedPlanningDB performs real Docker, directory and authenticated
// PostgreSQL checks outside queue transactions. Neither a pointer nor a public
// caller flag substitutes for the exact saved allocation and managed identity.
func (m *PrepareService) observeRetainedPlanningDB(ctx context.Context, p queue.PlanningRecoverySnapshot, rt *jobRuntime, op instanceaccess.Operation) error {
	if p.CloneRuntimeID != rt.instance.ID || p.Attempt.CloneID != p.Clone.ID || op != normalPlanningOperation(rt, "running") || op.Target != rt.runtimeDir {
		return queue.ErrPlanningSchema
	}
	inspector, ok := m.runtime.(interface {
		InspectManaged(context.Context, managedidentity.RuntimeBinding) (engineRuntime.Instance, error)
	})
	if !ok {
		return instanceaccess.ErrUnavailable
	}
	actual, err := inspector.InspectManaged(ctx, rt.instance.Binding)
	if err != nil || actual.ID != rt.instance.ID || actual.Binding != rt.instance.Binding {
		return instanceaccess.ErrConflict
	}
	info, err := os.Lstat(rt.runtimeDir)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return queue.ErrPlanningSchema
	}
	return m.verifyManagedRuntime(ctx, rt)
}

// captureNormalPlanningDB reads and parses the private capture once before
// choosing live retention or the unchanged absence-based non-prepare/empty path.
// The retained fact still precedes durable output, and success commits before
// private executor/input disposal (PG02/05/08).
func (m *PrepareService) captureNormalPlanningDB(ctx context.Context, q normalLiquibaseJournal, runtime normalLiquibaseRuntime, p preparedRequest, saved queue.PlanningRecoverySnapshot, tracked trackedExecutorResult, database *jobRuntime, captureDir string) (normalPlanningResult, error) {
	fail := func(err error) (normalPlanningResult, error) { return normalPlanningResult{}, err }
	capture, err := runtime.ReadCompleteLiquibaseOutput(ctx, saved.Attempt.PlannedExecutorName, saved.Attempt.ExecutorID, captureDir, engineRuntime.CaptureIdentity{JobID: saved.Attempt.JobID, AttemptNo: saved.Attempt.AttemptNo, ExecutionDigest: saved.Attempt.ExecutionDigest, HelperDigest: saved.Capture.HelperDigest, ExitCode: tracked.ExitCode}, planningCaptureMaxBytes)
	if err != nil {
		return fail(ErrPlanningCaptureUnavailable)
	}
	changes, err := parseLiquibaseUpdateSQL(string(capture.Stdout))
	if err != nil {
		return fail(ErrPlanningCaptureUnavailable)
	}
	metadata := make([]planningcodec.Changeset, len(changes))
	for i, c := range changes {
		metadata[i] = planningcodec.Changeset{ID: c.ID, Author: c.Author, Path: c.Path, SQLHash: c.SQLHash, Checksum: c.Checksum}
	}
	job, found, err := q.GetJob(ctx, saved.Attempt.JobID)
	if err != nil || !found {
		return fail(queue.ErrPlanningSchema)
	}
	retain := !p.request.PlanOnly && job.Purpose == "prepare" && len(metadata) > 0 && tracked.ExitCode == 0
	var ownerID int64
	if retain {
		journal, ok := q.(planningDBJournal)
		if !ok {
			return fail(queue.ErrPlanningSchema)
		}
		op := normalPlanningOperation(database, "running")
		ownerID, err = journal.RecordPlanningDBRetention(ctx, saved.Attempt, 1, op, func(ctx context.Context, actual queue.PlanningRecoverySnapshot, operation instanceaccess.Operation) error {
			return m.observeRetainedPlanningDB(ctx, actual, database, operation)
		}, m.normalTimestamp())
		if err != nil {
			return fail(err)
		}
	} else {
		preview := &jobRunner{}
		preview.setRuntime(database)
		if err := observePlanningDBDisposal(ctx, func(child context.Context) error { return m.cleanupRuntime(child, preview) }); err != nil {
			return fail(err)
		}
		ownerID, err = q.RecordPlanningCloneQuiescence(ctx, saved.Attempt, 1, saved.Clone.PlannedRuntimeName, database.instance.ID, m.normalTimestamp())
		if err != nil {
			return fail(err)
		}
	}
	output, err := q.RecordVerifiedPlanningCapture(ctx, saved.Attempt, 1, queue.PlanningCaptureProof{StdoutDigest: capture.StdoutDigest, StdoutBytes: int64(len(capture.Stdout)), StderrDigest: capture.StderrDigest, StderrBytes: int64(len(capture.Stderr)), ManifestRef: saved.Capture.Manifest}, m.normalTimestamp())
	if err != nil {
		return fail(err)
	}
	if _, err := q.SavePlanningResult(ctx, saved.Attempt.JobID, 1, capture.StdoutDigest, metadata, m.normalTimestamp()); err != nil {
		return fail(err)
	}
	if retain {
		err = preparecost.ObserveCall(ctx, preparecost.PlanRetain, func(child context.Context) error {
			return q.(planningDBJournal).CompleteRetainedPlanning(child, saved.Attempt, 1, tracked.ExitEvidenceID, ownerID, output, m.normalTimestamp())
		})
	} else {
		err = q.CompletePlanning(ctx, saved.Attempt, 1, tracked.ExitEvidenceID, ownerID, output, m.normalTimestamp())
	}
	if err != nil {
		return fail(err)
	}
	result, found, err := q.LoadPlanningResult(ctx, saved.Attempt.JobID)
	if err != nil || !found {
		return fail(queue.ErrPlanningSchema)
	}
	value := normalPlanningResult{PlanningResult: result}
	if retain {
		value.retained = &normalPlanningDB{runtime: database, preview: saved}
	}
	return value, nil
}

type normalPlanningDBAbsence struct {
	normalLiquibaseAbsence
	target string
}

func (v normalPlanningDBAbsence) VerifyReleasedClone(ctx context.Context, clone queue.ClonePlan, id string) error {
	if err := v.normalLiquibaseAbsence.VerifyReleasedClone(ctx, clone, id); err != nil {
		return err
	}
	if _, err := os.Lstat(v.target); !os.IsNotExist(err) {
		return fmt.Errorf("planning DB directory absence is unproved")
	}
	return nil
}

// disposeNormalPlanningDB saves intent before any physical mutation. Unknown
// request or completion leaves the owner pending and blocks another startup;
// this is not a recovery worker or a destructive fallback (PG06/08).
func (m *PrepareService) disposeNormalPlanningDB(ctx context.Context, q normalLiquibaseJournal, runtime normalLiquibaseRuntime, candidate *normalPlanningDB, reason string) error {
	journal, ok := q.(planningDBJournal)
	if !ok || candidate == nil {
		return queue.ErrPlanningSchema
	}
	return observePlanningDBDisposal(ctx, func(ctx context.Context) error {
		if _, err := journal.RequestPlanningDBDisposal(ctx, candidate.preview.Attempt, 1, reason, m.normalTimestamp()); err != nil {
			return err
		}
		preview := &jobRunner{}
		preview.setRuntime(candidate.runtime)
		if err := m.cleanupRuntime(ctx, preview); err != nil {
			return err
		}
		proof := normalPlanningDBAbsence{normalLiquibaseAbsence: normalLiquibaseAbsence{runtime: runtime, planning: candidate.preview}, target: candidate.runtime.runtimeDir}
		return journal.CompletePlanningDBDisposal(ctx, candidate.preview.Attempt, 1, normalPlanningOperation(candidate.runtime, "retired"), proof, m.normalTimestamp())
	})
}

// A cancellation before transfer has a different destructive owner from a
// working session. Persisted cancellation, not context cancellation alone,
// authorises disposal and finalisation. An uncertain transfer uses neither.
func (m *PrepareService) finishRetainedPlanningCancellation(ctx context.Context, q normalLiquibaseJournal, runtime normalLiquibaseRuntime, candidate *normalPlanningDB) error {
	cleanup, cancel := context.WithTimeout(context.WithoutCancel(ctx), 90*time.Second)
	defer cancel()
	if err := m.disposeNormalPlanningDB(cleanup, q, runtime, candidate, "accepted_cancel"); err != nil {
		return err
	}
	return m.finishDisposedPlanningCancellation(cleanup, q, candidate.preview.Attempt.JobID)
}
