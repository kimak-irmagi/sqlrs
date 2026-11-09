# #89: connected normal execution

Implementation checkpoint, 2026-11-07, under the previously approved delivery
split. This is not an extension of #170–#174 or a release-ready claim.

## Connected components

- Startup uses `sqliteconn.Open`, the verified final catalog and queue-owned
  instance publication pins. It explicitly supplies trusted container dependencies.
- Admission captures/leases immutable local roots. Binding pins the daemon-platform
  Liquibase manifest, verified JDBC, settings and managed database identity.
- The worker journals an independent preview before creating its resources.
  Preview capture yields metadata; preview resources retire before continuation.
- Plan-only stores just its real plan task, with predictions in a separate public
  projection. Cache explain saves a point-in-time decision, returns HTTP 202,
  and exposes that decision separately from a prepare result in all projections.
- Prepare skips the cached prefix and shares one PostgreSQL container and one
  Liquibase parent through remaining `update-count 1` CLI calls. Snapshotting may
  stop/start PostgreSQL. Exact predecessor authority bounds private output to two
  slots; no permanent JVM/callback integration is introduced.
- Cache verifies execution identity, managed seal and physical publication marker.
  Instance/use-count/intent/pin publish together; projection cleanup precedes
  terminal success and release of the publication pin.
- A manifest-proven child failure selects a sanitized immutable error and cleans
  exact resources before terminal failure. Accepted invocation cancellation is
  saved before signalling; independent stop/absence proofs precede disposal.
- The release JAR is transferred and checksum-verified during WSL installation,
  then rechecked by installed-engine validation. Runtime never downloads it.

Production does not fall back to host Liquibase. Independent injected-runner
tests retain their earlier path; psql does not switch executors. Interrupted or
unprovable work remains visibly owned in `recovery_required`; startup never
replans it from mutable files or repeats SQL. Automatic continuation, protected
cache handoff, extended SQLite qualification and expanded search/defaults/ref
grammar remain #170–#173. The initial adapter uses nonempty local search roots.

## Evidence and remaining gates

Actual-service Docker checks cover three changes in the same runner/database,
real rows/`DATABASECHANGELOG`, each checkpoint, an exact hit, two cached steps
plus one application, plan/explanation, and temporary absence before fallback.
Windows main functionality and separate failure/cancellation checks passed.
Linux UID 1000 failure/cancellation passed; the full main rerun with managed
shutdown/public deletion also passed (214.59 s). The preceding force-removal
fixture cleanup failure is fixed. On production commit `9128bf7`, all three
Windows Docker cases passed again with cross-package coverage: main 297.25 s,
known failure 66.50 s, cancellation 63.31 s. Full engine vet also passed.

