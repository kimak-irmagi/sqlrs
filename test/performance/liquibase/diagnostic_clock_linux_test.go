package benchmark

import (
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"os"
	"reflect"
	"strings"
	"testing"
	"time"
)

// CW01: independent wall/elapsed inputs exercise the actual approved validator,
// not a forged time.Time. See issue89-performance-diagnostic-tests.md.
func TestDiagnosticClockWindowReasons(t *testing.T) {
	for _, row := range []struct {
		a, b, elapsed int64
		reason        string
	}{
		{100, 100, 0, "wall_order"}, {200, 100, -1, "wall_order"},
		{100, 100, 1, "wall_order"}, {100, 200, 0, "elapsed_order"},
		{100, 200, -1, "elapsed_order"}, {100, 200, 99, "duration_mismatch"},
		{100, 200, 101, "duration_mismatch"}, {100, 200, 100, "none"},
		{100, 101, 1, "none"}, {100, 720000000101, 720000000001, "none"},
	} {
		err := validateDiagnosticWindow(row.a, row.b, time.Duration(row.elapsed))
		if diagnosticErrorClock(err).Reason != row.reason || (err == nil) != (row.reason == "none") {
			t.Fatal("window reason/precedence differs")
		}
		if err != nil && (!errors.Is(err, errDiagnosticTrace) || err.Error() != "incomplete diagnostic trace") {
			t.Fatal("original error contract lost")
		}
	}
}

// CW02: exercise the real summary and preserve its success/count/union contract.
func TestDiagnosticClockSummary(t *testing.T) {
	a, b := time.Unix(100, 0), time.Unix(101, 0)
	bad := []diagnosticInterval{{"other", 4, 3}}
	for _, row := range []struct {
		start, end time.Time
		rows       []diagnosticInterval
		reason     string
	}{
		{a, a, nil, "wall_order"}, {b, a, nil, "wall_order"},
		{a, b, bad, "interval_order"}, {b, a, bad, "wall_order"},
	} {
		_, err := summarizeDiagnosticIntervals(row.rows, row.start, row.end)
		if !errors.Is(err, errDiagnosticTrace) || diagnosticErrorClock(err).Reason != row.reason {
			t.Fatal("summary boundary/precedence lost")
		}
	}
	rows := []diagnosticInterval{{"host.java", 99000000000, 100600000000}, {"docker.copy", 100200000000, 100800000000}, {"other", 100900000000, 100900000000}}
	got, err := summarizeDiagnosticIntervals(rows, a, b)
	want := diagnosticTraceSummary{CoveredMS: 800, ResidualMS: 200, Commands: 2, Intervals: []diagnosticPublicInterval{{"host.java", 0, 600}, {"docker.copy", 200, 800}}}
	if err != nil || !reflect.DeepEqual(got, want) || diagnosticErrorClock(err) != (diagnosticClockContext{Reason: "none"}) {
		t.Fatal("successful summary changed")
	}
	// Add advances both readings equally; no host-clock drift assumption or sleep.
	now := time.Now()
	for _, pair := range [][2]time.Time{{a, b}, {now, now.Add(time.Second)}, {a, a.Add(13 * time.Minute)}} {
		got, err := summarizeDiagnosticIntervals(nil, pair[0], pair[1])
		if err != nil || got.Commands != 0 || got.CoveredMS != 0 || got.ResidualMS != float64(pair[1].Sub(pair[0]))/1e6 {
			t.Fatal("empty/long window rejected or hidden")
		}
	}
}

