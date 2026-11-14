package prepare

import (
	"context"

	"github.com/sqlrs/engine-local/internal/preparecost"
)

// beginPrepareCost grants no execution capability. It freezes a worker root
// before work, and emits only after its caller's final measured operation.
// Diagnostic failure never replaces prepare's outcome. CT06/CT07/CT10:
// docs/architecture/issue89-cost-attribution-and-trimming.md §3.
func (m *PrepareService) beginPrepareCost(ctx context.Context) (context.Context, func(preparecost.Outcome)) {
	if m.costEmitter == nil {
		return ctx, finishPrepareCostNoop
	}
	r := preparecost.NewRecorder(true, m.costSequence.Add(1))
	child, end := preparecost.Start(preparecost.WithRecorder(ctx, r), preparecost.Worker)
	return child, func(outcome preparecost.Outcome) {
		end(outcome)
		_ = m.costEmitter.Emit(r.Finish())
	}
}

func finishPrepareCostNoop(preparecost.Outcome) {}
