// Package resolver resolves mutable declarations into Runtime v2 identities.
// Requirements: docs/architecture/runtime-v2-resolver-structure.md.
package resolver

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/binary"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"unicode/utf8"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
)

var (
	ErrUnsupported        = errors.New("resolver: unsupported declaration")
	ErrDuplicate          = errors.New("resolver: duplicate descriptor")
	ErrInvalidDeclaration = errors.New("resolver: invalid declaration")
	ErrUnsafePath         = errors.New("resolver: unsafe path")
	ErrNotRegular         = errors.New("resolver: not a regular file")
	ErrChanged            = errors.New("resolver: source changed")
	ErrCorruptCache       = errors.New("resolver: corrupt cache")
	ErrIncompatibleCache  = errors.New("resolver: incompatible cache")
)

type Descriptor struct {
	Role                string `json:"role"`
	Owner               string `json:"owner"`
	Kind                string `json:"kind"`
	SpecificationSchema string `json:"specification_schema"`
	SemanticVersion     string `json:"semantic_version"`
}
type Workspace struct{ Root string }
type NormalizedDeclaration struct{ Declaration runtimev2.InputDeclaration }
type Resolution struct {
	Identity runtimev2.ResolvedExtensionIdentity
	Evidence json.RawMessage
}
type RevalidationStatus string

const (
	Current RevalidationStatus = "CURRENT"
	Stale   RevalidationStatus = "STALE"
	Unknown RevalidationStatus = "UNKNOWN"
)

type Revalidation struct {
	Status   RevalidationStatus
	Reason   string
	Evidence json.RawMessage
}

// Provenance identifies the resolver and normalized declaration that produced
// an outcome without placing mutable declaration spelling in logical identity.
type Provenance struct {
	Resolver              Descriptor
	NormalizedDeclaration json.RawMessage
}

// Freshness exposes the exact revalidation result and provider evidence used by
// the outcome. Evidence remains provider-owned and non-identity-bearing.
type Freshness struct {
	Status   RevalidationStatus
	Reason   string
	Evidence json.RawMessage
}
type Outcome struct {
	Resolution  Resolution
	PriorStatus RevalidationStatus
	Reason      string
	CacheHit    bool
	Provenance  Provenance
	Freshness   Freshness
}
type CacheKey struct {
	digest, workspaceScope string
	descriptor             Descriptor
	declaration            json.RawMessage
}

func (k CacheKey) String() string { return k.digest }

type CacheLoad struct {
	Hit        bool
	Resolution Resolution
}
type Artifact interface {
	io.ReadCloser
	Kind() string
}

type Resolver interface {
	Descriptor() Descriptor
	Normalize(context.Context, Workspace, runtimev2.InputDeclaration) (NormalizedDeclaration, error)
	Resolve(context.Context, Workspace, NormalizedDeclaration) (Resolution, error)
	ValidateResolution(Resolution) error
	Revalidate(context.Context, Workspace, Resolution) (Revalidation, error)
	Acquire(context.Context, Workspace, Resolution) (Artifact, error)
}

// NewCacheKey derives a path-free key from the physical workspace scope,
// resolver descriptor, and normalized declaration.
func NewCacheKey(workspace Workspace, descriptor Descriptor, normalized NormalizedDeclaration) (CacheKey, error) {
	root, err := filepath.EvalSymlinks(workspace.Root)
	if err != nil {
		return CacheKey{}, err
	}
	root, err = filepath.Abs(root)
	if err != nil {
		return CacheKey{}, err
	}
	declaration, err := json.Marshal(normalized.Declaration)
	if err != nil {
		return CacheKey{}, err
	}
	scope := sha256.Sum256([]byte(filepath.Clean(root)))
	parts := [][]byte{[]byte("sqlrs.resolution-cache.v1"), scope[:], []byte(descriptor.Role), []byte(descriptor.Owner), []byte(descriptor.Kind), []byte(descriptor.SpecificationSchema), []byte(descriptor.SemanticVersion), declaration}
	var preimage bytes.Buffer
	for _, part := range parts {
		_ = binary.Write(&preimage, binary.BigEndian, uint64(len(part)))
		_, _ = preimage.Write(part)
	}
	digest := sha256.Sum256(preimage.Bytes())
	return CacheKey{digest: hex.EncodeToString(digest[:]), workspaceScope: hex.EncodeToString(scope[:]), descriptor: descriptor, declaration: append(json.RawMessage(nil), declaration...)}, nil
}

type Cache interface {
	Load(context.Context, CacheKey) (CacheLoad, error)
	Store(context.Context, CacheKey, Resolution) error
}

type dispatchKey struct{ Role, Owner, Kind, SpecificationSchema string }
type registryEntry struct {
	descriptor Descriptor
	resolver   Resolver
}
type Registry struct{ resolvers map[dispatchKey]registryEntry }

func descriptorKey(value Descriptor) dispatchKey {
	return dispatchKey{value.Role, value.Owner, value.Kind, value.SpecificationSchema}
}

func NewRegistry(values ...Resolver) (Registry, error) {
	result := Registry{resolvers: make(map[dispatchKey]registryEntry, len(values))}
	for _, value := range values {
		if value == nil {
			return Registry{}, ErrUnsupported
		}
		descriptor := value.Descriptor()
		if !validDescriptor(descriptor) {
			return Registry{}, ErrInvalidDeclaration
		}
		key := descriptorKey(descriptor)
		if _, ok := result.resolvers[key]; ok {
			return Registry{}, ErrDuplicate
		}
		result.resolvers[key] = registryEntry{descriptor: descriptor, resolver: value}
	}
	return result, nil
}

