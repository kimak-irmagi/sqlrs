package runtimev2_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

	runtime "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
	"github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/schemaauthor"
	"github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/schemas/sqlrs"
)

// DL01–DL05/DL08: independent wire oracles for the approved declaration limits.
// Requirements: docs/architecture/runtime-go-declaration-limits-tests.md.
const dlValueLimit = 256 << 10

type dlValue interface {
	runtime.ExtensionDeclaration
	json.Marshaler
}
type dlDecoder interface {
	dlValue
	json.Unmarshaler
}
type dlRole struct {
	name string
	new  func(runtime.ExtensionSpecificationInput) (dlValue, error)
	zero func() dlDecoder
}

var dlRoles = []dlRole{
	{"input", func(v runtime.ExtensionSpecificationInput) (dlValue, error) { return runtime.NewInputDeclaration(v) }, func() dlDecoder { return &runtime.InputDeclaration{} }},
	{"execution", func(v runtime.ExtensionSpecificationInput) (dlValue, error) {
		return runtime.NewExecutionEnvironmentDeclaration(v)
	}, func() dlDecoder { return &runtime.ExecutionEnvironmentDeclaration{} }},
	{"deployment", func(v runtime.ExtensionSpecificationInput) (dlValue, error) {
		return runtime.NewDeploymentDeclaration(v)
	}, func() dlDecoder { return &runtime.DeploymentDeclaration{} }},
}

// dlWire is an oracle DTO, not a declaration or a size-helper implementation.
// It deliberately has no MarshalJSON and never calls candidate constructors.
type dlWire struct {
	SchemaVersion string                     `json:"schema_version"`
	Owner         string                     `json:"owner"`
	Kind          string                     `json:"kind"`
	Schema        string                     `json:"specification_schema"`
	Fields        []runtime.DeclarationField `json:"fields"`
}

func dlInput(value string) runtime.ExtensionSpecificationInput {
	return runtime.ExtensionSpecificationInput{SchemaVersion: runtime.SchemaVersion, Owner: "example.source", Kind: "opaque", SpecificationSchema: "example.source.v1", Fields: []runtime.DeclarationField{{Name: "payload", Value: value}}}
}
func dlOracle(v runtime.ExtensionSpecificationInput) dlWire {
	return dlWire{v.SchemaVersion, v.Owner, v.Kind, v.SpecificationSchema, v.Fields}
}
func dlJSON(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return b
}
func dlError(t *testing.T, err error, code runtime.ValidationCode, path string) {
	t.Helper()
	var problem *runtime.ValidationError
	if !errors.Is(err, runtime.ErrInvalid) || !errors.As(err, &problem) || problem.Code != code || problem.Path != path {
		t.Fatalf("error = %v; want %s at %s", err, code, path)
	}
}
func dlText(n int, multibyte bool) string {
	if !multibyte {
		return strings.Repeat("x", n)
	}
	return strings.Repeat("я", n/2) + strings.Repeat("x", n%2)
}

func TestDeclarationValueLimits(t *testing.T) {
	for _, role := range dlRoles {
		t.Run(role.name, func(t *testing.T) {
			for _, multibyte := range []bool{false, true} {
				for _, n := range []int{dlValueLimit - 1, dlValueLimit, dlValueLimit + 1} {
					v, err := role.new(dlInput(dlText(n, multibyte)))
					if n > dlValueLimit {
						dlError(t, err, runtime.CodeTooLarge, "fields[0].value")
						if v.Owner() != "" || len(v.Fields()) != 0 {
							t.Fatal("partial value")
						}
						continue
					}
					if err != nil {
						t.Fatalf("%d bytes multibyte=%v: %v", n, multibyte, err)
					}
					if len(v.Fields()[0].Value) != n {
						t.Fatal("value changed")
					}
					decoded := role.zero()
					if err := runtime.DecodeJSON(dlJSON(t, v), decoded); err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(v.Fields(), decoded.Fields()) {
						t.Fatal("round trip changed value")
					}
				}
			}
			for _, value := range []string{"", string([]byte{255})} {
				_, err := role.new(dlInput(value))
				dlError(t, err, runtime.CodeInvalidValue, "fields[0].value")
			}
			for _, count := range []int{255, 256, 257} {
				input := dlInput("v")
				input.Fields = nil
				for i := 0; i < count; i++ {
					input.Fields = append(input.Fields, runtime.DeclarationField{Name: fmt.Sprintf("f%03d", i), Value: "v"})
				}
				_, err := role.new(input)
				if count > 256 {
					dlError(t, err, runtime.CodeTooLarge, "fields")
				} else if err != nil {
					t.Fatal(err)
				}
			}
			for _, n := range []int{127, 128, 129} {
				input := dlInput("v")
				input.Fields[0].Name = "a" + strings.Repeat("b", n-1)
				_, err := role.new(input)
				if n > 128 {
					dlError(t, err, runtime.CodeTooLarge, "fields[0].name")
				} else if err != nil {
					t.Fatal(err)
				}
			}
		})
	}
}

