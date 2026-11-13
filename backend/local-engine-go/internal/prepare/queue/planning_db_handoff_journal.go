package queue

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"sort"
	"strings"

	"github.com/sqlrs/engine-local/internal/instanceaccess"
	"github.com/sqlrs/engine-local/internal/managedidentity"
)

// planningDBRetention binds the physical allocation to the immutable planner
// scope. It is live ownership evidence, never quiescence or absence.
// Requirements: issue89-planning-db-handoff-schema.md §§2–3.
type planningDBRetention struct {
	CloneGeneration  int                             `json:"clone_generation"`
	Fingerprint      string                          `json:"execution_fingerprint"`
	RuntimeName      string                          `json:"runtime_name"`
	RuntimeID        string                          `json:"runtime_id"`
	OperationRef     string                          `json:"operation_ref"`
	Target           string                          `json:"target_path"`
	SourceKind       string                          `json:"source_kind"`
	SourceID         string                          `json:"source_id"`
	PhysicalIdentity string                          `json:"physical_identity"`
	Identity         managedidentity.IdentityBinding `json:"identity"`
}

func planningDBSource(c ClonePlan) string {
	if c.SourceKind == "state" {
		return c.SourceStateID
	}
	return c.SourceImageID
}

func planningDBRetentionFor(p PlanningRecoverySnapshot, op instanceaccess.Operation) planningDBRetention {
	return planningDBRetention{p.Clone.Generation, p.Binding.AdapterBindingDigest, p.Clone.PlannedRuntimeName, p.CloneRuntimeID, op.Ref, op.Target, p.Clone.SourceKind, planningDBSource(p.Clone), op.PhysicalIdentity, op.Identity}
}

func readPlanningDBOperation(ctx context.Context, tx *sql.Tx, ref string) (instanceaccess.Operation, error) {
	var op instanceaccess.Operation
	var identity string
	err := tx.QueryRowContext(ctx, `SELECT operation_ref,target_path,source_ref,physical_identity,runtime_ref,stage,identity_json FROM managed_runtime_operations WHERE operation_ref=?`, ref).Scan(&op.Ref, &op.Target, &op.Source, &op.PhysicalIdentity, &op.RuntimeRef, &op.Stage, &identity)
	if err != nil {
		return op, err
	}
	if json.Unmarshal([]byte(identity), &op.Identity) != nil || op.Identity.Validate() != nil {
		return op, ErrPlanningSchema
	}
	return op, nil
}

// validatePlanningDBIdentity rechecks the full persisted tuple on the writer's
// connection after the external observer has returned. Caller phase strings do
// not substitute for the actual stopped successful planner or live operation.
func validatePlanningDBIdentity(ctx context.Context, tx *sql.Tx, a AttemptRecord, generation int, expected instanceaccess.Operation) (PlanningRecoverySnapshot, error) {
	p, found, err := loadPlanningAttemptTx(ctx, tx, a.JobID, a.AttemptNo)
	if err != nil || !found || a.TaskID != "plan" || a.AttemptNo != 1 || generation != a.WorkerGeneration || p.Attempt.AttemptPlan != a.AttemptPlan || p.Attempt.ExecutorID != a.ExecutorID || p.Attempt.PlannedExecutorName != a.PlannedExecutorName || p.ClonePhase != "active" || p.Binding.PolicyRevision != "lb-container-v2" || p.Capture.ProtocolRevision != "lb-capture-v2" {
		return p, ErrPlanningSchema
	}
	op, err := readPlanningDBOperation(ctx, tx, expected.Ref)
	if err != nil || op != expected || op.Stage != "running" || op.RuntimeRef != p.CloneRuntimeID || op.Source != planningDBSource(p.Clone) || !filepath.IsAbs(op.Target) || !strings.HasSuffix(filepath.ToSlash(op.Target), "/"+filepath.ToSlash(p.Clone.Ref)) {
		return p, ErrPlanningSchema
	}
	return p, nil
}

