# Managed lifecycle review fixes

Conversation timestamp: 2026-09-16 16:10 Asia/Novosibirsk (09:10 UTC).
GitHub user ID: @evilguest. Agent: Codex (GPT-6).
Status: accepted; the user requested fixes for the three PR review findings.

## Cancellation cleanup

Question: how should failed-job cleanup proceed after cancellation?
Alternatives: inherit the cancelled context; retry without a deadline; use an
independent bounded context.
Decision: detach cancellation and apply the existing 15-second cleanup deadline.
Rationale: normal cancellation must stop the container without unbounded cleanup;
uncertain stop must still preserve the physical clone.

## Unpublished state seals

Question: how can a later job recover from failed snapshot capture/publication?
Alternatives: delete every seal on retirement; require manual repair; replace
only an unpublished seal whose owner is already retired.
Decision: use the last option, under the existing state build exclusion, with
matching identity and an atomic check that no published state row exists.
Rationale: published capture provenance and active owners remain protected;
failed capture does not permanently poison the deterministic state key.
No schema or public API change is needed.

## Instance access concurrency

Question: how should execution remain excluded from retirement without blocking
unrelated instances?
Alternatives: one global mutex; release all exclusion before execution; retain
a cancellable fence for each instance reference.
Decision: per-instance fences cover activation, resolution, use and retirement.
Physical-operation metadata retains its short independent exclusion. Remove
unused fence entries after the last holder/waiter leaves.
Rationale: preserve retirement ordering while allowing independent instances to
make progress and cancelled waiters to return promptly.
