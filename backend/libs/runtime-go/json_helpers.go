package runtimev2

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"strings"
	"unicode/utf8"
)

// DecodeJSON validates one complete Runtime v2 JSON value and preserves the
// package's structured error contract even for syntax and trailing-token errors.
// Callers at persistence and remote trust boundaries should prefer this function
// to invoking encoding/json directly.
func DecodeJSON(data []byte, target json.Unmarshaler) error {
	if target == nil {
		return invalid(CodeInvalidShape, "$")
	}
	return target.UnmarshalJSON(data)
}

func decodeStrict(data []byte, target any) error {
	if len(data) > MaxJSONBytes {
		return invalid(CodeTooLarge, "$")
	}
	if !utf8.Valid(data) {
		return invalid(CodeInvalidValue, "$")
	}
	if err := rejectDuplicateMembers(data); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return normalizeJSONError(err)
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return err
	}
	return nil
}

// decodeCanonicalStrict applies the canonical-v1 public error taxonomy without
// changing the frozen Runtime v2 semantic-core decoder contract.
func decodeCanonicalStrict(data []byte, target any) error {
	if len(data) > MaxCanonicalEnvelopeBytes {
		return canonicalInvalid(CodeDocumentTooLarge, "")
	}
	if !utf8.Valid(data) {
		return canonicalInvalid(CodeSyntaxInvalid, "")
	}
	if err := rejectCanonicalDuplicateMembers(data); err != nil {
		return err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return normalizeCanonicalJSONError(err)
	}
	if _, err := decoder.Token(); err != io.EOF {
		return canonicalInvalid(CodeSyntaxInvalid, "")
	}
	return nil
}

func normalizeCanonicalJSONError(err error) error {
	var validation *ValidationError
	if errors.As(err, &validation) {
		return err
	}
	const unknownPrefix = "json: unknown field "
	if strings.HasPrefix(err.Error(), unknownPrefix) {
		return canonicalInvalid(CodeUnknownMember, strings.Trim(err.Error()[len(unknownPrefix):], `"`))
	}
	var typeError *json.UnmarshalTypeError
	if errors.As(err, &typeError) {
		path := typeError.Field
		if path == "" {
			path = ""
		}
		return canonicalInvalid(CodeShapeInvalid, path)
	}
	return canonicalInvalid(CodeSyntaxInvalid, "")
}

func rejectCanonicalDuplicateMembers(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := scanCanonicalJSONValue(decoder, "$"); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return canonicalInvalid(CodeSyntaxInvalid, "")
	}
	return nil
}

func scanCanonicalJSONValue(decoder *json.Decoder, path string) error {
	token, err := decoder.Token()
	if err != nil {
		return canonicalInvalid(CodeSyntaxInvalid, documentPath(path))
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]struct{}{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return canonicalInvalid(CodeSyntaxInvalid, documentPath(path))
			}
			key, ok := keyToken.(string)
			if !ok {
				return canonicalInvalid(CodeSyntaxInvalid, documentPath(path))
			}
			memberPath := key
			if path != "$" {
				memberPath = path + "." + key
			}
			if _, exists := seen[key]; exists {
				return canonicalInvalid(CodeDuplicateMember, memberPath)
			}
			seen[key] = struct{}{}
			if err := scanCanonicalJSONValue(decoder, memberPath); err != nil {
				return err
			}
		}
	case '[':
		index := 0
		for decoder.More() {
			if err := scanCanonicalJSONValue(decoder, fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
			index++
		}
	default:
		return canonicalInvalid(CodeSyntaxInvalid, documentPath(path))
	}
	if _, err := decoder.Token(); err != nil {
		return canonicalInvalid(CodeSyntaxInvalid, documentPath(path))
	}
	return nil
}

func documentPath(path string) string {
	if path == "$" {
		return ""
	}
	return path
}

func ensureJSONEOF(decoder *json.Decoder) error {
	if _, err := decoder.Token(); err == io.EOF {
		return nil
	}
	return invalid(CodeInvalidShape, "$")
}

func normalizeJSONError(err error) error {
	var validation *ValidationError
	if errors.As(err, &validation) {
		return err
	}
	var typeError *json.UnmarshalTypeError
	if errors.As(err, &typeError) {
		path := typeError.Field
		if path == "" {
			path = "$"
		}
		return invalid(CodeInvalidShape, path)
	}
	const unknownPrefix = "json: unknown field "
	if strings.HasPrefix(err.Error(), unknownPrefix) {
		return invalid(CodeInvalidShape, strings.Trim(err.Error()[len(unknownPrefix):], `"`))
	}
	return invalid(CodeInvalidShape, "$")
}

func rejectDuplicateMembers(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := scanJSONValue(decoder, "$"); err != nil {
		return err
	}
	if err := ensureJSONEOF(decoder); err != nil {
		return err
	}
	return nil
}

func scanJSONValue(decoder *json.Decoder, path string) error {
	token, err := decoder.Token()
	if err != nil {
		return invalid(CodeInvalidShape, path)
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]struct{}{}
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return invalid(CodeInvalidShape, path)
			}
			key, ok := keyToken.(string)
			if !ok {
				return invalid(CodeInvalidShape, path)
			}
			memberPath := key
			if path != "$" {
				memberPath = path + "." + key
			}
			if _, exists := seen[key]; exists {
				return invalid(CodeInvalidShape, memberPath)
			}
			seen[key] = struct{}{}
			if err := scanJSONValue(decoder, memberPath); err != nil {
				return err
			}
		}
	case '[':
		index := 0
		for decoder.More() {
			if err := scanJSONValue(decoder, fmt.Sprintf("%s[%d]", path, index)); err != nil {
				return err
			}
			index++
		}
	default:
		return invalid(CodeInvalidShape, path)
	}
	if _, err := decoder.Token(); err != nil {
		return invalid(CodeInvalidShape, path)
	}
	return nil
}

func isJSONNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}
