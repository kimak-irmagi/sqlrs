package prepare

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"

	"github.com/sqlrs/engine-local/internal/deletion"
	"github.com/sqlrs/engine-local/internal/prepare/queue"
	engineRuntime "github.com/sqlrs/engine-local/internal/runtime"
	storepkg "github.com/sqlrs/engine-local/internal/store"
)

// PG05: an obsolete planner must be rejected before reaching any destructive
// observation, including after normal session/instance retirement.
type planningDBStaleCleanupProbe struct{ calls int }

func (v *planningDBStaleCleanupProbe) VerifyReleasedClone(context.Context, queue.ClonePlan, string) error {
	v.calls++
	return errors.New("obsolete planner reached physical cleanup observation")
}

// PG09 counts actual managed DB allocations, not invocation references or helper
// containers. All calls delegate to the real runtime; no fake DB is substituted.
type planningDBNativeRuntime struct {
	*mainPathNativeRuntime
	starts []engineRuntime.Instance
}

func (r *planningDBNativeRuntime) StartManaged(ctx context.Context, request engineRuntime.ManagedStartRequest) (engineRuntime.Instance, error) {
	instance, err := r.mainPathNativeRuntime.StartManaged(ctx, request)
	if err == nil {
		r.starts = append(r.starts, instance)
	}
	return instance, err
}

