# #89 O6: bounded inventory reuse — tests

Status: IC01–IC08 and limited OT-IC01–OT-IC03 approved by @evilguest on
2026-11-12. Components O6 are approved
in [the amendment](issue89-measured-overhead-reduction.md). Implementation and
verification are in progress; see the results below. O5 storage and O7
transport are separate, so their approval does not block these cache tests.

## Requirements and new tests

1. **IC01 — real miss then immutable hit.** A new service calls the real injected
   inventory capability once for the same resolved digest/platform/rule key;
   repeated successful binding uses copied inventory. Both embedded and mounted
   driver selection retain their actual digest. A second service probes again.
   Real lookup spans occur on both calls; actual inventory spans only on misses.
2. **IC02 — identity and provenance boundaries.** Independently change manifest,
   OS, architecture, variant and rule revision: no cross-key hit. An invalid
   digest/binding is rejected before a hit/loader. Mutating a returned JAR slice
   or producer-owned slice cannot alter later selection. Unsupported Java/LB,
   duplicate JARs and invalid provenance remain errors, never valid cache hits.
3. **IC03 — failures and cancellation.** Probe/selection failure and producer
   cancellation create no entry; a later independent call performs another
   probe and may succeed. An already cancelled hit caller gets cancellation,
   not success. Error privacy/classification remains unchanged. No SQL retry.
4. **IC04 — concurrency without global I/O lock.** Barrier-controlled same-key
   calls share one successful probe; a cancelled waiter returns without cancelling
   the producer or another waiter. Producer failure wakes all waiters with failure,
   without silent retry; a different key can finish while the first is blocked.
   Assert all goroutines finish, with no sleeps used as ordering proof.
5. **IC05 — finite lifecycle.** Fill 32 ready entries, touch the oldest and add
   a 33rd: evict the least recently used other entry, not the touched one. Fill
   32 keyed producers: an extra key runs uncached without a new product error,
   unbounded retained map, blocked unrelated key or cached failure. After completion
   the cache remains bounded and evicted/missed keys may probe normally again.
6. **IC06 — true binding integration.** Two admitted jobs in the same service
   use the real queue/source store and identical immutable inventory but separate
   job bindings/projections. A cache hit still checks the resolved image/platform,
   calls image availability, selects the driver and obtains/materialises the
   per-job artifact as needed. Changed tag-to-digest resolution misses. Failing
   artifact publication or binding commit does not manufacture a completed job,
   new state or execution; the second job's fingerprint matches its real inputs.
7. **IC07 — independent measurement policy.** The test-only collector/campaign
   predeclares `per-process-v1` inventory expectations only for exact selected
   new-off/new-on artifacts; v1 and fixed old-v2 keep legacy expectations. An
   absent selector keeps legacy mode; unknown or misplaced selection fails before
   launching the engine. Strip inherited selection and derive expected [1,0]
   (new cold/cached), [1,1] (old-v2), [0,0] (v1), not from reported counters.
   Both Go and Node reject wrong observed counts. Existing migration-CLI/JVM
   accounting, exact binary hashes, data/oracles and original timer remain.
8. **IC08 — native/regression/coverage.** In Actions, cold then cached prepare
   on one real engine must show one actual probe then zero, with correct JDBC,
   data, unchanged CLI-per-change and container reuse, normal cleanup before
   fixture fallback. Ordinary Go/Node tests, vet, race checks where supported,
   and per-line cache/measurement-helper profiles follow (target 100%, minimum
   95%). A restart is independently covered as a cold service; no cache prewarm
   or workload is run on the laptop. Unchanged CT13 follows combined O5–O7 work.

Use existing private interfaces, real SQLite/CAS fixtures and new adjacent test
files; no global hook, production clock injector or exported testing factory.
The bounded cache is observed by same-package tests and loader counters, not
by exposing product cache state to CLI/API.

## Coupled existing tests: limited OT-IC scope

The user already requested correcting tests tied to removed actions. The final
inventory makes the permitted corrections precise; approval of this inventory
authorises only these cache-related changes, not other failing assertions:

- **OT-IC01:** `TestCompleteTimelineMainPathDocker` and
  `TestCostMainPathNamedAttributionDocker`: actual `runner.inventory_lookup=1`
  per prepare, inventory cold=1/cached=0. Preserve every DB count/ID, state,
  helper phase, execution count and cleanup assertion; O5/O7 changes are separate.
- **OT-IC02:** `complete-cost-boundaries.test.mjs`: replace the common mandatory
  inventory count with mandatory lookup and cold/hit-specific counts, using the
  independent literal workload fixture. Missing/wrong-boundary negatives and
  every other required operation remain. Historical arithmetic/parser fixtures
  do not acquire invented cache-hit evidence or have their old checks weakened.
- **OT-IC03:** `TestReleasedEngine/collect`: change only explicit measurement
  policy selection and the expected image-probe count for selected new artifacts;
  retain all original requests, timer boundaries, programs, stock launchers,
  timeouts, DB/cache/migration-CLI/cleanup checks. New controls prove wrong counts
  still fail. Parent `run-cost` supplies the exact member's policy to both Go
  collection and Node qualification; off/on use the same product binary.

Legacy helper behavior tests/default parser mode stay unchanged. Any other old
contradiction requires explicit review rather than broad fixture replacement.

## Critical and repeat review

Corrected: cold-start versus hit savings are distinct; a cache cannot bypass
Select/artifact handling; cancellation may not turn a hit into success; LRU
capacity includes in-flight bookkeeping; all failure checks prove their actual
boundary rather than an earlier refusal. Measurements declare artifact-bound
expectations before observing counts; merely accepting either zero or one was
rejected. Native data/cleanup and historical strict controls remain independent.

Repeat review checked all key dimensions, producer/waiter lifetimes, 32/33
capacity boundaries, deep-copy ownership, both driver modes, real binding failures,
the selector's legacy/unknown routes and unchanged timing. No remaining P0–P2
finding in this bounded inventory. This review preceded implementation.

## Implementation evidence, 2026-11-12 (not final acceptance)

- Added the per-service zero-value cache and exact binding lookup; same-key
  cancellable coalescing, successful copied inventory only, independent 32-entry
  ready/in-flight limits and LRU eviction. Every job retains resolution,
  availability, selection and driver artifact handling.
- IC01–IC05 and unchanged cost-binding regression tests pass. Both focused and
  full-run per-line profiles cover every statement in
  `liquibase_inventory_cache.go`: 100%; the HTML profile is generated under the
  ignored engine `coverage` directory, not used as a passing-suite claim.
- Repeat review reproduced cancellation between a completed probe and cache
  publication with a deterministic context barrier. Rechecking cancellation at
  publication fixes the demonstrated retained-entry defect; the focused profile
  remains 100%. A separate literal JAR oracle also checks that producer/consumer
  mutation cannot be hidden by using the production copy helper on both sides.
- The full collector Go suite passes; new `inventory_policy.go` is 100% covered.
  Collector vet and Linux/amd64 cross-compilation pass. All 100 performance Node
  tests pass; new/changed policy, collector-output, campaign and source-boundary
  helpers are 100% lines/branches/functions covered.
- Follow-up IC07 verification directly tests Go's rejection of wrong observed
  probe counts for all three predeclared policies. The real collector calls that
  tested predicate. Full collector suite/vet pass; both functions in the policy
  helper remain 100% covered. The Node performance suite now has 101 passing
  tests, including the independent actual-call-site check.
- Follow-up on 2026-11-13: the approved IC06 fixture corrections pass all four
  cases (embedded, mounted, publication failure, binding failure). The binding
  refusal uses the safe `ErrLiquibaseBinding` classification; after all negative
  and first-job immutability assertions, removing only the test trigger permits
  the exact same metadata binding. No Liquibase SQL is retried. The complete
  focused inventory suite and full prepare regression pass.
- OT-IC01/02/03 are applied within approved scope. Native execution and cold/hit
  counts are not yet qualified in Actions. Windows race compilation reported a
  cgo failure; the existing Linux opt-in probe now includes the focused race test.
  No laptop Docker/Liquibase workload or new CT13 timing has run.
