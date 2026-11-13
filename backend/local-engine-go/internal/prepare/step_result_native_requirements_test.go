package prepare

import (
	"bytes"
	"strings"
	"testing"

	"github.com/sqlrs/engine-local/internal/deletion"
	"github.com/sqlrs/engine-local/internal/preparecost"
	storepkg "github.com/sqlrs/engine-local/internal/store"
)

// IR09: real three-step prepare, not a shell surrogate. Each stock CLI call
// has exactly inspect/exec-result/inspect; cleanup precedes fixture fallback.
func TestStepResultMainPathNativeDocker(t *testing.T) {
	t.Setenv("SQLRS_PREPARE_COST", "1")
	f := newMainPathNativeFixture(t)
	var output bytes.Buffer
	f.m.costEmitter = preparecost.NewEmitter(&output)
	accepted, err := f.m.Submit(f.ctx, f.req)
	if err != nil {
		t.Fatal(err)
	}
	status, found := f.m.Get(accepted.JobID)
	if !found || status.Status != StatusSucceeded || status.Result == nil {
		t.Fatal("real result-mode prepare failed")
	}
	mainPathAssertRows(t, f.ctx, status.Result.DSN, 3, 2)
	owners, commands, operations := map[string]bool{}, map[string]int{}, map[string]int{}
	for _, line := range strings.Split(output.String(), "\n") {
		fields := strings.Fields(line)
		if len(fields) != 11 || fields[1] != "span" {
			continue
		}
		operations[fields[5]]++
		if fields[5] == "liquibase.invoke" {
			owners[fields[3]] = true
		}
		if fields[5] == "docker.command" {
			commands[fields[4]]++
		}
	}
	if len(owners) != 3 || operations["capture.result_decode"] != 3 || operations["capture.read"] != 0 || operations["docker.capture_copy"] != 0 || operations["capture.prune"] != 0 {
		t.Fatal("real inline path retained acquisition round trips", operations)
	}
	for owner := range owners {
		if commands[owner] != 3 {
			t.Fatal("step did not use exactly three native commands", commands[owner])
		}
	}
	if len(f.rt.runners) != 3 || len(f.rt.databases) != 3 {
		t.Fatal("SQL invocation count changed")
	}
	for i := range f.rt.runners {
		if f.rt.runners[i] != f.rt.runners[0] || f.rt.databases[i] != f.rt.databases[0] {
			t.Fatal("containers not reused")
		}
	}
	instances, err := f.st.ListInstances(f.ctx, storepkg.InstanceFilters{})
	if err != nil || len(instances) != 1 || instances[0].RuntimeID == nil || *instances[0].RuntimeID != f.rt.databases[2] {
		t.Fatal("working DB was not published intact", err)
	}
	for _, id := range f.rt.allocated {
		if id != *instances[0].RuntimeID && f.rt.VerifyContainerAbsent(f.ctx, id) != nil {
			t.Fatal("temporary native resource leaked")
		}
	}
	manager, err := deletion.NewManager(deletion.Options{Store: f.st, Runtime: f.rt, StateFS: f.fs, Access: f.access, StateStoreRoot: f.root})
	if err != nil {
		t.Fatal(err)
	}
	result, found, err := manager.DeleteInstance(f.ctx, instances[0].InstanceID, deletion.DeleteOptions{})
	if err != nil || !found || result.Outcome != deletion.OutcomeDeleted {
		t.Fatal("public instance cleanup failed", err)
	}
}
