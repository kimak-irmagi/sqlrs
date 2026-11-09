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

## TA01–TA06: archive acquisition and fixed build-source root

Status: TA01–TA06 approved by @evilguest on 2026-11-09 before tests/code; exact
reply time unavailable. The clarification at the end of
[the diagnostic design](issue89-performance-diagnostic-design.md) is approved.
Repeated old-test audit found no contradictions or amendments. Three preparation failures in
`42580413674` produced no measurements and do not satisfy DG11.

1. **TA01 — transport ordering and identical bytes.** Small independent fixtures
   with precomputed SHA256: primary success never calls alternatives; exhausted
   transient Node failures call system curl once; its transport failure enables
   the official fixed-asset API with `Accept: application/octet-stream`, without
   Authorization/new user URL. Curl success skips API; API success returns exact
   bytes. Observe addresses, headers, ordering and counts using per-call transport
   facilities, not global fetch/environment replacement. Production asset/pins
   are separately checked by TA04; a tiny fixture is not the actual release.
2. **TA02 — failure never enables a checksum bypass.** Wrong successful bytes
   from Node/curl/API, including HTTP-200 JSON/HTML, fail; no subsequent transport
   follows a mismatch. Transient API/body failures retain bounded retry/request
   deadlines; exhaustion remains preparation failure. Preserve immediate ordinary
   non-transient rejection (e.g. 404) and the existing no-API path. Errors cannot
   expose the private test marker or declare artifact readiness.
3. **TA03 — product-source root separate from observer tooling.** No nonempty
   optional root retains own-checkout selection. An explicit root accepts only an absolute
   first-checkout path equal to GITHUB_WORKSPACE; reject relative, foreign or
   unavailable roots and an empty GITHUB_WORKSPACE. Owned temporary directories and tiny Git fixtures
   verify wrong HEAD/dirty checkout before dependency/build work. Never mutate
   the real repo or build/run the product for negative cases. Exercise actual
   root selection, not just a YAML text search.
4. **TA04 — connected preparation/Actions provenance.** YAML and targeted script
   wiring: setup action comes from the exact observer checkout, executes its own
   prepare and explicitly passes the first checkout's root, still `668d505`.
   Original Go/Java, clean HEAD, `-trimpath` commands, JDBC/archive/metadata/tag
   and both original hashes are not substituted. Retain existing DG02 negative
   identity controls; production API URL/asset/archive pin are fixed in the actual
   call, not merely the stub. The original full-cohort workflow is unchanged.
   TA06 proves the real build; script structure alone cannot do so.
5. **TA05 — full small regressions and coverage.** All old/new Node/Go tests,
   Linux vet, script syntax and YAML; ordinary-helper target 100%, minimum 95%,
   with a new line report. Actual preparation/Actions and unmeasured system calls
   are not included in small-helper percentages. No old-test changes; separately
   resolve any new contradiction found.
6. **TA06 — real diagnostic retry.** Explicitly run the new pinned observer on
   the original binaries; acquire v1 via the reviewed API path and verify the
   original archive/both executable hashes before measured requests. Then run
   mandatory strace smoke and the same eight serial DG10/DG11 members with full
   DB/cache/cleanup oracles. Sanitized provenance links new preparation tooling
   to the original product; never publish raw traces/tokenized addresses. If the
   old URL recovers, its success does not prove the API path: independently prove
   actual API acquisition with the same hash before measurement. No laptop
   DB/Liquibase workloads. This remains diagnostic, not speed acceptance or
   permission to implement an optimisation.

Critical review and corrections:

- P1 tiny fixture pins pass despite a wrong production asset ID — TA01 separates
  small data from TA04/TA06 actual pins.
- P1 a broad catch converts checksum failure into fallback success — TA02 checks
  exact call counts and final rejection after wrong bytes are received.
- P1 only YAML is checked while builds use observer source — TA03 exercises real
  root selection/rejection, TA06 proves actual artifacts.
- P2 negative cases start builds/network before their intended failure — TA03
  requires the pre-effect boundary and owned temporary data.
- P2 an empty ordinary optional input is wrongly rejected as diagnostic mode —
  TA03 retains own-checkout selection; TA04 requires a nonempty root to be passed
  explicitly by diagnostics.
- P2 ordinary success hides an untested API or replaces qualification cohorts —
  TA06 demands actual API/original-binary proof and separate evidence.

Repeat review found no remaining P0–P2. Preliminary audit of old download/inputs/
workflow/diagnostic tests found no contradictions; no amendments are proposed.
The post-approval audit was repeated; implementation begins with tests.

