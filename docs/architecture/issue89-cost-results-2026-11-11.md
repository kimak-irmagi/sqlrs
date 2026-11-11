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
[proposed component amendment](issue89-cost-attribution-and-trimming.md#6-proposed-combined-completionprune-observation)
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

Separately, current source CI `42967847154` attempt 1 failed the old
`TestPrepareJobsDeleteBlockedWithoutForce` on Windows with `SQLITE_BUSY`.
An unchanged local repetition passed 20 times; attempt 2's unchanged failed CI
job also passes. Only aggregate `coverage-merge` is now red under the approved
exception. This is not a demonstrated fix for the first attempt's contention;
the original failure is retained, not hidden as a coverage-only refusal.
