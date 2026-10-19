package consumertest

import (
	"encoding/json"
	"strings"
	"testing"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
	"github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/resolver"
	"github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/schemas/sqlrs"
)

// ET08 is compiled as the ONLY test file in the staged/published consumer.
// It needs no generic authoring, conformance, internal or sibling-file helper.
func TestExecutionTransformStandaloneConsumer(t *testing.T) {
	identity, err := sqlrs.BuildExecutionEnvironmentIdentity(sqlrs.ExecutionEnvironmentInput{
		Runtime: sqlrs.ExecutionRuntime{Kind: sqlrs.ExecutionRuntimeOCI, IdentityKind: sqlrs.ExecutionIdentityImageManifest, IdentityDigest: "sha256:" + strings.Repeat("1", 64), Platform: sqlrs.ExecutionPlatform{OS: "linux", Architecture: "amd64"}},
		Argv:    []sqlrs.ExecutionText{{{Kind: sqlrs.ExecutionTextPartLiteral, Literal: "native-tool"}}}, Workdir: sqlrs.ExecutionPath{Root: "image"}, Network: sqlrs.ExecutionNetworkNone,
	})
	if err != nil {
		t.Fatal(err)
	}
	environment, err := runtimev2.NewExecutionEnvironmentDeclaration(runtimev2.ExtensionSpecificationInput{SchemaVersion: runtimev2.SchemaVersion, Owner: "sqlrs.execution", Kind: "environment", SpecificationSchema: "sqlrs.execution.declaration.v1"})
	if err != nil {
		t.Fatal(err)
	}
	descriptor := resolver.Descriptor{Role: "execution_environment", Owner: "sqlrs.execution", Kind: "environment", SpecificationSchema: "sqlrs.execution.declaration.v1", SemanticVersion: "1"}
	key, err := resolver.NewCacheKey(resolver.Workspace{Root: t.TempDir()}, descriptor, resolver.NormalizedDeclaration{Declaration: environment})
	if err != nil {
		t.Fatal(err)
	}
	record, err := resolver.NewCacheRecord(key, sqlrs.ExecutionEnvironmentSchema(), resolver.Resolution{Identity: identity, Evidence: json.RawMessage(`{}`)})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := resolver.DecodeCacheRecordJSON(raw, sqlrs.ExecutionEnvironmentSchema())
	if err != nil || !decoded.Matches(key) {
		t.Fatalf("execution result/cache round trip: %v", err)
	}
	resolved := decoded.Resolution().Identity
	declaration := runtimev2.TransformDeclaration{Kind: "execution", Reference: "native-tool", ExecutionEnvironment: &environment}
	extensions := runtimev2.ResolvedTransformExtensions{ExecutionEnvironment: &resolved}
	for _, content := range []string{"first", "second"} {
		input, err := runtimev2.NewInputDeclaration(runtimev2.ExtensionSpecificationInput{SchemaVersion: runtimev2.SchemaVersion, Owner: "sqlrs.workspace", Kind: "file", SpecificationSchema: "sqlrs.workspace-file.declaration.v1"})
		if err != nil {
			t.Fatal(err)
		}
		builder, err := runtimev2.NewExtensionIdentityBuilder(sqlrs.WorkspaceFileExtensionSchema())
		if err != nil {
			t.Fatal(err)
		}
		field, err := runtimev2.NewTextIdentityField("content.digest", content)
		if err != nil {
			t.Fatal(err)
		}
		if err = builder.AddIdentityField(field); err != nil {
			t.Fatal(err)
		}
		value, err := builder.Build()
		if err != nil {
			t.Fatal(err)
		}
		declaration.Inputs = append(declaration.Inputs, input)
		extensions.Inputs = append(extensions.Inputs, value.Identity())
	}
	composition, err := sqlrs.BuildExecutionTransform(declaration, extensions, nil)
	if err != nil {
		t.Fatal(err)
	}
	transform, err := runtimev2.NewTransformEnvelope(composition.Identity())
	if err != nil {
		t.Fatal(err)
	}
	if d := transform.Descriptor(); d.Provider() != "sqlrs" || d.SemanticKind() != "execution" || d.IdentitySchema() != "sqlrs.execution-transform.v1" {
		t.Fatal("wrong production transform")
	}
	child, err := runtimev2.NewDerivedStateEnvelope(runtimev2.CanonicalStateID("sha256:"+strings.Repeat("2", 64)), transform)
	if err != nil {
		t.Fatal(err)
	}
	for _, envelope := range []runtimev2.FingerprintEnvelope{transform, child} {
		raw, err = json.Marshal(envelope)
		if err != nil {
			t.Fatal(err)
		}
		var roundTrip runtimev2.FingerprintEnvelope
		if err = json.Unmarshal(raw, &roundTrip); err != nil || roundTrip.Descriptor().Digest() != envelope.Descriptor().Digest() {
			t.Fatalf("transform/lineage round trip: %v", err)
		}
	}
}
