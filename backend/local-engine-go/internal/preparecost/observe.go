package preparecost

import (
	"context"
	"errors"
)

// Observe surrounds one real work boundary without intercepting/replacing its
// value, error, cancellation or panic. The callback runs exactly once even if
// diagnostics are invalid. Disabled calls do not create a scope or closure.
// CT07/CT08: issue89-cost-attribution-and-trimming.md §3.
func Observe[T any](ctx context.Context, operation Operation, work func(context.Context) (T, error)) (T, error) {
	if !IsRecording(ctx) {
		return work(ctx)
	}
	child, finish := Start(ctx, operation)
	outcome := Panicked
	defer func() { finish(outcome) }()
	value, err := work(child)
	switch {
	case err == nil:
		outcome = Succeeded
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		outcome = Cancelled
	default:
		outcome = Failed
	}
	return value, err
}

// ObserveCall is the error-only form; it grants no retry/cleanup permission.
func ObserveCall(ctx context.Context, operation Operation, work func(context.Context) error) error {
	_, err := Observe(ctx, operation, func(child context.Context) (struct{}, error) {
		return struct{}{}, work(child)
	})
	return err
}
