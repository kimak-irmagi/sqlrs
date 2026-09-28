package runtimev2

import (
	"sort"
	"unicode/utf8"

	"github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/internal/canonicalv1"
)

type operationalObservationData struct {
	owner, semanticKind, observationSchema string
	fields                                 map[string]string
}

// OperationalObservation carries diagnostics outside canonical identity.
type OperationalObservation struct{ data *operationalObservationData }

// NewOperationalObservation constructs an immutable diagnostic value. A
// schema-bound builder still validates its owner, kind, schema, and field set.
func NewOperationalObservation(owner, semanticKind, observationSchema string, fields map[string]string) (OperationalObservation, error) {
	if err := validateIdentifier(owner, "owner"); err != nil {
		return OperationalObservation{}, canonicalizeValidationError(err)
	}
	if err := validateIdentifier(semanticKind, "semantic_kind"); err != nil {
		return OperationalObservation{}, canonicalizeValidationError(err)
	}
	if err := validateIdentifier(observationSchema, "observation_schema"); err != nil {
		return OperationalObservation{}, canonicalizeValidationError(err)
	}
	copyFields := make(map[string]string, len(fields))
	for name, value := range fields {
		if err := validateIdentifier(name, "fields.name"); err != nil {
			return OperationalObservation{}, canonicalizeValidationError(err)
		}
		if !utf8.ValidString(value) || len(value) > MaxCanonicalStringBytes {
			return OperationalObservation{}, canonicalInvalid(CodeValueInvalid, "fields."+name)
		}
		copyFields[name] = value
	}
	return OperationalObservation{data: &operationalObservationData{owner: owner, semanticKind: semanticKind, observationSchema: observationSchema, fields: copyFields}}, nil
}

func copyObservation(value OperationalObservation) OperationalObservation {
	if value.data == nil {
		return OperationalObservation{}
	}
	fields := make(map[string]string, len(value.data.fields))
	for name, item := range value.data.fields {
		fields[name] = item
	}
	data := *value.data
	data.fields = fields
	return OperationalObservation{data: &data}
}

type canonicalBuilder struct {
	provider, semanticKind, identitySchema, observationSchema string
	definitions                                               map[string]canonicalv1.FieldDefinition
	fields                                                    map[string]canonicalIdentityField
	observations                                              []OperationalObservation
	sealed                                                    bool
}

func newCanonicalBuilder(provider, semanticKind, identitySchema, observationSchema string, definitions []canonicalv1.FieldDefinition) *canonicalBuilder {
	indexed := make(map[string]canonicalv1.FieldDefinition, len(definitions))
	for _, definition := range definitions {
		indexed[definition.Name()] = definition
	}
	return &canonicalBuilder{provider: provider, semanticKind: semanticKind, identitySchema: identitySchema, observationSchema: observationSchema, definitions: indexed, fields: map[string]canonicalIdentityField{}}
}

func (b *canonicalBuilder) addIdentityField(field IdentityField) error {
	if b == nil || b.sealed {
		return canonicalInvalid(CodeShapeInvalid, "builder")
	}
	if !field.Valid() {
		return canonicalInvalid(CodeShapeInvalid, "field")
	}
	definition, exists := b.definitions[field.Name()]
	if !exists || definition.Role() != canonicalv1.FieldRoleSemantic {
		return canonicalInvalid(CodeValueInvalid, "field.name")
	}
	if definition.Kind() != string(field.Kind()) {
		return canonicalInvalid(CodeValueInvalid, "field.kind")
	}
	if _, exists := b.fields[field.Name()]; exists {
		return canonicalInvalid(CodeNonCanonical, "field.name")
	}
	b.fields[field.Name()] = canonicalIdentityField{field: field, disclosure: definition.Disclosure()}
	return nil
}

func (b *canonicalBuilder) addObservation(value OperationalObservation) error {
	if b == nil || b.sealed {
		return canonicalInvalid(CodeShapeInvalid, "builder")
	}
	if value.data == nil {
		return canonicalInvalid(CodeShapeInvalid, "observation")
	}
	if value.data.owner != b.provider || value.data.semanticKind != b.semanticKind || value.data.observationSchema != b.observationSchema {
		return canonicalInvalid(CodeValueInvalid, "observation")
	}
	for name := range value.data.fields {
		definition, exists := b.definitions[name]
		if !exists || definition.Role() != canonicalv1.FieldRoleOperational || definition.Kind() != string(IdentityFieldText) {
			return canonicalInvalid(CodeValueInvalid, "observation.fields."+name)
		}
	}
	b.observations = append(b.observations, copyObservation(value))
	return nil
}

