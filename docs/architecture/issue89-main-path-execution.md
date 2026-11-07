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
engine startup 94.92%, access 95.63%. The 95% gate remains unresolved; a separate
question explicitly asks how it applies after the #174 split. The largest
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
