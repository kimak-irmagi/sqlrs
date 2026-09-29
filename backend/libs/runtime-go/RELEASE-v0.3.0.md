# Runtime Go v0.3.0

This release adds the `sqlrs.runtime.v2.canonical.v1` semantic revision while
preserving the published `sqlrs.runtime.v2` v0.2.0 contract unchanged.

- Canonical values use typed trees, five separated hash domains, typed identity
  fields, schema-bound builders, strict recomputing envelopes, and safe/internal
  explanations.
- Migration is explicit side-by-side dispatch followed by re-resolution through
  a selected canonical schema. Legacy fingerprints and StateIDs are never
  reinterpreted or upgraded.
- The public resolver exposes the opaque, v0.2-wire-compatible `CacheRecord`
  codec and deterministic cache matching without changing legacy identities.
- The opt-in `composition` package provides strict, deterministic transform and
  recipe alias expansion without connecting aliases to the current engine path.
- Alias document schema: `sqlrs.runtime.v2.aliases.v1`.
- Expansion trace schema: `sqlrs.runtime.v2.alias-expansion-trace.v1`.
- Resolution-cache schema: `sqlrs.resolution-cache.v1` (v0.2 wire compatible).
- Conformance bundle schema: `sqlrs.runtime.conformance.bundle-schema.v1`.
- Conformance bundle: `runtime-v2-canonical-v1.1`.
- Manifest digest:
  `sha256:4009154c578097c86d42aa6f457208a2c84f7d70a86c696ad4ae37b55a78edb4`.

Existing `v0.1.1-rc.6` product state, cache, queue, and alias data remain legacy
data. This module does not automatically migrate, reinterpret, or classify it;
the executable CLI compatibility and `legacy_only` classification work remains
a separate integration step.

RC and GA source identity is recorded in a generated content-addressed release
attestation rather than in this source-controlled note.
