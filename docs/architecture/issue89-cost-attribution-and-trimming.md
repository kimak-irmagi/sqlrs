# #89: remove duplicate work and measure named operations

Status, 2026-11-10: @evilguest approved the direction and then explicitly approved
the reviewed component details below. Exact reply timestamp unavailable.
**CT01–CT14 are separately approved; implementation is starting tests-first.**
This replaces the unapproved proposal to refine strace command roles first.
No public CLI syntax, API, database schema or SQL-replay policy changes.

## 1. Source correspondence, not guessed Docker categories

References are released v1 `cab99c4` and measured v2 `668d505`; new results must
record their own exact source/binary/helper hashes. The report retains a static
operation-to-source catalog for both revisions, with stable operation and call-site
keys. Line numbers are presentation only, never the identity of a measurement.

| Action | Released v1 | v2 | Correspondence |
| --- | --- | --- | --- |
| Plan pending changes | `runLiquibaseUpdateSQL` | `planNormalContainerLiquibase` | Same purpose, different implementation/lifetime |
| Apply at most one change | `executeLiquibaseStep` / host runner | `executeNormalLiquibaseStep` → `invokeStep` / container helper | Same purpose, **not identical measured code** |
| Stop/status-check DB and relax permissions | `PostgresConnector.PrepareSnapshot` | Same connector | Same operations; preserve internal sub-operation keys |
| Copy/snapshot state | `StateFS.Snapshot` in `execution.go` | Same interface in `liquibase_normal_worker.go` | Same backend-dependent operation |
| Harden permissions and restart DB | `ResumeSnapshot` inside prior step | Same connector before next step/final publication | Same operation, shifted boundary |
| Verify managed DB / seal state | `verifyManagedRuntime` / `RecordSeal` | Same calls plus extra verification sites | Shared action; classify **each call site**, not all checks as new |
| Persist tasks/state and publish instance | Old executor/store/access | Normal worker/journal/access | Shared purpose, changed implementation; not all SQLite is additional |
| Resolve/probe/pin Liquibase image | No container binding | `bindFreshContainerLiquibase` / `InspectRunnerImage` | Additional v2 work |
| Private-output validation and retirement | No private-container protocol | `ReadCompleteLiquibaseStepOutput`, `VerifyLiquibaseCapturePruned` | Additional v2 work, with separate code keys |
| Session/readiness/attempt/slot journal | No equivalent protocol | Coordinator and `prepare/queue` | Additional v2 protocol, not permission to remove it |

Label relationships `shared`, `changed`, `shifted`, `additional`, or `unresolved`.
An unavailable v1 inner timing is **unmeasured**, not zero. Source correspondence
is independent of timings and does not imply identical DB/container counts.

## 2. Bounded first removal inventory

Remove duplicate mechanisms, not safety requirements. These are substitutions,
not proof that all surrounding functions are dead:

- **O1 — list before known-ID inspection.** Runtime-only positive observations in
  step invocation/output, readiness and prune verification already have the full
  saved ID and expected name. A private `inspectPresentContainer(ctx,name,id)`
  can inspect that ID once and validate full ID, exact name, phase and the same
  decoded labels. Reuse one JSON decoder. Keep before/after observations and
  cancellation. Failed inspect is an error, **never absence**; existing name-based
  discovery/adoption/deletion and absence methods remain unchanged. No container
  metadata cache across calls and no broad interface/psql change.
- **O2 — duplicate image ensure inside one fresh binding.** Fresh resolution
  already ensures the pinned digest before inventory; do not ensure it again
  after successful binding in the same call. Saved-binding entry still ensures
  its image once. No tag cache, skipped fresh resolution, cross-job inventory cache
  or implicit pull/retry after an ambiguous effect. External pruning may cause a
  later create to fail, as with any image-availability observation; never retarget.
- **O3 — three copies for one completed session capture.** Read the exact current
  invocation directory once with bounded `docker cp`, instead of three processes.
  A dedicated closed archive decoder accepts only the three complete regular files
  and the exact optional directory header. Preserve all manifest/ID/digest/EOF
  checks, per-file limits, aggregate cap `2*streamLimit+8192+archiveAllowance`,
  shared deadline, idle proof and pre/post parent observations. Reject extra,
  duplicate, missing, linked, sparse, partial or renamed entries and unsafe PAX.
  No extraction, arbitrary directory/root read, restart or writes. Return slices
  of one bounded buffer, not duplicate stream-sized buffers. Root planning output
  and standalone filesystem readers are unchanged in this first cut.
