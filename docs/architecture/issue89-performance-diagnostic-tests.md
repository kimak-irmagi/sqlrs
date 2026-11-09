# #89: exact-binary diagnostic tests

Status: DG01–DG12 and separately OT-DG01–OT-DG03 approved by @evilguest on
2026-11-08 after critical and repeat review (exact reply time unavailable).
The [component design](issue89-performance-diagnostic-design.md)
is approved on 2026-11-08. This inventory does not change P01b, product behaviour,
API/schema or the ongoing `668d505` cohorts. It is diagnostic localisation, not
another full statistical qualification or permission to implement a product fix.

## Final test inventory

1. **DG01 — explicit remote boundary.** Parse the new workflow and execute its
   event guard with allowed label, wrong label, ordinary push/update, fork and
   invalid ref. Require same-repository opt-in, read-only token, standard Ubuntu,
   distinct concurrency group, one serial job, 60-minute hard/earlier cleanup
   limit and sanitized-report-only upload. No cancellation or modification of
   the existing workflow, paid runner or laptop workload.
2. **DG02 — unchanged executables and correct launcher.** Reject wrong measured
   revision, dirty/wrong-platform build, changed v1/v2 bytes or instrumentation
   inserted into measured source before API work. Verify the fixed binaries and
   separate tooling revision/collector build. Linux v1 selects the actual
   `LiquibaseExec` script (`liquibase`), not a neighbouring `.bat`; two fixtures
   have deliberately different bytes. Missing/mismatched script refuses complete
   diagnostic provenance. Keep core/JDBC/image checks and old reports unchanged;
   derived JVM counts are labelled, not substituted for observed host processes.
3. **DG03 — bounded trace decoding and process segments.** Hand-authored records
   cover successful exec/execveat and exit, failed exec, escaped arguments,
   unfinished/resumed exec, same-PID re-exec, PID reuse after exit and interleaved
   different PIDs. Require exact expected intervals; a failed exec is not a
   launch, re-exec closes the old segment. Truncation, malformed/unpaired records,
   missing command completion or backwards time in one lifecycle are incomplete,
   not ignored/zero. A non-exec thread exit is not an invented command. Never
   put offending input or a private sentinel into an error.
4. **DG04 — fixed operation categories.** Positive and near-miss argv fixtures
   cover Docker lifecycle/inspect/copy, offline image probe, planner/step, PG
   control/identity, host Liquibase/Java and other. Classify executable/argument
   positions, not secret/SQL substrings. Preserve counts and unknown-category
   intervals; never claim observation of container-internal Java. Raw argv,
   paths, IDs and injected sentinels cannot appear in the serialized result.
5. **DG05 — intervals and attribution limits.** Known numeric examples cover
   both API endpoints, clipping crossing commands, outside startup/cleanup,
   nested/overlapping/adjacent intervals, empty subprocess work and residual time.
   Exclude the engine lifetime. Check union coverage, not a sum double-counting
   parent/child work; show per-category gross intervals as non-additive. Detect
   inconsistent wall/monotonic windows, nonfinite/negative values and impossible
   evidence. Unknown/residual time remains explicit, never called an exact cause.
6. **DG06 — private bounded transport.** Dedicated pipe/capped writer tests:
   exact limit, limit+1, long line/row cap, reader/writer failure, cancellation
   before and during acquisition. No more than 32 MiB is retained; raw trace
   stays in a new exclusive regular 0600 file under the owned root, with symlink/
   outside-path rejection. Observe reader termination and owned-group shutdown
   on failure; no generic-error success before the injected boundary.
7. **DG07 — exact process ownership and cleanup.** Test live direct-child/PGID/
   exact-executable proof, wrong parent/group/executable, unrelated PID, vanished
   process and unsafe zero/broad kill targets. Untraced PID equality stays intact.
   A tracer exiting before the engine does not prove completion. Tiny owned
   helper-process tests cover TERM, forced KILL, deadline and surviving child,
   reader join, process absence before private-root disposal and unchanged foreign
   control. Actual strace smoke runs on Actions against a tiny helper, not a
   local DB/Liquibase workload; required smoke cannot silently skip there.
