package prepare

import (
	"os"
	"path/filepath"
	"testing"

	storepkg "github.com/sqlrs/engine-local/internal/store"
)

// CPR06/08: the real normal caller must preserve a known failing second
// changeset even after native proof releases the published first output.
func TestCombinedCapturePruneKnownFailureDocker(t *testing.T) {
	f := newMainPathNativeFixture(t)
	project := filepath.Join(f.root, "combined-known-failure")
	if err := os.Mkdir(project, 0700); err != nil {
		t.Fatal(err)
	}
	const changes = "--liquibase formatted sql\n--changeset sqlrs:combined-first\nCREATE TABLE combined_first(id int);\n--changeset sqlrs:combined-failure\nINSERT INTO absent_combined_table VALUES(1);\n--changeset sqlrs:must-not-run\nCREATE TABLE must_not_run(id int);\n"
	if err := os.WriteFile(filepath.Join(project, "changelog.sql"), []byte(changes), 0600); err != nil {
		t.Fatal(err)
	}
	req := f.req
	req.WorkDir = project
	accepted, err := f.m.Submit(f.ctx, req)
	if err != nil {
		t.Fatal("native combined failure request refused")
	}
	status, found := f.m.Get(accepted.JobID)
	if !found || status.Status != StatusFailed || status.Result != nil || status.Error == nil || len(f.rt.runners) != 2 || len(f.rt.databases) != 2 || f.rt.runners[0] != f.rt.runners[1] || f.rt.databases[0] != f.rt.databases[1] {
		t.Fatal("combined proof hid failure, repeated SQL or replaced its working containers")
	}
	var successes, verified int
	if err := f.rt.db.QueryRowContext(f.ctx, "SELECT count(*) FROM prepare_tasks WHERE job_id=? AND type='state_execute' AND status='succeeded'", accepted.JobID).Scan(&successes); err != nil {
		t.Fatal(err)
	}
	if err := f.rt.db.QueryRowContext(f.ctx, "SELECT count(*) FROM prepare_liquibase_capture_retirements WHERE job_id=? AND reason='published_prune' AND verified_at IS NOT NULL AND absence_kind='path_absent'", accepted.JobID).Scan(&verified); err != nil {
		t.Fatal(err)
	}
	if successes != 1 || verified != 1 {
		t.Fatal("known second failure lost first publication or native prior-disposal proof")
	}
	// Final session cleanup is a different authority from the native published
	// prune above. It must also finish, not inflate the published-prune counter.
	var finalRecords, unretired, slots, disposals int
	if err := f.rt.db.QueryRowContext(f.ctx, "SELECT count(*) FROM prepare_liquibase_capture_retirements WHERE job_id=? AND reason='session_disposal'", accepted.JobID).Scan(&finalRecords); err != nil {
		t.Fatal(err)
	}
	if err := f.rt.db.QueryRowContext(f.ctx, "SELECT count(*) FROM prepare_liquibase_capture_retirements WHERE job_id=? AND (verified_at IS NULL OR absence_kind IS NULL)", accepted.JobID).Scan(&unretired); err != nil {
		t.Fatal(err)
	}
	if err := f.rt.db.QueryRowContext(f.ctx, "SELECT count(*) FROM prepare_liquibase_capture_slots WHERE job_id=?", accepted.JobID).Scan(&slots); err != nil {
		t.Fatal(err)
	}
	if err := f.rt.db.QueryRowContext(f.ctx, "SELECT count(*) FROM prepare_liquibase_session_disposals d JOIN prepare_liquibase_sessions s ON s.session_id=d.session_id WHERE s.job_id=? AND d.db_absent_at IS NOT NULL AND d.runner_absent_at IS NOT NULL", accepted.JobID).Scan(&disposals); err != nil {
		t.Fatal(err)
	}
	if finalRecords == 0 || unretired != 0 || slots != 0 || disposals != 1 {
		t.Fatal("known failure left incomplete final disposal or capture owners")
	}
	instances, err := f.st.ListInstances(f.ctx, storepkg.InstanceFilters{})
	if err != nil || len(instances) != 0 {
		t.Fatal("failed combined invocation published an instance")
	}
	for _, id := range f.rt.allocated {
		if err := f.rt.VerifyContainerAbsent(f.ctx, id); err != nil {
			t.Fatal("known combined failure leaked container before fallback")
		}
	}
}
