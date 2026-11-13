package benchmark

import (
	"strings"
	"testing"
)

// IC07: the receiver understands the real lookup key, not arbitrary cache text.
func TestInventoryLookupClosedStreamKey(t *testing.T) {
	stream := newCostStream()
	wire := strings.Replace(costStreamExample(1), "capture.read", "runner.inventory_lookup", 1)
	if err := stream.consume(strings.NewReader(wire)); err != nil {
		t.Fatal(err)
	}
	got, err := stream.await(t.Context(), 1)
	if err != nil || got.Spans[1].Operation != "runner.inventory_lookup" {
		t.Fatal("lookup evidence lost", err)
	}
	stream = newCostStream()
	wire = strings.Replace(wire, "runner.inventory_lookup", "runner.inventory_hit_private_value", 1)
	if err := stream.consume(strings.NewReader(wire)); err == nil {
		t.Fatal("unknown operation accepted")
	}
}
