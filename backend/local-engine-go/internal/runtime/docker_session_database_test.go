package runtime

import (
	"context"
	"strings"
	"testing"
)

// N04/C04: session database presence uses its exact separately validated
// database name. Widening the executor-name validator would let a psql or
// planning recovery call inspect the wrong resource class.
func TestInspectNamedSessionDatabaseContainer(t *testing.T) {
	id := strings.Repeat("d", 64)
	name := "sqlrs-db-session-1"
	runner := &fakeRunner{responses: []runResponse{{output: id}, {output: `{"Id":"` + id + `","Name":"/` + name + `","State":{"Status":"running","ExitCode":0}}`}}}
	rt := NewDocker(Options{Binary: "docker", Runner: runner})
	view, found, err := rt.InspectNamedDatabaseContainer(context.Background(), name)
	if err != nil || !found || view.ID != id || view.Status != "running" {
		t.Fatalf("database view=%+v found=%t err=%v", view, found, err)
	}
	before := len(runner.calls)
	if _, _, err := rt.InspectNamedContainer(context.Background(), name); err == nil || len(runner.calls) != before {
		t.Fatal("executor inspector accepted a database runtime name")
	}
	if _, _, err := rt.InspectNamedDatabaseContainer(context.Background(), "sqlrs-step-other"); err == nil || len(runner.calls) != before {
		t.Fatal("database inspector accepted an executor name")
	}
}
