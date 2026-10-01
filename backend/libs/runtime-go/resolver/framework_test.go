package resolver_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
	"github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/resolver"
	"github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/schemaauthor"
)

func TestRegistryDispatchesFullDescriptorAndRejectsDuplicates(t *testing.T) {
	one := &fakeResolver{descriptor: descriptor("one")}
	two := &fakeResolver{descriptor: descriptor("two")}
	registry, err := resolver.NewRegistry(one, two)
	if err != nil {
		t.Fatal(err)
	}
	if got, err := registry.Lookup(one.descriptor); err != nil || got.Descriptor() != one.descriptor {
		t.Fatalf("lookup = %v, %v", got, err)
	}
	partial := one.descriptor
	partial.SpecificationSchema = "other"
	if _, err := registry.Lookup(partial); !errors.Is(err, resolver.ErrUnsupported) {
		t.Fatalf("partial lookup = %v", err)
	}
	if _, err := resolver.NewRegistry(one, one); !errors.Is(err, resolver.ErrDuplicate) {
		t.Fatalf("duplicate = %v", err)
	}
}

func TestManagerCurrentAndFallbackMatrix(t *testing.T) {
	declaration := workspaceDeclaration(t)
	current := resolution(t, declaration)
	for _, test := range []struct {
		name         string
		status       resolver.RevalidationStatus
		resolveCalls int
	}{
		{"current", resolver.Current, 0},
		{"stale", resolver.Stale, 1},
		{"unknown", resolver.Unknown, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			provider := &fakeResolver{descriptor: descriptor("one"), resolution: current, revalidation: resolver.Revalidation{Status: test.status, Reason: "test"}}
			cache := &fakeCache{load: resolver.CacheLoad{Hit: true, Resolution: current}}
			registry, _ := resolver.NewRegistry(provider)
			manager, _ := resolver.NewManager(registry, cache)
			outcome, err := manager.ResolveCurrent(context.Background(), resolver.Workspace{Root: t.TempDir()}, declaration)
			if err != nil {
				t.Fatal(err)
			}
			if outcome.PriorStatus != test.status || provider.resolveCalls != test.resolveCalls || provider.acquireCalls != 0 {
				t.Fatalf("outcome=%+v calls=%d/%d", outcome, provider.resolveCalls, provider.acquireCalls)
			}
			if outcome.Provenance.Resolver != provider.descriptor || len(outcome.Provenance.NormalizedDeclaration) == 0 || outcome.Freshness.Status != test.status || string(outcome.Freshness.Evidence) != string(outcome.Resolution.Evidence) {
				t.Fatalf("metadata = %+v %+v", outcome.Provenance, outcome.Freshness)
			}
			if cache.storeCalls != 1 {
				t.Fatalf("store calls = %d", cache.storeCalls)
			}
		})
	}
}

func TestManagerFallbackErrorRetainsPriorStatus(t *testing.T) {
	declaration := workspaceDeclaration(t)
	provider := &fakeResolver{descriptor: descriptor("one"), resolution: resolution(t, declaration),
		revalidation: resolver.Revalidation{Status: resolver.Unknown, Reason: "weak_evidence"}, resolveErr: errors.New("offline")}
	cache := &fakeCache{load: resolver.CacheLoad{Hit: true, Resolution: provider.resolution}}
	registry, _ := resolver.NewRegistry(provider)
	manager, _ := resolver.NewManager(registry, cache)
	_, err := manager.ResolveCurrent(context.Background(), resolver.Workspace{Root: t.TempDir()}, declaration)
	var structured *resolver.Error
	if !errors.As(err, &structured) || structured.PriorStatus != resolver.Unknown || structured.Reason != "weak_evidence" || structured.Operation != "resolve" {
		t.Fatalf("error = %#v", err)
	}
}

func TestOutcomeDoesNotExposeProviderEvidenceBuffer(t *testing.T) {
	declaration := workspaceDeclaration(t)
	provider := &fakeResolver{descriptor: descriptor("one"), resolution: resolution(t, declaration)}
	registry, err := resolver.NewRegistry(provider)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := resolver.NewManager(registry, &fakeCache{})
	if err != nil {
		t.Fatal(err)
	}
	outcome, err := manager.ResolveCurrent(context.Background(), resolver.Workspace{Root: t.TempDir()}, declaration)
	if err != nil {
		t.Fatal(err)
	}
	outcome.Resolution.Evidence[0] = '['
	if string(provider.resolution.Evidence) != `{"schema_version":"test"}` || string(outcome.Freshness.Evidence) != `{"schema_version":"test"}` {
		t.Fatal("outcome leaked the provider's evidence buffer")
	}
}

type fakeResolver struct {
	descriptor                 resolver.Descriptor
	resolution                 resolver.Resolution
	revalidation               resolver.Revalidation
	resolveErr                 error
	resolveCalls, acquireCalls int
}

func (f *fakeResolver) Descriptor() resolver.Descriptor { return f.descriptor }
func (f *fakeResolver) IdentitySchema() runtimev2.ExtensionIdentitySchema {
	return fileTestSchema(f.descriptor.Owner, f.descriptor.Kind)
}
func fileTestSchema(owner, kind string) runtimev2.ExtensionIdentitySchema {
	schema, _ := schemaauthor.NewExtensionSchema(schemaauthor.SchemaInput{Provider: owner, SemanticKind: kind, IdentitySchema: "file.v1", Fields: []schemaauthor.FieldDefinition{{Name: "content.digest", Role: schemaauthor.FieldRoleSemantic, Kind: runtimev2.IdentityFieldText, Required: false, Disclosure: schemaauthor.DisclosurePublic}}})
	return schema
}
func (f *fakeResolver) Normalize(_ context.Context, _ resolver.Workspace, declaration runtimev2.ExtensionDeclaration) (resolver.NormalizedDeclaration, error) {
	return resolver.NormalizedDeclaration{Declaration: declaration}, nil
}

