package queue

// Normal-path journal operations from the approved Liquibase prototype.
// Installation and runtime activation are separate boundaries; these methods
// do not enable replay. See issue89-normal-path-storage-extraction.md.

import (
	"context"
	"database/sql"
	"strings"
	"time"
)

// cancelledSessionSuffixPredicate is shared by admission guards. The fact is
// tied to its original invocation and exact physical session, not a mutable
// task flag that a new worker could clear.
const cancelledSessionSuffixPredicate = `SELECT 1 FROM prepare_attempt_evidence cancel
JOIN prepare_liquibase_invocations previous USING(job_id,task_id,attempt_no)
WHERE previous.session_id=prepare_liquibase_sessions.session_id
AND cancel.kind='cancellation_requested' AND cancel.worker_generation=prepare_liquibase_sessions.worker_generation
AND json_extract(cancel.facts_json,'$.observation.session_id')=prepare_liquibase_sessions.session_id
AND json_extract(cancel.facts_json,'$.observation.runner_id')=prepare_liquibase_sessions.runner_id
AND json_extract(cancel.facts_json,'$.observation.session_scope')=1`

// LiquibaseSessionLaunch is the fully bound intent for a working clone and
// reusable runner. The first uncached task determines its initial cursor;
// planning has its own independently released preview environment.
type LiquibaseSessionLaunch struct {
	Session                      LiquibaseSessionRecord
	Clone                        ClonePlan
	FirstTaskID, ExecutionDigest string
	HelperLeaseID                string
	// RequireFirstSessionProof is trusted in-memory recovery context, not a
	// client option. Repeat §22's consistent no-history proof before admission.
	RequireFirstSessionProof bool
}

