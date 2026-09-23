package runtimev2

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestAllPublicJSONShapesRoundTrip(t *testing.T) {
	factoryIdentity, err := NewFactoryIdentity(FactoryIdentityInput{
		SchemaVersion: SchemaVersion, Provider: "sqlrs", Kind: "postgres", IdentitySchema: "factory.v1",
		Fields: []ResolvedField{{Name: "image", Value: "postgres:17"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	transformIdentity, err := NewTransformIdentity(TransformIdentityInput{
		SchemaVersion: SchemaVersion, Provider: "sqlrs", Kind: "psql", IdentitySchema: "transform.v1",
		Fields: []ResolvedField{{Name: "sql", Value: "select 1"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	factoryDeclaration := &FactoryDeclaration{Kind: "docker", Reference: "postgres:17", Arguments: []string{"--x"}, Attributes: map[string]string{"note": "factory"}}
	transformDeclaration := &TransformDeclaration{Kind: "psql", Reference: "one.sql", Arguments: []string{"-v"}, Attributes: map[string]string{"note": "transform"}}
	resolver := &ResolverObservation{Implementation: "builtin", Version: "v1"}
	factory, err := NewFactoryProvenance(FactoryProvenanceInput{Identity: factoryIdentity, Declaration: factoryDeclaration, Resolver: resolver})
	if err != nil {
		t.Fatal(err)
	}
	transform, err := NewTransformProvenance(TransformProvenanceInput{Identity: transformIdentity, Declaration: transformDeclaration, Resolver: resolver})
	if err != nil {
		t.Fatal(err)
	}

	for name, pair := range map[string]struct{ value, target any }{
		"factory identity":      {factoryIdentity, &ResolvedFactoryIdentity{}},
		"transform identity":    {transformIdentity, &ResolvedTransformIdentity{}},
		"factory declaration":   {*factoryDeclaration, &FactoryDeclaration{}},
		"transform declaration": {*transformDeclaration, &TransformDeclaration{}},
		"factory provenance":    {factory, &FactoryProvenance{}},
		"transform provenance":  {transform, &TransformProvenance{}},
	} {
		t.Run(name, func(t *testing.T) {
			raw, err := json.Marshal(pair.value)
			if err != nil {
				t.Fatal(err)
			}
			if err := json.Unmarshal(raw, pair.target); err != nil {
				t.Fatal(err)
			}
		})
	}

	recipe, err := NewRecipe(factory, []TransformProvenance{transform})
	if err != nil {
		t.Fatal(err)
	}
	var decodedRecipe Recipe
	roundTrip(t, recipe, &decodedRecipe)
	lineage, err := Build(decodedRecipe)
	if err != nil {
		t.Fatal(err)
	}
	var decodedLineage RecipeLineage
	roundTrip(t, lineage, &decodedLineage)
	step := lineage.Steps()[0]
	var decodedStep LineageStep
	roundTrip(t, step, &decodedStep)
	relative, err := Extend(lineage.Root().ID(), []TransformProvenance{transform})
	if err != nil {
		t.Fatal(err)
	}
	var decodedRelative RelativeLineage
	roundTrip(t, relative, &decodedRelative)

	if len(factoryIdentity.Fields()) != 1 || len(transformIdentity.Fields()) != 1 ||
		factoryIdentity.SchemaVersion() != SchemaVersion || factoryIdentity.Provider() != "sqlrs" ||
		factoryIdentity.Kind() != "postgres" || factoryIdentity.IdentitySchema() != "factory.v1" ||
		transformIdentity.SchemaVersion() != SchemaVersion || transformIdentity.Provider() != "sqlrs" ||
		transformIdentity.Kind() != "psql" || transformIdentity.IdentitySchema() != "transform.v1" ||
		factory.Identity().Fields()[0].Name != "image" || transform.Identity().Fields()[0].Name != "sql" ||
		factory.Declaration().Reference != "postgres:17" || transform.Declaration().Reference != "one.sql" ||
		factory.Resolver().Implementation != "builtin" || transform.Resolver().Version != "v1" ||
		recipe.Factory().Identity().Kind() != "postgres" || len(recipe.Transforms()) != 1 ||
		decodedLineage.Factory().Identity().Provider() != "sqlrs" || decodedRelative.Anchor() != lineage.Root().ID() ||
		decodedStep.Transform().Identity().Fields()[0].Name != "sql" || decodedStep.Fingerprint() == "" ||
		decodedLineage.Root().FactoryFingerprint() == "" || decodedRelative.EndpointID() == "" {
		t.Fatal("public accessor lost semantic data")
	}

	// Constructors and accessors own their collections.
	factoryDeclaration.Arguments[0] = "changed"
	factoryDeclaration.Attributes["note"] = "changed"
	copyDeclaration := factory.Declaration()
	copyDeclaration.Arguments[0] = "changed-again"
	copyDeclaration.Attributes["note"] = "changed-again"
	copyResolver := factory.Resolver()
	copyResolver.Version = "changed"
	if factory.Declaration().Arguments[0] != "--x" || factory.Declaration().Attributes["note"] != "factory" {
		t.Fatal("declaration storage is mutable through caller data")
	}
	if factory.Resolver().Version != "v1" {
		t.Fatal("resolver storage is mutable through accessor")
	}
	fields := transformIdentity.Fields()
	fields[0].Value = "changed"
	if transformIdentity.Fields()[0].Value != "select 1" {
		t.Fatal("identity storage is mutable through accessor")
	}
}

func TestInvalidConstructorsAndZeroValues(t *testing.T) {
	badIdentityInputs := []FactoryIdentityInput{
		{SchemaVersion: "wrong", Provider: "sqlrs", Kind: "psql", IdentitySchema: "v1"},
		{SchemaVersion: SchemaVersion, Provider: "BAD", Kind: "psql", IdentitySchema: "v1"},
		{SchemaVersion: SchemaVersion, Provider: "sqlrs", Kind: "BAD", IdentitySchema: "v1"},
		{SchemaVersion: SchemaVersion, Provider: "sqlrs", Kind: "psql", IdentitySchema: "BAD"},
		{SchemaVersion: SchemaVersion, Provider: "sqlrs", Kind: "psql", IdentitySchema: "v1", Fields: []ResolvedField{{Name: "x", Value: ""}}},
		{SchemaVersion: SchemaVersion, Provider: "sqlrs", Kind: "psql", IdentitySchema: "v1", Fields: []ResolvedField{{Name: "x", Value: "1"}, {Name: "x", Value: "2"}}},
	}
	for index, input := range badIdentityInputs {
		if value, err := NewFactoryIdentity(input); err == nil || value.valid() {
			t.Fatalf("factory identity case %d accepted", index)
		}
	}

	invalidJSONValues := []any{
		ResolvedFactoryIdentity{}, ResolvedTransformIdentity{}, FactoryProvenance{}, TransformProvenance{},
		State{}, LineageStep{}, Recipe{}, RecipeLineage{}, RelativeLineage{},
	}
	for _, value := range invalidJSONValues {
		if _, err := json.Marshal(value); !errors.Is(err, ErrInvalid) {
			t.Fatalf("zero value %T marshal error = %v", value, err)
		}
	}
	if DecodeJSON([]byte(`{}`), nil) == nil || (&ValidationError{}).Error() == "" || (*ValidationError)(nil).Error() == "" {
		t.Fatal("error contract not enforced")
	}

	if (ResolvedFactoryIdentity{}).Fields() != nil || (ResolvedTransformIdentity{}).Fields() != nil ||
		(ResolvedFactoryIdentity{}).SchemaVersion() != "" || (ResolvedFactoryIdentity{}).Provider() != "" ||
		(ResolvedFactoryIdentity{}).Kind() != "" || (ResolvedFactoryIdentity{}).IdentitySchema() != "" ||
		(ResolvedTransformIdentity{}).SchemaVersion() != "" || (ResolvedTransformIdentity{}).Provider() != "" ||
		(ResolvedTransformIdentity{}).Kind() != "" || (ResolvedTransformIdentity{}).IdentitySchema() != "" ||
		(FactoryProvenance{}).Identity().valid() || (TransformProvenance{}).Identity().valid() ||
		(FactoryProvenance{}).Declaration() != nil || (TransformProvenance{}).Declaration() != nil ||
		(FactoryProvenance{}).Resolver() != nil || (TransformProvenance{}).Resolver() != nil ||
		(Recipe{}).Factory().data != nil || (Recipe{}).Transforms() != nil ||
		(LineageStep{}).Transform().data != nil || (LineageStep{}).Fingerprint() != "" || (LineageStep{}).State().data != nil ||
		(RecipeLineage{}).Factory().data != nil || (RecipeLineage{}).Root().data != nil || (RecipeLineage{}).Steps() != nil || (RecipeLineage{}).Endpoint().data != nil ||
		(RelativeLineage{}).Anchor() != "" || (RelativeLineage{}).Steps() != nil || (RelativeLineage{}).EndpointID() != "" ||
		(State{}).ID() != "" || (State{}).ParentID() != "" || (State{}).FactoryFingerprint() != "" || (State{}).TransformFingerprint() != "" {
		t.Fatal("zero-value accessors are not inert")
	}

	if _, err := NewFactoryProvenance(FactoryProvenanceInput{}); err == nil {
		t.Fatal("zero factory identity accepted")
	}
	if _, err := NewTransformProvenance(TransformProvenanceInput{}); err == nil {
		t.Fatal("zero transform identity accepted")
	}
	if _, err := NewRecipe(FactoryProvenance{}, nil); err == nil {
		t.Fatal("zero factory provenance accepted")
	}
	if _, err := Build(Recipe{}); err == nil {
		t.Fatal("zero recipe accepted")
	}
	if _, err := FactoryState(FactoryProvenance{}); err == nil {
		t.Fatal("zero factory provenance produced a state")
	}
	if _, err := TransformFingerprint(ResolvedTransformIdentity{}); err == nil {
		t.Fatal("zero transform identity produced a fingerprint")
	}
	if _, err := Derive("bad", TransformProvenance{}); err == nil {
		t.Fatal("invalid parent accepted")
	}
	if _, err := Derive(StateID("sha256:"+strings.Repeat("0", 64)), TransformProvenance{}); err == nil {
		t.Fatal("zero transform provenance accepted")
	}
	if _, err := Extend("bad", nil); err == nil {
		t.Fatal("invalid relative anchor accepted")
	}
}

func TestStrictJSONShapeAndIntegrityMatrix(t *testing.T) {
	identityBase := `"schema_version":"sqlrs.runtime.v2","provider":"sqlrs","kind":"psql","identity_schema":"v1","fields":[]`
	identityCases := []string{
		`{}`, `{"provider":"sqlrs"}`, `{"schema_version":"sqlrs.runtime.v2"}`,
		`{"schema_version":"sqlrs.runtime.v2","provider":"sqlrs"}`,
		`{"schema_version":"sqlrs.runtime.v2","provider":"sqlrs","kind":"psql"}`,
		`{"schema_version":"sqlrs.runtime.v2","provider":"sqlrs","kind":"psql","identity_schema":"v1"}`,
		`{` + identityBase + `,"fields":[]}`, `[` + `{` + identityBase + `}` + `]`,
	}
	for index, raw := range identityCases {
		var value ResolvedTransformIdentity
		if err := DecodeJSON([]byte(raw), &value); !errors.Is(err, ErrInvalid) {
			t.Fatalf("identity case %d error = %v", index, err)
		}
	}

	declarationCases := []string{
		`{}`, `{"kind":"psql"}`, `{"kind":"psql","reference":"x"}`,
		`{"kind":"psql","reference":"x","arguments":[]}`,
		`{"kind":"BAD","reference":"x","arguments":[],"attributes":{}}`,
	}
	for index, raw := range declarationCases {
		var value TransformDeclaration
		if err := DecodeJSON([]byte(raw), &value); !errors.Is(err, ErrInvalid) {
			t.Fatalf("declaration case %d error = %v", index, err)
		}
	}

	provenanceCases := []string{
		`{}`, `{"identity":null}`, `{"identity":{},"declaration":null}`,
		`{"identity":{` + identityBase + `},"resolver":null}`,
		`{"identity":{` + identityBase + `},"resolver":{}}`,
		`{"identity":{` + identityBase + `},"resolver":{"implementation":"builtin"}}`,
		`{"identity":{` + identityBase + `},"resolver":{"implementation":"BAD","version":"v1"}}`,
	}
	for index, raw := range provenanceCases {
		var value TransformProvenance
		if err := DecodeJSON([]byte(raw), &value); !errors.Is(err, ErrInvalid) {
			t.Fatalf("provenance case %d error = %v", index, err)
		}
	}

	stateCases := []string{
		`{}`, `{"schema_version":"wrong"}`,
		`{"schema_version":"sqlrs.runtime.v2"}`,
		`{"schema_version":"sqlrs.runtime.v2","state_kind":"factory"}`,
		`{"schema_version":"sqlrs.runtime.v2","state_kind":"other","id":"sha256:` + strings.Repeat("0", 64) + `"}`,
		`{"schema_version":"sqlrs.runtime.v2","state_kind":"factory","id":"sha256:` + strings.Repeat("0", 64) + `","factory_fingerprint":null}`,
		`{"schema_version":"sqlrs.runtime.v2","state_kind":"factory","id":"sha256:` + strings.Repeat("0", 64) + `","factory_fingerprint":"sha256:` + strings.Repeat("1", 64) + `"}`,
	}
	for index, raw := range stateCases {
		var value State
		if err := DecodeJSON([]byte(raw), &value); !errors.Is(err, ErrInvalid) {
			t.Fatalf("state case %d error = %v", index, err)
		}
	}
}

func TestBoundsAndDigestParsing(t *testing.T) {
	validDigest := "sha256:" + strings.Repeat("a", 64)
	if raw, err := parseDigest(validDigest, "id"); err != nil || len(raw) != 32 {
		t.Fatalf("valid digest: %d %v", len(raw), err)
	}
	for _, value := range []string{"", "sha512:" + strings.Repeat("a", 64), "sha256:" + strings.Repeat("A", 64), "sha256:" + strings.Repeat("g", 64), validDigest + "0"} {
		if _, err := parseDigest(value, "id"); err == nil {
			t.Fatalf("invalid digest accepted: %q", value)
		}
	}
	if validateUTF8("", "x", true, 1) != nil || validateUTF8("é", "x", true, 1) == nil || validateUTF8(string([]byte{0xff}), "x", true, 2) == nil {
		t.Fatal("UTF-8 byte limits not enforced")
	}

	tooManyArguments := make([]string, MaxArguments+1)
	tooManyAttributes := make(map[string]string, MaxAttributes+1)
	for index := 0; index < MaxAttributes+1; index++ {
		tooManyAttributes["a"+itoa(index)] = "x"
	}
	for _, declaration := range []TransformDeclaration{
		{Kind: "psql", Reference: "x", Arguments: tooManyArguments, Attributes: map[string]string{}},
		{Kind: "psql", Reference: "x", Arguments: []string{}, Attributes: tooManyAttributes},
		{Kind: "psql", Reference: "x", Arguments: []string{""}, Attributes: map[string]string{}},
		{Kind: "psql", Reference: "x", Arguments: []string{}, Attributes: map[string]string{"BAD": "x"}},
	} {
		if _, err := json.Marshal(declaration); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid declaration accepted: %v", err)
		}
	}
	oversized := append([]byte(`{"schema_version":"sqlrs.runtime.v2"}`), make([]byte, MaxJSONBytes)...)
	var identity ResolvedTransformIdentity
	if err := DecodeJSON(oversized, &identity); !errors.Is(err, ErrInvalid) {
		t.Fatalf("oversized JSON error = %v", err)
	}
}

func TestNilUnmarshalReceivers(t *testing.T) {
	var factoryIdentity *ResolvedFactoryIdentity
	var transformIdentity *ResolvedTransformIdentity
	var factoryDeclaration *FactoryDeclaration
	var transformDeclaration *TransformDeclaration
	var factoryProvenance *FactoryProvenance
	var transformProvenance *TransformProvenance
	var state *State
	var step *LineageStep
	var recipe *Recipe
	var recipeLineage *RecipeLineage
	var relativeLineage *RelativeLineage
	for name, call := range map[string]func([]byte) error{
		"factory identity":      factoryIdentity.UnmarshalJSON,
		"transform identity":    transformIdentity.UnmarshalJSON,
		"factory declaration":   factoryDeclaration.UnmarshalJSON,
		"transform declaration": transformDeclaration.UnmarshalJSON,
		"factory provenance":    factoryProvenance.UnmarshalJSON,
		"transform provenance":  transformProvenance.UnmarshalJSON,
		"state":                 state.UnmarshalJSON,
		"step":                  step.UnmarshalJSON,
		"recipe":                recipe.UnmarshalJSON,
		"recipe lineage":        recipeLineage.UnmarshalJSON,
		"relative lineage":      relativeLineage.UnmarshalJSON,
	} {
		if err := call([]byte(`{}`)); !errors.Is(err, ErrInvalid) {
			t.Fatalf("%s nil receiver error = %v", name, err)
		}
	}
}

func TestLineageJSONRejectionMatrix(t *testing.T) {
	factory, transform := requirementProvenance(t)
	recipe, _ := NewRecipe(factory, []TransformProvenance{transform})
	lineage, _ := Build(recipe)
	relative, _ := Extend(lineage.Root().ID(), []TransformProvenance{transform})
	step := lineage.Steps()[0]

	testMutations(t, "step", step, func() any { return &LineageStep{} }, []func(map[string]any){
		func(v map[string]any) { delete(v, "transform") },
		func(v map[string]any) { v["transform"] = nil },
		func(v map[string]any) { delete(v, "transform_fingerprint") },
		func(v map[string]any) { delete(v, "state") },
		func(v map[string]any) { v["state"] = nil },
		func(v map[string]any) { v["transform"] = map[string]any{} },
		func(v map[string]any) { v["state"] = map[string]any{} },
		func(v map[string]any) { v["state"] = mustMap(t, lineage.Root()) },
		func(v map[string]any) { v["transform_fingerprint"] = "sha256:" + strings.Repeat("0", 64) },
		func(v map[string]any) { v["state"].(map[string]any)["id"] = "sha256:" + strings.Repeat("0", 64) },
	})

	testMutations(t, "recipe", recipe, func() any { return &Recipe{} }, []func(map[string]any){
		func(v map[string]any) { delete(v, "schema_version") },
		func(v map[string]any) { v["schema_version"] = "wrong" },
		func(v map[string]any) { delete(v, "factory") },
		func(v map[string]any) { v["factory"] = map[string]any{} },
		func(v map[string]any) { delete(v, "transforms") },
		func(v map[string]any) { v["transforms"] = []any{map[string]any{}} },
	})

	testMutations(t, "recipe lineage", lineage, func() any { return &RecipeLineage{} }, []func(map[string]any){
		func(v map[string]any) { delete(v, "schema_version") },
		func(v map[string]any) { v["schema_version"] = "wrong" },
		func(v map[string]any) { delete(v, "factory") },
		func(v map[string]any) { v["factory"] = map[string]any{} },
		func(v map[string]any) { delete(v, "root") },
		func(v map[string]any) { v["root"] = map[string]any{} },
		func(v map[string]any) { v["root"] = mustMap(t, lineage.Steps()[0].State()) },
		func(v map[string]any) {
			v["root"].(map[string]any)["id"] = "sha256:" + strings.Repeat("0", 64)
			v["root"].(map[string]any)["factory_fingerprint"] = "sha256:" + strings.Repeat("0", 64)
		},
		func(v map[string]any) { delete(v, "steps") },
		func(v map[string]any) { v["steps"] = []any{map[string]any{}} },
		func(v map[string]any) {
			v["steps"].([]any)[0].(map[string]any)["state"].(map[string]any)["parent_id"] = "sha256:" + strings.Repeat("1", 64)
		},
	})

	testMutations(t, "relative lineage", relative, func() any { return &RelativeLineage{} }, []func(map[string]any){
		func(v map[string]any) { delete(v, "schema_version") },
		func(v map[string]any) { v["schema_version"] = "wrong" },
		func(v map[string]any) { delete(v, "anchor") },
		func(v map[string]any) { v["anchor"] = "bad" },
		func(v map[string]any) { delete(v, "steps") },
		func(v map[string]any) { v["steps"] = []any{map[string]any{}} },
		func(v map[string]any) {
			v["steps"].([]any)[0].(map[string]any)["state"].(map[string]any)["parent_id"] = "sha256:" + strings.Repeat("1", 64)
		},
	})
}

func TestStateUnionRejectionMatrix(t *testing.T) {
	factory, transform := requirementProvenance(t)
	root, _ := FactoryState(factory)
	step, _ := Derive(root.ID(), transform)
	testMutations(t, "factory state", root, func() any { return &State{} }, []func(map[string]any){
		func(v map[string]any) { v["id"] = "bad" },
		func(v map[string]any) { v["factory_fingerprint"] = 1 },
		func(v map[string]any) { v["parent_id"] = root.ID() },
	})
	testMutations(t, "derived state", step.State(), func() any { return &State{} }, []func(map[string]any){
		func(v map[string]any) { v["factory_fingerprint"] = root.ID() },
		func(v map[string]any) { delete(v, "parent_id") },
		func(v map[string]any) { v["parent_id"] = nil },
		func(v map[string]any) { v["parent_id"] = 1 },
		func(v map[string]any) { v["parent_id"] = "bad" },
		func(v map[string]any) { delete(v, "transform_fingerprint") },
		func(v map[string]any) { v["transform_fingerprint"] = nil },
		func(v map[string]any) { v["transform_fingerprint"] = 1 },
		func(v map[string]any) { v["transform_fingerprint"] = "bad" },
		func(v map[string]any) { v["id"] = root.ID() },
	})
}

func TestConstructorBoundsAndDiagnosticErrors(t *testing.T) {
	factory, transform := requirementProvenance(t)
	tooManyTransforms := make([]TransformProvenance, MaxTransforms+1)
	if _, err := NewRecipe(factory, tooManyTransforms); !errors.Is(err, ErrInvalid) {
		t.Fatalf("oversized recipe error = %v", err)
	}
	if _, err := Extend(StateID("sha256:"+strings.Repeat("0", 64)), tooManyTransforms); !errors.Is(err, ErrInvalid) {
		t.Fatalf("oversized relative lineage error = %v", err)
	}
	if _, err := NewRecipe(factory, []TransformProvenance{{}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("nil transform recipe error = %v", err)
	}
	if _, err := Extend(StateID("sha256:"+strings.Repeat("0", 64)), []TransformProvenance{{}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("nil transform suffix error = %v", err)
	}
	brokenRecipe := Recipe{data: &recipeData{factory: factory, transforms: []TransformProvenance{{}}}}
	if _, err := Build(brokenRecipe); !errors.Is(err, ErrInvalid) {
		t.Fatalf("broken recipe build error = %v", err)
	}

	identity := transform.Identity()
	badDeclarations := []*TransformDeclaration{
		{Kind: "BAD", Reference: "x", Arguments: []string{}, Attributes: map[string]string{}},
		{Kind: "psql", Reference: "", Arguments: []string{}, Attributes: map[string]string{}},
	}
	for _, declaration := range badDeclarations {
		if _, err := NewTransformProvenance(TransformProvenanceInput{Identity: identity, Declaration: declaration}); !errors.Is(err, ErrInvalid) {
			t.Fatalf("bad declaration error = %v", err)
		}
	}
	if _, err := NewTransformProvenance(TransformProvenanceInput{Identity: identity, Resolver: &ResolverObservation{Implementation: "BAD", Version: "v1"}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad resolver error = %v", err)
	}
	if _, err := NewFactoryProvenance(FactoryProvenanceInput{Identity: factory.Identity(), Declaration: &FactoryDeclaration{Kind: "BAD"}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad factory declaration error = %v", err)
	}
	if _, err := NewFactoryProvenance(FactoryProvenanceInput{Identity: factory.Identity(), Resolver: &ResolverObservation{Implementation: "BAD", Version: "v1"}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("bad factory resolver error = %v", err)
	}
}

func TestMalformedJSONIsStructuredForEveryDecoder(t *testing.T) {
	decoders := map[string]json.Unmarshaler{
		"factory identity":      &ResolvedFactoryIdentity{},
		"transform identity":    &ResolvedTransformIdentity{},
		"factory declaration":   &FactoryDeclaration{},
		"transform declaration": &TransformDeclaration{},
		"factory provenance":    &FactoryProvenance{},
		"transform provenance":  &TransformProvenance{},
		"state":                 &State{},
		"step":                  &LineageStep{},
		"recipe":                &Recipe{},
		"recipe lineage":        &RecipeLineage{},
		"relative lineage":      &RelativeLineage{},
	}
	for name, decoder := range decoders {
		if err := DecodeJSON([]byte(`{"broken"`), decoder); !errors.Is(err, ErrInvalid) {
			t.Fatalf("%s malformed JSON error = %v", name, err)
		}
	}
}

func TestFactoryProvenanceRejectionAndRemainingValidationBranches(t *testing.T) {
	factory, transform := requirementProvenance(t)
	testMutations(t, "factory provenance", factory, func() any { return &FactoryProvenance{} }, []func(map[string]any){
		func(v map[string]any) { delete(v, "identity") },
		func(v map[string]any) { v["identity"] = nil },
		func(v map[string]any) { v["identity"] = map[string]any{} },
		func(v map[string]any) { v["declaration"] = nil },
		func(v map[string]any) { v["declaration"] = map[string]any{} },
		func(v map[string]any) { v["resolver"] = map[string]any{} },
	})

	validNilCollections := FactoryDeclaration{Kind: "docker", Reference: "postgres:17"}
	if _, err := json.Marshal(validNilCollections); err != nil {
		t.Fatalf("nil diagnostic collections should normalize: %v", err)
	}
	if _, err := json.Marshal(FactoryDeclaration{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid factory declaration marshal error = %v", err)
	}
	if _, err := json.Marshal(FactoryDeclaration{Kind: "docker", Reference: "x", Attributes: map[string]string{"note": strings.Repeat("x", MaxResolvedValueBytes+1)}}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("oversized diagnostic attribute error = %v", err)
	}

	input := TransformIdentityInput{SchemaVersion: SchemaVersion, Provider: "sqlrs", Kind: "psql", IdentitySchema: "v1", Fields: []ResolvedField{{Name: "x", Value: strings.Repeat("x", MaxResolvedValueBytes+1)}}}
	if _, err := NewTransformIdentity(input); !errors.Is(err, ErrInvalid) {
		t.Fatalf("oversized resolved value error = %v", err)
	}

	corruptTransform := transform
	corruptTransform.data.identity = ResolvedTransformIdentity{}
	if _, err := Derive(StateID("sha256:"+strings.Repeat("0", 64)), corruptTransform); !errors.Is(err, ErrInvalid) {
		t.Fatalf("corrupt transform error = %v", err)
	}

	if got := invalid(CodeInvalidShape, ""); !errors.Is(got, ErrInvalid) {
		t.Fatalf("empty error path did not normalize: %v", got)
	}
	sentinel := errors.New("sentinel")
	if prefixError(sentinel, "x") != sentinel || prefixError(&ValidationError{Code: CodeInvalidShape, Path: "$"}, "x").(*ValidationError).Path != "x" {
		t.Fatal("error prefix behavior differs")
	}
}

func requirementProvenance(t *testing.T) (FactoryProvenance, TransformProvenance) {
	t.Helper()
	factoryIdentity, err := NewFactoryIdentity(FactoryIdentityInput{SchemaVersion: SchemaVersion, Provider: "sqlrs", Kind: "postgres", IdentitySchema: "factory.v1", Fields: []ResolvedField{}})
	if err != nil {
		t.Fatal(err)
	}
	transformIdentity, err := NewTransformIdentity(TransformIdentityInput{SchemaVersion: SchemaVersion, Provider: "sqlrs", Kind: "psql", IdentitySchema: "transform.v1", Fields: []ResolvedField{}})
	if err != nil {
		t.Fatal(err)
	}
	factory, err := NewFactoryProvenance(FactoryProvenanceInput{Identity: factoryIdentity})
	if err != nil {
		t.Fatal(err)
	}
	transform, err := NewTransformProvenance(TransformProvenanceInput{Identity: transformIdentity})
	if err != nil {
		t.Fatal(err)
	}
	return factory, transform
}

func mustMap(t *testing.T, value any) map[string]any {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func testMutations(t *testing.T, name string, value any, target func() any, mutations []func(map[string]any)) {
	t.Helper()
	base := mustMap(t, value)
	for index, mutate := range mutations {
		t.Run(name+"/"+itoa(index), func(t *testing.T) {
			rawBase, _ := json.Marshal(base)
			var changed map[string]any
			_ = json.Unmarshal(rawBase, &changed)
			mutate(changed)
			raw, _ := json.Marshal(changed)
			if err := json.Unmarshal(raw, target()); !errors.Is(err, ErrInvalid) {
				t.Fatalf("mutation %d accepted or returned unstructured error: %v\n%s", index, err, raw)
			}
		})
	}
}

func roundTrip(t *testing.T, value, target any) {
	t.Helper()
	raw, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		t.Fatal(err)
	}
}