// dlSizedSpec independently fills 16 individually bounded fields to an exact
// encoded boundary. The oracle serializes only plain DTOs with encoding/json.
func dlSizedSpec(t *testing.T, size int, prefix string) (runtime.ExtensionSpecificationInput, []byte) {
	t.Helper()
	input := dlInput("v")
	input.Fields = nil
	for i := 0; i < 16; i++ {
		input.Fields = append(input.Fields, runtime.DeclarationField{Name: fmt.Sprintf("f%02d", i), Value: prefix})
	}
	left := size - len(dlJSON(t, dlOracle(input)))
	for i := range input.Fields {
		n := min(left, dlValueLimit-len(input.Fields[i].Value))
		if n < 0 {
			t.Fatal("bad oracle size")
		}
		input.Fields[i].Value += strings.Repeat("x", n)
		left -= n
	}
	if left != 0 {
		t.Fatal("oracle capacity")
	}
	raw := dlJSON(t, dlOracle(input))
	if len(raw) != size {
		t.Fatal("oracle encoded length")
	}
	return input, raw
}

func TestDeclarationNormalizedAndRawJSONLimits(t *testing.T) {
	for _, role := range dlRoles {
		t.Run(role.name, func(t *testing.T) {
			for _, prefix := range []string{"x", "q\"\\<\n\u2028\u2029"} {
				for _, n := range []int{runtime.MaxJSONBytes - 1, runtime.MaxJSONBytes, runtime.MaxJSONBytes + 1} {
					input, oracle := dlSizedSpec(t, n, prefix)
					value, err := role.new(input)
					if n > runtime.MaxJSONBytes {
						dlError(t, err, runtime.CodeTooLarge, "$")
						continue
					}
					if err != nil {
						t.Fatal(err)
					}
					if !bytes.Equal(dlJSON(t, value), oracle) {
						t.Fatal("normalized bytes differ from independent oracle")
					}
					if err := runtime.DecodeJSON(oracle, role.zero()); err != nil {
						t.Fatal(err)
					}
				}
			}
			valid := dlJSON(t, dlOracle(dlInput("seed")))
			for _, n := range []int{runtime.MaxJSONBytes - 1, runtime.MaxJSONBytes, runtime.MaxJSONBytes + 1} {
				raw := append(append([]byte(nil), valid[:len(valid)-1]...), bytes.Repeat([]byte(" "), n-len(valid))...)
				raw = append(raw, '}')
				for mode := 0; mode < 3; mode++ {
					decoded := role.zero()
					if err := runtime.DecodeJSON(valid, decoded); err != nil {
						t.Fatal(err)
					}
					var err error
					switch mode {
					case 0:
						err = runtime.DecodeJSON(raw, decoded)
					case 1:
						err = decoded.UnmarshalJSON(raw)
					case 2:
						err = json.Unmarshal(raw, decoded)
					}
					if n > runtime.MaxJSONBytes {
						dlError(t, err, runtime.CodeTooLarge, "$")
					} else if err != nil {
						t.Fatal(err)
					}
					if !bytes.Equal(dlJSON(t, decoded), valid) {
						t.Fatal("decode changed seed")
					}
				}
			}
			_, normalized := dlSizedSpec(t, runtime.MaxJSONBytes+1, "<")
			compact := bytes.ReplaceAll(normalized, []byte(`\u003c`), []byte("<"))
			if len(compact) > runtime.MaxJSONBytes {
				t.Fatal("raw oracle must fit")
			}
			seed := role.zero()
			if err := runtime.DecodeJSON(valid, seed); err != nil {
				t.Fatal(err)
			}
			dlError(t, runtime.DecodeJSON(compact, seed), runtime.CodeTooLarge, "$")
			if !bytes.Equal(dlJSON(t, seed), valid) {
				t.Fatal("normalized overflow mutated receiver")
			}
			outer := append(bytes.Repeat([]byte(" "), runtime.MaxJSONBytes), valid...)
			dlError(t, runtime.DecodeJSON(outer, role.zero()), runtime.CodeTooLarge, "$")
			dlError(t, role.zero().UnmarshalJSON(outer), runtime.CodeTooLarge, "$")
			if err := json.Unmarshal(outer, role.zero()); err != nil {
				t.Fatal("standard JSON must retain its own outer-whitespace behavior", err)
			}
		})
	}
}

