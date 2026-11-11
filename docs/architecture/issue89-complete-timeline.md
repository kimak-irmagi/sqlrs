# #89: a complete source-linked prepare timeline

Status, 2026-11-11: @evilguest explicitly approved this reviewed component
amendment with “Утвердить проект полной трассировки”. The overhead review journal,
additional tracing and Actions repeat are requested; the separately reviewed
[TL01–TL14 test inventory](issue89-complete-timeline-tests.md) is explicitly
approved too. Implementation starts with new tests; no old-test edits are waived.
It extends the [cost design](issue89-cost-attribution-and-trimming.md), not
execution, API, storage, cleanup permissions or the speed criterion.

## 1. What the present evidence cannot establish

The [review journal](issue89-cost-results-2026-11-11.md#overhead-review-journal-and-the-next-measurement)
records the concrete mechanisms. Current recording starts at the worker,
after API admission begins. Planning and session-startup parents still contain
seconds of unclassified self time. A balanced `unassigned` total is not a
causal explanation. Parent-inclusive times must not be summed with children.

Do not change behavior while establishing these costs: no inventory caching,
driver-policy change, capture bypass, removed access check, or preview DB reuse.
Those would confound this diagnostic repeat and require separate designs.

## 2. Responsibilities and clocks

- `internal/preparecost` remains the only product recorder, with closed operation
  keys, bounded in-memory records and emission on the existing private stream.
  Extend its existing recorder/report, not introduce a tracing service or store.
  The explicit complete-timeline mode adds an in-memory request-root scope,
  admission and worker children. A private pointer in the prepared request carries
  only that scope/sequence across dispatch, never the HTTP cancellation context.
  Historical worker-root recordings keep their shape; the new mode has explicit
  qualification rather than weakening the old worker-first checks. No DB field
  or public request value stores the pointer or controls this mode.
- On the selected Linux Actions run, engine and collector use the same host
  `CLOCK_MONOTONIC`. The collector supplies one private reference value before
  starting its engine. Only offsets from that reference leave either process;
  no calendar time, machine uptime, PID, paths, SQL or credentials are published.
  The private timeline selector/reference are owned by the collector, stripped
  on all other variants and never forwarded into Liquibase's environment.
- Add a closed optional clock/window frame to the existing internal cost stream;
  historical frames/reports keep their old interpretation. Selected timeline
  collection requires the frame and validates it; it never guesses alignment.
  Unsupported clocks, failed reads, backwards values, overflow or missing frames
  invalidate the measurement, not the prepare result. Default-off recording
  keeps its existing allocation/I/O guarantees. No public CLI/API field changes.
- `test/performance/liquibase` independently records API-window endpoints,
  POST/status-request intervals and polling gaps on that shared clock. Keep the
  original Go API timer for historical speed comparisons. Publish the new
  timeline-window duration and its signed difference from that timer separately;
  never alter old samples, invent a delay, clamp timestamps, or call that
  difference application work. Bracket both timer readings to validate the
  small difference against actual boundary-read uncertainty.
- `scripts/performance` independently validates and partitions host intervals,
  retains source coordinates and produces chronological plus aggregated output.
  Exact artifact identities qualify each report. The report owns no runtime
  resource or execution authority.
- New data shapes are a closed clock/window frame (domain/version, relative
  origin/window bounds), source-keyed spans, and ledger segments (relative
  start/end, owner kind, closed span references, phase/ordinal). Observer emission
  also has shared-clock start/end bounds: part of it may precede terminal status
  observation. It is not all automatically outside API time. Root/observer frames
  are still awaited only after the original API timer, with the old deadline.
  Existing wire/output quotas remain hard limits; bound segment count from the
  bounded span/request counts and reject over-budget owner lists before export.
- Non-Linux ordinary behavior and existing recordings are preserved. The new
  complete-timeline qualification is explicitly Linux-only, not silently
  emulated with calendar clocks. Helper-local 10 ms phases remain nested detail,
  never values on the host timeline or extra summable work.

## 3. Instrument actual call boundaries

Use existing context-based Observe/ObserveCall around actual calls; no new
function-valued product hooks, changed transactions or reordered effects.
Each new key has a static source symbol and a reviewed v1 counterpart or an
explicit changed/additional/unresolved relation. Shared purpose never implies
equal duration. The finite boundary inventory is:

1. Admission: request validation/input collection, managed-image preparation,
   immutable input publication, queue admission and dispatch. Link the admission
   observation to its actual worker using the existing process-local recording
   sequence, never a public job ID. Recording context must not propagate request
   cancellation into the detached worker or prolong admission for observation.
2. Binding: projection-gate wait, CAS materialization/settings, image resolution,
   ensure/inventory, driver publication/materialization and saved binding.
3. Planning: launch preparation/journal, actual preview DB creation, protected
   input/helper preparation, tracked executor create/start/wait and exit journal,
   actual preview DB disposal, capture acquisition/decoding/plan persistence,
   executor removal and journal/file release. Do not label all planning self
   time SQL parsing or all executor wait JVM work.
4. DB creation, separately for preview and working callers: managed-lock wait,
   base lookup/initialisation, ownership assignment, StateFS clone, managed
   container startup, ownership attachment and full access verification.
   Reuse this boundary vocabulary at both callers; keep phase/caller context.
5. Session startup: session/clone journal, helper/config materialization,
   reusable runner create/start, readiness observation and readiness journal.
6. Each plan ordinal, including cache hits: binding/status/cache decision,
   resume/access check/journal, invocation plan/admit, actual helper invocation,
   capture read/prune confirmation/complete, snapshot preparation/seal/clone/
   size/publication. Add journal and lock boundaries currently left in parent
   self time. Cached steps must appear with no invented invocation/DB activity.
7. Publication and cleanup: runner stop/confirmation, DB resume/handoff or
   cached-instance creation, final transaction, private-capture/container cleanup,
   input/projection/file release and final event/notification.

`managed_execution.go`, `liquibase_normal_worker.go`, container binding,
`tracked_executor.go`, the session coordinator and their called adapters supply
these boundaries. Trace service/admission calls too; adding only DB timers does
not satisfy the full-window requirement. Emit no per-operation disk/log write.
Failure/cancellation/panic paths preserve their existing results and cleanup.

## 4. Exact accounting is not the same as complete classification

The selected wall window is from immediately before the collector's POST to
its observation of terminal success, excluding fixture setup, result oracles,
post-timer observer waiting and test cleanup. Keep those outside-window costs
separately visible. Terminal publication may race trailing worker/report work:
intersect intervals with the window and explicitly identify outside-window tails,
never extend the timed API boundary to wait for the recorder.

Partition at every start/end boundary into ordered `[StartNS, EndNS)` segments:

- one deepest active engine span: operation/phase/ordinal/source owns the segment;
- concurrent incomparable engine branches: one overlap segment retains their
  closed owner references; do not sum both durations or guess a critical path;
- no engine span: classify observed POST/status transport/poll wait on the client
  axis, without adding client request time overlapping engine work a second time;
- self-time/gaps: retain the enclosing source owner plus `unresolved` when a
  causal operation is still unknown. Before/after-worker regions without a
  service observation are unresolved, not guessed network or SQLite work.

For integer nanoseconds require start=0, adjacency, nonnegative duration,
last-end=TimelineElapsedNS and `sum(DurationNS)=TimelineElapsedNS` exactly.
Nested inclusive tables and helper detail are explicitly non-additive. Derive
the per-step and operation totals from this ledger; percentages use its stated
denominator. Keep old inclusive views and original API denominator distinguishable.
Round only for display, retaining an exact total and rounding note.

The report separately lists source-local unresolved self time and concurrency;
putting all residual time under `prepare.worker` does not complete attribution.
Expose every unresolved segment, total and largest source regions. Do not claim
the whole regression is explained merely because arithmetic balances. A missing
required boundary is a qualification failure, not zero time or a valid partial
report. Do not invent a numeric causal-completeness threshold after seeing data.

## 5. Repeat and review

After separate test approval and implementation, run ordinary regressions,
independent Go/Node accounting checks, new helper coverage (minimum 95%), and
native three-step/cache/protection/cleanup checks in Actions. Then pin one new
candidate and repeat the unchanged CT13 schedule: actual v1, old v2, new recording
off/on, reverse order, eight members / 16 passes on one Ubuntu VM. Original v1
stays unmodified; its internal times are unavailable, not zero. Keep all ordinary
speed controls, artifacts, hashes, independent oracles and cleanup checks.
Compare the instrumented/off variants to disclose tracing cost. The retained
P01b speed gate and #170–#174 scope boundaries are unchanged; no laptop workloads.

Critical review corrected: summing nested operations; confusing worker/API
origins; claiming unknown self time as classified work; mixed helper clocks;
unmeasured DB reuse price; and changing product behavior during diagnosis.
Repeat review added admission/dispatch tracing, request-cancellation separation,
client/engine overlap handling, terminal/report races, explicit shared-clock
transport, old-timer reconciliation and unsupported-platform refusal. No remaining
P0–P2 contradiction found in this proposal. A final pass additionally made the
request/worker linkage, historical root shape, actual emission overlap and
bounded output ownership explicit. At that review implementation/tests had not
begun; the implementation checkpoint below supersedes that historical status.

## 6. Implementation checkpoint, not measurement acceptance

TL01–TL14 and the narrow new-oracle correction (seven outside-window intervals,
not six, with their exact owners/bounds) are explicitly approved. Tests first
failed for missing Go/Node timeline helpers, shared clock types, wire frames,
collector reader and admission carrier. Implemented so far:

- Independent bounded chronological partitioners preserve source owners,
  unresolved self, parallel owners, client overlap and original outside tails.
  The five Node arithmetic/timer tests pass with 100% new-helper lines. The
  original/full Node suite was green at 87 tests before the timer test was added.
- A private Linux clock/reference and closed clock/observer-window frames, with
  independent collector parsing and distinct complete-versus-legacy waits.
  The real Linux two-process clock/recorder tests are written but not executed
  on the Windows host. No native/shared-clock qualification is claimed yet.
- Opt-in in-memory admission/request/worker scopes, detached cancellation and
  asynchronous emission so Submit does not wait for a blocked observer. Focused
  new scope/Submit tests and unchanged old cost-worker tests pass on Windows.
- Independent signed timer-bracket reconciliation helpers pass; the original
  performance timer is not corrected. Focused collector stream/cost tests and
  vet pass without old-test edits. Complete collector selection and actual
  API-window probes are not connected yet.

Windows preparecost previously measured 97.6% after arithmetic/initial clock
work; adding Linux-only recorder/emitter branches produces 93.8% in the later
Windows-only profile. BuildTimeline itself is 96%. These are partial profiles,
not the TL12 coverage claim; Linux execution and remaining initial tests are
required. Detailed planning/DB/session probes, actual collector/report
integration, TL13 and the new CT13 campaign remain open. No new speed result,
full classification or #89 completion is asserted.

The later full ordinary Windows prepare package passes (44.819 s), the full
ordinary collector module passes, and all 88 Node tests pass. Old tests have not
been edited. OT-TL01 is separately requested for the existing Linux workload
collector; it is not covered by the new-test approval and is not yet applied.
