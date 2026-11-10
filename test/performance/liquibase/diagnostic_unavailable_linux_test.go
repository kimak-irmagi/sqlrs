package benchmark

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"syscall"
	"testing"
	"time"
)

const unavailableEntry = "2 100.1 ???( <unfinished ...>\n"

// UE01/UE02: the approved terminal-only form is not a zero-argument raw call.
// See issue89-performance-diagnostic-tests.md; expectations are literal timelines.
func TestDiagnosticUnavailableGrammar(t *testing.T) {
	for _, space := range []string{" ", "\t", " \t  "} {
		for _, terminal := range []string{"+++ exited with 0 +++", "+++ exited with 42 +++", "+++ killed by SIGTERM +++", "+++ killed by SIGSEGV (core dumped) +++"} {
			raw := "2 100.1 ???(" + space + "<unfinished ...>\n2 100.3 " + terminal + "\n"
			got, err := decodeMeasuredDiagnosticTrace(strings.NewReader(raw), 1)
			want := []diagnosticInterval{{"process.unattributed", 100100000000, 100300000000}}
			if err != nil || !reflect.DeepEqual(got, want) {
				t.Fatal("exact terminal-only entry not retained")
			}
			if _, err := decodeDiagnosticTrace(strings.NewReader(raw), 1); diagnosticDecodeCode(err) != 5 {
				t.Fatal("strict terminal policy changed")
			}
		}
	}
	for _, body := range []string{
		"???(<unfinished ...>", "syscall_0xabc( <unfinished ...>", "other( <unfinished ...>",
		"???(0 <unfinished ...>", "???(<unavailable> <unfinished ...>", "???() <unfinished ...>",
		"???( <unfinished ...>) = 0", "???( <unfinished ...> PRIVATE_SENTINEL",
		"PRIVATE_SENTINEL ???( <unfinished ...>", "???(\v<unfinished ...>",
		"???(\n<unfinished ...>", "???(\r<unfinished ...>", "???( <unfinished ...>\r",
	} {
		assertRawRejected(t, "2 100.1 "+body+"\n"+rawTestExit)
	}
	_, err := parseDiagnosticRaw("???( <unfinished ...>")
	if diagnosticDecodeCode(err) != 8 || diagnosticErrorRaw(err) != (diagnosticRawFailure{"fallback", "empty", "none", "arity"}) {
		t.Fatal("direct six-argument raw grammar relaxed")
	}
}

// UE02: other PIDs can interleave, but no event can intervene for this PID.
func TestDiagnosticUnavailableLifecycle(t *testing.T) {
	closed := "4 99.0 execve(\"/bin/java\", [\"java\"], 0x0) = 0\n4 99.1 +++ exited with 0 +++\n"
	for _, event := range []string{
		"???( <unfinished ...>", rawTestEntry + ") = 0", "execve(\"/bin/java\", [\"java\"], 0x0) = 0",
		"<... ??? resumed>) = 0", "<... execve resumed>) = 0", ") = 0", "PRIVATE_SENTINEL",
		"+++ exited with 0 +++ junk",
	} {
		assertRawRejected(t, closed+unavailableEntry+"2 100.2 "+event+"\n"+rawTestExit)
	}
	for _, prior := range []string{
		rawTestEntry + " <unfinished ...>",
		"execve(\"/bin/java\", [\"java\"], 0x0 <unfinished ...>",
	} {
		assertRawRejected(t, closed+"2 100.0 "+prior+"\n"+unavailableEntry+rawTestExit)
	}
	for _, suffix := range []string{"", "3 100.3 +++ exited with 0 +++\n", "2 100.0 +++ exited with 0 +++\n"} {
		assertRawRejected(t, closed+unavailableEntry+suffix)
	}
	raw := unavailableEntry + "3 100.15 ???(\t<unfinished ...>\n" + rawTestExit + "3 100.4 +++ exited with 1 +++\n"
	want := []diagnosticInterval{{"process.unattributed", 100100000000, 100300000000}, {"process.unattributed", 100150000000, 100400000000}}
	if got, err := decodeMeasuredDiagnosticTrace(strings.NewReader(raw), 1); err != nil || !reflect.DeepEqual(got, want) {
		t.Fatal("interleaved unavailable lifecycles conflated")
	}
	input := io.MultiReader(strings.NewReader(closed+unavailableEntry), helperFailingReader{errors.New("PRIVATE_SENTINEL")})
	if rows, err := decodeMeasuredDiagnosticTrace(input, 1); err == nil || rows != nil {
		t.Fatal("reader failure returned partial intervals")
	}
	assertRawRejected(t, closed+unavailableEntry+strings.Repeat("x", 65537))
	// Exact exits are valid by themselves; exceeding the record limit is not.
	assertRawRejected(t, closed+unavailableEntry+strings.Repeat("3 100.2 +++ exited with 0 +++\n", 250001)+rawTestExit)
}

