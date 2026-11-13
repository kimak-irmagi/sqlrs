package queue

// Normal completion prerequisites from the approved journal. Shared readers
// validate saved authority; no recovery/replay dispatcher is installed.
// See docs/architecture/issue89-normal-path-storage-extraction.md.

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"github.com/sqlrs/engine-local/internal/instanceaccess"
	"github.com/sqlrs/engine-local/internal/managedidentity"
	"github.com/sqlrs/engine-local/internal/prepare/finalizationcodec"
	"github.com/sqlrs/engine-local/internal/prepare/planningcodec"
	"github.com/sqlrs/engine-local/internal/store"
	storesqlite "github.com/sqlrs/engine-local/internal/store/sqlite"
	"path/filepath"
	"time"
)

// JobCancellation is the immutable accepted request, not a stop/absence proof.
// It is private queue data; it does not add fields to the public job response.
type JobCancellation struct {
	JobID       string
	RequestedAt string
}

func loadJobCancellation(ctx context.Context, reader cancellationRowReader, jobID string) (JobCancellation, bool, error) {
	var record JobCancellation
	var kind, binding sql.NullString
	err := reader.QueryRowContext(ctx, `SELECT c.job_id,c.requested_at,j.prepare_kind,b.job_id
FROM prepare_liquibase_job_cancellations c
LEFT JOIN prepare_jobs j ON j.job_id=c.job_id
LEFT JOIN liquibase_job_bindings b ON b.job_id=j.job_id WHERE c.job_id=?`, jobID).Scan(&record.JobID, &record.RequestedAt, &kind, &binding)
	if errors.Is(err, sql.ErrNoRows) {
		return JobCancellation{}, false, nil
	}
	if err != nil {
		return JobCancellation{}, false, err
	}
	if record.JobID != jobID || !validFinalizationUTC(record.RequestedAt) || !kind.Valid || kind.String != "lb" || !binding.Valid || binding.String != jobID {
		return JobCancellation{}, false, ErrLiquibaseFinalization
	}
	return record, true, nil
}