The three limited fixture amendments were approved and applied in `bfb4f30`:
canonical temporary roots in the named macOS preparations, the queue pin lookup
in managedExecutionFixture, and verified JDBC reader results in exactly three
legacy WSL installer/repair cases. Assertions and deliberately created links
remain unchanged. Both complete Windows modules pass. All 14 test/documentation
jobs in CI run 42370736037 pass, including Windows/Linux/macOS regressions.
The remaining coverage-merge job fails its unchanged 95% gate: engine 74.3%,
client 94.8%. Those ordinary CI profiles exclude the opt-in local Docker checks
and are not the cross-package union below. Repeated real-v1 comparison and PR
readiness remain open.
The installation-only 96.41%/97.22% figures do not describe this larger journal;
no whole-delivery 95% coverage or finished #89 is asserted here. The block union
of the successful full Windows run and the successful native run of the same
production code is 84.01% overall: prepare 87.04%, queue 63.80%, runtime 92.56%,
engine startup 94.92%, access 95.63%. The user subsequently approved an explicit
[#89-only exception](../adr/2026-11-07-issue89-coverage-exception.md): aggregate
coverage remediation belongs to #174; green regressions and fulfilled functional/
performance criteria remain mandatory. The shared CI threshold is unchanged.
This resolves coverage as a #89 acceptance blocker, not the red CI result or
merge authorization. The largest
uncovered files are completion_session_disposal_journal.go (262 statements),
completion_finalization_journal.go (173) and liquibase_normal_worker.go (156).
Per-block reports/HTML are in the ignored engine coverage directory.

## Preliminary real-v1 comparison (not performance acceptance)

The unchanged external benchmark collector ran the public jhipster workload
against a clean build of delivery revision `bfb4f301fa551fca64532bc21bcc2ee79e80cca3`
and the real v0.1.1-rc.7 artifact, revision
`cab99c4c7ced5b18f4dc43a65b002d84d8fef02d`. Both complete uncached/cached runs
passed the independent database oracle (eight changesets, then eight cache hits)
and cleaned their exact temporary resources. No sqlrs containers remained.

| Route / pass | Elapsed seconds | All Docker creates | Migration CLI starts |
| --- | ---: | ---: | ---: |
| v1 host / uncached | 262.173 | 3 | 9 |
| new container / uncached | 347.524 | 9 | 9 |
| v1 host / cached | 31.418 | 2 | 1 |
| new container / cached | 49.144 | 4 | 1 |

The container route adds one offline image-inspection CLI start per pass.
Derived stock-launcher JVM starts are 18/21 uncached and 2/5 cached (v1/new),
not independent process telemetry. Both routes use Liquibase 5.0.3 with identical
core/JDBC checksums and the same warmed PostgreSQL digest. Host Java is Temurin
21.0.2; container image metadata declares jdk-21.0.11+10. Host/container execution
and Java builds differ explicitly, as approved for the real-v1 comparison.
Engine SHA256: v1 `36316b24e61c6673a20f0c37523f0a2f4705b9b81ddc1fbc9146e68a578dd62d`;
new `94c144efce60aa243f457bf237966f8aea9461e29fe8ef31792d4768984a90d4`.

This first pair ran new then v1, without concurrent sqlrs workloads. The new
route is 32.6% slower uncached and 56.4% slower cached in this observation.
It is a warning, not a confirmed statistical regression: five alternating pairs,
v1/v1 noise controls and an independent confirmation series are still absent.
No speed acceptance, ready-for-merge status or completion of #89 is claimed.
The joint all-platform CI/local/native coverage union also remains below the
threshold: 13189/15636 statements (84.35%); the distinct CI gate is unchanged.

## Follow-up diagnostics after the coverage exception

The explicit coverage exception is recorded above and in #174; performance is
not waived. A separate diagnostic pair passed the same independent jhipster
database checks: new 286.093/83.264 s, real v1 229.828/33.941 s (uncached/cached).
These retained-store probes are marked Qualifying=false and are not added to
the statistical acceptance cohort. Read-only inspection exported only phases
and relative timestamps, never requests, result JSON, credentials or SQL.

| Journal interval | New seconds | Real v1 seconds |
| --- | ---: | ---: |
| Job admission to first working task | 65.094 | 56.848 |
| First working task, including startup | 42.080 | 20.506 |
| Median later working task (7 samples) | 22.082 | 20.673 |
| Final checkpoint to terminal result | 18.185 | 0.751 |
| Cached job: admission to plan completion | 59.913 | 17.976 |

Source inspection confirms two extra database allocations in the uncached path:
the separate working clone after preview, and the result clone after disposing
of that working clone. This explains concrete work, not a statistical claim that
all elapsed differences have one cause. A bounded
[working-instance handoff proposal](issue89-working-instance-handoff-design.md)
addresses only the final replacement; its components are approved. The separate
[storage amendment](issue89-working-instance-handoff-schema.md) is now approved.
The [test inventory](issue89-working-instance-handoff-tests.md) and limited
OT-WH01–OT-WH03 are also approved; implementation/verification is in progress.
No completed handoff or speed acceptance is asserted by the earlier evidence.

The unchanged collector also passed the mobilitydata-user public workload on the
same delivery artifact: all 12 changesets matched the independent database oracle,
then all 12 cache hits matched it again. Diagnostic times were 363.435/50.146 s;
total Docker creates were 9/4 (uncached/cached), the same counts as the eight-step
jhipster workload. It is Qualifying=false, not speed acceptance. Exact-root
container cleanup completed; no sqlrs containers remained. The 102-change
mobilitydata-main workload is not qualified by this separate 12-change check.
Redacted timestamp-only reports were retained outside the workspace. The two
private diagnostic stores remain ignored under the original worktree's .tmp:
the environment rejected the cleanup command; no alternative deletion was tried.

## Post-handoff qualification, 2026-11-08 (in progress)

The handoff and terminal job-cascade correction are implemented at delivery
revision `49f21709b6592dd3c74ae074a586f0dd0c695208`. The complete ordinary Windows
engine regression and vet passed; current native Windows/Linux functional
evidence and its limits are recorded in the [handoff test inventory](issue89-working-instance-handoff-tests.md).
No automatic replay, preview-DB adoption or expanded recovery delivery is claimed.

An isolated normal Git clone produced the Windows/amd64 comparison engine with
Go 1.25.5, `vcs.modified=false`, that exact revision and SHA256
`6dde978664afb57a2ef9ce39d7769e24293d8f8dda68e8e3632ede02c43ebd95`.
The standard project packager installed the verified JDBC 42.7.13 beside it.
The original unchanged external collector is used only to drive this separate
artifact, not to qualify the original mixed worktree's production implementation.
No post-handoff timing or speed acceptance follows from artifact preparation.

The first post-handoff paired jhipster observation subsequently passed both
independent DB oracles and exact cleanup, in order new then real v1:

| Route | Uncached | Cached | Docker creates uncached/cached |
| --- | ---: | ---: | ---: |
| New handoff artifact | 207.065 s | 34.453 s | 8 / 4 |
| Real v1 host artifact | 148.999 s | 25.418 s | 4 / 2 |

This observation still warns of slower new-route execution (+39.0%/+35.5%),
not completed statistical qualification. Do not compare its absolute times with
an older pair as a measured handoff speedup. Source/artifact/image/JDBC identity,
CLI/derived JVM counts and samples are recorded in the
[sample artifact](issue89-working-handoff-benchmark-samples-2026-11-08.json).
The reverse-order second pair also passed: v1 139.875/25.096 s, new
175.101/49.878 s (uncached/cached). Its spread is another reason not to infer a
speed pass. The first five alternating pairs, noise controls and independent
confirmation are incomplete. On 2026-11-08 the user requested retaining the full
protocol but moving it to GitHub Actions if feasible, to avoid laptop load.
The local scheduler was stopped; the already active collector was allowed to
finish cleanup. No collector or sqlrs test container remained afterward. The
third partial pair is excluded, regardless of its completed new-engine samples.
The [Actions design](issue89-performance-actions-design.md) for a pinned Linux
reference and same-VM paired collection was approved on 2026-11-08. The separate
[reviewed test plan](issue89-performance-actions-tests.md) was separately approved.
At that checkpoint no remote benchmark had run; no performance gate was waived.

Update: the approved collector subsequently passed the actual Ubuntu pilot in
[run 42475910841](https://github.com/kimak-irmagi/sqlrs/actions/runs/42475910841).
Both real engines passed full DB/cache and cleanup checks on JHipster and both
MobilityData routes (including 102 changes). The largest full-cohort estimate is
257.66 minutes including controls/overhead/contingency/reserve, below six hours.
See [pilot evidence](issue89-actions-pilot-2026-11-08.json) and the
[execution inventory](issue89-performance-actions-tests.md). Full PA12 collection
is now supported by the pilot budget estimate, not guaranteed to fit every VM;
no timing acceptance follows from the pilot.

The full opt-in workflow was launched on exact source `668d505`:
[42479055486](https://github.com/kimak-irmagi/sqlrs/actions/runs/42479055486).
It repeats the pilot before the complete comparison/control/confirmation jobs.
The small helper-only profile exposed a test-file classification issue;
[the bounded clarification](issue89-performance-actions-design.md#proposed-clarification-integration-test-versus-helper-coverage)
was approved by @evilguest on 2026-11-08. Its targeted helper test inventory
was separately approved as HC01–HC07 on 2026-11-08 and now passes: ordinary Go
helpers cover 174/174 statements (100%). The unchanged native scenario moved to
`_test.go`; 383 reclassified assertions are not counted as newly tested helpers.
No running cohort, product behavior or statistical rule changes. #89/#175 remain
open pending the remaining gates.

The four completed short-workload Actions cohorts now confirm slower v2 in
uncached and cached passes, beyond the observed controls on both independent
VMs. All six cohorts are now complete and retained, including both 102-change
cohorts. The diagnostic design, DG01–DG12 and OT-DG01–OT-DG03 are approved and
implemented in the observer only. DC01–DC03 are also approved and pass: ordinary
Go helpers cover 98.0% (494/504; previous helpers remain 100%), Node 100% lines;
Linux vet passes. The native strace smoke and
eight-member diagnostic have not run yet. Measured product bytes are unchanged. See
[paired results and retained evidence](issue89-actions-performance-results-2026-11-08.md).
This is a release-blocking timing failure, not a waived gate or completed #89.

2026-11-09: all three attempts of diagnostic run `42580413674` stopped before
measurement on release-archive HTTP 504. The official Releases API independently
returned the archive with the original SHA256; its binary was not run.
The preparation clarification and TA01–TA06 are approved. Implementation passes
32 Node tests/full small Go/vet (Node helpers 100% lines, Go 98.0%). Actual TA06
retry is not complete yet. No diagnostic report or established slowdown cause exists
yet; the product is unchanged.

`42603675428` subsequently passed preparation/API/strace and two plain members,
but failed on the first traced v1; the original incomplete report is retained in
the test-inventory amendment. DG03/DG09 now expose only closed failure codes for
the next retry. CI `42603673967`: the first native managed-access attempt failed
on retry after cancellation boundary 23; unchanged code/tests passed attempt 2.
This is not claimed as a fix for that failure's cause. Only the existing aggregate
coverage gate is red on attempt 2; the speed criterion is not waived.

Observer `a75316b` implements the separately approved EU measured-only uncertain
exec boundary, with strict tests unchanged. Small Go/vet pass at 527/537 (98.1%);
34 Node tests pass with ordinary helper lines 100%. Actual `42618850460` passed
setup/smoke/plain members, then failed traced member 3 at `trace_8`; its incomplete
report is retained in the diagnostic test inventory. Record shape and exact
product cause remain unknown. The failure-context project and EU04 clarification
are now approved. EU04 passes its additive test (35 Node tests, helper lines 100%);
the separate CX01–CX05 inventory was then approved before context implementation.
Product binaries and the performance gate remain unchanged.

Observer `9fab753` passes all small Go/vet (544/554, 98.2%) and 37 Node tests
(ordinary helper lines 100%). Native retry `42632130376` proves closed failure
transport: traced member 3 stops at `trace_8 / pending_entry / other`, last phase
`oracle`. The original report is retained; this does not prove the failed workload
passed or localise the product regression. Source CI `42632129606` passes all
regressions, only coverage-merge red. Further structural-prefix observation is
separately approved (recorded 2026-11-10), followed by separate PX01–PX05 approval.
Prefix-only implementation passes small Go/vet (579/589, 98.3%) and 39 Node tests
(ordinary helper lines 100%). Actual PX05 is pending. No new trace syntax or
product optimisation is implemented.
