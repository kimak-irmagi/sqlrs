package prepare

import (
	"context"
	"strings"
	"testing"

	"github.com/sqlrs/engine-local/internal/instanceaccess"
	"github.com/sqlrs/engine-local/internal/prepare/queue"
)

func TestManagedResultProjection(t *testing.T) {
	m := newManager(t, &fakeStore{})
	m.access = &instanceaccess.Service{}
	if err := m.queue.CreateJob(context.Background(), queue.JobRecord{JobID: "job", Status: StatusRunning, PrepareKind: "psql", ImageID: "postgres:17", CreatedAt: "2026-09-16T00:00:00Z"}); err != nil {
		t.Fatal(err)
	}
	result := Result{DSN: "postgres://managed:secret-canary@127.0.0.1/db", InstanceID: "instance"}
	if err := m.appendEvent("job", Event{Type: "result", Result: &result}); err != nil {
		t.Fatal(err)
	}
	rows, err := m.queue.ListEventsSince(context.Background(), "job", 0)
	if err != nil || len(rows) != 1 {
		t.Fatal(err)
	}
	if rows[0].ResultJSON == nil || strings.Contains(*rows[0].ResultJSON, "secret-canary") {
		t.Fatal("DSN persisted in event")
	}
	if !strings.Contains(result.DSN, "secret-canary") {
		t.Fatal("projection changed caller's authorized result")
	}
}
