// Package canonicalv1 owns opaque schema data shared by the public runtime and
// the deliberately separate schemaauthor trust boundary.
package canonicalv1

// FieldRole separates semantic identity input from operational observations.
type FieldRole string

const (
	FieldRoleSemantic    FieldRole = "semantic"
	FieldRoleOperational FieldRole = "operational"
)

// DisclosureClass controls whether an explanation may expose a field payload.
type DisclosureClass string

const (
	DisclosurePublic    DisclosureClass = "public"
	DisclosureProtected DisclosureClass = "protected"
)

// FieldDefinition is an immutable authored field rule.
type FieldDefinition struct {
	name       string
	role       FieldRole
	kind       string
	required   bool
	disclosure DisclosureClass
}

// NewFieldDefinition is restricted by Go's internal import boundary. Public
// callers author definitions through runtime-go/schemaauthor.
func NewFieldDefinition(name string, role FieldRole, kind string, required bool, disclosure DisclosureClass) FieldDefinition {
	return FieldDefinition{name: name, role: role, kind: kind, required: required, disclosure: disclosure}
}

func (f FieldDefinition) Name() string                { return f.name }
func (f FieldDefinition) Role() FieldRole             { return f.role }
func (f FieldDefinition) Kind() string                { return f.kind }
func (f FieldDefinition) Required() bool              { return f.required }
func (f FieldDefinition) Disclosure() DisclosureClass { return f.disclosure }

type schemaData struct {
	provider, semanticKind, identitySchema, observationSchema string
	fields                                                    []FieldDefinition
}

func newSchema(provider, semanticKind, identitySchema, observationSchema string, fields []FieldDefinition) *schemaData {
	return &schemaData{
		provider: provider, semanticKind: semanticKind, identitySchema: identitySchema,
		observationSchema: observationSchema, fields: append([]FieldDefinition(nil), fields...),
	}
}

// FactorySchema, TransformSchema, and ExtensionSchema are intentionally
// distinct so a schema cannot be supplied to the wrong builder.
type FactorySchema struct{ data *schemaData }
type TransformSchema struct{ data *schemaData }
type ExtensionSchema struct{ data *schemaData }

func NewFactorySchema(provider, semanticKind, identitySchema, observationSchema string, fields []FieldDefinition) FactorySchema {
	return FactorySchema{data: newSchema(provider, semanticKind, identitySchema, observationSchema, fields)}
}
func NewTransformSchema(provider, semanticKind, identitySchema, observationSchema string, fields []FieldDefinition) TransformSchema {
	return TransformSchema{data: newSchema(provider, semanticKind, identitySchema, observationSchema, fields)}
}
func NewExtensionSchema(provider, semanticKind, identitySchema, observationSchema string, fields []FieldDefinition) ExtensionSchema {
	return ExtensionSchema{data: newSchema(provider, semanticKind, identitySchema, observationSchema, fields)}
}

func (s FactorySchema) Valid() bool   { return s.data != nil }
func (s TransformSchema) Valid() bool { return s.data != nil }
func (s ExtensionSchema) Valid() bool { return s.data != nil }

func schemaProvider(data *schemaData) string {
	if data == nil {
		return ""
	}
	return data.provider
}
func schemaKind(data *schemaData) string {
	if data == nil {
		return ""
	}
	return data.semanticKind
}
func schemaIdentity(data *schemaData) string {
	if data == nil {
		return ""
	}
	return data.identitySchema
}
func schemaObservation(data *schemaData) string {
	if data == nil {
		return ""
	}
	return data.observationSchema
}
func schemaFields(data *schemaData) []FieldDefinition {
	if data == nil {
		return nil
	}
	return append([]FieldDefinition(nil), data.fields...)
}

func (s FactorySchema) Provider() string          { return schemaProvider(s.data) }
func (s FactorySchema) SemanticKind() string      { return schemaKind(s.data) }
func (s FactorySchema) IdentitySchema() string    { return schemaIdentity(s.data) }
func (s FactorySchema) ObservationSchema() string { return schemaObservation(s.data) }
func (s FactorySchema) Fields() []FieldDefinition { return schemaFields(s.data) }

func (s TransformSchema) Provider() string          { return schemaProvider(s.data) }
func (s TransformSchema) SemanticKind() string      { return schemaKind(s.data) }
func (s TransformSchema) IdentitySchema() string    { return schemaIdentity(s.data) }
func (s TransformSchema) ObservationSchema() string { return schemaObservation(s.data) }
func (s TransformSchema) Fields() []FieldDefinition { return schemaFields(s.data) }

func (s ExtensionSchema) Provider() string          { return schemaProvider(s.data) }
func (s ExtensionSchema) SemanticKind() string      { return schemaKind(s.data) }
func (s ExtensionSchema) IdentitySchema() string    { return schemaIdentity(s.data) }
func (s ExtensionSchema) ObservationSchema() string { return schemaObservation(s.data) }
func (s ExtensionSchema) Fields() []FieldDefinition { return schemaFields(s.data) }
