# #89: normal-path installation test inventory

Status: NI01–NI08 approved by @evilguest on 2026-11-07; implemented and passing.
Production execution is now connected; see the current
[execution evidence](issue89-main-path-execution.md). The later explicit
[#89 coverage exception](../adr/2026-11-07-issue89-coverage-exception.md) moves
aggregate remediation to #174 without waiving regressions or performance.
The [component boundary](issue89-normal-path-storage-extraction.md) was approved
by @evilguest on 2026-11-07. This inventory covers that boundary only, preserving
the previously approved MP01–MP07 normal-path requirements.

## Shared evidence rules

- Use real file-backed SQLite for startup, upgrade, rollback and independent
  visibility. In-memory SQLite is allowed for artifact/DDL inspection only.
- The released-baseline fixture is frozen from delivery base commit
  36e164a, tagged v0.2.0-rc.1 (published prerelease verified 2026-11-07), with
  provenance and hashes. Do not construct the historical input by
  calling the new installer or the current SchemaSQL.
- Archive the mechanically obtained approved composed target catalog with
  provenance/hashes in this repository. Neither CI nor tests may depend on the
  accumulated worktree, an uncommitted external checkout, private izess or Docker
  just to install metadata. No hand-authored reduced substitute is accepted.
- Expected version tuples, essential columns/FKs and protective outcomes are
  specified independently, not obtained solely from the function under test.
  Target marker tuple: liquibase_store_format=2, prepare_attempt_store_format=3,
  prepare_liquibase_session_store_format=4,
  prepare_liquibase_finalization_store_format=2,
  prepare_liquibase_cache_selection_store_format=1; exactly one slot=1 row each.
  Format 3 was introduced by [working-instance handoff](issue89-working-instance-handoff-schema.md);
  the current format 4 is introduced by [O5 planning-DB handoff](issue89-planning-db-handoff-schema.md).
  PG01 and bounded OT-PG01 are approved. Earlier results below for format 2
  remain historical and do not qualify the later formats.
- Compare logical catalog definitions and all persisted rows before/after a
  refusal, including old jobs/tasks/events, state/instance/access metadata,
  Runtime v2 data and unrelated settings. Do not require byte-identical SQLite
  files/WAL headers or mistake rollback equality for proof of no write attempt.
- For preflight refusals, observe actual SQL calls through a test-only wrapper
  of the real driver: zero mutating statements and zero inventory callbacks.
  Preserve Prepare/direct Exec/Query behavior and actual transaction outcomes;
  no global hook or product-side fault switch is proposed.
- Each negative case has a valid control reaching its intended phase. Require
  phase/call counters and error classification; an arbitrary earlier error is
  not PASS. Test corruption is explicit and completed before startup.
- Two reopens mean closing the handle and using two new physical connections,
  not invoking Initialize repeatedly on the original transaction.
- Existing tests are not shortened, skipped or rewritten by this inventory.

## NI01 — canonical extraction artifact

Location/name: queue, TestMainPathInstallationArtifact.

Verify the embedded final SQL and expected-object artifact against the frozen
approved composed catalog and recorded source hashes. Execute the definitions
on real SQLite with their required baseline prerequisites; compare complete
object names/types/definitions, target markers and foreign-key metadata.
Check essential private/v2 columns independently, including readiness, both
capture slots, predecessor retirement links and disposal identities.

A missing/changed guard, missing dependency or mismatched artifact digest must
be detected. The positive control must build the complete target without
replaying intermediate prototype migrations. No regeneration of expected
history during a test run; artifact hashes alone do not qualify behavior.

## NI02 — new store and exact current reopen

Location/name: managedstore, TestMainPathInstallationFresh.

Initialize a new store through the actual startup entry point. Check the complete
target, marker tuple and PRAGMA foreign_keys=1. Seed valid managed metadata and
an unrelated settings row; reopen twice and preserve domain identity, all rows
and catalog. Inventory runs once for the empty initial cutover and not on the
two already managed opens. A second independent reader observes the committed
target, not the installing transaction's provisional result.

The no-op current-format path must not rewrite schema, rows or versions.
Normal MP01–MP04 queue checks will additionally exercise retained guards; this
metadata test is not their replacement.

## NI03 — exact released baseline bridge

Location/name: managedstore, TestMainPathInstallationBaseline.

Use the frozen released managed schema without any new Liquibase namespace.
Prepare valid lineage/state/instance/access metadata, settings, terminal lb
history and psql jobs (queued, running and terminal), with nontrivial old job
fields, nullable values, request bytes, tasks and events.

Upgrade through startup, checking every old column/value and reference.
Only the approved new job fields acquire purpose=prepare and
cache_explain_result_json=NULL; no explanation, SQL result or resource proof
is invented. Domain identity and published metadata remain unchanged;
foreign_key_check is empty. Two independent reopens preserve the result.
The already managed upgrade must not invoke the fresh-store inventory callback
or perform Docker/filesystem cleanup.

## NI04 — unfinished old lb preflight

Location/name: managedstore, TestMainPathInstallationRejectsUnfinished.

On the exact released fixture, test lb queued/running/recovery_required
individually. Each refuses before mutating SQL, marker installation and
inventory. All catalog/rows remain unchanged after refusal and two rejected
reopens. Positive controls: terminal lb-only history and unfinished psql
without unfinished lb (NI03). Do not add a path to cancel, clean up or resume
unsupported old work.

## NI05 — absent versus damaged/unknown namespace

Location/name: managedstore, TestMainPathInstallationRejectsDamage.

After a valid installation and successful open, separately remove/change a
required table, trigger or index; remove a marker row/table; explicitly rebuild
a marker table with an unsupported definition/value as corruption preparation.
Reserved-name table/view and case-varied collisions start from the released
fixture without that target object, not an impossible duplicate SQLite name.
Also reject altered released prepare_jobs, a half-converted job table,
a partial target namespace and a frozen unsupported prototype namespace.

Every refusal is read-only before inventory and leaves the entire corrupt
input unchanged. Warming a prior successful open must not hide later damage.
Unrelated non-reserved settings are a positive control, not automatically
classified as corrupt. Damage preparation must not fail at an unrelated FK or
guard and thereby prevent reaching startup.

## NI06 — installation/bridge atomicity

Location/name: managedstore, TestMainPathInstallationAtomicity.

Use named phases, a real driver and actual writes for these independent cases:

1. BeginTx refusal: no DDL or new markers.
2. Failure after an actual target table is created: rollback removes the whole
   new namespace and preserves all old rows.
3. Released-job conversion failures after copying and after dropping the old
   table, before final replacement: rollback restores all old definitions,
   values, indexes, triggers and FK relationships.
4. Final catalog/FK verification failure before COMMIT: no partial target.
5. Cancellation before start and after an observed write: preserve cancellation
   and the same rollback conditions.
6. Actual COMMIT SQLITE_BUSY from an independent reader in rollback-journal mode
   with busy_timeout=0: confirm the real driver code and attempted COMMIT;
   no success from uncommitted same-connection readback.
7. Lost reply after real successful COMMIT: no second installation or replay;
   success is allowed only after full committed-evidence verification. If that
   verification is unavailable, report uncertainty, not false rollback/success.

For precommit failures, an independent connection verifies the original catalog
and rows; after releasing the fault, the same file can install and reopen.
For a committed lost reply, independently verify the complete target and
preserved old rows before two reopens. No worker-ready store may escape while
connection cleanup or commit outcome is unproved. The full #172 Close/race
matrix is not imported.

## NI07 — connection and startup authority

Location/name: managedstore, TestMainPathInstallationConnectionScope.

Verify the exact startup connection restores foreign_keys=1 and
busy_timeout=0 before normal work, on fresh, bridge, no-op and precommit failure
paths. Prove FK enforcement by a real violating write with the exact SQLite
foreign-key code and a corresponding valid write; observing a PRAGMA alone is
insufficient. A failed setup/restore cannot publish a ready store.
When an unclean physical connection must be retired, assert its exclusion and
test the configured replacement; do not require queries on a closed connection
or accept its unproved cleanliness. Full close-failure diagnostics remain #172.

Record entry-point ordering: metadata preflight, bounded conversion/install,
catalog/FK verification, COMMIT/readback, then normal work. Existing exclusive
engine-lock tests stay intact. No Docker callback is permitted inside SQL
transactions; installation never starts an executor.

## NI08 — psql non-cutover and normal-path gates

Location/name: managedstore, TestMainPathInstallationPsqlUnchanged; existing
MP01–MP07/regressions remain separate acceptance gates.

Through real startup/store/queue objects, perform the existing ordinary psql
job/task/status and managed-state writes under their valid prerequisites.
Verify expected committed data, zero newly created common-attempt/session rows
and no new recovery activation. A positive control must reach completion under
the retained guards, not merely create an empty job.

Run unchanged managedstore, queue, prepare, psql and Runtime v2 regression suites.
Run the approved actual three-step Liquibase/cache/cleanup checks on the
delivery branch after production wiring, separately from the metadata tests.
Do not credit a double, a skipped Docker test or a metadata-only PASS as complete
prepare acceptance.

## Execution and coverage

From backend/local-engine-go, create coverage as a directory if absent:

```text
go test ./internal/managedstore ./internal/prepare/queue -run '^TestMainPathInstallation' -v -count=1 -timeout=10m
go test ./... -count=1 -timeout=30m -coverprofile=coverage/issue89-main-installation-full.out
go tool cover -func=coverage/issue89-main-installation-full.out
go tool cover -html=coverage/issue89-main-installation-full.out -o coverage/issue89-main-installation-full.html
```

Keep separate Windows/Linux reports, require every proposed test name and
counter, and run the unchanged CLI module regressions. Detailed uncovered-line
analysis and any additional coverage iteration require the existing approval
process. The later #89-only exception linked above supersedes the original
blocking 95% acceptance requirement; #174 retains the target and shared CI
thresholds are unchanged. Native MP checks use their own explicit conditions,
not an implicit opt-in in these commands.

## Critical review, corrections and repeat review

| Finding | Correction |
| --- | --- |
| P1: building the old fixture with current DDL makes upgrade testing circular. | NI03 freezes released input with provenance; all old columns/rows are compared. |
| P1: a matching metadata hash can hide missing guards or dependencies. | NI01 executes real definitions, checks the full artifact and independent columns/FKs; MP guard behavior stays mandatory. |
| P1: rollback equality does not prove refusal before any write/inventory. | NI04/NI05 observe actual SQL mutations and callbacks, with controls reaching startup. |
| P1: synthetic Commit errors miss SQLite's surviving transaction. | NI06 retains actual reader-induced BUSY and independent committed-state checks; lost reply is a distinct outcome. |
| P1: checking foreign_keys once or replacing psql with a fake hides a cutover. | NI07 requires a real FK refusal/valid write; NI08 completes real queue/store writes without new attempts. |
| P2: treating all non-target names as corruption rejects valid existing settings. | Reserved-name refusal is separate from a permitted unrelated-data control. |
| P2: case-varied duplicates/unsupported marker values can fail during test setup. | NI05 starts collisions without the target object and explicitly builds a corrupt marker; it must reach startup. |
| P2: restoration checks could require reusing an unclean/closed connection. | NI07 permits verified exclusion and tests a configured replacement, never fictitious cleanup success. |
| P2: a passing installer can be misreported as a finished feature. | NI08 and execution rules explicitly preserve native main-path, regression and coverage gates. |

Repeat review checked the corrected cases against each numbered design rule.
No remaining P0–P2 was found in this inventory. No product code or old test is
changed by this document. After approval, audit old tests for contradictions
and request separate permission for any necessary fixture/assertion change.

## Implementation evidence, 2026-11-07

The final artifact contains 282 catalog objects; the frozen v0.2.0-rc.1 baseline
contains 57. Only prepare_jobs and its two named managed admission guards change
among existing objects. Provenance records the original dirty source honestly,
its source-file hashes and fixed artifact hashes; CI needs no other checkout.
The source prototype was executed only during extraction, not during startup.

All NI groups pass on Windows and Linux UID 1001. The real COMMIT BUSY test first
exposed surviving uncommitted DDL on the same connection; startup now clears or
excludes that exact connection before reconciling. Lost committed replies and
unavailable readback are separate checks. Connection setup/restoration failures
are exercised both before and after their actual effects. No old test changed.

The complete Windows engine module passed with
`-count=1 -timeout=10m -p=1 -coverprofile=coverage/issue89-normal-store-clean-full.out`.
The separately repeated artifact test and vet passed after the provenance
assertions were strengthened. Linux tests are native test binaries, run from
their package directories with UID 1001, not compilation-only checks.
Final NI05 review also reproduced late inventory for an unmarked partial managed
namespace; the preflight now refuses every reserved namespace, including case
variants, before inventory. Both new controls pass on Windows and Linux.

Coverage is not qualified: managedstore is 86.2%, queue is 86.4% in the ordinary
full run. A second complete run of both packages with shared instrumentation
(`coverage/issue89-normal-store-combined.out`) records real cross-package calls:
managedstore has 23 uncovered statements of 167 (86.23%); queue has 40 of 574
(93.03%) after that preflight correction. These are unioned block counters, not an average of per-test percentages.
Neither exceeds 95%; neither proves production Liquibase execution.

## Approved bounded coverage iteration IC01–IC05

Status: approved by @evilguest on 2026-11-07; implemented and verified.
No additional recovery delivery or broad old queue coverage is authorized.

Uncovered-line reports are the two profiles above and their adjacent HTML files.
The shared profile includes the final unmarked-namespace correction.
Order by the largest new-file deficit, then startup and remaining verification:

| ID | Remaining code / requirement | Proposed tests and evidence |
| --- | --- | --- |
| IC01 | liquibase_main_path_schema.go: 27 uncovered statements, including 117–119, 172–190, 203–221, 318–327; NI01/NI05/NI07. | Direct installer/verifier controls for nil/closed transactions, cancellation, incomplete base prerequisites, altered namespace and an already exact target. Retain NI05's unmarked reserved-namespace controls. Require safe classification, no repair or writes on refusal, unchanged rows/catalog; exact current installation is idempotent. No mutation of embedded globals to manufacture invalid JSON. |
| IC02 | Same file, reads/writes at 145–147, 245–247, 259–309, 335–349; NI03/NI06/NI07. | Observe real SQLite refusals at unfinished-job read, trigger suspension, temporary table creation, old/new count reads, EXCEPT read, replacement creation/copy/drop, trigger restoration, marker insertion/read and FK query/stream. One fault per case, actual prior effects, precise hit count, no ready format, same-handle and independent rollback checks, then clean reopen. Do not falsify count values or mutate canonical definitions to reach invariant-only branches. |
| IC03 | main_path_startup.go: 21 uncovered statements, setup/restore and readback paths at 43–48, 80–82, 97–115, 129–141, 163–177; NI06/NI07. | Fail initial PRAGMA, second BeginTx, repeated preflight, current Runtime v2 marker read, restoration query and readback Begin/domain-read/rollback. Require original error/cancellation classification, zero ready format, preserved committed-versus-uncommitted outcome, and configured replacement if cleanliness cannot be proved. Use real effects and the existing per-DB driver observer, not a new product hook. |
| IC04 | Same file, cleanup paths at 57–70 and 151–153; NI06/NI07. | Extend the test-only observer with an ordered fault list for raw ROLLBACK and subsequent real BEGIN/ROLLBACK proof. Check a failed cleanup excludes the exact connection; successful proof permits reuse. Distinguish real committed data from cleanup status, and preserve the primary failure. No physical Close diagnostic/late-error matrix from #172. |
| IC05 | store.go: two uncovered scan/stream error statements at 48–55; NI01/NI06. | A transparent real-row observer returns a Rows.Next error during catalog verification, with valid control reaching that read, preserved original catalog/rows and successful next startup. Investigate whether cancellation can actually reach the Scan refusal; do not invent incompatible sqlite_master column values to force it. If unreachable under the owned query, record a removal candidate for separate approval. Repeat complete package tests and shared/ordinary coverage on Windows and Linux; if still below threshold, report the actual remaining blocks and request another iteration. |

Review: the plan tests the existing installation contract, not undocumented
implementation details. Invariant-only branches (including impossible repeated
quarantine or a non-ErrConnDone sql.Conn.Close error) are candidates for a
separate justified removal, not grounds to fake driver behavior. No removal is
included here. Product boundaries and old tests remain unchanged. The existing
queue's 13 other uncovered statements are not silently credited or deleted;
this iteration does not claim that its prospective gain will meet 95%.
Repeat review also corrected the original IC05 wording: a driver cannot directly
inject sql.Rows.Scan failure while transparently preserving the returned values.
The proposal now tests the reachable stream error and audits Scan reachability
separately. No remaining P0–P2 was found in the corrected bounded proposal.

### IC results and repeat review, 2026-11-07

The four new tests in main_path_installation_coverage_test.go implement all five
groups: DirectBoundaries (9 cases plus reference cancellation), DirectCatalogue
(7 cases, each exercising install and verify), ObservedRefusals (25 cases,
including IC05's real stream refusal), and CleanupProof (6 cases). The ordered
observer is per test database, preserves real SQL values/effects and leaves the
original NI fault mode unchanged. No old test assertion/fixture was changed.

IC01 first reproduced a panic in VerifyLiquibaseMainPathSchema with a nil
transaction. The only product change is a three-line refusal guard. No API,
schema, recovery authority, queue executor or physical cleanup rule changed.

Complete managedstore/queue suites pass on Windows and Linux UID 1001, as do
the final strengthened IC assertions on both platforms. The complete Windows
engine module and vet pass. The ordinary full profile is
coverage/issue89-ic-windows-full.out. Linux runs execute the real native test
binaries, not only cross-compilation; no Docker/izess is needed by these tests.

| Package | Before IC | After IC, Windows | After IC, Linux UID 1001 |
| --- | --- | --- | --- |
| managedstore | 144/167 = 86.23% | 161/167 = 96.41% | 161/167 = 96.41% |
| prepare/queue | 534/574 = 93.03% | 560/576 = 97.22% | 560/576 = 97.22% |

Both packages meet the accepted 95% minimum; 100% is not claimed. These are
complete-package block unions with real cross-package calls, not averages of
per-test percentages or the percentage of a selected helper. The two extra
queue statements are the new nil guard. Ordinary queue-only coverage remains
86.3% because its startup checks execute in managedstore's suite; the ordinary
engine module and other packages do not inherit the two union percentages.

Windows sources: coverage/issue89-ic-windows-combined.out; Linux sources:
coverage/issue89-ic-linux-managedstore.out and issue89-ic-linux-queue.out.
Deduplicate identical file/start/end/statement blocks, taking the set union of
positive counters within each platform. Normalized union profiles and per-line
HTML reports are coverage/issue89-ic-{windows,linux}-union.{out,html}.

Remaining blocks, retained rather than hidden or deleted:

- main_path_startup.go: 5 statements, ranges 29–31, 46–48, 73–75, 137–139,
  208–210; guards/invariants and cancellation timing. Repeated quarantine and
  sql.Conn.Close's extra error branch remain removal candidates only, not an
  approved removal or an excuse to fabricate driver behavior.
- managedstore/store.go: 1 statement, 48–50. Go 1.25.5's Rows.scanLocked checks
  cancellation-driven closure between Next and Scan, so this error path is not
  proved dead. It remains protected; IC05 tests the deterministic stream error
  without substituting impossible catalog values or adding a product seam.
- liquibase_main_path_schema.go: 3 statements, 57–59, 219–221, 283–285:
  malformed embedded metadata, an object differing after earlier exact checks,
  and a copied-row comparison invariant. Frozen artifacts are not corrupted by
  tests merely to force those branches.
- Existing queue code: 13 statements (managed_schema.go:22–24 and
  sqlite_store.go:192–194, 200–202, 233–235, 243–245, 268–270, 300–302, 317–319,
  411–413, 433–435, 458–460, 477–479, 497–499). No claims of new qualification
  or deletion of these older paths are made.

Repeat review strengthened exact-connection exclusion, zero mutation attempts
on preflight failures, and independent primary/rollback/cleanup error identity.
Valid controls, same-handle checks before Close, independent committed visibility
and two physical reopens pass. No unresolved P0–P2 was found in this iteration.
The native production prepare/cache/cleanup and performance gates of MP05–MP07
remain open; IC completion does not close #89 or deferred issues #170–#174.
