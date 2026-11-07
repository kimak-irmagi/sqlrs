package prepare

import (
	"context"
	"errors"

	"github.com/sqlrs/engine-local/internal/prepare/planningcodec"
	"github.com/sqlrs/engine-local/internal/prepare/queue"
	engineRuntime "github.com/sqlrs/engine-local/internal/runtime"
)

// ErrPlanningCaptureUnavailable never includes raw updateSQL or diagnostic
// bytes. The caller decides whether a safe retry or recovery pause is allowed.
var ErrPlanningCaptureUnavailable = errors.New("complete Liquibase planning output is unavailable")

type planningCaptureReader interface {
	ReadCompleteLiquibaseOutput(context.Context, string, string, string, engineRuntime.CaptureIdentity, int64) (engineRuntime.CompleteCapture, error)
}

type planningCaptureJournal interface {
	LoadPlanningResult(context.Context, string) (queue.PlanningResult, bool, error)
	ReadAttemptObservation(context.Context, queue.BoundAttempt) (queue.AttemptObservation, error)
	ReadPlanningOutputEvidence(context.Context, queue.AttemptRecord, int, string) (int64, error)
	RecordVerifiedPlanningCapture(context.Context, queue.AttemptRecord, int, queue.PlanningCaptureProof, string) (int64, error)
	SavePlanningResult(context.Context, string, int, string, []planningcodec.Changeset, string) (string, error)
	CompletePlanning(context.Context, queue.AttemptRecord, int, int64, int64, int64, string) error
}

