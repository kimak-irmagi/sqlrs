package conformancev1_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
	"github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/schemas/conformancev1"
)

type identityFieldAdder interface {
	AddIdentityField(runtimev2.IdentityField) error
	AddObservation(runtimev2.OperationalObservation) error
}

func requireValidation(t *testing.T, err error, code runtimev2.ValidationCode, path string) {
	t.Helper()
	if !errors.Is(err, runtimev2.ErrInvalid) {
		t.Fatalf("error %v does not match ErrInvalid", err)
	}
	var validation *runtimev2.ValidationError
	if !errors.As(err, &validation) || validation.Code != code || validation.Path != path {
		t.Fatalf("validation = %#v, want %s at %s", validation, code, path)
	}
}

func addAllIdentityFields(t *testing.T, builder identityFieldAdder, locator, version string) {
	t.Helper()
	text, err := runtimev2.NewTextIdentityField(conformancev1.FieldLocator, locator)
	if err != nil {
		t.Fatal(err)
	}
	value, err := runtimev2.CanonicalString("private-plan")
	if err != nil {
		t.Fatal(err)
	}
	plan, err := runtimev2.NewCanonicalValueIdentityField(conformancev1.FieldPlan, value)
	if err != nil {
		t.Fatal(err)
	}
	reference, err := runtimev2.NewSecretReference("vault", "secret-id", version)
	if err != nil {
		t.Fatal(err)
	}
	credential, err := runtimev2.NewSecretReferenceIdentityField(conformancev1.FieldCredential, reference)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []runtimev2.IdentityField{text, plan, credential} {
		if err := builder.AddIdentityField(field); err != nil {
			t.Fatal(err)
		}
	}
}

func factoryDeclaration() runtimev2.FactoryDeclaration {
	return runtimev2.FactoryDeclaration{Kind: conformancev1.FactoryKind, Reference: "factory", Arguments: []string{}, Attributes: map[string]string{}}
}

func transformDeclaration() runtimev2.TransformDeclaration {
	return runtimev2.TransformDeclaration{Kind: conformancev1.TransformKind, Reference: "transform", Arguments: []string{}, Attributes: map[string]string{}}
}

func buildExtension(t *testing.T, locator, version string) runtimev2.CanonicalResolvedExtensionIdentity {
	t.Helper()
	builder, err := conformancev1.NewExtensionBuilder()
	if err != nil {
		t.Fatal(err)
	}
	addAllIdentityFields(t, builder, locator, version)
	composition, err := builder.Build()
	if err != nil {
		t.Fatal(err)
	}
	return composition.Identity()
}