func planningDBEligible(ctx context.Context, tx *sql.Tx, p PlanningRecoverySnapshot) error {
	var n int
	err := tx.QueryRowContext(ctx, `SELECT count(*) FROM prepare_jobs j JOIN liquibase_job_bindings b ON b.job_id=j.job_id
WHERE j.job_id=? AND j.prepare_kind='lb' AND j.purpose='prepare' AND j.plan_only=0 AND j.status='running'
AND b.execution_fingerprint=?
AND NOT EXISTS(SELECT 1 FROM prepare_liquibase_sessions WHERE job_id=j.job_id)
AND NOT EXISTS(SELECT 1 FROM prepare_step_attempts WHERE job_id=j.job_id AND task_id<>'plan')
AND NOT EXISTS(SELECT 1 FROM prepare_liquibase_job_cancellations WHERE job_id=j.job_id)
AND NOT EXISTS(SELECT 1 FROM prepare_liquibase_job_finalizations WHERE job_id=j.job_id)`, p.Attempt.JobID, p.Binding.AdapterBindingDigest).Scan(&n)
	if err != nil {
		return err
	}
	if n != 1 {
		return ErrPlanningSchema
	}
	return nil
}

// readPlanningDBFact validates scope and the digest of the exact stored bytes,
// including facts supplied through direct SQL rather than a trusted writer.
func readPlanningDBFact(ctx context.Context, tx *sql.Tx, a AttemptRecord, id int64, kind string) (json.RawMessage, error) {
	var raw, scope, digest string
	err := tx.QueryRowContext(ctx, `SELECT facts_json,scope_digest,facts_digest FROM prepare_attempt_evidence WHERE evidence_id=? AND job_id=? AND task_id=? AND attempt_no=? AND worker_generation=? AND kind=?`, id, a.JobID, a.TaskID, a.AttemptNo, a.WorkerGeneration, kind).Scan(&raw, &scope, &digest)
	if err != nil {
		return nil, err
	}
	var fact struct {
		CloneID     string          `json:"clone_id"`
		Execution   string          `json:"execution_digest"`
		Observation json.RawMessage `json:"observation"`
	}
	wantScope := commonEvidenceDigest(fmt.Sprintf("attempt-scope-v1\n%s\n%s\n%d\n%s\n%s", a.JobID, a.TaskID, a.AttemptNo, a.CloneID, a.ExecutionDigest))
	if digest != commonEvidenceDigest(raw) || scope != wantScope || json.Unmarshal([]byte(raw), &fact) != nil || fact.CloneID != a.CloneID || fact.Execution != a.ExecutionDigest || len(fact.Observation) == 0 {
		return nil, ErrPlanningSchema
	}
	return fact.Observation, nil
}

// RecordPlanningDBRetention observes physical identity outside SQL, then saves
// one attempt-scoped live-owner fact after repeating the full metadata check.
// Requirements: issue89-planning-db-handoff-schema.md §§2–3; PG02.
func (s *SQLiteStore) RecordPlanningDBRetention(ctx context.Context, a AttemptRecord, generation int, op instanceaccess.Operation, observe func(context.Context, PlanningRecoverySnapshot, instanceaccess.Operation) error, at string) (int64, error) {
	if s == nil || s.db == nil || observe == nil || !validFinalizationUTC(at) || a.AttemptNo != 1 || generation != a.WorkerGeneration {
		return 0, ErrPlanningSchema
	}
	// The first metadata read owns no transaction during the trusted callback.
	p, found, err := s.LoadPlanningAttempt(ctx, a.JobID, a.AttemptNo)
	if err != nil || !found || p.Attempt.Phase != "observed_stopped" || p.Attempt.Resolution != "" {
		return 0, ErrPlanningSchema
	}
	if err := observe(ctx, p, op); err != nil {
		return 0, err
	}
	return withFinalization(ctx, s, func(f *finalizationConnection) (int64, error) {
		p, err := validatePlanningDBIdentity(ctx, f.tx, a, generation, op)
		if err != nil {
			return 0, err
		}
		if p.Attempt.Phase != "observed_stopped" || p.Attempt.Resolution != "" {
			return 0, ErrPlanningSchema
		}
		if err := planningDBEligible(ctx, f.tx, p); err != nil {
			return 0, err
		}
		var successful int
		if err := f.tx.QueryRowContext(ctx, `SELECT count(*) FROM prepare_attempt_evidence WHERE job_id=? AND task_id='plan' AND attempt_no=1 AND kind='executor_inspection' AND worker_generation=? AND json_extract(facts_json,'$.observation.executor_id')=? AND json_extract(facts_json,'$.observation.exit_code')=0`, a.JobID, generation, a.ExecutorID).Scan(&successful); err != nil || successful != 1 {
			return 0, ErrPlanningSchema
		}
		observation, _ := json.Marshal(planningDBRetentionFor(p, op))
		id, err := appendAttemptEvidenceTx(ctx, f.tx, a, generation, "planning_db_retained", observation, at)
		if err != nil {
			return 0, err
		}
		if err := commitPlanningDB(ctx, f, a.JobID); err != nil {
			return 0, err
		}
		return id, nil
	})
}

