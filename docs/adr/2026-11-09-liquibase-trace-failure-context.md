# Export closed trace-failure context without admitting new records

- Conversation date: 2026-11-09, Asia/Novosibirsk; exact reply time unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Status: component amendment explicitly approved by the user after critical
  and repeat review. Separate CX01–CX05 test inventory awaits approval.
- Question: incomplete diagnostic 42618850460 reports code 8, which cannot
  distinguish an unrecognised record from an invalid pending entry. How to
  improve evidence without exporting private trace bytes?
- Alternatives: keep retrying without new context; export a scrubbed raw line;
  or export only fixed location/shape and the existing collector stage.
- Decision: the user approved the last option. Location is record/pending_entry/
  none; shape is superseded_exec/exec_entry/exec_resume/other/none. Preserve the
  existing code/rejection and the existing phase allowlist; no PID/argv/raw line.
- Rationale: identify which observer check fails without inventing a valid trace
  or a product cause. Last-reached stage does not prove a primary workload error.
  No new syntax acceptance, read boundary, trace flags, product change or altered
  eight-member protocol. Tests remain a separate approval before implementation.

See the [approved component amendment](../architecture/issue89-performance-diagnostic-design.md#approved-closed-context-for-an-unrecognised-trace-record)
and [proposed tests](../architecture/issue89-performance-diagnostic-tests.md#proposed-cx01cx05-closed-failure-context).
Prior localisation and uncertain-exec ADRs remain in force; none is superseded.
