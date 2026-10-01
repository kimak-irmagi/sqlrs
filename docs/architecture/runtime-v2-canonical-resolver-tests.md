# Runtime v2 canonical resolver: test design

Status: test list approved by @evilguest for #146/#147, 2026-10-01.
Requirements: [interaction flow](runtime-v2-canonical-resolver-flow.md) and
[component structure](runtime-v2-canonical-resolver-structure.md).

Tests specify the public contract, not private helper implementation. The
v0.5.0 resolver result is canonical; historical v0.2.0–v0.4.0 release fixtures
remain evidence for those published tags, but the current record decoder does
not accept their string-only cache records. Stage labels distinguish checks
possible on a PR from publication checks that require a reviewed merge.

| ID | Stage | Required evidence |
| --- | --- | --- |
| CR01 | PR | A package-external consumer uses a supported schema capability, registers one fake provider in the existing registry, and obtains `CanonicalResolvedExtensionIdentity` through `ResolveCurrent` without internal imports or a second resolver API. Provider authorship and ordinary consumption remain separate trust roles. |
| CR02 | PR | Four provider-shaped fakes for file, OCI, Git, and package use distinct declaration schemas, normalization rules, identity schemas, and typed field sets; each resolves and persists through the same manager/cache. Dispatch uses role/owner/kind/specification schema; duplicate, nil, invalid, mismatched-schema, and unsupported providers fail with stable codes. No network or real provider client is used. |
| CR03 | PR | An instrumented provider proves normalization runs before key construction; equivalent spellings share a key, while different physical workspace scopes and resolver semantic versions do not. Detectable schema mismatches (field kind, disclosure, or required fields) under the same descriptor are rejected on load or validation, never reused as `CURRENT`. A semantic-only change cannot be detected by the codec and requires a documented semantic-version bump. |
| CR04 | PR | Canonical identity accessors return provider, kind, schema, and sorted typed fields without letting a caller mutate the identity. Codec encoding rejects identity/schema mismatch. |
| CR05 | PR | Reviewed fixed fixtures, derived independently from the normative wire grammar rather than the encoder under test, pin JSON, canonical bytes, field commitments, and fingerprint for text, nested canonical value, and secret reference (including protected fields). Encode/decode also round-trips their exact kinds, full payloads, and disclosures; references contain no credential value. |
| CR06 | PR | Schema-bound decode rejects missing/extra/duplicate fields, wrong kind or disclosure, absent required field, invalid canonical-value envelope, malformed secret reference, forged commitment/fingerprint, noncanonical order, unknown/duplicate JSON members, invalid UTF-8/unpaired surrogates, trailing data, nulls, and size/depth/node limits without yielding a partial identity or unbounded allocation. Mutations cover each nesting level. |
| CR07 | PR | `CacheRecord` matches fixed independent record bytes, workspace-scope/key digest, and checksum from the normative grammar; it round-trips complete canonical resolution/evidence, copies caller buffers, and matches every key constituent. Mutants with recomputed valid checksums still reject wrong key/workspace/descriptor/schema, normalized declaration, typed payload, commitment, and fingerprint; wrong checksum/version, over 4 MiB/4096 fields/64 JSON levels, and duplicate/unknown/trailing JSON at outer and nested levels also reject. The new decoder rejects the published string-only record; an isolated consumer compiled against the immutable old tag rejects a new record. Neither version converts the other. |
| CR08 | PR | A valid cached `CURRENT` result skips fresh `Resolve` and `Acquire`, preserves canonical identity bytes, validates refreshed evidence, stores it before success, and reports exact hit/freshness. Invalid refreshed evidence and store failure return errors rather than restart-safe success. |
| CR09 | PR | `STALE` and `UNKNOWN` each trigger fresh resolution without acquisition and remain distinct in outcomes and failed-resolution errors. An explicit failure matrix covers miss, corrupt/incompatible load, invalid loaded result, invalid fresh result, invalid revalidation status, resolve/store failure, permission/I/O failure, and cancellation; it checks operation, code, prior status/reason, call counts, and absence of a partial result. |
| CR10 | PR | A child process writes a typed record and a different process loads it. Termination before atomic replacement leaves the old record or a miss; termination after replacement exposes a complete new generation to an ordinary process restart. A separate injected directory-sync error is returned as a durability error without a power-loss survival promise; successful file/directory sync calls are observed. Process termination does not claim to simulate power loss. Concurrent processes/readers never observe partial records; trusted-root/no-link rules remain enforced. |
| CR11 | PR | Bounded pruning removes only eligible canonical entries and ignores active temporary files, malformed outer envelopes/checksums/keys, and old-version records without following links. It does not claim to validate provider-schema-bound identity fields without that schema. |
| CR12 | PR | Migrated workspace-file provider emits canonical `content.digest`, preserves path/link/special-file safety and explicit digest-verified immutable `Acquire`, and never acquires implicitly. Native CI on Linux, macOS, and Windows repeats unchanged `CURRENT`, same-size changed bytes with restored mtime never `CURRENT`, deletion/non-file/size-change `STALE`, and unsupported or incomplete evidence `UNKNOWN`; enabled filesystem classes must retain their mandatory native gate. |
| CR13 | PR | Migrated SQLite adapter uses the existing table/DDL with the new codec, survives reopen, checks indexed and embedded keys, and atomically replaces evidence. Tests distinguish an incompatible outer row version from an old inner cache schema, reject both, and verify logical-state/materialization rows and the default runtime remain unaffected. |
| CR14 | PR/RC/GA | The same checked-in external-consumer suite runs from the staged local proxy and released public proxy with `GOWORK=off`, no `replace`, pseudo-version, or sibling checkout. It proves registration, canonical resolution, `CURRENT`, all three field kinds after a separate-process restart, and old-record rejection using exported packages only. |
| CR15 | PR | Existing canonical fingerprints/StateIDs and legacy semantic-core golden data remain byte-stable. The old cache fixture remains as a rejection oracle, not a current round-trip success oracle. Current resolver, SQLite, and release-consumer tests are updated to the canonical contract; no test is skipped. |
| CR16 | PR/RC | CI and release workflows explicitly execute the new identity/cache-decoder fuzz target, existing path fuzz target, unit/race suites, native platform tests, conformance bundle, dependency inventory, and per-line coverage on the candidate commit. Coverage targets 100% with 95% minimum; if below target, a separate line-level remediation plan and approval are required by repository rules. |
| CR17 | RC/GA | Release gates verify v0.5.0 notes name the breaking Go API and cache schema, lack of migration/reinterpretation, license/dependency inventory, checksums, and content-addressed provenance. RC and GA tags identify the same reviewed post-merge commit; staged and public consumer results, module zip/source, checksums, and attestations agree. A PR cannot itself close #147 before these publication checks pass. |

