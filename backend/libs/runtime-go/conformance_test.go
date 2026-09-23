package runtimev2

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestFactoryIdentitySensitivity(t *testing.T) {
	base := FactoryIdentityInput{
		SchemaVersion:  SchemaVersion,
		Provider:       "sqlrs",
		Kind:           "postgres",
		IdentitySchema: "factory.v1",
		Fields:         []ResolvedField{{Name: "image", Value: "postgres:17"}},
	}
	wantRoot, wantEndpoint := buildFactoryLineageIDs(t, base)
	mutations := map[string]func(*FactoryIdentityInput){
		"provider":        func(v *FactoryIdentityInput) { v.Provider = "other" },
		"kind":            func(v *FactoryIdentityInput) { v.Kind = "mysql" },
		"identity schema": func(v *FactoryIdentityInput) { v.IdentitySchema = "factory.v2" },
		"field name":      func(v *FactoryIdentityInput) { v.Fields[0].Name = "digest" },
		"field value":     func(v *FactoryIdentityInput) { v.Fields[0].Value = "postgres:18" },
		"field addition": func(v *FactoryIdentityInput) {
			v.Fields = append(v.Fields, ResolvedField{Name: "platform", Value: "linux/amd64"})
		},
		"field removal": func(v *FactoryIdentityInput) { v.Fields = nil },
	}
	for name, mutate := range mutations {
		t.Run(name, func(t *testing.T) {
			changed := base
			changed.Fields = append([]ResolvedField(nil), base.Fields...)
			mutate(&changed)
			gotRoot, gotEndpoint := buildFactoryLineageIDs(t, changed)
			if gotRoot == wantRoot || gotEndpoint == wantEndpoint {
				t.Fatalf("factory mutation did not propagate: root %s, endpoint %s", gotRoot, gotEndpoint)
			}
		})
	}
}

func TestPhysicalMetadataIsRejectedByPublicJSONShapes(t *testing.T) {
	baseFactory, baseTransform := requirementProvenance(t)
	factoryDeclaration := &FactoryDeclaration{Kind: "docker", Reference: "postgres:17", Arguments: []string{}, Attributes: map[string]string{}}
	transformDeclaration := &TransformDeclaration{Kind: "psql", Reference: "one.sql", Arguments: []string{}, Attributes: map[string]string{}}
	resolver := &ResolverObservation{Implementation: "builtin", Version: "v1"}
	factory, _ := NewFactoryProvenance(FactoryProvenanceInput{Identity: baseFactory.Identity(), Declaration: factoryDeclaration, Resolver: resolver})
	transform, _ := NewTransformProvenance(TransformProvenanceInput{Identity: baseTransform.Identity(), Declaration: transformDeclaration, Resolver: resolver})
	recipe, _ := NewRecipe(factory, []TransformProvenance{transform})
	lineage, _ := Build(recipe)
	relative, _ := Extend(lineage.Root().ID(), []TransformProvenance{transform})
	step := lineage.Steps()[0]
	physical := []string{"runtime_id", "job_id", "timestamp", "physical_path", "snapshot_path", "checkpoint_backend", "materialization"}
	shapes := []struct {
		name   string
		value  any
		target func() any
	}{
		{"factory identity", factory.Identity(), func() any { return &ResolvedFactoryIdentity{} }},
		{"transform identity", transform.Identity(), func() any { return &ResolvedTransformIdentity{} }},
		{"factory declaration", *factoryDeclaration, func() any { return &FactoryDeclaration{} }},
		{"transform declaration", *transformDeclaration, func() any { return &TransformDeclaration{} }},
		{"factory provenance", factory, func() any { return &FactoryProvenance{} }},
		{"transform provenance", transform, func() any { return &TransformProvenance{} }},
		{"factory state", lineage.Root(), func() any { return &State{} }},
		{"derived state", step.State(), func() any { return &State{} }},
		{"lineage step", step, func() any { return &LineageStep{} }},
		{"recipe", recipe, func() any { return &Recipe{} }},
		{"recipe lineage", lineage, func() any { return &RecipeLineage{} }},
		{"relative lineage", relative, func() any { return &RelativeLineage{} }},
	}
	for _, shape := range shapes {
		rawShape, _ := json.Marshal(shape.value)
		for _, key := range physical {
			t.Run(shape.name+"/"+key, func(t *testing.T) {
				if strings.Contains(string(rawShape), `"`+key+`"`) {
					t.Fatalf("marshaled semantic value contains physical metadata %q", key)
				}
				object := mustMap(t, shape.value)
				object[key] = "physical-value"
				raw, _ := json.Marshal(object)
				if err := json.Unmarshal(raw, shape.target()); !errors.Is(err, ErrInvalid) {
					t.Fatalf("physical metadata %q accepted: %v", key, err)
				}
			})
		}
	}
}