func (b *canonicalBuilder) build() (*canonicalIdentityData, []OperationalObservation, error) {
	if b == nil || b.sealed {
		return nil, nil, canonicalInvalid(CodeShapeInvalid, "builder")
	}
	for name, definition := range b.definitions {
		if definition.Role() == canonicalv1.FieldRoleSemantic && definition.Required() {
			if _, exists := b.fields[name]; !exists {
				return nil, nil, canonicalInvalid(CodeShapeInvalid, "fields."+name)
			}
		}
	}
	names := make([]string, 0, len(b.fields))
	for name := range b.fields {
		names = append(names, name)
	}
	sort.Strings(names)
	fields := make([]canonicalIdentityField, len(names))
	for index, name := range names {
		fields[index] = b.fields[name]
	}
	observations := make([]OperationalObservation, len(b.observations))
	for index, value := range b.observations {
		observations[index] = copyObservation(value)
	}
	b.sealed = true
	return &canonicalIdentityData{provider: b.provider, semanticKind: b.semanticKind, identitySchema: b.identitySchema, fields: fields}, observations, nil
}

// FactoryIdentityBuilder is a transactional single-use schema-bound builder.
type FactoryIdentityBuilder struct {
	builder     *canonicalBuilder
	declaration FactoryDeclaration
	extensions  bool
}

// TransformIdentityBuilder is a transactional single-use schema-bound builder.
type TransformIdentityBuilder struct {
	builder     *canonicalBuilder
	declaration TransformDeclaration
	extensions  bool
}

// ExtensionIdentityBuilder constructs a provider-owned extension identity.
type ExtensionIdentityBuilder struct{ builder *canonicalBuilder }

func NewFactoryIdentityBuilder(schema FactoryIdentitySchema, declaration FactoryDeclaration) (*FactoryIdentityBuilder, error) {
	if !schema.Valid() {
		return nil, canonicalInvalid(CodeShapeInvalid, "schema")
	}
	if err := validateFactoryDeclaration(&declaration); err != nil {
		return nil, prefixError(err, "declaration")
	}
	if declaration.Kind != schema.SemanticKind() {
		return nil, canonicalInvalid(CodeValueInvalid, "declaration.kind")
	}
	return &FactoryIdentityBuilder{builder: newCanonicalBuilder(schema.Provider(), schema.SemanticKind(), schema.IdentitySchema(), schema.ObservationSchema(), schema.Fields()), declaration: *copyFactoryDeclaration(&declaration)}, nil
}

func NewTransformIdentityBuilder(schema TransformIdentitySchema, declaration TransformDeclaration) (*TransformIdentityBuilder, error) {
	if !schema.Valid() {
		return nil, canonicalInvalid(CodeShapeInvalid, "schema")
	}
	if err := validateTransformDeclaration(&declaration); err != nil {
		return nil, prefixError(err, "declaration")
	}
	if declaration.Kind != schema.SemanticKind() {
		return nil, canonicalInvalid(CodeValueInvalid, "declaration.kind")
	}
	return &TransformIdentityBuilder{builder: newCanonicalBuilder(schema.Provider(), schema.SemanticKind(), schema.IdentitySchema(), schema.ObservationSchema(), schema.Fields()), declaration: *copyTransformDeclaration(&declaration)}, nil
}

// NewExtensionIdentityBuilder binds an extension identity to an authored schema.
func NewExtensionIdentityBuilder(schema ExtensionIdentitySchema) (*ExtensionIdentityBuilder, error) {
	if !schema.Valid() {
		return nil, canonicalInvalid(CodeShapeInvalid, "schema")
	}
	return &ExtensionIdentityBuilder{builder: newCanonicalBuilder(schema.Provider(), schema.SemanticKind(), schema.IdentitySchema(), schema.ObservationSchema(), schema.Fields())}, nil
}

func (b *FactoryIdentityBuilder) AddIdentityField(field IdentityField) error {
	if b == nil {
		return canonicalInvalid(CodeShapeInvalid, "builder")
	}
	return b.builder.addIdentityField(field)
}
func (b *FactoryIdentityBuilder) AddObservation(value OperationalObservation) error {
	if b == nil {
		return canonicalInvalid(CodeShapeInvalid, "builder")
	}
	return b.builder.addObservation(value)
}
func (b *TransformIdentityBuilder) AddIdentityField(field IdentityField) error {
	if b == nil {
		return canonicalInvalid(CodeShapeInvalid, "builder")
	}
	return b.builder.addIdentityField(field)
}
func (b *TransformIdentityBuilder) AddObservation(value OperationalObservation) error {
	if b == nil {
		return canonicalInvalid(CodeShapeInvalid, "builder")
	}
	return b.builder.addObservation(value)
}
func (b *ExtensionIdentityBuilder) AddIdentityField(field IdentityField) error {
	if b == nil {
		return canonicalInvalid(CodeShapeInvalid, "builder")
	}
	return b.builder.addIdentityField(field)
}
func (b *ExtensionIdentityBuilder) AddObservation(value OperationalObservation) error {
	if b == nil {
		return canonicalInvalid(CodeShapeInvalid, "builder")
	}
	return b.builder.addObservation(value)
}

