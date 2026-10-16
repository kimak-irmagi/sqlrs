package sqlrs_test

import (
	"bytes"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"sync"
	"testing"
	"unicode/utf8"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
	"github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/schemaauthor"
	"github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/schemas/sqlrs"
)

const executionDigest = "sha256:1111111111111111111111111111111111111111111111111111111111111111"

// ES01-ES02: public signatures and target-platform-independent profiles.
var (
	_ func() runtimev2.ExtensionIdentitySchema                                                    = sqlrs.ExecutionEnvironmentSchema
	_ func(sqlrs.ExecutionEnvironmentInput) (runtimev2.CanonicalResolvedExtensionIdentity, error) = sqlrs.BuildExecutionEnvironmentIdentity
	_ func(runtimev2.CanonicalResolvedExtensionIdentity) (sqlrs.ExecutionEnvironmentInput, error) = sqlrs.ParseExecutionEnvironmentIdentity
)

func executionBase() sqlrs.ExecutionEnvironmentInput {
	return sqlrs.ExecutionEnvironmentInput{
		Runtime: sqlrs.ExecutionRuntime{Kind: sqlrs.ExecutionRuntimeOCI, IdentityKind: sqlrs.ExecutionIdentityImageManifest, IdentityDigest: executionDigest, Platform: sqlrs.ExecutionPlatform{OS: "linux", Architecture: "amd64"}},
		Argv:    []sqlrs.ExecutionText{executionLiteral("tool")},
		Workdir: sqlrs.ExecutionPath{Root: "image", Relative: "work"}, Network: sqlrs.ExecutionNetworkNone,
	}
}

func executionLiteral(text string) sqlrs.ExecutionText {
	return sqlrs.ExecutionText{{Kind: sqlrs.ExecutionTextPartLiteral, Literal: text}}
}

func executionPathWord(root, relative string) sqlrs.ExecutionText {
	return sqlrs.ExecutionText{{Kind: sqlrs.ExecutionTextPartPath, Path: &sqlrs.ExecutionPath{Root: root, Relative: relative}, PathFormat: sqlrs.ExecutionPathNative}}
}

func executionResources(t testing.TB) sqlrs.ExecutionEnvironmentInput {
	t.Helper()
	v := executionBase()
	ref, err := runtimev2.NewSecretReference("vault", "seed-reference", "v1")
	if err != nil {
		t.Fatal(err)
	}
	v.Inputs = []sqlrs.ExecutionInputBinding{{Name: "project", Source: sqlrs.ExecutionInputReference{Provider: "files", Kind: "tree", IdentitySchema: "files.tree.v1", Fingerprint: runtimev2.CanonicalFingerprint(executionDigest)}, Profile: sqlrs.ExecutionInputRegularTree}}
	v.Mounts = []sqlrs.ExecutionMount{{Name: "project", InputName: "project", Kind: sqlrs.ExecutionMountInput, Target: sqlrs.ExecutionPath{Root: "image", Relative: "work"}, ReadOnly: true}, {Name: "scratch", Kind: sqlrs.ExecutionMountScratch, Target: sqlrs.ExecutionPath{Root: "image", Relative: "work/tmp"}}, {Name: "config", Kind: sqlrs.ExecutionMountSecret, SecretSlot: "seed", Target: sqlrs.ExecutionPath{Root: "image", Relative: "secrets/config"}, ReadOnly: true}}
	v.Workdir = sqlrs.ExecutionPath{Root: "mount:project"}
	v.Argv = append(v.Argv, append(executionLiteral("--file="), executionPathWord("mount:project", "change.sql")...), executionPathWord("mount:config", ""))
	v.Services = []sqlrs.ExecutionServiceBinding{{Role: "database", Protocol: "postgresql", Database: "app", User: "app", Transport: sqlrs.ExecutionServiceTCP, AuthenticationSecret: "auth", AuthenticationProfile: "postgresql.env.v1"}}
	v.Secrets = []sqlrs.ExecutionSecretBinding{{Name: "auth", Purpose: sqlrs.ExecutionSecretAuthentication, ServiceRole: "database"}, {Name: "seed", Purpose: sqlrs.ExecutionSecretSemantic, IdentityReference: &ref}}
	v.Environment = []sqlrs.ExecutionEnvironmentVariable{{Name: "DATABASE", Value: sqlrs.ExecutionText{{Kind: sqlrs.ExecutionTextPartService, ServiceRole: "database", ServiceField: sqlrs.ExecutionServiceDatabase}}}, {Name: "SEED", Value: sqlrs.ExecutionText{{Kind: sqlrs.ExecutionTextPartSecret, SecretSlot: "seed"}}}}
	return v
}

func executionBuild(t testing.TB, input sqlrs.ExecutionEnvironmentInput) runtimev2.CanonicalResolvedExtensionIdentity {
	t.Helper()
	identity, err := sqlrs.BuildExecutionEnvironmentIdentity(input)
	if err != nil {
		t.Fatal(err)
	}
	return identity
}

func executionFingerprint(t testing.TB, identity runtimev2.CanonicalResolvedExtensionIdentity) runtimev2.CanonicalFingerprint {
	t.Helper()
	fingerprint, err := runtimev2.CanonicalExtensionFingerprint(identity)
	if err != nil {
		t.Fatal(err)
	}
	return fingerprint
}

