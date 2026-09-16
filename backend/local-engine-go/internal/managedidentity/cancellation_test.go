package managedidentity

import (
	"context"
	"errors"
	"testing"
)

// These deterministic boundaries cover the cancellation requirements in
// docs/architecture/managed-database-identity-tests.md without sleeps or races.
func TestCancellationAfterMissingLookupPreventsGenerationAndWrite(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reads, writes := 0, 0
	service, err := NewService(fakeStore{
		find:    func(context.Context, BaseSelector) (Record, error) { cancel(); return Record{}, ErrNotFound },
		reserve: func(context.Context, Record) (Record, error) { writes++; return Record{}, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	service.entropy = entropyFunc(func(p []byte) (int, error) { reads++; return len(p), nil })
	got, err := service.ResolveOrReserveBase(ctx, validSelector())
	if !errors.Is(err, context.Canceled) || got != (IdentityBinding{}) || reads != 0 || writes != 0 {
		t.Fatalf("cancellation: identity=%+v err=%v reads=%d writes=%d", got, err, reads, writes)
	}
}
func TestCancellationAfterGenerationPreventsWrite(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reads, writes := 0, 0
	service, err := NewService(fakeStore{
		find:    func(context.Context, BaseSelector) (Record, error) { return Record{}, ErrNotFound },
		reserve: func(context.Context, Record) (Record, error) { writes++; return Record{}, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	service.entropy = entropyFunc(func(p []byte) (int, error) {
		reads++
		for i := range p {
			p[i] = 0x42
		}
		cancel()
		return len(p), nil
	})
	got, err := service.ResolveOrReserveBase(ctx, validSelector())
	if !errors.Is(err, context.Canceled) || got != (IdentityBinding{}) || reads != 1 || writes != 0 {
		t.Fatalf("cancellation: identity=%+v err=%v reads=%d writes=%d", got, err, reads, writes)
	}
}

type entropyFunc func([]byte) (int, error)

func (f entropyFunc) Read(p []byte) (int, error) { return f(p) }
