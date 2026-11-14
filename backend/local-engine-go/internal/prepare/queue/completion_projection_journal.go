package queue

// Normal completion prerequisites from the approved journal. Shared readers
// validate saved authority; no recovery/replay dispatcher is installed.
// See docs/architecture/issue89-normal-path-storage-extraction.md.

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
)

// CheckLiquibaseProjectionAllocation is a fresh admission read, not a cached
// missing-intent observation. Caller holds the keyed job work gate through the
// filesystem operation. Cancellation may accept concurrently; it does not undo
// an already admitted allocation, whose bytes the finalizer must rediscover.
func (s *SQLiteStore) CheckLiquibaseProjectionAllocation(ctx context.Context, jobID string) error {
	if s == nil || s.db == nil || jobID == "" {
		return ErrLiquibaseFinalization
	}
	var allowed int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM prepare_jobs j
JOIN liquibase_job_bindings b ON b.job_id=j.job_id WHERE j.job_id=?
AND j.prepare_kind='lb' AND j.status IN ('queued','running')
AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_job_finalizations f WHERE f.job_id=j.job_id)
AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_job_cancellations c WHERE c.job_id=j.job_id)`, jobID).Scan(&allowed); err != nil || allowed != 1 {
		return ErrLiquibaseFinalization
	}
	return nil
}

// CheckLiquibaseProjectionResume authorizes read-only reopening for exact
// journal reconciliation, including cancellation/uncertain-result retirement.
// It never grants allocation or another effect-capable command (LC05/15).
func (s *SQLiteStore) CheckLiquibaseProjectionResume(ctx context.Context, jobID string) error {
	if s == nil || s.db == nil || jobID == "" {
		return ErrLiquibaseFinalization
	}
	var allowed int
	if err := s.db.QueryRowContext(ctx, `SELECT count(*) FROM prepare_jobs j
