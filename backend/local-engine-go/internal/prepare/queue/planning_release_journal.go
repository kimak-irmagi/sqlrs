package queue

// Normal-path journal operations from the approved Liquibase prototype.
// Installation and runtime activation are separate boundaries; these methods
// do not enable replay. See issue89-normal-path-storage-extraction.md.

import (
	"context"
	"database/sql"
	"time"
)

// PlanningCaptureDisposal is immutable authority for physical cleanup only.
// It never selects success over a later accepted cancellation/failure and stores
// neither protected output nor a filesystem path. Private-capture schema §3.5.
type PlanningCaptureDisposal struct {
	JobID, TaskID                     string
	AttemptNo                         int
	Reason, AuthorityRef, RequestedAt string
}

// SavePrivatePlanningCaptureDisposal records exact existing durable authority
// before Docker removal. The SQL guard resolves its reason-specific reference;
// an unsaved buffer/hash is not a plan. A different immutable intent is never
// overwritten. Callers retain job exclusion and inspect the currently selected
// outcome independently when cleanup finishes. PC12/PC14.
func (s *SQLiteStore) SavePrivatePlanningCaptureDisposal(ctx context.Context, attempt AttemptRecord, generation int, reason, authority, at string) (PlanningCaptureDisposal, error) {
	if err := ctx.Err(); err != nil {
		return PlanningCaptureDisposal{}, err
	}
	if s == nil || s.db == nil || attempt.JobID == "" || attempt.TaskID != "plan" || attempt.AttemptNo < 1 || generation != attempt.WorkerGeneration || authority == "" || !validFinalizationUTC(at) {
		return PlanningCaptureDisposal{}, ErrPlanningSchema
	}
	switch reason {
	case "plan_saved", "selected_failure", "accepted_cancel", "common_discard":
	default:
		return PlanningCaptureDisposal{}, ErrPlanningSchema
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return PlanningCaptureDisposal{}, err
	}
	defer tx.Rollback()
	var exact int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM prepare_step_attempts a JOIN prepare_planning_attempt_artifacts artifact USING(job_id,task_id,attempt_no)
WHERE a.job_id=? AND a.task_id='plan' AND a.attempt_no=? AND a.clone_id=? AND a.execution_digest=? AND a.worker_generation=?
AND a.planned_executor_name=? AND coalesce(a.executor_id,'')=? AND artifact.capture_location='container_private' AND artifact.capture_protocol_revision='lb-capture-v2'`,
		attempt.JobID, attempt.AttemptNo, attempt.CloneID, attempt.ExecutionDigest, generation, attempt.PlannedExecutorName, attempt.ExecutorID).Scan(&exact); err != nil || exact != 1 {
		return PlanningCaptureDisposal{}, ErrPlanningSchema
	}
	if saved, found, err := readPrivatePlanningDisposal(ctx, tx, attempt.JobID, attempt.AttemptNo); err != nil {
		return PlanningCaptureDisposal{}, err
	} else if found {
		if saved.Reason != reason || saved.AuthorityRef != authority {
			return PlanningCaptureDisposal{}, ErrPlanningSchema
		}
		return saved, nil
	}
	want := PlanningCaptureDisposal{JobID: attempt.JobID, TaskID: "plan", AttemptNo: attempt.AttemptNo, Reason: reason, AuthorityRef: authority, RequestedAt: at}
	if _, err := tx.ExecContext(ctx, `INSERT INTO prepare_liquibase_planning_capture_disposals(job_id,task_id,attempt_no,reason,authority_ref,requested_at)VALUES(?,'plan',?,?,?,?)`, want.JobID, want.AttemptNo, want.Reason, want.AuthorityRef, want.RequestedAt); err != nil {
		return PlanningCaptureDisposal{}, err
	}
	if err := tx.Commit(); err != nil {
		readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		saved, found, readErr := readPrivatePlanningDisposal(readCtx, s.db, want.JobID, want.AttemptNo)
		if readErr == nil && found && saved.Reason == want.Reason && saved.AuthorityRef == want.AuthorityRef {
			return saved, nil
		}
		return PlanningCaptureDisposal{}, ErrPlanningSchema
	}
	return want, nil
}

func readPrivatePlanningDisposal(ctx context.Context, reader privateReadinessReader, jobID string, attempt int) (PlanningCaptureDisposal, bool, error) {
	var row PlanningCaptureDisposal
	err := reader.QueryRowContext(ctx, `SELECT d.job_id,d.task_id,d.attempt_no,d.reason,d.authority_ref,d.requested_at
FROM prepare_liquibase_planning_capture_disposals d JOIN prepare_planning_attempt_artifacts artifact USING(job_id,task_id,attempt_no)
WHERE d.job_id=? AND d.task_id='plan' AND d.attempt_no=? AND artifact.capture_location='container_private' AND artifact.capture_protocol_revision='lb-capture-v2'`, jobID, attempt).Scan(&row.JobID, &row.TaskID, &row.AttemptNo, &row.Reason, &row.AuthorityRef, &row.RequestedAt)
	if err == sql.ErrNoRows {
		return PlanningCaptureDisposal{}, false, nil
	}
	if err != nil {
		return PlanningCaptureDisposal{}, false, err
	}
	switch row.Reason {
	case "plan_saved", "selected_failure", "accepted_cancel", "common_discard":
	default:
		return PlanningCaptureDisposal{}, false, ErrPlanningSchema
	}
	if row.AuthorityRef == "" || !validFinalizationUTC(row.RequestedAt) {
		return PlanningCaptureDisposal{}, false, ErrPlanningSchema
	}
	return row, true, nil
}

// privatePlanningRetirementScope binds disposal and the saved DB-absence fact
// before the exact executor can be confirmed absent. It never chooses the job
// outcome or treats a missing capture file as non-start evidence.
const privatePlanningRetirementScope = `artifact.capture_location='container_private' AND artifact.capture_protocol_revision='lb-capture-v2'
AND EXISTS (SELECT 1 FROM prepare_liquibase_planning_capture_disposals disposition
 WHERE disposition.job_id=a.job_id AND disposition.task_id=a.task_id AND disposition.attempt_no=a.attempt_no)
AND (EXISTS (SELECT 1 FROM prepare_attempt_evidence quiet
 WHERE quiet.job_id=a.job_id AND quiet.task_id=a.task_id AND quiet.attempt_no=a.attempt_no AND quiet.worker_generation=a.worker_generation
 AND quiet.kind='db_quiescence' AND json_extract(quiet.facts_json,'$.observation.runtime_absent')=1
 AND json_extract(quiet.facts_json,'$.observation.runtime_name')=clone.planned_db_runtime_name
 AND json_extract(quiet.facts_json,'$.observation.runtime_id') IS clone.db_runtime_id)
OR EXISTS(SELECT 1 FROM prepare_liquibase_planning_db_ownership owner
 WHERE owner.job_id=a.job_id AND owner.task_id=a.task_id AND owner.attempt_no=a.attempt_no AND owner.clone_id=a.clone_id
 AND a.phase='closed' AND a.resolution='planning_result_saved'
 AND owner.plan_digest=(SELECT plan_digest FROM prepare_planning_intents WHERE job_id=a.job_id)))`

// RetirePrivatePlanningCapture confirms the absence of one exact private parent
// and stamps executor/stdout/stderr/manifest together. The caller must save its
// physical disposal intent before removing the parent. Protected host settings
// are deliberately not retired here. Callback work runs outside SQL. Requirements:
// private-capture schema §3.5, PC12/PC14; the legacy per-file method is not used.
func (s *SQLiteStore) RetirePrivatePlanningCapture(ctx context.Context, attempt AttemptRecord, generation int, verifier PlanningArtifactAbsenceVerifier, at string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if s == nil || s.db == nil || verifier == nil || attempt.JobID == "" || attempt.TaskID != "plan" || attempt.AttemptNo < 1 || generation != attempt.WorkerGeneration || !validFinalizationUTC(at) {
		return ErrPlanningSchema
	}
	// Pre-start cancellation already has exact DB/executor absence in its
	// dedicated closure proof, not a fabricated db_quiescence fact. Loading
	// validates that proof's hash/scope; the write rechecks its complete SQL
	// binding. Requirements: private-capture §3.5 and cancelled preview UP02/03.
	saved, found, err := s.LoadPlanningAttempt(ctx, attempt.JobID, attempt.AttemptNo)
	if err != nil || !found {
		return ErrPlanningSchema
	}
	scope := privatePlanningRetirementScope
	if saved.CancelledDisposal {
		scope = `artifact.capture_location='container_private' AND artifact.capture_protocol_revision='lb-capture-v2'
AND EXISTS (SELECT 1 FROM prepare_liquibase_planning_capture_disposals disposition
 WHERE disposition.job_id=a.job_id AND disposition.task_id=a.task_id AND disposition.attempt_no=a.attempt_no)
AND (` + cancelledPlanningProofSQL("a", "clone") + `)`
	}
	args := []any{attempt.JobID, attempt.AttemptNo, attempt.CloneID, attempt.ExecutionDigest, generation, attempt.PlannedExecutorName, attempt.ExecutorID}
	predicate := `a.job_id=? AND a.task_id='plan' AND a.attempt_no=? AND a.clone_id=? AND a.execution_digest=? AND a.worker_generation=? AND a.planned_executor_name=? AND coalesce(a.executor_id,'')=? AND ` + scope
	read := func(readCtx context.Context) (bool, error) {
		var times [4]sql.NullString
		err := s.db.QueryRowContext(readCtx, `SELECT artifact.executor_retired_at,artifact.output_retired_at,artifact.stderr_retired_at,artifact.manifest_retired_at
FROM prepare_planning_attempt_artifacts artifact JOIN prepare_step_attempts a USING(job_id,task_id,attempt_no)
JOIN prepare_clones clone ON clone.clone_id=a.clone_id AND clone.job_id=a.job_id WHERE `+predicate, args...).Scan(&times[0], &times[1], &times[2], &times[3])
		if err != nil {
			return false, ErrPlanningSchema
		}
		count := 0
		for _, stamp := range times {
			if stamp.Valid {
				if !validFinalizationUTC(stamp.String) {
					return false, ErrPlanningSchema
				}
				count++
			}
		}
		if count != 0 && count != 4 {
			return false, ErrPlanningSchema
		}
		return count == 4, nil
	}
	already, err := read(ctx)
	if err != nil {
		return err
	}
	if err := verifier.VerifyPlanningArtifactAbsent(ctx, "executor", attempt.PlannedExecutorName, attempt.ExecutorID); err != nil {
		return ErrPlanningSchema
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if already {
		return nil
	}
	result, writeErr := s.db.ExecContext(ctx, `UPDATE prepare_planning_attempt_artifacts SET executor_retired_at=?,output_retired_at=?,stderr_retired_at=?,manifest_retired_at=?
WHERE executor_retired_at IS NULL AND output_retired_at IS NULL AND stderr_retired_at IS NULL AND manifest_retired_at IS NULL
AND (job_id,task_id,attempt_no) IN (SELECT artifact.job_id,artifact.task_id,artifact.attempt_no
FROM prepare_planning_attempt_artifacts artifact JOIN prepare_step_attempts a USING(job_id,task_id,attempt_no)
JOIN prepare_clones clone ON clone.clone_id=a.clone_id AND clone.job_id=a.job_id WHERE `+predicate+`)`, append([]any{at, at, at, at}, args...)...)
	if writeErr == nil && oneRowChanged(result) == nil {
		return nil
	}
	// Native absence is stable for the exact full ID. Recover a lost write
	// reply by rereading the immutable tuple, never by deleting files again.
	readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
	defer cancel()
	if complete, err := read(readCtx); err == nil && complete {
		return nil
	}
	return ErrPlanningSchema
}

// PlanningArtifactAbsenceVerifier must check the exact logical ref against
// physical Docker/filesystem state. A failed inspection is not absence.
type PlanningArtifactAbsenceVerifier interface {
	VerifyPlanningArtifactAbsent(context.Context, string, string, string) error
}

// RetirePlanningArtifact records one monotonic cleanup milestone only after
// physical absence has been independently verified. It is repeatable after a
// crash between deletion and journal commit. See F03a in the test design.
func (s *SQLiteStore) RetirePlanningArtifact(ctx context.Context, attempt AttemptRecord, generation int, kind string, verifier PlanningArtifactAbsenceVerifier, at string) error {
	if s == nil || s.db == nil || verifier == nil || attempt.JobID == "" || attempt.TaskID != "plan" || attempt.AttemptNo < 1 || generation != attempt.WorkerGeneration || at == "" {
		return ErrPlanningSchema
	}
	column := ""
	switch kind {
	case "executor":
		column = "executor_retired_at"
	case "input":
		column = "protected_input_retired_at"
	case "stdout":
		column = "output_retired_at"
	case "stderr":
		column = "stderr_retired_at"
	case "manifest":
		column = "manifest_retired_at"
	default:
		return ErrPlanningSchema
	}
	var phase, resolution, executorID, inputRef, stdoutRef, stderrRef, manifestRef, plannedName string
	var retired, executorRetired sql.NullString
	err := s.db.QueryRowContext(ctx, `SELECT a.phase,a.resolution,coalesce(a.executor_id,''),a.planned_executor_name,capture.protected_input_ref,capture.complete_output_ref,capture.complete_stderr_ref,capture.capture_manifest_ref,capture.`+column+`,capture.executor_retired_at FROM prepare_step_attempts a JOIN prepare_planning_attempt_artifacts capture ON capture.job_id=a.job_id AND capture.task_id=a.task_id AND capture.attempt_no=a.attempt_no WHERE a.job_id=? AND a.task_id='plan' AND a.attempt_no=? AND a.clone_id=? AND a.execution_digest=? AND a.worker_generation=? AND a.planned_executor_name=?`, attempt.JobID, attempt.AttemptNo, attempt.CloneID, attempt.ExecutionDigest, generation, attempt.PlannedExecutorName).Scan(&phase, &resolution, &executorID, &plannedName, &inputRef, &stdoutRef, &stderrRef, &manifestRef, &retired, &executorRetired)
	if err != nil || phase != "closed" || resolution == "" || kind != "executor" && !executorRetired.Valid {
		return ErrPlanningSchema
	}
	ref := plannedName
	switch kind {
	case "input":
		ref = inputRef
	case "stdout":
		ref = stdoutRef
	case "stderr":
		ref = stderrRef
	case "manifest":
		ref = manifestRef
	}
	if err := verifier.VerifyPlanningArtifactAbsent(ctx, kind, ref, executorID); err != nil {
		return err
	}
	if retired.Valid {
		return nil
	}
	result, err := s.db.ExecContext(ctx, `UPDATE prepare_planning_attempt_artifacts SET `+column+`=? WHERE job_id=? AND task_id='plan' AND attempt_no=? AND `+column+` IS NULL`, at, attempt.JobID, attempt.AttemptNo)
	if err != nil || oneRowChanged(result) != nil {
		return ErrPlanningSchema
	}
	return nil
}

// ReleasePlanningClone verifies physical absence of the exact preview DB
// before marking a successfully resolved planning clone released. Other
// safe-replay resolutions use the shared clone recovery policy.
func (s *SQLiteStore) ReleasePlanningClone(ctx context.Context, attempt AttemptRecord, generation int, verifier CloneAbsenceVerifier) error {
	if s == nil || s.db == nil || verifier == nil || attempt.JobID == "" || attempt.TaskID != "plan" || attempt.AttemptNo < 1 || attempt.CloneID == "" || attempt.ExecutionDigest == "" || attempt.PlannedExecutorName == "" || generation != attempt.WorkerGeneration {
		return ErrPlanningSchema
	}
	// A transferred row remains history after normal session release; it never
	// revives the planner's destructive authority (schema §4, PG05).
	var owned int
	if err := s.db.QueryRowContext(ctx, "SELECT count(*) FROM prepare_liquibase_planning_db_ownership WHERE clone_id=?", attempt.CloneID).Scan(&owned); err != nil || owned != 0 {
		return ErrPlanningSchema
	}
	var clone ClonePlan
	var image, state sql.NullString
	var runtimeID, phase string
	err := s.db.QueryRowContext(ctx, `SELECT c.clone_id,c.clone_generation,c.source_kind,c.source_image_id,c.source_state_id,c.clone_ref,c.planned_db_runtime_name,c.db_runtime_id,c.phase FROM prepare_clones c JOIN prepare_step_attempts a ON a.clone_id=c.clone_id AND a.job_id=c.job_id JOIN prepare_planning_intents i ON i.job_id=a.job_id AND i.task_id=a.task_id AND i.execution_digest=a.execution_digest JOIN prepare_tasks t ON t.job_id=a.job_id AND t.task_id=a.task_id WHERE c.clone_id=? AND c.job_id=? AND c.phase IN ('quiescent','released') AND a.task_id='plan' AND a.attempt_no=? AND a.worker_generation=? AND a.planned_executor_name=? AND a.phase='closed' AND a.resolution='planning_result_saved' AND t.status='succeeded' AND i.result_attempt_no=a.attempt_no`, attempt.CloneID, attempt.JobID, attempt.AttemptNo, generation, attempt.PlannedExecutorName).Scan(&clone.ID, &clone.Generation, &clone.SourceKind, &image, &state, &clone.Ref, &clone.PlannedRuntimeName, &runtimeID, &phase)
	if err != nil || !dockerContainerID.MatchString(runtimeID) {
		return ErrPlanningSchema
	}
	clone.SourceImageID, clone.SourceStateID = image.String, state.String
	if err := verifier.VerifyReleasedClone(ctx, clone, runtimeID); err != nil {
		return err
	}
	if phase == "released" {
		return nil
	}
	result, err := s.db.ExecContext(ctx, `UPDATE prepare_clones SET phase='released' WHERE clone_id=? AND job_id=? AND phase='quiescent' AND planned_db_runtime_name=? AND db_runtime_id=? AND EXISTS (SELECT 1 FROM prepare_step_attempts a JOIN prepare_planning_intents i ON i.job_id=a.job_id AND i.task_id=a.task_id WHERE a.clone_id=prepare_clones.clone_id AND a.job_id=prepare_clones.job_id AND a.task_id='plan' AND a.attempt_no=? AND a.worker_generation=? AND a.execution_digest=i.execution_digest AND a.phase='closed' AND a.resolution='planning_result_saved')`, clone.ID, attempt.JobID, clone.PlannedRuntimeName, runtimeID, attempt.AttemptNo, generation)
	if err != nil || oneRowChanged(result) != nil {
		return ErrPlanningSchema
	}
	return nil
}
