package httpapi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"regexp"
	"strings"

	"github.com/sqlrs/engine-local/internal/prepare"
)

var liquibaseTagPattern = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_.-]{0,127}$`)

// decodePrepareRequest enforces the raw JSON presence rule from
// docs/architecture/liquibase-container-component-structure.md before the
// typed decoder can erase empty, null, case-varied, or escaped fields.
func decodePrepareRequest(body io.Reader) (prepare.Request, error) {
	data, err := io.ReadAll(body)
	if err != nil {
		return prepare.Request{}, err
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	start, err := decoder.Token()
	if err != nil {
		return prepare.Request{}, err
	}
	if start != json.Delim('{') {
		return prepare.Request{}, fmt.Errorf("prepare request must be a JSON object")
	}
	seen := make([]string, 0)
	hasRunnerKey := false
	hasVersionKey := false
	for decoder.More() {
		token, err := decoder.Token()
		if err != nil {
			return prepare.Request{}, err
		}
		key, ok := token.(string)
		if !ok {
			return prepare.Request{}, fmt.Errorf("invalid JSON field name")
		}
		for _, prior := range seen {
			if strings.EqualFold(prior, key) {
				return prepare.Request{}, fmt.Errorf("duplicate JSON field: %s", key)
			}
		}
		seen = append(seen, key)
		if strings.EqualFold(key, "liquibase_exec") || strings.EqualFold(key, "liquibase_exec_mode") || strings.EqualFold(key, "liquibase_env") {
			hasRunnerKey = true
		}
		if strings.EqualFold(key, "liquibase_version") {
			hasVersionKey = true
		}
		var value json.RawMessage
		if err := decoder.Decode(&value); err != nil {
			return prepare.Request{}, err
		}
	}
	if _, err := decoder.Token(); err != nil {
		return prepare.Request{}, err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return prepare.Request{}, fmt.Errorf("extra JSON payload")
		}
		return prepare.Request{}, err
	}
	var req prepare.Request
	strict := json.NewDecoder(bytes.NewReader(data))
	strict.DisallowUnknownFields()
	if err := strict.Decode(&req); err != nil {
		return prepare.Request{}, err
	}
	if hasRunnerKey {
		return prepare.Request{}, fmt.Errorf("client Liquibase runner settings are not allowed")
	}
	if hasVersionKey && (strings.TrimSpace(req.PrepareKind) != "lb" || !liquibaseTagPattern.MatchString(req.LiquibaseVersion)) {
		return prepare.Request{}, fmt.Errorf("invalid liquibase_version selector")
	}
	return req, nil
}
