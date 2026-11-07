package queue

// Known-outcome disposal only. These writers do not retry SQL or enable the
// interrupted-work recovery dispatcher deferred to #170.
import (
	"context"
	"database/sql"
	"github.com/sqlrs/engine-local/internal/prepare/finalizationcodec"
	"reflect"
)

// DiscardSelectedLiquibaseFailureSession disposes ALL children/captures of one
// stopped exact session under an immutable job failure, never natural failure
// or replay authority. Caller continuously holds exclusive job/state ownership.
// The mandatory native verifier runs without a SQLite transaction or eviction
// exclusion. Tasks, outcome, leases, child results and replay budget are intact.
// Requirements: failure storage §4, JF05/JF07/JF08.
func (s *SQLiteStore) DiscardSelectedLiquibaseFailureSession(ctx context.Context, intent FinalizationSnapshot, expected LiquibaseSessionRecord, verifier LiquibaseSessionAbsenceVerifier, at string) error {
	if s == nil || s.db == nil || verifier == nil || !validFinalizationUTC(at) || expected.JobID != intent.JobID {
		return ErrLiquibaseFinalization
	}
	owners, err := s.LoadLiquibaseFailureOwners(ctx, intent)
	if err != nil {
		return err
	}
	session, clone, invocations, retired, err := stoppedFailureSessionState(owners, expected)
	if err != nil {
		return err
	}
	if err := s.checkStoppedFailureEvidence(ctx, owners, invocations, retired); err != nil || retired {
		return err
	}
	if err := verifier.VerifyReleasedLiquibaseSession(ctx, session, clone.Plan, clone.DBRuntimeID); err != nil {
		return err
	}
	_, err = withFinalization(ctx, s, func(f *finalizationConnection) (struct{}, error) {
		return struct{}{}, discardSelectedFailureSessionTx(ctx, f, intent, expected, owners, session, clone, invocations, at)
	})
	return err
}

