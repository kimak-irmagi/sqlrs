package queue

// Normal-path journal operations from the approved Liquibase prototype.
// Installation and runtime activation are separate boundaries; these methods
// do not enable replay. See issue89-normal-path-storage-extraction.md.

import (
	"context"
	"fmt"
	"github.com/sqlrs/engine-local/internal/prepare/planningcodec"
)

// PlanningCaptureNames are job-private logical refs. Their host paths are
// resolved only by the trusted runtime, never supplied by the remote client.
type PlanningCaptureNames struct {
	ProtectedInput, Stdout, Stderr, Manifest string
	HelperDigest, ProtocolRevision           string
}

// ProjectLiquibasePlanningBinding derives the immutable generic identity of
// updateSQL from the already persisted Liquibase adapter binding. The helper
// digest and protocol are part of the command, so an engine upgrade cannot
// silently replace the executor. No credential or raw SQL enters this record.
// See P01/C01a in docs/architecture/liquibase-planning-recovery-tests.md.
func ProjectLiquibasePlanningBinding(record LiquibaseExecutionRecord, updateSQLArgs string, helperDigest string) (ExecutionBinding, error) {
	if !planningcodec.ValidDigest(record.ExecutionFingerprint) ||
		!planningcodec.ValidDigest(record.ImageManifestDigest) ||
		!planningcodec.ValidDigest(record.TargetIdentityDigest) ||
		!planningcodec.ValidDigest(record.BundleManifestDigest) ||
		!planningcodec.ValidDigest(record.EffectiveConfigDigest) ||
		record.PolicyRevision == "" || updateSQLArgs == "" ||
		!planningcodec.ValidDigest(helperDigest) {
		return ExecutionBinding{}, fmt.Errorf("complete saved Liquibase planning inputs are required")
	}
	binding := ExecutionBinding{
		PrepareKind: "lb",
		CommandDigest: commonLiquibaseDigest("common-lb-planning-command-v1",
			updateSQLArgs, record.EffectiveConfigDigest, helperDigest,
			planningcodec.CaptureProtocolRevision),
		InputManifestDigest:  record.BundleManifestDigest,
		ExecutorImageDigest:  record.ImageManifestDigest,
		TargetIdentityDigest: record.TargetIdentityDigest,
		PolicyRevision:       record.PolicyRevision,
		EffectScope:          "unknown",
		AdapterBindingDigest: record.ExecutionFingerprint,
	}
	binding.Digest = commonLiquibaseDigest("common-lb-planning-execution-v1",
		binding.PrepareKind, binding.CommandDigest, binding.InputManifestDigest,
		binding.ExecutorImageDigest, binding.TargetIdentityDigest,
		binding.PolicyRevision, binding.EffectScope, binding.AdapterBindingDigest)
	return binding, nil
}

// ActivatePlanningClone records the exact preview database ID after external
// materialization, but before the planned Liquibase executor may start. The
// common state-execute activation intentionally remains independent of the
// planning namespace. See P01/R01 in the planning recovery test design.
func (s *SQLiteStore) ActivatePlanningClone(ctx context.Context, attempt AttemptRecord, generation int, runtimeName, runtimeID string) error {
	if s == nil || s.db == nil || attempt.JobID == "" || attempt.TaskID != "plan" ||
		attempt.AttemptNo < 1 || attempt.CloneID == "" || attempt.ExecutionDigest == "" ||
		attempt.PlannedExecutorName == "" || attempt.LegacyUnbound || generation != attempt.WorkerGeneration ||
		runtimeName == "" || !dockerContainerID.MatchString(runtimeID) {
		return ErrPlanningSchema
	}
	result, err := s.db.ExecContext(ctx, `UPDATE prepare_clones
SET phase='active',db_runtime_id=?
WHERE clone_id=? AND job_id=? AND planned_db_runtime_name=?
AND db_runtime_id IS NULL AND phase='allocated'
AND EXISTS (SELECT 1 FROM prepare_step_attempts a
  JOIN prepare_jobs j ON j.job_id=a.job_id
  JOIN prepare_tasks t ON t.job_id=a.job_id AND t.task_id=a.task_id
  JOIN prepare_planning_intents i ON i.job_id=a.job_id AND i.task_id=a.task_id
    AND i.execution_digest=a.execution_digest
    AND (i.preview_clone_id=a.clone_id OR EXISTS (
      SELECT 1 FROM prepare_attempt_decisions d
      WHERE d.job_id=a.job_id AND d.task_id=a.task_id
        AND d.attempt_no=a.attempt_no-1 AND d.next_attempt_no=a.attempt_no
        AND d.target_clone_id=a.clone_id AND d.choice='replay_fresh'))
  WHERE a.job_id=prepare_clones.job_id AND a.clone_id=prepare_clones.clone_id
    AND a.task_id='plan' AND a.attempt_no=? AND a.execution_digest=?
    AND a.planned_executor_name=? AND a.worker_generation=?
    AND a.legacy_unbound=0 AND a.phase='planned' AND a.resolution IS NULL
    AND j.status='running' AND t.type='plan' AND t.status='running')`,
		runtimeID, attempt.CloneID, attempt.JobID, runtimeName,
		attempt.AttemptNo, attempt.ExecutionDigest,
		attempt.PlannedExecutorName, generation)
	if err != nil || oneRowChanged(result) != nil {
		return ErrPlanningSchema
	}
	return nil
}

