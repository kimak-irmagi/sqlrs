package runtimev2

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"unicode/utf8"
)

// The v0.5.0 wire format is specified in
// docs/architecture/runtime-v2-canonical-resolver-structure.md.
type canonicalExtensionJSON struct {
	SchemaVersion  string               `json:"schema_version"`
	Provider       string               `json:"provider"`
	Kind           string               `json:"kind"`
	IdentitySchema string               `json:"identity_schema"`
	Fields         []canonicalFieldJSON `json:"fields"`
	CanonicalBytes string               `json:"canonical_bytes"`
	Fingerprint    string               `json:"fingerprint"`
}

type canonicalFieldJSON struct {
	Name            string               `json:"name"`
	Kind            IdentityFieldKind    `json:"kind"`
	Disclosure      DisclosureClass      `json:"disclosure"`
	Commitment      CanonicalFingerprint `json:"commitment"`
	Text            *string              `json:"text,omitempty"`
	CanonicalValue  *string              `json:"canonical_value,omitempty"`
	SecretReference *secretReferenceJSON `json:"secret_reference,omitempty"`
}

type secretReferenceJSON struct {
	Provider   string `json:"provider"`
	Identifier string `json:"identifier"`
	Version    string `json:"version"`
}

// MarshalCanonicalExtensionIdentityJSON encodes a schema-checked identity.
// It cannot turn an opaque identity into an unvalidated cache value.
func MarshalCanonicalExtensionIdentityJSON(identity CanonicalResolvedExtensionIdentity, schema ExtensionIdentitySchema) ([]byte, error) {
	if !schema.Valid() || identity.data == nil || identity.Provider() != schema.Provider() || identity.Kind() != schema.SemanticKind() || identity.IdentitySchema() != schema.IdentitySchema() {
		return nil, canonicalInvalid(CodeShapeInvalid, "identity")
	}
	fields := identity.Fields()
	if len(fields) > MaxCanonicalValueNodes {
		return nil, canonicalInvalid(CodeLimitExceeded, "fields")
	}
	wire := canonicalExtensionJSON{SchemaVersion: CanonicalSchemaVersion, Provider: identity.Provider(), Kind: identity.Kind(), IdentitySchema: identity.IdentitySchema(), Fields: make([]canonicalFieldJSON, len(fields))}
	for index, item := range fields {
		field := item.Field
		wireField := canonicalFieldJSON{Name: field.Name(), Kind: field.Kind(), Disclosure: item.Disclosure, Commitment: field.Commitment()}
		switch field.Kind() {
		case IdentityFieldText:
			value := field.Text()
			wireField.Text = &value
		case IdentityFieldCanonicalValue:
			value := base64.StdEncoding.EncodeToString(field.CanonicalValue().CanonicalBytes())
			wireField.CanonicalValue = &value
		case IdentityFieldSecretReference:
			reference := field.SecretReference()
			wireField.SecretReference = &secretReferenceJSON{Provider: reference.Provider(), Identifier: reference.Identifier(), Version: reference.Version()}
		default:
			return nil, canonicalInvalid(CodeShapeInvalid, "fields")
		}
		wire.Fields[index] = wireField
	}
	preimage, err := CanonicalResolvedExtensionBytes(identity)
	if err != nil {
		return nil, err
	}
	fingerprint, err := CanonicalExtensionFingerprint(identity)
	if err != nil {
		return nil, err
	}
	wire.CanonicalBytes = base64.StdEncoding.EncodeToString(preimage)
	wire.Fingerprint = string(fingerprint)
	raw, err := json.Marshal(wire)
	if err != nil {
		return nil, err
	}
	if _, err := DecodeCanonicalExtensionIdentityJSON(raw, schema); err != nil {
		return nil, err
	}
	return raw, nil
}

