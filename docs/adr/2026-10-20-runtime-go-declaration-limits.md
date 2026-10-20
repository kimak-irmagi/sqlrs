# Independent unresolved declaration-value allowance

## Decision 1: enlarge source values without changing resolved/diagnostic limits

- Conversation timestamp: 2026-10-20 (Europe/Moscow).
- GitHub user id: @evilguest (41718235).
- Agent name/version: Codex / GPT-5.
- Question: how can a provider carry a bounded larger unresolved specification?
- Alternatives: retain the small source cap; add recursive values; use numbered
  transport fragments; raise every string limit; or give declarations an
  independent larger allowance under the existing aggregate JSON bound.
- Decision: the last alternative, explicitly approved by the user. Keep the
  existing string field/wire model, allow 256 KiB per declaration value, retain
  256 fields and 4 MiB total JSON, and preserve diagnostic/resolved/canonical
  limits. Implementation is #159; publication is independently #160.
- Rationale: source specifications and resolved/diagnostic strings have different
  needs. No new recursive model, registry, private codec or semantic schema is
  necessary. Scope the increase instead of weakening unrelated bounds.
- Compatibility: preserve normalized bytes of declarations which meet the new
  aggregate rule, StateIDs and immutable ownership. Aggregate admission is
  intentionally tightened, including compact raw inputs whose emitted JSON
  would exceed 4 MiB; this is not a promise to retain every former success.
  Earlier declaration/diagnostic ownership ADRs remain valid;
  their historical release contracts are not replaced. No accepted ADR is obsolete.

## Decision 2 (accepted): account whole nested JSON before aggregate encoding

- Conversation timestamp: 2026-10-20 (Europe/Moscow).
- GitHub user id involved in the parent decision: @evilguest (41718235).
- Agent name/version: Codex / GPT-5.
- Question: how can larger child fields retain a true 4 MiB whole-document bound?
- Alternatives: marshal a potentially oversized whole value and check later;
  duplicate JSON escaping; or incrementally account bounded members/envelopes
  using encoding/json before whole-value copying/encoding.
- Decision, approved after correction/review on 2026-10-20: the last alternative. A small internal
  root-package budget traverses existing wire projections and quotes only
  individually bounded scalar strings through encoding/json. Check the raw
  scalar lower bound before quoting, then charge exact escaped bytes; stop all
  traversal on failure before aggregate cloning/encoding. Do not marshal whole
  child documents or rely on Encoder writing to a limited writer. Local child
  callbacks prove early exit; public-call-path review/tests prove use before
  cloning, not just correctness of a private counter. Existing constructors/
  codecs own types and atomic updates. Nested factory/transform/recipe envelopes
  charge children and metadata. Field checks receive the declaration-specific
  or original diagnostic allowance explicitly. No new persistent ownership,
  global observer, stored source copy or size cache is introduced.
- Rationale: checking fields alone admits 64 MiB aggregates; checking only raw
  decoded bytes misses normalized escaping growth. Standard JSON accounting
  avoids another string/canonical encoder and bounds intermediate allocations.
- Approval boundary: detailed structure and revised DL01–DL08 were approved by
  the user's explicit response on 2026-10-20. Contradictory existing assertions require a separate
  explicit decision. No release version or merge/publication is authorized here.

## Decision 3: preserve decoder semantics while separating three size boundaries

- Conversation timestamp: 2026-10-20 (Europe/Moscow), user requested corrections
  after the critical review of the design and DL01–DL08.
- GitHub user id: @evilguest (41718235).
- Agent name/version: Codex / GPT-5.
- Question: should larger values also change shared JSON case matching and
  error handling, or treat all JSON byte measurements as interchangeable?
- Alternatives: silently harden the shared decoder and conflate byte limits;
  or keep decoding behavior and explicitly measure value/raw/normalized bytes.
- Decision for the revised design: the latter. Preserve current case matching,
  exact duplicate/unknown-member handling, null rules and atomic updates.
  Remove the proposed case-alias rejection from DL03. Test decoded UTF-8 value
  bytes, received raw JSON and standard normalized output independently. Include
  whitespace/escape growth and distinguish DecodeJSON/direct UnmarshalJSON
  from encoding/json preprocessing and syntax errors. Existing structured
  error codes/paths/wrappers remain; no new unknown-key sanitization contract.
- Rationale: raw bytes and emitted bytes can lie on opposite sides of the bound;
  the standard decoder can trim outer whitespace before delegation. A size
  increase must not silently introduce unrelated parser/security requirements.
- Approval boundary: the user requested the review correction, not immediate
  implementation or alteration of old assertions. Revised tests/structure were
  subsequently approved on 2026-10-20. Old assertions still need separate approval.
  No earlier accepted wire/identity ADR is obsolete.

## Decision 4: account for newly fallible existing composition calls

- Conversation timestamp: 2026-10-20 (Europe/Moscow), same review correction.
- GitHub user id: @evilguest (41718235).
- Agent name/version: Codex / GPT-5.
- Question: how do existing composition snapshots/expansion handle new
  declaration-aggregate constructor/encoding rejections?
- Alternatives: keep discarding errors based on the old count-only invariant;
  or propagate newly possible size failures and recheck copy invariants.
- Decision for the revised design: the latter. ExpandRecipe maps an aggregate
  size rejection to existing CodeExpansionTooLarge/source/pointer and returns a
  zero expansion and trace. Review ExpandTransform, copyFactory/copyTransform
  and expanded accessors: handle newly possible errors or prove the specific
  bounded immutable invariant. Never replace a valid value with an empty DTO or
  let a failed encoding's zero length bypass rejection. Preserve other existing
  mappings and subsequent valid use of the same catalog.
- Early-allocation refinement: charge each prospective output before copying/
  appending it or its trace entry, using existing composition checked addition.
  Count the bounded public empty-step recipe envelope/factory and separately
  admitted child encodings/commas; never measure the whole oversized expansion.
  The root's own preflight guards those bounded encodings and its final
  constructor remains authoritative. No public sizing API or source cache.
- Mapping audit also covers NewAliasDocument/DecodeAliasDocumentJSON: whole
  source size overflow retains CodeExpansionTooLarge/root location, whereas
  invalid fields retain CodeInvalidDocument/nested location. Do not let an
  earlier root aggregate error change category through an old catch-all wrapper,
  or reclassify all individual-field size errors as aggregate overflows.
- Rationale: the new root constructor can fail where the current caller ignores
  its error; correct local rejection alone would otherwise become false success.
- Approval boundary: this is internal compatibility of the existing optional
  composition package, not a new alias API/model or a downstream YAML dependency.
  DL04 is extended within the same eight groups. Existing assertions are not
  weakened; corrected structure/tests were subsequently approved on 2026-10-20.
