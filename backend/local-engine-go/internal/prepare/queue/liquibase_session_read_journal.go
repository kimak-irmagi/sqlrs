package queue

// Normal-path journal operations from the approved Liquibase prototype.
// Installation and runtime activation are separate boundaries; these methods
// do not enable replay. See issue89-normal-path-storage-extraction.md.

import (
	"context"
	"database/sql"
)

// scanFailureOwnerRows closes every result before the next stage and observes
// both row-stream and close errors, even for an empty set. scan must only append
// to the private in-progress snapshot; callers discard it on any stage failure.
func scanFailureOwnerRows(ctx context.Context, tx *sql.Tx, query string, job string, scan func(*sql.Rows) error) error {
	rows, err := tx.QueryContext(ctx, query, job)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		if err := scan(rows); err != nil {
			return err
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	return rows.Close()
}

// privateSessionDisposal retains physical targets independently of the eventual
// job outcome. The SQL guards own reason-specific authority and its lifetime.
// Requirements: liquibase-private-capture-storage-schema.md §§3.3–3.4.
type privateSessionDisposal struct {
	reason, authority, requested, runner, database                      string
	dbAbsent, runnerAbsent                                              sql.NullString
	nonstartBasis, nonstartScope, nonstartObservation, nonstartObserved sql.NullString
}

// readPrivateSessionDisposal binds saved physical ownership to the complete
// immutable session and clone, not just a caller-supplied session ID.
func readPrivateSessionDisposal(ctx context.Context, reader privateReadinessReader, session LiquibaseSessionRecord) (privateSessionDisposal, error) {
	var d privateSessionDisposal
	err := reader.QueryRowContext(ctx, `SELECT d.reason,d.authority_ref,d.requested_at,d.runner_id,d.db_runtime_id,d.db_absent_at,d.runner_absent_at,d.nonstart_basis,d.nonstart_scope_digest,d.nonstart_observation_digest,d.nonstart_observed_at
FROM prepare_liquibase_session_disposals d JOIN prepare_liquibase_sessions ON prepare_liquibase_sessions.session_id=d.session_id
JOIN prepare_clones clone ON clone.clone_id=prepare_liquibase_sessions.clone_id AND clone.job_id=prepare_liquibase_sessions.job_id
WHERE prepare_liquibase_sessions.session_id IN (SELECT session_id FROM prepare_liquibase_sessions WHERE `+liquibaseSessionIdentityPredicate+`) AND prepare_liquibase_sessions.runner_id=?
AND d.runner_id=prepare_liquibase_sessions.runner_id AND d.db_runtime_id=clone.db_runtime_id
AND prepare_liquibase_sessions.capture_location='container_private' AND prepare_liquibase_sessions.capture_protocol_revision='lb-step-capture-v2'`,
		append(sessionIdentityArgs(session), session.RunnerID)...).Scan(&d.reason, &d.authority, &d.requested, &d.runner, &d.database, &d.dbAbsent, &d.runnerAbsent, &d.nonstartBasis, &d.nonstartScope, &d.nonstartObservation, &d.nonstartObserved)
	if err != nil {
		return privateSessionDisposal{}, err
	}
	if !validFinalizationUTC(d.requested) || !dockerContainerID.MatchString(d.runner) || !dockerContainerID.MatchString(d.database) ||
		d.dbAbsent.Valid && !validFinalizationUTC(d.dbAbsent.String) || d.runnerAbsent.Valid && (!d.dbAbsent.Valid || !validFinalizationUTC(d.runnerAbsent.String)) {
		return privateSessionDisposal{}, ErrCommonAttempt
	}
	if d.reason == "proven_nonstart" {
		if !d.nonstartBasis.Valid || d.nonstartBasis.String != "attempt" && d.nonstartBasis.String != "session_uninvoked" ||
			!d.nonstartScope.Valid || !bindingDigest.MatchString(d.nonstartScope.String) ||
			!d.nonstartObservation.Valid || !bindingDigest.MatchString(d.nonstartObservation.String) ||
			!d.nonstartObserved.Valid || !validFinalizationUTC(d.nonstartObserved.String) {
			return privateSessionDisposal{}, ErrCommonAttempt
		}
	} else if d.nonstartBasis.Valid || d.nonstartScope.Valid || d.nonstartObservation.Valid || d.nonstartObserved.Valid {
		return privateSessionDisposal{}, ErrCommonAttempt
	}
	return d, nil
}

// privateFailureCaptureState retains exact rows, including old prune history
// and nullable timestamps. Counts alone cannot certify a lost COMMIT reply.
type privateFailureCaptureState struct {
	disposal    *privateSessionDisposal
	slots       []privateFailureCaptureSlot
	retirements []privateFailureCaptureRetirement
}

type privateFailureCaptureSlot struct {
	slot, attempt       int
	job, task, reserved string
	prior               sql.NullString
}

type privateFailureCaptureRetirement struct {
	ref, job, task, reason, requested string
	attempt                           int
	verified, absence                 sql.NullString
}

// readPrivateFailureCaptureState rejects missing/orphan ownership mappings,
// not just uncovered slots. Every invocation must own a slot or an already
// confirmed retirement; every slot/retirement must point back to that session.
func readPrivateFailureCaptureState(ctx context.Context, tx *sql.Tx, session LiquibaseSessionRecord) (privateFailureCaptureState, error) {
	var result privateFailureCaptureState
	disposal, err := readPrivateSessionDisposal(ctx, tx, session)
	if err == nil {
		result.disposal = &disposal
	} else if err != sql.ErrNoRows {
		return result, err
	}
	var invalid int
	err = tx.QueryRowContext(ctx, `SELECT
(SELECT count(*) FROM prepare_liquibase_invocations i WHERE i.session_id=? AND NOT EXISTS
 (SELECT 1 FROM prepare_liquibase_capture_slots s WHERE s.session_id=i.session_id AND s.job_id=i.job_id AND s.task_id=i.task_id AND s.attempt_no=i.attempt_no)
 AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_capture_retirements r WHERE r.session_id=i.session_id AND r.job_id=i.job_id AND r.task_id=i.task_id AND r.attempt_no=i.attempt_no AND r.verified_at IS NOT NULL))+
(SELECT count(*) FROM prepare_liquibase_capture_slots s WHERE s.session_id=? AND NOT EXISTS
 (SELECT 1 FROM prepare_liquibase_invocations i WHERE i.session_id=s.session_id AND i.job_id=s.job_id AND i.task_id=s.task_id AND i.attempt_no=s.attempt_no))+
(SELECT count(*) FROM prepare_liquibase_capture_retirements r WHERE r.session_id=? AND NOT EXISTS
 (SELECT 1 FROM prepare_liquibase_invocations i WHERE i.session_id=r.session_id AND i.job_id=r.job_id AND i.task_id=r.task_id AND i.attempt_no=r.attempt_no))`, session.SessionID, session.SessionID, session.SessionID).Scan(&invalid)
	if err != nil {
		return result, err
	}
	if invalid != 0 {
		return result, ErrLiquibaseFinalization
	}
	err = scanFailureOwnerRows(ctx, tx, `SELECT slot_no,job_id,task_id,attempt_no,reserved_at,prior_retirement_ref FROM prepare_liquibase_capture_slots WHERE session_id=? ORDER BY slot_no`, session.SessionID, func(rows *sql.Rows) error {
		var slot privateFailureCaptureSlot
		if err := rows.Scan(&slot.slot, &slot.job, &slot.task, &slot.attempt, &slot.reserved, &slot.prior); err != nil {
			return err
		}
		result.slots = append(result.slots, slot)
		return nil
	})
	if err != nil {
		return result, err
	}
	err = scanFailureOwnerRows(ctx, tx, `SELECT retirement_ref,job_id,task_id,attempt_no,reason,requested_at,verified_at,absence_kind FROM prepare_liquibase_capture_retirements WHERE session_id=? ORDER BY retirement_ref`, session.SessionID, func(rows *sql.Rows) error {
		var retirement privateFailureCaptureRetirement
		if err := rows.Scan(&retirement.ref, &retirement.job, &retirement.task, &retirement.attempt, &retirement.reason, &retirement.requested, &retirement.verified, &retirement.absence); err != nil {
			return err
		}
		result.retirements = append(result.retirements, retirement)
		return nil
	})
	return result, err
}
