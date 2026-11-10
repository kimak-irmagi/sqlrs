# Trim evidenced duplication and attribute costs to source operations

- Conversation date: 2026-11-10, Asia/Novosibirsk; exact reply time unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Status: direction and then the reviewed detailed components explicitly approved
  by the user; the subsequent test inventory requires separate approval.
- Question: continue refining external Docker categories before any product fix,
  or remove evidenced duplication and measure named operations/source sites?
- Alternatives: require another strace-only refinement first; optimise from totals
  without attribution; or combine reviewed removal of proven duplicate work with
  useful operation-level costs and an ordinary before/after performance rerun.
- Decision: user selected the last. Establish which v2 actions correspond to v1
  and which additional actions belong to which code; report calls/time per step,
  total time and percentage of complete prepare. Do not require complete
  attribution of every residual millisecond before correcting demonstrated waste.
- Rationale: successful native collection already proves a regression and supplies
  enough source-backed evidence to act. Continue to distinguish observed costs,
  instrumentation overhead and unmeasured work; a functional pass is not speed
  acceptance. The existing no-regression criterion remains unchanged.

The [component proposal](../architecture/issue89-cost-attribution-and-trimming.md)
separately specifies O1–O4, opt-in default-off internal recording, source mappings,
non-double-counted accounting, off/on and before/after controls. Those details are
not covered by the directional decision alone; they were subsequently explicitly
approved below. No new tests/code/Actions run before separate test approval.

Reviewed the performance-localisation, Actions, clock-context and measured-clock
ADRs. The external-only next-experiment choice in the localisation ADR is
superseded for the next campaign by this decision and the separately approved components.
Its original fixed-binary experiments and artifacts remain historical evidence;
privacy, independent oracles, cleanup, honest uncertainty and P01b remain valid.
Clock alignment applies to old trace evidence; it is not a reason to convert
in-process monotonic operation durations to calendar time.

## Approve the reviewed components

- Conversation date: 2026-11-10, Asia/Novosibirsk; exact reply time unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Question: approve O1–O4, default-off source-linked host/helper costs, correspondence
  and before/after/off/on Actions controls, or discuss the components first?
- Alternatives: retain the old implementation/external-only observer; or the
  bounded reviewed component design, without API/DB or performance-gate changes.
- Decision: user replied “Утвердить проект компонентов”. The design, including
  helper detail at 10 ms resolution and separate host spans, is approved.
  Separate tests and named old-test changes still require approval before code.
- Rationale: remove proven duplicate mechanisms while preserving checks, quantify
  operation costs and observation overhead without another strace-only detour.