func TestDeclarationStrictnessOwnershipAndErrors(t *testing.T) {
	for _, role := range dlRoles {
		t.Run(role.name, func(t *testing.T) {
			input := dlInput(strings.Repeat("x", 8192))
			input.Fields = append(input.Fields, runtime.DeclarationField{Name: "mode", Value: "exact"})
			value, err := role.new(input)
			if err != nil {
				t.Fatal(err)
			}
			input.Fields[0], input.Fields[1] = input.Fields[1], input.Fields[0]
			other, err := role.new(input)
			if err != nil {
				t.Fatal(err)
			}
			if !bytes.Equal(dlJSON(t, value), dlJSON(t, other)) {
				t.Fatal("field permutation changed bytes")
			}
			input.Fields[0].Value = "mutated"
			got := value.Fields()
			got[0].Value = "mutated"
			if value.Fields()[0].Value != "exact" {
				t.Fatal("accepted value aliases caller")
			}
			valid := dlJSON(t, value)
			alternate := bytes.Replace(valid, []byte(`"owner"`), []byte(`"Owner"`), 1)
			if err := runtime.DecodeJSON(alternate, role.zero()); err != nil {
				t.Fatal("changed case matching", err)
			}
			for _, bad := range [][]byte{
				append(append([]byte(nil), valid[:len(valid)-1]...), []byte(`,"unknown":1}`)...),
				append(append([]byte(nil), valid[:len(valid)-1]...), []byte(`,"owner":"example.source"}`)...),
				bytes.Replace(valid, append([]byte(`"fields":`), dlJSON(t, value.Fields())...), []byte(`"fields":null`), 1),
				append(append([]byte(nil), valid...), []byte(` {}`)...),
			} {
				decoded := role.zero()
				if err := runtime.DecodeJSON(valid, decoded); err != nil {
					t.Fatal(err)
				}
				if err := runtime.DecodeJSON(bad, decoded); !errors.Is(err, runtime.ErrInvalid) {
					t.Fatal("strict invalid document accepted", err)
				}
				if !bytes.Equal(dlJSON(t, decoded), valid) {
					t.Fatal("failed decode mutated receiver")
				}
			}
			marker := "synthetic-rejected-value-"
			_, err = role.new(dlInput(strings.Repeat(marker, dlValueLimit/len(marker)+1)))
			dlError(t, err, runtime.CodeTooLarge, "fields[0].value")
			if strings.Contains(err.Error(), marker) {
				t.Fatal("rejected value in error")
			}
		})
	}
}

// Parent oracle DTOs do not use any candidate marshaler or constructor.
type dlParentWire struct {
	Kind         string             `json:"kind"`
	Reference    string             `json:"reference"`
	Arguments    []string           `json:"arguments"`
	Attributes   map[string]string  `json:"attributes"`
	Inputs       []dlWire           `json:"inputs,omitempty"`
	Execution    *dlWire            `json:"execution_environment,omitempty"`
	Deployment   *dlWire            `json:"deployment,omitempty"`
	Capabilities []dlDiagnosticWire `json:"capabilities,omitempty"`
	Portability  []dlDiagnosticWire `json:"portability,omitempty"`
}
type dlDiagnosticWire struct {
	Version string                     `json:"schema_version"`
	Owner   string                     `json:"owner"`
	Kind    string                     `json:"kind"`
	Schema  string                     `json:"observation_schema"`
	Fields  []runtime.DeclarationField `json:"fields"`
}
type dlDocumentWire struct {
	Version     string       `json:"schema_version"`
	Declaration dlParentWire `json:"declaration"`
}
type dlRecipeWire struct {
	Version    string         `json:"schema_version"`
	Factory    dlParentWire   `json:"factory"`
	Transforms []dlParentWire `json:"transforms"`
}

