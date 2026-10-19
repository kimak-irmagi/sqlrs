# Runtime v2 execution schema: API and component structure

Status: **API and component structure approved** by @evilguest, 2026-10-16,
#150, after corrections and repeat review. This includes the refined
[interaction boundary](runtime-v2-execution-schema-flow.md). The test inventory
was separately approved on 2026-10-16. Implementation and tests are available in
[PR #152](https://github.com/kimak-irmagi/sqlrs/pull/152); final review/CI acceptance
applies to its latest commit. Immutable publication completed in v0.6.0 through
[#151](https://github.com/kimak-irmagi/sqlrs/issues/151). The approved
[tool-neutral transform facade](runtime-v2-execution-transform.md) in #156 binds
this unchanged execution identity; its implementation/publication is separate.

## 1. Ownership and scope

Only the existing standard-library-only `backend/libs/runtime-go` module changes.
No service, CLI, HTTP API, database, runtime provider or orchestration is added.

| Location | Responsibility |
| --- | --- |
| Root `canonical_value_inspect_v1.go` | Read-only inspection using existing canonical framing/decoding. |
| `schemas/sqlrs/execution.go` | Semantic input/read types, schema accessor and atomic Build/Parse. |
| `schemas/sqlrs/execution_validation.go` | Shared shape, name, path, reference and budget checks. |
| `schemas/sqlrs/execution_value.go` | Mapping between typed values and public canonical trees; no byte parser or hash implementation. |

The facade depends on public root APIs and, internally, `schemaauthor`, not on
`resolver`, Docker or filesystem packages. Providers own source/image proof,
authoring normalization, physical path projection, capability admission and
execution. The schema describes one invocation without requiring a new process
at each internal progress boundary. Unknown behavior-affecting source options
must be represented or rejected, never dropped.

## 2. Exact construction/read API

```go
func ExecutionEnvironmentSchema() runtimev2.ExtensionIdentitySchema
func BuildExecutionEnvironmentIdentity(ExecutionEnvironmentInput) (
    runtimev2.CanonicalResolvedExtensionIdentity, error)
func ParseExecutionEnvironmentIdentity(runtimev2.CanonicalResolvedExtensionIdentity) (
    ExecutionEnvironmentInput, error)

type ExecutionEnvironmentInput struct {
    Runtime     ExecutionRuntime
    Argv        []ExecutionText
    Environment []ExecutionEnvironmentVariable
    Workdir     ExecutionPath
    User        string
    Network     ExecutionNetworkMode
    Inputs      []ExecutionInputBinding
    Mounts      []ExecutionMount
    Services    []ExecutionServiceBinding
    Secrets     []ExecutionSecretBinding
}
type ExecutionRuntime struct {
    Kind            ExecutionRuntimeKind
    IdentityKind    ExecutionRuntimeIdentityKind
    IdentityDigest  string
    Platform        ExecutionPlatform
    DatabaseService string
}
type ExecutionPlatform struct { OS, Architecture, Variant string }
type ExecutionPath struct { Root, Relative string }
type ExecutionText []ExecutionTextPart
type ExecutionTextPart struct {
    Kind         ExecutionTextPartKind
    Literal      string
    SecretSlot   string
    ServiceRole  string
    ServiceField ExecutionServiceField
    Path         *ExecutionPath
    PathFormat   ExecutionPathFormat
}
type ExecutionEnvironmentVariable struct { Name string; Value ExecutionText }
type ExecutionInputBinding struct {
    Name       string
    Source     ExecutionInputReference
    Profile    ExecutionInputProfile
    Executable bool
}
type ExecutionInputReference struct {
    Provider, Kind, IdentitySchema string
    Fingerprint runtimev2.CanonicalFingerprint
}
type ExecutionMount struct {
    Name, InputName string
    Target         ExecutionPath
    Kind           ExecutionMountKind
    SecretSlot     string
    ReadOnly       bool
}
type ExecutionServiceBinding struct {
    Role, Protocol, Database, User string
    Transport                     ExecutionServiceTransport
    AuthenticationSecret          string
    AuthenticationProfile         string
}
type ExecutionSecretBinding struct {
    Name              string
    Purpose           ExecutionSecretPurpose
    ServiceRole       string
    IdentityReference *runtimev2.SecretReference
}
```

Input/read views are not a new identity or resolver result. Build is atomic and
returns the existing canonical extension; Parse validates and reads it.
No mutable builder, generic schema constructor, observation helper or second
wire format is exported. Named string enums and constants are:

| Type | Constants and values |
| --- | --- |
| `ExecutionRuntimeKind` | `ExecutionRuntimeHost=host`, `ExecutionRuntimeOCI=oci`, `ExecutionRuntimeDatabaseContainer=database-container` |
| `ExecutionRuntimeIdentityKind` | `ExecutionIdentityImageManifest=image-manifest`, `ExecutionIdentityHostBundle=host-bundle`, `ExecutionIdentityHostExecutable=host-executable` |
| `ExecutionTextPartKind` | `ExecutionTextPartLiteral=literal`, `ExecutionTextPartSecret=secret`, `ExecutionTextPartService=service`, `ExecutionTextPartPath=path` |
| `ExecutionPathFormat` | `ExecutionPathNative=native`, `ExecutionPathForwardSlash=forward-slash` |
| `ExecutionMountKind` | `ExecutionMountInput=input`, `ExecutionMountScratch=scratch`, `ExecutionMountSecret=secret` |
| `ExecutionInputProfile` | `ExecutionInputRegularFile=regular-file.v1`, `ExecutionInputRegularTree=regular-tree.v1` |
| `ExecutionSecretPurpose` | `ExecutionSecretAuthentication=authentication`, `ExecutionSecretSemantic=semantic` |
| `ExecutionServiceTransport` | `ExecutionServiceTCP=tcp`, `ExecutionServiceUnixSocket=unix-socket` |
| `ExecutionNetworkMode` | `ExecutionNetworkNone=none`, `ExecutionNetworkServices=services`, `ExecutionNetworkLocalContext=local-context` |
| `ExecutionServiceField` | `ExecutionServiceHost=host`, `ExecutionServicePort=port`, `ExecutionServiceDatabase=database`, `ExecutionServiceUser=user` |

Zero/unknown enum values fail. There is no per-call schema authoring.

## 3. Resolved invocation, not authoring defaults

Build receives effective argv, environment, workdir and user. It has no
inherit/clear flags, `*[]` command fields or image/replace environment mode.
Those distinctions belong to the declaration/resolution stage. A resolver reads
bounded digest-verified image configuration and applies the source adapter's
rules before Build, without acquiring layers or starting a tool.

For Compose, [entrypoint rules](https://docs.docker.com/reference/compose-file/services/#entrypoint)
and [environment rules](https://docs.docker.com/reference/compose-file/services/#environment)
are applied together: an explicit entrypoint suppresses image CMD unless an
explicit project command supplies arguments; a removed variable is absent from
the effective environment, while an empty value is present and empty.
The effective executable plus arguments become one ordered Argv list.
No additional splitting, shell insertion or variable expansion happens in Build.

This resolves the previous ambiguity: declarations preserve absent/empty
differences, while identical effective configurations have identical identities.
A generic decoded value cannot reinterpret nil as a new image-default request.
Workdir is explicit. User is the effective selected account; for an image, empty
means the platform-defined default account, not inheritance from a mutable live
container. Host requires the observed effective account (UID/principal/SID), not
permission to switch to another account. A provider unable to prove its account
mapping rejects the execution.

Argv must be nonempty. Its first word is a nonempty literal or one path part;
never a secret/service value. For host it must be a native-format path anchored
at `tool`, binding launch to the verified tool. All other word order is preserved.
Adjacent literal parts merge, redundant empty literal parts disappear, and one
empty literal represents an empty argument. Literal strings are never searched
for paths, shell syntax or embedded substitutions.

For complete profiles the environment is the full resolved semantic map.
Image-derived defaults must also be classified before Build; a known secret
cannot be copied as literal merely because it came from image configuration.
The only additional process-visible variables permitted are those prescribed by
an admitted authentication profile (§6), without collision with explicit names.
No arbitrary "operational injection" may override/add LANG, TZ, JVM options or
other process inputs outside identity. Host-executable has the explicitly limited
local-context rule in §7 instead.

## 4. Paths and named sources

Paths are typed coordinates, not physical host strings:

- Root `mount:<name>` identifies the visible destination of a named mount.
- Root `tool` identifies the verified host executable (empty Relative only) or
  host bundle directory. It is unavailable to image profiles.
- Root `workspace` identifies an attempt-owned local workspace for host mode.
- Root `image` identifies the POSIX image filesystem; Windows images use
  `image:<lowercase-drive-letter>`, e.g. `image:c`.
- Roots `service:<role>:socket`, `service:<role>:socket-directory` and
  `service:<role>:credential-file` identify owner-resolved endpoint files.
  They require empty Relative, an existing role, and respectively Unix-socket
  transport or an admitted file-producing authentication profile.

Relative is empty or a clean slash-separated relative path, with no leading,
trailing or repeated separator, backslash, NUL, `.` or `..` component. Path
coordinates are independent of how Windows/Unix store the physical workspace.
The library validates root syntax, runtime compatibility and references;
file/service-secret roots cannot serve as workdirs or mount destinations.
Providers additionally check destination-filesystem aliases, confinement,
reparse points/symlinks and effective permissions; string checks are not physical
authority.

A path part has exactly Path/PathFormat populated. Native produces the target
platform's native path; forward-slash uses slash separators, retaining a Windows
drive prefix where applicable. Rendering happens at execution, after physical
binding; no physical address enters identity. Workdir and mount destinations
are path values, not strings requiring implicit rewriting. Workdir must resolve
to a directory; known file-only roots are rejected without I/O, and tree-member
existence/type is checked by the source owner.

Example: `--changelog-file=` plus `{Root: mount:project, Relative: changelog.xml}`
is one typed word. One executor may bind it under a local temporary directory,
another at the declared container mount destination. Neither scans/replaces
arbitrary literal arguments.

Each named input is a **qualified reference** to an existing canonical resolved
source identity: provider, kind, identity-schema version and public fingerprint.
A reference is not an envelope or proof; providers must load/verify its full
identity and source capability before use. The facade checks names and fingerprint
syntax, not source existence or a foreign provider's semantics.

Materialization profiles make the observable filesystem contract explicit:

- `regular-file.v1`: exact referenced bytes as a regular file; Executable
  selects normalized 0755 versus 0644 mode. Source timestamps, owner IDs and
  host paths are not inherited.
- `regular-tree.v1`: a complete versioned source manifest covering relative
  names, directory/file kinds, every file's immutable content and executable
  flag, including empty directories. Executable on the whole binding must be
  false. A plain untyped hash of directory bytes is insufficient.
- Both profiles use fixed epoch timestamps, 0755 directories, regular files
  only, no links/special files/xattrs, and execution-account ownership as defined
  by the resolved execution context. File modes precede the mount access policy.
  Any profile change affects identity. Writable input mounts operate on private
  copies/overlays, never on the immutable source.

The referenced source schema owns its manifest encoding and verification.
A file-content identity can back a tree only if it identifies a versioned
manifest interpreted by an admitted source materializer; an arbitrary file hash
must not be relabelled a tree. No new source hash domain, tree resolver or source
I/O is implemented by #150. A missing compatible source owner is an explicit
execution-admission failure, not a relaxed manifest requirement.

## 5. Mounts, services and network policy

Mount names and input names are unique logical identifiers. Input mounts require
InputName and no SecretSlot; scratch mounts require neither; secret mounts
require a semantic secret slot, no InputName, and read-only access. Scratch is
read-write. Target must be an image root for image modes or workspace root for
host, not another mount/service/tool root; this prevents target-reference cycles.

Mounts are an **ordered list**, not a map. Nested targets are representable:
the order is application order and contributes to identity, consistent with
[OCI mount ordering](https://github.com/opencontainers/runtime-spec/blob/main/config.md#mounts).
The native adapter must resolve its effective mount order before Build. Exact
duplicate logical targets fail; additional physical aliases or unsupported
overlays fail provider admission. A mount-root path observes the final overlaid
namespace; it does not assert that a shadowed file still comes from its parent
input. Every layer remains represented in identity.

Services are logical target bindings, not caller-supplied connection addresses.
They have explicit TCP/Unix-socket transport, protocol, database and logical user.
Selecting database/user requires a nonempty value. Host/port parts require TCP;
socket-path roots require Unix-socket transport. Database-container runtime
requires its declared DatabaseService; other runtime kinds require it empty.

Network policy is independent of the existence of a target. None requests no
network attachments; local IPC/loopback use must still be checked by the provider.
Services requests connections only to declared targets. The schema does not ban
a database-container target or Unix socket under none. A provider must prove the
requested isolation/transport pairing or reject it before mutation; it cannot
silently downgrade isolation.

Local-context is representable only for host-executable: it explicitly requests
the opted-in local process's network context without claiming isolation or a
complete dependency set. Shared/complete profiles reject it. This permits an
ordinary local build's network use without relabelling it as confined/reusable;
the mandatory no-reuse/no-shared-replay gate still applies.

The facade checks structural consistency, not whether one physical provider can
implement every valid description. Rejection of privileged workloads, unsupported
filesystem semantics, network arrangements and tool execution remains provider
admission, with explicit capability/portability reasons.

## 6. Secret purpose and constrained authentication

A missing reference is not evidence of irrelevance:

- Semantic secrets require a valid non-secret immutable SecretReference.
  Unknown purpose, zero references and unversioned semantic inputs fail Build
  and Parse; they never become authentication implicitly.
- Authentication slots require ServiceRole and no IdentityReference. The matching
  service must point back through AuthenticationSecret, have a nonempty logical
  user, and name a versioned AuthenticationProfile. A slot belongs to one service.
  These values define authentication for that same logical role, not arbitrary
  secret input. An authentication slot cannot appear in generic text or a secret
  mount.
- An admitted authentication profile defines its secure environment/file
  injection and must not change semantic session settings, logical user or
  explicit argv/env/path fields. Unknown profiles, injection collisions and
  profiles unable to establish this property fail admission. No secret value,
  DSN or capability bearer enters identity, provenance, argv or logs.
- Semantic secret parts are permitted in environment values, not argv.
  Semantic secret mounts expose owner-delivered files; argv may reference the
  typed path to the file, never expand its contents. Actual injection remains
  inside the owner boundary.

Provider/source authors remain responsible for classification: general-purpose
tools can deliberately consume even an authentication credential as data. If its
use affects the result, it is semantic and requires a version. If this cannot be
established, no reusable-State execution is admitted; there is no value-name
heuristic or claim that the library proves arbitrary program behavior.

Services/secrets/inputs must have a declared use through a target, path, mount or
authentication relation. Unknown/duplicate names and invalid cross-references
fail. Unused descriptors are not added merely as provenance.
Declared network-policy target use also counts as use of a service; it need not
appear as a template part when an admitted project file selects that service.

## 7. Local host profile and mandatory reuse gate

Runtime kinds are host/oci/database-container. IdentityDigest is a lowercase
sha256 digest. Image kinds require image-manifest: the selected platform manifest,
not an index or daemon ID. Host uses either:

- host-bundle: a verified immutable tool/dependency bundle and resolved process
  context. Shared execution/reuse is still subject to provider capabilities.
- host-executable: only the selected executable is content-verified. Ordinary
  installed local tools are allowed without claiming their ambient libraries,
  PATH, OS services or other dependencies form an immutable bundle.

The latter binds an opaque current local context outside the canonical object.
Environment contains explicit overlays only; User is the observed account and
Workdir a workspace coordinate. The owner must recheck the selected executable
and context before start. Physical CWD, ambient environment (possibly secret),
host identifiers and opaque context handles are not copied into records.

**Mandatory gate:** Parse success is not replay/reuse approval. A host-executable
profile is local-only, executes again for each request, and cannot select a
reusable State, publish one, or authorize shared replay, even on the same host.
Resolution-record caching is distinct from reusable-State caching and does not
lift this restriction. These requirements are derivable from the semantic
IdentityKind; they cannot be overridden by a portability observation.

The owner applies the gate **before any reusable-State lookup/selection** and
again at publication/shared replay. Ineligibility propagates monotonically from
the source through all derived steps: a later complete-profile transform cannot
make a descendant of an incomplete result reusable. Only a newly verified
complete source/restore or factory establishes a new eligible baseline. Preserve
this qualification in durable workspace/attempt provenance across recovery;
a bare StateID or valid lineage envelope is not eligibility evidence. The pure
facade records the profile, not this execution-state lifecycle. Consumers must
implement these gates before enabling the profile for production.

The identity truthfully commits to the selected resource/extent, not to unknown
ambient dependencies. It is not a proof of a reproducible full result.
For database-container mode the owner must also prove the live tool files match
the declared image or reject the resolution; local-only cannot repair a false
content assertion.

Platform OS/architecture are required lowercase atoms
`[a-z0-9][a-z0-9._-]{0,63}`; variant is empty or such an atom. There is no DBMS-major,
repository or implicit host-platform selection. A provider's inability to
support an OS/profile is not a reason to reinterpret its identity.

## 8. Fixed schema, normal form and read-only tree API

Provider `sqlrs.execution`, kind `environment`, identity schema
`sqlrs.execution-environment.v1`. This is an approved, unpublished schema, not a
migration/reinterpretation of an existing v1 record.

| Required field | Kind / disclosure | Value |
| --- | --- | --- |
| primitive | text / public | Runtime kind |
| runtime | canonical-value / protected | identity_kind, digest, platform (os/architecture/variant); database_service only for database-container |
| invocation | canonical-value / protected | argv, environment, workdir, user |
| inputs | canonical-value / protected | Name -> source (provider/kind/identity_schema/fingerprint), profile, executable (yes/no for files only) |
| network | canonical-value / protected | mode and services (protocol, database, user, transport; authentication_secret/authentication_profile together when present) |
| mounts | canonical-value / protected | Ordered maps: name, target, kind, access; input_name for input or secret_slot for secret |
| secrets | canonical-value / protected | Name -> purpose plus service_role for authentication, or reference (provider/identifier/version) for semantic |

Paths are maps root/relative. Text parts are maps with kind and exactly the
corresponding literal; slot; role/field; or path/format. Argv and each word are
ordered lists. Keyed collections are canonical maps (nil/empty -> empty map).
Paths and scalar strings are never null. Boolean executable is encoded as the
closed string yes/no, because canonical-v1 has no boolean node; it is omitted for
trees. Mount access is read-only/read-write. All field/node shapes and named
variants are closed: missing/extra keys and unknown kinds fail.
Non-applicable Go input fields must also be zero/nil, not silently ignored:
for example semantic secret ServiceRole, authentication IdentityReference,
or Literal/SecretSlot on a path part. Mount access reflects ReadOnly; source
file/tree applicability checks precede construction.

Environment names match `[A-Za-z_][A-Za-z0-9_]{0,127}`; Windows names normalize
to ASCII uppercase before duplicate checks, otherwise case is preserved.
Binding names use `[a-z][a-z0-9._-]{0,63}`; protocol/profile/schema identifiers
use the existing identifier grammar. AuthenticationProfile additionally has a
nonzero .vN suffix. All text is bounded UTF-8 without NUL. There is no trimming
or Unicode normalization.

Non-secret references form a protected canonical map, constructed/reconstructed
only through validated SecretReference values. Existing scalar secret-reference
encoding and canonical tags/domains remain unchanged. The seven-field schema
exposes no operational fields or observation helper.

Parse checks exact owner/kind/schema, field kinds/disclosure, aggregate budgets
and nested shape; then shared validation, public reconstruction and canonical
byte comparison. Non-normalized identities fail instead of being repaired.
Build does not call Parse. No partially populated result is returned.

Root inspection uses the approved immutable API:
```go
type CanonicalValueKind string // null, string, list, map, set; zero invalid
func (CanonicalValue) Kind() CanonicalValueKind
func (CanonicalValue) StringValue() (string, bool)
func (CanonicalValue) Elements() ([]CanonicalValue, bool)
func (CanonicalValue) Entries() ([]CanonicalMapEntry, bool)
func (CanonicalValue) EncodedSize() int
```
Named constants are CanonicalValueKindNull/String/List/Map/Set. Wrong-kind/zero
returns zero/false; valid empty collections return non-nil empty slices/true.
Slices are defensive, children immutable, ordering canonical. EncodedSize is
no-copy, zero for invalid. Root implementation reuses validated child decoders;
the facade never parses canonical bytes. No caller input is retained.

## 9. Budgets, errors and integration obligations

Preflight raw collection counts, text sizes and checked arithmetic before
proportional allocation/sorting. These raw-input guards are distinct from the
generated-tree budget: literal merging cannot evade a documented raw limit.
Each collection uses MaxCanonicalCollectionMembers; raw strings collectively
use MaxCanonicalValueBytes, with existing per-string/key/depth limits.

Construct bounded nodes through public helpers; then enforce the sum of
NodeCount/EncodedSize over all generated fields before constructing the final
identity. Do not duplicate byte framing to predict its exact size. Temporary
memory is bounded by the fixed field count and root limits. Parse checks those
sums before child traversal/copying. Aggregate limits are MaxCanonicalValueNodes
and MaxCanonicalValueBytes; existing envelope limits also remain.

Errors use ValidationError/ErrInvalid and existing shape_invalid, value_invalid,
non_canonical, unknown_member and limit_exceeded codes. Only static paths and
numeric indices appear, never caller names, paths or secret references.
A pure semantic validation error is distinct from a later provider capability,
proof or authority failure.

Both fresh and cached resolutions must pass Parse via ValidateResolution, then
provider evidence validation. Generic cache integrity alone is insufficient.
Reuse qualification is checked before reusable-State planning and propagated
through the lineage (§7). Before execution, provider admission verifies manifests,
image/tool content, logical-to-physical bindings, secret/authentication profiles,
reuse eligibility, owner generation, isolation and complete storage inventory.
No cache/transport permission is inferred from a canonical fingerprint.

The facade performs none of that I/O. Existing result/cache/envelope formats,
canonical domains and other published schemas/vectors stay unchanged.
The implementation and checks are in PR #152, not an immutable module release.
Publication remains #151; [review corrections and repeat review](runtime-v2-execution-schema-review.md)
record this design's limits. The [test inventory](runtime-v2-execution-schema-tests.md)
was separately approved on 2026-10-16.

## 10. Proposed module publication (#151)

The user approved direct publication of an already reviewed source commit on
2026-10-16, without a version-specific preparation PR. The earlier metadata-only
PR sequence is withdrawn. The reusable [publication contract](runtime-go-module-publication.md)
and [automation checks](runtime-go-module-publication-tests.md) define separate
automation H and library source S, explicit manual modes, external notes and
verified-candidate promotion. The detailed contract/list were approved on
2026-10-16; local implementation does not yet prove module publication.

Automation PR #153 and post-merge CI passed. After the initial ref-create denial,
the user explicitly approved S=bf161c7cb6bf54b1ae6f8614e452963a5ddad36f for
v0.6.0-rc.1 and v0.6.0. Against #152's original merge, only module README
differs; Go source/tests, dependencies and vectors are unchanged. The RC tag
already fixes this source. A read-after-write automation correction must retain
it and resume the matching draft; no successful RC/GA release is claimed yet.
No RELEASE-v0.6.0.md, new library commit
or application integration is required. The unused version does not revive #140.
Keep #151 open until the final public-proxy, checksum and clean-consumer checks
pass; record the immutable version/source and run links in its GitHub report.
No version-specific roadmap PR is a release gate. See the
[ADR](../adr/2026-10-16-runtime-go-execution-schema-publication.md) and
[feature publication checks](runtime-v2-execution-schema-tests.md#publication-checks-151-proposed).
