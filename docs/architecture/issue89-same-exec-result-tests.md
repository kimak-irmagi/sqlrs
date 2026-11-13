# #89 O7: same-exec result transport — final test proposal

Status: IR01–IR10, exact format and limited OT-IR01–OT-IR08 approved by
@evilguest on 2026-11-12 after critical/repeat review. O7 components
are approved in [measured overhead reduction](issue89-measured-overhead-reduction.md).
This fixes the wire/quota details reserved for the final inventory. Tests-first
implementation is beginning; O5/O6 remain separate.

Implementation status, 2026-11-12: O7 code and approved OT-IR01–OT-IR08 are now
present. Ordinary runtime/observer checks have run; hosted native execution,
combined O5–O7 campaign and speed acceptance have not. Detailed evidence follows.

## Exact transport and observation contract

Runtime explicitly supplies `SQLRS_CAPTURE_RESULT=same-exec-v1`; an absent/empty
value keeps legacy direct-helper behavior. Unknown mode is refused before child
execution. `serve` and `probe-prune` remain non-SQL paths. Current helper bytes/
digest/config/readiness bind the capability; old evidence is not reinterpreted.

Binary stdout, in this exact order (LF headers, no implicit delimiter after data):

```text
sqlrs-lb-result-v1\n
manifest <N>\n<exact N bytes>
stdout <N>\n<exact N bytes>
stderr <N>\n<exact N bytes>
detail <N>\n<exact N bytes>
sqlrs-lb-result-end-v1 idle none\n
```

The only alternative footer is `sqlrs-lb-result-end-v1 idle absent\n`, requiring
the saved exact prune authority and actual prior-path absence. `none` is required
without that authority. The root helper observes no UID-1001 child before writing
the envelope, and again after writing stream payloads/before detail and footer.
A manifest or child-generated matching string is not this observation.

Lengths are canonical unsigned ASCII decimal, zero exactly `0`, no sign/leading
zero/space variation/overflow. Exactly four fixed members, no arbitrary paths,
ordering, duplicates or trailing bytes. Manifest <=8192 bytes, each stream <=the
admitted limit (1..64 MiB), detail <=4096 bytes; header line <=64 bytes and complete
wire <=`2*limit + 8192 + 4096 + 256`. Validate before slicing/allocating by a declared
size. Reuse the existing manifest/identity/EOF/length/hash verifier for all streams.

Private files are flushed by the unchanged durable protocol before the envelope.
Only complete transport exits zero; the verified manifest supplies child code
0..255, including known failure 7. Any transport error, quota/cancellation, missing
footer, changed parent or live child returns no accepted capture/receipt and
retains durable reconciliation data. Do not automatically reread or rerun SQL.
An explicit independent read still verifies actual private files; it is not a
successful fallback concealed inside InvokeTrackedLiquibaseStep.

Before SQL, require bounded binary capability, valid identities/authorization
and the exact live parent. Keep the original work context/deadline, no 30-second
deadline for the whole CLI; the existing local-command five-second drain bound
remains. After complete transport, inspect the same full parent ID/name/running
state before accepting it. Freeze copies of nullable authority before external
work. The only successful-path Docker operations are inspect / bounded helper
exec / inspect, with no separate cp, idle/prune exec or result-read inspect.

### Timings, not execution authority

The eight existing coarse helper phases and strict legacy parser remain intact.
Explicit result mode adds `result_prepare` (post-flush checks/size preparation)
and `result_emit` (headers/payload transfer and second idle/absence checks), in
that order. Sample result_emit before writing the detail member. Ten contiguous
10-ms-resolution regions are accepted only by the new result-specific parser/
recorder and step-invocation owner; planner/legacy helpers still require eight.
Invalid/missing/zero-length detail marks measurements unavailable, not a failed
valid child result. Invalid framing/over-quota detail is a transport failure.
The final detail/footer construction/write is explicitly unmeasured helper self
time, included in the host exec span; do not claim whole-envelope timing from
the two coarse regions or add them to their host parent.

Add real host `capture.result_decode` around framing/manifest/hash verification;
keep `capture.read` for actual independent reads only. Campaign/Go/Node select
`same-exec-v1` expectations for exact new off/on artifacts before collection,
never infer eight/ten phases from reported counts. Default legacy parsing remains
strict. The unchanged API timer and additive wall-time ledger include the whole
host invocation, decoding and both inspections without double-counting.

## New tests IR01–IR10

1. **IR01 — exact binary framing.** New runtime decoder tests use independently
   written literal frames, empty/ordinary/NUL/non-UTF8 streams and child 0/7/255.
   Check fixed members, headers/footer/EOF, real manifest binding and every hash/
   byte count; payload containing footer-like text cannot manufacture completion.
