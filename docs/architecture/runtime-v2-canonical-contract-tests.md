# Runtime v2 canonical v1: test design

Status: approved for issues #130 and #131 on 2026-09-27 at 23:31
Asia/Novosibirsk (16:31 UTC), after critical review and revision.

This plan verifies the approved
[canonical-v1 flow](runtime-v2-canonical-contract-flow.md),
[component structure](runtime-v2-canonical-contract-structure.md), and
[conformance-bundle schema](runtime-v2-conformance-bundle-schema.md). Stable test
IDs map requirements to executable evidence. Existing v0.2.0 suites remain
immutable legacy-v2 evidence and are never regenerated with canonical-v1 code.

## 1. Evidence model and oracles

Evidence runs at the stage where its dependencies exist:

| Stage | Available evidence | Gate result |
| --- | --- | --- |
| PR | source, tests, embedded draft bundle, merge base | deterministic unit/public/API, architecture, compatibility, bundle-policy checks |
| PR fuzz | bounded-time fuzz jobs and checked-in seed corpus | defect discovery; never the sole proof of a resource bound |
| RC | immutable RC tag, public proxy artifact | clean consumer, proxy zip/metadata, bundle and attestation-candidate checks |
| GA | immutable GA tag, checksum database, release asset | same-commit, checksum, final attestation and consumer checks |
| post-GA | successful GA evidence | issue #131 closure only; not a PR test |

The oracle policy is:

- **OR01 — locked known-answer vectors (PR):** reviewed input and exact preimage,
  digest, StateID, endpoint, entry digest, and detached-digest constants are
  checked in. Tests cannot update them.
- **OR02 — independent reference verifier (PR/RC):** a read-only Node
  implementation and the Go implementation independently recompute the same
  vectors. They share no generated encoder code or expected-output generator.
- **OR03 — vector-change policy (PR):** any offline generator output is review
  material only. Accepting it requires human review, a new bundle version, and
  the immutable-bundle policy check; CI never rewrites expected files.
- **OR04 — platform matrix (PR):** deterministic tests run on Linux and Windows;
  integer conversion/compile tests include `GOARCH=386`, while race tests run on
  a supported 64-bit runner.
- **OR05 — traceability (PR):** each implementation test/vector records one or
  more IDs below. An ID is complete only when every stated assertion exists;
  coverage tags alone do not satisfy it.

## 2. Canonical values and measurable budgets

- **CV01 — primitive known answers (PR):** null and representative UTF-8 strings
  assert exact node bytes, framed identity-value preimage, digest, and token.
- **CV02 — composite known answers (PR):** nested list/map/set fixtures assert
  exact count/length framing and complete canonical bytes, not only digests.
- **CV03 — deterministic ordering (PR):** exhaust every permutation up to five
  distinct map/set members and property-test larger seeded collections; all
  permutations produce the same bytes. Ordered lists remain order-sensitive.
- **CV04 — raw Unicode semantics (PR):** NFC/NFD-equivalent strings remain
  byte-distinct, no normalization occurs, valid NUL/non-ASCII values round-trip,
  and invalid UTF-8 is rejected rather than replaced.
- **CV05 — duplicate semantics (PR):** duplicate source map keys fail before
  sorting; duplicate encoded set members fail. Because keys are unnormalized
  strings, there is no separate implied normalization-collision class.
- **CV06 — token grammar and use boundary (PR):** exact lowercase SHA-256 syntax
  succeeds; uppercase, whitespace, truncation, extra separators/suffixes, and
  unsupported algorithms fail. Parsed tokens support comparison/display but no
  exported API can use one alone to construct a verified identity field.
- **CV07 — malformed binary tree (PR):** unknown tags, invalid null payload,
  overflow, truncation, trailing bytes, wrong count, duplicate/non-canonical
  collection order, and malformed nesting return stable code/path and no value.
- **CV08 — zero values and defensive copies (PR):** zero `CanonicalValue` and
  token values are invalid; constructors and byte/tree accessors isolate caller
  mutation.

Boundary tests use root depth 1, per-collection members, whole-tree node/byte
budgets, and the stricter key-byte rule defined by the flow:

- **LM01 — each scalar boundary (PR):** depth, total nodes, list/map/set members,
  string bytes, key bytes, canonical bytes, and decoded-envelope bytes each cover
  `limit-1`, `limit`, and `limit+1` with exact error code/path.
