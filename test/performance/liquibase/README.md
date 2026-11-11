# Liquibase performance collector

External Linux/amd64 engine collector for the approved
[PA01–PA12 plan](../../../docs/architecture/issue89-performance-actions-tests.md).
This is not product code and imports no internal engine package.

Launch through the separate **Liquibase performance (explicit opt-in)** workflow:
same-repository PR label `performance:pilot` or `performance:full`; after the
workflow is present on the default branch, manual dispatch accepts an exact
candidate SHA and `pilot`/`full`. Fork PRs and ordinary updates do not start it.
Use a new explicit launch to measure a newer commit; a running cohort is not
silently replaced when its PR moves.

Every fresh VM builds the fixed clean source with Go 1.25.5 and `-trimpath`; its
hash must equal the initial preparation VM's artifact hash. This provides the
same executable bytes without cross-VM build-path differences. Released RC7 is
downloaded, checksum/source/build-identity checked, never rebuilt. Acquisitions,
stock host distributions, images, Java, JDBC and builds precede all measurement.

Quick checks (no workload or Docker benchmark):

```sh
node --test scripts/performance/*.test.mjs
cd test/performance/liquibase
GOWORK=off go test -count=1 -timeout=2m ./...
```

The separate module intentionally uses `GOWORK=off`, not different Go caches.
The integration scenario lives in `collector_linux_test.go`; event stream/count,
validation and disposal helpers remain ordinary Go code. HC01–HC07 cover these
helpers at 174/174 statements (100%), not the integration scenario, Node process
adapters or product. See `docs/architecture/issue89-performance-actions-tests.md`
for the approved boundary, profile and separate native acceptance evidence.

The separate diagnostic observer now adds trace/journal/process helpers: DC01–DC03
pass at 494/504 ordinary Go statements (98.0% overall; the original 174 remain
100%). This does not count actual Actions orchestration or product coverage.
Its explicit `performance:diagnostic` label schedules eight serial members on
the original fixed binaries, not another acceptance cohort. Three attempts of
run `42580413674` failed on release-download HTTP 504 before any measurements.
The official-API acquisition and observer-owned preparation clarification is
approved along with TA01–TA06; preparation is implemented, with 32 small Node
tests passing (five ordinary helpers 100% lines), Go 98.0% and Linux vet passing.
Actual TA06 diagnostic retry remains unfinished. See
[diagnostic design](../../../docs/architecture/issue89-performance-diagnostic-design.md)
and [tests](../../../docs/architecture/issue89-performance-diagnostic-tests.md).
EU01–EU06 subsequently approve explicit measured decoding of terminal attempts
without an exec result as `exec.unconfirmed`. This records known time, not a
successful JVM/CLI; previous confirmed images end at attempt start. Strict/default
decoding and its old tests remain unchanged. `Commands` counts displayed intervals,
not successful launches; independent workload/CLI counts still qualify samples.
Small checks pass at 527/537 Go statements (98.1%) and 34 Node tests (ordinary
helper lines 100%). The EU04 field-dropping clarification is now approved and
passes an additive test (35 Node tests, ordinary helper lines 100%). The closed
failure-context project and separate CX01–CX05 are approved and implemented:
small Go/vet pass at 544/554 statements (98.2%), 37 Node tests pass (ordinary
helper lines 100%). Native CX05 run 42632130376 verifies failure-context transport
(trace_8 / pending_entry / other, last phase oracle), but the eight-member cohort
is incomplete. Further structural-prefix observation is separately approved;
PX01–PX05 are separately approved and implemented. Small Go/vet pass at 579/589
statements (98.3%), with new prefix helpers 100%; all 39 Node tests pass (ordinary
helper lines 100%). Native PX05 is open. No new record syntax is accepted.
Native PX05 run 42665130138 identifies the pending entry as unknown_syscall/other;
its original incomplete report is retained. A bounded raw-call grammar and
unattributed process-time amendment and separate UR01–UR07 tests are approved
(2026-11-10). After the separately approved UR01 boundary correction, full small
Go/vet pass at 644/654 statements (98.47%; new raw helpers 100%) and all 41 Node
tests pass (helper lines 100%). Native UR06/UR07 remain open; the raw-syscall
probe runs only in Actions, never on the laptop.
Native 42761268874 subsequently passes UR06 (0.08 s) and the two plain members,
but traced v1 member 3 still rejects an unknown pending record. Original report
retained; full raw-rejection detail and separate RD01–RD07 tests are approved
(2026-11-10). Detail implementation passes full small Go/vet (703/714 statements,
98.46%; new helpers 100%) and 43 Node tests (helper lines 100%). Native RD07 is
still open; strict/codes/CX/PX/UR assertions are unchanged.
Actual RD07 run 42772724450 verifies fallback/empty/none/arity at traced member 3;
the incomplete report is retained. This establishes the empty-entry observer
refusal, not the product cause. A separate terminal-only unavailable-entry
component boundary, UE01–UE07 and two named old-test amendments are approved and
implemented. Small Linux Go/vet pass at 715/726 statements (98.48%), all 44 Node
tests pass (helper lines 100% / branches 97.10%). Native UE07 still requires the
original eight members; no product change or speed verdict. Source CI 42772612811 passes all ordinary
regressions, only aggregate coverage red.
Native UE07 run 42782782114 passes trace decoding, then fails summary `clock`
at member 3. Original report retained; no exact clock/product cause established.
All regressions in CI 42782733993 pass, only aggregate coverage red. A separate
closed clock-context component amendment is approved; the separately reviewed
CW01–CW07 inventory is separately approved and implemented. Full small Linux
Go/vet passes at 747/758 statements (98.55%; new helpers 100%), all 46 Node tests
pass (ordinary helpers 100% lines / 97.32% branches). Native CW07 remains open;
no rejection, time-source or product change.
Native CW07 42789132449 verifies actual clock metadata: duration_mismatch,
wall/elapsed 73672499931/73672499940 ns (−9 ns). Original incomplete evidence
retained; CI 42789081930 passes all regression jobs, only aggregate coverage red.
Measured-only bounded alignment and separate MC01–MC08 are approved and
implemented. Full small Linux Go/vet passes at 765/776 statements (98.58%;
new helpers 100%), all 49 Node tests pass (helper lines 100% / branches 97.39%).
Native MC08 `42802379397` now completes all eight members and 16 passes; every
oracle and exact cleanup passed. Signed clock deltas are -10/-9/0 ns. The original
report and [analysis](../../../docs/architecture/issue89-diagnostic-results-2026-11-10.md)
are retained. Ordinary regression persists; route-dependent tracer overhead and
residual time preclude complete causal attribution. Strict helpers/default
shapes, product timing and the acceptance gate remain unchanged. Earlier open
native checkpoints above describe history, superseded by this complete cohort.
After discussion, the user selected source-level cost attribution plus evidenced
duplicate-work removal and a rerun, not further strace-only classification first.
The [new components](../../../docs/architecture/issue89-cost-attribution-and-trimming.md)
and separately reviewed CT01–CT14 tests are explicitly approved. The final
helper/observer transport refinement is also approved (2026-11-11); supplemental
DT01–DT08 and limited OT-CT05–OT-CT08 are separately approved/permitted. The
collector now waits for helper detail and observer completion after its original
API timer. Native `42964091822` passes both helper on/off/exit controls and the
full recorded three-step/cache and unchanged normal prepare checks. A small
line-mapped DT-C1 iteration was separately approved and raises the detail decoder
from 50/54 to 54/54 statements (100%); full ordinary Windows tests/vet pass.
Old fixed-binary evidence and
strict parsers stay intact; a new candidate must have a new pinned identity.

