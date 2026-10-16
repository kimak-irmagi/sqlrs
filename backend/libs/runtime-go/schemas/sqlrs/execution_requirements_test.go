package sqlrs_test

import (
	"bytes"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
	"github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/schemaauthor"
	"github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/schemas/sqlrs"
)

// The approved first coverage follow-up tests public requirements, not private
// helper branches. Requirements: execution-schema-coverage-plan.md, ES07–ES20.
func TestExecutionRawChannelsRejectMalformedAndOversizeText(t *testing.T) {
	channels := map[string]func(*sqlrs.ExecutionEnvironmentInput, string){
		"runtime":           func(v *sqlrs.ExecutionEnvironmentInput, s string) { v.User = s },
		"path":              func(v *sqlrs.ExecutionEnvironmentInput, s string) { v.Argv[1][1].Path.Relative = s },
		"environment-name":  func(v *sqlrs.ExecutionEnvironmentInput, s string) { v.Environment[0].Name = s },
		"environment-value": func(v *sqlrs.ExecutionEnvironmentInput, s string) { v.Environment[0].Value = executionLiteral(s) },
		"input":             func(v *sqlrs.ExecutionEnvironmentInput, s string) { v.Inputs[0].Source.IdentitySchema = s },
		"mount":             func(v *sqlrs.ExecutionEnvironmentInput, s string) { v.Mounts[0].Target.Relative = s },
		"service":           func(v *sqlrs.ExecutionEnvironmentInput, s string) { v.Services[0].Database = s },
		"secret-name":       func(v *sqlrs.ExecutionEnvironmentInput, s string) { v.Secrets[1].Name = s },
		"secret-reference": func(v *sqlrs.ExecutionEnvironmentInput, s string) {
			ref, err := runtimev2.NewSecretReference("vault", s, "v1")
			if err != nil {
				t.Fatal(err)
			}
			v.Secrets[1].IdentityReference = &ref
		},
	}
	for name, change := range channels {
		texts := []string{"sensitive\x00marker", strings.Repeat("x", runtimev2.MaxCanonicalStringBytes+1)}
		// Oversize references cannot be constructed by the public root API;
		// the reference channel still tests its additional NUL restriction.
		if name == "secret-reference" {
			texts = texts[:1]
		}
		for _, text := range texts {
			t.Run(name+fmt.Sprint(len(text)), func(t *testing.T) {
				input := executionResources(t)
				change(&input, text)
				identity, err := sqlrs.BuildExecutionEnvironmentIdentity(input)
				executionInvalid(t, identity, err)
				var validation *runtimev2.ValidationError
				errors.As(err, &validation)
				want := runtimev2.CodeValueInvalid
				if len(text) > runtimev2.MaxCanonicalStringBytes {
					want = runtimev2.CodeLimitExceeded
				}
				if validation.Code != want || strings.Contains(err.Error(), text) {
					t.Fatalf("wrong raw-channel validation: %v", err)
				}
			})
		}
	}
}

