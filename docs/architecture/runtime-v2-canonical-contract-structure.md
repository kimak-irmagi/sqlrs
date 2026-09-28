# Runtime v2 canonical v1 contract: component structure

Status: approved for issues #130 and #131 on 2026-09-27.

## Module layout

```text
backend/libs/runtime-go/
  canonical.go                       frozen legacy-v2 framing and hashing
  identity.go                        frozen legacy-v2 string fields
  canonical_value_v1.go              typed tree and civ1 token
  identity_field_v1.go               immutable text/value/reference fields
  secret_reference_v1.go             non-secret credential revision reference
  canonical_identity_v1.go           canonical-v1 identities and encoders
  canonical_state_v1.go              canonical-v1 state and lineage
  fingerprint_envelope_v1.go         strict recomputing transport envelope
  explain_v1.go                      safe/internal disclosure projections
  operational_observation_v1.go      non-identity diagnostics
  builder_v1.go                      schema-bound supported builders
  internal/canonicalv1/              shared implementation and opaque data
  schemaauthor/                      explicit low-level trust boundary
  conformance/
    canonical-v1/
      manifest.json
      manifest.sha256
      vectors/*.json
  RELEASE_NOTES-v0.3.0.md
```

The nested module remains standard-library-only. Legacy files/types continue to
accept only `sqlrs.runtime.v2`. New types accept only
`sqlrs.runtime.v2.canonical.v1`; JSON wire shapes use distinct required
discriminators and cannot be unmarshaled into the opposite revision.

## Public constants and limits

```go
const LegacySchemaVersion = "sqlrs.runtime.v2"
const CanonicalSchemaVersion = "sqlrs.runtime.v2.canonical.v1"

const MaxCanonicalValueDepth = 32
const MaxCanonicalValueNodes = 4096
const MaxCanonicalCollectionMembers = 256
const MaxCanonicalStringBytes = 4096
const MaxCanonicalMapKeyBytes = 1024
const MaxCanonicalValueBytes = 1 << 20
const MaxCanonicalEnvelopeBytes = 4 << 20
```

`SchemaVersion` remains an alias of `LegacySchemaVersion` for v0.2 source
compatibility. The five canonical-v1 domain constants and hash algorithm are
also exported. Any future identity-affecting change gets a new semantic schema
and new identity domains; these constants are never repointed.

## Canonical values

```go
type CanonicalValue struct { /* opaque immutable tree + bytes */ }
type CanonicalMapEntry struct {
    Key   string
    Value CanonicalValue
}

func CanonicalNull() CanonicalValue
func CanonicalString(string) (CanonicalValue, error)
func CanonicalList([]CanonicalValue) (CanonicalValue, error)
func CanonicalMap([]CanonicalMapEntry) (CanonicalValue, error)
func CanonicalSet([]CanonicalValue) (CanonicalValue, error)
func ParseCanonicalValueEnvelope([]byte) (CanonicalValue, error)
func ParseCanonicalValueToken(string) (CanonicalValueToken, error)

func (CanonicalValue) Token() CanonicalValueToken
func (CanonicalValue) CanonicalBytes() []byte
func (CanonicalValueToken) String() string
```

The zero values of `CanonicalValue` and `CanonicalValueToken` are invalid.
Accessors return defensive copies. Map input is an entry slice and duplicate
source keys are rejected before sorting. Constructors maintain aggregate
budgets incrementally; the decoder checks encoded lengths and counts before
allocation.

`internal/canonicalv1` separates validation from allocation with non-allocating
`allocationPlan` helpers used by the production constructor and decoder paths.
They validate u64-to-`int` conversion, remaining node/member/byte budgets, and
sort cardinality before any proportional `make`, recursion, or sort. Same-package
tests call these production helpers directly; there is no behavior-changing test
hook or alternate allocator.

## Typed identity fields and secret references

```go
type IdentityFieldKind string
const (
    IdentityFieldText             IdentityFieldKind = "text"
    IdentityFieldCanonicalValue   IdentityFieldKind = "canonical-value"
    IdentityFieldSecretReference  IdentityFieldKind = "secret-reference"
)

type IdentityField struct { /* opaque */ }
type SecretReference struct { /* opaque */ }

func NewTextIdentityField(name, value string) (IdentityField, error)
func NewCanonicalValueIdentityField(name string, value CanonicalValue) (IdentityField, error)
func NewSecretReference(provider, identifier, version string) (SecretReference, error)
func NewSecretReferenceIdentityField(name string, ref SecretReference) (IdentityField, error)
```

Read-only accessors expose name, kind, contribution commitment, and the
kind-specific non-secret value. A text value is never auto-parsed as a token.
`ParseCanonicalValueToken` supports comparison and display, but a parsed token
cannot construct an identity field. Canonical-value fields retain the complete
immutable tree, and their integrity envelopes always recompute the token.

There is no raw-secret constructor. Secret-reference provider uses identifier
validation; identifier and version are non-empty UTF-8 and budgeted. API docs
state that passing secret material as text violates the schema-author trust
contract and is rejected by supported schemas and their conformance tests.

