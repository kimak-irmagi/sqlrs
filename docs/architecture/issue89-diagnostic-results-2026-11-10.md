# #89: first complete diagnostic cohort

## Evidence and verdict

[Actions 42802379397](https://github.com/kimak-irmagi/sqlrs/actions/runs/42802379397)
completed all eight members and 16 fresh/cached passes. Every sample passed its
database/cache oracle and cleanup. This completes native MC08 and the original
diagnostic collection, **not performance acceptance or #89**.

- Measured v2 source: `668d5050ad154c1967bea4c6e40b91f3a703d5ba`.
- Observer: `81cc3a1f84f30e6c130ee8261c8e8b1ba8674d04`.
- v2 executable SHA256: `1367bce9ddaa44c0aa378b07c015dfb20a4c0d29f00d2f5a893905686fae14b0`.
- Released v1 executable SHA256: `b34c358d2d1e6e969f17d4e393641dd7cb53167870b8e4674db1381bf410bbca`.
- [Original report](issue89-diagnostic-evidence-2026-11-10/42802379397/jhipster-diagnostic.json),
  SHA256 `3e9bfe734748626f2b0f274bcfbc74ef3c8ccac980ea89f81e5efd3eda72af7d`.

Signed ClockDeltaNS for the eight traced passes in collection order:
`-10, -10, 0, -10, -10, 0, -10, -9`. No timing or measured binary changed.
CI `42802317646` passed every regression job; only aggregate `coverage-merge`
failed, subject to the already approved #89 exception/#174, not a speed waiver.

## Ordinary timings, seconds

These are two diagnostic pairs on one VM, not a replacement for the retained
full acceptance/noise/confirmation cohorts. Order was reversed in pair B.

| Pair | Fresh v1 | Fresh v2 | v2 minus v1 | Cached v1 | Cached v2 | v2 minus v1 |
| --- | ---: | ---: | ---: | ---: | ---: | ---: |
| A (members 1/2) | 43.110 | 53.007 | +9.897 | 7.919 | 9.838 | +1.920 |
| B (members 8/7) | 46.175 | 52.932 | +6.757 | 7.852 | 9.171 | +1.319 |

Descriptive means: fresh 44.642/52.969 s (+8.327 s, +18.65%); cached
7.886/9.505 s (+1.619 s, +20.54%). These tiny samples do not establish a new
confidence interval. They reproduce the direction of the existing regression.

Partition the ordinary fresh passes at the first executing step and the end of
the final task. Include final publication with the step chain so moving the DB
restart between tasks cannot masquerade as an added restart:

| Difference v2 minus v1 | Before first step | First step through final task | Remaining response tail | Total |
| --- | ---: | ---: | ---: | ---: |
| Pair A | +1.576 | +8.170 | +0.152 | +9.897 |
| Pair B | +3.170 | +3.575 | +0.012 | +6.757 |

Individual phase labels are not interchangeable between versions. In particular,
v2's final task takes 2.53–2.63 s versus v1's 0.051–0.052 s, but v1 resumes
PostgreSQL inside the preceding step; v2 resumes before the next step/final
publication. This is **not proof of an extra database startup**. The released
source must also be consulted before attributing a cross-version cost.

## What the trace proves, and what it does not

Tracing changes elapsed time substantially and differently for each route:
fresh mean overhead is +17.784 s for v1 and +10.482 s for v2 relative to each
route's ordinary members. Cached overhead is +2.759/+2.148 s. These are descriptive
differences, not a calibrated correction to subtract from individual intervals.
Host Java is traced in v1; container-internal Java is not traced in v2. The nearly
equal traced fresh totals (62.426/63.451 s) therefore do **not** refute the ordinary
regression or establish that v2's JVM is faster.

Both fresh traced members of each route have the same confirmed category counts:

| Category | v1 | v2 |
| --- | ---: | ---: |
| Generic Docker exec | 44 | 71 |
| Docker inspect | 1 | 47 |
| Docker copy | 0 | 28 |
| Step-helper calls | 0 | 15 |
| Docker run | 4 | 6 |

These categories are not all commands. `other` includes unclassified commands,
and `docker.step` mixes applying a change with observing prior-output disposal.
They cannot be interpreted as 15 migrations: independent workload counters
confirm eight changes and nine migration CLI starts (planning plus eight steps)
for both fresh routes. v2 additionally starts its image-inventory CLI once.
Cached routes have one migration CLI start each; v2 also has the inventory CLI.

Source-backed locations of additional work in the fixed v2 source:

- `bindFreshContainerLiquibase` calls `InspectRunnerImage` for each newly bound
  job. Its offline container executes `java -version`, `liquibase --version`
  and hashes JDBC candidates. Binding spans 1.35–1.67 s fresh and 1.06–1.09 s
  cached in ordinary v2, but this is the **whole binding phase**, not the isolated
  probe cost. Existing job-binding reuse does not imply cross-job inventory reuse.
- `InspectNamedContainer` performs both `container ls` and `container inspect`.
  `ReadCompleteLiquibaseStepOutput` observes the parent before/after three
  separate `docker cp` reads and a child-idle probe. `invokeStep` also observes
  the parent before execution.
- For subsequent steps, `VerifyLiquibaseCapturePruned` adds two parent
  observations, two idle probes and one `probe-prune` helper invocation. The
  source explains why helper-call count can exceed migration count, but the
  current report does not separate those helper modes or generic exec roles.

Under tracing, v2's inspect intervals total 1.238–1.244 s and copies
0.760–0.803 s. These are traced gross category durations, **not an estimate of
recoverable untraced time**. Nested shell/Java intervals must never be added as
exclusive costs. Union-uncovered fresh time is still 10.387–10.626 s for v2
and 11.410–11.585 s for v1; cached residual is 2.386–2.413/1.742–1.826 s.
Residual includes engine work and gaps, not a measured SQLite/FS/CPU bucket.

## Review and next boundary

Review rejected: blaming a 2.5 s final phase without accounting for shifted DB
restart; adding overlapping durations; treating helper calls as JVM starts;
calling all binding time image probing; treating residual as SQLite; and using
traced equality to dismiss the untraced regression. Repeated review found no
remaining P0–P2 in this evidence report. Exact attribution of the entire
regression remains open; no optimisation or merge readiness is claimed.

A narrow next observation should split **already captured** command arguments
into fixed roles (DB control, inventory, helper apply/probe, listing), without
new trace syntax or product instrumentation. Its component boundary and new
tests require separate approvals before code. Candidate fixes to evaluate
after attribution are reuse of verified immutable image inventory and reducing
redundant Docker round trips while preserving exact identity, private-output
integrity and disposal authority. Neither is approved or claimed sufficient.

Subsequent decision (2026-11-10): after discussing these limits, @evilguest chose
to remove demonstrated redundant work and add source-level cost attribution,
not require another strace-refinement cohort first. See the
[new component proposal](issue89-cost-attribution-and-trimming.md). The historical
numbers/limits above stand; do not retroactively claim exact causal attribution.
