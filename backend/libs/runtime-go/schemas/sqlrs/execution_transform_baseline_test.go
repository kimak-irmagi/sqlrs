package sqlrs_test

import (
	"encoding/hex"
	"strings"
	"testing"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
	"github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/schemas/sqlrs"
)

// ET09: capture precedes missing-API tests and production edits. Fixed inputs
// and literal expectations come from main 27936cb4d97e8acb4437ff76b92f6b4ae0354bb5.
// This fixture must not regenerate expectations using the implementation tested.
func TestExecutionTransformProductionBaseline(t *testing.T) {
	for _, tc := range []struct{ name, canonical, fingerprint string }{
		{"docker", "000000000000002b73716c72732e72756e74696d652e76322e63616e6f6e6963616c2e76312f666163746f72792d7374617465000000050001000000000000001d73716c72732e72756e74696d652e76322e63616e6f6e6963616c2e76310002000000000000000573716c727300030000000000000006646f636b65720004000000000000000f73716c72732e646f636b65722e763100050000000000000065000000020000000000000005696d6167650001959e6c9433a863266bd2461749587ce3f1e0a25df05813a65d6426a7efea8fa20000000000000008706c6174666f726d00012038aa3eb0f5ca22bf4fc9badf0e4a464e00a645116b1d20bf9e96a029cd5475", "sha256:8766971afa427d6687f413346f94be65508078d61025ecf2f3eed277cdd19ca3"},
		{"psql", "000000000000002773716c72732e72756e74696d652e76322e63616e6f6e6963616c2e76312f7472616e73666f726d000000050001000000000000001d73716c72732e72756e74696d652e76322e63616e6f6e6963616c2e76310002000000000000000573716c7273000300000000000000047073716c0004000000000000000d73716c72732e7073716c2e76310005000000000000003100000001000000000000000373716c0002a0ac0144e2e4dbaab580fb15d2b909279a97e8aa1b45833c8d05257b73ece55e", "sha256:56cb29b5caff168ce2e449a490ba440edd4348de70952762fafd49a5528aaae7"},
		{"psql-execution", "000000000000002773716c72732e72756e74696d652e76322e63616e6f6e6963616c2e76312f7472616e73666f726d000000050001000000000000001d73716c72732e72756e74696d652e76322e63616e6f6e6963616c2e76310002000000000000000573716c7273000300000000000000047073716c0004000000000000000d73716c72732e7073716c2e76310005000000000000007a00000002000000000000001f657874656e73696f6e2e657865637574696f6e5f656e7669726f6e6d656e7400014e0628d20d908c1c60ce656b9850a483a2dcef28571a5407878825a0b7d3cd38000000000000000373716c0002a0ac0144e2e4dbaab580fb15d2b909279a97e8aa1b45833c8d05257b73ece55e", "sha256:13335a918c4061fdea599ac8704738c3b634a3c10f0fd683b2da92c750a34d9f"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			canonical, fingerprint := executionTransformOldProduction(t, tc.name)
			if hex.EncodeToString(canonical) != tc.canonical || string(fingerprint) != tc.fingerprint {
				t.Fatal("production schema changed from recorded pre-implementation baseline")
			}
		})
	}
}

func executionTransformOldProduction(t testing.TB, name string) ([]byte, runtimev2.CanonicalFingerprint) {
	t.Helper()
	if name == "docker" {
		builder, err := sqlrs.NewDockerFactoryBuilder(runtimev2.FactoryDeclaration{Kind: "docker", Reference: "postgres:17"})
		if err != nil {
			t.Fatal(err)
		}
		for _, input := range [][2]string{{"image", "postgres@sha256:" + strings.Repeat("1", 64)}, {"platform", "linux/amd64"}} {
			field, err := runtimev2.NewTextIdentityField(input[0], input[1])
			if err != nil {
				t.Fatal(err)
			}
			if err = builder.AddIdentityField(field); err != nil {
				t.Fatal(err)
			}
		}
		composition, err := builder.Build()
		if err != nil {
			t.Fatal(err)
		}
		canonical, err := runtimev2.CanonicalFactoryIdentityBytes(composition.Identity())
		if err != nil {
			t.Fatal(err)
		}
		fingerprint, err := runtimev2.CanonicalFactoryFingerprint(composition.Identity())
		if err != nil {
			t.Fatal(err)
		}
		return canonical, fingerprint
	}
	declaration := runtimev2.TransformDeclaration{Kind: "psql", Reference: "query.sql"}
	if name == "psql-execution" {
		environment, err := runtimev2.NewExecutionEnvironmentDeclaration(runtimev2.ExtensionSpecificationInput{SchemaVersion: runtimev2.SchemaVersion, Owner: "sqlrs.execution", Kind: "environment", SpecificationSchema: "sqlrs.execution.declaration.v1"})
		if err != nil {
			t.Fatal(err)
		}
		declaration.ExecutionEnvironment = &environment
	}
	builder, err := sqlrs.NewPSQLTransformBuilder(declaration)
	if err != nil {
		t.Fatal(err)
	}
	value, err := runtimev2.CanonicalString("select 1")
	if err != nil {
		t.Fatal(err)
	}
	field, err := runtimev2.NewCanonicalValueIdentityField("sql", value)
	if err != nil {
		t.Fatal(err)
	}
	if err = builder.AddIdentityField(field); err != nil {
		t.Fatal(err)
	}
	if name == "psql-execution" {
		identity, err := sqlrs.BuildExecutionEnvironmentIdentity(sqlrs.ExecutionEnvironmentInput{
			Runtime: sqlrs.ExecutionRuntime{Kind: sqlrs.ExecutionRuntimeOCI, IdentityKind: sqlrs.ExecutionIdentityImageManifest, IdentityDigest: "sha256:" + strings.Repeat("1", 64), Platform: sqlrs.ExecutionPlatform{OS: "linux", Architecture: "amd64"}},
			Argv:    []sqlrs.ExecutionText{{{Kind: sqlrs.ExecutionTextPartLiteral, Literal: "tool"}}}, Workdir: sqlrs.ExecutionPath{Root: "image"}, Network: sqlrs.ExecutionNetworkNone,
		})
		if err != nil {
			t.Fatal(err)
		}
		if err = builder.BindExtensions(runtimev2.ResolvedTransformExtensions{ExecutionEnvironment: &identity}); err != nil {
			t.Fatal(err)
		}
	}
	composition, err := builder.Build()
	if err != nil {
		t.Fatal(err)
	}
	canonical, err := runtimev2.CanonicalTransformIdentityBytes(composition.Identity())
	if err != nil {
		t.Fatal(err)
	}
	fingerprint, err := runtimev2.CanonicalTransformFingerprint(composition.Identity())
	if err != nil {
		t.Fatal(err)
	}
	return canonical, fingerprint
}