// ResolvedFactoryExtensions preserves declaration role and input position.
type ResolvedFactoryExtensions struct {
	Inputs               []CanonicalResolvedExtensionIdentity
	ExecutionEnvironment *CanonicalResolvedExtensionIdentity
	Deployment           *CanonicalResolvedExtensionIdentity
}

// ResolvedTransformExtensions omits the factory-only deployment role.
type ResolvedTransformExtensions struct {
	Inputs               []CanonicalResolvedExtensionIdentity
	ExecutionEnvironment *CanonicalResolvedExtensionIdentity
}

func validateCanonicalExtension(declaration ExtensionDeclaration, identity CanonicalResolvedExtensionIdentity, path string) error {
	if identity.data == nil {
		return canonicalInvalid(CodeShapeInvalid, path)
	}
	if declaration == nil || declaration.Owner() != identity.data.provider || declaration.Kind() != identity.data.semanticKind {
		return canonicalInvalid(CodeValueInvalid, path)
	}
	return nil
}

func extensionIdentityField(name string, identity CanonicalResolvedExtensionIdentity) (IdentityField, error) {
	fingerprint, err := CanonicalExtensionFingerprint(identity)
	if err != nil {
		return IdentityField{}, err
	}
	return NewTextIdentityField(name, string(fingerprint))
}

func (b *FactoryIdentityBuilder) BindExtensions(value ResolvedFactoryExtensions) error {
	if b == nil || b.builder == nil || b.builder.sealed || b.extensions {
		return canonicalInvalid(CodeShapeInvalid, "extensions")
	}
	if len(value.Inputs) != len(b.declaration.Inputs) {
		return canonicalInvalid(CodeShapeInvalid, "extensions.inputs")
	}
	staged := make([]IdentityField, 0, len(value.Inputs)+2)
	for index, identity := range value.Inputs {
		path := "extensions.inputs[" + itoa(index) + "]"
		if err := validateCanonicalExtension(b.declaration.Inputs[index], identity, path); err != nil {
			return err
		}
		field, err := extensionIdentityField("extension.input."+itoa(index), identity)
		if err != nil {
			return err
		}
		staged = append(staged, field)
	}
	if (b.declaration.ExecutionEnvironment == nil) != (value.ExecutionEnvironment == nil) {
		return canonicalInvalid(CodeShapeInvalid, "extensions.execution_environment")
	}
	if value.ExecutionEnvironment != nil {
		if err := validateCanonicalExtension(*b.declaration.ExecutionEnvironment, *value.ExecutionEnvironment, "extensions.execution_environment"); err != nil {
			return err
		}
		field, err := extensionIdentityField("extension.execution_environment", *value.ExecutionEnvironment)
		if err != nil {
			return err
		}
		staged = append(staged, field)
	}
	if (b.declaration.Deployment == nil) != (value.Deployment == nil) {
		return canonicalInvalid(CodeShapeInvalid, "extensions.deployment")
	}
	if value.Deployment != nil {
		if err := validateCanonicalExtension(*b.declaration.Deployment, *value.Deployment, "extensions.deployment"); err != nil {
			return err
		}
		field, err := extensionIdentityField("extension.deployment", *value.Deployment)
		if err != nil {
			return err
		}
		staged = append(staged, field)
	}
	for _, field := range staged {
		b.builder.fields[field.Name()] = canonicalIdentityField{field: field, disclosure: DisclosurePublic}
	}
	b.extensions = true
	return nil
}

func (b *TransformIdentityBuilder) BindExtensions(value ResolvedTransformExtensions) error {
	if b == nil || b.builder == nil || b.builder.sealed || b.extensions {
		return canonicalInvalid(CodeShapeInvalid, "extensions")
	}
	if len(value.Inputs) != len(b.declaration.Inputs) {
		return canonicalInvalid(CodeShapeInvalid, "extensions.inputs")
	}
	staged := make([]IdentityField, 0, len(value.Inputs)+1)
	for index, identity := range value.Inputs {
		path := "extensions.inputs[" + itoa(index) + "]"
		if err := validateCanonicalExtension(b.declaration.Inputs[index], identity, path); err != nil {
			return err
		}
		field, err := extensionIdentityField("extension.input."+itoa(index), identity)
		if err != nil {
			return err
		}
		staged = append(staged, field)
	}
	if (b.declaration.ExecutionEnvironment == nil) != (value.ExecutionEnvironment == nil) {
		return canonicalInvalid(CodeShapeInvalid, "extensions.execution_environment")
	}
	if value.ExecutionEnvironment != nil {
		if err := validateCanonicalExtension(*b.declaration.ExecutionEnvironment, *value.ExecutionEnvironment, "extensions.execution_environment"); err != nil {
			return err
		}
		field, err := extensionIdentityField("extension.execution_environment", *value.ExecutionEnvironment)
		if err != nil {
			return err
		}
		staged = append(staged, field)
	}
	for _, field := range staged {
		b.builder.fields[field.Name()] = canonicalIdentityField{field: field, disclosure: DisclosurePublic}
	}
	b.extensions = true
	return nil
}

