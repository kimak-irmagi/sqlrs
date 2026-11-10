# Identify every summary clock rejection without changing acceptance

- Conversation approval: 2026-11-10, Asia/Novosibirsk; exact reply time unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Status: component amendment explicitly approved after critical and repeat
  review. Separate CW01–CW07 inventory subsequently approved before tests/code.
- Question: native UE07 passed trace decoding but reported only `clock` during
  summary. Guess the failing check, change clock domains, or collect complete
  closed evidence while retaining refusal?
- Alternatives: relax exact duration equality on speculation; choose a new clock
  mapping now; or expose the four existing rejection reasons and bounded spans.
- Decision: retain rejection and add a private reason-only/bounded-duration error
  plus separate Go/Node accessors and marker, only on the failed clock path.
  Reasons are wall_order, elapsed_order, duration_mismatch, interval_order, with
  unavailable none. Publish a pair of positive relative integer nanosecond spans
  only within the existing 12-minute test ceiling; no calendar endpoints, PID,
  raw trace, arbitrary messages or retained time.Time. A pure private window
  validator owns exactly the original first three checks in their original
  order, accepting independent wall/elapsed inputs. Reversed interval checking
  remains at its old loop boundary. Preserve errors.Is and fixed Error, earlier
  CX/PX/RD shapes, successful summaries, measured engines and original Actions
  protocol. Approve tests separately before implementation.
- Rationale: a nanosecond-exact wall/monotonic equality is a plausible refusal
  source, not established by `clock`. Obtain all branch facts in one observation
  without hiding time uncertainty or pretending it explains the product slowdown.
  A tolerance, affine mapping or monotonic stripping is explicitly not approved.

See the [approved component amendment](../architecture/issue89-performance-diagnostic-design.md#approved-complete-closed-context-for-summary-clock-rejection)
and [proposed CW tests](../architecture/issue89-performance-diagnostic-tests.md#proposed-cw01cw07-closed-clock-rejection-context).
Reviewed the earlier localisation, raw-rejection and terminal-unavailable ADRs:
none is obsolete. This extends failed-observation metadata only, not acceptance
or P01b. Native UE07 evidence stays unchanged in its original report.

## Approve CW01–CW07 before implementation

- Conversation date: 2026-11-10, Asia/Novosibirsk; exact reply time unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Question: proceed with the separately reviewed inventory without weakening
  time validation or rewriting the old summary/context tests?
- Alternatives: retain the current observation limit; or test the actual ordered
  scalar checks, real summary, safe metadata and native publication first.
- Decision: user explicitly approved CW01–CW07. Repeated old-test audit found
  no required change; write new tests before code. Retain existing checks,
  signatures, time sources, old helper shapes and all original native members.
- Rationale: distinguish all clock rejection reasons with bounded relative
  evidence without guessing drift, fabricating time.Time or waiving P01b.

Native CW07 42789132449 subsequently proves actual clock-context transport and
the precise duration_mismatch (wall/elapsed 73672499931/73672499940 ns, −9 ns).
The original incomplete artifact is retained in the test inventory. A measured-
only bounded-alignment component proposal is separate and not approved by this
ADR; strict helpers, existing rejection and the product gate remain in force.
