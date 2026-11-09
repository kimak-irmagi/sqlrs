# Preserve uncertain exec duration without claiming a successful launch

- Conversation date: 2026-11-09, Asia/Novosibirsk; exact reply time unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Status: component amendment and, subsequently, the separately reviewed
  EU01–EU06 inventory explicitly approved by the user. Small implementation passes;
  a post-approval EU04 field-policy conflict awaits separate clarification.
- Question: native diagnostic run 42612253733 again rejected a terminal process
  with a pending exec whose start was no later than the final API boundary.
  Keep rejecting it entirely, pretend a successful command, or retain uncertainty?
- Alternatives: retain full rejection and design another observer; infer a
  successful image; or explicitly report only the known attempt-to-exit duration.
- Decision: explicit measured decoding may emit `exec.unconfirmed`, without
  program/PID/argv, only for a valid observed entry with the same process's
  terminal record. End any previous confirmed image at attempt start. Keep
  strict/default and existing late-cleanup calls unchanged; do not accept malformed
  streams, EOF as exit, or unproved cleanup. Independent CLI counts remain intact.
- Rationale: known duration is useful even when execution outcome is not known.
  Neither a terminal record nor a zero exit proves that a requested JVM/CLI began.
  Material unknown time still limits causal attribution and does not waive P01b.

See the approved [design](../architecture/issue89-performance-diagnostic-design.md)
and approved [tests](../architecture/issue89-performance-diagnostic-tests.md#proposed-eu01eu06-unconfirmed-exec).
The earlier [localisation decision](2026-11-08-liquibase-performance-localisation.md)
remains in force: unchanged binaries, exact localisation before a product fix,
no raw trace export and no performance waiver. No prior ADR is superseded.

## Approve the separate EU01–EU06 test inventory

- Conversation date: 2026-11-09, Asia/Novosibirsk; exact reply time unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Question: implement the reviewed tests for the approved uncertainty boundary?
- Alternatives: retain the observer failure; or test explicit mode separately
  while preserving existing strict checks and the original speed criterion.
- Decision: user explicitly approved EU01–EU06 before new tests/implementation.
- Rationale: independently check intervals, absent evidence, windows/counts,
  private report parsing, explicit wiring and the same eight remote members.
  A subsequent audit found EU04's extra-field rejection conflicts with DG09's
  existing safe field dropping. That subcase is separately queried, not silently
  implemented or used to rewrite the old test. Independent cases proceed.
