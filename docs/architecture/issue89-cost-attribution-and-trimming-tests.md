# #89: tests for duplicate-work removal and operation costs

Status, 2026-11-10: [components approved](issue89-cost-attribution-and-trimming.md)
by @evilguest. CT01–CT14 below have undergone critical and repeat review and are
**explicitly approved before implementation**; exact reply timestamp unavailable.
CT approval itself does not authorise old test changes. The post-approval audit
below records separately approved OT-CT01/OT-CT02 and new-test corrections; preserve all
other controls.

## Inventory and independent expectations

1. **CT01 — source correspondence/catalog.** A closed, revisioned operation and
   call-site catalog covers shared/changed/shifted/additional/unresolved entries
   in design §1. Check pinned v1/v2 symbols against their actual source and the
   newly frozen candidate, not merely the current HEAD. In particular stop/
   status/permission changes, snapshot, restart and existing managed verification/
   seal cannot be wholly counted as v2 additions. Helper application is changed
   implementation, not a measured JVM-only interval. Absent inner v1 timings are
   unavailable. Every emitted key has a catalog/source entry; reject unknown keys/
   relation/version. Source guards supplement CT08/CT12 behavior, not replace it.
2. **CT02 — O1 exact positive inspection.** Real runtime method with recording
   runner: one inspect addressed to the full saved ID, no list, unchanged decoded
   fields. Reject empty/short/malformed IDs and invalid names before transport;
   reject foreign ID/name, malformed/truncated JSON, missing/unsupported phase,
   daemon error, vanished/renamed container and cancellation. Required caller
   phase/labels remain checked. Failures never mean absence or trigger list,
   removal, create or SQL. Exercise actual invocation/output/readiness/prune
   callers: both original pre/post boundaries are reached and later mismatches
   refuse success. Existing name-discovery and exact-absence methods stay intact.
3. **CT03 — O2 one ensure per binding entry.** Actual admitted immutable-input
   binding with real temporary store and counting resolver/runtime: fresh binding
   resolves once, ensures once before inventory, inventories once, then commits
   identical image/driver/config/fingerprint semantics. Saved binding ensures once
   without re-inventory; a different new job resolves again, including a moved
   tag. Errors at ensure/inventory/bind do not fabricate a binding, change pinned
   identity, start SQL or add an implicit retry. Prove counts and the exact reached
   failure with an independent stored-row/input check, not counters alone.
4. **CT04 — O3 bounded three-file bundle.** Real archive bytes through runtime:
   exactly one cp of the exact invocation directory; manifest/streams and hashes
   equal the independent expected bytes. Three ordinary files in any order, an
   exact optional directory header and supported safe PAX; empty streams valid.
   Reject missing/duplicate/extra/partial/renamed files, traversal/absolute paths,
   links/sparse entries, bad checksum/header/EOF/padding, unsafe/oversized PAX,
   per-file and aggregate cap+1, malicious integer overflow and invalid limits.
   Exact caps accepted. No extraction or allocation from declared huge sizes;
   no partial success. Deadline/cancellation/transport errors and missing bounded
   runner fail closed, without fallback to three cp/local host paths. Parent
   pre/post/idle/manifest checks remain effective; root planning and filesystem
   readers retain their existing behavior. Assert actual bytes/calls/errors and
   intended refusal boundary so a wrong earlier failure cannot pass.
5. **CT05 — O4 keep flush barriers.** Check both actual embedded helper scripts,
   not a duplicated toy: four terminal flush-command boundaries/order remain,
   one same-filesystem argument per boundary; preceding prune/readiness flushes
   unchanged. Native Actions verifies the pinned image's sync implementation and
   semantics before enabling this optimisation. A recording shim delegates to
   the real sync and verifies calls/ordering; it is never used for speed samples.
   Inject failure at each boundary and preserve the helper's nonzero result.
   Do not wrongly require a missing manifest after failure of the **final
   post-rename** flush; do require unchanged hashes/closed streams on success.
   Keep ownership, quotas, cleanup, helper digest and manifest revision checks.
   This is not a new power-loss/durability qualification or #170 recovery matrix.
6. **CT06 — default-off selection and recorder isolation.** Only explicit startup
   `SQLRS_PREPARE_COST=1` enables recording; absent/other values and default debug
   alone do not. Real constructor/worker propagation, separate roots/step ordinals,
   nil/no-op behavior: no timer storage/output or new execution. Image/request
   environment cannot enable a child; engine passes an explicit off/on override.
   Same product outcome, DB bytes, events, fingerprints and CLI counts with off/on.
   Measurement state is not persisted in the execution binding/schema/API.
7. **CT07 — span lifecycle/completeness.** Real recorder plus deterministic pure
   aggregation inputs (no forged time.Time or global clock hook): nested scopes,
   sibling scopes and concurrent roots/branches, double finish, post-finish write,
   foreign/missing parent, invalid key/ordinal, unfinished span, zero/negative/
   overflowing synthetic boundaries and 32768/32769 spans. Closed output is
   immutable. Invalid/lost evidence is incomplete, not silently zero/complete.
   Successful, error, cancellation and panic worker exits retain their original
   result/panic; measurement failure cannot grant success, replace an error or
   repeat cleanup/SQL. Run races on supported Linux helpers.
