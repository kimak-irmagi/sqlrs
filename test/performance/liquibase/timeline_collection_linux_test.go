package benchmark

import (
	"math"
	"testing"
)

// TL01: the collector's independent implementation uses actual kernel reads,
// never calendar/Go-process origins. The private absolute reference stays local.
func TestCollectorTimelineCommonClock(t *testing.T) {
	reference, err := costTimelineNow()
	if err != nil || reference <= 0 {
		t.Fatal("collector common clock unavailable")
	}
	read := costTimelineReader(reference)
	before, err := costTimelineNow()
	if err != nil {
		t.Fatal(err)
	}
	offset, err := read()
	if err != nil {
		t.Fatal(err)
	}
	after, err := costTimelineNow()
	if err != nil || reference+offset < before || reference+offset > after {
		t.Fatal("collector uses a different clock axis")
	}
	for _, invalid := range []int64{0, math.MaxInt64} {
		if value, err := costTimelineReader(invalid)(); err == nil || value != 0 {
			t.Fatal("unavailable common-clock evidence became a zero sample")
		}
	}
}