// discardSelectedFailureSessionTx keeps the logical retirement and its complete
// reconciliation on one owned physical connection. Native host work is already
// finished; no pool reborrow can certify its own uncommitted retirement.
// Requirements: private-capture ownership §5.1, SF04-T/OT-SF02.
func discardSelectedFailureSessionTx(ctx context.Context, f *finalizationConnection, intent FinalizationSnapshot, expected LiquibaseSessionRecord, owners LiquibaseFailureOwners, session LiquibaseSessionRecord, clone LiquibaseFailureClone, invocations []LiquibaseFailureInvocation, at string) error {
	tx := f.tx
	current, err := loadLiquibaseFailureOwnersTx(ctx, tx, intent)
	if err != nil {
		return err
	}
	actual, actualClone, children, retired, err := stoppedFailureSessionState(current, expected)
	if err != nil || retired {
		return ErrLiquibaseFinalization
	}
	if !reflect.DeepEqual(session, actual) || clone.Plan != actualClone.Plan || clone.DBRuntimeID != actualClone.DBRuntimeID || !reflect.DeepEqual(invocations, children) || !reflect.DeepEqual(owners.Restores, current.Restores) || !retiredFailureCloneOperations(current, clone.Plan) {
		return ErrLiquibaseFinalization
	}
	beforeEvidence, err := readSelectedFailureTailEvidence(ctx, tx, actual)
	if err != nil {
		return err
	}
	beforeCapture, err := readPrivateFailureCaptureState(ctx, tx, actual)
	if err != nil {
		return err
	}
	for _, child := range children {
		if child.Record.Attempt.Resolution != "" {
			continue
		}
		if err := disposeSelectedFailureInvocationTx(ctx, tx, current.Intent, actual, actualClone, child, at); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE prepare_liquibase_invocations SET retired_at=? WHERE session_id=? AND job_id=? AND retired_at IS NULL`, at, actual.SessionID, intent.JobID); err != nil {
		return err
	}
	// Abandon only this exact unpublished copy AFTER physical retirement. Its
	// immutable source row/reference survives, and no cached task is completed.
	if _, err := tx.ExecContext(ctx, `UPDATE prepare_liquibase_cache_restores SET restore_phase='abandoned',completed_at=? WHERE session_id=? AND job_id=? AND restore_phase IN ('requested','stopped','restored')`, at, actual.SessionID, intent.JobID); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE prepare_clones SET phase='released' WHERE clone_id=? AND job_id=? AND clone_generation=? AND clone_ref=? AND planned_db_runtime_name=? AND db_runtime_id=? AND phase IN ('active','quiescent')`, clone.Plan.ID, intent.JobID, clone.Plan.Generation, clone.Plan.Ref, clone.Plan.PlannedRuntimeName, clone.DBRuntimeID)
	if err != nil || oneRowChanged(result) != nil {
		return ErrLiquibaseFinalization
	}
	result, err = tx.ExecContext(ctx, `UPDATE prepare_liquibase_sessions SET runner_phase='retired',db_phase='stopped',retired_at=?,cursor_image_id=NULL,cursor_state_id=NULL WHERE `+liquibaseSessionIdentityPredicate+` AND runner_id=? AND runner_phase='stopped' AND retired_at IS NULL`, append(append([]any{at}, sessionIdentityArgs(actual)...), actual.RunnerID)...)
	if err != nil || oneRowChanged(result) != nil {
		return ErrLiquibaseFinalization
	}
	wanted, err := loadLiquibaseFailureOwnersTx(ctx, tx, intent)
	if err != nil {
		return err
	}
	wantedEvidence, err := readSelectedFailureTailEvidence(ctx, tx, actual)
	if err != nil {
		return err
	}
	return f.commit(func(readCtx context.Context, readTx *sql.Tx) (bool, error) {
		observed, err := loadLiquibaseFailureOwnersTx(readCtx, readTx, intent)
		if err != nil {
			return false, err
		}
		_, _, children, done, err := stoppedFailureSessionState(observed, expected)
		if err != nil {
			return false, err
		}
		evidence, err := readSelectedFailureTailEvidence(readCtx, readTx, actual)
		if err != nil {
			return false, err
		}
		capture, err := readPrivateFailureCaptureState(readCtx, readTx, actual)
		if err != nil || !reflect.DeepEqual(capture, beforeCapture) {
			return false, ErrLiquibaseFinalization
		}
		if done && reflect.DeepEqual(observed, wanted) && reflect.DeepEqual(evidence, wantedEvidence) {
			for _, child := range children {
				if child.Record.Attempt.Resolution == "discarded_clone" {
					if err := verifySelectedFailureDisposalTx(readCtx, readTx, child.Record.Attempt); err != nil {
						return false, err
					}
				}
			}
			return true, nil
		}
		if !done && reflect.DeepEqual(observed, current) && reflect.DeepEqual(evidence, beforeEvidence) {
			return false, nil
		}
		return false, ErrLiquibaseFinalization
	})
}

