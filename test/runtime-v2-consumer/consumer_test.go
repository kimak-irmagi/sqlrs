package consumertest

import (
	"encoding/json"
	"testing"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
	"github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/schemas/sqlrs"
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

func TestCanonicalV1StandaloneConsumer(t *testing.T) {
	value, err := runtimev2.CanonicalString("select 1")
	if err != nil || value.Token().String() == "" {
		t.Fatalf("canonical value: %v", err)
	}
	builder, err := sqlrs.NewDockerFactoryBuilder(runtimev2.FactoryDeclaration{Kind: "docker", Reference: "postgres:17", Arguments: []string{}, Attributes: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	image, _ := runtimev2.NewTextIdentityField("image", "postgres@sha256:abc")
	if err := builder.AddIdentityField(image); err != nil {
		t.Fatal(err)
	}
	composition, err := builder.Build()
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := runtimev2.NewFactoryEnvelope(composition.Identity())
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	var decoded runtimev2.FingerprintEnvelope
	if err := runtimev2.DecodeJSON(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	descriptor := decoded.Descriptor()
	if descriptor.EnvelopeVersion() != runtimev2.CanonicalEnvelopeVersion || descriptor.SchemaVersion() != runtimev2.CanonicalSchemaVersion || descriptor.Algorithm() != "sha256" {
		t.Fatalf("descriptor metadata unavailable: %q %q %q", descriptor.EnvelopeVersion(), descriptor.SchemaVersion(), descriptor.Algorithm())
	}
	if _, err := runtimev2.ExplainSafe(decoded); err != nil {
		t.Fatal(err)
	}
	if _, err := runtimev2.LoadConformanceBundle(); err != nil {
		t.Fatal(err)
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

func TestConsumerDispatchesLegacyAndCanonicalRevisionsExplicitly(t *testing.T) {
	legacy, err := runtimev2.NewTransformIdentity(runtimev2.TransformIdentityInput{
		SchemaVersion: runtimev2.SchemaVersion, Provider: "sqlrs", Kind: "psql", IdentitySchema: "transform.v1",
		Fields: []runtimev2.ResolvedField{{Name: "sql", Value: "select 1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	legacyJSON, _ := json.Marshal(legacy)

	builder, err := sqlrs.NewDockerFactoryBuilder(runtimev2.FactoryDeclaration{Kind: "docker", Reference: "postgres:17", Arguments: []string{}, Attributes: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	image, _ := runtimev2.NewTextIdentityField("image", "postgres@sha256:abc")
	_ = builder.AddIdentityField(image)
	composition, _ := builder.Build()
	canonical, _ := runtimev2.NewFactoryEnvelope(composition.Identity())
	canonicalJSON, _ := json.Marshal(canonical)

	dispatch := func(raw []byte) (string, error) {
		var probe struct {
			SchemaVersion string `json:"schema_version"`
			Descriptor    *struct {
				SchemaVersion string `json:"schema_version"`
			} `json:"descriptor"`
		}
		if err := json.Unmarshal(raw, &probe); err != nil {
			return "", err
		}
		switch {
		case probe.SchemaVersion == runtimev2.SchemaVersion:
			var value runtimev2.ResolvedTransformIdentity
			if err := runtimev2.DecodeJSON(raw, &value); err != nil {
				return "", err
			}
			return "legacy", nil
		case probe.Descriptor != nil && probe.Descriptor.SchemaVersion == runtimev2.CanonicalSchemaVersion:
			var value runtimev2.FingerprintEnvelope
			if err := runtimev2.DecodeJSON(raw, &value); err != nil {
				return "", err
			}
			return "canonical", nil
		default:
			return "", runtimev2.ErrInvalid
		}
	}
	if revision, err := dispatch(legacyJSON); err != nil || revision != "legacy" {
		t.Fatalf("legacy dispatch = %q, %v", revision, err)
	}
	if revision, err := dispatch(canonicalJSON); err != nil || revision != "canonical" {
		t.Fatalf("canonical dispatch = %q, %v", revision, err)
	}
	if _, err := dispatch([]byte(`{"schema_version":"future"}`)); err == nil {
		t.Fatal("unknown revision was guessed or upgraded")
	}
}
