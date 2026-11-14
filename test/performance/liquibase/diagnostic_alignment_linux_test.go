package benchmark

import (
	"encoding/json"
	"errors"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

// MC01: scalar inputs are real validator inputs, not forged time.Time values.
// Literal expectations implement the approved two-sided ten-microsecond bound.
func TestDiagnosticMeasuredWindowBounds(t *testing.T) {
	for _, want := range []int64{0, -1, 1, -9, 9, -9999, 9999, -10000, 10000} {
		got, err := validateMeasuredDiagnosticWindow(100, 100100, time.Duration(100000-want))
		if err != nil || got != want {
			t.Fatal("bounded signed clock delta differs")
		}
		strict := validateDiagnosticWindow(100, 100100, time.Duration(100000-want))
		if (strict == nil) != (want == 0) {
			t.Fatal("strict validator was relaxed")
		}
	}
	for _, row := range []struct{ a, b, elapsed, want int64 }{
		{0, 73672499931, 73672499940, -9},
		{0, math.MaxInt64, math.MaxInt64 - 9999, 9999},
		{-math.MaxInt64, 0, math.MaxInt64 - 1, 1},
	} {
		if got, err := validateMeasuredDiagnosticWindow(row.a, row.b, time.Duration(row.elapsed)); err != nil || got != row.want {
			t.Fatal("safe large/actual clock pair changed")
		}
	}
	for _, row := range []struct {
		a, b, elapsed int64
		reason        string
	}{
		{100, 100100, 89999, "duration_mismatch"}, {100, 100100, 110001, "duration_mismatch"},
		{100, 100, 0, "wall_order"}, {100, 99, -1, "wall_order"},
		{100, 200, 0, "elapsed_order"}, {100, 200, -1, "elapsed_order"},
		{math.MinInt64, math.MaxInt64, math.MaxInt64, "duration_mismatch"},
		{math.MinInt64, math.MaxInt64, 1, "duration_mismatch"},
		{math.MaxInt64, math.MinInt64, 1, "wall_order"},
	} {
		_, err := validateMeasuredDiagnosticWindow(row.a, row.b, time.Duration(row.elapsed))
		if !errors.Is(err, errDiagnosticTrace) || diagnosticErrorClock(err).Reason != row.reason {
			t.Fatal("invalid measured clock accepted or misclassified")
		}
	}
}

// MC02/MC03: real summary uses wall geometry; the strict helper stays unchanged.
func TestDiagnosticMeasuredSummary(t *testing.T) {
	a, b := time.Unix(100, 0), time.Unix(101, 0)
	rows := []diagnosticInterval{{"host.java", 99000000000, 100600000000}, {"docker.copy", 100200000000, 100800000000}, {"other", 101000000000, 101000000000}}
	original := append([]diagnosticInterval(nil), rows...)
	got, err := summarizeMeasuredDiagnosticIntervals(rows, a, b)
	if err != nil || got.ClockDeltaNS == nil || *got.ClockDeltaNS != 0 || got.CoveredMS != 800 || got.ResidualMS != 200 || got.Commands != 2 || !reflect.DeepEqual(got.Intervals, []diagnosticPublicInterval{{"host.java", 0, 600}, {"docker.copy", 200, 800}}) {
		t.Fatal("measured wall-axis summary differs")
	}
	if !reflect.DeepEqual(rows, original) || a != time.Unix(100, 0) || b != time.Unix(101, 0) {
		t.Fatal("summary mutated inputs")
	}
	data, err := json.Marshal(got)
	if err != nil || !strings.Contains(string(data), `"ClockDeltaNS":0`) {
		t.Fatal("measured zero omitted")
	}
	for _, secret := range []string{"100000000000", "101000000000", "PRIVATE_SENTINEL", "pid"} {
		if strings.Contains(string(data), secret) {
			t.Fatal("private input escaped measured summary")
		}
	}
	strict, err := summarizeDiagnosticIntervals(rows, a, b)
	if err != nil || strict.ClockDeltaNS != nil {
		t.Fatal("strict summary acquired alignment")
	}
	data, err = json.Marshal(strict)
	if err != nil || strings.Contains(string(data), "ClockDeltaNS") {
		t.Fatal("strict absence was serialized")
	}
	now := time.Now()
	for _, pair := range [][2]time.Time{{a, b}, {now, now.Add(time.Second)}} {
		empty, err := summarizeMeasuredDiagnosticIntervals(nil, pair[0], pair[1])
		if err != nil || empty.ClockDeltaNS == nil || *empty.ClockDeltaNS != 0 || empty.CoveredMS != 0 || empty.ResidualMS != 1000 || empty.Commands != 0 {
			t.Fatal("empty measured summary changed")
		}
	}
	for _, window := range [][2]int64{{100200000000, 100400000000}, {100800000000, 101200000000}} {
		value, err := summarizeMeasuredDiagnosticIntervals([]diagnosticInterval{{"other", window[0], window[1]}}, a, b)
		if err != nil || value.CoveredMS != 200 || value.ResidualMS != 800 || value.Commands != 1 {
			t.Fatal("measured boundary clipping changed")
		}
	}
}

// MC03: no wall-only span may be represented as an original monotonic span.
func TestDiagnosticMeasuredSummaryFailures(t *testing.T) {
	a, b := time.Unix(100, 0), time.Unix(101, 0)
	for _, pair := range [][2]time.Time{{a, a}, {b, a}} {
		got, err := summarizeMeasuredDiagnosticIntervals(nil, pair[0], pair[1])
		if !errors.Is(err, errDiagnosticTrace) || diagnosticErrorClock(err).Reason != "wall_order" || got.ClockDeltaNS != nil {
			t.Fatal("bad window published")
		}
	}
	bad := []diagnosticInterval{{"other", 4, 3}}
	got, err := summarizeMeasuredDiagnosticIntervals(bad, a, b)
	if !errors.Is(err, errDiagnosticTrace) || diagnosticErrorClock(err).Reason != "interval_order" || got.ClockDeltaNS != nil {
		t.Fatal("bad measured interval accepted")
	}
	// A known reason with unavailable spans is valid; a false unequal/equal
	// original pair remains prohibited by the unchanged CW normalizer tests.
	if diagnosticErrorClock(newDiagnosticClockError("interval_order", 0, 0)) != (diagnosticClockContext{Reason: "interval_order"}) {
		t.Fatal("known unavailable pair lost")
	}
	// Observe actual endpoints without requiring drift or a particular timing.
	x, y := time.Now(), time.Now()
	wall, elapsed := y.UnixNano()-x.UnixNano(), int64(y.Sub(x))
	reason := "interval_order"
	if y.UnixNano() <= x.UnixNano() {
		reason = "wall_order"
	} else if elapsed <= 0 {
		reason = "elapsed_order"
	} else if wall <= 0 || wall-elapsed < -10000 || wall-elapsed > 10000 {
		reason = "duration_mismatch"
	}
	got, err = summarizeMeasuredDiagnosticIntervals(bad, x, y)
	meta := diagnosticErrorClock(err)
	if !errors.Is(err, errDiagnosticTrace) || meta.Reason != reason || got.ClockDeltaNS != nil {
		t.Fatal("real failed summary changed classification")
	}
	if reason == "interval_order" && wall != elapsed && (meta.WallNS != 0 || meta.ElapsedNS != 0) {
		t.Fatal("wall core invented original clock evidence")
	}
}

// MC06: selected measurement mode does not replace API's monotonic duration.
func TestDiagnosticMeasuredAlignmentWiring(t *testing.T) {
	data, err := os.ReadFile("collector_linux_test.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	if !strings.Contains(source, "summarizeMeasuredDiagnosticIntervals(intervals, diagnostics[i].start, diagnostics[i].end)") || strings.Contains(source, "summarizeDiagnosticIntervals(intervals, diagnostics[i].start, diagnostics[i].end)") {
		t.Fatal("collector lacks explicit aligned mode")
	}
	if !strings.Contains(source, "ElapsedMS: float64(end.Sub(start)) / 1e6") || !strings.Contains(source, "ElapsedMS: float64(end.Sub(start)) / float64(time.Millisecond)") {
		t.Fatal("API elapsed source changed")
	}
}
