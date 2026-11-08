# Localise the confirmed #89 timing regression before proposing a fix

- Conversation date: 2026-11-08, Asia/Novosibirsk; exact reply timestamp unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Status: the no-regression criterion and request for exact localisation are
  approved by explicit user reply; the diagnostic component design is proposed,
  not yet approved. Existing P01b decisions remain in force.
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

The proposed implementation boundary uses external strace and existing journal
timestamps on unchanged fixed binaries; see
[component proposal](../architecture/issue89-performance-diagnostic-design.md).
Product hooks or a Docker wrapper were considered but not selected: the former
changes measured code, the latter changes process/signal behaviour. The narrow
diagnostic proposal requires its own approval and then a reviewed test inventory.