func (s *SQLiteStore) checkStoppedFailureEvidence(ctx context.Context, owners LiquibaseFailureOwners, children []LiquibaseFailureInvocation, retired bool) error {
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for _, child := range children {
		if child.Record.Attempt.Resolution == "discarded_clone" {
			if err := verifySelectedFailureDisposalTx(ctx, tx, child.Record.Attempt); err != nil {
				return err
			}
		} else if child.Record.Attempt.Resolution == "" {
			var session LiquibaseSessionRecord
			for _, item := range owners.Sessions {
				if item.SessionID == child.Record.SessionID {
					session = item
				}
			}
			if retired {
				return ErrLiquibaseFinalization
			}
			if _, err := readSelectedFailureStopLinks(ctx, tx, session, child.Record.Attempt); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

func disposeSelectedFailureInvocationTx(ctx context.Context, tx *sql.Tx, intent FinalizationSnapshot, session LiquibaseSessionRecord, clone LiquibaseFailureClone, child LiquibaseFailureInvocation, at string) error {
	a := child.Record.Attempt
	links, err := readSelectedFailureStopLinks(ctx, tx, session, a)
	if err != nil {
		return err
	}
	database, err := appendSessionEvidenceTx(ctx, tx, a, "db_quiescence", map[string]any{"session_id": session.SessionID, "runtime_name": clone.Plan.PlannedRuntimeName, "runtime_id": clone.DBRuntimeID, "runtime_absent": 1}, at)
	if err != nil {
		return err
	}
	if _, err := appendSessionEvidenceTx(ctx, tx, a, selectedFailureDisposalKind, map[string]any{
		"outcome_digest": intent.Digest, "session_id": session.SessionID, "runner_id": session.RunnerID,
		"invocation_ref": child.Record.InvocationRef, "clone_generation": clone.Plan.Generation,
		"runtime_name": clone.Plan.PlannedRuntimeName, "runtime_id": clone.DBRuntimeID,
		"executor_absent": 1, "runtime_absent": 1, "original_phase": a.Phase,
		"request_id": links.request, "ack_id": links.ack, "stop_id": links.stopped, "database_id": database,
	}, at); err != nil {
		return err
	}
	if err := verifySelectedFailureDisposalTx(ctx, tx, a); err != nil {
		return err
	}
	result, err := tx.ExecContext(ctx, `UPDATE prepare_step_attempts SET phase='closed',resolution='discarded_clone',finished_at=coalesce(finished_at,?) WHERE job_id=? AND task_id=? AND attempt_no=? AND clone_id=? AND execution_digest=? AND worker_generation=? AND executor_kind='session_exec' AND resolution IS NULL AND phase=?`, at, a.JobID, a.TaskID, a.AttemptNo, a.CloneID, a.ExecutionDigest, a.WorkerGeneration, a.Phase)
	if err != nil || oneRowChanged(result) != nil {
		return ErrLiquibaseFinalization
	}
	return nil
}

const selectedFailureDisposalKind = "selected_failure_disposal"

// LoadLiquibaseFailureOwners reads ALL historical/current owner locators in one
// read transaction. It neither chooses the latest attempt nor resolves a live
// runtime from a name. Any missing/inconsistent binding or unreadable stream
// returns zero authority. Native cleanup still needs exact per-owner rechecks
// and an independent absence verifier under the exclusive job/state ownership.
// Requirements: checkpoint recovery §28.2–3 and failure storage §§3–4, JF05/08.
func (s *SQLiteStore) LoadLiquibaseFailureOwners(ctx context.Context, intent FinalizationSnapshot) (LiquibaseFailureOwners, error) {
	var none LiquibaseFailureOwners
	if s == nil || s.db == nil || intent.JobID == "" {
		return none, ErrLiquibaseFinalization
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return none, failureOwnerReadError(ctx)
	}
	defer tx.Rollback()
	result, err := loadLiquibaseFailureOwnersTx(ctx, tx, intent)
	if err != nil {
		return none, err
	}
	if err := tx.Commit(); err != nil {
		return none, failureOwnerReadError(ctx)
	}
	return result, nil
}

// SelectLiquibaseJobFailure records the deliberate job-wide decision from
// checkpoint-capacity-recovery.md §28 and job-failure storage §2. It is NOT a
// child-exit result, cleanup proof, task failure or SQL-replay permission.
// A different valid winner returns rejection; callers must independently load
// and dispatch its validated outcome rather than falling back to generic failure.
func (s *SQLiteStore) SelectLiquibaseJobFailure(ctx context.Context, jobID string, outcome finalizationcodec.Outcome, at string) (FinalizationSnapshot, error) {
	if s == nil || s.db == nil || jobID == "" || !validFinalizationUTC(at) || outcome.Kind != finalizationcodec.Failure {
		return FinalizationSnapshot{}, ErrLiquibaseFinalization
	}
	canonical, digest, err := finalizationcodec.Encode(outcome)
	if err != nil {
		return FinalizationSnapshot{}, ErrLiquibaseFinalization
	}
	return withFinalization(ctx, s, func(f *finalizationConnection) (FinalizationSnapshot, error) {
		tx := f.tx
		if saved, found, err := loadFinalization(ctx, tx, jobID); err != nil {
			return FinalizationSnapshot{}, err
		} else if found {
			if saved.CanonicalJSON == canonical && saved.Digest == digest && saved.Outcome.Kind == outcome.Kind && saved.Outcome.EventProfile == outcome.EventProfile {
				return saved, nil
			}
			return FinalizationSnapshot{}, ErrLiquibaseFinalization
		}
		var version int
		if err := tx.QueryRowContext(ctx, `SELECT format_version FROM prepare_liquibase_finalization_store_format WHERE slot=1`).Scan(&version); err != nil || version != 2 {
			return FinalizationSnapshot{}, ErrLiquibaseFinalization
		}
		if err := validateEarlyJobFailure(ctx, tx, jobID, outcome); err != nil {
			return FinalizationSnapshot{}, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO prepare_liquibase_job_finalizations(job_id,format_revision,outcome_kind,event_profile,outcome_json,outcome_digest,created_at) VALUES(?,1,'failure',?,?,?,?)`, jobID, outcome.EventProfile, canonical, digest, at); err != nil {
			return FinalizationSnapshot{}, err
		}
		selected := FinalizationSnapshot{JobID: jobID, Outcome: outcome, CanonicalJSON: canonical, Digest: digest, CreatedAt: at}
		if err := f.commit(func(checkCtx context.Context, tx *sql.Tx) (bool, error) {
			observed, found, err := loadFinalization(checkCtx, tx, jobID)
			if err != nil {
				return false, err
			}
			if !found {
				return false, nil
			}
			if !sameFinalizationIntent(observed, selected) {
				return false, ErrLiquibaseFinalization
			}
			if err := validateEarlyJobFailure(checkCtx, tx, jobID, outcome); err != nil {
				return false, err
			}
			return true, nil
		}); err != nil {
			return FinalizationSnapshot{}, err
		}
		return selected, nil
	})
}

// SaveLiquibaseSessionFailureDisposal records physical cleanup authority and
// pending capture intents under the exact immutable selected failure. It does
// not remove resources, acknowledge absence, resolve SQL or release protection.
// Caller retains exclusive job/session ownership through subsequent cleanup.
// Requirements: liquibase-private-capture-ownership.md §5.1 and SF01–SF06.
func (s *SQLiteStore) SaveLiquibaseSessionFailureDisposal(ctx context.Context, intent FinalizationSnapshot, session LiquibaseSessionRecord, at string) error {
	if s == nil || s.db == nil || session.JobID != intent.JobID || !validFinalizationUTC(at) {
		return ErrLiquibaseFinalization
	}
	_, err := withFinalization(ctx, s, func(f *finalizationConnection) (struct{}, error) {
		owners, clone, err := selectedFailurePrivateOwnerTx(ctx, f.tx, intent, session)
		if err != nil {
			return struct{}{}, err
		}
		before, err := readPrivateFailureCaptureState(ctx, f.tx, session)
		if err != nil {
			return struct{}{}, err
		}
		basis, err := failurePhysicalAuthorityTx(ctx, f.tx, intent, session, owners, before.disposal)
		if err != nil {
			return struct{}{}, err
		}
		if err := writePrivateSessionDisposalTx(ctx, f.tx, session, basis.reason, basis.reference, session.RunnerID, clone.DBRuntimeID, at); err != nil {
			return struct{}{}, err
		}
		wanted, err := readPrivateFailureCaptureState(ctx, f.tx, session)
		if err != nil {
			return struct{}{}, err
		}
		return struct{}{}, f.commit(func(readCtx context.Context, tx *sql.Tx) (bool, error) {
			actualOwners, _, err := selectedFailurePrivateOwnerTx(readCtx, tx, intent, session)
			if err != nil {
				return false, err
			}
			actual, err := readPrivateFailureCaptureState(readCtx, tx, session)
			if err != nil {
				return false, err
			}
			if !reflect.DeepEqual(actualOwners, owners) {
				return false, ErrLiquibaseFinalization
			}
			actualBasis, err := failurePhysicalAuthorityTx(readCtx, tx, intent, session, actualOwners, actual.disposal)
			if err != nil || !reflect.DeepEqual(actualBasis, basis) {
				return false, ErrLiquibaseFinalization
			}
			if reflect.DeepEqual(actual, wanted) {
				return true, nil
			}
			if reflect.DeepEqual(actual, before) {
				return false, nil // Proven rollback, not a partial match.
			}
			return false, ErrLiquibaseFinalization
		})
	})
	return err
}
