// Package composition expands versioned Runtime v2 alias documents into
// engine-neutral declarations. Requirements:
// docs/architecture/runtime-v2-composition-structure.md.
package composition

import (
	"errors"
	"fmt"
)

// ErrorCode identifies a stable composition failure class.
type ErrorCode string

const (
	CodeInvalidDocument    ErrorCode = "invalid_document"
	CodeMissingReference   ErrorCode = "missing_reference"
	CodeAmbiguousReference ErrorCode = "ambiguous_reference"
	CodeWrongReferenceKind ErrorCode = "wrong_reference_kind"
	CodeCycle              ErrorCode = "cycle"
	CodeExpansionTooLarge  ErrorCode = "expansion_too_large"
)

// ErrInvalid matches every structured composition failure.
var ErrInvalid = errors.New("runtime v2 composition invalid")

// DiagnosticReference identifies one bounded alias definition in an error
// context. It contains diagnostics only and never declaration data.
type DiagnosticReference struct {
	Alias    string
	SourceID string
	Pointer  string
}

// Error exposes stable composition diagnostics without rejected declaration
// payloads. Cycle and candidate details are available through copy accessors.
type Error struct {
	Code     ErrorCode
	SourceID string
	Pointer  string

	cause      error
	cycle      []DiagnosticReference
	candidates []DiagnosticReference
}

// Error returns bounded primary diagnostics only.
func (e *Error) Error() string {
	if e == nil {
		return ErrInvalid.Error()
	}
	location := e.Pointer
	if e.SourceID != "" {
		if location == "" {
			location = e.SourceID
		} else {
			location = e.SourceID + ":" + location
		}
	}
	if location == "" {
		return fmt.Sprintf("%s: %s", ErrInvalid, e.Code)
	}
	return fmt.Sprintf("%s: %s at %s", ErrInvalid, e.Code, location)
}

// Unwrap preserves both the composition sentinel and an optional nested Runtime
// validation error.
func (e *Error) Unwrap() []error {
	if e == nil || e.cause == nil {
		return []error{ErrInvalid}
	}
	return []error{ErrInvalid, e.cause}
}

// Cycle returns a defensive copy of a closed recipe cycle.
func (e *Error) Cycle() []DiagnosticReference {
	if e == nil || len(e.cycle) == 0 {
		return nil
	}
	return append([]DiagnosticReference(nil), e.cycle...)
}

// Candidates returns a defensive copy of ambiguity candidates.
func (e *Error) Candidates() []DiagnosticReference {
	if e == nil || len(e.candidates) == 0 {
		return nil
	}
	return append([]DiagnosticReference(nil), e.candidates...)
}

func invalid(code ErrorCode, sourceID, pointer string, cause error) error {
	return &Error{Code: code, SourceID: sourceID, Pointer: pointer, cause: cause}
}

func invalidWithCycle(sourceID, pointer string, cycle []DiagnosticReference) error {
	return &Error{Code: CodeCycle, SourceID: sourceID, Pointer: pointer, cycle: append([]DiagnosticReference(nil), cycle...)}
}

func invalidWithCandidates(candidates []DiagnosticReference) error {
	return &Error{Code: CodeAmbiguousReference, candidates: append([]DiagnosticReference(nil), candidates...)}
}