func TestConstantsAndRoleSpecificBuilders(t *testing.T) {
	if conformancev1.Provider != "runtime-conformance" ||
		conformancev1.FactoryKind != "fixture-factory" ||
		conformancev1.TransformKind != "fixture-transform" ||
		conformancev1.ExtensionKind != "fixture-extension" ||
		conformancev1.FactoryIdentitySchema != "runtime-conformance.factory.v1" ||
		conformancev1.TransformIdentitySchema != "runtime-conformance.transform.v1" ||
		conformancev1.ExtensionIdentitySchema != "runtime-conformance.extension.v1" ||
		conformancev1.FactoryObservationSchema != "runtime-conformance.factory-observation.v1" ||
		conformancev1.TransformObservationSchema != "runtime-conformance.transform-observation.v1" ||
		conformancev1.ExtensionObservationSchema != "runtime-conformance.extension-observation.v1" ||
		conformancev1.ExtensionSpecificationSchema != "runtime-conformance.extension-specification.v1" {
		t.Fatal("schema constants changed")
	}
	if conformancev1.FieldLocator != "locator" || conformancev1.FieldPlan != "plan" ||
		conformancev1.FieldCredential != "credential" || conformancev1.DeclarationReference != "reference" {
		t.Fatal("field constants changed")
	}
	observations := []string{
		conformancev1.ObservationJobID, conformancev1.ObservationContainerID,
		conformancev1.ObservationTimestamp, conformancev1.ObservationPhysicalSize,
		conformancev1.ObservationMaterializationPath, conformancev1.ObservationCheckpointBackend,
	}
	if strings.Join(observations, ",") != "job_id,container_id,timestamp,physical_size,materialization_path,checkpoint_backend" {
		t.Fatal("observation constants changed")
	}

	factory, err := conformancev1.NewFactoryBuilder(factoryDeclaration())
	if err != nil {
		t.Fatal(err)
	}
	addAllIdentityFields(t, factory, "factory-locator", "v1")
	factoryResult, err := factory.Build()
	if err != nil {
		t.Fatal(err)
	}
	if identity := factoryResult.Identity(); identity.Provider() != conformancev1.Provider || identity.Kind() != conformancev1.FactoryKind || identity.IdentitySchema() != conformancev1.FactoryIdentitySchema {
		t.Fatalf("factory metadata = %q/%q/%q", identity.Provider(), identity.Kind(), identity.IdentitySchema())
	}

	transform, err := conformancev1.NewTransformBuilder(transformDeclaration())
	if err != nil {
		t.Fatal(err)
	}
	addAllIdentityFields(t, transform, "transform-locator", "v1")
	transformResult, err := transform.Build()
	if err != nil {
		t.Fatal(err)
	}
	if identity := transformResult.Identity(); identity.Provider() != conformancev1.Provider || identity.Kind() != conformancev1.TransformKind || identity.IdentitySchema() != conformancev1.TransformIdentitySchema {
		t.Fatalf("transform metadata = %q/%q/%q", identity.Provider(), identity.Kind(), identity.IdentitySchema())
	}

	extension := buildExtension(t, "extension-locator", "v1")
	envelope, err := runtimev2.NewExtensionEnvelope(extension)
	if err != nil {
		t.Fatal(err)
	}
	descriptor := envelope.Descriptor()
	if descriptor.Provider() != conformancev1.Provider || descriptor.SemanticKind() != conformancev1.ExtensionKind || descriptor.IdentitySchema() != conformancev1.ExtensionIdentitySchema {
		t.Fatalf("extension metadata = %q/%q/%q", descriptor.Provider(), descriptor.SemanticKind(), descriptor.IdentitySchema())
	}
}

func TestBuilderFailuresAreTransactional(t *testing.T) {
	wrong := factoryDeclaration()
	wrong.Kind = "other"
	builder, err := conformancev1.NewFactoryBuilder(wrong)
	if builder != nil {
		t.Fatal("wrong-kind constructor returned a builder")
	}
	requireValidation(t, err, runtimev2.CodeValueInvalid, "declaration.kind")

	builder, err = conformancev1.NewFactoryBuilder(factoryDeclaration())
	if err != nil {
		t.Fatal(err)
	}
	if _, err := builder.Build(); err == nil {
		t.Fatal("builder accepted missing locator")
	} else {
		requireValidation(t, err, runtimev2.CodeShapeInvalid, "fields.locator")
	}
	wrongField, _ := runtimev2.NewTextIdentityField("other", "value")
	if err := builder.AddIdentityField(wrongField); err == nil {
		t.Fatal("unknown field accepted")
	}
	locator, _ := runtimev2.NewTextIdentityField(conformancev1.FieldLocator, "locator")
	if err := builder.AddIdentityField(locator); err != nil {
		t.Fatalf("builder was corrupted by rejection: %v", err)
	}
	crossRole, _ := conformancev1.NewTransformObservation(map[string]string{conformancev1.ObservationJobID: "job"})
	if err := builder.AddObservation(crossRole); err == nil {
		t.Fatal("cross-role observation accepted")
	}
	valid, _ := conformancev1.NewFactoryObservation(nil)
	if err := builder.AddObservation(valid); err != nil {
		t.Fatalf("builder was corrupted by observation rejection: %v", err)
	}
	if _, err := builder.Build(); err != nil {
		t.Fatal(err)
	}
	if _, err := builder.Build(); err == nil {
		t.Fatal("sealed builder reused")
	}
}

