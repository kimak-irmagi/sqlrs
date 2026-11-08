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
