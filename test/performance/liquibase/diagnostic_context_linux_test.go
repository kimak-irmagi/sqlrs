package benchmark

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

// CX01: recognize a closed shape without accepting that shape as valid trace
// syntax. Fixtures are independent of the classifier and contain private decoys.
func TestDiagnosticContextShapesAndActualFailures(t *testing.T) {
	for _, row := range []struct{ raw, want string }{
		{"+++ superseded by execve in pid 987654321 +++", "superseded_exec"},
		{`execve("/PRIVATE_SENTINEL", [], 0x0) = ?`, "exec_entry"},
		{`execveat(AT_FDCWD, "/PRIVATE_SENTINEL", [], 0x0, 0) = ?`, "exec_entry"},
		{"<... execve resumed>) = ?", "exec_resume"},
		{"<... execveat resumed>) = ?", "exec_resume"},
		{"+++ superseded by execve in pid PRIVATE_SENTINEL +++", "other"},
		{"+++ superseded by execve in pid 12 +++ PRIVATE_SENTINEL", "other"},
		{"PRIVATE_SENTINEL +++ superseded by execve in pid 12 +++", "other"},
		{"+++ superseded by execve in pid 0 +++", "other"},
		{"+++ superseded by execve in pid -12 +++", "other"},
		{"<... execve resumed_PRIVATE_SENTINEL>", "other"},
		{`"+++ superseded by execve in pid 12 +++"`, "other"},
	} {
		if got := diagnosticRecordShape(row.raw); got != row.want {
			t.Fatalf("fixed shape differs: %s", got)
		}
	}
	for _, row := range []struct {
		raw, location, shape string
		code                 int
	}{
		{"2 100.1 +++ superseded by execve in pid 987654321 +++\n", "record", "superseded_exec", 8},
		{"2 100.1 PRIVATE_SENTINEL\n", "record", "other", 8},
		{"2 100.1 PRIVATE_SENTINEL <unfinished ...>\n2 100.2 +++ exited with 0 +++\n", "pending_entry", "other", 8},
		{"2 100.1 execve(\"/PRIVATE_SENTINEL\", [\"bad\" ...], 0x0 <unfinished ...>\n2 100.2 +++ exited with 0 +++\n", "pending_entry", "exec_entry", 16},
		{"2 100.1 execve(\"/PRIVATE_SENTINEL\", [\"bad\" ...], 0x0) = 0\n", "record", "exec_entry", 16},
	} {
		rows, err := decodeMeasuredDiagnosticTrace(strings.NewReader(row.raw), 1)
		context := diagnosticErrorContext(err)
		if rows != nil || err == nil || diagnosticDecodeCode(err) != row.code || context.Location != row.location || context.Shape != row.shape {
			t.Fatal("actual failed decoder lost its exact code/location/shape")
		}
		var original diagnosticDecodeError
		if !errors.As(err, &original) || original.code != row.code {
			t.Fatal("original checkpoint no longer reachable")
		}
		for _, candidate := range []error{err, fmt.Errorf("outer: %w", err)} {
			data, marshalErr := json.Marshal(diagnosticErrorContext(candidate))
			if marshalErr != nil || strings.Contains(string(data), "PRIVATE_SENTINEL") || strings.Contains(string(data), "987654321") || strings.Contains(candidate.Error(), "PRIVATE_SENTINEL") || diagnosticDecodeCode(candidate) != row.code {
				t.Fatal("wrapped context lost code or exposed private trace data")
			}
		}
	}
}

// CX02: corrupt/foreign metadata is not a raw-data transport. Direct checkpoint
// errors keep the original code and have explicitly unavailable context.
func TestDiagnosticContextDefaultsAreClosed(t *testing.T) {
	for _, row := range []struct {
		err             error
		location, shape string
	}{
		{nil, "none", "none"},
		{errors.New("PRIVATE_SENTINEL"), "none", "none"},
		{diagnosticDecodeError{8}, "none", "none"},
		{diagnosticContextError{cause: diagnosticDecodeError{8}, location: "PRIVATE_SENTINEL", shape: "PRIVATE_SENTINEL"}, "none", "none"},
		{diagnosticContextError{cause: diagnosticDecodeError{8}, location: "record", shape: "PRIVATE_SENTINEL"}, "record", "none"},
		{diagnosticContextError{cause: diagnosticDecodeError{8}, location: "pending_entry", shape: "exec_resume"}, "pending_entry", "exec_resume"},
	} {
		context := diagnosticErrorContext(row.err)
		data, err := json.Marshal(context)
		if err != nil || context.Location != row.location || context.Shape != row.shape || strings.Contains(string(data), "PRIVATE_SENTINEL") {
			t.Fatal("unknown metadata escaped its closed default")
		}
	}
}

// CX04: the collector exports only context extracted from the error; its public
// formatter cannot receive a raw body. Actual artifact wiring is also in CX05.
func TestDiagnosticContextCollectorWiring(t *testing.T) {
	data, err := os.ReadFile("collector_linux_test.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	start := strings.Index(source, "if traceErr != nil {")
	if start < 0 {
		t.Fatal("trace failure branch missing")
	}
	end := strings.Index(source[start:], "} else {")
	if end < 0 {
		t.Fatal("trace success branch missing")
	}
	branch := source[start : start+end]
	if !strings.Contains(branch, "diagnosticErrorContext(traceErr)") || !strings.Contains(branch, "P01B_DIAGNOSTIC_CONTEXT %s %s") || strings.Contains(branch, "p.text") || strings.Contains(branch, "scanner.Text") {
		t.Fatal("failure context is not safely wired to the actual collector")
	}
}