8. **CT08 — actual named host boundaries.** Counting runtime/DBMS/statefs/journal
   doubles at existing seams exercise planning, session startup, two successive
   uncached steps, cached steps/prefix, publication and cleanup; assert named
   caller key, nesting and real call counts for each executed action. In particular
   distinguish capture read, prune observation, journal-only work, DB restart,
   DB verification, snapshot and size measurement. Record failures at those same
   boundaries, not a synthetic cost-only success. Startup is once, restart moves
   between steps/final task without becoming extra; cached steps have zero applies.
   No new global interception interface. Native CT12 confirms actual wiring.
9. **CT09 — helper-local detail.** Fixed phase records for the actual helper's
   preflight/prune, CLI, drain, hash/rename, four flush regions. Off mode performs
   no uptime reads/output; on mode starts the same single CLI and no timer JVM/
   `date` process. Pure decoding of known local numeric records exercises 10 ms
   quantisation, zero short region, missing/invalid/reversed/overflowed readings
   and inconsistent containing host duration; native tests use real uptime,
   without requiring clock manipulation or a new helper clock override.
   Timer/parser failure preserves helper execution/exit and marks
   detail unavailable/incomplete. At most 4 KiB per invocation; no SQL/IDs/paths/
   arguments/streams in records. Host binds detail to its own invocation, not a
   claimed child identity. Keep local clock/resolution and cumulative uncertainty
   separate: no invented host-axis offsets, suspend equivalence or JVM inner cost.
   Syntax/source checks alone do not satisfy the native helper cases in CT12.
10. **CT10 — private framed transport.** Real pipe/fragmented reads: complete begin/
    span/end frames, closed key/number grammar, correct root association/count and
    bounded delayed end frame after the unchanged API timer. Reject duplicate,
    per-root reordered/incomplete/foreign frames, unknown fields/versions, nonfinite/
    overflowing values, 4 KiB cost-line+1 and 8 MiB engine cap+1; exact limits
    accepted. Missing end, broken pipe, read error and cancellation cannot qualify
    a partial measured sample. Unrelated huge/secret-containing log lines are
    drained with bounded storage, never parsed/uploaded/printed. Insert private
    sentinels in all irrelevant/error data and verify serialized reports/console.
    Valid interleaving of concurrent roots and reverse root completion are
    accepted; never mistake process-local start order for completion order.
    Product outcomes are unchanged; owned reader/process cleanup is confirmed.
11. **CT11 — independent accounting oracle.** Pure numeric examples with known
    results: nested/overlapping/adjacent spans, concurrent siblings, repeated key,
    empty/cache-hit step and once-per-job startup. Check counts, per-call versus
    per-step mean/distribution, inclusive time, child-union subtraction, category
    union ZTotalMS and `100*ZTotalMS/PrepareElapsedMS`. A shared-overlap bucket
    makes the additive overview conserve measured time; nested category rows
    remain explicitly non-additive. API denominator is positive and unchanged;
    uncovered admission/response/engine work remains unassigned, never SQLite.
    Reject invalid containment, over-attribution, bad associations and partial
    evidence. Helper-local rows retain resolution/uncertainty and do not also
    increase host totals. Verify Go/Node independently, not one implementation's
    output used as the other's expected answer. Concrete oracle: API 100 ms,
    parent [0,80], children [10,30]/[20,40] of one key: child union 30 ms (30%),
    raw call sum 40 ms, parent self 50 ms; additive child-only 10+10, shared 10,
    parent 50 and unassigned 20 sum to 100, not 110.
12. **CT12 — native functional proof in Actions.** No laptop workloads. Actual
    image/helper, root/non-root child isolation and fresh temporary stores:
    three-or-more changes in one LB/DB session, real output bundle/hash checks,
    correct prior-output retirement and unchanged final DB handoff. Prove reduced
    commands at the intended code boundary without a Docker wrapper in measured
    samples. Observe off/on host/helper rows, zero/unavailable distinction and
    exact per-step apply counts. Wrong parent/name, incomplete output and a
    surviving child still block completion. Oracles/cleanup are checked **before
    test cleanup**, not excused by it; retain foreign-resource controls. Required
    Actions cases may not silently skip. Existing Windows/Linux regressions stay.
13. **CT13 — pins/scheduling/observer control.** Pure Go/Node/workflow checks and
    actual short run: explicit same-repo launch only, one Ubuntu VM, four distinct
    routes in two reversed repetitions, eight members/16 passes. Actual v1 and
    old v2 retain exact hashes; new off/on use identical new executable/helper
    hashes and distinct measurement selection. Clear inherited measurement flags
    for other routes; warm-up outside API timers. No strace, laptop or private
    izess. Preserve host budget, process/resource ownership, incomplete reports
    and all oracles. Off/on totals expose observation overhead, not a made-up
    per-row correction. Do not run partial pairs as a complete speed verdict.
