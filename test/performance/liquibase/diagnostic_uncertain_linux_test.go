package benchmark

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"
)

// EU01/EU02: these hand-authored lifecycles retain time, never a successful
// requested image. Strict and measured decoding deliberately have separate calls.
func TestDiagnosticMeasuredUnconfirmedLifecycle(t *testing.T) {
	for _, call := range []string{
		`execve("/PRIVATE_SENTINEL/java", ["PRIVATE_SENTINEL"], 0x0 `,
		`execveat(AT_FDCWD, "/PRIVATE_SENTINEL/java", ["PRIVATE_SENTINEL"], 0x0, 0 `,
	} {
		for _, terminal := range []string{"+++ exited with 0 +++", "+++ exited with 1 +++", "+++ killed by SIGTERM +++"} {
			pending := "2 100.2 " + call + "<unfinished ...>\n2 100.5 " + terminal + "\n"
			if _, err := decodeDiagnosticTrace(strings.NewReader(pending), 1); diagnosticDecodeCode(err) != 5 {
				t.Fatal("strict pending-exit rejection changed")
			}
			for _, prior := range []bool{false, true} {
				raw := pending
				want := []diagnosticInterval{{"exec.unconfirmed", 100200000000, 100500000000}}
				if prior {
					raw = "2 100.0 execve(\"/bin/java\", [\"java\"], 0x0) = 0\n" + raw
					want = append([]diagnosticInterval{{"host.java", 100000000000, 100200000000}}, want...)
				}
				got, err := decodeMeasuredDiagnosticTrace(strings.NewReader(raw), 1)
				if err != nil || !reflect.DeepEqual(got, want) {
					t.Fatalf("uncertain image/time differs: %v", err)
				}
				data, err := json.Marshal(got)
				if err != nil || strings.Contains(string(data), "PRIVATE_SENTINEL") {
					t.Fatal("private command data escaped")
				}
			}
			if got, err := decodeMeasuredDiagnosticTrace(strings.NewReader(pending), 2); err != nil || len(got) != 0 {
				t.Fatal("engine lifetime counted as a child command")
			}
		}
	}
	// Failed exec retains the previously known image. A real success still
	// closes it and starts the confirmed category, not an uncertain interval.
	raw := "2 100.0 execve(\"/bin/java\", [\"java\"], 0x0) = 0\n" +
		"2 100.1 execve(\"/missing\", [\"missing\"], 0x0) = -1 ENOENT (missing)\n" +
		"2 100.2 execve(\"/bin/docker\", [\"docker\", \"inspect\"], 0x0 <unfinished ...>\n" +
		"2 100.3 <... execve resumed>) = 0\n2 100.5 +++ exited with 0 +++\n"
	want := []diagnosticInterval{{"host.java", 100000000000, 100200000000}, {"docker.inspect", 100200000000, 100500000000}}
	if got, err := decodeMeasuredDiagnosticTrace(strings.NewReader(raw), 1); err != nil || !reflect.DeepEqual(got, want) {
		t.Fatal("known exec outcomes changed")
	}
}

// EU02: measured mode is not permission to discard malformed or unfinished
// evidence. These cases do not depend on any private collector interception.
func TestDiagnosticMeasuredUnconfirmedRejectsMissingEvidence(t *testing.T) {
	entry := "2 100.1 execve(\"/bin/true\", [\"true\"], 0x0 <unfinished ...>\n"
	for name, raw := range map[string]string{
		"missing_exit":       entry,
		"foreign_exit":       entry + "3 100.2 +++ exited with 0 +++\n",
		"reverse_time":       entry + "2 100.0 +++ exited with 0 +++\n",
		"duplicate_pending":  entry + entry,
		"orphan_resume":      "2 100.2 <... execve resumed>) = 0\n",
		"not_exec_entry":     "2 100.1 PRIVATE_SENTINEL <unfinished ...>\n2 100.2 +++ exited with 0 +++\n",
		"missing_program":    "2 100.1 execve(0x0, [], 0x0 <unfinished ...>\n2 100.2 +++ exited with 0 +++\n",
		"truncated_argv":     "2 100.1 execve(\"/bin/true\", [\"true\", ...], 0x0 <unfinished ...>\n2 100.2 +++ exited with 0 +++\n",
		"bad_program_escape": "2 100.1 execve(\"\\q\", [], 0x0 <unfinished ...>\n2 100.2 +++ exited with 0 +++\n",
		"line_quota":         strings.Repeat("x", 65537),
		"row_quota":          strings.Repeat("2 100.1 +++ exited with 0 +++\n", 250001),
	} {
		t.Run(name, func(t *testing.T) {
			rows, err := decodeMeasuredDiagnosticTrace(strings.NewReader(raw), 1)
			if err == nil || rows != nil || strings.Contains(err.Error(), "PRIVATE_SENTINEL") {
				t.Fatal("incomplete/corrupt trace accepted or disclosed")
			}
		})
	}
	if rows, err := decodeMeasuredDiagnosticTrace(helperFailingReader{errors.New("PRIVATE_SENTINEL")}, 1); err == nil || rows != nil || strings.Contains(err.Error(), "PRIVATE_SENTINEL") {
		t.Fatal("read failure accepted or disclosed")
	}
	// Two lifecycles for PID 2 plus an interleaved PID 3 and non-exec PID 4.
	raw := entry + "3 100.15 execve(\"/bin/true\", [\"true\"], 0x0) = 0\n" +
		"4 100.16 +++ exited with 0 +++\n2 100.2 +++ exited with 0 +++\n" +
		"2 100.3 execve(\"/bin/java\", [\"java\"], 0x0) = 0\n" +
		"3 100.4 +++ exited with 0 +++\n2 100.5 +++ exited with 0 +++\n"
	want := []diagnosticInterval{{"exec.unconfirmed", 100100000000, 100200000000}, {"other", 100150000000, 100400000000}, {"host.java", 100300000000, 100500000000}}
	if got, err := decodeMeasuredDiagnosticTrace(strings.NewReader(raw), 1); err != nil || !reflect.DeepEqual(got, want) {
		t.Fatal("PID histories merged or fabricated")
	}
}

