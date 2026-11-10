# Identify every summary clock rejection without changing acceptance

- Conversation approval: 2026-11-10, Asia/Novosibirsk; exact reply time unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Status: component amendment explicitly approved after critical and repeat
  review. Separate CW01–CW07 inventory proposed; tests/code not authorised yet.
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
