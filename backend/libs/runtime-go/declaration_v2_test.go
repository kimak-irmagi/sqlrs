package runtimev2_test

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
)

func TestVersionedDeclarationDocumentsAndRecipe(t *testing.T) {
	input := workspaceInput(t, "migrations/main.sql")
	environment := executionEnvironment(t)
	deployment := deployment(t)
	capability := capability(t)
	portability := portability(t)
	factory := runtimev2.FactoryDeclaration{
		Kind: "docker", Reference: "postgres:17", Arguments: []string{}, Attributes: map[string]string{},
		Inputs: []runtimev2.InputDeclaration{input}, ExecutionEnvironment: &environment,
		Deployment: &deployment, Capabilities: []runtimev2.CapabilityObservation{capability},
		Portability: []runtimev2.PortabilityObservation{portability},
	}
	transform := runtimev2.TransformDeclaration{
		Kind: "psql", Reference: "migrations/main.sql", Arguments: []string{}, Attributes: map[string]string{},
		Inputs: []runtimev2.InputDeclaration{input}, ExecutionEnvironment: &environment,
		Capabilities: []runtimev2.CapabilityObservation{capability}, Portability: []runtimev2.PortabilityObservation{portability},
	}

	factoryDocument, err := runtimev2.NewFactoryDeclarationDocument(factory)
	if err != nil {
		t.Fatal(err)
	}
	transformDocument, err := runtimev2.NewTransformDeclarationDocument(transform)
	if err != nil {
		t.Fatal(err)
	}
	recipe, err := runtimev2.NewRecipeDeclaration(factory, []runtimev2.TransformDeclaration{transform})
	if err != nil {
		t.Fatal(err)
	}
	for name, test := range map[string]struct {
		value  any
		target any
	}{
		"factory":   {factoryDocument, &runtimev2.FactoryDeclarationDocument{}},
		"transform": {transformDocument, &runtimev2.TransformDeclarationDocument{}},
		"recipe":    {recipe, &runtimev2.RecipeDeclaration{}},
	} {
		t.Run(name, func(t *testing.T) {
			raw, err := json.Marshal(test.value)
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(string(raw), `"schema_version":"sqlrs.runtime.v2"`) {
				t.Fatalf("missing version: %s", raw)
			}
			if err := runtimev2.DecodeJSON(raw, test.target.(json.Unmarshaler)); err != nil {
				t.Fatal(err)
			}
		})
	}

	gotFactory := factoryDocument.Declaration()
	gotFactory.Reference = "changed"
	gotFactory.Inputs[0] = runtimev2.InputDeclaration{}
	if factoryDocument.Declaration().Reference != factory.Reference || factoryDocument.Declaration().Inputs[0].Kind() != "file" {
		t.Fatal("factory document accessor leaked mutable state")
	}
	transforms := recipe.Transforms()
	transforms[0].Reference = "changed"
	if recipe.Factory().Reference != factory.Reference || recipe.Transforms()[0].Reference != transform.Reference {
		t.Fatal("recipe accessor leaked mutable state")
	}
}

func TestVersionedDeclarationStrictDecodingIsAtomic(t *testing.T) {
	valid, err := runtimev2.NewFactoryDeclarationDocument(runtimev2.FactoryDeclaration{Kind: "docker", Reference: "postgres:17"})
	if err != nil {
		t.Fatal(err)
	}
	want := valid.Declaration().Reference
	cases := []string{
		`{}`,
		`{"schema_version":"wrong","declaration":{"kind":"docker","reference":"x","arguments":[],"attributes":{}}}`,
		`{"schema_version":"sqlrs.runtime.v2","schema_version":"sqlrs.runtime.v2","declaration":{}}`,
		`{"schema_version":"sqlrs.runtime.v2","declaration":null}`,
		`{"schema_version":"sqlrs.runtime.v2","declaration":{"kind":"docker","reference":"x","arguments":[],"attributes":{}},"extra":true}`,
		`{"schema_version":"sqlrs.runtime.v2","declaration":{"kind":"docker","reference":"x","arguments":[],"attributes":{}}} true`,
	}
	for _, raw := range cases {
		if err := runtimev2.DecodeJSON([]byte(raw), &valid); !errors.Is(err, runtimev2.ErrInvalid) {
			t.Fatalf("accepted %s: %v", raw, err)
		}
		if valid.Declaration().Reference != want {
			t.Fatal("failed decode mutated receiver")
		}
	}
	invalidUTF8 := append([]byte(`{"schema_version":"sqlrs.runtime.v2","declaration":"`), 0xff)
	if err := runtimev2.DecodeJSON(invalidUTF8, &valid); !errors.Is(err, runtimev2.ErrInvalid) {
		t.Fatalf("invalid UTF-8 error = %v", err)
	}
}

