# Runtime v2 alias composition: test design

Status: approved by @evilguest for issues #109 and #137 at
2026-09-28 18:28:21 +07:00. The #137 package passes L01-L13 at 100% statement
coverage.
Existing-test contradiction review completed 2026-09-28 18:28:21 +07:00;
no contradictions were found.

This plan verifies the approved
[interaction flow](runtime-v2-composition-flow.md) and
[component structure](runtime-v2-composition-structure.md). Test IDs are stable
requirement references. Production implementation follows the required review
of existing tests for contradictions.

## 1. Test layers and fixtures

The suite has four layers:

1. white-box package tests for validation, bounds, traversal, immutability, and
   exact error locations;
2. external-package conformance tests using only exported Go and JSON APIs;
3. reviewed JSON fixtures for one nested recipe trace and one standalone
   transform trace;
4. CLI-package tests for strict YAML adaptation and legacy classification,
   without routing current execution through Runtime v2.

Golden JSON asserts the documented wire shape, not incidental Go field layout.
Declaration comparisons are structural; JSON object order is asserted only for
the explicitly canonical alias-name order. Test helpers must not generate their
own expected traversal, origins, error pointers, or compatibility result.

## 2. Alias document and public API

- **D01 — valid variants:** construct and round-trip an empty document, a
  transform, a factory-only recipe, a recipe-prefix reference, a named-transform
  step, an inline-transform step, and the combined documented example.
- **D02 — closed unions:** reject zero or multiple alias variants, base variants,
  and step variants; reject missing, extra-for-variant, unknown, and `null`
  members at every level.
- **D03 — strict JSON:** reject wrong/missing schema versions, duplicate object
  members, trailing tokens, wrong JSON types, invalid UTF-8, and a nil decode
  target. Failed decoding leaves a non-zero receiver unchanged.
- **D04 — names:** accept boundary-valid lower-case alias names and reject empty,
  upper-case, leading non-letter, slash, control, non-ASCII, and overlong names
  in both definitions and references.
- **D05 — duplicate constructor names:** duplicate `NamedAliasInput` names fail
  before a value is returned; JSON duplicate keys fail before map materialization.
- **D06 — canonical transport:** semantic round trips preserve step order and
  emit aliases in unsigned bytewise name order independent of constructor or
  input-object order.
- **D07 — immutable snapshots:** mutation of constructor slices, nested Runtime
  declarations, arguments, attributes, and values returned from accessors cannot
  change a document, expanded value, trace, or later JSON output.
- **D08 — zero and empty values:** constructed empty documents/catalogs are valid;
  zero-value documents/catalogs/traces fail or expose only their documented
  empty accessor result and cannot be marshalled as valid data.
- **D09 — document bounds:** alias count and canonical JSON size are covered at
  `limit-1`, `limit`, and `limit+1` through constructors and decoders. Constructor
  accounting rejects oversize input atomically; allocation profiling is advisory
  evidence rather than a platform-sensitive correctness assertion.
- **D10 — nested Runtime validation:** every invalid factory/transform declaration
  remains discoverable as `runtimev2.ValidationError`, maps to
  `invalid_document`, preserves its stable validation code/path, and uses the
  enclosing declaration's composition pointer.

## 3. Catalog construction

- **C01 — empty and single-source catalogs:** an empty constructed catalog yields
  `missing_reference`; one valid source resolves every contained alias.
- **C02 — source IDs:** cover empty, invalid UTF-8, byte-length boundaries,
  duplicate IDs, bounded opaque Unicode IDs, and diagnostic preservation. The
  public package does not interpret IDs as paths; CLI-relative-path enforcement
  is covered separately by L04.
- **C03 — permutation determinism:** every permutation of a fixed three-source
  fixture, plus deterministic shuffled larger fixtures, produces equal expansion
  and trace output or the same error code, primary location, and contexts.
- **C04 — ambiguity:** duplicate alias definitions fail eagerly with
  `ambiguous_reference`; candidates are complete, bounded, sorted by `SourceID`,
  point to definitions, and do not depend on source order.
- **C05 — precedence:** simultaneous source-count, source-ID, zero-document,
  aggregate-size, and alias-ambiguity defects select the documented canonical
  error precedence.
- **C06 — aggregate bounds:** source-document count, total definition count,
  source-ID bytes, and `MaxCatalogBytes` use checked arithmetic and cover
  `limit-1`, `limit`, and `limit+1` without partial catalogs.
