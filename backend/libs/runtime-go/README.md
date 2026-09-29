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
