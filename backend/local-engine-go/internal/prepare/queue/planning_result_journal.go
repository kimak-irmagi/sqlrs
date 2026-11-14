package queue

// Normal-path journal operations from the approved Liquibase prototype.
// Installation and runtime activation are separate boundaries; these methods
// do not enable replay. See issue89-normal-path-storage-extraction.md.

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/sqlrs/engine-local/internal/prepare/planningcodec"
)

// PlanningResult is the verified durable metadata of one exact planning
// attempt. It excludes the raw SQL and protected capture bytes.
type PlanningResult struct {
	AttemptNo       int
	ExecutionDigest string
	OutputDigest    string
	PlanDigest      string
	Changesets      []planningcodec.Changeset
}

type planningResultReader interface {
	QueryRowContext(context.Context, string, ...any) *sql.Row
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func loadPlanningResult(ctx context.Context, reader planningResultReader, jobID string) (PlanningResult, bool, error) {
	var codec, execution string
	var attempt, count sql.NullInt64
	var output, digest, savedAt sql.NullString
	if err := reader.QueryRowContext(ctx, `SELECT plan_codec_revision,execution_digest,result_attempt_no,plan_digest,plan_count,parsed_output_digest,plan_saved_at FROM prepare_planning_intents WHERE job_id=?`, jobID).Scan(&codec, &execution, &attempt, &digest, &count, &output, &savedAt); err != nil {
		return PlanningResult{}, false, ErrPlanningSchema
	}
	if !savedAt.Valid {
		return PlanningResult{}, false, nil
	}
	if !attempt.Valid || attempt.Int64 <= 0 || !count.Valid || count.Int64 < 0 || !output.Valid || !digest.Valid {
		return PlanningResult{}, false, ErrPlanningSchema
	}
	rows, err := reader.QueryContext(ctx, `SELECT ordinal,id,author,path,sql_hash,coalesce(checksum,'') FROM prepare_planning_changesets WHERE job_id=? ORDER BY ordinal`, jobID)
	if err != nil {
		return PlanningResult{}, false, err
	}
	var changesets []planningcodec.Changeset
	for rows.Next() {
		var ordinal int
		var item planningcodec.Changeset
		if err := rows.Scan(&ordinal, &item.ID, &item.Author, &item.Path, &item.SQLHash, &item.Checksum); err != nil {
			_ = rows.Close()
			return PlanningResult{}, false, err
		}
		if ordinal != len(changesets) {
			_ = rows.Close()
			return PlanningResult{}, false, ErrPlanningSchema
		}
		changesets = append(changesets, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return PlanningResult{}, false, err
	}
	if err := rows.Close(); err != nil {
		return PlanningResult{}, false, err
	}
	if int64(len(changesets)) != count.Int64 {
		return PlanningResult{}, false, ErrPlanningSchema
	}
	recomputed, err := planningcodec.PlanDigest(codec, output.String, execution, changesets)
	if err != nil || recomputed != digest.String {
		return PlanningResult{}, false, ErrPlanningSchema
	}
	return PlanningResult{AttemptNo: int(attempt.Int64), ExecutionDigest: execution, OutputDigest: output.String, PlanDigest: digest.String, Changesets: changesets}, true, nil
}

// ErrPlanningSchema denotes a rejected or unavailable durable planning
// namespace. See docs/architecture/liquibase-planning-recovery-storage-schema.md.
var ErrPlanningSchema = errors.New("invalid durable planning schema")

// PlanningCaptureProof contains only metadata returned by the runtime's
// exact stopped-executor and complete-manifest verifier, never raw SQL.
type PlanningCaptureProof struct {
	StdoutDigest string
	StdoutBytes  int64
	StderrDigest string
	StderrBytes  int64
	ManifestRef  string
}

// RecordVerifiedPlanningCapture persists the complete-output milestone and
// its attempt-scoped evidence together. The caller must have independently
// verified exact Docker identity, synced manifest and both byte streams.
func (s *SQLiteStore) RecordVerifiedPlanningCapture(ctx context.Context, attempt AttemptRecord, generation int, proof PlanningCaptureProof, at string) (int64, error) {
	if s == nil || s.db == nil || attempt.TaskID != "plan" || attempt.AttemptNo <= 0 || generation != attempt.WorkerGeneration || at == "" ||
		!planningcodec.ValidDigest(proof.StdoutDigest) || !planningcodec.ValidDigest(proof.StderrDigest) ||
		proof.StdoutBytes < 0 || proof.StderrBytes < 0 || proof.ManifestRef == "" {
		return 0, ErrPlanningSchema
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var manifest, phase, executor string
	var savedDigest, savedAt sql.NullString
	var savedBytes sql.NullInt64
	if err := tx.QueryRowContext(ctx, `SELECT capture.capture_manifest_ref,a.phase,a.executor_id,capture.output_digest,capture.output_bytes,capture.output_complete_at FROM prepare_planning_attempt_artifacts capture JOIN prepare_step_attempts a ON a.job_id=capture.job_id AND a.task_id=capture.task_id AND a.attempt_no=capture.attempt_no WHERE a.job_id=? AND a.task_id='plan' AND a.attempt_no=? AND a.clone_id=? AND a.execution_digest=? AND a.worker_generation=? AND a.planned_executor_name=?`,
		attempt.JobID, attempt.AttemptNo, attempt.CloneID, attempt.ExecutionDigest, generation, attempt.PlannedExecutorName).Scan(&manifest, &phase, &executor, &savedDigest, &savedBytes, &savedAt); err != nil {
		return 0, ErrPlanningSchema
	}
	if manifest != proof.ManifestRef || executor == "" || phase != "observed_stopped" && phase != "closed" {
		return 0, ErrPlanningSchema
	}
	var successfulExit int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM prepare_attempt_evidence e WHERE e.job_id=? AND e.task_id='plan' AND e.attempt_no=? AND e.worker_generation=? AND e.kind='executor_inspection' AND json_extract(e.facts_json,'$.observation.executor_id')=? AND json_extract(e.facts_json,'$.observation.exit_code')=0`, attempt.JobID, attempt.AttemptNo, generation, executor).Scan(&successfulExit); err != nil || successfulExit < 1 {
		return 0, ErrPlanningSchema
	}
	if savedAt.Valid {
		if !savedDigest.Valid || !savedBytes.Valid || savedDigest.String != proof.StdoutDigest || savedBytes.Int64 != proof.StdoutBytes {
			return 0, ErrPlanningSchema
		}
		var evidenceID int64
		if err := tx.QueryRowContext(ctx, `SELECT evidence_id FROM prepare_attempt_evidence WHERE job_id=? AND task_id='plan' AND attempt_no=? AND worker_generation=? AND kind='planning_output' AND json_extract(facts_json,'$.observation.output_digest')=? AND json_extract(facts_json,'$.observation.output_bytes')=? AND json_extract(facts_json,'$.observation.stderr_digest')=? AND json_extract(facts_json,'$.observation.stderr_bytes')=? AND json_extract(facts_json,'$.observation.manifest_ref')=? ORDER BY evidence_id LIMIT 1`, attempt.JobID, attempt.AttemptNo, generation, proof.StdoutDigest, proof.StdoutBytes, proof.StderrDigest, proof.StderrBytes, proof.ManifestRef).Scan(&evidenceID); err != nil {
			return 0, ErrPlanningSchema
		}
		return evidenceID, nil
	}
	observation, err := json.Marshal(struct {
		OutputDigest string `json:"output_digest"`
		OutputBytes  int64  `json:"output_bytes"`
		StderrDigest string `json:"stderr_digest"`
		StderrBytes  int64  `json:"stderr_bytes"`
		ManifestRef  string `json:"manifest_ref"`
	}{proof.StdoutDigest, proof.StdoutBytes, proof.StderrDigest, proof.StderrBytes, proof.ManifestRef})
	if err != nil {
		return 0, err
	}
	id, err := appendAttemptEvidenceTx(ctx, tx, attempt, generation, "planning_output", observation, at)
	if err != nil {
		return 0, err
	}
	result, err := tx.ExecContext(ctx, `UPDATE prepare_planning_attempt_artifacts SET output_digest=?,output_bytes=?,output_complete_at=? WHERE job_id=? AND task_id='plan' AND attempt_no=? AND output_complete_at IS NULL`, proof.StdoutDigest, proof.StdoutBytes, at, attempt.JobID, attempt.AttemptNo)
	if err != nil || oneRowChanged(result) != nil {
		return 0, ErrPlanningSchema
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

// CompletePlanning binds exact exit, clone quiescence, and protected output
// evidence to the saved plan, then closes its attempt and stable task in the
// same transaction. No state is published by planning.
func (s *SQLiteStore) CompletePlanning(ctx context.Context, attempt AttemptRecord, generation int, exitID, quiescenceID, outputID int64, at string) error {
	if s == nil || s.db == nil || attempt.TaskID != "plan" || attempt.AttemptNo <= 0 || generation != attempt.WorkerGeneration || exitID <= 0 || quiescenceID <= exitID || outputID <= quiescenceID || at == "" {
		return ErrPlanningSchema
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if result, found, err := loadPlanningResult(ctx, tx, attempt.JobID); err != nil || !found || result.AttemptNo != attempt.AttemptNo || result.ExecutionDigest != attempt.ExecutionDigest {
		return ErrPlanningSchema
	}
	var phase string
	var resolution sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT phase,resolution FROM prepare_step_attempts WHERE job_id=? AND task_id='plan' AND attempt_no=? AND clone_id=? AND execution_digest=? AND worker_generation=? AND planned_executor_name=?`, attempt.JobID, attempt.AttemptNo, attempt.CloneID, attempt.ExecutionDigest, generation, attempt.PlannedExecutorName).Scan(&phase, &resolution); err != nil {
		return ErrPlanningSchema
	}
	decisionID := fmt.Sprintf("planning-complete:%s:%d", attempt.JobID, attempt.AttemptNo)
	if resolution.Valid {
		if resolution.String != "planning_result_saved" || phase != "closed" {
			return ErrPlanningSchema
		}
		var taskStatus string
		if err := tx.QueryRowContext(ctx, `SELECT status FROM prepare_tasks WHERE job_id=? AND task_id='plan'`, attempt.JobID).Scan(&taskStatus); err != nil || taskStatus != "succeeded" {
			return ErrPlanningSchema
		}
		var sameProof int
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM prepare_attempt_decisions d
WHERE d.decision_id=? AND d.job_id=? AND d.task_id='plan' AND d.attempt_no=?
AND d.worker_generation=? AND d.choice='complete_planning'
AND (SELECT count(*) FROM prepare_decision_evidence WHERE decision_id=d.decision_id)=3
AND EXISTS (SELECT 1 FROM prepare_decision_evidence WHERE decision_id=d.decision_id AND evidence_id=?)
AND EXISTS (SELECT 1 FROM prepare_decision_evidence WHERE decision_id=d.decision_id AND evidence_id=?)
AND EXISTS (SELECT 1 FROM prepare_decision_evidence WHERE decision_id=d.decision_id AND evidence_id=?)`, decisionID, attempt.JobID, attempt.AttemptNo, generation, exitID, quiescenceID, outputID).Scan(&sameProof); err != nil || sameProof != 1 {
			return ErrPlanningSchema
		}
		return nil
	}
	if phase != "observed_stopped" {
		return ErrPlanningSchema
	}
	var decisionNo int
	if err := tx.QueryRowContext(ctx, `SELECT coalesce(max(decision_no),0)+1 FROM prepare_attempt_decisions WHERE job_id=? AND task_id='plan' AND attempt_no=?`, attempt.JobID, attempt.AttemptNo).Scan(&decisionNo); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO prepare_attempt_decisions(decision_id,job_id,task_id,attempt_no,decision_no,choice,worker_generation,created_at) VALUES(?,?,'plan',?,?,'complete_planning',?,?)`, decisionID, attempt.JobID, attempt.AttemptNo, decisionNo, generation, at); err != nil {
		return err
	}
	for _, id := range []int64{exitID, quiescenceID, outputID} {
		if _, err := tx.ExecContext(ctx, `INSERT INTO prepare_decision_evidence(decision_id,evidence_id) VALUES(?,?)`, decisionID, id); err != nil {
			return err
		}
	}
	result, err := tx.ExecContext(ctx, `UPDATE prepare_step_attempts SET phase='closed',resolution='planning_result_saved' WHERE job_id=? AND task_id='plan' AND attempt_no=? AND phase='observed_stopped' AND resolution IS NULL`, attempt.JobID, attempt.AttemptNo)
	if err != nil || oneRowChanged(result) != nil {
		return ErrPlanningSchema
	}
	result, err = tx.ExecContext(ctx, `UPDATE prepare_tasks SET status='succeeded',finished_at=? WHERE job_id=? AND task_id='plan' AND status IN ('running','recovery_required')`, at, attempt.JobID)
	if err != nil || oneRowChanged(result) != nil {
		return ErrPlanningSchema
	}
	return tx.Commit()
}

// ReadPlanningOutputEvidence locates the unique already-verified protected
// output milestone for a saved plan. It is used after a crash between plan
// save and attempt completion, without re-reading or re-running Liquibase.
// See P04 in the planning recovery test design.
func (s *SQLiteStore) ReadPlanningOutputEvidence(ctx context.Context, attempt AttemptRecord, generation int, outputDigest string) (int64, error) {
	if s == nil || s.db == nil || attempt.JobID == "" || attempt.TaskID != "plan" ||
		attempt.AttemptNo < 1 || generation != attempt.WorkerGeneration ||
		!planningcodec.ValidDigest(outputDigest) {
		return 0, ErrPlanningSchema
	}
	rows, err := s.db.QueryContext(ctx, `SELECT e.evidence_id,
json_extract(e.facts_json,'$.observation.stderr_digest'),
json_extract(e.facts_json,'$.observation.stderr_bytes')
FROM prepare_planning_attempt_artifacts p
JOIN prepare_step_attempts a ON a.job_id=p.job_id AND a.task_id=p.task_id AND a.attempt_no=p.attempt_no
JOIN prepare_attempt_evidence e ON e.job_id=a.job_id AND e.task_id=a.task_id
  AND e.attempt_no=a.attempt_no AND e.worker_generation=a.worker_generation
WHERE a.job_id=? AND a.task_id='plan' AND a.attempt_no=? AND a.clone_id=?
AND a.execution_digest=? AND a.worker_generation=? AND a.planned_executor_name=?
AND p.output_complete_at IS NOT NULL AND p.output_digest=? AND p.output_bytes>=0
AND e.kind='planning_output'
AND json_extract(e.facts_json,'$.observation.output_digest')=p.output_digest
AND json_extract(e.facts_json,'$.observation.output_bytes')=p.output_bytes
AND json_extract(e.facts_json,'$.observation.manifest_ref')=p.capture_manifest_ref`,
		attempt.JobID, attempt.AttemptNo, attempt.CloneID, attempt.ExecutionDigest,
		generation, attempt.PlannedExecutorName, outputDigest)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	var id, stderrBytes int64
	var stderrDigest string
	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return 0, err
		}
		return 0, ErrPlanningSchema
	}
	if err := rows.Scan(&id, &stderrDigest, &stderrBytes); err != nil {
		return 0, ErrPlanningSchema
	}
	if rows.Next() || rows.Err() != nil || id <= 0 || !planningcodec.ValidDigest(stderrDigest) || stderrBytes < 0 {
		return 0, ErrPlanningSchema
	}
	return id, nil
}

