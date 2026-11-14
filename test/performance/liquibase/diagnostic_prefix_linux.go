package benchmark

import (
	"errors"
	"regexp"
	"strings"
)

// diagnosticPrefixContext contains fixed labels only, not an input excerpt.
// PX01–PX04 and the approved structural-prefix diagnostic amendment define it.
type diagnosticPrefixContext struct{ Prefix, KnownCall string }

var diagnosticNumericHeader = regexp.MustCompile(`^[1-9][0-9]*\s+[0-9]+\.[0-9]+\s+`)
var diagnosticUnknownCall = regexp.MustCompile(`^(?:syscall_0x[0-9a-fA-F]+|\?\?\?)\(`)
var diagnosticCallEntry = regexp.MustCompile(`^([A-Za-z_][A-Za-z0-9_]*)\(`)
var diagnosticCallResume = regexp.MustCompile(`^<\.\.\. ([A-Za-z_][A-Za-z0-9_]*) resumed>`)
var diagnosticPIDChange = regexp.MustCompile(`^<pid changed to [1-9][0-9]* \.\.\.>`)
var diagnosticSignalNotice = regexp.MustCompile(`^--- SIG[A-Z0-9]+(?: |$)`)
var diagnosticBracketNotice = regexp.MustCompile(`^\[\s*(?:pid\s+[1-9][0-9]*|Process\s+[1-9][0-9]*\s+(?:attached|detached))\s*\]`)

// knownDiagnosticCall never returns an arbitrary identifier. Matching a call
// token is not evidence that the requested syscall/image successfully executed.
func knownDiagnosticCall(name string) string {
	switch name {
	case "execve", "execveat", "exit", "exit_group", "restart_syscall":
		return name
	}
	return "other"
}

// classifyDiagnosticPrefix inspects an anchored prefix only, before arguments.
// Unknown-syscall syntax precedes generic entry to keep overlapping forms stable.
func classifyDiagnosticPrefix(body string) diagnosticPrefixContext {
	body = strings.Trim(body, " \t\r\n")
	switch {
	case body == "":
		return diagnosticPrefixContext{"empty", "none"}
	case diagnosticNumericHeader.MatchString(body):
		return diagnosticPrefixContext{"numeric_header", "none"}
	case diagnosticUnknownCall.MatchString(body):
		return diagnosticPrefixContext{"unknown_syscall", "other"}
	}
	if match := diagnosticCallEntry.FindStringSubmatch(body); match != nil {
		return diagnosticPrefixContext{"syscall_entry", knownDiagnosticCall(match[1])}
	}
	if match := diagnosticCallResume.FindStringSubmatch(body); match != nil {
		return diagnosticPrefixContext{"syscall_resume", knownDiagnosticCall(match[1])}
	}
	switch {
	case diagnosticPIDChange.MatchString(body):
		return diagnosticPrefixContext{"pid_change", "none"}
	case diagnosticExit.MatchString(body):
		return diagnosticPrefixContext{"exit_notice", "none"}
	case diagnosticSignalNotice.MatchString(body):
		return diagnosticPrefixContext{"signal_notice", "none"}
	case diagnosticBracketNotice.MatchString(body):
		return diagnosticPrefixContext{"bracket_notice", "none"}
	}
	return diagnosticPrefixContext{"other", "none"}
}

// newDiagnosticContext retains no raw body in its error. Extra prefix labels
// are computed only for the already-approved other shape; rejection is unchanged.
func newDiagnosticContext(cause error, location, body string) error {
	value := diagnosticContextError{cause: cause, location: location, shape: diagnosticRecordShape(body)}
	if value.shape == "other" {
		value.prefix = classifyDiagnosticPrefix(body)
	}
	return value
}

// diagnosticErrorPrefix validates stored/wrapped metadata independently from
// old context extraction, preserving its exact return shape (PX02/PX04).
func diagnosticErrorPrefix(err error) diagnosticPrefixContext {
	unavailable := diagnosticPrefixContext{"none", "none"}
	var value diagnosticContextError
	if !errors.As(err, &value) || value.shape != "other" {
		return unavailable
	}
	p := value.prefix
	switch p.Prefix {
	case "syscall_entry", "syscall_resume":
		switch p.KnownCall {
		case "execve", "execveat", "exit", "exit_group", "restart_syscall", "other":
			return p
		}
	case "unknown_syscall":
		if p.KnownCall == "other" {
			return p
		}
	case "empty", "numeric_header", "pid_change", "exit_notice", "signal_notice", "bracket_notice", "other", "none":
		if p.KnownCall == "none" {
			return p
		}
	}
	return unavailable
}
