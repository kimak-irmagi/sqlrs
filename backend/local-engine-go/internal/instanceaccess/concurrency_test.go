package instanceaccess

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"
)

// observingWaitContext signals when an operation starts listening for cancellation.
type observingWaitContext struct {
	context.Context
	waiting chan struct{}
	once    sync.Once
}

func (c *observingWaitContext) Done() <-chan struct{} {
	c.once.Do(func() { close(c.waiting) })
	return c.Context.Done()
}

// The channels represent a long runtime call without depending on SQL duration.
func TestAccessUseDoesNotBlockOtherInstances(t *testing.T) {
	s, b := accessFixture(t)
	ctx := context.Background()
	if err := s.Activate(ctx, "a", b, func(Secret) error { return nil }, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	done := make(chan error, 1)
	go func() {
		done <- s.Use(ctx, "a", func(AccessBinding, SecretBinding) error { close(entered); <-release; return nil })
	}()
	defer func() {
		unblock()
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("command did not start")
	}
	b.PhysicalIdentity, b.RuntimeRef = "physical-b", "runtime-b"
	other := make(chan error, 1)
	go func() {
		if err := s.Activate(ctx, "b", b, func(Secret) error { return nil }, func() error { return nil }); err != nil {
			other <- err
			return
		}
		if _, err := s.Lookup(ctx, "b"); err != nil {
			other <- err
			return
		}
		if _, err := s.Resolve(ctx, "b", b); err != nil {
			other <- err
			return
		}
		if err := s.Use(ctx, "b", func(AccessBinding, SecretBinding) error { return nil }); err != nil {
			other <- err
			return
		}
		if _, _, err := s.Bootstrap(ctx, b.IdentityBinding, true); err != nil {
			other <- err
			return
		}
		other <- s.Retire(ctx, "b", func() error { return nil })
	}()
	select {
	case err := <-other:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		unblock()
		<-other
		t.Fatal("long command on A blocked independent instance B")
	}
}

func TestAccessWaitCancellationAndRetirementFence(t *testing.T) {
	s, b := accessFixture(t)
	ctx := context.Background()
	if err := s.Activate(ctx, "a", b, func(Secret) error { return nil }, func() error { return nil }); err != nil {
		t.Fatal(err)
	}
	entered, release := make(chan struct{}), make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	done := make(chan error, 1)
	go func() {
		done <- s.Use(ctx, "a", func(AccessBinding, SecretBinding) error { close(entered); <-release; return nil })
	}()
	defer func() {
		unblock()
		if err := <-done; err != nil {
			t.Error(err)
		}
	}()
	select {
	case <-entered:
	case <-time.After(5 * time.Second):
		t.Fatal("command did not start")
	}
	for _, method := range []string{"activate", "resolve", "use", "lookup", "retire", "resume"} {
		cancelCtx, cancel := context.WithCancel(ctx)
		waitCtx := &observingWaitContext{Context: cancelCtx, waiting: make(chan struct{})}
		waiting := make(chan error, 1)
		go func() {
			var err error
			switch method {
			case "activate":
				err = s.Activate(waitCtx, "a", b, func(Secret) error { t.Error("cancelled activation ran"); return nil }, func() error { return nil })
			case "resolve":
				_, err = s.Resolve(waitCtx, "a", b)
			case "use":
				err = s.Use(waitCtx, "a", func(AccessBinding, SecretBinding) error { t.Error("cancelled command ran"); return nil })
			case "lookup":
				_, err = s.Lookup(waitCtx, "a")
			case "retire":
				err = s.Retire(waitCtx, "a", func() error { t.Error("cancelled deletion ran"); return nil })
			case "resume":
				_, _, err = s.ResumePublication(waitCtx, "a")
			}
			waiting <- err
		}()
		select {
		case <-waitCtx.waiting:
		case <-time.After(5 * time.Second):
			cancel()
			unblock()
			<-waiting
			t.Fatalf("%s never listened for cancellation", method)
		}
		cancel()
		select {
		case err := <-waiting:
			if !errors.Is(err, context.Canceled) {
				t.Fatalf("%s cancellation: %v", method, err)
			}
		case <-time.After(5 * time.Second):
			unblock()
			<-waiting
			t.Fatalf("%s ignored cancelled wait", method)
		}
	}
	removed := make(chan struct{})
	retired := make(chan error, 1)
	go func() { retired <- s.Retire(ctx, "a", func() error { close(removed); return nil }) }()
	select {
	case <-removed:
		t.Fatal("retirement overlapped active command")
	case <-time.After(50 * time.Millisecond):
	}
	unblock()
	select {
	case err := <-retired:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("retirement did not resume")
	}
	if _, err := s.Lookup(ctx, "a"); err != ErrConflict {
		t.Fatal("retired instance became accessible", err)
	}
}
