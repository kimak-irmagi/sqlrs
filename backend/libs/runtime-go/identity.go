package runtimev2

import (
	"encoding/json"
	"sort"
)

// ResolvedField is one provider-defined immutable identity input. Constructors
// copy it into opaque semantic values.
type ResolvedField struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// FactoryIdentityInput supplies all identity-bearing values for a constructor.
// The constructor copies and canonicalizes Fields.
type FactoryIdentityInput struct {
	SchemaVersion  string
	Provider       string
	Kind           string
	IdentitySchema string
	Fields         []ResolvedField
}

// TransformIdentityInput supplies identity-bearing transform values.
type TransformIdentityInput = FactoryIdentityInput

type identityData struct {
	schemaVersion  string
	provider       string
	kind           string
	identitySchema string
	fields         []ResolvedField
}

// ResolvedFactoryIdentity is an immutable resolved factory/deployment identity.
type ResolvedFactoryIdentity struct{ data *identityData }

// ResolvedTransformIdentity is an immutable resolved transform identity.
type ResolvedTransformIdentity struct{ data *identityData }

// NewFactoryIdentity validates and copies a resolved factory identity.
func NewFactoryIdentity(input FactoryIdentityInput) (ResolvedFactoryIdentity, error) {
	data, err := newIdentity(input)
	if err != nil {
		return ResolvedFactoryIdentity{}, err
	}
	return ResolvedFactoryIdentity{data: data}, nil
}

// NewTransformIdentity validates and copies a resolved transform identity.
func NewTransformIdentity(input TransformIdentityInput) (ResolvedTransformIdentity, error) {
	data, err := newIdentity(input)
	if err != nil {
		return ResolvedTransformIdentity{}, err
	}
	return ResolvedTransformIdentity{data: data}, nil
}

func newIdentity(input FactoryIdentityInput) (*identityData, error) {
	if input.SchemaVersion != SchemaVersion {
		return nil, invalid(CodeInvalidVersion, "schema_version")
	}
	if err := validateIdentifier(input.Provider, "provider"); err != nil {
		return nil, err
	}
	if err := validateIdentifier(input.Kind, "kind"); err != nil {
		return nil, err
	}
	if err := validateIdentifier(input.IdentitySchema, "identity_schema"); err != nil {
		return nil, err
	}
	if len(input.Fields) > MaxResolvedFields {
		return nil, invalid(CodeTooLarge, "fields")
	}
	fields := append([]ResolvedField(nil), input.Fields...)
	sort.Slice(fields, func(i, j int) bool { return fields[i].Name < fields[j].Name })
	for index, field := range fields {
		path := "fields[" + itoa(index) + "]"
		if err := validateIdentifier(field.Name, path+".name"); err != nil {
			return nil, err
		}
		if err := validateUTF8(field.Value, path+".value", false, MaxResolvedValueBytes); err != nil {
			return nil, err
		}
		if index > 0 && fields[index-1].Name == field.Name {
			return nil, invalid(CodeDuplicateField, path+".name")
		}
	}
	return &identityData{schemaVersion: input.SchemaVersion, provider: input.Provider, kind: input.Kind, identitySchema: input.IdentitySchema, fields: fields}, nil
}

func itoa(value int) string {
	if value == 0 {
		return "0"
	}
	var buffer [20]byte
	position := len(buffer)
	for value > 0 {
		position--
		buffer[position] = byte('0' + value%10)
		value /= 10
	}
	return string(buffer[position:])
}

func (i ResolvedFactoryIdentity) valid() bool   { return i.data != nil }
func (i ResolvedTransformIdentity) valid() bool { return i.data != nil }

// SchemaVersion returns the identity contract version, or an empty string for
// the zero value.
func (i ResolvedFactoryIdentity) SchemaVersion() string {
	if i.data == nil {
		return ""
	}
	return i.data.schemaVersion
}

// Provider returns the adapter namespace that resolved this factory identity.
func (i ResolvedFactoryIdentity) Provider() string {
	if i.data == nil {
		return ""
	}
	return i.data.provider
}

// Kind returns the provider-specific factory kind.
func (i ResolvedFactoryIdentity) Kind() string {
	if i.data == nil {
		return ""
	}
	return i.data.kind
}