Small TA01–TA05 execution is complete: the first test run rejected the not-yet
implemented helpers as expected, then implementation passed all **32 Node tests**,
script/YAML checks, full small Go tests and Linux vet. The five ordinary Node
helpers cover **100% lines / 95.75% branches**; Go covers **98.0% (494/504 statements)**.
The unfiltered Node profile is 83.70% lines: it also includes the partially
executed `prepare.mjs` process and imported JDBC code. That is a different
denominator, not complete preparation/JDBC coverage or proof of a real build.
No product files/old tests changed. Next is TA06/DG11 on Actions; neither an
exact slowdown cause nor successful diagnostic collection is claimed yet.

First actual retry `42603675428` verified original hashes, API acquisition and
strace smoke. Two plain v1/v2 members passed full oracles; member 3 (traced v1)
failed with generic `unknown`. Collection is **incomplete**; TA06/DG11 remain open.
The original sanitized report is
[retained separately](issue89-diagnostic-evidence-2026-11-09/42603675428/jhipster-diagnostic.json),
SHA256 `23f80833a0be7405535f5f8b69aa21c79b34da933b33a300f3ac9ffe1c94c366`.
This plain pair neither replaces full cohorts nor establishes a causal explanation.

Within approved DG03/DG09, closed failure output now distinguishes only fixed
`process_tree/transport/journal/endpoint/clock` or `trace_0`–`trace_17`. Zero means
no specific decoding checkpoint; 1–17 are fixed quota/framing/time/lifecycle/
argument/end-of-stream checks documented beside the decoder. No trace lines,
paths, PIDs or arguments are exported; unreviewed labels remain `unknown`.
Additional cases within DG03/DG09 check allowed codes, rejection of foreign labels
and no private marker; existing assertions are unchanged. Full small tests/vet
pass: Go **499/509 (98.0%)**, Node 33 tests, five ordinary helpers **100% lines /
95.83% branches**. No new trace-read boundary or product-change authority is
introduced; the next retry should identify its failed check, not expose raw data.

Retry `42606053245` again stopped at member 3, now with **trace_5**: a terminal
process record arrived with a pending exec. Its
[original incomplete report](issue89-diagnostic-evidence-2026-11-09/42606053245/jhipster-diagnostic.json)
is retained with SHA256 `1bf2afdcb2082524da3fb31f11e3fd41ef8180bc4ca3100faa04dbd0c3c3f6af`.

DG03/DG05 now separately test the final API boundary: a confirmed terminal
record can close a pending exec only if the attempt began **strictly after**
that boundary. It is not counted as a successful launch; the preceding confirmed
image history remains available for clipping. In-window/unknown-boundary/missing
terminal cases and default strict decoding still fail. The old `pending_exit`
negative case is unchanged. This tests a cleanup hypothesis, not a claim about
the previous measurement failure's cause. Full small Go tests/vet pass:
**504/514 (98.1%)**; Node is unchanged. A real retry is necessary: an in-window
trace_5 will still stop collection.

Retry `42612253733` again stopped at member 3 with `trace_5`; attempt start is
no later than the final API boundary, otherwise the exception would apply.
This does not yet locate it relative to both windows or identify any program.
The [original report](issue89-diagnostic-evidence-2026-11-09/42612253733/jhipster-diagnostic.json),
SHA256 `e0bacb484732934de17c0e6cc72b444273c71c9e09e09cb71a70d686b12223b1`, is retained.
All ordinary CI `42612254604` jobs pass; only the aggregate coverage gate is red.
The separately labelled `exec.unconfirmed` design was approved on 2026-11-09.
The inventory below was subsequently approved, then tests and explicit measured
mode were implemented; EU06's real diagnostic retry remains unfinished.
TA06/DG11 and exact slowdown localisation remain unfinished; speed acceptance is blocked.

## Proposed: EU01–EU06, unconfirmed exec

Status: EU01–EU06 explicitly approved by @evilguest on 2026-11-09 after critical
and repeat review; exact reply time unavailable. Observer-only addition.
Post-approval audit found one EU04/DG09 conflict about dropping versus rejecting
extra fields; a narrow clarification is requested separately. Other cases proceed
without changing existing tests.

1. **EU01 — exact interval, not a launch claim.** Independently authored execve/
   execveat entry-without-result records followed by the same PID's natural zero,
   nonzero or signal exit. Only explicit measured mode returns `exec.unconfirmed`
   with exact start/end; without a prior image, there are no other intervals.
   With a prior confirmed image, that image ends exactly at attempt start, not
   exit. Confirmed successful and explicitly failed exec retain their existing
   behaviour. Engine-PID cases remain excluded from child-command reporting.
   Neither category nor exit code establishes a successful JVM/CLI launch.
