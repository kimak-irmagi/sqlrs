package queue_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/sqlrs/engine-local/internal/prepare/queue"
)

// CP-PG03/PG06: persisted cached flags without real published state provenance
// cannot authorise deletion through either the writer or direct SQL. A direct
// caller must not bypass the reason-specific writer's existing cache checks.
func TestPlanningDBCacheFlagsDoNotGrantDisposal(t *testing.T) {
	for _, reason := range []string{"full_cache", "source_mismatch"} {
		t.Run(reason, func(t *testing.T) {
			f := newPlanningDBFixture(t, 0)
			f.complete(t)
			f.retireCapture(t)
			launch := f.firstLaunch(t)
			binding := queue.ExecutionBinding{Digest: launch.ExecutionDigest, PrepareKind: "lb", CommandDigest: pgDigest("9"), InputManifestDigest: f.record.BundleManifestDigest, ExecutorImageDigest: f.record.ImageManifestDigest, TargetIdentityDigest: f.record.TargetIdentityDigest, PolicyRevision: "lb-container-v2", EffectScope: "unknown", AdapterBindingDigest: f.record.ExecutionFingerprint}
			for _, task := range []string{"two", "three"} {
				handoffCheck(t, f.q.BindStep(t.Context(), f.attempt.JobID, task, binding, mainPathJournalUTC))
			}
			cached, succeeded, running := true, "succeeded", "running"
			handoffCheck(t, f.q.UpdateTask(t.Context(), f.attempt.JobID, "one", queue.TaskUpdate{Status: &succeeded, Cached: &cached}))
			observation := map[string]any{"retained_evidence_id": f.retained, "plan_digest": f.planDigest, "reason": reason, "authority": f.planDigest}
			if reason == "full_cache" {
				for _, task := range []string{"two", "three"} {
					handoffCheck(t, f.q.UpdateTask(t.Context(), f.attempt.JobID, task, queue.TaskUpdate{Status: &succeeded, Cached: &cached}))
				}
				observation["cached_states"] = []string{strings.Repeat("5", 64), strings.Repeat("6", 64), strings.Repeat("7", 64)}
			} else {
				handoffCheck(t, f.q.UpdateTask(t.Context(), f.attempt.JobID, "two", queue.TaskUpdate{Status: &running}))
				observation["first_task_id"] = "two"
				observation["first_execution_digest"] = launch.ExecutionDigest
				observation["input_kind"] = "state"
				observation["input_id"] = strings.Repeat("5", 64)
			}
			before := mainPathJournalRows(t, f.db)
			if id, e := f.q.RequestPlanningDBDisposal(t.Context(), f.attempt, 1, reason, mainPathJournalUTC); e == nil || id != 0 {
				t.Fatal("cached flags authorised physical cleanup", id, e)
			}
			f.unchanged(t, before)
			raw, e := json.Marshal(map[string]any{"clone_id": f.clone.ID, "execution_digest": f.attempt.ExecutionDigest, "observation": observation})
			handoffCheck(t, e)
			var scope string
			handoffCheck(t, f.db.QueryRow("SELECT scope_digest FROM prepare_attempt_evidence WHERE evidence_id=?", f.retained).Scan(&scope))
			inserted, e := f.db.Exec(`INSERT INTO prepare_attempt_evidence(job_id,task_id,attempt_no,kind,scope_digest,facts_json,facts_digest,worker_generation,observed_at) VALUES(?,'plan',1,'planning_db_disposal_requested',?,?,?,1,?)`, f.attempt.JobID, scope, string(raw), planningDBFactDigest(string(raw)), mainPathJournalUTC)
			handoffCheck(t, e)
			id, e := inserted.LastInsertId()
			handoffCheck(t, e)
			before = mainPathJournalRows(t, f.db)
			if _, e = f.db.Exec("UPDATE prepare_liquibase_planning_db_ownership SET disposal_requested_evidence_id=? WHERE job_id=?", id, f.attempt.JobID); e == nil {
				t.Fatal("direct SQL accepted unproved cached source")
			}
			f.unchanged(t, before)
			// The exact same owner remains disposable under real saved cancellation.
			_, e = f.q.RequestJobCancellation(t.Context(), f.attempt.JobID, mainPathJournalUTC)
			handoffCheck(t, e)
			_, e = f.q.RequestPlanningDBDisposal(t.Context(), f.attempt, 1, "accepted_cancel", mainPathJournalUTC)
			handoffCheck(t, e)
		})
	}
}