// PlanLiquibaseSession atomically pins the selected input, helper, clone and
// runner identities before any filesystem or Docker mutation. It is the
// ordinary first-session path; recovery replacements additionally need their
// saved replay decision. See the reusable-container storage schema, step 1.
func (s *SQLiteStore) PlanLiquibaseSession(ctx context.Context, launch LiquibaseSessionLaunch) (LiquibaseSessionRecord, error) {
	session, clone := launch.Session, launch.Clone
	if s == nil || s.db == nil || !sessionOpaqueRef.MatchString(session.SessionID) || session.JobID == "" ||
		session.CloneID != clone.ID || session.SessionNo < 1 || session.WorkerGeneration < 1 ||
		session.StartPosition < 0 || session.CreatedAt == "" || session.RunnerID != "" ||
		!sessionOpaqueRef.MatchString(session.ProtectedConfigRef) || session.PlannedRunnerName == "" ||
		!bindingDigest.MatchString(session.RunnerConfigDigest) || !bindingDigest.MatchString(session.CaptureHelperDigest) ||
		launch.FirstTaskID == "" || launch.ExecutionDigest == "" || launch.HelperLeaseID == "" ||
		clone.ID == "" || clone.Generation < 1 || clone.Ref == "" || clone.PlannedRuntimeName == "" ||
		clone.SourceKind != session.StartKind ||
		(session.StartKind == "image" && (clone.SourceImageID != session.StartID || clone.SourceStateID != "")) ||
		(session.StartKind == "state" && (clone.SourceStateID != session.StartID || clone.SourceImageID != "")) ||
		(session.StartKind != "image" && session.StartKind != "state") {
		return LiquibaseSessionRecord{}, ErrCommonAttempt
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return LiquibaseSessionRecord{}, err
	}
	defer tx.Rollback()
	if launch.RequireFirstSessionProof {
		task, found, err := readLiquibaseFirstSessionTask(ctx, tx, session.JobID)
		if err != nil {
			return LiquibaseSessionRecord{}, err
		}
		if !found || task.TaskID != launch.FirstTaskID || task.Position != session.StartPosition || task.InputKind == nil || *task.InputKind != session.StartKind || task.InputID == nil || *task.InputID != session.StartID {
			return LiquibaseSessionRecord{}, ErrCommonAttempt
		}
	}
	var eligible int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM prepare_jobs j
JOIN prepare_tasks t ON t.job_id=j.job_id
JOIN prepare_step_bindings sb ON sb.job_id=t.job_id AND sb.task_id=t.task_id
JOIN prepare_execution_bindings b ON b.execution_digest=sb.execution_digest
JOIN liquibase_job_bindings lb ON lb.job_id=j.job_id
WHERE j.job_id=? AND j.status='running' AND j.prepare_kind='lb' AND j.purpose='prepare'
AND t.task_id=? AND t.type='state_execute' AND t.status='running' AND t.position=?
AND t.input_kind=? AND t.input_id=? AND b.execution_digest=?
AND b.adapter_binding_digest=? AND lb.execution_fingerprint=b.adapter_binding_digest
AND NOT EXISTS (SELECT 1 FROM prepare_tasks earlier WHERE earlier.job_id=j.job_id
  AND earlier.type='state_execute' AND earlier.position<t.position AND earlier.status<>'succeeded')
AND NOT EXISTS (SELECT 1 FROM prepare_step_attempts a WHERE a.job_id=j.job_id AND a.resolution IS NULL)
AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_sessions live WHERE live.job_id=j.job_id AND live.retired_at IS NULL)`,
		session.JobID, launch.FirstTaskID, session.StartPosition, session.StartKind, session.StartID,
		launch.ExecutionDigest, session.ExecutionFingerprint).Scan(&eligible); err != nil || eligible != 1 {
		return LiquibaseSessionRecord{}, ErrCommonAttempt
	}
	var nextSession, nextClone int
	if err := tx.QueryRowContext(ctx, `SELECT coalesce(max(session_no),0)+1 FROM prepare_liquibase_sessions WHERE job_id=?`, session.JobID).Scan(&nextSession); err != nil {
		return LiquibaseSessionRecord{}, err
	}
	if err := tx.QueryRowContext(ctx, `SELECT coalesce(max(clone_generation),0)+1 FROM prepare_clones WHERE job_id=?`, session.JobID).Scan(&nextClone); err != nil || session.SessionNo != nextSession || clone.Generation != nextClone {
		return LiquibaseSessionRecord{}, ErrCommonAttempt
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO prepare_clones(clone_id,job_id,clone_generation,source_kind,
source_image_id,source_state_id,clone_ref,planned_db_runtime_name,phase,created_at) VALUES(?,?,?,?,?,?,?,?,'allocated',?)`,
		clone.ID, session.JobID, clone.Generation, clone.SourceKind, nullIfEmpty(clone.SourceImageID),
		nullIfEmpty(clone.SourceStateID), clone.Ref, clone.PlannedRuntimeName, session.CreatedAt); err != nil {
		return LiquibaseSessionRecord{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO prepare_artifact_leases(lease_id,artifact_kind,artifact_digest,owner_kind,owner_id,created_at)
VALUES(?,'capture_helper',?,'job',?,?) ON CONFLICT(artifact_kind,artifact_digest,owner_kind,owner_id) DO NOTHING`,
		launch.HelperLeaseID, session.CaptureHelperDigest, session.JobID, session.CreatedAt); err != nil {
		return LiquibaseSessionRecord{}, err
	}
	var cursorImage, cursorState any
	if session.StartKind == "image" {
		cursorImage = session.StartID
	} else {
		cursorState = session.StartID
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO prepare_liquibase_sessions(session_id,job_id,session_no,clone_id,
worker_generation,execution_fingerprint,start_kind,start_id,start_position,planned_runner_name,
runner_config_digest,capture_helper_digest,protected_config_ref,runner_phase,db_phase,
cursor_image_id,cursor_state_id,cursor_position,created_at,capture_location,capture_protocol_revision) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,'planned','running',?,?,?,?,'container_private','lb-step-capture-v2')`,
		session.SessionID, session.JobID, session.SessionNo, session.CloneID, session.WorkerGeneration,
		session.ExecutionFingerprint, session.StartKind, session.StartID, session.StartPosition, session.PlannedRunnerName,
		session.RunnerConfigDigest, session.CaptureHelperDigest, session.ProtectedConfigRef,
		cursorImage, cursorState, session.StartPosition, session.CreatedAt); err != nil {
		return LiquibaseSessionRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return LiquibaseSessionRecord{}, err
	}
	session.CursorKind, session.CursorID, session.CursorPosition = session.StartKind, session.StartID, session.StartPosition
	session.RunnerPhase, session.DBPhase = "planned", "running"
	return session, nil
}