// CW03/CW05: real errors retain only the known reason and bounded relative pair.
func TestDiagnosticClockMetadataAndMarker(t *testing.T) {
	_, intervalErr := summarizeDiagnosticIntervals([]diagnosticInterval{{"other", 4, 3}}, time.Unix(100, 0), time.Unix(101, 0))
	for _, row := range []struct {
		err    error
		want   diagnosticClockContext
		marker string
	}{
		{validateDiagnosticWindow(100, 99, 1), diagnosticClockContext{Reason: "wall_order"}, "wall_order none none"},
		{validateDiagnosticWindow(100, 200, 0), diagnosticClockContext{Reason: "elapsed_order"}, "elapsed_order none none"},
		{validateDiagnosticWindow(1700000000000000000, 1700000000000000100, 99), diagnosticClockContext{"duration_mismatch", 100, 99}, "duration_mismatch 100 99"},
		{validateDiagnosticWindow(100, 101, 2), diagnosticClockContext{"duration_mismatch", 1, 2}, "duration_mismatch 1 2"},
		{validateDiagnosticWindow(0, 720000000000, 719999999999), diagnosticClockContext{"duration_mismatch", 720000000000, 719999999999}, "duration_mismatch 720000000000 719999999999"},
		{validateDiagnosticWindow(0, 720000000001, 720000000000), diagnosticClockContext{Reason: "duration_mismatch"}, "duration_mismatch none none"},
		{validateDiagnosticWindow(0, 720000000000, 720000000001), diagnosticClockContext{Reason: "duration_mismatch"}, "duration_mismatch none none"},
		{validateDiagnosticWindow(math.MinInt64, math.MaxInt64, 1), diagnosticClockContext{Reason: "duration_mismatch"}, "duration_mismatch none none"},
		{validateDiagnosticWindow(math.MaxInt64, math.MinInt64, 1), diagnosticClockContext{Reason: "wall_order"}, "wall_order none none"},
		{intervalErr, diagnosticClockContext{"interval_order", 1000000000, 1000000000}, "interval_order 1000000000 1000000000"},
	} {
		for _, err := range []error{row.err, fmt.Errorf("outer: %w", row.err)} {
			var owned diagnosticClockError
			if !errors.As(err, &owned) || !errors.Is(err, errDiagnosticTrace) || diagnosticErrorClock(err) != row.want {
				t.Fatal("clock error lost metadata or sentinel")
			}
			if diagnosticClockMarker(err) != "P01B_DIAGNOSTIC_CLOCK "+row.marker {
				t.Fatal("real error-to-marker boundary differs")
			}
			data, marshalErr := json.Marshal(diagnosticErrorClock(err))
			if marshalErr != nil {
				t.Fatal(marshalErr)
			}
			for _, private := range []string{"1700000000000000000", "1700000000000000100", "PRIVATE_SENTINEL"} {
				if strings.Contains(fmt.Sprintf("%+v %s %s", err, data, diagnosticClockMarker(err)), private) {
					t.Fatal("private input escaped clock error")
				}
			}
		}
	}
}

// CW03: legacy/foreign and incoherent metadata never become clock evidence.
func TestDiagnosticClockUnavailable(t *testing.T) {
	want := diagnosticClockContext{Reason: "none"}
	for _, err := range []error{nil, errDiagnosticTrace, errors.New("PRIVATE_SENTINEL"), diagnosticDecodeError{8}} {
		if diagnosticErrorClock(err) != want || diagnosticClockMarker(err) != "P01B_DIAGNOSTIC_CLOCK none none none" {
			t.Fatal("foreign error acquired metadata")
		}
	}
	for _, detail := range []diagnosticClockContext{
		{}, {Reason: "none"}, {Reason: "PRIVATE_SENTINEL"},
		{"duration_mismatch", 100, 100}, {"interval_order", 100, 99},
		{"wall_order", 100, 100}, {"elapsed_order", 100, 100},
		{"duration_mismatch", 0, 1}, {"duration_mismatch", 1, 0},
		{"duration_mismatch", -1, 1}, {"duration_mismatch", 1, -1},
		{"duration_mismatch", 720000000001, 1}, {"duration_mismatch", 1, math.MaxInt64},
	} {
		if got := diagnosticErrorClock(diagnosticClockError{detail: detail}); got != want {
			t.Fatal("incoherent clock evidence trusted")
		}
	}
	// Type inspection supplements privacy outputs: the owned error can retain
	// bounded metadata only, not a time.Time, source string or arbitrary cause.
	typ := reflect.TypeOf(diagnosticClockError{})
	for i := 0; i < typ.NumField(); i++ {
		if typ.Field(i).Type != reflect.TypeOf(diagnosticClockContext{}) {
			t.Fatal("unexpected retained clock error state")
		}
	}
}

// CW05: preserve old literal markers/guards and use metadata from the real error.
func TestDiagnosticClockWiring(t *testing.T) {
	data, err := os.ReadFile("collector_linux_test.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	start := strings.Index(source, `t.Log("P01B_DIAGNOSTIC_FAILURE clock")`)
	if start < 0 || !strings.Contains(source[start:], "t.Log(diagnosticClockMarker(err))") {
		t.Fatal("clock failure publication missing")
	}
	data, err = os.ReadFile("diagnostic_trace_linux.go")
	if err != nil {
		t.Fatal(err)
	}
	source = string(data)
	if !strings.Contains(source, "validateDiagnosticWindow(a, b, end.Sub(start))") || !strings.Contains(source, `newDiagnosticClockError("interval_order", b-a, int64(end.Sub(start)))`) {
		t.Fatal("real summary bypasses approved clock boundaries")
	}
}
