package queue

import (
	"context"
	"database/sql"
	"encoding/json"
	"github.com/sqlrs/engine-local/internal/instanceaccess"
	"path/filepath"
	"reflect"
)

// planningDBPublicationTarget admits the original planner directory only after
// its exact durable transfer to the published session. O5 retains the original
// path and operation; a directory name alone never grants publication rights.
// Requirements: issue89-planning-db-handoff-schema.md §§2–3 / PG07–PG09.
func planningDBPublicationTarget(ctx context.Context, tx *sql.Tx, jobID, instanceID string, operation instanceaccess.Operation) bool {
	if filepath.Base(filepath.Dir(operation.Target)) != jobID+"-plan" {
		return false
	}
	o, err := readPlanningDBOwner(ctx, tx, jobID)
	if err != nil || !o.Session.Valid || o.Request.Valid || o.DisposedAt.Valid {
		return false
	}
	p, op, err := planningDBOwnerSnapshot(ctx, tx, o)
	if err != nil || op != operation || op.Source != planningDBSource(p.Clone) {
		return false
	}
	var exact int
	err = tx.QueryRowContext(ctx, `SELECT count(*) FROM prepare_liquibase_sessions s
JOIN prepare_liquibase_session_disposals d USING(session_id)
WHERE s.session_id=? AND s.job_id=? AND s.clone_id=? AND s.worker_generation=?
AND s.start_kind=? AND s.start_id=? AND d.reason='instance_transferred'
AND d.handoff_job_id=s.job_id AND d.handoff_instance_id=?
AND d.handoff_operation_ref=? AND d.handoff_runtime_dir=? AND d.db_runtime_id=?`,
		o.Session.String, jobID, p.Clone.ID, p.Attempt.WorkerGeneration, p.Clone.SourceKind,
		planningDBSource(p.Clone), instanceID, op.Ref, op.Target, op.RuntimeRef).Scan(&exact)
	return err == nil && exact == 1
}

// planningDBOwner is the durable transition record. Nullable fields represent
// retained, transferred, disposal-pending and disposed states; no phase cache
// or second physical clone is introduced. Schema §1.
type planningDBOwner struct {
	JobID, TaskID, CloneID, PlanDigest         string
	AttemptNo                                  int
	RetainedID                                 int64
	Session, FirstTask, Execution, Transferred sql.NullString
	Request, Disposal                          sql.NullInt64
	DisposedAt                                 sql.NullString
}

func readPlanningDBOwner(ctx context.Context, tx *sql.Tx, jobID string) (planningDBOwner, error) {
	var o planningDBOwner
	err := tx.QueryRowContext(ctx, `SELECT job_id,task_id,attempt_no,clone_id,retained_evidence_id,plan_digest,session_id,first_task_id,first_execution_digest,transferred_at,disposal_requested_evidence_id,disposal_evidence_id,disposed_at FROM prepare_liquibase_planning_db_ownership WHERE job_id=?`, jobID).Scan(&o.JobID, &o.TaskID, &o.AttemptNo, &o.CloneID, &o.RetainedID, &o.PlanDigest, &o.Session, &o.FirstTask, &o.Execution, &o.Transferred, &o.Request, &o.Disposal, &o.DisposedAt)
	return o, err
}

func planningDBOwnerSnapshot(ctx context.Context, tx *sql.Tx, o planningDBOwner) (PlanningRecoverySnapshot, instanceaccess.Operation, error) {
	// Read through the owned transaction; historical references grant no effects.
	p, found, err := loadPlanningAttemptTx(ctx, tx, o.JobID, o.AttemptNo)
	if err != nil || !found || p.Clone.ID != o.CloneID || p.Attempt.Phase != "closed" || p.Attempt.Resolution != "planning_result_saved" {
		return p, instanceaccess.Operation{}, ErrPlanningSchema
	}
	plan, found, err := loadPlanningResult(ctx, tx, o.JobID)
	if err != nil || !found || plan.PlanDigest != o.PlanDigest || len(plan.Changesets) == 0 {
		return p, instanceaccess.Operation{}, ErrPlanningSchema
	}
	raw, err := readPlanningDBFact(ctx, tx, p.Attempt, o.RetainedID, "planning_db_retained")
	if err != nil {
		return p, instanceaccess.Operation{}, err
	}
	var retained planningDBRetention
	if json.Unmarshal(raw, &retained) != nil {
		return p, instanceaccess.Operation{}, ErrPlanningSchema
	}
	op, err := readPlanningDBOperation(ctx, tx, retained.OperationRef)
	if err != nil {
		return p, instanceaccess.Operation{}, err
	}
	if retained != planningDBRetentionFor(p, op) || op.Source != planningDBSource(p.Clone) || op.RuntimeRef != p.CloneRuntimeID {
		return p, op, ErrPlanningSchema
	}
	return p, op, nil
}