## Oracle and execution rules

- CR05/CR07 fixtures derive from the
  [normative wire grammar](runtime-v2-canonical-resolver-structure.md#normative-v050-wire-grammar),
  which fixes JSON members, field-payload encoding/order, cache-key and
  checksum preimages, and size limits. Tests verify that grammar rather than
  silently defining it.
- Fixed expected canonical bytes, JSON, commitments, fingerprints, and cache
  checksums are reviewed from the format specification and checked in. Tests
  never generate their own expected values with the encoder being tested.
- Semantic corruption tests modify one field at a time and recompute a valid
  outer checksum with a test-only independent routine, leaving the dependent
  field commitment/fingerprint/key inconsistent on purpose. Otherwise they
  would prove only checksum rejection. Separate tests deliberately use a wrong
  checksum. A fully self-consistent rewrite by the trusted directory owner is
  not expected to be rejected: the checksum is not authentication.
- Cross-version decoding runs in two isolated module builds, not by importing
  two versions of the same Go module into one binary. PR checks can stage the
  immutable old tag locally; RC/GA checks use the public module proxy.
- Tests assert public errors/outcomes without including protected field values
  in diagnostic output. Full protected payloads may exist in the trusted cache;
  the tests must not misrepresent that as encryption or redaction at rest.
- Native continuity tests run in the existing three-platform CI matrix and
  fail when a required enabled class lacks its native proof; unknown/weak
  filesystems exercise the `UNKNOWN` fallback rather than a false `CURRENT`.

CR09 uses the existing observable error contract as its oracle: misses and
corrupt/incompatible records yield fresh-resolution outcomes with respectively
`cache_miss`, `corrupt_cache`, or `incompatible_cache` reasons; a provider-invalid
cached result yields `invalid_cached_resolution`. Failed fresh resolution after
`STALE`/`UNKNOWN` reports operation `resolve` with that exact prior status and
reason. Invalid fresh/refreshed results report `validate_resolution` and
`invalid_resolution`; load/store I/O or cancellation reports `cache_load` or
`cache_store` with the matching stable code and does not trigger fallback.

## Existing-test contradiction review after list approval

The review found three conflicting groups: resolver tests construct the old
`Resolution.Identity` and use the old cache signatures; the current
`CacheRecord` and clean-consumer tests expect a v0.2.0 cache fixture to decode
and re-encode successfully; workspace-file and SQLite tests inspect old
string-only fields. These tests must be migrated to the approved canonical
contract. The v0.2.0 fixture becomes a rejection oracle, not a round-trip
oracle. Historical semantic-core golden tests and immutable published-tag
evidence do not conflict and remain unchanged. The user approved updating the
old tests; none will be skipped. No additional contradiction was found in this
review.

## Approved coverage remediation and result

The 2026-10-02 Windows full-module profile has 4,274 covered statements out of
4,523 (94.5%); at least 23 more covered statements are needed for the 95%
minimum. The detailed per-line profile is available in the working session.
The largest uncovered files are `canonical_extension_json.go` (30 statements),
`resolver/directory_cache.go` (22), `canonical_builder_v1.go` (17), and
`json_helpers.go` (17). This plan does not relax tests or the threshold.

1. Trace the 30 codec statements to CR04–CR07. Add public-boundary tests for
   maximum-sized field sets and malformed values, names, payload arms,
   Unicode, and nesting where required. Remove branches unreachable after a
   validated schema-bound builder instead of testing impossible private states.
2. Trace the 22 directory-cache statements to CR07, CR10, and CR11. Exercise
   required read/write, sync, malformed-record, and pruning failure paths
   through controlled filesystem or narrow injected-error tests. Preserve
   cross-platform behavior; process termination does not simulate power loss.
3. Re-run complete module tests and per-line coverage. If still below 95%,
   inspect `canonical_builder_v1.go` and `json_helpers.go` next, adding only
   public-behavior tests or removing dead code. Request approval before a
   further coverage iteration if this one remains below the accepted minimum.

Historical semantic goldens and the approved canonical test list remain in
force; no test is skipped.

The user approved this plan. Boundary tests now cover malformed typed identity
JSON, Unicode and nesting, malformed cache JSON, and the 4,096-field limit in
both encoding and decoding. The 2026-10-02 full runtime-go module run passes at
4,297/4,523 covered statements (95.003% without rounding), above the 95%
minimum. No impossible internal-state tests or coverage-only production seams
were added; the remaining optional branch clean-up is not required for this
release.
