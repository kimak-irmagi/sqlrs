# Observe only fixed structural classes of an unknown trace prefix

- Conversation approval recorded: 2026-11-10, Asia/Novosibirsk; exact reply
  timestamp unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Status: component amendment explicitly approved by user reply after critical
  and repeat review. Separate PX01–PX05 inventory subsequently approved before
  tests/implementation. Small checks pass; actual remote PX05 is still pending.
- Question: run 42632130376 proves pending_entry/other but does not identify the
  prefix form. How to obtain useful evidence without exporting private trace text?
- Alternatives: retry with the same uncertainty; export a raw/scrubbed prefix;
  or emit two fixed grammar/call-class fields from the already-read private record.
- Decision: user chose fixed structural labels. Add tracePrefix/traceKnownCall
  only for an other-shaped failure. Fixed allowlists, anchored matching before
  arguments, no strings/PIDs/numbers/raw lengths/hashes. Preserve old context/code,
  failure and unchanged eight-member protocol/binaries. Exact tests are separate.
- Rationale: refine the observation limit without guessing successful execution,
  admitting a new syntax, expanding raw-data access or inferring a product cause.

See the [approved amendment](../architecture/issue89-performance-diagnostic-design.md#approved-structural-prefix-detail-for-the-remaining-other-case)
and [approved tests](../architecture/issue89-performance-diagnostic-tests.md#proposed-px01px05-structural-prefix-observation).
Earlier closed-context/localisation decisions remain in force; none is superseded.

## Approve PX01–PX05 before prefix tests and code

- Conversation date: 2026-11-10, Asia/Novosibirsk; exact reply time unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Question: proceed with the separately reviewed prefix-observation tests?
- Alternatives: retain the observation limit; or test grammar/privacy, unchanged
  real refusals and additive report wiring before the same remote retry.
- Decision: user explicitly approved PX01–PX05. Repeat old-test audit found no
  contradictions; preserve existing exact context fields with separate metadata.
- Rationale: obtain finer closed evidence without raw export, accepting new syntax
  or changing the product. New tests precede implementation and native evidence.

Native run 42665130138 subsequently verifies actual prefix transport:
pending_entry/other, unknown_syscall/other. Its
[original incomplete report](../architecture/issue89-diagnostic-evidence-2026-11-10/42665130138/jhipster-diagnostic.json)
is retained. No numeric payload or exact syscall is inferred. The separate
raw-call grammar proposal is not approved and does not change current rejection.