2. **IR02 — refusals/quotas.** Truncate each boundary; wrong version/order/member,
   duplicate/extra field, noncanonical/overflowing length, 8192/8193 manifest,
   per-stream limit/limit+1, 4096/4097 detail, oversized header/wire, wrong footer,
   missing EOF and foreign manifest each refuse with zero accepted capture/receipt.
   Use small admitted limits, not huge allocations. Opaque error text stays private.
3. **IR03 — exact runtime path.** New adjacent runner tests assert inspect / one
   bounded exact-helper exec / inspect, explicit mode/cost/prune overrides, configured
   Docker binary, frozen exact authority and no acquisition commands after success.
   Missing capability/invalid binding/cancelled input refuses before SQL; changed,
   stopped, renamed or unknown parent refuses. Keep complete work context, quota
   and drain rules. No exported receipt factory or product fault hook.
4. **IR04 — child versus transport outcomes.** Manifest 7 with transport exit 0
   is a known child failure. Transport failure even with complete-looking bytes,
   cancellation, incomplete footer or live child is unknown; preserve private
   files, zero receipt/completion, no automatic archive read/SQL retry. An explicit
   later independent read may recover the durable result without another CLI.
5. **IR05 — exact prune proof.** First invocation has `none`, no receipt. A valid
   saved prior link/authority plus `absent` seals the existing typed receipt only
   after both parent checks and valid capture. Vary each scope/digest/link and
   footer state independently; caller mutation cannot redirect the receipt.
   Actual queue completion/retirement still requires its saved exact authority;
   a manifest/footer alone cannot free a slot or permit SQL.
6. **IR06 — detail/privacy/cost.** New pure tests cover ten ordered contiguous
   phases, exact overflow/quantization bounds, disabled zero-work path, original
   strict eight-phase controls, owner binding and rejected forged/partial detail.
   Invalid diagnostic content stays unavailable without changing a valid capture;
   oversized framing remains refusal under IR02. No raw streams/diagnostic errors
   in logs, wire reports or public errors. New Go/Node receiver expectations are
   declared by artifact/mode and refuse cross-mode, wrong counts and bad sums.
7. **IR07 — independent read/legacy controls.** Keep all existing ReadComplete,
   archive, stopped-parent, direct-helper, separate prune and strict parser tests.
   New tests compare envelope versus explicit read results byte-for-byte and
   prove a stopped parent is never started. No automatic recovery is added.
8. **IR08 — native helper/runner.** Actions as ordinary Linux user: actual pinned
   helper returns arbitrary binary streams and child 7; exact same parent persists.
   Binary/code7 fixtures invoke the pinned helper directly with a short shell
   child, not shell arguments passed to InvokeTrackedLiquibaseStep (which always
   launches stock Liquibase). IR09 separately exercises real JVM/SQL.
   Genuine prune/idle checks, complete durable files and surviving-child refusal
   must reach their exact boundaries. Lost transport reply retains files and can
   be independently read; use a wrapper of actual transport, not fake SQL success.
   Cleanup own containers/files before fallback. No laptop native workload.
9. **IR09 — genuine prepare.** Actions three-change prepare, actual JVM/SQL/JDBC,
   one reusable runner, exact DB handoff, cached data and normal cleanup. Count
   one inline result per applied change, zero successful-path capture.read/cp/
   standalone prune. Existing O6 cold/hit inventory controls remain. No change to
   invocation admission, journal, state/access checks or stock CLI-per-change.
10. **IR10 — regressions/coverage/remeasurement.** Full ordinary engine/client/
    observer suites and vet, Linux native/race where supported, per-line new-helper
    coverage target100/min95. Combined O5–O7 then reruns unchanged eight-member
    CT13 in Actions with exact artifacts, data/cleanup oracles, helper expectations,
    original timer, source correspondence and exact additive sum. Speed gate stays.

## Limited existing-test corrections (separate approval)

- **OT-IR01:** only TestInvokeTrackedLiquibaseStepRequiresCapturedResult: use the
  inline envelope; child-failure fixture now has helper exit0/manifest7. Keep all
  three cases/identity/arguments. IR04 separately insists transport error refuses.
- **OT-IR02:** only TestStepHelperCostOwnsActualInvocationAndPreservesResult and
  TestStepHelperCostNeverEntersLogSink: local binary transport/envelope and exact
  new command counts; valid step detail ten phases. Keep off/on/missing/private/
  oversized/child-failed. Explicit semantic correction: oversized detail framing
  refuses transport instead of being accepted through the removed archive fallback;
  direct recorder tests preserve diagnostic non-interference. All other outcomes,
  privacy/owner/error controls remain. TestHelperTimerClosedBoundaries is unchanged:
  the legacy `cost_emit; exit child_exit` branch still exists.
- **OT-IR03:** only the positive preparation/counters of
  TestLiquibasePrivatePruneInvocationBinding and its local pruneBindingRunner:
  one bounded inline result, zero cp/standalone probe, exact authorization once.
  All invalid-identity cases still have zero external actions.
