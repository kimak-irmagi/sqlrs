# #89 O5: planning DB handoff — test inventory

Status: PG01–PG10 and limited OT-PG01/OT-PG02 approved by @evilguest on
2026-11-13 after critical/repeat review. Components and the
[storage schema](issue89-planning-db-handoff-schema.md) are approved. Tests-first
implementation has reached local journal/coordinator validation. New O5 files
meet the coverage minimum after CP2; Actions-only native acceptance remains open.

## Evidence and scope

Use real file-backed SQLite, enforced FK, actual queue/CAS writers and independent
readers. A refusal must reach its intended boundary, preserve the complete journal
and have a valid same-phase control. External observations must occur outside
transactions/eviction locks. Two reopens use new physical connections. No new
fault-injection API, automatic restart, replay, prototype migration or #170–#172
matrix is introduced. Native workloads run only in Actions.

## New tests

1. **PG01 — schema/install.** `TestPlanningDBHandoffSchema` (managedstore): fresh
   install and frozen released bridge produce session format 4 and unchanged
   other component versions. Independently check all 13 named columns, exact
   PK/unique/composite/deferred FK and live-owner index. Require real COMMIT FK
   refusal for an otherwise valid transfer without its future session; a same-
   transaction valid session commits. Check immutable/all-or-none/exclusive
   transitions and malformed time/digest directly. Unknown/partial/altered
   catalog and prototype format 3 fail before writes/probes. Preserve two reopens,
   the exact released fixture and psql controls; hash checks are not the oracle.
2. **PG02 — retained successful planning.** `TestCompleteRetainedPlanning`
   (queue): successful attempt 1, non-empty exact plan, private/v2 stopped planner,
   verified same live DB/operation and ordered exit < retention < output permit
   one atomic row/decision/three links/attempt/task completion. Clone remains
   active; no absence/quiescence fact or runtime deletion is invented. Vary exact
   job/clone/generation/execution/fingerprint/runtime/name/path/Source/identity,
   evidence scope/order/digest, plan bytes and missing evidence separately.
   Reject failed/live planner, attempt 2, zero changes, plan-only/cache explanation,
   recovery_required, existing working history, cancellation or selected failure.
   The existing absence-based completion control remains unchanged.
3. **PG03 — first-session transfer.** `TestPlanningDBFirstSessionTransfer`
   (queue): actual first uncached running task and binding with the original
   source permit one planned session/helper lease on the same clone/generation,
   DB ID/name/path/operation/Source. No clone insert or generation increment.
   A second transfer/session, cached-prefix mismatch, wrong task/input/execution,
   missing readiness authority for later SQL, disposal request, selected outcome
   and recovery_required cannot bypass admission. Direct SQL cannot adopt a
   planning clone without the exact matching transfer, even if source matches.
4. **PG04 — new-writer atomicity.** `TestPlanningDBHandoffCommit` (queue): only
   the three new metadata writers (retained completion, transfer, disposal
   completion), not all old queue methods. Per writer: refusal after its first
   effect, actual COMMIT BUSY from a peer reader, committed-but-lost reply and
   incomplete/unavailable settlement. Observe the original connection and a peer
   before Close, then reopen twice. Proved rollback preserves every row; known
   commit has exactly one complete effect; unknown settlement is not success.
   Full readback includes the schema's row/facts/links/plan/task/attempt/session/
   clone/operation tuple, not merely the ownership row. Explicit same-operation
   metadata reconciliation cannot create another session or SQL invocation.
5. **PG05 — private capture is separate.** `TestRetainedPlanningCaptureDisposal`
   (queue/prepare): after exact saved plan, retire the stopped planner and its
   private output/input under retained ownership without deleting/stopping the
   live DB or marking it absent/released. Wrong retained link, live child or
   uncertain result refuses cleanup. After transfer, the old planning reference
   cannot destroy the session DB, even after ordinary session retirement.
