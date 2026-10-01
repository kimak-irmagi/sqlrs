# Runtime Go v0.5.0

This release changes the current Go resolver API in place. `resolver.Resolution`
now carries a schema-bound `CanonicalResolvedExtensionIdentity`, and provider
registration and cache operations require an `ExtensionIdentitySchema`. It is a
breaking source change for intermediate v0.x users; no parallel legacy resolver
API is maintained.

- The workspace-file reference provider and local-engine SQLite adapter use the
  canonical result. The directory cache and SQLite table retain their storage
  mechanisms, but cache records use `sqlrs.resolution-cache.canonical.v1`.
- The old `sqlrs.resolution-cache.v1` string-only cache record is incompatible
  and is rejected rather than migrated or reinterpreted. The published
  `sqlrs.runtime.v2` v0.2.0 contract remains historical release evidence; its
  immutable tags and golden vectors are unchanged.
- The `sqlrs.runtime.v2.canonical.v1` semantic encoding, fingerprints, and
  StateIDs remain byte-stable. The cache stores complete typed identity fields,
  commitments, and canonical bytes. Provider evidence remains separate from
  logical identity. The checksum detects corruption, not malicious rewrite by
  a trusted cache owner.
- Revalidation still distinguishes `CURRENT`, `STALE`, and `UNKNOWN`.
  Acquisition remains explicit and is never performed by `ResolveCurrent`.
- The fixed workspace-file schema is exposed through `schemas/sqlrs`; generic
  schema authorship remains isolated in `schemaauthor`.

Unchanged contract markers: alias document schema
`sqlrs.runtime.v2.aliases.v1`, expansion trace schema
`sqlrs.runtime.v2.alias-expansion-trace.v1`, conformance bundle schema
`sqlrs.runtime.conformance.bundle-schema.v1`, and conformance bundle
`runtime-v2-canonical-v1.1`. The existing `v0.1.1-rc.6` product state, queue,
and alias data remain legacy data; this module does not migrate them.

RC and GA tags must identify the same reviewed post-merge commit. The exact
source commit, module checksums, dependency inventory, license evidence, and
generated provenance are bound by the content-addressed release attestation.