func TestExecutionAdditionalBindingAndVariantRequirements(t *testing.T) {
	for name, change := range map[string]func(*sqlrs.ExecutionEnvironmentInput){
		"invalid binding name": func(v *sqlrs.ExecutionEnvironmentInput) { v.Inputs[0].Name = "Bad" },
		"duplicate secret":     func(v *sqlrs.ExecutionEnvironmentInput) { v.Secrets = append(v.Secrets, v.Secrets[1]) },
		"unknown mount kind":   func(v *sqlrs.ExecutionEnvironmentInput) { v.Mounts[1].Kind = "overlay" },
		"unused input": func(v *sqlrs.ExecutionEnvironmentInput) {
			e := v.Inputs[0]
			e.Name = "unused"
			v.Inputs = append(v.Inputs, e)
		},
		"unused service": func(v *sqlrs.ExecutionEnvironmentInput) {
			v.Services = append(v.Services, sqlrs.ExecutionServiceBinding{Role: "unused", Protocol: "postgresql", Transport: sqlrs.ExecutionServiceTCP})
		},
		"unused secret": func(v *sqlrs.ExecutionEnvironmentInput) {
			e := v.Secrets[1]
			e.Name = "unused"
			v.Secrets = append(v.Secrets, e)
		},
		"database target absent": func(v *sqlrs.ExecutionEnvironmentInput) {
			v.Runtime.Kind = sqlrs.ExecutionRuntimeDatabaseContainer
			v.Runtime.DatabaseService = "absent"
		},
		"bad protocol":    func(v *sqlrs.ExecutionEnvironmentInput) { v.Services[0].Protocol = "Bad" },
		"bad transport":   func(v *sqlrs.ExecutionEnvironmentInput) { v.Services[0].Transport = "pipe" },
		"image tool root": func(v *sqlrs.ExecutionEnvironmentInput) { v.Argv = append(v.Argv, executionPathWord("tool", "")) },
		"wrong image drive": func(v *sqlrs.ExecutionEnvironmentInput) {
			v.Workdir.Root = "image:C"
			v.Runtime.Platform.OS = "windows"
		},
		"image root on Windows":  func(v *sqlrs.ExecutionEnvironmentInput) { v.Runtime.Platform.OS = "windows" },
		"image drive on POSIX":   func(v *sqlrs.ExecutionEnvironmentInput) { v.Workdir.Root = "image:c" },
		"unknown root":           func(v *sqlrs.ExecutionEnvironmentInput) { v.Workdir.Root = "unknown" },
		"malformed service root": func(v *sqlrs.ExecutionEnvironmentInput) { v.Workdir.Root = "service:database:socket:extra" },
		"unknown service root":   func(v *sqlrs.ExecutionEnvironmentInput) { v.Workdir.Root = "service:missing:socket" },
		"unknown service member": func(v *sqlrs.ExecutionEnvironmentInput) { v.Workdir.Root = "service:database:unknown" },
		"socket requires Unix": func(v *sqlrs.ExecutionEnvironmentInput) {
			v.Argv = append(v.Argv, executionPathWord("service:database:socket", ""))
		},
		"socket directory requires Unix":   func(v *sqlrs.ExecutionEnvironmentInput) { v.Workdir.Root = "service:database:socket-directory" },
		"credential file is not directory": func(v *sqlrs.ExecutionEnvironmentInput) { v.Workdir.Root = "service:database:credential-file" },
		"secret part extra fields":         func(v *sqlrs.ExecutionEnvironmentInput) { v.Environment[1].Value[0].Literal = "extra" },
		"service part extra fields":        func(v *sqlrs.ExecutionEnvironmentInput) { v.Environment[0].Value[0].SecretSlot = "seed" },
		"path part extra fields":           func(v *sqlrs.ExecutionEnvironmentInput) { v.Argv[1][1].ServiceRole = "database" },
		"missing path":                     func(v *sqlrs.ExecutionEnvironmentInput) { v.Argv[1][1].Path = nil },
		"wrong path format":                func(v *sqlrs.ExecutionEnvironmentInput) { v.Argv[1][1].PathFormat = "shell" },
		"empty database selection":         func(v *sqlrs.ExecutionEnvironmentInput) { v.Services[0].Database = "" },
		"empty user selection": func(v *sqlrs.ExecutionEnvironmentInput) {
			v.Services[0].AuthenticationProfile = ""
			v.Services[0].AuthenticationSecret = ""
			v.Secrets = v.Secrets[1:]
			v.Services[0].User = ""
			v.Environment[0].Value[0].ServiceField = sqlrs.ExecutionServiceUser
		},
		"merged literal overflow": func(v *sqlrs.ExecutionEnvironmentInput) {
			v.Argv = append(v.Argv, sqlrs.ExecutionText{{Kind: sqlrs.ExecutionTextPartLiteral, Literal: strings.Repeat("x", 3000)}, {Kind: sqlrs.ExecutionTextPartLiteral, Literal: strings.Repeat("y", 2000)}})
		},
	} {
		t.Run(name, func(t *testing.T) {
			v := executionResources(t)
			change(&v)
			identity, err := sqlrs.BuildExecutionEnvironmentIdentity(v)
			executionInvalid(t, identity, err)
		})
	}
}

