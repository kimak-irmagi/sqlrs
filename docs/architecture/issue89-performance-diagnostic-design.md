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
The separate EU01–EU06 inventory linked below was subsequently approved before
tests and implementation. The post-approval EU04 clarification is also approved:
retain existing safe field dropping, not a new rejection policy.

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
The explicit mode is implemented in the observer; current strict calls/tests
are unchanged. Small Go/Node regressions and Linux vet pass. Actual EU06 remains
unfinished; successful diagnostic collection and exact slowdown cause are not claimed.

## Approved: closed context for an unrecognised trace record

Status: explicitly approved by @evilguest on 2026-11-09; exact reply time
unavailable. The separate CX01–CX05 inventory was subsequently approved before
tests/implementation. Context now passes small regressions/vet; actual CX05 is open.

Run `42618850460` on observer `a75316b` passed preparation, native smoke and two
plain members, then stopped on traced v1 with `trace_8`. This code currently
can mean either an unrecognised top-level record or an invalid pending entry
seen at terminal handling. No trace bytes are retained in the artifact; neither
the actual record shape nor a product cause is established by the code alone.

Approve failure-only metadata from the same already-read private record:

- A closed location enum: `record`, `pending_entry`, or `none`.
- A closed shape enum: `superseded_exec`, `exec_entry`, `exec_resume`, `other`,
  or `none`. Recognise only anchored fixed prefixes/forms, including the numeric
  shape of a superseded-exec marker; never export its PID, text or arguments.
  `other` is a limit on knowledge, not permission to discard that record.
- Preserve the existing failure code and strict rejection. Also retain the
  already-emitted closed `P01B_FAILURE_STAGE` using the same stage allowlist as
  the ordinary cohort reporter, so cleanup decoding cannot hide the primary
  workload phase. Missing/foreign stage values become `collector_exit`.
  The stage records the last reached phase; it alone does not establish a
  primary workload error (cleanup failure can follow a successful workload).
- No acceptance of a new strace syntax, new raw-data read/export, syscall flags,
  product changes or altered eight-member protocol. Any later decoder amendment
  must be justified by the new evidence, not inferred from this proposal.

