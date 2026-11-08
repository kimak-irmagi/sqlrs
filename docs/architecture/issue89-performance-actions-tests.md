# #89: tests for off-laptop performance collection

Status: PA01–PA12 approved by @evilguest on 2026-11-08 after critical and repeat
review. Existing fetch/JDBC/release-workflow tests were audited: their separate
paths and smoke checks do not conflict; no old-test changes are authorized or
needed for the new performance-owned collector and full oracles.
Scope: the approved [Actions design](issue89-performance-actions-design.md),
not new product behavior or the recovery/coverage programs #170–#174.

## Boundaries and evidence

Use fast deterministic tests for preparation, scheduling, evidence validation
and workflow guards; they do not qualify engine behavior or speed. Use actual
external Linux engine artifacts and Docker only on the Actions VM for pilot
and measured series. No further laptop performance collection is planned.

The new collector lives in `test/performance/liquibase`; preparation/report
helpers and their Node tests live in `scripts/performance`. Workflow tests parse
the YAML and check executable guard behavior with representative event data,
not just search for suggestive text. Collector tests use private temporary
fixtures and subprocesses, not new injection hooks in product packages.

The delivery branch lacks the public MobilityData fixture acquisition/oracles,
and its JHipster release query is only a smoke check. Add performance-owned
copies of the full independently established public oracles with source/license
provenance. Do not replace the old release smoke tests or depend on files in the
prototype worktree. Fixture pins: JHipster `6b000b5d23a36c45e01472471b84a44fa2464044`
(8 changes, Liquibase 5.0.3, prod); MobilityData
`ed866e07b9af277ae78305d17e87ea34f4e6674b` (user: 12; main: 102; Liquibase 5.0.4).
Both sources are public Apache-2.0 projects; izess is not an input.

## Final proposed inventory

Each row is a bounded test group; negative cases must reach the named boundary
and assert that no later measured work/success report occurred.