14. **CT14 — regressions, coverage, unchanged speed gate.** Full affected engine
    and collector/Node tests/vet, then Linux/Windows regressions and CI (including
    the existing macOS jobs). Target
    new ordinary recorder/decoder/report helpers 100%, minimum 95%; report exact
    denominators separately from integration orchestration/product. Gaps require
    line-to-requirement analysis and approved iteration, not dead-code guesses.
    Aggregate product exception remains #89/#174 only. Preserve old parser/tests/
    reports; do not alter old tests without named approval. After short-run
    assessment, repeat full P01b with new pinned candidate: all three examples,
    complete paired/noise/confirmation cohorts in Actions. Functional success
    or a diagnostic report does not satisfy the no-regression gate.

## Critical review and corrections

- P1 false savings by weakened checks: CT02/CT04/CT05 test actual refusal/order,
  independently verifying reached boundaries and unchanged external outcomes.
- P1 noisy traces presented as ordinary cost: CT06/CT13 separate default-off,
  recording-on and before/after; no per-row overhead correction is invented.
- P1 helper clock made equivalent to host/Java timing: CT09/CT11 preserve its
  resolution, local axis, missing/zero distinction and non-additive role.
- P1 stderr leaks or partial output qualifies: CT10 uses actual pipes, bounded
  fragments/frames/end proof and private sentinels; no raw artifact.
- P1 source-only tests green while real paths bypass recording: CT08/CT12 require
  actual calls, native code wiring, data and cleanup before fixture disposal.
- P1 fabricated attribution: CT01/CT11 separately check correspondence, shifted
  restart, existing v1 checks, nested/concurrent accounting and unavailable time.
- P2 demanding absence after post-rename sync failure or changing old tests
  implicitly: CT05 preserves the real boundary; amendments need named approval.
- P1 treating concurrent root completion as malformed ordering, or checking
  overlap math against its own implementation: CT10 allows valid interleaving;
  CT11 contains an explicit independently calculated 100 ms example.

Repeat review found no remaining P0–P2 within this inventory. This is not a
coverage-only expansion, a new P01b statistical rule, or approval of old-test
rewrites. After inventory approval audit old tests, then write new tests first.

## Post-approval audit: OT-CT01 (approved)

Nine existing runtime tests encode the old list+inspect response sequence and,
where applicable, three file copies. Their semantic checks remain required:

- `TestDockerReadLiquibaseStepOutputRequiresIdleExactRunner`;
- `TestInvokeTrackedLiquibaseStepRequiresCapturedResult`;
- `TestLiquibasePrivatePruneInvocationBinding`;
- `TestCaptureAccessSessionPostReadProof`;
- `TestLiquibasePrivateReadinessExactObservation`;
- `TestLiquibasePrivateReadinessRejectsInvalidBinding`;
- `TestLiquibasePrivateReadinessDeadlineAndCancellation`;
- `TestMainPathCapturePruneProbe`;
- `TestMainPathCapturePruneEmptyAuthorityOverridesEnvironment`.

The user approved limited permission to supply exact-ID JSON responses instead of list+JSON
only in these tests/their readinessRequirementsRunner; for session reads provide
a one-copy three-file archive through a new fixture helper. Keep global
`accessInspection` and `privateCaptureArchives` unchanged for planning/discovery.
Update corresponding command/copy counts and indices, cancellation barrier 4→2
for final readiness inspection; model absent exact inspection with an explicit
Docker refusal, not successful empty JSON. Preserve every case, exit/EOF/hash/
binding/idle/privacy/deadline/cancellation assertion and precise failure boundary.
Native tests and all other old fixtures are unchanged. Any later contradiction
requires a separate named amendment. CT tests/independent implementation may
proceed; these nine old tests must not be weakened or rewritten beyond this permission.

### Additional full-runtime finding: OT-CT02 (approved)

`TestPrivateStepRuntimeLiveAndStopped` also supplies list+inspect and three copies.
The user approved changing only its fixture to `presentInspection`/`sessionBundleFixture`
and its copy count 3→1. Both running/exited cases, child exit 7 versus parent exit
23, and exactly one idle probe only for the running branch remain required.
No planning fixture change. The initial full ordinary runtime run failed these
two positive cases; after the approved amendment the full runtime passes.
This is not a native Docker result.

### New disabled-recorder test correction (approved)

`TestRecorderExplicitSelectionAndDisabledPath` puts a `reflect.DeepEqual` on two
Report values inside `AllocsPerRun`. That assertion itself boxes a slice-bearing
struct and adds one allocation. The user approved moving only the nil-recorder Report
comparison outside the allocation measurement; keep the zero-allocation test
for disabled WithRecorder/WithPhase/Start/end and both semantic assertions.
Do not misclassify test assertion overhead as production recording overhead.

### Exact boundary in new CT09 (approved)

The negative overflow example in
`TestHelperCostDetailRejectsIncompleteOrPrivateInput` uses 922337203685
centiseconds, which is still representable as int64 nanoseconds. The user approved replacing
only this negative value with 922337203686 and adding an exact-maximum positive
check. Do not narrow the product's valid range to satisfy a mistaken expectation;
retain the overflow guard.

