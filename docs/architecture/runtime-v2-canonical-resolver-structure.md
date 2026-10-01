# Runtime v2 canonical resolver: component structure

Status: component structure approved by @evilguest, 2026-10-01.
Interaction flow: [canonical resolver flow](runtime-v2-canonical-resolver-flow.md).

## Module and ownership

The public nested module remains
`github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go`. Version v0.5.0
changes the current `resolver` API in place. There is no second canonical
registry, manager, cache, or result package. The module imports no engine,
SQLite, Docker, Git, OCI client, or DBMS dependency. The current resolver
source under `resolver/` is reused; the canonical codec belongs to the
semantic core, while persistence orchestration stays in `resolver/`.

| Component | Responsibility |
| --- | --- |
| `runtimev2` canonical identity codec | Strictly encode/decode one `CanonicalResolvedExtensionIdentity` using a supplied `ExtensionIdentitySchema`; copy values; preserve field kinds and disclosure. |
| `resolver/framework.go` | One `Resolution`, `Resolver`, registry, manager, descriptor, key, and observable outcome. |
| `resolver/cache_record.go` | Closed canonical cache-record envelope, checksum, key verification, strict bounded JSON. |
| `resolver/directory_cache.go` | Owner-controlled directory, atomic publication, restart-safe loading. |
| `resolver/workspace_file.go` | Migrate the existing reference provider to the canonical result; preserve safe path, revalidation, and acquisition behavior. |
| local-engine SQLite adapter | Use the new record codec in the existing resolution table; no new schema or default-runtime path. |
| clean consumer tests | Exercise only exported Go packages from a staged and then published module. |

## Public contracts

The existing names are preferred to avoid an unnecessary parallel surface.
Exact spelling may be refined while writing approved tests, but these
relationships are normative:

```go
type Resolution struct {
    Identity runtimev2.CanonicalResolvedExtensionIdentity
    Evidence json.RawMessage
}

type Resolver interface {
    Descriptor() Descriptor
    IdentitySchema() runtimev2.ExtensionIdentitySchema
    Normalize(context.Context, Workspace, runtimev2.ExtensionDeclaration) (NormalizedDeclaration, error)
    Resolve(context.Context, Workspace, NormalizedDeclaration) (Resolution, error)
    ValidateResolution(Resolution) error
    Revalidate(context.Context, Workspace, Resolution) (Revalidation, error)
    Acquire(context.Context, Workspace, Resolution) (Artifact, error)
}

type Cache interface {
    Load(context.Context, CacheKey, runtimev2.ExtensionIdentitySchema) (CacheLoad, error)
    Store(context.Context, CacheKey, runtimev2.ExtensionIdentitySchema, Resolution) error
}
```

`Registry` captures each resolver's descriptor and schema once, rejects nil,
invalid, or duplicate providers, and verifies schema provider/kind against the
descriptor. The manager passes the selected schema to the cache. The resolver
semantic version participates in the key, and providers must increase it when
their normalization, identity schema, or evidence semantics change.

The semantic core exposes read-only `Provider`, `Kind`,
`IdentitySchema`, and `Fields` accessors on canonical extension identities
so a provider can validate and acquire its own result. It also exposes a
schema-bound identity JSON codec, not an unchecked identity constructor.
Encoding validates the identity against the supplied schema; decoding uses
`NewExtensionIdentityBuilder`, checks required fields, and compares the
reconstructed canonical bytes and fingerprint. The codec never accepts legacy
string-only identity JSON.

## Typed persistent format

The new record schema is `sqlrs.resolution-cache.canonical.v1`; the old
`sqlrs.resolution-cache.v1` domain is rejected by the current decoder.
`CacheRecord` stores schema version, key, workspace-scope digest, descriptor,
normalized declaration, resolution, and checksum. The resolution stores a
canonical identity and separately owned evidence. Each identity field stores
name, kind, disclosure, commitment, and exactly one payload: UTF-8 text, the
full canonical-value envelope, or secret-reference provider/identifier/version.
The decoder checks the field definition in the selected schema, full payload,
commitment, canonical identity bytes, and identity fingerprint. Secret-reference
fields contain identifiers only; the trusted cache still holds full payloads
of other field kinds, including fields marked protected, so providers must
never put raw credentials in them.

The codec rejects duplicate and unknown JSON members, trailing data, null
required values, noncanonical field order, oversized records/values, invalid
declarations, wrong key/descriptor/schema/version, and mismatched checksums.
It copies caller-owned slices and returns no partially decoded result.
Corrupt/incompatible records are observable invalidations; permission,
cancellation, and I/O failures abort. A cache read validates the generic
record before provider-owned evidence is passed to
`Resolver.ValidateResolution`. The manager validates fresh results before
storage. `CURRENT` updates evidence only and stores before returning.

The directory cache retains existing trusted-root, no-link, sync, atomic
replace, and bounded-pruning behavior. Pruning can inspect the validated
envelope/checksum/key without reconstructing provider-owned identity; malformed
or old-version entries are ignored. The SQLite adapter changes its Go method
signatures and record codec but not its table layout; old-version rows are
reported incompatible rather than migrated or overwritten as a hit.

### Normative v0.5.0 wire grammar

Status: wire-format addendum approved by @evilguest, 2026-10-01.