// PG09: genuine stock planner + three stock JVM calls + snapshots, using the
// SAME live planning/working/published database. Explicit Actions opt-in only.
// Requirements: issue89-planning-db-handoff-tests.md PG05/07/09.
func TestPlanningDBHandoffDocker(t *testing.T) {
	f := newMainPathNativeFixture(t)
	runtime := &planningDBNativeRuntime{mainPathNativeRuntime: f.rt}
	f.m.runtime = runtime
	var previewID, cloneRef, source, physical string
	f.rt.beforeInvoke = func(ctx context.Context, id engineRuntime.StepCaptureIdentity) {
		var dbID, ref, opSource, opPhysical string
		var generation int
		err := f.rt.db.QueryRowContext(ctx, `SELECT c.db_runtime_id,c.clone_ref,c.clone_generation,op.source_ref,op.physical_identity
FROM prepare_liquibase_planning_db_ownership owner
JOIN prepare_clones c ON c.clone_id=owner.clone_id
JOIN prepare_liquibase_sessions s ON s.session_id=owner.session_id AND s.clone_id=c.clone_id
JOIN managed_runtime_operations op ON op.runtime_ref=c.db_runtime_id
WHERE owner.job_id=? AND owner.session_id=? AND owner.transferred_at IS NOT NULL
AND owner.disposal_requested_evidence_id IS NULL AND c.phase='active' AND op.stage='running'`, id.JobID, id.SessionID).Scan(&dbID, &ref, &generation, &opSource, &opPhysical)
		if err != nil || generation != 1 || len(runtime.starts) != 1 || dbID != runtime.starts[0].ID {
			t.Fatal("first SQL lacks exact committed preview ownership or allocated another DB", err)
		}
		if previewID == "" {
			previewID, cloneRef, source, physical = dbID, ref, opSource, opPhysical
		}
		if dbID != previewID || ref != cloneRef || opSource != source || opPhysical != physical {
			t.Fatal("working step changed the planning DB/path/Source/physical identity")
		}
		var ready, retiredCapture int
		if err := f.rt.db.QueryRowContext(ctx, "SELECT count(*) FROM prepare_liquibase_session_readiness WHERE session_id=? AND runner_id=?", id.SessionID, id.RunnerID).Scan(&ready); err != nil {
			t.Fatal(err)
		}
		if err := f.rt.db.QueryRowContext(ctx, `SELECT count(*) FROM prepare_planning_attempt_artifacts WHERE job_id=?
AND executor_retired_at IS NOT NULL AND protected_input_retired_at IS NOT NULL AND output_retired_at IS NOT NULL
AND stderr_retired_at IS NOT NULL AND manifest_retired_at IS NOT NULL`, id.JobID).Scan(&retiredCapture); err != nil {
			t.Fatal(err)
		}
		if ready != 1 || retiredCapture != 1 {
			t.Fatal("SQL preceded runner readiness or private planner cleanup")
		}
	}
	accepted, err := f.m.Submit(f.ctx, f.req)
	if err != nil {
		t.Fatal(err)
	}
	status, found := f.m.Get(accepted.JobID)
	if !found || status.Status != StatusSucceeded || status.Result == nil {
		t.Fatal("handoff prepare did not succeed", status.Error)
	}
	mainPathAssertRows(t, f.ctx, status.Result.DSN, 3, 2)
	if len(runtime.starts) != 1 || len(f.rt.databases) != 3 || len(f.rt.runners) != 3 || previewID == "" {
		t.Fatal("physical DB count and stock SQL invocation count differ")
	}
	for i := range f.rt.databases {
		if f.rt.databases[i] != previewID || f.rt.runners[i] != f.rt.runners[0] {
			t.Fatal("a working step replaced DB/runner")
		}
	}
	instances, err := f.st.ListInstances(f.ctx, storepkg.InstanceFilters{})
	if err != nil || len(instances) != 1 || instances[0].RuntimeID == nil || *instances[0].RuntimeID != previewID {
		t.Fatal("publication replaced the retained DB", err)
	}
	assertTemporaryAbsent := func(allowed map[string]bool) {
		t.Helper()
		for _, id := range f.rt.allocated {
			if !allowed[id] && f.rt.VerifyContainerAbsent(f.ctx, id) != nil {
				t.Fatal("temporary container remains before fixture fallback")
			}
		}
		var held int
		if err := f.rt.db.QueryRowContext(f.ctx, "SELECT count(*) FROM prepare_liquibase_capture_slots").Scan(&held); err != nil || held != 0 {
			t.Fatal("private output slots remain", err)
		}
	}
	assertTemporaryAbsent(map[string]bool{previewID: true})
	preview, found, err := f.q.LoadPlanningAttempt(f.ctx, accepted.JobID, 1)
	if err != nil || !found {
		t.Fatal("published transfer lost planning history", err)
	}
	stale := &planningDBStaleCleanupProbe{}
	if err := f.q.ReleasePlanningClone(f.ctx, preview.Attempt, 1, stale); err == nil || stale.calls != 0 {
		t.Fatal("old planning reference reclaimed normally released session", err)
	}
	// Fully cached prepare still needs its independent preview and publication
	// DBs, and does not transfer the preview merely because it is healthy.
	f.rt.beforeInvoke = nil
	before := len(runtime.starts)
	second, err := f.m.Submit(f.ctx, f.req)
	if err != nil {
		t.Fatal(err)
	}
	hit, found := f.m.Get(second.JobID)
	if !found || hit.Status != StatusSucceeded || hit.Result == nil || len(runtime.starts)-before != 2 || len(f.rt.databases) != 3 {
		t.Fatal("full cache used incorrect reuse/SQL/allocation path", hit.Error)
	}
	mainPathAssertRows(t, f.ctx, hit.Result.DSN, 3, 2)
	var disposed string
	var transferred sql.NullString
	if err := f.rt.db.QueryRowContext(f.ctx, "SELECT disposed_at,session_id FROM prepare_liquibase_planning_db_ownership WHERE job_id=?", second.JobID).Scan(&disposed, &transferred); err != nil || disposed == "" || transferred.Valid {
		t.Fatal("full-cache preview was not disposed independently", err)
	}
	instances, err = f.st.ListInstances(f.ctx, storepkg.InstanceFilters{})
	if err != nil || len(instances) != 2 {
		t.Fatal("full-cache publication missing", err)
	}
	allowed := map[string]bool{}
	for _, instance := range instances {
		if instance.RuntimeID != nil {
			allowed[*instance.RuntimeID] = true
		}
	}
	assertTemporaryAbsent(allowed)
	deletes, err := deletion.NewManager(deletion.Options{Store: f.st, Runtime: runtime, StateFS: f.fs, Access: f.access, StateStoreRoot: f.root})
	if err != nil {
		t.Fatal(err)
	}
	for _, instance := range instances {
		result, found, err := deletes.DeleteInstance(f.ctx, instance.InstanceID, deletion.DeleteOptions{})
		if err != nil || !found || result.Outcome != deletion.OutcomeDeleted {
			t.Fatal("historical transfer pinned a published instance", err)
		}
	}
	if err := f.q.ReleasePlanningClone(f.ctx, preview.Attempt, 1, stale); err == nil || stale.calls != 0 {
		t.Fatal("instance removal revived obsolete planning cleanup authority", err)
	}
	result, found, err := deletes.DeleteState(f.ctx, status.Result.StateID, deletion.DeleteOptions{})
	if err != nil || !found || result.Outcome != deletion.OutcomeDeleted {
		t.Fatal("cannot remove final state for real prefix control", err)
	}
	// Same recipe, SQL3 missing: the preview's image cursor cannot stand in for
	// the cached SQL2 cursor. Exactly one JVM, but TWO actual DB allocations.
	before = len(runtime.starts)
	partial, err := f.m.Submit(f.ctx, f.req)
	if err != nil {
		t.Fatal(err)
	}
	resumed, found := f.m.Get(partial.JobID)
	if !found || resumed.Status != StatusSucceeded || resumed.Result == nil || len(runtime.starts)-before != 2 || len(f.rt.databases) != 4 {
		t.Fatal("cached prefix adopted the wrong DB cursor", resumed.Error)
	}
	mainPathAssertRows(t, f.ctx, resumed.Result.DSN, 3, 2)
	if f.rt.databases[3] == runtime.starts[before].ID {
		t.Fatal("cached prefix reused its empty planning DB")
	}
	if err := f.rt.db.QueryRowContext(f.ctx, "SELECT disposed_at,session_id FROM prepare_liquibase_planning_db_ownership WHERE job_id=?", partial.JobID).Scan(&disposed, &transferred); err != nil || disposed == "" || transferred.Valid {
		t.Fatal("cached-prefix preview ownership was not disposed", err)
	}
	instances, err = f.st.ListInstances(f.ctx, storepkg.InstanceFilters{})
	if err != nil || len(instances) != 1 || instances[0].RuntimeID == nil {
		t.Fatal("prefix result missing", err)
	}
	assertTemporaryAbsent(map[string]bool{*instances[0].RuntimeID: true})
	result, found, err = deletes.DeleteInstance(f.ctx, resumed.Result.InstanceID, deletion.DeleteOptions{})
	if err != nil || !found || result.Outcome != deletion.OutcomeDeleted {
		t.Fatal("prefix result deletion failed", err)
	}
	// Parent deletion must cascade immutable handoff history independently of
	// sibling order; it must not keep the removed operation/instance alive.
	for _, job := range []string{accepted.JobID, second.JobID, partial.JobID} {
		if err := f.q.DeleteJob(f.ctx, job); err != nil {
			t.Fatal("terminal handoff history blocked job deletion", err)
		}
	}
	if strings.TrimSpace(source) == "" || cloneRef == "" || physical == "" {
		t.Fatal("native identity oracle was never exercised")
	}
	assertTemporaryAbsent(map[string]bool{})
}
