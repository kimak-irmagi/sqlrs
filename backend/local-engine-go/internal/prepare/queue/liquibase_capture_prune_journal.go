package queue

// Normal-path journal operations from the approved Liquibase prototype.
// Installation and runtime activation are separate boundaries; these methods
// do not enable replay. See issue89-normal-path-storage-extraction.md.

import (
	"context"
	"database/sql"
	"reflect"
)

// ReadLiquibaseCapturePrunePlan reads one consistent snapshot. A nil result is
// valid only for a proven first invocation, including after a cached prefix.
// Read failures and broken predecessor links are never interpreted as nil.
func (s *SQLiteStore) ReadLiquibaseCapturePrunePlan(ctx context.Context, session LiquibaseSessionRecord, invocation LiquibaseInvocationRecord) (*CapturePrunePlan, error) {
	if s == nil || s.db == nil {
		return nil, ErrCommonAttempt
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	plan, err := readCapturePrunePlanTx(ctx, tx, session, invocation)
	if err != nil {
		return nil, err
	}
	return plan, tx.Rollback()
}

// readCapturePrunePlanTx validates current and prior identities independently
// of caller fields. Confirmed history may outlive an evictable state, but an
// unconfirmed retirement still needs the actual successful publication.
func readCapturePrunePlanTx(ctx context.Context, tx *sql.Tx, session LiquibaseSessionRecord, invocation LiquibaseInvocationRecord) (*CapturePrunePlan, error) {
	if invocation.ProtocolRevision != "lb-step-capture-v2" || invocation.SessionID != session.SessionID || invocation.HelperDigest != session.CaptureHelperDigest ||
		invocation.Attempt.JobID != session.JobID || invocation.Attempt.CloneID != session.CloneID ||
		invocation.Attempt.WorkerGeneration != session.WorkerGeneration || !dockerContainerID.MatchString(session.RunnerID) {
		return nil, ErrCommonAttempt
	}
	var exact int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM prepare_liquibase_sessions WHERE `+liquibaseSessionIdentityPredicate+`
AND runner_id=? AND capture_location='container_private' AND capture_protocol_revision='lb-step-capture-v2'`, append(sessionIdentityArgs(session), session.RunnerID)...).Scan(&exact); err != nil {
		return nil, err
	} else if exact != 1 {
		return nil, ErrCommonAttempt
	}
	var slot int
	var prior sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT slot.slot_no,slot.prior_retirement_ref
FROM prepare_liquibase_invocations i JOIN prepare_step_attempts a USING(job_id,task_id,attempt_no)
JOIN prepare_liquibase_capture_slots slot USING(job_id,task_id,attempt_no,session_id)
WHERE i.session_id=? AND i.job_id=? AND i.task_id=? AND i.attempt_no=?
AND i.invocation_ref=? AND i.stdout_ref=? AND i.stderr_ref=? AND i.manifest_ref=?
AND i.helper_digest=? AND i.protocol_revision='lb-step-capture-v2'
AND a.clone_id=? AND a.worker_generation=? AND a.execution_digest=? AND a.executor_kind='session_exec'`,
		session.SessionID, session.JobID, invocation.Attempt.TaskID, invocation.Attempt.AttemptNo,
		invocation.InvocationRef, invocation.StdoutRef, invocation.StderrRef, invocation.ManifestRef,
		invocation.HelperDigest, session.CloneID, session.WorkerGeneration, invocation.Attempt.ExecutionDigest).Scan(&slot, &prior)
	if err != nil {
		return nil, err
	}
	if slot != invocation.CaptureSlot || prior.Valid != (invocation.PriorRetirementRef != nil) ||
		prior.Valid && prior.String != *invocation.PriorRetirementRef {
		return nil, ErrCommonAttempt
	}
	if !prior.Valid {
		var previous int
		err := tx.QueryRowContext(ctx, `SELECT count(*) FROM prepare_liquibase_invocations old
JOIN prepare_tasks t ON t.job_id=old.job_id AND t.task_id=old.task_id
JOIN prepare_tasks current ON current.job_id=old.job_id AND current.task_id=?
WHERE old.session_id=? AND old.invocation_ref<>? AND t.position<=current.position`,
			invocation.Attempt.TaskID, session.SessionID, invocation.InvocationRef).Scan(&previous)
		if err != nil {
			return nil, err
		}
		if previous != 0 {
			return nil, ErrCommonAttempt
		}
		return nil, nil
	}
	plan := &CapturePrunePlan{RetirementRef: prior.String, SessionID: session.SessionID,
		RunnerID: session.RunnerID, WorkerGeneration: session.WorkerGeneration,
		HelperDigest: session.CaptureHelperDigest, NextInvocationRef: invocation.InvocationRef}
	var verified, absence sql.NullString
	var priorSlot sql.NullInt64
	err = tx.QueryRowContext(ctx, `SELECT r.job_id,r.task_id,r.attempt_no,old.invocation_ref,old.completion_digest,
r.verified_at,r.absence_kind,slot.slot_no
FROM prepare_liquibase_capture_retirements r
JOIN prepare_liquibase_invocations old USING(job_id,task_id,attempt_no,session_id)
JOIN prepare_step_attempts a USING(job_id,task_id,attempt_no)
JOIN prepare_tasks t USING(job_id,task_id)
LEFT JOIN prepare_liquibase_capture_slots slot USING(job_id,task_id,attempt_no,session_id)
WHERE r.retirement_ref=? AND r.session_id=? AND r.reason='published_prune'
AND old.invocation_ref<>? AND old.helper_digest=? AND old.protocol_revision='lb-step-capture-v2'
AND old.complete_at IS NOT NULL AND old.exit_code=0
AND a.clone_id=? AND a.worker_generation=? AND a.executor_kind='session_exec' AND a.phase='closed' AND a.resolution='published'
AND t.status='succeeded' AND t.type='state_execute'
AND (r.verified_at IS NOT NULL OR EXISTS (
 SELECT 1 FROM states state JOIN prepare_state_bindings binding USING(state_id)
 JOIN liquibase_state_bindings lb USING(state_id)
 JOIN prepare_attempt_decisions decision ON decision.job_id=a.job_id AND decision.task_id=a.task_id AND decision.attempt_no=a.attempt_no
 WHERE state.state_id=t.output_state_id AND state.prepare_kind='lb'
 AND binding.execution_digest=a.execution_digest AND lb.execution_fingerprint=? AND decision.choice='publish_original'))`,
		plan.RetirementRef, session.SessionID, invocation.InvocationRef, session.CaptureHelperDigest,
		session.CloneID, session.WorkerGeneration, session.ExecutionFingerprint).Scan(
		&plan.priorJob, &plan.priorTask, &plan.priorAttempt, &plan.PriorInvocationRef,
		&plan.PriorManifestDigest, &verified, &absence, &priorSlot)
	if err != nil {
		return nil, err
	}
	if !bindingDigest.MatchString(plan.PriorManifestDigest) || verified.Valid != absence.Valid || verified.Valid == priorSlot.Valid {
		return nil, ErrCommonAttempt
	}
	if verified.Valid {
		if !validFinalizationUTC(verified.String) || absence.String != "path_absent" && absence.String != "container_absent" {
			return nil, ErrCommonAttempt
		}
		plan.VerifiedAt, plan.AbsenceKind = verified.String, absence.String
	} else {
		if int(priorSlot.Int64) != 3-slot {
			return nil, ErrCommonAttempt
		}
	}
	// The reservation used the other of exactly two slots. Keep that identity
	// stable after its row disappears, rather than returning a different plan.
	plan.priorSlot = 3 - slot
	return plan, nil
}

