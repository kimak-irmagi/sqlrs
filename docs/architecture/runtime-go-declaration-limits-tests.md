# Runtime-go declaration limits: test inventory

Status: corrected DL01–DL08 approved by @evilguest on 2026-10-20. Requirements are in the
[approved contract and implementation structure](runtime-go-declaration-limits.md).
Tests precede implementation; no old assertion changes are authorized here.

| ID | Required evidence |
| --- | --- |
| DL01 | All three extension roles accept ASCII and multibyte UTF-8 values at 256 KiB minus one/exactly/plus one, reject empty/invalid UTF-8, retain the 256-field and identifier limits, and report safe errors without accepted partial values. The byte cases must be representable exact boundaries, not character counts. |
| DL02 | Separate the decoded-value, raw-JSON and normalized-JSON matrices described below. Check normalized 4 MiB minus one/exactly/plus one including metadata, punctuation and escaped scalars against independent encoding/json bytes. Check raw bounds independently, normalization growth and exact safe limit errors. |
| DL03 | All three roles round-trip large payloads; field permutations retain bytes. Preserve current exact duplicate/unknown-member, null, trailing-data and atomic-receiver behavior. Include the currently accepted alternate-case known member as a positive regression control; do NOT introduce rejection of case aliases. Source slices and accessor results cannot mutate the accepted value. |
| DL04 | Factory/transform DTOs, standalone documents and recipes charge children AND envelopes, including diagnostic children, omitted optionals and normalized empty collections. Check exact parent limits, mixed roles and ordering. Existing composition expansion must propagate newly possible constructor/encoding failures as CodeExpansionTooLarge with a zero result/trace; near-limit valid snapshots/expansions still work. |
| DL05 | Both diagnostic roles and resolved/canonical values retain original 4096-byte bounds, including non-ASCII counts. Legacy reference/argument/attribute limits and canonical vectors stay unchanged. Existing canonical builders/production facades accept a larger valid source and preserve their canonical error mapping and empty result for rejected aggregates. |
| DL06 | The public resolver key path accepts a larger normalized declaration and remains stable for equivalent field order. Public cache-record round trips retain the full source under their own envelope budget; oversized outer records reject, not truncate. No acquisition/effects or alternate private keys are introduced. |
| DL07 | A mandatory isolated consumer imports the actual module API without replace directives, round-trips a value above 4096 bytes and at the new boundary, rejects overflow and confirms old diagnostic/canonical bounds. Source-PR verification uses a clean local module distribution; published RC/final consumption is a separate #160 gate. |
| DL08 | Check the bounded production traversal and public-call-path controls below, not only final rejection. Limit errors have exact codes/paths and preserve nested errors.As/errors.Is; rejected value markers do not leak. Do not add sanitization of unknown member names or a new syntax-error contract. Run complete regressions, race/static checks and per-line changed-code coverage, minimum 95%; local duration is not performance acceptance. |

## DL02: independent size and error oracles

- DL01 tests decoded field-value bytes independently of their quoted length.
  Use representable multibyte boundaries with an ASCII suffix where needed.
- For all three roles, normalize documents to `MaxJSONBytes-1`, exactly the
  limit and one byte over; independently encode wire-equivalent fixture DTOs
  with encoding/json. The fixture oracle must not call the new size helper or
  the candidate public MarshalJSON to determine the expected size. Include
  quote/backslash/HTML/control/U+2028/U+2029 data and differing metadata lengths.
- Independently make raw JSON one byte below/at/above 4 MiB using INTERIOR
  whitespace while the normalized value fits. Also test compact raw bytes below
  the limit whose standard escaping makes the normalized document too large.
- Compare DecodeJSON, direct UnmarshalJSON and json.Unmarshal for inputs passed
  to the method unchanged. Test separately that DecodeJSON/direct methods see
  outer whitespace while json.Unmarshal may strip it before delegation. Do not
  impose an impossible whole-input guarantee on that standard-library entry.
- For a sole value overflow expect CodeTooLarge and `fields[i].value`; for a
  raw or normalized whole-boundary overflow expect CodeTooLarge and `$`.
  Cover constructors, own MarshalJSON and structured library decoding; retain
  existing nested prefixing/wrappers and receiver state after failure. The
  standard library may itself reject malformed JSON before delegation.

## DL04: composition and enclosing-boundary regressions

Use small valid catalog documents referring repeatedly to a bounded transform:
each input and individual transform fits but the expanded recipe exceeds 4 MiB.
Require CodeExpansionTooLarge at the existing logical source/pointer and no
declaration or trace. Check that the valid catalog remains usable for a later
successful expansion. Retain the existing oversized-expansion assertions.