// BeginFinalization selects a non-public immutable terminal outcome in a short
// transaction after rechecking all saved retirement authority. Ordinary prepare
// success is intentionally rejected: it must use the combined managed publisher.
// This method emits no public event and does not convert leases or clear pins.
// Requirements: liquibase-local-projection-cleanup-design.md, steps 2–4.
func (s *SQLiteStore) BeginFinalization(ctx context.Context, jobID string, outcome finalizationcodec.Outcome, at string) (FinalizationSnapshot, error) {
	if s == nil || s.db == nil || jobID == "" || !validFinalizationUTC(at) || outcome.Kind == finalizationcodec.PrepareSuccess {
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
			if saved.Digest != digest || saved.CanonicalJSON != canonical || saved.Outcome.Kind != outcome.Kind || saved.Outcome.EventProfile != outcome.EventProfile {
				return FinalizationSnapshot{}, ErrLiquibaseFinalization
			}
			return saved, nil
		}
		if err := validateFinalizationSelection(ctx, tx, jobID, outcome); err != nil {
			return FinalizationSnapshot{}, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO prepare_liquibase_job_finalizations
(job_id,format_revision,outcome_kind,event_profile,outcome_json,outcome_digest,created_at) VALUES(?,1,?,?,?,?,?)`,
			jobID, outcome.Kind, outcome.EventProfile, canonical, digest, at); err != nil {
			return FinalizationSnapshot{}, err
		}
		if outcome.Kind == finalizationcodec.Failure || outcome.Kind == finalizationcodec.Cancelled {
			// Recovering active tasks may re-enter running only within this same
			// atomic transaction. No intermediate status is externally visible and
			// the already inserted intent forbids any new execution authorization.
			if _, err := tx.ExecContext(ctx, `UPDATE prepare_tasks SET status='running' WHERE job_id=? AND status='recovery_required'`, jobID); err != nil {
				return FinalizationSnapshot{}, err
			}
			if _, err := tx.ExecContext(ctx, `UPDATE prepare_tasks SET status='failed',finished_at=?,error_json=? WHERE job_id=? AND status='running'`, at, canonical, jobID); err != nil {
				return FinalizationSnapshot{}, err
			}
		}
		selected := FinalizationSnapshot{JobID: jobID, Outcome: outcome, CanonicalJSON: canonical, Digest: digest, CreatedAt: at}
		if err := f.commit(func(readCtx context.Context, check *sql.Tx) (bool, error) {
			observed, found, err := loadFinalization(readCtx, check, jobID)
			if err != nil || !found {
				return false, err
			}
			if !sameFinalizationIntent(observed, selected) || observed.InstancePin != selected.InstancePin || observed.ProjectionsRemovedAt != "" {
				return false, ErrLiquibaseFinalization
			}
			if err := validateFinalizationSelection(readCtx, check, jobID, outcome); err != nil {
				return false, err
			}
			if outcome.Kind == finalizationcodec.Failure || outcome.Kind == finalizationcodec.Cancelled {
				var active int
				if err := check.QueryRowContext(readCtx, `SELECT count(*) FROM prepare_tasks WHERE job_id=? AND status IN ('running','recovery_required')`, jobID).Scan(&active); err != nil {
					return false, err
				}
				if active != 0 {
					return false, ErrLiquibaseFinalization
				}
			}
			selected = observed
			return true, nil
		}); err != nil {
			return FinalizationSnapshot{}, err
		}
		return selected, nil
	})
}

// validateFinalizationSelection uses only the supplied transaction. Resolved
// historical attempts/captures count too; a current stopped runner alone is
// never authority to dispose the complete job namespace.
func validateFinalizationSelection(ctx context.Context, tx *sql.Tx, jobID string, outcome finalizationcodec.Outcome) error {
	return validateFinalizationSelectionWithSession(ctx, tx, jobID, outcome, "", "")
}

// Only the atomic handoff publisher may exempt its exact pending private owner.
// Projection proof and terminal completion always use the strict wrapper above.
func validateFinalizationSelectionWithSession(ctx context.Context, tx *sql.Tx, jobID string, outcome finalizationcodec.Outcome, cloneID, sessionID string) error {
	var kind, purpose, status string
	var planOnly int
	if err := tx.QueryRowContext(ctx, `SELECT j.prepare_kind,j.purpose,j.status,j.plan_only
FROM prepare_jobs j JOIN liquibase_job_bindings b ON b.job_id=j.job_id WHERE j.job_id=?`, jobID).Scan(&kind, &purpose, &status, &planOnly); err != nil || kind != "lb" ||
		(status != "queued" && status != "running" && status != "recovery_required") {
		return ErrLiquibaseFinalization
	}
	request, cancelled, err := loadJobCancellation(ctx, tx, jobID)
	if err != nil || cancelled != (outcome.Kind == finalizationcodec.Cancelled) || cancelled && request.JobID != jobID {
		return ErrLiquibaseFinalization
	}
	allowedOperation := ""
	if outcome.Kind == finalizationcodec.PrepareSuccess {
		_, operation, err := verifyPrepareFinalizationReference(ctx, tx, jobID, outcome)
		if err != nil {
			return err
		}
		allowedOperation = operation.Ref
	}
	var unresolved, work int
	if err := tx.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM prepare_step_attempts WHERE job_id=?1 AND (phase<>'closed' OR resolution IS NULL))
+(SELECT count(*) FROM prepare_clones WHERE job_id=?1 AND phase<>'released' AND (?2='' OR clone_id IS NOT ?2))
+(SELECT count(*) FROM prepare_planning_attempt_artifacts WHERE job_id=?1 AND (executor_retired_at IS NULL OR protected_input_retired_at IS NULL OR output_retired_at IS NULL OR stderr_retired_at IS NULL OR manifest_retired_at IS NULL))
+(SELECT count(*) FROM prepare_liquibase_sessions WHERE job_id=?1 AND (?3='' OR session_id IS NOT ?3) AND (retired_at IS NULL OR runner_phase<>'retired' OR cursor_image_id IS NOT NULL OR cursor_state_id IS NOT NULL))
+(SELECT count(*) FROM prepare_liquibase_invocations WHERE job_id=?1 AND (?3='' OR session_id IS NOT ?3) AND retired_at IS NULL)
+(SELECT count(*) FROM prepare_liquibase_cache_restores WHERE job_id=?1 AND restore_phase NOT IN ('published','abandoned')),
(SELECT count(*) FROM prepare_tasks WHERE job_id=?1)+(SELECT count(*) FROM prepare_planning_intents WHERE job_id=?1)
+(SELECT count(*) FROM prepare_clones WHERE job_id=?1)+(SELECT count(*) FROM prepare_step_attempts WHERE job_id=?1)`, jobID, cloneID, sessionID).Scan(&unresolved, &work); err != nil || unresolved != 0 {
		return ErrLiquibaseFinalization
	}
	// Operations have exact engine-owned targets .../jobs/<job>/<operation>.
	// Enumerate instead of LIKE/GLOB: job IDs and paths are not SQL patterns,
	// and sibling jobs or earlier released assignments cannot be conflated.
	rows, err := tx.QueryContext(ctx, `SELECT operation_ref,target_path,stage,CASE WHEN `+jobOwnedOperationSQL("managed_runtime_operations", "?1")+` THEN 1 ELSE 0 END FROM managed_runtime_operations`, jobID)
	if err != nil {
		return ErrLiquibaseFinalization
	}
	for rows.Next() {
		var ref, target, stage string
		var owned int
		if err := rows.Scan(&ref, &target, &stage, &owned); err != nil {
			_ = rows.Close()
			return ErrLiquibaseFinalization
		}
		if owned != 0 {
			work++
			if stage != "retired" && ref != allowedOperation {
				_ = rows.Close()
				return ErrLiquibaseFinalization
			}
		}
	}
	streamErr := rows.Err()
	closeErr := rows.Close()
	if streamErr != nil || closeErr != nil {
		return ErrLiquibaseFinalization
	}
	switch outcome.Kind {
	case finalizationcodec.PrepareSuccess:
		if purpose != "prepare" || planOnly != 0 || outcome.EventProfile != finalizationcodec.StatusResult {
			return ErrLiquibaseFinalization
		}
	case finalizationcodec.Failure, finalizationcodec.Cancelled:
		want := finalizationcodec.Error
		if work != 0 {
			want = finalizationcodec.StatusError
		}
		if outcome.EventProfile != want || outcome.Kind == finalizationcodec.Cancelled && outcome.Problem.Code != "cancelled" {
			return ErrLiquibaseFinalization
		}
	case finalizationcodec.PlanSuccess, finalizationcodec.CacheExplainSuccess:
		plan, found, err := loadPlanningResult(ctx, tx, jobID)
		if err != nil || !found {
			return ErrLiquibaseFinalization
		}
		var pending int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM prepare_tasks WHERE job_id=? AND (task_id<>'plan' OR status<>'succeeded')`, jobID).Scan(&pending); err != nil || pending != 0 {
			return ErrLiquibaseFinalization
		}
		if outcome.Kind == finalizationcodec.PlanSuccess {
			var projection, digest, saved string
			if purpose != "prepare" || planOnly != 1 || outcome.Plan.PlanDigest != plan.PlanDigest {
				return ErrLiquibaseFinalization
			}
			if err := tx.QueryRowContext(ctx, `SELECT plan_projection_json,plan_projection_digest,plan_projection_saved_at FROM prepare_planning_intents WHERE job_id=?`, jobID).Scan(&projection, &digest, &saved); err != nil || saved == "" || !validPlanningProjection(projection) || planningProjectionDigest(projection) != digest || digest != outcome.Plan.ProjectionDigest {
				return ErrLiquibaseFinalization
			}
		} else {
			var decision, digest, saved string
			if purpose != "cache_explain" || outcome.Explanation.PlanDigest != plan.PlanDigest {
				return ErrLiquibaseFinalization
			}
			if err := tx.QueryRowContext(ctx, `SELECT cache_decision_json,cache_decision_digest,decision_saved_at FROM prepare_planning_intents WHERE job_id=?`, jobID).Scan(&decision, &digest, &saved); err != nil || saved == "" || digest != outcome.Explanation.DecisionDigest {
				return ErrLiquibaseFinalization
			}
			if _, err := planningcodec.DecodeDecision(decision, digest); err != nil {
				return ErrLiquibaseFinalization
			}
		}
	default:
		return ErrLiquibaseFinalization
	}
	return nil
}

// LiquibaseProjectionAbsenceVerifier inspects the exact owned namespace under
// exclusive job ownership. It must not consult sources/JDBC, invoke Liquibase or
// confuse an unavailable filesystem observation with physical absence.
type LiquibaseProjectionAbsenceVerifier interface {
	VerifyLiquibaseProjectionsAbsent(context.Context, FinalizationSnapshot) error
}

// sameFinalizationIntent validates the caller's typed snapshot too: changing a
// reference while retaining old canonical bytes/hash cannot authorize cleanup.
// Mutable proof/pin fields are reloaded, never trusted as retirement authority.
func sameFinalizationIntent(saved, candidate FinalizationSnapshot) bool {
	canonical, digest, err := finalizationcodec.Encode(candidate.Outcome)
	return err == nil && candidate.JobID == saved.JobID && candidate.CreatedAt == saved.CreatedAt &&
		canonical == candidate.CanonicalJSON && digest == candidate.Digest &&
		saved.CanonicalJSON == canonical && saved.Digest == digest &&
		saved.Outcome.Kind == candidate.Outcome.Kind && saved.Outcome.EventProfile == candidate.Outcome.EventProfile
}

// ConfirmProjectionsRemoved always performs a fresh physical check, even when
// an earlier process already saved the absence timestamp. The verifier runs
// outside SQL; the final short transaction rechecks the same immutable intent
// and retirement facts. Requirements: cleanup design, finalization step 5.
func (s *SQLiteStore) ConfirmProjectionsRemoved(ctx context.Context, intent FinalizationSnapshot, verifier LiquibaseProjectionAbsenceVerifier, at string) (FinalizationSnapshot, error) {
	if s == nil || s.db == nil || verifier == nil || !validFinalizationUTC(at) {
		return FinalizationSnapshot{}, ErrLiquibaseFinalization
	}
	saved, found, err := s.LoadFinalization(ctx, intent.JobID)
	if err != nil || !found || !sameFinalizationIntent(saved, intent) {
		return FinalizationSnapshot{}, errors.Join(ErrLiquibaseFinalization, ctx.Err())
	}
	if err := verifier.VerifyLiquibaseProjectionsAbsent(ctx, saved); err != nil {
		return FinalizationSnapshot{}, err
	}
	return withFinalization(ctx, s, func(f *finalizationConnection) (FinalizationSnapshot, error) {
		tx := f.tx
		saved, found, err = loadFinalization(ctx, tx, intent.JobID)
		if err != nil || !found || !sameFinalizationIntent(saved, intent) {
			return FinalizationSnapshot{}, ErrLiquibaseFinalization
		}
		if err := validateFinalizationSelection(ctx, tx, saved.JobID, saved.Outcome); err != nil {
			return FinalizationSnapshot{}, err
		}
		if saved.ProjectionsRemovedAt != "" {
			return saved, nil
		}
		if result, err := tx.ExecContext(ctx, `UPDATE prepare_liquibase_job_finalizations SET projections_removed_at=? WHERE job_id=? AND outcome_digest=? AND projections_removed_at IS NULL`, at, saved.JobID, saved.Digest); err != nil || oneRowChanged(result) != nil {
			return FinalizationSnapshot{}, errors.Join(ErrLiquibaseFinalization, err)
		}
		if err := f.commit(func(readCtx context.Context, check *sql.Tx) (bool, error) {
			observed, found, err := loadFinalization(readCtx, check, intent.JobID)
			if err != nil || !found {
				return false, err
			}
			if !sameFinalizationIntent(observed, intent) || observed.InstancePin != saved.InstancePin {
				return false, ErrLiquibaseFinalization
			}
			if observed.ProjectionsRemovedAt == "" {
				return false, nil
			}
			if observed.ProjectionsRemovedAt != at {
				return false, ErrLiquibaseFinalization
			}
			if err := validateFinalizationSelection(readCtx, check, observed.JobID, observed.Outcome); err != nil {
				return false, err
			}
			return true, nil
		}); err != nil {
			return FinalizationSnapshot{}, err
		}
		saved.ProjectionsRemovedAt = at
		return saved, nil
	})
}

// finalizationPublicResult exactly mirrors the existing prepare result shape;
// DSN is deliberately empty in durable managed-instance results/events. Access
// hydration remains in prepare and never enters this private journal.
type finalizationPublicResult struct {
	DSN                   string `json:"dsn"`
	InstanceID            string `json:"instance_id"`
	StateID               string `json:"state_id"`
	ImageID               string `json:"image_id"`
	PrepareKind           string `json:"prepare_kind"`
	PrepareArgsNormalized string `json:"prepare_args_normalized"`
}

type finalizationTerminalProjection struct {
	status, result, explanation, problem string
	events                               []EventRecord
}

// terminalProjection reconstructs public bytes from the immutable intent and
// saved job metadata only. It does not require a completed instance to remain
// forever: archived logical references must survive ordinary later deletion.
func terminalProjection(ctx context.Context, tx *sql.Tx, saved FinalizationSnapshot, at string) (finalizationTerminalProjection, error) {
	p := finalizationTerminalProjection{status: "succeeded"}
	switch saved.Outcome.Kind {
	case finalizationcodec.Failure, finalizationcodec.Cancelled:
		p.status, p.problem = "failed", saved.CanonicalJSON
	case finalizationcodec.CacheExplainSuccess:
		if err := tx.QueryRowContext(ctx, `SELECT cache_decision_json FROM prepare_planning_intents WHERE job_id=? AND cache_decision_digest=?`, saved.JobID, saved.Outcome.Explanation.DecisionDigest).Scan(&p.explanation); err != nil {
			return p, ErrLiquibaseFinalization
		}
	case finalizationcodec.PrepareSuccess:
		var image, args, kind string
		if err := tx.QueryRowContext(ctx, `SELECT coalesce(resolved_image_id,image_id),coalesce(prepare_args_normalized,''),prepare_kind FROM prepare_jobs WHERE job_id=?`, saved.JobID).Scan(&image, &args, &kind); err != nil || kind != "lb" {
			return p, ErrLiquibaseFinalization
		}
		// Closed scalar-string DTO: no unsupported values/custom marshalers.
		encoded, _ := json.Marshal(finalizationPublicResult{InstanceID: saved.Outcome.Prepare.InstanceID, StateID: saved.Outcome.Prepare.StateID, ImageID: image, PrepareKind: kind, PrepareArgsNormalized: args})
		p.result = string(encoded)
	case finalizationcodec.PlanSuccess:
	default:
		return p, ErrLiquibaseFinalization
	}
	profile := saved.Outcome.EventProfile
	if profile == finalizationcodec.Status || profile == finalizationcodec.StatusError || profile == finalizationcodec.StatusResult {
		p.events = append(p.events, EventRecord{JobID: saved.JobID, Type: "status", Ts: at, Status: &p.status})
	}
	if profile == finalizationcodec.Error || profile == finalizationcodec.StatusError {
		event := EventRecord{JobID: saved.JobID, Type: "error", Ts: at, ErrorJSON: &p.problem}
		if profile == finalizationcodec.Error {
			event.Status = &p.status
		}
		p.events = append(p.events, event)
	}
	if profile == finalizationcodec.Result || profile == finalizationcodec.StatusResult {
		payload := p.result
		var status *string
		if profile == finalizationcodec.Result {
			payload, status = p.explanation, &p.status
		}
		p.events = append(p.events, EventRecord{JobID: saved.JobID, Type: "result", Ts: at, Status: status, ResultJSON: &payload})
	}
	return p, nil
}

// CommitFinalization atomically publishes the chosen outcome/events, converts
// only this job's leases and clears its temporary pin. Reconciliation compares
// the original committed projection instead of extending finish/retention time.
// Caller holds job ownership after fresh absence confirmation; there is no I/O
// or nested connection in this transaction. See cleanup design, step 6.
func (s *SQLiteStore) CommitFinalization(ctx context.Context, intent FinalizationSnapshot, at, retainUntil string) error {
	if s == nil || s.db == nil || !validFinalizationUTC(at) || !validFinalizationUTC(retainUntil) {
		return ErrLiquibaseFinalization
	}
	finish, _ := time.Parse(time.RFC3339Nano, at)
	expires, _ := time.Parse(time.RFC3339Nano, retainUntil)
	if !expires.After(finish) {
		return ErrLiquibaseFinalization
	}
	_, err := withFinalization(ctx, s, func(f *finalizationConnection) (struct{}, error) {
		return struct{}{}, func() error {
			tx := f.tx
			saved, found, err := loadFinalization(ctx, tx, intent.JobID)
			if err != nil || !found || !sameFinalizationIntent(saved, intent) || saved.ProjectionsRemovedAt == "" || saved.ProjectionsRemovedAt != intent.ProjectionsRemovedAt {
				return ErrLiquibaseFinalization
			}
			var status string
			var previousFinished, previousResult, previousExplanation sql.NullString
			if err := tx.QueryRowContext(ctx, `SELECT status,finished_at,result_json,cache_explain_result_json FROM prepare_jobs WHERE job_id=?`, saved.JobID).Scan(&status, &previousFinished, &previousResult, &previousExplanation); err != nil {
				return ErrLiquibaseFinalization
			}
			if status == "succeeded" || status == "failed" {
				return verifyCommittedFinalization(ctx, tx, saved)
			}
			if previousFinished.Valid || previousResult.Valid || previousExplanation.Valid {
				return ErrLiquibaseFinalization
			}
			if err := validateFinalizationSelection(ctx, tx, saved.JobID, saved.Outcome); err != nil {
				return err
			}
			projection, err := terminalProjection(ctx, tx, saved, at)
			if err != nil {
				return err
			}
			// CS07/CS10: the exact selected outcome and confirmed projection absence
			// discharge only this choice's temporary ownership, before terminal status
			// in the SAME transaction. A later rejected write rolls the pin back. Do
			// not require an already terminal job here: that would deadlock its guard.
			if saved.Outcome.Kind == finalizationcodec.Failure || saved.Outcome.Kind == finalizationcodec.Cancelled {
				if _, err := tx.ExecContext(ctx, `UPDATE prepare_liquibase_cache_selections
SET released_at=?,release_outcome_digest=?
WHERE job_id=? AND transferred_at IS NULL AND released_at IS NULL`, at, saved.Digest, saved.JobID); err != nil {
					return err
				}
			}
			// The old recovery stop remains unchanged for every other transform. This
			// intermediate metadata value is private to the same atomic transaction;
			// intent guards prohibit new work and a failed terminal write rolls it back.
			if _, err := tx.ExecContext(ctx, `UPDATE prepare_jobs SET status='running' WHERE job_id=? AND status='recovery_required'`, saved.JobID); err != nil {
				return err
			}
			if result, err := tx.ExecContext(ctx, `UPDATE prepare_jobs SET status=?,finished_at=?,result_json=?,cache_explain_result_json=?,error_json=? WHERE job_id=? AND status IN ('queued','running')`,
				projection.status, at, nullableFinalizationText(projection.result), nullableFinalizationText(projection.explanation), nullableFinalizationText(projection.problem), saved.JobID); err != nil || oneRowChanged(result) != nil {
				return errors.Join(ErrLiquibaseFinalization, err)
			}
			if saved.Outcome.Kind == finalizationcodec.Failure {
				// §28 early selection intentionally retained active tasks. Discharge
				// only those tasks AFTER the matching failed-job write in this same
				// transaction. Existing late failures already changed their tasks; the
				// empty update preserves their original timestamps and prior history.
				// Do not broaden the global recovery-required terminal guard.
				if _, err := tx.ExecContext(ctx, `UPDATE prepare_tasks SET status='running' WHERE job_id=? AND status='recovery_required'`, saved.JobID); err != nil {
					return err
				}
				if _, err := tx.ExecContext(ctx, `UPDATE prepare_tasks SET status='failed',finished_at=?,error_json=? WHERE job_id=? AND status='running'`, at, saved.CanonicalJSON, saved.JobID); err != nil {
					return err
				}
			}
			for _, event := range projection.events {
				if _, err := tx.ExecContext(ctx, `INSERT INTO prepare_events(job_id,type,ts,status,result_json,error_json) VALUES(?,?,?,?,?,?)`,
					event.JobID, event.Type, event.Ts, event.Status, event.ResultJSON, event.ErrorJSON); err != nil {
					return err
				}
			}
			if _, err := tx.ExecContext(ctx, `UPDATE prepare_artifact_leases SET owner_kind='retention',expires_at=? WHERE owner_kind='job' AND owner_id=?`, retainUntil, saved.JobID); err != nil {
				return err
			}
			if saved.InstancePin != "" {
				if result, err := tx.ExecContext(ctx, `UPDATE prepare_liquibase_job_finalizations SET instance_pin=NULL WHERE job_id=? AND instance_pin=?`, saved.JobID, saved.InstancePin); err != nil || oneRowChanged(result) != nil {
					return ErrLiquibaseFinalization
				}
			}
			if err := verifyCommittedFinalization(ctx, tx, saved); err != nil {
				return err
			}
			return f.commit(func(readCtx context.Context, check *sql.Tx) (bool, error) {
				var observedStatus string
				if err := check.QueryRowContext(readCtx, `SELECT status FROM prepare_jobs WHERE job_id=?`, saved.JobID).Scan(&observedStatus); err != nil {
					return false, err
				}
				if observedStatus != "succeeded" && observedStatus != "failed" {
					return false, nil
				}
				if err := verifyCommittedFinalization(readCtx, check, saved); err != nil {
					return false, err
				}
				return true, nil
			})
		}()
	})
	return err
}

func nullableFinalizationText(value string) any {
	if value == "" {
		return nil
	}
	return value
}

// verifyCommittedFinalization validates the entire terminal set, not only a
// status flag. Extra/absent events, early lease conversion, a retained pin or a
// changed selected result cannot be mistaken for a successful lost-reply retry.
func verifyCommittedFinalization(ctx context.Context, tx *sql.Tx, intent FinalizationSnapshot) error {
	saved, found, err := loadFinalization(ctx, tx, intent.JobID)
	if err != nil || !found || !sameFinalizationIntent(saved, intent) || saved.ProjectionsRemovedAt == "" || saved.InstancePin != "" {
		return ErrLiquibaseFinalization
	}
	var status, at string
	var result, explanation, problem sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT status,finished_at,result_json,cache_explain_result_json,error_json FROM prepare_jobs WHERE job_id=?`, saved.JobID).Scan(&status, &at, &result, &explanation, &problem); err != nil || !validFinalizationUTC(at) {
		return ErrLiquibaseFinalization
	}
	projection, err := terminalProjection(ctx, tx, saved, at)
	if err != nil || status != projection.status || !sameFinalizationText(result, projection.result) || !sameFinalizationText(explanation, projection.explanation) || !sameFinalizationText(problem, projection.problem) {
		return ErrLiquibaseFinalization
	}
	var active int
	if saved.Outcome.Kind == finalizationcodec.Failure || saved.Outcome.Kind == finalizationcodec.Cancelled {
		// Reconciliation must not certify a terminal flag while an active task
		// remains. Queued suffix and already terminal task history stay intact.
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM prepare_tasks WHERE job_id=? AND status IN ('running','recovery_required')`, saved.JobID).Scan(&active); err != nil || active != 0 {
			return ErrLiquibaseFinalization
		}
	}
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM prepare_artifact_leases WHERE owner_kind='job' AND owner_id=?`, saved.JobID).Scan(&active); err != nil || active != 0 {
		return ErrLiquibaseFinalization
	}
	// Reconcile the whole terminal commit, including its release, not merely a
	// status flag after a lost reply. History preserves the original finish time
	// and immutable outcome digest; transferred choices retain their successor.
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM prepare_liquibase_cache_selections
WHERE job_id=? AND transferred_at IS NULL
AND (released_at IS NULL OR released_at IS NOT ? OR release_outcome_digest IS NOT ?)`, saved.JobID, at, saved.Digest).Scan(&active); err != nil || active != 0 {
		return ErrLiquibaseFinalization
	}
	rows, err := tx.QueryContext(ctx, `SELECT type,ts,status,result_json,error_json FROM prepare_events WHERE job_id=? AND task_id IS NULL AND (status IN ('succeeded','failed') OR (type IN ('result','error') AND ts=?)) ORDER BY seq`, saved.JobID, at)
	if err != nil {
		return ErrLiquibaseFinalization
	}
	defer rows.Close()
	index := 0
	for rows.Next() {
		var kind, timestamp string
		var eventStatus, eventResult, eventError sql.NullString
		if err := rows.Scan(&kind, &timestamp, &eventStatus, &eventResult, &eventError); err != nil || index >= len(projection.events) {
			return ErrLiquibaseFinalization
		}
		want := projection.events[index]
		if kind != want.Type || timestamp != want.Ts || !sameFinalizationNullable(eventStatus, want.Status) || !sameFinalizationNullable(eventResult, want.ResultJSON) || !sameFinalizationNullable(eventError, want.ErrorJSON) {
			return ErrLiquibaseFinalization
		}
		index++
	}
	if rows.Err() != nil || index != len(projection.events) {
		return ErrLiquibaseFinalization
	}
	return nil
}

func sameFinalizationNullable(value sql.NullString, expected *string) bool {
	return expected == nil && !value.Valid || expected != nil && value.Valid && value.String == *expected
}

func sameFinalizationText(value sql.NullString, expected string) bool {
	return value.Valid == (expected != "") && value.String == expected
}

// FinalizationSnapshot preserves the exact selected outcome and mutable local
// completion facts. It is private recovery data, not a public result or a
// retirement/deletion capability. Saved reference/retirement authority must be
// rechecked by the guarded writer/coordinator, not inferred from these bytes.
// See liquibase-local-projection-cleanup-design.md, finalization protocol.
type FinalizationSnapshot struct {
	JobID                string
	Outcome              finalizationcodec.Outcome
	CanonicalJSON        string
	Digest               string
	CreatedAt            string
	ProjectionsRemovedAt string
	InstancePin          string
}

// LoadFinalization distinguishes missing intent from unreadable/corrupt intent.
// It checks the version, admitted source binding, canonical payload/digest,
// duplicated logical references and nullable field shapes in one SQL snapshot.
// It performs no writes, physical absence check, dependency reads or effects.
// In particular, found=false does NOT authorize new work for a missing/terminal
// job, and a saved absence timestamp does NOT replace a fresh filesystem check.
// Archived logical references remain readable after ordinary instance deletion.
func (s *SQLiteStore) LoadFinalization(ctx context.Context, jobID string) (FinalizationSnapshot, bool, error) {
	if s == nil || s.db == nil || jobID == "" {
		return FinalizationSnapshot{}, false, ErrLiquibaseFinalization
	}
	return loadFinalization(ctx, s.db, jobID)
}

// loadFinalization shares validation with transaction-bound guarded writers.
// Never recursively acquire the store's only connection inside a transaction.
func loadFinalization(ctx context.Context, reader cancellationRowReader, jobID string) (FinalizationSnapshot, bool, error) {
	var result FinalizationSnapshot
	var revision int
	var kind finalizationcodec.Kind
	var profile finalizationcodec.EventProfile
	var instance, state, pin, removed, jobKind, binding sql.NullString
	err := reader.QueryRowContext(ctx, `SELECT f.job_id,f.format_revision,f.outcome_kind,f.event_profile,
f.instance_id,f.final_state_id,f.instance_pin,f.outcome_json,f.outcome_digest,f.created_at,
f.projections_removed_at,j.prepare_kind,b.job_id
FROM prepare_liquibase_job_finalizations f
LEFT JOIN prepare_jobs j ON j.job_id=f.job_id
LEFT JOIN liquibase_job_bindings b ON b.job_id=j.job_id WHERE f.job_id=?`, jobID).Scan(
		&result.JobID, &revision, &kind, &profile, &instance, &state, &pin,
		&result.CanonicalJSON, &result.Digest, &result.CreatedAt, &removed, &jobKind, &binding)
	if errors.Is(err, sql.ErrNoRows) {
		return FinalizationSnapshot{}, false, nil
	}
	if err != nil {
		// Scan conversion errors may contain the corrupt stored value. Do not
		// expose driver diagnostics or partial private outcome bytes upstream.
		if ctx.Err() != nil {
			return FinalizationSnapshot{}, false, ctx.Err()
		}
		return FinalizationSnapshot{}, false, ErrLiquibaseFinalization
	}
	if revision != 1 || result.JobID != jobID || !jobKind.Valid || jobKind.String != "lb" || !binding.Valid || binding.String != jobID ||
		!validFinalizationUTC(result.CreatedAt) || removed.Valid && !validFinalizationUTC(removed.String) {
		return FinalizationSnapshot{}, false, ErrLiquibaseFinalization
	}
	outcome, err := finalizationcodec.Decode(kind, profile, result.CanonicalJSON, result.Digest)
	if err != nil {
		return FinalizationSnapshot{}, false, ErrLiquibaseFinalization
	}
	if outcome.Prepare != nil {
		if !instance.Valid || instance.String != outcome.Prepare.InstanceID || !state.Valid || state.String != outcome.Prepare.StateID ||
			pin.Valid && pin.String != instance.String {
			return FinalizationSnapshot{}, false, ErrLiquibaseFinalization
		}
	} else if instance.Valid || state.Valid || pin.Valid {
		return FinalizationSnapshot{}, false, ErrLiquibaseFinalization
	}
	result.Outcome = outcome
	result.ProjectionsRemovedAt = removed.String
	result.InstancePin = pin.String
	return result, true, nil
}

// HasPublicationPin is the injected access/deletion fence lookup. Missing and
// unavailable/corrupt authority are distinct; an error never permits retirement.
// Immutable archived instance IDs without an active pin do not block deletion.
// Runtime authority uses current component format 2 only; v1 recognition belongs
// to the read-only managed-store cutover, not a live retirement fallback (JF02).
func (s *SQLiteStore) HasPublicationPin(ctx context.Context, instanceID string) (bool, error) {
	if s == nil || s.db == nil || instanceID == "" {
		return false, ErrLiquibaseFinalization
	}
	var count int
	var version int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*),coalesce(max(format_version),0) FROM prepare_liquibase_finalization_store_format WHERE slot=1`).Scan(&count, &version); err != nil || count != 1 || version != 2 {
		return false, ErrLiquibaseFinalization
	}
	var jobID string
	if err := s.db.QueryRowContext(ctx, `SELECT count(*),coalesce(min(job_id),'') FROM prepare_liquibase_job_finalizations WHERE instance_pin=?`, instanceID).Scan(&count, &jobID); err != nil {
		return false, ErrLiquibaseFinalization
	}
	if count == 0 {
		return false, nil
	}
	if count != 1 {
		return false, ErrLiquibaseFinalization
	}
	saved, found, err := s.LoadFinalization(ctx, jobID)
	if err != nil || !found || saved.InstancePin != instanceID || saved.Outcome.Prepare == nil || saved.Outcome.Prepare.InstanceID != instanceID {
		return false, ErrLiquibaseFinalization
	}
	return true, nil
}

