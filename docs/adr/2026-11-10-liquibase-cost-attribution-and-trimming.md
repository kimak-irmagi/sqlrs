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

## Approve CT01–CT14

- Conversation date: 2026-11-10, Asia/Novosibirsk; exact reply time unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Question: approve the reviewed CT01–CT14 inventory or discuss it first?
- Alternatives: postpone implementation; or tests-first O1–O4 and source-linked
  costs with protected outcomes, private transport and unchanged speed controls.
- Decision: user replied “Утвердить CT01–CT14”. No old-test rewrite is authorised;
  named contradictions found in the subsequent audit require separate approval.
- Rationale: test reduced work and retained protections independently, validate
  attribution math, then verify actual execution and ordinary speed in Actions.

## Approve limited OT-CT01 fixture changes

- Conversation date: 2026-11-10, Asia/Novosibirsk; exact reply time unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Question: retain obsolete list+inspect/three-copy fixtures or adapt the nine
  named tests to the approved exact-ID/single-archive acquisition path?
- Alternatives: retain the old mechanism; or update only named response queues,
  session archive fixtures, counts/indices and the readiness cancellation barrier.
- Decision: user replied “Разрешить ограниченный OT-CT01”. All semantic assertions,
  exact refusal boundaries and planning/discovery fixtures remain unchanged.
- Rationale: test the new mechanism without weakening completion, identity,
  hash, idle, privacy, cancellation or deadline requirements.

## Approve the disabled-recorder assertion correction

- Conversation date: 2026-11-10, Asia/Novosibirsk; exact reply time unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Question: count reflect assertion overhead as disabled-recorder overhead, or
  move only the independent empty-report assertion outside AllocsPerRun?
- Alternatives: retain the false allocation failure; or preserve both assertions
  while measuring only the disabled WithRecorder/WithPhase/Start/end path.
- Decision: user replied “Разрешить точечное исправление теста”.
- Rationale: preserve the zero-allocation requirement without counting the
  test framework's own struct boxing as product work.

## Approve limited OT-CT02

- Conversation date: 2026-11-10, Asia/Novosibirsk; exact reply time unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Question: adapt only TestPrivateStepRuntimeLiveAndStopped to exact-ID JSON and
  one archive, or retain its obsolete transport fixture?
- Alternatives: retain list+inspect/three copies; or change only this fixture and
  copy count 3→1, keeping both phases, child/parent exits and idle assertions.
- Decision: user replied “Разрешить ограниченный OT-CT02”.
- Rationale: preserve all semantic controls on the approved acquisition path;
  shared planning fixtures remain unchanged.

## Approve the exact CT09 integer boundaries

- Conversation date: 2026-11-10, Asia/Novosibirsk; exact reply time unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Question: incorrectly reject 922337203685 centiseconds, or test the exact
  representable maximum and first overflowing centisecond?
- Alternatives: narrow product arithmetic for a mistaken expectation; or use
  922337203686 as the negative case and retain the exact maximum as positive.
- Decision: user replied “Исправить точные границы CT09”.
- Rationale: preserve the full int64 nanosecond domain and overflow protection.

## Approve limited OT-CT03 collector integration

- Conversation date: 2026-11-10, Asia/Novosibirsk; exact reply time unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Question: keep discarding all stderr in the existing collector or connect the
  new explicit cost mode without moving API timing or weakening its oracles?
- Alternatives: duplicate native orchestration; or extend only collect's cost
  branch, environment filtering and owned-reader cleanup as described in OT-CT03.
- Decision: user replied “Разрешить ограниченный OT-CT03”. Binary pins, requests,
  API timer, strace, workload/cache/count checks and private-log rules remain.
- Rationale: reuse independently verified orchestration while collecting bounded
  complete observations after, not inside, the existing API timing boundary.

## Approve limited OT-CT04 native incomplete-archive expectation

- Conversation date: 2026-11-10, Asia/Novosibirsk; exact reply time unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Question: preserve the obsolete manifest-reader error text or observe O3's
  earlier rejection of the missing-manifest archive in the same native case?
- Alternatives: misclassify product archive failures as manifest-only errors;
  or change the named expectation and add independent live-parent/stream proofs.
- Decision: user replied “Разрешить ограниченный OT-CT04”. All three existing
  scenarios, negative refusal, hashes, runner reuse and exact cleanup remain.
- Rationale: retain the semantic protection and its precise new refusal boundary,
  without changing product errors to make an old fixture pass for a wrong reason.

## Approve actual intermediate outcomes in new CT12

- Conversation date: 2026-11-11, Asia/Novosibirsk; exact reply time unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Question: must every intermediate Docker readiness command succeed for a
  successful prepare, or must the observer retain the real intermediate result?
- Alternatives: fail the new test on legitimate transient readiness refusals;
  or retain closed actual command outcomes while requiring successful worker
  and mandatory completed operations, exact DB/counts/reuse/cleanup controls.
- Decision: user replied “Да, исправить это ожидание”.
- Rationale: successful prepare does not imply successful readiness probes on
  every attempt. The correction grants no product retry or execution authority.