type dlParentCodec interface {
	json.Marshaler
	json.Unmarshaler
}

func dlParentDecoder(kind string) dlParentCodec {
	switch kind {
	case "factory":
		return &runtime.FactoryDeclaration{}
	case "transform":
		return &runtime.TransformDeclaration{}
	case "factory-document":
		return &runtime.FactoryDeclarationDocument{}
	case "transform-document":
		return &runtime.TransformDeclarationDocument{}
	case "recipe":
		return &runtime.RecipeDeclaration{}
	}
	panic("test parent variant")
}

func dlParentFixture(t *testing.T, kind string, target int) ([]byte, func() ([]byte, error)) {
	t.Helper()
	specs := make([]runtime.ExtensionSpecificationInput, 16)
	oracle := dlParentWire{Kind: "execution", Reference: "fixture", Arguments: []string{}, Attributes: map[string]string{}}
	for i := range specs {
		specs[i] = dlInput(strings.Repeat("x", dlValueLimit))
		if i == 15 {
			specs[i] = dlInput("x")
		}
		oracle.Inputs = append(oracle.Inputs, dlOracle(specs[i]))
	}
	env := dlOracle(dlInput("environment"))
	oracle.Execution = &env
	if kind == "factory" || kind == "factory-document" {
		deployment := dlOracle(dlInput("deployment"))
		oracle.Deployment = &deployment
	}
	small := dlParentWire{Kind: "docker", Reference: "base", Arguments: []string{}, Attributes: map[string]string{}}
	wrap := func() any {
		switch kind {
		case "factory-document", "transform-document":
			return dlDocumentWire{runtime.SchemaVersion, oracle}
		case "recipe":
			return dlRecipeWire{runtime.SchemaVersion, small, []dlParentWire{oracle}}
		default:
			return oracle
		}
	}
	left := target - len(dlJSON(t, wrap()))
	if left < 0 || left >= dlValueLimit {
		t.Fatal("parent oracle capacity")
	}
	specs[15].Fields[0].Value += strings.Repeat("x", left)
	oracle.Inputs[15] = dlOracle(specs[15])
	raw := dlJSON(t, wrap())
	if len(raw) != target {
		t.Fatal("parent oracle length")
	}
	inputs := make([]runtime.InputDeclaration, 16)
	for i := range inputs {
		var err error
		inputs[i], err = runtime.NewInputDeclaration(specs[i])
		if err != nil {
			t.Fatal(err)
		}
	}
	environment, err := runtime.NewExecutionEnvironmentDeclaration(dlInput("environment"))
	if err != nil {
		t.Fatal(err)
	}
	transform := runtime.TransformDeclaration{Kind: "execution", Reference: "fixture", Inputs: inputs, ExecutionEnvironment: &environment}
	factory := runtime.FactoryDeclaration{Kind: "execution", Reference: "fixture", Inputs: inputs, ExecutionEnvironment: &environment}
	if oracle.Deployment != nil {
		d, err := runtime.NewDeploymentDeclaration(dlInput("deployment"))
		if err != nil {
			t.Fatal(err)
		}
		factory.Deployment = &d
	}
	return raw, func() ([]byte, error) {
		switch kind {
		case "factory":
			return factory.MarshalJSON()
		case "transform":
			return transform.MarshalJSON()
		case "factory-document":
			d, err := runtime.NewFactoryDeclarationDocument(factory)
			if err != nil {
				return nil, err
			}
			return d.MarshalJSON()
		case "transform-document":
			d, err := runtime.NewTransformDeclarationDocument(transform)
			if err != nil {
				return nil, err
			}
			return d.MarshalJSON()
		case "recipe":
			d, err := runtime.NewRecipeDeclaration(runtime.FactoryDeclaration{Kind: "docker", Reference: "base"}, []runtime.TransformDeclaration{transform})
			if err != nil {
				return nil, err
			}
			return d.MarshalJSON()
		}
		panic("test variant")
	}
}

