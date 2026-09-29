# Runtime Go v0.4.0

This additive release publishes `schemas/conformancev1`, a fixed
canonical-v1 fixture facade for external contract verification. It is not a
production provider schema and does not grant generic `schemaauthor` authority.

- External consumers can construct representative factory, transform, and
  resolved-extension identities using role-specific builders and declaration
  helpers.
- The facade covers text, canonical-value, and secret-reference identity fields,
  typed operational observations, extension bindings, and safe/internal
  disclosure.
- The published `sqlrs.runtime.v2` v0.2.0 contract and
  `sqlrs.runtime.v2.canonical.v1` retain their semantics. Existing fingerprints
  and StateIDs are not reinterpreted or rehashed, and no migration or
  default-runtime cutover is performed.
- Alias document schema: `sqlrs.runtime.v2.aliases.v1`.
- Expansion trace schema: `sqlrs.runtime.v2.alias-expansion-trace.v1`.
- Resolution-cache schema: `sqlrs.resolution-cache.v1` (v0.2 wire compatible).
- Conformance bundle schema:
  `sqlrs.runtime.conformance.bundle-schema.v1`.
- Conformance bundle: `runtime-v2-canonical-v1.1`.
- Manifest digest:
  `sha256:4009154c578097c86d42aa6f457208a2c84f7d70a86c696ad4ae37b55a78edb4`.
- The conformance vectors, bundle version, and detached manifest digest are
  unchanged from v0.3.0.

Existing `v0.1.1-rc.6` product state, cache, queue, and alias data remain legacy
data. This module does not automatically migrate or reinterpret them.

The exact source commit, module tag, checksums, license evidence, and generated
provenance are bound by the content-addressed release attestation rather than by
this source-controlled note.
