package composition

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"regexp"
	"strings"
	"unicode/utf8"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
)

const (
	AliasSchemaVersion          = "sqlrs.runtime.v2.aliases.v1"
	ExpansionTraceSchemaVersion = "sqlrs.runtime.v2.alias-expansion-trace.v1"
	MaxAliases                  = runtimev2.MaxTransforms
	MaxSourceDocuments          = 1_024
	MaxSourceIDBytes            = runtimev2.MaxResolvedValueBytes
	MaxCatalogBytes             = 32 << 20
	MaxTraceNodes               = MaxAliases + runtimev2.MaxTransforms
	MaxDiagnosticReferences     = MaxAliases + 1
	MaxPointerBytes             = runtimev2.MaxResolvedValueBytes
	MaxErrorTextBytes           = 16 << 10
	MaxTraceJSONBytes           = 32 << 20
)

var aliasNamePattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,127}$`)

func validAliasName(value string) bool {
	return len(value) <= runtimev2.MaxIdentifierBytes && aliasNamePattern.MatchString(value)
}

func validateSourceID(value string) bool {
	return value != "" && len(value) <= MaxSourceIDBytes && utf8.ValidString(value)
}

func validatePointer(value string) bool {
	return len(value) <= MaxPointerBytes && utf8.ValidString(value)
}

func checkedAdd(left, right, maximum int) (int, bool) {
	if left < 0 || right < 0 || left > maximum-right {
		return 0, false
	}
	return left + right, true
}

func decodeStrictJSON(data []byte, target any, maximum int, sourceID, pointer string) error {
	if target == nil {
		return invalid(CodeInvalidDocument, sourceID, pointer, nil)
	}
	if len(data) > maximum {
		return invalid(CodeExpansionTooLarge, sourceID, pointer, nil)
	}
	if !utf8.Valid(data) {
		return invalid(CodeInvalidDocument, sourceID, pointer, nil)
	}
	if err := rejectDuplicateMembers(data); err != nil {
		return invalid(CodeInvalidDocument, sourceID, pointer, err)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		var validation *runtimev2.ValidationError
		if errors.As(err, &validation) {
			return invalid(CodeInvalidDocument, sourceID, pointer, err)
		}
		return invalid(CodeInvalidDocument, sourceID, pointer, nil)
	}
	if _, err := decoder.Token(); err != io.EOF {
		return invalid(CodeInvalidDocument, sourceID, pointer, nil)
	}
	return nil
}

func rejectDuplicateMembers(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := scanJSONValue(decoder, "$"); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		return errors.New("trailing JSON value")
	}
	return nil
}

func scanJSONValue(decoder *json.Decoder, path string) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := map[string]struct{}{}
		for decoder.More() {
			keyToken, tokenErr := decoder.Token()
			if tokenErr != nil {
				return tokenErr
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("non-string JSON member")
			}
			if _, exists := seen[key]; exists {
				return fmt.Errorf("duplicate JSON member at %s/%s", path, key)
			}
			seen[key] = struct{}{}
			if err := scanJSONValue(decoder, path+"/"+key); err != nil {
				return err
			}
		}
	case '[':
		for index := 0; decoder.More(); index++ {
			if err := scanJSONValue(decoder, fmt.Sprintf("%s/%d", path, index)); err != nil {
				return err
			}
		}
	default:
		return errors.New("invalid JSON delimiter")
	}
	_, err = decoder.Token()
	return err
}

func isJSONNull(raw json.RawMessage) bool {
	return bytes.Equal(bytes.TrimSpace(raw), []byte("null"))
}

func definitionPointer(alias string) string { return "/aliases/" + alias }

func stepPointer(alias string, index int, member string) string {
	return fmt.Sprintf("/aliases/%s/steps/%d/%s", alias, index, member)
}

func parseStepPointer(value, alias, member string) (int, bool) {
	prefix := "/aliases/" + alias + "/steps/"
	suffix := "/" + member
	if !strings.HasPrefix(value, prefix) || !strings.HasSuffix(value, suffix) {
		return 0, false
	}
	raw := strings.TrimSuffix(strings.TrimPrefix(value, prefix), suffix)
	if raw == "" || (len(raw) > 1 && raw[0] == '0') {
		return 0, false
	}
	index := 0
	for _, digit := range raw {
		if digit < '0' || digit > '9' {
			return 0, false
		}
		index = index*10 + int(digit-'0')
		if index > runtimev2.MaxTransforms {
			return 0, false
		}
	}
	return index, true
}