6. **PG06 — non-reuse/disposal.** `TestRetainedPlanningDBNonReuse` (queue/prepare):
   cached prefix/source mismatch and full cache save their exact request before
   physical effects, then verify full DB ID, retired operation and absent directory
   before atomic disposed fact/time + clone release. Accepted cancellation and
   selected failure use their own saved authorities, not invented user requests.
   Transfer versus disposal-request race has exactly one committed winner;
   late outcome selection permits only exact existing-owner disposal facts, not
   new retention/transfer/session/SQL. An uncertain request commit permits no
   destructive effect; a failed/unknown physical observation does not start a
   replacement DB. Zero-change, plan-only and cache-explain use the
   unchanged ordinary cleanup route and create no retained row.
7. **PG07 — protection and history.** `TestPlanningDBOwnershipProtection`
   (queue/prepare): retained and disposal-pending owners block terminal completion,
   job deletion, source eviction/deletion and unrelated operation retirement.
   Matching disposal is allowed but Source/runtime/path mutation is not. After
   transfer, enumerate one physical owner, not two. After normal publication and
   retirement, transferred history alone does not pin the instance/state forever:
   real deletion succeeds, then terminal job cascade succeeds independently of
   sibling order. Direct history/fact/link deletion with the parent job present
   remains forbidden; foreign rows/files are unchanged.
8. **PG08 — coordinator order/no recovery expansion.**
   `TestNormalPlanningDBHandoffOrder` (prepare): actual saved suffix selects reuse
   only for a matching first input, then known committed transfer, runner readiness
   and invocation admission precede SQL. Observer doubles check the complete IDs
   and order, zero second DB starts/clones and zero destructive fallback on
   uncertain transfer. Reopen retained, transferred and disposal-pending states
   twice and run the existing classification paths: no automatic new session,
   SQL or broadened first-session recovery permission. This is durable-owner
   verification, not a new crash-recovery executor.
9. **PG09 — genuine Docker/data/cleanup.** `TestPlanningDBHandoffDocker`
   (prepare, Actions): a real three-change prepare proves preview DB ID equals
   each working-step DB ID and final published ID, with unchanged operation
   Source/path/generation. Use actual private capture, JDBC, SQL rows and snapshot
   cache contents. Count physical DB allocations separately from three invocation
   references; prove no destroy/recreate between preview and SQL1. Verify planner/
   runner/output cleanup before fixture fallback. A full-cache second prepare
   still uses the correct separate publication route; a real cached-prefix case
   refuses adoption of the wrong cursor. Normal instance/state/job deletion works.
