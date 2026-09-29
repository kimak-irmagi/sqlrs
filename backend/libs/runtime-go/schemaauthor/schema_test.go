package schemaauthor_test

import (
	"testing"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
	"github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/schemaauthor"
)

func validSchemaInput() schemaauthor.SchemaInput {
	return schemaauthor.SchemaInput{
		Provider: "provider", SemanticKind: "fixture", IdentitySchema: "fixture.v1",
		ObservationSchema: "fixture-observation.v1",
		Fields: []schemaauthor.FieldDefinition{
			{Name: "z_semantic", Role: schemaauthor.FieldRoleSemantic, Kind: runtimev2.IdentityFieldCanonicalValue, Required: true, Disclosure: schemaauthor.DisclosurePublic},
			{Name: "a_operational", Role: schemaauthor.FieldRoleOperational, Kind: runtimev2.IdentityFieldText, Disclosure: schemaauthor.DisclosureProtected},
		},
	}
}

func TestAuthoredSchemasPreserveTypedMetadataAndImmutableSortedFields(t *testing.T) {
	input := validSchemaInput()
	factory, err := schemaauthor.NewFactorySchema(input)
	if err != nil {
		t.Fatal(err)
	}
	transform, err := schemaauthor.NewTransformSchema(input)
	if err != nil {
		t.Fatal(err)
	}
	extension, err := schemaauthor.NewExtensionSchema(input)
	if err != nil {
		t.Fatal(err)
	}
	if !factory.Valid() || !transform.Valid() || !extension.Valid() {
		t.Fatal("valid authored schema reported invalid")
	}
	if factory.Provider() != input.Provider || factory.SemanticKind() != input.SemanticKind || factory.IdentitySchema() != input.IdentitySchema || factory.ObservationSchema() != input.ObservationSchema {
		t.Fatalf("factory metadata was not preserved: %#v", factory)
	}
	for name, fields := range map[string][]runtimev2.CanonicalFieldDefinition{
		"factory": factory.Fields(), "transform": transform.Fields(), "extension": extension.Fields(),
	} {
		if len(fields) != 2 || fields[0].Name() != "a_operational" || fields[1].Name() != "z_semantic" {
			t.Fatalf("%s fields are not canonical: %#v", name, fields)
		}
		if fields[0].Role() != schemaauthor.FieldRoleOperational || fields[0].Kind() != string(runtimev2.IdentityFieldText) || fields[0].Required() || fields[0].Disclosure() != schemaauthor.DisclosureProtected {
			t.Fatalf("%s operational field changed: %#v", name, fields[0])
		}
	}

	input.Fields[0].Name = "caller-mutation"
	fields := factory.Fields()
	fields[0] = runtimev2.CanonicalFieldDefinition{}
	if factory.Fields()[0].Name() != "a_operational" || factory.Fields()[1].Name() != "z_semantic" {
		t.Fatal("authored schema retained mutable input or exposed mutable fields")
	}
}

func TestSchemaAuthorRejectsEveryInvalidContractDimension(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*schemaauthor.SchemaInput)
	}{
		{"provider", func(v *schemaauthor.SchemaInput) { v.Provider = "Bad" }},
		{"semantic kind", func(v *schemaauthor.SchemaInput) { v.SemanticKind = "bad kind" }},
		{"identity schema", func(v *schemaauthor.SchemaInput) { v.IdentitySchema = "" }},
		{"field name", func(v *schemaauthor.SchemaInput) { v.Fields[0].Name = "Bad" }},
		{"reserved field", func(v *schemaauthor.SchemaInput) { v.Fields[0].Name = "extension.input" }},
		{"duplicate field", func(v *schemaauthor.SchemaInput) { v.Fields[0].Name = v.Fields[1].Name }},
		{"field role", func(v *schemaauthor.SchemaInput) { v.Fields[0].Role = schemaauthor.FieldRole("other") }},
		{"field kind", func(v *schemaauthor.SchemaInput) { v.Fields[0].Kind = runtimev2.IdentityFieldKind("other") }},
		{"disclosure", func(v *schemaauthor.SchemaInput) { v.Fields[0].Disclosure = schemaauthor.DisclosureClass("other") }},
		{"required observation", func(v *schemaauthor.SchemaInput) { v.Fields[1].Required = true }},
		{"non-text observation", func(v *schemaauthor.SchemaInput) { v.Fields[1].Kind = runtimev2.IdentityFieldCanonicalValue }},
		{"missing observation schema", func(v *schemaauthor.SchemaInput) { v.ObservationSchema = "" }},
		{"invalid optional observation schema", func(v *schemaauthor.SchemaInput) {
			v.Fields = v.Fields[:1]
			v.ObservationSchema = "Bad"
		}},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			input := validSchemaInput()
			testCase.mutate(&input)
			if value, err := schemaauthor.NewFactorySchema(input); err == nil || value.Valid() {
				t.Fatalf("invalid schema accepted: %#v", value)
			}
		})
	}

	invalid := validSchemaInput()
	invalid.Provider = "Bad"
	if value, err := schemaauthor.NewTransformSchema(invalid); err == nil || value.Valid() {
		t.Fatal("invalid transform schema accepted")
	}
	if value, err := schemaauthor.NewExtensionSchema(invalid); err == nil || value.Valid() {
		t.Fatal("invalid extension schema accepted")
	}
}