// ConfirmLiquibaseCapturePruned records a read-only observation then releases
// exactly the previous slot. This never changes B's result/publication or starts
// SQL. A failed COMMIT is settled on its owned connection before full readback.
func (s *SQLiteStore) ConfirmLiquibaseCapturePruned(ctx context.Context, session LiquibaseSessionRecord, invocation LiquibaseInvocationRecord, verifier LiquibaseCapturePruneVerifier, at string) error {
	if !validFinalizationUTC(at) {
		return ErrCommonAttempt
	}
	observed, err := s.ReadLiquibaseCapturePrunePlan(ctx, session, invocation)
	if err != nil {
		return err
	}
	if observed == nil {
		return ErrCommonAttempt
	}
	if observed.VerifiedAt != "" {
		return nil
	}
	if verifier == nil {
		return ErrCommonAttempt
	}
	if err := verifier.VerifyLiquibaseCapturePruned(ctx, *observed); err != nil {
		return err
	}
	_, err = withFinalization(ctx, s, func(f *finalizationConnection) (struct{}, error) {
		beforePlan, err := readCapturePrunePlanTx(ctx, f.tx, session, invocation)
		if err != nil {
			return struct{}{}, err
		}
		if !reflect.DeepEqual(beforePlan, observed) {
			return struct{}{}, ErrCommonAttempt
		}
		before, err := readPrivateFailureCaptureState(ctx, f.tx, session)
		if err != nil {
			return struct{}{}, err
		}
		result, err := f.tx.ExecContext(ctx, `UPDATE prepare_liquibase_capture_retirements SET verified_at=?,absence_kind='path_absent'
WHERE retirement_ref=? AND session_id=? AND verified_at IS NULL AND absence_kind IS NULL`, at, observed.RetirementRef, session.SessionID)
		if err != nil {
			return struct{}{}, err
		}
		if err := oneRowChanged(result); err != nil {
			return struct{}{}, err
		}
		result, err = f.tx.ExecContext(ctx, `DELETE FROM prepare_liquibase_capture_slots WHERE session_id=? AND slot_no=? AND job_id=? AND task_id=? AND attempt_no=?`,
			session.SessionID, observed.priorSlot, observed.priorJob, observed.priorTask, observed.priorAttempt)
		if err != nil {
			return struct{}{}, err
		}
		if err := oneRowChanged(result); err != nil {
			return struct{}{}, err
		}
		wantedPlan, err := readCapturePrunePlanTx(ctx, f.tx, session, invocation)
		if err != nil {
			return struct{}{}, err
		}
		wanted, err := readPrivateFailureCaptureState(ctx, f.tx, session)
		if err != nil {
			return struct{}{}, err
		}
		return struct{}{}, f.commit(func(readCtx context.Context, tx *sql.Tx) (bool, error) {
			actualPlan, err := readCapturePrunePlanTx(readCtx, tx, session, invocation)
			if err != nil {
				return false, err
			}
			actual, err := readPrivateFailureCaptureState(readCtx, tx, session)
			if err != nil {
				return false, err
			}
			if reflect.DeepEqual(actualPlan, wantedPlan) && reflect.DeepEqual(actual, wanted) {
				return true, nil
			}
			if reflect.DeepEqual(actualPlan, beforePlan) && reflect.DeepEqual(actual, before) {
				return false, nil
			}
			return false, ErrCommonAttempt
		})
	})
	return err
}
