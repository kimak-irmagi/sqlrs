# #89: reduce the overhead established by the complete timeline

Status: components approved by @evilguest on 2026-11-12 after critical/repeat
review: “Утвердить проект компонентов”. Storage changes and the final new/
changed-test inventory have received the separate approvals recorded below.
O6 implementation follows its separately approved
[test inventory](issue89-inventory-reuse-tests.md). O5
[schema](issue89-planning-db-handoff-schema.md) is approved; its
[final tests](issue89-planning-db-handoff-tests.md) and limited old-test changes
are approved on 2026-11-13. O7's
[wire/quota and test inventory](issue89-same-exec-result-tests.md), including
limited OT-IR01–OT-IR08, are approved. O6/O7 are implemented and undergoing
qualification. O5 DDL, journal writers and coordinator transfer are implemented;
new O5 files meet the local coverage minimum. [Native qualification on
`28cfc25`](https://github.com/kimak-irmagi/sqlrs/actions/runs/43566256871) passes,
including real planning/working/publication DB identity and physical allocation
counts, full/prefix cache, known failure, uncertain transfer and exact cleanup.
The user [accepted the measured overhead for #89 and authorized merge](../adr/2026-11-14-issue89-performance-acceptance-and-merge.md).
Full P01b qualification is not claimed and is no longer a merge prerequisite
for this delivery.

## 1. Evidence, outcome and boundaries

### Combined O5–O7 result (2026-11-14)

[CT13 run 43566888408](https://github.com/kimak-irmagi/sqlrs/actions/runs/43566888408)
completed on `28cfc25a6f32d3d1ff8a9398004bcd898043cd59`, engine SHA256
`ed3a8436cb7b2e110d393b3a705122c3ac2870bd7b0130150dcc100141fc5ade`.
All eight serial members / sixteen JHipster passes passed data and cleanup
oracles. Each route has two repetitions on the same hosted VM. Means:

| Route | Fresh, ms | Fully cached, ms |
| --- | ---: | ---: |
| Actual v1 | 48710.695 | 8450.713 |
| Old v2 (`668d505`) | 57614.793 | 10120.467 |
| Current v2, recording off | 51511.594 | 9113.265 |
| Current v2, recording on | 51630.634 | 9160.053 |

Ordinary current v2 saves **10.59% fresh / 9.95% cached** against the fixed old
v2, but remains **5.75% / 7.84% slower than actual v1**. The no-regression speed
criterion remains unmet; the user explicitly accepts this overhead for #89.
CT13 is a diagnostic comparison, not the full P01b
acceptance campaign or a noise bound. Different historical runs are not pooled;
the old-v2 comparison does not isolate O5–O7 from the earlier optimizations.
Recording changes these means by +0.23% / +0.51%.

Both recorded fresh passes have one DB startup/transfer, no planning DB disposal,
one cold inventory probe and eight same-exec result decodes with no separate
capture reads. Cached passes retain two DB startups, dispose the preview once,
perform no transfer or inventory probe, and execute no migration steps. Total
collector container-create counts are current v2 7/3 versus old v2 8/4 and v1
4/2 (fresh/cached); these include helper containers and are not DB-start counts.

The [retained ZIP](issue89-cost-evidence-2026-11-14/43566888408/jhipster-cost.zip)
preserves the exact original JSON, SHA256
`b383b6110360f8a1f6e7279edade54dd6c030aba7cef5575757e7c10252fc313`.
The benchmark uses the unchanged released-v1 and old-v2 pins, original API timer,
eight changesets, eight subsequent exact cache hits and independent cleanup.

### Reference before O5–O7

[CT13 run 43061874236](https://github.com/kimak-irmagi/sqlrs/actions/runs/43061874236)
on `d20848cce056910f18520558a614dea950b9eade` completed eight members / sixteen
JHipster passes with passing data/cleanup oracles. Means in milliseconds:

| Route | Fresh | Fully cached |
| --- | ---: | ---: |
| Actual v1 | 49730.778 | 8674.156 |
| Old v2 | 59677.655 | 10452.473 |
| Current v2, recording off | 57186.118 | 10249.719 |
| Current v2, recording on | 56837.705 | 10448.323 |

The [retained ZIP](issue89-cost-evidence-2026-11-12/43061874236/jhipster-cost.zip)
contains the exact original `jhipster-cost.json`, SHA256
`58a97a7d5414e52aeec98990f2b11456dcbff23173015286fa1edc465d17fea0`.
Compression avoids adding 9.2 MB / 246k repetitive JSON lines to the PR; original
bytes and every detailed timing interval are preserved and hash-verified.

That revision's ordinary route was +14.99% / +18.16% slower than v1. These two
diagnostic repetitions are not the full acceptance campaign or a noise bound.
Recording changes observed means by -0.61% / +1.94%; do not infer that recording
accelerates fresh execution. Enabled spans identify costs, not exact ordinary-
route deltas. All four timelines independently conserve their sampled window;
adding the separately measured 591–720 ns clock difference reconciles the
unchanged API elapsed. Unresolved self time averages 376.850 ms fresh and
103.589 ms cached; it is not silently allocated to a guessed cause.

Selected enabled fresh costs: second DB startup 2628.053 ms; image inventory
973.276 ms; eight result reads 1002.434 ms; invocation journal 66.620 ms;
seven after-resume access checks 177.635 ms. The last two are **not** removed
here. Neither are per-snapshot DB stops/resumes, stock CLI/JVM starts, state
verification, publication pins, manifest hashes or retained-output limits.

CLI/API, psql, search rules and SQL-retry permissions do not change. #170–#174
remain separate. The original speed test is unchanged; the #89 delivery exception
is recorded in the acceptance ADR above. No laptop workload is permitted.

## 2. O5: transfer the planning DB into the first working session

### Eligibility and sequence

The normal `prepare:lb` worker keeps the exact planning DB assigned while it
reads and validates the successful planner capture and saves the plan. Planning
must not pretend the live DB is absent or set its clone to `released`.

The coordinator builds the actual suffix and pins the first executable task
before selecting reuse. Reuse requires all of: ordinary prepare, non-empty
uncached suffix, exact first-task input equal to the preview's source, matching
job/generation/execution fingerprint and managed identity, completed successful
stock `update-sql` planner, fully verified saved plan/capture, no application
attempt, no accepted cancellation/selected failure and no prior working session.
Do not infer eligibility merely from a mutable `cached` flag or an in-memory DB.

Queue then atomically records transfer to one exact planned working session,
its first binding, helper/config identity and the existing clone/runtime/path/
managed operation. Preserve all physical IDs and immutable operation Source;
do not rename the container, move a live directory, allocate another clone,
reinitialise identity or create a second DB. The planning history still points
to its original clone; an explicit durable link distinguishes historical
reference from the current destructive owner. It is not two independent owners.

Only after known commit/full readback may prepare construct its working object
from that exact runtime and launch the ordinary reusable Liquibase runner.
The existing working-to-instance handoff remains unchanged after the last step.
Private planner executor/output/input are disposed separately under the saved
plan/transfer basis; a retained DB is proved owned, not proved absent.

An incompatible input (including a cached prefix), full cache, zero changes,
plan-only or cache explanation follows the old physical cleanup/new-source
route. No implicit reuse of a DB with a different cursor. Cancellation or known
failure before transfer uses the planning-owner cleanup; after transfer it uses
the session owner. Neither route may delete through an old historical reference.

### Modules, storage and failures

`prepare/liquibase_normal_worker.go` will carry a private planning outcome with
verified plan and optional exact runtime handle; that handle is not authority.
`prepare/queue` owns new narrow retained-plan/transfer operations and full
readback on the exact connection. `instanceaccess`, runtime and statefs retain
their existing identity and physical-operation responsibilities. No Docker or
filesystem operation is permitted inside the transfer transaction or eviction
lock. Shared cleanup must recognise the same persisted ownership.

This **requires a separately approved storage amendment**. Current
`CompletePlanning`, first-session proof and planning-disposal guards require
real DB absence/released preview. The amendment must distinguish saved plan,
retained planning DB and transferred DB without redefining absence or rewriting
historical evidence. It must cover installation/version checks, exact FK/guards,
source/path pins and job/instance/state deletion. Exact tables/columns/versions
are specified by the separately approved
[O5 storage amendment](issue89-planning-db-handoff-schema.md), now implemented.

Before transfer commit the DB remains owned by planning; afterwards by the
session. An ambiguous commit or failed observation never authorises another
session/SQL launch or destructive fallback. Keep ownership and
`recovery_required`. Process restart recovery remains #170; this amendment
must already leave an unambiguous durable owner at every crash boundary.

## 3. O6: bounded process-local inventory cache, not skipped compatibility

`PrepareService` owns a private cache, initialised with the service, not a global
hook or a client dependency. Cache key: exact resolved platform manifest digest,
OS/architecture/variant and internal inventory-rule revision. Mutable tags are
resolved as before; availability is ensured as before. Validate the exact
binding before a lookup. Tag/platform changes cannot reuse an unrelated result.

On a miss, call the existing real `InspectRunnerImage` and current
`driverbundle.Select`. Only successful, validated immutable inventory is cached;
no failure, cancellation, credentials, job result, mounted artifact lease or
filesystem path is retained. Keep embedded-vs-mounted selection and the actual
JAR digest in the unchanged execution fingerprint. Publish/materialise/check
the selected driver for each job using the existing artifact protocol.

Ready entries use a 32-entry LRU; callers receive deep copies. At most 32 keyed
in-flight records may be retained. Same-key waiters may coalesce, but waits are
cancellable and no lock spans Docker. A failed/cancelled producer publishes no
entry and wakes waiters with failure; it does not automatically retry. A later
independent request may perform the read-only probe again. If all slots are
occupied by producers, a new key takes the existing uncached path rather than
growing the cache, blocking unrelated keys or inventing a new product error.

First use after **every engine restart** still pays the actual probe. No disk
cache, new SQLite component, supported-image allowlist or benchmark prewarm is
introduced. This saves repeated prepare/full-cache costs within a process, not
the cold first-prepare second. Record that limit explicitly in the new results.
Choices rejected: deleting compatibility checks/always overriding embedded JDBC;
shipping digest allowlists; or persistent inventory storage in this small step.

Observe the actual lookup/wait in a new `runner.inventory_lookup` scope. Emit
`runner.inventory` only around an actual miss probe, not as an empty cache-hit
event. Update campaign/native expectations to require real lookups and the
correct cold-miss/subsequent-hit counts while retaining binding/data checks.

## 4. O7: return the durable result through the already-running exec

Add an explicitly selected internal result-transport mode to the pinned step
helper, not a live-JVM integration. After the existing durable capture/manifest
flush, the **same** exec returns a bounded binary result envelope containing the
three exact completed files, closed optional timing detail and a final transport
completion/idle/prune observation. The helper still launches one stock CLI per
step and retains the same private files/two-slot ownership until authorised
disposal. No host capture path or streaming raw Liquibase log is introduced.

`runtime` uses the existing bounded binary runner; child stdout/stderr belong
only to the envelope. Diagnostic stderr remains separately bounded/discarded.
The envelope has a separate fixed transport version, explicit finite lengths,
fixed members and no paths supplied by the child. Existing manifest identity,
EOF, stream limits/hashes and completion verification are reused, not replaced
by a zero Docker exit code. The exact wire layout/quotas form part of the final
test inventory before code. Helper bytes/digest change; manifest protocol and
DB fields need not change for this transport-only part.

Before accepting the envelope, runtime requires the exact pre-invocation parent
and unchanged post-invocation full ID/name/running state. Inside the pinned root
helper, actual no-UID-1001-child checks surround the result/previous-path absence
observations. Prior deletion still requires the saved exact prune permission.
Only runtime may seal the existing typed prune receipt after all checks. The
helper's own root process is not a surviving Liquibase child.

Transport success is separate from child success: a complete valid result with
child code 7 remains a known Liquibase failure, not an unknown command or success.
In selected envelope mode the helper exits zero only after successful transport;
the original child code is read exclusively from the verified manifest. Legacy
helper invocation keeps its original exit semantics. The invocation retains its
existing workload context/deadline; do not apply the old 30-second result-read
timeout to the whole potentially long-running CLI. Byte caps and existing bounded
command-drain cleanup still apply. Caller cancellation remains cancellation even
if complete private files happen to exist; it is not a successful response.
A missing/partial/oversized/malformed envelope, cancellation, unconfirmed helper
exit, live child or changed parent cannot qualify or retire a slot. Never rerun
SQL. Retain the durable capture for reconciliation; do not automatically retry
the CLI or make an error green through an unapproved fallback.

The independent `ReadCompleteLiquibaseStepOutput` and stopped-parent paths remain
real archive/identity/idle verifiers for explicit read-only reconciliation;
they do not start stopped containers. Normal successful invocation no longer
calls extra `docker cp`/idle exec/pre-read inspect after its result arrived in
the same exec. Its initial and final parent checks remain. If a required bounded
transport capability is absent, reject before invoking SQL, not after it.

Cost scopes must follow the new real boundaries: transport is nested in the
invocation; decode/verification has its own scope; old `capture.read` remains
for independent reads. Do not keep an empty scope just to satisfy old counts.
Report helper/envelope encoding cost and included transport time without adding
inclusive parents twice. Changed helper bytes require fresh pinned fingerprints;
old evidence is not reinterpreted and no historical migration is enabled.

## 5. Delivery, tests and measurement

Order: approve these components; separately review/approve O5 storage; then
review/approve the new tests and exact existing-test adjustments. O6/O7 can be
tested/implemented independently while O5 storage is being settled. Write new
tests first. Preserve behavioral refusals and native cleanup/ID assertions;
only obsolete command counts, setup, scope counts and the reviewed ownership
scenario may change. User authority to correct coupled tests is not permission
to remove safety assertions or fake successful runtime evidence.

Components, [O5 schema](issue89-planning-db-handoff-schema.md),
[O6 tests](issue89-inventory-reuse-tests.md) and
[O7 wire/tests](issue89-same-exec-result-tests.md) are approved. O5's separate
tests-first implementation is undergoing local validation. O6/O7 unit/observer evidence is not a replacement
for hosted native execution or the retained performance criterion.

Measure changed helpers per-line (target 100%, minimum 95%) under the existing
#89 aggregate exception/#174. Run ordinary/native regressions, then unchanged
CT13 in Actions: actual v1/old-v2/new-off/new-on, both serial repetitions and
fresh/cached passes, same inputs/timer/oracles, complete timeline and off/on
observer control. Do not prepopulate the new inventory cache. Record cold miss
and subsequent hit explicitly; adapting counters is not altering the workload.
Full speed qualification remains distinct from diagnostic results; it is no
longer a #89 merge prerequisite under the 2026-11-14 acceptance ADR.

## 6. Critical review and repeat review

Findings corrected before presentation:

- A live preview cannot satisfy today's absence proof: O5 explicitly requires
  a storage amendment and a retained owner, not weakened existing guards.
- Reusing the original source after a cached prefix could reapply SQL: eligibility
  uses the actual first task and saved binding/cursor, not just a cached count.
- A cache would hide cold-start cost if benchmark-warmed: O6 is process-local,
  cold probing remains measured, and no timing/prewarm protocol is changed.
- Cache hits could bypass driver provenance or leak mutations: validate/select,
  exact digest/platform/rules, per-job artifact handling and deep copies remain.
- A binary command error could erase a known child failure: O7 separates complete
  transport from manifest child outcome and preserves durable reconciliation.
- Returning bytes alone could fake idle/prune proof or expose secrets: explicit
  native observations, pre/post parent checks, private quotas and typed receipts
  remain mandatory; partial data and SQL retries are not accepted fallbacks.
- Old scope counters could falsely insist removed work still occurs: document
  new boundaries and review affected tests rather than emit fictional events.

Repeat review checked eligibility branches, outcome races, owner/path lifetime,
exact COMMIT uncertainty, cache cancellation/capacity, bounded transport, helper
timing/privacy, nonzero children and unchanged acceptance. No remaining P0–P2
finding at this component level. Exact schema and wire/test definitions remain
explicit next approval boundaries, not claimed implemented or tested here.
