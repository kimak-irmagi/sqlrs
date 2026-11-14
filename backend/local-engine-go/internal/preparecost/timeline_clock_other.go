//go:build !linux

package preparecost

func timelineClockSupported() bool { return false }

// Non-Linux keeps its existing ordinary recorder; a requested complete Linux
// clock cannot silently be replaced with calendar/process-local time.
func readTimelineMonotonicNS() (int64, error) { return 0, errEvidence }