// UE03: expected boundaries do not use the recognition/parser implementation.
func TestDiagnosticUnavailableTimelines(t *testing.T) {
	for _, row := range []struct {
		prefix, suffix string
		engine         int
		want           []diagnosticInterval
	}{
		{"", rawTestExit, 1, []diagnosticInterval{{"process.unattributed", 100100000000, 100300000000}}},
		{"2 100.0 execve(\"/bin/java\", [\"java\"], 0x0) = 0\n", rawTestExit, 1, []diagnosticInterval{{"host.java", 100000000000, 100100000000}, {"process.unattributed", 100100000000, 100300000000}}},
		{"2 100.0 execve(\"/bin/docker\", [\"docker\", \"inspect\"], 0x0) = 0\n", rawTestExit, 1, []diagnosticInterval{{"docker.inspect", 100000000000, 100100000000}, {"process.unattributed", 100100000000, 100300000000}}},
		{"2 100.0 ???(0,0,0,0,0,0) = 0\n", rawTestExit, 1, []diagnosticInterval{{"process.unattributed", 100000000000, 100300000000}}},
		{"", rawTestExit, 2, []diagnosticInterval{}},
		{"", "2 100.1 +++ exited with 0 +++\n", 1, []diagnosticInterval{{"process.unattributed", 100100000000, 100100000000}}},
		{"", rawTestExit + "2 101.0 execve(\"/bin/java\", [\"java\"], 0x0) = 0\n2 101.2 +++ exited with 0 +++\n", 1, []diagnosticInterval{{"process.unattributed", 100100000000, 100300000000}, {"host.java", 101000000000, 101200000000}}},
	} {
		got, err := decodeMeasuredDiagnosticTrace(strings.NewReader(row.prefix+unavailableEntry+row.suffix), row.engine)
		if err != nil || !reflect.DeepEqual(got, row.want) {
			t.Fatal("terminal-only timeline differs from independent expectation")
		}
	}
	if rows, err := decodeMeasuredDiagnosticTrace(strings.NewReader(unavailableEntry+"2 100.2 execve(\"/bin/java\", [], 0x0) = 0\n"+rawTestExit), 2); err == nil || rows != nil {
		t.Fatal("excluded engine bypassed lifecycle validation")
	}
}