- **OT-IR04:** only TestCombinedCapturePruneInvocation, through a local adapter:
  six old operations become inspect / apply-result / inspect. Other combined-read,
  receipt, negative and cost-boundary fixtures/tests remain unchanged.
- **OT-IR05:** only TestMainPathCapturePruneEmptyAuthorityOverridesEnvironment:
  local inline response and arguments from the actual bounded call. Keep exactly
  one empty authority override; no invented permission or broad fixture update.
- **OT-IR06:** only TestCostMainPathNamedAttributionDocker: fresh capture.read3→0,
  capture.result_decode=3, planner8/step10 exact helper phases. Cached counts and
  all data/ID/reuse/SQL/cleanup checks remain. Existing reusable-runner Docker
  tests require no relaxation: their explicit independent reads still execute.
- **OT-IR07:** only complete-cost-boundaries.test.mjs's applied-operation fixture
  substitutes actual result_decode for capture.read; preserve all independent
  missing/wrong-boundary controls and every other operation/count.
- **OT-IR08:** only TestReleasedEngine/collect: predeclared artifact-bound result
  policy selects the extended receiver before engine launch; parent passes it to
  Node too and clears inherited values on legacy routes. Hashes, workloads,
  launchers/CLI accounting, timers, timeouts, oracles and cleanup remain unchanged.

Every other contradiction requires a new narrow request. No O5, legacy absence,
failure/recovery or psql test changes are authorised by this proposal.

## Critical and repeat review

Corrected: frame lengths cannot be shell-text-delimited streams; footer/EOF and
real root idle/prune observations are mandatory; child7 is not helper transport7;
an exec quota/error cannot become success through a hidden reread. Freeze exact
authority and retain stopped-parent behavior. Distinguish diagnostic content
failure from envelope quota failure, preserve strict legacy helpers, and select
ten-phase expectations before counts rather than accept whichever appears.
The helper's final detail/footer write remains visible host time, not falsely
measured coarse helper time. No whole-CLI 30-second timeout or fake read spans.
Native shell-fixture execution is kept distinct from actual stock CLI invocation
to avoid the earlier fixture mistake of treating shell arguments as a program.

Repeat review traced every component invariant to IR01–IR09, recalculated finite
wire overhead, checked file durability and replay fences, native negative controls,
real three-command proof, old fixture isolation, caller cancellation and private
reporting. No remaining P0–P2 finding in this inventory. Approval/implementation
and measured speed gains are not claimed by this review.

## Implementation evidence (not acceptance), 2026-11-12

- Literal binary frames exercised all byte-prefix truncations, exact quotas,
  non-UTF8/empty streams, manifest identity/hashes and child exits 0/7/255 before
  implementation. The new envelope decoder has 100% statement coverage.
- The connected runtime uses only exact inspect / bounded exec-result / exact
  inspect on success, freezes nullable authority before effects, preserves the
  original deadline and rejects transport/quota/cancellation without a hidden
  read or SQL retry. Its changed invocation function is 98.4% covered.
- The helper preserves durable output and its legacy direct branch; explicit
  result mode performs actual idle/prior-absence checks before and after stream
  emission. Git Bash `-n` passes. Native helper/real three-step prepare tests are
  added to the opt-in Linux Actions probe; they have **not** run on the laptop.
- Full runtime and preparecost suites and vet passed after the permitted old
  fixture changes. Package coverage is 93.1%/93.8% under the existing aggregate
  exception; it is not a claim of package-wide 95%. Both new helper parsing/
  recording entry points and their shared parser/recorder are 100% covered.
- The full Go collector suite/vet and Linux/amd64 collector/runtime/prepare
  cross-compilation pass. New policy and changed helper-detail receiver functions
  have 100% statement coverage. The 103-test Node suite passed; selected new
  policy/detail/complete-output helpers have 100% lines/functions, 95.91% combined
  branches. This predates the extra full-receiver test below.
- An additional IR10 full-receiver oracle exposed missing `capture.result_decode`
  and `runner.inventory_lookup` keys in the independent host-accounting allowlist;
  observer handling is corrected. The same new test subsequently reached its
  own mistaken `row.Calls` assertion: the reviewed report uses `row.Count`.
  @evilguest approved only `Calls`→`Count` on 2026-11-12. After that exact
  correction the full Node suite passes 106/106. Eight decodings and conserved
  100ms remain asserted. The repeat coverage is 100% lines/functions for selected
  policy/detail/complete-output helpers, with 95.91% combined branches; no product
  or old-test changes were needed for this correction.
- Full engine regression initially passed every package except the new IC06
  fixture. After the separately approved fixture corrections, full prepare
  passes on 2026-11-13. O5 tests are approved and now being written; their schema
  checks are deliberately red before O5 implementation. No new CT13 timings,
  complete current #89 regression pass or merge readiness is claimed.
- Full Windows CLI regression (`go test ./... -count=1 -timeout=4m`) passes
  unchanged. No CLI source or test was modified by O7.