8. **DG08 — journal phase evidence.** Use real temporary SQLite and fixed readable
   columns for both schema versions; check these columns against the retained
   canonical definitions. Test exact job filtering, step order, UTC timestamps,
   valid optional/missing-version boundaries, malformed/missing mandatory time,
   query/stream failure and cancelled context. Emit only relative phase/ordinal
   data, never request_json/event messages/DSNs/SQL/identities. An unavailable
   boundary is marked unavailable, not zero. Real DG11 runs validate both full
   journals, not only the small column fixtures.
9. **DG09 — closed report/completeness.** Success/failure reports contain only
   typed finite numeric fields, reviewed enums and pinned provenance. Inject raw
   argv/log/DSN/SQL/secret sentinels and unknown nested fields; they cannot escape
   to console/artifacts/errors. Missing mode/pass/member, duplicate observation,
   wrong source/host, contradictory counters, trace/cleanup failure or malformed
   report cannot become complete or a speed pass.
10. **DG10 — fixed serial schedule and unchanged normal route.** Predeclare eight
    members: untraced pair 1 (v1,v2), traced pair 1 (v1,v2), traced pair 2 (v2,v1),
    untraced pair 2 (v2,v1); each has uncached/cached passes. Assert exact order,
    no concurrent member, fresh store per member and paired same-store cache.
    Report the observed traced/untraced differences separately without treating
    them as perfectly removable overhead. Without the explicit diagnostic mode,
    no tracer/trace directory/journal report is created; old API timers, two
    sample outputs and all workload/counter/cleanup checks remain unchanged.
11. **DG11 — real remote diagnosis.** Explicitly launch the guarded workflow on
    the fixed binaries after fast checks/strace smoke. All eight JHipster members
    must pass full DB oracle, 0/8 cache hits and verified exact cleanup; traced
    members must yield matched bounded observations and journal phases. Verify
    actual Linux script/core/driver, exact endpoint ownership, closed artifact
    schema and absence of all owned processes/private traces before success.
    Retain sanitized evidence and identify the operation/phase explaining the
    observed gap, or explicitly report the unresolved residual and propose the
    next narrower observation. No laptop benchmark or inferred causal claim.
12. **DG12 — regression and coverage boundary.** Run all old/new small Go tests,
    Linux compile/vet, Node helper/workflow tests and regular PR CI. No old test
    deletion/weakening. Ordinary Go and reusable Node helpers retain target 100%,
    minimum 95%; actual `testing.T` orchestration stays a test scenario under the
    already approved HC boundary, not hidden helper code. Obtain per-line results
    and request a new requirement-backed iteration if needed. Native orchestration
    and any uninstrumented process adapter are reported separately, not called
    covered by helper percentages. Existing P01b cohorts are never relabelled.

## Separately requested old-fixture amendments

These are not authorised merely by inventory approval. Only
`TestReleasedEngine` / `collect` in `collector_linux_test.go` is affected:

- **OT-DG01:** add an explicit traced launch branch and the separate exact /proc
  endpoint ownership proof; keep the original untraced PID equality and actual
  request/result/counter/oracle assertions.
- **OT-DG02:** correct only Linux v1 launcher provenance from the neighbouring
  `.bat` to its actual selected POSIX script. Keep version/core/driver identity
  checks and derived-vs-observed distinction; never rewrite saved evidence.
- **OT-DG03:** extend only traced cleanup/report preparation to stop and confirm
  the owned tree, join the reader and parse the private trace before root disposal
  and final sample emission. Retain all existing container/file cleanup, foreign
  control, failures-before-success and untraced timing boundaries.

Initial old-test audit found no conflicting expectations in the small suites:
the new workflow is independent, and parser tests do not require a specific v1
launcher hash. The three scenario edits above are explicit, not blanket approval
to alter other tests. Repeat the audit after approval and ask about new conflicts.

