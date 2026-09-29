package consumertest

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
	"github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/schemas/conformancev1"
)

type consumerAuthorizer struct{}

func (consumerAuthorizer) AuthorizeInternalDisclosure(context.Context) error { return nil }

func consumerExtension(t *testing.T, version string) runtimev2.CanonicalResolvedExtensionIdentity {
	t.Helper()
	builder, err := conformancev1.NewExtensionBuilder()
	if err != nil {
		t.Fatal(err)
	}
	locator, _ := runtimev2.NewTextIdentityField(conformancev1.FieldLocator, "extension")
	reference, _ := runtimev2.NewSecretReference("vault", "consumer-secret", version)
	credential, _ := runtimev2.NewSecretReferenceIdentityField(conformancev1.FieldCredential, reference)
	if err := builder.AddIdentityField(locator); err != nil {
		t.Fatal(err)
	}
	if err := builder.AddIdentityField(credential); err != nil {
		t.Fatal(err)
	}
	observation, _ := conformancev1.NewExtensionObservation(map[string]string{conformancev1.ObservationCheckpointBackend: "local"})
	if err := builder.AddObservation(observation); err != nil {
		t.Fatal(err)
	}
	result, err := builder.Build()
	if err != nil {
		t.Fatal(err)
	}
	return result.Identity()
}

func TestExternalConformanceFacade(t *testing.T) {
	input, err := conformancev1.NewInputDeclaration("input")
	if err != nil {
		t.Fatal(err)
	}
	environment, _ := conformancev1.NewExecutionEnvironmentDeclaration("environment")
	deployment, _ := conformancev1.NewDeploymentDeclaration("deployment")
	extension := consumerExtension(t, "v1")

	factory, err := conformancev1.NewFactoryBuilder(runtimev2.FactoryDeclaration{
		Kind: conformancev1.FactoryKind, Reference: "factory", Inputs: []runtimev2.InputDeclaration{input},
		ExecutionEnvironment: &environment, Deployment: &deployment,
	})
	if err != nil {
		t.Fatal(err)
	}
	locator, _ := runtimev2.NewTextIdentityField(conformancev1.FieldLocator, "factory")
	planValue, _ := runtimev2.CanonicalString("plan")
	plan, _ := runtimev2.NewCanonicalValueIdentityField(conformancev1.FieldPlan, planValue)
	reference, _ := runtimev2.NewSecretReference("vault", "consumer-secret", "v1")
	credential, _ := runtimev2.NewSecretReferenceIdentityField(conformancev1.FieldCredential, reference)
	for _, field := range []runtimev2.IdentityField{locator, plan, credential} {
		if err := factory.AddIdentityField(field); err != nil {
			t.Fatal(err)
		}
	}
	observation, _ := conformancev1.NewFactoryObservation(map[string]string{conformancev1.ObservationJobID: "job-one"})
	_ = factory.AddObservation(observation)
	if err := factory.BindExtensions(runtimev2.ResolvedFactoryExtensions{
		Inputs: []runtimev2.CanonicalResolvedExtensionIdentity{extension}, ExecutionEnvironment: &extension, Deployment: &extension,
	}); err != nil {
		t.Fatal(err)
	}
	factoryResult, err := factory.Build()
	if err != nil {
		t.Fatal(err)
	}
	factoryFingerprint, _ := runtimev2.CanonicalFactoryFingerprint(factoryResult.Identity())
	envelope, err := runtimev2.NewFactoryEnvelope(factoryResult.Identity())
	if err != nil {
		t.Fatal(err)
	}
	safe, _ := runtimev2.ExplainSafe(envelope)
	safeJSON, _ := json.Marshal(safe)
	if strings.Contains(string(safeJSON), "consumer-secret") {
		t.Fatalf("safe explanation leaked secret reference: %s", safeJSON)
	}
	internal, err := runtimev2.ExplainInternal(context.Background(), envelope, consumerAuthorizer{})
	if err != nil || !internal.Valid() {
		t.Fatalf("internal explanation = %v, %v", internal.Valid(), err)
	}

	transform, err := conformancev1.NewTransformBuilder(runtimev2.TransformDeclaration{
		Kind: conformancev1.TransformKind, Reference: "transform", Inputs: []runtimev2.InputDeclaration{input}, ExecutionEnvironment: &environment,
	})
	if err != nil {
		t.Fatal(err)
	}
	transformLocator, _ := runtimev2.NewTextIdentityField(conformancev1.FieldLocator, "transform")
	_ = transform.AddIdentityField(transformLocator)
	if err := transform.BindExtensions(runtimev2.ResolvedTransformExtensions{
		Inputs: []runtimev2.CanonicalResolvedExtensionIdentity{extension}, ExecutionEnvironment: &extension,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := transform.Build(); err != nil {
		t.Fatal(err)
	}

	build := func(version, job string) runtimev2.CanonicalFingerprint {
		builder, _ := conformancev1.NewFactoryBuilder(runtimev2.FactoryDeclaration{Kind: conformancev1.FactoryKind, Reference: "factory"})
		field, _ := runtimev2.NewTextIdentityField(conformancev1.FieldLocator, "factory")
		_ = builder.AddIdentityField(field)
		secret, _ := runtimev2.NewSecretReference("vault", "consumer-secret", version)
		secretField, _ := runtimev2.NewSecretReferenceIdentityField(conformancev1.FieldCredential, secret)
		_ = builder.AddIdentityField(secretField)
		observation, _ := conformancev1.NewFactoryObservation(map[string]string{conformancev1.ObservationJobID: job})
		_ = builder.AddObservation(observation)
		result, _ := builder.Build()
		fingerprint, _ := runtimev2.CanonicalFactoryFingerprint(result.Identity())
		return fingerprint
	}
	if build("v1", "one") != build("v1", "two") || build("v1", "one") == build("v2", "one") {
		t.Fatal("external sensitivity/isolation contract failed")
	}
	if factoryFingerprint == "" {
		t.Fatal("factory fingerprint unavailable")
	}
}