// DecodeCanonicalExtensionIdentityJSON validates one complete typed identity
// against the selected provider schema and all redundant integrity values.
func DecodeCanonicalExtensionIdentityJSON(raw []byte, schema ExtensionIdentitySchema) (CanonicalResolvedExtensionIdentity, error) {
	if !schema.Valid() {
		return CanonicalResolvedExtensionIdentity{}, canonicalInvalid(CodeShapeInvalid, "schema")
	}
	if len(raw) > MaxCanonicalEnvelopeBytes {
		return CanonicalResolvedExtensionIdentity{}, canonicalInvalid(CodeDocumentTooLarge, "")
	}
	if err := validateCanonicalJSONUnicode(raw); err != nil {
		return CanonicalResolvedExtensionIdentity{}, err
	}
	if err := validateCanonicalIdentityMembers(raw); err != nil {
		return CanonicalResolvedExtensionIdentity{}, err
	}
	var wire canonicalExtensionJSON
	if err := decodeCanonicalStrict(raw, &wire); err != nil {
		return CanonicalResolvedExtensionIdentity{}, err
	}
	if wire.SchemaVersion != CanonicalSchemaVersion || wire.Provider != schema.Provider() || wire.Kind != schema.SemanticKind() || wire.IdentitySchema != schema.IdentitySchema() || wire.Fields == nil || len(wire.Fields) > MaxCanonicalValueNodes {
		return CanonicalResolvedExtensionIdentity{}, canonicalInvalid(CodeShapeInvalid, "identity")
	}
	builder, err := NewExtensionIdentityBuilder(schema)
	if err != nil {
		return CanonicalResolvedExtensionIdentity{}, err
	}
	definitions := make(map[string]CanonicalFieldDefinition)
	for _, definition := range schema.Fields() {
		definitions[definition.Name()] = definition
	}
	previous := ""
	for index, item := range wire.Fields {
		if index > 0 && bytes.Compare([]byte(previous), []byte(item.Name)) >= 0 {
			return CanonicalResolvedExtensionIdentity{}, canonicalInvalid(CodeNonCanonical, "fields")
		}
		previous = item.Name
		definition, exists := definitions[item.Name]
		if !exists || definition.Disclosure() != item.Disclosure || definition.Kind() != string(item.Kind) {
			return CanonicalResolvedExtensionIdentity{}, canonicalInvalid(CodeValueInvalid, "fields")
		}
		payloadCount := 0
		if item.Text != nil {
			payloadCount++
		}
		if item.CanonicalValue != nil {
			payloadCount++
		}
		if item.SecretReference != nil {
			payloadCount++
		}
		if payloadCount != 1 {
			return CanonicalResolvedExtensionIdentity{}, canonicalInvalid(CodeShapeInvalid, "fields")
		}
		var field IdentityField
		switch item.Kind {
		case IdentityFieldText:
			if item.Text == nil {
				return CanonicalResolvedExtensionIdentity{}, canonicalInvalid(CodeShapeInvalid, "fields.text")
			}
			field, err = NewTextIdentityField(item.Name, *item.Text)
		case IdentityFieldCanonicalValue:
			if item.CanonicalValue == nil {
				return CanonicalResolvedExtensionIdentity{}, canonicalInvalid(CodeShapeInvalid, "fields.canonical_value")
			}
			var encoded []byte
			encoded, err = base64.StdEncoding.DecodeString(*item.CanonicalValue)
			if err == nil && base64.StdEncoding.EncodeToString(encoded) != *item.CanonicalValue {
				err = canonicalInvalid(CodeNonCanonical, "fields.canonical_value")
			}
			if err == nil {
				var value CanonicalValue
				value, err = ParseCanonicalValueEnvelope(encoded)
				if err == nil {
					field, err = NewCanonicalValueIdentityField(item.Name, value)
				}
			}
		case IdentityFieldSecretReference:
			if item.SecretReference == nil {
				return CanonicalResolvedExtensionIdentity{}, canonicalInvalid(CodeShapeInvalid, "fields.secret_reference")
			}
			var reference SecretReference
			reference, err = NewSecretReference(item.SecretReference.Provider, item.SecretReference.Identifier, item.SecretReference.Version)
			if err == nil {
				field, err = NewSecretReferenceIdentityField(item.Name, reference)
			}
		default:
			return CanonicalResolvedExtensionIdentity{}, canonicalInvalid(CodeShapeInvalid, "fields.kind")
		}
		if err != nil || field.Commitment() != item.Commitment {
			return CanonicalResolvedExtensionIdentity{}, canonicalInvalid(CodeValueInvalid, "fields.commitment")
		}
		if err := builder.AddIdentityField(field); err != nil {
			return CanonicalResolvedExtensionIdentity{}, err
		}
	}
	composition, err := builder.Build()
	if err != nil {
		return CanonicalResolvedExtensionIdentity{}, err
	}
	identity := composition.Identity()
	preimage, err := CanonicalResolvedExtensionBytes(identity)
	if err != nil {
		return CanonicalResolvedExtensionIdentity{}, err
	}
	encoded, err := base64.StdEncoding.DecodeString(wire.CanonicalBytes)
	if err != nil || base64.StdEncoding.EncodeToString(encoded) != wire.CanonicalBytes || !bytes.Equal(preimage, encoded) {
		return CanonicalResolvedExtensionIdentity{}, canonicalInvalid(CodeValueInvalid, "canonical_bytes")
	}
	fingerprint, err := CanonicalExtensionFingerprint(identity)
	if err != nil || string(fingerprint) != wire.Fingerprint {
		return CanonicalResolvedExtensionIdentity{}, canonicalInvalid(CodeValueInvalid, "fingerprint")
	}
	return identity, nil
}

