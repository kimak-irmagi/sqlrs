package benchmark

import (
	"errors"
	"regexp"
	"strings"
)

// diagnosticRawFailure contains only closed observation labels. RD01–RD06 in
// issue89-performance-diagnostic-tests.md require original-stage retention and
// separate reporting without admitting new syntax or retaining the input body.
type diagnosticRawFailure struct {
	NameFamily, ArgumentCount, ArgumentKind, RejectStage string
}

type diagnosticRawFailureError struct {
	cause  error
	detail diagnosticRawFailure
}

func (diagnosticRawFailureError) Error() string   { return "incomplete diagnostic trace" }
func (e diagnosticRawFailureError) Unwrap() error { return e.cause }

func unavailableDiagnosticRawFailure() diagnosticRawFailure {
	return diagnosticRawFailure{"none", "none", "none", "none"}
}

// normalizeDiagnosticRawFailure rejects an incoherent tuple as a whole (RD03).
// Count/kind describes syntax independently of the real parser rejection stage.
func normalizeDiagnosticRawFailure(value diagnosticRawFailure) diagnosticRawFailure {
	switch value.NameFamily {
	case "numeric", "fallback", "other":
	default:
		return unavailableDiagnosticRawFailure()
	}
	switch value.RejectStage {
	case "name", "framing", "arity", "argument", "result", "duration", "other":
	default:
		return unavailableDiagnosticRawFailure()
	}
	switch value.ArgumentCount {
	case "empty":
		if value.ArgumentKind == "none" {
			return value
		}
	case "short", "six", "extra":
		if value.ArgumentKind == "numeric" || value.ArgumentKind == "other" {
			return value
		}
	case "none":
		if value.ArgumentKind == "none" || value.ArgumentKind == "unavailable" {
			return value
		}
	}
	return unavailableDiagnosticRawFailure()
}

// diagnosticErrorRaw inspects only enum-only state, including wrapped failures.
// Foreign errors are never formatted or copied to a diagnostic marker (RD03).
func diagnosticErrorRaw(err error) diagnosticRawFailure {
	var value diagnosticRawFailureError
	if !errors.As(err, &value) {
		return unavailableDiagnosticRawFailure()
	}
	return normalizeDiagnosticRawFailure(value.detail)
}

var diagnosticRawName = regexp.MustCompile(`^(?:syscall_0x[0-9a-f]{1,16}|\?\?\?)$`)
var diagnosticObservationName = regexp.MustCompile(`^[A-Za-z_?][A-Za-z0-9_?]*$`)

// describeDiagnosticRawFailure classifies only an anchored, unambiguous region.
// It observes a rejected record, not a new accepted grammar. Empty and unavailable
// arguments are deliberately distinct; quoted/nested text is never comma-counted.
func describeDiagnosticRawFailure(body, stage string) diagnosticRawFailure {
	name, rest, found := strings.Cut(body, "(")
	if !found || !diagnosticObservationName.MatchString(name) {
		return unavailableDiagnosticRawFailure()
	}
	value := diagnosticRawFailure{"other", "none", "none", stage}
	if name == "???" {
		value.NameFamily = "fallback"
	} else if diagnosticRawName.MatchString(name) {
		value.NameFamily = "numeric"
	}
	var region string
	if strings.HasSuffix(rest, "<unfinished ...>") {
		region = strings.TrimSuffix(rest, "<unfinished ...>")
	} else if close := strings.IndexByte(rest, ')'); close >= 0 {
		region = rest[:close]
	} else {
		return normalizeDiagnosticRawFailure(value)
	}
	if strings.ContainsAny(region, "\"'()[]{}\r\n") {
		return normalizeDiagnosticRawFailure(value)
	}
	region = strings.Trim(region, " \t")
	switch region {
	case "":
		value.ArgumentCount = "empty"
	case "<unavailable>":
		value.ArgumentKind = "unavailable"
	default:
		args := strings.Split(region, ",")
		value.ArgumentCount, value.ArgumentKind = "short", "numeric"
		if len(args) == 6 {
			value.ArgumentCount = "six"
		} else if len(args) > 6 {
			value.ArgumentCount = "extra"
		}
		for _, arg := range args {
			if !diagnosticRawValue.MatchString(strings.Trim(arg, " \t")) {
				value.ArgumentKind = "other"
				break
			}
		}
	}
	return normalizeDiagnosticRawFailure(value)
}

// rejectDiagnosticRaw is called at the actual parser checkpoint, not inferred
// from shape. Only the fixed cause and labels survive that call (RD01/RD02).
func rejectDiagnosticRaw(body, stage string, code int) error {
	return diagnosticRawFailureError{cause: diagnosticDecodeError{code}, detail: describeDiagnosticRawFailure(body, stage)}
}
