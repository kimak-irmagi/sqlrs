package preparecost

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
)

// TL01: a separate ordinary test process reads the actual Linux host clock.
// No Docker, benchmark workload, raw uptime or product test hook is involved.
func TestTimelineClockRealSharedReference(t *testing.T) {
	if reference := os.Getenv("SQLRS_TEST_TIMELINE_REFERENCE"); reference != "" {
		clock, err := NewTimelineClock("1", reference)
		if err != nil {
			t.Fatal(err)
		}
		offset, err := clock.Read()
		if err != nil {
			t.Fatal(err)
		}
		fmt.Printf("TIMELINE_TEST_OFFSET %d\n", offset)
		return
	}
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
	path, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command := exec.CommandContext(t.Context(), path, "-test.run=^TestTimelineClockRealSharedReference$")
	command.Env = append(os.Environ(), "SQLRS_TEST_TIMELINE_REFERENCE="+strconv.FormatInt(reference, 10))
	output, err := command.Output()
	if err != nil {
		t.Fatal("shared-clock child failed")
	}
	line := strings.SplitN(string(output), "\n", 2)[0]
	value, err := strconv.ParseInt(strings.TrimPrefix(line, "TIMELINE_TEST_OFFSET "), 10, 64)
	if err != nil {
		t.Fatal("invalid closed relative clock response")
	}
	after, err := clock.Read()
	if err != nil || value < before || value > after {
		t.Fatal("processes do not share the relative monotonic axis")
	}
	future, err := NewTimelineClock("1", "9223372036854775807")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := future.Read(); err == nil {
		t.Fatal("future reference became qualified evidence")
	}
}