// planningDBSettlement reads complete rows, not a positive ownership count.
// All reconciliation is on the still-owned connection in a fresh read snapshot;
// no SQL application, physical cleanup or pooled read is retried (PG04).
func planningDBSettlement(ctx context.Context, tx *sql.Tx, jobID string) (string, error) {
	queries := []string{
		`SELECT * FROM prepare_liquibase_planning_db_ownership WHERE job_id=?`,
		`SELECT * FROM prepare_jobs WHERE job_id=?`, `SELECT * FROM prepare_tasks WHERE job_id=?`,
		`SELECT * FROM prepare_step_attempts WHERE job_id=?`, `SELECT * FROM prepare_clones WHERE job_id=?`,
		`SELECT * FROM prepare_planning_intents WHERE job_id=?`, `SELECT * FROM prepare_planning_changesets WHERE job_id=?`,
		`SELECT * FROM prepare_planning_attempt_artifacts WHERE job_id=?`, `SELECT * FROM prepare_attempt_evidence WHERE job_id=?`,
		`SELECT * FROM prepare_attempt_decisions WHERE job_id=?`,
		`SELECT l.* FROM prepare_decision_evidence l JOIN prepare_attempt_decisions d USING(decision_id) WHERE d.job_id=?`,
		`SELECT * FROM prepare_liquibase_sessions WHERE job_id=?`, `SELECT * FROM prepare_artifact_leases WHERE owner_id=?`,
		`SELECT op.* FROM managed_runtime_operations op JOIN prepare_clones c ON c.db_runtime_id=op.runtime_ref WHERE c.job_id=?`,
		`SELECT b.* FROM prepare_execution_bindings b JOIN prepare_step_bindings step USING(execution_digest) WHERE step.job_id=?`,
		`SELECT * FROM prepare_step_bindings WHERE job_id=?`,
	}
	var result [][]string
	for _, query := range queries {
		rows, err := tx.QueryContext(ctx, query, jobID)
		if err != nil {
			return "", err
		}
		columns, err := rows.Columns()
		if err != nil {
			rows.Close()
			return "", err
		}
		var values []string
		for rows.Next() {
			data := make([]any, len(columns))
			dest := make([]any, len(columns))
			for i := range data {
				dest[i] = &data[i]
			}
			if err := rows.Scan(dest...); err != nil {
				rows.Close()
				return "", err
			}
			encoded, err := json.Marshal(data)
			if err != nil {
				rows.Close()
				return "", err
			}
			values = append(values, string(encoded))
		}
		// SQLite has one result set: Next closes it on EOF and retains driver
		// close errors in Err. Cancellation also makes Err non-nil. CP2-R's
		// real-driver close-failure control preserves this refusal boundary.
		streamErr := rows.Err()
		_ = rows.Close()
		if streamErr != nil {
			return "", streamErr
		}
		sort.Strings(values)
		result = append(result, values)
	}
	encoded, err := json.Marshal(result)
	return string(encoded), err
}

func commitPlanningDB(ctx context.Context, f *finalizationConnection, jobID string) error {
	expected, err := planningDBSettlement(ctx, f.tx, jobID)
	if err != nil {
		return err
	}
	return f.commit(func(ctx context.Context, tx *sql.Tx) (bool, error) {
		actual, err := planningDBSettlement(ctx, tx, jobID)
		return actual == expected, err
	})
}