The separate `performance:cost-attribution` label now selects one same-repository
Ubuntu job: exact old v2 is prepared/retained, new v2 prepared once, then eight
serial JHipster members alternate released v1 / old v2 / new off / new on and
the reversed order. Setup/warm-up/native 5.0.3 image qualification are outside API
timing. Only closed `.tmp/performance/cost-reports/*.json` is uploaded; these are
not the unchanged full P01b acceptance reports. Pure campaign helpers are 100%
covered, all 82 Node tests pass. Actual campaign execution is still required.

That short CT13 campaign subsequently completed in Actions `42967877614` on
`48e6e11`: all eight members / 16 passes, oracles and cleanup. The immutable
[source-linked analysis](../../../docs/architecture/issue89-cost-results-2026-11-11.md)
retains the report/hash and 18.00% fresh / 20.83% cached ordinary new-v2 delay.
The approved next component change combines completion and actual prior-prune
observation under the existing exact container checks. It moves the normal
prune check into `capture.read`; compare old read+prune against the new combined
cost, never count a removed label as saved time. The standalone prune operation
and historical reports stay intact. CPR01–CPR09 and limited OT-CPR01–OT-CPR03
are approved; runtime/coordinator are connected and the explicit native probe
workflow now selects the new pinned-helper/wrapper/known-failure tests. Those
tests pass in Actions `42994708398` on `d4d9ccf`, including the complete/cached/
known-failure prepare controls. CT13 `43000348314` is incomplete after six members
on its second old-v2 control; retain that original report without accepting a
partial comparison. Unchanged same-VM whole retry `43002214752` completes all
eight members / 16 passes. Retain both original JSONs and source-linked analysis:
current v2 saves 4.31% fresh against old v2, but remains +15.77% fresh / +19.78%
cached versus actual v1. The original speed gate still blocks full acceptance;
the 35 fewer Docker commands are a count result, not an isolated timing A/B.
All regression jobs of source CI 42761251963 pass; aggregate coverage stays red.
Complete remote EU06 remains unfinished; no precise cause or speed acceptance.
The native collector test is explicitly opted in only by the Actions scheduler.
Its internal samples are emitted only after oracle verification and successful
exact cleanup. The parent never publishes raw child stdout/stderr. Private
stores/configuration/capture output are not workflow artifacts; only sanitized
JSON under `.tmp/performance/reports` is uploaded.

Public fixture/oracle provenance is retained alongside the performance-owned
copies under `fixtures/` (Apache-2.0 JHipster and MobilityData). The full oracle
expectations are independent of both measured engines. Existing release smoke
queries and tests are unchanged; no private izess access is needed.

`pilot` proves Linux compatibility, real uncached/cached DB results and estimates
the complete hosted budget including all controls/untimed costs and a 15-minute
cleanup/report reserve. The prediction includes 20% contingency; it is not a
completion guarantee. `full` first repeats that pilot, then each fixture's whole
five-pair cohort plus five control pairs stays on one fresh VM. Confirmation
uses separate fresh VMs with their own controls. Jobs are bounded to six hours;
no paid or self-hosted capacity is requested. Infeasible or interrupted collection
remains incomplete. A green collection is **not** a speed pass: apply retained
P01b evaluation to the validated evidence separately.
