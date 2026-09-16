package prepare

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	engineRuntime "github.com/sqlrs/engine-local/internal/runtime"
)

type cleanupContextRuntime struct {
	fakeRuntime
	t *testing.T
}

func (r *cleanupContextRuntime) Stop(ctx context.Context, id string) error {
	if ctx.Err() != nil {
		r.t.Error("cleanup inherited job cancellation", ctx.Err())
	}
	deadline, bounded := ctx.Deadline()
	if !bounded || time.Until(deadline) > 15*time.Second {
		r.t.Error("cleanup must retain its bounded deadline")
	}
	return r.fakeRuntime.Stop(ctx, id)
}

func TestManagedCleanupAfterCancellation(t *testing.T) {
	m, p, db, _, _ := managedExecutionFixture(t)
	op, err := m.access.Assign(context.Background(), "operation", filepath.Join(m.stateStoreRoot, "clone"), "source", p.managed.IdentityBinding)
	if err != nil {
		t.Fatal(err)
	}
	runtime := &cleanupContextRuntime{t: t}
	m.runtime = runtime
	removed := false
	runner := &jobRunner{rt: &jobRuntime{operation: op, instance: engineRuntime.Instance{ID: "container"}, cleanup: func() error {
		if len(runtime.stopCalls) != 1 {
			t.Error("clone removed before confirmed stop")
		}
		removed = true
		return nil
	}}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := m.cleanupRuntime(ctx, runner); err != nil {
		t.Fatal("cancelled job cleanup failed", err)
	}
	if !removed || runner.getRuntime() != nil || len(runtime.stopCalls) != 1 {
		t.Fatal("cancelled job retained its runtime or clone")
	}
	var stage string
	if err := db.QueryRow("SELECT stage FROM managed_runtime_operations WHERE operation_ref=?", op.Ref).Scan(&stage); err != nil || stage != "retired" {
		t.Fatal("operation not durably retired", stage, err)
	}
}