func validDescriptor(value Descriptor) bool {
	for _, component := range []string{value.Role, value.Owner, value.Kind, value.SpecificationSchema, value.SemanticVersion} {
		if component == "" || !utf8.ValidString(component) || component != strings.TrimSpace(component) {
			return false
		}
	}
	return true
}
func (r Registry) Lookup(key Descriptor) (Resolver, error) {
	value, ok := r.resolvers[descriptorKey(key)]
	if !ok {
		return nil, ErrUnsupported
	}
	return value.resolver, nil
}

type Manager struct {
	registry Registry
	cache    Cache
}

func NewManager(registry Registry, cache Cache) (Manager, error) {
	if cache == nil {
		return Manager{}, errors.New("resolver: nil cache")
	}
	return Manager{registry, cache}, nil
}

func outcomeFor(resolution Resolution, key CacheKey, status RevalidationStatus, reason string, cacheHit bool) Outcome {
	return Outcome{
		Resolution: resolution, PriorStatus: status, Reason: reason, CacheHit: cacheHit,
		Provenance: Provenance{Resolver: key.descriptor, NormalizedDeclaration: append(json.RawMessage(nil), key.declaration...)},
		Freshness:  Freshness{Status: status, Reason: reason, Evidence: append(json.RawMessage(nil), resolution.Evidence...)},
	}
}
func (m Manager) ResolveCurrent(ctx context.Context, workspace Workspace, declaration runtimev2.InputDeclaration) (Outcome, error) {
	keyDescriptor := Descriptor{Role: declaration.Role(), Owner: declaration.Owner(), Kind: declaration.Kind(), SpecificationSchema: declaration.SpecificationSchema()}
	entry, found := m.registry.resolvers[descriptorKey(keyDescriptor)]
	var provider Resolver
	if found {
		provider = entry.resolver
		keyDescriptor = entry.descriptor
	}
	if provider == nil {
		return Outcome{}, resolverError("dispatch", CodeUnsupportedKind, keyDescriptor, ErrUnsupported)
	}
	normalized, err := provider.Normalize(ctx, workspace, declaration)
	if err != nil {
		return Outcome{}, resolverError("normalize", classifyError(err), keyDescriptor, err)
	}
	key, err := NewCacheKey(workspace, keyDescriptor, normalized)
	if err != nil {
		return Outcome{}, resolverError("cache_key", classifyError(err), keyDescriptor, err)
	}
	fallbackReason := "cache_miss"
	loaded, err := m.cache.Load(ctx, key)
	if err != nil {
		if !errors.Is(err, ErrCorruptCache) && !errors.Is(err, ErrIncompatibleCache) {
			return Outcome{}, resolverError("cache_load", classifyError(err), keyDescriptor, err)
		}
		if errors.Is(err, ErrIncompatibleCache) {
			fallbackReason = "incompatible_cache"
		} else {
			fallbackReason = "corrupt_cache"
		}
		loaded = CacheLoad{}
	}
	if loaded.Hit {
		if err := provider.ValidateResolution(loaded.Resolution); err != nil {
			fallbackReason = "invalid_cached_resolution"
			loaded = CacheLoad{}
		} else {
			revalidation, err := provider.Revalidate(ctx, workspace, loaded.Resolution)
			if err != nil {
				return Outcome{}, resolverError("revalidate", classifyError(err), keyDescriptor, err)
			}
			if revalidation.Status != Current && revalidation.Status != Stale && revalidation.Status != Unknown {
				return Outcome{}, resolverError("revalidate", CodeInvalidResolution, keyDescriptor, ErrInvalidDeclaration)
			}
			if revalidation.Status == Current {
				loaded.Resolution.Evidence = append(json.RawMessage(nil), revalidation.Evidence...)
				if err := provider.ValidateResolution(loaded.Resolution); err != nil {
					return Outcome{}, resolverError("validate_resolution", CodeInvalidResolution, keyDescriptor, err)
				}
				if err := m.cache.Store(ctx, key, loaded.Resolution); err != nil {
					return Outcome{}, resolverError("cache_store", classifyError(err), keyDescriptor, err)
				}
				return outcomeFor(loaded.Resolution, key, Current, revalidation.Reason, true), nil
			}
			fresh, err := provider.Resolve(ctx, workspace, normalized)
			if err != nil {
				wrapped := resolverError("resolve", classifyError(err), keyDescriptor, err)
				wrapped.PriorStatus = revalidation.Status
				wrapped.Reason = revalidation.Reason
				return Outcome{}, wrapped
			}
			if err := provider.ValidateResolution(fresh); err != nil {
				return Outcome{}, resolverError("validate_resolution", CodeInvalidResolution, keyDescriptor, err)
			}
			if err := m.cache.Store(ctx, key, fresh); err != nil {
				return Outcome{}, resolverError("cache_store", classifyError(err), keyDescriptor, err)
			}
			return outcomeFor(fresh, key, revalidation.Status, revalidation.Reason, true), nil
		}
	}
	fresh, err := provider.Resolve(ctx, workspace, normalized)
	if err != nil {
		return Outcome{}, resolverError("resolve", classifyError(err), keyDescriptor, err)
	}
	if err := provider.ValidateResolution(fresh); err != nil {
		return Outcome{}, resolverError("validate_resolution", CodeInvalidResolution, keyDescriptor, err)
	}
	if err := m.cache.Store(ctx, key, fresh); err != nil {
		return Outcome{}, resolverError("cache_store", classifyError(err), keyDescriptor, err)
	}
	return outcomeFor(fresh, key, "", fallbackReason, false), nil
}
