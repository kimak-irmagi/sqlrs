# Record overhead findings and require a complete prepare timeline

- Conversation date: 2026-11-11, Asia/Novosibirsk; exact reply time unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Status: diagnostic objective and detailed component amendment approved;
  subsequent TL01–TL14 tests explicitly approved as well.
- Question: immediately change the next suspected source of overhead, or first
  record the review and classify the remaining wall time with additive tracing?
- Alternatives: implement preview DB handoff from coarse startup costs; continue
  describing only selected extra-operation totals; or add source-linked complete
  timing and repeat the controlled measurements without behavior changes.
- Decision: the user selected recording the review, adding the required tracing,
  and repeating measurements. The step-by-step sum must equal the whole selected
  wall window. Arithmetic conservation alone must not imply causal attribution.
- Rationale: inventory, retained-output proofs and journal/authentication checks
  have concrete measured costs, but those do not explain the whole slowdown.
  The remaining planning/startup/admission work needs direct observations before
  choosing another optimisation. Existing data and the speed gate stay intact.

The [review journal](../architecture/issue89-cost-results-2026-11-11.md#overhead-review-journal-and-the-next-measurement)
is recorded before the [component amendment](../architecture/issue89-complete-timeline.md).
The 2026-11-10 cost-attribution ADR remains valid for its prior scope. Its clause
allowing evidenced optimisations before complete attribution does not mandate a
behavior change in this next experiment. No previous guarantees/tests are
withdrawn. The new shared-clock/transport and boundary details were initially
proposals and subsequently approved below; separate test approval is not waived.

## Approve the reviewed complete-timeline components

- Conversation date: 2026-11-11, Asia/Novosibirsk; exact reply time unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Question: approve the reviewed request/admission/worker tracing, shared Linux
  monotonic clock, exact additive timeline and controlled CT13 repeat, or discuss
  the project first?
- Alternatives: retain worker-only coarse timings; discuss/revise the amendment;
  or approve it while preserving original timers, runtime behavior and speed gate.
- Decision: user replied “Утвердить проект полной трассировки”. The component
  amendment is approved; TL01–TL14 still require their own approval before code.
- Rationale: classify the remaining time without changing execution or disguising
  unknown source-local work as an explained cause merely to balance a total.

Post-approval documentation audit scanned architecture documents and project
READMEs for cost/clock/root/accounting references. The original cost §4 needed an
explicit historical-mode distinction; component structure, architecture index
and performance README now link the approved amendment as not yet implemented.
Old diagnostic artifacts/clocks remain historical; no previous ADR is revoked.

## Approve TL01–TL14

- Conversation date: 2026-11-11, Asia/Novosibirsk; exact reply time unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Question: approve the critically and repeatedly reviewed TL01–TL14 inventory
  covering common clocks, actual call boundaries, independent exact accounting,
  regressions, native qualification and unchanged CT13, or discuss it first?
- Alternatives: revise/discuss the inventory; or approve tests-first work without
  changes to old tests, execution behavior, schema or the speed gate.
- Decision: user replied “Утвердить TL01–TL14”.
- Rationale: prove actual instrumentation and exact accounting independently;
  preserve old contracts and expose unresolved source-local work honestly.

The initial old-test audit inspected recorder/catalog/wire, worker/native cost
tests and collector/campaign/output tests. New-mode selection must preserve the
legacy root/version, disabled-path allocations and old worker-only fixtures.
No conflicting old expectation has been changed. Any contradiction found during
connection or regressions still requires separate approval before editing it.

## Correct the new TL10 outside-window oracle

- Conversation date: 2026-11-11, Asia/Novosibirsk; exact reply time unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Question: retain an incorrect expected count of six outside-window intervals,
  or check all seven exact intervals implied by the approved TL10 vector?
- Alternatives: retain the mistaken count; or correct only the two new Go/Node
  expectations and strengthen them to the exact seven owners/start/end values.
- Decision: user replied “Исправить эталон на семь точных участков”.
- Rationale: preserve all tails (two parent, two child, three client), without
  changing product behavior, weakening preservation or editing old tests.

## Approve the limited OT-TL01 collector branch

- Conversation date: 2026-11-11, Asia/Novosibirsk; exact reply time unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Question: authorise the explicit complete-timeline branch of the existing
  TestReleasedEngine/collect, or leave the existing workload test unchanged?
- Alternatives: keep only synthetic/parser evidence; or add shared-clock
  brackets and client intervals to the actual workload without replacing its
  original API timer, requests, deadlines, or oracles.
- Decision: user replied “Разрешить ограниченный OT-TL01”. Only this old
  collector test and its closed result receive the reviewed opt-in branch.
- Rationale: actual admission/worker alignment needs the same host clock and
  original workload; a mathematically balanced synthetic ledger is insufficient.
