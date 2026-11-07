# #89: normal-path installation test inventory

Status: proposed NI01–NI08; not approved or implemented.
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
  prepare_liquibase_session_store_format=2,
  prepare_liquibase_finalization_store_format=2,
  prepare_liquibase_cache_selection_store_format=1; exactly one slot=1 row each.
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
process; 95% is not waived. Native MP checks use their own explicit conditions,
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