- **LM02 — combined budgets (PR):** fixtures independently exhaust nodes before
  bytes, bytes before nodes, per-collection members inside a nested tree, and key
  versus ordinary-string limits; the first documented validation phase wins.
- **LM03 — pre-allocation plan (PR):** same-package tests call the non-allocating
  production `allocationPlan` helpers with hostile u32/u64 counts, `MaxInt`/32-bit
  conversion edges, multiplication/addition overflow, and sort cardinality.
  Rejection occurs before a proportional `make`, recursion, or sort; no injected
  allocator or alternate test path is permitted.
- **LM04 — real decoder adversarial corpus (PR):** the public decoder handles
  maximum-valid and compact over-limit wide/deep/duplicate-heavy inputs within
  the fixed 4 MiB input cap with the exact expected value or code/path and no
  panic. Same-package plan counters prove a rejected plan never enters
  materialization or sorting. No machine-independent allocation-count or timing
  claim is made.
- **FZ01 — canonical fuzz invariants (PR fuzz):** token/tree/framing seeds fuzz
  no-panic, atomic failure, and successful decode→encode byte equality. Fuzzing
  does not claim to prove memory or time limits.

## 3. Typed fields, identities, and lineage

- **ID01 — kind separation (PR):** text, canonical-value, and secret-reference
  payloads assert exact kind tags and distinct field contributions; kind appears
  in both contribution and field-set preimages.
- **ID02 — no token sniffing (PR):** text beginning with `civ1:` remains text;
  canonical-value fields require a complete `CanonicalValue` tree.
- **ID03 — immutable opaque values (PR):** zero values fail and mutations of all
  constructor inputs/accessor results cannot change fields, references,
  identities, descriptors, envelopes, observations, or lineages.
- **ID04 — exact identity known answers (PR):** factory, transform, resolved
  extension, root state, and derived state assert every tag/length, field
  contribution, canonical preimage, domain, digest, StateID, and endpoint.
- **ID05 — sensitivity matrix (PR):** schema, provider/owner, semantic kind,
  identity schema, field name/kind/payload, parent, transform, and transform
  order each change the specified projection. Field permutations and operational
  observations do not.
- **ID06 — five-domain separation (PR):** equal payload bytes under
  identity-value, factory-state, transform, resolved-extension, and state domains
  produce five distinct digests; wrong kind/domain pairs are rejected.
- **ID07 — secret-reference semantics (PR):** provider, opaque identifier, and
  immutable version are required and independently identity-bearing. Supported
  builders expose no raw-secret channel. Tests claim only typed-channel and
  enumerated-sink safety; arbitrary text cannot be classified as secret by the
  generic trust boundary.
- **ID08 — revision isolation (PR):** legacy accepts only `sqlrs.runtime.v2`,
  canonical accepts only `sqlrs.runtime.v2.canonical.v1`; cross-revision decode,
  descriptor use, anchor use, and rehash fail explicitly.
- **LN01 — recipe lineage (PR):** factory-only, one-transform, and ordered
  multi-transform recipes recompute roots, every intermediate/final StateID, and
  endpoint; transform reordering changes lineage.
- **LN02 — relative lineage (PR):** verified factory/derived anchors with
  zero/one/many transforms recompute every link. Bare, token-only, unverified,
  legacy, or mismatched anchors fail before construction.

## 4. Composition, schema trust, and architecture

- **CO01 — canonical extension identity (PR):** exact extension vectors cover
  owner, semantic kind, identity schema, typed fields, and domain.
- **CO02 — role completeness (PR):** input, execution-environment, and deployment
  bindings enforce declaration count, position, role, owner, and kind; missing,
  extra, duplicate, reordered, unknown, cross-role, and transform-deployment
  cases fail atomically.
- **CO03 — legacy API freeze (PR):** all v0.2.0 `ExtensionFingerprint` and
  `ComposeResolvedFields` known answers remain unchanged; canonical functions
  accept only distinct canonical-v1 types.
- **BU01 — authored-schema validation (PR):** duplicate/unstable names, invalid
  role/kind/disclosure, impossible requiredness, and inconsistent observation
  schema fail; valid schemas and accessors are immutable.
- **BU02 — transactional builders (PR):** every required field/role is enforced;
  rejected methods leave state unchanged; successful build seals the single-use
  builder; identity, extension, observation, and executor-private channels cannot
  be converted or interchanged.
