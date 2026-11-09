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