- **C07 — snapshot isolation:** mutating source slices or replacing input
  documents after `NewCatalog` does not affect the catalog; concurrent read-only
  expansion is race-free.
- **C08 — no hidden lookup:** catalog creation and expansion perform no directory
  scan, environment/config lookup, implicit import, case normalization, or source
  precedence fallback.

## 4. Deterministic expansion

- **E01 — factory-only recipe:** expansion returns exactly one copied factory,
  zero transforms, and no synthetic step.
- **E02 — ordered steps:** named and inline transforms retain literal source
  order; repeated references remain repeated output transforms.
- **E03 — recipe prefix:** one- and multi-level `base.recipe` chains contribute
  the deepest factory and every prefix transform before child steps.
- **E04 — multiple migration roots:** distinct named migration transforms applied
  to one recipe remain sequential and are never sorted or parallelized.
- **E05 — standalone transform:** `ExpandTransform` returns the versioned
  `TransformDeclarationDocument` without requiring recipe membership.
- **E06 — top-level errors:** invalid target names, missing targets, and
  wrong-kind targets return the documented code and primary source/pointer.
- **E07 — nested reference errors:** missing and wrong-kind base/step references
  point to the exact escaped RFC 6901 reference location.
- **E08 — cycles:** self and multi-node prefix cycles return the exact closed
  cycle from the first revisited recipe through the repeated closing entry; the
  primary error points to the closing reference.
- **E09 — deep iterative chain:** the maximum valid recipe chain succeeds without
  recursion or stack growth; one definition/count beyond the bound fails
  deterministically.
- **E10 — output bounds:** transform count, expanded-declaration JSON size, trace
  node/origin counts, and trace JSON size cover their boundary matrices and
  return `expansion_too_large` atomically.
- **E11 — semantic exclusion:** alias/source renaming with references updated can
  change only trace diagnostics, while source-document reordering changes neither
  declaration nor trace. Equal expanded declarations contain no alias names or
  source paths injected by composition.
- **E12 — declaration sensitivity:** changing a referenced validated declaration
  or step order changes the expanded declaration; changing trace-only metadata
  does not.
- **E13 — provider opacity:** moving a composition source or changing only its
  `SourceID` never rebases `reference`, argument, attribute, or extension fields.
  The package performs no resolver/provider calls.
- **E14 — atomic failure:** every expansion failure returns zero results, no
  declaration, and no partial trace; a later valid call on the same catalog is
  unaffected.

## 5. Expansion trace and errors

- **T01 — recipe golden:** the nested recipe fixture asserts target/base/transform
  node order, parent IDs, definition pointers, reference pointers, factory
  origin, transform indexes, and exact schema version.
- **T02 — standalone golden:** the standalone fixture has one transform target,
  no factory member, and exactly one transform origin.
- **T03 — occurrence model:** repeated use of one alias creates distinct nodes
  with distinct reference pointers; inline transforms create no extra node and
  point to the containing recipe node.
- **T04 — strict trace decoding:** reject unknown/duplicate/missing/null members,
  wrong schema/kind, trailing tokens, invalid UTF-8, and failed decode mutation.
- **T05 — graph integrity:** reject non-dense IDs, non-zero target, target parent,
  missing non-target parent/reference pointer, non-contiguous recipe prefixes,
  non-chain recipe parents, transform nodes used as parents, forward/dangling
  links, unused nodes, and standalone traces with anything other than one target
  node.
- **T06 — origin and pointer integrity:** reject a factory not owned by the last
  recipe node, transform nodes used zero or multiple times, named-node/origin
  order disagreement, invalid recipe/step output order, non-contiguous step
  indexes, wrong/missing output indexes, invalid node aliases/source IDs, and
  non-canonical or overlong definition, reference, factory, named-transform, and
  inline-transform pointers.
- **T07 — accessors:** node, factory-origin, transform-origin, and trace accessors
  expose the exact values and defensive copies; absent parent/factory uses the
  documented boolean result.
- **T08 — trace size:** maximal valid trace counts and JSON bytes succeed;
  `MaxTraceNodes+1`, `MaxTransforms+1` origins, and `MaxTraceJSONBytes+1` fail
  without mutating a receiver.
