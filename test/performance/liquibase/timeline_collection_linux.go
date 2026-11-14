package benchmark

import "golang.org/x/sys/unix"

// costTimelineNow reads the same Linux clock as the engine without sharing
// its implementation. Absolute readings stay local to the collector process.
func costTimelineNow() (int64, error) {
	var value unix.Timespec
	if err := unix.ClockGettime(unix.CLOCK_MONOTONIC, &value); err != nil {
		return 0, errCostStream
	}
	return value.Nano(), nil
}

func costTimelineReader(reference int64) func() (int64, error) {
	return func() (int64, error) {
		now, err := costTimelineNow()
		if err != nil || reference <= 0 || now < reference || uint64(now-reference) > maxCostTimelineNumber {
			return 0, errCostStream
		}
		return now - reference, nil
	}
}