func TestLimitMinusOneLimitAndLimitPlusOne(t *testing.T) {
	for _, size := range []int{MaxIdentifierBytes - 1, MaxIdentifierBytes, MaxIdentifierBytes + 1} {
		input := FactoryIdentityInput{SchemaVersion: SchemaVersion, Provider: "a" + strings.Repeat("b", size-1), Kind: "kind", IdentitySchema: "v1"}
		_, err := NewFactoryIdentity(input)
		assertLimitResult(t, "identifier", size, MaxIdentifierBytes, err)
	}
	for _, size := range []int{MaxResolvedValueBytes - 1, MaxResolvedValueBytes, MaxResolvedValueBytes + 1} {
		input := TransformIdentityInput{SchemaVersion: SchemaVersion, Provider: "sqlrs", Kind: "psql", IdentitySchema: "v1", Fields: []ResolvedField{{Name: "sql", Value: utf8Bytes(size)}}}
		_, err := NewTransformIdentity(input)
		assertLimitResult(t, "resolved value", size, MaxResolvedValueBytes, err)
	}
	for _, count := range []int{MaxResolvedFields - 1, MaxResolvedFields, MaxResolvedFields + 1} {
		fields := make([]ResolvedField, count)
		for index := range fields {
			fields[index] = ResolvedField{Name: "f" + itoa(index), Value: "v"}
		}
		_, err := NewTransformIdentity(TransformIdentityInput{SchemaVersion: SchemaVersion, Provider: "sqlrs", Kind: "psql", IdentitySchema: "v1", Fields: fields})
		assertLimitResult(t, "resolved fields", count, MaxResolvedFields, err)
	}

	factory, transform := requirementProvenance(t)
	for _, count := range []int{MaxTransforms - 1, MaxTransforms, MaxTransforms + 1} {
		transforms := make([]TransformProvenance, count)
		for index := range transforms {
			transforms[index] = transform
		}
		_, err := NewRecipe(factory, transforms)
		assertLimitResult(t, "transforms", count, MaxTransforms, err)
		_, err = Extend(StateID("sha256:"+strings.Repeat("0", 64)), transforms)
		assertLimitResult(t, "relative transforms", count, MaxTransforms, err)
	}

	for _, count := range []int{MaxArguments - 1, MaxArguments, MaxArguments + 1} {
		arguments := make([]string, count)
		for index := range arguments {
			arguments[index] = "x"
		}
		_, err := json.Marshal(TransformDeclaration{Kind: "psql", Reference: "x", Arguments: arguments, Attributes: map[string]string{}})
		assertLimitResult(t, "arguments", count, MaxArguments, err)
	}
	for _, count := range []int{MaxAttributes - 1, MaxAttributes, MaxAttributes + 1} {
		attributes := make(map[string]string, count)
		for index := 0; index < count; index++ {
			attributes["a"+itoa(index)] = "x"
		}
		_, err := json.Marshal(TransformDeclaration{Kind: "psql", Reference: "x", Arguments: []string{}, Attributes: attributes})
		assertLimitResult(t, "attributes", count, MaxAttributes, err)
	}
	for _, size := range []int{MaxResolvedValueBytes - 1, MaxResolvedValueBytes, MaxResolvedValueBytes + 1} {
		value := utf8Bytes(size)
		declarations := []TransformDeclaration{
			{Kind: "psql", Reference: value, Arguments: []string{}, Attributes: map[string]string{}},
			{Kind: "psql", Reference: "x", Arguments: []string{value}, Attributes: map[string]string{}},
			{Kind: "psql", Reference: "x", Arguments: []string{}, Attributes: map[string]string{"note": value}},
		}
		for index, declaration := range declarations {
			_, err := json.Marshal(declaration)
			assertLimitResult(t, "diagnostic value "+itoa(index), size, MaxResolvedValueBytes, err)
		}
	}
	for _, size := range []int{MaxIdentifierBytes - 1, MaxIdentifierBytes, MaxIdentifierBytes + 1} {
		identifier := "a" + strings.Repeat("b", size-1)
		_, err := json.Marshal(TransformDeclaration{Kind: "psql", Reference: "x", Arguments: []string{}, Attributes: map[string]string{identifier: "x"}})
		assertLimitResult(t, "attribute key", size, MaxIdentifierBytes, err)
		_, err = NewTransformProvenance(TransformProvenanceInput{Identity: transform.Identity(), Resolver: &ResolverObservation{Implementation: "builtin", Version: identifier}})
		assertLimitResult(t, "resolver version", size, MaxIdentifierBytes, err)
	}

	validJSON := []byte(`{"schema_version":"sqlrs.runtime.v2","provider":"sqlrs","kind":"psql","identity_schema":"v1","fields":[]}`)
	for _, size := range []int{MaxJSONBytes - 1, MaxJSONBytes, MaxJSONBytes + 1} {
		raw := append(append([]byte(nil), validJSON...), []byte(strings.Repeat(" ", size-len(validJSON)))...)
		var target ResolvedTransformIdentity
		err := target.UnmarshalJSON(raw)
		assertLimitResult(t, "JSON bytes", size, MaxJSONBytes, err)
	}
}