// ActivateLiquibaseSessionClone attaches the exact DB runtime after the
// journaled clone is materialized and its managed identity is verified.
func (s *SQLiteStore) ActivateLiquibaseSessionClone(ctx context.Context, session LiquibaseSessionRecord, runtimeName, runtimeID string) error {
	if s == nil || s.db == nil || runtimeName == "" || !dockerContainerID.MatchString(runtimeID) {
		return ErrCommonAttempt
	}
	result, err := s.db.ExecContext(ctx, `UPDATE prepare_clones SET db_runtime_id=?,phase='active'
WHERE clone_id=? AND job_id=? AND planned_db_runtime_name=? AND phase IN ('allocated','active')
AND (db_runtime_id IS NULL OR db_runtime_id=?)
AND EXISTS (SELECT 1 FROM prepare_liquibase_sessions WHERE `+liquibaseSessionIdentityPredicate+`
  AND runner_phase='planned' AND retired_at IS NULL)`, append([]any{runtimeID, session.CloneID, session.JobID,
		runtimeName, runtimeID}, sessionIdentityArgs(session)...)...)
	if err != nil {
		return err
	}
	return oneRowChanged(result)
}

// LiquibaseSessionAbsenceVerifier reports completed physical retirement and
// verified absence of the exact journal-named runner, DB, bytes and private
// files. Native verification runs outside SQL and shared eviction exclusion;
// the coordinated cache-transfer path can pass its exact transient observation
// while continuously holding job/state ownership. Recovery must inspect anew,
// not reconstruct that in-memory proof. Failure leaves every pin intact.
type LiquibaseSessionAbsenceVerifier interface {
	VerifyReleasedLiquibaseSession(context.Context, LiquibaseSessionRecord, ClonePlan, string) error
}