All three approved corrections are applied. The full ordinary runtime and
preparecost tests pass; the latter is 99.6% covered before worker/collector wiring.

### OT-CT03: connect cost observations in collect (approved)

The existing Linux `TestReleasedEngine/collect` discards stderr and can inherit
`SQLRS_PREPARE_COST`. Its oracles/timer/process ownership must remain authoritative.
The user approved only an explicitly selected cost branch: remove inherited measurement
flags for every route; enable them only for the selected new-v2 measured route;
attach a bounded private stderr reader there; after the existing API timer stops,
await the matching complete root with a bounded deadline; retain only validated
numeric/enum evidence and observer-wait duration. Extend existing owned-process
cleanup to close/wait that reader before deleting its root and exposing samples.
No trace branch change, extra prepare requests, timing-boundary move, relaxed
artifact identity, oracle/counter/event/cleanup assertion or raw-log output.
Before/after source selection and the unchanged API time remain independent.

Critical/repeat review: missing/late frames must fail the measured sample, not
extend API time; cancellation/read/byte failures must use the existing exact
process cleanup; off/v1/old-v2 cannot inherit measurements. Existing collect
code was unchanged before this named approval. New receiver tests may proceed.

### Native finding OT-CT04 (approved)

Actions `42854975079` confirms GNU coreutils 9.4 and the same-filesystem directory
sync prerequisite. In the existing real reusable-runner test, three JVMs in one
runner and the surviving-child case pass. Only `incomplete manifest` fails its
old exact error text: O3 rejects the two-file archive before the old manifest
reader. The user approved changing only that expected refusal to `errCaptureArtifactRead`
and independently confirming the exact running parent and both unchanged private
streams after deleting only manifest.complete. Retain all three scenarios,
exact-ID cleanup, exit/hash/reuse/prune assertions and the negative refusal.
Do not map unrelated archive errors to a fabricated manifest-specific error in
product code just to satisfy the obsolete expectation. Other old assertions stay.

### CT12 native intermediate outcomes (approved 2026-11-11)

The user approved correcting only the new test's overstrict expectation that
every intermediate Docker command succeeds. Keep each closed actual outcome,
including refused/timed-out readiness probes; require the successful worker and
mandatory completed named operations. Preserve exact database data, cache/step
counts, parent/database reuse and cleanup-before-fallback assertions. Do not
change product retry behavior or turn a terminal prepare failure into success.

## Detail transport DT01–DT08 (approved, 2026-11-11)

Concrete cases for the approved [§3 refinement](issue89-cost-attribution-and-trimming.md),
within CT06–CT11/CT14. No new recovery matrix or speed criterion. After approval,
write tests first at existing boundaries, without global clock hooks or new
production injection interfaces.

1. **DT01 — owned evidence.** Bind detail to the recorder's own narrow planning
   or step invocation and actual containing-call duration. Check successive
   steps/concurrent roots; reject missing/foreign/wrong-kind/duplicate owners,
   late writes, malformed/incomplete phases and incompatible duration. Invalid
   detail is unavailable, not eight zeros; valid quantised zero stays distinct.
   Input, returned reports and nested slices cannot mutate retained evidence.
   Original errors/panics and the disabled zero-allocation path remain unchanged.
2. **DT02 — helper timers.** Both actual embedded scripts use builtin uptime
   reads only when explicitly enabled: eight ordered phases, no date/timer
   subprocess/JVM. Serve/readiness/prune stay off despite inherited settings.
   Keep all existing numeric/order/quota/overflow/host-bound decoder cases.
   Actual host-runner cases supply missing/malformed/oversized/private output:
   detail becomes unavailable without changing CLI/capture results or leaking
   raw bytes. Check timer failure cannot enter execution-failure control flow.
   Native Actions verifies real uptime availability, on/off exit/stream equality
   and one CLI start, not artificial clock manipulation. Four sync barriers and
   capture protocol remain; changed helper bytes receive their actual new pin.
3. **DT03 — literal codec oracles.** Preserve begin/span/end and span counts.
   After spans, before end: `sqlrs-prepare-cost-v1 helper ROOT OWNER PHASE START_CS END_CS HOST_NS`,
   or `sqlrs-prepare-cost-v1 helper ROOT OWNER unavailable HOST_NS`.
   After end: `sqlrs-prepare-cost-v1 observer ROOT ENCODE_NS LOCK_WAIT_NS WRITE_NS`.
   Each owner has eight existing phases or one unavailable marker, never both.
   Independently written literal cases test arity, unknown names, canonical
   uint64 IDs/nonnegative int64 durations/bounded centiseconds, missing/extra/
   duplicate phases, wrong root/owner/kind/duration. HOST_NS is identical across
   an owner's phases and cannot exceed its containing owner span. An incomplete
   host report suppresses detail as well as spans. No partial/private output.