func TestExecutionProfilesAndCompatibility(t *testing.T) {
	for _, kind := range []sqlrs.ExecutionRuntimeKind{sqlrs.ExecutionRuntimeHost, sqlrs.ExecutionRuntimeOCI, sqlrs.ExecutionRuntimeDatabaseContainer} {
		for _, identityKind := range []sqlrs.ExecutionRuntimeIdentityKind{sqlrs.ExecutionIdentityHostExecutable, sqlrs.ExecutionIdentityHostBundle, sqlrs.ExecutionIdentityImageManifest} {
			for _, network := range []sqlrs.ExecutionNetworkMode{sqlrs.ExecutionNetworkNone, sqlrs.ExecutionNetworkServices, sqlrs.ExecutionNetworkLocalContext} {
				t.Run(string(kind)+"/"+string(identityKind)+"/"+string(network), func(t *testing.T) {
					input := executionBase()
					input.Runtime.Kind, input.Runtime.IdentityKind, input.Network = kind, identityKind, network
					if kind == sqlrs.ExecutionRuntimeHost {
						input.User = "uid:1000"
						input.Workdir.Root = "workspace"
						input.Argv[0] = executionPathWord("tool", "")
						if identityKind == sqlrs.ExecutionIdentityHostBundle {
							input.Argv[0][0].Path.Relative = "bin/tool"
						}
					}
					if network == sqlrs.ExecutionNetworkServices || kind == sqlrs.ExecutionRuntimeDatabaseContainer {
						input.Services = []sqlrs.ExecutionServiceBinding{{Role: "database", Protocol: "postgresql", Database: "app", User: "app", Transport: sqlrs.ExecutionServiceUnixSocket}}
					}
					if kind == sqlrs.ExecutionRuntimeDatabaseContainer {
						input.Runtime.DatabaseService = "database"
					}
					allowed := (kind == sqlrs.ExecutionRuntimeHost && identityKind != sqlrs.ExecutionIdentityImageManifest || kind != sqlrs.ExecutionRuntimeHost && identityKind == sqlrs.ExecutionIdentityImageManifest) && (network != sqlrs.ExecutionNetworkLocalContext || identityKind == sqlrs.ExecutionIdentityHostExecutable && kind == sqlrs.ExecutionRuntimeHost)
					identity, err := sqlrs.BuildExecutionEnvironmentIdentity(input)
					raw := executionGeneric(t, executionMatrixFields(input))
					read, readErr := sqlrs.ParseExecutionEnvironmentIdentity(raw)
					if !allowed {
						executionInvalid(t, identity, err)
						var validation *runtimev2.ValidationError
						if !errors.As(readErr, &validation) || !reflect.DeepEqual(read, sqlrs.ExecutionEnvironmentInput{}) {
							t.Fatalf("Parse accepted rejected combination: %v", readErr)
						}
						return
					}
					if err != nil {
						t.Fatal(err)
					}
					if readErr != nil || executionFingerprint(t, raw) != executionFingerprint(t, identity) {
						t.Fatalf("independent profile representation: %v", readErr)
					}
					parsed, err := sqlrs.ParseExecutionEnvironmentIdentity(identity)
					if err != nil {
						t.Fatal(err)
					}
					if executionFingerprint(t, identity) != executionFingerprint(t, executionBuild(t, parsed)) {
						t.Fatal("round trip changed identity")
					}
				})
			}
		}
	}
	for _, input := range []sqlrs.ExecutionEnvironmentInput{executionBase(), executionResources(t)} {
		for _, windows := range []bool{false, true} {
			if windows {
				input.Runtime.Platform.OS = "windows"
				input.Workdir = sqlrs.ExecutionPath{Root: "image:c", Relative: "work"}
				for index := range input.Mounts {
					input.Mounts[index].Target.Root = "image:c"
				}
			}
			identity := executionBuild(t, input)
			parsed, err := sqlrs.ParseExecutionEnvironmentIdentity(identity)
			if err != nil || executionFingerprint(t, identity) != executionFingerprint(t, executionBuild(t, parsed)) {
				t.Fatalf("platform round trip: %v", err)
			}
		}
	}
}

// ES03: the expected schema mapping is independent of the facade under test.
func executionMinimalFields() map[string]any {
	return map[string]any{
		"primitive":  "oci",
		"runtime":    map[string]any{"identity_kind": "image-manifest", "digest": executionDigest, "platform": map[string]any{"os": "linux", "architecture": "amd64", "variant": ""}},
		"invocation": map[string]any{"argv": []any{[]any{map[string]any{"kind": "literal", "literal": "tool"}}}, "environment": map[string]any{}, "workdir": map[string]any{"root": "image", "relative": "work"}, "user": ""},
		"inputs":     map[string]any{}, "network": map[string]any{"mode": "none", "services": map[string]any{}}, "mounts": []any{}, "secrets": map[string]any{},
	}
}

// Matrix fixtures are deliberately assembled without Build, including rejected
// profiles, so Parse cannot inherit Build's compatibility decision.
func executionMatrixFields(input sqlrs.ExecutionEnvironmentInput) map[string]any {
	fields := executionMinimalFields()
	fields["primitive"] = string(input.Runtime.Kind)
	runtime := fields["runtime"].(map[string]any)
	runtime["identity_kind"] = string(input.Runtime.IdentityKind)
	if input.Runtime.Kind == sqlrs.ExecutionRuntimeDatabaseContainer {
		runtime["database_service"] = input.Runtime.DatabaseService
	}
	invocation := fields["invocation"].(map[string]any)
	invocation["workdir"] = map[string]any{"root": input.Workdir.Root, "relative": input.Workdir.Relative}
	invocation["user"] = input.User
	if input.Runtime.Kind == sqlrs.ExecutionRuntimeHost {
		path := input.Argv[0][0].Path
		invocation["argv"] = []any{[]any{map[string]any{"kind": "path", "path": map[string]any{"root": path.Root, "relative": path.Relative}, "format": "native"}}}
	}
	services := map[string]any{}
	for _, service := range input.Services {
		services[service.Role] = map[string]any{"protocol": service.Protocol, "database": service.Database, "user": service.User, "transport": string(service.Transport)}
	}
	fields["network"] = map[string]any{"mode": string(input.Network), "services": services}
	return fields
}

