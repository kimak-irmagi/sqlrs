package prepare

import (
	"bytes"
	"context"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sqlrs/engine-local/internal/preparecost"
)

type timelineBlockingWriter struct {
	first            sync.Once
	entered, release chan struct{}
	output           bytes.Buffer
}

func (w *timelineBlockingWriter) Write(data []byte) (int, error) {
	w.first.Do(func() { close(w.entered); <-w.release })
	return w.output.Write(data)
}

// TL03's lifecycle test uses the unchanged legacy recorder solely to observe
// scope ownership on any OS. Actual common-clock/native proof remains separate.
func TestTimelineSubmissionScopeDoesNotBlockAdmissionForDelivery(t *testing.T) {
	for _, workerFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "admission-first", true: "worker-first"}[workerFirst], func(t *testing.T) {
			writer := &timelineBlockingWriter{entered: make(chan struct{}), release: make(chan struct{})}
			defer close(writer.release)
			scope := newSubmissionCost(preparecost.NewRecorder(true, 1), preparecost.NewEmitter(writer))
			scope.reserveWorker()
			admissionCtx, admissionEnd := preparecost.Start(preparecost.CarryScope(context.Background(), scope.root), preparecost.Admission)
			workerCtx, workerEnd := preparecost.Start(preparecost.CarryScope(context.Background(), scope.root), preparecost.Worker)
			if admissionCtx.Err() != nil || workerCtx.Err() != nil {
				t.Fatal("measurement changed context")
			}
			first, last := admissionEnd, workerEnd
			if workerFirst {
				first, last = workerEnd, admissionEnd
			}
			first(preparecost.Succeeded)
			scope.complete(preparecost.Succeeded)
			select {
			case <-writer.entered:
				t.Fatal("report emitted before both scopes finished")
			default:
			}
			returned := make(chan struct{})
			go func() { last(preparecost.Succeeded); scope.complete(preparecost.Succeeded); close(returned) }()
			ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			select {
			case <-writer.entered:
			case <-ctx.Done():
				t.Fatal("report was not delivered")
			}
			select {
			case <-returned:
			case <-ctx.Done():
				t.Fatal("admission/worker return waits for blocked diagnostic writer")
			}
		})
	}
}

// The real Submit rejection path must retain its validation result even when
// the selected common clock is unavailable on the test host.
func TestTimelineSubmissionInvalidRequestKeepsValidation(t *testing.T) {
	t.Setenv("SQLRS_PREPARE_COST", "1")
	t.Setenv("SQLRS_PREPARE_TIMELINE", "1")
	t.Setenv("SQLRS_PREPARE_TIMELINE_REFERENCE", "invalid-private-reference")
	m := newManager(t, &fakeStore{})
	m.containerLiquibase = &LiquibaseContainerDeps{}
	writer := &timelineBlockingWriter{entered: make(chan struct{}), release: make(chan struct{})}
	close(writer.release)
	m.costEmitter = preparecost.NewEmitter(writer)
	_, err := m.Submit(t.Context(), Request{PrepareKind: "lb"})
	if err == nil {
		t.Fatal("trace changed validation rejection into success")
	}
	ctx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancel()
	select {
	case <-writer.entered:
	case <-ctx.Done():
		t.Fatal("actual Submit did not finish its failed observation")
	}
	if strings.Contains(err.Error(), "invalid-private-reference") {
		t.Fatal("private tracing state escaped into validation")
	}
}
