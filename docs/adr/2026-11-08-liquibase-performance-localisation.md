# Localise the confirmed #89 timing regression before proposing a fix

- Conversation date: 2026-11-08, Asia/Novosibirsk; exact reply timestamp unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Status: the no-regression criterion and request for exact localisation are
  approved by explicit user reply; the diagnostic component design was separately
  approved afterwards on the same date. Existing P01b decisions remain in force.
- Question: completed JHipster and 12-change MobilityData cohorts reproduce
  slower v2 above controls on independent VMs. Waive container overhead, make a
  speculative optimisation, or measure the exact operation costs first?
- Alternatives: change the acceptable budget; infer a cause from aggregate
  counters/source; or preserve the criterion and obtain a diagnostic breakdown.
- Approved decision: preserve the criterion, find the exact slowdown location,
  repeat runs with instrumentation/logging if needed, then propose a fix.
- Rationale: an aggregate observation does not justify an implementation change;
  a green collection is not speed acceptance. Keep the two pending 102-change
  cohorts running and do not silently expand #89 with #104 or other deferred work.

The approved implementation boundary uses external strace and existing journal
timestamps on unchanged fixed binaries; see
[component design](../architecture/issue89-performance-diagnostic-design.md).
Product hooks or a Docker wrapper were considered but not selected: the former
changes measured code, the latter changes process/signal behaviour. The narrow
diagnostic implementation follows its separately approved test inventory below.

## Observe fixed executables externally, not through product hooks

Historical boundary, completed by Actions `42802379397`. **Superseded for the
next campaign only** by the [2026-11-10 trimming/cost-attribution decision](2026-11-10-liquibase-cost-attribution-and-trimming.md)
and its separately approved components. Do not relabel the old measured binaries
or invalidate the original reports; privacy/oracle/cleanup/P01b rules persist.

- Conversation date: 2026-11-08, Asia/Novosibirsk; exact reply time unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Status: approved by explicit reply after critical and repeat review.
- Question: which observation boundary can localise time without changing either
  measured engine or leaking private command arguments?
- Alternatives: product timing hooks; a replacement Docker command wrapper;
  external strace with existing journal timestamps and separately validated output.
- Decision: the last option on a separate explicitly launched Ubuntu job, using
  fixed binaries, two untraced and two traced pairs, exact process ownership,
  bounded private trace acquisition and closed numeric/categorical reports.
- Rationale: no product/API/schema changes or signal-altering Docker shim;
  observation overhead remains visible and is not accepted as P01b evidence.
  Original reports, including the launcher-inventory defect, remain unchanged.

## Approve the diagnostic tests and three explicit collector amendments

- Conversation date: 2026-11-08, Asia/Novosibirsk; exact reply time unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Status: both items explicitly approved in the latest user reply.
- Question: implement DG01–DG12 and amend the existing collector, or leave the
  observed executable ownership, Linux launcher provenance and cleanup invalid?
- Alternatives: no observer; new product hooks; or the reviewed diagnostic tests
  with only OT-DG01–OT-DG03 changes to the existing collect scenario.
- Decision: approve DG01–DG12 and, separately, OT-DG01–OT-DG03: traced child proof,
  actual POSIX launcher hash and joined trace/tree disposal before evidence.
- Rationale: retain all existing workload/oracle/count/timer/normal cleanup
  assertions while making unchanged-binary diagnostics trustworthy. This is not
  approval of an optimisation or a waiver of speed/coverage requirements.