// IdentitySchema returns the provider-owned schema for the resolved fields.
func (i ResolvedFactoryIdentity) IdentitySchema() string {
	if i.data == nil {
		return ""
	}
	return i.data.identitySchema
}

// SchemaVersion returns the identity contract version, or an empty string for
// the zero value.
func (i ResolvedTransformIdentity) SchemaVersion() string {
	if i.data == nil {
		return ""
	}
	return i.data.schemaVersion
}

// Provider returns the adapter namespace that resolved this transform.
func (i ResolvedTransformIdentity) Provider() string {
	if i.data == nil {
		return ""
	}
	return i.data.provider
}

// Kind returns the provider-specific transform kind.
func (i ResolvedTransformIdentity) Kind() string {
	if i.data == nil {
		return ""
	}
	return i.data.kind
}

// IdentitySchema returns the provider-owned schema for the resolved fields.
func (i ResolvedTransformIdentity) IdentitySchema() string {
	if i.data == nil {
		return ""
	}
	return i.data.identitySchema
}

// Fields returns a defensive copy in canonical name order.
func (i ResolvedFactoryIdentity) Fields() []ResolvedField {
	if i.data == nil {
		return nil
	}
	return append([]ResolvedField(nil), i.data.fields...)
}

// Fields returns a defensive copy in canonical name order.
func (i ResolvedTransformIdentity) Fields() []ResolvedField {
	if i.data == nil {
		return nil
	}
	return append([]ResolvedField(nil), i.data.fields...)
}

type identityWire struct {
	SchemaVersion  *string          `json:"schema_version"`
	Provider       *string          `json:"provider"`
	Kind           *string          `json:"kind"`
	IdentitySchema *string          `json:"identity_schema"`
	Fields         *[]ResolvedField `json:"fields"`
}

func identityToWire(data *identityData) identityWire {
	fields := append([]ResolvedField(nil), data.fields...)
	return identityWire{SchemaVersion: &data.schemaVersion, Provider: &data.provider, Kind: &data.kind, IdentitySchema: &data.identitySchema, Fields: &fields}
}

func identityFromWire(wire identityWire) (*identityData, error) {
	if wire.SchemaVersion == nil {
		return nil, invalid(CodeInvalidShape, "schema_version")
	}
	if wire.Provider == nil {
		return nil, invalid(CodeInvalidShape, "provider")
	}
	if wire.Kind == nil {
		return nil, invalid(CodeInvalidShape, "kind")
	}
	if wire.IdentitySchema == nil {
		return nil, invalid(CodeInvalidShape, "identity_schema")
	}
	if wire.Fields == nil {
		return nil, invalid(CodeInvalidShape, "fields")
	}
	return newIdentity(FactoryIdentityInput{SchemaVersion: *wire.SchemaVersion, Provider: *wire.Provider, Kind: *wire.Kind, IdentitySchema: *wire.IdentitySchema, Fields: *wire.Fields})
}

func (i ResolvedFactoryIdentity) MarshalJSON() ([]byte, error) {
	if !i.valid() {
		return nil, invalid(CodeInvalidShape, "$")
	}
	return json.Marshal(identityToWire(i.data))
}
func (i *ResolvedFactoryIdentity) UnmarshalJSON(data []byte) error {
	if i == nil {
		return invalid(CodeInvalidShape, "$")
	}
	var wire identityWire
	if err := decodeStrict(data, &wire); err != nil {
		return err
	}
	value, err := identityFromWire(wire)
	if err != nil {
		return err
	}
	i.data = value
	return nil
}
func (i ResolvedTransformIdentity) MarshalJSON() ([]byte, error) {
	if !i.valid() {
		return nil, invalid(CodeInvalidShape, "$")
	}
	return json.Marshal(identityToWire(i.data))
}
func (i *ResolvedTransformIdentity) UnmarshalJSON(data []byte) error {
	if i == nil {
		return invalid(CodeInvalidShape, "$")
	}
	var wire identityWire
	if err := decodeStrict(data, &wire); err != nil {
		return err
	}
	value, err := identityFromWire(wire)
	if err != nil {
		return err
	}
	i.data = value
	return nil
}