| ID | Level and cases | Required observation |
| --- | --- | --- |
| PA01 | Deterministic workflow/guard tests: explicit same-repository PR label and manual dispatch; wrong label, ordinary push/PR update, fork, malformed ref/SHA; candidate preparation and measured job configuration. | Only opted-in allowed events launch collection. Resolve/build the exact selected head SHA, never interpolate event text as shell commands or substitute a merge ref. Read-only permissions, no secrets/persisted checkout credentials, standard Ubuntu 24.04, no paid/self-hosted target. Measurement jobs do not cancel a running cohort on a routine push; one active collector per VM. |
| PA02 | Preparation tests: pinned RC7 archive/source and extracted build metadata; candidate clean revision/platform/hash; missing file, wrong checksum/revision, dirty or wrong-platform build, unsafe archive entry. | Verify before executing an artifact; reject each mismatch with no fallback build of v1. All series use the same declared candidate artifact, not rebuilt moving heads. Corrupt downloads/extraction do not execute or overwrite outside the owned directory. |
| PA03 | Preparation tests: locked public source acquisition, full oracle presence, JDBC installation in both routes, pinned LB/DB images and stock launchers/core; missing/changed source or dependency. | Full source commits and original LF bytes, exactly 8/12/102 expected changelog entries, independently stored schema/identity expectations. Fetch/verify/warm before timing. Verify the same project JDBC bytes for both engines, matching Liquibase versions/core; retain actual host/container Java and launcher identities without pretending they are identical. No private checkout/network requirement. Existing project JDBC resolver is reused, not bypassed. |
| PA04 | Collector tests plus actual Linux pilot: native executable/launcher paths, sanitized child environment, actual route-specific API payloads, exact loopback engine PID/endpoint; startup/HTTP/terminal-status failures and cancellation. | RC7 uses its supported Linux host launcher and the candidate its container contract. Read back supported copy-backend settings for both; no host PG/state configuration leaks into private stores. Failed or mismatched startup/unsupported config ends the run before a timed sample can qualify. Polling stops on failed/recovery-required; no unbounded wait or diagnostic body leakage. |
| PA05 | Collector/evidence tests: uncached/cached workload, oracle mismatch, wrong order/checksum/schema or missing cache proof; real pilot and series for all fixtures. | Timed interval is monotonic API submission through successful terminal result, equally defined for both engines; readiness, acquisition and oracle queries stay outside. Fresh store per engine run; cached follow-up uses that same store. Query the actual result DB and require full ordered changelog and schema oracle, plus 0 versus 8/12/102 actual state-cache hits. A successful HTTP status or task count alone cannot qualify. Mutating a changelog identity/checksum or schema expectation must independently fail. |
| PA06 | Docker/count tests: event stream start/drain, invalid/truncated/error stream, unrelated creates, inspection versus migration commands; actual pilot observation. | Count all owned creates in the measurement interval, including short-lived helpers, without wrappers on measured commands. Do not silently lose late-delivered events or assume a missing probe is zero. Record observed migration/inspection CLI separately; JVM totals, if derived from pinned launcher bytes, are explicitly labelled derived. Missing/ambiguous observation makes the sample incomplete, not faster. No fixed Windows container total is imposed on Linux. |
| PA07 | Scheduler/completeness tests with synthetic samples: alternating order, paired controls, duplicate/reused/missing pair, wrong fixture/pass/host/artifact, incomplete confirmation; timers and serial execution. | At least five distinct alternating v1/v2 pairs per fixture and uncached/cached pass, same-sized v1/v1 controls, all on the cohort's one VM. Independent confirmation uses a separate fresh VM and its own local controls with the same artifacts/settings. No cross-VM or Windows/Linux pooling, silent retries replacing a slow valid sample, parallel measurement or counting a pilot as a pair. Complete acquisition is not a speed pass; existing P01b evaluation remains separate. |
| PA08 | Budget/deadline tests and actual pilot: preparation/runtime/oracle/cleanup costs, insufficient remaining time, near limit, timeout/cancellation. | First establish the small route, then measure both routes for the 102-change pilot to estimate the largest full cohort plus controls. Include untimed overhead and an explicit cleanup/upload reserve in the six-hour budget. Refuse an infeasible cohort, retain an incomplete report and request a new decision; do not truncate pair count, scatter the cohort or buy capacity. A process deadline precedes the job hard limit, leaving cleanup time. A prediction is not proof of completion. |
| PA09 | Cleanup tests: normal result, engine failure, collector error, deadline/cancellation, cleanup error; resources with similar names/outside the owned root. | Gracefully stop then boundedly terminate/reap only owned processes; remove only exact validated job-owned Docker resources and private files. Never prune Docker globally or delete by broad name prefix alone. Check absence before test cleanup can hide a leak. Failure of cleanup makes collection unsuccessful; retain only safe diagnostics. Hard VM termination cannot guarantee finally blocks: partial evidence never qualifies. |
| PA10 | Report/upload tests: successful and failed runs, nested unknown fields and secret/DSN/SQL/raw-log sentinels; incomplete series, timeout, fixture/oracle mismatch. | Allowlist typed report fields and upload only the dedicated sanitized report directory, never the working tree/store/log glob. No captured SQL, credentials, engine.json or DB bytes in artifacts, Actions summaries or console diagnostics, including failures. Persist run/VM/phase identity, artifact/source/dependency/image/backend provenance, samples/spread/counts, oracle and cleanup outcomes. Invalid or incomplete data cannot become complete collection or automatic speed acceptance. |
| PA11 | Real explicitly launched remote pilot, before full scheduling. | Run actual published Linux RC7 and exact candidate, first JHipster then both MobilityData routes including all 102 changes, uncached and cached, with independent full DB checks, event observation and verified cleanup. Record executable compatibility, source/driver/image identities, actual snapshot backend and total costs. Green synthetic checks or the pilot do not close the performance criterion. If the six-hour feasibility check fails, stop before full cohorts and report the blocker. |
| PA12 | Real remote protocol and regressions after pilot feasibility. | For all three fixtures collect the retained five-pair alternating comparisons and same-sized controls on one VM per cohort; independent confirmation has its own VM and controls. Preserve uncached/cached results separately; evaluate the existing P01b rule only after validating completeness/provenance. Run new fast tests with existing relevant fetch/JDBC/workflow checks and normal PR CI; do not weaken existing tests/coverage gates. Incomplete, noisy/inconclusive or confirmed-regression outcomes do not close #89 or permit merge. |