4. **DT04 — emission ownership.** Recording/blocking writers observe report then
   exactly one observer for that root under the same lock; concurrent roots do
   not interleave writes. Nonnegative encoding/lock-wait/framed-write timings
   describe actual intervals, not exact scheduler-dependent values. Independently
   inject short/error first and second writes: first failure emits no observer,
   second failure cannot confirm delivery. Retain nil-writer, safe errors and
   validation-before-I/O. Snapshot copying and marker construction/write are
   explicitly unmeasured, never claimed as zero or total observer cost.
5. **DT05 — full completion.** Real fragmented pipes delay observer beyond end;
   end alone cannot qualify a measured root. Refuse observer-before-end,
   duplicate/foreign observer, helper-after-end, bad durations or missing expected
   detail. Accept valid interleaving/reversed root completion. Exercise EOF/read
   error/broken pipe/cancellation/deadline after end, existing exact line/engine
   caps and cap+1 including new records, private long-log draining and immutable
   snapshots. Wait for both markers under one existing deadline after the
   unchanged API timer; retain owned process/reader/file cleanup. Diagnostic
   failure never changes prepare success or repeats SQL. Standalone host-only
   codec tests are not full measured qualification.
6. **DT06 — actual wiring.** Counting runners reach actual tracked planning and
   step calls. One narrow planning invocation receives its source-catalog entry,
   not the whole preview DB lifetime; one detail per applied step, none for cached
   steps. Explicit on/off overrides cover both paths; probes/serve/prune stay off.
   Host duration covers the helper call, not subsequent capture reading. Preserve
   original failures/cancellation/panics, command/capture/cleanup counts and no
   retries. No helper data in API, captured Liquibase bytes or exposed errors.
7. **DT07 — independent accounting/report.** Go/Node independently validate
   owners/order/bounds/observer and sanitize output. Literal oracle: API 100 ms,
   worker 80 ms, invocation 30 ms, helper CLI 20 ms + seven zero phases; observer
   encoding/wait/write 2/3/4 ms. Host total still conserves 100 ms (20 unassigned);
   helper 20 and observer 9 are separate non-additive detail, not added/subtracted.
   Preserve source/step association, helper 10 ms resolution/cumulative bound,
   measured observer components and two explicitly unmeasured categories.
   Missing is not zero. Refuse missing/duplicate/wrong owners/observer, unsafe
   JSON integers, negative/overflowed values and incomplete later roots. Existing
   extra-field dropping/privacy remains. Do not require observer durations to
   fit the API window; overall impact still needs off/on controls.
8. **DT08 — qualification.** Existing explicit Actions probe, no laptop workload:
   three-change fresh pass has four helper owners (planner + three applies),
   cached pass one planner/zero applies; eight phases per owner, one observer per
   root. Retain all DB/count/reuse/output-retirement/handoff/cleanup-before-fallback
   assertions and unchanged default-off full prepare. Run affected ordinary Go
   tests/vet, collector Linux compilation/vet and all Node tests; separately
   measure ordinary helpers (target 100%, minimum 95%). Functional qualification
   does not replace approved before/after/off/on runs or the full P01b gate.

### Named old-test conflicts (OT-CT05–OT-CT08; separately permitted)

- **OT-CT05:** only `TestCostWireWriterAndConcurrentRoots`: four lines per root
  become five (observer after the same end). Preserve 16 roots, root/end checks,
  all write failures and validation-before-I/O. Pure `TestCostWireClosedFrames`
  stays unchanged as a span-only encoder oracle.
- **OT-CT06:** only positive preparation of
  `TestCostCollectionQualifiesActualWorkerAndAPIWindow`: add valid planning
  owner/detail and observer. Preserve all 14 negative cases and their intended
  failure boundaries; shared `costStreamExample`/standalone stream tests stay.
- **OT-CT07:** only common positive fixture of the five groups in
  `scripts/performance/cost-output.test.mjs`: add planning owner, planning/apply
  detail and observer. Adjust derived IDs only to avoid introduced collisions or
  wrong earlier failures; retain every existing rejection/source/conservation/
  privacy/outcome/zero-cache-apply assertion. New missing-detail refusals are
  additional cases, not replacements.
- **OT-CT08:** only `TestCostMainPathNamedAttributionDocker`: recognise new closed
  records/narrow planning count and add exact DT08 counts/associations. Preserve
  current operation/step/outcome checks, real DB rows, reuse, publication and
  cleanup; unknown records still fail. Other native tests stay unchanged.

No blanket fixture rewrite or alternate dead product API is authorised.
Further contradictions require named approval.

### Critical and repeat review

- P1 premature completion/self-measurement: require observer; explicitly leave
  snapshot/marker cost unmeasured, never fold these into a claimed total.
- P1 plausible wrong per-step attribution: host selects the narrow owner and
  duration; independent literals test identity, clock domain and conservation.
  Repeat review also made identical per-owner duration, owner containment and
  suppression of helper payload on incomplete host reports explicit.
- P1 timer/diagnostic failure changes execution: require explicit off overrides,
  original CLI outcomes, unchanged API timing and owned cleanup.
