package preparecost

import (
	"context"
	"sync"
	"time"
)

// Enabled is the exact private startup selector. The default debug level does
// not enable timing or make untrusted child environment a measurement authority.
func Enabled(value string) bool { return value == "1" }

// Report is a caller-owned snapshot. Complete=false never qualifies attribution;
// it is diagnostic state, not a replacement for the transformation's outcome.
type Report struct {
	Version   int
	Root      uint64
	ElapsedNS int64
	Complete  bool
	Spans     []Span
	Helpers   []HelperObservation
	Clock     *TimelineClockFrame
	clock     *TimelineClock
}

// TimelineClockFrame identifies only a shared-clock domain and relative origin.
// The host's absolute monotonic reference is never serialised.
type TimelineClockFrame struct {
	Domain   string
	OriginNS int64
}

// Recorder holds at most MaxSpans for one prepare. It uses monotonic elapsed
// time and no per-operation I/O. It owns neither SQL nor cleanup permissions.
// CT06/CT07: docs/architecture/issue89-cost-attribution-and-trimming.md §3.
type Recorder struct {
	mu         sync.Mutex
	root       uint64
	origin     time.Time
	spans      []Span
	ended      []bool
	lastEnd    int64
	finished   bool
	invalid    bool
	helpers    map[int]HelperObservation
	clock      *TimelineClock
	clockFrame *TimelineClockFrame
}

type scopeKey struct{}
type scope struct {
	recorder *Recorder
	parent   int
	phase    Phase
	step     int
}

// NewRecorder is allocation-free when disabled. Sequence zero is unqualified;
// the caller assigns positive process-local sequences, never job/container IDs.
func NewRecorder(enabled bool, sequence uint64) *Recorder {
	if !enabled {
		return nil
	}
	return &Recorder{root: sequence, origin: time.Now(), invalid: sequence == 0}
}

// NewTimelineRecorder is selected separately from the legacy worker recorder.
// A failed/missing common clock invalidates evidence, never application work.
func NewTimelineRecorder(clock *TimelineClock, sequence uint64) *Recorder {
	r := NewRecorder(true, sequence)
	origin, err := clock.Read()
	if err != nil {
		r.invalid = true
		return r
	}
	r.clock = clock
	r.clockFrame = &TimelineClockFrame{Domain: "linux-monotonic-relative-v1", OriginNS: origin}
	return r
}

// relativeNow is called under the recorder lock. Only selected timeline mode
// reads the common clock; legacy/default-off semantics remain unchanged.
func (r *Recorder) relativeNow() int64 {
	if r.clock == nil {
		return time.Since(r.origin).Nanoseconds()
	}
	now, err := r.clock.Read()
	if err != nil || now < r.clockFrame.OriginNS {
		r.invalid = true
		return r.lastEnd
	}
	return now - r.clockFrame.OriginNS
}

// WithRecorder attaches an observer without replacing context cancellation.
// Crossing existing roots is a diagnostic error, not a reparenting capability.
func WithRecorder(ctx context.Context, r *Recorder) context.Context {
	if r == nil {
		return ctx
	}
	if previous, ok := ctx.Value(scopeKey{}).(scope); ok {
		if previous.recorder == r {
			return ctx
		}
		r.invalidate()
	}
	return context.WithValue(ctx, scopeKey{}, scope{recorder: r, phase: PhaseWorker})
}

// CarryScope transfers only observation metadata onto the destination context.
// It never transfers HTTP cancellation/values to a detached worker. Foreign
// roots are refused rather than silently reparented (complete timeline §2).
func CarryScope(ctx, parent context.Context) context.Context {
	from, ok := parent.Value(scopeKey{}).(scope)
	if !ok || from.recorder == nil {
		return ctx
	}
	if previous, ok := ctx.Value(scopeKey{}).(scope); ok && previous.recorder != from.recorder {
		previous.recorder.invalidate()
		from.recorder.invalidate()
		return ctx
	}
	return context.WithValue(ctx, scopeKey{}, from)
}

// IsRecording describes the context's explicit selection, not report validity.
// No arbitrary environment value is forwarded to the protected helper.
func IsRecording(ctx context.Context) bool {
	current, ok := ctx.Value(scopeKey{}).(scope)
	return ok && current.recorder != nil
}