// validateCanonicalIdentityMembers enforces exact member spelling: Go's
// DisallowUnknownFields otherwise accepts case-insensitive struct names.
func validateCanonicalIdentityMembers(raw []byte) error {
	root, err := canonicalObjectMembers(raw, "schema_version", "provider", "kind", "identity_schema", "fields", "canonical_bytes", "fingerprint")
	if err != nil {
		return err
	}
	var fields []json.RawMessage
	if err := json.Unmarshal(root["fields"], &fields); err != nil {
		return canonicalInvalid(CodeShapeInvalid, "fields")
	}
	if len(fields) > MaxCanonicalValueNodes {
		return canonicalInvalid(CodeLimitExceeded, "fields")
	}
	for _, rawField := range fields {
		field, err := canonicalObjectMembers(rawField, "name", "kind", "disclosure", "commitment", "text", "canonical_value", "secret_reference")
		if err != nil {
			return err
		}
		if reference, exists := field["secret_reference"]; exists {
			if _, err := canonicalObjectMembers(reference, "provider", "identifier", "version"); err != nil {
				return err
			}
		}
	}
	return nil
}

func canonicalObjectMembers(raw []byte, allowed ...string) (map[string]json.RawMessage, error) {
	var members map[string]json.RawMessage
	if err := json.Unmarshal(raw, &members); err != nil || members == nil {
		return nil, canonicalInvalid(CodeShapeInvalid, "")
	}
	permitted := make(map[string]bool, len(allowed))
	for _, name := range allowed {
		permitted[name] = true
	}
	for name := range members {
		if !permitted[name] {
			return nil, canonicalInvalid(CodeUnknownMember, name)
		}
	}
	return members, nil
}

// validateCanonicalJSONUnicode rejects Unicode repairs performed by encoding/json.
func validateCanonicalJSONUnicode(raw []byte) error {
	if !utf8.Valid(raw) {
		return canonicalInvalid(CodeSyntaxInvalid, "")
	}
	inside, escaped := false, false
	depth := 0
	for index := 0; index < len(raw); index++ {
		ch := raw[index]
		if !inside {
			switch ch {
			case '"':
				inside = true
			case '{', '[':
				depth++
				if depth > 64 {
					return canonicalInvalid(CodeLimitExceeded, "")
				}
			case '}', ']':
				depth--
			}
			continue
		}
		if escaped {
			escaped = false
			if ch != 'u' {
				continue
			}
			if index+4 >= len(raw) {
				return canonicalInvalid(CodeSyntaxInvalid, "")
			}
			first, ok := parseJSONHex4(raw[index+1 : index+5])
			if !ok {
				return canonicalInvalid(CodeSyntaxInvalid, "")
			}
			index += 4
			if first >= 0xdc00 && first <= 0xdfff {
				return canonicalInvalid(CodeSyntaxInvalid, "")
			}
			if first >= 0xd800 && first <= 0xdbff {
				if index+6 >= len(raw) || raw[index+1] != '\\' || raw[index+2] != 'u' {
					return canonicalInvalid(CodeSyntaxInvalid, "")
				}
				second, ok := parseJSONHex4(raw[index+3 : index+7])
				if !ok || second < 0xdc00 || second > 0xdfff {
					return canonicalInvalid(CodeSyntaxInvalid, "")
				}
				index += 6
			}
			continue
		}
		if ch == '\\' {
			escaped = true
		} else if ch == '"' {
			inside = false
		}
	}
	return nil
}

func parseJSONHex4(raw []byte) (uint16, bool) {
	var result uint16
	for _, ch := range raw {
		result <<= 4
		switch {
		case ch >= '0' && ch <= '9':
			result |= uint16(ch - '0')
		case ch >= 'a' && ch <= 'f':
			result |= uint16(ch - 'a' + 10)
		case ch >= 'A' && ch <= 'F':
			result |= uint16(ch - 'A' + 10)
		default:
			return 0, false
		}
	}
	return result, true
}