## Approve helper-detail and observer-cost transport

- Conversation date: 2026-11-11, Asia/Novosibirsk; exact reply time unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Question: how to carry helper-local phases and report-emission measurements
  without confusing host clocks, accepting a prematurely complete report or
  claiming circular measurement of all observer overhead?
- Alternatives: retain host-only frames; add helper times to host totals; or
  bind closed helper observations to host-selected invocation spans and send
  a separate observer completion frame after the framed report write.
- Decision: user replied “Утвердить уточнение передачи”. Keep begin/span/end,
  send helper detail before end and observer encoding/lock-wait/write durations
  afterwards under the same writer lock. Full measured collection requires both
  terminal markers after the unchanged API timer. Snapshot copying and the
  observer marker's construction/write remain explicitly unmeasured; off/on
  controls assess the overall effect. Supplemental tests and named conflicting
  old-fixture amendments require separate approval before implementation.
- Rationale: preserve independent execution outcomes, bounded private evidence,
  helper clock resolution and non-additive detail without inventing precision
  or total observer overhead. No API, DB, capture or cleanup authority changes.
- Relationship: refines CT06–CT11/CT14 above; does not supersede the optimisation,
  source-correspondence, privacy, regression or unchanged speed requirements.

## Approve DT01–DT08 and limited OT-CT05–OT-CT08

- Conversation date: 2026-11-11, Asia/Novosibirsk; exact reply time unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Question: approve the reviewed concrete helper/observer tests and adapt only
  four named conflicting old preparations, or revisit their design first?
- Alternatives: discuss the inventory; or write approved tests first while
  preserving the old semantic assertions and their exact refusal boundaries.
- Decision: user separately replied “Утвердить DT01–DT08” and
  “Разрешить OT-CT05–OT-CT08”. No other old-test changes are authorised.
- Rationale: qualify owned timing, complete private delivery and independent
  accounting without changing product outcomes or the blocking speed criterion.

## Approve line-mapped DT-C1 detail-decoder coverage

- Conversation date: 2026-11-11, Asia/Novosibirsk; exact reply time unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Question: cover four remaining detail-decoder refusal operators and incomplete
  phases at end, or discuss the remaining 50/54 statement gap first?
- Alternatives: defer the iteration; or add independent targeted literal/direct
  cases under existing DT03/DT05/DT07 and repeat the complete profile/tests/vet.
- Decision: user replied “Разрешить адресную итерацию”. No old-test or product
  boundary changes, new injection capability or dead-code removal authorised.
- Rationale: verify existing safe-refusal requirements and reach the helper
  coverage target without changing execution or performance acceptance.

## Implement approved CT13 without widening preparation roots

- Conversation date: 2026-11-11, Asia/Novosibirsk; implementation under the
  previously explicit CT01–CT14 approval, not a new user reply.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Question: how to prepare both fixed versions on one VM while keeping the
  existing strict hosted-workspace-only preparation boundary?
- Alternatives: introduce multi-root preparation permissions; or prepare old v2
  in the hosted root, retain its verified executable/JDBC privately, then switch
  the clean root to the selected new revision and prepare new v2 once.
- Decision: agent implementation uses serial clean root checkouts and retained
  exact old bytes. Source/build transitions finish before all eight measurements;
  new off/on use the same executable/helper. Existing preparation tests and root
  policy remain unchanged. A separate explicit read-only label starts this cohort;
  historical diagnostic/full-P01b workflows and evidence remain separate.
- Rationale: implement the approved same-VM experiment with existing authority,
  preserving independent hashes and controls rather than expanding permissions.

## Propose one combined native completion/prune observation

- Conversation timestamp: 2026-11-11 06:13:48 UTC (13:13:48 Asia/Novosibirsk),
  proposal preparation; no user approval reply yet.
- GitHub participant: @evilguest (41718235), requested source-linked costs and
  removal of demonstrated extra work; this new boundary awaits their decision.
- Agent: Codex / GPT-5.
- Question: reduce the measured repeated prior-prune Docker round trips without
  replacing real absence/idle/identity checks with a manifest assertion?
- Alternatives: retain all current round trips; trust only the completed
  manifest's prior link; or combine the existing real read-only absence probe
  and idle proofs with the native completion observation under its unchanged
  exact pre/post parent checks and deadline.
- Proposed decision: prefer the combined native observation with a runtime-owned
  in-memory receipt bound to exact saved authority and completion. Queue guards,
  transaction/readback rules and explicit standalone proof remain unchanged.
  Details and critical/repeat review are in cost-attribution components §6.
  This is not permission to change product code or existing tests.
- Rationale: CT13 measures the five-command prune branch at 213.829 ms per
  later step / 1.497 s total, versus about 8 ms per journal trio. Target actual
  transport duplication first, disclose shifted measurement boundaries and do
  not claim this alone resolves the remaining 18% fresh regression. Image
  inventory caching/general recovery/SQLite refactoring are outside this proposal.