## Retained acceptance rule and execution order

P01b remains the user-approved actual-release comparison: record elapsed time,
spread, CLI/derived-JVM starts, container creates, platform, fixed source and
image/helper/driver identities. Preserve its predeclared v1/v1 noise envelope
and paired-median v2-minus-v1 evaluation. A positive excess recurring in an
independent confirmation blocks release; a positive delta within noise is
inconclusive, not a pass. Missing measurements/baseline remain unresolved.
This test inventory does not introduce a replacement statistical rule/calculator.
Separate confirmation controls cannot be borrowed from the initial VM.

After approval: audit existing tests for contradictions; write the new fast
tests, implement only the approved transport/collector, run those checks, then
launch the real pilot remotely. Only a feasible pilot authorizes proceeding to
the already retained full collection. No new product/API/storage work is
included. Measure coverage of new helpers/collector separately (target 100%,
minimum 95%); the explicit
#89 exception defers general engine coverage to #174, not functional or timing
acceptance. Further coverage iterations still follow AGENTS.md.

## Critical review, corrections and repeat review

1. P1: copying the Windows collector would not exercise Linux. PA02/PA04/PA11
   require the real Linux archive and stock launcher, exact endpoint and supported
   settings before measurement; no product compatibility shim is authorized.
2. P1: the delivery checkout lacks the full oracles. PA03/PA05 explicitly add
   public, pinned performance-owned data; forbid current smoke checks or
   candidate-generated expected values as substitutes.
3. P1: a workflow could collect invalid/mixed or partial cohorts and appear
   successful. PA07/PA10 reject wrong identities, reused/missing pairs and
   incomplete controls; collection and the retained acceptance rule stay separate.
4. P1: a small pilot could underestimate the largest case or omit overhead.
   PA08/PA11 include the actual 102-change case, full run costs and cleanup
   reserve; no guaranteed fit or hidden truncation is claimed.
5. P1: failure diagnostics/cleanup could disclose private data or destroy other
   resources. PA09/PA10 test failure paths, exact ownership and secret sentinels;
   cleanup is checked before test-framework disposal and raw logs never upload.
6. P2: stream latency and old platform counts could create falsely cheap samples.
   PA06 requires drained valid events, explicit derived counts and Linux-native
   observations, not blindly copied Windows totals.

Repeat review checked opt-in bootstrap, immutable artifacts across VMs, distinct
control/confirmation identity, no silent sample replacement, privacy on failure,
and hard-timeout limits. No further P0–P2 issue found in this bounded inventory.
Compatibility, budget and performance are still unproven; no tests have been
implemented or executed by this document.

## First implementation checkpoint

The approved inventory is now being implemented. Nine new deterministic Node
tests and the unchanged fetch/JDBC/release-workflow checks pass locally. The
three exercised pure modules (`control`, `inputs`, `collector-output`) have
100% line and 96.48% branch coverage; this is **not** aggregate harness coverage.
The standalone Linux Go collector cross-compiles and passes `go vet`; its actual
unit/native execution belongs to the remote setup/pilot and is not claimed here.
The module uses `GOWORK=off` because it is deliberately outside the product
workspace, without changing Go cache paths. No product files or old tests changed.

Candidate builds use fixed source/Go 1.25.5 and trimmed paths on each VM; every
subsequent VM must match the initial candidate hash. Source/header/core/driver
checks and Docker event barriers precede sample acceptance. Reports remain
incomplete until child exit, full DB oracle and exact cleanup all succeed.
PA11/PA12, six-hour feasibility and complete adapter/collector coverage remain
open until real execution; no acceptance follows from this checkpoint.

