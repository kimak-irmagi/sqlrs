# Preserve main-path test boundaries with private capture v2

- Conversation date: 2026-11-06, Asia/Novosibirsk; exact reply timestamp unavailable.
- GitHub participant: @evilguest.
- Agent: Codex / GPT-5.
- Status: Accepted, limited to OT-MP02 and OT-MP03.

## Decision 1: OT-MP02, reusable runner fixture

Question: how should the existing three-JVM, incomplete-manifest and surviving-child
checks reach their intended boundaries after the approved private/v2 protocol?

Alternatives: retain host-shared output and an arbitrary waiting parent; weaken the
private protocol to accept that fixture; update only the fixture to the accepted
serve/readiness/private-output protocol.

Decision: update only TestLiquibaseStepCaptureReusableRunnerDocker. Keep its three
variants and real JVMs in one exact parent. Verify native readiness, use explicitly
declared lower-level prior-publication authority, and remove an intentionally damaged
manifest only inside that exact test-owned container. Prove a complete private capture
and the live untrusted PID before requiring the idle-probe rejection. Bound the live
fixture by mandatory parent cleanup rather than a short timed sleep. No SQLite
publication or recovery qualification is inferred from helper-level setup.

Rationale: preserve the old requirements without false early refusals or weakening
production ownership. Product behavior, schema and API are unchanged.

## Decision 2: OT-MP03, subprocess startup and cancellation

Question: should every bounded-output case use the same one-second deadline even
when a cold test-process start itself exceeds that budget?

Alternatives: retain the shared second; retry/warm up until green; increase every
case's budget; separate ordinary startup from the targeted cancellation cases.

Decision: only in TestCaptureAccessBoundedBinaryRunner, give non-cancellation cases
five seconds. Retain one second for stall, immediate cancellation for already_cancelled,
all 11 cases, exact bytes, stdout/stderr caps, no partial success or diagnostic leakage,
and the existing seven-second cleanup bound. No product timeout changes or hidden
warmup/retry are authorized.

Rationale: startup timing is not the cancellation requirement; the target cancellation
and resource bounds must remain tested independently.

## Review and consistency

Both narrow proposals underwent critical and repeated review before approval. They
implement the accepted [private container ownership](../architecture/liquibase-container-component-structure.md),
not a replacement for it. No prior ADR requiring host-shared captures or a universal
one-second test startup budget was found. Earlier genuine test failures remain recorded;
focused success does not establish full #89 or coverage acceptance.
