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
	// Canonical-v1 stable integrity taxonomy. Legacy codes above remain frozen.
	CodeDocumentTooLarge       ValidationCode = "document_too_large"
	CodeSyntaxInvalid          ValidationCode = "syntax_invalid"
	CodeShapeInvalid           ValidationCode = "shape_invalid"
	CodeValueInvalid           ValidationCode = "value_invalid"
	CodeUnknownMember          ValidationCode = "unknown_member"
	CodeDuplicateMember        ValidationCode = "duplicate_member"
	CodeRevisionMismatch       ValidationCode = "revision_mismatch"
	CodeLimitExceeded          ValidationCode = "limit_exceeded"
	CodeNonCanonical           ValidationCode = "non_canonical"
	CodeDescriptorInvalid      ValidationCode = "descriptor_invalid"
	CodeCommitmentMismatch     ValidationCode = "commitment_mismatch"
	CodeDigestMismatch         ValidationCode = "digest_mismatch"
	CodeLineageMismatch        ValidationCode = "lineage_mismatch"
	CodeEndpointMismatch       ValidationCode = "endpoint_mismatch"
	CodeAuthorizationDenied    ValidationCode = "authorization_denied"
	CodePathInvalid            ValidationCode = "path_invalid"
	CodeFileMissing            ValidationCode = "file_missing"
	CodeFileUnlisted           ValidationCode = "file_unlisted"
	CodeSizeMismatch           ValidationCode = "size_mismatch"
	CodeFileDigestMismatch     ValidationCode = "file_digest_mismatch"
	CodeManifestDigestMismatch ValidationCode = "manifest_digest_mismatch"
	CodeBundleMetadataMismatch ValidationCode = "bundle_metadata_mismatch"
	CodeVectorInvalid          ValidationCode = "vector_invalid"
	CodeRelationMismatch       ValidationCode = "relation_mismatch"
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
	if e.Path == "" {
		return fmt.Sprintf("%s: %s", ErrInvalid, e.Code)
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

// canonicalInvalid preserves an empty document-wide path as required by the
// canonical-v1 public error contract.
func canonicalInvalid(code ValidationCode, path string) error {
	return &ValidationError{Code: code, Path: path}
}

func canonicalizeValidationError(err error) error {
	var validation *ValidationError
	if !errors.As(err, &validation) {
		return err
	}
	code := validation.Code
	switch code {
	case CodeInvalidShape:
		code = CodeShapeInvalid
	case CodeInvalidValue, CodeInvalidVersion:
		code = CodeValueInvalid
	case CodeDuplicateField:
		code = CodeNonCanonical
	case CodeTooLarge:
		code = CodeLimitExceeded
	}
	return canonicalInvalid(code, validation.Path)
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
