# #89 complete timeline: TL01–TL14

Status, 2026-11-11: [components approved](issue89-complete-timeline.md).
This inventory underwent critical/repeat review and @evilguest explicitly approved
TL01–TL14. Tests-first implementation is beginning. Existing CT/DT/CPR,
strict legacy parsers and native protections remain mandatory; no old-test edit,
new execution policy, recovery matrix or schema change is authorised here.

## Test inventory

1. **TL01 — selection, clocks and disabled behavior.** Only explicit selected
   new-v2 recording enables the complete mode. Absent/false/malformed selectors
   and invalid reference values cannot silently enable it. Test private selector/
   reference stripping for v1/old-v2/new-off and child environments. Real Linux
   engine/collector clock reads share a reference across separate processes;
   values are nondecreasing and public evidence contains only relative offsets.
   Unsupported complete mode is unqualified, not a prepare failure or calendar
   fallback. Existing disabled-path zero allocations/no I/O remain asserted.
2. **TL02 — closed transport and legacy separation.** Round-trip the new clock/
   request-root/observer frames through actual writer and independent Go/Node
   readers. Reject wrong domain/version, missing/duplicate/out-of-order frames,
   wrong root/parent/ordinal, invalid integers, overflow, truncation and quotas.
   An old worker-root report still passes its unchanged legacy reader, but cannot
   qualify as complete mode; a complete root cannot masquerade as legacy data.
   No partial success after a later bad frame. Preserve original work errors.
3. **TL03 — admission and detached dispatch.** Exercise actual Submit/admission
   and worker entry with new local doubles on existing interfaces. One root links
   validation/input/managed-image preparation, immutable admission and dispatch
   to its own worker; concurrent requests cannot exchange spans. Test admission
   rejection, worker starting before Submit returns, and cancellation after
   successful admission. Closing the HTTP context must not cancel the detached
   worker or block admission for report delivery. Close/report the root once,
   after all actually opened scopes finish, including reversed finish order.
4. **TL04 — binding and planning boundaries.** Through actual source call sites,
   verify gate wait, CAS/settings, image resolve/ensure/inventory, JDBC and saved
   binding; planning launch/preparation, executor create/start/wait/exit journal,
   preview disposal, capture/decode/plan save, executor/input/helper release.
   Explicit barriers and call counters prove an interval encloses the intended
   call, not a nearby generic parent. One injected existing-interface failure per
   group must reach that group, retain its original result and prevent later
   effects. No wall-duration threshold, additional JVM or changed runtime order.
5. **TL05 — isolate the two DB lifetimes.** Preview and working routes separately
   observe managed-lock wait, source/base preparation, assignment, StateFS clone,
   StartManaged, attachment and access verification. Confirm actual preview stop/
   clone disposal has separate boundaries. Use existing lock/barrier/runtime
   seams and observed call order, not sleeps. A contended lock is recorded as
   waiting, not DB execution. Correctly sourced cached-state creation is distinct
   from base creation; a warm base has no invented initialisation call. Trace
   current two-DB behavior without implementing reuse or assigning all startup
   self time to clone/DB start.
6. **TL06 — session and every logical step.** Observe session/clone journal,
   helper/config, runner create/start/readiness and its journal; then step binding,
   status/cache decision, resume/check/journal, plan/admit/invoke, capture/prune/
   complete, snapshot/seal/size/publication. Exercise fresh three-step, full-cache
   and cached-prefix cases using existing boundaries. Ordinals and phases are
   exact; cached ordinals remain visible without invented invocations or working
   DBs. Runner reuse and existing SQL counts stay unchanged. Each required
   boundary's omission is rejected; conditional boundaries follow actual calls.
