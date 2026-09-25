package runtimev2

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestAllExtensionRoleAccessorsAndJSON(t *testing.T) {
	input := ExtensionSpecificationInput{SchemaVersion: SchemaVersion, Owner: "owner", Kind: "kind", SpecificationSchema: "owner.kind.v1", Fields: []DeclarationField{}}
	in, _ := NewInputDeclaration(input)
	environment, _ := NewExecutionEnvironmentDeclaration(input)
	deployment, _ := NewDeploymentDeclaration(input)
	values := []struct {
		name                         string
		value                        json.Marshaler
		target                       json.Unmarshaler
		role                         func() string
		version, owner, kind, schema func() string
		fields                       func() []DeclarationField
	}{
		{"input", in, &InputDeclaration{}, in.Role, in.SchemaVersion, in.Owner, in.Kind, in.SpecificationSchema, in.Fields},
		{"environment", environment, &ExecutionEnvironmentDeclaration{}, environment.Role, environment.SchemaVersion, environment.Owner, environment.Kind, environment.SpecificationSchema, environment.Fields},
		{"deployment", deployment, &DeploymentDeclaration{}, deployment.Role, deployment.SchemaVersion, deployment.Owner, deployment.Kind, deployment.SpecificationSchema, deployment.Fields},
	}
	for _, test := range values {
		t.Run(test.name, func(t *testing.T) {
			if test.role() == "" || test.version() != SchemaVersion || test.owner() != "owner" || test.kind() != "kind" || test.schema() != "owner.kind.v1" || len(test.fields()) != 0 {
				t.Fatal("accessor mismatch")
			}
			raw, err := test.value.MarshalJSON()
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(raw), `"fields":[]`) {
				t.Fatalf("fields not canonical: %s", raw)
			}
			if err := test.target.UnmarshalJSON(raw); err != nil {
				t.Fatal(err)
			}
		})
	}

	zeroes := []json.Marshaler{InputDeclaration{}, ExecutionEnvironmentDeclaration{}, DeploymentDeclaration{}}
	for _, zero := range zeroes {
		if _, err := zero.MarshalJSON(); !errors.Is(err, ErrInvalid) {
			t.Fatalf("zero marshal=%v", err)
		}
	}
	var nilInput *InputDeclaration
	var nilEnvironment *ExecutionEnvironmentDeclaration
	var nilDeployment *DeploymentDeclaration
	for _, call := range []func([]byte) error{nilInput.UnmarshalJSON, nilEnvironment.UnmarshalJSON, nilDeployment.UnmarshalJSON} {
		if err := call([]byte(`{}`)); !errors.Is(err, ErrInvalid) {
			t.Fatalf("nil receiver=%v", err)
		}
	}
	invalid := []string{`{}`, `{"schema_version":"sqlrs.runtime.v2"}`, `{"schema_version":"sqlrs.runtime.v2","owner":"owner","kind":"kind","specification_schema":"owner.kind.v1","fields":null}`, `{"schema_version":"wrong","owner":"owner","kind":"kind","specification_schema":"owner.kind.v1","fields":[]}`}
	for _, raw := range invalid {
		var value InputDeclaration
		if err := DecodeJSON([]byte(raw), &value); !errors.Is(err, ErrInvalid) {
			t.Fatalf("accepted %s: %v", raw, err)
		}
	}
}