- **T09 — stable error envelope:** failures returned by composition constructors,
  catalog expansion, dedicated decoders, and value-level JSON validation match
  `ErrInvalid`, support `errors.As(*Error)`, carry the expected code/source/
  pointer, and return zero irrelevant contexts. Dedicated decoders wrap malformed
  syntax; a native `encoding/json` syntax error raised before `UnmarshalJSON` is
  asserted only as a decode failure.
- **T10 — context bounds:** cycle and ambiguity contexts respect
  `MaxDiagnosticReferences`; returned slices are defensive copies.
- **T11 — error precedence:** table tests cover simultaneous invalid target,
  missing/wrong-kind reference, cycle, and output-limit conditions and assert
  the first failure in the documented target/base/deepest-to-target step
  traversal, including the resolve-before-projected-limit rule.
- **T12 — non-disclosure:** composition error strings may contain only code and
  their caller-supplied bounded primary `SourceID`/pointer; they never include
  declaration references, arguments, attributes, contexts, or other payload.
  CLI tests separately prove that generated source IDs are not absolute paths.
- **T13 — diagnostic text bounds:** generated pointers are canonical and within
  `MaxPointerBytes`; overlong decoded pointers fail atomically. Every
  package-produced error string is valid UTF-8 and at most `MaxErrorTextBytes`,
  including errors built from maximum-size accepted source IDs and pointers.

## 6. CLI YAML and legacy compatibility

- **L01 — YAML equivalence:** the documented YAML maps to the same
  `AliasDocument` as strict JSON/constructor input without introducing a second
  semantic model.
- **L02 — YAML rejection:** reject duplicate keys, anchors, aliases, merge keys,
  custom tags, directives, multiple documents, unknown fields, invalid UTF-8,
  and raw byte-limit overflow. Boundary cases cover `MaxYAMLDepth` and
  `MaxYAMLNodes`; boolean, integer, float, timestamp, binary, and null scalars
  are rejected rather than coerced to strings. Depth starts at one for the root
  mapping; node counts include keys and exclude the document wrapper. The walk
  is iterative.
- **L03 — YAML atomicity:** `DecodeAliasDocumentYAML` rejects a nil target, and
  every adapter failure leaves a non-zero target unchanged and returns no
  partial document or constructor input.
- **L04 — translation input:** canonical absolute workspace/alias paths, contained
  alias location, exact derived workspace-relative slash `SourceID`, effective
  image, and closed `ImageSource` values are validated before adapter invocation.
  Path cases cover non-canonical, equal-root, outside-root, absolute/mismatched
  source IDs, backslashes, dot segments, and byte overflow. The image matrix
  covers explicit-image/alias-source, inherited-image/workspace-or-global-source,
  empty-image/zero-source, and every inconsistent combination.
- **L05 — result invariants:** checked constructors accept exactly one of a valid
  translated recipe or non-empty legacy-only reason/remedy and reject every
  mixed, empty, unknown-status, or zero-value state. Legacy-only messages count
  UTF-8 bytes, accept `limit-1` and `limit`, reject `limit+1` atomically, and
  preserve valid UTF-8.
- **L06 — prepare translation:** a deterministic fake provider receives an
  unchanged prepare definition and fully materialized defaults, and its complete
  recipe is returned as `translated` with an immutable declaration.
- **L07 — run classification:** every legacy run alias returns
  `legacy_semantics_unsupported` without invoking the provider adapter.
- **L08 — unavailable/default classifications:** nil provider and unavailable
  effective default produce `runtime_v2_provider_unavailable` and
  `legacy_default_unavailable` respectively, with actionable bounded messages;
  run classification, provider absence, default absence, and kind mismatch obey
  their documented precedence.
- **L09 — kind handshake:** lower-case exact adapter kind succeeds; empty,
  differently cased, or mismatched kinds are contract errors and never yield an
  approximate declaration.
- **L10 — provider-declared unsupported input:** an adapter may return a precise
  `legacy_only` result for unsupported arguments; ordinary lack of support is
  not surfaced as an operational error.
- **L11 — provider conformance:** a reusable suite requires every production
  adapter to cover all supported file-bearing arguments, materialize
  alias-relative paths into the explicit provider/workspace form exactly once,
  apply defaults, reject unsupported forms, and return immutable declarations.
  Equivalent fully materialized inputs produce equal declarations regardless of
  diagnostic `SourceID`/`ImageSource`; adapters never copy that provenance into
  semantic declaration fields. A fake proves the suite itself; no provider is
  claimed supported until it runs the suite.
