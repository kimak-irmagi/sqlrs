package benchmark

import (
	"strings"
	"testing"
)

// CT01/CT08/CT10: the independently decoded stream accepts each newly connected
// host source key, but not an arbitrary source symbol supplied by a child.
func TestCostStreamConnectedHostKeys(t *testing.T) {
	for _, operation := range []string{"prepare.planning", "prepare.session_startup", "prepare.step"} {
		stream := newCostStream()
		wire := strings.Replace(costStreamExample(1), "capture.read", operation, 1)
		if err := stream.consume(strings.NewReader(wire)); err != nil {
			t.Fatal("connected closed host source key rejected")
		}
		got, err := stream.await(t.Context(), 1)
		if err != nil || got.Spans[1].Operation != operation {
			t.Fatal("connected host source key changed")
		}
	}
}
