# Parse bounded unknown syscall records without inventing an executable identity

- Conversation approval recorded: 2026-11-10, Asia/Novosibirsk; exact reply
  timestamp unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Status: component amendment and, subsequently, the separately reviewed
  UR01–UR07 test inventory explicitly approved before tests/code.
- Question: native run 42665130138 proves pending_entry/other with the
  unknown_syscall prefix, but the decoder assumes every unfinished entry is exec.
  How to retain valid evidence without falsely assigning Java/Docker time?
- Alternatives: retain the full observer failure; ignore unknown records;
  or validate a separate bounded numeric grammar and retain unattributed time.
- Decision: user chose the separate measured-only grammar and
  `process.unattributed` until a confirmed executable or exact terminal event.
  Validate whole records, exact pending/resume association and lifecycle. Do not
  restore the previous category on an unknown return, count a successful exec,
  or treat EOF as exit. Preserve default strict mode and invalid PX02 cases.
  Require a tiny native invalid-syscall Actions probe before the original eight
  diagnostic members. No laptop workload, product/API/schema or trace-flag change.
- Rationale: the strace reference source explicitly has an unknown-call raw
  fallback even with the chosen filter. Closed prefix evidence identifies that
  family, not numeric payload or a regression cause. Retain this uncertainty
  honestly rather than discard it or infer an executable identity.

See the [approved component amendment](../architecture/issue89-performance-diagnostic-design.md#approved-measured-raw-syscall-records-with-unattributed-process-time)
and [separately approved tests](../architecture/issue89-performance-diagnostic-tests.md#proposed-ur01ur07-raw-syscall-grammar-and-unattributed-time).
The earlier localisation, exec-uncertainty, closed-context and structural-prefix
ADRs remain in force; none is superseded. This approval does not establish the
exact product cause, complete the eight members or waive the P01b speed gate.

## Approve UR01–UR07 before implementation

- Conversation date: 2026-11-10, Asia/Novosibirsk; exact reply time unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Question: proceed with the reviewed numeric grammar/lifecycle/privacy/native
  probe inventory while retaining earlier strict negative tests?
- Alternatives: retain the current observer failure; or build the separately
  reviewed tests first and implement only that measured grammar.
- Decision: user explicitly approved UR01–UR07. Repeated old-test audit found
  no contradiction: PX02 is an invalid decimal/incomplete tuple; strict DG/EU,
  CX shape, tracer flags and the old native smoke remain unchanged.
- Rationale: distinguish a genuinely observed raw call from a false JVM launch
  before rerunning the same eight members; no product change or speed waiver.

## Correct the new UR01 duration boundary

- Conversation date: 2026-11-10, Asia/Novosibirsk; exact reply time unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Question: UR01 incorrectly classified 9223372036.0 seconds as overflowing a
  signed 64-bit nanosecond duration. Correct that new expectation?
- Alternatives: keep the mistaken negative, contradicting the approved grammar;
  or test the actual maximum and the first overflowing nanosecond.
- Decision: user explicitly authorised moving 9223372036.0 to positives, adding
  the positive maximum 9223372036.854775807 and replacing only that negative with
  9223372036.854775808. Old tests/product/range remain unchanged.
- Rationale: retain the exact approved numeric range rather than change code to
  satisfy an incorrect test. This is a test correction, not relaxed validation.