Actual Ubuntu fast Node/Go tests and `go vet` now also pass in the new ordinary
CI job. The preparation VM verified published Linux RC7 and the second VM's
candidate hash matched the first VM. However, runs 42470634262, 42470856975,
42471396560 and 42471864295 all stopped **before measurement**: respectively
Temurin provider selector, JDK HTTP 504, release download, and exhausted release
transport. The selector now uses the provider's full name for the same Java
build; acquisition has bounded retries/verified same-URL system fallback. No
sample, actual-engine workload compatibility or full budget is established by
these runs. Run 42472235367 retries preparation on source 16ee722; native
qualification remains open. The later helper additions retain 100% line coverage
of the four exercised Node modules, not the complete process adapters/Go harness.

Run 42472235367 completed both preparation stages and entered the actual RC7
JHipster uncached workload, which failed before a verified result. The sanitized
artifact has zero records and `failureStage=uncached`, so no sample is accepted.
Fixed failure classes are now added to distinguish known input/driver/runtime
operations without retaining raw engine messages/details/SQL. This improves
diagnosis only; it does not relax either artifact or workload validation.

Run 42473269289 reproduced the uncached refusal without a known engine class.
Reading actual RC7 `prepareRequest` showed a native-path port defect: it resolves
paths from process `os.Getwd()`, not request `work_dir`; Windows batch handling
had hidden that distinction. PA04 now tests/sets the released process and host
CLI to the fixture's project directory with `searchPath=.` and native logical
changelog names. Source bytes, full DB oracle and released binary stay unchanged.
HTTP rejection status is separately allowlisted for diagnosis. No benchmark
compatibility or timing pass is inferred until a new real run succeeds.

Run 42474455740 narrowed the remaining boundary to `private_cleanup`, with no
reported oracle mismatch dimensions. PA09 now checks nonce-marked private-root
disposal after exact owner-container absence: a pinned Docker helper removes
container-owned files without relaxing permissions, following symlinks or
elevating either measured engine. The helper's full ID is retired and checked;
failed cleanup cannot emit samples. Container/file cleanup failures are distinct
safe enums. This is test teardown, not a change to product deletion/recovery.

## Successful actual Ubuntu pilot

