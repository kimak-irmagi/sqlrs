package canonicalv1

import "testing"

func TestFieldDefinitionAndTypedSchemasAreImmutable(t *testing.T) {
	field := NewFieldDefinition("locator", FieldRoleSemantic, "text", true, DisclosurePublic)
	if field.Name() != "locator" || field.Role() != FieldRoleSemantic || field.Kind() != "text" || !field.Required() || field.Disclosure() != DisclosurePublic {
		t.Fatalf("field definition changed: %#v", field)
	}
	fields := []FieldDefinition{field}
	factory := NewFactorySchema("provider", "factory", "factory.v1", "factory-observation.v1", fields)
	transform := NewTransformSchema("provider", "transform", "transform.v1", "transform-observation.v1", fields)
	extension := NewExtensionSchema("provider", "extension", "extension.v1", "extension-observation.v1", fields)
	if !factory.Valid() || !transform.Valid() || !extension.Valid() {
		t.Fatal("constructed schema reported invalid")
	}
	if factory.Provider() != "provider" || factory.SemanticKind() != "factory" || factory.IdentitySchema() != "factory.v1" || factory.ObservationSchema() != "factory-observation.v1" {
		t.Fatalf("factory schema metadata changed: %#v", factory)
	}
	if transform.Provider() != "provider" || transform.SemanticKind() != "transform" || transform.IdentitySchema() != "transform.v1" || transform.ObservationSchema() != "transform-observation.v1" {
		t.Fatalf("transform schema metadata changed: %#v", transform)
	}
	if extension.Provider() != "provider" || extension.SemanticKind() != "extension" || extension.IdentitySchema() != "extension.v1" || extension.ObservationSchema() != "extension-observation.v1" {
		t.Fatalf("extension schema metadata changed: %#v", extension)
	}
	fields[0] = FieldDefinition{}
	returned := factory.Fields()
	returned[0] = FieldDefinition{}
	if factory.Fields()[0].Name() != "locator" || transform.Fields()[0].Name() != "locator" || extension.Fields()[0].Name() != "locator" {
		t.Fatal("schema fields were not defensively copied")
	}
}

func TestZeroTypedSchemasAreSafeAndInvalid(t *testing.T) {
	var factory FactorySchema
	var transform TransformSchema
	var extension ExtensionSchema
	if factory.Valid() || transform.Valid() || extension.Valid() {
		t.Fatal("zero schema reported valid")
	}
	for name, values := range map[string][]string{
		"factory":   {factory.Provider(), factory.SemanticKind(), factory.IdentitySchema(), factory.ObservationSchema()},
		"transform": {transform.Provider(), transform.SemanticKind(), transform.IdentitySchema(), transform.ObservationSchema()},
		"extension": {extension.Provider(), extension.SemanticKind(), extension.IdentitySchema(), extension.ObservationSchema()},
	} {
		for _, value := range values {
			if value != "" {
				t.Fatalf("zero %s schema exposed metadata %q", name, value)
			}
		}
	}
	if factory.Fields() != nil || transform.Fields() != nil || extension.Fields() != nil {
		t.Fatal("zero schema exposed fields")
	}
}