Alternatives: keep retrying without additional evidence; publish a scrubbed raw
line (rejected because arguments may be private); or export only fixed context.
The last is recommended. Critical review: P1 raw substring leakage is prevented
by rebuilding fixed enums; P1 a shape label is mistaken for a valid record is
prevented by retaining rejection; P2 cleanup obscures an earlier failure is
addressed by preserving the existing phase separately. Repeat review found no
remaining P0–P2. The project and separately reviewed
[CX01–CX05 inventory](issue89-performance-diagnostic-tests.md#proposed-cx01cx05-closed-failure-context)
are approved; tests and context implementation pass small checks. No new syntax
is accepted; real failure context still needs CX05 evidence.

CX05 run `42632130376` has now proven failure-context transport:
`trace_8 / pending_entry / other`, last-reached stage `oracle`. The report is
retained in the test inventory. This does not establish a specific syntax or a
product cause; the eight-member diagnostic is still incomplete.

## Approved: structural prefix detail for the remaining other case

Status: explicit @evilguest approval recorded on 2026-11-10; exact reply timestamp
unavailable. Separate PX01–PX05 tests subsequently approved before implementation;
prefix metadata now passes small regressions/vet. Native PX05 remains open.

The new report localises the check but the approved shape enum deliberately
leaves `other` unexplained. Approve two additional failure-only closed fields
from the same already-read record, only when the existing shape is `other`:

- `tracePrefix`: `empty`, `numeric_header`, `syscall_entry`, `syscall_resume`,
  `pid_change`, `unknown_syscall`, `exit_notice`, `signal_notice`, `bracket_notice`,
  `other`, or unavailable `none`. These are fixed anchored grammatical classes,
  not a scrubbed/hashed/shortened copy of input. Strip framing whitespace only;
  never inspect a quoted argument to infer the leading form.
- `traceKnownCall`: only `execve`, `execveat`, `exit`, `exit_group`,
  `restart_syscall`, `other`, or unavailable `none`. Match a call token only at
  an anchored entry/resume position; an arbitrary syscall name maps to `other`,
  never to its actual spelling. No string/number/PID/path/argv or raw length.

Keep the original location/shape/code and strict failure. This adds observation,
not syntax acceptance or a new time interval. Missing/foreign metadata defaults
to `none`; existing reports and successful samples stay unchanged. No new read
boundary, tracing flags, measured binary, local workload or eight-member schedule.
The extra evidence may still be insufficient; state that limit rather than infer
a successful command or product cause.

Alternatives: keep the same ambiguous retry; export raw/redacted prefix text;
or add only fixed structural classes. Recommend the last. Critical review fixed
P1 argument leakage by forbidding copied strings/lengths and matching before
arguments; P1 inferred valid exec by preserving rejection; P2 accidental overriding
of existing context by keeping fields independent. Repeat review found no remaining
P0–P2. Component amendment and the separately reviewed
[PX01–PX05 inventory](issue89-performance-diagnostic-tests.md#proposed-px01px05-structural-prefix-observation)
are approved and implemented; existing strict rejection and context tests remain
unchanged. Small evidence does not establish a product cause; actual PX05 is pending.

Actual PX05 run `42665130138` reports pending_entry/other and
**unknown_syscall/other**. The original incomplete report is linked in the test
inventory. This is a raw-syscall-shaped entry being validated as exec, not proof
that the requested program launched or that a specific syscall caused slowdown.

## Approved: measured raw syscall records with unattributed process time

Status: explicitly approved by @evilguest on 2026-11-10; exact reply timestamp
unavailable. The separate UR01–UR07 inventory was subsequently approved before
tests/code. The user separately authorised the exact UR01 duration correction.
Full small Go/Node checks and Linux vet now pass; native UR06 passes in
42761268874, but UR07 still rejects an unknown pending record at member 3.

The [strace 6.8 source](https://github.com/strace/strace/blob/v6.8/src/syscall.c#L1309)
has explicit unknown-syscall formatting: a raw fallback, a numeric syscall name
when available, and fallback tracing qualifiers. Thus selecting execve/execveat
does not justify assuming every unfinished record is an exec. This explains why
the observer needs a non-exec grammar, not the original regression's exact cause.
The remote report identifies the family, not its raw numeric payload or exact
runtime strace version; do not infer those values from the reference source.
See also its [raw/default tracing flags](https://github.com/strace/strace/blob/v6.8/src/defs.h#L345).

Approved measured-only extension, retaining default strict decoding:

1. Add a separate bounded raw-call parser for exact unknown-call forms
   (`syscall_0x…` or `???`), numeric raw arguments only, no dereferenced strings.
   Define the exact Linux/amd64 grammar in the separately reviewed test inventory
   before implementation. Cover entry, matching resume, explicit return and
   no-result/terminal forms; validate the whole record, not the prefix classifier.
   Mismatched names/PIDs, malformed fields, reversed time and missing required
   completion remain failures. No arbitrary syscall identifier/line skipping.
2. A validated unknown call does not establish an executable identity or a
   successful exec. End any previous confirmed image at the unknown entry's start
   and retain **process.unattributed** until the next confirmed executable image
   or exact terminal record for that PID. Do not restart the previous category on
   an unknown return or attribute uncertainty to Java/Docker. A pending call needs
   a matching result or exact terminal event; EOF is not process absence. Engine
   lifetime remains excluded. Clip/union the same API windows without double count.
3. The new category means unattributed process time, not syscall latency or a
   successful CLI/JVM start. Commands still counts displayed intervals. Existing
   independent workload/CLI counts, DB/cache oracles and cleanup remain mandatory.
   Material unattributed time limits localisation and cannot become a claimed
   product cause. Add only the fixed category to Node's report allowlist; no raw
   syscall number, argument, PID or input text is exported.
4. Preserve old strict rejection and current PX02 invalid/raw-incomplete negative
   fixtures with their original codes. Only complete validated raw grammar gains
   a measured path; prefix metadata is never an authorisation to accept input.
   No product/API/DB changes, new trace payloads/flags or altered eight-member
   protocol. First require an additional tiny native Actions probe of an invalid
   numeric syscall on the same Ubuntu tracer; no DB/Liquibase or laptop workload.

Alternatives: retain the full observer rejection; drop unknown records (rejected
because it hides uncertainty); or parse their bounded grammar and keep time
explicitly unattributed. Recommend the last. Critical review fixed P1 prefix-only
acceptance with full grammar; P1 false executable attribution with a distinct
category/early previous-image boundary; P1 missing-exit acceptance with exact
lifecycle checks; P2 weakening old negative tests with the strict/default and
invalid-fixture controls; P2 a green observer mistaken for acceptance with the
unchanged P01b limit. Repeat review found no remaining P0–P2. The component
amendment and the separately reviewed
[UR01–UR07 inventory](issue89-performance-diagnostic-tests.md#proposed-ur01ur07-raw-syscall-grammar-and-unattributed-time)
are approved. Implementation retains strict/default parsing. Native UR06 passes,
but the eight-member collection remains incomplete. See the test checkpoint.

## Proposed: complete fixed detail of raw-grammar rejection

Status: reviewed proposal, not approved/implemented. Run 42761268874 proves that
native numeric compatibility is insufficient: the workload's unknown pending
record is still not accepted, and the current report cannot explain why. Do not
broaden grammar based on that family label alone.

For context only, [strace's entry decoder](https://github.com/strace/strace/blob/v6.8/src/syscall.c#L546)
also has a register/argument-read failure path printing an entry before arguments.
That is a possible source of a different shape, **not established by this report**.
The next observation must distinguish shapes rather than adopt that hypothesis.

Proposed component boundary, without any new accepted syntax:

1. At the existing unknown-raw rejection point, derive only four fixed enums
   from the already-read private record. `rawNameFamily`: `numeric`, `fallback`,
   `other`, `none`; `rawArgumentCount`: `empty`, `short`, `six`, `extra`, `none`
   (closed arity classes, not raw lengths/counts); `rawArgumentKind`: `numeric`,
   `unavailable`, `other`, `none`; `rawRejectStage`: `name`, `framing`, `arity`,
   `argument`, `result`, `duration`, `other`, `none`. Each has unavailable `none`.
   `short` means 1–5 comma-separated slots, `extra` more than six; classify slots
   only within an anchored argument region. `numeric` requires the existing raw
   token grammar; `unavailable` only the exact `<unavailable>` marker occupying
   that region; otherwise `other`. Unlocatable/ambiguous regions give `none`.
   Empty arguments have kind `none`; the unavailable marker is not an argument
   list and has count `none`. Nonempty slot classes permit only numeric/other
   kinds. Do not split quoted/nested ambiguous text to manufacture a slot count.
2. Keep the raw parser's real rejection stage when available; otherwise report
   `other`, never an invented more precise stage. An unfinished entry at terminal
   keeps the original pending-entry context: its missing return is not a reason
   to pretend successful exec. Derive metadata from that entry, not exit text.
   Preserve original error code, wrapped-error inspection, prefix and old context.
   Store only enums in errors; no body, syscall number, PID, tokens, errno text,
   raw length, hash or scrubbed substring. No extra reading or trace flags.
   Ownership stays in the standalone Go benchmark module: the raw parser creates
   a private enum-only failure value; pending state retains that value from the
   original parse, not a guessed stage from re-parsing a trimmed fragment. The
   existing error wrapper/accessor exposes only that value. Nothing is persisted.
3. A separate Go metadata accessor/marker and separate Node reconstruction add
   these fields only on an actual failed unknown-syscall record. Do not change
   the exact old CX/PX helper shapes, successful records, interval totals or field
   dropping. Validate enum values/combinations on both sides; foreign/missing
   values become `none`, never copied strings. A raw diagnostic failure retains
   the same report rejection, last-reached stage and cleanup evidence.
   Use a separate fixed `P01B_DIAGNOSTIC_RAW_CONTEXT` marker and Node helper in
   diagnostic.mjs; run-diagnostic.mjs remains the sanitized report owner. No new
   public API, DB table, product package or observer success path is introduced.
4. Keep all strict/default and malformed PX02/UR negatives, UR06 prerequisite,
   original binaries and eight serial members. Do not accept a previously rejected
   record, infer a successful syscall/JVM, discard a trace, export raw data or
   optimise the product. Test inventory is separate after component approval.

Alternatives: continue guessing grammar; retain the observer failure and choose
a different observer; or expose complete fixed rejection structure at once.
Recommend the last. Critical review: P1 token leakage — enum-only construction
before publication; P1 fabricated diagnosis — explicit unavailable/ambiguous
states and preserved parser stage; P1 widening grammar — rejection unchanged;
P2 old exact helpers silently changed — separate metadata boundary; P2 smoke
mistaken for workload compatibility — retain independent native/eight-member
evidence. Repeat review found no remaining P0–P2 within this proposal. This still
may not establish a product cause; unsupported observations remain a blocker.
