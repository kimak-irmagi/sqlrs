# Runtime v2 canonical v1 public-contract decisions

Conversation timestamp: 2026-09-27 Asia/Novosibirsk.
GitHub user ID: @evilguest (41718235). Agent: OpenAI Codex (GPT-5).
Status: approved for issues #130 and #131 on 2026-09-27 at 22:27
Asia/Novosibirsk (15:27 UTC).

## Decision 1: side-by-side semantic revision

Question: should typed fields reinterpret existing `sqlrs.runtime.v2` values or
define a new semantic revision?

Alternatives: silently change v2 bytes; migrate legacy values during decode;
retain legacy-v2 unchanged and introduce `sqlrs.runtime.v2.canonical.v1` with
separate types/decoders/domains.

Decision: use the separate canonical-v1 revision. `SchemaVersion`
remains legacy for source compatibility; new APIs use `CanonicalSchemaVersion`.
Cross-revision decode and rehash are errors. Migration is explicit re-resolution
from source declarations.

Rationale: immutable v0.2.0 fingerprints and StateIDs retain one meaning while
typed canonical values can have an unambiguous new encoding.

## Decision 2: canonical-value digest tokens

Question: how should structured provider values enter typed fields?

Alternatives: inline JSON; Base64 the complete tree; hash a type-tagged framed
tree in a dedicated domain and use a versioned digest token.

Decision: canonicalize null/string/list/map/set trees, hash
`encodeString(sqlrs.runtime.v2/identity-value) || encodeBytes(root-node)`, and
represent the result as `civ1:sha256:<hex>`. Map input is an entry slice; maps
and sets reject canonical duplicates and sort by encoded bytes. Export depth,
node, member, string/key, total-byte, and envelope budgets checked before
allocation/sorting.

Rationale: the token is compact and delimiter-safe; the full tree remains
available in integrity envelopes and conformance vectors for recomputation.

## Decision 3: typed fields, commitments, and secret references

Question: how can field kinds remain unambiguous, explanations redact protected
identifiers, and credential revisions affect identity without hashing secrets?

Alternatives: keep string-only fields; encode raw payloads directly and accept
that redacted explanations cannot recompute; use typed payloads plus per-field
commitments and a non-secret reference type.

Decision: opaque fields have kind `text`, `canonical-value`, or
`secret-reference`. Text is never token-sniffed. Secret references contain only
provider, opaque stable ID, and immutable version. The identity field set hashes
framed name/kind/payload into a contribution commitment and hashes ordered
name/kind/commitment entries in the identity domain.

Rationale: kind confusion is impossible; safe explanations can recompute the
identity from commitments after redaction; a secret version changes identity;
raw secret material has no model type and is never unkeyed-hashed.

## Decision 4: explicit schema-author trust boundary

Question: how should generic extensibility coexist with enforceable production
composition rules?

Alternatives: field-name denylist; expose generic constructors everywhere;
isolate generic authoring in `schemaauthor`, enforce repository imports, and make
production builders schema-bound.

Decision: `schemaauthor` is public but explicitly trusted. Repository
architecture checks permit it only in approved schema packages/conformance.
Schemas declare field role, kind, requiredness, and disclosure. Production
adapters use single-use schema-bound builders with separate identity,
operational-observation, and extension channels.

Rationale: an external malicious schema author remains possible and documented,
while every supported repository path becomes structurally testable without
pretending a blacklist proves semantics.

## Decision 5: role-complete canonical extension composition

Question: how are legacy extension APIs preserved while canonical-v1 proves no
declared extension was omitted?

Alternatives: mutate existing functions; accept arbitrary binding slices;
freeze legacy functions and add type-separated canonical-v1 composition that
compares declarations with role-specific results.

Decision: legacy `ExtensionFingerprint` and `ComposeResolvedFields`
retain old signatures/bytes. Canonical-v1 uses separately typed functions and
builders. Inputs are positional/count-preserving; environment/deployment
presence and owner/kind must match; binding names are fixed.

Rationale: there is no silent legacy reinterpretation, and completeness becomes
a public invariant.

## Decision 6: recomputing envelopes and two disclosure profiles

Question: how should explain/persistence transport detect tampering and redact
protected identifiers?

Alternatives: persist bare digests; strict full envelopes only; expose protected
per-field commitments in safe output; verified full envelopes plus one-way safe
and authorized internal projections.

Decision: `FingerprintEnvelope` carries descriptor plus typed subject
and recomputes all values/digests. State/recipe/relative envelopes recompute
links and endpoints. Supported identity construction requires a full
`CanonicalValue` tree; parsed tokens cannot construct fields. Safe explain
replaces protected payloads, opaque secret identifiers, and their per-field
commitments with typed redaction markers. It is a one-way projection, not a
trust-producing proof. Internal explain requires an explicit authorization
callback and may disclose protected non-secret values. Neither profile contains
raw secrets.