// RetirePublishedLiquibaseSession releases a normally completed session only
// after physical absence. The final conditional transaction rechecks the
// exact identities and retires all captures, clone and cursor together. See
// C01a/S04 and storage-schema ordered transition 7.
func (s *SQLiteStore) RetirePublishedLiquibaseSession(ctx context.Context, session LiquibaseSessionRecord,
	verifier LiquibaseSessionAbsenceVerifier, at string) error {
	if s == nil || s.db == nil || verifier == nil || at == "" {
		return ErrCommonAttempt
	}
	var clone ClonePlan
	var sourceImage, sourceState sql.NullString
	var runtimeID string
	query := `SELECT c.clone_id,c.clone_generation,c.source_kind,c.source_image_id,c.source_state_id,
c.clone_ref,c.planned_db_runtime_name,c.db_runtime_id FROM prepare_clones c
WHERE c.clone_id=? AND c.job_id=? AND c.phase='quiescent'
AND EXISTS (SELECT 1 FROM prepare_liquibase_sessions WHERE ` + liquibaseSessionIdentityPredicate + `
  AND runner_id=? AND runner_phase='stopped' AND retired_at IS NULL)
AND NOT EXISTS (SELECT 1 FROM prepare_step_attempts a WHERE a.clone_id=c.clone_id
  AND (a.phase<>'closed' OR a.resolution IS NULL OR a.resolution<>'published'))
AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_cache_restores r WHERE r.session_id=? AND r.restore_phase NOT IN ('published','abandoned'))`
	args := append([]any{session.CloneID, session.JobID}, sessionIdentityArgs(session)...)
	args = append(args, session.RunnerID, session.SessionID)
	if err := s.db.QueryRowContext(ctx, query, args...).Scan(&clone.ID, &clone.Generation, &clone.SourceKind,
		&sourceImage, &sourceState, &clone.Ref, &clone.PlannedRuntimeName, &runtimeID); err != nil || !dockerContainerID.MatchString(runtimeID) {
		return ErrCommonAttempt
	}
	clone.SourceImageID, clone.SourceStateID = sourceImage.String, sourceState.String
	saved, found, err := s.ReadLiquibaseSession(ctx, session.SessionID)
	if err != nil || !found || saved.RunnerID != session.RunnerID || saved.RunnerPhase != "stopped" {
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
	var exact int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM prepare_liquibase_sessions WHERE `+liquibaseSessionIdentityPredicate+`
AND runner_id=? AND runner_phase='stopped' AND retired_at IS NULL
AND NOT EXISTS (SELECT 1 FROM prepare_step_attempts a WHERE a.clone_id=prepare_liquibase_sessions.clone_id
  AND (a.phase<>'closed' OR a.resolution IS NULL OR a.resolution<>'published'))
AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_cache_restores r WHERE r.session_id=prepare_liquibase_sessions.session_id AND r.restore_phase NOT IN ('published','abandoned'))`,
		append(sessionIdentityArgs(session), session.RunnerID)...).Scan(&exact); err != nil || exact != 1 {
		return ErrCommonAttempt
	}
	if _, err := tx.ExecContext(ctx, `UPDATE prepare_liquibase_invocations SET retired_at=?
WHERE session_id=? AND retired_at IS NULL`, at, session.SessionID); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE prepare_clones SET phase='released'
WHERE clone_id=? AND job_id=? AND planned_db_runtime_name=? AND db_runtime_id=? AND phase='quiescent'`,
		session.CloneID, session.JobID, clone.PlannedRuntimeName, runtimeID)
	if err != nil || oneRowChanged(result) != nil {
		return ErrCommonAttempt
	}
	result, err = tx.ExecContext(ctx, `UPDATE prepare_liquibase_sessions SET runner_phase='retired',retired_at=?,
cursor_image_id=NULL,cursor_state_id=NULL WHERE `+liquibaseSessionIdentityPredicate+`
AND runner_id=? AND runner_phase='stopped' AND retired_at IS NULL`,
		append(append([]any{at}, sessionIdentityArgs(session)...), session.RunnerID)...)
	if err != nil || oneRowChanged(result) != nil {
		return ErrCommonAttempt
	}
	return tx.Commit()
}

// ReadLiquibaseSession reads the current durable phases and cursor without
// inspecting Docker or adopting any physical resource by name.
func (s *SQLiteStore) ReadLiquibaseSession(ctx context.Context, sessionID string) (LiquibaseSessionRecord, bool, error) {
	if s == nil || s.db == nil || sessionID == "" {
		return LiquibaseSessionRecord{}, false, ErrCommonAttempt
	}
	var record LiquibaseSessionRecord
	err := s.db.QueryRowContext(ctx, `SELECT session_id,job_id,clone_id,session_no,worker_generation,
execution_fingerprint,start_kind,start_id,start_position,planned_runner_name,
runner_config_digest,capture_helper_digest,protected_config_ref,coalesce(runner_id,''),
runner_phase,db_phase,CASE WHEN cursor_state_id IS NOT NULL THEN 'state'
  WHEN cursor_image_id IS NOT NULL THEN 'image' ELSE '' END,
coalesce(cursor_state_id,cursor_image_id,''),cursor_position,created_at,retired_at
FROM prepare_liquibase_sessions WHERE session_id=?`, sessionID).Scan(
		&record.SessionID, &record.JobID, &record.CloneID, &record.SessionNo, &record.WorkerGeneration,
		&record.ExecutionFingerprint, &record.StartKind, &record.StartID, &record.StartPosition,
		&record.PlannedRunnerName, &record.RunnerConfigDigest, &record.CaptureHelperDigest,
		&record.ProtectedConfigRef, &record.RunnerID, &record.RunnerPhase, &record.DBPhase,
		&record.CursorKind, &record.CursorID, &record.CursorPosition, &record.CreatedAt, &record.RetiredAt)
	if err == sql.ErrNoRows {
		return LiquibaseSessionRecord{}, false, nil
	}
	return record, err == nil, err
}