7. **TL07 — publication, cleanup and work outcomes.** Separately observe working
   DB handoff versus cached-instance creation, runner stop, private disposal,
   final transaction, projections/files and terminal notification. With existing
   interfaces check success, known child failure, cancellation and panic retain
   original outcomes, identity/cleanup sequence and no repeated SQL. A recording/
   encoding failure cannot replace the work result, skip cleanup or grant retry.
   Normal-path tests only: do not introduce a new recovery/COMMIT fault matrix.
8. **TL08 — actual collector window and timer reconciliation.** Exercise the
   selected collector's real POST/status/poll branches against a small local HTTP
   server without Docker. Record shared-clock start/end, request and polling
   intervals; preserve its original Go API timer. Check positive/negative/zero
   timer differences against independently supplied boundary brackets, impossible
   differences, backwards/overflow endpoints and wrong reference. Qualification
   must not wait for the report inside the timed window. Setup/oracles/cleanup
   and post-timer reader waiting remain outside it. No claim that a whole HTTP
   request is pure network time when server work is unobserved.
9. **TL09 — independent exact partition oracles.** Hand-written expected segments
   are checked independently in Go and Node, without importing one partitioner
   into the other or generating expectations from the implementation. Mandatory
   base vector: window [100,200), engine parent [110,190), children [120,150) and
   [150,180), POST [100,125), poll gap [125,195), status [195,200). Expect client
   POST=10, unresolved parent self=20, children=30+30, client poll=5, status=5:
   exactly 100 ns, not 240 ns by summing all inclusive engine/client intervals.
   Also check
   empty engine evidence in a pure arithmetic vector, zero-length calls,
   coincident/adjacent boundaries, reversed input order, one-nanosecond intervals,
   invalid containment/cycles/IDs and safe-integer limits. Pure empty-vector
   arithmetic success cannot qualify a workload with missing required spans.
10. **TL10 — concurrency, tails and observer costs.** Add an incomparable sibling
    [135,145) to TL09: exactly 10 ns belongs to one overlap segment with both
    owners, replacing 10 ns of the child's exclusive time. Cover overlapping
    same-key calls, admission/worker concurrency, observer emission before/after
    API end, root/worker tails outside the window and helpers with coarse clocks.
    Clip only intersection accounting, preserve original intervals/tails, never
    extend API timing or add helper/inclusive durations. Bound segment/owner
    counts and output; reject excess rather than publish a partial ledger.
11. **TL11 — classification, source evidence and private report.** Each real
    segment carries a closed owner/phase/ordinal/source mapping; aggregation is
    independently recomputed from the ledger. Assert adjacency, final endpoint
    and exact nanosecond sum, step totals and stated percentage denominators.
    List all source-local unresolved segments, their total and largest owners;
    synthetic seconds of unknown planning work must remain unknown despite a
    balanced sum. Missing required boundary, forged mapping, quota/parse error,
    incomplete root/observer or bad artifact provenance cannot qualify. Sentinel
    private fields are removed/refused under existing parser contracts; none can
    leak through errors or owner lists. Rounding is display-only; v1 internal
    timings remain unavailable, not zero or inferred from v2 measurements.
12. **TL12 — compatibility, regressions and coverage.** Run old CT/DT/CPR and
    closed-wire tests unchanged, including old worker-first qualification, then
    ordinary engine/client regressions on Windows/Linux and vet. New clock,
    partition, parser and report helpers target 100%, minimum 95%, with line-level
    reports; the aggregate #89 coverage exception still belongs to #174. Any
    further coverage iteration or old-test contradiction is separately presented,
    not silently addressed by weakening assertions or excluding difficult code.
13. **TL13 — real Linux qualification before the campaign.** Explicit Actions
    native three-change prepare followed by full-cache prepare: independently
    verify DB contents, SQL/CLI counts, single reusable Liquibase runner and
    working DB, exact final DB handoff and cleanup before fallback cleanup.
    Observe preview and working lifetimes, every actual ordinal and required
    boundary, common-clock transport, client/server/observer overlap and exact
    ledger totals. Full cache has preview activity but no working-session/step
    SQL. Require the precise native test PASS, not zero matches/skip. A successful
    synthetic transport test or ledger alone cannot replace native qualification.
