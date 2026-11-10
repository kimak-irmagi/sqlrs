# Retain terminal-proved time for the exact unavailable fallback entry

- Conversation approval: 2026-11-10, Asia/Novosibirsk; exact reply time unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Status: component design explicitly approved after critical and repeat review.
  UE01–UE07 and named OT-UE01/OT-UE02 are proposed, not approved or implemented.
- Question: how to handle the empty pending fallback established by actual
  run 42772724450 without inventing a syscall, successful JVM or product cause?
- Alternatives: retain full refusal; discard the entry; invent numeric arguments
  or an executable; or retain only terminal-proved unattributed process time.
- Decision: the measured decoder alone may recognise exactly `???(`, at least
  one horizontal space/tab and `<unfinished ...>` over the whole record body.
  Keep a private pending tag separate from a decoded six-argument raw call.
  The next event of that PID must be the existing exact natural/signal terminal;
  other PIDs may interleave but cannot close it. Duplicate pending, intermediate
  events, malformed framing, reversed time, EOF, quota and reader failures fail.
  Retain `process.unattributed` only after proof, end a known image at entry,
  merge earlier unattributed time without overlap and validate/exclude engine
  time. Strict/cutoff and parseDiagnosticRaw remain unchanged. No new report
  category, API, schema, trace flags, product code or retry authority.
- Rationale: the native enum-only evidence establishes an empty entry, not
  decoded arguments or executable identity. Keep its uncertainty visible rather
  than lose time or assert a launch. This may still fail on intermediate events
  in the real workload, and does not localise the product regression or waive P01b.

See the [approved design](../architecture/issue89-performance-diagnostic-design.md#approved-terminal-unavailable-fallback-entry-measured-mode-only)
and [proposed tests](../architecture/issue89-performance-diagnostic-tests.md#proposed-ue01ue07-terminal-unavailable-entry).
Reviewed earlier raw-rejection, six-argument raw-call, exec-uncertainty and
closed-context ADRs: none is obsolete. This is a separately tagged terminal-only
exception, not a relaxation of the raw parser or the old metadata contract.