10. **PG10 — regressions/coverage/timing.** Full engine, client and observer
    regressions/vet on supported platforms, per-line coverage of new helpers
    (target 100%, minimum 95%; existing #174 aggregate exception unchanged).
    Native qualification in Actions, then unchanged eight-member CT13 after
    combined O5–O7. Require one actual startup for fresh planning+working, two
    for full-cache planning+publication, retaining all step/snapshot/data checks.
    Original API timer, source-linked additive sum and speed gate are unchanged.

## Limited old-test changes, separately requested

- **OT-PG01:** only current target expectations in
  `main_path_installation_test.go` and `TestWorkingInstanceHandoffSchema`:
  session version 3→4, other markers unchanged. In
  `TestMainPathInstallationArtifact`, update only the three current SQL/catalog/
  target hashes and corresponding current amendment definitions/provenance.
  Do not rewrite the frozen released SQL/catalog hashes or extraction source
  history. New PG01 independently checks format 3 refusal and the new guards.
- **OT-PG02:** only `TestCompleteTimelineMainPathDocker` and
  `qualifyCompleteCostBoundaries` / `complete-cost-boundaries.test.mjs`: fresh
  planning+startup DB operation counts 2→1, cached planning+publication stays 2;
  fresh `plan.dispose` becomes 0 and cached remains 1. `plan.release` stays 1
  in both passes because real private planner cleanup remains. Add `plan.retain`
  around retained completion (1 in each pass) and `plan.db_transfer` around the
  actual first-session transfer (fresh=1/cached=0). Preserve
  negative missing-boundary tests and native data/cleanup/timer assertions.
  Add these two exact new keys to the existing closed source catalog/receiver;
  do not simulate removed calls with empty spans.

No change to `runners`/`databases` length=3 is requested: these arrays contain
three invocation references, not three DB allocations. Do not alter ordinary
absence-proof, recovery, failure, cancellation or history tests. Any further
contradiction requires a separate narrow proposal before editing.

## Critical and repeat review

Corrected: distinguish actual allocation from per-invocation ID observations;
exercise the deferred FK at COMMIT rather than accept an earlier guard error;
preserve both mismatched-prefix and full-cache branches; check private cleanup
without fake DB absence; block old-reference destruction after transfer; retain
source/operation protection but allow ordinary published-instance deletion.
New transaction failures are limited to the three new writers, not a cross-product
of all former failure protocols. Reopens explicitly do not authorise execution.

Repeat review traced each schema transition and fact to PG01–PG08, checked native
data and physical allocation evidence in PG09, exact cold/cache timing boundaries
in PG10, and separated all old corrections. No remaining P0–P2 finding in this
inventory. Implementation and approval are not claimed by this review.

## Resumed implementation audit, 2026-11-13

Verified `taidon-issue89` on `codex/issue89-main-flow`; existing changes are
preserved. This reviews written tests, not the previously approved inventory.
At the initial audit, production O5 DDL and writers were absent. See the later
implementation checkpoint below.

| Area | Resumed finding | Added during audit |
| --- | --- | --- |
| PG01 | Named fields/FK, index, version, prototype refusal and two reopens; uniqueness and composite grouping were missing. | Independent types/nullability/unique keys/composite FK actions; damage to the new index/trigger/namespace rejected before inventory/writes. |
| PG02 | Success and some refusals; incorrect `DecisionSaved=true` expectation. | Real live planner with positive control after exit; independent legacy completion control. |
| PG03 | Exact transfer, changed arguments, direct INSERT, deferred FK, readiness and second transfer. | Persisted recovery/cancel/failure/task-status refusals; both transfer/disposal-request winner serializations using an independent connection. |
| PG04 | Real BUSY/lost reply/first-effect failure/partial and unavailable settlement; equal journals alone did not prove a complete effect. | Independent ownership/plan/task/attempt/facts/links/session/helper/clone/operation oracle after actual COMMIT. |
| PG05 | Successful private cleanup over a live DB; obsolete-reference refusal before session release. | Foreign-link and physical-observation refusals with same-cleanup control; native obsolete-reference assertions after normal release and instance deletion. |
| PG06 | Metadata cleanup covered only cancellation; native test already had full/prefix cache paths. | Actual three-checkpoint cache producer; full_cache/source_mismatch metadata paths; selected_failure before retention/transfer and after request; uncertain request COMMIT grants no cleanup authority. |
| PG07 | Live retained-owner protection; instance/state/job removal in the native test. | Invalid direct ownership updates in all four states and a no-op control. |
| PG08–PG09 | Both use `newMainPathNativeFixture`, requiring Actions/Docker. | Local execution remains prohibited. Local `ok` with SKIP proves neither coordinator ordering nor native acceptance. |
| PG10/OT-PG | Complete acceptance and bounded old O5 expectations remain outstanding. | Permitted local regressions repeated; original timers and speed gate unchanged. |

At that initial checkpoint the suite was not yet ready for implementation. Outstanding
independent controls include a real second attempt and prior working history
(PG02), evidence completeness/scope/digests and changed plan bytes, full source/
eviction protection (PG07), and local coordinator doubles (PG08). Negative
fixtures behind the absent writers have not reached their final boundaries;
their failures cannot yet be classified as product failures or qualified tests.
OT-PG01/OT-PG02 remain approved but were not applied during this audit.

Approved bounded fixture correction: `PlanningRecoverySnapshot.DecisionSaved`
comes from `prepare_planning_intents.decision_saved_at` for `cache_explain`,
not from `prepare_attempt_decisions.choice='complete_planning'`.
`TestPlanningDBLegacyCompletionSnapshotControl` passes using existing writers
and proves complete_planning exists with `DecisionSaved=false`. @evilguest
approved requiring `DecisionSaved=false` in the new `TestCompleteRetainedPlanning` and
the added PG04 oracle, preserving independent decision and three-link checks.
The correction is applied; product semantics and old tests remain unchanged.
Approval is recorded in `2026-11-13-planning-db-handoff-test-approval.md`.

Initial audit validation:

- `TestPlanningDBFixture`, `TestPlanningDBCacheFixture` and
  `TestPlanningDBLegacyCompletionSnapshotControl` pass. The new cache producer's
  missing published-prune confirmation before the third invocation was fixed;
  existing tests/product were unchanged.
- A separate run of existing managedstore/queue/prepare tests excluding new red
  O5 tests passes. Full runtime/preparecost and 106 Node tests pass; vet passes
  for managedstore/queue/prepare.
- The selected complete O5 suite builds and remains expectedly red: current
  version 3, missing ownership table and new writers. New fixtures beyond that
  boundary remain unqualified. Native tests use their normal opt-in skip.
- No Docker, workload, CT13 or implementation coverage measurement was run;
  neither speed acceptance nor O5 completion is claimed.

## Implementation checkpoint, 2026-11-13

The initial audit gaps above were addressed before the corresponding writers:
real second-attempt and working-history refusals, hostile evidence scope/digest,
saved-plan immutability, state-source deletion with real sentinel bytes, eviction
read errors, and local transfer-refusal/indeterminate coordinator controls. The
new metadata publication control executes all three actual apply publications,
DB-only final resume, access activation, instance publication and private session
retirement on the original planner clone/path/operation. It preserves the native
PG09 requirement for physical startup counts and actual PostgreSQL data.

Implemented: session format 4 and canonical ownership guards; retention, atomic
completion/transfer, ordered request and proved disposal; separate private
capture cleanup; source protection before file deletion/eviction; coordinator
reuse with no new allocation or generation. Final resume/publication permit
the closed planning attempt only through the exact transferred owner. Publication
accepts the original `jobs/<job>-plan/<operation>` path only with that receipt,
its retained fact and exact operation/session/instance identity.

OT-PG01/OT-PG02 are applied within their approved limits; released sources and
historical hashes remain unchanged. The separately approved IC07 correction adds
only `plan.retain`; all 106 Node tests pass. Observer unit tests pass with
GOWORK=off; vet and Linux prepare test cross-compilation pass. No Docker or
workload was executed locally. Initial coverage is below the required 95%;
coverage qualification and hosted native/speed acceptance remain open.

## Coverage iteration CP-PG (approved 2026-11-13)

First per-line measurement (Go statements, 2026-11-13):

| New file | Coverage | Uncovered / total |
| --- | ---: | ---: |
| `prepare/planning_db_handoff.go` | 2.08% | 94 / 96 |
| `queue/planning_db_handoff_journal.go` | 82.43% | 26 / 148 |
| `queue/planning_db_disposal_journal.go` | 78.76% | 24 / 113 |
| `queue/planning_db_transfer_journal.go` | 80.00% | 22 / 110 |
| `store/sqlite/prepare_protection.go` | 75.00% | 2 / 8 |
| `store/prepare_protection.go` | 66.67% | 1 / 3 |

Exact zero-hit source blocks and source text: local
`backend/local-engine-go/coverage/issue89-o5-line-report.{json,md}`.
Profiles: full prepare and O5 queue instrumenting actual store/deletion calls.
These are new-file figures; most changed functions are also below 95%.


Root AGENTS.md §6 requires approval of the coverage-increase plan. This is not
reapproval of PG01–PG10 or OT-PG01/OT-PG02. The target remains 100%, minimum 95%
for changed helpers under the existing #89/#174 aggregate exception.

1. **CP-PG01 — coordinator, PG02/05/06/08.** Add local runtime/journal doubles
   and real temporary directories for exact live observation, authenticated
   verification success/refusal, changed IDs/binding/path, symlink/missing path,
   capture read/parse errors, absent job, retention refusal and output/save/commit
   refusal. Check the ordered successful capture/retention/completion/private
   cleanup path and the existing empty/plan-only/explanation/failed-planner path.
   A failed or uncertain step permits neither SQL nor a new startup.
2. **CP-PG02 — disposal/cancellation, PG05/06/08.** Through the actual coordinator
   methods, require known request commit before cleanup; request refusal/unknown
   must make zero destructive calls. Exercise exact cleanup failure, DB absence
   failure, remaining directory, completion failure and accepted cancellation.
   Context cancellation alone must not select destructive authority.
3. **CP-PG03 — journal, PG02/03/04/06.** Cover unreadable or inconsistent
   retained-owner/plan/fact/operation/suffix data and complete-snapshot query/row
   failures through existing test driver wrappers. Reconciliation failure must
   return no authority and preserve unrelated rows. Add direct-SQL request
   refusals for cached flags without real state/execution provenance and for a
   mismatched first task after an unproved prefix; strengthen the existing
   reason guard if these controls expose a bypass. Keep positive controls using
   actual cached publications. No restart/replay fault matrix is added.
4. **CP-PG04 — publication, PG07/09.** Exercise the narrow original-planner-path
   exception against a missing/foreign transfer, changed operation/Source and
   mismatched session/instance. Require the exact valid publication control;
   the old job-directory path and legacy working handoff tests stay unchanged.
5. **CP-PG05 — source reader, PG07.** Instrument actual cross-package calls so
   coverage includes the real deletion tests. Cover standalone absent namespace,
   installed active/released clones and unreadable owner metadata before file
   effects. Review remaining zero-hit branches against these requirements;
   remove only demonstrably unreachable error/dead branches, never guards.

Work follows decreasing uncovered-statement count, beginning with the coordinator.
Record exact uncovered source blocks in the local `coverage/issue89-o5-line-report`
JSON/Markdown artifacts. Remeasure per file/helper and rerun relevant regressions.
If the threshold remains unmet, seek the next iteration approval as §6 requires.
Do not change old tests beyond existing approvals. Docker/workloads/CT13 stay
Actions-only; hosted acceptance and roadmap completion are not claimed here.

Latest regressions: all internal engine packages passed in the broad run;
the sole cmd/sqlrs-engine timer failure under parallel checks did not recur
in an isolated full package run (PASS). Full queue passed (270 s), as did
managedstore; one duplicate concurrent queue run hit its 5-minute timeout and
is not a PASS. The final instrumented O5 suite passed in 63 s. No test was
weakened.

## CP-PG01–CP-PG05 result and proposed CP2

The approved first coverage iteration is implemented. Full queue (228 s),
prepare (92 s), managedstore (71 s), store/sqlite (33 s), vet and Linux prepare
test cross-compilation pass. The source-reader cross-package control also passes.
No old test was weakened. Target hashes were mechanically refreshed only within
OT-PG01 after the new request guard was strengthened; historical hashes remain
frozen. Docker, native PostgreSQL and CT13 were not run locally.

Two reproduced defects were corrected: retained-owner reads now recheck the
operation Source/runtime ID, and direct SQL disposal requests now require real
cached-state/execution provenance, exact prefix order and saved authority. The
actual cache publication controls still pass. Coordinator statement coverage is
100%; that includes the authenticated-verification refusal path, not proof of
real successful SCRAM, which remains PG09/Actions.

| Changed O5 file | Statements covered | Remaining zero-hit statements |
| --- | ---: | ---: |
| `queue/planning_db_handoff_journal.go` | 91.22% | 13 / 148 |
| `queue/planning_db_disposal_journal.go` | 89.38% | 12 / 113 |
| `queue/planning_db_transfer_journal.go` | 94.55% | 6 / 110 |
| `prepare/planning_db_handoff.go` | 100% | 0 / 96 |
| `store/sqlite/prepare_protection.go` | 100% | 0 / 8 |
| `store/prepare_protection.go` | 100% | 0 / 3 |

Exact source blocks: local `coverage/issue89-cp-line-report.{json,md}`; generator
`coverage/issue89-cp-report.py`. Cross-package instrumentation is essential: the
SQLite tests alone do not instrument the wrapper in package store. The three
journal files remain below 95%; completion/roadmap acceptance is not claimed.

**Second iteration CP2 (approved 2026-11-13), in decreasing uncovered
statement count:**

1. **CP2-H — retention/completion, PG02/04.** Reach the exact saved-attempt
   comparison with an independent observer accepting the physical tuple; change
   the observed attempt phase between the first read and transactional recheck;
   reject inconsistent idempotent completion and an unproved completion phase.
   Exercise unreadable/cancelled full-snapshot rows and non-JSON-representable
   values as failures, never as a successful COMMIT. Preserve the existing
   three-writer BUSY/lost-reply matrix; do not expand it to every old writer.
2. **CP2-D — disposal authority/recheck, PG06/07.** Use real published cached
   states for noncached/incomplete-prefix/wrong-first-binding refusals, plus
   request owner/fact read failures, a changed complete tuple between absence
   observation and the second read, stale clone phase, and conflicting repeated
   disposal proof/time. Require whole-journal equality on refusal and exact
   same-phase controls. No restart/replay recovery matrix is added.
3. **CP2-T — state source and readback, PG03/07.** Add a real published-state
   planning source and positive first-session transfer preserving exact Source,
   DB/clone/generation/path/operation. Existing source-protection tests use a real
   state but do not yet exercise this positive transfer branch. Preserve image
   fixture defaults; add a source-aware launch fixture. Reject unreadable or
   malformed retained facts, unavailable saved suffix and changed idempotent
   session readback without allocating/retrying.
4. **CP2-R — dead branch audit, PG04.** Review the two separate Close-error
   branches after fully draining Rows. With the current SQLite driver, Next
   closes Rows on EOF; database/sql stores Close errors in Rows.Err, already
   covered by the new close-failure control. Remove only the duplicated checks
   if this trace holds for all supported paths; retain real close/row errors,
   context cancellation and all authority checks. Other uncovered defensive
   branches remain unless similarly proved unreachable or exercised.

Remeasure the changed helpers and rerun affected regressions after CP2. If the
threshold remains unmet, seek another iteration approval under AGENTS.md §6.
The target/old-test limits and Actions-only native acceptance stay unchanged.

## CP2 result, 2026-11-13

CP2-H/D/T/R passes. The full queue run passes (182 s); the corrected CP2
selection passes (19 s), including real published-state Source transfer, exact
operation/attempt rechecks, refused/changed cache authority and repeated disposal
proofs. Two duplicate Close-error branches were removed only after tracing
database/sql.Next/Close and the installed SQLite driver; both real close-error
controls pass before and after the change. Row/context errors still refuse.

The first post-CP2 report revealed a test-adapter preparation bug: index-only row
overrides returned an earlier query error. The adapter now actually applies them
and counts changed cells; all three phase cases require one changed cell. The
corrected selection reaches those guards. This corrects new test preparation,
not product requirements or old tests.

| New O5 file | Final coverage | Zero-hit statements |
| --- | ---: | ---: |
| `queue/planning_db_handoff_journal.go` | 96.60% | 5 / 147 |
| `queue/planning_db_disposal_journal.go` | 100% | 0 / 113 |
| `queue/planning_db_transfer_journal.go` | 100% | 0 / 109 |
| `prepare/planning_db_handoff.go` | 100% | 0 / 96 |
| `store/sqlite/prepare_protection.go` | 100% | 0 / 8 |
| `store/prepare_protection.go` | 100% | 0 / 3 |

Combined new-file statement coverage is **471/476 (98.95%)**, meeting the 95%
minimum; 100% remains the target. The five zero-hit defensive statements are
retention-evidence COMMIT failure and the Columns/Scan errors while assembling
the complete snapshot. They remain guards, not discarded requirements.
This is not whole-engine aggregate coverage or a claim that every function is
100%; the #89/#174 exception remains unchanged. Exact blocks are in local
`coverage/issue89-cp2-line-report.{json,md}`, generated by
`coverage/issue89-cp2-report.py` from the full queue plus corrected CP2 selection
and unchanged-source full prepare/store/source-reader profiles.

The roadmap now records implemented O5–O7 with local coverage evidence. Actual
native allocation/identity/data/cleanup qualification and the complete combined
CT13 series still belong in Actions. Original timing/speed criteria are retained;
no new hosted speed pass, completed #89 or merge readiness is claimed.

## Persisted purpose qualification (2026-11-14)

Hosted native run 43467869209 passed runtime and normal-prepare checks, then
failed TL13 because `plan.retain` was absent. O5 had read `Purpose` from the
base `GetJob` projection, which deliberately omits component-only fields.
The coordinator now uses the existing admitted `LoadContainerJobView` reader;
the base queue reader and schema remain unchanged.

`TestPlanningCoordinatorPersistedPurpose` supplements PG02/PG08 and CP-PG01
with real SQLite admission and purpose reads at the coordinator boundary.
Before the fix, its prepare case selected absence-based cleanup and failed.
After the fix, all three cases and the full prepare package pass, as does vet.
The controls preserve plan-only and cache-explain cleanup. Existing ordered
failure assertions and TL13's required retention span are unchanged. Hosted
native qualification and CT13 must be repeated on the corrected source.

The next hosted run, 43564979105 on `550110f`, passed successful prepare and
same-exec checks but exposed the known-failure branch after transfer. Failure
selection still required an untransferred preview, and its session inventory
mistook the closed planner sharing the clone for an unassociated session child.
PG06/PG08 now include `TestPlanningDBSelectedFailureAfterTransfer`: real transfer,
known nonzero child completion, failure selection/readback, exact owner inventory,
stopped-session disposal intent and refusal of obsolete planner cleanup. The test
failed independently at both incorrect guards before their fixes.

The Go selection check and its SQL admission trigger now accept the exact durable
first-session transfer, checking clone, Source, generation, execution fingerprint,
first task/binding and absence of planner disposal. Complete owner comparisons
include the O5 row; only its exact closed planner is excluded from session-child
disposal. This preserves PG06's known-failure cleanup requirement without granting
SQL replay or implementing deferred recovery. The 299-object current catalog and
its three target hashes change; frozen released/source provenance is unchanged.

[Native qualification 43566256871](https://github.com/kimak-irmagi/sqlrs/actions/runs/43566256871)
passes on `28cfc25`: TL13, actual PG09 fresh/full/prefix database allocation and
identity checks, PG08 ordering/uncertain-transfer/reopen controls, and the
unchanged known-second-step-failure cleanup. Runtime envelope/prune and normal
prepare controls also pass. The final full local queue run passes (168.096 s),
as do the managedstore suite and vet. Current source CI 43566258042 passes all
15 test/documentation jobs; only aggregate coverage fails (engine 79.4%, CLI
94.8%) under the unchanged #89/#174 exception. These are functional results,
not performance acceptance.