// PublishInstanceAndBeginFinalization is the local managed-DB publication
// boundary. Caller holds the job gate THEN the instance-access activation fence;
// native password application/verification has finished before this callback.
// Intent/pin, instance/use-count and the actual prepare_instance task commit
// together. This method does no native I/O, event publication or lease release.
// Requirements: projection-cleanup design, protocol step 3, LC11.
func (s *SQLiteStore) PublishInstanceAndBeginFinalization(ctx context.Context, jobID string, entry store.InstanceCreate,
	binding managedidentity.RuntimeBinding, operation instanceaccess.Operation) (FinalizationSnapshot, error) {
	return s.publishInstanceAndBeginFinalization(ctx, jobID, entry, binding, operation, nil)
}

// PublishWorkingInstanceAndBeginFinalization atomically transfers the exact
// completed working database, never a newly materialized output clone. Caller
// holds the job and access activation fences. Handoff schema §2 / WH02–WH03.
func (s *SQLiteStore) PublishWorkingInstanceAndBeginFinalization(ctx context.Context, session LiquibaseSessionRecord, entry store.InstanceCreate,
	binding managedidentity.RuntimeBinding, operation instanceaccess.Operation) (FinalizationSnapshot, error) {
	if session.RunnerPhase != "stopped" || session.DBPhase != "running" || session.RetiredAt != nil || session.CursorKind != "state" ||
		session.CursorID != entry.StateID || operation.Source != session.StartID {
		return FinalizationSnapshot{}, ErrLiquibaseFinalization
	}
	return s.publishInstanceAndBeginFinalization(ctx, session.JobID, entry, binding, operation, &session)
}

