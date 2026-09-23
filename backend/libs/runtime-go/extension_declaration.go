package runtimev2

import (
	"encoding/json"
	"sort"
)

// DeclarationField is one provider-owned unresolved specification field.
// Requirements: runtime-v2-declaration-structure.md.
type DeclarationField struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// ExtensionSpecificationInput supplies a complete versioned provider
// specification. Constructors validate and defensively copy Fields.
type ExtensionSpecificationInput struct {
	SchemaVersion       string
	Owner               string
	Kind                string
	SpecificationSchema string
	Fields              []DeclarationField
}

type extensionSpecificationData struct {
	schemaVersion       string
	owner               string
	kind                string
	specificationSchema string
	fields              []DeclarationField
}

// InputDeclaration is an immutable input-resource specification.
type InputDeclaration struct{ data *extensionSpecificationData }

// ExecutionEnvironmentDeclaration is an immutable execution-environment specification.
type ExecutionEnvironmentDeclaration struct{ data *extensionSpecificationData }

// DeploymentDeclaration is an immutable deployment specification.
type DeploymentDeclaration struct{ data *extensionSpecificationData }

// NewInputDeclaration validates an input-resource specification.
func NewInputDeclaration(input ExtensionSpecificationInput) (InputDeclaration, error) {
	data, err := newExtensionSpecification(input)
	return InputDeclaration{data: data}, err
}

// NewExecutionEnvironmentDeclaration validates an execution-environment specification.
func NewExecutionEnvironmentDeclaration(input ExtensionSpecificationInput) (ExecutionEnvironmentDeclaration, error) {
	data, err := newExtensionSpecification(input)
	return ExecutionEnvironmentDeclaration{data: data}, err
}

// NewDeploymentDeclaration validates a deployment specification.
func NewDeploymentDeclaration(input ExtensionSpecificationInput) (DeploymentDeclaration, error) {
	data, err := newExtensionSpecification(input)
	return DeploymentDeclaration{data: data}, err
}

func newExtensionSpecification(input ExtensionSpecificationInput) (*extensionSpecificationData, error) {
	if input.SchemaVersion != SchemaVersion {
		return nil, invalid(CodeInvalidVersion, "schema_version")
	}
	if err := validateIdentifier(input.Owner, "owner"); err != nil {
		return nil, err
	}
	if err := validateIdentifier(input.Kind, "kind"); err != nil {
		return nil, err
	}
	if err := validateIdentifier(input.SpecificationSchema, "specification_schema"); err != nil {
		return nil, err
	}
	fields, err := canonicalDeclarationFields(input.Fields, "fields")
	if err != nil {
		return nil, err
	}
	return &extensionSpecificationData{input.SchemaVersion, input.Owner, input.Kind, input.SpecificationSchema, fields}, nil
}

func canonicalDeclarationFields(source []DeclarationField, path string) ([]DeclarationField, error) {
	if len(source) > MaxResolvedFields {
		return nil, invalid(CodeTooLarge, path)
	}
	fields := append([]DeclarationField(nil), source...)
	sort.Slice(fields, func(i, j int) bool { return fields[i].Name < fields[j].Name })
	for index, field := range fields {
		fieldPath := path + "[" + itoa(index) + "]"
		if err := validateIdentifier(field.Name, fieldPath+".name"); err != nil {
			return nil, err
		}
		if err := validateUTF8(field.Value, fieldPath+".value", false, MaxResolvedValueBytes); err != nil {
			return nil, err
		}
		if index > 0 && fields[index-1].Name == field.Name {
			return nil, invalid(CodeDuplicateField, fieldPath+".name")
		}
	}
	return fields, nil
}

func copyDeclarationFields(source []DeclarationField) []DeclarationField {
	return append([]DeclarationField(nil), source...)
}

func (d *extensionSpecificationData) clone() *extensionSpecificationData {
	if d == nil {
		return nil
	}
	return &extensionSpecificationData{d.schemaVersion, d.owner, d.kind, d.specificationSchema, copyDeclarationFields(d.fields)}
}

func specificationSchema(data *extensionSpecificationData) string {
	if data == nil {
		return ""
	}
	return data.specificationSchema
}
func specificationOwner(data *extensionSpecificationData) string {
	if data == nil {
		return ""
	}
	return data.owner
}
func specificationKind(data *extensionSpecificationData) string {
	if data == nil {
		return ""
	}
	return data.kind
}
func specificationFields(data *extensionSpecificationData) []DeclarationField {
	if data == nil {
		return nil
	}
	return copyDeclarationFields(data.fields)
}
func specificationVersion(data *extensionSpecificationData) string {
	if data == nil {
		return ""
	}
	return data.schemaVersion
}

