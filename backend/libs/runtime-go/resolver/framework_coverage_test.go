package resolver

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
)

type coverageResolver struct {
	descriptor                 Descriptor
	normalized                 NormalizedDeclaration
	resolution                 Resolution
	revalidation               Revalidation
	normalizeErr, resolveErr   error
	validateErr, revalidateErr error
	validateResults            []error
	validateCalls              int
}

func (r *coverageResolver) Descriptor() Descriptor { return r.descriptor }
func (r *coverageResolver) Normalize(context.Context, Workspace, runtimev2.InputDeclaration) (NormalizedDeclaration, error) {
	return r.normalized, r.normalizeErr
}
func (r *coverageResolver) Resolve(context.Context, Workspace, NormalizedDeclaration) (Resolution, error) {
	return r.resolution, r.resolveErr
}
func (r *coverageResolver) ValidateResolution(Resolution) error {
	if r.validateCalls < len(r.validateResults) {
		result := r.validateResults[r.validateCalls]
		r.validateCalls++
		return result
	}
	return r.validateErr
}
func (r *coverageResolver) Revalidate(context.Context, Workspace, Resolution) (Revalidation, error) {
	return r.revalidation, r.revalidateErr
}
func (*coverageResolver) Acquire(context.Context, Workspace, Resolution) (Artifact, error) {
	return nil, errors.New("unused")
}

type coverageCache struct {
	load     CacheLoad
	loadErr  error
	storeErr error
}

func (c *coverageCache) Load(context.Context, CacheKey) (CacheLoad, error) { return c.load, c.loadErr }
func (c *coverageCache) Store(context.Context, CacheKey, Resolution) error { return c.storeErr }

