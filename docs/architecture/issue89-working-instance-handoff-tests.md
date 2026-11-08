# #89: working-instance handoff tests

Status: WH01–WH09 and the limited OT-WH01–OT-WH03 changes approved by
@evilguest on 2026-11-07 after critical and repeat review. Implementation and
verification are in progress; this is not a completed WH01–WH09 acceptance claim.
The [components](issue89-working-instance-handoff-design.md) and
[schema](issue89-working-instance-handoff-schema.md) are approved. This document
records that explicit approval; additional old-test changes still need approval.

## Scope and evidence

Only replace the unnecessary final working-database destruction/recreation.
No preview adoption, automatic SQL replay, prototype migration, #104 shortcut,
JVM integration or full #170–#172 failure matrix. Public CLI/API are unchanged.
Two reopens below prove durable ownership, not automatic recovery execution.

Use real file-backed SQLite with FK enforcement, actual queue writers and
independent connections for committed visibility. Each negative case has a
valid same-phase control, a classified error and evidence that the intended
boundary was reached. Compare all journal/catalog rows on refused writes;
do not equate an arbitrary earlier error or rollback on Close with success.
External runtime observations must happen outside SQLite transactions and the
shared eviction lock. Test-only wrappers may observe the real driver using
existing boundaries; no new production injection interface is authorized.

The test names below are proposed names, not claims of existing coverage.

## WH01 — installation and independent schema checks

New `TestWorkingInstanceHandoffSchema` in managedstore. Fresh installation and
the exact frozen released-baseline bridge produce session format **3**, the
same other component versions and no new tables. Independently assert the six
named columns/types, deferred outcome FK with cascade, all-or-none identity,
canonical digest, immutable fields and NULL database-absence/nonstart fields.
The two valid disposal families remain distinct: transferred live DB versus
ordinary disposal with actual DB absence. A transfer without its future outcome
must fail COMMIT with FK constraint, not merely fail an unrelated insert guard.
Use a complete valid same-transaction control to reach that COMMIT boundary.

Two physical reopens preserve the exact target and rows. Old prototype format
2, unknown format, a missing handoff column/guard and altered handoff definition
are rejected before mutations/inventory; keep the existing released-baseline
and installation fault tests. Do not build historical input with the new DDL.
Updated artifact hashes alone do not establish these behavioral properties.

## WH02 — exact publication authority

New `TestWorkingInstanceHandoffAuthority` in queue. Build a real completed suffix
whose original operation Source differs from its final state. A valid final
invocation/cursor, stopped parent, verified access and exact runtime/operation/
directory permit the writer. Change one authority at a time: job/session/clone,
execution/generation, physical DB/runner ID, state/cursor, operation/target/access
identity; unfinished SQL/invocation, another active owner, missing stop proof,
accepted cancellation, selected failure or prior outcome each refuse the write.
Require unchanged journal and no external actions. Exercise SQL guards directly
for malformed/foreign transfer, late attachment to an existing outcome and
handoff identity mutation; do not disable competing product guards to pass.
The ordinary cache-clone publication still rejects operation Source differing
from its output state: the new exception is specific to the exact handoff.

## WH03 — one atomic publication and minimal commit failures

New `TestWorkingInstanceHandoffPublication` and
`TestWorkingInstanceHandoffCommit` in queue. A successful transaction publishes
one exact instance, immutable prepare_success, bounded publication pin, transfer,
one state-use increment and completed prepare_instance task. A peer sees all,
never a standalone committed transfer. Source remains the original session
input; instance state is the final cursor. No database-absence fact is invented.

Limit new fault cases to this new writer: refusal after the transfer write and
after the instance/task effects; actual COMMIT BUSY from a second reader;
committed-but-lost reply; unavailable or partial settlement readback. Observe
the precise fault and actual SQLite effects. Check the original handle and a
peer before closing, then two physical reopens. A proved rollback leaves all
rows unchanged; known commit is exactly one publication. Explicit retry of the
same metadata operation cannot double-count. Unknown settlement is not success
and cannot authorize cleanup/new SQL. Settlement compares every handoff field,
both physical IDs, outcome/pin, instance/access/operation, use count and task;
partial evidence fails closed. No unrelated writer/Close matrix is imported.

