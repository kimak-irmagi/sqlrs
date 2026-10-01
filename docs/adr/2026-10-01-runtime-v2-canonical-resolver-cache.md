# Canonical Runtime v2 resolver result and cache

Conversation timestamp: 2026-10-01 00:34 Asia/Novosibirsk (2026-09-30 17:34 UTC).
GitHub user ID: @evilguest (41718235). Agent: Codex (GPT-5).
Approval timestamp: 2026-10-01 Asia/Novosibirsk.
Status: resolver-boundary and wire-format decisions accepted for issues #146
and #147.

## Question

With no requirement to preserve intermediate v0.x APIs, should canonical-v1
resolution replace the existing resolver result
or introduce a second resolver result, registry, manager, and cache?

## Alternatives

1. Preserve the existing resolver and add a parallel canonical API. This
   duplicates the mechanism solely for compatibility with consumers that do
   not exist.
2. Convert old string fields to typed canonical fields. This loses field-kind
   information and silently changes identity meaning.
3. Change the existing resolver result to canonical-v1, version its record,
   and migrate the actual in-repository users. Previously published tags stay
   immutable; old records are rejected rather than converted.
4. Store only canonical fingerprints. This cannot reconstruct full typed
   fields after restart or support provider validation.

## Decision and rationale

Choose alternative 3. There is one current resolver contract. The existing
workspace-file provider and SQLite cache adapter migrate with it. Persistent
decode uses the selected provider's schema-bound builder and complete typed
values, not unchecked construction. The new record rejects the old format;
there is no migration or reinterpretation. The established separation of
resolution, revalidation, and acquisition and the atomic publication
discipline remain. Version 0.5.0 may contain breaking source changes, while
already published tags remain immutable. Only successful RC/GA publication
completes #147.

Issue #146 was revised to permit the breaking v0.5.0 source change and include
migration of the existing workspace-file reference provider.

## Wire-format addendum (accepted)

Conversation timestamp: 2026-10-01 19:04 Asia/Novosibirsk (12:04 UTC).
GitHub user ID: @evilguest (41718235). Agent: Codex (GPT-5).
Approval timestamp: 2026-10-01 Asia/Novosibirsk.

Question: what exact persistent representation lets independent fixtures
verify complete typed identity after restart, without using the encoder under
test as their oracle?

Alternatives: persist only a fingerprint and field commitments; represent
canonical structured values as ordinary JSON; or store the complete typed
payload and canonical binary envelope in a closed, versioned JSON record.
For integrity, use either an unspecified checksum over parsed values or a
fixed checksum preimage over the emitted typed JSON object.

Decision: use the closed
`sqlrs.resolution-cache.canonical.v1` JSON grammar in the
[component structure](../architecture/runtime-v2-canonical-resolver-structure.md#normative-v050-wire-grammar).
Canonical structured values and identity bytes use standard padded base64 so
their original bytes survive; text and secret references have distinct payload
arms. The workspace-scope/key grammar keeps the existing length-prefixed
binary construction with a new cache domain. The checksum is SHA-256 of the
specified `encoding/json.Marshal` representation with `checksum` omitted.
Strict limits and schema-bound reconstruction precede provider validation.

Rationale: exact independent fixtures can detect a symmetric encoder/decoder
defect, full payloads preserve typed field meaning, and domain-separated keys
reject old records. The checksum detects accidental corruption but does not
authenticate a cache owner who can consistently rewrite a record.
