package sqlrs_test

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"sync"
	"testing"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
	"github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/schemaauthor"
	"github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/schemas/sqlrs"
)

// ET01–ET07 requirements: runtime-v2-execution-transform-tests.md.
var _ func(runtimev2.TransformDeclaration, runtimev2.ResolvedTransformExtensions, []runtimev2.OperationalObservation) (runtimev2.TransformIdentityComposition, error) = sqlrs.BuildExecutionTransform

func executionTransformDeclaration(t testing.TB) runtimev2.TransformDeclaration {
	t.Helper()
	environment, err := runtimev2.NewExecutionEnvironmentDeclaration(runtimev2.ExtensionSpecificationInput{SchemaVersion: runtimev2.SchemaVersion, Owner: "sqlrs.execution", Kind: "environment", SpecificationSchema: "sqlrs.execution.declaration.v1"})
	if err != nil {
		t.Fatal(err)
	}
	return runtimev2.TransformDeclaration{Kind: "execution", Reference: "native-command", ExecutionEnvironment: &environment}
}

func executionTransformInput(t testing.TB, digest string) (runtimev2.InputDeclaration, runtimev2.CanonicalResolvedExtensionIdentity) {
	t.Helper()
	declaration, err := runtimev2.NewInputDeclaration(runtimev2.ExtensionSpecificationInput{SchemaVersion: runtimev2.SchemaVersion, Owner: "sqlrs.workspace", Kind: "file", SpecificationSchema: "sqlrs.workspace-file.declaration.v1"})
	if err != nil {
		t.Fatal(err)
	}
	builder, err := runtimev2.NewExtensionIdentityBuilder(sqlrs.WorkspaceFileExtensionSchema())
	if err != nil {
		t.Fatal(err)
	}
	field, err := runtimev2.NewTextIdentityField("content.digest", digest)
	if err != nil {
		t.Fatal(err)
	}
	if err = builder.AddIdentityField(field); err != nil {
		t.Fatal(err)
	}
	composition, err := builder.Build()
	if err != nil {
		t.Fatal(err)
	}
	return declaration, composition.Identity()
}

func executionTransformObservation(t testing.TB, owner, kind, schema string, fields map[string]string) runtimev2.OperationalObservation {
	t.Helper()
	observation, err := runtimev2.NewOperationalObservation(owner, kind, schema, fields)
	if err != nil {
		t.Fatal(err)
	}
	return observation
}

func executionTransformJob(t testing.TB, job string) runtimev2.OperationalObservation {
	return executionTransformObservation(t, "sqlrs", "execution", "sqlrs.execution-transform.observation.v1", map[string]string{"job_id": job})
}