func coverageDeclaration(t *testing.T) runtimev2.InputDeclaration {
	t.Helper()
	value, err := runtimev2.NewInputDeclaration(runtimev2.ExtensionSpecificationInput{
		SchemaVersion: runtimev2.SchemaVersion, Owner: "owner", Kind: "kind",
		SpecificationSchema: "owner.kind.v1", Fields: []runtimev2.DeclarationField{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func coverageResolution(t *testing.T) Resolution {
	t.Helper()
	identity, err := runtimev2.NewResolvedExtensionIdentity(runtimev2.ResolvedExtensionIdentityInput{
		SchemaVersion: runtimev2.SchemaVersion, Owner: "owner", Kind: "kind",
		IdentitySchema: "owner.kind.v1", Fields: []runtimev2.ResolvedField{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return Resolution{Identity: identity, Evidence: []byte(`{}`)}
}

func TestManagerFailureBranches(t *testing.T) {
	declaration := coverageDeclaration(t)
	resolution := coverageResolution(t)
	descriptor := Descriptor{Role: "input", Owner: "owner", Kind: "kind", SpecificationSchema: "owner.kind.v1", SemanticVersion: "1"}
	workspace := Workspace{Root: t.TempDir()}
	normalized := NormalizedDeclaration{Declaration: declaration}

	if _, err := NewManager(Registry{}, nil); err == nil {
		t.Fatal("nil cache accepted")
	}
	if _, err := NewRegistry(nil); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("nil resolver: %v", err)
	}
	if _, err := NewRegistry(&coverageResolver{}); !errors.Is(err, ErrInvalidDeclaration) {
		t.Fatalf("invalid descriptor: %v", err)
	}
	registry, _ := NewRegistry()
	manager, _ := NewManager(registry, &coverageCache{})
	if _, err := manager.ResolveCurrent(context.Background(), workspace, declaration); !errors.Is(err, ErrUnsupported) {
		t.Fatalf("unsupported: %v", err)
	}

	tests := []struct {
		name      string
		provider  *coverageResolver
		cache     *coverageCache
		operation string
	}{
		{"normalize", &coverageResolver{descriptor: descriptor, normalizeErr: ErrInvalidDeclaration}, &coverageCache{}, "normalize"},
		{"cache load", &coverageResolver{descriptor: descriptor, normalized: normalized}, &coverageCache{loadErr: os.ErrPermission}, "cache_load"},
		{"revalidate", &coverageResolver{descriptor: descriptor, normalized: normalized, revalidateErr: os.ErrPermission}, &coverageCache{load: CacheLoad{Hit: true, Resolution: resolution}}, "revalidate"},
		{"current store", &coverageResolver{descriptor: descriptor, normalized: normalized, revalidation: Revalidation{Status: Current}}, &coverageCache{load: CacheLoad{Hit: true, Resolution: resolution}, storeErr: os.ErrPermission}, "cache_store"},
		{"fallback validate", &coverageResolver{descriptor: descriptor, normalized: normalized, resolution: resolution, revalidation: Revalidation{Status: Stale}, validateResults: []error{nil, ErrInvalidDeclaration}}, &coverageCache{load: CacheLoad{Hit: true, Resolution: resolution}}, "validate_resolution"},
		{"fallback store", &coverageResolver{descriptor: descriptor, normalized: normalized, resolution: resolution, revalidation: Revalidation{Status: Unknown}}, &coverageCache{load: CacheLoad{Hit: true, Resolution: resolution}, storeErr: os.ErrPermission}, "cache_store"},
		{"miss resolve", &coverageResolver{descriptor: descriptor, normalized: normalized, resolveErr: os.ErrNotExist}, &coverageCache{}, "resolve"},
		{"miss validate", &coverageResolver{descriptor: descriptor, normalized: normalized, resolution: resolution, validateErr: ErrInvalidDeclaration}, &coverageCache{}, "validate_resolution"},
		{"miss store", &coverageResolver{descriptor: descriptor, normalized: normalized, resolution: resolution}, &coverageCache{storeErr: os.ErrPermission}, "cache_store"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			registry, _ := NewRegistry(test.provider)
			manager, _ := NewManager(registry, test.cache)
			_, err := manager.ResolveCurrent(context.Background(), workspace, declaration)
			var structured *Error
			if !errors.As(err, &structured) || structured.Operation != test.operation {
				t.Fatalf("error = %#v", err)
			}
		})
	}

	for _, cacheErr := range []error{ErrCorruptCache, ErrIncompatibleCache} {
		provider := &coverageResolver{descriptor: descriptor, normalized: normalized, resolution: resolution}
		registry, _ := NewRegistry(provider)
		manager, _ := NewManager(registry, &coverageCache{loadErr: cacheErr})
		if _, err := manager.ResolveCurrent(context.Background(), workspace, declaration); err != nil {
			t.Fatalf("recover from %v: %v", cacheErr, err)
		}
	}
	provider := &coverageResolver{descriptor: descriptor, normalized: normalized, resolution: resolution, validateResults: []error{ErrInvalidDeclaration, nil}}
	registry, _ = NewRegistry(provider)
	manager, _ = NewManager(registry, &coverageCache{load: CacheLoad{Hit: true, Resolution: resolution}})
	if _, err := manager.ResolveCurrent(context.Background(), workspace, declaration); err != nil {
		t.Fatalf("invalid cached value fallback: %v", err)
	}

	provider = &coverageResolver{descriptor: descriptor, normalized: NormalizedDeclaration{}}
	registry, _ = NewRegistry(provider)
	manager, _ = NewManager(registry, &coverageCache{})
	if _, err := manager.ResolveCurrent(context.Background(), workspace, declaration); err == nil {
		t.Fatal("invalid normalized value accepted")
	}
	provider = &coverageResolver{descriptor: descriptor, normalized: normalized, revalidation: Revalidation{Status: "BROKEN"}}
	registry, _ = NewRegistry(provider)
	manager, _ = NewManager(registry, &coverageCache{load: CacheLoad{Hit: true, Resolution: resolution}})
	if _, err := manager.ResolveCurrent(context.Background(), workspace, declaration); err == nil {
		t.Fatal("unknown revalidation status accepted")
	}
	provider = &coverageResolver{descriptor: descriptor, normalized: normalized, revalidation: Revalidation{Status: Current, Evidence: []byte(`{"bad":true}`)}, validateResults: []error{nil, ErrInvalidDeclaration}}
	registry, _ = NewRegistry(provider)
	manager, _ = NewManager(registry, &coverageCache{load: CacheLoad{Hit: true, Resolution: resolution}})
	if _, err := manager.ResolveCurrent(context.Background(), workspace, declaration); err == nil {
		t.Fatal("invalid refreshed evidence accepted")
	}
}

func TestErrorAndClassificationBranches(t *testing.T) {
	var nilError *Error
	if nilError.Error() != "resolver error" || nilError.Unwrap() != nil {
		t.Fatal("nil error behavior")
	}
	cause := errors.New("cause")
	value := resolverError("op", CodeIO, Descriptor{}, cause)
	if value.Error() != "resolver op: io" || !errors.Is(value, cause) {
		t.Fatalf("envelope: %v", value)
	}
	for input, want := range map[error]ErrorCode{
		ErrInvalidDeclaration: CodeInvalidDeclaration,
		ErrUnsupported:        CodeUnsupportedKind,
		ErrDuplicate:          CodeDuplicateKind,
		ErrCorruptCache:       CodeCorruptCache,
		ErrIncompatibleCache:  CodeIncompatibleCache,
		ErrUnsafePath:         CodeUnsafePath,
		ErrNotRegular:         CodeNotRegular,
		ErrChanged:            CodeChanged,
		os.ErrNotExist:        CodeNotFound,
		os.ErrPermission:      CodePermissionDenied,
		context.Canceled:      CodeCancelled,
		cause:                 CodeIO,
	} {
		if got := classifyError(input); got != want {
			t.Errorf("classify %v = %s", input, got)
		}
	}
}

func TestCacheKeyFailureBranches(t *testing.T) {
	if _, err := NewCacheKey(Workspace{Root: filepath.Join(t.TempDir(), "missing")}, Descriptor{}, NormalizedDeclaration{Declaration: coverageDeclaration(t)}); err == nil {
		t.Fatal("missing workspace accepted")
	}
	if _, err := NewCacheKey(Workspace{Root: t.TempDir()}, Descriptor{}, NormalizedDeclaration{}); err == nil {
		t.Fatal("invalid normalized declaration accepted")
	}
}