func factoryNeedsExtensions(value FactoryDeclaration) bool {
	return len(value.Inputs) != 0 || value.ExecutionEnvironment != nil || value.Deployment != nil
}
func transformNeedsExtensions(value TransformDeclaration) bool {
	return len(value.Inputs) != 0 || value.ExecutionEnvironment != nil
}

// FactoryIdentityComposition keeps observations separate from its identity.
type FactoryIdentityComposition struct {
	identity     CanonicalFactoryIdentity
	observations []OperationalObservation
	root         CanonicalStateID
}

// TransformIdentityComposition keeps observations separate from its identity.
type TransformIdentityComposition struct {
	identity     CanonicalTransformIdentity
	observations []OperationalObservation
}

// ExtensionIdentityComposition keeps extension observations outside identity.
type ExtensionIdentityComposition struct {
	identity     CanonicalResolvedExtensionIdentity
	observations []OperationalObservation
}

func (b *FactoryIdentityBuilder) Build() (FactoryIdentityComposition, error) {
	if b == nil {
		return FactoryIdentityComposition{}, canonicalInvalid(CodeShapeInvalid, "builder")
	}
	if factoryNeedsExtensions(b.declaration) && !b.extensions {
		return FactoryIdentityComposition{}, canonicalInvalid(CodeShapeInvalid, "extensions")
	}
	data, observations, err := b.builder.build()
	if err != nil {
		return FactoryIdentityComposition{}, err
	}
	identity := CanonicalFactoryIdentity{data: data}
	fingerprint, err := CanonicalFactoryFingerprint(identity)
	if err != nil {
		return FactoryIdentityComposition{}, err
	}
	return FactoryIdentityComposition{identity: identity, observations: observations, root: CanonicalStateID(fingerprint)}, nil
}

func (b *TransformIdentityBuilder) Build() (TransformIdentityComposition, error) {
	if b == nil {
		return TransformIdentityComposition{}, canonicalInvalid(CodeShapeInvalid, "builder")
	}
	if transformNeedsExtensions(b.declaration) && !b.extensions {
		return TransformIdentityComposition{}, canonicalInvalid(CodeShapeInvalid, "extensions")
	}
	data, observations, err := b.builder.build()
	if err != nil {
		return TransformIdentityComposition{}, err
	}
	return TransformIdentityComposition{identity: CanonicalTransformIdentity{data: data}, observations: observations}, nil
}

func (b *ExtensionIdentityBuilder) Build() (ExtensionIdentityComposition, error) {
	if b == nil {
		return ExtensionIdentityComposition{}, canonicalInvalid(CodeShapeInvalid, "builder")
	}
	data, observations, err := b.builder.build()
	if err != nil {
		return ExtensionIdentityComposition{}, err
	}
	return ExtensionIdentityComposition{identity: CanonicalResolvedExtensionIdentity{data: data}, observations: observations}, nil
}

func (c FactoryIdentityComposition) Identity() CanonicalFactoryIdentity {
	return CanonicalFactoryIdentity{data: copyCanonicalIdentityData(c.identity.data)}
}
func (c FactoryIdentityComposition) RootStateID() CanonicalStateID { return c.root }
func (c FactoryIdentityComposition) Observations() []OperationalObservation {
	result := make([]OperationalObservation, len(c.observations))
	for index, value := range c.observations {
		result[index] = copyObservation(value)
	}
	return result
}
func (c TransformIdentityComposition) Identity() CanonicalTransformIdentity {
	return CanonicalTransformIdentity{data: copyCanonicalIdentityData(c.identity.data)}
}
func (c TransformIdentityComposition) Observations() []OperationalObservation {
	result := make([]OperationalObservation, len(c.observations))
	for index, value := range c.observations {
		result[index] = copyObservation(value)
	}
	return result
}
func (c ExtensionIdentityComposition) Identity() CanonicalResolvedExtensionIdentity {
	return CanonicalResolvedExtensionIdentity{data: copyCanonicalIdentityData(c.identity.data)}
}
func (c ExtensionIdentityComposition) Observations() []OperationalObservation {
	result := make([]OperationalObservation, len(c.observations))
	for index, value := range c.observations {
		result[index] = copyObservation(value)
	}
	return result
}