- **BU03 — supported production builders (PR):** for every checked-in supported
  schema, semantic input changes identity while timestamps, paths, runtime/job
  IDs, cache/checkpoint details, acquisition observations, and execution-only
  credentials do not enter canonical bytes.
- **BU04 — AST/package-graph boundary (PR policy):** the exact allowlist and a
  production dependency through an approved schema facade are accepted. Fixtures
  with a direct/bypassing graph path, alias, constructor reference, generic type
  in an exported signature, wrapper, or re-export are rejected. All repository
  production packages and graph paths are enumerated, not sampled.

## 5. Integrity, errors, and disclosure

- **IN01 — descriptor matrix (PR):** each identity kind has exactly one allowed
  schema/domain/algorithm and provider/owner metadata shape; all wrong pairs and
  unknown values fail.
- **IN02 — full-subject recomputation (PR):** canonical-value, factory,
  transform, extension, recipe, and relative envelopes recompute tree tokens,
  field contributions, identity/state digests, links, and endpoints. Omitting a
  canonical-value AST fails; a token alone never yields verified status.
- **IN03 — single-fault tampering (PR):** separate fixtures mutate each descriptor
  field, public/protected payload, contribution, digest, parent, step, StateID,
  endpoint, or discriminator and assert the specified code/path.
- **IN04 — validation precedence (PR):** multi-fault fixtures cover only the
  documented phase order: byte budget/syntax → shape/discriminator → limits →
  descriptor → payload commitment → identity digest → lineage/endpoint.
- **IN05 — strict semantic JSON (PR):** duplicate/unknown members, invalid UTF-8,
  null where forbidden, trailing values, and invalid shapes fail atomically;
  insignificant whitespace and object-member permutations succeed. Tests do not
  invent a canonical JSON order.
- **IN06 — no standalone trust (PR):** descriptors, parsed tokens, explanations,
  and bare anchors have no decoder/conversion that produces a trusted envelope,
  identity, or canonical StateID.
- **EX01 — safe one-way projection (PR):** protected payloads, opaque secret IDs,
  and their per-field commitments are absent and replaced by typed markers;
  aggregate digest/StateIDs/links match the verified source envelope. Safe output
  cannot be decoded or supplied as proof of a redacted value.
- **EX02 — internal authorization (PR):** allow with live context reveals only
  permitted protected non-secret values/reference components. Nil authorizer,
  deny, returned error, and pre-canceled context return the zero projection and
  no partial serialization.
- **EX03 — runtime canary sinks (PR):** a randomly generated, non-checked-in
  canary is supplied through executor-private credential input and a wrong-kind
  request to a supported schema. Builder errors, `%v`/`%+v` of returned errors,
  captured logs, produced envelopes/explanations, and bundle output do not contain
  it. Generic text values are deliberately outside this claim because the public
  schema-author trust boundary cannot classify arbitrary text as a secret.

## 6. Conformance bundle and repository policy

- **CB01 — detached-digest known answer (PR):** exact domain/schema framing,
  UTF-8 entry order, u32 count, u64 sizes, raw entry hashes, and final digest are
  independently recomputed; manifest/digest files are excluded.
- **CB02 — in-memory path matrix (PR):** empty, `.`, `..`, repeated separators,
  absolute/UNC, drive-prefixed, backslash, control-character, invalid UTF-8,
  uppercase, non-ASCII, invalid segment, duplicate, unsorted, reserved
  manifest/digest, missing, extra, and unlisted paths fail before vectors.
- **CB03 — filesystem object policy (PR policy):** a no-follow walk accepts only
  regular files and rejects symlinks, junctions/reparse points, directories in
  entry positions, and other non-regular objects. The in-memory parser makes no
  symlink assertion.
- **CB04 — file bytes and entry integrity (PR):** vector/manifest files require
  UTF-8, LF, and one terminal newline; wrong size, uppercase/malformed digest,
  changed bytes, CRLF, missing newline, missing file, and extra file fail. The
  detached digest file must be exactly one `sha256:<lowercase-hex>\n` line.
- **CB05 — manifest metadata binding (PR):** changing bundle-schema, bundle, or
  semantic version against the expected descriptor fails even when the detached
  digest still matches entries. The current loader uses exact compiled constants.
- **CB06 — vector schema (PR):** vector/case/relation IDs obey grammar, global
  uniqueness, and order; tags are unique/sorted; operation, expected shape,
  error code/path, comparison, projection, and referenced case IDs are valid.
  Mis-tagged cases do not satisfy required coverage.
