package runtimev2

import (
	"encoding/json"
	"sort"
	"strings"
)

const extensionDomain = "sqlrs.runtime.v2/resolved-extension"

// ResolvedExtensionIdentityInput supplies provider-owned immutable identity fields.
type ResolvedExtensionIdentityInput struct {
	SchemaVersion  string
	Owner          string
	Kind           string
	IdentitySchema string
	Fields         []ResolvedField
}

type resolvedExtensionData struct {
	schemaVersion  string
	owner          string
	kind           string
	identitySchema string
	fields         []ResolvedField
}

// ResolvedExtensionIdentity is a provider-qualified resource identity. It is
// deliberately not a factory or transform identity.
type ResolvedExtensionIdentity struct{ data *resolvedExtensionData }

// NewResolvedExtensionIdentity validates and copies an extension identity.
func NewResolvedExtensionIdentity(input ResolvedExtensionIdentityInput) (ResolvedExtensionIdentity, error) {
	if input.SchemaVersion != SchemaVersion {
		return ResolvedExtensionIdentity{}, invalid(CodeInvalidVersion, "schema_version")
	}
	if err := validateIdentifier(input.Owner, "owner"); err != nil {
		return ResolvedExtensionIdentity{}, err
	}
	if err := validateIdentifier(input.Kind, "kind"); err != nil {
		return ResolvedExtensionIdentity{}, err
	}
	if err := validateIdentifier(input.IdentitySchema, "identity_schema"); err != nil {
		return ResolvedExtensionIdentity{}, err
	}
	if len(input.Fields) > MaxResolvedFields {
		return ResolvedExtensionIdentity{}, invalid(CodeTooLarge, "fields")
	}
	// Preserve an explicitly empty collection as [] rather than nil. The wire
	// contract requires fields to be present and array-shaped, so values emitted
	// by MarshalJSON must remain valid input to UnmarshalJSON.
	fields := append([]ResolvedField{}, input.Fields...)
	sort.Slice(fields, func(i, j int) bool { return fields[i].Name < fields[j].Name })
	for index, field := range fields {
		path := "fields[" + itoa(index) + "]"
		if err := validateIdentifier(field.Name, path+".name"); err != nil {
			return ResolvedExtensionIdentity{}, err
		}
		if err := validateUTF8(field.Value, path+".value", false, MaxResolvedValueBytes); err != nil {
			return ResolvedExtensionIdentity{}, err
		}
		if index > 0 && fields[index-1].Name == field.Name {
			return ResolvedExtensionIdentity{}, invalid(CodeDuplicateField, path+".name")
		}
	}
	return ResolvedExtensionIdentity{data: &resolvedExtensionData{input.SchemaVersion, input.Owner, input.Kind, input.IdentitySchema, fields}}, nil
}

func (i ResolvedExtensionIdentity) valid() bool { return i.data != nil }
func (i ResolvedExtensionIdentity) SchemaVersion() string {
	if i.data == nil {
		return ""
	}
	return i.data.schemaVersion
}
func (i ResolvedExtensionIdentity) Owner() string {
	if i.data == nil {
		return ""
	}
	return i.data.owner
}
func (i ResolvedExtensionIdentity) Kind() string {
	if i.data == nil {
		return ""
	}
	return i.data.kind
}
func (i ResolvedExtensionIdentity) IdentitySchema() string {
	if i.data == nil {
		return ""
	}
	return i.data.identitySchema
}
func (i ResolvedExtensionIdentity) Fields() []ResolvedField {
	if i.data == nil {
		return nil
	}
	return append([]ResolvedField{}, i.data.fields...)
}

// ExtensionFingerprint hashes every identity-bearing extension field using its
// dedicated canonical domain. Requirements: runtime-v2-declaration-structure.md.
func ExtensionFingerprint(identity ResolvedExtensionIdentity) (Fingerprint, error) {
	if !identity.valid() {
		return "", invalid(CodeInvalidShape, "identity")
	}
	canonical := encodeRecord(extensionDomain, []canonicalField{
		{tag: 1, payload: []byte(identity.data.schemaVersion)},
		{tag: 2, payload: []byte(identity.data.owner)},
		{tag: 3, payload: []byte(identity.data.kind)},
		{tag: 4, payload: []byte(identity.data.identitySchema)},
		{tag: 5, payload: encodeFieldSet(identity.data.fields)},
	})
	return Fingerprint(hashBytes(canonical)), nil
}

