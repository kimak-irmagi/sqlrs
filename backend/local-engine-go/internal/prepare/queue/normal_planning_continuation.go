package queue

// Normal planning continuation and step identity. See
// docs/architecture/issue89-normal-path-storage-extraction.md.

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/sqlrs/engine-local/internal/prepare/planningcodec"
	"reflect"
	"regexp"
)

// NextCloneGeneration is a planning hint. PlanBoundAttempt rechecks it in its
// write transaction, so concurrent planners cannot allocate the same clone.
func (s *SQLiteStore) NextCloneGeneration(ctx context.Context, jobID string) (int, error) {
	if s == nil || s.db == nil || jobID == "" {
		return 0, ErrCommonAttempt
	}
	var generation int
	if err := s.db.QueryRowContext(ctx, `SELECT coalesce(max(clone_generation),0)+1
FROM prepare_clones WHERE job_id=?`, jobID).Scan(&generation); err != nil {
		return 0, err
	}
	return generation, nil
}

var commonLiquibaseHash = regexp.MustCompile(`^[0-9a-f]{64}$`)

// ProjectLiquibaseStepBinding derives the same generic step identity during
// normal execution and verified v1-state migration. The adapter fingerprint
// already binds the exact argument vector; the stored normalized command and
// task metadata bind this particular changeset without re-reading a project.
func ProjectLiquibaseStepBinding(record LiquibaseExecutionRecord,
	argsNormalized, taskHash, changesetID, changesetAuthor, changesetPath,
	outputStateID string) (ExecutionBinding, error) {
	if record.ExecutionFingerprint == "" || record.ImageManifestDigest == "" ||
		record.TargetIdentityDigest == "" || record.BundleManifestDigest == "" ||
		record.EffectiveConfigDigest == "" || record.PolicyRevision == "" ||
		argsNormalized == "" || !commonLiquibaseHash.MatchString(taskHash) ||
		outputStateID == "" {
		return ExecutionBinding{}, fmt.Errorf("complete saved Liquibase step is required")
	}
	binding := ExecutionBinding{
		PrepareKind: "lb",
		CommandDigest: commonLiquibaseDigest("common-lb-command-v2", argsNormalized,
			taskHash, changesetID, changesetAuthor, changesetPath,
			record.EffectiveConfigDigest),
		InputManifestDigest:  record.BundleManifestDigest,
		ExecutorImageDigest:  record.ImageManifestDigest,
		TargetIdentityDigest: record.TargetIdentityDigest,
		PolicyRevision:       record.PolicyRevision, EffectScope: "unknown",
		AdapterBindingDigest: record.ExecutionFingerprint,
	}
	binding.Digest = commonLiquibaseDigest("common-lb-execution-v2",
		binding.PrepareKind, binding.CommandDigest, binding.InputManifestDigest,
		binding.ExecutorImageDigest, binding.TargetIdentityDigest,
		binding.PolicyRevision, binding.EffectScope, binding.AdapterBindingDigest)
	return binding, nil
}

