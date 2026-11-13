package queue

import (
	"context"
	"database/sql"
	"encoding/json"
	"reflect"

	"github.com/sqlrs/engine-local/internal/instanceaccess"
)

// planningDBDisposalRequest records the existing reason-specific authority;
// reason text by itself never grants physical cleanup (schema §5, PG06).
type planningDBDisposalRequest struct {
	RetainedID     int64    `json:"retained_evidence_id"`
	PlanDigest     string   `json:"plan_digest"`
	Reason         string   `json:"reason"`
	Authority      string   `json:"authority"`
	FirstTask      string   `json:"first_task_id,omitempty"`
	FirstExecution string   `json:"first_execution_digest,omitempty"`
	InputKind      string   `json:"input_kind,omitempty"`
	InputID        string   `json:"input_id,omitempty"`
	CachedStates   []string `json:"cached_states,omitempty"`
}

func planningDBDisposalAuthority(ctx context.Context, tx *sql.Tx, o planningDBOwner, p PlanningRecoverySnapshot, reason string) (planningDBDisposalRequest, error) {
	want := planningDBDisposalRequest{RetainedID: o.RetainedID, PlanDigest: o.PlanDigest, Reason: reason}
	switch reason {
	case "accepted_cancel":
		_, found, err := loadJobCancellation(ctx, tx, o.JobID)
		if err != nil || !found {
			return want, ErrPlanningSchema
		}
		want.Authority = o.JobID
	case "selected_failure":
		selected, found, err := loadFinalization(ctx, tx, o.JobID)
		if err != nil || !found || selected.Outcome.Kind != "failure" {
			return want, ErrPlanningSchema
		}
		want.Authority = selected.Digest
	case "source_mismatch", "full_cache":
		if err := planningDBEligible(ctx, tx, p); err != nil {
			return want, err
		}
		tasks, err := readPlanningDBSuffix(ctx, tx, p)
		if err != nil || len(tasks) == 0 {
			return want, ErrPlanningSchema
		}
		want.Authority = o.PlanDigest
		for _, task := range tasks {
			if task.Status != "succeeded" {
				if reason != "source_mismatch" || task.Status != "running" || task.Cached != nil && *task.Cached || task.InputKind == nil || task.InputID == nil || *task.InputKind == p.Clone.SourceKind && *task.InputID == planningDBSource(p.Clone) {
					return want, ErrPlanningSchema
				}
				want.FirstTask, want.InputKind, want.InputID = task.TaskID, *task.InputKind, *task.InputID
				if err := tx.QueryRowContext(ctx, `SELECT step.execution_digest FROM prepare_step_bindings step JOIN prepare_execution_bindings b USING(execution_digest) WHERE step.job_id=? AND step.task_id=? AND b.adapter_binding_digest=? AND b.prepare_kind='lb' AND b.policy_revision='lb-container-v2'`, o.JobID, task.TaskID, p.Binding.AdapterBindingDigest).Scan(&want.FirstExecution); err != nil {
					return want, ErrPlanningSchema
				}
				return want, nil
			}
			if task.Cached == nil || !*task.Cached || task.OutputStateID == nil {
				return want, ErrPlanningSchema
			}
			var valid int
			err := tx.QueryRowContext(ctx, `SELECT count(*) FROM states state JOIN prepare_state_bindings binding ON binding.state_id=state.state_id
JOIN prepare_step_bindings step ON step.execution_digest=binding.execution_digest AND step.job_id=? AND step.task_id=?
JOIN liquibase_state_bindings lb ON lb.state_id=state.state_id JOIN prepare_jobs j ON j.job_id=step.job_id
WHERE state.state_id=? AND state.state_fingerprint=state.state_id AND state.prepare_kind='lb'
AND state.image_id=j.resolved_image_id AND coalesce(state.lineage_ref,'')=coalesce(j.lineage_ref,'') AND coalesce(state.identity_digest,'')=coalesce(j.identity_digest,'')
AND lb.execution_fingerprint=?`, o.JobID, task.TaskID, *task.OutputStateID, p.Binding.AdapterBindingDigest).Scan(&valid)
			if err != nil || valid != 1 {
				return want, ErrPlanningSchema
			}
			want.CachedStates = append(want.CachedStates, *task.OutputStateID)
		}
		if reason != "full_cache" {
			return want, ErrPlanningSchema
		}
	default:
		return want, ErrPlanningSchema
	}
	return want, nil
}