## Canonical-v1 identities and states

New opaque types are distinct from their legacy counterparts:

```go
type CanonicalFactoryIdentity struct { /* opaque */ }
type CanonicalTransformIdentity struct { /* opaque */ }
type CanonicalResolvedExtensionIdentity struct { /* opaque */ }
type CanonicalState struct { /* opaque */ }
type CanonicalRecipeLineage struct { /* opaque */ }
type CanonicalRelativeLineage struct { /* opaque */ }

func CanonicalFactoryIdentityBytes(CanonicalFactoryIdentity) ([]byte, error)
func CanonicalTransformIdentityBytes(CanonicalTransformIdentity) ([]byte, error)
func CanonicalResolvedExtensionBytes(CanonicalResolvedExtensionIdentity) ([]byte, error)
func CanonicalDerivedStateBytes(CanonicalStateID, FingerprintEnvelope) ([]byte, error)
```

Canonical fields are sorted by UTF-8 name bytes and unique. The derived-state
encoder accepts only a verified transform envelope. Legacy `StateID` and
canonical-v1 `CanonicalStateID` are distinct named types even though both render
as SHA-256 strings.

## Explicit schema-authoring boundary

Both the root package and `schemaauthor` use
`internal/canonicalv1`, avoiding an import cycle. The internal package owns the
opaque data; root and `schemaauthor` expose separate aliases/wrappers.

`schemaauthor` defines generic schema inputs:

```go
type FieldRole string       // semantic or operational
type DisclosureClass string // public or protected
type FieldDefinition struct {
    Name       string
    Role       FieldRole
    Kind       runtimev2.IdentityFieldKind
    Required   bool
    Disclosure DisclosureClass
}
type SchemaInput struct {
    Provider, SemanticKind, IdentitySchema, ObservationSchema string
    Fields []FieldDefinition
}

func NewFactorySchema(SchemaInput) (FactoryIdentitySchema, error)
func NewTransformSchema(SchemaInput) (TransformIdentitySchema, error)
func NewExtensionSchema(SchemaInput) (ExtensionIdentitySchema, error)
```

An AST and Go package-graph architecture checker rejects direct imports of
`runtime-go/schemaauthor`, aliases, constructor references, generic type
exposure, and re-exports outside the exact allowlist
`backend/libs/runtime-go/schemas/**`, conformance fixtures, and explicit
architecture tests. A production package may depend transitively on
`schemaauthor` only through an approved schema facade; every package-graph path
must enter the allowlist before reaching the trust-boundary package. Approved
schema packages expose only schema-specific build functions.

An external author may still deliberately misuse the public low-level package.
That trust limitation is documented and is not disguised as a field-name
denylist.

## Supported builders and extension completeness

Root builders are bound to an already authored schema:

```go
type FactoryIdentityBuilder struct { /* opaque */ }
type TransformIdentityBuilder struct { /* opaque */ }
type ExtensionIdentityBuilder struct { /* opaque */ }

func NewFactoryIdentityBuilder(FactoryIdentitySchema, FactoryDeclaration) (*FactoryIdentityBuilder, error)
func NewTransformIdentityBuilder(TransformIdentitySchema, TransformDeclaration) (*TransformIdentityBuilder, error)

func (*FactoryIdentityBuilder) AddIdentityField(IdentityField) error
func (*FactoryIdentityBuilder) AddObservation(OperationalObservation) error
func (*FactoryIdentityBuilder) BindExtensions(ResolvedFactoryExtensions) error
func (*FactoryIdentityBuilder) Build() (FactoryIdentityComposition, error)
```

Transform and extension builders expose the corresponding methods. Builders are
single-use, not concurrency-safe, and transactional: a rejected method call does
not mutate state, and successful `Build` seals the builder. Schema definitions
enforce field kind, role, requiredness, and disclosure class. Observation values
cannot be converted to `IdentityField`.

`ResolvedFactoryExtensions` contains ordered inputs plus optional execution
environment and deployment; transform omits deployment. Builders compare count,
position, owner, and semantic kind with the declaration. Canonical binding names
are fixed. `CanonicalExtensionFingerprint` and
`ComposeCanonicalResolvedFields` accept only canonical-v1 types.

Legacy `ExtensionFingerprint` and `ComposeResolvedFields` retain their existing
signatures and bytes and are explicitly documented as legacy-v2.

## Operational observations

`OperationalObservation` is a separately versioned opaque value with owner,
semantic kind, observation schema, sorted fields, and disclosure class per
field. It is carried alongside composition output, never inside identity.
Supported schemas place job IDs, runtime/container handles, timestamps, physical
sizes, materialization paths, and snapshot/checkpoint backends here.

Raw secret values are invalid observations. Execution-only credentials remain in
the executor's private input channel, outside Runtime v2 semantic/provenance
objects.

## Fingerprint envelopes, integrity, and explain