## WH04 — final-publication-only database resume

New `TestWorkingInstanceHandoffResume` in queue/prepare. With the final checkpoint
published and parent stopped, persist restart request before resuming the same
DB; record exact readiness before activation/publication. A currently running
matching DB is reused without replacement. No remaining SQL, failed invocation,
missing stop proof, cancellation or selected failure can take this narrow route;
it never admits another session_exec. Stop, resume, identity and activation
failures retain the exact job owner and do not invoke destructive cleanup or
allocate another result DB. The runtime double checks IDs/order and reports
which exact refusal was reached; credentials never appear in diagnostics.

## WH05 — private-only cleanup and logical retirement

New `TestWorkingInstanceHandoffPrivateCleanup` in queue/prepare. Only a known
committed exact transfer and live publication pin permit parent/private cleanup.
Observe exact parent/output absence, retire both retained capture places and
private owners, and release session/clone job ownership without changing their
historical IDs. Assert zero StopDB, working-directory removal and DB-absence
writes. The live instance DB/path/operation remain unchanged and protected.
Missing/wrong receipt or pin refuses cleanup; parent removal/absence and logical
retirement failures retain protection and prevent terminal success. Terminal
completion requires all former private/projection owners released, then removes
only the existing pin. Observe SQL/lock freedom during external callbacks.

## WH06 — outcome order, deletion and retained history

New `TestWorkingInstanceHandoffOwnership` in queue/prepare. Deterministic barriers
show cancellation or failure selected first prevents handoff; committed success
cannot be replaced by a later outcome. No new outcome precedence is introduced.
Active publication/private ownership blocks job/instance deletion. After normal
completion, deleting the job must not delete the instance-owned DB/directory;
ordinary instance deletion can remove them, and state deletion remains subject
only to normal references. Two reopens after retirement and after ordinary
deletion retain valid history without repinning or requiring a deleted live
instance/state. Unrelated owners/rows remain unchanged. This is ownership proof,
not a new deletion/recovery service or cascade permission expansion.

## WH07 — normal coordinator and unaffected routes

New `TestWorkingInstanceHandoffCoordinator` in prepare uses actual queue records
and observing runtime/access doubles. Require final checkpoint → exact parent
stop → same DB readiness/access activation → committed publication/transfer →
private cleanup → terminal result. No new StartManaged after the last working
step, no fallback to ordinary destructive session disposal, no SQL after the
final cursor. Publication/cleanup failures retain ownership as above, without
claiming automatic restart recovery. Pure cache hits retain the verified-clone
route; plan-only and cache explain retain preview disposal, not handoff.
Existing failure, cancellation, capture-prune and psql regressions stay intact.

## WH08 — native Docker identity, data and cleanup

Strengthen `TestMainPathLiquibasePrepareDocker` only with OT-WH03 below; add
`TestWorkingInstanceHandoffDockerOwnership` for job-deletion ownership/order.
Run on native Windows and Linux as an ordinary UID 1000 user, not root-only.
For three uncached changes and for a cached prefix followed by an uncached tail,
the published runtime ID equals the last working DB ID. Independently query
actual user data and DATABASECHANGELOG, including the expected execution count.
No final replacement DB is allocated; one Liquibase parent serves the suffix,
with the existing two-place output bound and independently verified prefixes.

Before test fallback cleanup, the parent/private owners are gone and only the
published DB remains from the job's allocated containers. Delete the terminal
job and query the same live DB again; ordinary instance deletion then removes
that exact runtime/directory. Track only test-owned IDs; never stop unrelated
Docker containers. Keep genuine cache-hit, plan-only and saved cache-explain
checks and all old assertions. Opted-in Docker runs with missing prerequisites
fail rather than silently skip. A metadata-only/double test cannot replace this.

## WH09 — regressions and the existing performance gate

Run both full Go modules and vet, the three existing normal-path Docker tests
plus WH08, and unchanged applicable CI on Windows/Linux/macOS. Measure coverage
without claiming 95%: the explicit #89 exception transfers aggregate remediation
to #174 and does not change the shared CI threshold. No coverage-only iteration
or permission to rewrite other old tests is included.

