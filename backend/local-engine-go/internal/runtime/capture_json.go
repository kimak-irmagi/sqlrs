package runtime

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"unicode/utf8"
)

const captureRequiredFields = "protocol job_id task_id attempt_no execution_digest helper_digest child_exit_code stdout_eof stderr_eof stdout_bytes stdout_sha256 stderr_bytes stderr_sha256"

// decodeCaptureObject validates the closed v2 wire schema before decoding into a
// typed manifest. encoding/json alone accepts duplicate keys, null scalar values
// and absent zero-valued fields; those are not completion evidence. The nullable
// field is required too, but may explicitly be null. No payload enters an error.
// Requirements: liquibase-private-capture-storage-schema.md section 4.
func decodeCaptureObject(data []byte, target any, required, nullable string) bool {
	if !utf8.Valid(data) {
		return false
	}
	allowed := make(map[string]bool)
	for _, field := range strings.Fields(required) {
		allowed[field] = true
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	opening, err := decoder.Token()
	if err != nil {
		return false
	}
	if delimiter, ok := opening.(json.Delim); !ok || delimiter != '{' {
		return false
	}
	seen := make(map[string]bool, len(allowed))
	for decoder.More() {
		key, err := decoder.Token()
		if err != nil {
			return false
		}
		// Inside an object, Token returns a decoded string key or an error.
		// Comparing decoded keys also catches escaped duplicate field names.
		field := key.(string)
		if !allowed[field] || seen[field] {
			return false
		}
		seen[field] = true
		var raw json.RawMessage
		if err := decoder.Decode(&raw); err != nil {
			return false
		}
		if field != nullable && bytes.Equal(bytes.TrimSpace(raw), []byte("null")) {
			return false
		}
	}
	if _, err := decoder.Token(); err != nil {
		return false
	}
	var trailing any
	if decoder.Decode(&trailing) != io.EOF || len(seen) != len(allowed) {
		return false
	}
	return json.Unmarshal(data, target) == nil
}
