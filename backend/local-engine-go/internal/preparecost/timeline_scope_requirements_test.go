package preparecost

import (
	"context"
	"testing"
)

// TL03: carry only measurement scope, never the HTTP cancellation or values.
func TestTimelineScopeCarryPreservesDestinationContext(t *testing.T) {
	r := NewRecorder(true, 1)
	root, closeRoot := Start(WithRecorder(context.Background(), r), Request)
	http, cancel := context.WithCancel(context.Background())
	cancel()
	admission, closeAdmission := Start(WithPhase(CarryScope(http, root), PhaseAdmission, 0), Admission)
	if admission.Err() != context.Canceled {
		t.Fatal("HTTP cancellation was discarded")
	}
	worker, closeWorker := Start(CarryScope(context.Background(), root), Worker)
	if worker.Err() != nil {
		t.Fatal("detached worker inherited HTTP cancellation")
	}
	closeWorker(Succeeded)
	closeAdmission(Cancelled)
	closeRoot(Succeeded)
	report := r.Finish()
	if !report.Complete || len(report.Spans) != 3 || report.Spans[1].Parent != 1 || report.Spans[2].Parent != 1 {
		t.Fatal("admission/worker lost the same request parent")
	}
	plain := context.Background()
	if CarryScope(plain, plain) != plain {
		t.Fatal("disabled carry allocated a context")
	}
	other := NewRecorder(true, 2)
	target := WithRecorder(plain, other)
	if CarryScope(target, root) != target || other.Finish().Complete {
		t.Fatal("foreign root silently adopted another scope")
	}
}
