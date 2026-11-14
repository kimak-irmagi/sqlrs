package queue

// Accepted cancellation of a current normal invocation. No replay dispatcher
// or interrupted-work recovery is activated here.
import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"time"
)

// RequestJobCancellation admits a queued/running, source-bound LB job in one
// short transaction, mutually exclusive with finalization intent selection.
// It takes no work gate, performs no external effect and emits no event.
// The caller may signal work only after a successful return. A commit error
// must be reconciled through a durable read before acceptance is returned.
// See liquibase-local-projection-cleanup-design.md, cancellation admission.
func (s *SQLiteStore) RequestJobCancellation(ctx context.Context, jobID, at string) (JobCancellation, error) {
	if s == nil || s.db == nil {
		return JobCancellation{}, ErrLiquibaseFinalization
	}
	if jobID == "" || !validFinalizationUTC(at) {
		return JobCancellation{}, ErrJobCancellationRejected
	}
	return withFinalization(ctx, s, func(f *finalizationConnection) (JobCancellation, error) {
		tx := f.tx
		var kind, status string
		var binding sql.NullString
		err := tx.QueryRowContext(ctx, `SELECT j.prepare_kind,j.status,b.job_id
FROM prepare_jobs j LEFT JOIN liquibase_job_bindings b ON b.job_id=j.job_id WHERE j.job_id=?`, jobID).Scan(&kind, &status, &binding)
		if errors.Is(err, sql.ErrNoRows) {
			return JobCancellation{}, ErrJobCancellationRejected
		}
		if err != nil {
			return JobCancellation{}, err
		}
		if kind != "lb" || !binding.Valid || (status != "queued" && status != "running") {
			return JobCancellation{}, ErrJobCancellationRejected
		}
		var intent bool
		if err := tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM prepare_liquibase_job_finalizations WHERE job_id=?)`, jobID).Scan(&intent); err != nil {
			return JobCancellation{}, err
		}
		if intent {
			return JobCancellation{}, ErrJobCancellationRejected
		}
		record, found, err := loadJobCancellation(ctx, tx, jobID)
		if err != nil {
			return JobCancellation{}, err
		}
		if !found {
			record = JobCancellation{JobID: jobID, RequestedAt: at}
			if _, err := tx.ExecContext(ctx, `INSERT INTO prepare_liquibase_job_cancellations(job_id,requested_at) VALUES(?,?)`, jobID, at); err != nil {
				return JobCancellation{}, err
			}
		}
		if err := f.commit(func(checkCtx context.Context, tx *sql.Tx) (bool, error) {
			observed, found, err := loadJobCancellation(checkCtx, tx, jobID)
			if err != nil {
				return false, err
			}
			if !found {
				return false, nil
			}
			if observed != record {
				return false, ErrJobCancellationIndeterminate
			}
			return true, nil
		}); err != nil {
			return JobCancellation{}, err
		}
		return record, nil
	})
}

// LoadJobCancellation also reads requests retained in recovery/terminal
// history. A later status does not erase a previously accepted cancellation.
// A missing row is distinct from a malformed/orphaned row: the latter returns
// no partial authority and cannot authorize ordinary execution as a fallback.
func (s *SQLiteStore) LoadJobCancellation(ctx context.Context, jobID string) (JobCancellation, bool, error) {
	if s == nil || s.db == nil || jobID == "" {
		return JobCancellation{}, false, ErrLiquibaseFinalization
	}
	return loadJobCancellation(ctx, s.db, jobID)
}

// SaveLiquibaseSessionCancellationDisposal freezes exact attached targets and
// capture intents under an already accepted job cancellation. It does not admit
// cancellation, resolve the invocation, select a final outcome or replay SQL.
// Caller retains job/session exclusion and must first journal the exact stop.
// Requirements: private-capture storage schema §3.4, PC12/PC17 and CS07.
func (s *SQLiteStore) SaveLiquibaseSessionCancellationDisposal(ctx context.Context, session LiquibaseSessionRecord, i LiquibaseInvocationRecord, at string) error {
	if s == nil || s.db == nil || !validFinalizationUTC(at) {
		return ErrCommonAttempt
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := verifyLiquibaseSessionStop(ctx, tx, session, i); err != nil {
		return err
	}
	// This helper validates immutable invocation identity, not cache authority.
	if err := verifyCacheSelectionInvocation(ctx, tx, session, i); err != nil {
		return err
	}
	var runner, database string
	if err := tx.QueryRowContext(ctx, `SELECT prepare_liquibase_sessions.runner_id,clone.db_runtime_id
FROM prepare_liquibase_sessions JOIN prepare_clones clone ON clone.clone_id=prepare_liquibase_sessions.clone_id AND clone.job_id=prepare_liquibase_sessions.job_id
WHERE prepare_liquibase_sessions.session_id IN (SELECT session_id FROM prepare_liquibase_sessions WHERE `+liquibaseSessionIdentityPredicate+`)
AND prepare_liquibase_sessions.runner_id=? AND prepare_liquibase_sessions.runner_phase='stopped' AND prepare_liquibase_sessions.retired_at IS NULL
AND EXISTS (SELECT 1 FROM prepare_liquibase_job_cancellations cancellation WHERE cancellation.job_id=prepare_liquibase_sessions.job_id)`, append(sessionIdentityArgs(session), session.RunnerID)...).Scan(&runner, &database); err != nil {
		return err
	}
	return s.savePrivateSessionDisposalTx(ctx, tx, session, "accepted_cancel", session.JobID, runner, database, at)
}

// savePrivateSessionDisposalTx shares only persistence/readback mechanics.
// Each caller verifies its own authority and exact stopped invocation first;
// immutable SQL guards independently resolve the reason-specific reference.
func (s *SQLiteStore) savePrivateSessionDisposalTx(ctx context.Context, tx *sql.Tx, session LiquibaseSessionRecord, reason, authority, runner, database, at string) error {
	if err := writePrivateSessionDisposalTx(ctx, tx, session, reason, authority, runner, database, at); err != nil {
		return err
	}
	if err := tx.Commit(); err != nil {
		readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		got, readErr := readPrivateSessionDisposal(readCtx, s.db, session)
		var missing int
		checkErr := s.db.QueryRowContext(readCtx, `SELECT count(*) FROM prepare_liquibase_capture_slots slot WHERE slot.session_id=? AND NOT EXISTS
(SELECT 1 FROM prepare_liquibase_capture_retirements r WHERE r.session_id=slot.session_id AND r.job_id=slot.job_id AND r.task_id=slot.task_id AND r.attempt_no=slot.attempt_no)`, session.SessionID).Scan(&missing)
		if readErr == nil && checkErr == nil && missing == 0 && got.reason == reason && got.authority == authority && got.runner == runner && got.database == database {
			return nil
		}
		return err
	}
	return nil
}

// RequestLiquibaseSessionCancellation records the coordinator's explicit
// cancellation intent for the exact unresolved invocation. It grants no replay
// or terminal transition: C01b still requires acknowledged stop and absence.
func (s *SQLiteStore) RequestLiquibaseSessionCancellation(ctx context.Context, session LiquibaseSessionRecord,
	i LiquibaseInvocationRecord, at string) error {
	if s == nil || s.db == nil || at == "" || i.SessionID != session.SessionID ||
		i.Attempt.JobID != session.JobID || i.Attempt.CloneID != session.CloneID || i.Attempt.WorkerGeneration != session.WorkerGeneration {
		return ErrCommonAttempt
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var exact int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM prepare_liquibase_sessions WHERE `+liquibaseSessionIdentityPredicate+`
AND runner_id=? AND retired_at IS NULL AND EXISTS (SELECT 1 FROM prepare_step_attempts a
JOIN prepare_liquibase_invocations i USING(job_id,task_id,attempt_no)
WHERE i.session_id=prepare_liquibase_sessions.session_id AND a.job_id=? AND a.task_id=? AND a.attempt_no=?
AND i.invocation_ref=? AND a.execution_digest=? AND a.worker_generation=prepare_liquibase_sessions.worker_generation
AND a.clone_id=prepare_liquibase_sessions.clone_id AND a.resolution IS NULL
AND a.phase IN ('planned','exec_requested','exec_completed'))`,
		append(sessionIdentityArgs(session), session.RunnerID, i.Attempt.JobID, i.Attempt.TaskID, i.Attempt.AttemptNo, i.InvocationRef, i.Attempt.ExecutionDigest)...).Scan(&exact); err != nil || exact != 1 {
		return ErrCommonAttempt
	}
	if _, err := appendSessionEvidenceTx(ctx, tx, i.Attempt, "cancellation_requested", map[string]any{"session_id": session.SessionID, "runner_id": session.RunnerID}, at); err != nil {
		return err
	}
	return tx.Commit()
}