func TestDecodedValuesDoNotAliasSourceBytes(t *testing.T) {
	_, transform := requirementProvenance(t)
	raw, err := json.Marshal(transform.Identity())
	if err != nil {
		t.Fatal(err)
	}
	var decoded ResolvedTransformIdentity
	if err := json.Unmarshal(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	want, _ := TransformFingerprint(decoded)
	for index := range raw {
		raw[index] = 'x'
	}
	got, err := TransformFingerprint(decoded)
	if err != nil || got != want {
		t.Fatalf("source mutation changed decoded identity: %s, %v", got, err)
	}
}

func buildFactoryLineageIDs(t *testing.T, input FactoryIdentityInput) (StateID, StateID) {
	t.Helper()
	identity, err := NewFactoryIdentity(input)
	if err != nil {
		t.Fatal(err)
	}
	factory, err := NewFactoryProvenance(FactoryProvenanceInput{Identity: identity})
	if err != nil {
		t.Fatal(err)
	}
	_, transform := requirementProvenance(t)
	recipe, err := NewRecipe(factory, []TransformProvenance{transform})
	if err != nil {
		t.Fatal(err)
	}
	lineage, err := Build(recipe)
	if err != nil {
		t.Fatal(err)
	}
	return lineage.Root().ID(), lineage.Endpoint().ID()
}

func assertLimitResult(t *testing.T, name string, got, limit int, err error) {
	t.Helper()
	if got <= limit && err != nil {
		t.Fatalf("%s size %d rejected: %v", name, got, err)
	}
	if got > limit && !errors.Is(err, ErrInvalid) {
		t.Fatalf("%s size %d accepted or returned unstructured error: %v", name, got, err)
	}
}

func utf8Bytes(size int) string {
	value := strings.Repeat("é", size/2)
	if size%2 != 0 {
		value += "a"
	}
	return value
}
