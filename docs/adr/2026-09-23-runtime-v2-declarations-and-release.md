# Runtime v2 declarations and module-release decisions

Conversation timestamp: 2026-09-23 22:52 Asia/Novosibirsk (15:52 UTC).
GitHub user ID: @evilguest (41718235). Agent: Codex (GPT-5).
Approval timestamp: 2026-09-24 00:18 Asia/Novosibirsk (2026-09-23 17:18 UTC).
Status: accepted for issues #108, #123, and #124.

## Decision 1: preserve #107 golden transport with versioned documents

Question: how should standalone declarations gain a required version without
changing #107 provenance golden vectors?

Alternatives: change every nested declaration wire shape; keep standalone shapes
unversioned; retain nested diagnostic shapes and add required versioned document
wrappers plus a versioned `RecipeDeclaration`.

Decision: keep existing nested diagnostic declaration JSON unchanged
when no new extension fields exist. Add `FactoryDeclarationDocument`,
`TransformDeclarationDocument`, and `RecipeDeclaration`, each with required
`schema_version: sqlrs.runtime.v2` and strict bounded decoding.

Rationale: standalone persistence/transport fails closed on versions while the
already-reviewed identity/lineage vectors remain byte-for-byte unchanged.

## Decision 2: typed provider extension roles

Question: how should file/input, execution-environment, and deployment specs be
represented without provider types in the core?

Alternatives: unqualified maps; one generic extension type with a mutable role
string; distinct opaque Go types sharing an owner/kind/schema/field envelope.

Decision: add distinct `InputDeclaration`,
`ExecutionEnvironmentDeclaration`, and `DeploymentDeclaration` types. Their
specifications use owner, kind, specification schema, and canonical unique
name/value fields. Dispatch uses the full role-qualified tuple.

Rationale: provider payloads remain extensible and versioned while Go prevents
cross-role substitution and the core can enforce bounds deterministically.

## Decision 3: resolved extensions are not transforms

Question: should a resolver directly return a Runtime v2 factory/transform?

Alternatives: direct transform identity; an opaque digest; a qualified
`ResolvedExtensionIdentity` composed later by an adapter.

Decision: resolvers return `ResolvedExtensionIdentity`. It is immutable
and provider-qualified but cannot be passed directly to lineage operations.
Adapters explicitly map one or more extensions into complete factory/transform
identity fields.

Rationale: file contents, OCI environments, and deployments are semantic inputs,
not necessarily complete execution steps; explicit composition avoids category
errors and hidden field loss.

`ResolvedExtensionIdentity` receives its own domain-separated
`ExtensionFingerprint`. The mandatory `ComposeResolvedFields` helper binds every
supplied, uniquely named extension into reserved `extension.*` resolved fields.
Adapters with extension declarations must use this helper and pass conformance
tests proving that none were omitted.

## Decision 4: diagnostics remain non-identity

Question: where should capability and portability data live?

Alternatives: resolved identity fields; untyped maps; separately typed versioned
diagnostic observations.

Decision: add separate `CapabilityObservation` and
`PortabilityObservation` values and retain resolver observations as diagnostics.
Identity constructors do not accept them.

Rationale: explainability is retained without making host/runtime availability
change logical StateIDs.

## Decision 5: immutable tag remediation and next release

Question: how should the prematurely published nested-module `v0.1.0` be handled
after discovering #123 requires #124 in the first recommended release?

Alternatives: delete/move/reuse the tag; leave it silently recommended; preserve
it, warn consumers, retract it, and publish the completed contract as `v0.2.0`.

Decision: never move or reuse `backend/libs/runtime-go/v0.1.0`. Its
GitHub prerelease notes identify schema/source and warn against new adoption. Add
`retract v0.1.0` with a reason to the next `go.mod`; publish the combined #108/#124
contract as `backend/libs/runtime-go/v0.2.0` after all gates pass.

Rationale: the Go proxy has already observed `v0.1.0`, so immutability is the
only checksum-safe choice. Retraction and a new minor version give consumers a
deterministic, actionable upgrade path.

## Decision 6: nested-module release gate

Question: how should future Runtime v2 module tags be validated?

Alternatives: reuse the product `v*` binary workflow; manual checks; a dedicated
workflow for `backend/libs/runtime-go/v*` plus a clean external consumer.

Decision: add a dedicated two-mode nested-module workflow. A manual
preflight validates the proposed version/commit and all local quality gates
before tag creation. A tag-push mode verifies the immutable tag and
resolves/downloads/tests it with `GOWORK=off`, no `replace`, a clean module cache,
and bounded public-proxy propagation retries. Release notes must name
`sqlrs.runtime.v2` and the source commit.

Rationale: product and nested-module tags have different prefixes and artifacts;
post-publication verification proves what external consumers actually receive.

The planned `v0.2.0` commit is first published immutably as `v0.2.0-rc.1`. GA
uses the same commit only after clean-consumer/public-proxy checks pass. The PR
is organized as four independently buildable commits: declaration boundary,
resolver framework, file/cache/artifact implementation, and release automation.

## Decision 7: staged acceptance without a release cycle

Conversation refinement timestamp: 2026-09-24 00:59 Asia/Novosibirsk
(2026-09-23 17:59 UTC).

Question: how can public-proxy verification gate a tag without requiring that tag
to exist before its own PR merges?

Alternatives: require GA consumption in PR; skip public verification; separate
PR/preflight, RC post-tag, and GA post-tag closure gates.

Decision: PR/preflight runs a local clean consumer with no `replace`; the immutable
RC is then verified through the public proxy and checksum database; GA may target
only the same successful RC commit; and a final GA public-consumer check closes
issue #123.

Rationale: every externally visible artifact is tested while no workflow depends
on a not-yet-existing tag.

## Decision 8: tag immutability is a repository policy gate

Question: can a release workflow alone guarantee that a published module tag is
immutable?

Alternatives: trust convention; rely only on proxy checksums; verify a repository
ruleset protecting the nested-module tag namespace and minimize workflow rights.

Decision: RC and GA gates verify repository policy rejects update/deletion of
`backend/libs/runtime-go/v*`, and release jobs use least-privilege permissions.
Missing or unverifiable protection blocks publication as an external policy
failure. Proxy and checksum-database verification remains independently required.
Repository administrators remain an explicit governance boundary rather than an
attacker the workflow can cryptographically constrain.

Rationale: workflow correctness cannot prevent a privileged force-update by
itself; immutability needs both repository enforcement and public content proofs.
