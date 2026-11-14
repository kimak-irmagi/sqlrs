package preparecost

import "strconv"

// TimelineClock retains a collector-owned host monotonic reference in memory.
// It has no environment/log access and is never passed to Liquibase or stored.
// Requirements: issue89-complete-timeline.md §2 (TL01).
type TimelineClock struct{ reference int64 }

// NewTimelineClock selects only exact "1". Disabled construction neither reads
// the clock nor parses an inherited reference; malformed/unsupported selected
// clocks return closed diagnostic failure, not permission to change execution.
func NewTimelineClock(selection, reference string) (*TimelineClock, error) {
	if !Enabled(selection) {
		return nil, nil
	}
	parsed, err := strconv.ParseInt(reference, 10, 64)
	if err != nil || parsed <= 0 || strconv.FormatInt(parsed, 10) != reference || !timelineClockSupported() {
		return nil, errEvidence
	}
	return &TimelineClock{reference: parsed}, nil
}

// Read returns only reference-relative nanoseconds on the shared Linux axis.
// No calendar fallback, uptime or raw syscall error enters the wire/report.
func (c *TimelineClock) Read() (int64, error) {
	if c == nil {
		return 0, errEvidence
	}
	now, err := readTimelineMonotonicNS()
	if err != nil {
		return 0, errEvidence
	}
	return timelineClockOffset(now, c.reference)
}

// timelineClockOffset validates interoperable relative integers before they
// leave the process. Large absolute uptime is allowed; it is never published.
func timelineClockOffset(now, reference int64) (int64, error) {
	if reference <= 0 || now < reference || now-reference > maxTimelineNumber {
		return 0, errEvidence
	}
	return now - reference, nil
}