func TestAllDiagnosticAccessorsAndJSON(t *testing.T) {
	input := DiagnosticObservationInput{SchemaVersion: SchemaVersion, Owner: "owner", Kind: "kind", ObservationSchema: "owner.observation.v1", Fields: []DeclarationField{}}
	capability, _ := NewCapabilityObservation(input)
	portability, _ := NewPortabilityObservation(input)
	if capability.SchemaVersion() != SchemaVersion || capability.Owner() != "owner" || capability.Kind() != "kind" || capability.ObservationSchema() != "owner.observation.v1" || len(capability.Fields()) != 0 || capability.Role() != "capability" {
		t.Fatal("capability accessors")
	}
	if portability.SchemaVersion() != SchemaVersion || portability.Owner() != "owner" || portability.Kind() != "kind" || portability.ObservationSchema() != "owner.observation.v1" || len(portability.Fields()) != 0 || portability.Role() != "portability" {
		t.Fatal("portability accessors")
	}
	for _, pair := range []struct {
		value  json.Marshaler
		target json.Unmarshaler
	}{{capability, &CapabilityObservation{}}, {portability, &PortabilityObservation{}}} {
		raw, err := pair.value.MarshalJSON()
		if err != nil {
			t.Fatal(err)
		}
		if err := pair.target.UnmarshalJSON(raw); err != nil {
			t.Fatal(err)
		}
	}
	for _, zero := range []json.Marshaler{CapabilityObservation{}, PortabilityObservation{}} {
		if _, err := zero.MarshalJSON(); !errors.Is(err, ErrInvalid) {
			t.Fatalf("zero=%v", err)
		}
	}
	var nilCapability *CapabilityObservation
	var nilPortability *PortabilityObservation
	if !errors.Is(nilCapability.UnmarshalJSON([]byte(`{}`)), ErrInvalid) || !errors.Is(nilPortability.UnmarshalJSON([]byte(`{}`)), ErrInvalid) {
		t.Fatal("nil diagnostic receiver")
	}
	for _, mutate := range []func(*DiagnosticObservationInput){func(v *DiagnosticObservationInput) { v.SchemaVersion = "bad" }, func(v *DiagnosticObservationInput) { v.Owner = "BAD" }, func(v *DiagnosticObservationInput) { v.Kind = "BAD" }, func(v *DiagnosticObservationInput) { v.ObservationSchema = "BAD" }, func(v *DiagnosticObservationInput) { v.Fields = []DeclarationField{{Name: "x", Value: ""}} }} {
		changed := input
		mutate(&changed)
		if _, err := NewCapabilityObservation(changed); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid diagnostic=%v", err)
		}
	}
	for _, raw := range []string{`{}`, `{"schema_version":"sqlrs.runtime.v2","owner":"owner","kind":"kind","observation_schema":"owner.v1","fields":null}`} {
		var value CapabilityObservation
		if err := DecodeJSON([]byte(raw), &value); !errors.Is(err, ErrInvalid) {
			t.Fatalf("accepted diagnostic %s", raw)
		}
	}
}

