package runtimev2_test

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
	"github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/schemaauthor"
)

func TestFactoryEnvelopeStrictRoundTripAndTampering(t *testing.T) {
	identity := testEnvelopeFactoryIdentity(t)
	envelope, err := runtimev2.NewFactoryEnvelope(identity)
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
	if decoded.Descriptor().Kind() != runtimev2.FingerprintKindFactoryState || decoded.Descriptor().Digest() != envelope.Descriptor().Digest() {
		t.Fatal("verified descriptor changed during round trip")
	}
	tampered := strings.Replace(string(raw), string(envelope.Descriptor().Digest()), "sha256:"+strings.Repeat("0", 64), 1)
	if err := runtimev2.DecodeJSON([]byte(tampered), &decoded); err == nil {
		t.Fatal("tampered digest accepted")
	} else {
		var validation *runtimev2.ValidationError
		if !errors.As(err, &validation) || validation.Code != runtimev2.CodeDigestMismatch || validation.Path != "descriptor.digest" {
			t.Fatalf("unexpected tamper error: %#v", err)
		}
	}
	withUnknown := strings.Replace(string(raw), `"descriptor":{`, `"unknown":1,"descriptor":{`, 1)
	if err := runtimev2.DecodeJSON([]byte(withUnknown), &decoded); err == nil {
		t.Fatal("unknown member accepted")
	}
}

func TestCanonicalValueEnvelopeCarriesTreeAndRecomputesToken(t *testing.T) {
	value, _ := runtimev2.CanonicalString("payload")
	envelope, err := runtimev2.NewCanonicalValueEnvelope(value)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(envelope)
	if !strings.Contains(string(raw), `"canonical_hex"`) {
		t.Fatal("canonical-value envelope omitted complete tree bytes")
	}
	var decoded runtimev2.FingerprintEnvelope
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	tampered := strings.Replace(string(raw), value.Token().String(), "civ1:sha256:"+strings.Repeat("0", 64), 1)
	if err := json.Unmarshal([]byte(tampered), &decoded); err == nil {
		t.Fatal("token not recomputed from tree")
	}
}