func TestDeclarationParentAggregateLimits(t *testing.T) {
	for _, kind := range []string{"factory", "transform", "factory-document", "transform-document", "recipe"} {
		t.Run(kind, func(t *testing.T) {
			var seed []byte
			for _, n := range []int{runtime.MaxJSONBytes - 1, runtime.MaxJSONBytes, runtime.MaxJSONBytes + 1} {
				oracle, encode := dlParentFixture(t, kind, n)
				raw, err := encode()
				if n > runtime.MaxJSONBytes {
					dlError(t, err, runtime.CodeTooLarge, "$")
					if len(raw) != 0 {
						t.Fatal("partial encoding")
					}
					decoded := dlParentDecoder(kind)
					if err := runtime.DecodeJSON(seed, decoded); err != nil {
						t.Fatal(err)
					}
					dlError(t, runtime.DecodeJSON(oracle, decoded), runtime.CodeTooLarge, "$")
					if !bytes.Equal(dlJSON(t, decoded), seed) {
						t.Fatal("parent overflow mutated receiver")
					}
					continue
				}
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(raw, oracle) {
					t.Fatal("parent bytes differ from independent oracle")
				}
				decoded := dlParentDecoder(kind)
				if err := runtime.DecodeJSON(oracle, decoded); err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(dlJSON(t, decoded), oracle) {
					t.Fatal("parent round trip changed bytes")
				}
				seed = oracle
			}
		})
	}
	for _, kind := range []string{"factory", "transform"} {
		if kind == "factory" {
			a := runtime.FactoryDeclaration{Kind: "docker", Reference: "base"}
			b := a
			b.Arguments = []string{}
			b.Attributes = map[string]string{}
			b.Inputs = []runtime.InputDeclaration{}
			if !bytes.Equal(dlJSON(t, a), dlJSON(t, b)) {
				t.Fatal("nil normalization changed")
			}
		} else {
			a := runtime.TransformDeclaration{Kind: "execution", Reference: "base"}
			b := a
			b.Arguments = []string{}
			b.Attributes = map[string]string{}
			b.Inputs = []runtime.InputDeclaration{}
			if !bytes.Equal(dlJSON(t, a), dlJSON(t, b)) {
				t.Fatal("nil normalization changed")
			}
		}
	}
}

func TestDeclarationDiagnosticsAndCanonicalLimitsUnchanged(t *testing.T) {
	if runtime.MaxResolvedValueBytes != 4096 || runtime.MaxCanonicalStringBytes != 4096 {
		t.Fatal("unrelated string limits changed")
	}
	for _, kind := range []string{"capability", "portability"} {
		for _, n := range []int{4095, 4096, 4097} {
			input := runtime.DiagnosticObservationInput{SchemaVersion: runtime.SchemaVersion, Owner: "example", Kind: kind, ObservationSchema: "example.observation.v1", Fields: []runtime.DeclarationField{{Name: "note", Value: dlText(n, true)}}}
			var err error
			if kind == "capability" {
				_, err = runtime.NewCapabilityObservation(input)
			} else {
				_, err = runtime.NewPortabilityObservation(input)
			}
			if n > 4096 {
				dlError(t, err, runtime.CodeTooLarge, "fields[0].value")
			} else if err != nil {
				t.Fatal(err)
			}
		}
	}
	if _, err := runtime.CanonicalString(strings.Repeat("x", 4097)); err == nil {
		t.Fatal("canonical limit enlarged")
	}
	input := runtime.DiagnosticObservationInput{SchemaVersion: runtime.SchemaVersion, Owner: "example", Kind: "capability", ObservationSchema: "example.observation.v1"}
	for i := 0; i < 256; i++ {
		input.Fields = append(input.Fields, runtime.DeclarationField{Name: fmt.Sprintf("f%03d", i), Value: strings.Repeat("<", 4096)})
	}
	capability, err := runtime.NewCapabilityObservation(input)
	if err != nil {
		t.Fatal("standalone diagnostic contract changed", err)
	}
	_, err = (runtime.FactoryDeclaration{Kind: "docker", Reference: "base", Capabilities: []runtime.CapabilityObservation{capability}}).MarshalJSON()
	dlError(t, err, runtime.CodeTooLarge, "$")
	for i := 0; i < 3; i++ {
		d := runtime.TransformDeclaration{Kind: "execution", Reference: "base"}
		path := "reference"
		switch i {
		case 0:
			d.Reference = strings.Repeat("x", 4097)
		case 1:
			d.Arguments = []string{strings.Repeat("x", 4097)}
			path = "arguments[0]"
		case 2:
			d.Attributes = map[string]string{"note": strings.Repeat("x", 4097)}
			path = "attributes.note"
		}
		_, err := d.MarshalJSON()
		dlError(t, err, runtime.CodeTooLarge, path)
	}
}

