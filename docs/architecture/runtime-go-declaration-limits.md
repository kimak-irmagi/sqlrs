# Runtime-go: independent unresolved declaration limits

Status: the public contract was approved by @evilguest (41718235) on 2026-10-20.
The corrected implementation structure and [test inventory](runtime-go-declaration-limits-tests.md)
were also approved on 2026-10-20. Implementation is [#159](https://github.com/kimak-irmagi/sqlrs/issues/159);
publication is independently tracked by [#160](https://github.com/kimak-irmagi/sqlrs/issues/160).
The implementation is present locally; verification evidence is recorded in the
test inventory. No merge or release is claimed.

## Public contract

Keep `DeclarationField{Name, Value string}` and all three opaque extension roles:
`InputDeclaration`, `ExecutionEnvironmentDeclaration`, `DeploymentDeclaration`.
Add `MaxDeclarationValueBytes = 256 << 10` for each unresolved field value,
measured in UTF-8 bytes. Keep the 256-field count, identifier rules, nonempty
valid UTF-8 values, unique sorted names and defensive ownership.

Three independent sizes apply; none substitutes for another:

| Size | Limit and measurement |
| --- | --- |
| Source field value | 256 KiB of decoded UTF-8 bytes, before JSON quoting. |
| Normalized declaration document | 4 MiB of compact bytes emitted by the existing `MarshalJSON`/`json.Marshal` path, including all keys, punctuation, metadata and default HTML/Unicode escaping. |
| Raw JSON presented to the library decoder | Existing 4 MiB input bound, including whitespace and escape spelling in the bytes actually received. |

Constructors and encoders enforce the normalized bound. Decoding enforces both
the raw bound at its entry and the normalized bound before committing a value.
A short raw spelling which grows beyond the normalized bound rejects; a padded
oversized raw spelling rejects even when the normalized value would fit. Nothing
is truncated. These bounds apply to declaration documents in this change, not a
new aggregate rule for standalone diagnostics or resolved/canonical types.

`DecodeJSON` and direct `UnmarshalJSON` receive the supplied bytes and enforce
the whole-input bound. Standard `encoding/json.Unmarshal` can remove outer
whitespace before calling the method; the library can bound only the slice it
receives there. Use interior whitespace for shared raw-bound controls and test
the outer-whitespace distinction explicitly. Likewise a caller's indentation
is not the library's normalized encoding. No new streaming/HTTP API is added.

The aggregate rule applies at typed extension boundaries and at the enclosing
factory, transform, standalone wrappers and recipe boundaries, including any
diagnostic children they contain. Valid children do not authorize an oversized
aggregate. Mutable factory/transform DTO assignment is not admission; validate
at their public encoding/decoding and document-construction boundaries. Retain
all existing count/nesting limits. Do not change JSON shapes or make provider
payloads recursive.

`MaxResolvedValueBytes`, diagnostic values, canonical strings, fingerprints and
State construction are unchanged. In particular, the existing shared field
validator also serves diagnostics; increasing its limit globally is forbidden.

## Components, ownership and flow

Stay inside the existing runtime-go root package. A small internal
`declaration_limits.go` helper owns bounded standard-JSON size accounting; the
existing declaration/document/recipe files retain their constructors and codecs.
Field validation receives an explicit allowance: the declaration-specific one
for source values, the original allowance for diagnostics. No new service,
persistent store, CLI, HTTP API, module or provider dependency is introduced.

Construction validates bounded field lists, qualification and strings, checks
the normalized aggregate, then returns an immutable snapshot. Enclosing values
charge each child plus their own envelope before copying/encoding the whole
aggregate. Decoding uses the same constructors and commits receiver replacement
only after complete success. Encoding retains zero-value rejection and must not
return successful oversized or partial documents.

### Bounded size measurement before aggregate allocation

Use a private `declarationJSONBudget` with `used`/`limit` byte counts and checked
addition (`n > limit-used` rejects, without overflowing). Counts are nonnegative
and `0 <= used <= limit` is maintained throughout traversal. Its private string and
array/object traversal helpers measure the existing wire projections; they do
not serialize a second wire format. Share the projections' optional-field and
empty-collection rules with the codecs, rather than inventing another selection
of fields. In particular count normalized nil arguments/attributes, omitted
empty optional slices, arrays, separators and standalone/recipe envelopes.

Pass only individually bounded, validated strings/keys to `encoding/json` for
escaping. A scalar's raw byte count plus quotes is an allocation-free lower
bound: if even that cannot fit, reject before quoting it. Otherwise quote that
scalar alone, charge its exact encoded length and release the temporary before
continuing. No arbitrary DTO, list or child document may be passed to
`json.Marshal` during preflight. The largest new scalar is 256 KiB; even its
worst escaped temporary is bounded by six bytes per source byte plus quotes.
Do not quote a complete diagnostic object merely because its individual fields
are small; visit and charge its fields under the same parent budget.

Traversal stops on the first budget error and must not visit later members or
children, deep-copy the aggregate or attempt full encoding after that failure.
Only successful preflight permits the existing whole-value snapshot/encoding.
Sizing must not call a public `MarshalJSON` which re-enters the same preflight.
There is no mutable global test observer, stored source copy, new size cache or
generic reflection-based JSON encoder.

The production traversal helpers receive local child-visitor callbacks. Tests
observe those callbacks on exhausted/exact budgets, prove no tail visits after
failure and check that the state never exceeds its limit. Public boundary cases
and review of the actual constructor/codec call paths additionally establish
that this preflight precedes aggregate cloning/encoding; a private helper test
alone is not sufficient evidence. A bounded writer wrapped around
`json.Encoder.Encode` is NOT early-allocation protection: Encode first buffers
the complete value. Do not substitute wall-clock thresholds or machine-specific
heap numbers for the traversal controls.

Caller-owned strings may already be large; this prevents a second oversized
aggregate allocation, not the caller's original allocation. Decoders already
receive a bounded raw document; no promise of streaming decode is introduced.

### Existing composition callers and atomic failure

Audit existing callers whose previously valid snapshots made constructor or
encoding errors appear impossible. In particular, `composition.ExpandRecipe`
currently discards both `NewRecipeDeclaration` and `json.Marshal` errors. A new
aggregate rejection must become `CodeExpansionTooLarge` with the existing
logical source/pointer, and a zero expansion AND zero trace; it must not turn
into success because the failed encoding has length zero. No partial result or
truncated recipe is returned. Preserve the other established error mappings.

ExpandRecipe must also charge each output step BEFORE cloning/appending it and
its output trace entry; merely checking the final root constructor is too late.
Use the existing composition checked-addition helper and an operation-local
byte count. Get exact envelope/factory cost from the bounded public empty-step
recipe encoding, then charge each already validated transform's bounded public
encoding and its comma. The root scalar-only preflight above guards these
individual public encodings; composition may use their lengths but must never
encode the whole prospective expansion to measure it. No duplicated root wire
encoder, exported sizing API, persistent metric or size cache is needed. Reject
before copying a step that cannot fit. The final root constructor remains the
authoritative aggregate check, and trace/count limits remain independent.

Review `ExpandTransform`, `copyFactory`, `copyTransform` and expanded-result
copy/accessor paths too. Either handle a newly possible error explicitly or
establish the bounded immutable invariant that makes that particular operation
infallible. Snapshot helpers must never silently replace a valid value with a
zero DTO when an enclosing standalone wrapper adds its own overhead. Exercise
near-boundary positive controls as well as over-budget expansion. This is an
internal compatibility adjustment to the existing optional composition package,
not a new alias model, service or consumer-side YAML requirement.

Include `NewAliasDocument`/`DecodeAliasDocumentJSON` and their nested error
adapters in that audit, not just successful expansion. Preserve the distinction:

| Failure | Composition outcome |
| --- | --- |
| Whole source-document raw/normalized size overflow | Existing CodeExpansionTooLarge with its root document location. |
| Whole expanded-recipe size overflow | CodeExpansionTooLarge at the target source/definition pointer, zero expansion/trace. |
| Invalid individual field or structural input | Existing CodeInvalidDocument mapping and nested location; do not reclassify every CodeTooLarge as an aggregate overflow. |

A new root aggregate failure observed earlier in a child constructor must not
silently become CodeInvalidDocument solely because the old adapter wrapped all
constructor failures that way. Isolate these cases in the tests. This adjustment
does not change the composition schema or its independent catalog/trace limits.

Include existing canonical declaration builders/production facades in the caller
review: a larger valid source must remain usable, and an aggregate rejection
must keep their established canonical error mapping and empty-result behavior.
Do not return a legacy CodeTooLarge where the facade promises CodeLimitExceeded.
The low-level `NewFactoryIdentityBuilder`/`NewTransformIdentityBuilder` retain
their existing declaration-validator codes and `declaration` prefix; the
`schemas/sqlrs.BuildExecutionTransform` facade maps them to its canonical taxonomy.
This change does not revise that pre-existing distinction.

The resolver continues to normalize and key the public declaration. No private
key, payload fingerprint, framing codec or fallback is added. Cache-record and
other outer envelopes keep their own existing limits; a declaration near its
own limit is not a promise that every larger enclosing record will fit.

## Compatibility and verification

Existing values which meet the new aggregate rule retain their normalized bytes
and semantic results. Admission is intentionally tightened for oversized
normalized declarations: some constructors previously succeeded, and some
compact raw spellings were readable, even though their emitted JSON exceeded
4 MiB. Do not promise to retain that success or describe the change as purely
additive. Historical tags/golden bytes are not rewritten; valid identity/State
algorithms and still-accepted declaration bytes remain unchanged.

Do not add case-folding rejection or otherwise harden the shared decoder in
#159. Preserve its current member-name matching, exact duplicate/unknown-member
handling, null rules and receiver atomicity. DL03 is a regression control for
those behaviors, not authorization for a broader JSON contract change.

Isolate one violation in exact error-oracle cases. A too-large source field is
`CodeTooLarge` at `fields[i].value` (the existing sorted field index); a new
whole-document/raw-size failure is `CodeTooLarge` at `$` for that boundary.
Retain existing nested prefixing and `errors.As`/`errors.Is` behavior, including
standard JSON wrappers. Malformed input passed through `encoding/json.Unmarshal`
may be rejected by the standard library before our method runs; tests must not
demand a library `ValidationError` in that case. `DecodeJSON` is the structured
library-error entry. No new precedence is promised for multiply invalid inputs.
Rejected field VALUES must not occur in errors; existing diagnostic paths may
include supplied member names, and this change does not sanitize unknown keys.

The separate test inventory covers direct and nested API paths, exact byte
boundaries, unchanged limits, immutable accessors and a standalone consumer.
Existing test contradictions require an explicit old-versus-new decision before
any assertion is changed. Coverage target is 100%, minimum 95% for changed code.
Publication/RC/version authorization remains separate from source implementation.

## Repeated design review (2026-10-20)

The correction addresses four review findings: new fallible composition calls,
an unapproved case-matching change in DL03, missing observable early-rejection
controls, and conflation of value/raw/normalized byte limits and compatibility.
The second pass also traced composition's copying order and source-document
error adapters; the revised flow now charges steps before copying and preserves
the distinction between aggregate and individual-field failures. Static review
of the corrected proposal found no further blocking issue. This is not evidence
that an implementation satisfies the contract; implementation evidence is
recorded separately in the test inventory.
The user subsequently approved the detailed structure and revised DL01–DL08 on
2026-10-20. Existing assertion changes still require a separate decision;
approval alone is not implementation, coverage or runtime acceptance.
