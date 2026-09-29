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
- `sqlrs.runtime.v2` and `sqlrs.runtime.v2.canonical.v1` retain their published
  semantics. Existing fingerprints and StateIDs are not reinterpreted or
  rehashed, and no migration or default-runtime cutover is performed.
- Conformance bundle schema:
  `sqlrs.runtime.conformance.bundle-schema.v1`.
- Conformance bundle: `runtime-v2-canonical-v1.1`.
- Manifest digest:
  `sha256:4009154c578097c86d42aa6f457208a2c84f7d70a86c696ad4ae37b55a78edb4`.
- The conformance vectors, bundle version, and detached manifest digest are
  unchanged from v0.3.0.

The exact source commit, module tag, checksums, license evidence, and generated
provenance are bound by the content-addressed release attestation rather than by
this source-controlled note.
