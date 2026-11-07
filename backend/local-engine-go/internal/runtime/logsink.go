package runtime

import "context"

type logSinkKey struct{}

func WithLogSink(ctx context.Context, sink LogSink) context.Context {
	if sink == nil {
		return ctx
	}
	return context.WithValue(ctx, logSinkKey{}, sink)
}

func LogSinkFromContext(ctx context.Context) LogSink {
	return logSinkFromContext(ctx)
}

// WithoutLogSink masks public diagnostics without losing cancellation or other
// context values. Protected planning output is never a public log source; see
// liquibase-container-component-structure.md.
func WithoutLogSink(ctx context.Context) context.Context {
	if ctx == nil || logSinkFromContext(ctx) == nil {
		return ctx
	}
	return context.WithValue(ctx, logSinkKey{}, LogSink(nil))
}

func logSinkFromContext(ctx context.Context) LogSink {
	if ctx == nil {
		return nil
	}
	if sink, ok := ctx.Value(logSinkKey{}).(LogSink); ok {
		return sink
	}
	return nil
}
