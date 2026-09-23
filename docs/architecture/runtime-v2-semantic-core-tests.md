# Runtime v2 semantic core: test design

Status: approved and implemented for issue #107 on 2026-09-23.

This document defines conformance evidence for the approved
[interaction flow](runtime-v2-semantic-core-flow.md) and
[component structure](runtime-v2-semantic-core-structure.md). Tests verify the
public Runtime v2 identity contract, not implementation-specific helper layout.

## 1. Test layers and oracles

The suite has four layers:

1. white-box unit tests for exact canonical bytes, parsing, validation, and
   defensive copying;
2. public-package conformance tests using only exported API and JSON;
3. checked-in golden vectors shared with non-Go consumers;
4. module-boundary tests from local-engine and an independent Go module.

Golden expected values must not be generated or rewritten by the package under
test. A fixture is a conformance envelope with separate `input` and `expected`
objects. For `recipe`, the nested `input.value` is the exact public `Recipe`
JSON shape. For `relative`, it is a test-only construction input containing
`schema_version`, `anchor`, and public transform-provenance values. `expected`
records every canonical hash preimage in lowercase hex, each resulting digest,
all intermediate states, and the endpoint.

The test-only envelope has this exact shape:

```json
{
  "case": "G01",
  "comparison_group": "optional-name",
  "input": {"type": "recipe", "value": {}},
  "expected": {
    "canonical": {
      "factory": "optional-lowercase-hex",
      "transforms": [],
      "derived_states": []
    },
    "transform_fingerprints": [],
    "states": [],
    "endpoint": "sha256:..."
  }
}
```

`input.type` is `recipe` or `relative`. `comparison_group` and
`expected.canonical.factory` are omitted when not applicable; explicit `null` is
invalid. Arrays preserve transform/state order. `{}` placeholders contain a full
public value, never extension data. The verifier rejects unknown or duplicate
envelope members.

Initial values are reviewed against the normative grammar and cross-checked by
`scripts/maintenance/verify-runtime-v2-golden.mjs`. The command is:

```text
node scripts/maintenance/verify-runtime-v2-golden.mjs backend/libs/runtime-go/testdata/golden
```

The verifier reads only `input` when recomputing bytes and hashes, then compares
its result with `expected`. It shares no encoder code, generated constants, or
expected-value imports with the Go module. This command is a required PR and
release gate. Fixture updates require explicit schema/ADR review and have no
automatic update flag.

JSON object member order is not contractual. Round-trip tests compare validated
semantic values and recomputed identities, not raw JSON bytes.

## 2. Golden vectors

Checked-in fixtures under `testdata/golden` cover:

| ID | Fixture | Required evidence |
| --- | --- | --- |
| G01 | `factory-only.json` | Factory canonical bytes, root fingerprint/StateID, zero steps, endpoint. |
| G02 | `one-step.json` | Factory/root, transform bytes/fingerprint, derived state and endpoint. |
| G03 | `multi-step.json` | Three distinct transforms, every intermediate state, ordered endpoint. |
| G04 | `relative.json` | External anchor, two new steps, no factory/root record, endpoint. |
| G05 | `same-endpoint-recipe.json` | Recipe history used for the equal-endpoint comparison. |
| G06 | `same-endpoint-relative.json` | Relative history with the same root anchor, steps, and endpoint as G05. |

Each fixture is tested in-process, in a fresh helper subprocess, and by the
independent verifier. Every factory, transform, and derived-state hash includes
its exact canonical preimage bytes in `expected`. Linux and Windows CI consume
the same checked-in fixtures.

## 3. Canonical encoding tests

- **C01 — integer encoding:** verify boundary values for big-endian `u16`, `u32`,
  and `u64`.
- **C02 — length framing:** verify empty and non-empty byte payload framing and
  distinguish `("ab", "c")` from `("a", "bc")`.
- **C03 — record fields:** inspect encoder output and verify ascending numeric
  tags, exact field count, and exactly one occurrence of every required tag. The
  package does not implement a canonical-byte decoder.
- **C04 — field-set ordering:** all permutations of three fields produce the same
  canonical bytes and fingerprint; duplicate ASCII names fail.
- **C05 — exact strings:** UTF-8 bytes are encoded without normalization; distinct
  NFC and NFD values remain distinct.