14. **TL14 — unchanged controlled CT13 repeat.** First test actual scheduler/
    workflow selection, stripped environments, exact artifact checks and full
    series/cleanup qualification. Then run real JHipster CT13 in Actions: released
    v1, old v2, new off/on, reversed order, eight members / 16 passes on one VM,
    no strace or laptop workload. Timeline mode only on the two new-on members;
    ordinary elapsed, oracles, cleanup, helper identity and incomplete-run
    preservation unchanged. Retain original JSON/hash and chronological reports
    for both fresh/cache samples, source-local residuals and recording overhead.
    Do not average away missing samples or call collection success a speed pass.

## Placement and order

New sibling test files live in existing preparecost/prepare/runtime packages,
`test/performance/liquibase` and `scripts/performance`; no new framework or
product test hook. TL01–TL02/TL08–TL11 first establish failing clock/codec/ledger
tests; TL03–TL07 establish failing actual-call boundary tests. Only then implement
the approved behavior-neutral recorder/call-site/collector changes. TL12 precedes
TL13 and TL14. Reuse existing doubles/interfaces in new tests; editing their
old shared preparations is not authorised by this inventory.

Ordinary verification commands, with exact final test names recorded during
implementation, are `go test ./internal/preparecost -count=1 -coverprofile=coverage/timeline.out`,
the corresponding focused prepare/runtime tests, `node --test
scripts/performance/*.test.mjs`, full package regressions and vet. Linux-only
collector/native execution runs in Actions. Do not cross-compile then pretend
the executable ran, run real Docker on the laptop, or redirect Go caches.

## Critical and repeat review

Corrected findings before presentation:

- Exact old API nanoseconds were confused with the new shared-clock window:
  TL08/TL11 check separate durations plus real bracketed reconciliation.
- Merely balanced synthetic data did not prove the new probes were called:
  TL03–TL07 require actual call barriers/counters and TL13 native qualification.
- Cached/conditional calls were assigned fixed fresh-run counts: TL06/TL13 now
  distinguish all-cache/prefix/fresh and validate actual paths.
- Client transport, engine work and observer output could be counted twice:
  TL09–TL10 include hand-written overlapping-window and terminal-tail oracles.
- Parent self time could masquerade as a diagnosed cause: TL11 requires explicit
  unresolved source-local intervals even when every nanosecond is accounted for.
- New request roots could weaken historical tests, and admission cancellation
  could leak to the worker: TL02/TL03/TL12 preserve both contracts explicitly.

Repeat review verified all seven design boundary groups, finite data/clock/
frame failures, privacy/quotas, timer and helper-clock separation, native-data/
cleanup proof and unchanged full repeat. No remaining P0–P2 inconsistency found
in this inventory. This is a reviewed plan, not evidence of passing tests.

## OT-TL01: addressable existing collector change, approved

The post-approval old-test audit found that real collection lives in
`TestReleasedEngine/collect` in `test/performance/liquibase/collector_linux_test.go`.
The new arithmetic/parser tests do not authorise editing this old workload test.
Proposed limited change: only with the explicit complete-timeline selection,
sample the shared clock immediately around the existing start/end `time.Now`,
observe existing POST/status calls and polling gaps, pass the private reference
to the selected new-v2 engine, and await/qualify the new request-root/clock/
observer-window evidence after the original API timer. Extend its closed cost
record with the actual relative window, brackets and client intervals.

Retain the legacy off/worker-only branch, original API start/end and elapsed,
exact binaries/helper identities, workload payloads, timeout/deadlines, order,
DB/cache/CLI/container oracles, native input prerequisites and exact process/
reader/resource cleanup. Nothing moves setup/oracle/report waiting inside the
API timer. No old assertion is removed or relaxed, no pinned v1 instrumentation
is added, and no other old test is included in this permission.