// EU03: two one-second API windows separated by a one-second gap. All expected
// clipped lengths and union totals are independent literal numbers.
func TestDiagnosticMeasuredUnconfirmedWindows(t *testing.T) {
	for _, row := range []struct {
		name    string
		lo, hi  int64
		covered [2]float64
	}{
		{"first", 100200000000, 100400000000, [2]float64{200, 0}},
		{"second", 102200000000, 102400000000, [2]float64{0, 200}},
		{"cross_boundary", 100800000000, 101200000000, [2]float64{200, 0}},
		{"gap", 101200000000, 101800000000, [2]float64{0, 0}},
		{"after", 103200000000, 103800000000, [2]float64{0, 0}},
		{"both", 100800000000, 102200000000, [2]float64{200, 200}},
		{"empty", 100200000000, 100200000000, [2]float64{0, 0}},
	} {
		t.Run(row.name, func(t *testing.T) {
			for i, seconds := range []int64{100, 102} {
				got, err := summarizeDiagnosticIntervals([]diagnosticInterval{{"exec.unconfirmed", row.lo, row.hi}}, time.Unix(seconds, 0), time.Unix(seconds+1, 0))
				count := 0
				if row.covered[i] > 0 {
					count = 1
				}
				if err != nil || got.CoveredMS != row.covered[i] || got.ResidualMS != 1000-row.covered[i] || got.Commands != count {
					t.Fatal("window/zero-duration accounting differs")
				}
			}
		})
	}
	rows := []diagnosticInterval{{"host.java", 100000000000, 100200000000}, {"exec.unconfirmed", 100200000000, 100500000000}, {"docker.exec", 100100000000, 100400000000}}
	got, err := summarizeDiagnosticIntervals(rows, time.Unix(100, 0), time.Unix(101, 0))
	if err != nil || got.CoveredMS != 500 || got.ResidualMS != 500 || got.Commands != 3 {
		t.Fatal("overlap was added rather than unioned")
	}
}

// EU05 uses the actual private pipe and finish path. No Docker/strace workload
// runs locally; existing group-stop/transport regressions remain in the suite.
func TestDiagnosticMeasuredFinishAndWiring(t *testing.T) {
	for _, measured := range []bool{false, true} {
		trace, err := newDiagnosticTrace(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		fd, err := syscall.Dup(int(trace.writer.Fd()))
		if err != nil {
			trace.abort()
			t.Fatal(err)
		}
		writer := os.NewFile(uintptr(fd), "private-test-writer")
		trace.begin(0)
		_, writeErr := io.WriteString(writer, "2 100.1 execve(\"/bin/true\", [\"true\"], 0x0 <unfinished ...>\n2 100.2 +++ exited with 0 +++\n")
		writer.Close()
		if writeErr != nil {
			trace.abort()
			t.Fatal(writeErr)
		}
		var rows []diagnosticInterval
		if measured {
			rows, err = trace.finishMeasured(1)
		} else {
			rows, err = trace.finish(1)
		}
		if measured {
			if err != nil || !reflect.DeepEqual(rows, []diagnosticInterval{{"exec.unconfirmed", 100100000000, 100200000000}}) {
				t.Fatal("measured finish did not select explicit policy")
			}
		} else if diagnosticDecodeCode(err) != 5 {
			t.Fatal("default finish is no longer strict")
		}
	}
	collector, err := os.ReadFile("collector_linux_test.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(collector)
	stop, finish := strings.Index(source, "stopDiagnosticGroup("), strings.Index(source, "trace.finishMeasured(enginePID)")
	if stop < 0 || finish <= stop || !strings.Contains(source[stop:finish], `P01B_DIAGNOSTIC_FAILURE process_tree`) {
		t.Fatal("measured collector must stop its owned tree before decoding")
	}
}