// RequestLiquibaseSessionRunnerCreate records intent only after the exact
// database runtime is attached. It does not authorize reissuing an ambiguous
// Docker create: the coordinator must inspect the saved planned name first.
func (s *SQLiteStore) RequestLiquibaseSessionRunnerCreate(ctx context.Context, session LiquibaseSessionRecord) error {
	return s.advanceLiquibaseSessionRunner(ctx, session, []string{"planned", "create_requested"}, "create_requested", true)
}

// AttachLiquibaseSessionRunner saves a full Docker ID before start. Recovery
// can repeat attachment of that same ID, but cannot replace it with another.
func (s *SQLiteStore) AttachLiquibaseSessionRunner(ctx context.Context, session LiquibaseSessionRecord, id string) error {
	if s == nil || s.db == nil || !dockerContainerID.MatchString(id) ||
		(session.RunnerID != "" && session.RunnerID != id) {
		return ErrCommonAttempt
	}
	result, err := s.db.ExecContext(ctx, `UPDATE prepare_liquibase_sessions
SET runner_id=?,runner_phase='created'
WHERE `+liquibaseSessionIdentityPredicate+`
AND runner_phase IN ('create_requested','created') AND retired_at IS NULL
AND (runner_id IS NULL OR runner_id=?)`, append([]any{id}, append(sessionIdentityArgs(session), id)...)...)
	if err != nil {
		return err
	}
	return oneRowChanged(result)
}

// RequestLiquibaseSessionRunnerStart commits the exact-ID start boundary.
func (s *SQLiteStore) RequestLiquibaseSessionRunnerStart(ctx context.Context, session LiquibaseSessionRecord) error {
	return s.advanceLiquibaseSessionRunner(ctx, session, []string{"created", "start_requested"}, "start_requested", true)
}

// MarkLiquibaseSessionRunnerRunning records a successful start observation.
func (s *SQLiteStore) MarkLiquibaseSessionRunnerRunning(ctx context.Context, session LiquibaseSessionRecord) error {
	return s.advanceLiquibaseSessionRunner(ctx, session, []string{"start_requested", "running"}, "running", true)
}

// RequestLiquibaseSessionRunnerStop persists intent before signalling the
// exact attached runner; it never treats a missing response as acknowledgement.
func (s *SQLiteStore) RequestLiquibaseSessionRunnerStop(ctx context.Context, session LiquibaseSessionRecord) error {
	return s.advanceLiquibaseSessionRunner(ctx, session, []string{"created", "start_requested", "running", "stop_requested"}, "stop_requested", false)
}

// AcknowledgeLiquibaseSessionRunnerStop records only a received exact-ID
// Docker signal acknowledgement, separately from the following inspection.
func (s *SQLiteStore) AcknowledgeLiquibaseSessionRunnerStop(ctx context.Context, session LiquibaseSessionRecord) error {
	return s.advanceLiquibaseSessionRunner(ctx, session, []string{"stop_requested", "stop_acknowledged"}, "stop_acknowledged", false)
}

// MarkLiquibaseSessionRunnerStopped records inspection after acknowledged
// stop. The caller must inspect the exact saved name and full ID first.
func (s *SQLiteStore) MarkLiquibaseSessionRunnerStopped(ctx context.Context, session LiquibaseSessionRecord) error {
	return s.advanceLiquibaseSessionRunner(ctx, session, []string{"stop_acknowledged", "stopped"}, "stopped", false)
}

