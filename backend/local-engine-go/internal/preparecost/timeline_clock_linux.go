package preparecost

import "golang.org/x/sys/unix"

func timelineClockSupported() bool { return true }

// readTimelineMonotonicNS uses the same kernel clock in collector and engine.
// This private absolute reading is never an exported observation.
func readTimelineMonotonicNS() (int64, error) {
	var value unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_MONOTONIC, &value); err != nil {
		return 0, errEvidence
	}
	return value.Nano(), nil
}
