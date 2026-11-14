package queue

// Normal-path journal operations from the approved Liquibase prototype.
// Installation and runtime activation are separate boundaries; these methods
// do not enable replay. See issue89-normal-path-storage-extraction.md.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/sqlrs/engine-local/internal/prepare/planningcodec"
	"strings"
)

const cancelledPlanningEvidence = "cancelled_planning_disposal"

// validateCancelledPlanningDisposal recognizes only the dedicated, hash-checked
// cancellation proof. Ordinary replay remains governed by its original facts.
func validateCancelledPlanningDisposal(ctx context.Context, tx *sql.Tx, saved PlanningRecoverySnapshot) (bool, error) {
	a := saved.Attempt
	var raw, digest, scope string
	err := tx.QueryRowContext(ctx, `SELECT facts_json,facts_digest,scope_digest FROM prepare_attempt_evidence WHERE job_id=? AND task_id=? AND attempt_no=? AND kind=?`, a.JobID, a.TaskID, a.AttemptNo, cancelledPlanningEvidence).Scan(&raw, &digest, &scope)
	if errors.Is(err, sql.ErrNoRows) {
		return false, nil
	}
	if saved.ClonePhase != "retained" && saved.ClonePhase != "released" {
		return false, ErrPlanningSchema
	}
	if err != nil || commonEvidenceDigest(raw) != digest || scope != commonEvidenceDigest(fmt.Sprintf("attempt-scope-v1\n%s\n%s\n%d\n%s\n%s", a.JobID, a.TaskID, a.AttemptNo, a.CloneID, a.ExecutionDigest)) {
		return false, ErrPlanningSchema
	}
	var valid int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM prepare_step_attempts a JOIN prepare_clones c ON c.clone_id=a.clone_id AND c.job_id=a.job_id WHERE a.job_id=? AND a.task_id=? AND a.attempt_no=? AND a.phase='closed' AND a.resolution='discarded_clone' AND `+cancelledPlanningProofSQL("a", "c"), a.JobID, a.TaskID, a.AttemptNo).Scan(&valid); err != nil || valid != 1 {
		return false, ErrPlanningSchema
	}
	return true, nil
}

// cancelledPlanningProofSQL is shared by the container guard and the recovery
// reader. Cancellation and a resolution label alone never confer authority.
// Its two aliases are compiled identifiers, not values supplied by a client.
func cancelledPlanningProofSQL(attempt, clone string) string {
	return strings.NewReplacer("$a", attempt, "$c", clone).Replace(`(
$a.executor_kind='container' AND $a.task_id='plan' AND $a.legacy_unbound=0
AND $a.attempt_no IN (1,2)
AND EXISTS (SELECT 1 FROM prepare_jobs j JOIN liquibase_job_bindings jb ON jb.job_id=j.job_id
 JOIN liquibase_execution_bindings lb ON lb.execution_fingerprint=jb.execution_fingerprint
 JOIN prepare_execution_bindings b ON b.execution_digest=$a.execution_digest
 WHERE j.job_id=$a.job_id AND j.prepare_kind='lb' AND j.purpose IN ('prepare','cache_explain')
 AND b.prepare_kind='lb' AND b.policy_revision='lb-container-v2'
 AND lb.execution_policy_revision=b.policy_revision AND lb.execution_fingerprint=b.adapter_binding_digest
 AND lb.bundle_manifest_digest=b.input_manifest_digest)
AND (SELECT count(*) FROM prepare_attempt_evidence unique_proof WHERE unique_proof.job_id=$a.job_id
 AND unique_proof.task_id=$a.task_id AND unique_proof.attempt_no=$a.attempt_no
 AND unique_proof.kind='cancelled_planning_disposal')=1
AND EXISTS (SELECT 1 FROM prepare_attempt_evidence p
 JOIN prepare_liquibase_job_cancellations request ON request.job_id=p.job_id
 WHERE p.job_id=$a.job_id AND p.task_id=$a.task_id AND p.attempt_no=$a.attempt_no
 AND p.worker_generation=$a.worker_generation AND p.kind='cancelled_planning_disposal'
 AND json_extract(p.facts_json,'$.clone_id')=$a.clone_id
 AND json_extract(p.facts_json,'$.execution_digest')=$a.execution_digest
 AND json_extract(p.facts_json,'$.observation.clone_generation')=$c.clone_generation
 AND json_extract(p.facts_json,'$.observation.executor_name')=$a.planned_executor_name
 AND json_extract(p.facts_json,'$.observation.executor_id')=coalesce($a.executor_id,'')
 AND json_extract(p.facts_json,'$.observation.runtime_name')=$c.planned_db_runtime_name
 AND json_extract(p.facts_json,'$.observation.runtime_id')=coalesce($c.db_runtime_id,'')
 AND json_extract(p.facts_json,'$.observation.requested_at')=request.requested_at
 AND json_extract(p.facts_json,'$.observation.finished_at')=$a.finished_at
 AND json_extract(p.facts_json,'$.observation.executor_absent')=1
 AND json_extract(p.facts_json,'$.observation.runtime_absent')=1
 AND ((json_extract(p.facts_json,'$.observation.phase')='planned' AND $a.executor_id IS NULL AND $a.started_at IS NULL
       AND ((json_extract(p.facts_json,'$.observation.clone_phase')='allocated' AND $c.db_runtime_id IS NULL)
         OR (json_extract(p.facts_json,'$.observation.clone_phase')='active' AND $c.db_runtime_id IS NOT NULL)))
   OR (json_extract(p.facts_json,'$.observation.phase')='created' AND $a.executor_id IS NOT NULL AND $a.started_at IS NULL
       AND $c.db_runtime_id IS NOT NULL AND json_extract(p.facts_json,'$.observation.clone_phase')='active')
   OR (json_extract(p.facts_json,'$.observation.phase')='observed_stopped'
       AND $a.executor_id IS NOT NULL AND $c.db_runtime_id IS NOT NULL
       AND json_extract(p.facts_json,'$.observation.clone_phase') IN ('active','quiescent')
       AND EXISTS (SELECT 1 FROM prepare_attempt_evidence exit
        WHERE exit.job_id=$a.job_id AND exit.task_id=$a.task_id AND exit.attempt_no=$a.attempt_no
        AND exit.worker_generation=$a.worker_generation AND exit.kind='executor_inspection'
        AND exit.evidence_id<p.evidence_id
        AND json_extract(exit.facts_json,'$.clone_id')=$a.clone_id
        AND json_extract(exit.facts_json,'$.execution_digest')=$a.execution_digest
        AND json_extract(exit.facts_json,'$.observation.executor_id')=$a.executor_id
        AND json_extract(exit.facts_json,'$.observation.exit_code') BETWEEN 0 AND 255
        AND (NOT EXISTS (SELECT 1 FROM prepare_attempt_evidence req
          WHERE req.job_id=$a.job_id AND req.task_id=$a.task_id AND req.attempt_no=$a.attempt_no
          AND req.kind IN ('termination_requested','termination_acknowledged'))
         OR EXISTS (SELECT 1 FROM prepare_attempt_evidence req JOIN prepare_attempt_evidence ack
          ON ack.job_id=req.job_id AND ack.task_id=req.task_id AND ack.attempt_no=req.attempt_no
          WHERE req.job_id=$a.job_id AND req.task_id=$a.task_id AND req.attempt_no=$a.attempt_no
          AND req.kind='termination_requested' AND ack.kind='termination_acknowledged'
          AND req.worker_generation=$a.worker_generation AND ack.worker_generation=$a.worker_generation
          AND req.evidence_id<ack.evidence_id AND ack.evidence_id<exit.evidence_id
          AND json_extract(req.facts_json,'$.observation.executor_id')=$a.executor_id
          AND json_extract(ack.facts_json,'$.observation.executor_id')=$a.executor_id)))))))`)
}

// PlanningRecoverySnapshot is the exact, persisted launch identity available
// to a worker after restart. It contains only logical protected refs, never
// captured SQL or credentials. See R01/P04 in the planning test design.
type PlanningRecoverySnapshot struct {
	Attempt        AttemptRecord
	Binding        ExecutionBinding
	Clone          ClonePlan
	Capture        PlanningCaptureNames
	ClonePhase     string
	CloneRuntimeID string
	PlanSaved      bool
	OutputComplete bool
	DecisionSaved  bool
	// CancelledDisposal is derived from validated immutable cancellation proof,
	// never from the resolution name alone. See cancelled-preview UP02/04.
	CancelledDisposal bool
}

// loadPlanningAttemptTx lets cancellation recheck the complete launch inside
// its short write transaction, without nesting a second store connection.
func loadPlanningAttemptTx(ctx context.Context, tx *sql.Tx, jobID string, attemptNo int) (PlanningRecoverySnapshot, bool, error) {
	var codec, purpose string
	var savedPlan, savedDecision sql.NullString
	err := tx.QueryRowContext(ctx, `SELECT i.plan_codec_revision,j.purpose,i.plan_saved_at,i.decision_saved_at
FROM prepare_planning_intents i JOIN prepare_jobs j ON j.job_id=i.job_id WHERE i.job_id=?`, jobID).
		Scan(&codec, &purpose, &savedPlan, &savedDecision)
	if errors.Is(err, sql.ErrNoRows) {
		return PlanningRecoverySnapshot{}, false, nil
	}
	if err != nil || codec != planningcodec.PlanRevision || purpose != "cache_explain" && purpose != "prepare" {
		return PlanningRecoverySnapshot{}, false, ErrPlanningSchema
	}
	var snapshot PlanningRecoverySnapshot
	var sourceImage, sourceState, cloneRuntimeID, executorID, started, finished, resolution sql.NullString
	var outputComplete sql.NullString
	var enforcement, adapter sql.NullString
	var legacyUnbound, partialPrefix int
	err = tx.QueryRowContext(ctx, `SELECT a.job_id,a.task_id,a.attempt_no,a.clone_id,a.execution_digest,
a.worker_generation,a.planned_executor_name,a.executor_id,a.phase,a.created_at,
a.started_at,a.finished_at,a.resolution,a.legacy_unbound,
c.clone_id,c.clone_generation,c.source_kind,c.source_image_id,c.source_state_id,
c.clone_ref,c.planned_db_runtime_name,c.db_runtime_id,c.phase,
p.protected_input_ref,p.complete_output_ref,p.complete_stderr_ref,p.capture_manifest_ref,
p.capture_helper_digest,p.capture_protocol_revision,p.output_complete_at,
b.execution_digest,b.prepare_kind,b.command_digest,b.input_manifest_digest,
b.executor_image_digest,b.target_identity_digest,b.policy_revision,b.effect_scope,
b.partial_prefix_safe,b.enforcement_digest,b.adapter_binding_digest
FROM prepare_step_attempts a
JOIN prepare_planning_intents i ON i.job_id=a.job_id AND i.task_id=a.task_id
  AND i.execution_digest=a.execution_digest
JOIN prepare_execution_bindings b ON b.execution_digest=a.execution_digest
JOIN prepare_clones c ON c.clone_id=a.clone_id AND c.job_id=a.job_id
JOIN prepare_planning_attempt_artifacts p ON p.job_id=a.job_id AND p.task_id=a.task_id
  AND p.attempt_no=a.attempt_no
WHERE a.job_id=? AND a.task_id='plan' AND (?=0 OR a.attempt_no=?)
ORDER BY a.attempt_no DESC LIMIT 1`, jobID, attemptNo, attemptNo).
		Scan(&snapshot.Attempt.JobID, &snapshot.Attempt.TaskID, &snapshot.Attempt.AttemptNo,
			&snapshot.Attempt.CloneID, &snapshot.Attempt.ExecutionDigest,
			&snapshot.Attempt.WorkerGeneration, &snapshot.Attempt.PlannedExecutorName,
			&executorID, &snapshot.Attempt.Phase, &snapshot.Attempt.CreatedAt,
			&started, &finished, &resolution, &legacyUnbound,
			&snapshot.Clone.ID, &snapshot.Clone.Generation, &snapshot.Clone.SourceKind,
			&sourceImage, &sourceState, &snapshot.Clone.Ref,
			&snapshot.Clone.PlannedRuntimeName, &cloneRuntimeID, &snapshot.ClonePhase,
			&snapshot.Capture.ProtectedInput, &snapshot.Capture.Stdout,
			&snapshot.Capture.Stderr, &snapshot.Capture.Manifest,
			&snapshot.Capture.HelperDigest, &snapshot.Capture.ProtocolRevision, &outputComplete,
			&snapshot.Binding.Digest, &snapshot.Binding.PrepareKind,
			&snapshot.Binding.CommandDigest, &snapshot.Binding.InputManifestDigest,
			&snapshot.Binding.ExecutorImageDigest, &snapshot.Binding.TargetIdentityDigest,
			&snapshot.Binding.PolicyRevision, &snapshot.Binding.EffectScope,
			&partialPrefix, &enforcement, &adapter)
	if err != nil {
		return PlanningRecoverySnapshot{}, false, ErrPlanningSchema
	}
	snapshot.Attempt.ExecutorID = executorID.String
	snapshot.Attempt.StartedAt, snapshot.Attempt.FinishedAt = started.String, finished.String
	snapshot.Attempt.Resolution, snapshot.Attempt.LegacyUnbound = resolution.String, legacyUnbound != 0
	snapshot.Clone.SourceImageID, snapshot.Clone.SourceStateID = sourceImage.String, sourceState.String
	snapshot.CloneRuntimeID = cloneRuntimeID.String
	if snapshot.Attempt.Resolution == "discarded_clone" {
		cancelled, err := validateCancelledPlanningDisposal(ctx, tx, snapshot)
		if err != nil {
			return PlanningRecoverySnapshot{}, false, err
		}
		snapshot.CancelledDisposal = cancelled
	}
	snapshot.OutputComplete = outputComplete.Valid
	snapshot.Binding.PartialPrefixSafe = partialPrefix == 1
	snapshot.Binding.EnforcementDigest, snapshot.Binding.AdapterBindingDigest = enforcement.String, adapter.String
	refs := []string{snapshot.Capture.ProtectedInput, snapshot.Capture.Stdout, snapshot.Capture.Stderr, snapshot.Capture.Manifest}
	if snapshot.Attempt.LegacyUnbound || snapshot.Attempt.AttemptNo < 1 ||
		snapshot.Attempt.PlannedExecutorName != commonAttemptName(jobID, "plan", snapshot.Attempt.AttemptNo) ||
		snapshot.Clone.ID != snapshot.Attempt.CloneID || snapshot.Clone.Generation < 1 ||
		snapshot.Clone.Ref == "" || snapshot.Clone.PlannedRuntimeName == "" ||
		snapshot.Binding.Digest != snapshot.Attempt.ExecutionDigest || snapshot.Binding.PrepareKind != "lb" ||
		snapshot.Binding.PolicyRevision == "" || partialPrefix < 0 || partialPrefix > 1 ||
		!planningcodec.ValidDigest(snapshot.Attempt.ExecutionDigest) ||
		!planningcodec.ValidDigest(snapshot.Capture.HelperDigest) ||
		snapshot.Capture.ProtocolRevision != planningcodec.CaptureProtocolRevision ||
		(snapshot.Clone.SourceKind == "image" && (snapshot.Clone.SourceImageID == "" || snapshot.Clone.SourceStateID != "")) ||
		(snapshot.Clone.SourceKind == "state" && (snapshot.Clone.SourceStateID == "" || snapshot.Clone.SourceImageID != "")) ||
		(snapshot.Clone.SourceKind != "image" && snapshot.Clone.SourceKind != "state") ||
		(snapshot.ClonePhase == "allocated" && cloneRuntimeID.Valid) ||
		((snapshot.ClonePhase == "active" || snapshot.ClonePhase == "quiescent") && !cloneRuntimeID.Valid) ||
		((snapshot.ClonePhase == "retained" || snapshot.ClonePhase == "released") && !cloneRuntimeID.Valid && snapshot.Attempt.Resolution != "never_started" && !snapshot.CancelledDisposal) ||
		(cloneRuntimeID.Valid && !dockerContainerID.MatchString(cloneRuntimeID.String)) ||
		((snapshot.Attempt.Phase == "planned" || snapshot.Attempt.Phase == "create_requested") && executorID.Valid) ||
		(snapshot.Attempt.Phase != "planned" && snapshot.Attempt.Phase != "create_requested" &&
			snapshot.Attempt.Resolution != "never_started" && !snapshot.CancelledDisposal && !executorID.Valid) ||
		(executorID.Valid && !dockerContainerID.MatchString(executorID.String)) {
		return PlanningRecoverySnapshot{}, false, ErrPlanningSchema
	}
	for i, ref := range refs {
		if ref == "" {
			return PlanningRecoverySnapshot{}, false, ErrPlanningSchema
		}
		for j := 0; j < i; j++ {
			if ref == refs[j] {
				return PlanningRecoverySnapshot{}, false, ErrPlanningSchema
			}
		}
	}
	var helperLease int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM prepare_artifact_leases
WHERE owner_id=? AND owner_kind IN ('job','retention') AND artifact_kind='capture_helper'
AND artifact_digest=?`, jobID, snapshot.Capture.HelperDigest).Scan(&helperLease); err != nil || helperLease != 1 {
		return PlanningRecoverySnapshot{}, false, ErrPlanningSchema
	}
	if attemptNo == 0 && validatePlanningReplayChain(ctx, tx, snapshot) != nil {
		return PlanningRecoverySnapshot{}, false, ErrPlanningSchema
	}
	if savedPlan.Valid {
		result, found, err := loadPlanningResult(ctx, tx, jobID)
		if err != nil || !found || result.ExecutionDigest != snapshot.Attempt.ExecutionDigest ||
			(attemptNo == 0 && result.AttemptNo != snapshot.Attempt.AttemptNo) {
			return PlanningRecoverySnapshot{}, false, ErrPlanningSchema
		}
		snapshot.PlanSaved = result.AttemptNo == snapshot.Attempt.AttemptNo
	}
	if savedDecision.Valid {
		if !savedPlan.Valid || purpose != "cache_explain" {
			return PlanningRecoverySnapshot{}, false, ErrPlanningSchema
		}
		var encoded, digest string
		if err := tx.QueryRowContext(ctx, `SELECT cache_decision_json,cache_decision_digest FROM prepare_planning_intents WHERE job_id=?`, jobID).Scan(&encoded, &digest); err != nil {
			return PlanningRecoverySnapshot{}, false, ErrPlanningSchema
		}
		if _, err := planningcodec.DecodeDecision(encoded, digest); err != nil {
			return PlanningRecoverySnapshot{}, false, ErrPlanningSchema
		}
		snapshot.DecisionSaved = snapshot.PlanSaved
	}
	return snapshot, true, nil
}

