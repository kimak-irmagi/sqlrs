# #89: source-linked JHipster costs after O1–O4

## Result and immutable evidence

[Actions 42967877614](https://github.com/kimak-irmagi/sqlrs/actions/runs/42967877614)
completed eight serial members / 16 fresh and cached passes on one Ubuntu VM.
Every database/cache oracle, counter and cleanup check passed. The pinned
JHipster 5.0.3 image prerequisite ran successfully. This completes the short
CT13 acquisition, **not the unchanged full P01b acceptance or #89**.

- Old v2 revision: `668d5050ad154c1967bea4c6e40b91f3a703d5ba`;
  executable SHA256 `1367bce9ddaa44c0aa378b07c015dfb20a4c0d29f00d2f5a893905686fae14b0`.
- New v2 / observer revision: `48e6e116561f8079e5669205944757ef117368da`;
  executable SHA256 `c09c087573a3bbc9c58f013913641d2e082d23f3a119d426d2ea31be45edbaeb`.
- Actual released v1 revision: `cab99c4c7ced5b18f4dc43a65b002d84d8fef02d`;
  executable SHA256 `b34c358d2d1e6e969f17d4e393641dd7cb53167870b8e4674db1381bf410bbca`.
- Collector SHA256: `a93a7bde930acc9250b186291fea8ddf219f220d5fa50b7a19312a811fef32c8`.
- [Original sanitized JSON](issue89-cost-evidence-2026-11-11/42967877614/jhipster-cost.json),
  SHA256 `5ae475ef29bf4663031fb9c678396bcf9e968dd7f9e2042e713d0bd082acdde1`.

Order: v1, old v2, new v2 off, new v2 on, new v2 on, new v2 off, old v2, v1.
Both new modes used the same executable and helper digest. No strace was used.
Only the two explicitly enabled members produced source-linked costs. All
figures below are descriptive means of two observations, not confidence bounds.
Original earlier full and diagnostic evidence is not replaced or relabelled.

## End-to-end prepare, seconds

| Variant | Fresh | Fully cached |
| --- | ---: | ---: |
| Actual released v1 | 48.263 | 8.556 |
| Old v2, before O1–O4 | 57.952 | 10.240 |
| New v2, measurement off | 56.952 | 10.337 |
| Same new v2, measurement on | 56.718 | 10.185 |

New ordinary v2 is 1.000 s / 1.73% faster than old v2 fresh, but 0.097 s
slower cached in this small series. It remains 8.688 s / 18.00% slower than v1
fresh and 1.782 s / 20.83% slower cached. The no-regression criterion is not met.
There is no reason to launch the expensive full acceptance suite as a substitute
for fixing an already reproduced short-workload regression.

On-minus-off means are −0.233 s fresh / −0.153 s cached. These do not mean
negative recording overhead or prove zero overhead: two observations and the
chosen ordering cannot separate it from variation. Encoding/writer timings in
the enabled samples are separate post-API evidence, not an end-to-end correction.

## Which work corresponds to v1?

The report's static catalog links each operation to exact source symbols in
the measured candidate and, where applicable, the released v1 source. “Shared”
means corresponding purpose, not identical code, command count or duration.
The released binary is not internally instrumented by this campaign.

| Work | Candidate source / symbol | Relationship to actual v1 |
| --- | --- | --- |
| Preview SQL / changelog plan | `internal/prepare/tracked_executor.go`, `runTrackedExecutor` | `components.go`, `PrepareService.runLiquibaseUpdateSQL`; now container/capture based |
| Apply at most one change | `internal/runtime/liquibase_step_capture.go`, `InvokeTrackedLiquibaseStep` | `execution.go`, `taskExecutor.executeLiquibaseStep`; CLI inside a reused container instead of on the host |
| Stop PostgreSQL for snapshot | `internal/prepare/liquibase_normal_worker.go`, `observeExecuteNormalLiquibaseStep` → `PrepareSnapshot` | Same connector operation in `executeStateTask` |
| Resume PostgreSQL | Same worker → `ResumeSnapshot` | Same connector; seven resumes move to the following step, the eighth to publication |
| Verify applied DB, seal, snapshot, size | Same worker → `verifyManagedRuntime`, `RecordSeal`, `Snapshot`, `storeUsageFn` | Corresponding verification/seal/snapshot/accounting in `executeStateTask` |
| Publish state / final instance | Same worker → `PublishLiquibaseSessionState`, `publishNormalLiquibaseWorkingInstance` | Corresponding publication / `createInstance`, with different ownership protocol |

Across eight steps the seven `db.resume.step` calls cost 15.571 s, and
`db.resume.publication` costs 2.225 s. Their 17.796 s total is **not 17.796 s
of additional work**: v1 also resumes the DB eight times. The connector source
has no diff from that release. The final DB is handed off, not recreated.
Snapshot creation totals 0.578 s; stopping for snapshots totals 2.140 s.

Counters independently show nine migration CLI starts fresh (planning + eight
applications) and one cached in every variant. v2 additionally runs its image
inventory once: ten/two total Liquibase CLI starts versus v1's nine/one.
Fresh container creations are eight in either v2 versus four in v1, cached
four versus two. These are actual whole-workload counts, not a claim that a new
working Liquibase or DB container is created for every step.

## Additional v2 operations: X / Z / ZTotal / Zratio

Times and ratios below use the enabled fresh prepare denominator, about
56.718 s. Rows are means of separately validated samples. The five action rows are
disjoint in the current sequential path; their child `docker.command` / copy
rows must not be added again.

| Additional action | Calls | Mean per call, ms | ZTotal, ms | Zratio |
| --- | ---: | ---: | ---: | ---: |
| Read and validate completed private capture (`capture.read`) | 8 | 108.065 | 864.522 | 1.524% |
| Separately verify prior output was pruned (`capture.prune`) | 7 | 213.829 | 1496.805 | 2.639% |
| Plan / admit / complete invocation in journal | 8 each | 8.155 per trio | 65.243 | 0.115% |
| Verify DB again after resume | 7 | 24.512 | 171.587 | 0.303% |
| Inspect immutable runner image / Java / driver inventory | 1 | 1001.290 | 1001.290 | 1.765% |
| Total of the above, without nested rows | — | — | 3599.448 | 6.346% |

Code locations:

- Read: `internal/runtime/liquibase_step_capture.go`,
  `readObservedLiquibaseStepOutput`: exact pre/post inspect, one bounded archive,
  unchanged manifest/hash verification, and idle probe. The archive copy is
  already inside this row: 8 calls, 139.599 ms total, not another 139.599 ms.
- Prune: `internal/runtime/liquibase_prune.go`,
  `verifyObservedLiquibaseCapturePruned`: pre/post exact inspect and three execs
  (idle / read-only prune proof / idle), five Docker round trips per later step.
- Journal trio: `internal/prepare/liquibase_session_coordinator.go`, `invokeStep`:
  `PlanLiquibaseSessionInvocation`, `RequestLiquibaseSessionExec`,
  `CompleteLiquibaseSessionInvocation`. The trio is about 8 ms, not the main
  source of the observed multi-second regression.
- After-resume verification: `liquibase_normal_worker.go`,
  `observeExecuteNormalLiquibaseStep` → `verifyManagedRuntime`.
- Inventory: `internal/runtime/runner_image_inspect.go`,
  `inspectObservedRunnerImage`: separate offline container runs Java version,
  Liquibase version and PostgreSQL JAR hashes. It is repeated per fresh binding
  entry/job, not on every step.

Actual per-step additional cost, averaging the two enabled fresh samples:

| Step | Read, ms | Prune, ms | Journal trio, ms | After-resume verification, ms | Total, ms |
| --- | ---: | ---: | ---: | ---: | ---: |
| 1 | 108.117 | 0 | 7.615 | 0 | 115.733 |
| 2 | 105.688 | 211.874 | 8.313 | 24.368 | 350.243 |
| 3 | 109.017 | 213.321 | 7.813 | 24.822 | 354.972 |
| 4 | 109.305 | 213.997 | 8.335 | 24.697 | 356.335 |
| 5 | 107.916 | 212.971 | 7.957 | 24.027 | 352.872 |
| 6 | 106.483 | 210.679 | 8.374 | 24.370 | 349.905 |
| 7 | 110.421 | 220.307 | 8.442 | 24.640 | 363.811 |
| 8 | 107.575 | 213.656 | 8.394 | 24.662 | 354.287 |

## Helper internals and accounting limits

Across eight applications, the helper's CLI region totals 19.700 / 19.820 s;
its seven non-CLI regions total 0.490 / 0.460 s. These are container-local
10 ms readings, with up to 80 ms cumulative uncertainty per helper; a zero
phase is below resolution, not proof of no cost. This split is useful for
rejecting a multi-second-per-step flush hypothesis, not for microsecond claims.
It does not isolate the JVM from Liquibase's own launcher or changelog work.

Helper intervals are nested in host invocation costs and not placed on the
host timeline. They are not added to host totals. The host invocation also
includes capture reading; the helper's `HostElapsedNS` is the narrower command.

Fresh report encoding costs 2.308 / 1.158 ms; cached encoding costs
0.091 / 0.097 ms. Actual lock-wait and first-write durations are retained in
the JSON. Snapshot copying and observer-marker construction/write are explicitly
unmeasured. These values are not the total recording overhead.

Host parents still contain substantial work without finer spans. For example,
fresh planning is 7.366 / 7.207 s, with 4.256 / 4.167 s of parent self time;
startup is 2.850 / 2.843 s with 2.665 / 2.657 s self. The planning source includes
preview-DB lifecycle, reconciliation and cleanup, and startup includes working
DB and runner creation. Parent self time does **not** prove SQLite or Docker
alone caused it. Fully cached planning is 6.010 / 5.994 s and publication is
2.728 / 2.727 s, also not directly comparable to individual v1 phase labels.

The tiny outside-span remainder (fresh 50.672 / 114.599 ms) only means the
top-level worker encloses almost all API time. It does not mean all inner cost
is causally explained. Likewise, the additional 3.599 s above is measured v2
work, not a complete decomposition of the 8.688 s cross-version regression.

## Review and next bounded optimisation candidates

Review rejected these misleading conclusions: “Docker alone costs 83.7% on top
of everything”, “the final DB resume is extra”, “negative observer overhead”,
“the journal causes the regression”, and “all unassigned cost is SQLite”.
Parent/child double counting, image identity, explicit recording mode, all 16
oracles/cleanup and the immutable file checksum were checked again. No new
P0–P2 issue in this report was found on repeat review.

The next design should target the repeated capture/prune round trips first:
carry the existing exact prior-disposal proof in the same bounded observation
as completion, without dropping idle/identity/publication/absence checks. The
[component amendment](issue89-cost-attribution-and-trimming.md#6-combined-completionprune-observation)
retains a real read-only absence probe, not an inference from the manifest.
The current separate prune branch alone measures about 1.497 s; this is an
optimisation target, **not a promised saving or a complete fix**. A second
candidate is reusing successful image inventory by immutable image identity
and probe revision, never by a mutable tag and never caching failures. It
could help repeated jobs, not eliminate cold inventory automatically.

Neither proposed protocol/cache is implemented or approved by this report.
Their component/test approvals are required before code changes. There is no
new general recovery programme or broad SQLite refactoring proposed here.
Remaining large planning/startup/changed-CLI costs must stay visible while
testing any bounded fix; full P01b remains blocking after short regressions
are resolved.

Subsequently @evilguest approved the combined native observation components on
2026-11-11. CPR01–CPR09 and the narrowly audited old-test changes were later
approved separately. Inventory caching is not part of those approvals; measured
evidence above remains unchanged.

Separately, current source CI `42967847154` attempt 1 failed the old
`TestPrepareJobsDeleteBlockedWithoutForce` on Windows with `SQLITE_BUSY`.
An unchanged local repetition passed 20 times; attempt 2's unchanged failed CI
job also passes. Only aggregate `coverage-merge` is now red under the approved
exception. This is not a demonstrated fix for the first attempt's contention;
the original failure is retained, not hidden as a coverage-only refusal.

## Combined-observation follow-up: qualification, incomplete comparison

Candidate `d4d9ccfd3c30f6b72b30358e010c18cc9d5e0d11` passes the complete native
prerequisite in [Actions 42994708398](https://github.com/kimak-irmagi/sqlrs/actions/runs/42994708398):
actual pinned-helper/inner-order controls, three-step and fully cached prepare,
known second-step failure, exact cleanup, unchanged three-JVM/manifest/survivor
checks. The first attempt's new fixture incorrectly sent a shell command to
the stock-Liquibase API; its separately approved test-only correction changes
neither production code nor the pinned helper. All source regression jobs in
CI `42994686911` pass; only the previously excepted aggregate coverage gate is red.

[Repeat CT13 43000348314](https://github.com/kimak-irmagi/sqlrs/actions/runs/43000348314)
is **incomplete**, not a replacement for the complete original report above.
Six members finished their oracle/count/cleanup checks; collection of the
second old-v2 member failed with closed cause/cleanup classifications `unknown`.
This does not identify a candidate regression, establish a transient cause or
authorise dropping that control. The [original incomplete JSON](issue89-cost-evidence-2026-11-11/43000348314/jhipster-cost.json)
is retained without reserialization, SHA256
`7fa22f753b5a73f5b4b891ca9c3b1c6de9f69f9836838e2c5787205e2382fd41`.
Whole retry `43002214752` used the unchanged candidate/protocol on a fresh VM;
its complete result is separate below. The incomplete attempt remains retained.

## Completed repeat CT13 after combined observation

[Actions 43002214752](https://github.com/kimak-irmagi/sqlrs/actions/runs/43002214752)
completed all eight members / 16 passes on one VM, in the original reversed
four-route order. Every oracle/cleanup flag and the native 5.0.3 prerequisite
was checked. New off/on use one binary and helper. No strace or laptop workload.

- Candidate: `d4d9ccfd3c30f6b72b30358e010c18cc9d5e0d11`;
  executable SHA256 `f5374628f9863cf83400ec65ac6084272ad1a6bb72b09527d10a2b66de663bd6`.
- New helper digest in both modes:
  `sha256:3c269e3a7b25363a26a96fd88772bd35b4a5acd7c12f6cfd58578c3e91c1240f`.
- Actual v1, old `668d505` executable and collector hashes are unchanged from
  the original evidence above. This is not an isolated same-VM `48e6e11`/CPR A/B.
- [Original complete JSON](issue89-cost-evidence-2026-11-11/43002214752/jhipster-cost.json),
  SHA256 `59776bbe97307a23a7e65cd97f38cc0e2386d54ef06f019066d28b4265d1d22a`.

Descriptive means of two observations per cell, seconds:

| Variant | Fresh | Fully cached |
| --- | ---: | ---: |
| Actual released v1 | 48.218 | 8.498 |
| Old v2 `668d505` | 58.333 | 10.336 |
| Current v2, recording off | 55.820 | 10.179 |
| Same current v2, recording on | 55.528 | 10.337 |

Ordinary current v2 saves 2.513 s / 4.31% fresh versus old v2, including O1–O4
and combined observation, not CPR alone. The cached mean saves 0.157 s / 1.52%,
with overlapping sample ranges: not a proven reliable cached improvement.
Against actual v1 it remains **7.602 s / 15.77% slower fresh and 1.681 s /
19.78% slower cached**. The short-workload regression remains; do not claim
full P01b or merge readiness. Recording-on minus off is −0.293 s fresh /
+0.158 s cached, variation rather than negative/known zero observer overhead.

### Current additional work, without double counting

Rows use the two enabled fresh passes (about 55.528 s). Ratios are means of
their independently validated per-sample ratios. These rows are disjoint;
their nested Docker/copy/helper intervals must not be added again.

| Additional action | Calls | Mean/call, ms | ZTotal, ms | Zratio |
| --- | ---: | ---: | ---: | ---: |
| Combined private read / idle / prior absence | 8 | 120.024 | 960.191 | 1.729% |
| Plan + admit + complete journal trio | 8 trios | 8.510 | 68.080 | 0.123% |
| Extra verify after DB resume | 7 | 25.478 | 178.347 | 0.321% |
| Immutable image/Java/driver inventory | 1 | 953.282 | 953.282 | 1.717% |

These selected additional operations total 2,159.900 ms / 3.890%, **not the
whole v1 regression**. Source mappings are the table and catalog above, with
`combined_capture_prune.go:readCombinedStepPrune` now inside the runtime read.
The normal `capture.prune` count is zero, but its real proof remains in that read.
Compared with the earlier instrumented `48e6e11` report, Docker-command counts
fall from 142 to 107: exactly 35 = seven later steps × five removed round trips.
That is an observed count change, not a same-VM wall-clock difference between
those two instrumented revisions. Old read+prune (864.522 + 1,496.805 ms) and new
combined read (960.191 ms) came from different VMs; do not claim their subtraction
as an isolated causal saving. The actual same-VM aggregate improvement is above.

Migration CLI counts remain nine fresh / one cached in all variants. v2 still
adds its inventory CLI (ten/two total versus v1 nine/one) and creates eight/four
containers versus v1 four/two; no per-step container regression was reintroduced.
Current enabled planning costs 7.174 s fresh / 6.068 s cached, self time
4.164 / 3.005 s. Working startup is 2.844 s, self 2.660 s. Those coarse parent
costs remain changed work, not evidence that SQLite or DB startup alone caused
the unexplained difference. Next-scope discussion must preserve that distinction.

## Source finding: preview-to-working DB reuse differs

The released `jobCoordinator.planLiquibaseChangesets` retains the preview DB
in the job runner for ordinary prepare. Its `taskExecutor.ensureRuntime`
reuses it when the requested first input is the same image/source. This is
visible in the pinned v1 `manager.go` and `execution.go`, not inferred from a
container-count difference. The current `observePlanNormalContainerLiquibase`
instead calls `cleanupRuntime` for that DB unconditionally; later
`observeStartNormalLiquibaseSession` calls `startManagedRuntimeWithPlan` again.
Thus v2 can perform a second DB clone/start after preview where v1 reuses one.

This is a specific additional action, **not a measured price for that action**:
the existing `prepare.session_startup` also contains runner/config/readiness and
journal work. Assigning its entire duration to the second DB would be wrong.
An optimisation candidate is an exact ownership handoff only when the first
uncached input matches the preview source. A different cached prefix still
requires its own correctly sourced DB; plan-only/fully cached behavior must not
be conflated with that route. Any handoff needs its own reviewed component,
storage and test approvals. No implementation or broad recovery change is
authorised by this source finding, and no saving is promised before measurement.

## Overhead review journal and the next measurement

2026-11-11, Asia/Novosibirsk; exact message times unavailable.
Participant: @evilguest (41718235); agent: Codex / GPT-5.
The user requested an explanation first, then a written record, tracing of the
remaining time, and another measurement. This request **does not authorise
removing protections or changing execution behavior**.

1. `runner.inventory` is neither SQL validation nor a migration invocation.
   `InspectRunnerImage` starts a separate container, `java -version`,
   `liquibase --version`, and hashes discovered PostgreSQL JDBC JARs.
   `bindFreshContainerLiquibase` / `driverbundle.Select` use this to select an
   embedded/supplied driver and bind its identity. Supplying a driver and a
   correct identity do not inherently require repeating this inventory for
   every new prepare. Its present cost is about 953 ms fresh / 1021 ms cached;
   this is a chosen mechanism, not a Liquibase requirement. A replacement is
   neither designed nor authorised yet.
2. `capture.read` is not just log reading: two exact-container observations,
   stdout/stderr/manifest archive transfer, ID/EOF/length/hash checks, and proof
   that no child or previously published output remains. Released v1 obtained
   streams and the result directly from process execution. V2's retained result
   supports the chosen lost-response recovery protocol; reusing a container
   alone does not require it. About 120 ms/step covers the whole sequence, not
   copying text.
3. The three journal writes (plan, admit, complete) cost about 8.5 ms/step
   together. They distinguish possible-execution boundaries after a failure;
   the particular three-operation split is an implementation decision, not a
   Liquibase requirement. This is not the main cause of a 7.6-second regression.
4. `db.verify.after_resume` additionally calls the full `VerifyManagedAccess`,
   not a ping: correct administrative credentials, replication, and rejection
   of wrong/absent passwords on both routes (six connections). V1 already
   checked before snapshots; v2 adds the after-resume check. For the seven later
   **changeset applications**, not plan-building steps, this is about 25.5 ms
   plus 8.5 ms of journal writes, or roughly 34 ms.
5. Recorded and ordinary passes are different observations. The selected
   disjoint extra operations total 2.160 s but do not explain the entire 7.602 s
   gap. The remainder cannot automatically be labelled SQLite, JVM, or the cost
   of a second DB. Preview DB deletion followed by another startup is proven
   in source; its separate duration has not yet been isolated.
6. The present catalog/summary is insufficient for full classification: the
   worker begins after the API timer; substantial self time remains inside
   planning/session startup. Conserving the total through `unassigned` proves
   arithmetic, not the cause of those intervals.

The next result must contain chronological non-overlapping source-linked
intervals, separate API/queue/observer boundaries, and explicit overlaps and
unresolved regions. Their sum must equal the selected window's wall time;
nested/helper durations must not be added twice. Both DB creation/disposal
boundaries need actual timings, not renamed parent timers. Historical reports,
hashes and the speed gate remain unchanged; workloads run only in Actions.

### Pinned-source correspondence correction before the next campaign

Direct inspection of `cab99c4c7ced5b18f4dc43a65b002d84d8fef02d` found the
released managed startup in `internal/prepare/managed_execution.go`, not only
the generic `execution.go` wrapper. That path already has `managedMu`, source/
base setup, `access.Assign`, StateFS clone, runtime start, `access.Attach` and
full startup access verification. Released `Submit` also binds managed identity.
The new diagnostic catalog therefore marks those unchanged mechanisms as shared
and links the actual managed symbol. Tracked clone/name startup is changed;
its entire duration is not automatically additional. The number of startups
and their individual costs must be measured separately. Historical measurements
and the extra after-resume check are unchanged. No nonexistent v1 zero costs
or subtraction of different passes are introduced by this source correction.