func TestObservationsAreDeterministicIsolatedAndDefensive(t *testing.T) {
	all := map[string]string{
		conformancev1.ObservationJobID: "", conformancev1.ObservationContainerID: "container",
		conformancev1.ObservationTimestamp: "time", conformancev1.ObservationPhysicalSize: "size",
		conformancev1.ObservationMaterializationPath: "path", conformancev1.ObservationCheckpointBackend: "backend",
	}
	for name, constructor := range map[string]func(map[string]string) (runtimev2.OperationalObservation, error){
		"factory":   conformancev1.NewFactoryObservation,
		"transform": conformancev1.NewTransformObservation,
		"extension": conformancev1.NewExtensionObservation,
	} {
		if _, err := constructor(nil); err != nil {
			t.Fatalf("%s nil observation: %v", name, err)
		}
		if _, err := constructor(map[string]string{}); err != nil {
			t.Fatalf("%s empty observation: %v", name, err)
		}
		if _, err := constructor(all); err != nil {
			t.Fatalf("%s complete observation: %v", name, err)
		}
	}

	build := func(value string) (runtimev2.CanonicalFingerprint, []runtimev2.OperationalObservation) {
		builder, err := conformancev1.NewFactoryBuilder(factoryDeclaration())
		if err != nil {
			t.Fatal(err)
		}
		locator, _ := runtimev2.NewTextIdentityField(conformancev1.FieldLocator, "same")
		_ = builder.AddIdentityField(locator)
		fields := map[string]string{conformancev1.ObservationJobID: value}
		observation, err := conformancev1.NewFactoryObservation(fields)
		if err != nil {
			t.Fatal(err)
		}
		fields["unknown"] = "mutation"
		if err := builder.AddObservation(observation); err != nil {
			t.Fatalf("observation retained caller map: %v", err)
		}
		result, err := builder.Build()
		if err != nil {
			t.Fatal(err)
		}
		first := result.Observations()
		first[0] = runtimev2.OperationalObservation{}
		if result.Observations()[0] == (runtimev2.OperationalObservation{}) {
			t.Fatal("composition exposed a mutable observation slice")
		}
		fingerprint, err := runtimev2.CanonicalFactoryFingerprint(result.Identity())
		if err != nil {
			t.Fatal(err)
		}
		return fingerprint, result.Observations()
	}
	one, observations := build("one")
	two, _ := build("two")
	if one != two {
		t.Fatal("observation changed canonical identity")
	}
	if len(observations) != 1 {
		t.Fatalf("observations = %d", len(observations))
	}
	observations[0] = runtimev2.OperationalObservation{}
	_, fresh := build("one")
	if len(fresh) != 1 || fresh[0] == (runtimev2.OperationalObservation{}) {
		t.Fatal("returned observation slice was not defensive")
	}
}

