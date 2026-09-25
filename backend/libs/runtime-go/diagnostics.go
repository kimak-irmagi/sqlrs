package runtimev2

import "encoding/json"

// DiagnosticObservationInput supplies a versioned, non-identity observation.
// Requirements: runtime-v2-declaration-structure.md.
type DiagnosticObservationInput struct {
	SchemaVersion     string
	Owner             string
	Kind              string
	ObservationSchema string
	Fields            []DeclarationField
}

type diagnosticObservationData struct {
	schemaVersion     string
	owner             string
	kind              string
	observationSchema string
	fields            []DeclarationField
}

// CapabilityObservation records non-identity capability diagnostics.
type CapabilityObservation struct{ data *diagnosticObservationData }

// PortabilityObservation records non-identity portability diagnostics.
type PortabilityObservation struct{ data *diagnosticObservationData }

// NewCapabilityObservation validates and copies capability diagnostics.
func NewCapabilityObservation(input DiagnosticObservationInput) (CapabilityObservation, error) {
	data, err := newDiagnosticObservation(input)
	return CapabilityObservation{data: data}, err
}

// NewPortabilityObservation validates and copies portability diagnostics.
func NewPortabilityObservation(input DiagnosticObservationInput) (PortabilityObservation, error) {
	data, err := newDiagnosticObservation(input)
	return PortabilityObservation{data: data}, err
}

func newDiagnosticObservation(input DiagnosticObservationInput) (*diagnosticObservationData, error) {
	if input.SchemaVersion != SchemaVersion {
		return nil, invalid(CodeInvalidVersion, "schema_version")
	}
	if err := validateIdentifier(input.Owner, "owner"); err != nil {
		return nil, err
	}
	if err := validateIdentifier(input.Kind, "kind"); err != nil {
		return nil, err
	}
	if err := validateIdentifier(input.ObservationSchema, "observation_schema"); err != nil {
		return nil, err
	}
	fields, err := canonicalDeclarationFields(input.Fields, "fields")
	if err != nil {
		return nil, err
	}
	return &diagnosticObservationData{input.SchemaVersion, input.Owner, input.Kind, input.ObservationSchema, fields}, nil
}

func (d *diagnosticObservationData) clone() *diagnosticObservationData {
	if d == nil {
		return nil
	}
	return &diagnosticObservationData{d.schemaVersion, d.owner, d.kind, d.observationSchema, copyDeclarationFields(d.fields)}
}

func (d CapabilityObservation) Role() string  { return "capability" }
func (d PortabilityObservation) Role() string { return "portability" }
func (d CapabilityObservation) SchemaVersion() string {
	if d.data == nil {
		return ""
	}
	return d.data.schemaVersion
}
func (d PortabilityObservation) SchemaVersion() string {
	if d.data == nil {
		return ""
	}
	return d.data.schemaVersion
}
func (d CapabilityObservation) Owner() string {
	if d.data == nil {
		return ""
	}
	return d.data.owner
}
func (d PortabilityObservation) Owner() string {
	if d.data == nil {
		return ""
	}
	return d.data.owner
}
func (d CapabilityObservation) Kind() string {
	if d.data == nil {
		return ""
	}
	return d.data.kind
}
func (d PortabilityObservation) Kind() string {
	if d.data == nil {
		return ""
	}
	return d.data.kind
}
func (d CapabilityObservation) ObservationSchema() string {
	if d.data == nil {
		return ""
	}
	return d.data.observationSchema
}
func (d PortabilityObservation) ObservationSchema() string {
	if d.data == nil {
		return ""
	}
	return d.data.observationSchema
}
func (d CapabilityObservation) Fields() []DeclarationField {
	if d.data == nil {
		return nil
	}
	return copyDeclarationFields(d.data.fields)
}
func (d PortabilityObservation) Fields() []DeclarationField {
	if d.data == nil {
		return nil
	}
	return copyDeclarationFields(d.data.fields)
}

type diagnosticObservationWire struct {
	SchemaVersion     *string             `json:"schema_version"`
	Owner             *string             `json:"owner"`
	Kind              *string             `json:"kind"`
	ObservationSchema *string             `json:"observation_schema"`
	Fields            *[]DeclarationField `json:"fields"`
}

func marshalDiagnostic(data *diagnosticObservationData) ([]byte, error) {
	if data == nil {
		return nil, invalid(CodeInvalidShape, "$")
	}
	fields := copyDeclarationFields(data.fields)
	return json.Marshal(diagnosticObservationWire{&data.schemaVersion, &data.owner, &data.kind, &data.observationSchema, &fields})
}
func unmarshalDiagnostic(raw []byte) (*diagnosticObservationData, error) {
	var wire diagnosticObservationWire
	if err := decodeStrict(raw, &wire); err != nil {
		return nil, err
	}
	if wire.SchemaVersion == nil {
		return nil, invalid(CodeInvalidShape, "schema_version")
	}
	if wire.Owner == nil {
		return nil, invalid(CodeInvalidShape, "owner")
	}
	if wire.Kind == nil {
		return nil, invalid(CodeInvalidShape, "kind")
	}
	if wire.ObservationSchema == nil {
		return nil, invalid(CodeInvalidShape, "observation_schema")
	}
	if wire.Fields == nil {
		return nil, invalid(CodeInvalidShape, "fields")
	}
	return newDiagnosticObservation(DiagnosticObservationInput{*wire.SchemaVersion, *wire.Owner, *wire.Kind, *wire.ObservationSchema, *wire.Fields})
}

func (d CapabilityObservation) MarshalJSON() ([]byte, error)  { return marshalDiagnostic(d.data) }
func (d PortabilityObservation) MarshalJSON() ([]byte, error) { return marshalDiagnostic(d.data) }
func (d *CapabilityObservation) UnmarshalJSON(raw []byte) error {
	if d == nil {
		return invalid(CodeInvalidShape, "$")
	}
	data, err := unmarshalDiagnostic(raw)
	if err != nil {
		return err
	}
	d.data = data
	return nil
}
func (d *PortabilityObservation) UnmarshalJSON(raw []byte) error {
	if d == nil {
		return invalid(CodeInvalidShape, "$")
	}
	data, err := unmarshalDiagnostic(raw)
	if err != nil {
		return err
	}
	d.data = data
	return nil
}
