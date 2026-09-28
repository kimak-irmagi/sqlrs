# Runtime v2 Go module

This directory is the independent Go module
`github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go`. It defines the
engine-neutral Runtime v2 contracts. Published `v0.2.0` defines the immutable
legacy discriminator `sqlrs.runtime.v2`, versioned declarations, and reusable
resolver infrastructure. Issues #130/#131 add a separate
`sqlrs.runtime.v2.canonical.v1` revision with typed identity fields and a
manifest-backed conformance bundle. All new APIs remain opt-in; the current
engine does not adopt them implicitly.

## Version policy

Module tags use the repository-relative prefix `backend/libs/runtime-go/v*`.
The published `v0.1.0` is immutable, retracted, and superseded because it
predates the versioned declaration and resolver boundary. Do not select it for
new dependencies.

Version `v0.2.0` remains the published legacy-v2 dependency. Its values must be
decoded only as `sqlrs.runtime.v2` and never upgraded or rehashed in place. The
#110 persistence work adds an opaque, v0.2-wire-compatible resolver
`CacheRecord` API without changing those identity domains or StateIDs. Together
with the opt-in composition package below, these additions are planned for
`v0.3.0`; that version is published only
after the exact release commit passes the conformance-bundle, release-candidate,
clean-consumer, and public-module-proxy gates. Consumers must not pin an
unreleased version or use a local `replace` as a production dependency.

Architecture contracts are documented in the
[Runtime v2 index](../../../docs/architecture/README.md).

Issue #109 adds the opt-in `composition` subpackage for deterministic
transform/recipe alias expansion. The implementation is present in this module
but is not yet available from the immutable published versions above; it does
not change the current engine path. See the
[composition structure](../../../docs/architecture/runtime-v2-composition-structure.md).
