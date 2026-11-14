package preparecost

import (
	"context"
	"reflect"
	"sync"
	"testing"
)

// CT06/CT07: default debug cannot turn measurements on, and the disabled path
// does not allocate scopes/store observations or change cancellation semantics.
func TestRecorderExplicitSelectionAndDisabledPath(t *testing.T) {
	for _, value := range []string{"", "0", "true", " 1", "1 ", "private-input"} {
		if Enabled(value) {
			t.Fatal("non-explicit startup switch enabled measurements")
		}
	}
	if !Enabled("1") {
		t.Fatal("explicit startup switch ignored")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	if recorder := NewRecorder(false, 1); recorder != nil {
		t.Fatal("disabled recorder allocated storage")
	}
	if allocations := testing.AllocsPerRun(100, func() {
		plain := WithRecorder(ctx, nil)
		plain = WithPhase(plain, PhaseStep, 1)
		child, end := Start(plain, CaptureRead)
		end(Failed)
		if child != ctx || IsRecording(child) {
			t.Fatal("disabled path changed context/report")
		}
	}); allocations != 0 {
		t.Fatalf("disabled recording allocated %v objects", allocations)
	}
	if !reflect.DeepEqual((*Recorder)(nil).Finish(), Report{}) {
		t.Fatal("disabled path changed report")
	}
	cancel()
	if ctx.Err() != context.Canceled {
		t.Fatal("observation changed cancellation")
	}
}

func TestRecorderNestedScopesAndSnapshotOwnership(t *testing.T) {
	r := NewRecorder(true, 7)
	ctx := WithRecorder(t.Context(), r)
	if !IsRecording(ctx) || WithRecorder(ctx, r) != ctx {
		t.Fatal("recorder was lost or reattached as a different scope")
	}
	root, finishRoot := Start(ctx, Worker)
	step := WithPhase(root, PhaseStep, 2)
	_, finishRead := Start(step, CaptureRead)
	finishRead(Failed)
	_, finishPrune := Start(step, PruneObserve)
	finishPrune(Cancelled)
	finishRoot(Succeeded)
	got := r.Finish()
	if !got.Complete || got.Version != 1 || got.Root != 7 || len(got.Spans) != 3 || got.ElapsedNS < 0 {
		t.Fatalf("complete observation not retained: %+v", got)
	}
	for index, span := range got.Spans {
		if span.ID != index+1 || span.StartNS < 0 || span.EndNS < span.StartNS || span.EndNS > got.ElapsedNS {
			t.Fatal("recorder created invalid monotonic endpoints")
		}
		if index > 0 && (span.Parent != 1 || span.Phase != PhaseStep || span.Step != 2) {
			t.Fatal("child lost parent/logical step")
		}
	}
	before := r.Finish()
	got.Spans[0].Operation = "private-mutated-key"
	if !reflect.DeepEqual(before, r.Finish()) {
		t.Fatal("snapshot mutation or repeated finish changed observations")
	}
	if _, err := Summarize(before.Spans, max(before.ElapsedNS, 1)); err != nil {
		t.Fatal("real spans do not satisfy the independent accounting contract", err)
	}
}

func TestRecorderInvalidScopesAreIncomplete(t *testing.T) {
	for _, variant := range []string{"unknown-key", "unknown-outcome", "invalid-phase", "step-zero", "foreign-root", "closed-parent", "unfinished", "double-end", "late-start", "late-end", "zero-root", "zero-recorder"} {
		t.Run(variant, func(t *testing.T) {
			r := NewRecorder(true, 3)
			if variant == "zero-root" {
				r = NewRecorder(true, 0)
			}
			if variant == "zero-recorder" {
				r = &Recorder{}
			}
			ctx := WithRecorder(t.Context(), r)
			root, closeRoot := Start(ctx, Worker)
			switch variant {
			case "unknown-key":
				_, end := Start(root, "private-unknown-key")
				end(Succeeded)
			case "unknown-outcome":
				_, end := Start(root, CaptureRead)
				end("private-error-text")
			case "invalid-phase":
				_, end := Start(WithPhase(root, "private-phase", 0), CaptureRead)
				end(Succeeded)
			case "step-zero":
				_, end := Start(WithPhase(root, PhaseStep, 0), CaptureRead)
				end(Succeeded)
			case "foreign-root":
				other := WithRecorder(root, NewRecorder(true, 9))
				if WithRecorder(other, r) == root {
					t.Fatal("foreign carrier silently regained an old scope")
				}
			case "closed-parent":
				closeRoot(Succeeded)
				_, end := Start(root, CaptureRead)
				end(Succeeded)
				if r.Finish().Complete {
					t.Fatal("closed parent accepted a new child")
				}
				return
			case "unfinished":
				Start(root, CaptureRead)
			case "double-end":
				_, end := Start(root, CaptureRead)
				end(Succeeded)
				end(Succeeded)
			case "late-start":
				closeRoot(Succeeded)
				before := r.Finish()
				_, end := Start(root, CaptureRead)
				end(Succeeded)
				if r.Finish().Complete || !before.Complete || len(before.Spans) != 1 {
					t.Fatal("late write admitted or changed an earlier snapshot")
				}
				return
			case "late-end":
				_, end := Start(root, CaptureRead)
				r.Finish()
				end(Succeeded)
			}
			closeRoot(Succeeded)
			got := r.Finish()
			if got.Complete {
				t.Fatal("invalid or lost measurement silently qualified")
			}
			for _, span := range got.Spans {
				if span.Operation == "private-unknown-key" || span.Outcome == "private-error-text" || span.Phase == "private-phase" {
					t.Fatal("arbitrary input retained in a cost report")
				}
			}
		})
	}
}

func TestRecorderConcurrencyQuotaAndPanic(t *testing.T) {
	r := NewRecorder(true, 1)
	ctx, end := Start(WithRecorder(t.Context(), r), Worker)
	var workers sync.WaitGroup
	for index := range 32 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			_, finish := Start(WithPhase(ctx, PhaseStep, index+1), CaptureRead)
			finish(Succeeded)
		}()
	}
	workers.Wait()
	end(Succeeded)
	got := r.Finish()
	if !got.Complete || len(got.Spans) != 33 {
		t.Fatal("concurrent scopes lost observations")
	}
	if _, err := Summarize(got.Spans, max(got.ElapsedNS, 1)); err != nil {
		t.Fatal("concurrent scopes are inconsistent", err)
	}
	r = NewRecorder(true, 2)
	ctx = WithRecorder(t.Context(), r)
	for range MaxSpans {
		_, finish := Start(ctx, CaptureRead)
		finish(Succeeded)
	}
	if got := r.Finish(); !got.Complete || len(got.Spans) != MaxSpans {
		t.Fatal("exact recorder quota rejected")
	}
	r = NewRecorder(true, 3)
	ctx = WithRecorder(t.Context(), r)
	for range MaxSpans + 1 {
		_, finish := Start(ctx, CaptureRead)
		finish(Succeeded)
	}
	if got := r.Finish(); got.Complete || len(got.Spans) != MaxSpans {
		t.Fatal("overflowed recorder qualified or exceeded storage bound")
	}
	r = NewRecorder(true, 4)
	ctx = WithRecorder(t.Context(), r)
	marker := &struct{}{}
	func() {
		defer func() {
			if recover() != marker {
				t.Fatal("measurement replaced the original panic")
			}
		}()
		_, finish := Start(ctx, CaptureRead)
		defer finish(Panicked)
		panic(marker)
	}()
	if got := r.Finish(); !got.Complete || got.Spans[0].Outcome != Panicked {
		t.Fatal("panic observation lost its closed outcome")
	}
}