// SavePlanningDecision commits a canonical cache explanation once, after a
// verified durable plan. Repeated recovery returns the original bytes and
// rejects a changed lookup. See F01 in the planning recovery test design.
func (s *SQLiteStore) SavePlanningDecision(ctx context.Context, jobID string, decision planningcodec.Decision, at string) (string, string, error) {
	if s == nil || s.db == nil || jobID == "" || at == "" {
		return "", "", ErrPlanningSchema
	}
	encoded, digest, err := planningcodec.CanonicalDecision(decision)
	if err != nil {
		return "", "", err
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return "", "", err
	}
	defer tx.Rollback()
	var purpose string
	var savedJSON, savedDigest, savedAt sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT j.purpose,i.cache_decision_json,i.cache_decision_digest,i.decision_saved_at FROM prepare_planning_intents i JOIN prepare_jobs j ON j.job_id=i.job_id WHERE i.job_id=?`, jobID).Scan(&purpose, &savedJSON, &savedDigest, &savedAt); err != nil || purpose != "cache_explain" {
		return "", "", ErrPlanningSchema
	}
	if _, found, err := loadPlanningResult(ctx, tx, jobID); err != nil || !found {
		return "", "", ErrPlanningSchema
	}
	if savedAt.Valid {
		if !savedJSON.Valid || !savedDigest.Valid {
			return "", "", ErrPlanningSchema
		}
		if _, err := planningcodec.DecodeDecision(savedJSON.String, savedDigest.String); err != nil || savedJSON.String != encoded || savedDigest.String != digest {
			return "", "", ErrPlanningSchema
		}
		return encoded, digest, nil
	}
	if savedJSON.Valid || savedDigest.Valid {
		return "", "", ErrPlanningSchema
	}
	result, err := tx.ExecContext(ctx, `UPDATE prepare_planning_intents SET cache_decision_json=?,cache_decision_digest=?,decision_saved_at=? WHERE job_id=? AND decision_saved_at IS NULL AND plan_saved_at IS NOT NULL`, encoded, digest, at, jobID)
	if err != nil {
		return "", "", err
	}
	changed, err := result.RowsAffected()
	if err != nil || changed != 1 {
		return "", "", ErrPlanningSchema
	}
	if err := tx.Commit(); err != nil {
		return "", "", err
	}
	return encoded, digest, nil
}

// LoadPlanningDecision validates the persisted bytes on recovery, without
// recomputing the cache lookup.
func (s *SQLiteStore) LoadPlanningDecision(ctx context.Context, jobID string) (planningcodec.Decision, bool, error) {
	if s == nil || s.db == nil {
		return planningcodec.Decision{}, false, ErrPlanningSchema
	}
	if _, found, err := s.LoadPlanningResult(ctx, jobID); err != nil || !found {
		return planningcodec.Decision{}, false, ErrPlanningSchema
	}
	var encoded, digest, at sql.NullString
	if err := s.db.QueryRowContext(ctx, `SELECT cache_decision_json,cache_decision_digest,decision_saved_at FROM prepare_planning_intents WHERE job_id=?`, jobID).Scan(&encoded, &digest, &at); err != nil {
		return planningcodec.Decision{}, false, ErrPlanningSchema
	}
	if !at.Valid {
		return planningcodec.Decision{}, false, nil
	}
	if !encoded.Valid || !digest.Valid {
		return planningcodec.Decision{}, false, ErrPlanningSchema
	}
	decision, err := planningcodec.DecodeDecision(encoded.String, digest.String)
	if err != nil {
		return planningcodec.Decision{}, false, ErrPlanningSchema
	}
	return decision, true, nil
}

// AppendPlanningSuffix preserves the succeeded plan task and its attempt
// history while atomically adding the exact execution continuation once.
// A second worker may observe the same suffix but cannot replace it. Only an
// ordinary prepare may receive executable tasks: plan-only predictions belong
// exclusively to the public plan projection (planning recovery design, P05).
func (s *SQLiteStore) AppendPlanningSuffix(ctx context.Context, jobID string, tasks []TaskRecord) error {
	if s == nil || s.db == nil || jobID == "" || len(tasks) < 2 ||
		tasks[0].JobID != jobID || tasks[0].TaskID != "plan" || tasks[0].Type != "plan" {
		return ErrPlanningSchema
	}
	for i, task := range tasks {
		if task.JobID != jobID || task.Position != i || i > 0 &&
			(task.TaskID == "" || task.TaskID == "plan" || task.Status != "queued") {
			return ErrPlanningSchema
		}
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var eligible int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM prepare_jobs j
JOIN prepare_planning_intents i ON i.job_id=j.job_id
JOIN prepare_tasks p ON p.job_id=j.job_id AND p.task_id='plan'
JOIN prepare_clones c ON c.clone_id=i.preview_clone_id
WHERE j.job_id=? AND j.purpose='prepare' AND j.plan_only=0 AND j.prepare_kind='lb' AND j.status='running'
  AND i.plan_saved_at IS NOT NULL AND p.status='succeeded' AND p.position=0
  AND (c.phase='released' OR EXISTS(SELECT 1 FROM prepare_liquibase_planning_db_ownership owner
    WHERE owner.job_id=j.job_id AND owner.clone_id=c.clone_id AND owner.session_id IS NULL
    AND owner.disposal_requested_evidence_id IS NULL AND owner.disposed_at IS NULL AND c.phase='active')) AND NOT EXISTS (
    SELECT 1 FROM prepare_planning_attempt_artifacts a WHERE a.job_id=j.job_id
    AND (a.executor_retired_at IS NULL OR a.protected_input_retired_at IS NULL
      OR a.output_retired_at IS NULL OR a.stderr_retired_at IS NULL
      OR a.manifest_retired_at IS NULL))`, jobID).Scan(&eligible); err != nil || eligible != 1 {
		return ErrPlanningSchema
	}
	rows, err := tx.QueryContext(ctx, `SELECT job_id,task_id,position,type,status,planner_kind,input_kind,input_id,image_id,resolved_image_id,task_hash,output_state_id,cached,instance_mode,changeset_id,changeset_author,changeset_path,started_at,finished_at,error_json FROM prepare_tasks WHERE job_id=? ORDER BY position`, jobID)
	if err != nil {
		return err
	}
	var saved []TaskRecord
	for rows.Next() {
		item, err := scanTask(rows)
		if err != nil {
			_ = rows.Close()
			return err
		}
		saved = append(saved, item)
	}
	if err := rows.Err(); err != nil {
		_ = rows.Close()
		return err
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if len(saved) == 0 || saved[0].TaskID != "plan" || saved[0].Status != "succeeded" || saved[0].Position != 0 {
		return ErrPlanningSchema
	}
	if len(saved) > 1 {
		if len(saved) != len(tasks) {
			return ErrPlanningSchema
		}
		for i := 1; i < len(tasks); i++ {
			actual, expected := saved[i], tasks[i]
			actual.Status, expected.Status = "", ""
			actual.StartedAt, actual.FinishedAt, actual.ErrorJSON = nil, nil, nil
			expected.StartedAt, expected.FinishedAt, expected.ErrorJSON = nil, nil, nil
			actual.Cached, expected.Cached = nil, nil
			if !reflect.DeepEqual(actual, expected) {
				return ErrPlanningSchema
			}
		}
		return nil
	}
	const insert = `INSERT INTO prepare_tasks (job_id,task_id,position,type,status,planner_kind,input_kind,input_id,image_id,resolved_image_id,task_hash,output_state_id,cached,instance_mode,changeset_id,changeset_author,changeset_path,started_at,finished_at,error_json) VALUES (?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`
	for _, task := range tasks[1:] {
		if _, err := tx.ExecContext(ctx, insert, task.JobID, task.TaskID, task.Position, task.Type, task.Status,
			nullString(task.PlannerKind), nullString(task.InputKind), nullString(task.InputID),
			nullString(task.ImageID), nullString(task.ResolvedImageID), nullString(task.TaskHash),
			nullString(task.OutputStateID), nullBool(task.Cached), nullString(task.InstanceMode),
			nullString(task.ChangesetID), nullString(task.ChangesetAuthor), nullString(task.ChangesetPath),
			nullString(task.StartedAt), nullString(task.FinishedAt), nullString(task.ErrorJSON)); err != nil {
			return err
		}
	}
	return tx.Commit()
}