func TestObservationValidationOrderLimitsAndSafety(t *testing.T) {
	zero, err := conformancev1.NewFactoryObservation(map[string]string{"z_unknown": "", "a_unknown": ""})
	if zero != (runtimev2.OperationalObservation{}) {
		t.Fatal("unknown name returned a non-zero observation")
	}
	requireValidation(t, err, runtimev2.CodeUnknownMember, "fields.a_unknown")

	unsafe := []string{"bad name", "BAD", "line\nbreak", string([]byte{0xff})}
	for _, name := range unsafe {
		zero, err = conformancev1.NewFactoryObservation(map[string]string{name: "do-not-echo"})
		if zero != (runtimev2.OperationalObservation{}) {
			t.Fatal("unsafe name returned a non-zero observation")
		}
		requireValidation(t, err, runtimev2.CodeValueInvalid, "fields.name")
		if strings.Contains(err.Error(), name) || strings.Contains(err.Error(), "do-not-echo") {
			t.Fatalf("unsafe input leaked through error: %q", err)
		}
	}

	invalid := string([]byte{0xff})
	_, err = conformancev1.NewFactoryObservation(map[string]string{
		conformancev1.ObservationJobID: invalid,
		"z bad":                        invalid,
	})
	requireValidation(t, err, runtimev2.CodeValueInvalid, "fields.name")
	_, err = conformancev1.NewFactoryObservation(map[string]string{
		conformancev1.ObservationTimestamp: invalid,
		conformancev1.ObservationJobID:     invalid,
	})
	requireValidation(t, err, runtimev2.CodeValueInvalid, "fields.job_id")

	for _, value := range []string{
		strings.Repeat("a", runtimev2.MaxCanonicalStringBytes-1),
		strings.Repeat("é", runtimev2.MaxCanonicalStringBytes/utf8.RuneLen('é')),
	} {
		if _, err := conformancev1.NewFactoryObservation(map[string]string{conformancev1.ObservationJobID: value}); err != nil {
			t.Fatalf("valid boundary rejected: %v", err)
		}
	}
	tooLong := strings.Repeat("é", runtimev2.MaxCanonicalStringBytes/2) + "a"
	zero, err = conformancev1.NewFactoryObservation(map[string]string{conformancev1.ObservationJobID: tooLong})
	requireValidation(t, err, runtimev2.CodeLimitExceeded, "fields.job_id")
	if zero != (runtimev2.OperationalObservation{}) || strings.Contains(err.Error(), tooLong) {
		t.Fatal("limit error returned data or leaked the value")
	}
}

func TestDeclarationHelpersAndBoundaries(t *testing.T) {
	reference := "input\x00данные"
	input, err := conformancev1.NewInputDeclaration(reference)
	if err != nil {
		t.Fatal(err)
	}
	environment, err := conformancev1.NewExecutionEnvironmentDeclaration("environment")
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := conformancev1.NewDeploymentDeclaration("deployment")
	if err != nil {
		t.Fatal(err)
	}
	for _, declaration := range []runtimev2.ExtensionDeclaration{input, environment, deployment} {
		if declaration.SchemaVersion() != runtimev2.SchemaVersion || declaration.Owner() != conformancev1.Provider || declaration.Kind() != conformancev1.ExtensionKind || declaration.SpecificationSchema() != conformancev1.ExtensionSpecificationSchema {
			t.Fatalf("declaration metadata = %q/%q/%q/%q", declaration.SchemaVersion(), declaration.Owner(), declaration.Kind(), declaration.SpecificationSchema())
		}
		fields := declaration.Fields()
		if len(fields) != 1 || fields[0].Name != conformancev1.DeclarationReference {
			t.Fatalf("declaration fields = %#v", fields)
		}
	}
	if input.Fields()[0].Value != reference {
		t.Fatal("reference did not round-trip")
	}
	fields := input.Fields()
	fields[0].Value = "mutation"
	if input.Fields()[0].Value != reference {
		t.Fatal("declaration fields were not defensive")
	}

	for _, bad := range []struct {
		value string
		code  runtimev2.ValidationCode
	}{
		{"", runtimev2.CodeValueInvalid},
		{string([]byte{0xff}), runtimev2.CodeValueInvalid},
		{strings.Repeat("a", runtimev2.MaxResolvedValueBytes+1), runtimev2.CodeLimitExceeded},
	} {
		zero, err := conformancev1.NewInputDeclaration(bad.value)
		if zero != (runtimev2.InputDeclaration{}) {
			t.Fatal("invalid reference returned a non-zero declaration")
		}
		requireValidation(t, err, bad.code, "reference")
		if strings.Contains(err.Error(), bad.value) && bad.value != "" {
			t.Fatal("reference leaked through error")
		}
		for _, construct := range []func(string) error{
			func(value string) error {
				_, err := conformancev1.NewExecutionEnvironmentDeclaration(value)
				return err
			},
			func(value string) error { _, err := conformancev1.NewDeploymentDeclaration(value); return err },
		} {
			requireValidation(t, construct(bad.value), bad.code, "reference")
		}
	}
	for _, value := range []string{
		strings.Repeat("a", runtimev2.MaxResolvedValueBytes-1),
		strings.Repeat("é", runtimev2.MaxResolvedValueBytes/2),
	} {
		if _, err := conformancev1.NewDeploymentDeclaration(value); err != nil {
			t.Fatalf("valid reference boundary rejected: %v", err)
		}
	}
}