```go
type FingerprintKind string
type FingerprintDescriptor struct { /* immutable metadata */ }
type FingerprintEnvelope struct { /* descriptor + typed subject */ }
type CanonicalRecipeEnvelope struct { /* strict verified lineage */ }
type CanonicalRelativeEnvelope struct { /* strict verified lineage */ }

func NewCanonicalValueEnvelope(CanonicalValue) (FingerprintEnvelope, error)
func NewFactoryEnvelope(CanonicalFactoryIdentity) (FingerprintEnvelope, error)
func NewTransformEnvelope(CanonicalTransformIdentity) (FingerprintEnvelope, error)
func NewExtensionEnvelope(CanonicalResolvedExtensionIdentity) (FingerprintEnvelope, error)
func NewDerivedStateEnvelope(CanonicalStateID, FingerprintEnvelope) (FingerprintEnvelope, error)
```

Strict JSON unmarshal is atomic and recomputes all available value tokens, field
commitments, compact digests, state IDs, parent links, and endpoints. A relative
envelope requires a verified factory-state or derived-state anchor envelope;
bare external anchors are insufficient for the persisted/explain surface.

The five allowed descriptor kinds have a fixed domain/schema/algorithm mapping.
Provider metadata is required for factory, transform, and extension subjects and
forbidden for canonical values and derived states. A descriptor is decoded only
inside an envelope; there is no public standalone unmarshal that could create a
trusted descriptor without its canonical subject.

```go
type ExplainProfile string // safe
type InternalDisclosureAuthorizer interface {
    AuthorizeInternalDisclosure(context.Context) error
}

func ExplainSafe(FingerprintEnvelope) (FingerprintExplanation, error)
func ExplainInternal(context.Context, FingerprintEnvelope, InternalDisclosureAuthorizer) (FingerprintExplanation, error)
```

Safe explanations retain field names and kinds but replace protected payloads,
opaque secret identifiers, and their per-field commitments with typed redaction
markers. They retain the aggregate digest and lineage IDs already verified by
the source envelope, but are not independently verifiable proofs of redacted
payloads. `FingerprintExplanation` has no public unmarshal/trust constructor and
cannot be passed where an envelope or identity is required.

Internal explanations are constructed only after a non-nil authorizer succeeds
for a live context. Nil, denial, error, or canceled context returns the zero
projection. Neither profile contains raw secret material. Full envelopes alone
verify payload-to-commitment links and recompute identity and lineage.

## Conformance and release components

```go
type ConformanceBundle struct { /* manifest + immutable file map */ }
type ConformanceBundleDescriptor struct { /* expected schema/version tuple */ }

func LoadConformanceBundle() (ConformanceBundle, error)
func ParseConformanceBundle(expected ConformanceBundleDescriptor, manifest, detachedDigest []byte, files map[string][]byte) (ConformanceBundle, error)
func (ConformanceBundle) BundleSchemaVersion() string
func (ConformanceBundle) BundleVersion() string
func (ConformanceBundle) ManifestDigest() string
func (ConformanceBundle) Files() map[string][]byte
func (ConformanceBundle) Verify() error
```

All returned bytes/maps are defensive copies. Loading first verifies safe paths,
lexicographic manifest order, sizes, file digests, and detached manifest digest,
then strict semantic vectors. The external consumer calls this public API from
the public-proxy module and never locates module-cache files.

The in-memory parser rejects unsafe path strings but has no filesystem objects
and therefore makes no symlink claim. A separate repository/package filesystem
check walks the source bundle without following links, rejects symlinks and
non-regular files, and supplies ordinary file bytes to the parser. The expected
descriptor binds bundle-schema version, bundle version, and semantic schema;
`LoadConformanceBundle` uses compiled current constants. This closes the metadata
gap intentionally left by the mandated detached-digest preimage.

The pre-merge immutable-bundle policy compares changed paths and bytes with the
merge base and requires a new bundle directory/version. Release preflight repeats
the comparison against the latest published module tag. It is a repository
policy check, not an invariant inferable from one directory in isolation.

Checked-in compatibility and migration notes contain static schema and bundle
facts, not a self-referential source SHA. Release automation generates a
content-addressed attestation asset from the tagged commit containing source SHA,
module/tag, canonical schema, bundle-schema version, bundle version, and manifest
digest. The workflow refuses to overwrite an existing asset; verifiers still
compare its contents with the protected tag and public proxy rather than treating
release-asset storage as intrinsically immutable. RC checks validate the tag,
proxy, typed envelopes, bundle, and clean consumer. GA checks validate the same
commit, checksum database, attestation, and legacy-v2 non-reinterpretation
vectors before issue closure.

The attestation JSON has fixed member order
`module`, `tag`, `source_sha`, `canonical_schema`, `bundle_schema_version`,
`bundle_version`, `manifest_digest`, uses UTF-8/LF and one terminal newline, and
has no extra members. A companion `.sha256` asset contains the SHA-256 of those
exact bytes as `sha256:<lowercase-hex>\n`; the digest is also used in the asset
name. This is content addressing, while tag/proxy comparison remains the trust
check.