func TestTypedExtensionRolesCanonicalizationAndIsolation(t *testing.T) {
	base := runtimev2.ExtensionSpecificationInput{
		SchemaVersion: runtimev2.SchemaVersion, Owner: "example", Kind: "artifact",
		SpecificationSchema: "example.artifact.v1",
		Fields:              []runtimev2.DeclarationField{{Name: "z", Value: "last"}, {Name: "a", Value: "first"}},
	}
	input, err := runtimev2.NewInputDeclaration(base)
	if err != nil {
		t.Fatal(err)
	}
	environment, err := runtimev2.NewExecutionEnvironmentDeclaration(base)
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := runtimev2.NewDeploymentDeclaration(base)
	if err != nil {
		t.Fatal(err)
	}
	if input.Role() == environment.Role() || input.Role() == deployment.Role() || environment.Role() == deployment.Role() {
		t.Fatal("typed declaration roles collapsed")
	}
	if got := input.Fields(); len(got) != 2 || got[0].Name != "a" || got[1].Name != "z" {
		t.Fatalf("fields not canonical: %+v", got)
	}
	base.Fields[0].Value = "mutated"
	fields := input.Fields()
	fields[0].Value = "mutated"
	if input.Fields()[0].Value != "first" {
		t.Fatal("extension declaration aliases caller state")
	}

	for _, mutate := range []func(*runtimev2.ExtensionSpecificationInput){
		func(v *runtimev2.ExtensionSpecificationInput) { v.SchemaVersion = "wrong" },
		func(v *runtimev2.ExtensionSpecificationInput) { v.Owner = "BAD" },
		func(v *runtimev2.ExtensionSpecificationInput) { v.Fields = append(v.Fields, v.Fields[0]) },
	} {
		changed := base
		changed.Fields = []runtimev2.DeclarationField{{Name: "a", Value: "first"}}
		mutate(&changed)
		if _, err := runtimev2.NewInputDeclaration(changed); !errors.Is(err, runtimev2.ErrInvalid) {
			t.Fatalf("invalid extension accepted: %v", err)
		}
	}
}

func TestResolvedExtensionFingerprintAndComposition(t *testing.T) {
	base := runtimev2.ResolvedExtensionIdentityInput{
		SchemaVersion: runtimev2.SchemaVersion, Owner: "sqlrs.workspace", Kind: "file",
		IdentitySchema: "sqlrs.workspace-file.v1",
		Fields:         []runtimev2.ResolvedField{{Name: "content.digest", Value: "sha256:" + strings.Repeat("a", 64)}},
	}
	identity, err := runtimev2.NewResolvedExtensionIdentity(base)
	if err != nil {
		t.Fatal(err)
	}
	fingerprint, err := runtimev2.ExtensionFingerprint(identity)
	if err != nil || fingerprint == "" {
		t.Fatalf("fingerprint = %q, %v", fingerprint, err)
	}
	if want := runtimev2.Fingerprint("sha256:11ec555cb44c71f50efc9eaea6ed65f52e6d9fbdbef05502d6e80f550ce203d3"); fingerprint != want {
		t.Fatalf("extension golden fingerprint = %s, want %s", fingerprint, want)
	}
	for name, mutate := range map[string]func(*runtimev2.ResolvedExtensionIdentityInput){
		"owner":  func(v *runtimev2.ResolvedExtensionIdentityInput) { v.Owner = "other" },
		"kind":   func(v *runtimev2.ResolvedExtensionIdentityInput) { v.Kind = "archive" },
		"schema": func(v *runtimev2.ResolvedExtensionIdentityInput) { v.IdentitySchema = "sqlrs.workspace-file.v2" },
		"field": func(v *runtimev2.ResolvedExtensionIdentityInput) {
			v.Fields[0].Value = "sha256:" + strings.Repeat("b", 64)
		},
	} {
		t.Run(name, func(t *testing.T) {
			changed := base
			changed.Fields = append([]runtimev2.ResolvedField(nil), base.Fields...)
			mutate(&changed)
			value, err := runtimev2.NewResolvedExtensionIdentity(changed)
			if err != nil {
				t.Fatal(err)
			}
			got, _ := runtimev2.ExtensionFingerprint(value)
			if got == fingerprint {
				t.Fatal("identity mutation did not change fingerprint")
			}
		})
	}

	fields, err := runtimev2.ComposeResolvedFields(
		[]runtimev2.ResolvedField{{Name: "mode", Value: "strict"}},
		[]runtimev2.ExtensionBinding{{Name: "source", Identity: identity}},
	)
	if err != nil {
		t.Fatal(err)
	}
	if len(fields) != 2 || fields[0].Name != "extension.source" || fields[0].Value != string(fingerprint) || fields[1].Name != "mode" {
		t.Fatalf("composed fields = %+v", fields)
	}
	for _, test := range []struct {
		base     []runtimev2.ResolvedField
		bindings []runtimev2.ExtensionBinding
	}{
		{base: []runtimev2.ResolvedField{{Name: "extension.source", Value: "x"}}},
		{bindings: []runtimev2.ExtensionBinding{{Name: "source", Identity: identity}, {Name: "source", Identity: identity}}},
		{bindings: []runtimev2.ExtensionBinding{{Name: "BAD", Identity: identity}}},
	} {
		if _, err := runtimev2.ComposeResolvedFields(test.base, test.bindings); !errors.Is(err, runtimev2.ErrInvalid) {
			t.Fatalf("invalid composition accepted: %v", err)
		}
	}
}

