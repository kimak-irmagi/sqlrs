package runtimev2

import (
	"errors"
	"fmt"
	"regexp"
	"unicode/utf8"
)

// ValidationCode identifies a stable class of rejected semantic input.
type ValidationCode string

const (
	CodeInvalidVersion    ValidationCode = "invalid_version"
	CodeInvalidShape      ValidationCode = "invalid_shape"
	CodeInvalidValue      ValidationCode = "invalid_value"
	CodeDuplicateField    ValidationCode = "duplicate_field"
	CodeTooLarge          ValidationCode = "too_large"
	CodeIntegrityMismatch ValidationCode = "integrity_mismatch"
)

// ErrInvalid matches every validation failure without exposing input values.
var ErrInvalid = errors.New("runtime v2 semantic value invalid")

// ValidationError reports a stable code and field path. It never includes the
// rejected value. Requirements: runtime-v2-semantic-core-structure.md.
type ValidationError struct {
	Code ValidationCode
	Path string
}

func (e *ValidationError) Error() string {
	if e == nil {
		return ErrInvalid.Error()
	}
	return fmt.Sprintf("%s: %s at %s", ErrInvalid, e.Code, e.Path)
}

func (e *ValidationError) Unwrap() error { return ErrInvalid }

func invalid(code ValidationCode, path string) error {
	if path == "" {
		path = "$"
	}
	return &ValidationError{Code: code, Path: path}
}

var identifierPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,127}$`)

func validateIdentifier(value, path string) error {
	if len(value) > MaxIdentifierBytes {
		return invalid(CodeTooLarge, path)
	}
	if !identifierPattern.MatchString(value) {
		return invalid(CodeInvalidValue, path)
	}
	return nil
}

func validateUTF8(value, path string, allowEmpty bool, maximum int) error {
	if !utf8.ValidString(value) || (!allowEmpty && value == "") {
		return invalid(CodeInvalidValue, path)
	}
	if len(value) > maximum {
		return invalid(CodeTooLarge, path)
	}
	return nil
}

func prefixError(err error, prefix string) error {
	var validation *ValidationError
	if !errors.As(err, &validation) {
		return err
	}
	path := validation.Path
	if path == "$" || path == "" {
		path = prefix
	} else if prefix != "" {
		path = prefix + "." + path
	}
	return &ValidationError{Code: validation.Code, Path: path}
}