// RecordPlanningCloneQuiescence binds verified physical absence of the exact
// preview database to the stopped planning executor in one transaction. It
// does not itself inspect Docker. See P04/R03 in the planning test design.
func (s *SQLiteStore) RecordPlanningCloneQuiescence(ctx context.Context, attempt AttemptRecord, generation int, runtimeName, runtimeID, at string) (int64, error) {
	if s == nil || s.db == nil || attempt.JobID == "" || attempt.TaskID != "plan" ||
		attempt.AttemptNo < 1 || attempt.CloneID == "" || attempt.ExecutionDigest == "" ||
		attempt.PlannedExecutorName == "" || attempt.LegacyUnbound ||
		!dockerContainerID.MatchString(attempt.ExecutorID) || !dockerContainerID.MatchString(runtimeID) ||
		runtimeName == "" || generation != attempt.WorkerGeneration || at == "" {
		return 0, ErrPlanningSchema
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	result, err := tx.ExecContext(ctx, `UPDATE prepare_clones SET phase='quiescent'
WHERE clone_id=? AND job_id=? AND planned_db_runtime_name=? AND db_runtime_id=?
AND phase='active' AND EXISTS (SELECT 1 FROM prepare_step_attempts a
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
    AND a.planned_executor_name=? AND a.executor_id=?
    AND a.worker_generation=? AND a.legacy_unbound=0
    AND a.phase='observed_stopped' AND a.resolution IS NULL
    AND j.status IN ('running','recovery_required')
    AND t.type='plan' AND t.status IN ('running','recovery_required')
    AND EXISTS (SELECT 1 FROM prepare_attempt_evidence e
      WHERE e.job_id=a.job_id AND e.task_id=a.task_id
        AND e.attempt_no=a.attempt_no AND e.kind='executor_inspection'
        AND e.worker_generation=a.worker_generation
        AND json_extract(e.facts_json,'$.observation.executor_id')=a.executor_id))`,
		attempt.CloneID, attempt.JobID, runtimeName, runtimeID,
		attempt.AttemptNo, attempt.ExecutionDigest, attempt.PlannedExecutorName,
		attempt.ExecutorID, generation)
	if err != nil || oneRowChanged(result) != nil {
		return 0, ErrPlanningSchema
	}
	observation, err := json.Marshal(struct {
		RuntimeAbsent bool   `json:"runtime_absent"`
		RuntimeName   string `json:"runtime_name"`
		RuntimeID     string `json:"runtime_id"`
	}{true, runtimeName, runtimeID})
	if err != nil {
		return 0, err
	}
	id, err := appendAttemptEvidenceTx(ctx, tx, attempt, generation, "db_quiescence", observation, at)
	if err != nil {
		return 0, err
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	return id, nil
}

// SavePlanningResult commits ordered metadata and its canonical digest in
// one transaction. A retry may observe the same saved result, but may not
// substitute a different attempt, output, order, or metadata. The SQL guard
// additionally requires exact stopped-executor and capture milestones.
// See P03/P04 in docs/architecture/liquibase-planning-recovery-tests.md.
func (s *SQLiteStore) SavePlanningResult(ctx context.Context, jobID string, attemptNo int, outputDigest string, changesets []planningcodec.Changeset, at string) (string, error) {
	if s == nil || s.db == nil || jobID == "" || attemptNo <= 0 || at == "" || !planningcodec.ValidDigest(outputDigest) {
		return "", ErrPlanningSchema
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var codec, execution string
	var savedAttempt, planCount sql.NullInt64
	var savedDigest, savedOutput, savedAt sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT plan_codec_revision,execution_digest,result_attempt_no,plan_digest,plan_count,parsed_output_digest,plan_saved_at FROM prepare_planning_intents WHERE job_id=?`, jobID).Scan(&codec, &execution, &savedAttempt, &savedDigest, &planCount, &savedOutput, &savedAt); err != nil {
		return "", ErrPlanningSchema
	}
	digest, err := planningcodec.PlanDigest(codec, outputDigest, execution, changesets)
	if err != nil {
		return "", err
	}
	if savedAt.Valid {
		old, found, err := loadPlanningResult(ctx, tx, jobID)
		if err != nil || !found || old.AttemptNo != attemptNo || old.PlanDigest != digest || old.OutputDigest != outputDigest || !samePlanningChangesets(old.Changesets, changesets) {
			return "", ErrPlanningSchema
		}
		return digest, nil
	}
	if savedAttempt.Valid || savedDigest.Valid || savedOutput.Valid || planCount.Valid {
		return "", ErrPlanningSchema
	}
	var existing int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM prepare_planning_changesets WHERE job_id=?`, jobID).Scan(&existing); err != nil || existing != 0 {
		return "", ErrPlanningSchema
	}
	for ordinal, item := range changesets {
		if _, err := tx.ExecContext(ctx, `INSERT INTO prepare_planning_changesets(job_id,ordinal,id,author,path,sql_hash,checksum) VALUES(?,?,?,?,?,?,?)`, jobID, ordinal, item.ID, item.Author, item.Path, item.SQLHash, item.Checksum); err != nil {
			return "", err
		}
	}
	result, err := tx.ExecContext(ctx, `UPDATE prepare_planning_intents SET result_attempt_no=?,plan_digest=?,plan_count=?,parsed_output_digest=?,plan_saved_at=? WHERE job_id=? AND plan_saved_at IS NULL`, attemptNo, digest, len(changesets), outputDigest, at, jobID)
	if err != nil {
		return "", err
	}
	changed, err := result.RowsAffected()
	if err != nil || changed != 1 {
		return "", ErrPlanningSchema
	}
	if err := tx.Commit(); err != nil {
		return "", err
	}
	return digest, nil
}

// LoadPlanningResult validates the saved revision, ordered rows, count and
// canonical digest before any recovery worker can use the plan.
func (s *SQLiteStore) LoadPlanningResult(ctx context.Context, jobID string) (PlanningResult, bool, error) {
	if s == nil || s.db == nil {
		return PlanningResult{}, false, ErrPlanningSchema
	}
	return loadPlanningResult(ctx, s.db, jobID)
}

func samePlanningChangesets(a, b []planningcodec.Changeset) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