Audit discarded constructor/encoding errors in ExpandRecipe, ExpandTransform,
copyFactory/copyTransform and expanded accessors. Add positive near-boundary
copy/expansion controls, including standalone wrapper overhead. No new public
copy API or alias semantics are required. Parent-size controls distinguish nil
and empty arguments/attributes and omitted empty optional slices; they also
include diagnostic children so a whole-child marshal cannot bypass preflight.
Expansion charges the exact envelope/factory, each output step and commas before
copying/appending that step or its trace entry. Verify that this uses existing
bounded public child encodings/checked additions, not serialization of the whole
oversized expansion; the final constructor alone cannot prove early rejection.
NewAliasDocument/DecodeAliasDocumentJSON controls distinguish aggregate source
overflow (CodeExpansionTooLarge at the document root) from invalid individual
fields/structure (existing CodeInvalidDocument/nested location). A new early
root-size error must not be accidentally reclassified by an old catch-all
adapter. Do not reclassify all leaf CodeTooLarge errors as aggregate failures.

## DL08: early-rejection evidence without timing thresholds

- Exercise the production budget's checked additions below/at/over the limit,
  including safe rejection of a very large positive addition without integer
  overflow or allocating a correspondingly huge buffer.
- Use the production array/object traversal helper with local counting child
  visitors. On exhausted/separator-overflow/child-overflow budgets, later child
  visitors must not run; an exact valid budget must visit the expected children.
  This is the helper used by real sizing walks, not a separate test algorithm.
- Review and test the actual public constructor/codec boundaries: they must
  route through preflight before aggregate snapshots/encoding; rejection alone
  or private-counter tests alone do not establish that ordering. Scalar quoting
  is individually bounded and may not marshal a whole DTO/list/diagnostic child.
  Include composition's charge-before-step-copy ordering in this call-path audit;
  its atomic chunks are already guarded public encodings, not unchecked DTOs.
- Keep bounded many-field/escaping adversarial integration inputs, but do not
  assert machine-specific heap totals or elapsed time. Do not count a bounded
  writer around json.Encoder as evidence: its aggregate buffer already exists
  before Write. There is no mutable global instrumentation hook.
- Check errors through both structured library entry points and standard JSON
  wrapping, using single-fault oracles. Put synthetic rejected markers in VALUES,
  not arbitrary member names which the existing path contract may echo. Do not
  turn this size change into general shared-decoder hardening.

After approval inspect existing tests for contradictory old declaration-value
expectations. Request an explicit decision before changing them; unchanged
diagnostic/resolved/canonical expectations are negative controls, not candidates
for relaxation. Preserve existing independently fixed conformance vectors.

## Local implementation evidence (2026-10-20–21)

Tests were added before production changes. The public boundary tests failed on
the original 4096-byte source bound; the diagnostic-parent control also exposed
the missing normalized aggregate check. No existing test/assertion was changed.

The complete seven-package module passes with the CI coverage command:

```powershell
go test ./... -count=1 -timeout=15m '-coverpkg=./...' '-covermode=count' '-coverprofile=coverage-runtime-all.out'
go vet ./...
```

The timeout is a local watchdog, not timing/performance acceptance or a CI change.
Use quoted flags in PowerShell: earlier unquoted dotted flags did not retain the
intended final profile. Only the inspected `mode: count` profile above supplies
the final results: **5121/5338 (95.93%)** module statements, **868/898 (96.66%)**
across touched implementation files, and **72/72 (100%)** in the new
`declaration_limits.go`. The existing CI checker and all seven checker tests pass;
no additional coverage iteration or assertion relaxation was needed.

The complete isolated external consumer passes using a new local file-proxy
version, `GOWORK=off`, no replace and standard Go caches. Its new DL07 fixture is
automatically included by the existing source and release consumers. Both
diagnostic roles retain 4096-byte controls there. Independent legacy/extension
and canonical-bundle verifiers pass; bundle-policy/verifier tests pass with one
existing Windows symbolic-link case unavailable on this host. No golden bytes,
workflow or publication tooling were changed.

Call-path review confirms scalar-only quoting, a shared parent budget before
aggregate snapshots/encoding, and output-step charging before composition copies
and trace append. Review found and corrected premature argument/attribute copying
in `declarationFromWire`; the final full module/static run includes that fix.
The low-level canonical builders retain their existing declaration-validator
codes/prefix; the production execution facade still returns canonical
`CodeLimitExceeded` and an empty result for an oversized declaration.

The full Windows race run is **not green**: its sole failing test was the existing
`TestWindowsUSNEvidenceCurrentAndMutation` (`windows-unknown`, no NTFS/USN token).
The same unchanged test fails under `-race` on the clean pre-#159 worktree at
`6a8d2f728d2c17847255df4267a35ab0fe140f03`; its production and test files match
the #159 base. The ordinary full run passes. No USN check was skipped or weakened.
Linux race/fuzz, other OS qualification and GitHub CI remain source-PR gates;
local consumer success is not the separate #160 public-publication gate.
The final focused race run of all new declaration/composition/cache scenarios
passes after the copying-order correction. This does not turn the failed full
Windows race run into a successful one.