func TestExtensionCompositionRolesAndTransactionality(t *testing.T) {
	inputOne, _ := conformancev1.NewInputDeclaration("one")
	inputTwo, _ := conformancev1.NewInputDeclaration("two")
	environment, _ := conformancev1.NewExecutionEnvironmentDeclaration("environment")
	deployment, _ := conformancev1.NewDeploymentDeclaration("deployment")
	declaration := factoryDeclaration()
	declaration.Inputs = []runtimev2.InputDeclaration{inputOne, inputTwo}
	declaration.ExecutionEnvironment = &environment
	declaration.Deployment = &deployment
	one := buildExtension(t, "one", "v1")
	two := buildExtension(t, "two", "v1")
	env := buildExtension(t, "environment", "v1")
	dep := buildExtension(t, "deployment", "v1")

	builder, err := conformancev1.NewFactoryBuilder(declaration)
	if err != nil {
		t.Fatal(err)
	}
	locator, _ := runtimev2.NewTextIdentityField(conformancev1.FieldLocator, "factory")
	_ = builder.AddIdentityField(locator)
	if err := builder.BindExtensions(runtimev2.ResolvedFactoryExtensions{Inputs: []runtimev2.CanonicalResolvedExtensionIdentity{one}}); err == nil {
		t.Fatal("incomplete binding accepted")
	}
	complete := runtimev2.ResolvedFactoryExtensions{Inputs: []runtimev2.CanonicalResolvedExtensionIdentity{one, two}, ExecutionEnvironment: &env, Deployment: &dep}
	if err := builder.BindExtensions(complete); err != nil {
		t.Fatalf("rejected bind corrupted builder: %v", err)
	}
	result, err := builder.Build()
	if err != nil {
		t.Fatal(err)
	}
	forward, _ := runtimev2.CanonicalFactoryFingerprint(result.Identity())

	reordered, _ := conformancev1.NewFactoryBuilder(declaration)
	_ = reordered.AddIdentityField(locator)
	if err := reordered.BindExtensions(runtimev2.ResolvedFactoryExtensions{Inputs: []runtimev2.CanonicalResolvedExtensionIdentity{two, one}, ExecutionEnvironment: &env, Deployment: &dep}); err != nil {
		t.Fatal(err)
	}
	reorderedResult, _ := reordered.Build()
	backward, _ := runtimev2.CanonicalFactoryFingerprint(reorderedResult.Identity())
	if forward == backward {
		t.Fatal("input position did not affect factory identity")
	}

	transformDeclaration := transformDeclaration()
	transformDeclaration.Inputs = []runtimev2.InputDeclaration{inputOne}
	transformDeclaration.ExecutionEnvironment = &environment
	transform, err := conformancev1.NewTransformBuilder(transformDeclaration)
	if err != nil {
		t.Fatal(err)
	}
	transformLocator, _ := runtimev2.NewTextIdentityField(conformancev1.FieldLocator, "transform")
	_ = transform.AddIdentityField(transformLocator)
	if err := transform.BindExtensions(runtimev2.ResolvedTransformExtensions{Inputs: []runtimev2.CanonicalResolvedExtensionIdentity{one}, ExecutionEnvironment: &env}); err != nil {
		t.Fatal(err)
	}
	if _, err := transform.Build(); err != nil {
		t.Fatal(err)
	}
}

