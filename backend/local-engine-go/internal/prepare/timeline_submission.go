package prepare

import (
	"context"
	"strings"
	"sync"

	"github.com/sqlrs/engine-local/internal/preparecost"
)

// submissionCost owns diagnostic scopes only, never job/resource authority.
// The in-memory prepared-request pointer links admission to its detached worker;
// no HTTP cancellation, secret or persistent binding is carried. Requirements:
// issue89-complete-timeline.md §2 and TL03.
type submissionCost struct {
	mu       sync.Mutex
	root     context.Context
	recorder *preparecost.Recorder
	emitter  *preparecost.Emitter
	end      func(preparecost.Outcome)
	pending  int
	outcome  preparecost.Outcome
}

func newSubmissionCost(recorder *preparecost.Recorder, emitter *preparecost.Emitter) *submissionCost {
	root, end := preparecost.Start(preparecost.WithRecorder(context.Background(), recorder), preparecost.Request)
	return &submissionCost{root: root, end: end, recorder: recorder, emitter: emitter, pending: 1, outcome: preparecost.Succeeded}
}

// beginSubmissionCost freezes selection at service startup. An unavailable
// selected clock produces incomplete diagnostics, not an admission error.
func (m *PrepareService) beginSubmissionCost(req Request) *submissionCost {
	if !m.costTimeline || m.costEmitter == nil || strings.TrimSpace(req.PrepareKind) != "lb" || m.containerLiquibase == nil {
		return nil
	}
	return newSubmissionCost(preparecost.NewTimelineRecorder(m.costClock, m.costSequence.Add(1)), m.costEmitter)
}

// reserveWorker runs before dispatch, so a fast worker cannot close admission's
// root before the real Submit scope finishes. There is one private dispatcher.
func (s *submissionCost) reserveWorker() {
	s.mu.Lock()
	s.pending++
	s.mu.Unlock()
}

// complete is called once by each scope after its span ends. Emission is
// diagnostic asynchronous work: a blocked private writer cannot block Submit.
func (s *submissionCost) complete(outcome preparecost.Outcome) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.outcome = submissionOutcome(s.outcome, outcome)
	s.pending--
	if s.pending == 0 {
		s.end(s.outcome)
		go func() { _ = s.emitter.Emit(s.recorder.Finish()) }()
	}
}

func submissionOutcome(current, next preparecost.Outcome) preparecost.Outcome {
	for _, outcome := range []preparecost.Outcome{preparecost.Panicked, preparecost.Failed, preparecost.Cancelled} {
		if current == outcome || next == outcome {
			return outcome
		}
	}
	return preparecost.Succeeded
}

// beginSubmissionWorker adopts the request's observation scope while retaining
// the worker's independent cancellation. Legacy calls retain their old root.
func (m *PrepareService) beginSubmissionWorker(ctx context.Context, p preparedRequest) (context.Context, func(preparecost.Outcome)) {
	if p.cost == nil {
		return m.beginPrepareCost(ctx)
	}
	child, end := preparecost.Start(preparecost.CarryScope(ctx, p.cost.root), preparecost.Worker)
	return child, func(outcome preparecost.Outcome) { end(outcome); p.cost.complete(outcome) }
}
