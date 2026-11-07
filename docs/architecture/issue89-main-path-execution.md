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

Engine regression currently has two old preparations without the required pin
lookup; the client has three old WSL preparations without JDBC. Their limited
updates were requested, not silently applied. macOS canonical temporary roots
also await the separately requested preparation approval. Full profiles, final
native reruns, repeated real-v1 comparison, CI and PR readiness remain open.
The installation-only 96.41%/97.22% figures do not describe this larger journal;
no whole-delivery 95% coverage or finished #89 is asserted here. The diagnostic
block union of the failing full run and successful current-head native run is
84.01% overall: prepare 87.06%, queue 63.80%, runtime 92.56%, engine startup
94.92%, access 95.63%. It is not a green-suite acceptance result. The largest
uncovered files are completion_session_disposal_journal.go (262 statements),
completion_finalization_journal.go (173) and liquibase_normal_worker.go (156).
Per-block reports/HTML are in the ignored engine coverage directory.
