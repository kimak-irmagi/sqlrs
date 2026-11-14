package benchmark

import (
	"strings"
	"testing"
)

// CT01/CT08/CT10: each connected step/publication source key must survive
// independent decoding; arbitrary names remain rejected by the original tests.
func TestCostStreamConnectedStepAndPublicationKeys(t *testing.T) {
	for _, operation := range []string{"liquibase.invoke", "db.prepare_snapshot", "db.resume.step", "db.resume.publication",
		"db.verify.after_apply", "db.verify.after_resume", "state.snapshot", "state.size", "state.seal",
		"journal.state_publish", "journal.invocation_plan", "journal.invocation_admit", "journal.invocation_complete",
		"prepare.publication", "prepare.final_cleanup"} {
		stream := newCostStream()
		wire := strings.Replace(costStreamExample(1), "capture.read", operation, 1)
		if err := stream.consume(strings.NewReader(wire)); err != nil {
			t.Fatal("connected closed step/publication key rejected")
		}
		got, err := stream.await(t.Context(), 1)
		if err != nil || got.Spans[1].Operation != operation {
			t.Fatal("connected step/publication key changed")
		}
	}
}