func TestDiagnosticsNeverBecomeIdentity(t *testing.T) {
	capability := capability(t)
	portability := portability(t)
	if capability.Role() != "capability" || portability.Role() != "portability" {
		t.Fatal("diagnostic roles collapsed")
	}
	identity := resolvedWorkspaceInput(t, "a")
	want, _ := runtimev2.ExtensionFingerprint(identity)
	factory := runtimev2.FactoryDeclaration{Kind: "docker", Reference: "postgres:17", Capabilities: []runtimev2.CapabilityObservation{capability}, Portability: []runtimev2.PortabilityObservation{portability}}
	document, err := runtimev2.NewFactoryDeclarationDocument(factory)
	if err != nil {
		t.Fatal(err)
	}
	factory.Capabilities = nil
	factory.Portability = nil
	other, err := runtimev2.NewFactoryDeclarationDocument(factory)
	if err != nil {
		t.Fatal(err)
	}
	if document.Declaration().Reference != other.Declaration().Reference {
		t.Fatal("diagnostics changed declaration semantics")
	}
	got, _ := runtimev2.ExtensionFingerprint(identity)
	if got != want {
		t.Fatal("diagnostics changed extension identity")
	}
}

func workspaceInput(t *testing.T, path string) runtimev2.InputDeclaration {
	t.Helper()
	value, err := runtimev2.NewInputDeclaration(runtimev2.ExtensionSpecificationInput{
		SchemaVersion: runtimev2.SchemaVersion, Owner: "sqlrs.workspace", Kind: "file",
		SpecificationSchema: "sqlrs.workspace-file.declaration.v1",
		Fields:              []runtimev2.DeclarationField{{Name: "path", Value: path}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func executionEnvironment(t *testing.T) runtimev2.ExecutionEnvironmentDeclaration {
	t.Helper()
	value, err := runtimev2.NewExecutionEnvironmentDeclaration(runtimev2.ExtensionSpecificationInput{
		SchemaVersion: runtimev2.SchemaVersion, Owner: "example", Kind: "oci",
		SpecificationSchema: "example.oci.v1", Fields: []runtimev2.DeclarationField{{Name: "image", Value: "example@sha256:abc"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func deployment(t *testing.T) runtimev2.DeploymentDeclaration {
	t.Helper()
	value, err := runtimev2.NewDeploymentDeclaration(runtimev2.ExtensionSpecificationInput{
		SchemaVersion: runtimev2.SchemaVersion, Owner: "example", Kind: "local",
		SpecificationSchema: "example.local.v1", Fields: []runtimev2.DeclarationField{{Name: "profile", Value: "test"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func capability(t *testing.T) runtimev2.CapabilityObservation {
	t.Helper()
	value, err := runtimev2.NewCapabilityObservation(runtimev2.DiagnosticObservationInput{
		SchemaVersion: runtimev2.SchemaVersion, Owner: "example", Kind: "filesystem",
		ObservationSchema: "example.filesystem.v1", Fields: []runtimev2.DeclarationField{{Name: "class", Value: "local"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func portability(t *testing.T) runtimev2.PortabilityObservation {
	t.Helper()
	value, err := runtimev2.NewPortabilityObservation(runtimev2.DiagnosticObservationInput{
		SchemaVersion: runtimev2.SchemaVersion, Owner: "example", Kind: "platform",
		ObservationSchema: "example.platform.v1", Fields: []runtimev2.DeclarationField{{Name: "os", Value: "portable"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func resolvedWorkspaceInput(t *testing.T, seed string) runtimev2.ResolvedExtensionIdentity {
	t.Helper()
	value, err := runtimev2.NewResolvedExtensionIdentity(runtimev2.ResolvedExtensionIdentityInput{
		SchemaVersion: runtimev2.SchemaVersion, Owner: "sqlrs.workspace", Kind: "file",
		IdentitySchema: "sqlrs.workspace-file.v1", Fields: []runtimev2.ResolvedField{{Name: "content.digest", Value: "sha256:" + strings.Repeat(seed, 64)}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}