func TestDeclarationLargeSourceProductionFacade(t *testing.T) {
	identity, err := sqlrs.BuildExecutionEnvironmentIdentity(sqlrs.ExecutionEnvironmentInput{
		Runtime: sqlrs.ExecutionRuntime{Kind: sqlrs.ExecutionRuntimeOCI, IdentityKind: sqlrs.ExecutionIdentityImageManifest, IdentityDigest: "sha256:" + strings.Repeat("a", 64), Platform: sqlrs.ExecutionPlatform{OS: "linux", Architecture: "amd64"}},
		Argv:    []sqlrs.ExecutionText{{{Kind: sqlrs.ExecutionTextPartLiteral, Literal: "tool"}}}, Workdir: sqlrs.ExecutionPath{Root: "image"}, User: "0:0", Network: sqlrs.ExecutionNetworkNone,
	})
	if err != nil {
		t.Fatal(err)
	}
	source := dlInput(strings.Repeat("x", dlValueLimit))
	source.Owner = "sqlrs.execution"
	source.Kind = "environment"
	decl, err := runtime.NewExecutionEnvironmentDeclaration(source)
	if err != nil {
		t.Fatal(err)
	}
	_, err = sqlrs.BuildExecutionTransform(runtime.TransformDeclaration{Kind: "execution", Reference: "fixture", ExecutionEnvironment: &decl}, runtime.ResolvedTransformExtensions{ExecutionEnvironment: &identity}, nil)
	if err != nil {
		t.Fatal("larger source is not usable through production facade", err)
	}
	observation := runtime.DiagnosticObservationInput{SchemaVersion: runtime.SchemaVersion, Owner: "example", Kind: "capability", ObservationSchema: "example.observation.v1"}
	for i := 0; i < 256; i++ {
		observation.Fields = append(observation.Fields, runtime.DeclarationField{Name: fmt.Sprintf("f%03d", i), Value: strings.Repeat("<", 4096)})
	}
	capability, err := runtime.NewCapabilityObservation(observation)
	if err != nil {
		t.Fatal(err)
	}
	result, err := sqlrs.BuildExecutionTransform(runtime.TransformDeclaration{Kind: "execution", Reference: "fixture", ExecutionEnvironment: &decl, Capabilities: []runtime.CapabilityObservation{capability}}, runtime.ResolvedTransformExtensions{ExecutionEnvironment: &identity}, nil)
	dlError(t, err, runtime.CodeLimitExceeded, "declaration")
	if !reflect.DeepEqual(result, runtime.TransformIdentityComposition{}) {
		t.Fatal("facade returned partial composition")
	}
}

