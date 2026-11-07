package queue

// Normal-path journal operations from the approved Liquibase prototype.
// Installation and runtime activation are separate boundaries; these methods
// do not enable replay. See issue89-normal-path-storage-extraction.md.

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/sqlrs/engine-local/internal/statefs"
	"github.com/sqlrs/engine-local/internal/store"
)

// RecordLiquibaseSessionDBStopped records the distinct stopped-but-present
// PostgreSQL proof after the exact completed child. The caller has verified
// pg_ctl stop and that the same database container remains alive. This form
// cannot authorize discarding a clone or replaying on another writable copy.
func (s *SQLiteStore) RecordLiquibaseSessionDBStopped(ctx context.Context, session LiquibaseSessionRecord,
	invocation LiquibaseInvocationRecord, runtimeName, runtimeID, at string) (int64, error) {
	if s == nil || s.db == nil || runtimeName == "" || !dockerContainerID.MatchString(runtimeID) || at == "" {
		return 0, ErrCommonAttempt
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	if err := verifySessionInvocationTx(ctx, tx, session, invocation, "exec_completed"); err != nil {
		return 0, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE prepare_clones SET phase='quiescent'
WHERE clone_id=? AND job_id=? AND planned_db_runtime_name=? AND db_runtime_id=? AND phase='active'`,
		session.CloneID, session.JobID, runtimeName, runtimeID)
	if err != nil || oneRowChanged(result) != nil {
		return 0, ErrCommonAttempt
	}
	id, err := appendSessionEvidenceTx(ctx, tx, invocation.Attempt, "db_quiescence", map[string]any{
		"session_id": session.SessionID, "runtime_name": runtimeName, "runtime_id": runtimeID,
		"postgres_stopped": 1, "runtime_present": 1}, at)
	if err != nil {
		return 0, err
	}
	result, err = tx.ExecContext(ctx, `UPDATE prepare_liquibase_sessions SET db_phase='stopped'
WHERE `+liquibaseSessionIdentityPredicate+` AND db_phase='running' AND runner_id=? AND runner_phase='running' AND retired_at IS NULL`,
		append(sessionIdentityArgs(session), session.RunnerID)...)
	if err != nil || oneRowChanged(result) != nil {
		return 0, ErrCommonAttempt
	}
	return id, tx.Commit()
}

// PublishLiquibaseSessionState commits one sealed checkpoint and moves the
// live cursor without releasing its working clone. Completion, stopped-DB and
// publication proofs remain separate ordered observations. See the reusable
// container storage schema, ordered transitions 4 and 5.
func (s *SQLiteStore) PublishLiquibaseSessionState(ctx context.Context, session LiquibaseSessionRecord,
	invocation LiquibaseInvocationRecord, entry store.StateCreate, stateDir, fsKind string,
	completionID, quiescenceID int64, at string) error {
	if s == nil || s.db == nil || entry.StateID == "" || entry.StateFingerprint != entry.StateID ||
		entry.PrepareKind != "lb" || entry.ImageID == "" || entry.CreatedAt == "" || at == "" ||
		entry.LiquibaseExecutionFingerprint != session.ExecutionFingerprint || completionID < 1 || quiescenceID <= completionID {
		return ErrCommonAttempt
	}
	if err := statefs.VerifyPublishedMarker(stateDir, fsKind); err != nil {
		return ErrCommonAttempt
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	// An interrupted response to commit is resolved against the already
	// published state and the original ordered decision, never by rerunning SQL.
	var published int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM prepare_step_attempts a
JOIN prepare_tasks t ON t.job_id=a.job_id AND t.task_id=a.task_id
JOIN states state ON state.state_id=t.output_state_id
JOIN prepare_state_bindings b ON b.state_id=state.state_id
JOIN liquibase_state_bindings lb ON lb.state_id=state.state_id
JOIN prepare_liquibase_invocations i ON i.job_id=a.job_id AND i.task_id=a.task_id AND i.attempt_no=a.attempt_no
JOIN prepare_attempt_decisions d ON d.job_id=a.job_id AND d.task_id=a.task_id AND d.attempt_no=a.attempt_no
JOIN prepare_decision_evidence complete_link ON complete_link.decision_id=d.decision_id AND complete_link.evidence_id=?
JOIN prepare_decision_evidence q_link ON q_link.decision_id=d.decision_id AND q_link.evidence_id=?
WHERE a.job_id=? AND a.task_id=? AND a.attempt_no=? AND a.clone_id=? AND a.execution_digest=?
AND a.worker_generation=? AND a.executor_kind='session_exec' AND a.resolution='published'
AND a.phase='closed' AND t.status='succeeded' AND b.execution_digest=a.execution_digest
AND i.session_id=? AND i.invocation_ref=? AND d.choice='publish_original'
AND state.state_id=? AND state.parent_state_id IS ? AND state.state_fingerprint=?
AND state.image_id=? AND state.prepare_kind='lb' AND state.prepare_args_normalized=?
AND state.lineage_ref IS ? AND state.identity_digest IS ? AND lb.execution_fingerprint=?`,
		completionID, quiescenceID, invocation.Attempt.JobID, invocation.Attempt.TaskID,
		invocation.Attempt.AttemptNo, session.CloneID, invocation.Attempt.ExecutionDigest,
		session.WorkerGeneration, session.SessionID, invocation.InvocationRef, entry.StateID,
		entry.ParentStateID, entry.StateFingerprint, entry.ImageID, entry.PrepareArgsNormalized,
		entry.LineageRef, entry.IdentityDigest, entry.LiquibaseExecutionFingerprint).Scan(&published); err != nil {
		return err
	}
	if published == 1 {
		return tx.Commit()
	}
	if err := verifySessionInvocationDBTx(ctx, tx, session, invocation, "stopped", "exec_completed"); err != nil {
		return err
	}
	var eligible int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM prepare_liquibase_sessions s
JOIN prepare_clones c ON c.clone_id=s.clone_id AND c.job_id=s.job_id
JOIN prepare_tasks t ON t.job_id=s.job_id AND t.task_id=?
JOIN prepare_jobs j ON j.job_id=t.job_id
JOIN prepare_liquibase_invocations i ON i.session_id=s.session_id AND i.job_id=t.job_id AND i.task_id=t.task_id AND i.attempt_no=?
JOIN prepare_attempt_evidence complete ON complete.evidence_id=?
JOIN prepare_attempt_evidence q ON q.evidence_id=?
WHERE s.session_id=? AND c.phase='quiescent' AND c.db_runtime_id IS NOT NULL
AND j.status IN ('running','recovery_required') AND t.status IN ('running','recovery_required')
AND t.type='state_execute' AND t.output_state_id=? AND t.position=s.cursor_position
AND coalesce(nullif(j.resolved_image_id,''),j.image_id)=?
AND coalesce(j.lineage_ref,'')=? AND coalesce(j.identity_digest,'')=?
AND ((t.input_kind='image' AND t.input_id=s.cursor_image_id AND ? IS NULL)
  OR (t.input_kind='state' AND t.input_id=s.cursor_state_id AND t.input_id=?))
AND i.complete_at IS NOT NULL AND i.exit_code=0
AND complete.job_id=t.job_id AND complete.task_id=t.task_id AND complete.attempt_no=i.attempt_no
AND complete.worker_generation=s.worker_generation AND complete.kind='exec_completion'
AND json_extract(complete.facts_json,'$.observation.runner_id')=s.runner_id
AND json_extract(complete.facts_json,'$.observation.session_id')=s.session_id
AND json_extract(complete.facts_json,'$.observation.invocation_ref')=i.invocation_ref
AND json_extract(complete.facts_json,'$.observation.manifest_digest')=i.completion_digest
AND json_extract(complete.facts_json,'$.observation.exit_code')=0
AND json_extract(complete.facts_json,'$.observation.children_absent')=1
AND q.job_id=t.job_id AND q.task_id=t.task_id AND q.attempt_no=i.attempt_no
AND q.worker_generation=s.worker_generation AND q.kind='db_quiescence' AND q.evidence_id>complete.evidence_id
AND json_extract(q.facts_json,'$.observation.session_id')=s.session_id
AND json_extract(q.facts_json,'$.observation.runtime_name')=c.planned_db_runtime_name
AND json_extract(q.facts_json,'$.observation.runtime_id')=c.db_runtime_id
AND json_extract(q.facts_json,'$.observation.postgres_stopped')=1
AND json_extract(q.facts_json,'$.observation.runtime_present')=1`, invocation.Attempt.TaskID,
		invocation.Attempt.AttemptNo, completionID, quiescenceID, session.SessionID,
		entry.StateID, entry.ImageID, entry.LineageRef, entry.IdentityDigest,
		entry.ParentStateID, entry.ParentStateID).Scan(&eligible); err != nil || eligible != 1 {
		return fmt.Errorf("%w: session publication proof", ErrCommonAttempt)
	}
	if err := insertSessionPublishedStateTx(ctx, tx, entry, invocation.Attempt.ExecutionDigest); err != nil {
		return err
	}
	pubID, err := appendSessionEvidenceTx(ctx, tx, invocation.Attempt, "publication",
		map[string]any{"session_id": session.SessionID, "state_id": entry.StateID}, at)
	if err != nil {
		return err
	}
	var decisionNo int
	if err := tx.QueryRowContext(ctx, `SELECT coalesce(max(decision_no),0)+1 FROM prepare_attempt_decisions
WHERE job_id=? AND task_id=? AND attempt_no=?`, invocation.Attempt.JobID, invocation.Attempt.TaskID,
		invocation.Attempt.AttemptNo).Scan(&decisionNo); err != nil {
		return err
	}
	decisionID := commonEvidenceDigest(fmt.Sprintf("attempt-decision-v1\n%s\n%s\n%d\n%d",
		invocation.Attempt.JobID, invocation.Attempt.TaskID, invocation.Attempt.AttemptNo, decisionNo))
	if _, err := tx.ExecContext(ctx, `INSERT INTO prepare_attempt_decisions(decision_id,job_id,task_id,attempt_no,
decision_no,choice,worker_generation,created_at) VALUES(?,?,?,?,?,'publish_original',?,?)`, decisionID,
		invocation.Attempt.JobID, invocation.Attempt.TaskID, invocation.Attempt.AttemptNo, decisionNo, session.WorkerGeneration, at); err != nil {
		return err
	}
	for _, id := range []int64{completionID, quiescenceID, pubID} {
		if _, err := tx.ExecContext(ctx, `INSERT INTO prepare_decision_evidence(decision_id,evidence_id) VALUES(?,?)`, decisionID, id); err != nil {
			return err
		}
	}
	result, err := tx.ExecContext(ctx, `UPDATE prepare_step_attempts SET phase='closed',resolution='published'
WHERE job_id=? AND task_id=? AND attempt_no=? AND executor_kind='session_exec' AND phase='exec_completed' AND resolution IS NULL`,
		invocation.Attempt.JobID, invocation.Attempt.TaskID, invocation.Attempt.AttemptNo)
	if err != nil || oneRowChanged(result) != nil {
		return fmt.Errorf("%w: session attempt publication: %v", ErrCommonAttempt, err)
	}
	result, err = tx.ExecContext(ctx, `UPDATE prepare_tasks SET status='succeeded',finished_at=?,error_json=NULL
WHERE job_id=? AND task_id=? AND status IN ('running','recovery_required')`, at, invocation.Attempt.JobID, invocation.Attempt.TaskID)
	if err != nil || oneRowChanged(result) != nil {
		return ErrCommonAttempt
	}
	result, err = tx.ExecContext(ctx, `UPDATE prepare_liquibase_sessions SET cursor_image_id=NULL,
cursor_state_id=?,cursor_position=cursor_position+1 WHERE `+liquibaseSessionIdentityPredicate+` AND db_phase='stopped' AND retired_at IS NULL`,
		append([]any{entry.StateID}, sessionIdentityArgs(session)...)...)
	if err != nil || oneRowChanged(result) != nil {
		return ErrCommonAttempt
	}
	return tx.Commit()
}

func insertSessionPublishedStateTx(ctx context.Context, tx *sql.Tx, entry store.StateCreate, executionDigest string) error {
	if _, err := tx.ExecContext(ctx, `INSERT INTO states(state_id,parent_state_id,state_fingerprint,image_id,
prepare_kind,prepare_args_normalized,created_at,size_bytes,last_used_at,use_count,status,lineage_ref,identity_digest)
VALUES(?,?,?,?,?,?,?,?,?,0,?,?,?)`, entry.StateID, entry.ParentStateID, entry.StateFingerprint,
		entry.ImageID, entry.PrepareKind, entry.PrepareArgsNormalized, entry.CreatedAt, entry.SizeBytes,
		entry.CreatedAt, entry.Status, entry.LineageRef, entry.IdentityDigest); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO liquibase_state_bindings(state_id,execution_fingerprint) VALUES(?,?)`, entry.StateID, entry.LiquibaseExecutionFingerprint); err != nil {
		return err
	}
	_, err := tx.ExecContext(ctx, `INSERT INTO prepare_state_bindings(state_id,execution_digest) VALUES(?,?)`, entry.StateID, executionDigest)
	return err
}