func executionNode(t testing.TB, input any) runtimev2.CanonicalValue {
	t.Helper()
	var value runtimev2.CanonicalValue
	var err error
	switch input := input.(type) {
	case string:
		value, err = runtimev2.CanonicalString(input)
	case []any:
		values := make([]runtimev2.CanonicalValue, len(input))
		for index, child := range input {
			values[index] = executionNode(t, child)
		}
		value, err = runtimev2.CanonicalList(values)
	case map[string]any:
		entries := make([]runtimev2.CanonicalMapEntry, 0, len(input))
		for name, child := range input {
			entries = append(entries, runtimev2.CanonicalMapEntry{Key: name, Value: executionNode(t, child)})
		}
		value, err = runtimev2.CanonicalMap(entries)
	case runtimev2.CanonicalValue:
		value = input
	default:
		t.Fatalf("unexpected fixture node %T", input)
	}
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func executionGeneric(t testing.TB, fields map[string]any) runtimev2.CanonicalResolvedExtensionIdentity {
	t.Helper()
	builder, err := runtimev2.NewExtensionIdentityBuilder(sqlrs.ExecutionEnvironmentSchema())
	if err != nil {
		t.Fatal(err)
	}
	for name, data := range fields {
		var field runtimev2.IdentityField
		if name == "primitive" {
			field, err = runtimev2.NewTextIdentityField(name, data.(string))
		} else {
			field, err = runtimev2.NewCanonicalValueIdentityField(name, executionNode(t, data))
		}
		if err != nil {
			t.Fatal(err)
		}
		if err = builder.AddIdentityField(field); err != nil {
			t.Fatal(err)
		}
	}
	composition, err := builder.Build()
	if err != nil {
		t.Fatal(err)
	}
	return composition.Identity()
}

func TestExecutionIndependentMinimalShape(t *testing.T) {
	actual := executionBuild(t, executionBase())
	expected := executionGeneric(t, executionMinimalFields())
	actualBytes, _ := runtimev2.CanonicalResolvedExtensionBytes(actual)
	expectedBytes, _ := runtimev2.CanonicalResolvedExtensionBytes(expected)
	if !bytes.Equal(actualBytes, expectedBytes) || executionFingerprint(t, actual) != executionFingerprint(t, expected) {
		t.Fatal("facade mapping differs from independent schema fixture")
	}
	if actual.Provider() != "sqlrs.execution" || actual.Kind() != "environment" || actual.IdentitySchema() != "sqlrs.execution-environment.v1" || len(actual.Fields()) != 7 {
		t.Fatal("wrong schema descriptor")
	}
	for _, field := range actual.Fields() {
		want := runtimev2.DisclosureProtected
		if field.Field.Name() == "primitive" {
			want = runtimev2.DisclosurePublic
		}
		if field.Disclosure != want {
			t.Fatalf("wrong disclosure for %s", field.Field.Name())
		}
	}
}

func TestExecutionParseChecksSchemaAndDisclosure(t *testing.T) {
	if input, err := sqlrs.ParseExecutionEnvironmentIdentity(runtimev2.CanonicalResolvedExtensionIdentity{}); err == nil || !reflect.DeepEqual(input, sqlrs.ExecutionEnvironmentInput{}) {
		t.Fatal("zero identity accepted")
	}
	for _, mode := range []string{"owner", "kind", "schema", "missing", "extra", "primitive-kind", "invocation-kind", "disclosure"} {
		t.Run(mode, func(t *testing.T) {
			fields := executionMinimalFields()
			definition := schemaauthor.SchemaInput{Provider: "sqlrs.execution", SemanticKind: "environment", IdentitySchema: "sqlrs.execution-environment.v1"}
			if mode == "owner" {
				definition.Provider = "other"
			}
			if mode == "kind" {
				definition.SemanticKind = "other"
			}
			if mode == "schema" {
				definition.IdentitySchema = "sqlrs.execution-environment.v2"
			}
			if mode == "missing" {
				delete(fields, "inputs")
			}
			if mode == "extra" {
				fields["extra"] = map[string]any{}
			}
			for name := range fields {
				kind, disclosure := runtimev2.IdentityFieldCanonicalValue, schemaauthor.DisclosureProtected
				if name == "primitive" {
					kind, disclosure = runtimev2.IdentityFieldText, schemaauthor.DisclosurePublic
				}
				if mode == "primitive-kind" && name == "primitive" {
					kind = runtimev2.IdentityFieldCanonicalValue
				}
				if mode == "invocation-kind" && name == "invocation" {
					kind = runtimev2.IdentityFieldText
				}
				if mode == "disclosure" && name == "invocation" {
					disclosure = schemaauthor.DisclosurePublic
				}
				definition.Fields = append(definition.Fields, schemaauthor.FieldDefinition{Name: name, Role: schemaauthor.FieldRoleSemantic, Kind: kind, Required: true, Disclosure: disclosure})
			}
			schema, err := schemaauthor.NewExtensionSchema(definition)
			if err != nil {
				t.Fatal(err)
			}
			builder, err := runtimev2.NewExtensionIdentityBuilder(schema)
			if err != nil {
				t.Fatal(err)
			}
			for _, def := range definition.Fields {
				var field runtimev2.IdentityField
				if def.Kind == runtimev2.IdentityFieldText {
					field, err = runtimev2.NewTextIdentityField(def.Name, "oci")
				} else {
					field, err = runtimev2.NewCanonicalValueIdentityField(def.Name, executionNode(t, fields[def.Name]))
				}
				if err != nil {
					t.Fatal(err)
				}
				if err = builder.AddIdentityField(field); err != nil {
					t.Fatal(err)
				}
			}
			composition, err := builder.Build()
			if err != nil {
				t.Fatal(err)
			}
			input, err := sqlrs.ParseExecutionEnvironmentIdentity(composition.Identity())
			var validation *runtimev2.ValidationError
			if !errors.As(err, &validation) || validation.Path == "" || !reflect.DeepEqual(input, sqlrs.ExecutionEnvironmentInput{}) {
				t.Fatalf("wrong schema accepted: %v", err)
			}
		})
	}
}

func TestExecutionFileModesAndWindowsEnvironment(t *testing.T) {
	input := executionResources(t)
	input.Inputs[0].Profile = sqlrs.ExecutionInputRegularFile
	input.Workdir = sqlrs.ExecutionPath{Root: "image", Relative: "work"}
	input.Argv[1][1].Path.Relative = ""
	plain := executionBuild(t, input)
	input.Inputs[0].Executable = true
	if executionFingerprint(t, plain) == executionFingerprint(t, executionBuild(t, input)) {
		t.Fatal("executable mode omitted")
	}
	input.Workdir = sqlrs.ExecutionPath{Root: "mount:project"}
	identity, err := sqlrs.BuildExecutionEnvironmentIdentity(input)
	executionInvalid(t, identity, err)
	input = executionBase()
	input.Runtime.Platform.OS = "windows"
	input.Workdir.Root = "image:c"
	input.Environment = []sqlrs.ExecutionEnvironmentVariable{{Name: "Path", Value: executionLiteral("logical")}}
	lower := executionBuild(t, input)
	input.Environment[0].Name = "PATH"
	if executionFingerprint(t, lower) != executionFingerprint(t, executionBuild(t, input)) {
		t.Fatal("Windows env names were not normalized")
	}
	input.Environment = append(input.Environment, sqlrs.ExecutionEnvironmentVariable{Name: "path", Value: executionLiteral("other")})
	identity, err = sqlrs.BuildExecutionEnvironmentIdentity(input)
	executionInvalid(t, identity, err)
	input.Runtime.Platform.OS = "linux"
	input.Workdir.Root = "image"
	if _, err := sqlrs.BuildExecutionEnvironmentIdentity(input); err != nil {
		t.Fatalf("POSIX env case was folded: %v", err)
	}
}

func TestExecutionServicePathsAndTransport(t *testing.T) {
	for _, suffix := range []string{"socket", "socket-directory", "credential-file"} {
		input := executionResources(t)
		input.Services[0].Transport = sqlrs.ExecutionServiceUnixSocket
		input.Services[0].AuthenticationProfile = "postgresql.file.v1"
		input.Argv = append(input.Argv, executionPathWord("service:database:"+suffix, ""))
		executionBuild(t, input)
		input.Argv[len(input.Argv)-1][0].Path.Relative = "child"
		identity, err := sqlrs.BuildExecutionEnvironmentIdentity(input)
		executionInvalid(t, identity, err)
	}
	input := executionResources(t)
	input.Environment[0].Value[0].ServiceField = sqlrs.ExecutionServiceHost
	executionBuild(t, input)
	input.Services[0].Transport = sqlrs.ExecutionServiceUnixSocket
	identity, err := sqlrs.BuildExecutionEnvironmentIdentity(input)
	executionInvalid(t, identity, err)
	input = executionResources(t)
	input.Secrets[0].Name = "other" // Broken service backlink.
	identity, err = sqlrs.BuildExecutionEnvironmentIdentity(input)
	executionInvalid(t, identity, err)
}

func TestExecutionNormalizationAndOrderedSemantics(t *testing.T) {
	one := executionResources(t)
	before := executionFingerprint(t, executionBuild(t, one))
	one.Environment[0], one.Environment[1] = one.Environment[1], one.Environment[0]
	one.Secrets[0], one.Secrets[1] = one.Secrets[1], one.Secrets[0]
	one.Argv[0] = sqlrs.ExecutionText{{Kind: sqlrs.ExecutionTextPartLiteral, Literal: "to"}, {Kind: sqlrs.ExecutionTextPartLiteral}, {Kind: sqlrs.ExecutionTextPartLiteral, Literal: "ol"}}
	if before != executionFingerprint(t, executionBuild(t, one)) {
		t.Fatal("keyed/literal normalization changed identity")
	}
	one.Mounts[0], one.Mounts[1] = one.Mounts[1], one.Mounts[0]
	if before == executionFingerprint(t, executionBuild(t, one)) {
		t.Fatal("ordered mounts ignored")
	}
	base := executionBase()
	empty := executionBase()
	empty.Inputs = []sqlrs.ExecutionInputBinding{}
	empty.Environment = []sqlrs.ExecutionEnvironmentVariable{}
	if executionFingerprint(t, executionBuild(t, base)) != executionFingerprint(t, executionBuild(t, empty)) {
		t.Fatal("nil/empty collections differ")
	}
	empty.Environment = []sqlrs.ExecutionEnvironmentVariable{{Name: "EMPTY", Value: executionLiteral("")}}
	if executionFingerprint(t, executionBuild(t, base)) == executionFingerprint(t, executionBuild(t, empty)) {
		t.Fatal("empty environment confused with absence")
	}
}

func TestExecutionSemanticDimensionChangesIdentity(t *testing.T) {
	for name, change := range map[string]func(*sqlrs.ExecutionEnvironmentInput){
		"digest": func(v *sqlrs.ExecutionEnvironmentInput) {
			v.Runtime.IdentityDigest = "sha256:" + strings.Repeat("2", 64)
		},
		"architecture":    func(v *sqlrs.ExecutionEnvironmentInput) { v.Runtime.Platform.Architecture = "arm64" },
		"variant":         func(v *sqlrs.ExecutionEnvironmentInput) { v.Runtime.Platform.Variant = "v8" },
		"argument":        func(v *sqlrs.ExecutionEnvironmentInput) { v.Argv = append(v.Argv, executionLiteral("extra")) },
		"user":            func(v *sqlrs.ExecutionEnvironmentInput) { v.User = "1000" },
		"workdir":         func(v *sqlrs.ExecutionEnvironmentInput) { v.Workdir.Relative = "subdir" },
		"source provider": func(v *sqlrs.ExecutionEnvironmentInput) { v.Inputs[0].Source.Provider = "other" },
		"source kind":     func(v *sqlrs.ExecutionEnvironmentInput) { v.Inputs[0].Source.Kind = "manifest" },
		"source schema":   func(v *sqlrs.ExecutionEnvironmentInput) { v.Inputs[0].Source.IdentitySchema = "files.tree.v2" },
		"source fingerprint": func(v *sqlrs.ExecutionEnvironmentInput) {
			v.Inputs[0].Source.Fingerprint = runtimev2.CanonicalFingerprint("sha256:" + strings.Repeat("2", 64))
		},
		"mount access":           func(v *sqlrs.ExecutionEnvironmentInput) { v.Mounts[0].ReadOnly = false },
		"mount target":           func(v *sqlrs.ExecutionEnvironmentInput) { v.Mounts[0].Target.Relative = "other" },
		"service database":       func(v *sqlrs.ExecutionEnvironmentInput) { v.Services[0].Database = "other" },
		"service user":           func(v *sqlrs.ExecutionEnvironmentInput) { v.Services[0].User = "other" },
		"service protocol":       func(v *sqlrs.ExecutionEnvironmentInput) { v.Services[0].Protocol = "postgresql.v2" },
		"authentication profile": func(v *sqlrs.ExecutionEnvironmentInput) { v.Services[0].AuthenticationProfile = "postgresql.env.v2" },
		"network":                func(v *sqlrs.ExecutionEnvironmentInput) { v.Network = sqlrs.ExecutionNetworkServices },
		"secret version": func(v *sqlrs.ExecutionEnvironmentInput) {
			ref, _ := runtimev2.NewSecretReference("vault", "seed-reference", "v2")
			v.Secrets[1].IdentityReference = &ref
		},
	} {
		t.Run(name, func(t *testing.T) {
			before, after := executionResources(t), executionResources(t)
			change(&after)
			if executionFingerprint(t, executionBuild(t, before)) == executionFingerprint(t, executionBuild(t, after)) {
				t.Fatal("semantic field omitted")
			}
		})
	}
}

func executionInvalid(t testing.TB, identity runtimev2.CanonicalResolvedExtensionIdentity, err error) {
	t.Helper()
	var validation *runtimev2.ValidationError
	if !errors.Is(err, runtimev2.ErrInvalid) || !errors.As(err, &validation) || validation.Code == "" || validation.Path == "" {
		t.Fatalf("expected typed static-path validation error, got %v", err)
	}
	if identity != (runtimev2.CanonicalResolvedExtensionIdentity{}) {
		t.Fatal("partial identity returned")
	}
}

func TestExecutionInputRejectionMatrix(t *testing.T) {
	for name, change := range map[string]func(*sqlrs.ExecutionEnvironmentInput){
		"empty argv":            func(v *sqlrs.ExecutionEnvironmentInput) { v.Argv = nil },
		"empty executable":      func(v *sqlrs.ExecutionEnvironmentInput) { v.Argv[0] = executionLiteral("") },
		"runtime zero":          func(v *sqlrs.ExecutionEnvironmentInput) { v.Runtime.Kind = "" },
		"runtime unknown":       func(v *sqlrs.ExecutionEnvironmentInput) { v.Runtime.Kind = "docker" },
		"identity zero":         func(v *sqlrs.ExecutionEnvironmentInput) { v.Runtime.IdentityKind = "" },
		"identity unknown":      func(v *sqlrs.ExecutionEnvironmentInput) { v.Runtime.IdentityKind = "image-index" },
		"network zero":          func(v *sqlrs.ExecutionEnvironmentInput) { v.Network = "" },
		"network unknown":       func(v *sqlrs.ExecutionEnvironmentInput) { v.Network = "internet" },
		"digest":                func(v *sqlrs.ExecutionEnvironmentInput) { v.Runtime.IdentityDigest = "latest" },
		"platform":              func(v *sqlrs.ExecutionEnvironmentInput) { v.Runtime.Platform.OS = "Linux" },
		"database variant":      func(v *sqlrs.ExecutionEnvironmentInput) { v.Runtime.DatabaseService = "database" },
		"path absolute":         func(v *sqlrs.ExecutionEnvironmentInput) { v.Workdir.Relative = "/work" },
		"path traversal":        func(v *sqlrs.ExecutionEnvironmentInput) { v.Workdir.Relative = "../work" },
		"path repeated":         func(v *sqlrs.ExecutionEnvironmentInput) { v.Workdir.Relative = "a//b" },
		"path backslash":        func(v *sqlrs.ExecutionEnvironmentInput) { v.Workdir.Relative = `a\b` },
		"host root":             func(v *sqlrs.ExecutionEnvironmentInput) { v.Workdir.Root = "workspace" },
		"missing mount":         func(v *sqlrs.ExecutionEnvironmentInput) { v.Workdir.Root = "mount:missing" },
		"file workdir":          func(v *sqlrs.ExecutionEnvironmentInput) { v.Workdir.Root = "mount:config" },
		"mount cycle":           func(v *sqlrs.ExecutionEnvironmentInput) { v.Mounts[0].Target.Root = "mount:scratch" },
		"mount duplicate":       func(v *sqlrs.ExecutionEnvironmentInput) { v.Mounts[1].Name = v.Mounts[0].Name },
		"target duplicate":      func(v *sqlrs.ExecutionEnvironmentInput) { v.Mounts[1].Target = v.Mounts[0].Target },
		"input missing":         func(v *sqlrs.ExecutionEnvironmentInput) { v.Mounts[0].InputName = "missing" },
		"scratch readonly":      func(v *sqlrs.ExecutionEnvironmentInput) { v.Mounts[1].ReadOnly = true },
		"scratch input":         func(v *sqlrs.ExecutionEnvironmentInput) { v.Mounts[1].InputName = "project" },
		"secret writable":       func(v *sqlrs.ExecutionEnvironmentInput) { v.Mounts[2].ReadOnly = false },
		"secret slot missing":   func(v *sqlrs.ExecutionEnvironmentInput) { v.Mounts[2].SecretSlot = "missing" },
		"tree executable":       func(v *sqlrs.ExecutionEnvironmentInput) { v.Inputs[0].Executable = true },
		"input profile":         func(v *sqlrs.ExecutionEnvironmentInput) { v.Inputs[0].Profile = "tree" },
		"source schema":         func(v *sqlrs.ExecutionEnvironmentInput) { v.Inputs[0].Source.IdentitySchema = "Bad" },
		"source fingerprint":    func(v *sqlrs.ExecutionEnvironmentInput) { v.Inputs[0].Source.Fingerprint = "sha256:abc" },
		"source duplicate":      func(v *sqlrs.ExecutionEnvironmentInput) { v.Inputs = append(v.Inputs, v.Inputs[0]) },
		"env duplicate":         func(v *sqlrs.ExecutionEnvironmentInput) { v.Environment = append(v.Environment, v.Environment[0]) },
		"env name":              func(v *sqlrs.ExecutionEnvironmentInput) { v.Environment[0].Name = "not-valid" },
		"literal tagged fields": func(v *sqlrs.ExecutionEnvironmentInput) { v.Argv[0][0].SecretSlot = "seed" },
		"literal NUL":           func(v *sqlrs.ExecutionEnvironmentInput) { v.Argv[0][0].Literal = "bad\x00value" },
		"literal UTF8":          func(v *sqlrs.ExecutionEnvironmentInput) { v.Argv[0][0].Literal = string([]byte{255}) },
		"secret argv": func(v *sqlrs.ExecutionEnvironmentInput) {
			v.Argv = append(v.Argv, sqlrs.ExecutionText{{Kind: sqlrs.ExecutionTextPartSecret, SecretSlot: "seed"}})
		},
		"unknown part":           func(v *sqlrs.ExecutionEnvironmentInput) { v.Argv[0][0].Kind = "template" },
		"secret version missing": func(v *sqlrs.ExecutionEnvironmentInput) { v.Secrets[1].IdentityReference = nil },
		"secret purpose":         func(v *sqlrs.ExecutionEnvironmentInput) { v.Secrets[1].Purpose = "execution-only" },
		"semantic service":       func(v *sqlrs.ExecutionEnvironmentInput) { v.Secrets[1].ServiceRole = "database" },
		"authentication version": func(v *sqlrs.ExecutionEnvironmentInput) {
			v.Secrets[0].IdentityReference = v.Secrets[1].IdentityReference
		},
		"authentication backlink": func(v *sqlrs.ExecutionEnvironmentInput) { v.Services[0].AuthenticationSecret = "other" },
		"authentication user":     func(v *sqlrs.ExecutionEnvironmentInput) { v.Services[0].User = "" },
		"authentication profile":  func(v *sqlrs.ExecutionEnvironmentInput) { v.Services[0].AuthenticationProfile = "postgresql.env" },
		"authentication text":     func(v *sqlrs.ExecutionEnvironmentInput) { v.Environment[1].Value[0].SecretSlot = "auth" },
		"service duplicate":       func(v *sqlrs.ExecutionEnvironmentInput) { v.Services = append(v.Services, v.Services[0]) },
		"service unknown field":   func(v *sqlrs.ExecutionEnvironmentInput) { v.Environment[0].Value[0].ServiceField = "dsn" },
		"service absent":          func(v *sqlrs.ExecutionEnvironmentInput) { v.Environment[0].Value[0].ServiceRole = "missing" },
	} {
		t.Run(name, func(t *testing.T) {
			input := executionResources(t)
			change(&input)
			identity, err := sqlrs.BuildExecutionEnvironmentIdentity(input)
			executionInvalid(t, identity, err)
		})
	}
}

func TestExecutionParseRejectsGenericValidSemanticMutants(t *testing.T) {
	for name, mutate := range map[string]func(map[string]any){
		"unknown runtime identity":  func(v map[string]any) { v["runtime"].(map[string]any)["identity_kind"] = "image-index" },
		"incompatible identity":     func(v map[string]any) { v["runtime"].(map[string]any)["identity_kind"] = "host-executable" },
		"missing runtime member":    func(v map[string]any) { delete(v["runtime"].(map[string]any), "digest") },
		"extra runtime member":      func(v map[string]any) { v["runtime"].(map[string]any)["extra"] = "" },
		"wrong runtime shape":       func(v map[string]any) { v["runtime"] = []any{} },
		"wrong platform shape":      func(v map[string]any) { v["runtime"].(map[string]any)["platform"] = "linux" },
		"missing invocation member": func(v map[string]any) { delete(v["invocation"].(map[string]any), "workdir") },
		"empty command":             func(v map[string]any) { v["invocation"].(map[string]any)["argv"] = []any{} },
		"redundant literals": func(v map[string]any) {
			v["invocation"].(map[string]any)["argv"] = []any{[]any{map[string]any{"kind": "literal", "literal": "to"}, map[string]any{"kind": "literal", "literal": "ol"}}}
		},
		"unexpected part field": func(v map[string]any) {
			v["invocation"].(map[string]any)["argv"] = []any{[]any{map[string]any{"kind": "literal", "literal": "tool", "slot": "hidden"}}}
		},
		"invalid workdir": func(v map[string]any) {
			v["invocation"].(map[string]any)["workdir"] = map[string]any{"root": "image", "relative": "../escape"}
		},
		"unknown network":    func(v map[string]any) { v["network"].(map[string]any)["mode"] = "bridge" },
		"wrong inputs shape": func(v map[string]any) { v["inputs"] = []any{} },
		"wrong mount shape":  func(v map[string]any) { v["mounts"] = map[string]any{} },
		"wrong secret shape": func(v map[string]any) { v["secrets"] = []any{} },
	} {
		t.Run(name, func(t *testing.T) {
			fields := executionMinimalFields()
			mutate(fields)
			raw := executionGeneric(t, fields)
			parsed, err := sqlrs.ParseExecutionEnvironmentIdentity(raw)
			var validation *runtimev2.ValidationError
			if !errors.Is(err, runtimev2.ErrInvalid) || !errors.As(err, &validation) || validation.Path == "" || !reflect.DeepEqual(parsed, sqlrs.ExecutionEnvironmentInput{}) {
				t.Fatalf("accepted malformed semantic identity: %v", err)
			}
		})
	}
}

func TestExecutionRawAndGeneratedBudgets(t *testing.T) {
	for _, count := range []int{runtimev2.MaxCanonicalCollectionMembers, runtimev2.MaxCanonicalCollectionMembers + 1} {
		input := executionBase()
		input.Argv = make([]sqlrs.ExecutionText, count)
		for i := range input.Argv {
			input.Argv[i] = executionLiteral("tool")
		}
		identity, err := sqlrs.BuildExecutionEnvironmentIdentity(input)
		if count == runtimev2.MaxCanonicalCollectionMembers {
			if err != nil {
				t.Fatal(err)
			}
		} else {
			executionInvalid(t, identity, err)
		}
	}
	for _, text := range []string{strings.Repeat("x", runtimev2.MaxCanonicalStringBytes+1), strings.Repeat("x", runtimev2.MaxCanonicalStringBytes)} {
		input := executionBase()
		input.Argv[0] = executionLiteral(text)
		identity, err := sqlrs.BuildExecutionEnvironmentIdentity(input)
		if len(text) > runtimev2.MaxCanonicalStringBytes {
			executionInvalid(t, identity, err)
		} else if err != nil {
			t.Fatal(err)
		}
	}
	input := executionBase()
	input.Argv[0] = make(sqlrs.ExecutionText, runtimev2.MaxCanonicalCollectionMembers+1)
	for i := range input.Argv[0] {
		input.Argv[0][i] = sqlrs.ExecutionTextPart{Kind: sqlrs.ExecutionTextPartLiteral}
	}
	identity, err := sqlrs.BuildExecutionEnvironmentIdentity(input)
	executionInvalid(t, identity, err)
	input = executionBase()
	input.Argv = make([]sqlrs.ExecutionText, runtimev2.MaxCanonicalCollectionMembers)
	for i := range input.Argv {
		input.Argv[i] = executionLiteral(strings.Repeat("x", runtimev2.MaxCanonicalStringBytes))
	}
	input.Environment = []sqlrs.ExecutionEnvironmentVariable{{Name: "EXTRA", Value: executionLiteral("overflow")}}
	identity, err = sqlrs.BuildExecutionEnvironmentIdentity(input)
	executionInvalid(t, identity, err)
}

func TestExecutionErrorsDoNotDiscloseCallerValues(t *testing.T) {
	for _, marker := range []string{"private-sensitive-marker", `C:\private\sensitive`, "opaque-secret-reference"} {
		input := executionBase()
		input.Environment = []sqlrs.ExecutionEnvironmentVariable{{Name: marker, Value: executionLiteral("value")}}
		if marker == "opaque-secret-reference" {
			input.Environment[0].Name = "SAFE"
			input.Environment[0].Value = sqlrs.ExecutionText{{Kind: sqlrs.ExecutionTextPartSecret, SecretSlot: marker}}
		}
		identity, err := sqlrs.BuildExecutionEnvironmentIdentity(input)
		executionInvalid(t, identity, err)
		if strings.Contains(err.Error(), marker) {
			t.Fatal("validation leaked caller data")
		}
	}
}

func TestExecutionInputAndParsedOutputAreOwned(t *testing.T) {
	input := executionResources(t)
	identity := executionBuild(t, input)
	before := executionFingerprint(t, identity)
	input.Argv[1][1].Path.Relative = "changed"
	input.Environment[0].Name = "CHANGED"
	input.Inputs[0].Name = "changed"
	*input.Secrets[1].IdentityReference = runtimev2.SecretReference{}
	parsed, err := sqlrs.ParseExecutionEnvironmentIdentity(identity)
	if err != nil {
		t.Fatal(err)
	}
	parsed.Argv[1][1].Path.Relative = "other"
	parsed.Secrets[1].IdentityReference = nil
	if executionFingerprint(t, identity) != before {
		t.Fatal("retained caller buffers")
	}
	fresh, err := sqlrs.ParseExecutionEnvironmentIdentity(identity)
	if err != nil {
		t.Fatal(err)
	}
	if fresh.Argv[1][1].Path.Relative != "change.sql" {
		t.Fatal("aliased parse output")
	}
	var workers sync.WaitGroup
	for range 4 {
		workers.Add(1)
		go func() {
			defer workers.Done()
			for range 10 {
				value, err := sqlrs.ParseExecutionEnvironmentIdentity(identity)
				if err != nil {
					t.Error(err)
					return
				}
				copy, err := sqlrs.BuildExecutionEnvironmentIdentity(value)
				if err != nil {
					t.Error(err)
					return
				}
				fingerprint, err := runtimev2.CanonicalExtensionFingerprint(copy)
				if err != nil || fingerprint != before {
					t.Error("concurrent drift")
					return
				}
			}
		}()
	}
	workers.Wait()
}

type executionAuthorizer struct {
	mode   string
	calls  int
	cancel context.CancelFunc
}

func (a *executionAuthorizer) AuthorizeInternalDisclosure(context.Context) error {
	a.calls++
	if a.cancel != nil {
		a.cancel()
	}
	if a.mode != "allow" {
		return errors.New("denied")
	}
	return nil
}

func TestExecutionProtectedExplanationsAndWireRoundTrip(t *testing.T) {
	identity := executionBuild(t, executionResources(t))
	envelope, err := runtimev2.NewExtensionEnvelope(identity)
	if err != nil {
		t.Fatal(err)
	}
	safe, err := runtimev2.ExplainSafe(envelope)
	if err != nil {
		t.Fatal(err)
	}
	safeJSON, err := json.Marshal(safe)
	if err != nil {
		t.Fatal(err)
	}
	var projection struct {
		Fields []struct {
			Name         string          `json:"name"`
			Redacted     bool            `json:"redacted"`
			Text         *string         `json:"text"`
			CanonicalHex string          `json:"canonical_hex"`
			Commitment   string          `json:"commitment"`
			Secret       json.RawMessage `json:"secret_reference"`
		} `json:"fields"`
	}
	if err = json.Unmarshal(safeJSON, &projection); err != nil {
		t.Fatal(err)
	}
	if len(projection.Fields) != 7 {
		t.Fatal("missing explanation fields")
	}
	for _, field := range projection.Fields {
		if field.Name != "primitive" && (!field.Redacted || field.Text != nil || field.CanonicalHex != "" || field.Commitment != "" || field.Secret != nil) {
			t.Fatal("protected disclosure")
		}
	}
	allow := &executionAuthorizer{mode: "allow"}
	internal, err := runtimev2.ExplainInternal(context.Background(), envelope, allow)
	if err != nil || allow.calls != 1 {
		t.Fatal("authorization not checked")
	}
	internalJSON, _ := json.Marshal(internal)
	if err = json.Unmarshal(internalJSON, &projection); err != nil {
		t.Fatal(err)
	}
	fields := map[string]runtimev2.IdentityField{}
	for _, field := range identity.Fields() {
		fields[field.Field.Name()] = field.Field
	}
	for _, field := range projection.Fields {
		if field.Name != "primitive" {
			raw, err := hex.DecodeString(field.CanonicalHex)
			if err != nil || !bytes.Equal(raw, fields[field.Name].CanonicalValue().CanonicalBytes()) || field.Commitment != string(fields[field.Name].Commitment()) || field.Secret != nil {
				t.Fatal("internal projection changed protected field")
			}
		}
	}
	for _, mode := range []string{"nil", "deny", "error", "cancel-before", "cancel-during"} {
		ctx, cancel := context.WithCancel(context.Background())
		authorizer := &executionAuthorizer{mode: "allow"}
		var gate runtimev2.InternalDisclosureAuthorizer = authorizer
		if mode == "nil" {
			gate = nil
		}
		if mode == "deny" || mode == "error" {
			authorizer.mode = mode
		}
		if mode == "cancel-before" {
			cancel()
		}
		if mode == "cancel-during" {
			authorizer.cancel = cancel
		}
		result, err := runtimev2.ExplainInternal(ctx, envelope, gate)
		cancel()
		var validation *runtimev2.ValidationError
		if !errors.As(err, &validation) || validation.Code != runtimev2.CodeAuthorizationDenied || result.Valid() {
			t.Fatalf("authorization failure=%v", err)
		}
		wantCalls := 1
		if mode == "nil" || mode == "cancel-before" {
			wantCalls = 0
		}
		if authorizer.calls != wantCalls {
			t.Fatalf("authorization calls=%d, want=%d", authorizer.calls, wantCalls)
		}
	}
	raw, err := json.Marshal(envelope)
	if err != nil {
		t.Fatal(err)
	}
	var decoded runtimev2.FingerprintEnvelope
	if err = json.Unmarshal(raw, &decoded); err != nil || decoded.Descriptor().Digest() != envelope.Descriptor().Digest() {
		t.Fatalf("envelope round trip: %v", err)
	}
}

// ES23 checks existing composition/StateID semantics, not State-cache admission.
func TestExecutionTransformCompositionAndOperationalSeparation(t *testing.T) {
	env, err := runtimev2.NewExecutionEnvironmentDeclaration(runtimev2.ExtensionSpecificationInput{SchemaVersion: runtimev2.SchemaVersion, Owner: "sqlrs.execution", Kind: "environment", SpecificationSchema: "sqlrs.execution.declaration.v1"})
	if err != nil {
		t.Fatal(err)
	}
	build := func(input sqlrs.ExecutionEnvironmentInput, job string) (runtimev2.CanonicalFingerprint, runtimev2.CanonicalFingerprint) {
		identity := executionBuild(t, input)
		builder, err := sqlrs.NewPSQLTransformBuilder(runtimev2.TransformDeclaration{Kind: "psql", Reference: "query", ExecutionEnvironment: &env})
		if err != nil {
			t.Fatal(err)
		}
		field, err := runtimev2.NewCanonicalValueIdentityField("sql", executionNode(t, "select 1"))
		if err != nil {
			t.Fatal(err)
		}
		if err = builder.AddIdentityField(field); err != nil {
			t.Fatal(err)
		}
		observation, err := runtimev2.NewOperationalObservation("sqlrs", "psql", "sqlrs.psql.observation.v1", map[string]string{"job_id": job})
		if err != nil {
			t.Fatal(err)
		}
		if err = builder.AddObservation(observation); err != nil {
			t.Fatal(err)
		}
		if err = builder.BindExtensions(runtimev2.ResolvedTransformExtensions{ExecutionEnvironment: &identity}); err != nil {
			t.Fatal(err)
		}
		composition, err := builder.Build()
		if err != nil {
			t.Fatal(err)
		}
		fingerprint, err := runtimev2.CanonicalTransformFingerprint(composition.Identity())
		if err != nil {
			t.Fatal(err)
		}
		envelope, err := runtimev2.NewTransformEnvelope(composition.Identity())
		if err != nil {
			t.Fatal(err)
		}
		derived, err := runtimev2.NewDerivedStateEnvelope(runtimev2.CanonicalStateID(executionDigest), envelope)
		if err != nil {
			t.Fatal(err)
		}
		return fingerprint, derived.Descriptor().Digest()
	}
	one, oneState := build(executionBase(), "local-job")
	two, twoState := build(executionBase(), "remote-job")
	if one != two || oneState != twoState {
		t.Fatal("operational observation changed semantic identity")
	}
	changed := executionBase()
	changed.Argv = append(changed.Argv, executionLiteral("other"))
	three, threeState := build(changed, "remote-job")
	if one == three || oneState == threeState {
		t.Fatal("execution environment was not bound into transform/StateID")
	}
}

// ES27 uses bounded raw inputs; fixed valid seeds prevent early-rejection-only fuzzing.
func FuzzExecutionEnvironmentInput(f *testing.F) {
	f.Add([]byte("tool"))
	f.Add([]byte{})
	f.Add([]byte{255})
	f.Fuzz(func(t *testing.T, data []byte) {
		if len(data) > runtimev2.MaxCanonicalStringBytes+1 {
			return
		}
		input := executionBase()
		input.Argv[0] = executionLiteral(string(data))
		identity, err := sqlrs.BuildExecutionEnvironmentIdentity(input)
		if err != nil {
			return
		}
		parsed, err := sqlrs.ParseExecutionEnvironmentIdentity(identity)
		if err != nil {
			t.Fatal(err)
		}
		if executionFingerprint(t, identity) != executionFingerprint(t, executionBuild(t, parsed)) {
			t.Fatal("round trip differs")
		}
	})
}

func FuzzExecutionEnvironmentIdentity(f *testing.F) {
	f.Add("none", "image-manifest", "tool")
	f.Add("local-context", "host-executable", "tool")
	f.Add("none", "image-manifest", "")
	f.Fuzz(func(t *testing.T, network, kind, command string) {
		if len(network)+len(kind)+len(command) > runtimev2.MaxCanonicalStringBytes || strings.ContainsRune(network+kind+command, 0) {
			return
		}
		for _, s := range []string{network, kind, command} {
			if !utf8.ValidString(s) {
				return
			}
		}
		fields := executionMinimalFields()
		fields["runtime"].(map[string]any)["identity_kind"] = kind
		fields["network"].(map[string]any)["mode"] = network
		fields["invocation"].(map[string]any)["argv"] = []any{[]any{map[string]any{"kind": "literal", "literal": command}}}
		identity := executionGeneric(t, fields)
		parsed, err := sqlrs.ParseExecutionEnvironmentIdentity(identity)
		if err != nil {
			return
		}
		if executionFingerprint(t, identity) != executionFingerprint(t, executionBuild(t, parsed)) {
			t.Fatal("Parse repaired noncanonical input")
		}
	})
}
