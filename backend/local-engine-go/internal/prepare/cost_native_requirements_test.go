package prepare

import (
	"bytes"
	"strconv"
	"strings"
	"testing"

	"github.com/sqlrs/engine-local/internal/deletion"
	"github.com/sqlrs/engine-local/internal/preparecost"
	storepkg "github.com/sqlrs/engine-local/internal/store"
)

// CT08/CT12: actual connected host boundaries, exact native data, reuse and
// product cleanup before the fixture's fallback. Run only in Actions.
func TestCostMainPathNamedAttributionDocker(t *testing.T) {
	t.Setenv("SQLRS_PREPARE_COST", "1")
	f := newMainPathNativeFixture(t)
	var output bytes.Buffer
	f.m.costEmitter = preparecost.NewEmitter(&output)
	counts := []map[string]int{
		{"prepare.planning": 1, "runner.inventory": 1, "prepare.session_startup": 1, "prepare.step": 3,
			"liquibase.invoke": 3, "capture.read": 3, "capture.prune": 2, "db.prepare_snapshot": 3,
			"db.resume.step": 2, "db.resume.publication": 1, "db.verify.after_apply": 3, "db.verify.after_resume": 2,
			"state.snapshot": 3, "state.size": 3, "state.seal": 3, "journal.state_publish": 3,
			"journal.invocation_plan": 3, "journal.invocation_admit": 3, "journal.invocation_complete": 3,
			"prepare.publication": 1, "prepare.final_cleanup": 1},
		{"prepare.planning": 1, "runner.inventory": 1, "prepare.session_startup": 0, "prepare.step": 0,
			"liquibase.invoke": 0, "capture.read": 0, "capture.prune": 0, "db.prepare_snapshot": 0,
			"db.resume.step": 0, "db.resume.publication": 0, "db.verify.after_apply": 0, "db.verify.after_resume": 0,
			"state.snapshot": 0, "state.size": 0, "state.seal": 0, "journal.state_publish": 0,
			"journal.invocation_plan": 0, "journal.invocation_admit": 0, "journal.invocation_complete": 0,
			"prepare.publication": 1, "prepare.final_cleanup": 1},
	}
	for pass := 0; pass < 2; pass++ {
		output.Reset()
		accepted, err := f.m.Submit(f.ctx, f.req)
		if err != nil {
			t.Fatal("measured native request refused")
		}
		status, found := f.m.Get(accepted.JobID)
		if !found || status.Status != StatusSucceeded || status.Result == nil {
			t.Fatal("measurement changed native prepare outcome")
		}
		mainPathAssertRows(t, f.ctx, status.Result.DSN, 3, 2)
		observed := map[string]int{}
		stepCalls := map[int]int{}
		begins, ends, observers := 0, 0, 0
		helperOwners := map[string]int{}
		ownerOperations := map[string]string{}
		for _, line := range strings.Split(strings.TrimSpace(output.String()), "\n") {
			fields := strings.Fields(line)
			if len(fields) < 3 || fields[0] != preparecost.WirePrefix[:len(preparecost.WirePrefix)-1] || fields[2] != strconv.Itoa(pass+1) {
				t.Fatal("native output crossed root or closed transport")
			}
			switch fields[1] {
			case "begin":
				begins++
				if len(fields) != 6 || fields[5] != "1" {
					t.Fatal("native measurement incomplete")
				}
			case "span":
				if len(fields) != 11 {
					t.Fatal("native source interval incomplete")
				}
				switch preparecost.Outcome(fields[10]) {
				case preparecost.Succeeded, preparecost.Failed, preparecost.Cancelled, preparecost.Panicked:
				default:
					t.Fatal("native source interval has an unknown outcome")
				}
				// Approved CT12 correction: readiness may retry a failed Docker
				// command. Preserve that actual result, not a fabricated success.
				// The worker and completed named operations must still succeed.
				if fields[5] != string(preparecost.DockerCommand) && fields[10] != string(preparecost.Succeeded) {
					t.Fatal("native mandatory completed operation failed")
				}
				observed[fields[5]]++
				ownerOperations[fields[3]] = fields[5]
				if fields[5] == "prepare.step" {
					ordinal, err := strconv.Atoi(fields[7])
					if err != nil || fields[6] != "step" {
						t.Fatal("native apply has no logical step")
					}
					stepCalls[ordinal]++
				}
			case "helper":
				if len(fields) != 8 || (ownerOperations[fields[3]] != "liquibase.plan_invoke" && ownerOperations[fields[3]] != "liquibase.invoke") {
					t.Fatal("native helper lacks exact invocation owner or complete phase")
				}
				phases := []string{"preflight", "cli", "drain", "stream_flush", "hash_rename", "renamed_flush", "manifest_flush", "final_flush"}
				index := helperOwners[fields[3]]
				if index >= len(phases) || fields[4] != phases[index] {
					t.Fatal("native helper phase order differs")
				}
				helperOwners[fields[3]]++
			case "observer":
				observers++
				if len(fields) != 6 || ends != 1 {
					t.Fatal("native observer does not follow complete report")
				}
				for _, value := range fields[3:] {
					if n, err := strconv.ParseInt(value, 10, 64); err != nil || n < 0 {
						t.Fatal("invalid native observer duration")
					}
				}
			case "end":
				ends++
			default:
				t.Fatal("native report contains an unreviewed record")
			}
		}
		if begins != 1 || ends != 1 || observed["prepare.worker"] != 1 || observed["docker.command"] == 0 {
			t.Fatal("native report lacks a complete worker/command tree")
		}
		wantOwners := 4
		if pass == 1 {
			wantOwners = 1
		}
		if observers != 1 || len(helperOwners) != wantOwners || observed["liquibase.plan_invoke"] != 1 {
			t.Fatal("native helper/observer owner counts differ")
		}
		for _, count := range helperOwners {
			if count != 8 {
				t.Fatal("native helper detail incomplete")
			}
		}
		for operation, count := range counts[pass] {
			if observed[operation] != count {
				t.Fatalf("native closed source %s count=%d want=%d", operation, observed[operation], count)
			}
		}
		if pass == 0 && (len(stepCalls) != 3 || stepCalls[1] != 1 || stepCalls[2] != 1 || stepCalls[3] != 1) || pass == 1 && len(stepCalls) != 0 {
			t.Fatal("native cached/per-step association differs")
		}
		instances, err := f.st.ListInstances(f.ctx, storepkg.InstanceFilters{})
		if err != nil || len(instances) != pass+1 {
			t.Fatal("native publication count differs")
		}
		kept := map[string]bool{}
		for _, instance := range instances {
			if instance.RuntimeID == nil {
				t.Fatal("native publication has no database")
			}
			kept[*instance.RuntimeID] = true
		}
		for _, id := range f.rt.allocated {
			if !kept[id] && f.rt.VerifyContainerAbsent(f.ctx, id) != nil {
				t.Fatal("native temporary container leaked before fixture cleanup")
			}
		}
	}
	if len(f.rt.runners) != 3 || len(f.rt.databases) != 3 {
		t.Fatal("recording changed CLI counts")
	}
	for i := range f.rt.runners {
		if f.rt.runners[i] != f.rt.runners[0] || f.rt.databases[i] != f.rt.databases[0] {
			t.Fatal("recording changed normal container reuse")
		}
	}
	manager, err := deletion.NewManager(deletion.Options{Store: f.st, Runtime: f.rt, StateFS: f.fs, Access: f.access, StateStoreRoot: f.root})
	if err != nil {
		t.Fatal(err)
	}
	instances, err := f.st.ListInstances(f.ctx, storepkg.InstanceFilters{})
	if err != nil {
		t.Fatal(err)
	}
	for _, instance := range instances {
		result, found, err := manager.DeleteInstance(f.ctx, instance.InstanceID, deletion.DeleteOptions{})
		if err != nil || !found || result.Outcome != deletion.OutcomeDeleted {
			t.Fatal("native public instance cleanup failed")
		}
	}
}
