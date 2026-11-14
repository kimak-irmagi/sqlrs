package queue

// Normal-path journal operations from the approved Liquibase prototype.
// Installation and runtime activation are separate boundaries; these methods
// do not enable replay. See issue89-normal-path-storage-extraction.md.

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"
)

// reservePrivateCaptureTx runs after common-attempt insertion and before its
// invocation, in the same transaction. The deferred FK prevents an orphan slot
// from committing. SQL independently validates publication/pins and ready scope.
// Requirements: private-capture schema §3.2, PC07/08; no Docker or SQL child runs.
func reservePrivateCaptureTx(ctx context.Context, tx *sql.Tx, session LiquibaseSessionRecord, attempt AttemptRecord, at string) (int, *string, error) {
	rows, err := tx.QueryContext(ctx, `SELECT slot.slot_no,slot.job_id,slot.task_id,slot.attempt_no,
retirement.retirement_ref,retirement.reason,retirement.verified_at
FROM prepare_liquibase_capture_slots slot LEFT JOIN prepare_liquibase_capture_retirements retirement
ON retirement.session_id=slot.session_id AND retirement.job_id=slot.job_id AND retirement.task_id=slot.task_id AND retirement.attempt_no=slot.attempt_no
WHERE slot.session_id=? ORDER BY slot.slot_no`, session.SessionID)
	if err != nil {
		return 0, nil, err
	}
	type existingSlot struct {
		number, attempt       int
		job, task             string
		ref, reason, verified sql.NullString
	}
	var occupied []existingSlot
	for rows.Next() {
		var entry existingSlot
		if err := rows.Scan(&entry.number, &entry.job, &entry.task, &entry.attempt, &entry.ref, &entry.reason, &entry.verified); err != nil {
			_ = rows.Close()
			return 0, nil, err
		}
		occupied = append(occupied, entry)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return 0, nil, err
	}
	if err := rows.Close(); err != nil {
		return 0, nil, err
	}
	if len(occupied) > 1 {
		return 0, nil, ErrCommonAttempt
	}
	slot := 1
	var prior *string
	if len(occupied) == 1 {
		old := occupied[0]
		if old.number != 1 && old.number != 2 {
			return 0, nil, ErrCommonAttempt
		}
		slot = 3 - old.number
		if old.verified.Valid || old.ref.Valid && old.reason.String != "published_prune" {
			return 0, nil, ErrCommonAttempt
		}
		ref := old.ref.String
		if !old.ref.Valid {
			// A deterministic engine-generated identity allows exact readback;
			// it contains no path, SQL or user-selected authorization fields.
			binding, _ := json.Marshal([]any{"lb-capture-retirement-v1", session.SessionID, old.job, old.task, old.attempt})
			ref = "capture-retirement-" + strings.TrimPrefix(commonEvidenceDigest(string(binding)), "sha256:")
			if _, err := tx.ExecContext(ctx, `INSERT INTO prepare_liquibase_capture_retirements(retirement_ref,job_id,task_id,attempt_no,session_id,reason,requested_at)
VALUES(?,?,?,?,?,'published_prune',?)`, ref, old.job, old.task, old.attempt, session.SessionID, at); err != nil {
				return 0, nil, err
			}
		}
		prior = &ref
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO prepare_liquibase_capture_slots(session_id,slot_no,job_id,task_id,attempt_no,reserved_at,prior_retirement_ref)
VALUES(?,?,?,?,?,?,?)`, session.SessionID, slot, attempt.JobID, attempt.TaskID, attempt.AttemptNo, at, prior); err != nil {
		return 0, nil, err
	}
	return slot, prior, nil
}

// LiquibaseSessionReadiness stores only a verified marker identity and size.
// Protocol/helper/config/generation come from the immutable session, not caller
// JSON. It grants no SQL authority independently of invocation admission.
type LiquibaseSessionReadiness struct {
	SessionID, RunnerID, ReadyDigest, VerifiedAt string
	ReadyBytes                                   int64
}

// LiquibaseSessionReadinessVerifier is implemented by the trusted coordinator's
// exact runtime adapter. It must inspect the saved running full ID before/after
// reading the bounded private marker and compare every initialization binding.
// Digest strings alone are not a substitute. Requirements: private-capture §3.1.
type LiquibaseSessionReadinessVerifier interface {
	VerifyLiquibaseSessionReady(context.Context, LiquibaseSessionRecord) (string, int64, error)
}

// RecordLiquibaseSessionReadiness observes Docker outside SQL, then conditionally
// records the immutable tuple under the caller's job/session exclusion. A later
// cancellation/disposal or changed runner refuses admission. Lost commit replies
// use exact readback, never initialization or SQL replay. PC05/PC17.
func (s *SQLiteStore) RecordLiquibaseSessionReadiness(ctx context.Context, session LiquibaseSessionRecord, verifier LiquibaseSessionReadinessVerifier, at string) (LiquibaseSessionReadiness, error) {
	if err := ctx.Err(); err != nil {
		return LiquibaseSessionReadiness{}, err
	}
	if s == nil || s.db == nil || verifier == nil || session.SessionID == "" || !dockerContainerID.MatchString(session.RunnerID) || !validFinalizationUTC(at) {
		return LiquibaseSessionReadiness{}, ErrCommonAttempt
	}
	digest, size, err := verifier.VerifyLiquibaseSessionReady(ctx, session)
	if err := ctx.Err(); err != nil {
		return LiquibaseSessionReadiness{}, err
	}
	if err != nil || !bindingDigest.MatchString(digest) || size < 1 || size > 8192 {
		return LiquibaseSessionReadiness{}, ErrCommonAttempt
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return LiquibaseSessionReadiness{}, err
	}
	defer tx.Rollback()
	var exact int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM prepare_liquibase_sessions WHERE `+liquibaseSessionIdentityPredicate+`
AND runner_id=? AND runner_phase='running' AND retired_at IS NULL AND capture_location='container_private' AND capture_protocol_revision='lb-step-capture-v2'
AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_session_disposals d WHERE d.session_id=prepare_liquibase_sessions.session_id)
AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_job_cancellations c WHERE c.job_id=prepare_liquibase_sessions.job_id)
AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_job_finalizations f WHERE f.job_id=prepare_liquibase_sessions.job_id)`, append(sessionIdentityArgs(session), session.RunnerID)...).Scan(&exact); err != nil || exact != 1 {
		return LiquibaseSessionReadiness{}, ErrCommonAttempt
	}
	want := LiquibaseSessionReadiness{SessionID: session.SessionID, RunnerID: session.RunnerID, ReadyDigest: digest, ReadyBytes: size, VerifiedAt: at}
	if saved, found, err := readPrivateReadiness(ctx, tx, session.SessionID); err != nil {
		return LiquibaseSessionReadiness{}, err
	} else if found {
		if saved.RunnerID != want.RunnerID || saved.ReadyDigest != want.ReadyDigest || saved.ReadyBytes != want.ReadyBytes {
			return LiquibaseSessionReadiness{}, ErrCommonAttempt
		}
		return saved, nil
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO prepare_liquibase_session_readiness(session_id,runner_id,ready_digest,ready_bytes,verified_at) VALUES(?,?,?,?,?)`, want.SessionID, want.RunnerID, want.ReadyDigest, want.ReadyBytes, want.VerifiedAt); err != nil {
		return LiquibaseSessionReadiness{}, err
	}
	if err := tx.Commit(); err != nil {
		readCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		saved, found, readErr := readPrivateReadiness(readCtx, s.db, session.SessionID)
		if readErr == nil && found && saved == want {
			return saved, nil
		}
		return LiquibaseSessionReadiness{}, ErrCommonAttempt
	}
	return want, nil
}

type privateReadinessReader interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
}

func readPrivateReadiness(ctx context.Context, reader privateReadinessReader, sessionID string) (LiquibaseSessionReadiness, bool, error) {
	var row LiquibaseSessionReadiness
	err := reader.QueryRowContext(ctx, `SELECT r.session_id,r.runner_id,r.ready_digest,r.ready_bytes,r.verified_at FROM prepare_liquibase_session_readiness r
JOIN prepare_liquibase_sessions s ON s.session_id=r.session_id AND s.runner_id=r.runner_id
WHERE r.session_id=? AND s.capture_location='container_private' AND s.capture_protocol_revision='lb-step-capture-v2'`, sessionID).Scan(&row.SessionID, &row.RunnerID, &row.ReadyDigest, &row.ReadyBytes, &row.VerifiedAt)
	if err == sql.ErrNoRows {
		return LiquibaseSessionReadiness{}, false, nil
	}
	if err != nil {
		return LiquibaseSessionReadiness{}, false, err
	}
	if !dockerContainerID.MatchString(row.RunnerID) || !bindingDigest.MatchString(row.ReadyDigest) || row.ReadyBytes < 1 || row.ReadyBytes > 8192 || !validFinalizationUTC(row.VerifiedAt) {
		return LiquibaseSessionReadiness{}, false, ErrCommonAttempt
	}
	return row, true, nil
}

var sessionOpaqueRef = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$`)

func validInvocationPlan(plan LiquibaseInvocationPlan) bool {
	return sessionOpaqueRef.MatchString(plan.InvocationRef) &&
		plan.StdoutRef == plan.InvocationRef+"-stdout" && plan.StderrRef == plan.InvocationRef+"-stderr" &&
		plan.ManifestRef == plan.InvocationRef+"-manifest" && plan.ProtocolRevision == "lb-step-capture-v2" && validFinalizationUTC(plan.CreatedAt)
}

// PlanLiquibaseSessionInvocation commits the common attempt and capture
// references together, only at the exact current cursor in a running session.
// Replay attempts are admitted by their separate atomic recovery decision.
func (s *SQLiteStore) PlanLiquibaseSessionInvocation(ctx context.Context, session LiquibaseSessionRecord,
	taskID, executionDigest string, plan LiquibaseInvocationPlan) (LiquibaseInvocationRecord, error) {
	if s == nil || s.db == nil || taskID == "" || executionDigest == "" || !validInvocationPlan(plan) ||
		!dockerContainerID.MatchString(session.RunnerID) {
		return LiquibaseInvocationRecord{}, ErrCommonAttempt
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return LiquibaseInvocationRecord{}, err
	}
	defer tx.Rollback()
	var eligible int
	query := `SELECT count(*) FROM prepare_liquibase_sessions WHERE ` + liquibaseSessionIdentityPredicate + `
AND runner_id=? AND runner_phase='running' AND db_phase='running' AND retired_at IS NULL
AND EXISTS (SELECT 1 FROM prepare_tasks t JOIN prepare_jobs j ON j.job_id=t.job_id
  JOIN prepare_step_bindings sb ON sb.job_id=t.job_id AND sb.task_id=t.task_id
  JOIN prepare_execution_bindings b ON b.execution_digest=sb.execution_digest
  WHERE t.job_id=prepare_liquibase_sessions.job_id AND t.task_id=?
    AND j.status='running' AND j.prepare_kind='lb' AND j.purpose='prepare'
    AND t.status='running' AND t.type='state_execute'
    AND t.position=prepare_liquibase_sessions.cursor_position
    AND ((t.input_kind='image' AND t.input_id=prepare_liquibase_sessions.cursor_image_id)
      OR (t.input_kind='state' AND t.input_id=prepare_liquibase_sessions.cursor_state_id))
    AND b.execution_digest=? AND b.prepare_kind='lb'
    AND b.adapter_binding_digest=prepare_liquibase_sessions.execution_fingerprint)
AND EXISTS (SELECT 1 FROM prepare_clones c WHERE c.clone_id=prepare_liquibase_sessions.clone_id
  AND c.job_id=prepare_liquibase_sessions.job_id AND c.phase='active' AND c.db_runtime_id IS NOT NULL)
AND capture_location='container_private' AND capture_protocol_revision='lb-step-capture-v2'
AND EXISTS (SELECT 1 FROM prepare_liquibase_session_readiness ready WHERE ready.session_id=prepare_liquibase_sessions.session_id AND ready.runner_id=prepare_liquibase_sessions.runner_id)
AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_session_disposals disposal WHERE disposal.session_id=prepare_liquibase_sessions.session_id)
AND NOT EXISTS (` + cancelledSessionSuffixPredicate + `)
AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_cache_restores r
  WHERE r.session_id=prepare_liquibase_sessions.session_id AND r.restore_phase NOT IN ('published','abandoned'))`
	args := append(sessionIdentityArgs(session), session.RunnerID, taskID, executionDigest)
	if err := tx.QueryRowContext(ctx, query, args...).Scan(&eligible); err != nil || eligible != 1 {
		return LiquibaseInvocationRecord{}, ErrCommonAttempt
	}
	attempt := AttemptRecord{AttemptPlan: AttemptPlan{JobID: session.JobID, TaskID: taskID,
		CloneID: session.CloneID, ExecutionDigest: executionDigest, WorkerGeneration: session.WorkerGeneration,
		CreatedAt: plan.CreatedAt}, AttemptNo: 1, Phase: "planned"}
	if _, err := tx.ExecContext(ctx, `INSERT INTO prepare_step_attempts(job_id,task_id,attempt_no,
clone_id,execution_digest,worker_generation,executor_kind,planned_executor_name,phase,created_at)
VALUES(?,?,1,?,?,?,'session_exec',NULL,'planned',?)`, attempt.JobID, attempt.TaskID,
		attempt.CloneID, attempt.ExecutionDigest, attempt.WorkerGeneration, plan.CreatedAt); err != nil {
		return LiquibaseInvocationRecord{}, err
	}
	slot, prior, err := reservePrivateCaptureTx(ctx, tx, session, attempt, plan.CreatedAt)
	if err != nil {
		return LiquibaseInvocationRecord{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO prepare_liquibase_invocations(job_id,task_id,attempt_no,
session_id,invocation_ref,stdout_ref,stderr_ref,manifest_ref,helper_digest,protocol_revision)
VALUES(?,?,1,?,?,?,?,?,?,?)`, attempt.JobID, taskID, session.SessionID, plan.InvocationRef,
		plan.StdoutRef, plan.StderrRef, plan.ManifestRef, session.CaptureHelperDigest, plan.ProtocolRevision); err != nil {
		return LiquibaseInvocationRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return LiquibaseInvocationRecord{}, err
	}
	return LiquibaseInvocationRecord{LiquibaseInvocationPlan: plan, SessionID: session.SessionID,
		HelperDigest: session.CaptureHelperDigest, Attempt: attempt, CaptureSlot: slot, PriorRetirementRef: prior}, nil
}

// RequestLiquibaseSessionExec saves the last boundary before docker exec.
// Repeating the call is rejected: recovery must reconcile a requested child
// rather than treating an idempotent database write as a new execution permit.
func (s *SQLiteStore) RequestLiquibaseSessionExec(ctx context.Context, session LiquibaseSessionRecord,
	invocation LiquibaseInvocationRecord, at string) error {
	if s == nil || s.db == nil || at == "" {
		return ErrCommonAttempt
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if err := verifySessionInvocationTx(ctx, tx, session, invocation, "planned"); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE prepare_liquibase_invocations SET start_requested_at=?
WHERE job_id=? AND task_id=? AND attempt_no=? AND session_id=? AND start_requested_at IS NULL AND complete_at IS NULL`,
		at, invocation.Attempt.JobID, invocation.Attempt.TaskID, invocation.Attempt.AttemptNo, invocation.SessionID)
	if err != nil || oneRowChanged(result) != nil {
		return ErrCommonAttempt
	}
	result, err = tx.ExecContext(ctx, `UPDATE prepare_step_attempts SET phase='exec_requested',started_at=?
WHERE job_id=? AND task_id=? AND attempt_no=? AND executor_kind='session_exec' AND phase='planned' AND resolution IS NULL`,
		at, invocation.Attempt.JobID, invocation.Attempt.TaskID, invocation.Attempt.AttemptNo)
	if err != nil || oneRowChanged(result) != nil {
		return ErrCommonAttempt
	}
	return tx.Commit()
}

// CompleteLiquibaseSessionInvocation records a verified child manifest and
// exec_completion evidence in one transaction. A re-read of the exact same
// result returns the original proof; different exits or capture hashes fail.
func (s *SQLiteStore) CompleteLiquibaseSessionInvocation(ctx context.Context, session LiquibaseSessionRecord,
	invocation LiquibaseInvocationRecord, completion LiquibaseInvocationCompletion, at string) (int64, error) {
	if s == nil || s.db == nil || at == "" || completion.ExitCode < 0 || completion.ExitCode > 255 ||
		!bindingDigest.MatchString(completion.ManifestDigest) || completion.ManifestBytes < 1 {
		return 0, ErrCommonAttempt
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if err := verifySessionInvocationTx(ctx, tx, session, invocation, "exec_requested", "exec_completed"); err != nil {
		return 0, err
	}
	var savedCode sql.NullInt64
	var savedDigest sql.NullString
	var savedBytes sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT exit_code,completion_digest,completion_bytes FROM prepare_liquibase_invocations
WHERE job_id=? AND task_id=? AND attempt_no=?`, invocation.Attempt.JobID, invocation.Attempt.TaskID,
		invocation.Attempt.AttemptNo).Scan(&savedCode, &savedDigest, &savedBytes); err != nil {
		return 0, err
	}
	if savedCode.Valid {
		if int(savedCode.Int64) != completion.ExitCode || savedDigest.String != completion.ManifestDigest || savedBytes.Int64 != completion.ManifestBytes {
			return 0, ErrCommonAttempt
		}
		var evidenceID int64
		if err := tx.QueryRowContext(ctx, `SELECT evidence_id FROM prepare_attempt_evidence WHERE job_id=?
AND task_id=? AND attempt_no=? AND kind='exec_completion'
AND json_extract(facts_json,'$.observation.invocation_ref')=?
AND json_extract(facts_json,'$.observation.manifest_digest')=?`, invocation.Attempt.JobID,
			invocation.Attempt.TaskID, invocation.Attempt.AttemptNo, invocation.InvocationRef, completion.ManifestDigest).Scan(&evidenceID); err != nil {
			return 0, err
		}
		return evidenceID, tx.Commit()
	}
	result, err := tx.ExecContext(ctx, `UPDATE prepare_liquibase_invocations
SET exit_code=?,completion_digest=?,completion_bytes=?,complete_at=?
WHERE job_id=? AND task_id=? AND attempt_no=? AND start_requested_at IS NOT NULL AND complete_at IS NULL`,
		completion.ExitCode, completion.ManifestDigest, completion.ManifestBytes, at,
		invocation.Attempt.JobID, invocation.Attempt.TaskID, invocation.Attempt.AttemptNo)
	if err != nil || oneRowChanged(result) != nil {
		return 0, ErrCommonAttempt
	}
	result, err = tx.ExecContext(ctx, `UPDATE prepare_step_attempts SET phase='exec_completed',finished_at=?
WHERE job_id=? AND task_id=? AND attempt_no=? AND executor_kind='session_exec' AND phase='exec_requested' AND resolution IS NULL`,
		at, invocation.Attempt.JobID, invocation.Attempt.TaskID, invocation.Attempt.AttemptNo)
	if err != nil || oneRowChanged(result) != nil {
		return 0, ErrCommonAttempt
	}
	observation := map[string]any{"session_id": session.SessionID, "runner_id": session.RunnerID,
		"invocation_ref": invocation.InvocationRef, "manifest_ref": invocation.ManifestRef,
		"manifest_digest": completion.ManifestDigest, "manifest_bytes": completion.ManifestBytes,
		"exit_code": completion.ExitCode, "stdout_eof": true, "stderr_eof": true, "children_absent": true}
	id, err := appendSessionEvidenceTx(ctx, tx, invocation.Attempt, "exec_completion", observation, at)
	if err != nil {
		return 0, err
	}
	return id, tx.Commit()
}

func verifySessionInvocationTx(ctx context.Context, tx *sql.Tx, session LiquibaseSessionRecord,
	invocation LiquibaseInvocationRecord, phases ...string) error {
	return verifySessionInvocationDBTx(ctx, tx, session, invocation, "running", phases...)
}

func verifySessionInvocationDBTx(ctx context.Context, tx *sql.Tx, session LiquibaseSessionRecord,
	invocation LiquibaseInvocationRecord, dbPhase string, phases ...string) error {
	if len(phases) == 0 || invocation.SessionID != session.SessionID ||
		invocation.Attempt.JobID != session.JobID || invocation.Attempt.CloneID != session.CloneID ||
		invocation.Attempt.WorkerGeneration != session.WorkerGeneration || invocation.HelperDigest != session.CaptureHelperDigest {
		return ErrCommonAttempt
	}
	var exactSession int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM prepare_liquibase_sessions WHERE `+liquibaseSessionIdentityPredicate+`
AND runner_id=? AND runner_phase='running' AND db_phase=? AND retired_at IS NULL`,
		append(sessionIdentityArgs(session), session.RunnerID, dbPhase)...).Scan(&exactSession); err != nil || exactSession != 1 {
		return ErrCommonAttempt
	}
	var phase string
	if err := tx.QueryRowContext(ctx, `SELECT a.phase FROM prepare_liquibase_invocations i
JOIN prepare_step_attempts a ON a.job_id=i.job_id AND a.task_id=i.task_id AND a.attempt_no=i.attempt_no
WHERE i.job_id=? AND i.task_id=? AND i.attempt_no=? AND i.session_id=?
AND i.invocation_ref=? AND i.stdout_ref=? AND i.stderr_ref=? AND i.manifest_ref=?
AND i.helper_digest=? AND i.protocol_revision=? AND i.retired_at IS NULL
AND a.clone_id=? AND a.execution_digest=? AND a.worker_generation=?
AND a.executor_kind='session_exec' AND a.resolution IS NULL`, invocation.Attempt.JobID,
		invocation.Attempt.TaskID, invocation.Attempt.AttemptNo, session.SessionID,
		invocation.InvocationRef, invocation.StdoutRef, invocation.StderrRef, invocation.ManifestRef,
		invocation.HelperDigest, invocation.ProtocolRevision, invocation.Attempt.CloneID,
		invocation.Attempt.ExecutionDigest, invocation.Attempt.WorkerGeneration).Scan(&phase); err != nil {
		return ErrCommonAttempt
	}
	for _, allowed := range phases {
		if phase == allowed {
			return nil
		}
	}
	return ErrCommonAttempt
}

func appendSessionEvidenceTx(ctx context.Context, tx *sql.Tx, attempt AttemptRecord, kind string,
	observation map[string]any, at string) (int64, error) {
	facts, err := json.Marshal(map[string]any{"clone_id": attempt.CloneID,
		"execution_digest": attempt.ExecutionDigest, "observation": observation})
	if err != nil || len(facts) > 8192 {
		return 0, ErrCommonAttempt
	}
	scope := commonEvidenceDigest(fmt.Sprintf("attempt-scope-v1\n%s\n%s\n%d\n%s\n%s", attempt.JobID,
		attempt.TaskID, attempt.AttemptNo, attempt.CloneID, attempt.ExecutionDigest))
	result, err := tx.ExecContext(ctx, `INSERT INTO prepare_attempt_evidence(job_id,task_id,attempt_no,kind,
scope_digest,facts_json,facts_digest,worker_generation,observed_at) VALUES(?,?,?,?,?,?,?,?,?)`,
		attempt.JobID, attempt.TaskID, attempt.AttemptNo, kind, scope, string(facts),
		commonEvidenceDigest(string(facts)), attempt.WorkerGeneration, at)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}
