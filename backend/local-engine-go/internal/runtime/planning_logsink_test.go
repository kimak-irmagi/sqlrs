package runtime

import (
	"context"
	"testing"
)

// MP07: a protected planning command must not inherit public diagnostics.
func TestWithoutLogSinkPreservesContextControl(t *testing.T) {
	type valueKey struct{}
	parent, cancel := context.WithCancel(context.WithValue(context.Background(), valueKey{}, "retained"))
	defer cancel()
	withSink := WithLogSink(parent, func(string) {})
	without := WithoutLogSink(withSink)
	if LogSinkFromContext(without) != nil || without.Value(valueKey{}) != "retained" {
		t.Fatal("log removal lost another context value")
	}
	cancel()
	if without.Err() != context.Canceled {
		t.Fatal("cancellation was lost")
	}
}
