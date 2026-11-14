package preparecost

import (
	"bytes"
	"strconv"
	"strings"
	"testing"
)

// TL01/TL02: real shared-clock coordinates and emitted window, with no private
// reference or product test hook exposed to the observer.
func TestTimelineRecorderSharedCoordinatesAndObserver(t *testing.T) {
	reference, err := readTimelineMonotonicNS()
	if err != nil {
		t.Fatal(err)
	}
	clock, err := NewTimelineClock("1", strconv.FormatInt(reference, 10))
	if err != nil {
		t.Fatal(err)
	}
	before, err := clock.Read()
	if err != nil {
		t.Fatal(err)
	}
	r := NewTimelineRecorder(clock, 12)
	root, end := Start(WithRecorder(t.Context(), r), Request)
	_, admission := Start(WithPhase(root, PhaseAdmission, 0), Admission)
	_, worker := Start(root, Worker)
	worker(Succeeded)
	admission(Succeeded)
	end(Succeeded)
	report := r.Finish()
	after, err := clock.Read()
	if err != nil || !report.Complete || report.Clock == nil || report.Clock.OriginNS < before ||
		report.Clock.OriginNS+report.ElapsedNS > after || len(report.Spans) != 3 {
		t.Fatal("report is not on the shared reference axis", err)
	}
	copy := r.Finish()
	copy.Clock.OriginNS = 0
	if r.Finish().Clock.OriginNS != report.Clock.OriginNS {
		t.Fatal("snapshot aliases clock frame")
	}
	var output bytes.Buffer
	if err := NewEmitter(&output).Emit(report); err != nil {
		t.Fatal(err)
	}
	var start, finish int64
	for _, line := range strings.Split(output.String(), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 5 && fields[1] == "observer-window" {
			start, err = strconv.ParseInt(fields[3], 10, 64)
			if err != nil {
				t.Fatal(err)
			}
			finish, err = strconv.ParseInt(fields[4], 10, 64)
			if err != nil {
				t.Fatal(err)
			}
		}
	}
	if start < report.Clock.OriginNS+report.ElapsedNS || finish < start || finish == 0 {
		t.Fatal("observer window is absent or not separately timed")
	}
	if strings.Contains(output.String(), strconv.FormatInt(reference, 10)) {
		t.Fatal("absolute host uptime escaped")
	}
	broken, err := NewTimelineClock("1", "9223372036854775807")
	if err != nil {
		t.Fatal(err)
	}
	if NewTimelineRecorder(broken, 13).Finish().Complete {
		t.Fatal("unreadable relative clock qualified")
	}
}
