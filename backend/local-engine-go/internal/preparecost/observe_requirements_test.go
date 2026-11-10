package preparecost

import (
	"context"
	"errors"
	"testing"
)

// CT07/CT08: observation preserves the exact callback count, value/error/panic
// and cancellation. Invalid diagnostics never acquire work/retry authority.
func TestObservePreservesWorkOutcomes(t *testing.T) {
	for _, variant := range []string{"off", "success", "failure", "cancel", "deadline", "panic", "invalid-key"} {
		t.Run(variant, func(t *testing.T) {
			r := NewRecorder(variant != "off", 1)
			ctx := WithRecorder(t.Context(), r)
			op := CaptureRead
			if variant == "invalid-key" {
				op = "private-key"
			}
			marker := errors.New("original-private-error")
			var workErr error
			wantOutcome := Succeeded
			switch variant {
			case "failure":
				workErr, wantOutcome = marker, Failed
			case "cancel":
				workErr, wantOutcome = context.Canceled, Cancelled
			case "deadline":
				workErr, wantOutcome = context.DeadlineExceeded, Cancelled
			case "panic":
				wantOutcome = Panicked
			}
			calls, panics := 0, 0
			func() {
				defer func() {
					if got := recover(); got != nil {
						panics++
						if variant != "panic" || got != marker {
							t.Fatal("observation replaced panic")
						}
					}
				}()
				value, err := Observe(ctx, op, func(actual context.Context) (int, error) {
					calls++
					if actual.Err() != ctx.Err() {
						t.Fatal("observation replaced cancellation")
					}
					if variant == "panic" {
						panic(marker)
					}
					return 42, workErr
				})
				if value != 42 || err != workErr {
					t.Fatal("observation replaced work result")
				}
			}()
			if calls != 1 || (panics == 1) != (variant == "panic") {
				t.Fatal("observation repeated or swallowed work")
			}
			report := r.Finish()
			if variant == "off" {
				if len(report.Spans) != 0 {
					t.Fatal("disabled observation recorded work")
				}
			} else if variant == "invalid-key" {
				if report.Complete || len(report.Spans) != 0 {
					t.Fatal("invalid key qualified or leaked")
				}
			} else if !report.Complete || len(report.Spans) != 1 || report.Spans[0].Outcome != wantOutcome {
				t.Fatal("observation lost real work outcome", report)
			}
		})
	}
}

func TestObserveDisabledAllocationsAndErrorOnlyCall(t *testing.T) {
	ctx := t.Context()
	callback := func(actual context.Context) (int, error) {
		if actual != ctx {
			t.Fatal("disabled observer replaced context")
		}
		return 7, nil
	}
	if count := testing.AllocsPerRun(100, func() {
		value, err := Observe(ctx, Worker, callback)
		if value != 7 || err != nil {
			t.Fatal("disabled observer replaced value")
		}
	}); count != 0 {
		t.Fatal("disabled callback wrapper allocated", count)
	}
	r := NewRecorder(true, 9)
	marker := errors.New("private-work-marker")
	err := ObserveCall(WithRecorder(ctx, r), PruneObserve, func(context.Context) error { return marker })
	if err != marker || r.Finish().Spans[0].Outcome != Failed {
		t.Fatal("error-only observation changed failure")
	}
}