// RequestPlanningDBDisposal commits exact reason/authority before destruction.
// An unavailable COMMIT readback returns no evidence ID or cleanup permission.
// A later selected outcome cannot replace an already saved request (PG06).
func (s *SQLiteStore) RequestPlanningDBDisposal(ctx context.Context, a AttemptRecord, generation int, reason, at string) (int64, error) {
	if s == nil || s.db == nil || a.TaskID != "plan" || a.AttemptNo != 1 || generation != a.WorkerGeneration || !validFinalizationUTC(at) {
		return 0, ErrPlanningSchema
	}
	return withFinalization(ctx, s, func(f *finalizationConnection) (int64, error) {
		o, err := readPlanningDBOwner(ctx, f.tx, a.JobID)
		if err != nil {
			return 0, err
		}
		p, _, err := planningDBOwnerSnapshot(ctx, f.tx, o)
		if err != nil {
			return 0, err
		}
		if p.Attempt.AttemptPlan != a.AttemptPlan || p.Attempt.ExecutorID != a.ExecutorID || p.Attempt.PlannedExecutorName != a.PlannedExecutorName || o.Session.Valid || o.DisposedAt.Valid {
			return 0, ErrPlanningSchema
		}
		if o.Request.Valid {
			raw, err := readPlanningDBFact(ctx, f.tx, a, o.Request.Int64, "planning_db_disposal_requested")
			if err != nil {
				return 0, err
			}
			var saved planningDBDisposalRequest
			if json.Unmarshal(raw, &saved) != nil || saved.Reason != reason || saved.RetainedID != o.RetainedID || saved.PlanDigest != o.PlanDigest {
				return 0, ErrPlanningSchema
			}
			return o.Request.Int64, nil
		}
		want, err := planningDBDisposalAuthority(ctx, f.tx, o, p, reason)
		if err != nil {
			return 0, err
		}
		raw, _ := json.Marshal(want)
		id, err := appendAttemptEvidenceTx(ctx, f.tx, a, generation, "planning_db_disposal_requested", raw, at)
		if err != nil {
			return 0, err
		}
		if _, err := f.tx.ExecContext(ctx, `UPDATE prepare_liquibase_planning_db_ownership SET disposal_requested_evidence_id=? WHERE job_id=?`, id, a.JobID); err != nil {
			return 0, err
		}
		if err := commitPlanningDB(ctx, f, a.JobID); err != nil {
			return 0, err
		}
		return id, nil
	})
}

type planningDBDisposalProof struct {
	RetainedID      int64               `json:"retained_evidence_id"`
	RequestID       int64               `json:"request_evidence_id"`
	Database        planningDBRetention `json:"database"`
	RuntimeAbsent   bool                `json:"runtime_absent"`
	DirectoryAbsent bool                `json:"directory_absent"`
}

// CompletePlanningDBDisposal verifies full runtime AND clone-directory absence
// outside SQL, then atomically records the proof/time and releases that exact
// clone. A transferred historical planner cannot enter this path (PG04/06).
func (s *SQLiteStore) CompletePlanningDBDisposal(ctx context.Context, a AttemptRecord, generation int, expected instanceaccess.Operation, verify CloneAbsenceVerifier, at string) error {
	if s == nil || s.db == nil || verify == nil || a.TaskID != "plan" || a.AttemptNo != 1 || generation != a.WorkerGeneration || !validFinalizationUTC(at) {
		return ErrPlanningSchema
	}
	read := func(ctx context.Context, tx *sql.Tx) (planningDBOwner, PlanningRecoverySnapshot, instanceaccess.Operation, error) {
		o, err := readPlanningDBOwner(ctx, tx, a.JobID)
		if err != nil {
			return o, PlanningRecoverySnapshot{}, instanceaccess.Operation{}, err
		}
		p, op, err := planningDBOwnerSnapshot(ctx, tx, o)
		if err != nil || o.Session.Valid || !o.Request.Valid || op != expected || op.Stage != "retired" || p.Attempt.AttemptPlan != a.AttemptPlan || p.Attempt.ExecutorID != a.ExecutorID || p.Attempt.PlannedExecutorName != a.PlannedExecutorName {
			return o, p, op, ErrPlanningSchema
		}
		if _, err := readPlanningDBFact(ctx, tx, a, o.Request.Int64, "planning_db_disposal_requested"); err != nil {
			return o, p, op, err
		}
		return o, p, op, nil
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return err
	}
	o, p, op, readErr := read(ctx, tx)
	rollbackErr := tx.Rollback()
	if readErr != nil {
		return readErr
	}
	if rollbackErr != nil {
		return rollbackErr
	}
	if err := verify.VerifyReleasedClone(ctx, p.Clone, p.CloneRuntimeID); err != nil {
		return err
	}
	_, err = withFinalization(ctx, s, func(f *finalizationConnection) (struct{}, error) {
		current, saved, actual, err := read(ctx, f.tx)
		if err != nil {
			return struct{}{}, err
		}
		if !reflect.DeepEqual(current, o) || saved.Clone != p.Clone || actual != op {
			return struct{}{}, ErrPlanningSchema
		}
		proof := planningDBDisposalProof{o.RetainedID, o.Request.Int64, planningDBRetentionFor(saved, actual), true, true}
		raw, _ := json.Marshal(proof)
		if o.Disposal.Valid {
			existing, err := readPlanningDBFact(ctx, f.tx, a, o.Disposal.Int64, "planning_db_disposed")
			if err != nil || string(existing) != string(raw) || !o.DisposedAt.Valid || o.DisposedAt.String != at || saved.ClonePhase != "released" {
				return struct{}{}, ErrPlanningSchema
			}
			return struct{}{}, nil
		}
		if saved.ClonePhase != "active" {
			return struct{}{}, ErrPlanningSchema
		}
		id, err := appendAttemptEvidenceTx(ctx, f.tx, a, generation, "planning_db_disposed", raw, at)
		if err != nil {
			return struct{}{}, err
		}
		if _, err := f.tx.ExecContext(ctx, `UPDATE prepare_liquibase_planning_db_ownership SET disposal_evidence_id=?,disposed_at=? WHERE job_id=?`, id, at, a.JobID); err != nil {
			return struct{}{}, err
		}
		if _, err := f.tx.ExecContext(ctx, `UPDATE prepare_clones SET phase='released' WHERE clone_id=? AND job_id=? AND phase='active'`, o.CloneID, a.JobID); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, commitPlanningDB(ctx, f, a.JobID)
	})
	return err
}