// WithPhase separates step ordinals from once-per-job work. Invalid phase data
// never enters a report; it invalidates qualification without changing ctx.Err.
func WithPhase(ctx context.Context, phase Phase, step int) context.Context {
	current, ok := ctx.Value(scopeKey{}).(scope)
	if !ok || current.recorder == nil {
		return ctx
	}
	if !validPhase(phase, step) {
		current.recorder.invalidate()
		return ctx
	}
	current.phase, current.step = phase, step
	return context.WithValue(ctx, scopeKey{}, current)
}

// Start opens a closed-catalog operation. Its finish function only observes:
// it never recovers a panic, returns a work error or repeats any external action.
// Disabled calls preserve the exact context and allocate no scope/closure.
func Start(ctx context.Context, operation Operation) (context.Context, func(Outcome)) {
	current, ok := ctx.Value(scopeKey{}).(scope)
	if !ok || current.recorder == nil {
		return ctx, finishNoop
	}
	r := current.recorder
	r.mu.Lock()
	defer r.mu.Unlock()
	_, known := Describe(operation)
	if r.finished || r.root == 0 || r.origin.IsZero() || !known || !validPhase(current.phase, current.step) || len(r.spans) >= MaxSpans {
		r.invalid = true
		return ctx, finishNoop
	}
	if current.parent != 0 {
		parent := r.spans[current.parent-1]
		if r.ended[current.parent-1] || parent.Step > 0 && (parent.Step != current.step || parent.Phase != current.phase) {
			r.invalid = true
			return ctx, finishNoop
		}
	}
	index := len(r.spans)
	r.spans = append(r.spans, Span{ID: index + 1, Parent: current.parent, Operation: operation,
		Phase: current.phase, Step: current.step, StartNS: r.relativeNow()})
	r.ended = append(r.ended, false)
	current.parent = index + 1
	child := context.WithValue(ctx, scopeKey{}, current)
	return child, func(outcome Outcome) { r.end(index, outcome) }
}

func finishNoop(Outcome) {}

func (r *Recorder) invalidate() {
	r.mu.Lock()
	r.invalid = true
	r.mu.Unlock()
}

func (r *Recorder) end(index int, outcome Outcome) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.finished || r.ended[index] || !validOutcome(outcome) {
		r.invalid = true
		return
	}
	span := &r.spans[index]
	span.EndNS, span.Outcome = r.relativeNow(), outcome
	r.ended[index] = true
	r.lastEnd = max(r.lastEnd, span.EndNS)
	if span.Parent != 0 {
		parent := r.spans[span.Parent-1]
		if r.ended[span.Parent-1] && span.EndNS > parent.EndNS {
			r.invalid = true
		}
	}
}

// Finish freezes observations and copies them without timing serialization/I/O
// as work. Repeated snapshots do not alias; rejected late writes can mark a later
// snapshot incomplete but cannot mutate a previously returned report's bytes.
func (r *Recorder) Finish() Report {
	if r == nil {
		return Report{}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.finished = true
	for _, ended := range r.ended {
		if !ended {
			r.invalid = true
		}
	}
	var helpers []HelperObservation
	// Span order makes snapshots deterministic even when branches finish in a
	// different order. All slices are owned by the returned snapshot (DT01).
	for _, span := range r.spans {
		if observation, found := r.helpers[span.ID]; found {
			if observation.HostElapsedNS > span.EndNS-span.StartNS {
				observation = HelperObservation{Owner: span.ID}
			}
			observation.Detail.Regions = append([]HelperRegion(nil), observation.Detail.Regions...)
			helpers = append(helpers, observation)
		}
	}
	var clockFrame *TimelineClockFrame
	if r.clockFrame != nil {
		copy := *r.clockFrame
		clockFrame = &copy
	}
	return Report{Version: 1, Root: r.root, ElapsedNS: r.lastEnd,
		Complete: !r.invalid && r.root != 0, Spans: append([]Span(nil), r.spans...), Helpers: helpers,
		Clock: clockFrame, clock: r.clock}
}