// ExtensionBinding gives one resolved extension a role-specific adapter name.
type ExtensionBinding struct {
	Name     string
	Identity ResolvedExtensionIdentity
}

// ComposeResolvedFields binds every supplied extension into the reserved
// extension.* namespace and returns a canonical defensive copy.
func ComposeResolvedFields(base []ResolvedField, bindings []ExtensionBinding) ([]ResolvedField, error) {
	result := append([]ResolvedField(nil), base...)
	seen := map[string]struct{}{}
	for index, field := range result {
		if strings.HasPrefix(field.Name, "extension.") {
			return nil, invalid(CodeInvalidValue, "base["+itoa(index)+"].name")
		}
		if err := validateIdentifier(field.Name, "base["+itoa(index)+"].name"); err != nil {
			return nil, err
		}
		if err := validateUTF8(field.Value, "base["+itoa(index)+"].value", false, MaxResolvedValueBytes); err != nil {
			return nil, err
		}
		if _, ok := seen[field.Name]; ok {
			return nil, invalid(CodeDuplicateField, "base["+itoa(index)+"].name")
		}
		seen[field.Name] = struct{}{}
	}
	for index, binding := range bindings {
		if err := validateIdentifier(binding.Name, "bindings["+itoa(index)+"].name"); err != nil {
			return nil, err
		}
		name := "extension." + binding.Name
		if _, ok := seen[name]; ok {
			return nil, invalid(CodeDuplicateField, "bindings["+itoa(index)+"].name")
		}
		fingerprint, err := ExtensionFingerprint(binding.Identity)
		if err != nil {
			return nil, prefixError(err, "bindings["+itoa(index)+"]")
		}
		seen[name] = struct{}{}
		result = append(result, ResolvedField{Name: name, Value: string(fingerprint)})
	}
	if len(result) > MaxResolvedFields {
		return nil, invalid(CodeTooLarge, "fields")
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Name < result[j].Name })
	return result, nil
}

type resolvedExtensionWire struct {
	SchemaVersion  *string          `json:"schema_version"`
	Owner          *string          `json:"owner"`
	Kind           *string          `json:"kind"`
	IdentitySchema *string          `json:"identity_schema"`
	Fields         *[]ResolvedField `json:"fields"`
}

func (i ResolvedExtensionIdentity) MarshalJSON() ([]byte, error) {
	if !i.valid() {
		return nil, invalid(CodeInvalidShape, "$")
	}
	fields := i.Fields()
	return json.Marshal(resolvedExtensionWire{&i.data.schemaVersion, &i.data.owner, &i.data.kind, &i.data.identitySchema, &fields})
}
func (i *ResolvedExtensionIdentity) UnmarshalJSON(raw []byte) error {
	if i == nil {
		return invalid(CodeInvalidShape, "$")
	}
	var wire resolvedExtensionWire
	if err := decodeStrict(raw, &wire); err != nil {
		return err
	}
	if wire.SchemaVersion == nil {
		return invalid(CodeInvalidShape, "schema_version")
	}
	if wire.Owner == nil {
		return invalid(CodeInvalidShape, "owner")
	}
	if wire.Kind == nil {
		return invalid(CodeInvalidShape, "kind")
	}
	if wire.IdentitySchema == nil {
		return invalid(CodeInvalidShape, "identity_schema")
	}
	if wire.Fields == nil {
		return invalid(CodeInvalidShape, "fields")
	}
	value, err := NewResolvedExtensionIdentity(ResolvedExtensionIdentityInput{*wire.SchemaVersion, *wire.Owner, *wire.Kind, *wire.IdentitySchema, *wire.Fields})
	if err != nil {
		return err
	}
	i.data = value.data
	return nil
}