// reconcilePlanningCapture uses a saved plan without touching Docker. With
// no saved plan, it checks the exact stopped executor and protected capture,
// then commits output evidence, ordered metadata, and completion. Raw SQL is
// held only in memory for parsing. See P03/P04/C03 in the planning tests.
func reconcilePlanningCapture(ctx context.Context, journal planningCaptureJournal, reader planningCaptureReader,
	snapshot queue.PlanningRecoverySnapshot, captureDir string, maxStreamBytes int64, at string) (queue.PlanningResult, error) {
	if journal == nil || snapshot.Attempt.TaskID != "plan" ||
		snapshot.Attempt.JobID == "" || snapshot.Attempt.AttemptNo < 1 || at == "" {
		return queue.PlanningResult{}, queue.ErrPlanningSchema
	}
	saved, found, err := journal.LoadPlanningResult(ctx, snapshot.Attempt.JobID)
	if err != nil {
		return queue.PlanningResult{}, err
	}
	if found {
		if saved.AttemptNo != snapshot.Attempt.AttemptNo || saved.ExecutionDigest != snapshot.Attempt.ExecutionDigest {
			return queue.PlanningResult{}, queue.ErrPlanningSchema
		}
		if snapshot.Attempt.Phase == "closed" && snapshot.Attempt.Resolution == "planning_result_saved" {
			return saved, nil
		}
		if snapshot.Attempt.Phase != "observed_stopped" || snapshot.Attempt.Resolution != "" || snapshot.ClonePhase != "quiescent" {
			return queue.PlanningResult{}, queue.ErrPlanningSchema
		}
		observation, err := journal.ReadAttemptObservation(ctx, queue.BoundAttempt{
			Attempt: snapshot.Attempt, Binding: snapshot.Binding, Clone: snapshot.Clone,
			ClonePhase: snapshot.ClonePhase, DBRuntimeID: snapshot.CloneRuntimeID,
		})
		if err != nil || observation.ExitEvidenceID <= 0 ||
			observation.QuiescenceEvidenceID <= observation.ExitEvidenceID || observation.ExitCode != 0 {
			return queue.PlanningResult{}, ErrPlanningCaptureUnavailable
		}
		outputID, err := journal.ReadPlanningOutputEvidence(ctx, snapshot.Attempt,
			snapshot.Attempt.WorkerGeneration, saved.OutputDigest)
		if err != nil || outputID <= observation.QuiescenceEvidenceID {
			return queue.PlanningResult{}, ErrPlanningCaptureUnavailable
		}
		if err := journal.CompletePlanning(ctx, snapshot.Attempt, snapshot.Attempt.WorkerGeneration,
			observation.ExitEvidenceID, observation.QuiescenceEvidenceID, outputID, at); err != nil {
			return queue.PlanningResult{}, err
		}
		return saved, nil
	}
	if reader == nil || captureDir == "" || maxStreamBytes <= 0 || snapshot.Attempt.ExecutorID == "" ||
		snapshot.Attempt.Phase != "observed_stopped" || snapshot.ClonePhase != "quiescent" ||
		snapshot.Capture.Manifest == "" || snapshot.Capture.HelperDigest == "" {
		return queue.PlanningResult{}, queue.ErrPlanningSchema
	}
	observation, err := journal.ReadAttemptObservation(ctx, queue.BoundAttempt{
		Attempt: snapshot.Attempt, Binding: snapshot.Binding, Clone: snapshot.Clone,
		ClonePhase: snapshot.ClonePhase, DBRuntimeID: snapshot.CloneRuntimeID,
	})
	if err != nil || observation.ExitEvidenceID <= 0 ||
		observation.QuiescenceEvidenceID <= observation.ExitEvidenceID || observation.ExitCode != 0 {
		return queue.PlanningResult{}, ErrPlanningCaptureUnavailable
	}
	capture, err := reader.ReadCompleteLiquibaseOutput(ctx,
		snapshot.Attempt.PlannedExecutorName, snapshot.Attempt.ExecutorID, captureDir,
		engineRuntime.CaptureIdentity{JobID: snapshot.Attempt.JobID, AttemptNo: snapshot.Attempt.AttemptNo,
			ExecutionDigest: snapshot.Attempt.ExecutionDigest, HelperDigest: snapshot.Capture.HelperDigest,
			ExitCode: observation.ExitCode}, maxStreamBytes)
	if err != nil {
		return queue.PlanningResult{}, ErrPlanningCaptureUnavailable
	}
	proof := queue.PlanningCaptureProof{
		StdoutDigest: capture.StdoutDigest, StdoutBytes: int64(len(capture.Stdout)),
		StderrDigest: capture.StderrDigest, StderrBytes: int64(len(capture.Stderr)),
		ManifestRef: snapshot.Capture.Manifest,
	}
	outputID, err := journal.RecordVerifiedPlanningCapture(ctx, snapshot.Attempt,
		snapshot.Attempt.WorkerGeneration, proof, at)
	if err != nil {
		return queue.PlanningResult{}, err
	}
	parsed, err := parseLiquibaseUpdateSQL(string(capture.Stdout))
	if err != nil {
		return queue.PlanningResult{}, ErrPlanningCaptureUnavailable
	}
	metadata := make([]planningcodec.Changeset, len(parsed))
	for i, item := range parsed {
		metadata[i] = planningcodec.Changeset{
			ID: item.ID, Author: item.Author, Path: item.Path,
			SQLHash: item.SQLHash, Checksum: item.Checksum,
		}
	}
	if _, err := journal.SavePlanningResult(ctx, snapshot.Attempt.JobID,
		snapshot.Attempt.AttemptNo, capture.StdoutDigest, metadata, at); err != nil {
		return queue.PlanningResult{}, err
	}
	if err := journal.CompletePlanning(ctx, snapshot.Attempt, snapshot.Attempt.WorkerGeneration,
		observation.ExitEvidenceID, observation.QuiescenceEvidenceID, outputID, at); err != nil {
		return queue.PlanningResult{}, err
	}
	result, found, err := journal.LoadPlanningResult(ctx, snapshot.Attempt.JobID)
	if err != nil || !found {
		return queue.PlanningResult{}, queue.ErrPlanningSchema
	}
	return result, nil
}