// DiscardCancelledLiquibaseSession disposes a cancelled attempt only after
// exact physical retirement. It neither guesses its result nor creates a
// warning/replay. The append-only cancellation and stop facts survive a crash.
func (s *SQLiteStore) DiscardCancelledLiquibaseSession(ctx context.Context, session LiquibaseSessionRecord,
	i LiquibaseInvocationRecord, verifier LiquibaseSessionAbsenceVerifier, at string) error {
	return s.discardLiquibaseSession(ctx, session, i, verifier, at, true)
}

// verifyLiquibaseSessionStop rejects missing stop facts before invoking
// destructive physical cleanup. The resolution trigger rechecks them in the
// final transaction together with the verifier's new DB-absence observation.
func verifyLiquibaseSessionStop(ctx context.Context, reader sessionStopProofReader, session LiquibaseSessionRecord, i LiquibaseInvocationRecord) error {
	var exact int
	err := reader.QueryRowContext(ctx, `SELECT count(*) FROM prepare_attempt_evidence request
JOIN prepare_attempt_evidence ack USING(job_id,task_id,attempt_no,worker_generation)
JOIN prepare_attempt_evidence stopped USING(job_id,task_id,attempt_no,worker_generation)
WHERE request.job_id=? AND request.task_id=? AND request.attempt_no=? AND request.worker_generation=?
AND request.kind='termination_requested' AND ack.kind='termination_acknowledged' AND stopped.kind='executor_inspection'
AND request.evidence_id<ack.evidence_id AND ack.evidence_id<stopped.evidence_id
AND json_extract(request.facts_json,'$.clone_id')=?
AND json_extract(ack.facts_json,'$.clone_id')=json_extract(request.facts_json,'$.clone_id')
AND json_extract(stopped.facts_json,'$.clone_id')=json_extract(request.facts_json,'$.clone_id')
AND json_extract(request.facts_json,'$.execution_digest')=?
AND json_extract(ack.facts_json,'$.execution_digest')=json_extract(request.facts_json,'$.execution_digest')
AND json_extract(stopped.facts_json,'$.execution_digest')=json_extract(request.facts_json,'$.execution_digest')
AND json_extract(request.facts_json,'$.observation.session_id')=? AND json_extract(ack.facts_json,'$.observation.session_id')=?
AND json_extract(stopped.facts_json,'$.observation.session_id')=?
AND json_extract(request.facts_json,'$.observation.runner_id')=? AND json_extract(ack.facts_json,'$.observation.runner_id')=?
AND json_extract(stopped.facts_json,'$.observation.runner_id')=? AND json_extract(stopped.facts_json,'$.observation.runner_stopped')=1`,
		session.JobID, i.Attempt.TaskID, i.Attempt.AttemptNo, session.WorkerGeneration, session.CloneID, i.Attempt.ExecutionDigest,
		session.SessionID, session.SessionID, session.SessionID, session.RunnerID, session.RunnerID, session.RunnerID).Scan(&exact)
	if err != nil || exact == 0 {
		return ErrCommonAttempt
	}
	return nil
}

