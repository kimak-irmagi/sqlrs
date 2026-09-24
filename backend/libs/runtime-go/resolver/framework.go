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

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
)

var (
	ErrUnsupported        = errors.New("resolver: unsupported declaration")
	ErrDuplicate          = errors.New("resolver: duplicate descriptor")
	ErrInvalidDeclaration = errors.New("resolver: invalid declaration")
	ErrUnsafePath         = errors.New("resolver: unsafe path")
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
type Outcome struct {
	Resolution  Resolution
	PriorStatus RevalidationStatus
	Reason      string
	CacheHit    bool
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
type Registry struct{ resolvers map[dispatchKey]Resolver }

func descriptorKey(value Descriptor) dispatchKey {
	return dispatchKey{value.Role, value.Owner, value.Kind, value.SpecificationSchema}
}

func NewRegistry(values ...Resolver) (Registry, error) {
	result := Registry{resolvers: make(map[dispatchKey]Resolver, len(values))}
	for _, value := range values {
		if value == nil {
			return Registry{}, ErrUnsupported
		}
		key := descriptorKey(value.Descriptor())
		if _, ok := result.resolvers[key]; ok {
			return Registry{}, ErrDuplicate
		}
		result.resolvers[key] = value
	}
	return result, nil
}
func (r Registry) Lookup(key Descriptor) (Resolver, error) {
	value, ok := r.resolvers[descriptorKey(key)]
	if !ok {
		return nil, ErrUnsupported
	}
	return value, nil
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
func (m Manager) ResolveCurrent(ctx context.Context, workspace Workspace, declaration runtimev2.InputDeclaration) (Outcome, error) {
	keyDescriptor := Descriptor{Role: declaration.Role(), Owner: declaration.Owner(), Kind: declaration.Kind(), SpecificationSchema: declaration.SpecificationSchema()}
	var provider Resolver
	provider = m.registry.resolvers[descriptorKey(keyDescriptor)]
	if provider != nil {
		keyDescriptor = provider.Descriptor()
	}
	if provider == nil {
		return Outcome{}, ErrUnsupported
	}
	normalized, err := provider.Normalize(ctx, workspace, declaration)
	if err != nil {
		return Outcome{}, resolverError("normalize", classifyError(err), keyDescriptor, err)
	}
	key, err := NewCacheKey(workspace, keyDescriptor, normalized)
	if err != nil {
		return Outcome{}, err
	}
	loaded, err := m.cache.Load(ctx, key)
	if err != nil {
		if !errors.Is(err, ErrCorruptCache) && !errors.Is(err, ErrIncompatibleCache) {
			return Outcome{}, resolverError("cache_load", classifyError(err), keyDescriptor, err)
		}
		loaded = CacheLoad{}
	}
	if loaded.Hit {
		if err := provider.ValidateResolution(loaded.Resolution); err != nil {
			loaded = CacheLoad{}
		} else {
			revalidation, err := provider.Revalidate(ctx, workspace, loaded.Resolution)
			if err != nil {
				return Outcome{}, resolverError("revalidate", classifyError(err), keyDescriptor, err)
			}
			if revalidation.Status == Current {
				loaded.Resolution.Evidence = append(json.RawMessage(nil), revalidation.Evidence...)
				if err := m.cache.Store(ctx, key, loaded.Resolution); err != nil {
					return Outcome{}, resolverError("cache_store", classifyError(err), keyDescriptor, err)
				}
				return Outcome{loaded.Resolution, Current, revalidation.Reason, true}, nil
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
			return Outcome{fresh, revalidation.Status, revalidation.Reason, true}, nil
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
	return Outcome{Resolution: fresh}, nil
}