func (d InputDeclaration) Role() string                         { return "input" }
func (d ExecutionEnvironmentDeclaration) Role() string          { return "execution_environment" }
func (d DeploymentDeclaration) Role() string                    { return "deployment" }
func (d InputDeclaration) SchemaVersion() string                { return specificationVersion(d.data) }
func (d ExecutionEnvironmentDeclaration) SchemaVersion() string { return specificationVersion(d.data) }
func (d DeploymentDeclaration) SchemaVersion() string           { return specificationVersion(d.data) }
func (d InputDeclaration) Owner() string                        { return specificationOwner(d.data) }
func (d ExecutionEnvironmentDeclaration) Owner() string         { return specificationOwner(d.data) }
func (d DeploymentDeclaration) Owner() string                   { return specificationOwner(d.data) }
func (d InputDeclaration) Kind() string                         { return specificationKind(d.data) }
func (d ExecutionEnvironmentDeclaration) Kind() string          { return specificationKind(d.data) }
func (d DeploymentDeclaration) Kind() string                    { return specificationKind(d.data) }
func (d InputDeclaration) SpecificationSchema() string          { return specificationSchema(d.data) }
func (d ExecutionEnvironmentDeclaration) SpecificationSchema() string {
	return specificationSchema(d.data)
}
func (d DeploymentDeclaration) SpecificationSchema() string { return specificationSchema(d.data) }
func (d InputDeclaration) Fields() []DeclarationField       { return specificationFields(d.data) }
func (d ExecutionEnvironmentDeclaration) Fields() []DeclarationField {
	return specificationFields(d.data)
}
func (d DeploymentDeclaration) Fields() []DeclarationField { return specificationFields(d.data) }

type extensionSpecificationWire struct {
	SchemaVersion       *string             `json:"schema_version"`
	Owner               *string             `json:"owner"`
	Kind                *string             `json:"kind"`
	SpecificationSchema *string             `json:"specification_schema"`
	Fields              *[]DeclarationField `json:"fields"`
}

func specificationWire(data *extensionSpecificationData) (extensionSpecificationWire, error) {
	if data == nil {
		return extensionSpecificationWire{}, invalid(CodeInvalidShape, "$")
	}
	fields := copyDeclarationFields(data.fields)
	return extensionSpecificationWire{&data.schemaVersion, &data.owner, &data.kind, &data.specificationSchema, &fields}, nil
}

func specificationFromWire(w extensionSpecificationWire) (*extensionSpecificationData, error) {
	if w.SchemaVersion == nil {
		return nil, invalid(CodeInvalidShape, "schema_version")
	}
	if w.Owner == nil {
		return nil, invalid(CodeInvalidShape, "owner")
	}
	if w.Kind == nil {
		return nil, invalid(CodeInvalidShape, "kind")
	}
	if w.SpecificationSchema == nil {
		return nil, invalid(CodeInvalidShape, "specification_schema")
	}
	if w.Fields == nil {
		return nil, invalid(CodeInvalidShape, "fields")
	}
	return newExtensionSpecification(ExtensionSpecificationInput{*w.SchemaVersion, *w.Owner, *w.Kind, *w.SpecificationSchema, *w.Fields})
}

func marshalSpecification(data *extensionSpecificationData) ([]byte, error) {
	wire, err := specificationWire(data)
	if err != nil {
		return nil, err
	}
	return json.Marshal(wire)
}
func unmarshalSpecification(raw []byte) (*extensionSpecificationData, error) {
	var wire extensionSpecificationWire
	if err := decodeStrict(raw, &wire); err != nil {
		return nil, err
	}
	return specificationFromWire(wire)
}

func (d InputDeclaration) MarshalJSON() ([]byte, error) { return marshalSpecification(d.data) }
func (d ExecutionEnvironmentDeclaration) MarshalJSON() ([]byte, error) {
	return marshalSpecification(d.data)
}
func (d DeploymentDeclaration) MarshalJSON() ([]byte, error) { return marshalSpecification(d.data) }
func (d *InputDeclaration) UnmarshalJSON(raw []byte) error {
	if d == nil {
		return invalid(CodeInvalidShape, "$")
	}
	data, err := unmarshalSpecification(raw)
	if err != nil {
		return err
	}
	d.data = data
	return nil
}
func (d *ExecutionEnvironmentDeclaration) UnmarshalJSON(raw []byte) error {
	if d == nil {
		return invalid(CodeInvalidShape, "$")
	}
	data, err := unmarshalSpecification(raw)
	if err != nil {
		return err
	}
	d.data = data
	return nil
}
func (d *DeploymentDeclaration) UnmarshalJSON(raw []byte) error {
	if d == nil {
		return invalid(CodeInvalidShape, "$")
	}
	data, err := unmarshalSpecification(raw)
	if err != nil {
		return err
	}
	d.data = data
	return nil
}