// RequestLiquibaseSessionDBRestart commits restart intent after publication.
// No unresolved command may be running while PostgreSQL is brought back up.
func (s *SQLiteStore) RequestLiquibaseSessionDBRestart(ctx context.Context, session LiquibaseSessionRecord) error {
	if s == nil || s.db == nil {
		return ErrCommonAttempt
	}
	result, err := s.db.ExecContext(ctx, `UPDATE prepare_liquibase_sessions SET db_phase='restart_requested'
WHERE `+liquibaseSessionIdentityPredicate+` AND runner_id=? AND runner_phase='running'
AND db_phase IN ('stopped','restart_requested') AND retired_at IS NULL
AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_cache_restores r WHERE r.session_id=prepare_liquibase_sessions.session_id AND r.restore_phase NOT IN ('published','abandoned'))
AND NOT EXISTS (SELECT 1 FROM prepare_step_attempts a WHERE a.clone_id=prepare_liquibase_sessions.clone_id AND a.resolution IS NULL)`,
		append(sessionIdentityArgs(session), session.RunnerID)...)
	if err != nil {
		return err
	}
	return oneRowChanged(result)
}

// MarkLiquibaseSessionDBReady commits an exact-runtime readiness observation
// with clone reactivation. The caller has verified managed identity and SQL
// readiness in this same database container after pg_ctl start.
func (s *SQLiteStore) MarkLiquibaseSessionDBReady(ctx context.Context, session LiquibaseSessionRecord, runtimeName, runtimeID string) error {
	if s == nil || s.db == nil || runtimeName == "" || !dockerContainerID.MatchString(runtimeID) {
		return ErrCommonAttempt
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var eligible int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM prepare_liquibase_sessions WHERE `+liquibaseSessionIdentityPredicate+`
AND runner_id=? AND runner_phase='running' AND db_phase IN ('restart_requested','running') AND retired_at IS NULL`,
		append(sessionIdentityArgs(session), session.RunnerID)...).Scan(&eligible); err != nil || eligible != 1 {
		return ErrCommonAttempt
	}
	result, err := tx.ExecContext(ctx, `UPDATE prepare_clones SET phase='active' WHERE clone_id=? AND job_id=?
AND planned_db_runtime_name=? AND db_runtime_id=? AND phase IN ('quiescent','active')`, session.CloneID, session.JobID, runtimeName, runtimeID)
	if err != nil || oneRowChanged(result) != nil {
		return ErrCommonAttempt
	}
	result, err = tx.ExecContext(ctx, `UPDATE prepare_liquibase_sessions SET db_phase='running' WHERE `+liquibaseSessionIdentityPredicate+`
AND db_phase IN ('restart_requested','running') AND retired_at IS NULL`, sessionIdentityArgs(session)...)
	if err != nil || oneRowChanged(result) != nil {
		return ErrCommonAttempt
	}
	return tx.Commit()
}