JOIN liquibase_job_bindings b ON b.job_id=j.job_id WHERE j.job_id=?
AND j.prepare_kind='lb' AND j.status IN ('queued','running','recovery_required')
AND NOT EXISTS (SELECT 1 FROM prepare_liquibase_job_finalizations f WHERE f.job_id=j.job_id)`, jobID).Scan(&allowed); err != nil || allowed != 1 {
		return ErrLiquibaseFinalization
	}
	return nil
}

// CheckLiquibaseProjectionCleanup revalidates the exact selected nonterminal
// intent and all saved retirement authority under one SQLite snapshot. The
// caller retains exclusive job ownership throughout physical traversal. Neither
// a marker file nor an archived/deleted job is cleanup authority on its own.
func (s *SQLiteStore) CheckLiquibaseProjectionCleanup(ctx context.Context, intent FinalizationSnapshot) error {
	if s == nil || s.db == nil {
		return ErrLiquibaseFinalization
	}
	tx, err := s.db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return err
	}
	defer tx.Rollback()
	saved, found, err := loadFinalization(ctx, tx, intent.JobID)
	if err != nil || !found || !sameFinalizationIntent(saved, intent) {
		return ErrLiquibaseFinalization
	}
	return validateFinalizationSelection(ctx, tx, saved.JobID, saved.Outcome)
}

// planningProjectionDigest binds the exact canonical public-plan bytes to the
// immutable planning intent. See liquibase-planning-recovery-storage-schema.md.
func planningProjectionDigest(encoded string) string {
	sum := sha256.Sum256([]byte(encoded))
	return "sha256:" + hex.EncodeToString(sum[:])
}

// projectedTask mirrors the public PlanTask JSON without allowing an
// executable status, output, credential, or arbitrary nested input field.
type projectedTask struct {
	TaskID      string `json:"task_id"`
	Type        string `json:"type"`
	PlannerKind string `json:"planner_kind,omitempty"`
	Input       *struct {
		Kind string `json:"kind"`
		ID   string `json:"id"`
	} `json:"input,omitempty"`
	ImageID         string `json:"image_id,omitempty"`
	ResolvedImageID string `json:"resolved_image_id,omitempty"`
	TaskHash        string `json:"task_hash,omitempty"`
	OutputStateID   string `json:"output_state_id,omitempty"`
	Cached          *bool  `json:"cached,omitempty"`
	InstanceMode    string `json:"instance_mode,omitempty"`
	ChangesetID     string `json:"changeset_id,omitempty"`
	ChangesetAuthor string `json:"changeset_author,omitempty"`
	ChangesetPath   string `json:"changeset_path,omitempty"`
}

// decodePlanningProjection accepts only the canonical public PlanTask shape
// and ordered plan/resolve/execute/instance sequence.
func decodePlanningProjection(encoded string) ([]projectedTask, bool) {
	decoder := json.NewDecoder(bytes.NewReader([]byte(encoded)))
	decoder.DisallowUnknownFields()
	var tasks []projectedTask
	if decoder.Decode(&tasks) != nil || len(tasks) < 3 {
		return nil, false
	}
	var extra any
	if decoder.Decode(&extra) != io.EOF {
		return nil, false
	}
	canonical, err := json.Marshal(tasks)
	if err != nil || string(canonical) != encoded {
		return nil, false
	}
	seen := map[string]bool{}
	executeCount := 0
	previousState := ""
	for i, task := range tasks {
		if task.TaskID == "" || seen[task.TaskID] {
			return nil, false
		}
		seen[task.TaskID] = true
		switch {
		case i == 0:
			if task.TaskID != "plan" || task.Type != "plan" || task.PlannerKind != "lb" {
				return nil, false
			}
		case i == len(tasks)-1:
			if task.TaskID != "prepare-instance" || task.Type != "prepare_instance" || task.Input == nil ||
				task.Input.Kind != "state" || task.Input.ID != previousState || task.InstanceMode != "ephemeral" {
				return nil, false
			}
		case task.Type == "resolve_image":
			if i != 1 || task.TaskID != "resolve-image" || task.ImageID == "" || task.ResolvedImageID == "" {
				return nil, false
			}
		case task.Type == "state_execute":
			if task.TaskID != fmt.Sprintf("execute-%d", executeCount) || task.Cached == nil || task.Input == nil || task.OutputStateID == "" || task.TaskHash == "" {
				return nil, false
			}
			if executeCount == 0 && (task.Input.Kind != "image" || task.Input.ID == "") ||
				executeCount > 0 && (task.Input.Kind != "state" || task.Input.ID != previousState) {
				return nil, false
			}
			previousState = task.OutputStateID
			executeCount++
		default:
			return nil, false
		}
	}
	if executeCount == 0 || tasks[len(tasks)-1].Input.ID != tasks[len(tasks)-2].OutputStateID {
		return nil, false
	}
	return tasks, true
}

func validPlanningProjection(encoded string) bool {
	_, valid := decodePlanningProjection(encoded)
	return valid
}

// SavePlanningProjection persists the validated plan-only status projection
// once; a repeated save must supply identical bytes. The executable queue is
// not extended. See A04b/P05b in liquibase-planning-recovery-tests.md.
func (s *SQLiteStore) SavePlanningProjection(ctx context.Context, jobID, encoded, at string) error {
	if s == nil || s.db == nil || jobID == "" || at == "" || !validPlanningProjection(encoded) {
		return ErrPlanningSchema
	}
	digest := planningProjectionDigest(encoded)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var savedJSON, savedDigest, savedAt sql.NullString
	var eligible int
	if err := tx.QueryRowContext(ctx, `SELECT i.plan_projection_json,i.plan_projection_digest,i.plan_projection_saved_at,
    (SELECT count(*) FROM prepare_jobs j JOIN prepare_tasks t ON t.job_id=j.job_id AND t.task_id='plan'
     WHERE j.job_id=i.job_id AND j.purpose='prepare' AND j.prepare_kind='lb' AND j.plan_only=1
       AND j.status IN ('running','succeeded') AND t.status='succeeded'
       AND NOT EXISTS(SELECT 1 FROM prepare_tasks other WHERE other.job_id=j.job_id AND other.task_id<>'plan'))
    FROM prepare_planning_intents i WHERE i.job_id=? AND i.plan_saved_at IS NOT NULL`, jobID).Scan(&savedJSON, &savedDigest, &savedAt, &eligible); err != nil || eligible != 1 {
		return ErrPlanningSchema
	}
	plan, found, err := loadPlanningResult(ctx, tx, jobID)
	if err != nil || !found {
		return ErrPlanningSchema
	}
	projected, _ := decodePlanningProjection(encoded)
	var steps []projectedTask
	for _, task := range projected {
		if task.Type == "state_execute" {
			steps = append(steps, task)
		}
	}
	if len(steps) != max(1, len(plan.Changesets)) {
		return ErrPlanningSchema
	}
	for i, changeset := range plan.Changesets {
		if steps[i].ChangesetID != changeset.ID || steps[i].ChangesetAuthor != changeset.Author || steps[i].ChangesetPath != changeset.Path {
			return ErrPlanningSchema
		}
	}
	if savedAt.Valid {
		if !savedJSON.Valid || !savedDigest.Valid || savedJSON.String != encoded || savedDigest.String != digest {
			return ErrPlanningSchema
		}
		return nil
	}
	if savedJSON.Valid || savedDigest.Valid {
		return ErrPlanningSchema
	}
	if _, err := tx.ExecContext(ctx, `UPDATE prepare_planning_intents SET plan_projection_json=?,plan_projection_digest=?,plan_projection_saved_at=? WHERE job_id=? AND plan_projection_saved_at IS NULL`, encoded, digest, at, jobID); err != nil {
		return err
	}
	return tx.Commit()
}

// LoadPlanningProjection validates persisted bytes and digest. An admitted
// job may have no planning intent during prebinding, and a verified plan may
// still await its one-time projection save. See A02/A05/P05b in
// liquibase-planning-recovery-tests.md.
func (s *SQLiteStore) LoadPlanningProjection(ctx context.Context, jobID string) (string, bool, error) {
	if s == nil || s.db == nil || jobID == "" {
		return "", false, ErrPlanningSchema
	}
	var encoded, digest, at sql.NullString
	if err := s.db.QueryRowContext(ctx, `SELECT plan_projection_json,plan_projection_digest,plan_projection_saved_at FROM prepare_planning_intents WHERE job_id=?`, jobID).Scan(&encoded, &digest, &at); errors.Is(err, sql.ErrNoRows) {
		return "", false, nil
	} else if err != nil {
		return "", false, ErrPlanningSchema
	}
	if !at.Valid && !encoded.Valid && !digest.Valid {
		return "", false, nil
	}
	if !at.Valid || !encoded.Valid || !digest.Valid || !validPlanningProjection(encoded.String) || planningProjectionDigest(encoded.String) != digest.String {
		return "", false, ErrPlanningSchema
	}
	return encoded.String, true, nil
}