// discardLiquibaseSession shares physical retirement and atomic journal
// cleanup while retaining distinct natural-failure and cancellation proofs.
func (s *SQLiteStore) discardLiquibaseSession(ctx context.Context, session LiquibaseSessionRecord,
	invocation LiquibaseInvocationRecord, verifier LiquibaseSessionAbsenceVerifier, at string, cancelled bool) error {
	if s == nil || s.db == nil || verifier == nil || at == "" || invocation.SessionID != session.SessionID ||
		invocation.Attempt.JobID != session.JobID || invocation.Attempt.CloneID != session.CloneID ||
		invocation.Attempt.WorkerGeneration != session.WorkerGeneration {
		return ErrCommonAttempt
	}
	var clone ClonePlan
	var image, state sql.NullString
	var runtimeID string
	eligibility := `i.complete_at IS NOT NULL AND i.exit_code<>0 AND a.phase='exec_completed'`
	clonePhase := `c.phase='quiescent'`
	if cancelled {
		clonePhase = `c.phase IN ('active','quiescent')`
		eligibility = `a.phase IN ('planned','exec_requested','exec_completed') AND EXISTS (
SELECT 1 FROM prepare_attempt_evidence cancel WHERE cancel.job_id=a.job_id AND cancel.task_id=a.task_id
AND cancel.attempt_no=a.attempt_no AND cancel.worker_generation=a.worker_generation AND cancel.kind='cancellation_requested'
AND json_extract(cancel.facts_json,'$.observation.session_id')=i.session_id
AND json_extract(cancel.facts_json,'$.observation.runner_id')=?)`
	}
	args := append(append([]any{session.CloneID, session.JobID}, sessionIdentityArgs(session)...), session.RunnerID,
		session.SessionID, invocation.Attempt.JobID, invocation.Attempt.TaskID, invocation.Attempt.AttemptNo, invocation.InvocationRef)
	if cancelled {
		args = append(args, session.RunnerID)
	}
	args = append(args, invocation.Attempt.ExecutionDigest)
	if err := s.db.QueryRowContext(ctx, `SELECT c.clone_id,c.clone_generation,c.source_kind,
c.source_image_id,c.source_state_id,c.clone_ref,c.planned_db_runtime_name,c.db_runtime_id
FROM prepare_clones c WHERE c.clone_id=? AND c.job_id=? AND `+clonePhase+`
AND EXISTS (SELECT 1 FROM prepare_liquibase_sessions WHERE `+liquibaseSessionIdentityPredicate+`
  AND runner_id=? AND runner_phase='stopped' AND retired_at IS NULL)
AND EXISTS (SELECT 1 FROM prepare_liquibase_invocations i JOIN prepare_step_attempts a
  ON a.job_id=i.job_id AND a.task_id=i.task_id AND a.attempt_no=i.attempt_no
  WHERE i.session_id=? AND i.job_id=? AND i.task_id=? AND i.attempt_no=?
    AND i.invocation_ref=? AND `+eligibility+`
    AND a.resolution IS NULL AND a.execution_digest=?)`, args...).Scan(&clone.ID, &clone.Generation,
		&clone.SourceKind, &image, &state, &clone.Ref, &clone.PlannedRuntimeName, &runtimeID); err != nil {
		return fmt.Errorf("session discard preflight: %w", err)
	}
	if !dockerContainerID.MatchString(runtimeID) {
		return ErrCommonAttempt
	}
	clone.SourceImageID, clone.SourceStateID = image.String, state.String
	if cancelled {
		if err := verifyLiquibaseSessionStop(ctx, s.db, session, invocation); err != nil {
			return err
		}
	}
	saved, found, err := s.ReadLiquibaseSession(ctx, session.SessionID)
	if err != nil || !found {
		return ErrCommonAttempt
	}
	if err := verifier.VerifyReleasedLiquibaseSession(ctx, saved, clone, runtimeID); err != nil {
		return err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err := appendSessionEvidenceTx(ctx, tx, invocation.Attempt, "db_quiescence", map[string]any{
		"session_id": session.SessionID, "runtime_name": clone.PlannedRuntimeName,
		"runtime_id": runtimeID, "runtime_absent": 1}, at); err != nil {
		return err
	}
	if cancelled {
		if _, err := tx.ExecContext(ctx, `UPDATE prepare_clones SET phase='quiescent' WHERE clone_id=? AND job_id=? AND phase IN ('active','quiescent')`, session.CloneID, session.JobID); err != nil {
			return err
		}
	}
	result, err := tx.ExecContext(ctx, `UPDATE prepare_step_attempts SET phase='closed',resolution='discarded_clone'
WHERE job_id=? AND task_id=? AND attempt_no=? AND clone_id=? AND execution_digest=?
AND worker_generation=? AND executor_kind='session_exec' AND phase IN ('planned','exec_requested','exec_completed') AND resolution IS NULL`,
		invocation.Attempt.JobID, invocation.Attempt.TaskID, invocation.Attempt.AttemptNo, session.CloneID,
		invocation.Attempt.ExecutionDigest, session.WorkerGeneration)
	if err != nil {
		return err
	}
	if oneRowChanged(result) != nil {
		return ErrCommonAttempt
	}
	problem := `{"code":"liquibase_failed","message":"Liquibase child returned a failure result"}`
	if cancelled {
		problem = `{"code":"cancelled","message":"job cancelled"}`
	}
	result, err = tx.ExecContext(ctx, `UPDATE prepare_tasks SET status='failed',finished_at=?,error_json=?
WHERE job_id=? AND task_id=? AND status IN ('running','recovery_required')`, at, problem,
		invocation.Attempt.JobID, invocation.Attempt.TaskID)
	if err != nil {
		return err
	}
	if oneRowChanged(result) != nil {
		return ErrCommonAttempt
	}
	if _, err := tx.ExecContext(ctx, `UPDATE prepare_liquibase_invocations SET retired_at=? WHERE session_id=? AND retired_at IS NULL`, at, session.SessionID); err != nil {
		return err
	}
	result, err = tx.ExecContext(ctx, `UPDATE prepare_clones SET phase='released' WHERE clone_id=? AND job_id=?
AND phase='quiescent' AND planned_db_runtime_name=? AND db_runtime_id=?`, session.CloneID, session.JobID, clone.PlannedRuntimeName, runtimeID)
	if err != nil {
		return fmt.Errorf("session clone retirement: %w", err)
	}
	if oneRowChanged(result) != nil {
		return ErrCommonAttempt
	}
	result, err = tx.ExecContext(ctx, `UPDATE prepare_liquibase_sessions SET runner_phase='retired',retired_at=?,
cursor_image_id=NULL,cursor_state_id=NULL WHERE `+liquibaseSessionIdentityPredicate+`
AND runner_id=? AND runner_phase='stopped' AND retired_at IS NULL`,
		append(append([]any{at}, sessionIdentityArgs(session)...), session.RunnerID)...)
	if err != nil {
		return fmt.Errorf("session journal retirement: %w", err)
	}
	if oneRowChanged(result) != nil {
		return ErrCommonAttempt
	}
	return tx.Commit()
}
