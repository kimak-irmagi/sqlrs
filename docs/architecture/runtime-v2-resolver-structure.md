# Runtime v2 resolver: module and component structure

Status: approved by @evilguest for issue #108, 2026-09-24.

## Module boundary

Add package `resolver` to the existing independent module:

```text
backend/libs/runtime-go/resolver/
  framework.go           contracts, immutable registry, manager, cache key
  directory_cache.go     restart-safe atomic directory implementation
  cache_prune.go         bounded namespace/age/count pruning
  artifact_store.go      trusted content-addressed acquired artifacts
  private_directory.go   trusted-root construction
  link_unix.go           Unix link detection
  link_windows.go        Windows symlink/reparse-point detection
  replace_unix.go        atomic cache publication on Unix
  replace_windows.go     atomic cache publication on Windows
  errors.go              common operation/error model
  workspace_file.go      reference file resolver
  filesystem_class.go   cheap-revalidation capability classification
  filesystem_boundary.go cross-filesystem transition detection
  filemeta_windows.go    Windows continuity evidence
  filemeta_fallback.go   safe UNKNOWN behavior elsewhere
```

The package consumes the closed versioned `runtimev2.ExtensionDeclaration`
interface, implemented only by input, execution-environment, and deployment
declarations, and returns the shared `runtimev2.ResolvedExtensionIdentity`
introduced by issue #124. An engine
adapter explicitly composes that resolved extension into its factory or
transform identity. The resolver otherwise uses the Go standard library,
including traversal-resistant Go 1.25 `os.Root`, and imports no engine, Docker,
Liquibase, DBMS, SQLite, or StateFS code.

## Public contracts

```go
type Resolver interface {
    Descriptor() Descriptor
    Normalize(context.Context, Workspace, runtimev2.ExtensionDeclaration) (NormalizedDeclaration, error)
    Resolve(context.Context, Workspace, NormalizedDeclaration) (Resolution, error)
    ValidateResolution(Resolution) error
    Revalidate(context.Context, Workspace, Resolution) (Revalidation, error)
    Acquire(context.Context, Workspace, Resolution) (Artifact, error)
}

type Cache interface {
    Load(context.Context, CacheKey) (CacheLoad, error)
    Store(context.Context, CacheKey, Resolution) error
}

type PrunableCache interface {
    Prune(context.Context, PrunePolicy) (PruneResult, error)
}

type ArtifactStore interface {
    PublishVerified(context.Context, io.Reader, ContentDigest) (Artifact, error)
}

func NewRegistry(resolvers ...Resolver) (Registry, error)
func NewManager(registry Registry, cache Cache) (Manager, error)
func NewWorkspaceFileResolver(artifacts ArtifactStore) (Resolver, error)
func (m Manager) ResolveCurrent(ctx context.Context, workspace Workspace, declaration runtimev2.ExtensionDeclaration) (Outcome, error)
```

Exact Go spelling may be adjusted while implementing approved tests, but these
boundaries are normative:

- registry dispatch is by validated role/owner/kind/specification-schema tuple
  and rejects duplicate tuples;
- normalization, resolution, revalidation, and acquisition remain independently
  callable;
- the manager calls `ValidateResolution` after cache load and after `Resolve`,
  before provider evidence can be used or persisted;
- `Resolution` separates immutable identity from declaration, provenance,
  freshness, and provider evidence;
- `Artifact` contains physical handle/location data outside identity;
- file acquisition returns only a digest-verified immutable artifact published
  by a trusted `ArtifactStore`, never the mutable workspace handle;
- `Outcome` exposes cache outcome and exact revalidation status;
- constructors and strict JSON decoders copy mutable inputs, reject unknown or
  duplicate members, and leave receivers unchanged after failure.

## Identity compatibility

`runtimev2.ResolvedExtensionIdentity` contains `Owner`, `Kind`,
`IdentitySchema`, and canonical `[]runtimev2.ResolvedField`. It is not itself a
factory or transform. The consuming adapter deliberately maps or composes its
fields into a Runtime v2 constructor input; no one-resource-to-one-transform
conversion helper is provided. This prevents a file from being mistaken for a
complete execution transform.

`runtimev2.ExtensionFingerprint` uses a new domain-separated canonical record
over extension schema, owner, kind, identity schema, and every resolved field.
`runtimev2.ComposeResolvedFields` accepts uniquely named `ExtensionBinding`
values and returns ordinary identity-bearing `ResolvedField` values whose names
are reserved under `extension.*`. It rejects duplicate bindings and cannot omit
any extension supplied to it. Adapters must use this helper, and conformance
tests enumerate every declaration extension and prove that changing it changes
the containing factory/transform fingerprint.

The workspace-file identity is:

```json
{
  "schema_version": "sqlrs.runtime.v2",
  "owner": "sqlrs.workspace",
  "kind": "file",
  "identity_schema": "sqlrs.workspace-file.v1",
  "fields": [
    {"name": "content.digest", "value": "sha256:..."}
  ]
}
```

