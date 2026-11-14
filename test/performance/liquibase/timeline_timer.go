package benchmark

// costTimelineTimer keeps the unchanged Go elapsed and independently sampled
// shared-clock bounds distinct. Requirements: complete-timeline.md §2, TL08.
type costTimelineTimer struct {
	Window, StartBracket, EndBracket costTimelineWindow
	APIElapsedNS                     int64
}

// qualifyTimelineTimer validates actual read uncertainty without adjusting
// either elapsed time or calling the signed difference application work.
func qualifyTimelineTimer(evidence costTimelineTimer) (int64, error) {
	for _, interval := range []costTimelineWindow{evidence.Window, evidence.StartBracket, evidence.EndBracket} {
		if interval.StartNS < 0 || interval.EndNS < interval.StartNS || uint64(interval.EndNS) > maxCostTimelineNumber {
			return 0, errCostStream
		}
	}
	if evidence.APIElapsedNS <= 0 || evidence.Window.StartNS == evidence.Window.EndNS ||
		evidence.StartBracket.EndNS > evidence.EndBracket.StartNS ||
		evidence.Window.StartNS < evidence.StartBracket.StartNS || evidence.Window.StartNS > evidence.StartBracket.EndNS ||
		evidence.Window.EndNS < evidence.EndBracket.StartNS || evidence.Window.EndNS > evidence.EndBracket.EndNS ||
		evidence.APIElapsedNS < evidence.EndBracket.StartNS-evidence.StartBracket.EndNS ||
		evidence.APIElapsedNS > evidence.EndBracket.EndNS-evidence.StartBracket.StartNS {
		return 0, errCostStream
	}
	return evidence.Window.EndNS - evidence.Window.StartNS - evidence.APIElapsedNS, nil
}
