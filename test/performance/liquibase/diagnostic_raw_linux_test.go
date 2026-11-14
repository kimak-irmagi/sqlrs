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

const rawTestEntry = "syscall_0xabc(0, 0x1, 0xffffffffffffffff, 0x2, 0, 0x3"
const rawTestExit = "2 100.3 +++ exited with 0 +++\n"

// UR01 validates the entire bounded Linux/amd64 grammar, not the prefix label.
// Expected segments are literals independent of the implementation's parser.
func TestDiagnosticRawGrammar(t *testing.T) {
	for _, name := range []string{"syscall_0xabc", "???", "syscall_0xffffffffffffffff"} {
		entry := strings.Replace(rawTestEntry, "syscall_0xabc", name, 1)
		for _, returned := range []string{"0", "0x1", "0xffffffffffffffff", "-1 ENOSYS (Function not implemented)", "-1 E1_ (PRIVATE_SENTINEL)", "?", "? <unavailable>"} {
			for _, duration := range []string{"", " <0.000001>", " <1.123456789>", " <9223372036.0>", " <9223372036.854775807>"} {
				for _, split := range []bool{false, true} {
					raw := "2 100.1 " + entry + ") = " + returned + duration + "\n" + rawTestExit
					if split {
						raw = "2 100.1 " + entry + " <unfinished ...>\n2 100.2 <... " + name + " resumed>) = " + returned + duration + "\n" + rawTestExit
					}
					got, err := decodeMeasuredDiagnosticTrace(strings.NewReader(raw), 1)
					if err != nil || !reflect.DeepEqual(got, []diagnosticInterval{{"process.unattributed", 100100000000, 100300000000}}) {
						t.Fatal("valid raw grammar did not retain unknown process time")
					}
					data, err := json.Marshal(got)
					if err != nil || strings.Contains(string(data), "PRIVATE_SENTINEL") || strings.Contains(string(data), "ffffffffffffffff") || strings.Contains(string(data), "syscall_") {
						t.Fatal("raw fields escaped serialization")
					}
				}
			}
		}
	}
	for _, body := range []string{
		"syscall_0x1(0,0,0,0,0,0)\t=\t0",
		"???(0 ,\t0x1, 0x2,0,0,0) = 0 <0.1>",
	} {
		if _, err := decodeMeasuredDiagnosticTrace(strings.NewReader("2 100.1 "+body+"\n"+rawTestExit), 1); err != nil {
			t.Fatal("horizontal formatting rejected")
		}
	}
	bad := []string{
		strings.Replace(rawTestEntry, "0xabc", "0x", 1),
		strings.Replace(rawTestEntry, "0xabc", "0xABC", 1),
		strings.Replace(rawTestEntry, "0xabc", "0x10000000000000000", 1),
		strings.Replace(rawTestEntry, "syscall_0xabc", "private_call", 1),
		strings.Replace(rawTestEntry, "0x1", "1", 1),
		strings.Replace(rawTestEntry, "0x1", "-1", 1),
		strings.Replace(rawTestEntry, "0x1", "0xG", 1),
		strings.Replace(rawTestEntry, "0x1", "0x10000000000000000", 1),
		strings.Replace(rawTestEntry, "0x1", `"PRIVATE_SENTINEL"`, 1),
		strings.Replace(rawTestEntry, "0x1", "NULL", 1),
		strings.Replace(rawTestEntry, "0, 0x1,", "0 0x1,", 1),
		strings.Replace(rawTestEntry, ", 0x3", "", 1), rawTestEntry + ", 0",
		strings.Replace(rawTestEntry, ", 0x3", ",", 1), rawTestEntry + "junk",
		strings.Replace(rawTestEntry, ", 0x3", ",\r0x3", 1),
		strings.Replace(rawTestEntry, ", 0x3", ",\n0x3", 1),
	}
	for _, entry := range bad {
		for _, suffix := range []string{") = 0\n", " <unfinished ...>\n"} {
			assertRawRejected(t, "2 100.1 "+entry+suffix+rawTestExit)
		}
	}
	for _, returned := range []string{
		"1", "-1", "-2", "+0", "0x", "0x10000000000000000", "0xA", "0 junk", "0 <0..1>",
		"0 <1.1234567890>", "0 <-1.1>", "0 <9223372036.854775808>", "0 <0.>",
		"-1 errno (text)", "-1 1ERR (text)", "-1 " + strings.Repeat("E", 33) + " (text)",
		"-1 E ()", "-1 E (" + strings.Repeat("a", 257) + ")", "-1 E (nested(text))",
		"-1 E (PRIVATE_SENTINEL\r)", "? other", "? <unavailable> junk",
	} {
		assertRawRejected(t, "2 100.1 "+rawTestEntry+") = "+returned+"\n"+rawTestExit)
	}
	_, err := decodeDiagnosticTrace(strings.NewReader("2 100.1 "+rawTestEntry+") = 0\n"+rawTestExit), 1)
	if diagnosticDecodeCode(err) != 8 {
		t.Fatal("strict complete-record rejection changed")
	}
}

