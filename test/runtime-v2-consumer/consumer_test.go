package consumertest

import (
	"testing"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
)

func TestStandaloneConsumer(t *testing.T) {
	identity, err := runtimev2.NewTransformIdentity(runtimev2.TransformIdentityInput{
		SchemaVersion:  runtimev2.SchemaVersion,
		Provider:       "sqlrs",
		Kind:           "psql",
		IdentitySchema: "transform.v1",
		Fields:         []runtimev2.ResolvedField{{Name: "sql", Value: "select 1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if fingerprint, err := runtimev2.TransformFingerprint(identity); err != nil || fingerprint == "" {
		t.Fatalf("fingerprint = %q, error = %v", fingerprint, err)
	}
	if identity.Provider() != "sqlrs" || identity.Kind() != "psql" || identity.IdentitySchema() != "transform.v1" {
		t.Fatalf("resolved identity accessors lost data: %+v", identity.Fields())
	}
}

func TestVersionedExtensionConsumer(t *testing.T) {
	declaration, err := runtimev2.NewInputDeclaration(runtimev2.ExtensionSpecificationInput{
		SchemaVersion: runtimev2.SchemaVersion, Owner: "sqlrs.workspace", Kind: "file",
		SpecificationSchema: "sqlrs.workspace-file.declaration.v1",
		Fields:              []runtimev2.DeclarationField{{Name: "path", Value: "input.sql"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	document, err := runtimev2.NewFactoryDeclarationDocument(runtimev2.FactoryDeclaration{
		Kind: "docker", Reference: "postgres:17", Inputs: []runtimev2.InputDeclaration{declaration},
	})
	if err != nil || len(document.Declaration().Inputs) != 1 {
		t.Fatalf("document = %+v, %v", document, err)
	}
	resolved, err := runtimev2.NewResolvedExtensionIdentity(runtimev2.ResolvedExtensionIdentityInput{
		SchemaVersion: runtimev2.SchemaVersion, Owner: "sqlrs.workspace", Kind: "file", IdentitySchema: "sqlrs.workspace-file.v1",
		Fields: []runtimev2.ResolvedField{{Name: "content.digest", Value: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	fields, err := runtimev2.ComposeResolvedFields(nil, []runtimev2.ExtensionBinding{{Name: "source", Identity: resolved}})
	if err != nil || len(fields) != 1 {
		t.Fatalf("fields = %+v, %v", fields, err)
	}
}
