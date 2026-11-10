# Preserve fixed raw-grammar rejection detail without accepting new records

- Conversation approval recorded: 2026-11-10, Asia/Novosibirsk; exact reply
  timestamp unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Status: component amendment explicitly approved after critical and repeat
  review. Separate RD01–RD07 inventory awaits approval before tests/code.
- Question: native UR06 passed, but workload run 42761268874 again rejected an
  unknown pending record. Obtain the actual rejection structure or guess syntax?
- Alternatives: keep guessing extensions; retain the failure and choose another
  observer; or expose complete fixed rejection detail from the existing private
  record without changing acceptance.
- Decision: user chose four fixed labels: name family, argument-count class,
  argument kind and actual rejection stage. Preserve enum-only detail from the
  original parser failure through pending state; do not infer a new stage from
  the later trimmed fragment. Add separate Go/Node metadata accessors and a fixed
  marker only on an actual unknown-raw failure. Preserve old codes, strict mode,
  CX/PX shapes, successful intervals, raw-data privacy and eight-member protocol.
- Rationale: native compatibility of a valid numeric call is not evidence that
  the workload record has that grammar. Closed structural detail can inform a
  later decision without leaking values, pretending success or changing product
  behaviour. Even these labels do not guarantee localisation or waive P01b.

See the [approved component amendment](../architecture/issue89-performance-diagnostic-design.md#approved-complete-fixed-detail-of-raw-grammar-rejection)
and [separate proposed tests](../architecture/issue89-performance-diagnostic-tests.md#proposed-rd01rd07-fixed-raw-rejection-detail).
Earlier localisation, closed-context/prefix and raw-call grammar ADRs remain in
force; none is superseded. No new syscall syntax or product fix is authorised.
