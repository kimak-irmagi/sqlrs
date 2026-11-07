package queue

// Normal-path journal operations from the approved Liquibase prototype.
// Installation and runtime activation are separate boundaries; these methods
// do not enable replay. See issue89-normal-path-storage-extraction.md.

import (
	"context"
	"database/sql"
)

func readLiquibaseFirstSessionTask(ctx context.Context, tx *sql.Tx, jobID string) (TaskRecord, bool, error) {
	var eligible int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM prepare_jobs j
JOIN liquibase_job_bindings jb ON jb.job_id=j.job_id
JOIN liquibase_execution_bindings lb ON lb.execution_fingerprint=jb.execution_fingerprint
WHERE j.job_id=? AND j.prepare_kind='lb' AND j.purpose='prepare' AND j.plan_only=0
AND j.status IN ('running','recovery_required') AND lb.execution_policy_revision='lb-container-v2'
AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_job_cancellations WHERE job_id=j.job_id)
AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_job_finalizations WHERE job_id=j.job_id)
AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_sessions WHERE job_id=j.job_id)
AND NOT EXISTS (SELECT 1 FROM prepare_step_attempts WHERE job_id=j.job_id AND task_id<>'plan')`, jobID).Scan(&eligible); err != nil {
		return TaskRecord{}, false, err
	}
	if eligible != 1 {
		return TaskRecord{}, false, nil
	}
	preview, found, err := loadPlanningAttemptTx(ctx, tx, jobID, 0)
	if err != nil {
		return TaskRecord{}, false, err
	}
	if !found || !preview.PlanSaved || preview.Attempt.Phase != "closed" || preview.Attempt.Resolution != "planning_result_saved" || preview.ClonePhase != "released" || preview.Binding.PolicyRevision != "lb-container-v2" {
		return TaskRecord{}, false, nil
	}
	if err := tx.QueryRowContext(ctx, `SELECT (SELECT count(*) FROM prepare_planning_attempt_artifacts WHERE job_id=?1 AND
(executor_retired_at IS NULL OR protected_input_retired_at IS NULL OR output_retired_at IS NULL OR stderr_retired_at IS NULL OR manifest_retired_at IS NULL))
+(SELECT count(*) FROM prepare_clones WHERE job_id=?1 AND phase<>'released')`, jobID).Scan(&eligible); err != nil {
		return TaskRecord{}, false, err
	}
	if eligible != 0 {
		return TaskRecord{}, false, nil
	}
	plan, found, err := loadPlanningResult(ctx, tx, jobID)
	if err != nil || !found {
		return TaskRecord{}, false, err
	}
	rows, err := tx.QueryContext(ctx, `SELECT job_id,task_id,position,type,status,planner_kind,input_kind,input_id,image_id,resolved_image_id,task_hash,output_state_id,cached,instance_mode,changeset_id,changeset_author,changeset_path,started_at,finished_at,error_json FROM prepare_tasks WHERE job_id=? ORDER BY position`, jobID)
	if err != nil {
		return TaskRecord{}, false, err
	}
	var tasks []TaskRecord
	for rows.Next() {
		task, err := scanTask(rows)
		if err != nil {
			_ = rows.Close()
			return TaskRecord{}, false, err
		}
		tasks = append(tasks, task)
	}
	streamErr, closeErr := rows.Err(), rows.Close()
	if streamErr != nil {
		return TaskRecord{}, false, streamErr
	}
	if closeErr != nil {
		return TaskRecord{}, false, closeErr
	}
	if len(tasks) == 0 || tasks[0].TaskID != "plan" || tasks[0].Status != "succeeded" {
		return TaskRecord{}, false, nil
	}
	var candidate TaskRecord
	ordinal := 0
	previousOutput := ""
	for _, task := range tasks {
		if task.Type != "state_execute" {
			continue
		}
		if ordinal >= len(plan.Changesets) || task.InputKind == nil || task.InputID == nil || task.OutputStateID == nil {
			return TaskRecord{}, false, nil
		}
		change := plan.Changesets[ordinal]
		// TaskHash is a cumulative engine fingerprint, not the raw changeset
		// SQL hash. The ordinary pinned-binding path verifies its exact digest.
		if task.ChangesetID == nil || *task.ChangesetID != change.ID || task.ChangesetAuthor == nil || *task.ChangesetAuthor != change.Author || task.ChangesetPath == nil || *task.ChangesetPath != change.Path || task.TaskHash == nil || *task.TaskHash == "" {
			return TaskRecord{}, false, nil
		}
		if ordinal == 0 {
			inputID := preview.Clone.SourceImageID
			if preview.Clone.SourceKind == "state" {
				inputID = preview.Clone.SourceStateID
			}
			if *task.InputKind != preview.Clone.SourceKind || *task.InputID != inputID {
				return TaskRecord{}, false, nil
			}
		}
		if ordinal > 0 && (*task.InputKind != "state" || *task.InputID != previousOutput) {
			return TaskRecord{}, false, nil
		}
		if task.Status == "succeeded" {
			if candidate.TaskID != "" || task.Cached == nil || !*task.Cached {
				return TaskRecord{}, false, nil
			}
		} else if candidate.TaskID == "" {
			if task.Status != "running" && task.Status != "recovery_required" && task.Status != "queued" {
				return TaskRecord{}, false, nil
			}
			candidate = task
		} else if task.Status != "queued" {
			return TaskRecord{}, false, nil
		}
		previousOutput = *task.OutputStateID
		ordinal++
	}
	if ordinal != len(plan.Changesets) || candidate.TaskID == "" {
		return TaskRecord{}, false, nil
	}
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM prepare_tasks t JOIN prepare_step_bindings sb USING(job_id,task_id)
JOIN prepare_execution_bindings b ON b.execution_digest=sb.execution_digest JOIN liquibase_job_bindings lb ON lb.job_id=t.job_id
WHERE t.job_id=? AND t.task_id=? AND b.prepare_kind='lb' AND b.policy_revision='lb-container-v2'
AND b.adapter_binding_digest=lb.execution_fingerprint`, jobID, candidate.TaskID).Scan(&eligible); err != nil {
		return TaskRecord{}, false, err
	}
	if eligible != 1 {
		return TaskRecord{}, false, nil
	}
	return candidate, true, nil
}
