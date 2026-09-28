package runtimev2_test

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"fmt"
	"strings"
	"testing"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
	"github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/schemaauthor"
)

func TestCanonicalFactoryBuilderBindsSchemaAndProducesKnownIdentity(t *testing.T) {
	schema, err := schemaauthor.NewFactorySchema(schemaauthor.SchemaInput{
		Provider:          "acme",
		SemanticKind:      "database",
		IdentitySchema:    "acme.database.v1",
		ObservationSchema: "acme.database.observation.v1",
		Fields: []schemaauthor.FieldDefinition{
			{Name: "locator", Role: schemaauthor.FieldRoleSemantic, Kind: runtimev2.IdentityFieldText, Required: true, Disclosure: schemaauthor.DisclosurePublic},
			{Name: "plan", Role: schemaauthor.FieldRoleSemantic, Kind: runtimev2.IdentityFieldCanonicalValue, Required: true, Disclosure: schemaauthor.DisclosureProtected},
			{Name: "job_id", Role: schemaauthor.FieldRoleOperational, Kind: runtimev2.IdentityFieldText, Disclosure: schemaauthor.DisclosureProtected},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	declaration := runtimev2.FactoryDeclaration{Kind: "database", Reference: "db", Arguments: []string{}, Attributes: map[string]string{}}
	builder, err := runtimev2.NewFactoryIdentityBuilder(schema, declaration)
	if err != nil {
		t.Fatal(err)
	}
	locator, _ := runtimev2.NewTextIdentityField("locator", "primary")
	planValue, _ := runtimev2.CanonicalMap([]runtimev2.CanonicalMapEntry{{Key: "region", Value: mustCanonicalString(t, "eu")}})
	plan, _ := runtimev2.NewCanonicalValueIdentityField("plan", planValue)
	if err := builder.AddIdentityField(plan); err != nil {
		t.Fatal(err)
	}
	if err := builder.AddIdentityField(locator); err != nil {
		t.Fatal(err)
	}
	observation, err := runtimev2.NewOperationalObservation("acme", "database", "acme.database.observation.v1", map[string]string{"job_id": "volatile-42"})
	if err != nil {
		t.Fatal(err)
	}
	if err := builder.AddObservation(observation); err != nil {
		t.Fatal(err)
	}
	composition, err := builder.Build()
	if err != nil {
		t.Fatal(err)
	}
	identity := composition.Identity()
	got, err := runtimev2.CanonicalFactoryIdentityBytes(identity)
	if err != nil {
		t.Fatal(err)
	}
	expectedFields := testU32(2)
	for _, field := range []runtimev2.IdentityField{locator, plan} {
		rawCommitment, err := hex.DecodeString(string(field.Commitment())[7:])
		if err != nil {
			t.Fatal(err)
		}
		expectedFields = append(expectedFields, testString(field.Name())...)
		expectedFields = append(expectedFields, testU16(testKindTag(field.Kind()))...)
		expectedFields = append(expectedFields, rawCommitment...)
	}
	expected := testRecord(runtimev2.CanonicalFactoryStateDomain, []testField{
		{1, []byte(runtimev2.CanonicalSchemaVersion)}, {2, []byte("acme")}, {3, []byte("database")},
		{4, []byte("acme.database.v1")}, {5, expectedFields},
	})
	if !bytes.Equal(got, expected) {
		t.Fatalf("identity bytes mismatch\n got %x\nwant %x", got, expected)
	}
	fingerprint, err := runtimev2.CanonicalFactoryFingerprint(identity)
	if err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(expected)
	if string(fingerprint) != "sha256:"+hex.EncodeToString(digest[:]) {
		t.Fatalf("unexpected fingerprint %q", fingerprint)
	}
	if runtimev2.CanonicalStateID(fingerprint) != composition.RootStateID() {
		t.Fatal("factory fingerprint and root state ID must match")
	}
	if len(composition.Observations()) != 1 {
		t.Fatal("operational observation was not retained outside identity")
	}
	if err := builder.AddIdentityField(locator); err == nil {
		t.Fatal("successful Build must seal the builder")
	}
}

func TestCanonicalBuilderIsTransactionalAndSchemaStrict(t *testing.T) {
	schema, err := schemaauthor.NewFactorySchema(schemaauthor.SchemaInput{
		Provider: "acme", SemanticKind: "database", IdentitySchema: "acme.database.v1",
		ObservationSchema: "acme.database.observation.v1",
		Fields: []schemaauthor.FieldDefinition{
			{Name: "locator", Role: schemaauthor.FieldRoleSemantic, Kind: runtimev2.IdentityFieldText, Required: true, Disclosure: schemaauthor.DisclosurePublic},
			{Name: "job_id", Role: schemaauthor.FieldRoleOperational, Kind: runtimev2.IdentityFieldText, Disclosure: schemaauthor.DisclosureProtected},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	declaration := runtimev2.FactoryDeclaration{Kind: "database", Reference: "db", Arguments: []string{}, Attributes: map[string]string{}}
	builder, err := runtimev2.NewFactoryIdentityBuilder(schema, declaration)
	if err != nil {
		t.Fatal(err)
	}
	wrongName, _ := runtimev2.NewTextIdentityField("other", "x")
	if err := builder.AddIdentityField(wrongName); err == nil {
		t.Fatal("unknown field accepted")
	}
	wrongKindValue, _ := runtimev2.CanonicalString("x")
	wrongKind, _ := runtimev2.NewCanonicalValueIdentityField("locator", wrongKindValue)
	if err := builder.AddIdentityField(wrongKind); err == nil {
		t.Fatal("wrong field kind accepted")
	}
	if _, err := builder.Build(); err == nil {
		t.Fatal("failed AddIdentityField calls must not satisfy required field")
	}
	locator, _ := runtimev2.NewTextIdentityField("locator", "primary")
	if err := builder.AddIdentityField(locator); err != nil {
		t.Fatal(err)
	}
	if err := builder.AddIdentityField(locator); err == nil {
		t.Fatal("duplicate field accepted")
	}
	if _, err := builder.Build(); err != nil {
		t.Fatal(err)
	}
}

func TestCanonicalBuilderDistinguishesAbsentFromExplicitNull(t *testing.T) {
	schema, err := schemaauthor.NewFactorySchema(schemaauthor.SchemaInput{
		Provider: "acme", SemanticKind: "optional", IdentitySchema: "acme.optional.v1",
		Fields: []schemaauthor.FieldDefinition{{Name: "value", Role: schemaauthor.FieldRoleSemantic, Kind: runtimev2.IdentityFieldCanonicalValue, Disclosure: schemaauthor.DisclosurePublic}},
	})
	if err != nil {
		t.Fatal(err)
	}
	declaration := runtimev2.FactoryDeclaration{Kind: "optional", Reference: "x", Arguments: []string{}, Attributes: map[string]string{}}
	absentBuilder, _ := runtimev2.NewFactoryIdentityBuilder(schema, declaration)
	absent, err := absentBuilder.Build()
	if err != nil {
		t.Fatal(err)
	}
	nullBuilder, _ := runtimev2.NewFactoryIdentityBuilder(schema, declaration)
	nullField, _ := runtimev2.NewCanonicalValueIdentityField("value", runtimev2.CanonicalNull())
	if err := nullBuilder.AddIdentityField(nullField); err != nil {
		t.Fatal(err)
	}
	explicitNull, err := nullBuilder.Build()
	if err != nil {
		t.Fatal(err)
	}
	absentFingerprint, _ := runtimev2.CanonicalFactoryFingerprint(absent.Identity())
	nullFingerprint, _ := runtimev2.CanonicalFactoryFingerprint(explicitNull.Identity())
	if absentFingerprint == nullFingerprint {
		t.Fatal("absent optional field collapsed into explicit canonical null")
	}
}

// TestCanonicalSecretCanaryDoesNotReachReturnedSinks covers EX03 with a
// per-run value that is never checked into fixtures.
func TestCanonicalSecretCanaryDoesNotReachReturnedSinks(t *testing.T) {
	random := make([]byte, 24)
	if _, err := rand.Read(random); err != nil {
		t.Fatal(err)
	}
	canary := "secret-canary-" + hex.EncodeToString(random)
	schema, err := schemaauthor.NewFactorySchema(schemaauthor.SchemaInput{
		Provider: "acme", SemanticKind: "secret-test", IdentitySchema: "acme.secret-test.v1",
		Fields: []schemaauthor.FieldDefinition{{Name: "credential", Role: schemaauthor.FieldRoleSemantic, Kind: runtimev2.IdentityFieldSecretReference, Required: true, Disclosure: schemaauthor.DisclosureProtected}},
	})
	if err != nil {
		t.Fatal(err)
	}
	declaration := runtimev2.FactoryDeclaration{Kind: "secret-test", Reference: "x", Arguments: []string{}, Attributes: map[string]string{}}
	wrongBuilder, _ := runtimev2.NewFactoryIdentityBuilder(schema, declaration)
	wrongKind, _ := runtimev2.NewTextIdentityField("credential", canary)
	wrongErr := wrongBuilder.AddIdentityField(wrongKind)
	if wrongErr == nil || strings.Contains(fmt.Sprintf("%v %+v", wrongErr, wrongErr), canary) {
		t.Fatalf("wrong-kind secret handling leaked or accepted canary: %v", wrongErr)
	}

	builder, _ := runtimev2.NewFactoryIdentityBuilder(schema, declaration)
	reference, _ := runtimev2.NewSecretReference("vault", "opaque-id", "rotation-1")
	credential, _ := runtimev2.NewSecretReferenceIdentityField("credential", reference)
	_ = builder.AddIdentityField(credential)
	composition, _ := builder.Build()
	envelope, _ := runtimev2.NewFactoryEnvelope(composition.Identity())
	explanation, _ := runtimev2.ExplainSafe(envelope)
	for name, value := range map[string]any{"envelope": envelope, "safe explanation": explanation} {
		raw := fmt.Sprintf("%v %+v", value, value)
		if strings.Contains(raw, canary) {
			t.Fatalf("%s leaked canary", name)
		}
	}
	bundle, err := runtimev2.LoadConformanceBundle()
	if err != nil {
		t.Fatal(err)
	}
	for name, raw := range bundle.Files() {
		if bytes.Contains(raw, []byte(canary)) {
			t.Fatalf("bundle file %s leaked runtime canary", name)
		}
	}
}

func TestSchemaAuthorRejectsInvalidDefinitionsAndCopiesInputs(t *testing.T) {
	base := schemaauthor.SchemaInput{
		Provider: "acme", SemanticKind: "database", IdentitySchema: "acme.database.v1",
		ObservationSchema: "acme.database.observation.v1",
		Fields:            []schemaauthor.FieldDefinition{{Name: "locator", Role: schemaauthor.FieldRoleSemantic, Kind: runtimev2.IdentityFieldText, Required: true, Disclosure: schemaauthor.DisclosurePublic}},
	}
	schema, err := schemaauthor.NewFactorySchema(base)
	if err != nil {
		t.Fatal(err)
	}
	base.Fields[0].Name = "mutated"
	if schema.Fields()[0].Name() != "locator" {
		t.Fatal("schema retained caller-owned field definition")
	}
	invalid := []schemaauthor.SchemaInput{
		{Provider: "Bad", SemanticKind: "database", IdentitySchema: "v1", ObservationSchema: "obs.v1"},
		{Provider: "acme", SemanticKind: "database", IdentitySchema: "v1", ObservationSchema: "obs.v1", Fields: []schemaauthor.FieldDefinition{{Name: "x", Role: "unknown", Kind: runtimev2.IdentityFieldText, Disclosure: schemaauthor.DisclosurePublic}}},
		{Provider: "acme", SemanticKind: "database", IdentitySchema: "v1", ObservationSchema: "obs.v1", Fields: []schemaauthor.FieldDefinition{{Name: "x", Role: schemaauthor.FieldRoleOperational, Kind: runtimev2.IdentityFieldCanonicalValue, Required: true, Disclosure: schemaauthor.DisclosurePublic}}},
		{Provider: "acme", SemanticKind: "database", IdentitySchema: "v1", ObservationSchema: "obs.v1", Fields: []schemaauthor.FieldDefinition{{Name: "x", Role: schemaauthor.FieldRoleSemantic, Kind: runtimev2.IdentityFieldText, Disclosure: schemaauthor.DisclosurePublic}, {Name: "x", Role: schemaauthor.FieldRoleSemantic, Kind: runtimev2.IdentityFieldText, Disclosure: schemaauthor.DisclosurePublic}}},
	}
	for index, input := range invalid {
		if _, err := schemaauthor.NewFactorySchema(input); err == nil {
			t.Fatalf("invalid schema %d accepted", index)
		}
	}
}

func TestCanonicalExtensionBindingEnforcesRolePositionOwnerAndKind(t *testing.T) {
	extensionSchema, err := schemaauthor.NewExtensionSchema(schemaauthor.SchemaInput{
		Provider: "storage", SemanticKind: "bucket", IdentitySchema: "storage.bucket.v1",
		Fields: []schemaauthor.FieldDefinition{{Name: "name", Role: schemaauthor.FieldRoleSemantic, Kind: runtimev2.IdentityFieldText, Required: true, Disclosure: schemaauthor.DisclosurePublic}},
	})
	if err != nil {
		t.Fatal(err)
	}
	extensionBuilder, _ := runtimev2.NewExtensionIdentityBuilder(extensionSchema)
	name, _ := runtimev2.NewTextIdentityField("name", "assets")
	if err := extensionBuilder.AddIdentityField(name); err != nil {
		t.Fatal(err)
	}
	extension, err := extensionBuilder.Build()
	if err != nil {
		t.Fatal(err)
	}
	inputDeclaration, err := runtimev2.NewInputDeclaration(runtimev2.ExtensionSpecificationInput{
		SchemaVersion: runtimev2.SchemaVersion, Owner: "storage", Kind: "bucket", SpecificationSchema: "storage.bucket.spec.v1", Fields: []runtimev2.DeclarationField{},
	})
	if err != nil {
		t.Fatal(err)
	}
	factorySchema, _ := schemaauthor.NewFactorySchema(schemaauthor.SchemaInput{
		Provider: "acme", SemanticKind: "database", IdentitySchema: "acme.database.v1",
		Fields: []schemaauthor.FieldDefinition{{Name: "locator", Role: schemaauthor.FieldRoleSemantic, Kind: runtimev2.IdentityFieldText, Required: true, Disclosure: schemaauthor.DisclosurePublic}},
	})
	declaration := runtimev2.FactoryDeclaration{Kind: "database", Reference: "db", Arguments: []string{}, Attributes: map[string]string{}, Inputs: []runtimev2.InputDeclaration{inputDeclaration}}
	builder, _ := runtimev2.NewFactoryIdentityBuilder(factorySchema, declaration)
	locator, _ := runtimev2.NewTextIdentityField("locator", "primary")
	if err := builder.AddIdentityField(locator); err != nil {
		t.Fatal(err)
	}
	if _, err := builder.Build(); err == nil {
		t.Fatal("missing required extension binding accepted")
	}
	if err := builder.BindExtensions(runtimev2.ResolvedFactoryExtensions{Inputs: []runtimev2.CanonicalResolvedExtensionIdentity{extension.Identity()}}); err != nil {
		t.Fatal(err)
	}
	composition, err := builder.Build()
	if err != nil {
		t.Fatal(err)
	}
	encoded, _ := runtimev2.CanonicalFactoryIdentityBytes(composition.Identity())
	if !bytes.Contains(encoded, []byte("extension.input.0")) {
		t.Fatal("extension contribution missing from identity")
	}
}

func TestCanonicalTransformAndExtensionBuilderChannels(t *testing.T) {
	transformSchema, err := schemaauthor.NewTransformSchema(schemaauthor.SchemaInput{
		Provider: "acme", SemanticKind: "migration", IdentitySchema: "acme.migration.v1", ObservationSchema: "acme.migration.observation.v1",
		Fields: []schemaauthor.FieldDefinition{
			{Name: "script", Role: schemaauthor.FieldRoleSemantic, Kind: runtimev2.IdentityFieldText, Required: true, Disclosure: schemaauthor.DisclosurePublic},
			{Name: "job_id", Role: schemaauthor.FieldRoleOperational, Kind: runtimev2.IdentityFieldText, Disclosure: schemaauthor.DisclosureProtected},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtimev2.NewTransformIdentityBuilder(runtimev2.TransformIdentitySchema{}, runtimev2.TransformDeclaration{}); err == nil {
		t.Fatal("zero transform schema accepted")
	}
	if _, err := runtimev2.NewTransformIdentityBuilder(transformSchema, runtimev2.TransformDeclaration{Kind: "wrong", Reference: "x", Arguments: []string{}, Attributes: map[string]string{}}); err == nil {
		t.Fatal("wrong transform kind accepted")
	}
	builder, err := runtimev2.NewTransformIdentityBuilder(transformSchema, runtimev2.TransformDeclaration{Kind: "migration", Reference: "x", Arguments: []string{}, Attributes: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	script, _ := runtimev2.NewTextIdentityField("script", "select 1")
	if err := builder.AddIdentityField(script); err != nil {
		t.Fatal(err)
	}
	badObservation, _ := runtimev2.NewOperationalObservation("other", "migration", "acme.migration.observation.v1", map[string]string{"job_id": "1"})
	if err := builder.AddObservation(badObservation); err == nil {
		t.Fatal("mismatched observation accepted")
	}
	observation, _ := runtimev2.NewOperationalObservation("acme", "migration", "acme.migration.observation.v1", map[string]string{"job_id": "1"})
	if err := builder.AddObservation(observation); err != nil {
		t.Fatal(err)
	}
	composition, err := builder.Build()
	if err != nil {
		t.Fatal(err)
	}
	if len(composition.Observations()) != 1 || composition.Identity().Provider() != "acme" || composition.Identity().Kind() != "migration" || composition.Identity().IdentitySchema() != "acme.migration.v1" {
		t.Fatal("transform composition accessors")
	}
	if _, err := runtimev2.CanonicalTransformIdentityBytes(composition.Identity()); err != nil {
		t.Fatal(err)
	}
	if _, err := runtimev2.CanonicalTransformFingerprint(composition.Identity()); err != nil {
		t.Fatal(err)
	}

	extensionSchema, _ := schemaauthor.NewExtensionSchema(schemaauthor.SchemaInput{Provider: "storage", SemanticKind: "bucket", IdentitySchema: "storage.bucket.v1", ObservationSchema: "storage.bucket.observation.v1", Fields: []schemaauthor.FieldDefinition{
		{Name: "name", Role: schemaauthor.FieldRoleSemantic, Kind: runtimev2.IdentityFieldText, Required: true, Disclosure: schemaauthor.DisclosurePublic},
		{Name: "checkpoint", Role: schemaauthor.FieldRoleOperational, Kind: runtimev2.IdentityFieldText, Disclosure: schemaauthor.DisclosureProtected},
	}})
	extensionBuilder, _ := runtimev2.NewExtensionIdentityBuilder(extensionSchema)
	name, _ := runtimev2.NewTextIdentityField("name", "assets")
	_ = extensionBuilder.AddIdentityField(name)
	extensionObservation, _ := runtimev2.NewOperationalObservation("storage", "bucket", "storage.bucket.observation.v1", map[string]string{"checkpoint": "volatile"})
	if err := extensionBuilder.AddObservation(extensionObservation); err != nil {
		t.Fatal(err)
	}
	extension, err := extensionBuilder.Build()
	if err != nil {
		t.Fatal(err)
	}
	if len(extension.Observations()) != 1 {
		t.Fatal("extension observation missing")
	}
	if _, err := runtimev2.CanonicalResolvedExtensionBytes(extension.Identity()); err != nil {
		t.Fatal(err)
	}
}

func TestCanonicalBuilderInvalidInputsAndNilReceivers(t *testing.T) {
	if _, err := runtimev2.NewFactoryIdentityBuilder(runtimev2.FactoryIdentitySchema{}, runtimev2.FactoryDeclaration{}); err == nil {
		t.Fatal("zero factory schema accepted")
	}
	var factory *runtimev2.FactoryIdentityBuilder
	field, _ := runtimev2.NewTextIdentityField("x", "x")
	if factory.AddIdentityField(field) == nil || factory.AddObservation(runtimev2.OperationalObservation{}) == nil || factory.BindExtensions(runtimev2.ResolvedFactoryExtensions{}) == nil {
		t.Fatal("nil factory builder accepted call")
	}
	if _, err := factory.Build(); err == nil {
		t.Fatal("nil factory builder built")
	}
	var transform *runtimev2.TransformIdentityBuilder
	if transform.AddIdentityField(field) == nil || transform.AddObservation(runtimev2.OperationalObservation{}) == nil || transform.BindExtensions(runtimev2.ResolvedTransformExtensions{}) == nil {
		t.Fatal("nil transform builder accepted call")
	}
	if _, err := transform.Build(); err == nil {
		t.Fatal("nil transform builder built")
	}
	var extension *runtimev2.ExtensionIdentityBuilder
	if extension.AddIdentityField(field) == nil || extension.AddObservation(runtimev2.OperationalObservation{}) == nil {
		t.Fatal("nil extension builder accepted call")
	}
	if _, err := extension.Build(); err == nil {
		t.Fatal("nil extension builder built")
	}
	if _, err := runtimev2.NewOperationalObservation("Bad", "kind", "schema", nil); err == nil {
		t.Fatal("invalid observation owner accepted")
	}
	if _, err := runtimev2.NewOperationalObservation("owner", "Bad", "schema", nil); err == nil {
		t.Fatal("invalid observation kind accepted")
	}
	if _, err := runtimev2.NewOperationalObservation("owner", "kind", "Bad", nil); err == nil {
		t.Fatal("invalid observation schema accepted")
	}
	if _, err := runtimev2.NewOperationalObservation("owner", "kind", "schema", map[string]string{"bad name": "x"}); err == nil {
		t.Fatal("invalid observation field accepted")
	}
	if _, err := runtimev2.NewOperationalObservation("owner", "kind", "schema", map[string]string{"value": string([]byte{0xff})}); err == nil {
		t.Fatal("invalid observation value accepted")
	}
}

func TestCanonicalExtensionRoleCompletenessMatrix(t *testing.T) {
	makeExtension := func(owner, kind, name string) runtimev2.CanonicalResolvedExtensionIdentity {
		schema, err := schemaauthor.NewExtensionSchema(schemaauthor.SchemaInput{Provider: owner, SemanticKind: kind, IdentitySchema: owner + "." + kind + ".v1", Fields: []schemaauthor.FieldDefinition{{Name: "name", Role: schemaauthor.FieldRoleSemantic, Kind: runtimev2.IdentityFieldText, Required: true, Disclosure: schemaauthor.DisclosurePublic}}})
		if err != nil {
			t.Fatal(err)
		}
		builder, _ := runtimev2.NewExtensionIdentityBuilder(schema)
		field, _ := runtimev2.NewTextIdentityField("name", name)
		_ = builder.AddIdentityField(field)
		composition, err := builder.Build()
		if err != nil {
			t.Fatal(err)
		}
		return composition.Identity()
	}
	input := makeExtension("storage", "input", "source")
	environment := makeExtension("runtime", "environment", "linux")
	deployment := makeExtension("runtime", "deployment", "postgres")
	wrong := makeExtension("wrong", "input", "source")
	declare := func(owner, kind string) runtimev2.ExtensionSpecificationInput {
		return runtimev2.ExtensionSpecificationInput{SchemaVersion: runtimev2.SchemaVersion, Owner: owner, Kind: kind, SpecificationSchema: owner + "." + kind + ".spec.v1", Fields: []runtimev2.DeclarationField{}}
	}
	inputDecl, _ := runtimev2.NewInputDeclaration(declare("storage", "input"))
	envDecl, _ := runtimev2.NewExecutionEnvironmentDeclaration(declare("runtime", "environment"))
	deploymentDecl, _ := runtimev2.NewDeploymentDeclaration(declare("runtime", "deployment"))
	factorySchema, _ := schemaauthor.NewFactorySchema(schemaauthor.SchemaInput{Provider: "acme", SemanticKind: "database", IdentitySchema: "acme.database.v1", Fields: []schemaauthor.FieldDefinition{{Name: "locator", Role: schemaauthor.FieldRoleSemantic, Kind: runtimev2.IdentityFieldText, Required: true, Disclosure: schemaauthor.DisclosurePublic}}})
	declaration := runtimev2.FactoryDeclaration{Kind: "database", Reference: "db", Arguments: []string{}, Attributes: map[string]string{}, Inputs: []runtimev2.InputDeclaration{inputDecl}, ExecutionEnvironment: &envDecl, Deployment: &deploymentDecl}
	newFactory := func() *runtimev2.FactoryIdentityBuilder {
		builder, err := runtimev2.NewFactoryIdentityBuilder(factorySchema, declaration)
		if err != nil {
			t.Fatal(err)
		}
		locator, _ := runtimev2.NewTextIdentityField("locator", "db")
		_ = builder.AddIdentityField(locator)
		return builder
	}
	builder := newFactory()
	if err := builder.BindExtensions(runtimev2.ResolvedFactoryExtensions{}); err == nil {
		t.Fatal("missing input accepted")
	}
	if err := builder.BindExtensions(runtimev2.ResolvedFactoryExtensions{Inputs: []runtimev2.CanonicalResolvedExtensionIdentity{wrong}, ExecutionEnvironment: &environment, Deployment: &deployment}); err == nil {
		t.Fatal("wrong owner accepted")
	}
	if err := builder.BindExtensions(runtimev2.ResolvedFactoryExtensions{Inputs: []runtimev2.CanonicalResolvedExtensionIdentity{input}, Deployment: &deployment}); err == nil {
		t.Fatal("missing environment accepted")
	}
	if err := builder.BindExtensions(runtimev2.ResolvedFactoryExtensions{Inputs: []runtimev2.CanonicalResolvedExtensionIdentity{input}, ExecutionEnvironment: &environment}); err == nil {
		t.Fatal("missing deployment accepted")
	}
	if err := builder.BindExtensions(runtimev2.ResolvedFactoryExtensions{Inputs: []runtimev2.CanonicalResolvedExtensionIdentity{input}, ExecutionEnvironment: &environment, Deployment: &deployment}); err != nil {
		t.Fatal(err)
	}
	if err := builder.BindExtensions(runtimev2.ResolvedFactoryExtensions{}); err == nil {
		t.Fatal("second binding accepted")
	}
	if _, err := builder.Build(); err != nil {
		t.Fatal(err)
	}

	transformSchema, _ := schemaauthor.NewTransformSchema(schemaauthor.SchemaInput{Provider: "acme", SemanticKind: "migration", IdentitySchema: "acme.migration.v1", Fields: []schemaauthor.FieldDefinition{{Name: "script", Role: schemaauthor.FieldRoleSemantic, Kind: runtimev2.IdentityFieldText, Required: true, Disclosure: schemaauthor.DisclosurePublic}}})
	transformDecl := runtimev2.TransformDeclaration{Kind: "migration", Reference: "x", Arguments: []string{}, Attributes: map[string]string{}, Inputs: []runtimev2.InputDeclaration{inputDecl}, ExecutionEnvironment: &envDecl}
	newTransform := func() *runtimev2.TransformIdentityBuilder {
		b, err := runtimev2.NewTransformIdentityBuilder(transformSchema, transformDecl)
		if err != nil {
			t.Fatal(err)
		}
		script, _ := runtimev2.NewTextIdentityField("script", "x")
		_ = b.AddIdentityField(script)
		return b
	}
	transform := newTransform()
	if _, err := transform.Build(); err == nil {
		t.Fatal("unbound transform built")
	}
	if err := transform.BindExtensions(runtimev2.ResolvedTransformExtensions{Inputs: []runtimev2.CanonicalResolvedExtensionIdentity{input}, ExecutionEnvironment: &environment}); err != nil {
		t.Fatal(err)
	}
	if _, err := transform.Build(); err != nil {
		t.Fatal(err)
	}
	if _, err := transform.Build(); err == nil {
		t.Fatal("sealed transform rebuilt")
	}
}

func TestCanonicalBuilderRemainingTransactionalFailures(t *testing.T) {
	schema, _ := schemaauthor.NewFactorySchema(schemaauthor.SchemaInput{Provider: "acme", SemanticKind: "database", IdentitySchema: "acme.database.v1", ObservationSchema: "acme.database.observation.v1", Fields: []schemaauthor.FieldDefinition{{Name: "locator", Role: schemaauthor.FieldRoleSemantic, Kind: runtimev2.IdentityFieldText, Required: true, Disclosure: schemaauthor.DisclosurePublic}, {Name: "job_id", Role: schemaauthor.FieldRoleOperational, Kind: runtimev2.IdentityFieldText, Disclosure: schemaauthor.DisclosureProtected}}})
	if _, err := runtimev2.NewFactoryIdentityBuilder(schema, runtimev2.FactoryDeclaration{}); err == nil {
		t.Fatal("invalid declaration accepted")
	}
	builder, _ := runtimev2.NewFactoryIdentityBuilder(schema, runtimev2.FactoryDeclaration{Kind: "database", Reference: "db", Arguments: []string{}, Attributes: map[string]string{}})
	if err := builder.AddIdentityField(runtimev2.IdentityField{}); err == nil {
		t.Fatal("zero identity field accepted")
	}
	if err := builder.AddObservation(runtimev2.OperationalObservation{}); err == nil {
		t.Fatal("zero observation accepted")
	}
	unknown, _ := runtimev2.NewOperationalObservation("acme", "database", "acme.database.observation.v1", map[string]string{"unknown": "x"})
	if err := builder.AddObservation(unknown); err == nil {
		t.Fatal("unknown observation field accepted")
	}
	locator, _ := runtimev2.NewTextIdentityField("locator", "db")
	_ = builder.AddIdentityField(locator)
	if _, err := builder.Build(); err != nil {
		t.Fatal(err)
	}
	if _, err := builder.Build(); err == nil {
		t.Fatal("sealed factory rebuilt")
	}
	if _, err := runtimev2.NewExtensionIdentityBuilder(runtimev2.ExtensionIdentitySchema{}); err == nil {
		t.Fatal("zero extension schema accepted")
	}

	inputSchema, _ := schemaauthor.NewExtensionSchema(schemaauthor.SchemaInput{Provider: "storage", SemanticKind: "input", IdentitySchema: "storage.input.v1", Fields: []schemaauthor.FieldDefinition{{Name: "name", Role: schemaauthor.FieldRoleSemantic, Kind: runtimev2.IdentityFieldText, Required: true, Disclosure: schemaauthor.DisclosurePublic}}})
	extensionBuilder, _ := runtimev2.NewExtensionIdentityBuilder(inputSchema)
	name, _ := runtimev2.NewTextIdentityField("name", "x")
	_ = extensionBuilder.AddIdentityField(name)
	extension, _ := extensionBuilder.Build()
	inputDecl, _ := runtimev2.NewInputDeclaration(runtimev2.ExtensionSpecificationInput{SchemaVersion: runtimev2.SchemaVersion, Owner: "storage", Kind: "input", SpecificationSchema: "storage.input.spec.v1", Fields: []runtimev2.DeclarationField{}})
	transformSchema, _ := schemaauthor.NewTransformSchema(schemaauthor.SchemaInput{Provider: "acme", SemanticKind: "migration", IdentitySchema: "acme.migration.v1", Fields: []schemaauthor.FieldDefinition{{Name: "script", Role: schemaauthor.FieldRoleSemantic, Kind: runtimev2.IdentityFieldText, Required: true, Disclosure: schemaauthor.DisclosurePublic}}})
	newTransform := func() *runtimev2.TransformIdentityBuilder {
		b, _ := runtimev2.NewTransformIdentityBuilder(transformSchema, runtimev2.TransformDeclaration{Kind: "migration", Reference: "x", Arguments: []string{}, Attributes: map[string]string{}, Inputs: []runtimev2.InputDeclaration{inputDecl}})
		script, _ := runtimev2.NewTextIdentityField("script", "x")
		_ = b.AddIdentityField(script)
		return b
	}
	transform := newTransform()
	if err := transform.BindExtensions(runtimev2.ResolvedTransformExtensions{}); err == nil {
		t.Fatal("missing transform input accepted")
	}
	if err := transform.BindExtensions(runtimev2.ResolvedTransformExtensions{Inputs: []runtimev2.CanonicalResolvedExtensionIdentity{extension.Identity()}}); err != nil {
		t.Fatal(err)
	}
	if err := transform.BindExtensions(runtimev2.ResolvedTransformExtensions{}); err == nil {
		t.Fatal("second transform binding accepted")
	}
}

func mustCanonicalString(t *testing.T, value string) runtimev2.CanonicalValue {
	t.Helper()
	result, err := runtimev2.CanonicalString(value)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

type testField struct {
	tag     uint16
	payload []byte
}

func testKindTag(kind runtimev2.IdentityFieldKind) uint16 {
	switch kind {
	case runtimev2.IdentityFieldText:
		return 1
	case runtimev2.IdentityFieldCanonicalValue:
		return 2
	case runtimev2.IdentityFieldSecretReference:
		return 3
	default:
		return 0
	}
}

func testRecord(domain string, fields []testField) []byte {
	result := testString(domain)
	result = append(result, testU32(uint32(len(fields)))...)
	for _, field := range fields {
		result = append(result, testU16(field.tag)...)
		result = append(result, testBytes(field.payload)...)
	}
	return result
}

func testString(value string) []byte { return testBytes([]byte(value)) }
func testBytes(value []byte) []byte {
	result := make([]byte, 8, 8+len(value))
	binary.BigEndian.PutUint64(result, uint64(len(value)))
	return append(result, value...)
}
func testU16(value uint16) []byte {
	result := make([]byte, 2)
	binary.BigEndian.PutUint16(result, value)
	return result
}
func testU32(value uint32) []byte {
	result := make([]byte, 4)
	binary.BigEndian.PutUint32(result, value)
	return result
}