Run [42475910841](https://github.com/kimak-irmagi/sqlrs/actions/runs/42475910841)
on source `3d3e8e6` completed both real engines for all three public fixtures,
including 102 changes. Every uncached/cached DB oracle, actual cache-hit count
and exact container/private-store cleanup passed. The full sanitized
[pilot evidence](issue89-actions-pilot-2026-11-08.json) is retained; it is not
an acceptance cohort and will not be pooled into one.

| Fixture | v1 uncached / cached | v2 uncached / cached | Estimated complete cohort with controls/reserve |
| --- | --- | --- | --- |
| JHipster (8) | 40.422 / 7.959 s | 52.914 / 9.219 s | 43.77 min |
| MobilityData user (12) | 55.988 / 7.992 s | 67.897 / 9.118 s | 50.32 min |
| MobilityData main (102) | 535.886 / 16.615 s | 581.679 / 10.621 s | 257.66 min |

Budgets include preparation, measured and untimed member costs, 20% contingency
and the 15-minute reserve. All fit the six-hour bound; this remains a prediction.
The mixed timing differences are not a speed verdict. PA12 therefore proceeds
to the retained alternating comparisons/controls and independent confirmation.
All ordinary CI tests also pass on this source; only the unchanged aggregate
coverage check is red under the explicit #89 exception.

Before full collection, PA09 adds the explicit outside-root symlink control and
unit faults for cancellation, helper create/start/removal/absence and residual
files. These strengthen teardown only; measured product execution is unchanged.

## HC01–HC07: targeted ordinary Go helper coverage

Status: proposed after critical and repeat review; approval requested before
new tests or the approved file split are implemented. One bounded iteration,
not a product change or another native performance run. The boundary was approved
on 2026-11-08; PA01–PA12 remain unchanged.

Baseline: helpers-only Linux profile on `668d505`,
`.tmp/performance/unit-current.out`: 135/557 statements (24.2%), not native or
product coverage. Descending uncovered count: `collector_linux.go` 398/410,
`cleanup.go` 21/53, `contract.go` 2/60, `private_root.go` 1/34. Most of the first
file is the integration test, not dead code. Report denominator changes separately
from newly exercised requirements; do not promise a resulting percentage.

1. **HC01 — event integrity (PA06/PA10).** Keep scanner, decoding, event storage,
   stream errors and counting in ordinary Go helpers. Move only `testing.T`
   scenario/process/assertion orchestration to Linux `_test.go`. Test valid create
   events, malformed/truncated JSON, wrong action, missing image/name, nonpositive
   time, scanner read failure and oversized record. Verify exact decoded events;
   a broken stream cannot produce a valid count. Use in-memory/failing readers,
   not Docker.
2. **HC02 — exact counts (PA06).** Exercise the time.Time wrapper and nanosecond
   path, both inclusive endpoints and adjacent outside events; mixed images and
   durable sqlrs names distinguish all creates from unnamed pinned-image probes.
   Preserve existing interval and stream-error tests unchanged.
3. **HC03 — command adapter (PA09/PA10).** Use a temporary fake `docker` executable
   through test-scoped PATH. Check exact arguments/output on success, nonzero exit
   and already-cancelled context. Failure returns no partial output or private
   diagnostic sentinel. Restore PATH; no parallel environment mutation/real daemon.
   This does not replace native Docker qualification.
4. **HC04 — exact cleanup failures (PA09).** Test list failure, 64-character nonhex
   ID, malformed/empty/multiple inspect records, mismatched ID and owned mount with
   unrecognized name. Inspect failure is resolved only by successful exact-ID
   empty listing; still-present/list-error cases reject. Test removal failure and
   failed post-removal absence query. Require exact calls/order/IDs, no removal
   without proved ownership, continued handling of another valid entry and overall
   failure. Preserve foreign-root/name-prefix rejection and positive cleanup.
   All effects use the existing command double.
5. **HC05 — oracle shape (PA05/PA10).** Short/long changelog rows and wrong expected
   row shape report only `row_shape`, never values/private sentinel. Preserve
   matching/schema/count/dimension controls. This diagnostic test does not replace
   the independent full database oracle.
6. **HC06 — owner marker (PA09).** Missing, directory and symlink marker reject
   before Docker; exact root/foreign target contents remain unchanged. Preserve
   existing token/root/link, cancellation and helper retirement controls. No
   permission relaxation or actual private-store deletion.
7. **HC07 — boundary and full remeasurement.** Retain `TestReleasedEngine` and all
   native assertions; compile the external Linux binary with `GOWORK=off`, run all
   small Go tests/vet and Node helper tests, obtain the detailed ordinary-helper
   line profile. Report assertion moves and newly tested branches separately.
   Target 100%, minimum 95% for ordinary helpers; further shortfalls require
   another approved iteration. Do not cancel/restart the immutable running
   `668d505` cohort. No laptop prepare/timings; native/statistical acceptance
   remains PA12.

### Critical and repeat review

- P1: hiding actual observer implementation. HC01 retains scanner/errors/storage/
  counts in ordinary helpers and exercises their failures.
- P1: negative cases failing before the intended boundary. HC03–HC06 require
  observed exact calls/IDs/order and positive controls, not generic errors alone.
- P1: mocks mistaken for native acceptance or pooled source revisions. HC07
  separates evidence and preserves the selected measurement SHA.
- P2: mutation of user Docker/PATH or following links. Only scoped fake
  executables/command doubles and temporary no-follow controls apply.
- P2: calling denominator reduction added coverage. Baseline and HC07 explicitly
  distinguish reclassification from requirements newly tested.

Repeat review found no remaining P0–P2 in this bounded inventory. No new product
API/schema/seam, deleted test or weakened assertion. Existing helper expectations
were checked without finding contradictions; repeat the required post-approval
audit and ask separately if one is discovered.