func executionTransformBuild(t testing.TB, d runtimev2.TransformDeclaration, e runtimev2.ResolvedTransformExtensions, observations ...runtimev2.OperationalObservation) runtimev2.TransformIdentityComposition {
	t.Helper()
	result, err := sqlrs.BuildExecutionTransform(d, e, observations)
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func executionTransformEnvelope(t testing.TB, c runtimev2.TransformIdentityComposition) runtimev2.FingerprintEnvelope {
	t.Helper()
	e, err := runtimev2.NewTransformEnvelope(c.Identity())
	if err != nil {
		t.Fatal(err)
	}
	return e
}

func TestExecutionTransformPublicProfilesAndExactBindings(t *testing.T) {
	for _, kind := range []sqlrs.ExecutionRuntimeKind{sqlrs.ExecutionRuntimeOCI, sqlrs.ExecutionRuntimeHost, sqlrs.ExecutionRuntimeDatabaseContainer} {
		for _, count := range []int{0, 2, runtimev2.MaxResolvedFields} {
			t.Run(fmt.Sprintf("%s/%d", kind, count), func(t *testing.T) {
				input := executionBase()
				input.Runtime.Kind = kind
				if kind == sqlrs.ExecutionRuntimeHost {
					input.Runtime.IdentityKind, input.Workdir.Root, input.User = sqlrs.ExecutionIdentityHostExecutable, "workspace", "uid:1000"
					input.Argv[0] = executionPathWord("tool", "")
				}
				if kind == sqlrs.ExecutionRuntimeDatabaseContainer {
					input.Runtime.DatabaseService, input.Network = "database", sqlrs.ExecutionNetworkServices
					input.Services = []sqlrs.ExecutionServiceBinding{{Role: "database", Transport: sqlrs.ExecutionServiceTCP, Protocol: "postgresql"}}
				}
				identity := executionBuild(t, input)
				d := executionTransformDeclaration(t)
				e := runtimev2.ResolvedTransformExtensions{ExecutionEnvironment: &identity}
				want := map[string]string{"extension.execution_environment": string(executionFingerprint(t, identity))}
				for n := 0; n < count; n++ {
					decl, value := executionTransformInput(t, fmt.Sprintf("content-%d", n))
					d.Inputs, e.Inputs = append(d.Inputs, decl), append(e.Inputs, value)
					want[fmt.Sprintf("extension.input.%d", n)] = string(executionFingerprint(t, value))
				}
				envelope := executionTransformEnvelope(t, executionTransformBuild(t, d, e))
				descriptor := envelope.Descriptor()
				if descriptor.Provider() != "sqlrs" || descriptor.SemanticKind() != "execution" || descriptor.IdentitySchema() != "sqlrs.execution-transform.v1" || descriptor.Domain() != runtimev2.CanonicalTransformDomain {
					t.Fatal("wrong fixed schema")
				}
				raw, err := json.Marshal(envelope)
				if err != nil {
					t.Fatal(err)
				}
				var wire struct {
					Subject struct {
						Fields []struct{ Name, Kind, Text string }
					}
				}
				if err = json.Unmarshal(raw, &wire); err != nil {
					t.Fatal(err)
				}
				got := map[string]string{}
				for _, field := range wire.Subject.Fields {
					if field.Kind != "text" {
						t.Fatal("binding is not typed text")
					}
					got[field.Name] = field.Text
				}
				if len(wire.Subject.Fields) != len(want) || !reflect.DeepEqual(got, want) {
					t.Fatal("incorrect binding names, positions or independently computed fingerprints")
				}
			})
		}
	}
}

func TestExecutionTransformClosedFailuresAreAtomicAndSafe(t *testing.T) {
	const marker = "protected_caller_marker"
	type inputs struct {
		d runtimev2.TransformDeclaration
		e runtimev2.ResolvedTransformExtensions
		o []runtimev2.OperationalObservation
	}
	for _, tc := range []struct {
		name   string
		code   runtimev2.ValidationCode
		path   string
		change func(*inputs)
	}{
		{"missing declaration", runtimev2.CodeShapeInvalid, "declaration.execution_environment", func(v *inputs) { v.d.ExecutionEnvironment = nil }},
		{"zero declaration", runtimev2.CodeShapeInvalid, "declaration.execution_environment", func(v *inputs) { v.d.ExecutionEnvironment = &runtimev2.ExecutionEnvironmentDeclaration{} }},
		{"wrong declaration owner", runtimev2.CodeValueInvalid, "declaration", func(v *inputs) {
			e, err := runtimev2.NewExecutionEnvironmentDeclaration(runtimev2.ExtensionSpecificationInput{SchemaVersion: runtimev2.SchemaVersion, Owner: marker, Kind: "environment", SpecificationSchema: "other.v1"})
			if err != nil {
				t.Fatal(err)
			}
			v.d.ExecutionEnvironment = &e
		}},
		{"wrong declaration kind", runtimev2.CodeValueInvalid, "declaration", func(v *inputs) {
			e, err := runtimev2.NewExecutionEnvironmentDeclaration(runtimev2.ExtensionSpecificationInput{SchemaVersion: runtimev2.SchemaVersion, Owner: "sqlrs.execution", Kind: "wrong", SpecificationSchema: "other.v1"})
			if err != nil {
				t.Fatal(err)
			}
			v.d.ExecutionEnvironment = &e
		}},
		{"kind", runtimev2.CodeValueInvalid, "declaration", func(v *inputs) { v.d.Kind = marker }},
		{"empty kind", runtimev2.CodeValueInvalid, "declaration", func(v *inputs) { v.d.Kind = "" }},
		{"arguments", runtimev2.CodeValueInvalid, "declaration", func(v *inputs) { v.d.Arguments = []string{marker} }},
		{"attributes", runtimev2.CodeValueInvalid, "declaration", func(v *inputs) { v.d.Attributes = map[string]string{marker: marker} }},
		{"empty reference", runtimev2.CodeValueInvalid, "declaration", func(v *inputs) { v.d.Reference = "" }},
		{"invalid reference", runtimev2.CodeValueInvalid, "declaration", func(v *inputs) { v.d.Reference = marker + "\xff" }},
		{"reference limit", runtimev2.CodeLimitExceeded, "declaration", func(v *inputs) { v.d.Reference = strings.Repeat("x", runtimev2.MaxResolvedValueBytes+1) }},
		{"input declaration limit", runtimev2.CodeLimitExceeded, "declaration", func(v *inputs) {
			for len(v.d.Inputs) <= runtimev2.MaxResolvedFields {
				v.d.Inputs = append(v.d.Inputs, v.d.Inputs[0])
				v.e.Inputs = append(v.e.Inputs, v.e.Inputs[0])
			}
		}},
		{"zero input declaration", runtimev2.CodeShapeInvalid, "declaration", func(v *inputs) { v.d.Inputs[0] = runtimev2.InputDeclaration{} }},
		{"zero capability", runtimev2.CodeShapeInvalid, "declaration", func(v *inputs) { v.d.Capabilities = []runtimev2.CapabilityObservation{{}} }},
		{"zero portability", runtimev2.CodeShapeInvalid, "declaration", func(v *inputs) { v.d.Portability = []runtimev2.PortabilityObservation{{}} }},
		{"missing resolved environment", runtimev2.CodeShapeInvalid, "extensions.execution_environment", func(v *inputs) { v.e.ExecutionEnvironment = nil }},
		{"zero resolved environment", runtimev2.CodeShapeInvalid, "extensions.execution_environment", func(v *inputs) { v.e.ExecutionEnvironment = &runtimev2.CanonicalResolvedExtensionIdentity{} }},
		{"foreign resolved environment", runtimev2.CodeShapeInvalid, "extensions.execution_environment", func(v *inputs) { v.e.ExecutionEnvironment = &v.e.Inputs[0] }},
		{"missing input", runtimev2.CodeShapeInvalid, "extensions.inputs", func(v *inputs) { v.e.Inputs = nil }},
		{"extra input", runtimev2.CodeShapeInvalid, "extensions.inputs", func(v *inputs) { v.e.Inputs = append(v.e.Inputs, v.e.Inputs[0]) }},
		{"zero input", runtimev2.CodeShapeInvalid, "extensions.inputs", func(v *inputs) { v.e.Inputs[0] = runtimev2.CanonicalResolvedExtensionIdentity{} }},
		{"wrong input owner", runtimev2.CodeValueInvalid, "extensions.inputs", func(v *inputs) {
			decl, err := runtimev2.NewInputDeclaration(runtimev2.ExtensionSpecificationInput{SchemaVersion: runtimev2.SchemaVersion, Owner: "other", Kind: "file", SpecificationSchema: "other.file.v1"})
			if err != nil {
				t.Fatal(err)
			}
			v.d.Inputs[0] = decl
		}},
		{"wrong input kind", runtimev2.CodeValueInvalid, "extensions.inputs", func(v *inputs) {
			decl, err := runtimev2.NewInputDeclaration(runtimev2.ExtensionSpecificationInput{SchemaVersion: runtimev2.SchemaVersion, Owner: "sqlrs.workspace", Kind: "tree", SpecificationSchema: "sqlrs.workspace-tree.v1"})
			if err != nil {
				t.Fatal(err)
			}
			v.d.Inputs[0] = decl
		}},
		{"zero observation", runtimev2.CodeShapeInvalid, "observations", func(v *inputs) { v.o = append(v.o, runtimev2.OperationalObservation{}) }},
		{"foreign observation owner", runtimev2.CodeValueInvalid, "observations", func(v *inputs) {
			v.o = append(v.o, executionTransformObservation(t, marker, "execution", "sqlrs.execution-transform.observation.v1", nil))
		}},
		{"foreign observation kind", runtimev2.CodeValueInvalid, "observations", func(v *inputs) {
			v.o = append(v.o, executionTransformObservation(t, "sqlrs", marker, "sqlrs.execution-transform.observation.v1", nil))
		}},
		{"foreign observation schema", runtimev2.CodeValueInvalid, "observations", func(v *inputs) {
			v.o = append(v.o, executionTransformObservation(t, "sqlrs", "execution", marker, nil))
		}},
		{"unknown observation key", runtimev2.CodeValueInvalid, "observations", func(v *inputs) {
			v.o = append(v.o, executionTransformObservation(t, "sqlrs", "execution", "sqlrs.execution-transform.observation.v1", map[string]string{marker: marker}))
		}},
		{"observation count", runtimev2.CodeLimitExceeded, "observations", func(v *inputs) {
			for len(v.o) <= runtimev2.MaxResolvedFields {
				v.o = append(v.o, v.o[0])
			}
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			identity := executionBuild(t, executionBase())
			decl, value := executionTransformInput(t, "content")
			v := inputs{d: executionTransformDeclaration(t), e: runtimev2.ResolvedTransformExtensions{ExecutionEnvironment: &identity, Inputs: []runtimev2.CanonicalResolvedExtensionIdentity{value}}, o: []runtimev2.OperationalObservation{executionTransformJob(t, "valid-first")}}
			v.d.Inputs = []runtimev2.InputDeclaration{decl}
			executionTransformBuild(t, v.d, v.e, v.o...) // accepted paired control
			tc.change(&v)
			result, err := sqlrs.BuildExecutionTransform(v.d, v.e, v.o)
			var problem *runtimev2.ValidationError
			if !errors.As(err, &problem) || !errors.Is(err, runtimev2.ErrInvalid) || problem.Code != tc.code || problem.Path != tc.path {
				t.Fatalf("wrong closed error: %v", err)
			}
			if !reflect.DeepEqual(result, runtimev2.TransformIdentityComposition{}) {
				t.Fatal("partial result escaped after failure")
			}
			if strings.Contains(fmt.Sprintf("%s %+v %#v %s %s", err, err, err, problem.Code, problem.Path), marker) {
				t.Fatal("caller marker escaped in error")
			}
		})
	}
}

func TestExecutionTransformRejectsForgedEnvironmentSemantics(t *testing.T) {
	for _, tc := range []struct {
		name string
		code runtimev2.ValidationCode
	}{{"nested", runtimev2.CodeValueInvalid}, {"noncanonical", runtimev2.CodeNonCanonical}, {"schema", runtimev2.CodeShapeInvalid}} {
		t.Run(tc.name, func(t *testing.T) {
			identity := executionTransformForged(t, tc.name)
			if identity.Provider() != "sqlrs.execution" || identity.Kind() != "environment" {
				t.Fatal("negative control does not cross an owner-only check")
			}
			result, err := sqlrs.BuildExecutionTransform(executionTransformDeclaration(t), runtimev2.ResolvedTransformExtensions{ExecutionEnvironment: &identity}, nil)
			var problem *runtimev2.ValidationError
			if !errors.As(err, &problem) || problem.Code != tc.code || problem.Path != "extensions.execution_environment" || !reflect.DeepEqual(result, runtimev2.TransformIdentityComposition{}) {
				t.Fatalf("invalid cached execution identity accepted: %v", err)
			}
		})
	}
}

func executionTransformForged(t testing.TB, mode string) runtimev2.CanonicalResolvedExtensionIdentity {
	t.Helper()
	fields := executionMinimalFields()
	if mode == "nested" {
		fields["primitive"] = "unsupported"
	}
	if mode == "noncanonical" {
		fields["invocation"].(map[string]any)["argv"] = []any{[]any{map[string]any{"kind": "literal", "literal": "to"}, map[string]any{"kind": "literal", "literal": "ol"}}}
	}
	definition := schemaauthor.SchemaInput{Provider: "sqlrs.execution", SemanticKind: "environment", IdentitySchema: "sqlrs.execution-environment.v1"}
	if mode == "schema" {
		definition.IdentitySchema = "sqlrs.other.v1"
	}
	for name := range fields {
		kind, disclosure := runtimev2.IdentityFieldCanonicalValue, schemaauthor.DisclosureProtected
		if name == "primitive" {
			kind, disclosure = runtimev2.IdentityFieldText, schemaauthor.DisclosurePublic
		}
		definition.Fields = append(definition.Fields, schemaauthor.FieldDefinition{Name: name, Kind: kind, Role: schemaauthor.FieldRoleSemantic, Required: true, Disclosure: disclosure})
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
	return composition.Identity()
}

func TestExecutionTransformSensitivityLineageAndMetadata(t *testing.T) {
	base := executionBase()
	identity := executionBuild(t, base)
	d := executionTransformDeclaration(t)
	decl, first := executionTransformInput(t, "first")
	_, second := executionTransformInput(t, "second")
	d.Inputs = []runtimev2.InputDeclaration{decl, decl}
	e := runtimev2.ResolvedTransformExtensions{ExecutionEnvironment: &identity, Inputs: []runtimev2.CanonicalResolvedExtensionIdentity{first, second}}
	original := executionTransformEnvelope(t, executionTransformBuild(t, d, e))
	parent := runtimev2.CanonicalStateID(executionDigest)
	child := func(envelope runtimev2.FingerprintEnvelope, p runtimev2.CanonicalStateID) runtimev2.FingerprintEnvelope {
		c, err := runtimev2.NewDerivedStateEnvelope(p, envelope)
		if err != nil {
			t.Fatal(err)
		}
		return c
	}
	originalChild := child(original, parent)
	for name, change := range map[string]func(*sqlrs.ExecutionEnvironmentInput){"command": func(v *sqlrs.ExecutionEnvironmentInput) { v.Argv[0] = executionLiteral("other") }, "environment": func(v *sqlrs.ExecutionEnvironmentInput) {
		v.Environment = []sqlrs.ExecutionEnvironmentVariable{{Name: "VAR", Value: executionLiteral("value")}}
	}, "image": func(v *sqlrs.ExecutionEnvironmentInput) {
		v.Runtime.IdentityDigest = "sha256:" + strings.Repeat("2", 64)
	}, "platform": func(v *sqlrs.ExecutionEnvironmentInput) { v.Runtime.Platform.Architecture = "arm64" }} {
		t.Run(name, func(t *testing.T) {
			v := executionBase()
			change(&v)
			changed := executionBuild(t, v)
			extensions := e
			extensions.ExecutionEnvironment = &changed
			envelope := executionTransformEnvelope(t, executionTransformBuild(t, d, extensions))
			if envelope.Descriptor().Digest() == original.Descriptor().Digest() || child(envelope, parent).Descriptor().Digest() == originalChild.Descriptor().Digest() {
				t.Fatal("semantic change omitted from transform or child")
			}
		})
	}
	for _, pair := range [][]runtimev2.CanonicalResolvedExtensionIdentity{{second, first}, {first, first}} {
		changed := e
		changed.Inputs = pair
		envelope := executionTransformEnvelope(t, executionTransformBuild(t, d, changed))
		if envelope.Descriptor().Digest() == original.Descriptor().Digest() || child(envelope, parent).Descriptor().Digest() == originalChild.Descriptor().Digest() {
			t.Fatal("input order or identity ignored")
		}
	}
	capability, err := runtimev2.NewCapabilityObservation(runtimev2.DiagnosticObservationInput{SchemaVersion: runtimev2.SchemaVersion, Owner: "node", Kind: "capability", ObservationSchema: "node.capability.v1"})
	if err != nil {
		t.Fatal(err)
	}
	portability, err := runtimev2.NewPortabilityObservation(runtimev2.DiagnosticObservationInput{SchemaVersion: runtimev2.SchemaVersion, Owner: "node", Kind: "portability", ObservationSchema: "node.portability.v1"})
	if err != nil {
		t.Fatal(err)
	}
	d.Reference, d.Capabilities, d.Portability = strings.Repeat("x", runtimev2.MaxResolvedValueBytes), []runtimev2.CapabilityObservation{capability}, []runtimev2.PortabilityObservation{portability}
	metadata := executionTransformEnvelope(t, executionTransformBuild(t, d, e, executionTransformJob(t, "other-job")))
	if metadata.Descriptor().Digest() != original.Descriptor().Digest() || child(metadata, parent).Descriptor().Digest() != originalChild.Descriptor().Digest() {
		t.Fatal("operational metadata changed identity")
	}
	if child(original, runtimev2.CanonicalStateID("sha256:"+strings.Repeat("2", 64))).Descriptor().Digest() == originalChild.Descriptor().Digest() {
		t.Fatal("parent omitted from child")
	}
	for _, envelope := range []runtimev2.FingerprintEnvelope{original, originalChild} {
		raw, err := json.Marshal(envelope)
		if err != nil {
			t.Fatal(err)
		}
		var decoded runtimev2.FingerprintEnvelope
		if err = json.Unmarshal(raw, &decoded); err != nil || decoded.Descriptor().Digest() != envelope.Descriptor().Digest() {
			t.Fatalf("envelope round trip: %v", err)
		}
		corrupt := bytes.Replace(raw, []byte(envelope.Descriptor().Digest()), []byte("sha256:"+strings.Repeat("0", 64)), 1)
		if err = json.Unmarshal(corrupt, &decoded); err == nil {
			t.Fatal("tampered commitment accepted")
		}
	}
}

func TestExecutionTransformObservationOwnershipAndConcurrency(t *testing.T) {
	identity := executionBuild(t, executionBase())
	d := executionTransformDeclaration(t)
	e := runtimev2.ResolvedTransformExtensions{ExecutionEnvironment: &identity}
	inputDeclaration, inputIdentity := executionTransformInput(t, "immutable-input")
	d.Inputs, e.Inputs = []runtimev2.InputDeclaration{inputDeclaration}, []runtimev2.CanonicalResolvedExtensionIdentity{inputIdentity}
	fields := map[string]string{"job_id": "first"}
	first := executionTransformObservation(t, "sqlrs", "execution", "sqlrs.execution-transform.observation.v1", fields)
	want := []runtimev2.OperationalObservation{first, executionTransformJob(t, "second"), first}
	for _, count := range []int{0, 3, runtimev2.MaxResolvedFields} {
		observations := make([]runtimev2.OperationalObservation, count)
		for n := range observations {
			observations[n] = want[n%len(want)]
		}
		result := executionTransformBuild(t, d, e, observations...)
		if !reflect.DeepEqual(result.Observations(), observations) {
			t.Fatal("observation values/order/repeats lost")
		}
	}
	observations := append([]runtimev2.OperationalObservation(nil), want...)
	result := executionTransformBuild(t, d, e, observations...)
	before, err := runtimev2.CanonicalTransformIdentityBytes(result.Identity())
	if err != nil {
		t.Fatal(err)
	}
	fields["job_id"], observations[0], d.Arguments, d.Attributes = "mutated", runtimev2.OperationalObservation{}, []string{"mutated"}, map[string]string{"mutated": "value"}
	d.Inputs[0], e.Inputs[0] = runtimev2.InputDeclaration{}, runtimev2.CanonicalResolvedExtensionIdentity{}
	view := result.Observations()
	view[0] = runtimev2.OperationalObservation{}
	if !reflect.DeepEqual(result.Observations(), want) {
		t.Fatal("caller mutation escaped immutable observation ownership")
	}
	after, err := runtimev2.CanonicalTransformIdentityBytes(result.Identity())
	if err != nil || !bytes.Equal(before, after) {
		t.Fatal("caller mutation changed identity")
	}
	if _, err = sqlrs.BuildExecutionTransform(d, e, nil); err == nil {
		t.Fatal("invalid call accepted")
	}
	d = executionTransformDeclaration(t)
	d.Inputs, e.Inputs = []runtimev2.InputDeclaration{inputDeclaration}, []runtimev2.CanonicalResolvedExtensionIdentity{inputIdentity}
	valid := executionTransformBuild(t, d, e)
	if executionTransformEnvelope(t, valid).Descriptor().Digest() != executionTransformEnvelope(t, result).Descriptor().Digest() {
		t.Fatal("failed call or observations contaminated retry")
	}
	changed := executionTransformBuild(t, d, e, executionTransformJob(t, "changed"))
	if reflect.DeepEqual(changed.Observations(), result.Observations()) {
		t.Fatal("job metadata was discarded")
	}
	var wg sync.WaitGroup
	for n := 0; n < 12; n++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c, err := sqlrs.BuildExecutionTransform(d, e, want)
			if err != nil {
				t.Error(err)
				return
			}
			raw, err := runtimev2.CanonicalTransformIdentityBytes(c.Identity())
			if err != nil || !bytes.Equal(raw, before) || !reflect.DeepEqual(c.Observations(), want) {
				t.Error("independent concurrent call changed result")
			}
		}()
	}
	wg.Wait()
}

