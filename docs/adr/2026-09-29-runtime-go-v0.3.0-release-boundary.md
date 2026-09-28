# Runtime Go v0.3.0 release boundary

Status: approved for issue #133 on 2026-09-29.

## Decision 1: release from a readiness descendant of PR #135

Conversation timestamp: 2026-09-29 00:56 Asia/Novosibirsk (2026-09-28 17:56
UTC). GitHub user ID: @evilguest (41718235). Agent: OpenAI Codex (GPT-5).

Question: which commit should publish runtime-go v0.3.0 after PR #135 tightened
the release workflow?

Alternatives: tag the exact PR #134 merge; tag the exact PR #135 merge; or merge
a small issue-#133 release-readiness descendant of #135 and tag that commit.

Decision: merge a release-readiness PR descended from PR #135. Its merge commit
is the single candidate for both `backend/libs/runtime-go/v0.3.0-rc.1` and
`backend/libs/runtime-go/v0.3.0`. Run manual validation for that exact commit
before creating the RC tag; promote GA only after the RC tag workflow and public
proxy/checksum verification succeed.

Rationale: PR #135 supplies the hardened publication rules, while the descendant
can add evidence for the already-merged composition and cache surfaces without
changing the immutable candidate between RC and GA.

## Decision 2: reuse one clean consumer at both proxy boundaries

Conversation timestamp: 2026-09-29 00:56 Asia/Novosibirsk (2026-09-28 17:56
UTC). GitHub user ID: @evilguest (41718235). Agent: OpenAI Codex (GPT-5).

Question: how should release gates prove the composition and CacheRecord public
surfaces are present in both the staged archive and the published module?

Alternatives: retain the narrow inline public-proxy smoke test; duplicate a
second larger public-proxy test; or run the same checked-in external-consumer
sources against the staged file proxy and the public proxy/checksum database.

Decision: use the same checked-in clean-consumer suite at both boundaries. It
covers factory-only, one-step, multi-step, and nested composition; exact order,
rename invariance, cycle, missing, and ambiguous references; and public
CacheRecord construction plus byte-compatible v0.2 fixture decoding.

Rationale: one oracle prevents staged and public verification from drifting and
tests the exported package surface without a workspace or `replace` directive.

## Decision 3: preserve the product migration boundary

Conversation timestamp: 2026-09-29 00:56 Asia/Novosibirsk (2026-09-28 17:56
UTC). GitHub user ID: @evilguest (41718235). Agent: OpenAI Codex (GPT-5).

Question: does publishing the library classify or migrate existing product data?

Alternatives: imply automatic migration; make executable CLI compatibility a
release blocker; or state the legacy boundary and keep CLI integration separate.

Decision: release notes explicitly classify existing `v0.1.1-rc.6` state,
cache, queue, and alias data as unchanged legacy data. Runtime-go v0.3.0 performs
no automatic migration or reinterpretation. Executable CLI `legacy_only`
classification remains the later compatibility slice of issue #109 and is not a
library publication gate.

Rationale: the module release makes the public contracts consumable without
silently changing product execution or claiming integration that has not landed.
