//go:build linux

package benchmark

import (
	"errors"
	"fmt"
	"time"
)

// diagnosticClockContext contains only a closed reason and optional bounded
// relative spans. Zero spans mean both unavailable, not a zero-duration sample.
// CW01–CW05 in issue89-performance-diagnostic-tests.md prohibit epoch/PID data.
type diagnosticClockContext struct {
	Reason            string
	WallNS, ElapsedNS int64
}

// diagnosticClockError retains no time.Time, interval, source text or foreign
// cause. The original fixed sentinel remains available through errors.Is.
type diagnosticClockError struct{ detail diagnosticClockContext }

func (diagnosticClockError) Error() string { return "incomplete diagnostic trace" }
func (diagnosticClockError) Unwrap() error { return errDiagnosticTrace }

const diagnosticClockSpanLimitNS = int64(12 * time.Minute)

// normalizeDiagnosticClockContext rejects forged/incoherent pairs as a whole.
// The actual constructor withholds unpublishable spans before retaining state.
func normalizeDiagnosticClockContext(value diagnosticClockContext) diagnosticClockContext {
	unavailable := diagnosticClockContext{Reason: "none"}
	switch value.Reason {
	case "wall_order", "elapsed_order", "duration_mismatch", "interval_order":
	default:
		return unavailable
	}
	if value.WallNS == 0 && value.ElapsedNS == 0 {
		return value
	}
	if value.WallNS <= 0 || value.ElapsedNS <= 0 || value.WallNS > diagnosticClockSpanLimitNS || value.ElapsedNS > diagnosticClockSpanLimitNS {
		return unavailable
	}
	if value.Reason == "duration_mismatch" && value.WallNS != value.ElapsedNS || value.Reason == "interval_order" && value.WallNS == value.ElapsedNS {
		return value
	}
	return unavailable
}

// newDiagnosticClockError never retains unbounded readings. The ceiling affects
// diagnostic metadata only; it must not decide whether a window is accepted.
func newDiagnosticClockError(reason string, wallNS, elapsedNS int64) error {
	value := diagnosticClockContext{Reason: reason}
	if (reason == "duration_mismatch" || reason == "interval_order") && wallNS > 0 && elapsedNS > 0 && wallNS <= diagnosticClockSpanLimitNS && elapsedNS <= diagnosticClockSpanLimitNS {
		value.WallNS, value.ElapsedNS = wallNS, elapsedNS
	}
	return diagnosticClockError{detail: normalizeDiagnosticClockContext(value)}
}

// validateDiagnosticWindow preserves the original checks and their ordering.
// Scalar inputs let CW01 vary elapsed independently without forging time.Time.
func validateDiagnosticWindow(wallStartNS, wallEndNS int64, elapsed time.Duration) error {
	wallNS := wallEndNS - wallStartNS
	if wallEndNS <= wallStartNS {
		return newDiagnosticClockError("wall_order", wallNS, int64(elapsed))
	}
	if elapsed <= 0 {
		return newDiagnosticClockError("elapsed_order", wallNS, int64(elapsed))
	}
	if elapsed != time.Duration(wallNS) {
		return newDiagnosticClockError("duration_mismatch", wallNS, int64(elapsed))
	}
	return nil
}

// validateMeasuredDiagnosticWindow applies only the approved measured allowance
// (MC01): positive ordered spans and at most ten microseconds of signed drift.
// Positive spans make their subtraction safe; overflowed wall spans never pass.
// The original validator remains strict and supplies all rejection metadata.
func validateMeasuredDiagnosticWindow(wallStartNS, wallEndNS int64, elapsed time.Duration) (int64, error) {
	err := validateDiagnosticWindow(wallStartNS, wallEndNS, elapsed)
	if err == nil {
		return 0, nil
	}
	wallNS := wallEndNS - wallStartNS
	if diagnosticErrorClock(err).Reason != "duration_mismatch" || wallNS <= 0 {
		return 0, err
	}
	delta := wallNS - int64(elapsed)
	if delta < -10000 || delta > 10000 {
		return 0, err
	}
	return delta, nil
}

// summarizeMeasuredDiagnosticIntervals checks the original monotonic elapsed
// before using wall-only endpoints for trace geometry (MC02/MC03). A wall-core
// error retains its reason, but never mislabels that core's span as monotonic.
func summarizeMeasuredDiagnosticIntervals(rows []diagnosticInterval, start, end time.Time) (diagnosticTraceSummary, error) {
	delta, err := validateMeasuredDiagnosticWindow(start.UnixNano(), end.UnixNano(), end.Sub(start))
	if err != nil {
		return diagnosticTraceSummary{}, err
	}
	result, err := summarizeDiagnosticIntervals(rows, start.Round(0), end.Round(0))
	if err != nil {
		return diagnosticTraceSummary{}, newDiagnosticClockError(diagnosticErrorClock(err).Reason, 0, 0)
	}
	result.ClockDeltaNS = &delta
	return result, nil
}

// diagnosticErrorClock exposes only normalised metadata, including wrapped
// owned errors. It never formats or copies a caller-supplied error message.
func diagnosticErrorClock(err error) diagnosticClockContext {
	var value diagnosticClockError
	if !errors.As(err, &value) {
		return diagnosticClockContext{Reason: "none"}
	}
	return normalizeDiagnosticClockContext(value.detail)
}

// diagnosticClockMarker is the fixed private-error/public-log boundary. Node
// independently validates its grammar; no calendar endpoint can reach this line.
func diagnosticClockMarker(err error) string {
	value := diagnosticErrorClock(err)
	if value.WallNS == 0 {
		return fmt.Sprintf("P01B_DIAGNOSTIC_CLOCK %s none none", value.Reason)
	}
	return fmt.Sprintf("P01B_DIAGNOSTIC_CLOCK %s %d %d", value.Reason, value.WallNS, value.ElapsedNS)
}