func (s *SQLiteStore) publishInstanceAndBeginFinalization(ctx context.Context, jobID string, entry store.InstanceCreate,
	binding managedidentity.RuntimeBinding, operation instanceaccess.Operation, handoff *LiquibaseSessionRecord) (FinalizationSnapshot, error) {
	sum := sha256.Sum256([]byte("instance\x00" + jobID + "\x00"))
	if s == nil || s.db == nil || jobID == "" || entry.InstanceID != hex.EncodeToString(sum[:])[:32] || entry.StateID == "" ||
		!validFinalizationUTC(entry.CreatedAt) || entry.Status == nil || *entry.Status != store.InstanceStatusActive ||
		entry.RuntimeID == nil || entry.RuntimeDir == nil || *entry.RuntimeID != binding.RuntimeRef || binding.Validate() != nil ||
		operation.Ref == "" || (handoff == nil && operation.Source != entry.StateID) || operation.Target != *entry.RuntimeDir ||
		operation.PhysicalIdentity != binding.PhysicalIdentity || operation.Identity != binding.IdentityBinding {
		return FinalizationSnapshot{}, ErrLiquibaseFinalization
	}
	outcome := finalizationcodec.Outcome{Kind: finalizationcodec.PrepareSuccess, EventProfile: finalizationcodec.StatusResult,
		Prepare: &finalizationcodec.PrepareReference{InstanceID: entry.InstanceID, StateID: entry.StateID}}
	canonical, digest, err := finalizationcodec.Encode(outcome)
	if err != nil {
		return FinalizationSnapshot{}, ErrLiquibaseFinalization
	}
	return withFinalization(ctx, s, func(f *finalizationConnection) (FinalizationSnapshot, error) {
		tx := f.tx
		validate := func(ctx context.Context, tx *sql.Tx) error {
			if handoff != nil {
				if err := verifyWorkingInstanceTransfer(ctx, tx, *handoff, entry, binding, operation, digest); err != nil {
					return err
				}
				return validateFinalizationSelectionWithSession(ctx, tx, jobID, outcome, handoff.CloneID, handoff.SessionID)
			}
			return validateFinalizationSelection(ctx, tx, jobID, outcome)
		}
		if saved, found, err := loadFinalization(ctx, tx, jobID); err != nil {
			return FinalizationSnapshot{}, err
		} else if found {
			if saved.CanonicalJSON != canonical || saved.Digest != digest {
				return FinalizationSnapshot{}, ErrLiquibaseFinalization
			}
			if err := validate(ctx, tx); err != nil {
				return FinalizationSnapshot{}, err
			}
			verified, op, err := verifyPrepareFinalizationReference(ctx, tx, jobID, outcome)
			if err != nil || verified != binding || op.Ref != operation.Ref || op.Target != operation.Target || op.Source != operation.Source || op.PhysicalIdentity != operation.PhysicalIdentity || op.Identity != operation.Identity {
				return FinalizationSnapshot{}, ErrLiquibaseFinalization
			}
			var image string
			var expires sql.NullString
			if err := tx.QueryRowContext(ctx, `SELECT image_id,expires_at FROM instances WHERE instance_id=?`, entry.InstanceID).Scan(&image, &expires); err != nil || image != entry.ImageID || expires.Valid != (entry.ExpiresAt != nil) || entry.ExpiresAt != nil && expires.String != *entry.ExpiresAt {
				return FinalizationSnapshot{}, ErrLiquibaseFinalization
			}
			return saved, nil
		}
		// The FK is deliberately deferred: the pin is recorded BEFORE publication,
		// and neither half can become visible through another SQLite connection.
		var previousUses int64
		if err := tx.QueryRowContext(ctx, `SELECT coalesce(use_count,0) FROM states WHERE state_id=?`, entry.StateID).Scan(&previousUses); err != nil {
			return FinalizationSnapshot{}, err
		}
		if handoff != nil {
			if err := writeWorkingInstanceTransfer(ctx, tx, *handoff, entry, binding, operation, digest); err != nil {
				return FinalizationSnapshot{}, err
			}
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO prepare_liquibase_job_finalizations
(job_id,format_revision,outcome_kind,event_profile,instance_id,instance_pin,final_state_id,outcome_json,outcome_digest,created_at)
VALUES(?,1,'prepare_success','status_result',?,?,?,?,?,?)`, jobID, entry.InstanceID, entry.InstanceID, entry.StateID, canonical, digest, entry.CreatedAt); err != nil {
			return FinalizationSnapshot{}, err
		}
		if err := storesqlite.InsertInstanceTx(ctx, tx, entry); err != nil {
			return FinalizationSnapshot{}, err
		}
		result, err := tx.ExecContext(ctx, `UPDATE prepare_tasks SET status='succeeded',finished_at=?,error_json=NULL
WHERE job_id=? AND task_id='prepare-instance' AND type='prepare_instance' AND status='running'
AND input_kind='state' AND input_id=?`, entry.CreatedAt, jobID, entry.StateID)
		if err != nil || oneRowChanged(result) != nil {
			return FinalizationSnapshot{}, ErrLiquibaseFinalization
		}
		if err := validate(ctx, tx); err != nil {
			return FinalizationSnapshot{}, err
		}
		verified, op, err := verifyPrepareFinalizationReference(ctx, tx, jobID, outcome)
		if err != nil || verified != binding || op.Ref != operation.Ref || op.Target != operation.Target || op.Source != operation.Source || op.PhysicalIdentity != operation.PhysicalIdentity || op.Identity != operation.Identity {
			return FinalizationSnapshot{}, ErrLiquibaseFinalization
		}
		intent := FinalizationSnapshot{JobID: jobID, Outcome: outcome, CanonicalJSON: canonical, Digest: digest, CreatedAt: entry.CreatedAt, InstancePin: entry.InstanceID}
		if err := f.commit(func(readCtx context.Context, check *sql.Tx) (bool, error) {
			observed, found, err := loadFinalization(readCtx, check, jobID)
			if err != nil || !found {
				return false, err
			}
			if !sameFinalizationIntent(observed, intent) || observed.InstancePin != entry.InstanceID || observed.ProjectionsRemovedAt != "" {
				return false, ErrLiquibaseFinalization
			}
			if err := validate(readCtx, check); err != nil {
				return false, err
			}
			verified, op, err := verifyPrepareFinalizationReference(readCtx, check, jobID, outcome)
			if err != nil || verified != binding || op.Ref != operation.Ref || op.Target != operation.Target || op.Source != operation.Source || op.PhysicalIdentity != operation.PhysicalIdentity || op.Identity != operation.Identity {
				return false, ErrLiquibaseFinalization
			}
			var image, created, lastUsed string
			var expires sql.NullString
			var uses int64
			if err := check.QueryRowContext(readCtx, `SELECT i.image_id,i.created_at,i.expires_at,coalesce(s.use_count,0),s.last_used_at FROM instances i JOIN states s ON s.state_id=i.state_id WHERE i.instance_id=?`, entry.InstanceID).Scan(&image, &created, &expires, &uses, &lastUsed); err != nil {
				return false, err
			}
			if image != entry.ImageID || created != entry.CreatedAt || expires.Valid != (entry.ExpiresAt != nil) || entry.ExpiresAt != nil && expires.String != *entry.ExpiresAt || uses != previousUses+1 || lastUsed != entry.CreatedAt {
				return false, ErrLiquibaseFinalization
			}
			intent = observed
			return true, nil
		}); err != nil {
			return FinalizationSnapshot{}, err
		}
		return intent, nil
	})
}

// verifyPrepareFinalizationReference consumes only the supplied transaction.
// A current running user-instance operation is the ONLY non-retired operation
// allowed during successful cleanup; no other job-owned runtime is exempted.
func verifyPrepareFinalizationReference(ctx context.Context, tx *sql.Tx, jobID string, outcome finalizationcodec.Outcome) (managedidentity.RuntimeBinding, instanceaccess.Operation, error) {
	var none managedidentity.RuntimeBinding
	var op instanceaccess.Operation
	if outcome.Prepare == nil {
		return none, op, ErrLiquibaseFinalization
	}
	var raw, secretRef, stage, domain, runtimeID, runtimeDir, identityJSON string
	var image, lineage, identity string
	var eligible int
	err := tx.QueryRowContext(ctx, `SELECT i.runtime_id,i.runtime_dir,a.binding_json,a.secret_ref,a.stage,f.domain_ref,
o.operation_ref,o.target_path,o.source_ref,o.identity_json,o.physical_identity,o.runtime_ref,o.stage,
j.resolved_image_id,j.lineage_ref,j.identity_digest
FROM prepare_jobs j JOIN liquibase_job_bindings jb ON jb.job_id=j.job_id
JOIN instances i ON i.instance_id=? AND i.state_id=? AND i.status='active'
JOIN states st ON st.state_id=i.state_id AND st.prepare_kind='lb' AND st.image_id=i.image_id
JOIN liquibase_state_bindings lb ON lb.state_id=st.state_id AND lb.execution_fingerprint=jb.execution_fingerprint
JOIN instance_access a ON a.instance_ref=i.instance_id
JOIN managed_runtime_operations o ON o.runtime_ref=i.runtime_id AND o.target_path=i.runtime_dir
AND (o.source_ref=i.state_id OR EXISTS (
 SELECT 1 FROM prepare_liquibase_session_disposals d JOIN prepare_liquibase_sessions s ON s.session_id=d.session_id
 JOIN prepare_clones c ON c.clone_id=s.clone_id AND c.job_id=s.job_id
 WHERE d.reason='instance_transferred' AND d.handoff_job_id=j.job_id AND s.job_id=j.job_id
 AND d.handoff_instance_id=i.instance_id AND d.handoff_state_id=i.state_id
 AND d.handoff_operation_ref=o.operation_ref AND d.handoff_runtime_dir=o.target_path
 AND d.db_runtime_id=i.runtime_id AND c.db_runtime_id=i.runtime_id AND d.runner_id=s.runner_id
 AND o.source_ref=s.start_id))
JOIN managed_store_format f ON f.slot=1
JOIN prepare_liquibase_job_finalizations selected ON selected.job_id=j.job_id AND selected.instance_pin=i.instance_id
AND (o.source_ref=i.state_id OR EXISTS (SELECT 1 FROM prepare_liquibase_session_disposals d
 WHERE d.handoff_job_id=j.job_id AND d.reason='instance_transferred'
 AND d.handoff_instance_id=i.instance_id AND d.handoff_outcome_digest=selected.outcome_digest))
WHERE j.job_id=? AND j.purpose='prepare' AND j.plan_only=0 AND j.prepare_kind='lb' AND j.status IN ('running','recovery_required')
AND i.image_id=j.resolved_image_id AND st.lineage_ref=j.lineage_ref AND st.identity_digest=j.identity_digest`,
		outcome.Prepare.InstanceID, outcome.Prepare.StateID, jobID).Scan(&runtimeID, &runtimeDir, &raw, &secretRef, &stage, &domain,
		&op.Ref, &op.Target, &op.Source, &identityJSON, &op.PhysicalIdentity, &op.RuntimeRef, &op.Stage, &image, &lineage, &identity)
	if err != nil {
		return none, op, ErrLiquibaseFinalization
	}
	var access instanceaccess.AccessBinding
	if json.Unmarshal([]byte(raw), &access) != nil || json.Unmarshal([]byte(identityJSON), &op.Identity) != nil || access.RuntimeBinding.Validate() != nil ||
		stage != "verified" || op.Stage != "running" || access.InstanceRef != outcome.Prepare.InstanceID || access.DomainRef != domain ||
		access.AccessVersion != op.PhysicalIdentity || access.PhysicalIdentity != op.PhysicalIdentity || access.RuntimeRef != runtimeID ||
		access.IdentityBinding != op.Identity || access.LineageRef != lineage || access.IdentityDigest != identity || runtimeDir != op.Target ||
		!filepath.IsAbs(op.Target) || filepath.Base(op.Target) != op.Ref || filepath.Base(filepath.Dir(filepath.Dir(op.Target))) != "jobs" {
		return none, op, ErrLiquibaseFinalization
	}
	if filepath.Base(filepath.Dir(op.Target)) != jobID && !planningDBPublicationTarget(ctx, tx, jobID, outcome.Prepare.InstanceID, op) {
		return none, op, ErrLiquibaseFinalization
	}
	secret := instanceaccess.SecretBinding{DomainRef: domain, OwnerRef: access.InstanceRef, IdentityDigest: identity, Version: access.PhysicalIdentity, Purpose: "instance"}
	if secret.Validate() != nil || secret.Ref() != secretRef || access.SecretRef != secretRef {
		return none, op, ErrLiquibaseFinalization
	}
	if _, found, err := loadPlanningResult(ctx, tx, jobID); err != nil || !found {
		return none, op, ErrLiquibaseFinalization
	}
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM prepare_tasks WHERE job_id=? AND task_id='prepare-instance'
AND type='prepare_instance' AND status='succeeded' AND input_kind='state' AND input_id=?`, jobID, outcome.Prepare.StateID).Scan(&eligible); err != nil || eligible != 1 {
		return none, op, ErrLiquibaseFinalization
	}
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM prepare_tasks WHERE job_id=? AND status<>'succeeded'`, jobID).Scan(&eligible); err != nil || eligible != 0 {
		return none, op, ErrLiquibaseFinalization
	}
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM prepare_tasks WHERE job_id=? AND task_id='plan' AND type='plan' AND status='succeeded'`, jobID).Scan(&eligible); err != nil || eligible != 1 {
		return none, op, ErrLiquibaseFinalization
	}
	return access.RuntimeBinding, op, nil
}