- P2 flaky/impossible checks: controlled writer ordering/actual uptime, no exact
  scheduler duration or native clock mutation. Old negatives must reach their
  own fault after fixture expansion, not pass due to missing metadata.

Repeat review found no remaining P0–P2 in this bounded supplement. @evilguest
separately approved DT01–DT08 and permitted OT-CT05–OT-CT08; exact reply time
unavailable. Tests-first implementation may proceed. Approval is not a runtime
qualification or a speed result.

### Line-mapped follow-up DT-C1 (approved and implemented)

First complete collector profile: `cost_detail.go` 50/54 statements (92.59%).
The four uncovered returns implement existing DT03/DT05/DT07 refusals, not dead
code: malformed helper arity (line 32), invalid/repeated unavailable marker (49),
local time above the host bound (75), and absent/invalid observer in direct
qualification (94). Add independent literal cases for these, plus incomplete
phases before end (`cost_stream.go` line 165), then repeat full helper tests/vet
and the per-line profile. No old tests, product boundaries or code removal.
Critical/repeat review retains reached-boundary checks, safe errors and valid
controls. @evilguest separately replied “Разрешить адресную итерацию” on
2026-11-11 (exact time unavailable). The added literal/direct cases pass; the
complete repeat profile gives `cost_detail.go` 54/54 statements (100%), all its
functions 100%, collector cost selection/qualification 100%, stream parser 98.3%
and other stream functions 100%. Full Windows ordinary module tests/vet pass;
its whole 58.9% profile is not Linux integration/helper coverage. Linux build/vet
and prior native proof remain separately identified; no speed verdict follows.

### CT13 execution checkpoint (2026-11-11)

Actions `42967877614` on `48e6e11` completes the actual image prerequisite and
all eight serial before/after/off/on members / 16 passes. Each workload's data,
cache, counters and cleanup qualifies before its costs leave the private child
output. Both new modes have the same executable/helper. The retained JSON and
independently recalculated source/step totals are in
[the results](issue89-cost-results-2026-11-11.md). The new ordinary fresh route
is about 1 s faster than old v2, still 18.00% slower than actual v1; cached is
20.83% slower than v1. CT14's unchanged full P01b remains blocked, not waived.
The old Windows HTTP/SQLite BUSY CI failure is retained separately: its unchanged
job passes on attempt 2, as do 20 local repeats. Only aggregate coverage is now
red; this does not claim a fix for the first attempt's contention.

## Combined native observation: CPR01–CPR09

Status: components §6 and **CPR01–CPR09 separately approved** on 2026-11-11;
@evilguest replied “Утвердить CPR01–CPR09”, exact reply time unavailable.
Tests-first implementation may proceed. This supplement adds no API/schema, general recovery matrix, image
inventory cache or permission to modify existing tests. Test names below use
the `TestCombinedCapturePrune` prefix.

1. **CPR01 — real runtime, ordered successful observation.** A recording bounded
   runner supplies independently authored exact inspect JSON, valid tar/manifest/
   streams and the combined proof response. Call the real runtime with a saved
   prior authorization. Require order: exact pre-inspect, one archive, one
   bounded root exec, exact post-inspect; no list, separate idle/prune round trip,
   create, removal or restart. For `InvokeTrackedLiquibaseStep`, the initial
   inspect and applying helper exec remain, so six total Docker operations
   including the archive, not six extra operations. Check exact IDs/name, every
   authorization environment value, explicit diagnostics-off, unchanged hash/
   bytes/exit and a valid receipt only after the last inspection. The combined
   script must perform idle / pinned read-only `probe-prune` / idle, never apply
   or delete. The fixed complete response, not zero exit, qualifies it. First
   invocation without prior authority keeps the old read/idle path and no receipt.
2. **CPR02 — receipt ownership and exact matching.** Obtain a positive receipt
   only through real runtime observation at CPR01's seam. Zero/default receipt,
   local manifest-only decoding and manually assembled capture cannot prove
   native absence. Independently vary each saved authorization field (retirement,
   session, generation, runner, predecessor invocation/digest, current invocation,
   helper) and the expected current job/task/attempt/execution identity and
   completion digest/length: refuse each mismatch. Copy pointer-backed links/
   authorization before retention; mutating caller values cannot retarget the
   original proof. A copy may match only the same original observation, not a
   different invocation. No test-only exported proof constructor/global hook.
3. **CPR03 — intended refusal boundaries and quotas.** With otherwise valid
   earlier responses, independently fail pre-inspect, archive, the combined
   exec and post-inspect. The three internal script stages are exercised by
   CPR08's actual-command harness, not falsely distinguished by returning the
   same fake Docker error. Both new exec/parent errors
   and cancellation/deadline paths must reach their selected boundary, return
   no usable receipt, make no later call and keep bounded output private. Include
   proof response missing/partial/foreign/extra bytes, nonzero command status,
   and changed post-ID/name/state. Assert bounded-runner capability and fixed
   stdout/stderr limits before command effects; existing cap/cap+1 runner tests
   stay intact. A valid manifest with prior link alone is insufficient if actual
   absence fails. Existing archive/hash/EOF refusals remain, not rewritten as
   acceptable partial output. Positive control reaches all four read operations.
