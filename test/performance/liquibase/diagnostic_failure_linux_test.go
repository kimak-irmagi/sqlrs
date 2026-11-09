package benchmark

import (
	"errors"
	"strings"
	"testing"
)

// DG03/DG09: fixed parser checkpoints explain incomplete evidence without
// exposing even one original trace byte, PID, argument, path or credential.
func TestDiagnosticDecodeFailureCodesAreClosed(t *testing.T) {
	for _, row := range []struct {
		raw  string
		code int
	}{
		{"PRIVATE_SENTINEL\n", 2},
		{"2 100.1 execve(\"/bin/java\", [\"java\" ...], 0x0) = 0\n", 16},
		{"2 100.1 execve(\"/bin/java\", [\"java\"], 0x0) = 0\n", 17},
	} {
		_, err := decodeDiagnosticTrace(strings.NewReader(row.raw), 1)
		if err == nil || diagnosticDecodeCode(err) != row.code || strings.Contains(err.Error(), "PRIVATE_SENTINEL") {
			t.Fatal("decoder checkpoint missing or private")
		}
	}
	for _, err := range []error{nil, errors.New("PRIVATE_SENTINEL"), diagnosticDecodeError{0}, diagnosticDecodeError{99}} {
		if diagnosticDecodeCode(err) != 0 {
			t.Fatal("unreviewed checkpoint exposed")
		}
	}
	for i := 1; i <= 17; i++ {
		if diagnosticDecodeCode(diagnosticDecodeError{i}) != i {
			t.Fatal("reviewed checkpoint lost")
		}
	}
}

// DG03/DG05: an exact terminal record can close an unconfirmed attempt wholly
// after the final API boundary. It must never invent a launch or repair an
// incomplete execution inside the measured window. Default decoding stays strict.
func TestDiagnosticInterruptedExecAfterObservedWindow(t *testing.T) {
	trace := "2 100.1 execve(\"/bin/true\", [\"true\"], 0x0 <unfinished ...>\n2 100.2 +++ killed by SIGTERM +++\n"
	if _, err := decodeDiagnosticTrace(strings.NewReader(trace), 1); diagnosticDecodeCode(err) != 5 {
		t.Fatal("default decoding no longer strict")
	}
	rows, err := decodeDiagnosticTrace(strings.NewReader(trace), 1, 100050000000)
	if err != nil || len(rows) != 0 {
		t.Fatal("confirmed after-window disposal was attributed as a launch")
	}
	for _, cutoff := range []int64{100150000000, 100300000000} {
		if _, err := decodeDiagnosticTrace(strings.NewReader(trace), 1, cutoff); diagnosticDecodeCode(err) != 5 {
			t.Fatal("in-window incomplete execution accepted")
		}
	}
	withoutExit := strings.Split(trace, "\n")[0] + "\n"
	if _, err := decodeDiagnosticTrace(strings.NewReader(withoutExit), 1, 100050000000); diagnosticDecodeCode(err) != 17 {
		t.Fatal("unconfirmed process absence accepted")
	}
	prior := "2 100.0 execve(\"/bin/echo\", [\"echo\"], 0x0) = 0\n" + trace
	rows, err = decodeDiagnosticTrace(strings.NewReader(prior), 1, 100050000000)
	if err != nil || len(rows) != 1 || rows[0].StartNS != 100000000000 || rows[0].EndNS != 100200000000 {
		t.Fatal("preceding confirmed image history was lost")
	}
}