- **L12 — current CLI regression:** `plan`, `prepare`, `run`, `alias create`, and
  `alias check` keep their existing parsing, execution path, and records; merely
  importing or invoking classification cannot select Runtime v2 execution.
- **L13 — hostile adapter contract:** nil and typed-nil adapters, zero result with
  nil error, result plus non-nil error, invalid/oversized legacy-only result, and
  panic-free kind mismatch return the documented classification or zero-result
  contract error without leaking a partial declaration.

## 7. Conformance, fuzz, dependency, and sequencing gates

- **X01 — external API conformance:** an external-package test constructs,
  expands, inspects, serializes, and decodes values using only the documented
  exported API.
- **X02 — downstream identity check:** an external integration test gives graphs
  to the same explicit deterministic resolver test provider. Differently named
  graphs with equal expanded declarations produce equal resolved identity, and
  trace changes never enter resolver inputs. Changing an identity-bearing child
  declaration or swapping two distinct transform steps changes the resolved
  recipe fingerprint and resulting StateID through the existing semantic core.
- **X03 — fuzz document JSON:** seeded with every valid variant and boundary;
  fuzzing never panics, accepts trailing/duplicate data, mutates a receiver on
  failure, or returns a value that fails immediate revalidation.
- **X04 — fuzz trace JSON:** seeded with both goldens and graph mutations; every
  accepted trace satisfies all dense-node, parent, variant, pointer, and origin
  invariants.
- **X05 — fuzz graph expansion:** small bounded generated catalogs either expand
  to a deliberately recursive reference flattener with an independent data model
  or return a structured error, without production recursion, nondeterminism, or
  partial output.
- **X06 — race/concurrency:** concurrent expansion and serialization of one
  small immutable catalog pass `go test -race`; caller mutation cannot race
  through a retained internal reference.
- **X07 — dependency boundary:** Runtime module dependency checks continue to
  allow only the standard library and its own module packages; neither CLI/YAML,
  resolver implementation, engine, storage, nor execution packages enter
  `composition`.
- **X08 — staged release boundary:** the public composition package and external
  consumer pass before the CLI module pins a published immutable version. The
  compatibility slice compiles without a parent/subpackage import cycle and does
  not become a default-runtime cutover gate.
- **X09 — release success matrix:** the clean external consumer expands
  factory-only, one-step, multi-step, and nested recipes and verifies exact
  transform order plus the public alias-document and expansion-trace schemas.
- **X10 — release identity and failures:** that consumer proves alias renaming is
  identity-neutral, step reordering is identity-bearing, and missing, cycle, and
  cross-source ambiguity failures retain their stable public codes and ordered
  diagnostics.
- **X11 — release cache compatibility:** the same consumer constructs and
  round-trips `resolver.CacheRecord` through exported APIs and decodes then
  byte-for-byte re-encodes the published v0.2.0 wire fixture. This is historical
  v0.3.0 release evidence; the v0.5.0 consumer uses the canonical record from
  [the new resolver plan](runtime-v2-canonical-resolver-tests.md).
- **X12 — one staged/public oracle:** the staged file-proxy gate and the public
  proxy/checksum gate run the same checked-in clean-consumer test sources. The
  public gate must not substitute a narrower inline smoke test.

## 8. Coverage and acceptance

Deterministic package tests run with `go test ./... -count=1`; the external
consumer runs with `GOWORK=off`. Fuzz seeds run in the ordinary PR suite and
timed campaigns follow the existing Runtime v2 nightly/release policy. Race
tests run on the supported race-enabled CI platform. The 10,000-node and
32-MiB boundary fixtures run once in a deterministic resource suite, not under
race or timed fuzzing.

Coverage is measured separately for `runtime-go/composition` and
`frontend/cli-go/internal/alias/runtimev2` with per-line reports. The target is
100%; 95% is the minimum. Any shortfall follows the repository's separately
approved remediation loop.

The composition-module release slice is acceptable when D01-D10, C01-C08,
E01-E14, T01-T13, X01-X12, and its staged/public consumer gates pass. Those
gates published runtime-go v0.3.0 and closed #109. The extracted #137
compatibility slice passes L01-L13 independently; executable CLI `legacy_only`
classification was not a runtime-go v0.3.0 publication gate.