Repeat the already agreed real-v1 benchmark procedure against exact new artifacts:
same public workloads, locked Liquibase/JDBC versions, uncached/cached cases,
independent DB oracle, container/CLI counts and existing repeated-measurement/
noise criteria. Keep the approved host-v1/container-new difference explicit.
No single pair or removed allocation substitutes for the speed criterion.
The earlier 12-change mobilitydata-user check does not qualify the 102-change
mobilitydata-main workload; retain its outstanding qualification. No private
izess dependency. No new absolute timing threshold is invented by this plan.

Commands, from each module (PowerShell: quote dotted flags as a whole):

```powershell
# backend/local-engine-go; Docker cases run separately with existing opt-in/JDBC setup
go test ./internal/managedstore ./internal/prepare/queue ./internal/prepare -run '^TestWorkingInstanceHandoff' -count=1 -timeout=10m
go test ./... -count=1 -timeout=20m '-coverprofile=coverage/issue89-working-handoff.out'
go vet ./...
go test ./internal/prepare -run '^(TestMainPathLiquibasePrepareDocker|TestMainPathLiquibaseKnownFailureDocker|TestMainPathLiquibaseCancellationDocker|TestWorkingInstanceHandoffDockerOwnership)$' -count=1 -timeout=30m
# frontend/cli-go
go test ./... -count=1 -timeout=20m
go vet ./...
```

## Separate permission: limited old-test/artifact changes

- **OT-WH01:** only `niAssertTarget` in managedstore/main_path_installation_test.go:
  session marker 2→3. All other marker expectations and invalid-format-99 cases
  stay unchanged; add new format-2 rejection in WH01 rather than rewrite history.
- **OT-WH02:** only current-target expectations in
  queue/main_path_installation_artifact_test.go and its three current artifacts:
  assets/liquibase-main-schema.sql, assets/liquibase-main-catalog.json and
  managedstore/testdata/issue89-target-catalog.json. Update the three pinned
  hashes and reviewed handoff-object definitions/count only. Append amendment
  provenance in issue89-schema-provenance.json and update its current artifact
  hashes; preserve the frozen released SQL/catalog hashes, historical composed
  282 objects/144 source-file identities and released 57-object provenance.
  Review the exact semantic DDL diff before accepting new target hashes; every
  unrelated definition remains identical. WH01 independently verifies behavior.
- **OT-WH03:** only strengthen TestMainPathLiquibasePrepareDocker with equality
  of published and last working DB IDs in the initial and suffix cases, using
  existing observations. Preserve every current data/cache/prefix/cleanup
  assertion, the fallback cleanup and separate failure/cancellation tests.

These permissions are separate from approving new tests. No tests are removed,
skipped or weakened. Any additional contradiction requires a separate decision.

## Critical review, corrections and repeat review

1. P1: equality of metadata would miss publishing a different live database.
   WH08 checks the actual runtime ID, user data/changelog and job deletion before
   fallback cleanup; WH07 also forbids the extra allocation.
2. P1: new golden hashes could merely reproduce a bad generator. WH01 asserts
   columns/FK/guard outcomes independently; OT-WH02 preserves historical sources
   and requires a reviewed exact DDL difference, not blind regeneration.
3. P1: Close can conceal uncommitted effects. WH03 observes real COMMIT BUSY,
   exact fault reachability and same-handle/peer state before close; uncertain
   settlement does not authorize success or disposal.
4. P1: normal cleanup could delete the transferred job-shaped directory, or
   permanent FKs could retain it forever. WH05/WH06/WH08 cover both active
   protection and ordinary post-terminal job/instance/state lifetimes.
5. P2: a broad failure matrix would expand #89 again. Limit new faults to the
   new writer/ownership transitions; reopens are durable-read checks, not #170
   automatic recovery. Existing installation/connection matrices run unchanged.
6. P2: one faster run or a small MobilityData sample could falsely close the
   speed gate. WH09 preserves the existing benchmark protocol and outstanding
   main workload; it neither waives acceptance nor promises a measured gain.

