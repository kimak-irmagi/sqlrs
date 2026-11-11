package benchmark

import "testing"

func timelineTimerExample() costTimelineTimer {
	return costTimelineTimer{Window: costTimelineWindow{100, 200},
		StartBracket: costTimelineWindow{99, 101}, EndBracket: costTimelineWindow{199, 201}, APIElapsedNS: 100}
}

// TL08 uses measured endpoint brackets, not a guessed fixed clock tolerance.
// Its signed difference is diagnostic, never a correction to speed samples.
func TestTimelineTimerBracketedSignedDifference(t *testing.T) {
	for _, elapsed := range []int64{98, 99, 100, 101, 102} {
		evidence := timelineTimerExample()
		evidence.APIElapsedNS = elapsed
		delta, err := qualifyTimelineTimer(evidence)
		if err != nil || delta != 100-elapsed {
			t.Fatal("bracketed timer difference", delta, err)
		}
	}
	for _, mutate := range []func(*costTimelineTimer){
		func(v *costTimelineTimer) { v.APIElapsedNS = 97 },
		func(v *costTimelineTimer) { v.APIElapsedNS = 103 },
		func(v *costTimelineTimer) { v.APIElapsedNS = 0 },
		func(v *costTimelineTimer) { v.Window.StartNS = 98 },
		func(v *costTimelineTimer) { v.Window.EndNS = 202 },
		func(v *costTimelineTimer) { v.StartBracket.StartNS = -1 },
		func(v *costTimelineTimer) { v.StartBracket.EndNS = 98 },
		func(v *costTimelineTimer) { v.EndBracket.EndNS = 198 },
		func(v *costTimelineTimer) { v.EndBracket.EndNS = 9007199254740992 },
		func(v *costTimelineTimer) { v.EndBracket.StartNS = 100 },
		func(v *costTimelineTimer) { v.Window.EndNS = v.Window.StartNS },
	} {
		evidence := timelineTimerExample()
		mutate(&evidence)
		if delta, err := qualifyTimelineTimer(evidence); err == nil || delta != 0 {
			t.Fatal("unproven timer alignment qualified", delta, err)
		}
	}
}
