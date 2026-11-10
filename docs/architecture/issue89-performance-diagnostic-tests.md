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
extra fields. The user explicitly approved retaining safe field dropping and
clarifying EU04 on 2026-11-09. Existing tests remain unchanged.

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
   interval shape. Drop extra fields at every nesting level, preserving DG09;
   reject near-miss labels, bad bounds, wrong interval counts and forged summary
   totals. Program/argv sentinels appear in
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
The independently identified EU04 extra-field conflict was subsequently resolved
by explicit user approval: preserve DG09 field dropping and add its new-category
control. The old DG09 behaviour/test is unchanged; no new rejection policy exists.
EU06's eight-member Actions retry is the next independent stage; no successful
trace cohort or exact product cause is established by these small checks.

Actual EU06 retry [42618850460](https://github.com/kimak-irmagi/sqlrs/actions/runs/42618850460)
passed setup, native strace smoke and both plain members, but failed at traced
member 3 with **trace_8**. The original sanitized incomplete report is retained
[unchanged](issue89-diagnostic-evidence-2026-11-09/42618850460/jhipster-diagnostic.json),
SHA256 `b1b706b50a8b725286ecf2d34987c552c3ca9324e5cb35a226a965bdba15dc4a`.
It does not identify which of the two record-kind checks failed. No new format
is accepted and no exact product cause is claimed. A failure-context proposal
was subsequently approved, followed by separate CX01–CX05 approval before tests
and implementation. EU06/TA06/DG11 remain incomplete.

## Proposed: CX01–CX05, closed failure context

Status: component amendment and, subsequently, this separately reviewed inventory
explicitly approved by @evilguest on 2026-11-09; exact reply time unavailable.
Post-approval old-test audit found no contradictions; old assertions are unchanged.

1. **CX01 — exact record shape, not syntax acceptance.** Hand-authored fixed
   forms cover `superseded_exec` (numeric ID, never exported), `exec_entry`
   (execve/execveat), `exec_resume` and `other`. Require anchored matching:
   a quoted argument containing the same marker, a malformed numeric marker,
   trailing private text and near-miss spellings cannot become superseded exec.
   Actual decoder failures separately prove `record` for a top-level unknown
   record and `pending_entry` for a malformed pending entry at terminal handling.
   Preserve exact old code 8, failure and no partial result. Classification alone
   must not make the record valid or silently invent an execution interval.
2. **CX02 — preserve the error and closed context.** Check direct and wrapped
   errors through the real code extractor: the old numeric code, generic safe
   error and errors.As/unwrap behaviour remain usable. Missing context, unrelated
   errors, unknown location/shape and private sentinels yield only safe defaults
   (`none`), never original text. Inspect both error and serialized context for
   argument/path/PID sentinels. Existing strict/measured/late-cleanup tests remain
   unchanged; code/context are independent of successful CLI/JVM counts.
3. **CX03 — Node allowlist and the existing collector stage.** Decode explicit
   Go-style failure markers into only the approved location/shape enums. Missing,
   foreign and suffixed near-miss values become `none`; do not copy raw strings.
   Preserve existing diagnostic failure code and cleanup boundary independently.
   For stage, test every existing cohort value (`identity`, `engine_start`,
   `configuration`, `uncached`, `cached`, `oracle`); missing/foreign values become
   `collector_exit`. The stage means last reached phase, not proof of a workload
   failure: an `oracle` stage plus a cleanup parser error remains incomplete.
   Unknown extra private properties never enter the rebuilt metadata.
4. **CX04 — real failed-decoder/report wiring.** Test the real decoder with
   independent failing inputs and assert exact location/shape/code, then exercise
   the actual Node helper on expected closed log markers. Addressed collector/
   reporter wiring checks require emission of this context only on failure and
   persistence of context plus existing stage/code before reporting incomplete.
   Do not satisfy the requirement with pure shape tests or YAML search alone.
   No raw-body argument can reach the public formatter; old sample parsing,
   timers, successful report records and strict error assertions stay unchanged.
   Real orchestration is additionally checked in CX05, not claimed unit-covered.
5. **CX05 — regressions, coverage and an actual diagnostic retry.** Full small
   Go/Node tests, Linux vet, script/Actions syntax and per-line helper coverage
   (target 100%, minimum 95%). Then explicitly repeat the same eight-member
   Actions protocol on a pinned observer and the original executable hashes,
   with native strace smoke, DB/cache/count/cleanup controls. On a repeated
   decoding failure require closed context in the retained incomplete report;
   `other` remains an observation limit, not permission to skip the record.
   If collection succeeds, keep it separate from P01b acceptance. Retain old
   reports unchanged; no local Liquibase/DB workload and no product fix authorised.

Critical review and corrections:

- P1 fixed shape silently admits new syntax: CX01/CX04 require the same rejection
  and numeric code after classification, not successful decoding.
- P1 tests cover the classifier but cannot distinguish the failing call site:
  CX01/CX04 require actual record versus pending-entry decoder paths.
- P1 context wrapping loses the original code or exposes raw input: CX02 checks
  extraction/wrapping and private sentinels, not only the display message.
- P1 last phase is misreported as the primary workload failure: CX03 explicitly
  preserves the phase's limited meaning and independent cleanup/decode failure.
- P2 helpers pass while the actual artifact omits context: CX04 checks addressed
  wiring; CX05 requires a real retained report, not inferred orchestration coverage.

Repeat review found no remaining P0–P2. Preliminary audit of DG09 code allowlists,
EU strict/late-cleanup assertions and old error construction found no requirement
to weaken old tests. Repeat that audit after approval; separately agree any new
contradiction. No changes to old assertions are requested by this inventory.

EU04 clarification execution: its additive new-category nested-field control
passes with unchanged parsing/product code. Full Node regression: **35 tests**,
five ordinary helpers **100% lines / 96.67% branches**. Old DG09 assertions remain
unchanged. This does not execute CX01–CX05 or finish the remote EU06 cohort.

### CX small execution checkpoint

First runs failed on absent Go context helpers and the missing Node export.
Implementation then passed the full small Go suite and Linux vet: **544/554
statements (98.2%)**, profile `.tmp/performance/diag-cx.out`. New shape/context
helpers are 100%; 10 other operators remain uncovered. All **37 Node tests**
pass; five ordinary helpers cover **100% lines / 96.75% branches**. Native
collector/report orchestration is not claimed unit-covered. Context only augments
failure evidence; old numeric codes, strict/default/late-cleanup assertions,
successful samples and measured executable bytes are unchanged. CX05's actual
eight-member retry remains to run; no successful collection or exact cause claimed.

Actual CX05 retry [42632130376](https://github.com/kimak-irmagi/sqlrs/actions/runs/42632130376)
passed setup/native smoke and both plain members; traced member 3 failed with
`trace_8`, **pending_entry / other**, last-reached stage `oracle`, cleanup boundary
`unknown`. The original sanitized incomplete report is retained
[unchanged](issue89-diagnostic-evidence-2026-11-09/42632130376/jhipster-diagnostic.json),
SHA256 `8dd2dcd447f281581d990028b0c6de8192436f523f88956a4d553e75b0ad1356`.
This verifies actual failure-context transport, not complete eight-member
collection or workload success for the failed member. The superseded-exec
hypothesis is not established by this evidence. Existing fixed classifiers cannot
identify the pending prefix beyond `other`; do not treat it as valid exec input.
Ordinary CI `42632129606` passes all regression jobs; only coverage-merge is red.
Further prefix observation is separately approved, not an implemented relaxation;
its exact PX01–PX05 inventory was subsequently approved before tests/code.

## Proposed: PX01–PX05, structural prefix observation

Status: explicit @evilguest component approval recorded on 2026-11-10; exact reply
timestamp unavailable. This inventory was subsequently explicitly approved on
2026-11-10 before tests/code. Repeated old-test audit found no contradictions;
the separate additive boundary preserves old exact-shape assertions.

1. **PX01 — independent grammar/call matrix.** Hand-authored inputs and exact
   expected pairs cover every approved prefix: empty/whitespace, an additional
   numeric header, entry/resume, PID-change, unknown syscall, exit/signal/bracket
   notices and other. Cover each known call (execve, execveat, exit, exit_group,
   restart_syscall), an unlisted name mapping to `other`, and unavailable `none`.
   Unknown-syscall form takes precedence over generic entry; known call names
   require the anchored entry/resume token, never substrings/quoted arguments.
   Near-miss spelling, bad numeric framing and injected markers cannot create
   a more specific unrelated class. No actual call name outside the fixed list,
   raw numbers, strings, lengths, hashes or argv enter the result.
2. **PX02 — actual error path, unchanged rejection.** Real measured-decoder
   fixtures cover empty pending prefix, unknown syscall entry and an additional
   header before terminal handling, plus an ordinary rejected record. Assert
   exact original code, location/shape, new prefix pair and no partial intervals.
   Nil/unrelated errors and known shapes have unavailable prefix metadata;
   missing/foreign stored fields and wrapped errors preserve safe defaults and
   original checkpoint access. Serialize contexts with private name/argument/PID
   sentinels and prove none escape. Keep default/late-cleanup assertions intact.
3. **PX03 — additive Node transport with safe defaults.** Decode new fixed markers
   in a separate prefix-metadata helper; keep the existing context helper's exact
   return shape and CX03 deep-equality assertions unchanged. Independently test
   all allowed pairs and missing/unknown/suffixed labels; non-call classes cannot
   assert a known call. Old-style output without new markers yields unavailable
   values. Extra fields/private sentinels cannot leave rebuilt metadata. Existing
   stage/code/cleanup values remain independent and unchanged.
4. **PX04 — explicit wiring, only for other failures.** Real Go error fixtures
   plus addressed collector/reporter wiring checks require extraction without
   passing a raw body into the public formatter. Keep the old context marker and
   existing helper call; add the new fields separately only on failure with shape
   `other`. Assert no new fields on successful samples or known-shape reports,
   and persistence before the incomplete report is returned. Pure classification
   or source inspection alone is insufficient: PX02 and PX05 exercise real paths.
5. **PX05 — full small checks and the same remote protocol.** Full Go/Node tests,
   Linux vet, script/Actions syntax and per-line helper coverage (target 100%,
   minimum 95%). Explicitly launch the pinned observer against the same executable
   hashes, native strace smoke, eight serial members and original DB/cache/count/
   cleanup controls. If `other` repeats, require new closed fields in the retained
   incomplete report; unknown remains unknown, not discarded. Preserve original
   reports, distinguish observer failure from product slowdown and retain P01b.
   No DB/Liquibase workload on the laptop; no syntax or product fix authorised.

Critical review and corrections:

- P1 class/name inferred from private argv: PX01 fixes token positions and tests
  quoted decoys; PX02/PX03 verify both serialization boundaries.
- P1 overlapping patterns give unstable meaning: PX01 requires explicit
  unknown-syscall precedence and independently specified expected pairs.
- P1 old context tests silently need rewriting: PX03/PX04 require a separate
  additive helper/marker, preserving the existing exact context shape and tests.
- P1 valid-looking metadata admits an invalid trace: PX02 preserves real rejection,
  old code and absence of partial intervals; PX04 adds fields only on failure.
- P2 small tests are mistaken for actual artifact evidence: PX05 requires a real
  retained report and keeps unexplained `other`/speed qualification limitations.

Repeat review found no remaining P0–P2. Preliminary old-test audit found no
contradiction with this additive boundary; repeat after approval. No old test
changes are requested. New contradictions need separate user direction.

### PX small execution checkpoint (2026-11-10)

The first Go build and Node test failed on the absent prefix helpers/export.
Implementation then passed the full small Go suite and Linux vet: **579/589
statements (98.3%)**, profile `.tmp/performance/diag-px.out`; new prefix helpers
are 100%, the same 10 other operators remain uncovered. All **39 Node tests**
pass; five ordinary helpers cover **100% lines / 96.86% branches**. Old CX/DG/EU
assertions and exact context-helper return shape are unchanged. No new syntax
is accepted; ordinary success samples/product bytes remain unchanged. Actual
PX05 and complete eight-member collection remain unfinished until remote evidence.

Native PX05 [42665130138](https://github.com/kimak-irmagi/sqlrs/actions/runs/42665130138)
passed setup/smoke/plain members, then failed at traced member 3. It proves actual
prefix transport: `trace_8 / pending_entry / other`, **unknown_syscall / other**,
last stage `oracle`, cleanup boundary `unknown`. The original incomplete report
is retained [unchanged](issue89-diagnostic-evidence-2026-11-10/42665130138/jhipster-diagnostic.json),
SHA256 `567adac8bc4c45991289787321227e355fb7e592cb790d6d24202fd678ea3587`.
The collector treated an unknown-syscall-shaped unfinished record as an exec
entry, then correctly failed its exec validation. This establishes an observer
grammar limitation, not the exact syscall/value, workload success or a product
regression cause. Ordinary CI `42665129132` passes all regression jobs; only the
unchanged coverage-merge gate is red. A measured-only
raw-syscall proposal is separate; no new syntax has been accepted yet.

## Proposed UR01–UR07: raw syscall grammar and unattributed time

Status: component amendment explicitly approved by @evilguest on 2026-11-10;
this separate test inventory is explicitly approved by the subsequent user reply
on 2026-11-10 (exact time unavailable). Post-approval old-test audit found no
contradictions; existing test assertions are not authorised
to change. This remains diagnostic work, not a product fix or a P01b waiver.

### Exact bounded grammar to test (Linux/amd64, measured mode only)

Reference provenance: strace 6.8
[raw fallback and result](https://github.com/strace/strace/blob/v6.8/src/syscall.c#L817),
[six-argument limit](https://github.com/strace/strace/blob/v6.8/src/defs.h#L73),
[printargs](https://github.com/strace/strace/blob/v6.8/src/util.c#L1753) and
[hex formatting](https://github.com/strace/strace/blob/v6.8/src/print_fields.h#L316).
This defines a bounded observer subset, not a claim about the hidden numeric
payload or exact strace version of run 42665130138. UR06 validates native output.

- Name: exactly `syscall_0x` followed by 1–16 lowercase hex digits, or exactly
  `???`. A raw argument is `0` or `0x` plus 1–16 lowercase hex digits, parsed
  without overflow as unsigned 64-bit. Exactly six comma-separated arguments;
  only horizontal formatting spaces/tabs around separators are allowed. No
  nonzero decimal argument, sign, quote, symbolic token, pointer dereference,
  truncated tuple, seventh argument or trailing text.
- Complete: `name(args) = result`; pending: `name(args <unfinished ...>` without
  the closing parenthesis; resume: `<... name resumed>) = result`. Allow only
  horizontal formatting whitespace at those boundaries. Pending already has
  all six arguments. A resume must match the exact name token and PID of that
  pending entry; no cross-call/PID completion or inferred thread handoff.
- A numeric result is `0` or the bounded hex form above. An error result is
  exactly `-1 ERRNO (description)`, with a 1–32 character uppercase ASCII
  letter/digit/underscore errno starting with a letter, and 1–256 printable ASCII
  description characters excluding parentheses/control characters. These
  private fields are syntax-checked, never exported. No other signed/decimal
  numeric result or arbitrary suffix. An optional final `<seconds.fraction>`
  duration has nonnegative, overflow-checked nanosecond range and 1–9 fractional
  digits; it is not used to derive process duration.
- Exact `?` or `? <unavailable>` instead of a result is explicitly unresolved,
  not success. Such a record, or a pending entry without resume, requires an
  exact terminal event of the same PID; EOF, another PID's exit or a later exec
  does not substitute for the missing completion. A numeric/error result clears
  the pending call, but never restores the old executable category. All active
  process histories still require their terminal event. Existing quotas, timestamp
  and terminal grammar remain in force. Grammar not listed here remains rejected.

### Test groups

1. **UR01 — full grammar, not prefix permission.** Hand-authored positive/negative
   tables with independent expected results for both names, zero/max 64-bit
   values, six arguments, horizontal alignment, complete/pending/resumed forms,
   numeric/errno/no-result returns and optional duration. Mutate each delimiter,
   count, numeric bound, return/description and suffix independently. Overflow,
   17 hex digits, uppercase/nonhex/decimal tokens, quoted decoys, arbitrary names,
   CR/newline injection and valid prefix plus junk must fail with no partial
   intervals. Keep byte/line/row quotas and safe fixed errors. A valid complete
   record fails in strict mode with the existing record-kind code; old PX02
   malformed pending records keep their exact code 8/context/prefix.
2. **UR02 — exact lifecycle association.** Real measured decoder, interleaved
   PIDs and exact same-name resume; natural exit 0/1 and signal termination close
   a validated pending/unresolved call without inventing its result. Wrong PID,
   name, reversed time, duplicate pending, orphan resume, an incompatible event
   while a call is pending, missing terminal and failing/truncated reader reject
   the whole trace. Test missing terminal after both a completed call and a later
   confirmed image. Engine PID remains excluded but is still validated. Keep
   strict pending-exit code 5, old cutoff rules, existing EU and PX negatives.
3. **UR03 — honest executable/time boundaries.** Literal expected segments for:
   unknown call without a prior image; known Java then unknown; repeated unknown
   calls; explicit error/zero/hex return; failed exec after unknown; confirmed
   complete/resumed exec after a completed unknown call; and a valid unresolved
   exec ending in a terminal event after unknown. End the preceding known image
   at the first unknown entry, not its return. Retain `process.unattributed`
   without restoring Java/Docker on return/failed exec; a confirmed exec starts
   its category at its observed entry, while an unresolved exec uses the existing
   `exec.unconfirmed` boundary. Do not double-count a PID, merge different PIDs or
   count raw calls as successful CLI/JVM starts. Include sequential PID histories.
4. **UR04 — windows, Node report and privacy.** Two API windows with a gap;
   before/inside/crossing/after/zero-length intervals and overlap with confirmed
   and exec.unconfirmed segments. Independent literal union/residual/count
   expectations, no double accounting. Node accepts only the new fixed category
   alongside existing labels, validates finite ordered bounds/counts/unions,
   rejects lookalikes/bad totals and safely drops extra fields as DG09 requires.
   Both Go and Node serialization must omit numeric syscall/name/args/PID/private
   errno-description sentinels. Preserve exact CX helper shape and old reports;
   `Commands` remains interval count, not successful launches. Material unattributed
   time must stay explicitly unknown in the analysis, not relabelled as Java.
5. **UR05 — actual private transport and explicit wiring.** Exercise the real pipe
   and finishMeasured path with a valid raw lifecycle and a malformed/missing-exit
   control; strict finish remains rejecting. No partial success on read/limit/
   process-absence failure. Preserve exact tracer arguments, private bounds and
   owned-process cleanup. Verify measured collector/report wiring without raw
   publication and unchanged pins/eight-member order. Source checks supplement,
   not replace, actual decoding/transport tests. Existing native smoke is unchanged.
6. **UR06 — tiny native prerequisite, Actions only.** Add a separate opt-in test
   before the eight members on the same Ubuntu/strace. Compile a tiny checked-in
   fixture under testdata, make a deliberately invalid numeric syscall with six
   explicit raw arguments, independently require ENOSYS and clean fixture exit.
   Use the exact normal trace flags/private pipe; require at least one fully
   validated raw record for the exact owned child with the expected fixture
   number/six arguments, an unattributed interval and terminal evidence. Numeric
   comparisons stay private. Otherwise fail before measurements: a filtered-out call
   must not produce a green probe. No need to force nondeterministic unfinished
   formatting: UR01/UR02 cover split forms. Require a finite probe deadline,
   confirmed tree/reader completion and private-file disposal on success/failure;
   no raw trace/command output or numeric payload artifacts. Failure is closed,
   not a skip. No DB/Liquibase, laptop execution, new tracing flags or changed old
   smoke test. Small tests verify opt-in and fail-fast workflow placement.
7. **UR07 — regressions, coverage and original diagnostic members.** Run the full
   small Linux Go suite/vet and Node tests, script/workflow syntax checks; obtain
   per-line ordinary-helper coverage (target 100%, minimum 95%). If below target,
   submit a requirement-based follow-up plan rather than silently iterate. With
   a pinned observer, run UR06 then all original eight serial members using the
   unchanged v1/v2 hashes, workload/CLI counts, DB/cache oracles and exact cleanup.
   Retain only sanitized reports and unchanged earlier evidence. Any unsupported
   record still fails explicitly; no causal conclusion from a significant
   unattributed/residual interval. Native diagnostic success is not P01b acceptance;
   a product fix and repeated speed qualification are still required separately.

### Critical and repeat review

Corrected before submission:

- P1 prefix acceptance admitted the old malformed PX02 example: UR01 fixes the
  full six-argument hex grammar and preserves the exact old negative/code.
- P1 successful result restored an unproved image, or a pending call borrowed
  another PID's completion: UR02/UR03 require exact association and keep time
  unknown until a confirmed image/terminal event.
- P1 elapsed syscall time was confused with process lifetime or double-counted:
  UR03/UR04 specify literal segment/window unions, including failed exec.
- P1 a native test could pass without observing the unknown call, or rely on a
  nondeterministic split: UR06 requires a real validated raw record/ENOSYS, while
  hand-authored tests independently cover pending/resume/terminal forms.
- P2 report changes contradicted DG09 or bypassed the actual reader: UR04 keeps
  safe field dropping; UR05 exercises the private pipe, not just source matching.
- P2 observer success was mistaken for speed acceptance: UR07 retains the exact
  qualification gate and explicitly limits attribution of unknown time.

Repeat review found no remaining P0–P2 within this bounded inventory. Preliminary
old-test audit found no required changes: PX02's `syscall_0xabc(123456789` is an
invalid incomplete decimal tuple; DG strict, EU, CX, tracer flags and old smoke
stay unchanged. Repeat that audit after approval; ask separately about any new
contradiction. UR06 may reveal another unsupported native form; stop with closed
evidence rather than broaden the grammar without a separately reviewed decision.

### UR local checkpoint (2026-11-10, not qualified yet)

Tests preceded implementation: the initial Go build failed on the absent raw
parser; both new Node tests failed on the category/workflow. Local implementation
then passed all **41 Node tests** (five ordinary helpers **100% lines / 96.90%
branches**), Linux vet and a compile-only C syntax check (no native probe locally).
The added resumed-corruption test exposed ScanLines silently dropping terminal
CR; measured framing now rejects that control byte, leaving strict mode unchanged.

The full small Go run has one failure in new UR01: its negative
`<9223372036.0>` is actually in the approved signed-64-bit nanosecond range.
A separate correction request proposes moving it to positives and testing exact
maximum `<9223372036.854775807>` versus overflow `<9223372036.854775808>`.
That expectation has **not** been changed pending approval. All other small
regressions pass, including old DG/EU/CX/PX and the new corruption control.
Provisional line profile `.tmp/performance/diag-ur-full.out`: **644/654 statements
(98.47%)**, raw parser/duration helpers 100%, the same 10 other statements remain
uncovered. This is a failing-run profile, not a successful coverage qualification.
No old test was changed; no native UR06/UR07 launch or performance success claimed.

### UR small checks after approved correction (2026-11-10)

The user explicitly authorised only that UR01 boundary correction. Both valid
9223372036.0 and exact maximum 9223372036.854775807 now pass, while
9223372036.854775808 fails. The freshly rebuilt full small Linux Go suite and vet
pass: **644/654 statements (98.47%)**, profile `.tmp/performance/diag-ur-full.out`;
new raw parser/duration helpers are 100%. The same 10 other uncovered statements
remain; no removal or further coverage iteration was undertaken. All **41 Node
tests** pass, five ordinary helpers **100% lines / 96.90% branches**; syntax and
compile-only C checks pass. Default strict/PX02/EU/CX assertions are unchanged.
This qualifies the small helper boundary, not native UR06/UR07 or product speed.
Source CI `42667271833` also passes every regression job (only the unchanged
aggregate coverage gate is red); it predates these local observer changes.

### Native UR06 passed; UR07 remains incomplete (2026-11-10)

Run [42761268874](https://github.com/kimak-irmagi/sqlrs/actions/runs/42761268874)
uses exact observer `370225fc7759f899b3e46f0ac367f68fe1b59f8e`. Native
`TestDiagnosticRawSyscallSmoke` passes (0.08 s): actual filtered raw output, exact
child/arguments, independent ENOSYS, measured decoding and private disposal.
The two ordinary members pass their full DB/cache/cleanup oracles. Traced member
3 (v1) still rejects `trace_8 / pending_entry / other / unknown_syscall / other`,
last stage `oracle`, cleanup boundary `unknown`. Native positive compatibility
does not prove the workload's failing record matches the six-numeric-argument
subset; its argument shape/value is not in this report. No new grammar is inferred.

The original incomplete JSON is retained
[unchanged](issue89-diagnostic-evidence-2026-11-10/42761268874/jhipster-diagnostic.json),
SHA256 `bedbd5ec577dae5afa3026b4cd4cbb4f6d51fe20af440ffee1e97bc38befdb97`.
Source CI [42761251963](https://github.com/kimak-irmagi/sqlrs/actions/runs/42761251963)
passes all regression jobs; only the unchanged aggregate coverage gate is red.
No successful eight-member collection, exact product cause or speed acceptance
is claimed. At that checkpoint further detail required separate approval; the
component amendment is now approved, with the separate inventory below pending.

## Proposed RD01–RD07: fixed raw-rejection detail

Status: the component amendment is explicitly approved by @evilguest on
2026-11-10 (exact reply time unavailable). This separate inventory has undergone
critical and repeat review and was explicitly approved by the subsequent user
reply on 2026-11-10 (exact time unavailable). Repeated old-test audit found no
contradictions. No authorisation to change old tests, accepted syntax, binaries or P01b.

### Closed expectations and unavailable values

Use the four exact allowlists in the approved design. Name family `numeric`
requires the bounded lowercase UR01 name, `fallback` is exactly `???`; other
names are `other`, never copied identifiers. Known-but-invalid numeric-name
spellings do not become valid raw calls through this observation.

Count/kind pairs are exactly: `empty/none`; each of `short`, `six`, `extra`
with `numeric` or `other`; `none/unavailable`; `none/none`. Empty means an
unambiguously empty argument region, not unreadable evidence. Missing/ambiguous
regions remain `none/none`, even when commas appear inside quoted/nested text.
The exact `<unavailable>` region is not a list and has no arity. Horizontal
formatting is allowed only where UR01 already allows it. Counts are closed
classes, not the number of bytes/characters or a numeric syscall payload.

The unavailable tuple has all four values `none`. Invalid/missing fields or an
incompatible tuple reconstruct to that tuple, not a partly trusted stronger
claim. Independently known shape with no precise parser stage uses `other` for
stage; a stage is never inferred just from arity/kind. A non-none tuple requires
an observed name family and a real failure (`name`, `framing`, `arity`, `argument`,
`result`, `duration` or `other`). Validate the count/kind combinations on both
sides; unavailable name/stage cannot authenticate the other fields.

### Test groups

1. **RD01 — independent shape matrix and real parser stages.** Hand-authored
   expected labels for numeric/fallback/other names; empty, one and five slots,
   six, seven and many slots; zero/max raw hex values, nonzero decimal/mixed
   tokens, the exact unavailable marker and similar-looking strings. Cover
   complete/unfinished forms, horizontal alignment and missing/ambiguous regions.
   Quotes/nesting/private commas, arbitrary identifiers, oversized/uppercase
   numeric-name spellings and embedded marker decoys never expose tokens or
   manufacture a six-argument claim. Exercise each reachable real failure stage
   with the actual raw parser: invalid name, frame, arity, argument, return and
   overflow-checked duration. Assert its original code and fixed Error text;
   invalid duration syntax remains its actual rejection stage, not a guessed
   overflow diagnosis. Valid grammar creates no failure detail. Literal labels
   are specified independently, not computed by the helper being tested.
2. **RD02 — original pending error survives to the exact terminal event.** Use
   the actual measured decoder for `???(` with empty arguments, a short numeric
   tuple, old PX02's decimal/incomplete tuple, and unavailable/extra arguments;
   natural/signal terminal variants. Require the original raw-failure labels,
   exact old code 8/pending_entry/other/prefix, and no intervals. In particular,
   arity must not turn into framing after the unfinished suffix was removed.
   Interleave different PIDs and choose distinct failures: only the terminal
   owner's saved metadata may be returned. Complete raw failures and malformed
   matched resumes carry their real parser stage; orphan/wrong-PID/name resume,
   read/quota/CR/time/missing-terminal failures do not fabricate raw-parser
   metadata. Keep strict/default/cutoff errors and all existing UR negatives.
3. **RD03 — Go accessor, privacy and unavailable state.** Real and wrapped
   errors retain errors.As/code/context/prefix and the four independent labels.
   Nil, unrelated/legacy errors, unavailable metadata, foreign enum values and
   incompatible count/kind/name/stage tuples yield the all-none tuple. Check
   every allowed value/pair independently, including the meaningful distinction
   `empty/none` versus `none/none`. Fixed Error, formatted error, metadata JSON
   and marker formatting contain no private program/argument/PID/errno/hex
   sentinels, numeric payload, raw length, hash or source substring. The error
   value stores only enums/cause, not a retained raw-record string. No successful
   interval/count changes; generic errors stay private.
   Privacy assertions on Error/formatting apply to errors created by this parser/
   decoder. Caller-supplied foreign errors are inspected only through the safe
   accessor; do not require rewriting an arbitrary caller's Error string.
4. **RD04 — separate Node reconstruction and old exact shapes.** Independent
   Go-style marker fixtures for every allowed field/pair and unavailable/default
   tuple. Reject foreign names, enum suffixes, missing fields, embedded fake
   markers and incompatible tuples to all-none without echoing input. Extra
   object fields continue to be dropped under DG09, not a new rejection policy.
   Preserve deep-equal old CX's five fields and PX's two fields with/without the
   new marker; old reports are unchanged. New helper returns only the four new
   fields. Include privacy sentinels and wrapped Go-style log prefixes; labels
   are read only from the exact fixed marker, not a substring in a quoted value.
   Conflicting duplicate markers are unavailable, not an arbitrary first match.
5. **RD05 — real private pipe and report-boundary equivalence.** Feed bounded
   hand-authored raw lifecycles through the actual private reader/finishMeasured
   and inspect genuine returned metadata. Format the separate marker from those
   enums and compare an independent literal line; RD04 separately consumes that
   line grammar. Keep strict finish rejecting and unavailable. Malformed/read/
   missing-exit controls return no partial intervals. Distinguish this small
   boundary proof from the full Go-to-Node Actions transport required by RD07;
   no fake test is presented as a native workload run.
6. **RD06 — actual guarded wiring, no old-helper rewrite.** Go logs the new
   marker only after a trace error with old `other` shape and `unknown_syscall`
   prefix, using only the metadata accessor. Node adds its separate helper only
   in the same failed unknown-raw case, before persist/reject. Preserve the
   literal old CX/PX calls/guard and success path so old source/shape assertions
   remain valid. Ordinary exec, other unknown record types, strict/generic
   failures, successful samples and reports without markers gain no fabricated
   detail. Source checks supplement RD02/RD05/RD07 rather than substitute for
   real parsing. No new reading, flags, pins or raw-output artifact.
7. **RD07 — full small checks and original native protocol.** Full Linux small
   Go tests/vet, Node tests, script/workflow syntax and per-line ordinary-helper
   coverage (target 100%, minimum 95%); any necessary coverage iteration needs
   a separate requirement-based approval. Explicitly launch a pinned observer
   on Actions only: preserve old smoke plus UR06, then the original eight serial
   members and original v1/v2 binary hashes, independent DB/cache/CLI oracles
   and exact cleanup. If raw rejection recurs, retain the unchanged incomplete
   report with the four closed fields; missing detail is a failed observation,
   not permission to ignore a record. Retain previous report bytes. No laptop
   workload, changed raw-call acceptance, product fix or P01b waiver. Metadata
   may still leave uncertainty; do not infer a product cause from it alone.

### Critical and repeat review

Corrections incorporated before submission:

- P1 saved arity failure became a different failure after trimming: RD02 requires
  original enum-only metadata and a case that would otherwise become framing.
- P1 empty/unavailable/ambiguous arguments were conflated or quoted commas
  counted: explicit count/kind pairs and RD01/RD03/RD04 negative controls.
- P1 classification invented a validation stage or admitted a record: RD01
  exercises real parser failures; RD02/RD05 require unchanged error/no intervals.
- P1 enums were checked separately but combined into a false stronger claim:
  both accessors require coherent tuples, otherwise all-none; stage is not
  synthesised from shape and the original parser is tested independently.
- P1 new marker leaked values or replaced old exact metadata: RD03/RD04 check
  both serialization boundaries; RD06 retains separate helpers/old guards.
- P1 privacy checks accidentally demanded rewriting foreign caller errors:
  RD03 limits Error-string assertions to parser-owned errors and tests arbitrary
  foreign errors only through the safe accessor.
- P2 source matching or a passing synthetic probe replaced real evidence:
  RD05 uses the private transport, RD07 requires the original native protocol and
  distinguishes partial collection from speed acceptance.

Repeat review found no remaining P0–P2 within this bounded inventory. Preliminary
old-test audit found no necessary changes: CX/PX exact shapes and their literal
wiring remain separate, strict DG/EU and all UR negative cases still fail as
before. Re-run the audit after approval; ask separately about any contradiction.

### RD small execution checkpoint (2026-11-10)

After test approval, the first Go build/Node run failed on absent detail helpers
and export. Implementation then passed the full small Linux Go suite and vet:
**703/714 statements (98.46%)**, profile `.tmp/performance/diag-rd-full.out`;
all new detail helpers are **100%**. The 11 uncovered statements are the previous
10 plus the raw-parser missing-opening-parenthesis rejection; that is a real
framing requirement, not dead code. No further coverage iteration/removal was
undertaken. All **43 Node tests** pass, five ordinary helpers **100% lines /
97.08% branches**; script/workflow syntax and diff checks pass. Original
PX02/CX/EU/UR assertions and literal old guards remain intact. The typed error
and pending state keep only closed detail from the original failed parse;
metadata construction never authorises raw input or creates successful intervals.
Native RD07 remains required: small checks do not prove report transport in the
real failing workload, complete collection or the product's regression cause.

### Native RD07 proves the rejected shape, collection still incomplete

Run [42772724450](https://github.com/kimak-irmagi/sqlrs/actions/runs/42772724450)
uses exact observer `c8643b7d3a8a21ed76f4d71b611a73b1041be3a4`. Setup, both
native probes and the two plain members pass. Traced v1 member 3 rejects the
same pending entry, now with actual closed detail:
`fallback / empty / none / arity`. Thus the failed raw parse saw `???(` with
an empty argument region, not six decoded numeric arguments. Its original arity
failure survived to the same PID's terminal record. The metadata does not prove
the absent syscall number, the kernel cause, a successful JVM, the absence of
intervening same-PID events or successful oracles for the failed traced member.

The original incomplete report is retained
[unchanged](issue89-diagnostic-evidence-2026-11-10/42772724450/jhipster-diagnostic.json),
SHA256 `6b3962a8a3648c9fdd37313a6c9322a812612cc86c5130f3ca45d187c01d944c`.
CI [42772612811](https://github.com/kimak-irmagi/sqlrs/actions/runs/42772612811)
passes all regression jobs, only unchanged aggregate coverage red. This verifies
real Go→Node→artifact metadata transport, not complete diagnostic collection or
product speed acceptance. A narrow unavailable-entry component rule is now
separately approved; no new acceptance or old-test change is implemented under
RD01–RD07.

## Proposed UE01–UE07: terminal unavailable entry

Status: component boundary, this reviewed inventory and, separately, the two
named old-test amendments explicitly approved on 2026-11-10 before tests/code.
Repeated audit found only those two contradictions. No product optimisation,
schema change or P01b waiver.

1. **UE01 — exact form and separate raw grammar.** Measured decoding accepts
   the exact `???( <unfinished ...>` and horizontal-space/tab variants only
   with a valid terminal. Reject no separating whitespace, numeric/other name,
   arguments (including a single zero), `<unavailable>`, closing parenthesis,
   result, extra prefix/suffix, newline/control-character injection and CR.
   Exercise rejection through the decoder, not just a pattern helper. Keep
   parseDiagnosticRaw rejecting the same empty entry with its existing arity
   code/detail: recognition is a separate pending kind, not a zero-argument call.
2. **UE02 — exact next same-PID terminal.** Independently specified histories
   accept natural exit 0/nonzero and signal/core-dump terminal forms already
   permitted by the decoder. Different PIDs may interleave, including another
   unavailable entry with its own exit. Another PID's exit alone cannot complete
   this entry. Reject each same-PID intermediate event: another unavailable
   entry, valid raw call, exec, resume, result or unknown marker, even if followed
   by an exact exit. An already pending raw/exec attempt must also reject a new
   unavailable entry. Reject malformed terminal, reverse time, missing exit,
   read failure, line/record quota and unfinished EOF. Each failure returns no
   partial intervals, including when an earlier independent PID already closed.
3. **UE03 — independent literal timeline expectations.** Check no previous
   image; previous confirmed Java/Docker image ending at unavailable entry;
   previous unattributed time retaining its earlier start without a duplicate;
   engine PID excluded but invalid engine history still rejected; zero duration;
   and a later lifecycle reusing a PID without inheriting a pending tag/image.
   Assert exact category/start/end interval lists, not only total time. The
   unavailable segment never becomes host.java, exec.unconfirmed or a decoded
   syscall latency. PID time ordering and old known-exec accounting remain.
4. **UE04 — API windows, union, counts and report privacy.** Feed UE03's decoded
   intervals through the actual Go summary into independent expected windows:
   before/inside/after boundaries, clipping, overlap of different PIDs and zero
   length. Check exact covered/residual time and displayed-interval Commands
   semantics, not an inferred successful JVM count. Separately parse a literal
   valid Node report with the existing process.unattributed category; independent
   CLI-start counters are unchanged. Reject wrong totals/bounds/category/count;
   drop injected private extras under unchanged DG09. No new report field or
   metadata marker is needed on success, and no raw text/PID/argument escapes.
5. **UE05 — actual private transport and incomplete reads.** Hand-authored
   histories use the real pipe reader and finishMeasured: valid exact entry/exit
   yields UE03's literal interval; missing exit, intermediate same-PID event,
   reader failure and quotas yield no partial result. Check reader termination,
   descriptor closure at the existing finish/abort ownership boundary and
   private-file removal by the owning root cleanup, not a mock successful read.
   Do not require finish itself to unlink its retained private file.
   The same valid fixture through strict
   finish still fails with its old code and unavailable metadata. Preserve the
   bounded transport/lifecycle tests; do not create a real strace workload locally.
6. **UE06 — old guards and bounded scope.** Run all existing DG/EU/CX/PX/UR/RD
   controls, including strict/default/cutoff tests, numeric parser negatives and
   the empty direct-parser failure. Only OT-UE01/OT-UE02 below may change after
   separate approval. Check measured wiring remains explicit, trace flags/pins,
   native smoke and UR06 unchanged; source assertions supplement behavioural
   UE01–UE05, not replace them. Re-audit contradictions after approval and ask
   separately about any additional change. No nondeterministic native generation
   of a register-read failure is required or advertised as proved by a unit test.
7. **UE07 — complete small verification and unchanged native protocol.** Run
   full small Linux Go/vet, all Node tests, script/workflow syntax and diff checks;
   measure ordinary helpers per line (target 100%, minimum 95%). A further
   coverage iteration requires its own requirement-based plan/approval. Then
   explicitly launch the pinned observer in Actions only: unchanged smoke/UR06,
   all eight original serial members, fixed product hashes, DB/cache/CLI oracles
   and exact group/resource cleanup. Retain all earlier reports byte-for-byte.
   Preserve any new incomplete report; do not skip unsupported events to finish.
   Acceptance requires all eight members and disclosed observation cost/noise;
   materially unattributed time prevents an unsupported precise product-cause
   claim. Observer success alone never qualifies product speed or closes #89.

### Separately proposed old-test amendments

Only `test/performance/liquibase/diagnostic_raw_detail_linux_test.go`:

- **OT-UE01:** in TestDiagnosticRawDetailPending, replace only the first table
  row `{"???(", "fallback", "empty", "none"}` with
  `{"???(0", "fallback", "short", "numeric"}`. Keep arity failure code/context,
  all three terminal variants, strict code 5, every other row and assertions.
  UE01–UE03 provide the separately specified positive empty-entry controls.
- **OT-UE02:** in TestDiagnosticRawDetailPrivatePipe, replace only its shared
  `???( <unfinished ...>` fixture with `???(0 <unfinished ...>` and measured
  marker expectation `fallback empty none arity` with
  `fallback short numeric arity`. Keep measured failure, strict/missing-exit,
  actual pipe and all marker/privacy assertions. UE05 separately tests successful
  transport of the exact empty entry; RD05 remains a real rejection test.

Do not alter TestDiagnosticRawDetailParserStages, Shape, interleaved numeric
failure, EOF/CR/time/quota controls, Node metadata fixtures or UR/PX negatives.
Pure shape/metadata tests may still legitimately describe fallback/empty/arity;
they do not authorise decoder acceptance. Existing RD01–RD07 execution evidence
is historical, not retroactively declared passing under a new observer.

### Critical and repeat review

Corrections incorporated before submission:

- P1 accepting a broad empty call: UE01 tests full-body near-misses through the
  decoder and keeps direct numeric parsing rejected.
- P1 another PID's exit or intervening event silently closes the entry: UE02
  covers both directions of pending-kind collisions and each same-PID event.
- P1 duplicate time or a fabricated successful JVM: literal UE03 intervals,
  independently calculated UE04 union/windows and separate CLI-start counters.
- P1 incomplete streams leak earlier results: failures after a closed independent
  lifecycle still return no partial intervals; UE05 checks the real transport.
- P2 synthetic/native evidence conflation: UE06 explicitly avoids requiring a
  nondeterministic native read failure; UE07 retains the unchanged workload gate.
- P2 silently weakened RD controls: only two named fixtures change, separately
  approved; direct raw parsing and other old negative controls remain intact.

Repeat review found no remaining P0–P2 in this bounded inventory. This is a test
proposal, not a claim that code, coverage or the native protocol already passes.

### UE small execution checkpoint (2026-11-10)

After both approvals, repeated audit confirmed only OT-UE01/OT-UE02 needed
changes; their exact three-line fixture/marker diff preserves the other tests.
Tests were written first. The first Linux run of the unchanged decoder failed
all five new Go test groups on their positive paths (strict and negative private
transport cases passed); the new Node report check already passed, as expected
for the existing report category. Only then was the measured decoder changed.

Full small Linux Go and vet now pass. Profile
`.tmp/performance/diag-UE-full.out`: **715/726 statements (98.48%)**; all 12
added ordinary-helper statements are covered. The same 11 old uncovered
statements remain: journal read errors (5), pipe construction/group termination
(3), direct raw-parser framing (1), timestamp (1), exec return rejection (1).
These are existing validation/failure requirements, not demonstrated dead code;
no further iteration or removal was undertaken. All **44 Node tests** pass,
five ordinary helpers **100% lines / 97.10% branches**. These denominators do
not cover native orchestration or product code. Syntax/workflow/diff checks
pass. No product bytes, trace flags, old smoke or UR06 changed. Native UE07
remains open; small checks do not establish workload compatibility or a cause.

### Native UE07: trace accepted, time summary refused

Run [42782782114](https://github.com/kimak-irmagi/sqlrs/actions/runs/42782782114)
uses observer `39756f766d07335caed9e3db3f4e17a343a36393`. Preparation, both
native probes and two plain members pass. Traced v1 member 3 fails with
`diagnosticFailure=clock`, `traceLocation=none`, `traceShape=none`, last stage
`oracle`, cleanup evidence unavailable. Unlike the prior trace_8, this marker
is emitted only after finishMeasured returned no trace error. Thus this run
passed decoding but failed interval summarisation; no successful traced sample,
complete cohort, successful cleanup or product cause is claimed.

The original [incomplete report](issue89-diagnostic-evidence-2026-11-10/42782782114/jhipster-diagnostic.json)
is retained byte-for-byte, SHA256
`383a8bd5e016defacd01ee63c349ffd7b65c15c5e5f16b48ae9928097bcd3d9f`.
Both fixed product hashes are unchanged. Plain uncached/cached elapsed values
are v1 50084.888471/8680.393956 ms and v2 60853.474780/10813.360001 ms;
these diagnostic members are not a new acceptance cohort. All regression jobs
of CI [42782733993](https://github.com/kimak-irmagi/sqlrs/actions/runs/42782733993)
pass; only unchanged aggregate coverage is red. UE07 remains incomplete.

The summary's clock error currently conflates wall/elapsed ordering, unequal
wall and monotonic spans, and reversed decoded intervals. Current evidence
cannot distinguish them. A separate component proposal records all these
branches without changing rejection; UE approval does not authorise that change.

## Proposed CW01–CW07: closed clock rejection context

Status: components explicitly approved on 2026-11-10; this critically and
repeatedly reviewed test inventory awaits separate approval. No new tests/code
or old-test changes yet. This identifies observer refusal, not a product cause.

1. **CW01 — actual window checks and precedence.** Test the approved pure
   validateDiagnosticWindow helper with independent inputs and literal expected
   reasons: equal/reversed wall endpoints → wall_order; increasing wall with
   zero/negative elapsed → elapsed_order; positive unequal spans in either
   direction, including a one-nanosecond difference → duration_mismatch;
   exact positive equality → success. Combine invalid conditions to assert the
   original short-circuit precedence. Check errors.Is(err, errDiagnosticTrace)
   and the unchanged fixed Error. Equal positive spans above 12 minutes still
   succeed: the metadata ceiling must not become a new acceptance limit.
2. **CW02 — real summary integration and preserved outcomes.** Exercise actual
   summarizeDiagnosticIntervals with Unix-based windows: equal/reversed window,
   reversed decoded interval after a valid window and simultaneous bad window/
   bad interval. Assert the independently expected reason and precedence. Valid
   clipped/overlapping, empty and zero-length work retains exact old covered/
   residual/Commands results, with no failure metadata. A time.Now value and
   its Add-derived endpoint exercise the actual public boundary without assuming
   host drift or sleep timing; no unsafe/fabricated time.Time layout. CW01 proves
   independent elapsed/mismatch branches; source wiring plus these real calls
   checks their integration, not an impossible forced wall/monotonic fixture.
   Summary errors remain errors and cannot produce a publishable success.
3. **CW03 — Go metadata, bounds and privacy.** Inspect real CW01/CW02 errors,
   errors wrapped with %w and errors.As/Is. Assert valid reason and exact relative
   spans; wall_order/elapsed_order have unavailable pairs, duration_mismatch has
   differing positive spans, interval_order has equal positive spans. Test one
   nanosecond, exact ceiling 720000000000 ns and above-ceiling spans. For genuine
   errors whose spans cannot be published, retain the known reason and withhold
   both values. Nil/legacy/foreign errors give none plus an unavailable pair;
   invented reasons and incoherent metadata must not be trusted. Never expose a
   half pair, non-positive/out-of-range/overflowing number, mismatch with equal
   spans or interval_order with unequal spans. Error/format/JSON/marker tests
   use independent privacy sentinels and bounded expected values; the owned
   error stores no calendar endpoint, interval/PID, time.Time or raw text.
   Do not require rewriting arbitrary foreign errors' Error strings: inspect
   them only through the safe accessor, as in RD03.
4. **CW04 — separate Node parsing and unchanged old shapes.** Independently
   specified fixed marker lines cover all four reasons, unavailable pair, exact
   integer ceiling and coherent present pairs. Parse only full marker lines,
   including the normal Go file:line prefix. Missing/unknown reasons, missing
   tokens, half pair, fractions/NaN/infinity/negative/too-large integers,
   inconsistent reason/pair, quoted embedded markers and extra source tokens
   yield unavailable metadata without echoing input. Identical duplicate lines
   may agree; conflicting lines must not pick an arbitrary first result. Compare
   literal three-field output with null unavailable durations. Existing CX's
   five fields, PX's two and RD's four are deep-equal with/without the new line;
   successful report reconstruction and DG09 field dropping remain unchanged.
5. **CW05 — real error-to-marker boundary and guarded publication.** Format
   actual CW01/CW02 error metadata into the fixed Go marker and compare literal
   reason/span or unavailable lines; CW04 consumes independent literals of the
   same grammar. Preserve the old clock marker. Check the collector obtains
   metadata only from the summary error, and the Node runner calls its separate
   helper only when diagnosticFailure is clock, before persist/reject. Existing
   CX/PX/RD literal wiring stays intact. Successful summaries, trace/journal/
   transport errors and reports without the marker gain no fabricated clock
   reason or durations. No raw buffer, timestamps or interval values are copied
   into the report. Source guards supplement actual errors and native CW07;
   they do not alone prove native transport or successful cleanup.
6. **CW06 — old regressions and complete small verification.** Preserve all
   DG/EU/CX/PX/UR/RD/UE checks, default/strict/cutoff behaviour, time sources,
   fixed binaries/trace flags, real smoke/UR06 and eight-member schedule. Full
   small Linux Go/vet, all Node tests, syntax/workflow/diff checks; per-line
   coverage of ordinary helpers, target 100%, minimum 95%. If another coverage
   iteration is needed, obtain a requirement-based plan/approval first. No old
   test changes are proposed; re-audit after approval and ask separately about
   any contradiction. No product workload or clock manipulation on the laptop.
7. **CW07 — unchanged native attempt and retained evidence.** Explicitly launch
   the approved observer in Actions: setup/smoke/UR06, then the original eight
   serial members on both original executable hashes with independent DB/cache/
   CLI oracles and exact cleanup. If clock recurs, the original incomplete
   artifact must carry the actual known reason and coherent bounded pair or
   explicit unavailability; no missing fields may be called proof of a cause.
   If it does not recur, do not claim native failure transport was demonstrated;
   require all eight and disclose observer cost/noise for complete collection.
   Retain old and new original artifacts byte-for-byte. Do not retry unchanged
   instrumentation after a proved new blocker, skip members or relax refusal.
   Any clock mapping/fix needs separate design; product P01b remains blocking.

### Critical and repeat review

Corrections incorporated before submission:

- P1 testing only an accessor/fabricated error misses a real rejection: CW01
  exercises the actual pure validator; CW02 covers the real summary loop and
  precedence; CW05/CW07 distinguish small publication from native transport.
- P1 independently forced time.Time layouts or assumed host drift make the
  suite unsafe/flaky: use the approved scalar boundary and deterministic Add,
  with no sleep threshold, layout trick or host-clock modification.
- P1 metadata bounds silently change acceptance: CW01 explicitly succeeds
  beyond the ceiling; CW03 tests reason retained while the pair is withheld.
- P1 two individually valid durations imply an impossible branch: CW03/CW04
  check reason/pair coherence, all-or-none values and exact integer bounds.
- P1 numeric evidence leaks original readings or arbitrary error text: relative
  spans only, private sentinels at both boundaries and foreign-error inspection
  limited to the safe accessor; fixed error cause/semantics preserved.
- P2 additive metadata breaks old exact contexts: CW04 deep-equal old shapes,
  CW05 separate helpers/guards, no proposed old-test rewrites.
- P2 a nonrecurring clock error is reported as demonstrated transport or a
  complete series as a product fix: CW07 states both evidence limits explicitly.

Repeat review found no remaining P0–P2 within this bounded inventory. Preliminary
old-test audit found no required change: existing summary tests require error,
not identity equality; clock remains the same failure kind; exact CX/PX/RD
shapes and source guards stay separate. Repeat this audit after test approval.
