package preparecost

import (
	"math"
	"runtime"
	"testing"
)

// TL01: a private reference cannot enable recording, and failure to obtain a
// qualified clock must not become an execution or public-configuration choice.
func TestTimelineClockExplicitSelection(t *testing.T) {
	for _, value := range []string{"", "0", "true", " 1", "1 ", "private-sentinel"} {
		clock, err := NewTimelineClock(value, "private-sentinel")
		if clock != nil || err != nil {
			t.Fatal("nonexplicit selection acquired a clock", value)
		}
	}
	if allocations := testing.AllocsPerRun(100, func() {
		clock, err := NewTimelineClock("0", "private-sentinel")
		if clock != nil || err != nil {
			t.Fatal("disabled clock changed result")
		}
	}); allocations != 0 {
		t.Fatalf("disabled clock allocated %v", allocations)
	}
	for _, reference := range []string{"", "0", "-1", "+1", "01", "1 ", "private-sentinel", "9223372036854775808"} {
		clock, err := NewTimelineClock("1", reference)
		if clock != nil || err == nil || err.Error() != "invalid prepare cost evidence" {
			t.Fatal("invalid/private clock reference escaped", clock, err)
		}
	}
	clock, err := NewTimelineClock("1", "1")
	if runtime.GOOS == "linux" {
		if err != nil || clock == nil {
			t.Fatal("Linux common clock unavailable", err)
		}
	} else if err == nil || clock != nil {
		t.Fatal("unsupported host silently invented a clock")
	}
	if _, err := (*TimelineClock)(nil).Read(); err == nil {
		t.Fatal("absent clock acquired valid evidence")
	}
}

func TestTimelineClockRelativeIntegerBounds(t *testing.T) {
	for _, tc := range []struct {
		now, reference, want int64
		valid                bool
	}{
		{100, 99, 1, true}, {100, 100, 0, true},
		{math.MaxInt64, math.MaxInt64 - 1, 1, true},
		{9007199254740992, 1, 9007199254740991, true},
		{9007199254740993, 1, 0, false}, {99, 100, 0, false},
		{100, 0, 0, false}, {100, -1, 0, false}, {-1, 1, 0, false},
	} {
		got, err := timelineClockOffset(tc.now, tc.reference)
		if (err == nil) != tc.valid || got != tc.want {
			t.Fatalf("relative clock %v: got=%d err=%v", tc, got, err)
		}
	}
}