- **O4 — repeated same-filesystem flush within one command.** Both v2 helpers
  pass stdout and stderr in the same guarded capture directory to each of two
  `sync -f` calls. Replace each pair with its containing capture directory:
  six terminal filesystem sync operations become four, at the **same four phase
  boundaries**, without removing the final post-manifest-rename flush. Preserve
  file existence/type checks, completed bytes, ownership, return-code handling,
  manifest protocol and helper digest binding. First verify the pinned image's
  sync implementation in Actions; refuse the change for unsupported semantics.

[GNU sync semantics](https://www.gnu.org/s/coreutils/manual/html_node/sync-invocation.html)
and [coreutils 9.4 implementation](https://github.com/coreutils/coreutils/blob/v9.4/src/sync.c)
support O4's reasoning, not a claim about the runtime version or saved milliseconds.
Do not merge flush phases, replace syncfs with fsync, drop idle/identity checks,
remove COMMIT readback, cache safety decisions, or remove other code solely to
improve coverage. Newly found candidates need their own evidence/review.

## 3. In-process costs, default off

External tracing is no longer a prerequisite. Add an ordinary internal package
`internal/preparecost`, shared by prepare/runtime/DBMS through context, not global
mutable hooks. It owns a per-prepare recorder and fixed operation/call-site enums;
it grants no execution or cleanup authority and persists no DB state.

The proposed explicit internal measurement switch is `SQLRS_PREPARE_COST=1` at
engine startup, independent of the existing default debug log level. No new CLI
flag/config endpoint. Absent switch: nil/no-op recorder, no output or span storage.
Enabled: `PrepareService` starts a recorder for each worker, propagates it through
context to named boundaries and finishes on every exit path. A process-local
sequence correlates roots; ordinal identifies a logical step, with a separate
`startup/planning/publication/cleanup` phase for once-per-job work. Never emit job,
session/container IDs, DSNs, SQL, paths, arbitrary strings, arguments or errors.

Ordinary code owns the recorder, enum catalog, aggregation and bounded transport;
native orchestration remains an integration test. Each span has a recorder-local
ID/parent, fixed key, phase, step ordinal, relative monotonic start/end and closed
outcome. Recorders are isolated/synchronised, reject malformed/cross-root scopes,
finish once, and mark unfinished/dropped/overflowed evidence incomplete rather
than making up time or changing the prepare outcome. At most 32768 spans/root.

Buffer during execution. After finishing, emit allowlisted numeric/enum records
on private engine stderr with an anchored versioned prefix, begin/end frames,
declared count and completeness flag. End the measured execution root at its
last operation; serialize/flush afterwards and track that overhead separately.
Terminal job status does not wait for diagnostic output. After its unchanged API
timer ends, the collector separately waits for the matching end frame with a
bounded deadline. No logging per operation on the hot path. The harness drains
stderr continuously through a bounded line parser: discard all unrelated logs
without retaining/printing them, accept only the closed record grammar, and
upload only validated cost summaries. Cap accepted cost bytes at 8 MiB/engine,
cost-line length 4 KiB; unrelated long lines are drained/discarded in bounded
fragments, not accumulated or mistaken for broken cost records. Quota/read
failure makes measurement incomplete and triggers
the existing owned-process cleanup, not silent partial success. Late/missing
end frames cannot qualify a measured member. No raw log artifact.

Measure named host boundaries around resolution/inventory, projection/settings,
planning, runtime/session startup, each invocation, capture read/validation,
prune observation, DB verification/stop/restart, StateFS snapshot/size work,
step/session journal calls, publication and final cleanup. Child spans for actual
Docker transport retain their caller key; they are not a separate additive cost.
Do not infer inner JVM costs from the outer invocation: that combined interval
is labelled `changed`, and unobserved JVM internals stay explicit.

### Helper-local detail without more JVMs or a new binary

When measurement is explicitly enabled, the trusted v2 helper also measures its
own preflight/prune, child CLI, stream draining, stream flush, hash/rename,
renamed-stream flush, manifest preparation/flush and final rename/flush regions.
Use shell builtin `read` of `/proc/uptime`: boot-relative time, **10 ms resolution**,
not a new `date` process or calendar time. Off mode performs no timer reads or
output. [Linux 6.8 implementation](https://github.com/torvalds/linux/blob/v6.8/fs/proc/uptime.c)
documents the boot-time source and hundredth-second rendering. Verify availability
in the pinned image. Never present these measurements as nanosecond precision.
The engine sets the helper measurement switch explicitly from its recorder;
image/request environment cannot opt a child in or leak measurements into CLI output.

Emit only fixed helper-phase/numeric records on the helper's own Docker stdout,
never the captured Liquibase stdout/stderr; output is at most 4 KiB/invocation.
The runtime consumes/validates them privately and binds them to its current
invocation scope. No extra capture file, manifest/schema field or execution
authority. Timer/format failure does not change helper execution/exit or invent
zero costs: the host marks helper detail incomplete. Default-off helper bytes
are unchanged semantically, though their digest must be newly pinned.

Retain helper-local offsets/durations and resolution separately from host
monotonic spans: do not manufacture an absolute placement inside the Docker call.
Quantisation can make a short region zero; distinguish it from unavailable and
report uncertainty up to 10 ms per region (cumulative uncertainty separately).
The boot-time clock includes suspension; do not assert it has Go's clock semantics.
Incoherent/overflowed/nonmonotonic observations or helper durations incompatible
with the containing host call invalidate this detail, not prepare. Do not claim
to detect every possible clock-namespace change. Helper rows are nested, non-additive detail
of their outer invocation; they cannot also increase the host additive total.
This gives source-linked costs for helper-only work, including sync boundaries,
without claiming any knowledge of Java internals or removing durability phases.

## 4. Accounting and requested report

The collector's original API timer remains the denominator `PrepareElapsedMS`.
It includes admission/wait/response work outside the worker root. Export those
uncovered portions as unassigned, not fabricated SQLite cost. Report recorder
elapsed duration and observer overhead separately. No cross-process clock subtraction
is needed for operation durations; retain the original clock rules for old data.

For every operation/call site and logical step output:

- relation to v1 and exact source symbol/revision;
- actual call count, sum and distribution of call durations (including failures);
- inclusive time (contains children) and self time (subtract union of children);
- `ZTotalMS`: union of this operation's intervals over the prepare;
- `Zratio = 100 * ZTotalMS / PrepareElapsedMS`;
- overall call count and `MeanPerCallMS`, plus per-step values. Mean per call is
  not mean per step: show zero-call/cache-hit steps and once-per-job phases.

Category unions can overlap and are marked **non-additive**. An additive overview
uses self-time leaves with unassigned time; if concurrent branches overlap, put
their overlap into an explicit shared bucket instead of assigning it twice.
Validate containment, counts, nonnegative finite durations, complete framing and
root/step association. Do not sum percentages across mixed hierarchy levels.

Example output shape, **not measured values**: `step 3 | capture.read |
ReadCompleteLiquibaseStepOutput | additional | calls N | Z ms | prepare union
ZTotal ms | Zratio %`. A companion table maps the shared/changed/shifted v1 actions.

## 5. Re-run and acceptance

Freeze a new product commit and reproducible executable hash after approved
tests/implementation; never relabel `668d505` as the fixed candidate. Preserve
all old reports. Separate same-VM short Actions JHipster experiment, two reversed
repetitions of four routes: released v1, old v2 `668d505`, new v2 recorder off,
same new executable recorder on. Each member has fresh/cached passes (eight
members, 16 passes); warm-up/acquisition outside timing. No strace or laptop load.
Compare new off/on to quantify observer impact and old/new off to measure the
optimisation; never call instrumentation-on values unobserved production cost.
No precise per-row overhead correction is inferred from aggregate overhead.

All existing DB/cache/CLI/container-reuse/publication/cleanup oracles remain.
Missing cost evidence fails the measured report, not silently passing as zero.
If short ordinary results permit, repeat the unchanged full P01b v1/v2 paired,
noise-control and independent-confirmation protocol for all three public examples
against the new pinned candidate. It remains blocking; no performance waiver,
privately sourced izess input, JVM callbacks, #104 or #170–#174 scope expansion.

## 6. Critical and repeat review

Corrected before presentation: treating shared-purpose operations as identical
code; treating changed/shifted work or existing v1 access checks as wholly new;
assuming unknown residual is SQLite; removing absence semantics with known-ID
inspection; losing archive caps/type/manifest checks; weakening durability while
deduplicating flushes; enabling costs via default debug; uncontrolled log payloads;
adding nested/concurrent times twice; attributing startup to every step; changing
the candidate without recording new identity; omitting the observer-off control.

Repeat review found no remaining P0–P2 in this bounded design. No speed saving
is promised in advance. Components are now approved; the
[reviewed test inventory](issue89-cost-attribution-and-trimming-tests.md) is
separately approved. Named old-test contradictions still require their own approval.

## 7. Implementation checkpoint (2026-11-10, not acceptance)

- CT01–CT14 and limited OT-CT01 are approved. New inspect/archive tests first
  failed for absent implementations; updated old fixtures failed on the old
  list+inspect path before the consumer cutover. Fresh O2 tests reproduced
  `ensure, inventory, ensure` rather than the required single ensure.
- O1 now uses the saved full ID directly at invocation, output pre/post,
  readiness pre/post and prune pre/post. Discovery/absence remain unchanged.
  O3 is connected only for session output: one bounded directory archive, the
  same completion verifier, and unchanged idle/pre/post proofs. O2 fresh/saved/
  moved-tag/refused ensure/refused inventory/refused real SQLite binding tests
  pass; no inventory/tag cache or implicit retry was introduced.
- Focused connected runtime tests pass, including exact command/copy order,
  binary stream bytes/hashes, foreign/renamed/changed parent, idle refusal,
  incomplete archive and cancellation. Ordinary runtime full run has two
  failures in the unamended `TestPrivateStepRuntimeLiveAndStopped` fixture;
  limited OT-CT02 is requested separately, not silently applied.
- First line-level measurements: O1 inspect/shared decoder 100%; new archive
  reader 84/87 statements (96.55%; decoder function 95.2%). The three uncovered
  statements are USTAR prefix joining, legacy regular-header normalization,
  and the final standard-reader EOF refusal. They are not declared dead code.
  Package-wide selected-test coverage is not a full-runtime/feature verdict.
- Preliminary `preparecost` owns a closed initial source catalog, recorder,
  pure union/self/exclusive accounting and helper-local detail decoder. Exact
  numeric overlap and quota cases pass. It is **not** connected to the worker,
  helper scripts, private stderr transport, collector, Node or Actions yet.
  The recorder run exposes one allocation in its own reflect-based assertion;
  the helper decoder run exposes an incorrect negative overflow example.
  Narrow new-test corrections are awaiting approval; neither failed run is
  reported as a green coverage qualification.
- Focused prepare binding tests and `go vet ./internal/preparecost
  ./internal/runtime` pass. O4 remains gated on the pinned image's actual sync
  semantics in Actions. No native workload/benchmark ran on the laptop, no new
  candidate was frozen, and no performance improvement is claimed. The old
  diagnostic artifact SHA256 remains
  `3e9bfe734748626f2b0f274bcfbc74ef3c8ccac980ea89f81e5efd3eda72af7d`.

### Approved corrections and green ordinary checkpoint

The user approved OT-CT02 and both new-test corrections; they are applied without
other old-test changes. Full ordinary engine `go test ./... -count=1 -timeout=12m`
passes on Windows, including prepare, queue and managedstore. Native opt-ins were
checked to be off; this is not native Docker evidence. `go vet` of preparecost,
runtime and prepare passes. All 51 Node performance-helper/workflow tests pass.

New ordinary preparecost helpers measure 99.6% overall; the encoder, helper-detail
decoder and outcome-preserving Observe/ObserveCall boundaries are individually
100%. Real runtime read/inventory tests now verify the actual nested Docker/copy
spans, phase/step, counts and unchanged failure results. Docker command spans are
shared-purpose v1 counterparts, not a claim that every command is additional.
The v1 `DockerRuntime.run` and `hostLiquibaseRunner.Run` symbols were inspected
directly at the pinned released source `cab99c4c7ced5b18f4dc43a65b002d84d8fef02d`.

The new private emitter has closed begin/span/end frames, validates complete
evidence before output, and serializes concurrent roots. It is not connected to
the worker/collector yet; no full transport/attribution qualification is claimed.
The initial source catalog remains deliberately partial until further named
boundaries are tested and connected.

The separate `.github/workflows/liquibase-cost-probe.yml` runs only on the explicit
`performance:cost-probe` label of this repository's own PR, on Ubuntu 24.04 with
read-only permissions and a 20-minute bound. Its new reviewed Node checks pass.
It runs `TestCostSyncImplementationDocker` (actual GNU utility/version/hash and
same filesystem, before O4 changes) and the unchanged real reusable-runner test
(three JVM calls, incomplete manifest and surviving-child refusal). Only the
public image is acquired, no database/JDBC/strace/acceptance cohort is involved;
there is no raw-log artifact. Native exact-ID cleanup remains mandatory. A green
probe permits implementing O4; it does not qualify durability under power loss,
the four-flush failure matrix or the subsequent unchanged speed gate.

### Native prerequisite and O4 implementation checkpoint

Foundation commit `8ef7704b5b7eb25b7832f1b9d8fef0bb4e77efb0` is pushed to draft
PR #175. [Native prerequisite run 42854975079](https://github.com/kimak-irmagi/sqlrs/actions/runs/42854975079)
passes the real GNU check (0.41 s): image
`sha256:a93be6d6c4fb12016d4bce092b11d4ed25f2f3d6165431215c7f24bf02539c67`,
coreutils 9.4, sync binary SHA256
`5c395b95d52686ad295045ccb2272ec8858359df6bdc9655d3feffe650790969`.
The existing three-JVM/single-runner and surviving-child cases pass too. The run
as a whole is **not green**: its old incomplete-manifest error expectation
conflicts with earlier O3 archive rejection; separately proposed OT-CT04 must
preserve the negative refusal and independent parent/stream controls.

After this real prerequisite, the two embedded helpers now use one containing
directory argument at each of the first two terminal flush boundaries. All four
phase boundaries remain. New tests first failed on the old paired arguments;
the actual embedded-byte/order checks now pass. Readiness/prune flushes remain
unchanged; the content-addressed helper digest changes automatically with bytes,
not the helper protocol or database schema.

New `TestCostHelperFlushBoundariesDocker` is ready for the next Actions run:
both real helpers, positive control plus each of four independently injected
flush failures. Its root-private recording shim checks exact files/arguments/
order and delegates to real `/usr/bin/sync`; it is absent from speed samples.
Helper nonzero exits and unchanged complete stream hashes remain required. The
last failing flush is not incorrectly required to erase an already renamed
manifest. These ten native cases have **not run yet**; ordinary tests/vet and the
workflow checks do not replace them. The workflow runs the GNU prerequisite first.
Worker/collector/helper-timer wiring and the new before/after/off/on series remain
open. OT-CT03 for the existing collector is pending; it is not changed implicitly.

The user subsequently approved limited OT-CT03/OT-CT04. The named native
incomplete-manifest case now checks the O3 archive refusal and independently
confirms the unchanged live parent and both private streams first. Collector
orchestration is still unchanged while its new ordinary receiver is built first.
O4 runtime/preparecost tests and vet pass (preparecost 99.6%); all 51 Node checks
pass. The later full Windows run has a separate intermittent HTTP failure:
`TestPrepareJobsDeleteBlockedWithoutForce` reports repeated SQLITE_BUSY and a
status lookup failure. Its isolated retry passes (1.37 s), then the entire HTTP
package passes unchanged (35.435 s); no root-cause fix/waiver is claimed. All
other packages in that full run passed. Source CI `42854932663` passed every job
that actually ran; macOS-fast was cancelled with no steps after waiting for a
runner, so that run is not reported as a complete green regression qualification.

### Native O4 qualification and first worker/collector connection (2026-11-11)

[Run 42858997116](https://github.com/kimak-irmagi/sqlrs/actions/runs/42858997116)
at `b7cacc92cd812a6e985b629c11a3e182c77a388f` is green. Both actual helpers pass
their positive control and all four injected flush refusals (ten cases total,
2.69 s). The reusable-runner test passes all three scenarios (5.28 s), including
approved OT-CT04. GNU 9.4/image/sync hashes match the prerequisite above. Exact
owned cleanup passes. This qualifies O4's tested command/order/exit boundaries,
not power-loss behavior or a performance improvement.

Approved OT-CT03 is now connected: the caller-owned stderr pipe drains bounded
closed frames; the unchanged API timer stops before waiting up to ten seconds
for its exact root. The reader is closed/waited after process teardown and
before private-root deletion. Inherited selectors are stripped on every route;
only explicit new-v2, untraced collection enables recording. Final stream errors
or an extra root prevent sample publication. No raw log is retained or emitted.
Reader helpers cover 113/114 statements (99.12%); selection/qualification helpers
cover 20/20 (100%). All 53 Node checks and ordinary Windows collector tests/vet
pass; Linux collector cross-compilation/vet pass. Native collection is still open.

The actual constructor freezes the exact startup selector. A worker now owns
its root and emits after registration cleanup, retaining success/failure/cancel/
panic control-flow outcomes without another status query. New tests verify exact
selection, two roots, disabled behavior, capability refusal and original panic
identity/cleanup. Planning, session startup and step entry points propagate named
scopes; reached-journal refusal tests pass. Full ordinary prepare passes (29.265 s);
preparecost remains 99.6%. Detailed host/helper boundaries, observer overhead,
Node reporting and before/after/off/on Actions remain unfinished; no candidate
or new speed result is declared.

CI `42859166525` is not green: Windows-fast fails
`TestAccessUseDoesNotBlockOtherInstances` at its five-second independent-instance
barrier; every other executed job passes, including macOS. Three unchanged local
targeted repetitions pass (2.639 s total). This is not a demonstrated root-cause
fix, a relaxed deadline, or a complete regression qualification.
