package queue

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"reflect"

	"github.com/sqlrs/engine-local/internal/instanceaccess"
	"github.com/sqlrs/engine-local/internal/managedidentity"
	"github.com/sqlrs/engine-local/internal/store"
)

// writeWorkingInstanceTransfer consumes a completed, stopped-parent session.
// The deferred FK forbids committing this future-bound row without the exact
// success intent. No native effects occur under the owned SQL transaction.
// Requirements: issue89-working-instance-handoff-schema.md §2.
func writeWorkingInstanceTransfer(ctx context.Context, tx *sql.Tx, session LiquibaseSessionRecord, entry store.InstanceCreate, binding managedidentity.RuntimeBinding, operation instanceaccess.Operation, digest string) error {
	var exact int
	err := tx.QueryRowContext(ctx, `SELECT count(*) FROM prepare_liquibase_sessions s
JOIN prepare_clones c ON c.clone_id=s.clone_id AND c.job_id=s.job_id
JOIN states st ON st.state_id=s.cursor_state_id
JOIN liquibase_state_bindings lb ON lb.state_id=st.state_id AND lb.execution_fingerprint=s.execution_fingerprint
JOIN managed_state_seals seal ON seal.state_id=st.state_id AND seal.operation_ref=?
WHERE s.session_id IN (SELECT session_id FROM prepare_liquibase_sessions WHERE `+liquibaseSessionIdentityPredicate+`)
AND s.start_kind=? AND s.start_id=? AND s.start_position=? AND s.session_no=? AND s.created_at=?
AND s.runner_id=? AND s.runner_phase='stopped' AND s.db_phase='running' AND s.retired_at IS NULL
AND s.cursor_state_id=? AND s.cursor_position=? AND c.phase='active' AND c.db_runtime_id=?
AND NOT EXISTS (SELECT 1 FROM prepare_tasks t WHERE t.job_id=s.job_id AND t.type='state_execute' AND t.status<>'succeeded')
AND EXISTS (SELECT 1 FROM prepare_tasks t JOIN prepare_liquibase_invocations i USING(job_id,task_id)
 JOIN prepare_step_attempts a USING(job_id,task_id,attempt_no)
 WHERE t.job_id=s.job_id AND t.position=s.cursor_position-1 AND t.output_state_id=s.cursor_state_id
 AND i.session_id=s.session_id AND a.clone_id=s.clone_id AND a.worker_generation=s.worker_generation
 AND a.phase='closed' AND a.resolution='published')
AND NOT EXISTS (SELECT 1 FROM prepare_step_attempts a WHERE a.clone_id=c.clone_id AND (a.phase<>'closed' OR a.resolution IS NOT 'published'))
AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_cache_restores r WHERE r.session_id=s.session_id AND r.restore_phase NOT IN ('published','abandoned'))
AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_job_cancellations cancel WHERE cancel.job_id=s.job_id)
AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_job_finalizations selected WHERE selected.job_id=s.job_id)`,
		append(append([]any{operation.Ref}, sessionIdentityArgs(session)...), session.StartKind, session.StartID, session.StartPosition, session.SessionNo, session.CreatedAt, session.RunnerID, entry.StateID, session.CursorPosition, binding.RuntimeRef)...).Scan(&exact)
	if err != nil || exact != 1 {
		return ErrLiquibaseFinalization
	}
	var seal string
	if err := tx.QueryRowContext(ctx, "SELECT binding_json FROM managed_state_seals WHERE state_id=? AND operation_ref=?", entry.StateID, operation.Ref).Scan(&seal); err != nil {
		return ErrLiquibaseFinalization
	}
	var sealed managedidentity.RuntimeBinding
	if json.Unmarshal([]byte(seal), &sealed) != nil || sealed != binding {
		return ErrLiquibaseFinalization
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO prepare_liquibase_session_disposals
(session_id,reason,authority_ref,requested_at,runner_id,db_runtime_id,handoff_job_id,handoff_instance_id,handoff_state_id,handoff_operation_ref,handoff_runtime_dir,handoff_outcome_digest)
VALUES(?,'instance_transferred',?,?,?,?,?,?,?,?,?,?)`, session.SessionID, digest, entry.CreatedAt, session.RunnerID, binding.RuntimeRef, session.JobID, entry.InstanceID, entry.StateID, operation.Ref, operation.Target, digest); err != nil {
		return err
	}
	return writePrivateSessionDisposalTx(ctx, tx, session, "instance_transferred", digest, session.RunnerID, binding.RuntimeRef, entry.CreatedAt)
}

// verifyWorkingInstanceTransfer compares the complete immutable transfer and
// its live publication binding. It never treats partial readback as a commit.
func verifyWorkingInstanceTransfer(ctx context.Context, tx *sql.Tx, session LiquibaseSessionRecord, entry store.InstanceCreate, binding managedidentity.RuntimeBinding, operation instanceaccess.Operation, digest string) error {
	var job, instance, state, op, target, outcome, runner, database, created string
	var absent sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT d.handoff_job_id,d.handoff_instance_id,d.handoff_state_id,d.handoff_operation_ref,d.handoff_runtime_dir,d.handoff_outcome_digest,d.runner_id,d.db_runtime_id,d.requested_at,d.db_absent_at
FROM prepare_liquibase_session_disposals d JOIN prepare_liquibase_sessions s ON s.session_id=d.session_id
JOIN prepare_clones c ON c.clone_id=s.clone_id AND c.job_id=s.job_id
JOIN prepare_liquibase_job_finalizations f ON f.job_id=d.handoff_job_id AND f.instance_pin=d.handoff_instance_id
WHERE d.session_id IN (SELECT session_id FROM prepare_liquibase_sessions WHERE `+liquibaseSessionIdentityPredicate+`)
AND s.start_kind=? AND s.start_id=? AND s.start_position=? AND s.session_no=? AND s.created_at=?
AND d.reason='instance_transferred' AND d.authority_ref=d.handoff_outcome_digest
AND s.job_id=d.handoff_job_id AND s.runner_id=d.runner_id AND c.db_runtime_id=d.db_runtime_id
AND f.outcome_kind='prepare_success' AND f.outcome_digest=d.handoff_outcome_digest
AND f.instance_id=d.handoff_instance_id AND f.final_state_id=d.handoff_state_id`, append(sessionIdentityArgs(session), session.StartKind, session.StartID, session.StartPosition, session.SessionNo, session.CreatedAt)...).Scan(&job, &instance, &state, &op, &target, &outcome, &runner, &database, &created, &absent)
	if err != nil || job != session.JobID || instance != entry.InstanceID || state != entry.StateID || op != operation.Ref || target != operation.Target || outcome != digest || runner != session.RunnerID || database != binding.RuntimeRef || created != entry.CreatedAt || absent.Valid {
		return ErrLiquibaseFinalization
	}
	// Identity arguments deliberately exclude mutable cursor/phase fields; before
	// private retirement those still must exactly match this publication request.
	var phase, dbPhase string
	var cursor sql.NullString
	var position int
	var retired sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT runner_phase,db_phase,cursor_state_id,cursor_position,retired_at FROM prepare_liquibase_sessions WHERE session_id=?`, session.SessionID).Scan(&phase, &dbPhase, &cursor, &position, &retired); err != nil {
		return ErrLiquibaseFinalization
	}
	if phase != session.RunnerPhase || dbPhase != session.DBPhase || cursor.String != session.CursorID || position != session.CursorPosition || retired.Valid != (session.RetiredAt != nil) {
		return ErrLiquibaseFinalization
	}
	return nil
}

// LiquibaseTransferredSessionVerifier observes only disappearance of the exact
// stopped parent/private bytes. The database is intentionally live; this must
// never be implemented by a database-absence verifier. Handoff schema §3.
type LiquibaseTransferredSessionVerifier interface {
	VerifyTransferredLiquibaseSession(context.Context, LiquibaseSessionRecord, ClonePlan, string) error
}

// RetireTransferredLiquibaseSession drops private/job ownership, not the live
// database. It rechecks the committed receipt/pin before and after native I/O,
// then settles the complete capture and logical-owner transition atomically.
func (s *SQLiteStore) RetireTransferredLiquibaseSession(ctx context.Context, session LiquibaseSessionRecord, verifier LiquibaseTransferredSessionVerifier, at string) error {
	if s == nil || s.db == nil || verifier == nil || !validFinalizationUTC(at) {
		return ErrLiquibaseFinalization
	}
	clone, _, database, err := s.ReadLiquibaseSessionClone(ctx, session)
	if err != nil {
		return err
	}
	readOwner := func(ctx context.Context, tx *sql.Tx, expected LiquibaseSessionRecord) (FinalizationSnapshot, error) {
		intent, found, err := loadFinalization(ctx, tx, session.JobID)
		if err != nil || !found || intent.Outcome.Prepare == nil || intent.InstancePin != intent.Outcome.Prepare.InstanceID || intent.ProjectionsRemovedAt != "" {
			return FinalizationSnapshot{}, ErrLiquibaseFinalization
		}
		binding, op, err := verifyPrepareFinalizationReference(ctx, tx, session.JobID, intent.Outcome)
		if err != nil || binding.RuntimeRef != database {
			return FinalizationSnapshot{}, ErrLiquibaseFinalization
		}
		entry := store.InstanceCreate{InstanceID: intent.Outcome.Prepare.InstanceID, StateID: intent.Outcome.Prepare.StateID, CreatedAt: intent.CreatedAt}
		if err := verifyWorkingInstanceTransfer(ctx, tx, expected, entry, binding, op, intent.Digest); err != nil {
			return FinalizationSnapshot{}, err
		}
		return intent, nil
	}
	check, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return err
	}
	_, err = readOwner(ctx, check, session)
	before, captureErr := readPrivateFailureCaptureState(ctx, check, session)
	closeErr := check.Rollback()
	if err != nil || captureErr != nil || closeErr != nil {
		return errors.Join(err, captureErr, closeErr)
	}
	if err := verifier.VerifyTransferredLiquibaseSession(ctx, session, clone, database); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	_, err = withFinalization(ctx, s, func(f *finalizationConnection) (struct{}, error) {
		intent, err := readOwner(ctx, f.tx, session)
		if err != nil {
			return struct{}{}, err
		}
		current, err := readPrivateFailureCaptureState(ctx, f.tx, session)
		if err != nil || !reflect.DeepEqual(current, before) {
			return struct{}{}, ErrLiquibaseFinalization
		}
		if _, err = f.tx.ExecContext(ctx, `UPDATE prepare_liquibase_session_disposals SET runner_absent_at=? WHERE session_id=? AND runner_absent_at IS NULL`, at, session.SessionID); err != nil {
			return struct{}{}, err
		}
		if _, err = f.tx.ExecContext(ctx, `UPDATE prepare_liquibase_capture_retirements SET verified_at=?,absence_kind='container_absent' WHERE session_id=? AND verified_at IS NULL`, at, session.SessionID); err != nil {
			return struct{}{}, err
		}
		if _, err = f.tx.ExecContext(ctx, `DELETE FROM prepare_liquibase_capture_slots WHERE session_id=?`, session.SessionID); err != nil {
			return struct{}{}, err
		}
		if _, err = f.tx.ExecContext(ctx, `UPDATE prepare_liquibase_invocations SET retired_at=? WHERE session_id=? AND retired_at IS NULL`, at, session.SessionID); err != nil {
			return struct{}{}, err
		}
		result, err := f.tx.ExecContext(ctx, `UPDATE prepare_clones SET phase='released' WHERE clone_id=? AND job_id=? AND db_runtime_id=? AND phase='active'`, session.CloneID, session.JobID, database)
		if err != nil || oneRowChanged(result) != nil {
			return struct{}{}, errors.Join(ErrLiquibaseFinalization, err)
		}
		result, err = f.tx.ExecContext(ctx, `UPDATE prepare_liquibase_sessions SET runner_phase='retired',retired_at=?,cursor_image_id=NULL,cursor_state_id=NULL WHERE `+liquibaseSessionIdentityPredicate+` AND runner_id=? AND runner_phase='stopped' AND db_phase='running' AND retired_at IS NULL`, append(append([]any{at}, sessionIdentityArgs(session)...), session.RunnerID)...)
		if err != nil || oneRowChanged(result) != nil {
			return struct{}{}, errors.Join(ErrLiquibaseFinalization, err)
		}
		wanted, err := readPrivateFailureCaptureState(ctx, f.tx, session)
		if err != nil {
			return struct{}{}, err
		}
		retired := session
		retired.RunnerPhase = "retired"
		retired.CursorKind = ""
		retired.CursorID = ""
		retired.RetiredAt = &at
		if _, err = readOwner(ctx, f.tx, retired); err != nil {
			return struct{}{}, err
		}
		if err = validateFinalizationSelection(ctx, f.tx, intent.JobID, intent.Outcome); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, f.commit(func(ctx context.Context, tx *sql.Tx) (bool, error) {
			actual, err := readPrivateFailureCaptureState(ctx, tx, session)
			if err != nil {
				return false, err
			}
			if reflect.DeepEqual(actual, before) {
				return false, nil
			}
			if !reflect.DeepEqual(actual, wanted) {
				return false, ErrLiquibaseFinalization
			}
			if _, err := readOwner(ctx, tx, retired); err != nil {
				return false, err
			}
			if err := validateFinalizationSelection(ctx, tx, intent.JobID, intent.Outcome); err != nil {
				return false, err
			}
			return true, nil
		})
	})
	return err
}