Normalized/absolute paths, timestamps, file ID, cache key, and acquisition
location are excluded from identity. Provenance records original and normalized
declarations and resolver implementation/semantic version. Wall-clock resolution
time is intentionally not part of the current persistent envelope.

## Revalidation and artifact types

`RevalidationStatus` is a closed enum: `CURRENT`, `STALE`, or `UNKNOWN`.
`Revalidation` also carries a stable reason code and refreshed freshness. It
never carries a replacement identity; only `Resolve` creates one.

File freshness evidence is a private versioned strict-JSON value containing
path, size, mtime, stable file identity, change token, filesystem class, evidence
revision, and whether continuity is strong. Weak fields are observations and
never become identity fields. The generic cache bounds and validates the JSON
envelope; `ValidateResolution` applies the provider-specific closed schema and
rejects missing or unknown evidence fields before revalidation.

Enabled cheap paths are revisioned per filesystem: `ntfs-usn-v1` on Windows,
native device/inode/ctime revisions for ext4, XFS, and Btrfs on Linux, and APFS
on macOS. Strong evidence is sampled both before and after the stable double
content read; both samples and final stat metadata must match. Mandatory native
tests overwrite bytes while preserving size and restoring mtime; such a change
must not return `CURRENT`. Overlay, network, virtual, unknown, and unrecognized
filesystems remain conservative `UNKNOWN`.

`Artifact` is a small interface with artifact kind and `Close`. Concrete values
stay provider-owned. The file implementation returns an opened immutable
content-addressed artifact plus diagnostic store location; the caller closes it.
The store streams and hashes into a temporary file, verifies the expected digest,
syncs, atomically publishes, and revalidates an existing object before reuse.
The returned artifact handle refers to the same opened object generation whose
bytes were validated; validation followed by an unchecked path reopen is forbidden.
Future OCI, Git, and package resolvers can return their own artifact handles.

## Error model

Operations return `*resolver.Error` with operation, stable code, and resolver
descriptor. Wrapped system errors remain available through `Unwrap`; messages never
expose file contents. Codes include `invalid_declaration`, `unsupported_kind`,
`duplicate_kind`, `not_found`, `unsafe_path`, `not_regular`,
`changed_during_resolution`, `invalid_resolution`, `incompatible_cache`,
`corrupt_cache`, `permission_denied`, `unavailable`, and `io`. No operation
returns a partial resolution or artifact. Context errors remain matchable.
When fallback resolution fails after revalidation, `Error` retains the prior
`RevalidationStatus` and stable reason without exposing a partial new resolution.

## Data ownership and concurrency

`Registry`, `Manager`, `Resolution`, `ResolvedExtensionIdentity`, and normalized
declarations are immutable and safe for concurrent reads. There is no global
registry. A cache owns only its configured directory. Writes serialize per key
in-process; atomic replacement makes readers see an old or new complete record.
Cross-process last-writer-wins is safe because records self-validate against the
same normalized key and semantic version. Same-directory temporary files and
platform-specific replacement publish complete generations; readers ignore
temporary files and treat a truncated record as an invalidation, never a hit.
Named package-private publication barriers let subprocess conformance tests kill
writers after write, file sync, replacement, and directory sync and prove this
behavior rather than merely exercising successful concurrent completion.

Workspace files, cache records, and content-addressed acquired artifacts are
persistent. Registry, locks, manager, and opened handles are in memory. Trusted
cache/artifact directories are outside the mutable workspace and reject links.
On Unix, newly created roots and files use owner-only modes. On Windows,
constructors require an existing root, verify current-user ownership, reject
reparse roots and broad write-capable DACL entries, and let child objects inherit
that validated ACL. Deployments that cannot establish this trusted root must
fail before constructing the store rather than downgrade silently. Records remain
checksummed, but directory ownership is the authentication boundary.
The package never persists logical states or materializations; that belongs to
later Runtime v2 persistence work.

`DirectoryCache` additionally implements `PrunableCache`. Policies bound inert
schema/semantic-version namespaces by age and entry count. Pruning uses the same
rooted no-link discipline, skips active temporary files, and is never performed
implicitly on a read path.

## Compatibility boundary

The package is dormant library code. It does not alter legacy IDs/cache records,
commands, HTTP handlers, or execution. Each record has cache-schema and resolver
semantic versions; unknown versions fail closed and cannot reinterpret old data.

## Module release versioning

Go versions the nested module, not individual packages. Tag
`backend/libs/runtime-go/v0.1.0` exists at commit `52bb255`, but was published
before the complete declaration boundary from issue #124. It is immutable and
will be retracted in the next `go.mod`; external consumers must not select it for
new dependencies.

The combined #108/#124 release first publishes
`backend/libs/runtime-go/v0.2.0-rc.1`. After the exact RC commit passes the
clean-consumer and public-proxy gates, the immutable GA tag targets:

```text
backend/libs/runtime-go/v0.2.0
```

After publication, consumers pin it with:

```text
go get github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go@v0.2.0
```

Package `resolver` receives no independent tag. Module release versions,
resolver semantic versions, cache schema versions, and identity schema versions
are distinct and do not implicitly reinterpret one another.
