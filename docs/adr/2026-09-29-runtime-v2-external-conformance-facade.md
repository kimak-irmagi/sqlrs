# Runtime v2 external conformance facade

- Conversation timestamp: 2026-09-29 Asia/Novosibirsk
- GitHub user ID: @evilguest (41718235)
- Agent name/version: OpenAI Codex / GPT-5
- Status: Accepted for `sqlrs#138` on 2026-09-29

## Decision 1: expose a fixed schema facade to external contract consumers

### Question discussed

How can an external consumer construct representative canonical-v1 factory,
transform, extension, secret-reference, and diagnostic cases without crossing
the generic schema-authoring trust boundary or copying a provider schema?

### Alternatives considered

1. Allow external contract tests to import `schemaauthor` directly.
2. Export prebuilt identity envelopes or require consumers to parse golden
   vector files.
3. Add a fixed schema-specific `schemas/conformancev1` facade that delegates to
   production builders but exposes no generic schema definition.

### Chosen solution

Choose alternative 3. The facade owns fixed factory, transform, and extension
schemas with public text, protected canonical-value, protected
secret-reference, and protected operational fields. It exports builder
constructors, kind-specific observation constructors, narrow role-specific
extension-declaration helpers, and stable constants for every schema identifier
and permitted fixture field.

### Brief rationale

Direct schema authoring would make the external test a second schema owner.
Prebuilt envelopes would verify decoding but not the supported construction
paths. A fixed facade exercises the real builders while keeping schema authority
and field classification in the public module.

## Decision 2: keep the facade non-production and non-extensible

### Question discussed

Should the conformance facade accept arbitrary provider, kind, field
definitions, or disclosure policy so it can serve as a general provider API?

### Alternatives considered

1. Export a configurable schema factory.
2. Export fixed identity schemas but allow arbitrary observation and declaration
   fields.
3. Export only the fixed vocabulary required to verify canonical-v1 behavior.

### Chosen solution

Choose alternative 3. The facade is documented as contract-verification API.
It exports no schema types or generic field definitions. Observation helpers
accept only the fixed operational vocabulary. Extension-declaration helpers
accept one non-secret reference string and create the fixed declaration field
themselves. Production providers continue to own separate approved packages
under `schemas/**`.

### Brief rationale

Extensibility would recreate `schemaauthor` under a less explicit name and
weaken the architecture boundary. A fixed vocabulary is sufficient for
external acceptance and makes accidental production use visible.

## Decision 3: version and enforce the fixture boundary

### Question discussed

How should the facade remain stable across future semantic revisions, and how
can the repository enforce that production code does not adopt a test schema?

### Alternatives considered

1. Keep an unversioned `schemas/conformance` package and update it to the latest
   semantic revision.
2. Version the package but rely only on documentation to discourage production
   imports.
3. Bind `schemas/conformancev1` permanently to canonical-v1 and reject imports
   from repository non-test Go files outside the facade itself.

### Chosen solution

Choose alternative 3. Exact identity, observation, and specification schema
identifiers are normative exported constants. Existing identifiers and builder
semantics are never repointed. An AST architecture test permits the facade in
tests and the external clean-consumer fixture but rejects repository production
imports.

### Brief rationale

An unversioned fixture would make future upgrades ambiguous. Documentation alone
cannot prevent accidental engine adoption. A frozen package path plus an
executable import boundary preserves deterministic external tests without
turning the fixture into a production provider.

## Decision 4: make programmatic validation deterministic and diagnostic-safe

### Question discussed

How should map-based observation helpers handle empty inputs, unsafe field
names, multiple invalid fields, byte limits, and public Runtime validation-error
compatibility?

### Alternatives considered

1. Delegate directly to `NewOperationalObservation` and inherit map iteration
   order and its combined value/limit result.
2. Reject every empty map/value and report every unknown key at a path containing
   the original key.