Repeat review found no remaining P0–P2 contradiction within this bounded plan.
This is a design review, not executed test evidence or release acceptance.

## Implementation checkpoint, 2026-11-07

New schema/physical-reopen/damage tests pass, as do exact-authority variants,
deferred outcome COMMIT refusal, nullable-shape/immutability checks, real COMMIT
BUSY/rollback, ten lost-reply/full-transfer-readback cases, cancellation after
the real instance INSERT, private-parent refusal, retirement and terminal
history after ordinary metadata deletion. The authority audit additionally
found and fixed acceptance of a mismatched session start kind/position, number
and creation time; each now has a reproducing negative control. Strict final
proof still rejects pending private owners; only publication exempts its exact
transferred session. The unused old normal destroy/recreate coordinator helper
was removed; ordinary failure/cancellation disposal and queue APIs remain.

The existing strengthened native main test passed at the implementation
checkpoint: Windows 278.28 s, Linux/amd64 in Ubuntu as UID 1000 140.15 s. Both
cover the three-step run, exact hit, plan/explanation, independently restored
prefixes and one new suffix step, with the published ID equal to the last
working DB and cleanup checked before fallback. These are functional test times,
not a benchmark comparison or qualification of subsequent source edits.
The cleared-PGDATA Windows run also passed the separate known-failure and
cancellation tests. A first attempt inherited an unrelated application PGDATA
and correctly refused native access; only the child test process environment
was cleared. A later concurrent run timed out in plan-only and fallback; the
isolated complete main rerun above passed without test/timeout changes.

The complete client module passed. The engine run passed every package except
the existing startup idle-request timing test; that test subsequently passed
three isolated repetitions unchanged. The earlier full-run profile totals
75.0%, not a current all-green acceptance profile; the approved #89 exception
still applies. Engine vet passed at the checkpoint. No test-owned sqlrs Docker
containers remained after the native runs; unrelated containers were untouched.

WH06's new real queue test and Docker job-deletion test remain **red** on the
shared decision-evidence cascade guard. The proposed bounded correction in
[schema §4](issue89-working-instance-handoff-schema.md) awaits separate approval;
no guard bypass, skipped assertion or old-test weakening was applied. Remaining
WH02–WH07 negative/order checks, final all-current-source regressions/CI and
the unchanged real-v1 speed qualification are still required. This checkpoint
does not close #89 or make PR #175 ready.

## Cascade correction and bounded checks, 2026-11-08

The separately approved correction to the decision-evidence deletion guard is
implemented. The current catalog/provenance and three pinned hashes reflect only
that additional definition change; frozen release/composed-source provenance is
unchanged. TestWorkingInstanceHandoffTerminalJobDeletion now passes: direct link
deletion while the terminal job exists still fails with SQLITE_CONSTRAINT_TRIGGER
(1811) and the exact retention message, without row changes; deleting the actual
terminal job succeeds and retains the published instance.

TestWorkingInstanceHandoffDockerOwnership passed on native Windows (169.69 s).
Before fallback cleanup, temporary containers were absent; deleting the terminal
job left the same live database queryable and its directory intact. Ordinary
instance deletion then removed its exact runtime. This is functional evidence,
not benchmark acceptance or Linux qualification.

Additional approved WH02/WH03/WH05/WH06 cases pass: publication without activated
access, the unchanged strict cache-publisher source rule, selected failure before
publication, late cancellation/failure after committed success, five missing or
foreign cleanup-owner variants, cancellation after the real prepare-instance task
UPDATE, and four private-retirement transaction cases (after logical release,
real COMMIT BUSY, lost reply, unavailable settlement after a real commit).
They observe the precise reached boundary, retained publication pin and live
instance, with no partial journal effects after rollback. The new test's first
run corrected its own expectations to the existing exact SQL refusal and required
status_error failure profile; no product protection was relaxed.

Final current-source regressions, remaining coordinator/order checks, native Linux
qualification and real-v1 performance acceptance remain open. The earlier red
cascade checkpoint above is historical and resolved, not a current blocker.
