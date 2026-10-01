# Runtime v2 canonical resolver: interaction flow

Status: interaction flow approved by @evilguest, 2026-10-01.

The existing `resolver.Resolution` changes to carry
`CanonicalResolvedExtensionIdentity`. There is one current resolver registry,
manager, result, and cache-record codec, not parallel legacy and canonical
APIs. There is no compatibility requirement for consumers of intermediate
v0.x releases, so version 0.5.0 may break source compatibility. Previously
published tags remain immutable.

The in-repository workspace-file provider and SQLite cache adapter use the
old resolver result today. They migrate with the contract and remain tested.
No CLI, HTTP API, database table, or default engine prepare-path change is
proposed. Issue #146 must be revised to remove its source-compatibility and
canonical-file-provider out-of-scope clauses before implementation.

## Resolution

1. The caller supplies an extension declaration and workspace. The registry
   selects a provider by role, owner, kind, and specification schema and
   rejects duplicate descriptors.
2. The provider normalizes the declaration. The manager derives a key from
   workspace scope, cache-record version, descriptor and semantic version, and
   normalized declaration. Declaration spelling and workspace paths do not
   enter the resulting identity.
3. On a hit, the record decoder uses the selected provider's
   `ExtensionIdentitySchema` to reconstruct each field from its original kind
   and complete value. It rejects incompatible versions, duplicate/unknown
   members, invalid or oversized values, checksum/key mismatches, and records
   from the old cache format. The provider validates identity and evidence
   before revalidation.
4. `CURRENT` retains the cached canonical identity, updates only freshness
   evidence, and durably stores the refreshed record. It calls neither fresh
   `Resolve` nor `Acquire`.
5. `STALE` and `UNKNOWN` remain distinct and observable. Both cause fresh
   resolution, followed by provider validation and storage. A failed resolve
   preserves the prior status and reason without returning a partial result.
   Miss/corruption/incompatible version causes fresh resolution; cancellation,
   permission, and ordinary I/O failures do not become misses.
6. `Acquire` remains a separate explicit provider call returning a physical
   artifact outside identity. `ResolveCurrent` never acquires an artifact.

## Durable result

The new record version, provisionally `sqlrs.resolution-cache.canonical.v1`,
contains the complete typed field payload, its kind and disclosure, the
provider/kind/identity schema, the canonical fingerprint, provider-owned
evidence, and key/provenance metadata. Text stays text; a canonical value
stores its exact envelope rather than only its digest; a secret reference
stores its non-secret provider/identifier/version. The schema-bound builder
reconstructs the identity, then the decoder checks canonical bytes and
fingerprint. The old string-only record is incompatible, never converted.

Records live in an engine-owned directory outside the workspace. Their
checksum detects accidental corruption, not a malicious directory owner.
Writes reuse the established temporary-file, sync, atomic-replace, and
directory-sync sequence. A failed durability step is reported; no partial
record or restart-safe success is claimed. The SQLite adapter uses the same
new record codec in its existing table and treats old records as incompatible.

## Publication order

The implementation PR prepares source, tests, external-consumer checks, and
release notes for v0.5.0. Issue #147 is complete only after merge, RC and GA
tags on the same reviewed commit, and successful public Go module
proxy/checksum-database verification. The PR must not claim that a release
already exists.
