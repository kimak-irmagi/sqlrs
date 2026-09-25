package resolver

import (
	"context"
	"errors"
	"fmt"
	"os"
)

type ErrorCode string

const (
	CodeInvalidDeclaration ErrorCode = "invalid_declaration"
	CodeUnsupportedKind    ErrorCode = "unsupported_kind"
	CodeDuplicateKind      ErrorCode = "duplicate_kind"
	CodeNotFound           ErrorCode = "not_found"
	CodeUnsafePath         ErrorCode = "unsafe_path"
	CodeNotRegular         ErrorCode = "not_regular"
	CodeChanged            ErrorCode = "changed_during_resolution"
	CodeInvalidResolution  ErrorCode = "invalid_resolution"
	CodeCorruptCache       ErrorCode = "corrupt_cache"
	CodeIncompatibleCache  ErrorCode = "incompatible_cache"
	CodePermissionDenied   ErrorCode = "permission_denied"
	CodeCancelled          ErrorCode = "cancelled"
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
	case errors.Is(err, ErrIncompatibleCache):
		return CodeIncompatibleCache
	case errors.Is(err, ErrUnsafePath):
		return CodeUnsafePath
	case errors.Is(err, ErrNotRegular):
		return CodeNotRegular
	case errors.Is(err, ErrChanged):
		return CodeChanged
	case errors.Is(err, os.ErrNotExist):
		return CodeNotFound
	case errors.Is(err, os.ErrPermission):
		return CodePermissionDenied
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return CodeCancelled
	default:
		return CodeIO
	}
}