4. **CPR04 — standalone/stopped paths do not fabricate proof.** No-prior read
   keeps its one archive/idle behavior. An exact exited parent can still yield
   the same known child result and stream bytes, but receives no exec/restart
   and no combined receipt; this holds even with a prior manifest link. Retain
   and exercise real standalone `VerifyLiquibaseCapturePruned` with its prior
   exact binding, read-only command and refusal behavior. It must not become a
   no-op or be automatically used as a success fallback for a missing receipt.
5. **CPR05 — coordinator plus real queue authority/atomicity.** Through the
   existing seams, use a real temporary managed SQLite queue and a runtime
   observation backed by controlled native-transport responses. Publish the
   first state, admit the second invocation, observe its exact completion and
   confirm pruning: only the previous slot is released, exact retirement becomes
   verified, current slot/history/state protection remain. Third invocation can
   use the freed slot; no extra Liquibase SQL or proof command is issued by the queue
   verifier. Wrong/missing receipt or changed saved authority leaves the journal
   unchanged and cannot advance completion. Physical probe is outside the write
   transaction. Reuse the existing journal boundaries for a refusal before
   effect and a lost successful COMMIT reply: independently read the committed
   rows, preserve the correct before/after state and never repeat SQL/native
   observation automatically. Do not add the deferred exhaustive COMMIT matrix.
6. **CPR06 — child outcome and cancellation are not overwritten.** Exercise
   child exits 0 and 7 with independently valid captures. Successful prune proof
   does not replace the original exit; the ordinary caller still selects failure
   for 7, without applying another change. A failed proof leaves prior protection
   unresolved, not a fake complete invocation/job. Cancellation before read,
   during bounded observation and before confirmation preserves cancellation/
   refusal and records no fabricated absence. Verify exact reached calls and
   journal state, not merely a non-nil error. No automatic replay/recovery added.
7. **CPR07 — cost boundaries, including recording off.** Actual runtime +
   preparecost recorder: combined native proof is inside the existing
   `capture.read` parent; normal completion has no separate `capture.prune` span
   or duplicated command. Standalone proof still has its real `capture.prune`
   boundary. No recording yields no records and the same transport/child result;
   enabling recording does not enable helper timers for the read-only probe.
   Independently specified old read+prune and new combined interval examples
   must conserve API time and produce the same total for equal work, not an
   apparent saving from the renamed boundary. Preserve source catalog keys and
   historical helper/observer decoders; no product API timing change.
8. **CPR08 — actual Docker and ordinary prepare, Actions only.** A new native
   runtime test uses the actual pinned helper and exact real container. Second
   invocation removes the first output under the saved authority; combined read
   supplies a matching receipt and unchanged streams/child exit, with the parent
   still reused. Separate controlled cases recreate the previous directory after
   the child's valid manifest and retain a surviving untrusted child: real absence/
   idle checks must refuse, despite otherwise valid capture. Separately execute
   the actual combined shell command in a fresh controlled container with a
   read-only **test-only probe stand-in**. Deterministically fail its first idle,
   absence reply and second idle (the stand-in starts a UID-1001 survivor before
   returning the otherwise correct reply). A positive control in the same
   harness must reach all three stages and return a valid complete response.
   Assert actual stage/order and refusal;
   command source/text checks alone do not qualify these stages. This harness
   qualifies the wrapper only, never substitutes for the real pinned-helper
   cases above. No product hook or existing fixture mutation. Check the intended
   boundary and exact resource cleanup before fallback. Add the tests to the
   existing explicit native Actions selection; zero selected/skipped tests are
   not proof. Real connected three-step/cache prepare must still verify prefix/
   final DB rows, once-per-step apply, one reusable runner/working DB, journal
   slot history, published DB handoff and normal cleanup before fixture cleanup.
   Existing native assertions are not implicitly changed by this list; address
   any contradicting fixture/count expectation separately after approval.
9. **CPR09 — regression, coverage and same-VM comparison.** Full ordinary
   engine/client tests and vet on current source, Windows/Linux via established
   CI; new focused tests are selected explicitly, not silently skipped. Obtain
   per-line profiles for new runtime/prepare proof helpers: target 100%, minimum
   95%; the existing #89/#174 aggregate exception is not helper qualification.
   Any shortfall requires the normal line-mapped approved iteration. After native
   qualification, repeat the existing eight-member CT13 in Actions, pinning new
   binary/helper/collector hashes and retaining original `42967877614`. Report
   combined read versus previous read+prune, unchanged timer, per-step counts,
   non-additive helper detail and off/on variation honestly. No laptop benchmark,
   protocol/noise waiver or inferred full-P01b pass. Full P01b follows only after
   resolving the short-workload regression; the original speed gate remains.

### Critical review, corrections and repeat review

- P1 false native proof: added a positive real-runtime receipt and explicit
  rejection of default/local-manifest/foreign proof. Receipt must bind both the
  committed prior authority and exact current completion, including copied data.