The JSON object below names every permitted member. Members are emitted in
the shown order by the Go encoder; decoders reject unknown/duplicate members
at any level outside provider-owned `evidence`. They still reject duplicate
members inside `evidence`, but its vocabulary and order belong to the provider.
Invalid UTF-8 bytes and unpaired JSON Unicode surrogates are rejected rather
than replaced; all decoded strings are valid UTF-8. Required members cannot
be `null`; `evidence`
must contain one non-null JSON value. The identity field array is sorted by
the unsigned UTF-8 bytes of `name` and has no repeated names. There are at
most 4096 identity fields, at most 64 JSON nesting levels (the root object is
level 1; each nested object or array adds one), and at most
4 MiB of complete record bytes (before decoding or allocation). Existing
canonical-value depth/node/string limits apply inside `canonical_value`.

```json
{
  "schema_version": "sqlrs.resolution-cache.canonical.v1",
  "key": "<64 lowercase hex digits>",
  "workspace_scope": "<64 lowercase hex digits>",
  "resolver": {
    "role": "<string>",
    "owner": "<string>",
    "kind": "<string>",
    "specification_schema": "<string>",
    "semantic_version": "<string>"
  },
  "normalized_declaration": "<existing versioned declaration JSON object>",
  "resolution": {
    "identity": {
      "schema_version": "sqlrs.runtime.v2.canonical.v1",
      "provider": "<string>",
      "kind": "<string>",
      "identity_schema": "<string>",
      "fields": ["<typed field objects in name order>"],
      "canonical_bytes": "<standard padded base64>",
      "fingerprint": "sha256:<64 lowercase hex digits>"
    },
    "evidence": "<provider-owned non-null JSON value>"
  },
  "checksum": "sha256:<64 lowercase hex digits>"
}
```

The quoted placeholders for `normalized_declaration`, `fields`, and
`evidence` illustrate positions, not literal JSON strings. The declaration
uses its already published role-specific JSON object. A typed field has
members `name`, `kind`, `disclosure`, `commitment`, then exactly one payload:
`text` (a string, including empty), `canonical_value` (standard padded base64
of the complete `CanonicalValue.CanonicalBytes()` envelope), or
`secret_reference` (an object with `provider`, `identifier`, `version` strings
in that order). Kind is respectively `text`, `canonical-value`, or
`secret-reference`; disclosure is `public` or `protected`. The field
commitment is the value from `IdentityField.Commitment()` and must be
recomputed from its decoded name, kind, and complete payload. Base64 must
round-trip byte-for-byte through standard padded encoding. `canonical_bytes`
is the complete `CanonicalResolvedExtensionBytes` result, also standard padded
base64. `fingerprint` is `CanonicalExtensionFingerprint`. Schema-bound
reconstruction must match both integrity copies; a checksum alone cannot
authenticate a rewrite by the trusted cache owner.

The workspace-scope digest is SHA-256 of the UTF-8 bytes of the cleaned,
absolute, symlink-resolved physical workspace root (the current
`EvalSymlinks` -> `Abs` -> `Clean` order). The key preimage is the ordered
sequence of these parts: cache schema string, raw 32-byte workspace-scope
digest, resolver role, owner, kind, specification schema, semantic version,
and `json.Marshal` of the normalized declaration. Each part is preceded by
its uint64 big-endian byte length. `key` is the lowercase hex SHA-256 of the
concatenation. Thus only the cache domain changes from the v0.2.0 key grammar;
old and new entries cannot share a key.

For checksum calculation, omit the `checksum` member entirely and serialize
the remaining typed object using Go `encoding/json.Marshal` with default HTML
escaping and the member order above. Preserve provider-owned evidence as
validated `json.RawMessage`; the standard marshaler compacts it. `checksum`
is `sha256:` plus lowercase hex SHA-256 of those exact serialized bytes.
The encoder emits that representation; the decoder accepts only a complete,
strictly parsed object whose recomputed checksum and all semantic checks
pass. An unsupported top-level schema version returns `ErrIncompatibleCache`
before checksum validation. After a valid outer checksum, an unsupported
identity schema version also returns `ErrIncompatibleCache`; malformed
known-version data, including an invalid checksum or wrong key, returns
`ErrCorruptCache`. Provider evidence is checked
by the selected resolver after generic decoding. Identity-only codec errors
are ordinary canonical validation errors until wrapped by the cache boundary.

## Data ownership and concurrency

Canonical identities and schema definitions are immutable values; accessors
return defensive copies where a slice can escape. `Resolution.Evidence` is
copied at cache and outcome boundaries. Registry, manager, schema selection,
and provider descriptor live in memory. Persistent data consists only of
versioned cache records and separately acquired content-addressed artifacts.
Workspace paths, acquisition locations, timestamps, and provider evidence
never enter logical identity. Concurrent callers may repeat resolution work;
the same-key publication is atomic and readers validate complete generations.

The existing workspace-file provider obtains its fixed extension schema from
the approved `schemas/sqlrs` facade. It has one required public text field,
`content.digest`. The zero-argument facade exposes this specific immutable
schema capability, not generic schema authorship. The provider builds identity
from the computed digest. Its declaration remains the existing versioned
input declaration. `Acquire` reads the digest from the typed canonical field
and still returns only a verified immutable artifact. This is migration of an
existing provider, not a new provider family.

## Release

The PR includes v0.5.0 release notes that identify the breaking Go API and
cache-record schema change. The existing release workflow validates unit,
coverage, race, fuzz-smoke, conformance, external-consumer, provenance, and
public-proxy gates. Issue #147 completes only when RC and GA tags refer to the
same reviewed post-merge commit and both resolve through the public module
proxy and checksum database.