func TestExecutionTransformTypedOrderAndDiagnosticLimits(t *testing.T) {
	identity := executionBuild(t, executionBase())
	d := executionTransformDeclaration(t)
	fileDeclaration, file := executionTransformInput(t, "file")
	executionDeclaration, err := runtimev2.NewInputDeclaration(runtimev2.ExtensionSpecificationInput{SchemaVersion: runtimev2.SchemaVersion, Owner: "sqlrs.execution", Kind: "environment", SpecificationSchema: "sqlrs.execution-input.v1"})
	if err != nil {
		t.Fatal(err)
	}
	d.Inputs = []runtimev2.InputDeclaration{fileDeclaration, executionDeclaration}
	e := runtimev2.ResolvedTransformExtensions{ExecutionEnvironment: &identity, Inputs: []runtimev2.CanonicalResolvedExtensionIdentity{file, identity}}
	executionTransformBuild(t, d, e)
	e.Inputs[0], e.Inputs[1] = e.Inputs[1], e.Inputs[0]
	result, err := sqlrs.BuildExecutionTransform(d, e, nil)
	var problem *runtimev2.ValidationError
	if !errors.As(err, &problem) || problem.Code != runtimev2.CodeValueInvalid || problem.Path != "extensions.inputs" || !reflect.DeepEqual(result, runtimev2.TransformIdentityComposition{}) {
		t.Fatalf("typed reorder accepted: %v", err)
	}
	e.Inputs[0], e.Inputs[1] = e.Inputs[1], e.Inputs[0]
	capability, err := runtimev2.NewCapabilityObservation(runtimev2.DiagnosticObservationInput{SchemaVersion: runtimev2.SchemaVersion, Owner: "node", Kind: "capability", ObservationSchema: "node.capability.v1"})
	if err != nil {
		t.Fatal(err)
	}
	portability, err := runtimev2.NewPortabilityObservation(runtimev2.DiagnosticObservationInput{SchemaVersion: runtimev2.SchemaVersion, Owner: "node", Kind: "portability", ObservationSchema: "node.portability.v1"})
	if err != nil {
		t.Fatal(err)
	}
	for n := 0; n < runtimev2.MaxResolvedFields; n++ {
		d.Capabilities = append(d.Capabilities, capability)
		d.Portability = append(d.Portability, portability)
	}
	executionTransformBuild(t, d, e)
	for _, kind := range []string{"capability", "portability"} {
		changed := d
		if kind == "capability" {
			changed.Capabilities = append(append([]runtimev2.CapabilityObservation(nil), d.Capabilities...), capability)
		} else {
			changed.Portability = append(append([]runtimev2.PortabilityObservation(nil), d.Portability...), portability)
		}
		result, err = sqlrs.BuildExecutionTransform(changed, e, nil)
		if !errors.As(err, &problem) || problem.Code != runtimev2.CodeLimitExceeded || problem.Path != "declaration" || !reflect.DeepEqual(result, runtimev2.TransformIdentityComposition{}) {
			t.Fatalf("diagnostic limit lost: %v", err)
		}
	}
}