func TestManagerDispatchesEveryTypedDeclarationRole(t *testing.T) {
	input, err := runtimev2.NewInputDeclaration(runtimev2.ExtensionSpecificationInput{SchemaVersion: runtimev2.SchemaVersion, Owner: "owner", Kind: "kind", SpecificationSchema: "owner.kind.v1", Fields: []runtimev2.DeclarationField{}})
	if err != nil {
		t.Fatal(err)
	}
	environment, err := runtimev2.NewExecutionEnvironmentDeclaration(runtimev2.ExtensionSpecificationInput{SchemaVersion: runtimev2.SchemaVersion, Owner: "owner", Kind: "kind", SpecificationSchema: "owner.kind.v1", Fields: []runtimev2.DeclarationField{}})
	if err != nil {
		t.Fatal(err)
	}
	deployment, err := runtimev2.NewDeploymentDeclaration(runtimev2.ExtensionSpecificationInput{SchemaVersion: runtimev2.SchemaVersion, Owner: "owner", Kind: "kind", SpecificationSchema: "owner.kind.v1", Fields: []runtimev2.DeclarationField{}})
	if err != nil {
		t.Fatal(err)
	}
	declarations := []runtimev2.ExtensionDeclaration{input, environment, deployment}
	providers := make([]*fakeResolver, 0, len(declarations))
	registered := make([]resolver.Resolver, 0, len(declarations))
	for _, declaration := range declarations {
		provider := &fakeResolver{descriptor: resolver.Descriptor{Role: declaration.Role(), Owner: declaration.Owner(), Kind: declaration.Kind(), SpecificationSchema: declaration.SpecificationSchema(), SemanticVersion: "1"}}
		providers = append(providers, provider)
		registered = append(registered, provider)
	}
	registry, err := resolver.NewRegistry(registered...)
	if err != nil {
		t.Fatal(err)
	}
	manager, err := resolver.NewManager(registry, &fakeCache{})
	if err != nil {
		t.Fatal(err)
	}
	workspace := resolver.Workspace{Root: t.TempDir()}
	for index, declaration := range declarations {
		if _, err := manager.ResolveCurrent(context.Background(), workspace, declaration); err != nil {
			t.Fatalf("role %q: %v", declaration.Role(), err)
		}
		if providers[index].resolveCalls != 1 {
			t.Fatalf("role %q resolve calls = %d", declaration.Role(), providers[index].resolveCalls)
		}
	}
}
func (f *fakeResolver) Resolve(context.Context, resolver.Workspace, resolver.NormalizedDeclaration) (resolver.Resolution, error) {
	f.resolveCalls++
	if f.resolveErr != nil {
		return resolver.Resolution{}, f.resolveErr
	}
	return f.resolution, nil
}
func (f *fakeResolver) ValidateResolution(resolver.Resolution) error { return nil }
func (f *fakeResolver) Revalidate(context.Context, resolver.Workspace, resolver.Resolution) (resolver.Revalidation, error) {
	return f.revalidation, nil
}
func (f *fakeResolver) Acquire(context.Context, resolver.Workspace, resolver.Resolution) (resolver.Artifact, error) {
	f.acquireCalls++
	return nil, errors.New("unexpected")
}

type fakeCache struct {
	load       resolver.CacheLoad
	storeCalls int
}

func (f *fakeCache) Load(context.Context, resolver.CacheKey, runtimev2.ExtensionIdentitySchema) (resolver.CacheLoad, error) {
	return f.load, nil
}
func (f *fakeCache) Store(context.Context, resolver.CacheKey, runtimev2.ExtensionIdentitySchema, resolver.Resolution) error {
	f.storeCalls++
	return nil
}

func descriptor(kind string) resolver.Descriptor {
	return resolver.Descriptor{Role: "input", Owner: "sqlrs.workspace", Kind: kind, SpecificationSchema: "sqlrs.workspace-file.declaration.v1", SemanticVersion: "1"}
}
func workspaceDeclaration(t *testing.T) runtimev2.InputDeclaration {
	t.Helper()
	return workspaceDeclarationNoTest()
}
func workspaceDeclarationNoTest() runtimev2.InputDeclaration {
	value, _ := runtimev2.NewInputDeclaration(runtimev2.ExtensionSpecificationInput{SchemaVersion: runtimev2.SchemaVersion, Owner: "sqlrs.workspace", Kind: "one", SpecificationSchema: "sqlrs.workspace-file.declaration.v1", Fields: []runtimev2.DeclarationField{{Name: "path", Value: "x"}}})
	return value
}
func resolution(t *testing.T, declaration runtimev2.ExtensionDeclaration) resolver.Resolution {
	t.Helper()
	schema, _ := schemaauthor.NewExtensionSchema(schemaauthor.SchemaInput{Provider: declaration.Owner(), SemanticKind: declaration.Kind(), IdentitySchema: "file.v1", Fields: []schemaauthor.FieldDefinition{{Name: "content.digest", Role: schemaauthor.FieldRoleSemantic, Kind: runtimev2.IdentityFieldText, Required: true, Disclosure: schemaauthor.DisclosurePublic}}})
	builder, _ := runtimev2.NewExtensionIdentityBuilder(schema)
	field, _ := runtimev2.NewTextIdentityField("content.digest", "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	_ = builder.AddIdentityField(field)
	composition, _ := builder.Build()
	identity := composition.Identity()
	return resolver.Resolution{Identity: identity, Evidence: json.RawMessage(`{"schema_version":"test"}`)}
}