func TestExecutionHostPathsAndAdditionalTargetSelections(t *testing.T) {
	host := executionBase()
	host.Runtime.Kind = sqlrs.ExecutionRuntimeHost
	host.Runtime.IdentityKind = sqlrs.ExecutionIdentityHostExecutable
	host.User = "uid:1000"
	host.Workdir.Root = "workspace"
	host.Argv[0] = executionPathWord("tool", "")
	host.Mounts = []sqlrs.ExecutionMount{{Name: "scratch", Kind: sqlrs.ExecutionMountScratch, Target: sqlrs.ExecutionPath{Root: "workspace", Relative: "tmp"}}}
	executionBuild(t, host)
	for _, change := range []func(*sqlrs.ExecutionEnvironmentInput){
		func(v *sqlrs.ExecutionEnvironmentInput) { v.Argv[0] = executionLiteral("tool") },
		func(v *sqlrs.ExecutionEnvironmentInput) { v.Argv[0][0].PathFormat = sqlrs.ExecutionPathForwardSlash },
		func(v *sqlrs.ExecutionEnvironmentInput) { v.Argv[0][0].Path.Relative = "child" },
		func(v *sqlrs.ExecutionEnvironmentInput) { v.Workdir.Root = "tool" },
		func(v *sqlrs.ExecutionEnvironmentInput) { v.Mounts[0].Target.Root = "image" },
		func(v *sqlrs.ExecutionEnvironmentInput) { v.Workdir.Root = "image" },
	} {
		copy := host
		copy.Argv = []sqlrs.ExecutionText{executionPathWord("tool", "")}
		copy.Mounts = append([]sqlrs.ExecutionMount(nil), host.Mounts...)
		change(&copy)
		identity, err := sqlrs.BuildExecutionEnvironmentIdentity(copy)
		executionInvalid(t, identity, err)
	}
	v := executionResources(t)
	v.Environment[0].Value[0].ServiceField = sqlrs.ExecutionServiceUser
	executionBuild(t, v)
	v.Environment[0].Value[0].ServiceField = sqlrs.ExecutionServicePort
	executionBuild(t, v)
	v.Argv[1][1].PathFormat = sqlrs.ExecutionPathForwardSlash
	executionBuild(t, v)
}

func TestExecutionMultipleBindingsAreOrderIndependent(t *testing.T) {
	v := executionResources(t)
	other := v.Inputs[0]
	other.Name = "another"
	v.Inputs = append(v.Inputs, other)
	v.Mounts = append(v.Mounts, sqlrs.ExecutionMount{Name: "another", InputName: "another", Kind: sqlrs.ExecutionMountInput, Target: sqlrs.ExecutionPath{Root: "image", Relative: "another"}, ReadOnly: true})
	v.Services = append(v.Services, sqlrs.ExecutionServiceBinding{Role: "another", Protocol: "postgresql", Database: "other", User: "other", Transport: sqlrs.ExecutionServiceTCP})
	v.Network = sqlrs.ExecutionNetworkServices
	before := executionFingerprint(t, executionBuild(t, v))
	v.Inputs[0], v.Inputs[1] = v.Inputs[1], v.Inputs[0]
	v.Services[0], v.Services[1] = v.Services[1], v.Services[0]
	after := executionBuild(t, v)
	if before != executionFingerprint(t, after) {
		t.Fatal("keyed binding order changed identity")
	}
	parsed, err := sqlrs.ParseExecutionEnvironmentIdentity(after)
	if err != nil || parsed.Inputs[0].Name != "another" || parsed.Services[0].Role != "another" {
		t.Fatalf("canonical read order: %v", err)
	}
}