Critical/repeat review checked selector/reference stripping, unknown clock
refusal, cancelled/partial reader evidence, failed calls, timer comparison,
unmeasured payload/self gaps and terminal/report races. Those remain explicit
measurement failures or source-local unknowns, not valid zero-cost samples.
No remaining P0–P2 inconsistency found in this limited amendment. The user
explicitly approved this limited change: “Разрешить ограниченный OT-TL01”.

## OT-TL02: current parser name in orchestration audit, approved

The unchanged `CT13: actual parent verifies identities, owns bounded private
output and uses unchanged collector` in `cost-campaign-workflow.test.mjs`
checks for a literal call named `parseCostOutput`. The new explicitly selected
timeline must use `parseCompleteCostOutput`, since the old parser deliberately
requires a worker-only root. Proposed change: only that one parser name in the
test's symbol list. Preserve every artifact/private-output/budget/cleanup and
unchanged workload assertion, and all old parser behavior tests unmodified.

Critical review rejected keeping an unreachable old-parser call just to satisfy
the source-string assertion, or weakening old root validation. Repeat review
verified the new parser has actual positive/negative timing/source/oracle tests
and the explicit selection is tested separately. No remaining P0–P2 finding in
this limited proposal. The user approved: “Разрешить замену только имени разборщика”.

## LC01–LC03: line-report-driven helper coverage follow-up, approved

The full ordinary Windows collector profile is retained locally as
`.tmp/timeline-current-collector-windows.out`. It reports `begin` 94.1% and
`qualifyTimelinePass` 89.5%; timer/selection/environment/client-kind helpers are
100%. Five positive-statement zero-count blocks remain in `timeline_collection.go`
(41, 71, 91, 103, 131 at this revision). This is not whole-module qualification:
Linux clock execution is separate, and aggregate #89 coverage stays in #174.

1. **LC01:** reject further intervals after poisoned evidence and at the finite
   client quota; assert no extra clock call or appended interval. Requirement:
   TL01/TL08/TL10 reject partial/quota evidence, not implementation-only detail.
2. **LC02:** exact independent failures for an unqualified timer, malformed span
   range/parent containment, wrong admission phase and duplicate POST/missing
   final status. Retain a positive request/worker/helper control, original timer,
   unchanged safe error and no partial result. Requirements: TL02/TL08/TL11.
3. **LC03:** positive Node report with request/worker/observer tails beyond API
   end, preserving exact owners/parents and original elapsed; independently check
   clipping and tails. This exercises the report's unused outside-owner callback
   (new report has 100% lines / 95.45% functions). Requirements: TL10/TL11.

Order starts with the largest portable gap (`qualifyTimelinePass`), then `begin`,
then the single Node callback. Repeat full ordinary collector tests/vet and new
Go per-line/Node line reports; Linux qualification remains mandatory separately.
Target 100%, minimum 95%; no promised gain before measurement, no new product
boundary, dead-code deletion, synthetic kernel failure or old-test change.

Critical review checked original timer/closed errors, phase/root independence,
finite quotas and the distinction between clip-only accounting and preserving
inclusive tails. Repeat review tied all five uncovered statement blocks to
approved requirements and kept real Linux/native tests separate. No remaining
P0–P2 issue in this limited follow-up. The user approved: “Утвердить LC01–LC03”.

LC01–LC03 are implemented. The user separately approved correcting two new LC03
tail-kind expectations (`source` → existing `engine`), without changing owners,
parents or bounds. Full collector tests/vet pass; portable collection/timer
helpers reach 100% statements. All 96 Node tests pass; complete-report and
partition helpers both reach 100% lines/functions (branches 93.58% / 99.02%).
Hosted probe [43058276396](https://github.com/kimak-irmagi/sqlrs/actions/runs/43058276396)
passed on `08b3816`, including the actual Linux complete-timeline prepare and
previous native regressions. The subsequent LC03 change is test-only.