- P1 false green on an earlier error: every refusal has valid preceding responses
  and reached-boundary/no-later-effect assertions; native recreated-directory and
  live-child controls cannot pass simply because the fixture was not initialized.
- P1 indistinguishable inner faults: a mocked single exec cannot establish which
  inner stage failed. Add actual-wrapper execution with a separately labelled
  controlled probe stand-in, alongside the real pinned-helper qualification;
  make the second-idle survivor visible before the stand-in replies.
- P1 atomicity/authority loss: use real queue state/publication/slot transitions,
  retain before-effect/lost-reply controls and observe no automatic SQL/native
  retry. No new general recovery qualification is implied.
- P2 bad count/speed oracle: four read operations versus six whole invocation
  operations, archive counted once; compare combined work, not the vanished label.
  No predicted wall-clock saving is a test assertion.
- P2 scope/old-test conflict: stopped/first/standalone paths and exit 7 remain;
  old native counts/fakes may contradict the new boundary and need a separate
  post-approval audit/decision. No prior test deletion/weakening is authorised.

Repeat review found no remaining P0–P2 in this bounded list. The tests check
existing guards and this one new observation boundary, not all #170–#174 work.
Approval is not runtime qualification or a performance result.

### Post-approval audit: limited old-test changes approved

The audit found these concrete contradictions. On 2026-11-11 @evilguest
separately replied “Разрешить OT-CPR01–OT-CPR03” (exact time unavailable).
The following are the exact approved/implemented limits, not permission for
other existing-test changes:

- **OT-CPR01:** only `TestLiquibasePrivatePruneInvocationBinding/control` needs
  the new bounded combined-proof response instead of its unbounded idle response.
  Use a test-local runner adapter, not a change to the shared capture runner.
  Preserve all 12 variants, invalid-authority zero-call checks, one archive,
  exact authorization fields and six total Docker operations; separately count
  ordinary/bounded operations rather than reusing the old five-text-call count.
- **OT-CPR02:** only the successful `sessionCoordinatorRuntime` invocation
  fixture must obtain the opaque receipt by calling the real runtime through
  controlled transport, not synthesize it. In `TestLiquibaseSessionCoordinatorReusesRunner`
  and `TestMainPathCapturePruneDispatch`, replace the placeholder execution
  strings `digest`/`binding` by valid fixed SHA256 values so real runtime
  validation reaches the intended boundary. Keep create/start/apply/remove counts,
  durable-intent ordering, exact prior links, missing-capability and lost-response
  cases. Failed invocations keep their original injected error, not a manufactured
  success. No public/test-only receipt factory or broad fixture rewriting.
- **OT-CPR03:** only fresh-pass expected `capture.prune` count in
  `TestCostMainPathNamedAttributionDocker` changes from 2 to 0. Retain three
  `capture.read`/apply steps, every DB/state/journal/helper/observer/reuse/handoff/
  cleanup assertion and the unchanged cached controls. New CPR07/CPR08 directly
  qualify that the actual proof moved inside capture reading; a zero label alone
  cannot pass them. No historical report or standalone probe test changes.

Critical/repeat audit keeps `capturePruneRuntime`'s pre-SQL capability refusal
intact (including the existing missing-probe case), even though the normal path
will consume a receipt rather than call its standalone method. No new fallback
or automatic SQL/native retry is introduced. Other contradictions, if found,
must be reported separately; these proposals are not blanket authorisation.

### CPR implementation checkpoint (2026-11-11)

- New tests first exposed the missing receipt method, then coordinator acceptance
  of missing receipts/cancellation. The runtime now seals a value-bound private
  receipt only after archive/idle/absence/final-parent checks; prepare matches it
  without issuing the standalone probe. Cancellation during final inspection
  retains its original classification.
- Ordinary runtime/coordinator and real SQLite tests pass, including before-effect
  refusal, one committed lost reply, independent readback and exact third-slot
  reuse. CPR02's changed-authority inputs are internally consistent, so the test
  exercises retained receipt matching rather than an earlier argument mismatch.
- Per-line profiles show 100% statement coverage for the three new runtime
  receipt helpers and both prepare match/projection helpers. This is not a claim
  of 95% for the existing runtime/prepare packages; #89's #174 exception remains.
- All 83 Node tests pass after workflow selection changes. Windows/Linux vet
  passes for runtime/prepare/queue. The broad local parallel run had old HTTP
  timeouts and hit its 6-minute package budget in managedstore/queue; unchanged
  sequential reruns of engine command/managedstore/queue pass. Client rerun is
  pending; the failed broad attempt is not described as successful.
- CPR08 source tests and explicit Actions selectors are present, but not yet
  qualified natively. Review found the new known-failure test's counter combines
  `published_prune` and final `session_disposal` records. The user separately
  approved splitting those checks: exactly one `published_prune/path_absent`,
  plus complete final records, empty slots and actual absence of all containers.
- No new CT13 has run. The immutable original report, 18% fresh regression and
  full performance acceptance requirement remain unchanged.
