# Runtime v2 Go module

This directory is the independent Go module
`github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go`. It defines the
engine-neutral `sqlrs.runtime.v2` semantic contract and, starting with the
planned v0.2 line, versioned declarations and reusable resolver infrastructure.
All new APIs remain opt-in; the current engine does not adopt them implicitly.

## Version policy

Module tags use the repository-relative prefix `backend/libs/runtime-go/v*`.
The published `v0.1.0` is immutable but superseded because it predates the
versioned declaration and resolver boundary. Do not select it for new
dependencies. It will be retracted by the next module release.

The first recommended external dependency will be `v0.2.0`, published only
after the same commit passes the release-candidate, clean-consumer, and public
module-proxy gates. Until that release exists, consumers should not pin an
unreleased version or use a local `replace` as a production dependency.

Architecture contracts are documented in the
[Runtime v2 index](../../../docs/architecture/README.md).