3. Define a facade-owned validation pass with sorted names, safe paths, explicit
   empty-value semantics, exact byte limits, and ordinary Runtime validation
   errors.

### Chosen solution

Choose alternative 3. The facade accepts nil/empty observation maps and empty
operational values. It copies the map, sorts names by raw Go string bytes,
validates all names before values, and validates values in the same order.
Unsafe names return `CodeValueInvalid` at `fields.name` without embedding the
name; well-formed unknown identifiers return `CodeUnknownMember` at their field
path.
Reference and observation limits count UTF-8 bytes. Every failure returns the
zero result and remains compatible with `runtimev2.ErrInvalid` and
`runtimev2.ValidationError` without exposing rejected payloads.

### Brief rationale

Go map iteration cannot define a stable public error. Echoing malformed or
control-bearing keys into an error path makes diagnostics unsafe, while rejecting
empty operational values would narrow the existing Runtime observation contract
without a requirement. A small deterministic validation pass preserves the root
error taxonomy and makes boundary behavior externally testable.

## Decision 5: approve the complete facade and release test addendum

### Question discussed

Which evidence is required before implementing and publishing the fixed
conformance facade?

### Alternatives considered

1. Reuse only the existing generic canonical-builder tests.
2. Add a minimal happy-path external consumer test.
3. Require the reviewed CF01-CF12 and RF01-RF02 matrix covering exact API,
   validation, transactionality, disclosure, architecture, compatibility,
   concurrency, clean consumption, and immutable release gates.

### Chosen solution

Choose alternative 3. The test addendum in
`docs/architecture/runtime-v2-canonical-contract-tests.md` was approved on
2026-09-29 after the unsafe-name, deterministic-error, exact-API, authorization,
transactionality, byte-limit, dependency, and release-race gaps were closed.

### Brief rationale

The facade exists specifically to provide external conformance evidence, so a
happy-path-only test would not prove its trust boundary or stable diagnostics.
The complete matrix exercises the public construction paths without creating a
second schema owner and preserves the immutable existing bundle.

## Decision 6: make the Runtime module release harness version-independent

### Question discussed

Should each Runtime module release edit workflow regexes, synthetic consumer
versions, release-note paths, and contract-test literals for the new version?

### Alternatives considered

1. Copy or edit the workflow and tests for every release cycle.
2. Keep the workflow permanently tied to the already published `v0.3.0` cycle.
3. Validate generic Runtime module semver/RC inputs, select release notes from the
   requested base version, and build clean consumers in temporary modules whose
   staged version is independent of the release number.

### Chosen solution

Choose alternative 3. Release automation accepts the supported Runtime module
semver and optional `-rc.N`, derives the release-notes file from the GA base
version, and preserves same-commit RC/GA and immutable-tag gates. Source-tree and
public-proxy consumers create temporary modules without `replace`; checked-in
test sources do not pin the next release version. Contract tests assert the
generic policy and retain `v0.3.0` only as historical compatibility evidence.

### Brief rationale

The module is expected to release frequently. Requiring mechanical version edits
in safety tests creates recurring review noise and makes stale assertions likely.
Parameterizing version selection keeps the immutable release controls while
allowing future releases without weakening or rewriting their tests.

## Contradiction check

The accepted Runtime v2 canonical-contract ADR keeps `schemaauthor` public but
trusted and permits imports only from approved schema packages and conformance
fixtures. This facade is an approved schema package and does not change that
decision. It preserves the existing schema, domains, bundle, vectors, and
legacy-v2 behavior, so no accepted ADR is obsolete.

## Related records

- `docs/architecture/runtime-v2-canonical-contract-structure.md`
- `docs/architecture/runtime-v2-canonical-contract-flow.md`
- `docs/adr/2026-09-27-runtime-v2-public-canonical-contract.md`
- `kimak-irmagi/sqlrs#138`
- `kimak-irmagi/sqlrs#139`
