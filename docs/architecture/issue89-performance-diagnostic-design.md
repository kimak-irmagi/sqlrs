# #89: isolate the confirmed performance regression

Status: approved by @evilguest on 2026-11-08 after critical and repeat review;
exact reply timestamp unavailable. DG01–DG12 and, separately, OT-DG01–OT-DG03
were subsequently approved before tests and implementation. On 2026-11-08 @evilguest
retained the speed criterion and requested exact localisation, with additional
instrumented runs if needed. Existing acceptance run `42479055486` is unchanged.

## Question and chosen boundary

The complete short cohorts prove slower v2 but contain totals, not operation
durations. Extra image probing/planning is a hypothesis, not a measured cause.
Start with a diagnostic-only JHipster run on standard Actions Ubuntu, observing
the **same exact v1/v2 executables** without new product hooks, API or schema.

Use external `strace` process-execution timestamps/lifetimes plus selected existing
SQLite journal timestamps. The [strace manual](https://man7.org/linux/man-pages/man1/strace.1.html)
documents following children, timestamped execution and process exits without
recompiling the observed program. Do not replace the Docker executable with a
wrapper: that changes subprocess/signal behaviour. Do not trace syscall payloads
such as read/write, or attach to unrelated/user processes.

## Components, flow and ownership

1. A separate explicit same-repository `performance:diagnostic` PR-label workflow
   owns a fresh Ubuntu 24.04 VM, read-only token and an independent concurrency
   group. No paid runner, laptop run, cancellation of the full run or ordinary
   push trigger. One job, 60-minute bound with an earlier cleanup deadline.
2. Checkout fixed measured source `668d505` at the workspace root. Checkout the
   exact diagnostic tooling revision under an ignored `.tmp` child. Use the
   existing preparation/warm-up procedure against the fixed source. The approved
   clarification below allows the observer's preparation script with an explicit
   first-checkout source root; require candidate
   SHA256 `1367bce9ddaa44c0aa378b07c015dfb20a4c0d29f00d2f5a893905686fae14b0`
   and the existing pinned v1 SHA256. Compile the observer separately; only its
   private collector executable is replaced. No measured source/artifact patch.
   Audit found the original Linux v1 observer hashes `liquibase.bat` while the
   actual native request executes `liquibase`. Diagnostic provenance must use
   the actual selected executable script, verify it against the pinned extracted
   distribution and never describe its hash as the Windows launcher. Preserve
   original collected samples unchanged; derived JVM totals remain provisional
   until the actual script is verified. Separate tests cover this observer fix.
3. Extend only the external collector with an explicit diagnostic mode. Launch
   the exact engine under `strace` for traced members, otherwise unchanged. Both
   remain in the owned process group. Keep native API, full DB/cache oracles,
   event counters, closure and exact cleanup; the existing untraced route remains
   the default and its old assertions are not weakened.
   The launcher PID is strace in traced mode: independently verify the published
   engine PID is its live direct child in the same owned process group, running
   the exact selected executable, before trusting the loopback endpoint/token.
   Preserve the current direct PID equality in untraced mode. A separate named
   test-fixture amendment is required; never merely drop the ownership check.
4. Run two untraced v1/v2 pairs and two traced v1/v2 pairs, reversing order in the
   second pair of each kind; every member has fresh-store/cached passes. Compare
   like modes, report tracing overhead separately. This is localisation, not
   another acceptance cohort or authority to waive/redefine P01b.
5. Trace exec/process exit only, with timestamps. Private files are mode 0600
   under the nonce-owned collector root, capped at 32 MiB and a fixed row/line
   bound. Acquire trace through a dedicated pipe and a capped writer, not a
   periodically checked unbounded file; join the reader on shutdown. Terminate
   the owned group on overflow/deadline. Raw argv may contain
   credentials/SQL: never print/upload traces, stdout/stderr or DB/config files.
6. Pure ordinary Go helpers decode the bounded trace, correlate successful exec
   and terminal records by PID, reject truncated/ambiguous evidence and produce
   closed categories: Docker inspect/create/start/stop/remove/copy, image probe,
   planner/step invocation, PostgreSQL control/identity check, host Liquibase,
   host Java and other. No raw argv, paths, container/job IDs or messages escape.
   A successful re-exec of the same PID closes the preceding executable segment;
   failed exec attempts are not successful launches. Exclude the engine's own
   lifetime from subprocess cost. The host trace does not see container-internal
   JVMs: their synchronous Docker exec interval is observed, not invented child
   process telemetry.
7. Outside timed intervals read only allowlisted timestamps/ordinals from the
   existing job/task/planning/session/invocation journal. Do not export SQL,
   request_json, events' messages, DSNs or instance identities. Emit coarse job
   phases and step ordinals relative to each request, documenting version-specific
   missing boundaries rather than treating missing time as zero.
8. The diagnostic runner validates fixed binaries and ordinary workload results,
   uploads only typed numeric/categorical JSON to its own report directory.
   Include trace/source/version identity, pass/mode, API elapsed time, observed
   subprocess intervals/counts and journal phase intervals. Keep nested parent/
   child intervals distinct: never sum overlapping commands as exclusive time.
   Report uncovered/residual time; an unexplained residual is not a claimed cause.

The collector joins the traced engine/process tree before final trace parsing,
then disposes private files through the existing exact-root protocol. Tracer exit
alone is not proof of engine exit. Failed attach/parse/cleanup, clock regression,
unmatched completed command or contradictory counters leaves diagnosis incomplete.
No hard runner termination is reported as successful cleanup.

If this identifies only a broad phase (for example binding/network or SQLite),
report that limit and propose a narrower next observation; do not claim a precise
cause from source speculation. An actual product fix is proposed separately,
with requirement-backed tests before implementation.

## Critical and repeat review

- P1: observer code silently changes the candidate. Fixed source and both actual
  executable hashes are mandatory; observer build/data have separate ownership.
- P1: tracing leaks secrets or changes qualification. Closed parsed output only;
  private bounded files; traced data are diagnostic, never mixed into P01b.
- P1: strace exits while the engine survives. Existing group ownership plus exact
  runtime cleanup is retained and tested; no daemonising tracer flags.
- P1: the existing endpoint PID assertion accidentally accepts a foreign service
  or rejects strace's real child. Traced mode has a separate exact /proc child/
  process-group/executable proof; untraced equality is unchanged.
- P1: overlapping Java/shell/Docker intervals are counted twice. Retain intervals
  and union coverage, not a fictitious additive cost breakdown or unobserved
  process ancestry.
- P2: absent/failed process observation looks like zero overhead. Bounded parser,
  ordinal/counter reconciliation and explicit incomplete/residual outcomes.

Repeat review found no remaining P0–P2 in this component boundary. No old-test
change is authorised by this design alone. The separate reviewed test list and
three explicit collector amendments have since been approved.

## Approved clarification: acquire the same archive through Releases API

Status: approved by @evilguest on 2026-11-09; exact reply time unavailable.
TA01–TA06 were separately approved before tests/code. Preparation is implemented
and passes small tests; its actual Actions retry is not complete yet. Diagnostic run
`42580413674` failed three times before measurements: its browser download URL
returned HTTP 504 through both Node and curl. Unchanged retries have stopped;
strace/the eight members never ran and no diagnostic report was produced.
The official API for the same asset `583092383` returned 16,798,404 bytes with
exact SHA256 `da6132c95185f2a11561628458d34b22e7cb928c574d8ecec9f06d44fc8d0f01`.
The binary was not executed. This is preparation transport failure, not diagnosis.

Approved preparation boundary only:

1. `download.mjs` retains bounded retries and mandatory byte verification. After
   the existing Node/curl transports fail, allow the official GitHub Releases API
   with fixed asset ID and `Accept: application/octet-stream`. No user-provided
   URL/version/token. Successful wrong bytes, including JSON/HTML, always fail;
   checksum failure must never enable another bypass.
2. Updated `prepare.mjs` runs from the separately pinned observer checkout but
   accepts an explicit absolute source root. Diagnostics allow only the first
   checkout `668d505`, equal to `GITHUB_WORKSPACE`. Clean HEAD, unchanged build
   commands/compiler, exact old v2 SHA256, v1 archive/metadata/executable hash,
   JDBC and release-tag checks remain mandatory. No product files are modified.
3. Diagnostics use the same composite setup action from the observer checkout:
   it runs its own `prepare.mjs` with that fixed source root. This replaces the
   requirement to execute the old preparation script itself, not the measured
   sources/binaries, and avoids a duplicate build workflow. Without an explicit
   root, ordinary setup retains its own checkout selection. The old acceptance
   workflow and already collected cohorts remain unchanged.
4. Mandatory small tests/strace, eight serial members and proven cleanup follow
   preparation. Budget, read-only token, private reports and off-laptop workload
   restrictions remain unchanged.

Alternatives: wait only for the browser URL, rebuild v1, patch the checked-out
source, or obtain identical verified bytes through the official API. The last
was selected. None of the three failed attempts measured anything or becomes
a successful sample retrospectively.

Critical review: P1 observer tooling substitutes the product — explicit source
root/clean HEAD and both original executable hashes remain mandatory; P1 JSON
or wrong bytes enable a bypass — byte verification is mandatory and final;
P2 duplicated setup/global transport hooks — reuse the same action, with no
fetch interception, source patch or expanded permission. Repeat review found no
remaining P0–P2. Exact acquisition/root checks TA01–TA06 are approved, and the
repeated old-test audit found no contradictions. No old test changed; TA06
remains the actual remote stage.

## Approved: terminal exec without a confirmed result

Status: explicitly approved by @evilguest on 2026-11-09; exact reply time unavailable.
The separate EU01–EU06 inventory linked below still needs approval before tests
or implementation are written.

2026-11-09: retry `42612253733` again failed at member 3 with `trace_5`.
Its exec attempt started no later than the final API boundary, so the narrow
late-cleanup exception correctly did not apply. This does not establish that
the record is a migration application or that any specific program was launched.

Approve an explicit measured-decoding mode only:

- Given an observed exec entry and exact terminal record for the same process,
  but no exec result, retain **exec.unconfirmed** from attempt start to terminal
  record. No program name, PID or argv is exported. The preceding confirmed
  image ends at attempt start; its identity cannot cover the uncertain span.
- This interval is not a successful CLI/JVM start. Existing independent CLI
  counts, DB/cache oracles and cleanup remain mandatory. `Commands` continues
  to count displayed intervals, not successful CLIs; the distinct category keeps
  uncertainty explicit instead of falsely attributing it to Java/Docker.
- Default strict decoding and old negative tests remain. Corrupt/truncated
  streams, missing terminal records, unrelated PIDs, reversed time and unproved
  cleanup still fail. No skipping lines or treating EOF as proof of process exit.
- Material unexplained cost in these intervals limits analysis: it cannot be
  named as a specific regression cause. No product/API/schema/SQL-replay authority
  changes, new read boundary or raw-data export.

Alternative: retain full rejection and separately design another observation
method. Recommend the explicit uncertain interval: distinguish known duration
from unknown execution result honestly. Critical review addressed P1 false
success/attribution and P1 corrupt-stream acceptance with the stated limits;
repeat review found no additional P0–P2. The decision is approved; see separate
[EU01–EU06 tests](issue89-performance-diagnostic-tests.md#proposed-eu01eu06-unconfirmed-exec).
This mode is not implemented and current strict checks have not been weakened.