func TestExecutionNestedClosedShapeRequirements(t *testing.T) {
	for name, change := range map[string]func(map[string]any){
		"wrong scalar kind": func(v map[string]any) { v["runtime"].(map[string]any)["digest"] = []any{} },
		"file invalid flag": func(v map[string]any) {
			p := v["inputs"].(map[string]any)["project"].(map[string]any)
			p["profile"] = "regular-file.v1"
			p["executable"] = "true"
		},
		"file missing flag": func(v map[string]any) {
			v["inputs"].(map[string]any)["project"].(map[string]any)["profile"] = "regular-file.v1"
		},
		"tree extra flag":       func(v map[string]any) { v["inputs"].(map[string]any)["project"].(map[string]any)["executable"] = "no" },
		"invalid mount access":  func(v map[string]any) { v["mounts"].([]any)[0].(map[string]any)["access"] = "shared" },
		"scratch input member":  func(v map[string]any) { v["mounts"].([]any)[1].(map[string]any)["input_name"] = "project" },
		"scratch secret member": func(v map[string]any) { v["mounts"].([]any)[1].(map[string]any)["secret_slot"] = "seed" },
		"auth version member": func(v map[string]any) {
			v["secrets"].(map[string]any)["auth"].(map[string]any)["reference"] = map[string]any{"provider": "vault", "identifier": "ref", "version": "v1"}
		},
		"semantic role member": func(v map[string]any) {
			v["secrets"].(map[string]any)["seed"].(map[string]any)["service_role"] = "database"
		},
		"invalid secret reference": func(v map[string]any) {
			v["secrets"].(map[string]any)["seed"].(map[string]any)["reference"].(map[string]any)["version"] = ""
		},
		"unknown secret purpose": func(v map[string]any) { v["secrets"].(map[string]any)["seed"].(map[string]any)["purpose"] = "legacy" },
		"unknown text kind": func(v map[string]any) {
			v["invocation"].(map[string]any)["argv"] = []any{[]any{map[string]any{"kind": "unknown"}}}
		},
		"empty primitive": func(v map[string]any) { v["primitive"] = "" },
	} {
		t.Run(name, func(t *testing.T) {
			fields := executionResourceFields()
			change(fields)
			executionParseInvalid(t, executionGeneric(t, fields), "")
		})
	}
	fields := executionMinimalFields()
	delete(fields, "inputs")
	fields["extra"] = map[string]any{}
	definition := schemaauthor.SchemaInput{Provider: "sqlrs.execution", SemanticKind: "environment", IdentitySchema: "sqlrs.execution-environment.v1"}
	for name := range fields {
		kind, disclosure := runtimev2.IdentityFieldCanonicalValue, schemaauthor.DisclosureProtected
		if name == "primitive" {
			kind, disclosure = runtimev2.IdentityFieldText, schemaauthor.DisclosurePublic
		}
		definition.Fields = append(definition.Fields, schemaauthor.FieldDefinition{Name: name, Role: schemaauthor.FieldRoleSemantic, Kind: kind, Disclosure: disclosure, Required: true})
	}
	schema, err := schemaauthor.NewExtensionSchema(definition)
	if err != nil {
		t.Fatal(err)
	}
	builder, err := runtimev2.NewExtensionIdentityBuilder(schema)
	if err != nil {
		t.Fatal(err)
	}
	for name, value := range fields {
		var field runtimev2.IdentityField
		if name == "primitive" {
			field, err = runtimev2.NewTextIdentityField(name, value.(string))
		} else {
			field, err = runtimev2.NewCanonicalValueIdentityField(name, executionNode(t, value))
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
	executionParseInvalid(t, composition.Identity(), runtimev2.CodeUnknownMember)
}

func executionParseInvalid(t testing.TB, identity runtimev2.CanonicalResolvedExtensionIdentity, want runtimev2.ValidationCode) {
	t.Helper()
	value, err := sqlrs.ParseExecutionEnvironmentIdentity(identity)
	var validation *runtimev2.ValidationError
	if !errors.Is(err, runtimev2.ErrInvalid) || !errors.As(err, &validation) || !reflect.DeepEqual(value, sqlrs.ExecutionEnvironmentInput{}) || want != "" && validation.Code != want {
		t.Fatalf("Parse returned partial/accepted invalid identity: %v", err)
	}
}

func TestExecutionFileReadViewsAndGeneratedLimits(t *testing.T) {
	for _, executable := range []bool{false, true} {
		v := executionResources(t)
		v.Inputs[0].Profile = sqlrs.ExecutionInputRegularFile
		v.Inputs[0].Executable = executable
		v.Workdir = sqlrs.ExecutionPath{Root: "image", Relative: "work"}
		v.Argv[1][1].Path.Relative = ""
		identity := executionBuild(t, v)
		parsed, err := sqlrs.ParseExecutionEnvironmentIdentity(identity)
		if err != nil || parsed.Inputs[0].Executable != executable {
			t.Fatalf("file mode read: %v", err)
		}
	}
	for _, count := range []int{245, 250} {
		v := executionBase()
		v.Argv = make([]sqlrs.ExecutionText, count)
		for i := range v.Argv {
			v.Argv[i] = executionLiteral(strings.Repeat("x", 4096))
		}
		identity, err := sqlrs.BuildExecutionEnvironmentIdentity(v)
		if count == 245 {
			if err != nil {
				t.Fatalf("bounded control unexpectedly fails: %v", err)
			}
		} else {
			executionInvalid(t, identity, err)
			var validation *runtimev2.ValidationError
			errors.As(err, &validation)
			if validation.Code != runtimev2.CodeLimitExceeded {
				t.Fatal("generated bytes did not hit the limit")
			}
		}
	}
	// Many short path parts cross the node budget without a raw count/text
	// overflow. Every source/path reference remains valid and bounded.
	v := executionResources(t)
	v.Environment = nil
	v.Argv = []sqlrs.ExecutionText{executionLiteral("tool")}
	for range 64 {
		word := sqlrs.ExecutionText{}
		for range 8 {
			word = append(word, executionPathWord("mount:project", "")[0])
		}
		v.Argv = append(v.Argv, word)
	}
	// Keep both declared secrets/services used without increasing the large argv.
	v.Environment = executionResources(t).Environment
	identity, err := sqlrs.BuildExecutionEnvironmentIdentity(v)
	executionInvalid(t, identity, err)
	var validation *runtimev2.ValidationError
	errors.As(err, &validation)
	if validation.Code != runtimev2.CodeLimitExceeded {
		t.Fatalf("generated nodes: %v", err)
	}
	// Each node is legal, but the combined forest exceeds 4096 nodes. Parse
	// must reject the aggregate before trying to interpret its malformed shape.
	fields := executionMinimalFields()
	large := make([]any, 255)
	for i := range large {
		large[i] = map[string]any{"a": "b", "c": "d", "e": "f", "g": "h"}
	}
	fields["inputs"], fields["secrets"] = large, large
	executionParseInvalid(t, executionGeneric(t, fields), runtimev2.CodeLimitExceeded)
	// An otherwise valid generic map with a wrong nested type is independently
	// encoded; it is not a checksum-corruption test.
	control := executionGeneric(t, executionMinimalFields())
	before, _ := runtimev2.CanonicalResolvedExtensionBytes(control)
	parsed, err := sqlrs.ParseExecutionEnvironmentIdentity(control)
	if err != nil {
		t.Fatal(err)
	}
	after, _ := runtimev2.CanonicalResolvedExtensionBytes(executionBuild(t, parsed))
	if !bytes.Equal(before, after) {
		t.Fatal("control was normalized unexpectedly")
	}
}

func TestExecutionAggregateForestLimitsBeforeConstructionAndTraversal(t *testing.T) {
	for _, count := range []int{130, 150} {
		t.Run(fmt.Sprint(count), func(t *testing.T) {
			v := executionBase()
			for i := range count {
				name := fmt.Sprintf("source-%03d", i)
				v.Inputs = append(v.Inputs, sqlrs.ExecutionInputBinding{Name: name, Profile: sqlrs.ExecutionInputRegularTree, Source: sqlrs.ExecutionInputReference{Provider: "files", Kind: "tree", IdentitySchema: "files.tree.v1", Fingerprint: runtimev2.CanonicalFingerprint(executionDigest)}})
				v.Mounts = append(v.Mounts, sqlrs.ExecutionMount{Name: name, InputName: name, Kind: sqlrs.ExecutionMountInput, Target: sqlrs.ExecutionPath{Root: "image", Relative: name}, ReadOnly: true})
			}
			identity, err := sqlrs.BuildExecutionEnvironmentIdentity(v)
			if count == 130 {
				if err != nil {
					t.Fatalf("legal forest rejected: %v", err)
				}
				return
			}
			executionInvalid(t, identity, err)
			var validation *runtimev2.ValidationError
			errors.As(err, &validation)
			if validation.Code != runtimev2.CodeLimitExceeded || validation.Path != "fields" {
				t.Fatalf("did not reject combined forest budget: %v", err)
			}
		})
	}
	// The byte sum exceeds the budget while node counts and each individual
	// value remain legal; semantic shape is intentionally not traversed first.
	fields := executionMinimalFields()
	stringsNode := make([]any, 128)
	for i := range stringsNode {
		stringsNode[i] = strings.Repeat("x", runtimev2.MaxCanonicalStringBytes)
	}
	fields["inputs"], fields["secrets"] = stringsNode, stringsNode
	executionParseInvalid(t, executionGeneric(t, fields), runtimev2.CodeLimitExceeded)
}