2. **EU02 — strict mode and insufficient evidence.** Keep existing negative
   tests, including `pending_exit`, unchanged. New paired checks require the
   same EU01 input to fail in strict mode; the old narrow late-cleanup boundary
   also remains unchanged. Measured mode still rejects absent terminal records,
   another PID's exit, reversed time, duplicate pending, orphan resume, quotas,
   read failure and malformed entries without partial success. Specifically test
   a line ending in `<unfinished ...>` that is not a valid exec entry: the new
   branch must not bypass format checks. Interleaved PIDs, reuse after exit and
   a non-exec thread exit cannot merge histories or invent intervals. Errors
   contain no original data.
3. **EU03 — windows, union and counts.** Hand-calculated expectations cover
   uncertainty inside each API window, crossing a boundary, between windows,
   strictly after the final boundary and spanning both windows. Clip separately;
   outside-window time contributes nothing. The prior image and uncertain span
   of one PID cannot overlap; other-PID overlaps use union, not addition.
   Assert exact `CoveredMS`, `ResidualMS`, `Commands` and zero duration: no negative
   residual or empty added interval. `Commands` counts displayed intervals;
   independent Liquibase application/planning counts do not increase.
4. **EU04 — closed report and explicit uncertainty.** Go serialization and the
   actual Node decoder accept only the exact new category in the existing
   interval shape. Reject near-miss labels, extra fields, bad bounds, wrong
   interval counts and forged summary totals. Program/argv sentinels appear in
   neither JSON nor errors. Plain runs retain null Trace. The interval is not
   relabelled Java/Docker and cannot change independent counters. Material unknown
   time stays a visible limitation, not a causal finding or speed acceptance.
5. **EU05 — explicit wiring and finished observation.** A named internal measured
   entry point leaves default strict and existing late-boundary calls unchanged.
   A small test through actual finish with a private temporary stream/file proves
   mode selection and exact intervals, not just collector text. Collector wiring
   checks require owned-tree stop, reader join, parsing, then report emission;
   transport/unproved-stop failures cannot be repaired by an uncertain interval.
   EU06 additionally proves actual tree ordering. No raw stream/new read boundary
   or product changes.
6. **EU06 — regressions and real retry.** Full small Go/Node tests, Linux vet,
   script/Actions syntax and a fresh per-line helper profile (target 100%, minimum
   95%). Then explicitly launch Actions on a pinned new observer: identical
   binaries/hashes, mandatory native strace smoke, the same eight serial members,
   DB/cache oracles, exact counters and cleanup. Do not rewrite old incomplete
   reports. Retain another failure as failure, never delete records to obtain a
   green report. Complete diagnostic collection alone neither closes P01b nor
   authorises a product fix.

Critical review and corrections:

- P1 attributing unknown time to a prior JVM: EU01/EU03 require its exact boundary
  and independent numeric expectations.
- P1 accepting malformed unfinished input unchecked: EU02 requires a distinct
  negative control before interval creation; EOF/another PID's exit is not proof.
- P1 accepting any terminal PID/zero exit as a successful launch: EU01/EU02 check
  linkage, PID reuse and unchanged independent counts.
- P2 only the decoder is exercised, leaving collector wiring absent: EU05 tests
  actual finish and collector selection; EU06 confirms actual Actions.
- P2 Go accepts a category that Node drops/distorts: EU04 covers both sides,
  shape/totals and private-data exclusion.
- P2 green collection claims a regression fix: EU04/EU06 retain attribution limits
  and the existing speed criterion.

Repeat review found no remaining P0–P2 in this inventory. Preliminary audit of
strict/late-cleanup tests indicates a separate explicit mode can preserve them
unchanged. Repeat that audit after approval and separately agree any new conflict.
No local Liquibase/DB workload: only small observer checks may run on the laptop.

### Small execution checkpoint (2026-11-09)

The first EU build failed on the missing measured decoder/finish methods; the
first Node test rejected the new category. Implementation then passed the complete
small Go suite and Linux vet: **527/537 statements (98.1%)**. The per-line profile
is `.tmp/performance/diag-eu.out`; 10 operators remain uncovered, including eight
pre-existing process/journal/clock branches. All **34 Node tests** pass; the five
ordinary helpers cover **100% lines / 96.65% branches**. This is not product or
Actions-orchestration coverage. Existing strict/late-cleanup tests and assertions
are unchanged. Collector wiring changes only the explicit measured decoder call.
The independently identified EU04 extra-field conflict is pending clarification;
the old DG09 safe field-dropping behaviour/test is unchanged. No new assertion
about rejecting those fields has been added. All other new cases pass.
EU06's eight-member Actions retry is the next independent stage; no successful
trace cohort or exact product cause is established by these small checks.