func assertRawRejected(t *testing.T, raw string) {
	t.Helper()
	rows, err := decodeMeasuredDiagnosticTrace(strings.NewReader(raw), 1)
	if err == nil || rows != nil || strings.Contains(err.Error(), "PRIVATE_SENTINEL") {
		t.Fatal("invalid/incomplete raw evidence accepted or disclosed")
	}
}

// UR02 preserves pending identity, terminal proof and all-or-nothing decoding.
func TestDiagnosticRawLifecycle(t *testing.T) {
	pending := "2 100.1 " + rawTestEntry + " <unfinished ...>\n"
	for _, terminal := range []string{"+++ exited with 0 +++", "+++ exited with 1 +++", "+++ killed by SIGTERM +++"} {
		raw := pending + "2 100.3 " + terminal + "\n"
		got, err := decodeMeasuredDiagnosticTrace(strings.NewReader(raw), 1)
		if err != nil || !reflect.DeepEqual(got, []diagnosticInterval{{"process.unattributed", 100100000000, 100300000000}}) {
			t.Fatal("raw terminal proof failed")
		}
		if _, err := decodeDiagnosticTrace(strings.NewReader(raw), 1); diagnosticDecodeCode(err) != 5 {
			t.Fatal("strict pending terminal rule changed")
		}
		if got, err := decodeMeasuredDiagnosticTrace(strings.NewReader(raw), 2); err != nil || len(got) != 0 {
			t.Fatal("engine lifetime was counted")
		}
	}
	for _, raw := range []string{
		pending, pending + "3 100.3 +++ exited with 0 +++\n",
		pending + "2 100.0 +++ exited with 0 +++\n", pending + pending,
		"2 100.2 <... syscall_0xabc resumed>) = 0\n" + rawTestExit,
		pending + "3 100.2 <... syscall_0xabc resumed>) = 0\n" + rawTestExit,
		pending + "2 100.2 <... syscall_0xabd resumed>) = 0\n" + rawTestExit,
		pending + "2 100.2 <... ??? resumed>) = 0\n" + rawTestExit,
		pending + "2 100.2 execve(\"/bin/true\", [], 0x0) = 0\n" + rawTestExit,
		pending + "2 100.2 " + rawTestEntry + ") = 0\n" + rawTestExit,
		"2 100.1 " + rawTestEntry + ") = 0\n",
		"2 100.1 " + rawTestEntry + ") = ?\n",
		"2 100.1 " + rawTestEntry + ") = ? <unavailable>\n2 100.2 execve(\"/bin/true\", [], 0x0) = 0\n" + rawTestExit,
		"2 100.1 " + rawTestEntry + ") = 0\n2 100.2 execve(\"/bin/true\", [], 0x0) = 0\n",
		"2 100.0 execve(\"/bin/true\", [], 0x0 <unfinished ...>\n2 100.1 " + rawTestEntry + ") = 0\n" + rawTestExit,
	} {
		assertRawRejected(t, raw)
	}
	if rows, err := decodeMeasuredDiagnosticTrace(helperFailingReader{errors.New("PRIVATE_SENTINEL")}, 1); err == nil || rows != nil {
		t.Fatal("reader failure accepted")
	}
	raw := pending + "3 100.15 ???(0,0,0,0,0,0) = 0\n2 100.2 <... syscall_0xabc resumed>) = 0\n" + rawTestExit + "3 100.4 +++ exited with 0 +++\n"
	want := []diagnosticInterval{{"process.unattributed", 100100000000, 100300000000}, {"process.unattributed", 100150000000, 100400000000}}
	if got, err := decodeMeasuredDiagnosticTrace(strings.NewReader(raw), 1); err != nil || !reflect.DeepEqual(got, want) {
		t.Fatal("interleaved PID histories changed")
	}
}