// validatePlanningReplayChain verifies the complete local two-attempt link,
// including the single warning written with the first replay decision. A
// latest attempt cannot be trusted merely because its own row is coherent.
func validatePlanningReplayChain(ctx context.Context, tx *sql.Tx,
	snapshot PlanningRecoverySnapshot) error {
	if snapshot.Attempt.AttemptNo == 1 {
		var replayDecisions int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM prepare_attempt_decisions
WHERE job_id=? AND task_id='plan' AND choice='replay_fresh'`,
			snapshot.Attempt.JobID).Scan(&replayDecisions); err != nil || replayDecisions != 0 {
			return ErrPlanningSchema
		}
		return nil
	}
	if snapshot.Attempt.AttemptNo != 2 || snapshot.Binding.PolicyRevision != "lb-container-v2" {
		return ErrPlanningSchema
	}
	var linked, warnings int
	var oldResolution sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT count(*),min(old.resolution) FROM prepare_step_attempts old
JOIN prepare_clones old_clone ON old_clone.clone_id=old.clone_id AND old_clone.job_id=old.job_id
JOIN prepare_planning_attempt_artifacts old_capture ON old_capture.job_id=old.job_id
  AND old_capture.task_id=old.task_id AND old_capture.attempt_no=old.attempt_no
JOIN prepare_attempt_decisions d ON d.job_id=old.job_id AND d.task_id=old.task_id
  AND d.attempt_no=old.attempt_no AND d.worker_generation=old.worker_generation
WHERE old.job_id=? AND old.task_id='plan' AND old.attempt_no=1
AND old.phase='closed' AND old.resolution IN ('discarded_clone','never_started')
AND old.execution_digest=?
AND (old.resolution='never_started' AND old.started_at IS NULL
     OR old.resolution='discarded_clone' AND old.executor_id IS NOT NULL)
AND old_clone.phase IN ('retained','released')
AND old_clone.source_kind=? AND old_clone.source_image_id IS ?
AND old_clone.source_state_id IS ? AND old_clone.clone_generation+1=?
AND old_capture.output_complete_at IS NULL
AND d.choice='replay_fresh' AND d.target_clone_id=? AND d.next_attempt_no=2`,
		snapshot.Attempt.JobID, snapshot.Attempt.ExecutionDigest,
		snapshot.Clone.SourceKind,
		optionalPlanningSource(snapshot.Clone.SourceImageID),
		optionalPlanningSource(snapshot.Clone.SourceStateID),
		snapshot.Clone.Generation, snapshot.Clone.ID).Scan(&linked, &oldResolution); err != nil || linked != 1 {
		return ErrPlanningSchema
	}
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM prepare_events
WHERE job_id=? AND type='log'
AND message LIKE 'WARNING [liquibase_external_effect_replay]%'`,
		snapshot.Attempt.JobID).Scan(&warnings); err != nil ||
		(oldResolution.String == "discarded_clone" && warnings != 1) ||
		(oldResolution.String == "never_started" && warnings != 0) ||
		(oldResolution.String != "discarded_clone" && oldResolution.String != "never_started") {
		return ErrPlanningSchema
	}
	return nil
}

func optionalPlanningSource(value string) any {
	if value == "" {
		return nil
	}
	return value
}

// LoadPlanningAttempt reads one historical attempt using the same identity
// and capture checks as the latest-attempt reader. It permits cleanup of a
// discarded clone after a later replay attempt has been committed.
func (s *SQLiteStore) LoadPlanningAttempt(ctx context.Context, jobID string,
	attemptNo int) (PlanningRecoverySnapshot, bool, error) {
	if attemptNo < 1 {
		return PlanningRecoverySnapshot{}, false, ErrPlanningSchema
	}
	return s.loadPlanningAttempt(ctx, jobID, attemptNo)
}

func (s *SQLiteStore) loadPlanningAttempt(ctx context.Context, jobID string,
	attemptNo int) (PlanningRecoverySnapshot, bool, error) {
	if s == nil || s.db == nil || jobID == "" {
		return PlanningRecoverySnapshot{}, false, ErrPlanningSchema
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return PlanningRecoverySnapshot{}, false, err
	}
	defer tx.Rollback()
	snapshot, found, err := loadPlanningAttemptTx(ctx, tx, jobID, attemptNo)
	if err != nil {
		return PlanningRecoverySnapshot{}, false, err
	}
	if err := tx.Commit(); err != nil {
		return PlanningRecoverySnapshot{}, false, err
	}
	return snapshot, found, nil
}
