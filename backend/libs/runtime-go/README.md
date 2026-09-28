# Runtime v2 Go module

This directory is the independent Go module
`github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go`. It defines the
engine-neutral `sqlrs.runtime.v2` semantic contract and, starting with the
published v0.2 line, versioned declarations and reusable resolver infrastructure.
All new APIs remain opt-in; the current engine does not adopt them implicitly.

## Version policy

Module tags use the repository-relative prefix `backend/libs/runtime-go/v*`.
The published `v0.1.0` is immutable, retracted, and superseded because it
predates the versioned declaration and resolver boundary. Do not select it for
new dependencies.

Published `v0.2.0` is the first recommended external dependency. The approved
#110 persistence design adds an opaque, wire-compatible resolver `CacheRecord`
API in additive `v0.3.0`; it does not change Runtime v2 identity domains or
StateIDs. External consumers should pin immutable tags and never use a local
`replace` as a production dependency.

Architecture contracts are documented in the
[Runtime v2 index](../../../docs/architecture/README.md).

Issue #109 adds the opt-in `composition` subpackage for deterministic
transform/recipe alias expansion. The implementation is present in this module
but is not yet available from the immutable published versions above; it does
not change the current engine path. See the
[composition structure](../../../docs/architecture/runtime-v2-composition-structure.md).