// PlanningLaunch combines the common exact attempt with its protected output
// identities. All rows must commit before preview clone or executor creation.
type PlanningLaunch struct {
	BoundLaunch
	Capture PlanningCaptureNames
}

// PlanBoundPlanningAttempt reserves the stable plan task, execution binding,
// preview clone, artifact leases, intent, attempt, and capture refs in one
// transaction. See P01 in the planning recovery test design.
func (s *SQLiteStore) PlanBoundPlanningAttempt(ctx context.Context, plan PlanningLaunch) (AttemptRecord, error) {
	launch := plan.BoundLaunch
	if s == nil || s.db == nil || launch.Attempt.JobID == "" || launch.Attempt.TaskID != "plan" ||
		launch.Attempt.CloneID != launch.Clone.ID || launch.Attempt.ExecutionDigest != launch.Binding.Digest ||
		launch.Attempt.CreatedAt == "" || launch.Attempt.WorkerGeneration < 0 ||
		launch.Clone.Generation != 1 || launch.Clone.ID == "" || launch.Clone.Ref == "" || launch.Clone.PlannedRuntimeName == "" ||
		launch.Binding.PrepareKind != "lb" || launch.Binding.PolicyRevision == "" ||
		launch.Clone.SourceKind != "image" && launch.Clone.SourceKind != "state" ||
		plan.Capture.ProtectedInput == "" || plan.Capture.Stdout == "" || plan.Capture.Stderr == "" || plan.Capture.Manifest == "" ||
		plan.Capture.ProtectedInput == plan.Capture.Stdout || plan.Capture.ProtectedInput == plan.Capture.Stderr || plan.Capture.ProtectedInput == plan.Capture.Manifest ||
		plan.Capture.Stdout == plan.Capture.Stderr || plan.Capture.Stdout == plan.Capture.Manifest || plan.Capture.Stderr == plan.Capture.Manifest ||
		!planningcodec.ValidDigest(plan.Capture.HelperDigest) || plan.Capture.ProtocolRevision != planningcodec.CaptureProtocolRevision {
		return AttemptRecord{}, ErrPlanningSchema
	}
	if launch.Clone.SourceKind == "image" && (launch.Clone.SourceImageID == "" || launch.Clone.SourceStateID != "") ||
		launch.Clone.SourceKind == "state" && (launch.Clone.SourceStateID == "" || launch.Clone.SourceImageID != "") {
		return AttemptRecord{}, ErrPlanningSchema
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return AttemptRecord{}, err
	}
	defer tx.Rollback()
	var eligible, existing int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM prepare_jobs j JOIN liquibase_job_bindings jb ON jb.job_id=j.job_id WHERE j.job_id=? AND j.status='running' AND j.prepare_kind='lb' AND j.purpose IN ('prepare','cache_explain') AND (j.purpose<>'cache_explain' OR j.plan_only=1)`, launch.Attempt.JobID).Scan(&eligible); err != nil || eligible != 1 {
		return AttemptRecord{}, ErrPlanningSchema
	}
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM prepare_tasks WHERE job_id=?`, launch.Attempt.JobID).Scan(&existing); err != nil || existing != 0 {
		return AttemptRecord{}, ErrPlanningSchema
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO prepare_tasks(job_id,task_id,position,type,status,planner_kind) VALUES(?,'plan',0,'plan','running','lb')`, launch.Attempt.JobID); err != nil {
		return AttemptRecord{}, err
	}
	partial := 0
	if launch.Binding.PartialPrefixSafe {
		partial = 1
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO prepare_execution_bindings(execution_digest,prepare_kind,command_digest,input_manifest_digest,executor_image_digest,target_identity_digest,policy_revision,effect_scope,partial_prefix_safe,enforcement_digest,adapter_binding_digest,created_at) VALUES(?,?,?,?,?,?,?,?,?,?,?,?) ON CONFLICT(execution_digest) DO NOTHING`,
		launch.Binding.Digest, launch.Binding.PrepareKind, launch.Binding.CommandDigest, launch.Binding.InputManifestDigest, launch.Binding.ExecutorImageDigest, launch.Binding.TargetIdentityDigest, launch.Binding.PolicyRevision, launch.Binding.EffectScope, partial, nullIfEmpty(launch.Binding.EnforcementDigest), nullIfEmpty(launch.Binding.AdapterBindingDigest), launch.Attempt.CreatedAt); err != nil {
		return AttemptRecord{}, err
	}
	if err := verifyExecutionBinding(ctx, tx, launch.Binding, partial); err != nil {
		return AttemptRecord{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO prepare_step_bindings(job_id,task_id,execution_digest,bound_at) VALUES(?,'plan',?,?)`, launch.Attempt.JobID, launch.Binding.Digest, launch.Attempt.CreatedAt); err != nil {
		return AttemptRecord{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO prepare_clones(clone_id,job_id,clone_generation,source_kind,source_image_id,source_state_id,clone_ref,planned_db_runtime_name,phase,created_at) VALUES(?,?,?,?,?,?,?,?,'allocated',?)`,
		launch.Clone.ID, launch.Attempt.JobID, 1, launch.Clone.SourceKind, nullIfEmpty(launch.Clone.SourceImageID), nullIfEmpty(launch.Clone.SourceStateID), launch.Clone.Ref, launch.Clone.PlannedRuntimeName, launch.Attempt.CreatedAt); err != nil {
		return AttemptRecord{}, err
	}
	helperLeased := false
	for _, lease := range launch.Leases {
		if lease.ID == "" || lease.Kind == "" || !planningcodec.ValidDigest(lease.Digest) {
			return AttemptRecord{}, ErrPlanningSchema
		}
		if lease.Kind == "capture_helper" && lease.Digest == plan.Capture.HelperDigest {
			helperLeased = true
		}
		if _, err := tx.ExecContext(ctx, `INSERT INTO prepare_artifact_leases(lease_id,artifact_kind,artifact_digest,owner_kind,owner_id,created_at) VALUES(?,?,?,'job',?,?)`, lease.ID, lease.Kind, lease.Digest, launch.Attempt.JobID, launch.Attempt.CreatedAt); err != nil {
			return AttemptRecord{}, err
		}
	}
	if !helperLeased {
		return AttemptRecord{}, ErrPlanningSchema
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO prepare_planning_intents(job_id,task_id,execution_digest,preview_clone_id,plan_codec_revision,created_at) VALUES(?,'plan',?,?,?,?)`, launch.Attempt.JobID, launch.Binding.Digest, launch.Clone.ID, planningcodec.PlanRevision, launch.Attempt.CreatedAt); err != nil {
		return AttemptRecord{}, err
	}
	name := commonAttemptName(launch.Attempt.JobID, "plan", 1)
	if _, err := tx.ExecContext(ctx, `INSERT INTO prepare_step_attempts(job_id,task_id,attempt_no,clone_id,execution_digest,worker_generation,planned_executor_name,phase,legacy_unbound,created_at) VALUES(?,'plan',1,?,?,?,?, 'planned',0,?)`, launch.Attempt.JobID, launch.Clone.ID, launch.Binding.Digest, launch.Attempt.WorkerGeneration, name, launch.Attempt.CreatedAt); err != nil {
		return AttemptRecord{}, err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO prepare_planning_attempt_artifacts(job_id,task_id,attempt_no,protected_input_ref,complete_output_ref,complete_stderr_ref,capture_manifest_ref,capture_helper_digest,capture_protocol_revision,capture_location) VALUES(?,'plan',1,?,?,?,?,?,?,'container_private')`, launch.Attempt.JobID, plan.Capture.ProtectedInput, plan.Capture.Stdout, plan.Capture.Stderr, plan.Capture.Manifest, plan.Capture.HelperDigest, plan.Capture.ProtocolRevision); err != nil {
		return AttemptRecord{}, err
	}
	if err := tx.Commit(); err != nil {
		return AttemptRecord{}, err
	}
	return AttemptRecord{AttemptPlan: launch.Attempt, AttemptNo: 1, PlannedExecutorName: name, Phase: "planned"}, nil
}
