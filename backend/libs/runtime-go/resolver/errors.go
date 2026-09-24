package resolver

import (
	"errors"
	"fmt"
)

type ErrorCode string

const (
	CodeInvalidDeclaration ErrorCode = "invalid_declaration"
	CodeUnsupportedKind    ErrorCode = "unsupported_kind"
	CodeDuplicateKind      ErrorCode = "duplicate_kind"
	CodeInvalidResolution  ErrorCode = "invalid_resolution"
	CodeCorruptCache       ErrorCode = "corrupt_cache"
	CodeUnavailable        ErrorCode = "unavailable"
	CodeIO                 ErrorCode = "io"
)

// Error is the stable resolver failure envelope. It retains a prior
// revalidation result without exposing declaration values or file contents.
type Error struct {
	Operation   string
	Code        ErrorCode
	Descriptor  Descriptor
	PriorStatus RevalidationStatus
	Reason      string
	Err         error
}

func (e *Error) Error() string {
	if e == nil {
		return "resolver error"
	}
	return fmt.Sprintf("resolver %s: %s", e.Operation, e.Code)
}
func (e *Error) Unwrap() error {
	if e == nil {
		return nil
	}
	return e.Err
}
func resolverError(operation string, code ErrorCode, descriptor Descriptor, err error) *Error {
	return &Error{Operation: operation, Code: code, Descriptor: descriptor, Err: err}
}
func classifyError(err error) ErrorCode {
	switch {
	case errors.Is(err, ErrInvalidDeclaration):
		return CodeInvalidDeclaration
	case errors.Is(err, ErrUnsupported):
		return CodeUnsupportedKind
	case errors.Is(err, ErrDuplicate):
		return CodeDuplicateKind
	case errors.Is(err, ErrCorruptCache):
		return CodeCorruptCache
	default:
		return CodeIO
	}
}