// readPlanningDBSuffix checks the saved changeset sequence and input chain,
// including a real cached prefix. It does not grant any recovery launch permit.
func readPlanningDBSuffix(ctx context.Context, tx *sql.Tx, p PlanningRecoverySnapshot) ([]TaskRecord, error) {
	plan, found, err := loadPlanningResult(ctx, tx, p.Attempt.JobID)
	if err != nil || !found {
		return nil, ErrPlanningSchema
	}
	rows, err := tx.QueryContext(ctx, `SELECT job_id,task_id,position,type,status,planner_kind,input_kind,input_id,image_id,resolved_image_id,task_hash,output_state_id,cached,instance_mode,changeset_id,changeset_author,changeset_path,started_at,finished_at,error_json FROM prepare_tasks WHERE job_id=? ORDER BY position`, p.Attempt.JobID)
	if err != nil {
		return nil, err
	}
	var tasks []TaskRecord
	for rows.Next() {
		t, err := scanTask(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		if t.Type == "state_execute" {
			tasks = append(tasks, t)
		}
	}
	// With SQLite's single result set, exhausted Next has already closed Rows;
	// Err includes close failures. Keep the cancellation/error boundary (CP2-R).
	streamErr := rows.Err()
	_ = rows.Close()
	if streamErr != nil {
		return nil, streamErr
	}
	if len(tasks) != len(plan.Changesets) {
		return nil, ErrPlanningSchema
	}
	previous := planningDBSource(p.Clone)
	for i, t := range tasks {
		kind := "state"
		if i == 0 {
			kind = p.Clone.SourceKind
		}
		c := plan.Changesets[i]
		if t.InputKind == nil || *t.InputKind != kind || t.InputID == nil || *t.InputID != previous || t.OutputStateID == nil || t.TaskHash == nil || *t.TaskHash == "" || t.ChangesetID == nil || *t.ChangesetID != c.ID || t.ChangesetAuthor == nil || *t.ChangesetAuthor != c.Author || t.ChangesetPath == nil || *t.ChangesetPath != c.Path {
			return nil, ErrPlanningSchema
		}
		previous = *t.OutputStateID
	}
	return tasks, nil
}

// TransferPlanningDBToFirstSession atomically transfers one exact retained
// allocation to the true first application. It neither allocates another clone
// nor touches Docker/files; only a known commit may launch the runner (PG03/04).
func (s *SQLiteStore) TransferPlanningDBToFirstSession(ctx context.Context, l LiquibaseSessionLaunch) (LiquibaseSessionRecord, error) {
	want, c := l.Session, l.Clone
	if s == nil || s.db == nil || l.RequireFirstSessionProof || !sessionOpaqueRef.MatchString(want.SessionID) || want.JobID == "" || want.SessionNo != 1 || want.CloneID != c.ID || want.WorkerGeneration < 1 || !validFinalizationUTC(want.CreatedAt) || want.RunnerID != "" || want.RetiredAt != nil || !sessionOpaqueRef.MatchString(want.ProtectedConfigRef) || want.PlannedRunnerName == "" || !bindingDigest.MatchString(want.RunnerConfigDigest) || !bindingDigest.MatchString(want.CaptureHelperDigest) || l.HelperLeaseID == "" || l.FirstTaskID == "" || !bindingDigest.MatchString(l.ExecutionDigest) || c.SourceKind != want.StartKind || planningDBSource(c) != want.StartID {
		return LiquibaseSessionRecord{}, ErrPlanningSchema
	}
	want.RunnerPhase, want.DBPhase = "planned", "running"
	want.CursorKind, want.CursorID, want.CursorPosition = want.StartKind, want.StartID, want.StartPosition
	return withFinalization(ctx, s, func(f *finalizationConnection) (LiquibaseSessionRecord, error) {
		tx := f.tx
		o, err := readPlanningDBOwner(ctx, tx, want.JobID)
		if err != nil {
			return LiquibaseSessionRecord{}, err
		}
		p, op, err := planningDBOwnerSnapshot(ctx, tx, o)
		if err != nil {
			return LiquibaseSessionRecord{}, err
		}
		if p.Clone != c || p.ClonePhase != "active" || op.Stage != "running" || p.Attempt.WorkerGeneration != want.WorkerGeneration || p.Binding.AdapterBindingDigest != want.ExecutionFingerprint || o.Request.Valid || o.DisposedAt.Valid {
			return LiquibaseSessionRecord{}, ErrPlanningSchema
		}
		if o.Session.Valid {
			if o.Session.String != want.SessionID || o.FirstTask.String != l.FirstTaskID || o.Execution.String != l.ExecutionDigest || o.Transferred.String != want.CreatedAt {
				return LiquibaseSessionRecord{}, ErrPlanningSchema
			}
			var actual LiquibaseSessionRecord
			err := tx.QueryRowContext(ctx, `SELECT session_id,job_id,clone_id,session_no,worker_generation,execution_fingerprint,start_kind,start_id,start_position,planned_runner_name,runner_config_digest,capture_helper_digest,protected_config_ref,coalesce(runner_id,''),runner_phase,db_phase,CASE WHEN cursor_state_id IS NULL THEN 'image' ELSE 'state' END,coalesce(cursor_state_id,cursor_image_id,''),cursor_position,created_at FROM prepare_liquibase_sessions WHERE session_id=?`, want.SessionID).Scan(&actual.SessionID, &actual.JobID, &actual.CloneID, &actual.SessionNo, &actual.WorkerGeneration, &actual.ExecutionFingerprint, &actual.StartKind, &actual.StartID, &actual.StartPosition, &actual.PlannedRunnerName, &actual.RunnerConfigDigest, &actual.CaptureHelperDigest, &actual.ProtectedConfigRef, &actual.RunnerID, &actual.RunnerPhase, &actual.DBPhase, &actual.CursorKind, &actual.CursorID, &actual.CursorPosition, &actual.CreatedAt)
			if err != nil || !reflect.DeepEqual(actual, want) {
				return LiquibaseSessionRecord{}, ErrPlanningSchema
			}
			return actual, nil
		}
		if err := planningDBEligible(ctx, tx, p); err != nil {
			return LiquibaseSessionRecord{}, err
		}
		tasks, err := readPlanningDBSuffix(ctx, tx, p)
		if err != nil || len(tasks) == 0 {
			return LiquibaseSessionRecord{}, ErrPlanningSchema
		}
		first := tasks[0]
		if first.TaskID != l.FirstTaskID || first.Position != want.StartPosition || first.Status != "running" || first.Cached != nil && *first.Cached {
			return LiquibaseSessionRecord{}, ErrPlanningSchema
		}
		var bound int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM prepare_step_bindings step JOIN prepare_execution_bindings b USING(execution_digest) WHERE step.job_id=? AND step.task_id=? AND step.execution_digest=? AND b.prepare_kind='lb' AND b.policy_revision='lb-container-v2' AND b.adapter_binding_digest=?`, want.JobID, l.FirstTaskID, l.ExecutionDigest, want.ExecutionFingerprint).Scan(&bound); err != nil || bound != 1 {
			return LiquibaseSessionRecord{}, ErrPlanningSchema
		}
		if _, err := tx.ExecContext(ctx, `UPDATE prepare_liquibase_planning_db_ownership SET session_id=?,first_task_id=?,first_execution_digest=?,transferred_at=? WHERE job_id=?`, want.SessionID, l.FirstTaskID, l.ExecutionDigest, want.CreatedAt, want.JobID); err != nil {
			return LiquibaseSessionRecord{}, err
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO prepare_artifact_leases(lease_id,artifact_kind,artifact_digest,owner_kind,owner_id,created_at) VALUES(?,'capture_helper',?,'job',?,?) ON CONFLICT(artifact_kind,artifact_digest,owner_kind,owner_id) DO NOTHING`, l.HelperLeaseID, want.CaptureHelperDigest, want.JobID, want.CreatedAt); err != nil {
			return LiquibaseSessionRecord{}, err
		}
		var image, state any
		if want.StartKind == "image" {
			image = want.StartID
		} else {
			state = want.StartID
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO prepare_liquibase_sessions(session_id,job_id,session_no,clone_id,worker_generation,execution_fingerprint,start_kind,start_id,start_position,planned_runner_name,runner_config_digest,capture_helper_digest,protected_config_ref,runner_phase,db_phase,cursor_image_id,cursor_state_id,cursor_position,created_at,capture_location,capture_protocol_revision) VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,'planned','running',?,?,?,?,'container_private','lb-step-capture-v2')`, want.SessionID, want.JobID, want.SessionNo, want.CloneID, want.WorkerGeneration, want.ExecutionFingerprint, want.StartKind, want.StartID, want.StartPosition, want.PlannedRunnerName, want.RunnerConfigDigest, want.CaptureHelperDigest, want.ProtectedConfigRef, image, state, want.StartPosition, want.CreatedAt); err != nil {
			return LiquibaseSessionRecord{}, err
		}
		if err := commitPlanningDB(ctx, f, want.JobID); err != nil {
			return LiquibaseSessionRecord{}, err
		}
		return want, nil
	})
}