- **C06 — digest representation:** textual lowercase `sha256:` values decode to
  exactly 32 raw hash-input bytes.
- **C07 — domain separation:** the same payload under factory-state, transform,
  and derived-state domains produces three different digests.
- **C08 — fixed domains:** verify the exact v2 domain bytes used by each public
  operation; unsupported schema versions are rejected under V01 rather than
  hashed in a caller-selected domain.

## 4. Identity sensitivity and diagnostic independence

Table-driven tests mutate exactly one valid resolved identity input at a time:

- **I01:** provider;
- **I02:** kind;
- **I03:** identity schema;
- **I04:** resolved field name;
- **I05:** resolved field value;
- **I06:** add one field;
- **I07:** remove one field.

Every accepted mutation changes the corresponding factory state or transform
fingerprint. A transform mutation changes its derived StateID and all later
descendants. A factory mutation changes the root and every descendant.

- **D01:** change declaration reference spelling.
- **D02:** change declaration arguments and their order.
- **D03:** change diagnostic attributes.
- **D04:** change resolver implementation/build metadata.

Each diagnostic mutation must alter provenance JSON where represented but must
not alter resolved identity, canonical bytes, fingerprints, StateIDs, or endpoint.

## 5. Lineage construction and integrity

- **L01:** factory-only `Build` returns one root, zero steps, and no synthetic
  transform.
- **L02:** `Build(A,B)` and `Build(B,A)` use distinct A/B transforms and produce
  different intermediate states and endpoints.
- **L03:** the same transform under two different parent StateIDs produces
  different derived StateIDs.
- **L04:** every step parent equals the preceding root/step StateID.
- **L05:** empty `Extend` returns the unchanged anchor and zero states/steps.
- **L06:** recipe and relative histories with an equal endpoint remain distinct
  Go types and JSON shapes.
- **L07:** decoded lineage independently recomputes each transform fingerprint,
  parent link, resulting StateID, and endpoint without the original `Recipe`.
- **L08:** no state or identity JSON exposes runtime ID, job ID, timestamp,
  physical path, checkpoint backend, or materialization metadata.

- **T01:** modify factory identity under an existing root.
- **T02:** modify root ID or factory fingerprint.
- **T03:** modify a relative anchor.
- **T04:** modify step transform identity.
- **T05:** modify step transform fingerprint.
- **T06:** modify derived-state parent ID.
- **T07:** modify resulting StateID.
- **T08:** reorder lineage steps.

Validated decoding rejects every tamper with `integrity_mismatch` or a more
specific structural code and the exact field/index path.

## 6. JSON and digest parsing

- **J01:** every public semantic value round-trips through JSON and retains equal
  identity and provenance.
- **J02:** unknown JSON members are rejected at root and nested levels.
- **J03:** duplicate JSON member names are rejected rather than last-value-wins.
- **J04:** trailing JSON values/tokens are rejected.
- **J05:** missing, `null`, empty, and wrong-type required members are rejected.
- **J06:** field-name casing is exact.
- **J07:** digest parsing rejects missing/wrong prefixes, uppercase hex, non-hex,
  truncated, and oversized values.
- **J08:** invalid UTF-8 is rejected before canonicalization.
- **J09:** JSON matches the normative allowlist for every public shape; physical
  metadata and alternate state-union fields are rejected as unknown.

J02-J09 use public `DecodeJSON` at the trust boundary. A compatibility test also
proves that `encoding/json.Unmarshal` accepts every valid golden value; native Go
syntax errors returned before `UnmarshalJSON` are not required to match
`ErrInvalid`.

Identifier and version validation is explicit:

- **V01:** any schema version other than `sqlrs.runtime.v2` is rejected and never
  fingerprinted.
- **V02:** empty identifiers are rejected.
- **V03:** uppercase, leading digits, and leading dot/hyphen/underscore are
  rejected.
- **V04:** whitespace, slash, colon, NUL/control bytes, and non-ASCII identifiers
  are rejected.
- **V05:** valid identifiers at the maximum byte length are accepted.
- **V06:** an identity with a required but empty `fields` array is valid.

## 7. Bounds and errors

Boundary matrices cover `limit-1`, `limit`, and `limit+1`:

- **B01:** identifier byte length;
- **B02:** resolved-value byte length, including multi-byte UTF-8;
- **B03:** resolved field count;
- **B04:** recipe/relative transform count;
- **B05:** top-level semantic JSON size.
- **B06:** diagnostic declaration/observation string, argument, and attribute
  count/size limits.

Constructor and JSON paths return the same stable validation code for the same
violation. Byte limits count bytes, not runes.

- **E01:** every failure matches `errors.Is(err, ErrInvalid)` and
  `errors.As(err, *ValidationError)`.
- **E02:** every failure has the expected stable `Code` and field/index `Path` and
  does not include rejected values in its error string.
- **E03:** functions returning values produce a zero result on failure.
- **E04:** failed `UnmarshalJSON` leaves an existing non-zero receiver unchanged;
  successful decoding atomically replaces it.

The 4 MiB JSON test proves rejection after bytes reach `UnmarshalJSON`; it does
not claim to prevent the caller or `encoding/json` from allocating the input.

## 8. Immutability

- **A01:** mutate input field and transform slices after construction.
- **A02:** mutate declaration arguments and diagnostic maps.
- **A03:** mutate fields and steps returned by accessors.
- **A04:** mutate provenance inputs after `Build` or `Extend`.
- **A05:** mutate the source byte slice after JSON decoding.

Previously built semantic values, canonical bytes, fingerprints, StateIDs,
endpoint, and later JSON output must remain unchanged. Accessors must return
defensive copies wherever they expose slices, maps, or byte arrays.

## 9. Process, platform, and fuzz tests

- **P01:** a helper subprocess recomputes all golden values in a fresh process.
- **P02:** Linux and Windows CI produce the checked-in golden results.
- **F01:** fuzz JSON decoding; no panic, excessive recursion, or accepted value
  whose embedded fingerprints/StateIDs fail immediate recomputation.
- **F02:** fuzz digest parsing and canonical scalar encoding.
- **F03:** fuzz valid field permutations and assert invariant fingerprints.
- **F04:** fuzz lineage JSON mutation and require either full validation or a
  structured rejection with no partial result.

Fuzz corpora include all golden vectors and boundary cases. The PR gate executes
the seed corpus through ordinary tests. Nightly CI runs each fuzz target for 30
seconds in a separate command; release verification runs each for 5 minutes.
"Excessive recursion" means a nested input within 4 MiB must receive a structured
error or valid result without panic or stack exhaustion. Fuzzing supplements but
does not replace exact golden, boundary, or cross-process tests.

## 10. Dependency and import conformance

- **M01:** `go.mod` has no `require` entries. A `go list -deps -json .` check
  permits only standard-library packages and packages whose import path begins
  with the Runtime v2 module path.
- **M02:** an external-package test in local-engine imports
  `github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go` and constructs a minimal
  factory-only lineage, proving no reverse/circular dependency.
- **M03:** `test/runtime-v2-consumer/go.mod` requires the module at `v0.0.0`, uses
  `replace github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go =>
  ../../backend/libs/runtime-go`, and passes `go test ./...` with `GOWORK=off`.
  Its dependency graph contains no local-engine import.
- **M04:** after the first nested-module tag is published, release verification
  repeats M03 without `replace`. M04 is a release gate, not a pre-publication PR
  gate for issue #107.

## 11. Coverage and execution gates

Deterministic module tests run with `go test ./... -count=1`. M02 runs through the
local-engine package test; M03 runs from its consumer directory with `GOWORK=off`.
The independent Node verifier runs separately. Race detection runs on Linux
amd64; Linux and Windows amd64 run deterministic/golden tests. Linux arm64 is a
release conformance target when a project runner is available.

Coverage is measured per package with a line report: 100% is the target and 95%
the minimum. Any shortfall follows the repository's documented approval loop
before adding tests or removing dead code.

Issue #107 is ready for implementation review only when G01-G06, C01-C08,
I01-I07, D01-D04, L01-L08, T01-T08, J01-J09, V01-V06, B01-B06, E01-E04,
A01-A05, P01-P02, the fuzz seed corpus, M01-M03, and the independent verifier
pass. Timed fuzz campaigns are nightly/release gates. M04 becomes mandatory at
publication time.
