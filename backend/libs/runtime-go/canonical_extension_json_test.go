package runtimev2_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"testing"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
	"github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/schemaauthor"
)

// CR04-CR06: the public codec preserves typed, schema-bound extension fields.
func TestCanonicalExtensionIdentityJSONRoundTrip(t *testing.T) {
	schema, err := schemaauthor.NewExtensionSchema(schemaauthor.SchemaInput{
		Provider: "sample", SemanticKind: "artifact", IdentitySchema: "sample.artifact.v1",
		Fields: []schemaauthor.FieldDefinition{
			{Name: "a.text", Role: schemaauthor.FieldRoleSemantic, Kind: runtimev2.IdentityFieldText, Required: true, Disclosure: schemaauthor.DisclosurePublic},
			{Name: "b.value", Role: schemaauthor.FieldRoleSemantic, Kind: runtimev2.IdentityFieldCanonicalValue, Required: true, Disclosure: schemaauthor.DisclosureProtected},
			{Name: "c.reference", Role: schemaauthor.FieldRoleSemantic, Kind: runtimev2.IdentityFieldSecretReference, Required: true, Disclosure: schemaauthor.DisclosureProtected},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	builder, err := runtimev2.NewExtensionIdentityBuilder(schema)
	if err != nil {
		t.Fatal(err)
	}
	textField, err := runtimev2.NewTextIdentityField("a.text", "sha256:abc")
	if err != nil {
		t.Fatal(err)
	}
	value, err := runtimev2.CanonicalMap([]runtimev2.CanonicalMapEntry{{Key: "branch", Value: runtimev2.CanonicalNull()}})
	if err != nil {
		t.Fatal(err)
	}
	valueField, err := runtimev2.NewCanonicalValueIdentityField("b.value", value)
	if err != nil {
		t.Fatal(err)
	}
	reference, err := runtimev2.NewSecretReference("vault", "item", "7")
	if err != nil {
		t.Fatal(err)
	}
	referenceField, err := runtimev2.NewSecretReferenceIdentityField("c.reference", reference)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []runtimev2.IdentityField{textField, valueField, referenceField} {
		if err := builder.AddIdentityField(field); err != nil {
			t.Fatal(err)
		}
	}
	composition, err := builder.Build()
	if err != nil {
		t.Fatal(err)
	}
	identity := composition.Identity()
	if identity.Provider() != "sample" || identity.Kind() != "artifact" || identity.IdentitySchema() != "sample.artifact.v1" {
		t.Fatal("identity metadata lost")
	}
	fields := identity.Fields()
	if len(fields) != 3 || fields[0].Field.Name() != "a.text" || fields[1].Disclosure != runtimev2.DisclosureProtected {
		t.Fatal("typed fields lost")
	}
	fields[0] = runtimev2.CanonicalIdentityField{}
	if identity.Fields()[0].Field.Name() != "a.text" {
		t.Fatal("fields accessor exposed mutable storage")
	}
	raw, err := runtimev2.MarshalCanonicalExtensionIdentityJSON(identity, schema)
	if err != nil {
		t.Fatal(err)
	}
	decoded, err := runtimev2.DecodeCanonicalExtensionIdentityJSON(raw, schema)
	if err != nil {
		t.Fatal(err)
	}
	wantBytes, _ := runtimev2.CanonicalResolvedExtensionBytes(identity)
	gotBytes, _ := runtimev2.CanonicalResolvedExtensionBytes(decoded)
	if !bytes.Equal(gotBytes, wantBytes) {
		t.Fatal("canonical bytes changed")
	}
	got := decoded.Fields()
	if got[0].Field.Text() != "sha256:abc" || !bytes.Equal(got[1].Field.CanonicalValue().CanonicalBytes(), value.CanonicalBytes()) || got[2].Field.SecretReference().Version() != "7" {
		t.Fatal("field payload changed")
	}
	var object map[string]any
	if err := json.Unmarshal(raw, &object); err != nil {
		t.Fatal(err)
	}
	if object["schema_version"] != runtimev2.CanonicalSchemaVersion {
		t.Fatal("wrong wire schema")
	}
	for _, mutant := range [][]byte{
		bytes.Replace(raw, []byte(`"kind":"artifact"`), []byte(`"kind":"wrong"`), 1),
		bytes.Replace(raw, []byte(`"kind":"text"`), []byte(`"kind":"canonical-value"`), 1),
		bytes.Replace(raw, []byte(`"disclosure":"protected"`), []byte(`"disclosure":"public"`), 1),
		bytes.Replace(raw, []byte(`"fingerprint":"sha256:`), []byte(`"fingerprint":"sha256:0`), 1),
		append(append([]byte(nil), raw...), []byte(` true`)...),
		bytes.Replace(raw, []byte(`"provider":"sample"`), []byte(`"provider":"\ud800"`), 1),
		bytes.Replace(raw, []byte(`"provider":"sample"`), []byte(`"Provider":"sample"`), 1),
		append(append(bytes.Repeat([]byte("["), 65), []byte("0")...), bytes.Repeat([]byte("]"), 65)...),
		append([]byte(`{"provider":"`), 0xff),
		[]byte(`{"provider":"\u12xz"}`),
		[]byte(`{"provider":"\udc00"}`),
		[]byte(`{"provider":"\ud800"}`),
		[]byte(`{"provider":"\ud800\u0041"}`),
		[]byte(`{"provider":"\ud800\u12xz"}`),
		[]byte(`{"provider":"\u123"}`),
	} {
		if _, err := runtimev2.DecodeCanonicalExtensionIdentityJSON(mutant, schema); err == nil {
			t.Fatalf("accepted mutant: %s", mutant)
		}
	}
	mutate := func(change func(map[string]any, []any)) []byte {
		var document map[string]any
		if err := json.Unmarshal(raw, &document); err != nil {
			t.Fatal(err)
		}
		fields := document["fields"].([]any)
		change(document, fields)
		encoded, err := json.Marshal(document)
		if err != nil {
			t.Fatal(err)
		}
		return encoded
	}
	for name, mutant := range map[string][]byte{
		"missing field":         mutate(func(document map[string]any, fields []any) { document["fields"] = fields[:2] }),
		"missing fields member": mutate(func(document map[string]any, _ []any) { delete(document, "fields") }),
		"null fields":           mutate(func(document map[string]any, _ []any) { document["fields"] = nil }),
		"fields as object":      mutate(func(document map[string]any, _ []any) { document["fields"] = map[string]any{} }),
		"null field":            mutate(func(_ map[string]any, fields []any) { fields[0] = nil }),
		"too many fields": mutate(func(document map[string]any, fields []any) {
			many := make([]any, runtimev2.MaxCanonicalValueNodes+1)
			for index := range many {
				many[index] = fields[0]
			}
			document["fields"] = many
		}),
		"duplicate field": mutate(func(document map[string]any, fields []any) { document["fields"] = append(fields, fields[0]) }),
		"out of order":    mutate(func(_ map[string]any, fields []any) { fields[0], fields[1] = fields[1], fields[0] }),
		"extra field": mutate(func(document map[string]any, fields []any) {
			document["fields"] = append(fields, map[string]any{"name": "z.extra", "kind": "text", "disclosure": "public", "commitment": "sha256:bad", "text": "x"})
		}),
		"wrong commitment":           mutate(func(_ map[string]any, fields []any) { fields[0].(map[string]any)["commitment"] = "sha256:bad" }),
		"wrong canonical bytes":      mutate(func(document map[string]any, _ []any) { document["canonical_bytes"] = "AA==" }),
		"invalid base64":             mutate(func(_ map[string]any, fields []any) { fields[1].(map[string]any)["canonical_value"] = "???" }),
		"invalid canonical envelope": mutate(func(_ map[string]any, fields []any) { fields[1].(map[string]any)["canonical_value"] = "AA==" }),
		"noncanonical base64":        mutate(func(_ map[string]any, fields []any) { fields[1].(map[string]any)["canonical_value"] = "AA==\n" }),
		"wrong text value":           mutate(func(_ map[string]any, fields []any) { fields[0].(map[string]any)["text"] = "changed" }),
		"empty field name":           mutate(func(_ map[string]any, fields []any) { fields[0].(map[string]any)["name"] = "" }),
		"malformed secret reference": mutate(func(_ map[string]any, fields []any) {
			fields[2].(map[string]any)["secret_reference"].(map[string]any)["version"] = ""
		}),
		"extra payload arm": mutate(func(_ map[string]any, fields []any) { fields[0].(map[string]any)["canonical_value"] = "AA==" }),
		"wrong payload arm": mutate(func(_ map[string]any, fields []any) {
			field := fields[0].(map[string]any)
			delete(field, "text")
			field["canonical_value"] = "AA=="
		}),
		"null payload":          mutate(func(_ map[string]any, fields []any) { fields[0].(map[string]any)["text"] = nil }),
		"unknown nested member": mutate(func(_ map[string]any, fields []any) { fields[0].(map[string]any)["unknown"] = true }),
		"case-variant field member": mutate(func(_ map[string]any, fields []any) {
			field := fields[0].(map[string]any)
			field["Name"] = field["name"]
			delete(field, "name")
		}),
		"case-variant secret member": mutate(func(_ map[string]any, fields []any) {
			reference := fields[2].(map[string]any)["secret_reference"].(map[string]any)
			reference["Provider"] = reference["provider"]
			delete(reference, "provider")
		}),
		"unknown secret member": mutate(func(_ map[string]any, fields []any) {
			fields[2].(map[string]any)["secret_reference"].(map[string]any)["unknown"] = true
		}),
		"wrong version": mutate(func(document map[string]any, _ []any) { document["schema_version"] = "future" }),
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := runtimev2.DecodeCanonicalExtensionIdentityJSON(mutant, schema); err == nil {
				t.Fatal("accepted malformed typed identity")
			}
		})
	}
	if _, err := runtimev2.MarshalCanonicalExtensionIdentityJSON(identity, runtimev2.ExtensionIdentitySchema{}); err == nil {
		t.Fatal("encoder accepted unbound schema")
	}
	if _, err := runtimev2.DecodeCanonicalExtensionIdentityJSON(raw, runtimev2.ExtensionIdentitySchema{}); err == nil {
		t.Fatal("decoder accepted unbound schema")
	}
	if _, err := runtimev2.DecodeCanonicalExtensionIdentityJSON(bytes.Repeat([]byte(" "), runtimev2.MaxCanonicalEnvelopeBytes+1), schema); err == nil {
		t.Fatal("decoder accepted oversized envelope")
	}
}

// CR04/CR07: a valid schema may define more fields than one cache identity
// permits; the encoder must enforce the wire limit before serializing it.
func TestCanonicalExtensionIdentityJSONFieldLimit(t *testing.T) {
	definitions := make([]schemaauthor.FieldDefinition, runtimev2.MaxCanonicalValueNodes+1)
	for index := range definitions {
		definitions[index] = schemaauthor.FieldDefinition{
			Name: fmt.Sprintf("field.%04d", index), Role: schemaauthor.FieldRoleSemantic,
			Kind: runtimev2.IdentityFieldText, Disclosure: schemaauthor.DisclosurePublic,
		}
	}
	schema, err := schemaauthor.NewExtensionSchema(schemaauthor.SchemaInput{
		Provider: "sample", SemanticKind: "artifact", IdentitySchema: "sample.artifact.v1", Fields: definitions,
	})
	if err != nil {
		t.Fatal(err)
	}
	builder, err := runtimev2.NewExtensionIdentityBuilder(schema)
	if err != nil {
		t.Fatal(err)
	}
	for _, definition := range definitions {
		field, err := runtimev2.NewTextIdentityField(definition.Name, "x")
		if err != nil {
			t.Fatal(err)
		}
		if err := builder.AddIdentityField(field); err != nil {
			t.Fatal(err)
		}
	}
	composition, err := builder.Build()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := runtimev2.MarshalCanonicalExtensionIdentityJSON(composition.Identity(), schema); err == nil {
		t.Fatal("encoder accepted more than the field limit")
	}
}

// FuzzDecodeCanonicalExtensionIdentityJSON keeps malformed trusted-boundary
// inputs from producing an identity that cannot be encoded and decoded again.
func FuzzDecodeCanonicalExtensionIdentityJSON(f *testing.F) {
	schema, err := schemaauthor.NewExtensionSchema(schemaauthor.SchemaInput{Provider: "sample", SemanticKind: "artifact", IdentitySchema: "sample.artifact.v1"})
	if err != nil {
		f.Fatal(err)
	}
	builder, err := runtimev2.NewExtensionIdentityBuilder(schema)
	if err != nil {
		f.Fatal(err)
	}
	composition, err := builder.Build()
	if err != nil {
		f.Fatal(err)
	}
	valid, err := runtimev2.MarshalCanonicalExtensionIdentityJSON(composition.Identity(), schema)
	if err != nil {
		f.Fatal(err)
	}
	f.Add(valid)
	f.Add([]byte(`{}`))
	f.Add([]byte(`{"schema_version":"future"}`))
	f.Fuzz(func(t *testing.T, raw []byte) {
		identity, err := runtimev2.DecodeCanonicalExtensionIdentityJSON(raw, schema)
		if err != nil {
			return
		}
		reencoded, err := runtimev2.MarshalCanonicalExtensionIdentityJSON(identity, schema)
		if err != nil {
			t.Fatalf("accepted identity did not re-encode: %v", err)
		}
		if _, err := runtimev2.DecodeCanonicalExtensionIdentityJSON(reencoded, schema); err != nil {
			t.Fatalf("accepted identity did not round trip: %v", err)
		}
	})
}

// CR05: this fixed wire was calculated from the documented length-prefixed
// grammar with a separate Node crypto/Buffer calculation, not this Go encoder.
func TestCanonicalExtensionIdentityIndependentWire(t *testing.T) {
	const expected = `{"schema_version":"sqlrs.runtime.v2.canonical.v1","provider":"sample","kind":"artifact","identity_schema":"sample.artifact.v1","fields":[{"name":"a.text","kind":"text","disclosure":"public","commitment":"sha256:37571fc365d01e63b19cc66503d96058f360979ed5c7857ea140014b668e658e","text":"x"},{"name":"b.value","kind":"canonical-value","disclosure":"protected","commitment":"sha256:a18d79db3aeefd30bcffe28970d3cc0ce7f505cd4766bcf31b972c58b261cd12","canonical_value":"AAIAAAAAAAAAFgAAAAEAAAAAAAAACgAAAAAAAAAAAAA="},{"name":"c.reference","kind":"secret-reference","disclosure":"protected","commitment":"sha256:03cbbafb8d0638f6a2cd53f3d7b2320a687205744e0ca0c28f5c60b09ce75a0a","secret_reference":{"provider":"vault","identifier":"id","version":"1"}}],"canonical_bytes":"AAAAAAAAADBzcWxycy5ydW50aW1lLnYyLmNhbm9uaWNhbC52MS9yZXNvbHZlZC1leHRlbnNpb24AAAAFAAEAAAAAAAAAHXNxbHJzLnJ1bnRpbWUudjIuY2Fub25pY2FsLnYxAAIAAAAAAAAABnNhbXBsZQADAAAAAAAAAAhhcnRpZmFjdAAEAAAAAAAAABJzYW1wbGUuYXJ0aWZhY3QudjEABQAAAAAAAACaAAAAAwAAAAAAAAAGYS50ZXh0AAE3Vx/DZdAeY7GcxmUD2WBY82CXntXHhX6hQAFLZo5ljgAAAAAAAAAHYi52YWx1ZQACoY152zru/TC8/+KJcNPMDOf1Bc1HZrzzG5csWLJhzRIAAAAAAAAAC2MucmVmZXJlbmNlAAMDy7r7jQY49qLNU/PXsjIKaHIFdE4MoMKPXGCwnOdaCg==","fingerprint":"sha256:2484115829af62e1d780159830a4ada7acd4e0e3f2170b873ee0515063121777"}`
	schema, err := schemaauthor.NewExtensionSchema(schemaauthor.SchemaInput{Provider: "sample", SemanticKind: "artifact", IdentitySchema: "sample.artifact.v1", Fields: []schemaauthor.FieldDefinition{
		{Name: "a.text", Role: schemaauthor.FieldRoleSemantic, Kind: runtimev2.IdentityFieldText, Required: true, Disclosure: schemaauthor.DisclosurePublic},
		{Name: "b.value", Role: schemaauthor.FieldRoleSemantic, Kind: runtimev2.IdentityFieldCanonicalValue, Required: true, Disclosure: schemaauthor.DisclosureProtected},
		{Name: "c.reference", Role: schemaauthor.FieldRoleSemantic, Kind: runtimev2.IdentityFieldSecretReference, Required: true, Disclosure: schemaauthor.DisclosureProtected},
	}})
	if err != nil {
		t.Fatal(err)
	}
	builder, err := runtimev2.NewExtensionIdentityBuilder(schema)
	if err != nil {
		t.Fatal(err)
	}
	textField, err := runtimev2.NewTextIdentityField("a.text", "x")
	if err != nil {
		t.Fatal(err)
	}
	value, err := runtimev2.CanonicalList([]runtimev2.CanonicalValue{runtimev2.CanonicalNull()})
	if err != nil {
		t.Fatal(err)
	}
	valueField, err := runtimev2.NewCanonicalValueIdentityField("b.value", value)
	if err != nil {
		t.Fatal(err)
	}
	reference, err := runtimev2.NewSecretReference("vault", "id", "1")
	if err != nil {
		t.Fatal(err)
	}
	referenceField, err := runtimev2.NewSecretReferenceIdentityField("c.reference", reference)
	if err != nil {
		t.Fatal(err)
	}
	for _, field := range []runtimev2.IdentityField{textField, valueField, referenceField} {
		if err := builder.AddIdentityField(field); err != nil {
			t.Fatal(err)
		}
	}
	composition, err := builder.Build()
	if err != nil {
		t.Fatal(err)
	}
	actual, err := runtimev2.MarshalCanonicalExtensionIdentityJSON(composition.Identity(), schema)
	if err != nil {
		t.Fatal(err)
	}
	if string(actual) != expected {
		t.Fatalf("canonical identity wire disagrees with independent fixture\nactual: %s\nexpected: %s", actual, expected)
	}
	if _, err := runtimev2.DecodeCanonicalExtensionIdentityJSON([]byte(expected), schema); err != nil {
		t.Fatalf("independent fixture rejected: %v", err)
	}
}
