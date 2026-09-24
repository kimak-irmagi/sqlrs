package resolver_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
	"github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/resolver"
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

type fakeResolver struct {
	descriptor                 resolver.Descriptor
	resolution                 resolver.Resolution
	revalidation               resolver.Revalidation
	resolveErr                 error
	resolveCalls, acquireCalls int
}

func (f *fakeResolver) Descriptor() resolver.Descriptor { return f.descriptor }
func (f *fakeResolver) Normalize(context.Context, resolver.Workspace, runtimev2.InputDeclaration) (resolver.NormalizedDeclaration, error) {
	return resolver.NormalizedDeclaration{Declaration: workspaceDeclarationNoTest()}, nil
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

func (f *fakeCache) Load(context.Context, resolver.CacheKey) (resolver.CacheLoad, error) {
	return f.load, nil
}
func (f *fakeCache) Store(context.Context, resolver.CacheKey, resolver.Resolution) error {
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
func resolution(t *testing.T, declaration runtimev2.InputDeclaration) resolver.Resolution {
	t.Helper()
	identity, _ := runtimev2.NewResolvedExtensionIdentity(runtimev2.ResolvedExtensionIdentityInput{SchemaVersion: runtimev2.SchemaVersion, Owner: declaration.Owner(), Kind: declaration.Kind(), IdentitySchema: "file.v1", Fields: []runtimev2.ResolvedField{{Name: "content.digest", Value: "sha256:" + string(make([]byte, 0)) + "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}}})
	return resolver.Resolution{Identity: identity, Evidence: json.RawMessage(`{"schema_version":"test"}`)}
}