// UE04: real decoded timelines feed clipping/union; Commands is not JVM starts.
func TestDiagnosticUnavailableWindows(t *testing.T) {
	raw := "2 100.0 execve(\"/bin/java\", [\"java\"], 0x0) = 0\n" + unavailableEntry + "3 100.2 ???( <unfinished ...>\n" + rawTestExit + "3 100.5 +++ exited with 0 +++\n"
	rows, err := decodeMeasuredDiagnosticTrace(strings.NewReader(raw), 1)
	if err != nil {
		t.Fatal("window fixture rejected")
	}
	for _, row := range []struct {
		lo, hi            int64
		covered, residual float64
		intervals         []diagnosticPublicInterval
	}{
		{99000000000, 99500000000, 0, 500, []diagnosticPublicInterval{}},
		{100000000000, 101000000000, 500, 500, []diagnosticPublicInterval{{"host.java", 0, 100}, {"process.unattributed", 100, 300}, {"process.unattributed", 200, 500}}},
		{100200000000, 100400000000, 200, 0, []diagnosticPublicInterval{{"process.unattributed", 0, 100}, {"process.unattributed", 0, 200}}},
		{100500000000, 101000000000, 0, 500, []diagnosticPublicInterval{}},
	} {
		got, err := summarizeDiagnosticIntervals(rows, time.Unix(0, row.lo), time.Unix(0, row.hi))
		if err != nil || got.CoveredMS != row.covered || got.ResidualMS != row.residual || got.Commands != len(row.intervals) || !reflect.DeepEqual(got.Intervals, row.intervals) {
			t.Fatal("unavailable window/union differs")
		}
		data, err := json.Marshal(got)
		if err != nil || strings.Contains(string(data), "???") || strings.Contains(string(data), "100100000000") {
			t.Fatal("private source data escaped summary")
		}
	}
	zero, err := decodeMeasuredDiagnosticTrace(strings.NewReader(unavailableEntry+"2 100.1 +++ exited with 0 +++\n"), 1)
	if err != nil {
		t.Fatal("zero fixture rejected")
	}
	got, err := summarizeDiagnosticIntervals(zero, time.Unix(100, 0), time.Unix(101, 0))
	if err != nil || got.Commands != 0 || got.CoveredMS != 0 || got.ResidualMS != 1000 || len(got.Intervals) != 0 {
		t.Fatal("zero-length entry counted")
	}
}

// UE05: finish joins the real private reader and closes descriptors, while the
// owning root cleanup, not finish, removes private bytes. No native workload.
func TestDiagnosticUnavailablePrivatePipe(t *testing.T) {
	for _, mode := range []string{"measured", "strict", "missing_exit", "intermediate", "reader_error", "quota"} {
		t.Run(mode, func(t *testing.T) {
			root, err := os.MkdirTemp(t.TempDir(), "private-")
			if err != nil {
				t.Fatal(err)
			}
			trace, err := newDiagnosticTrace(root)
			if err != nil {
				t.Fatal(err)
			}
			fd, err := syscall.Dup(int(trace.writer.Fd()))
			if err != nil {
				trace.abort()
				t.Fatal(err)
			}
			writer := os.NewFile(uintptr(fd), "private-unavailable-writer")
			if mode == "reader_error" {
				trace.reader.Close()
			}
			trace.begin(0)
			raw := unavailableEntry + rawTestExit
			switch mode {
			case "missing_exit":
				raw = unavailableEntry
			case "intermediate":
				raw = unavailableEntry + "2 100.2 ???(0,0,0,0,0,0) = 0\n" + rawTestExit
			case "quota":
				raw = unavailableEntry + strings.Repeat("x", 65537) + "\n" + rawTestExit
			}
			_, writeErr := io.WriteString(writer, raw)
			writer.Close()
			if writeErr != nil && mode != "reader_error" {
				trace.abort()
				t.Fatal("fixture write failed")
			}
			var rows []diagnosticInterval
			if mode == "strict" {
				rows, err = trace.finish(1)
			} else {
				rows, err = trace.finishMeasured(1)
			}
			if mode == "measured" {
				if err != nil || !reflect.DeepEqual(rows, []diagnosticInterval{{"process.unattributed", 100100000000, 100300000000}}) {
					t.Fatal("private unavailable transport failed")
				}
			} else if err == nil || rows != nil {
				t.Fatal("incomplete/strict transport accepted")
			}
			if mode == "strict" && (diagnosticDecodeCode(err) != 5 || diagnosticErrorRaw(err) != (diagnosticRawFailure{"none", "none", "none", "none"})) {
				t.Fatal("strict transport contract changed")
			}
			for _, file := range []*os.File{trace.writer, trace.reader, trace.file} {
				if _, err := file.Stat(); !errors.Is(err, os.ErrClosed) {
					t.Fatal("private descriptor remains open")
				}
			}
			select {
			case <-trace.done:
				t.Fatal("reader completion was not consumed")
			default:
			}
			// Remove only this exact child of t.TempDir; owner cleanup is explicit.
			if err := os.Remove(filepath.Join(root, "diagnostic.trace")); err != nil {
				t.Fatal(err)
			}
			if err := os.Remove(root); err != nil {
				t.Fatal(err)
			}
			if _, err := os.Stat(root); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("private bytes retained")
			}
		})
	}
}
