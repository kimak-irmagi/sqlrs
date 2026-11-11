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