Rationale: full transport is self-verifying, token-only inputs cannot masquerade
as recomputed subjects, safe output does not add a low-entropy guessing oracle,
and authorization remains an explicit integration responsibility.

## Decision 7: manifest-backed multi-file conformance bundle

Question: how can one public bundle be independently packaged and verified?

Alternatives: one large JSON file; module-cache testdata; an embedded directory
with strict manifest, detached digest, and independently parsed vectors.

Decision: use bundle schema
`sqlrs.runtime.conformance.bundle-schema.v1`, bundle version
`runtime-v2-canonical-v1.1`, fixed digest domain
`sqlrs.runtime.conformance.bundle.v1`, lexicographically ordered safe paths, and
the exact size/content-digest preimage required by issue #130. Go, Node, fresh
processes, and the public consumer verify the same files. Published bundle
content is immutable; any vector change creates a new bundle version.

The public in-memory parser binds an expected bundle-schema/bundle/semantic
descriptor; filesystem symlinks are handled by a separate no-follow repository
check. The Node verifier is read-only and shares no generated encoder or expected
generator with Go. Pre-merge and release checks compare against merge base and
the latest published tag respectively.

Rationale: packaging, corruption, traversal, partial-bundle, and semantic errors
are independently detectable without copied expected hashes.

## Decision 8: one PR and immutable v0.3.0 release

Question: which module version and release sequence should publish canonical-v1?

Alternatives: patch v0.2.x; wait for v1.0.0; publish the additive API/new semantic
revision as the next pre-1.0 minor.

Decision: merge one PR from
`feature/runtime-v2-canonical-contract-130-131`, record/preflight its exact
completion SHA on `main`, and publish `v0.3.0-rc.1` then `v0.3.0` from that same
SHA. Release evidence names canonical schema, legacy separation, migration
boundary, bundle-schema/bundle versions, manifest digest, and source SHA.
Checked-in notes contain only static facts; a generated content-addressed,
non-overwriting release attestation binds the dynamic source SHA and tuple after
tagging and is verified against the protected tag/proxy.

Rationale: Go consumers receive an additive module feature release, while the
semantic discriminator—not the module version—prevents any legacy identity
reinterpretation. Issue #131 remains open until public-proxy verification passes.

## Decision 9: executable evidence boundaries after test-plan review

Conversation timestamp: 2026-09-27 22:49 Asia/Novosibirsk (15:49 UTC).
GitHub user ID: @evilguest (41718235). Agent: OpenAI Codex (GPT-5).

Question: how should the test plan avoid circular or untestable guarantees while
preserving the security, resource-bound, architecture, and release requirements?

Alternatives: keep broad test groups and treat release checks as unit tests; use
allocation hooks and text search as proxies; or split deterministic PR evidence,
independent reference verification, repository policy, RC, GA, and closure gates
with explicit measurable oracles.

Decision: use the staged evidence model. Production parsing first builds a
bounded allocation plan before allocation/sort; deterministic adversarial tests
exercise that path, while fuzzing is limited to panic/round-trip properties.
Architecture enforcement uses AST plus package graph, permits transitive use only
through an approved schema facade, and rejects bypasses and re-exports.
Structured errors follow documented validation phases and single-fault fixtures.
Release/proxy/tag checks execute only at their actual RC/GA stages. Secret tests
claim only typed-channel and enumerated-sink guarantees and use runtime-generated
canaries, never checked-in secret fixtures.

Rationale: every acceptance statement now has an observable pass/fail mechanism,
without claiming that fuzzing proves memory bounds, text search proves package
architecture, or a PR can verify a tag and public artifact that do not yet exist.

## Decision 10: approve the revised canonical-v1 test plan

Conversation timestamp: 2026-09-27 23:31 Asia/Novosibirsk (16:31 UTC).
GitHub user ID: @evilguest (41718235). Agent: OpenAI Codex (GPT-5).

Question: should the revised 63-ID evidence matrix proceed to contradiction
review and test implementation?

Alternatives: approve the revised matrix; request another revision; reject the
matrix and reopen the canonical-v1 evidence design.

Decision: approve the English/Russian test plan and proceed to the mandatory
review of existing module, resolver, architecture, golden, and release tests for
contradictions before changing tests.

Rationale: the revised plan separates PR/RC/GA evidence, uses measurable resource
and architecture checks, removes token-only and redacted-proof overclaims, and
defines stable independent oracles and error semantics.
