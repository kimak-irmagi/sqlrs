package consumertest

import (
	"encoding/json"
	"strings"
	"testing"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
	"github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/schemas/sqlrs"
)

// ES26 runs through the existing staged file-proxy fixture without replace,
// internal imports, schemaauthor, a conformance schema or a published version.
func TestProductionExecutionSchemaStandaloneConsumer(t *testing.T) {
	identity, err := sqlrs.BuildExecutionEnvironmentIdentity(sqlrs.ExecutionEnvironmentInput{
		Runtime: sqlrs.ExecutionRuntime{Kind: sqlrs.ExecutionRuntimeOCI, IdentityKind: sqlrs.ExecutionIdentityImageManifest, IdentityDigest: "sha256:" + strings.Repeat("1", 64), Platform: sqlrs.ExecutionPlatform{OS: "linux", Architecture: "amd64"}},
		Argv:    []sqlrs.ExecutionText{{{Kind: sqlrs.ExecutionTextPartLiteral, Literal: "tool"}}}, Workdir: sqlrs.ExecutionPath{Root: "image"}, Network: sqlrs.ExecutionNetworkNone,
	})
	if err != nil {
		t.Fatal(err)
	}
	parsed, err := sqlrs.ParseExecutionEnvironmentIdentity(identity)
	if err != nil || len(parsed.Argv) != 1 {
		t.Fatalf("parse: %v", err)
	}
	for _, field := range identity.Fields() {
		if field.Field.Kind() == runtimev2.IdentityFieldCanonicalValue {
			value := field.Field.CanonicalValue()
			if value.EncodedSize() != len(value.CanonicalBytes()) || value.Kind() == "" {
				t.Fatal("public inspection unavailable")
			}
		}
	}
	env, err := runtimev2.NewExecutionEnvironmentDeclaration(runtimev2.ExtensionSpecificationInput{SchemaVersion: runtimev2.SchemaVersion, Owner: "sqlrs.execution", Kind: "environment", SpecificationSchema: "sqlrs.execution.declaration.v1"})
	if err != nil {
		t.Fatal(err)
	}
	builder, err := sqlrs.NewPSQLTransformBuilder(runtimev2.TransformDeclaration{Kind: "psql", Reference: "query", ExecutionEnvironment: &env})
	if err != nil {
		t.Fatal(err)
	}
	sql, err := runtimev2.CanonicalString("select 1")
	if err != nil {
		t.Fatal(err)
	}
	field, err := runtimev2.NewCanonicalValueIdentityField("sql", sql)
	if err != nil {
		t.Fatal(err)
	}
	if err = builder.AddIdentityField(field); err != nil {
		t.Fatal(err)
	}
	if err = builder.BindExtensions(runtimev2.ResolvedTransformExtensions{ExecutionEnvironment: &identity}); err != nil {
		t.Fatal(err)
	}
	composition, err := builder.Build()
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := runtimev2.NewTransformEnvelope(composition.Identity())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	var decoded runtimev2.FingerprintEnvelope
	if err = json.Unmarshal(raw, &decoded); err != nil || decoded.Descriptor().Digest() != envelope.Descriptor().Digest() {
		t.Fatalf("serialization: %v", err)
	}
}
