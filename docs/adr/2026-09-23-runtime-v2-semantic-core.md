# Runtime v2 semantic core decisions

Conversation timestamps: 2026-09-23 14:59, 15:37, and 16:36 Asia/Novosibirsk
(07:59, 08:37, and 09:36 UTC). GitHub user ID: @evilguest (41718235).
Agent: Codex (GPT-5). Status: accepted for issue #107, including the corrections
from the critical architecture review.

## Decision 1: public module and canonical repository

Question: where should the reusable semantic core live and what should consumers
import?

Alternatives: local-engine `internal`; a public package in the local-engine
module; a separate nested module; a separate repository under another owner.

Decision: create `backend/libs/runtime-go` with module path
`github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go`, package `runtimev2`, and
add it to `go.work`. Nested-module releases use tags such as
`backend/libs/runtime-go/v0.1.0`. The repository was renamed from `taidon` to
`sqlrs`; the local `origin` must use the canonical address.

Rationale: the nested module enforces forbidden dependencies and is externally
fetchable from the real repository while local, shared, and conformance consumers
share one contract. Runtime v2 names the schema, not the Go module major version.

## Decision 2: canonical identity encoding

Question: should JSON bytes, canonical JSON, or typed binary bytes define hashes?

Alternatives: ordinary JSON; a canonical JSON profile; a domain-separated,
tagged, length-delimited binary encoding with JSON retained for transport.

Decision: use the normative binary grammar in the architecture flow with
big-endian lengths/tags, valid UTF-8 without normalization, ASCII field names,
raw 32-byte digest inputs, and SHA-256. JSON is not a hash input.

Rationale: independent implementations can reproduce exact bytes without relying
on Go JSON behavior, map iteration, or diagnostic transport evolution.

## Decision 3: provider-neutral resolved identity

Question: how should generic identities remain explicit and interoperable?

Alternatives: provider-specific core types; an opaque caller-computed digest;
unqualified name/value attributes; provider and identity-schema-qualified fields.

Decision: resolved factory and transform identities contain `Provider`, `Kind`,
`IdentitySchema`, and unique named `ResolvedField` values. Resolver build/version
is diagnostic; semantic changes select a new identity schema.

Rationale: the core stays provider-neutral while avoiding accidental equivalence
between independently defined field vocabularies. It guarantees inclusion of
supplied fields; providers remain accountable for schema completeness.

## Decision 4: identity and diagnostics are separate values

Question: should unresolved declarations live inside the hashed resolved object?

Alternatives: hash declarations; embed but exclude them implicitly; represent
identity and optional declaration/observation as separate typed layers.

Decision: use immutable `ResolvedFactoryIdentity` and
`ResolvedTransformIdentity`, wrapped by `FactoryProvenance` and
`TransformProvenance` when diagnostic declaration/observation data is needed.

Rationale: type boundaries make identity-bearing fields explicit and allow
mutable declaration spelling to change without changing resolved identity.

## Decision 5: self-contained lineage representation

Question: how should factory-only, recipe, and relative histories retain enough
provenance for independent verification?

Alternatives: synthetic root transform; endpoint IDs only; states containing only
transform fingerprints; explicit root/anchor plus self-contained lineage steps.

Decision: a root state has a factory fingerprint and no transform. Each
`LineageStep` contains transform provenance, its fingerprint, and its resulting
state. `RecipeLineage` contains factory provenance, root, and steps;
`RelativeLineage` contains an external anchor and new steps.

Rationale: factory-only recipes are natural, serialized lineage can recompute
every edge, and logical history remains independent of physical checkpoints.

## Decision 6: recipe and relative histories use distinct types

Question: should one tagged union or separate public types distinguish histories?

Alternatives: `Lineage{Kind,...}` with runtime validation; separate
`RecipeLineage` and `RelativeLineage` types.

Decision: use separate types and JSON shapes. Equal parent and transform inputs
still produce equal endpoint StateIDs; the distinct history container does not
alter the required state formula.

Rationale: impossible combinations are reduced at compile time and canonical and
relative histories remain distinct by construction without corrupting StateID.

## Decision 7: immutable semantic values and strict decoding

Question: how should public Go values preserve validation after construction?

Alternatives: exported mutable structs; mutable DTOs plus repeated validation;
opaque semantic values with constructors, defensive copies, and custom JSON.

Decision: declarations remain bounded DTOs, while identities, states, steps, and
lineages use private fields and validated constructors/JSON methods. Unknown JSON
fields and integrity mismatches are rejected.

Rationale: callers cannot mutate slices/maps behind a previously verified
fingerprint, and stored or remote values cross one explicit validation boundary.

## Decision 8: bounded validation and error contract

Question: what resource and error behavior belongs in the public core?

Alternatives: unbounded inputs and free-form errors; caller-defined limits;
versioned package limits with structured code/path errors.

Decision: use the documented limits for names, values, field counts, recipe
steps, and JSON size. Return `ValidationError{Code, Path}`, matchable through
`ErrInvalid`, without rejected values or partial results.

Rationale: shared and local consumers receive deterministic failure semantics and
remote input cannot request unbounded canonicalization work.

## Decision 9: relation to managed database identity

Question: should the semantic module import or directly reuse current local
managed-identity types?

Alternatives: import local types; duplicate their persistence identifiers;
remain independent and let a later adapter define a factory identity schema.

Decision: keep the module independent. A later adapter may map logical immutable
values such as image/init digests, policy, engine kind, and SQL-observable managed
username. Store/authorization locators and integrity copies remain outside logical
Runtime v2 identity.

Rationale: this preserves the public module boundary, avoids physical/store scope
in logical IDs, and leaves the exact integration to its own reviewed change.

## Decision 10: strict JSON transport schema

Question: should golden fixtures define JSON implicitly, or should public JSON
have a normative schema before implementation?

Alternatives: infer JSON from Go structs/fixtures; publish only examples and allow
unknown fields; define exact member names, discriminated state shapes, required
and optional members, and atomic decoding behavior.

Decision: use the exact JSON shapes in the component-structure document. Object
order is insignificant, but unknown and duplicate members, null required values,
invalid UTF-8, and trailing tokens fail. `state_kind` discriminates factory and
derived states. Failed unmarshal leaves an existing receiver unchanged. Trust
boundaries use public `DecodeJSON` so malformed syntax is normalized before Go's
`encoding/json` can return its own pre-`UnmarshalJSON` error.

Rationale: persistence and remote consumers need a contract independent of Go
field layout, and failed decoding must not expose a partially validated value.

## Decision 11: independent and traceable conformance tests

Question: how should tests prove the byte-level protocol without using the
implementation as its own oracle?

Alternatives: implementation-generated snapshots; Go-only golden hashes; reviewed
canonical preimages plus an independent verifier and fully identified matrices.

Decision: every golden fixture stores exact preimage bytes and expected results
in a test-only envelope. A Node.js verifier recomputes from input without sharing
Go encoder code. All diagnostics, tampering, validation, boundary, error, and
immutability cases have stable test IDs; PR, nightly, release, and post-publication
gates are separate.

Rationale: exact preimages expose encoder mistakes, stable IDs preserve
requirement-to-test traceability, and release-only checks do not falsely block the
first unpublished module version.