const liquibaseSessionIdentityPredicate = `session_id=? AND job_id=? AND clone_id=?
AND worker_generation=? AND execution_fingerprint=? AND planned_runner_name=?
AND runner_config_digest=? AND capture_helper_digest=? AND protected_config_ref=?`

func sessionIdentityArgs(session LiquibaseSessionRecord) []any {
	return []any{session.SessionID, session.JobID, session.CloneID, session.WorkerGeneration,
		session.ExecutionFingerprint, session.PlannedRunnerName, session.RunnerConfigDigest,
		session.CaptureHelperDigest, session.ProtectedConfigRef}
}

func (s *SQLiteStore) advanceLiquibaseSessionRunner(ctx context.Context, session LiquibaseSessionRecord,
	from []string, to string, requireDB bool) error {
	if s == nil || s.db == nil || session.SessionID == "" || session.JobID == "" ||
		session.CloneID == "" || session.WorkerGeneration < 1 || len(from) == 0 ||
		(to != "create_requested" && !dockerContainerID.MatchString(session.RunnerID)) {
		return ErrCommonAttempt
	}
	placeholders := strings.TrimSuffix(strings.Repeat("?,", len(from)), ",")
	query := `UPDATE prepare_liquibase_sessions SET runner_phase=?
WHERE ` + liquibaseSessionIdentityPredicate + ` AND retired_at IS NULL
AND coalesce(runner_id,'')=? AND runner_phase IN (` + placeholders + `)
AND EXISTS (SELECT 1 FROM prepare_jobs j WHERE j.job_id=prepare_liquibase_sessions.job_id
  AND j.status IN ('running','recovery_required'))`
	args := append([]any{to}, sessionIdentityArgs(session)...)
	args = append(args, session.RunnerID)
	for _, phase := range from {
		args = append(args, phase)
	}
	if requireDB {
		query += ` AND db_phase='running' AND EXISTS (SELECT 1 FROM prepare_clones c
  WHERE c.clone_id=prepare_liquibase_sessions.clone_id
    AND c.job_id=prepare_liquibase_sessions.job_id AND c.phase='active'
    AND c.db_runtime_id IS NOT NULL)`
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var previous string
	if err := tx.QueryRowContext(ctx, `SELECT runner_phase FROM prepare_liquibase_sessions WHERE `+liquibaseSessionIdentityPredicate, sessionIdentityArgs(session)...).Scan(&previous); err != nil {
		return ErrCommonAttempt
	}
	result, err := tx.ExecContext(ctx, query, args...)
	if err != nil {
		return err
	}
	if err := oneRowChanged(result); err != nil {
		return err
	}
	if previous != to {
		kind := map[string]string{"stop_requested": "termination_requested", "stop_acknowledged": "termination_acknowledged", "stopped": "executor_inspection"}[to]
		if kind != "" {
			var attempt AttemptRecord
			attempt.JobID, attempt.CloneID, attempt.WorkerGeneration = session.JobID, session.CloneID, session.WorkerGeneration
			err := tx.QueryRowContext(ctx, `SELECT a.task_id,a.attempt_no,a.execution_digest FROM prepare_step_attempts a
JOIN prepare_liquibase_invocations i USING(job_id,task_id,attempt_no)
WHERE i.session_id=? AND a.clone_id=? AND a.worker_generation=? AND a.resolution IS NULL`,
				session.SessionID, session.CloneID, session.WorkerGeneration).Scan(&attempt.TaskID, &attempt.AttemptNo, &attempt.ExecutionDigest)
			if err != nil && err != sql.ErrNoRows {
				return err
			}
			if err == nil {
				if _, err := appendSessionEvidenceTx(ctx, tx, attempt, kind, map[string]any{
					"session_id": session.SessionID, "runner_id": session.RunnerID, "executor_id": session.RunnerID, "runner_stopped": to == "stopped"}, time.Now().UTC().Format(time.RFC3339Nano)); err != nil {
					return err
				}
			}
		}
	}
	return tx.Commit()
}