func TestExplainProfilesRedactBeforeAuthorization(t *testing.T) {
	envelope, err := runtimev2.NewFactoryEnvelope(testEnvelopeFactoryIdentity(t))
	if err != nil {
		t.Fatal(err)
	}
	safe, err := runtimev2.ExplainSafe(envelope)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(safe)
	if strings.Contains(string(raw), "protected-locator") || strings.Contains(string(raw), "secret-id") {
		t.Fatalf("safe explanation leaked protected data: %s", raw)
	}
	if !strings.Contains(string(raw), `"redacted":true`) {
		t.Fatal("safe explanation lacks typed redaction marker")
	}
	if _, err := runtimev2.ExplainInternal(context.Background(), envelope, nil); err == nil {
		t.Fatal("nil authorizer accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	authorizer := testAuthorizer{}
	if explanation, err := runtimev2.ExplainInternal(ctx, envelope, authorizer); err == nil || explanation.Valid() {
		t.Fatal("canceled context returned a partial explanation")
	}
	internal, err := runtimev2.ExplainInternal(context.Background(), envelope, authorizer)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ = json.Marshal(internal)
	if !strings.Contains(string(raw), "protected-locator") || !strings.Contains(string(raw), "secret-id") {
		t.Fatalf("authorized internal explanation omitted reference data: %s", raw)
	}
}

func TestDerivedStateEnvelopeBindsParentAndVerifiedTransform(t *testing.T) {
	transformSchema, err := schemaauthor.NewTransformSchema(schemaauthor.SchemaInput{
		Provider: "acme", SemanticKind: "migration", IdentitySchema: "acme.migration.v1",
		Fields: []schemaauthor.FieldDefinition{{Name: "script", Role: schemaauthor.FieldRoleSemantic, Kind: runtimev2.IdentityFieldText, Required: true, Disclosure: schemaauthor.DisclosurePublic}},
	})
	if err != nil {
		t.Fatal(err)
	}
	builder, _ := runtimev2.NewTransformIdentityBuilder(transformSchema, runtimev2.TransformDeclaration{Kind: "migration", Reference: "001", Arguments: []string{}, Attributes: map[string]string{}})
	script, _ := runtimev2.NewTextIdentityField("script", "create table t(id int)")
	if err := builder.AddIdentityField(script); err != nil {
		t.Fatal(err)
	}
	composition, err := builder.Build()
	if err != nil {
		t.Fatal(err)
	}
	transform, err := runtimev2.NewTransformEnvelope(composition.Identity())
	if err != nil {
		t.Fatal(err)
	}
	parentEnvelope, _ := runtimev2.NewFactoryEnvelope(testEnvelopeFactoryIdentity(t))
	parent := runtimev2.CanonicalStateID(parentEnvelope.Descriptor().Digest())
	derived, err := runtimev2.NewDerivedStateEnvelope(parent, transform)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(derived)
	var decoded runtimev2.FingerprintEnvelope
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Descriptor().Digest() != derived.Descriptor().Digest() {
		t.Fatal("derived state digest changed")
	}
	tampered := strings.Replace(string(raw), string(parent), "sha256:"+strings.Repeat("0", 64), 1)
	if err := json.Unmarshal([]byte(tampered), &decoded); err == nil {
		t.Fatal("tampered parent accepted")
	}
}

type testAuthorizer struct{}

func (testAuthorizer) AuthorizeInternalDisclosure(context.Context) error { return nil }

func testEnvelopeFactoryIdentity(t *testing.T) runtimev2.CanonicalFactoryIdentity {
	t.Helper()
	schema, err := schemaauthor.NewFactorySchema(schemaauthor.SchemaInput{
		Provider: "acme", SemanticKind: "database", IdentitySchema: "acme.database.v1",
		Fields: []schemaauthor.FieldDefinition{
			{Name: "locator", Role: schemaauthor.FieldRoleSemantic, Kind: runtimev2.IdentityFieldText, Required: true, Disclosure: schemaauthor.DisclosureProtected},
			{Name: "plan", Role: schemaauthor.FieldRoleSemantic, Kind: runtimev2.IdentityFieldCanonicalValue, Required: true, Disclosure: schemaauthor.DisclosurePublic},
			{Name: "credential", Role: schemaauthor.FieldRoleSemantic, Kind: runtimev2.IdentityFieldSecretReference, Required: true, Disclosure: schemaauthor.DisclosureProtected},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	builder, err := runtimev2.NewFactoryIdentityBuilder(schema, runtimev2.FactoryDeclaration{Kind: "database", Reference: "db", Arguments: []string{}, Attributes: map[string]string{}})
	if err != nil {
		t.Fatal(err)
	}
	locator, _ := runtimev2.NewTextIdentityField("locator", "protected-locator")
	reference, _ := runtimev2.NewSecretReference("vault", "secret-id", "version-1")
	credential, _ := runtimev2.NewSecretReferenceIdentityField("credential", reference)
	planValue, _ := runtimev2.CanonicalString("stable-plan")
	plan, _ := runtimev2.NewCanonicalValueIdentityField("plan", planValue)
	if err := builder.AddIdentityField(locator); err != nil {
		t.Fatal(err)
	}
	if err := builder.AddIdentityField(credential); err != nil {
		t.Fatal(err)
	}
	if err := builder.AddIdentityField(plan); err != nil {
		t.Fatal(err)
	}
	composition, err := builder.Build()
	if err != nil {
		t.Fatal(err)
	}
	return composition.Identity()
}

func TestEnvelopeDescriptorAndSingleFaultMatrix(t *testing.T) {
	if _, err := runtimev2.NewCanonicalValueEnvelope(runtimev2.CanonicalValue{}); err == nil {
		t.Fatal("zero value envelope accepted")
	}
	if _, err := runtimev2.NewFactoryEnvelope(runtimev2.CanonicalFactoryIdentity{}); err == nil {
		t.Fatal("zero factory envelope accepted")
	}
	if _, err := runtimev2.NewTransformEnvelope(runtimev2.CanonicalTransformIdentity{}); err == nil {
		t.Fatal("zero transform envelope accepted")
	}
	if _, err := runtimev2.NewExtensionEnvelope(runtimev2.CanonicalResolvedExtensionIdentity{}); err == nil {
		t.Fatal("zero extension envelope accepted")
	}
	if _, err := json.Marshal(runtimev2.FingerprintEnvelope{}); err == nil {
		t.Fatal("zero envelope marshaled")
	}
	if _, err := json.Marshal(runtimev2.FingerprintExplanation{}); err == nil {
		t.Fatal("zero explanation marshaled")
	}
	zeroDescriptor := (runtimev2.FingerprintEnvelope{}).Descriptor()
	if zeroDescriptor.Kind() != "" || zeroDescriptor.Domain() != "" || zeroDescriptor.Digest() != "" || zeroDescriptor.Provider() != "" || zeroDescriptor.SemanticKind() != "" || zeroDescriptor.IdentitySchema() != "" {
		t.Fatal("zero descriptor accessors")
	}

	envelope, _ := runtimev2.NewFactoryEnvelope(testEnvelopeFactoryIdentity(t))
	descriptor := envelope.Descriptor()
	if !envelope.Valid() || descriptor.Domain() != runtimev2.CanonicalFactoryStateDomain || descriptor.Provider() != "acme" || descriptor.SemanticKind() != "database" || descriptor.IdentitySchema() != "acme.database.v1" {
		t.Fatal("descriptor accessors")
	}
	raw, _ := json.Marshal(envelope)
	var document map[string]any
	if err := json.Unmarshal(raw, &document); err != nil {
		t.Fatal(err)
	}

	cases := []func(map[string]any){
		func(d map[string]any) { d["descriptor"].(map[string]any)["schema_version"] = runtimev2.SchemaVersion },
		func(d map[string]any) { d["descriptor"].(map[string]any)["algorithm"] = "md5" },
		func(d map[string]any) {
			d["descriptor"].(map[string]any)["domain"] = runtimev2.CanonicalTransformDomain
		},
		func(d map[string]any) { delete(d["descriptor"].(map[string]any), "provider") },
		func(d map[string]any) {
			d["subject"].(map[string]any)["fields"].([]any)[0].(map[string]any)["commitment"] = "sha256:" + strings.Repeat("0", 64)
		},
		func(d map[string]any) {
			d["subject"].(map[string]any)["fields"].([]any)[0].(map[string]any)["disclosure"] = "unknown"
		},
	}
	for index, mutate := range cases {
		var candidate map[string]any
		encoded, _ := json.Marshal(document)
		_ = json.Unmarshal(encoded, &candidate)
		mutate(candidate)
		fault, _ := json.Marshal(candidate)
		var decoded runtimev2.FingerprintEnvelope
		if err := json.Unmarshal(fault, &decoded); err == nil {
			t.Fatalf("fault %d accepted", index)
		}
	}

	var decoded runtimev2.FingerprintEnvelope
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	original := decoded.Descriptor().Digest()
	if err := json.Unmarshal([]byte(`{"descriptor":null}`), &decoded); err == nil || decoded.Descriptor().Digest() != original {
		t.Fatal("failed decode mutated receiver")
	}
	if err := json.Unmarshal(make([]byte, runtimev2.MaxCanonicalEnvelopeBytes+1), &decoded); err == nil {
		t.Fatal("oversized envelope accepted")
	}
}

func TestCanonicalEnvelopeStrictJSONTaxonomy(t *testing.T) {
	envelope, err := runtimev2.NewFactoryEnvelope(testEnvelopeFactoryIdentity(t))
	if err != nil {
		t.Fatal(err)
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	duplicate := strings.Replace(string(raw), `"descriptor":`, `"descriptor":null,"descriptor":`, 1)
	unknown := strings.Replace(string(raw), `{`, `{"unexpected":true,`, 1)
	cases := []struct {
		name string
		raw  []byte
		code runtimev2.ValidationCode
		path string
	}{
		{"duplicate member", []byte(duplicate), runtimev2.CodeDuplicateMember, "descriptor"},
		{"unknown member", []byte(unknown), runtimev2.CodeUnknownMember, "unexpected"},
		{"invalid UTF-8", []byte{0xff}, runtimev2.CodeSyntaxInvalid, ""},
		{"truncated JSON", raw[:len(raw)-1], runtimev2.CodeSyntaxInvalid, ""},
		{"trailing JSON", append(append([]byte(nil), raw...), []byte(` {}`)...), runtimev2.CodeSyntaxInvalid, ""},
		{"wrong member type", []byte(`{"descriptor":[],"subject":{}}`), runtimev2.CodeShapeInvalid, "descriptor"},
		{"document too large", make([]byte, runtimev2.MaxCanonicalEnvelopeBytes+1), runtimev2.CodeDocumentTooLarge, ""},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			var decoded runtimev2.FingerprintEnvelope
			err := runtimev2.DecodeJSON(test.raw, &decoded)
			var validation *runtimev2.ValidationError
			if !errors.As(err, &validation) || validation.Code != test.code || validation.Path != test.path {
				t.Fatalf("error = %#v, want %s at %s", err, test.code, test.path)
			}
			if decoded.Valid() {
				t.Fatal("failed strict decode produced a trusted envelope")
			}
		})
	}
}

type denyingAuthorizer struct{ err error }

func (a denyingAuthorizer) AuthorizeInternalDisclosure(context.Context) error { return a.err }

type cancelingAuthorizer struct{ cancel context.CancelFunc }

func (a cancelingAuthorizer) AuthorizeInternalDisclosure(context.Context) error {
	a.cancel()
	return nil
}

func TestExplainAuthorizationFailureMatrix(t *testing.T) {
	envelope, _ := runtimev2.NewFactoryEnvelope(testEnvelopeFactoryIdentity(t))
	for _, authorizer := range []runtimev2.InternalDisclosureAuthorizer{denyingAuthorizer{errors.New("denied")}, denyingAuthorizer{nil}} {
		if explanation, err := runtimev2.ExplainInternal(context.Background(), envelope, authorizer); authorizer.(denyingAuthorizer).err != nil && (err == nil || explanation.Valid()) {
			t.Fatal("authorization failure returned projection")
		}
	}
	if _, err := runtimev2.ExplainSafe(runtimev2.FingerprintEnvelope{}); err == nil {
		t.Fatal("zero envelope explained")
	}
	ctx, cancel := context.WithCancel(context.Background())
	if explanation, err := runtimev2.ExplainInternal(ctx, envelope, cancelingAuthorizer{cancel}); err == nil || explanation.Valid() {
		t.Fatal("authorization-time cancellation returned projection")
	}
	transformValue, _ := runtimev2.CanonicalString("x")
	valueEnvelope, _ := runtimev2.NewCanonicalValueEnvelope(transformValue)
	parent := runtimev2.CanonicalStateID("sha256:" + strings.Repeat("1", 64))
	if _, err := runtimev2.NewDerivedStateEnvelope(parent, valueEnvelope); err == nil {
		t.Fatal("non-transform derived state accepted")
	}
	if _, err := runtimev2.NewDerivedStateEnvelope("invalid", runtimev2.FingerprintEnvelope{}); err == nil {
		t.Fatal("invalid parent accepted")
	}
}

func TestEnvelopeSubjectShapeAndDescriptorMatrix(t *testing.T) {
	factoryEnvelope, _ := runtimev2.NewFactoryEnvelope(testEnvelopeFactoryIdentity(t))
	factoryRaw, _ := json.Marshal(factoryEnvelope)
	var base map[string]any
	_ = json.Unmarshal(factoryRaw, &base)
	mutations := []func(map[string]any){
		func(d map[string]any) { delete(d, "subject") },
		func(d map[string]any) { d["descriptor"].(map[string]any)["digest"] = "bad" },
		func(d map[string]any) {
			fields := d["subject"].(map[string]any)["fields"].([]any)
			fields[0], fields[2] = fields[2], fields[0]
		},
		func(d map[string]any) {
			d["subject"].(map[string]any)["fields"].([]any)[1].(map[string]any)["canonical_hex"] = "00"
		},
		func(d map[string]any) {
			d["subject"].(map[string]any)["fields"].([]any)[2].(map[string]any)["canonical_hex"] = "zz"
		},
		func(d map[string]any) {
			d["subject"].(map[string]any)["fields"].([]any)[2].(map[string]any)["token"] = "civ1:sha256:" + strings.Repeat("0", 64)
		},
		func(d map[string]any) {
			d["subject"].(map[string]any)["fields"].([]any)[0].(map[string]any)["secret_reference"].(map[string]any)["identifier"] = ""
		},
		func(d map[string]any) {
			d["subject"].(map[string]any)["fields"].([]any)[1].(map[string]any)["kind"] = "unknown"
		},
	}
	for index, mutate := range mutations {
		var candidate map[string]any
		raw, _ := json.Marshal(base)
		_ = json.Unmarshal(raw, &candidate)
		mutate(candidate)
		fault, _ := json.Marshal(candidate)
		var decoded runtimev2.FingerprintEnvelope
		if err := json.Unmarshal(fault, &decoded); err == nil {
			t.Fatalf("subject fault %d accepted", index)
		}
	}

	value, _ := runtimev2.CanonicalString("x")
	valueEnvelope, _ := runtimev2.NewCanonicalValueEnvelope(value)
	valueRaw, _ := json.Marshal(valueEnvelope)
	var valueDoc map[string]any
	_ = json.Unmarshal(valueRaw, &valueDoc)
	valueDoc["descriptor"].(map[string]any)["provider"] = "acme"
	fault, _ := json.Marshal(valueDoc)
	var decoded runtimev2.FingerprintEnvelope
	if err := json.Unmarshal(fault, &decoded); err == nil {
		t.Fatal("canonical-value provider metadata accepted")
	}

	extensionSchema, _ := schemaauthor.NewExtensionSchema(schemaauthor.SchemaInput{Provider: "storage", SemanticKind: "bucket", IdentitySchema: "storage.bucket.v1", Fields: []schemaauthor.FieldDefinition{{Name: "name", Role: schemaauthor.FieldRoleSemantic, Kind: runtimev2.IdentityFieldText, Required: true, Disclosure: schemaauthor.DisclosurePublic}}})
	extensionBuilder, _ := runtimev2.NewExtensionIdentityBuilder(extensionSchema)
	name, _ := runtimev2.NewTextIdentityField("name", "assets")
	_ = extensionBuilder.AddIdentityField(name)
	extension, _ := extensionBuilder.Build()
	extensionEnvelope, err := runtimev2.NewExtensionEnvelope(extension.Identity())
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(extensionEnvelope)
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if decoded.Descriptor().Kind() != runtimev2.FingerprintKindResolvedExtension {
		t.Fatal("extension kind lost")
	}
}

func TestFingerprintDescriptorExposesCompleteVersionedIdentity(t *testing.T) {
	envelope, err := runtimev2.NewFactoryEnvelope(testEnvelopeFactoryIdentity(t))
	if err != nil {
		t.Fatal(err)
	}
	descriptor := envelope.Descriptor()
	if descriptor.EnvelopeVersion() != runtimev2.CanonicalEnvelopeVersion || descriptor.SchemaVersion() != runtimev2.CanonicalSchemaVersion || descriptor.Algorithm() != "sha256" {
		t.Fatalf("incomplete descriptor accessors: envelope=%q schema=%q algorithm=%q", descriptor.EnvelopeVersion(), descriptor.SchemaVersion(), descriptor.Algorithm())
	}
	var zero runtimev2.FingerprintDescriptor
	if zero.EnvelopeVersion() != "" || zero.SchemaVersion() != "" || zero.Algorithm() != "" {
		t.Fatal("zero descriptor reported trusted metadata")
	}
}

func TestCanonicalIdentityEnvelopeRequiresFieldsMember(t *testing.T) {
	schema, err := schemaauthor.NewExtensionSchema(schemaauthor.SchemaInput{Provider: "acme", SemanticKind: "empty", IdentitySchema: "acme.empty.v1"})
	if err != nil {
		t.Fatal(err)
	}
	builder, err := runtimev2.NewExtensionIdentityBuilder(schema)
	if err != nil {
		t.Fatal(err)
	}
	composition, err := builder.Build()
	if err != nil {
		t.Fatal(err)
	}
	envelope, err := runtimev2.NewExtensionEnvelope(composition.Identity())
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := json.Marshal(envelope)
	var document map[string]any
	_ = json.Unmarshal(raw, &document)
	delete(document["subject"].(map[string]any), "fields")
	fault, _ := json.Marshal(document)
	var decoded runtimev2.FingerprintEnvelope
	err = json.Unmarshal(fault, &decoded)
	var validation *runtimev2.ValidationError
	if !errors.As(err, &validation) || validation.Code != runtimev2.CodeShapeInvalid || validation.Path != "subject.fields" {
		t.Fatalf("error = %#v, want shape_invalid at subject.fields", err)
	}
}

func TestDerivedEnvelopeNestedFailureMatrix(t *testing.T) {
	transformSchema, _ := schemaauthor.NewTransformSchema(schemaauthor.SchemaInput{Provider: "acme", SemanticKind: "migration", IdentitySchema: "acme.migration.v1", Fields: []schemaauthor.FieldDefinition{{Name: "script", Role: schemaauthor.FieldRoleSemantic, Kind: runtimev2.IdentityFieldText, Required: true, Disclosure: schemaauthor.DisclosurePublic}}})
	builder, _ := runtimev2.NewTransformIdentityBuilder(transformSchema, runtimev2.TransformDeclaration{Kind: "migration", Reference: "x", Arguments: []string{}, Attributes: map[string]string{}})
	script, _ := runtimev2.NewTextIdentityField("script", "x")
	_ = builder.AddIdentityField(script)
	composition, _ := builder.Build()
	transform, _ := runtimev2.NewTransformEnvelope(composition.Identity())
	derived, _ := runtimev2.NewDerivedStateEnvelope(runtimev2.CanonicalStateID("sha256:"+strings.Repeat("1", 64)), transform)
	raw, _ := json.Marshal(derived)
	var base map[string]any
	_ = json.Unmarshal(raw, &base)
	mutations := []func(map[string]any){func(d map[string]any) { delete(d["subject"].(map[string]any), "transform") }, func(d map[string]any) { d["subject"].(map[string]any)["transform"] = map[string]any{"bad": true} }, func(d map[string]any) {
		d["descriptor"].(map[string]any)["digest"] = "sha256:" + strings.Repeat("0", 64)
	}}
	for index, mutate := range mutations {
		var candidate map[string]any
		copyRaw, _ := json.Marshal(base)
		_ = json.Unmarshal(copyRaw, &candidate)
		mutate(candidate)
		fault, _ := json.Marshal(candidate)
		var decoded runtimev2.FingerprintEnvelope
		if err := json.Unmarshal(fault, &decoded); err == nil {
			t.Fatalf("derived fault %d accepted", index)
		}
	}
}