type allowAuthorizer struct{}

func (allowAuthorizer) AuthorizeInternalDisclosure(context.Context) error { return nil }

type denyAuthorizer struct{ err error }

func (a denyAuthorizer) AuthorizeInternalDisclosure(context.Context) error { return a.err }

type cancelAuthorizer struct{ cancel context.CancelFunc }

func (a cancelAuthorizer) AuthorizeInternalDisclosure(context.Context) error {
	a.cancel()
	return nil
}

func TestSensitivityAndDisclosure(t *testing.T) {
	build := func(version, observation string) (runtimev2.CanonicalFingerprint, runtimev2.FingerprintEnvelope) {
		builder, err := conformancev1.NewFactoryBuilder(factoryDeclaration())
		if err != nil {
			t.Fatal(err)
		}
		addAllIdentityFields(t, builder, "public-locator", version)
		value, _ := conformancev1.NewFactoryObservation(map[string]string{conformancev1.ObservationJobID: observation})
		_ = builder.AddObservation(value)
		result, err := builder.Build()
		if err != nil {
			t.Fatal(err)
		}
		fingerprint, _ := runtimev2.CanonicalFactoryFingerprint(result.Identity())
		envelope, _ := runtimev2.NewFactoryEnvelope(result.Identity())
		return fingerprint, envelope
	}
	one, envelope := build("v1", "one")
	two, _ := build("v2", "one")
	observationOnly, _ := build("v1", "two")
	if one == two || one != observationOnly {
		t.Fatal("secret or observation sensitivity is incorrect")
	}

	safe, err := runtimev2.ExplainSafe(envelope)
	if err != nil {
		t.Fatal(err)
	}
	safeJSON, _ := json.Marshal(safe)
	safeText := string(safeJSON)
	for _, forbidden := range []string{"private-plan", "secret-id", `"canonical_hex"`, `"secret_reference"`} {
		if strings.Contains(safeText, forbidden) {
			t.Fatalf("safe explanation leaked %q: %s", forbidden, safeJSON)
		}
	}
	if !strings.Contains(safeText, "public-locator") || !strings.Contains(safeText, `"redacted":true`) || !strings.Contains(safeText, `"commitment":"sha256:`) {
		t.Fatalf("safe explanation omitted public/redaction evidence: %s", safeJSON)
	}

	internal, err := runtimev2.ExplainInternal(context.Background(), envelope, allowAuthorizer{})
	if err != nil {
		t.Fatal(err)
	}
	internalJSON, _ := json.Marshal(internal)
	if !strings.Contains(string(internalJSON), "secret-id") || !strings.Contains(string(internalJSON), `"canonical_hex"`) {
		t.Fatalf("internal explanation omitted protected data: %s", internalJSON)
	}

	checks := []func() (runtimev2.FingerprintExplanation, error){
		func() (runtimev2.FingerprintExplanation, error) {
			return runtimev2.ExplainInternal(context.Background(), envelope, nil)
		},
		func() (runtimev2.FingerprintExplanation, error) {
			return runtimev2.ExplainInternal(context.Background(), envelope, denyAuthorizer{errors.New("deny")})
		},
		func() (runtimev2.FingerprintExplanation, error) {
			ctx, cancel := context.WithCancel(context.Background())
			cancel()
			return runtimev2.ExplainInternal(ctx, envelope, allowAuthorizer{})
		},
		func() (runtimev2.FingerprintExplanation, error) {
			ctx, cancel := context.WithCancel(context.Background())
			return runtimev2.ExplainInternal(ctx, envelope, cancelAuthorizer{cancel})
		},
	}
	for _, check := range checks {
		projection, err := check()
		if projection.Valid() {
			t.Fatal("authorization failure returned a partial projection")
		}
		requireValidation(t, err, runtimev2.CodeAuthorizationDenied, "")
	}
}
