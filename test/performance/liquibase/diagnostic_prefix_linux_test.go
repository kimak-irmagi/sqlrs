package benchmark

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
)

// PX01: grammar classes are not accepted trace syntax. The values/expected pairs
// are independently authored; arbitrary names and quoted decoys stay private.
func TestDiagnosticPrefixGrammar(t *testing.T) {
	for _, row := range []struct{ raw, prefix, call string }{
		{"", "empty", "none"}, {" \t ", "empty", "none"},
		{"123456789 100.5 execve(\"PRIVATE_SENTINEL\", [], 0x0)", "numeric_header", "none"},
		{"123456789 not-time execve(\"PRIVATE_SENTINEL\", [], 0x0)", "other", "none"},
		{"execve(\"PRIVATE_SENTINEL\", [], 0x0)", "syscall_entry", "execve"},
		{"execveat(AT_FDCWD, \"PRIVATE_SENTINEL\", [], 0x0)", "syscall_entry", "execveat"},
		{"exit(0", "syscall_entry", "exit"}, {"exit_group(0", "syscall_entry", "exit_group"},
		{"restart_syscall(0", "syscall_entry", "restart_syscall"},
		{"PRIVATE_SENTINEL(123456789", "syscall_entry", "other"},
		{"execve_PRIVATE_SENTINEL(0", "syscall_entry", "other"},
		{"syscall_0xabc(123456789", "unknown_syscall", "other"},
		{"???(123456789", "unknown_syscall", "other"},
		{"<... execve resumed>)", "syscall_resume", "execve"},
		{"<... execveat resumed>)", "syscall_resume", "execveat"},
		{"<... exit resumed>)", "syscall_resume", "exit"},
		{"<... exit_group resumed>)", "syscall_resume", "exit_group"},
		{"<... restart_syscall resumed>)", "syscall_resume", "restart_syscall"},
		{"<... PRIVATE_SENTINEL resumed>)", "syscall_resume", "other"},
		{"<... execve resumed_PRIVATE_SENTINEL>)", "other", "none"},
		{"<pid changed to 123456789 ...>", "pid_change", "none"},
		{"<pid changed to PRIVATE_SENTINEL ...>", "other", "none"},
		{"+++ exited with 0 +++", "exit_notice", "none"},
		{"+++ killed by SIGTERM +++", "exit_notice", "none"},
		{"--- SIGCHLD {PRIVATE_SENTINEL} ---", "signal_notice", "none"},
		{"[ Process 123456789 attached ]", "bracket_notice", "none"},
		{"[pid 123456789]", "bracket_notice", "none"},
		{"[PRIVATE_SENTINEL]", "other", "none"},
		{`"execve(PRIVATE_SENTINEL"`, "other", "none"},
		{"PRIVATE_SENTINEL <... execve resumed>", "other", "none"},
		{`futex("exit_group(PRIVATE_SENTINEL", 0)`, "syscall_entry", "other"},
	} {
		got := classifyDiagnosticPrefix(row.raw)
		data, err := json.Marshal(got)
		if got.Prefix != row.prefix || got.KnownCall != row.call || err != nil || strings.Contains(string(data), "PRIVATE_SENTINEL") || strings.Contains(string(data), "123456789") {
			t.Fatal("prefix classification differs or exposes raw input")
		}
	}
}

// PX02: failed real decoding retains its original checkpoint, not a synthetic
// successful interval. Prefix detail is independent of old context shape.
func TestDiagnosticPrefixActualErrors(t *testing.T) {
	for _, row := range []struct{ body, prefix, call string }{
		{"", "empty", "none"},
		{"syscall_0xabc(123456789", "unknown_syscall", "other"},
		{"123456789 100.05 execve(\"PRIVATE_SENTINEL\", [], 0x0)", "numeric_header", "none"},
		{"exit_group(123456789", "syscall_entry", "exit_group"},
	} {
		raw := "2 100.1 " + row.body + " <unfinished ...>\n2 100.2 +++ exited with 0 +++\n"
		rows, err := decodeMeasuredDiagnosticTrace(strings.NewReader(raw), 1)
		old := diagnosticErrorContext(err)
		if err == nil || rows != nil || diagnosticDecodeCode(err) != 8 || old.Location != "pending_entry" || old.Shape != "other" {
			t.Fatal("original rejection/context changed")
		}
		for _, candidate := range []error{err, fmt.Errorf("outer: %w", err)} {
			prefix := diagnosticErrorPrefix(candidate)
			data, marshalErr := json.Marshal(prefix)
			if prefix.Prefix != row.prefix || prefix.KnownCall != row.call || diagnosticDecodeCode(candidate) != 8 || marshalErr != nil || strings.Contains(string(data), "PRIVATE_SENTINEL") || strings.Contains(string(data), "123456789") {
				t.Fatal("real prefix context missing or private")
			}
		}
	}
	if _, err := decodeMeasuredDiagnosticTrace(strings.NewReader("2 100.1 PRIVATE_SENTINEL\n"), 1); diagnosticDecodeCode(err) != 8 || diagnosticErrorPrefix(err).Prefix != "other" || diagnosticErrorContext(err).Location != "record" {
		t.Fatal("ordinary record prefix path missing")
	}
	for _, err := range []error{
		nil, errors.New("PRIVATE_SENTINEL"), diagnosticDecodeError{8},
		diagnosticContextError{cause: diagnosticDecodeError{8}, shape: "exec_entry", prefix: diagnosticPrefixContext{"syscall_entry", "execve"}},
		diagnosticContextError{cause: diagnosticDecodeError{8}, shape: "other"},
		diagnosticContextError{cause: diagnosticDecodeError{8}, shape: "other", prefix: diagnosticPrefixContext{"PRIVATE_SENTINEL", "PRIVATE_SENTINEL"}},
		diagnosticContextError{cause: diagnosticDecodeError{8}, shape: "other", prefix: diagnosticPrefixContext{"empty", "execve"}},
		diagnosticContextError{cause: diagnosticDecodeError{8}, shape: "other", prefix: diagnosticPrefixContext{"syscall_entry", "PRIVATE_SENTINEL"}},
	} {
		if got := diagnosticErrorPrefix(err); got.Prefix != "none" || got.KnownCall != "none" {
			t.Fatal("missing/foreign prefix is not unavailable")
		}
	}
}

// PX04 preserves the old formatter/helper call. Only the extra marker/helper
// appears inside an other-shaped failure; native transport is proven in PX05.
func TestDiagnosticPrefixWiring(t *testing.T) {
	data, err := os.ReadFile("collector_linux_test.go")
	if err != nil {
		t.Fatal(err)
	}
	source := string(data)
	start := strings.Index(source, `if context.Shape == "other" {`)
	if start < 0 {
		t.Fatal("prefix export not guarded by other shape")
	}
	end := strings.Index(source[start:], "\n\t\t\t\t}")
	if end < 0 {
		t.Fatal("prefix guard boundary missing")
	}
	branch := source[start : start+end]
	if !strings.Contains(branch, "diagnosticErrorPrefix(traceErr)") || !strings.Contains(branch, "P01B_DIAGNOSTIC_PREFIX %s %s") || strings.Contains(branch, "scanner.Text") || strings.Contains(branch, "p.text") {
		t.Fatal("raw body reaches prefix publication or wiring missing")
	}
}