// CompleteRetainedPlanning commits the nonempty verified plan, exact live owner,
// three ordered evidence links and stable task completion as one effect.
// The legacy absence-based CompletePlanning path remains separate (PG02/PG04).
func (s *SQLiteStore) CompleteRetainedPlanning(ctx context.Context, a AttemptRecord, generation int, exitID, retainedID, outputID int64, at string) error {
	if s == nil || s.db == nil || a.TaskID != "plan" || a.AttemptNo != 1 || generation != a.WorkerGeneration || exitID <= 0 || retainedID <= exitID || outputID <= retainedID || !validFinalizationUTC(at) {
		return ErrPlanningSchema
	}
	_, err := withFinalization(ctx, s, func(f *finalizationConnection) (struct{}, error) {
		tx := f.tx
		plan, found, err := loadPlanningResult(ctx, tx, a.JobID)
		if err != nil || !found || plan.AttemptNo != 1 || plan.ExecutionDigest != a.ExecutionDigest || len(plan.Changesets) == 0 {
			return struct{}{}, ErrPlanningSchema
		}
		observation, err := readPlanningDBFact(ctx, tx, a, retainedID, "planning_db_retained")
		if err != nil {
			return struct{}{}, err
		}
		var fact planningDBRetention
		if json.Unmarshal(observation, &fact) != nil {
			return struct{}{}, ErrPlanningSchema
		}
		op, err := readPlanningDBOperation(ctx, tx, fact.OperationRef)
		if err != nil {
			return struct{}{}, err
		}
		p, err := validatePlanningDBIdentity(ctx, tx, a, generation, op)
		if err != nil {
			return struct{}{}, err
		}
		canonical, _ := json.Marshal(planningDBRetentionFor(p, op))
		if string(observation) != string(canonical) {
			return struct{}{}, ErrPlanningSchema
		}
		for _, proof := range []struct {
			id   int64
			kind string
		}{{exitID, "executor_inspection"}, {outputID, "planning_output"}} {
			if _, err := readPlanningDBFact(ctx, tx, a, proof.id, proof.kind); err != nil {
				return struct{}{}, err
			}
		}
		decisionID := fmt.Sprintf("planning-complete:%s:%d", a.JobID, a.AttemptNo)
		if p.Attempt.Phase == "closed" && p.Attempt.Resolution == "planning_result_saved" {
			var n int
			err := tx.QueryRowContext(ctx, `SELECT count(*) FROM prepare_liquibase_planning_db_ownership o JOIN prepare_tasks t ON t.job_id=o.job_id AND t.task_id='plan' WHERE o.job_id=? AND o.clone_id=? AND o.retained_evidence_id=? AND o.plan_digest=? AND t.status='succeeded'
AND (SELECT count(*) FROM prepare_decision_evidence WHERE decision_id=? AND evidence_id IN (?,?,?))=3
AND (SELECT count(*) FROM prepare_decision_evidence WHERE decision_id=?)=3`, a.JobID, a.CloneID, retainedID, plan.PlanDigest, decisionID, exitID, retainedID, outputID, decisionID).Scan(&n)
			if err != nil || n != 1 {
				return struct{}{}, ErrPlanningSchema
			}
			return struct{}{}, nil
		}
		if p.Attempt.Phase != "observed_stopped" || p.Attempt.Resolution != "" {
			return struct{}{}, ErrPlanningSchema
		}
		if err := planningDBEligible(ctx, tx, p); err != nil {
			return struct{}{}, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO prepare_liquibase_planning_db_ownership(job_id,task_id,attempt_no,clone_id,retained_evidence_id,plan_digest) VALUES(?,'plan',1,?,?,?)`, a.JobID, a.CloneID, retainedID, plan.PlanDigest); err != nil {
			return struct{}{}, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO prepare_attempt_decisions(decision_id,job_id,task_id,attempt_no,decision_no,choice,worker_generation,created_at) VALUES(?,?,'plan',1,1,'complete_planning',?,?)`, decisionID, a.JobID, generation, at); err != nil {
			return struct{}{}, err
		}
		for _, id := range []int64{exitID, retainedID, outputID} {
			if _, err := tx.ExecContext(ctx, `INSERT INTO prepare_decision_evidence(decision_id,evidence_id) VALUES(?,?)`, decisionID, id); err != nil {
				return struct{}{}, err
			}
		}
		if _, err := tx.ExecContext(ctx, `UPDATE prepare_step_attempts SET phase='closed',resolution='planning_result_saved' WHERE job_id=? AND task_id='plan' AND attempt_no=1`, a.JobID); err != nil {
			return struct{}{}, err
		}
		if _, err := tx.ExecContext(ctx, `UPDATE prepare_tasks SET status='succeeded',finished_at=? WHERE job_id=? AND task_id='plan'`, at, a.JobID); err != nil {
			return struct{}{}, err
		}
		return struct{}{}, commitPlanningDB(ctx, f, a.JobID)
	})
	return err
}
