# Observe only fixed structural classes of an unknown trace prefix

- Conversation approval recorded: 2026-11-10, Asia/Novosibirsk; exact reply
  timestamp unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Status: component amendment explicitly approved by user reply after critical
  and repeat review. Separate PX01–PX05 test inventory awaits approval.
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
and [proposed tests](../architecture/issue89-performance-diagnostic-tests.md#proposed-px01px05-structural-prefix-observation).
Earlier closed-context/localisation decisions remain in force; none is superseded.