// Mixed optional children and legacy collections must use the same wire
// selection/escaping in budget checks as in successful normalized encoding.
func TestDeclarationMixedChildrenNormalization(t *testing.T) {
	source := dlInput("q\"\\<>&\n\u2028\u2029я")
	input, err := runtime.NewInputDeclaration(source)
	if err != nil {
		t.Fatal(err)
	}
	environment, err := runtime.NewExecutionEnvironmentDeclaration(source)
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := runtime.NewDeploymentDeclaration(source)
	if err != nil {
		t.Fatal(err)
	}
	diagnostic := runtime.DiagnosticObservationInput{SchemaVersion: runtime.SchemaVersion, Owner: "example", Kind: "observation", ObservationSchema: "example.observation.v1", Fields: source.Fields}
	capability, err := runtime.NewCapabilityObservation(diagnostic)
	if err != nil {
		t.Fatal(err)
	}
	portability, err := runtime.NewPortabilityObservation(diagnostic)
	if err != nil {
		t.Fatal(err)
	}
	diagOracle := dlDiagnosticWire{diagnostic.SchemaVersion, diagnostic.Owner, diagnostic.Kind, diagnostic.ObservationSchema, diagnostic.Fields}
	extension := dlOracle(source)
	oracle := dlParentWire{Kind: "docker", Reference: "q\"<я", Arguments: []string{"q\"\\<>&\n\u2028\u2029я"}, Attributes: map[string]string{"mode": "<>&я"}, Inputs: []dlWire{extension}, Execution: &extension, Deployment: &extension, Capabilities: []dlDiagnosticWire{diagOracle}, Portability: []dlDiagnosticWire{diagOracle}}
	factory := runtime.FactoryDeclaration{Kind: oracle.Kind, Reference: oracle.Reference, Arguments: oracle.Arguments, Attributes: oracle.Attributes, Inputs: []runtime.InputDeclaration{input}, ExecutionEnvironment: &environment, Deployment: &deployment, Capabilities: []runtime.CapabilityObservation{capability}, Portability: []runtime.PortabilityObservation{portability}}
	if !bytes.Equal(dlJSON(t, factory), dlJSON(t, oracle)) {
		t.Fatal("mixed factory wire differs from independent oracle")
	}
	transform := runtime.TransformDeclaration{Kind: factory.Kind, Reference: factory.Reference, Arguments: factory.Arguments, Attributes: factory.Attributes, Inputs: factory.Inputs, ExecutionEnvironment: factory.ExecutionEnvironment, Capabilities: factory.Capabilities, Portability: factory.Portability}
	oracle.Deployment = nil
	if !bytes.Equal(dlJSON(t, transform), dlJSON(t, oracle)) {
		t.Fatal("mixed transform wire differs from independent oracle")
	}
}

// DL05: low-level canonical builders historically propagate declaration
// validation codes/prefixes, while BuildExecutionTransform normalizes them to
// its closed canonical taxonomy. A size change must preserve both boundaries.
func TestDeclarationCanonicalBuilderAggregateFailure(t *testing.T) {
	factorySchema, err := schemaauthor.NewFactorySchema(schemaauthor.SchemaInput{Provider: "example", SemanticKind: "docker", IdentitySchema: "example.factory.v1"})
	if err != nil {
		t.Fatal(err)
	}
	transformSchema, err := schemaauthor.NewTransformSchema(schemaauthor.SchemaInput{Provider: "example", SemanticKind: "execution", IdentitySchema: "example.transform.v1"})
	if err != nil {
		t.Fatal(err)
	}
	input, err := runtime.NewInputDeclaration(dlInput(strings.Repeat("x", dlValueLimit)))
	if err != nil {
		t.Fatal(err)
	}
	factory := runtime.FactoryDeclaration{Kind: "docker", Reference: "base", Inputs: []runtime.InputDeclaration{input}}
	transform := runtime.TransformDeclaration{Kind: "execution", Reference: "step", Inputs: []runtime.InputDeclaration{input}}
	if _, err := runtime.NewFactoryIdentityBuilder(factorySchema, factory); err != nil {
		t.Fatal(err)
	}
	if _, err := runtime.NewTransformIdentityBuilder(transformSchema, transform); err != nil {
		t.Fatal(err)
	}
	for i := 1; i < 16; i++ {
		factory.Inputs = append(factory.Inputs, input)
		transform.Inputs = append(transform.Inputs, input)
	}
	factoryBuilder, err := runtime.NewFactoryIdentityBuilder(factorySchema, factory)
	dlError(t, err, runtime.CodeTooLarge, "declaration")
	if factoryBuilder != nil {
		t.Fatal("partial factory builder")
	}
	transformBuilder, err := runtime.NewTransformIdentityBuilder(transformSchema, transform)
	dlError(t, err, runtime.CodeTooLarge, "declaration")
	if transformBuilder != nil {
		t.Fatal("partial transform builder")
	}
}
