# Runtime v2 Go module

This directory is the independent Go module
`github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go`. It defines the
engine-neutral Runtime v2 contracts. Published `v0.2.0` defines the immutable
legacy discriminator `sqlrs.runtime.v2`, versioned declarations, and reusable
resolver infrastructure. PR #135 completed issues #130/#131 with a separate
`sqlrs.runtime.v2.canonical.v1` revision, typed identity fields, and a
manifest-backed conformance bundle. Those APIs were published in `v0.3.0` from
commit `6e60578c`. All new APIs remain opt-in; the current engine does not adopt
them implicitly.

## Version policy

Module tags use the repository-relative prefix `backend/libs/runtime-go/v*`.
The published `v0.1.0` is immutable, retracted, and superseded because it
predates the versioned declaration and resolver boundary. Do not select it for
new dependencies.

Version `v0.2.0` remains the immutable legacy-v2 release. Its values must be
decoded only as `sqlrs.runtime.v2` and never upgraded or rehashed in place. The
issue #110 persistence work adds an opaque, v0.2-wire-compatible resolver
`CacheRecord` API without changing those identity domains or StateIDs. Together
with the opt-in composition package below, these additions are available in
`v0.3.0`, whose exact release commit passed the conformance-bundle,
release-candidate, clean-consumer, and public-module-proxy gates. Consumers must
not pin an unreleased version or use a local `replace` as a production
dependency.

Architecture contracts are documented in the
[Runtime v2 index](../../../docs/architecture/README.md).

Issue #109 added the opt-in `composition` subpackage for deterministic
transform/recipe alias expansion. It is available in `v0.3.0` and does not
change the current engine path. See the
[composition structure](../../../docs/architecture/runtime-v2-composition-structure.md).

The `schemas/conformancev1` facade tracked by issues #138/#139 is available in
`v0.4.0`. It exposes a fixed canonical-v1 schema family for external contract
verification without granting generic `schemaauthor` authority. It is not a
production provider schema. See the
[canonical-v1 structure](../../../docs/architecture/runtime-v2-canonical-contract-structure.md#schema-safe-external-conformance-facade).

Issues #146/#147 are published in v0.5.0. That release changes the existing
`resolver.Resolution` to carry `CanonicalResolvedExtensionIdentity` and
introduces a new versioned cache record. It does not add a parallel resolver
API, migrate old cache records, or reinterpret previously published identities.
The workspace-file reference provider and the in-repository SQLite adapter
migrate with this breaking source change. See the
[canonical resolver flow](../../../docs/architecture/runtime-v2-canonical-resolver-flow.md)
and [structure](../../../docs/architecture/runtime-v2-canonical-resolver-structure.md).

The CLI owns alias YAML decoding and legacy compatibility. Its recursive
expansion uses the published `composition` package and produces a versioned
`RecipeDeclaration`. A future engine request carries that declaration and the
required source inputs. The proposed public alias package in
[issue #140](https://github.com/kimak-irmagi/sqlrs/issues/140) was closed without
publication; the Runtime Go module retains its standard-library-only dependency
boundary.

Issue [#150](https://github.com/kimak-irmagi/sqlrs/issues/150) adds the approved
[production execution-schema interaction boundary](../../../docs/architecture/runtime-v2-execution-schema-flow.md)
to the existing module, not an execution service or a second resolver result.
The [concrete API and component structure](../../../docs/architecture/runtime-v2-execution-schema-structure.md)
were approved on 2026-10-16. Implementation and tests are available in
[PR #152](https://github.com/kimak-irmagi/sqlrs/pull/152), merged as 892a5007 with
[successful post-merge CI](https://github.com/kimak-irmagi/sqlrs/actions/runs/38886272700).
The facade is published in [v0.6.0](https://github.com/kimak-irmagi/sqlrs/releases/tag/backend/libs/runtime-go/v0.6.0),
with completed publication evidence in [#151](https://github.com/kimak-irmagi/sqlrs/issues/151).
The [first coverage follow-up](../../../docs/architecture/runtime-v2-execution-schema-coverage-plan.md)
records historical measured results and CI follow-up. The [test inventory](../../../docs/architecture/runtime-v2-execution-schema-tests.md)
was separately approved on 2026-10-16; tests precede implementation.

For #151, direct publication of an already reviewed source commit was approved
on 2026-10-16. The approved reusable
[manual publication contract](../../../docs/architecture/runtime-go-module-publication.md)
separates release automation from the tagged library source and accepts release
notes outside that source. Its detailed structure/tests were approved and
implemented and verified by the completed v0.6.0 RC/final publication.
No version-specific readiness PR is required.

Issue [#156](https://github.com/kimak-irmagi/sqlrs/issues/156) adds the approved
[tool-neutral execution-transform API](../../../docs/architecture/runtime-v2-execution-transform.md):
one atomic `BuildExecutionTransform` using existing declaration, execution/input
bindings and composition result types. It does not reinterpret the PSQL schema,
add dummy SQL, or create another command/result representation. The approved
[tests](../../../docs/architecture/runtime-v2-execution-transform-tests.md),
implementation and standalone-consumer checks pass locally; source CI and merge
remain separate gates.
No released version contains this new function yet; its immutable publication
is separate in [#157](https://github.com/kimak-irmagi/sqlrs/issues/157), through
the existing release workflow without a version-only PR or separate worktree.