- **CB07 — semantic coverage (PR):** vectors exercise canonical values/limits,
  five domains, composition, recipe/relative lineage, same-resolution equality,
  identity changes, diagnostics-only changes, secret revision, redaction,
  tampering, builder metadata exclusion, and legacy non-reinterpretation.
- **CB08 — cross-case relations (PR):** required equality/inequality projections
  are recomputed from referenced cases; recorded digests are never compared only
  to copies of themselves.
- **CB09 — bundle immutability (PR policy/RC):** PR compares with merge base and
  requires a new directory/version for any existing content/path change; RC
  repeats against the latest published module tag. Initial unpublished content
  has no false historical baseline.
- **CB10 — API ownership and concurrency (PR):** manifest/files/accessors are
  defensive copies; repeated and concurrent read/verify calls are deterministic
  and race-free under `go test -race`.

## 7. Legacy compatibility and explicit migration

- **CP01 — legacy golden lock (PR):** existing v0.2.0 binary/JSON goldens,
  fingerprints, StateIDs, endpoints, and public tests pass byte-for-byte without
  modification.
- **CP02 — side-by-side consumer (PR):** one external test module holds both
  revisions and dispatches using the explicit discriminator in consumer code;
  neither library decoder guesses, upgrades, or falls back.
- **CP03 — re-resolution migration boundary (PR):** the existing declaration and
  resolver flow plus an explicitly selected supported canonical schema builds a
  new canonical value. There is no new generic migration API, and no API accepts
  a legacy fingerprint/StateID as sufficient canonical input.

## 8. Release gates

- **RL01 — static release material (PR):** checked-in notes name canonical schema,
  immutable legacy separation, side-by-side/re-resolution boundary,
  bundle-schema/bundle versions, and manifest digest. They contain no placeholder
  or self-referential source SHA.
- **RL02 — source-tree clean consumer (PR):** a temporary module outside
  `go.work`, using no `replace`, consumes a staged module zip through a temporary
  file-backed `GOPROXY` and exercises canonical values, a supported builder,
  strict envelope, safe explain, and bundle verification. This is not presented
  as an internet public-proxy test.
- **RL03 — RC public consumer (RC):** the immutable `v0.3.0-rc.N` tag resolves
  through the public proxy; zip/module metadata/checksum and embedded bundle
  match the tagged commit, and the clean consumer passes.
- **RL04 — generated attestation (RC/GA):** automation generates a
  content-addressed, non-overwriting attestation asset from the tag rather than
  modifying source. It binds module/tag, exact source SHA, canonical schema,
  bundle-schema/bundle versions, and manifest digest. Tests assert exact member
  order, UTF-8/LF/terminal newline, companion digest line, digest-bearing asset
  name, no extras, tag/proxy comparison, and every-field tampering rejection.
- **RL05 — same-commit GA (GA):** exact `backend/libs/runtime-go/v0.3.0` and an
  already verified RC tag point to the same completion commit; public proxy and
  checksum database serve matching content and the final attestation asset.
- **RL06 — closure (post-GA):** issue #131 closes only after RL03–RL05 evidence
  succeeds. A pre-tag failure publishes nothing. A failure discovered after an
  RC/GA tag exists leaves that tag immutable and blocks promotion/closure; a
  transient verification may be rerun against the same tag, never by moving it.

## 9. Existing-test contradiction and coverage review

Review completed 2026-09-27 after approval. The existing module, resolver,
architecture, workflow, golden, external-consumer, and release tests contain no
requirement contradiction with this plan. Their `sqlrs.runtime.v2` constants,
domains, bytes, and v0.2.0 release assertions are legacy-v2 evidence required by
CP01/CO03 and remain unchanged.

The review found additive harness gaps, not conflicting expectations: the local
consumer currently uses a `replace`, workflows verify only legacy goldens and
legacy public API, and no canonical-v1 bundle/reference verifier exists yet.
RL02 replaces that local-only consumer path with a temporary file-backed proxy;
legacy consumer assertions stay and canonical assertions are added. Existing Go
suites, both legacy Node golden verifiers, and the release-workflow contract test
passed before test implementation.

After implementation, coverage is measured according to the repository policy.
Coverage work is requirement-driven: uncovered branches are mapped to an
approved requirement or treated as candidate dead code; it is not satisfied by
tests of undocumented implementation details.
