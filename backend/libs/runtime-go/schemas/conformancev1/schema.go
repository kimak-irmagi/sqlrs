// Package conformancev1 provides stable fixtures for external contract verification.
// It is not a production provider and must not be used to model application schemas.
package conformancev1

import (
	"regexp"
	"sort"
	"unicode/utf8"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
	"github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/schemaauthor"
)

const (
	// Provider owns every identity and declaration exposed by this facade.
	Provider = "runtime-conformance"
	// FactoryKind identifies the conformance factory fixture.
	FactoryKind = "fixture-factory"
	// TransformKind identifies the conformance transform fixture.
	TransformKind = "fixture-transform"
	// ExtensionKind identifies the conformance extension fixture.
	ExtensionKind = "fixture-extension"

	// FactoryIdentitySchema identifies the factory identity schema.
	FactoryIdentitySchema = "runtime-conformance.factory.v1"
	// TransformIdentitySchema identifies the transform identity schema.
	TransformIdentitySchema = "runtime-conformance.transform.v1"
	// ExtensionIdentitySchema identifies the extension identity schema.
	ExtensionIdentitySchema = "runtime-conformance.extension.v1"
	// FactoryObservationSchema identifies factory operational observations.
	FactoryObservationSchema = "runtime-conformance.factory-observation.v1"
	// TransformObservationSchema identifies transform operational observations.
	TransformObservationSchema = "runtime-conformance.transform-observation.v1"
	// ExtensionObservationSchema identifies extension operational observations.
	ExtensionObservationSchema = "runtime-conformance.extension-observation.v1"
	// ExtensionSpecificationSchema identifies fixture extension declarations.
	ExtensionSpecificationSchema = "runtime-conformance.extension-specification.v1"

	// FieldLocator is the required public text identity field.
	FieldLocator = "locator"
	// FieldPlan is the optional protected canonical-value identity field.
	FieldPlan = "plan"
	// FieldCredential is the optional protected secret-reference identity field.
	FieldCredential = "credential"

	// ObservationJobID identifies a job observation.
	ObservationJobID = "job_id"
	// ObservationContainerID identifies a container observation.
	ObservationContainerID = "container_id"
	// ObservationTimestamp identifies a timestamp observation.
	ObservationTimestamp = "timestamp"
	// ObservationPhysicalSize identifies a physical-size observation.
	ObservationPhysicalSize = "physical_size"
	// ObservationMaterializationPath identifies a materialization-path observation.
	ObservationMaterializationPath = "materialization_path"
	// ObservationCheckpointBackend identifies a checkpoint-backend observation.
	ObservationCheckpointBackend = "checkpoint_backend"

	// DeclarationReference names the single fixture declaration field.
	DeclarationReference = "reference"
)

var observationNamePattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,127}$`)

var observationNames = map[string]struct{}{
	ObservationJobID:               {},
	ObservationContainerID:         {},
	ObservationTimestamp:           {},
	ObservationPhysicalSize:        {},
	ObservationMaterializationPath: {},
	ObservationCheckpointBackend:   {},
}

func fieldDefinitions() []schemaauthor.FieldDefinition {
	return []schemaauthor.FieldDefinition{
		{Name: FieldLocator, Role: schemaauthor.FieldRoleSemantic, Kind: runtimev2.IdentityFieldText, Required: true, Disclosure: schemaauthor.DisclosurePublic},
		{Name: FieldPlan, Role: schemaauthor.FieldRoleSemantic, Kind: runtimev2.IdentityFieldCanonicalValue, Disclosure: schemaauthor.DisclosureProtected},
		{Name: FieldCredential, Role: schemaauthor.FieldRoleSemantic, Kind: runtimev2.IdentityFieldSecretReference, Disclosure: schemaauthor.DisclosureProtected},
		{Name: ObservationJobID, Role: schemaauthor.FieldRoleOperational, Kind: runtimev2.IdentityFieldText, Disclosure: schemaauthor.DisclosureProtected},
		{Name: ObservationContainerID, Role: schemaauthor.FieldRoleOperational, Kind: runtimev2.IdentityFieldText, Disclosure: schemaauthor.DisclosureProtected},
		{Name: ObservationTimestamp, Role: schemaauthor.FieldRoleOperational, Kind: runtimev2.IdentityFieldText, Disclosure: schemaauthor.DisclosureProtected},
		{Name: ObservationPhysicalSize, Role: schemaauthor.FieldRoleOperational, Kind: runtimev2.IdentityFieldText, Disclosure: schemaauthor.DisclosureProtected},
		{Name: ObservationMaterializationPath, Role: schemaauthor.FieldRoleOperational, Kind: runtimev2.IdentityFieldText, Disclosure: schemaauthor.DisclosureProtected},
		{Name: ObservationCheckpointBackend, Role: schemaauthor.FieldRoleOperational, Kind: runtimev2.IdentityFieldText, Disclosure: schemaauthor.DisclosureProtected},
	}
}

func factorySchema() (runtimev2.FactoryIdentitySchema, error) {
	return schemaauthor.NewFactorySchema(schemaauthor.SchemaInput{Provider: Provider, SemanticKind: FactoryKind, IdentitySchema: FactoryIdentitySchema, ObservationSchema: FactoryObservationSchema, Fields: fieldDefinitions()})
}

func transformSchema() (runtimev2.TransformIdentitySchema, error) {
	return schemaauthor.NewTransformSchema(schemaauthor.SchemaInput{Provider: Provider, SemanticKind: TransformKind, IdentitySchema: TransformIdentitySchema, ObservationSchema: TransformObservationSchema, Fields: fieldDefinitions()})
}

func extensionSchema() (runtimev2.ExtensionIdentitySchema, error) {
	return schemaauthor.NewExtensionSchema(schemaauthor.SchemaInput{Provider: Provider, SemanticKind: ExtensionKind, IdentitySchema: ExtensionIdentitySchema, ObservationSchema: ExtensionObservationSchema, Fields: fieldDefinitions()})
}

// NewFactoryBuilder constructs a builder bound to the conformance factory schema.
func NewFactoryBuilder(declaration runtimev2.FactoryDeclaration) (*runtimev2.FactoryIdentityBuilder, error) {
	schema, err := factorySchema()
	if err != nil {
		return nil, err
	}
	return runtimev2.NewFactoryIdentityBuilder(schema, declaration)
}

// NewTransformBuilder constructs a builder bound to the conformance transform schema.
func NewTransformBuilder(declaration runtimev2.TransformDeclaration) (*runtimev2.TransformIdentityBuilder, error) {
	schema, err := transformSchema()
	if err != nil {
		return nil, err
	}
	return runtimev2.NewTransformIdentityBuilder(schema, declaration)
}

// NewExtensionBuilder constructs a builder bound to the conformance extension schema.
func NewExtensionBuilder() (*runtimev2.ExtensionIdentityBuilder, error) {
	schema, err := extensionSchema()
	if err != nil {
		return nil, err
	}
	return runtimev2.NewExtensionIdentityBuilder(schema)
}

func newObservation(kind, observationSchema string, source map[string]string) (runtimev2.OperationalObservation, error) {
	fields := make(map[string]string, len(source))
	names := make([]string, 0, len(source))
	for name, value := range source {
		names = append(names, name)
		fields[name] = value
	}
	sort.Strings(names)
	for _, name := range names {
		if !utf8.ValidString(name) || !observationNamePattern.MatchString(name) {
			return runtimev2.OperationalObservation{}, &runtimev2.ValidationError{Code: runtimev2.CodeValueInvalid, Path: "fields.name"}
		}
		if _, ok := observationNames[name]; !ok {
			return runtimev2.OperationalObservation{}, &runtimev2.ValidationError{Code: runtimev2.CodeUnknownMember, Path: "fields." + name}
		}
	}
	for _, name := range names {
		value := fields[name]
		if !utf8.ValidString(value) {
			return runtimev2.OperationalObservation{}, &runtimev2.ValidationError{Code: runtimev2.CodeValueInvalid, Path: "fields." + name}
		}
		if len(value) > runtimev2.MaxCanonicalStringBytes {
			return runtimev2.OperationalObservation{}, &runtimev2.ValidationError{Code: runtimev2.CodeLimitExceeded, Path: "fields." + name}
		}
	}
	return runtimev2.NewOperationalObservation(Provider, kind, observationSchema, fields)
}

// NewFactoryObservation validates and constructs a factory observation.
func NewFactoryObservation(fields map[string]string) (runtimev2.OperationalObservation, error) {
	return newObservation(FactoryKind, FactoryObservationSchema, fields)
}

// NewTransformObservation validates and constructs a transform observation.
func NewTransformObservation(fields map[string]string) (runtimev2.OperationalObservation, error) {
	return newObservation(TransformKind, TransformObservationSchema, fields)
}

// NewExtensionObservation validates and constructs an extension observation.
func NewExtensionObservation(fields map[string]string) (runtimev2.OperationalObservation, error) {
	return newObservation(ExtensionKind, ExtensionObservationSchema, fields)
}

func declarationInput(reference string) (runtimev2.ExtensionSpecificationInput, error) {
	if !utf8.ValidString(reference) || reference == "" {
		return runtimev2.ExtensionSpecificationInput{}, &runtimev2.ValidationError{Code: runtimev2.CodeValueInvalid, Path: DeclarationReference}
	}
	if len(reference) > runtimev2.MaxResolvedValueBytes {
		return runtimev2.ExtensionSpecificationInput{}, &runtimev2.ValidationError{Code: runtimev2.CodeLimitExceeded, Path: DeclarationReference}
	}
	return runtimev2.ExtensionSpecificationInput{
		SchemaVersion: runtimev2.SchemaVersion, Owner: Provider, Kind: ExtensionKind,
		SpecificationSchema: ExtensionSpecificationSchema,
		Fields:              []runtimev2.DeclarationField{{Name: DeclarationReference, Value: reference}},
	}, nil
}

// NewInputDeclaration constructs a conformance input declaration.
func NewInputDeclaration(reference string) (runtimev2.InputDeclaration, error) {
	input, err := declarationInput(reference)
	if err != nil {
		return runtimev2.InputDeclaration{}, err
	}
	return runtimev2.NewInputDeclaration(input)
}

// NewExecutionEnvironmentDeclaration constructs a conformance execution-environment declaration.
func NewExecutionEnvironmentDeclaration(reference string) (runtimev2.ExecutionEnvironmentDeclaration, error) {
	input, err := declarationInput(reference)
	if err != nil {
		return runtimev2.ExecutionEnvironmentDeclaration{}, err
	}
	return runtimev2.NewExecutionEnvironmentDeclaration(input)
}

// NewDeploymentDeclaration constructs a conformance deployment declaration.
func NewDeploymentDeclaration(reference string) (runtimev2.DeploymentDeclaration, error) {
	input, err := declarationInput(reference)
	if err != nil {
		return runtimev2.DeploymentDeclaration{}, err
	}
	return runtimev2.NewDeploymentDeclaration(input)
}