// UR03: returns and failed execs cannot restore a previous executable identity.
func TestDiagnosticRawImageBoundaries(t *testing.T) {
	java := "2 100.0 execve(\"/bin/java\", [\"java\"], 0x0) = 0\n"
	unknown := "2 100.1 " + rawTestEntry + ") = 0\n"
	for _, later := range []string{
		"", "2 100.2 ???(0,0,0,0,0,0) = -1 ENOSYS (Function not implemented)\n",
		"2 100.2 execve(\"/missing\", [], 0x0) = -1 ENOENT (missing)\n",
	} {
		want := []diagnosticInterval{{"host.java", 100000000000, 100100000000}, {"process.unattributed", 100100000000, 100300000000}}
		if got, err := decodeMeasuredDiagnosticTrace(strings.NewReader(java+unknown+later+rawTestExit), 1); err != nil || !reflect.DeepEqual(got, want) {
			t.Fatal("unknown time was reassigned to a known image")
		}
	}
	for _, execBody := range []string{
		"2 100.2 execve(\"/bin/docker\", [\"docker\", \"inspect\"], 0x0) = 0\n",
		"2 100.2 execve(\"/bin/docker\", [\"docker\", \"inspect\"], 0x0 <unfinished ...>\n2 100.25 <... execve resumed>) = 0\n",
		"2 100.2 execve(\"/bin/docker\", [\"docker\", \"inspect\"], 0x0 <unfinished ...>\n",
	} {
		category := "docker.inspect"
		if !strings.Contains(execBody, "= 0") {
			category = "exec.unconfirmed"
		}
		want := []diagnosticInterval{{"host.java", 100000000000, 100100000000}, {"process.unattributed", 100100000000, 100200000000}, {category, 100200000000, 100300000000}}
		if got, err := decodeMeasuredDiagnosticTrace(strings.NewReader(java+unknown+execBody+rawTestExit), 1); err != nil || !reflect.DeepEqual(got, want) {
			t.Fatal("confirmed/uncertain exec boundary changed")
		}
	}
	raw := unknown + rawTestExit + "2 101.0 ???(0,0,0,0,0,0) = 0\n2 101.2 +++ exited with 0 +++\n"
	if got, err := decodeMeasuredDiagnosticTrace(strings.NewReader(raw), 1); err != nil || !reflect.DeepEqual(got, []diagnosticInterval{{"process.unattributed", 100100000000, 100300000000}, {"process.unattributed", 101000000000, 101200000000}}) {
		t.Fatal("sequential PID lifetimes merged")
	}
}

// UR04 keeps literal clipping/union expectations, not a sum of raw call times.
func TestDiagnosticRawWindows(t *testing.T) {
	for _, row := range []struct {
		lo, hi int64
		want   [2]float64
	}{
		{99200000000, 99400000000, [2]float64{0, 0}},
		{100200000000, 100400000000, [2]float64{200, 0}},
		{100800000000, 101200000000, [2]float64{200, 0}},
		{101200000000, 101800000000, [2]float64{0, 0}},
		{100800000000, 102200000000, [2]float64{200, 200}},
		{103200000000, 103800000000, [2]float64{0, 0}},
		{100200000000, 100200000000, [2]float64{0, 0}},
	} {
		for i, seconds := range []int64{100, 102} {
			got, err := summarizeDiagnosticIntervals([]diagnosticInterval{{"process.unattributed", row.lo, row.hi}}, time.Unix(seconds, 0), time.Unix(seconds+1, 0))
			count := 0
			if row.want[i] > 0 {
				count = 1
			}
			if err != nil || got.CoveredMS != row.want[i] || got.ResidualMS != 1000-row.want[i] || got.Commands != count {
				t.Fatal("unattributed window/count differs")
			}
		}
	}
	got, err := summarizeDiagnosticIntervals([]diagnosticInterval{{"host.java", 100000000000, 100200000000}, {"process.unattributed", 100200000000, 100500000000}, {"exec.unconfirmed", 100100000000, 100400000000}}, time.Unix(100, 0), time.Unix(101, 0))
	if err != nil || got.CoveredMS != 500 || got.ResidualMS != 500 || got.Commands != 3 {
		t.Fatal("unattributed overlap double counted")
	}
}

// UR05 uses the actual private transport; strict decoding stays a separate call.
func TestDiagnosticRawPrivateTransport(t *testing.T) {
	for _, mode := range []string{"measured", "strict", "malformed", "missing_exit"} {
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
		raw := "2 100.1 " + rawTestEntry + ") = 0\n" + rawTestExit
		if mode == "malformed" {
			raw = strings.Replace(raw, "0x1", "PRIVATE_SENTINEL", 1)
		}
		if mode == "missing_exit" {
			raw = strings.TrimSuffix(raw, rawTestExit)
		}
		_, writeErr := io.WriteString(writer, raw)
		writer.Close()
		if writeErr != nil {
			trace.abort()
			t.Fatal("private fixture write failed")
		}
		var rows []diagnosticInterval
		if mode == "strict" {
			rows, err = trace.finish(1)
		} else {
			rows, err = trace.finishMeasured(1)
		}
		if mode == "measured" {
			if err != nil || !reflect.DeepEqual(rows, []diagnosticInterval{{"process.unattributed", 100100000000, 100300000000}}) {
				t.Fatal("private measured transport failed")
			}
		} else if err == nil || rows != nil {
			t.Fatal("invalid/strict transport succeeded")
		}
	}
}
