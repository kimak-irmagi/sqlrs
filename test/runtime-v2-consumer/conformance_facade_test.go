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

func consumerMust[T any](value T, err error) T {
	if err != nil {
		panic(err)
	}
	return value
}

func consumerExtension(t *testing.T, version string) runtimev2.CanonicalResolvedExtensionIdentity {
	t.Helper()
	builder, err := conformancev1.NewExtensionBuilder()
	if err != nil {
		t.Fatal(err)
	}
	locator := consumerMust(runtimev2.NewTextIdentityField(conformancev1.FieldLocator, "extension"))
	reference := consumerMust(runtimev2.NewSecretReference("vault", "consumer-secret", version))
	credential := consumerMust(runtimev2.NewSecretReferenceIdentityField(conformancev1.FieldCredential, reference))
	if err := builder.AddIdentityField(locator); err != nil {
		t.Fatal(err)
	}
	if err := builder.AddIdentityField(credential); err != nil {
		t.Fatal(err)
	}
	observation := consumerMust(conformancev1.NewExtensionObservation(map[string]string{conformancev1.ObservationCheckpointBackend: "local"}))
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
	environment := consumerMust(conformancev1.NewExecutionEnvironmentDeclaration("environment"))
	deployment := consumerMust(conformancev1.NewDeploymentDeclaration("deployment"))
	extension := consumerExtension(t, "v1")

	factory, err := conformancev1.NewFactoryBuilder(runtimev2.FactoryDeclaration{
		Kind: conformancev1.FactoryKind, Reference: "factory", Inputs: []runtimev2.InputDeclaration{input},
		ExecutionEnvironment: &environment, Deployment: &deployment,
	})
	if err != nil {
		t.Fatal(err)
	}
	locator := consumerMust(runtimev2.NewTextIdentityField(conformancev1.FieldLocator, "factory"))
	planValue := consumerMust(runtimev2.CanonicalString("plan"))
	plan := consumerMust(runtimev2.NewCanonicalValueIdentityField(conformancev1.FieldPlan, planValue))
	reference := consumerMust(runtimev2.NewSecretReference("vault", "consumer-secret", "v1"))
	credential := consumerMust(runtimev2.NewSecretReferenceIdentityField(conformancev1.FieldCredential, reference))
	for _, field := range []runtimev2.IdentityField{locator, plan, credential} {
		if err := factory.AddIdentityField(field); err != nil {
			t.Fatal(err)
		}
	}
	observation := consumerMust(conformancev1.NewFactoryObservation(map[string]string{conformancev1.ObservationJobID: "job-one"}))
	if err := factory.AddObservation(observation); err != nil {
		t.Fatal(err)
	}
	if err := factory.BindExtensions(runtimev2.ResolvedFactoryExtensions{
		Inputs: []runtimev2.CanonicalResolvedExtensionIdentity{extension}, ExecutionEnvironment: &extension, Deployment: &extension,
	}); err != nil {
		t.Fatal(err)
	}
	factoryResult, err := factory.Build()
	if err != nil {
		t.Fatal(err)
	}
	factoryFingerprint := consumerMust(runtimev2.CanonicalFactoryFingerprint(factoryResult.Identity()))
	envelope, err := runtimev2.NewFactoryEnvelope(factoryResult.Identity())
	if err != nil {
		t.Fatal(err)
	}
	safe := consumerMust(runtimev2.ExplainSafe(envelope))
	safeJSON := consumerMust(json.Marshal(safe))
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
	transformLocator := consumerMust(runtimev2.NewTextIdentityField(conformancev1.FieldLocator, "transform"))
	if err := transform.AddIdentityField(transformLocator); err != nil {
		t.Fatal(err)
	}
	if err := transform.BindExtensions(runtimev2.ResolvedTransformExtensions{
		Inputs: []runtimev2.CanonicalResolvedExtensionIdentity{extension}, ExecutionEnvironment: &extension,
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := transform.Build(); err != nil {
		t.Fatal(err)
	}

	build := func(version, job string) runtimev2.CanonicalFingerprint {
		builder := consumerMust(conformancev1.NewFactoryBuilder(runtimev2.FactoryDeclaration{Kind: conformancev1.FactoryKind, Reference: "factory"}))
		field := consumerMust(runtimev2.NewTextIdentityField(conformancev1.FieldLocator, "factory"))
		if err := builder.AddIdentityField(field); err != nil {
			t.Fatal(err)
		}
		secret := consumerMust(runtimev2.NewSecretReference("vault", "consumer-secret", version))
		secretField := consumerMust(runtimev2.NewSecretReferenceIdentityField(conformancev1.FieldCredential, secret))
		if err := builder.AddIdentityField(secretField); err != nil {
			t.Fatal(err)
		}
		observation := consumerMust(conformancev1.NewFactoryObservation(map[string]string{conformancev1.ObservationJobID: job}))
		if err := builder.AddObservation(observation); err != nil {
			t.Fatal(err)
		}
		result := consumerMust(builder.Build())
		return consumerMust(runtimev2.CanonicalFactoryFingerprint(result.Identity()))
	}
	if build("v1", "one") != build("v1", "two") || build("v1", "one") == build("v2", "one") {
		t.Fatal("external sensitivity/isolation contract failed")
	}
	if factoryFingerprint == "" {
		t.Fatal("factory fingerprint unavailable")
	}
}