## Critical review, corrections and repeat review

- P1: launch instrumentation could defeat the endpoint PID check or leave the
  engine alive. DG07 and OT-DG01/03 require positive/negative ownership proofs,
  exact process absence, reader join and unchanged untraced checks.
- P1: a parser could pass invented self-consistent fixtures but fail real strace.
  DG03 includes real-format split/re-exec/interleaving cases and independent
  expected intervals; DG07 requires actual strace smoke before DG11.
- P1: totals can double-count or hide unknown time. DG05 separately tests union,
  clipped windows and residuals; a large residual does not establish causality.
- P1: privacy/quota cases could pass on an earlier failure. DG06/09 observe the
  exact injected boundary and check private/public bytes and positive controls.
- P2: instrumented timings might qualify speed or a different binary. DG02/10/11
  pin both bytes and separate modes; no P01b replacement or live-run changes.
- P2: reduced schema fixtures or moved test assertions could inflate confidence.
  DG08 checks canonical columns and actual journals; DG12 separates coverage
  reclassification, genuine helpers and real orchestration evidence.

Repeat review found no remaining P0–P2 in this bounded diagnostic inventory.
The post-approval audit found no additional old-test contradictions. Only the
three named fixture amendments are authorised; implementation now follows tests.

## DC01–DC03: approved line-report follow-up

Approved by @evilguest on 2026-11-09 before new tests; exact reply time unavailable.
The repeated audit found no additional old-test contradictions or amendments.

First full small-test run after implementation: Go 91.0% statements; Node 100%
lines. Previous Go helpers remain 100% covered. Native strace and DB/Liquibase
are intentionally not run locally; DG11 and strace smoke await Actions.
Profile: `.tmp/performance/diag-all.out`; DG12's minimum is not yet met.

Order by uncovered statement count:

1. **DC02 — trace, DG03/04/06:** `diagnostic_trace_linux.go`, 18 statements.
   Independent malformed PID/time/exec/argument lists, two pending calls on one
   PID, exit before resumed, row quota; positive helper categories and invalid
   zero transport quota. Private markers never escape.
2. **DC01 — journal, DG08:** `diagnostic_journal.go`, 15 statements. Real temporary
   SQLite: missing mandatory timestamps, invalid task/invocation types/times,
   missing required columns, optional-table query failures. No global DB stubs;
   demonstrate the intended read was reached and retain positive controls.
3. **DC03 — processes, DG06/07:** `diagnostic_process_linux.go`, 12 statements.
   Exact strace arguments (explicit tracer signal-blocking mode), malformed
   /proc, seek failure on a closed file, reader timeout/join, missing Wait proof
   despite group absence. Check actual owned process/reader completion and the
   foreign control. Do not invent os.Pipe/resource failures with global hooks.

Then repeat all small tests, vet and line coverage. DG11 remains separate real
evidence, not a substitute for small negative tests. No new product boundaries,
schema, old-test changes, permissions or dead-code deletion are proposed.
Critical review excluded synthetic OS failures/global interception; repeat
review found no P0–P2 in this bounded follow-up.

The first new-test run exposed marker-like argument strings being misread as
strace syntax and missing explicit tracer signal mode. Parsing now checks the
record's actual ending, not arbitrary substrings; `-I 3` retains the tracer until
tracees exit when the owned group receives TERM. Cleanup review also prevented
premature CommandContext cancellation of strace: exact t.Cleanup group shutdown
owns it instead. These changes affect the observer, not product binaries or API.

The repeated full small-test run and Linux vet pass. Profile
`.tmp/performance/diag-dc-final.out`: **494/504 statements (98.0%)**, exceeding
the 95% minimum; previous Go helpers remain 100%. All 26 Node tests pass, with
100% lines / 95.73% branches in the five imported helpers. Actual Actions
orchestration is not included in those percentages. Ten uncovered statements
are defensive SQLite stream/catalog, os.Pipe/transport, malformed time/exec
result failures; they are not claimed tested or dead. Next is DG11 on Actions.