func TestResolvedExtensionJSONAndCompositionBranches(t *testing.T) {
	input := ResolvedExtensionIdentityInput{SchemaVersion: SchemaVersion, Owner: "owner", Kind: "kind", IdentitySchema: "owner.kind.v1", Fields: []ResolvedField{}}
	identity, _ := NewResolvedExtensionIdentity(input)
	if identity.SchemaVersion() != SchemaVersion || identity.Owner() != "owner" || identity.Kind() != "kind" || identity.IdentitySchema() != "owner.kind.v1" || len(identity.Fields()) != 0 {
		t.Fatal("identity accessors")
	}
	raw, err := json.Marshal(identity)
	if err != nil {
		t.Fatal(err)
	}
	var decoded ResolvedExtensionIdentity
	if err := DecodeJSON(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	var nilIdentity *ResolvedExtensionIdentity
	if !errors.Is(nilIdentity.UnmarshalJSON(raw), ErrInvalid) {
		t.Fatal("nil identity receiver")
	}
	if _, err := (ResolvedExtensionIdentity{}).MarshalJSON(); !errors.Is(err, ErrInvalid) {
		t.Fatal("zero identity marshal")
	}
	invalid := []string{`{}`, `{"schema_version":"sqlrs.runtime.v2","owner":"owner","kind":"kind","identity_schema":"owner.v1","fields":null}`}
	for _, text := range invalid {
		if err := DecodeJSON([]byte(text), &decoded); !errors.Is(err, ErrInvalid) {
			t.Fatalf("accepted %s", text)
		}
	}
	for _, mutate := range []func(*ResolvedExtensionIdentityInput){func(v *ResolvedExtensionIdentityInput) { v.SchemaVersion = "bad" }, func(v *ResolvedExtensionIdentityInput) { v.Owner = "BAD" }, func(v *ResolvedExtensionIdentityInput) { v.Kind = "BAD" }, func(v *ResolvedExtensionIdentityInput) { v.IdentitySchema = "BAD" }, func(v *ResolvedExtensionIdentityInput) { v.Fields = []ResolvedField{{Name: "x", Value: ""}} }, func(v *ResolvedExtensionIdentityInput) {
		v.Fields = []ResolvedField{{Name: "x", Value: "1"}, {Name: "x", Value: "2"}}
	}, func(v *ResolvedExtensionIdentityInput) { v.Fields = make([]ResolvedField, MaxResolvedFields+1) }} {
		changed := input
		mutate(&changed)
		if _, err := NewResolvedExtensionIdentity(changed); !errors.Is(err, ErrInvalid) {
			t.Fatalf("invalid identity=%v", err)
		}
	}
	if _, err := ExtensionFingerprint(ResolvedExtensionIdentity{}); !errors.Is(err, ErrInvalid) {
		t.Fatal("zero fingerprint")
	}
	if _, err := ComposeResolvedFields([]ResolvedField{{Name: "x", Value: "1"}, {Name: "x", Value: "2"}}, nil); !errors.Is(err, ErrInvalid) {
		t.Fatal("duplicate base")
	}
	if _, err := ComposeResolvedFields([]ResolvedField{{Name: "x", Value: ""}}, nil); !errors.Is(err, ErrInvalid) {
		t.Fatal("empty base")
	}
	if _, err := ComposeResolvedFields(nil, []ExtensionBinding{{Name: "x", Identity: ResolvedExtensionIdentity{}}}); !errors.Is(err, ErrInvalid) {
		t.Fatal("zero binding")
	}
	if _, err := ComposeResolvedFields(make([]ResolvedField, MaxResolvedFields), []ExtensionBinding{{Name: "x", Identity: identity}}); !errors.Is(err, ErrInvalid) {
		t.Fatal("oversized composition")
	}
}

func TestVersionedDocumentAndRecipeFailureBranches(t *testing.T) {
	validFactory := FactoryDeclaration{Kind: "docker", Reference: "image"}
	validTransform := TransformDeclaration{Kind: "psql", Reference: "one.sql"}
	transformDocument, _ := NewTransformDeclarationDocument(validTransform)
	if transformDocument.Declaration().Reference != "one.sql" {
		t.Fatal("transform accessor")
	}
	raw, _ := json.Marshal(transformDocument)
	var decoded TransformDeclarationDocument
	if err := DecodeJSON(raw, &decoded); err != nil {
		t.Fatal(err)
	}
	if (FactoryDeclarationDocument{}).Declaration().Kind != "" || (TransformDeclarationDocument{}).Declaration().Kind != "" {
		t.Fatal("zero accessor")
	}
	if _, err := json.Marshal(FactoryDeclarationDocument{}); !errors.Is(err, ErrInvalid) {
		t.Fatal("zero factory doc")
	}
	if _, err := json.Marshal(TransformDeclarationDocument{}); !errors.Is(err, ErrInvalid) {
		t.Fatal("zero transform doc")
	}
	var nilFactory *FactoryDeclarationDocument
	var nilTransform *TransformDeclarationDocument
	var nilRecipe *RecipeDeclaration
	if !errors.Is(nilFactory.UnmarshalJSON([]byte(`{}`)), ErrInvalid) || !errors.Is(nilTransform.UnmarshalJSON([]byte(`{}`)), ErrInvalid) || !errors.Is(nilRecipe.UnmarshalJSON([]byte(`{}`)), ErrInvalid) {
		t.Fatal("nil document receiver")
	}
	recipe, _ := NewRecipeDeclaration(validFactory, []TransformDeclaration{})
	if len(recipe.Transforms()) != 0 {
		t.Fatal("empty recipe")
	}
	recipeRaw, _ := json.Marshal(recipe)
	var decodedRecipe RecipeDeclaration
	if err := DecodeJSON(recipeRaw, &decodedRecipe); err != nil {
		t.Fatal(err)
	}
	if (RecipeDeclaration{}).Factory().Kind != "" || (RecipeDeclaration{}).Transforms() != nil {
		t.Fatal("zero recipe accessors")
	}
	if _, err := json.Marshal(RecipeDeclaration{}); !errors.Is(err, ErrInvalid) {
		t.Fatal("zero recipe marshal")
	}
	tooMany := make([]TransformDeclaration, MaxTransforms+1)
	if _, err := NewRecipeDeclaration(validFactory, tooMany); !errors.Is(err, ErrInvalid) {
		t.Fatal("oversized recipe")
	}
	if _, err := NewFactoryDeclarationDocument(FactoryDeclaration{}); !errors.Is(err, ErrInvalid) {
		t.Fatal("invalid factory")
	}
	if _, err := NewTransformDeclarationDocument(TransformDeclaration{}); !errors.Is(err, ErrInvalid) {
		t.Fatal("invalid transform")
	}
}

func TestZeroValueAccessorsAndMissingWireFields(t *testing.T) {
	zeroSpecifications := []interface {
		SchemaVersion() string
		Owner() string
		Kind() string
		SpecificationSchema() string
		Fields() []DeclarationField
	}{InputDeclaration{}, ExecutionEnvironmentDeclaration{}, DeploymentDeclaration{}}
	for _, value := range zeroSpecifications {
		if value.SchemaVersion() != "" || value.Owner() != "" || value.Kind() != "" || value.SpecificationSchema() != "" || value.Fields() != nil {
			t.Fatal("non-empty zero specification")
		}
	}
	zeroDiagnostics := []interface {
		SchemaVersion() string
		Owner() string
		Kind() string
		ObservationSchema() string
		Fields() []DeclarationField
	}{CapabilityObservation{}, PortabilityObservation{}}
	for _, value := range zeroDiagnostics {
		if value.SchemaVersion() != "" || value.Owner() != "" || value.Kind() != "" || value.ObservationSchema() != "" || value.Fields() != nil {
			t.Fatal("non-empty zero diagnostic")
		}
	}
	zeroIdentity := ResolvedExtensionIdentity{}
	if zeroIdentity.SchemaVersion() != "" || zeroIdentity.Owner() != "" || zeroIdentity.Kind() != "" || zeroIdentity.IdentitySchema() != "" || zeroIdentity.Fields() != nil {
		t.Fatal("non-empty zero extension identity")
	}
	if (*diagnosticObservationData)(nil).clone() != nil || (*extensionSpecificationData)(nil).clone() != nil {
		t.Fatal("nil clone became non-nil")
	}

	for _, raw := range []string{
		`{}`,
		`{"schema_version":"sqlrs.runtime.v2"}`,
		`{"schema_version":"sqlrs.runtime.v2","owner":"owner"}`,
		`{"schema_version":"sqlrs.runtime.v2","owner":"owner","kind":"kind"}`,
		`{"schema_version":"sqlrs.runtime.v2","owner":"owner","kind":"kind","specification_schema":"owner.kind.v1"}`,
	} {
		var value InputDeclaration
		if err := json.Unmarshal([]byte(raw), &value); !errors.Is(err, ErrInvalid) {
			t.Errorf("specification accepted %s: %v", raw, err)
		}
	}
	for _, raw := range []string{
		`{"schema_version":"sqlrs.runtime.v2"}`,
		`{"schema_version":"sqlrs.runtime.v2","owner":"owner"}`,
		`{"schema_version":"sqlrs.runtime.v2","owner":"owner","kind":"kind"}`,
		`{"schema_version":"sqlrs.runtime.v2","owner":"owner","kind":"kind","observation_schema":"owner.kind.v1"}`,
	} {
		var value CapabilityObservation
		if err := json.Unmarshal([]byte(raw), &value); !errors.Is(err, ErrInvalid) {
			t.Errorf("diagnostic accepted %s: %v", raw, err)
		}
	}
}

func TestDeclarationExtensionValidationBranches(t *testing.T) {
	if !errors.Is(validateFactoryDeclaration(nil), ErrInvalid) || !errors.Is(validateTransformDeclaration(nil), ErrInvalid) {
		t.Fatal("nil declaration accepted")
	}
	baseFactory := FactoryDeclaration{Kind: "factory", Reference: "ref", Arguments: []string{}, Attributes: map[string]string{}}
	if err := validateFactoryDeclaration(&FactoryDeclaration{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid factory base: %v", err)
	}
	if err := validateTransformDeclaration(&TransformDeclaration{}); !errors.Is(err, ErrInvalid) {
		t.Fatalf("invalid transform base: %v", err)
	}

	tests := []struct {
		name   string
		mutate func(*FactoryDeclaration)
	}{
		{"inputs too large", func(v *FactoryDeclaration) { v.Inputs = make([]InputDeclaration, MaxResolvedFields+1) }},
		{"empty input", func(v *FactoryDeclaration) { v.Inputs = []InputDeclaration{{}} }},
		{"empty environment", func(v *FactoryDeclaration) { v.ExecutionEnvironment = &ExecutionEnvironmentDeclaration{} }},
		{"capabilities too large", func(v *FactoryDeclaration) { v.Capabilities = make([]CapabilityObservation, MaxResolvedFields+1) }},
		{"empty capability", func(v *FactoryDeclaration) { v.Capabilities = []CapabilityObservation{{}} }},
		{"portability too large", func(v *FactoryDeclaration) { v.Portability = make([]PortabilityObservation, MaxResolvedFields+1) }},
		{"empty portability", func(v *FactoryDeclaration) { v.Portability = []PortabilityObservation{{}} }},
		{"empty deployment", func(v *FactoryDeclaration) { v.Deployment = &DeploymentDeclaration{} }},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			value := baseFactory
			test.mutate(&value)
			if err := validateFactoryDeclaration(&value); !errors.Is(err, ErrInvalid) {
				t.Fatalf("accepted: %v", err)
			}
		})
	}
}
