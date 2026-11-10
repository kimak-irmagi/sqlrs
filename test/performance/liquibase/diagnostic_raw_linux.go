package benchmark

import (
	"math"
	"regexp"
	"strconv"
	"strings"
)

// diagnosticRawCall is private parser state, never report data. UR01–UR03 in
// issue89-performance-diagnostic-tests.md define the bounded Linux/amd64 subset.
// A return clears a pending call, but does not establish an executable identity.
type diagnosticRawCall struct {
	name                string
	args                [6]uint64
	pending, unresolved bool
}

var diagnosticRawRecord = regexp.MustCompile(`^(syscall_0x[0-9a-f]{1,16}|\?\?\?)\(([^()\r\n]*)(?:\)[ \t]*=[ \t]*(.*)|[ \t]+<unfinished \.\.\.>)$`)
var diagnosticRawValue = regexp.MustCompile(`^(?:0|0x[0-9a-f]{1,16})$`)
var diagnosticRawError = regexp.MustCompile(`^-1 [A-Z][A-Z0-9_]{0,31} \([\x20-\x27\x2a-\x7e]{1,256}\)$`)
var diagnosticRawDuration = regexp.MustCompile(`^(.*)[ \t]+<([0-9]+\.[0-9]{1,9})>$`)
var diagnosticRawResume = regexp.MustCompile(`^<\.\.\. (syscall_0x[0-9a-f]{1,16}|\?\?\?) resumed>(.*)$`)

// parseDiagnosticRaw validates a whole record, including all six raw arguments.
// No prefix classifier authorises this grammar. Errors retain no input bytes.
func parseDiagnosticRaw(body string) (diagnosticRawCall, error) {
	name, _, found := strings.Cut(body, "(")
	if !found {
		return diagnosticRawCall{}, rejectDiagnosticRaw(body, "framing", 8)
	}
	if !diagnosticRawName.MatchString(name) {
		return diagnosticRawCall{}, rejectDiagnosticRaw(body, "name", 8)
	}
	m := diagnosticRawRecord.FindStringSubmatch(body)
	if m == nil {
		return diagnosticRawCall{}, rejectDiagnosticRaw(body, "framing", 8)
	}
	call := diagnosticRawCall{name: m[1], pending: strings.HasSuffix(body, "<unfinished ...>")}
	args := strings.Split(m[2], ",")
	if len(args) != len(call.args) {
		return diagnosticRawCall{}, rejectDiagnosticRaw(body, "arity", 8)
	}
	for i, value := range args {
		value = strings.Trim(value, " \t")
		if !diagnosticRawValue.MatchString(value) {
			return diagnosticRawCall{}, rejectDiagnosticRaw(body, "argument", 8)
		}
		// The anchored 1–16 hex-digit grammar proves uint64 conversion cannot
		// overflow. No synthetic unreachable numeric-error branch is needed.
		call.args[i], _ = strconv.ParseUint(value, 0, 64)
	}
	if call.pending {
		return call, nil
	}
	returned := m[3]
	if duration := diagnosticRawDuration.FindStringSubmatch(returned); duration != nil {
		if !validDiagnosticRawDuration(duration[2]) {
			return diagnosticRawCall{}, rejectDiagnosticRaw(body, "duration", 9)
		}
		returned = duration[1]
	}
	if returned == "?" || returned == "? <unavailable>" {
		call.unresolved = true
		return call, nil
	}
	if !diagnosticRawValue.MatchString(returned) && !diagnosticRawError.MatchString(returned) {
		return diagnosticRawCall{}, rejectDiagnosticRaw(body, "result", 9)
	}
	return call, nil
}

// validDiagnosticRawDuration bounds -T formatting independently of process time.
// Its caller has already checked a decimal fraction of 1–9 digits (UR01).
func validDiagnosticRawDuration(value string) bool {
	seconds, fraction, _ := strings.Cut(value, ".")
	s, err := strconv.ParseUint(seconds, 10, 64)
	if err != nil || s > math.MaxInt64/1_000_000_000 {
		return false
	}
	ns, _ := strconv.ParseUint(fraction+strings.Repeat("0", 9-len(fraction)), 10, 64)
	return s < math.MaxInt64/1_000_000_000 || ns <= math.MaxInt64%1_000_000_000
}
