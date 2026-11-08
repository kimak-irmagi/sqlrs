# #89: native Actions performance results (2026-11-08)

Status: all six complete cohorts collected; the Actions workflow succeeded.
The collected short workloads already reproduce a release-blocking
regression under the retained P01b rule. #89 is not complete and PR #175 remains
draft. Successful collection is not speed acceptance. Do not waive the timing
criterion or pool these data with the pilot or another VM.

## Provenance and validation

Run [42479055486](https://github.com/kimak-irmagi/sqlrs/actions/runs/42479055486),
fixed candidate source `668d5050ad154c1967bea4c6e40b91f3a703d5ba`.
Actual published Linux v1 source: `cab99c4c7ced5b18f4dc43a65b002d84d8fef02d`.
Executable SHA256: v1 `b34c358d2d1e6e969f17d4e393641dd7cb53167870b8e4674db1381bf410bbca`,
v2 `1367bce9ddaa44c0aa378b07c015dfb20a4c0d29f00d2f5a893905686fae14b0`.
Both routes use the same locked JDBC and `copy` snapshot backend. Their real
host/container Java differences remain explicit in the samples.

Collector audit caveat: the Linux v1 request executes `distribution/liquibase`,
but the report's launcher inventory reads `liquibase.bat`. That is a provenance
defect in the observer, not a demonstrated cause of slower v2. API elapsed time,
DB oracle, cache hits and Docker/CLI observations do not come from that hash;
the timing regression remains observed, but native-launcher provenance and
derived JVM totals are not fully qualified. Preserve original evidence bytes;
verify the actually executed Linux script and trace-derived observations in the
separate diagnostic run before relying on launcher/JVM metadata.

Each analysed cohort passed the existing `validateCohort`: 20 distinct engine
runs, five alternating v1/v2 pairs and five v1/v1 control pairs, two passes per
run, exact order/host/identity and independent full DB/cache/cleanup checks.
For the four short-workload cohorts, artifact identities were also compared
across each fixture's two phases and match; the VM IDs differ as required.
No cross-VM timings are pooled.

## Completed cohorts

All numbers below are seconds. V1/V2 columns are route medians for description;
the decision column is the **median of the five paired v2-minus-v1 differences**,
not the difference of those route medians. The last column is the maximum
absolute individual v1/v1 pair difference, a conservative observed control bound.
Every paired-median regression exceeds even that bound on both independent VMs;
the conclusion does not depend on choosing a smaller control-median envelope.

| Fixture | Phase | Pass | v1 median | v2 median | Paired median delta | Max absolute control delta |
| --- | --- | --- | ---: | ---: | ---: | ---: |
| JHipster (8) | comparison | uncached | 43.883 | 54.483 | +10.123 | 0.856 |
| JHipster (8) | confirmation | uncached | 40.955 | 50.729 | +9.956 | 3.078 |
| JHipster (8) | comparison | cached | 8.416 | 9.463 | +1.233 | 0.767 |
| JHipster (8) | confirmation | cached | 7.945 | 9.322 | +1.335 | 0.204 |
| MobilityData user (12) | comparison | uncached | 66.606 | 78.473 | +11.877 | 1.610 |
| MobilityData user (12) | confirmation | uncached | 67.631 | 79.141 | +11.578 | 0.406 |
| MobilityData user (12) | comparison | cached | 8.451 | 10.110 | +1.659 | 0.400 |
| MobilityData user (12) | confirmation | cached | 8.543 | 9.901 | +1.357 | 0.210 |

The descriptive route-median slowdown is about 24% for uncached JHipster and
17–18% for uncached MobilityData user. Cached preparation is also slower on both
workloads. The workload/oracle passed: this is a speed failure, not missing SQL
or a failed result counted as a fast sample. A successful 102-change result
cannot negate confirmed failures of other required fixtures.

## Observations, not yet a causal profile

- Both engines run one planning CLI plus one migration CLI per uncached change;
  cached passes still run a planner. The proposed no-replanning fast path is
  separate issue #104, not silently enabled by #89.
- V2 additionally runs one offline image-inspection CLI/container per request,
  including a cached request; the observed count is not assumed to be zero.
- Observed creates are v1 4/2 versus v2 8/4 (uncached/cached) for both 8- and
  12-change workloads. This rules out a container-per-changeset explanation in
  those cohorts, but does not attribute seconds to any individual operation.
- Source inspection confirms image resolution/probing precedes planning and
  the planning environment is still prepared before exact state-cache reuse.
  Extra container/probe work is a candidate source of fixed overhead, not a
  measured causal explanation. No optimisation or contract change is made here.

## Raw evidence references

The original sanitized reports were downloaded under
`.tmp/performance/evidence/42479055486/cohorts` and retained byte-for-byte in
[the evidence directory](issue89-performance-evidence-2026-11-08/), with explicit
LF checkout rules. Every retained file's SHA256 matches the downloaded report.
All six reports are retained independently of Actions' 14-day artifact expiry.

### 102-change comparison update

`mobilitydata-main-comparison.json` passed the existing completeness validator:
20 members, full independent DB/cache/cleanup verification. Uncached route
medians are 573.847 s (v1) / 597.790 s (v2), paired-median delta +25.290 s;
the maximum absolute individual control-pair delta is 34.492 s. Cached medians
are 12.750 / 10.525 s, paired-median delta -2.215 s, max absolute control delta
1.815 s. Preserve the control outlier;
do not cherry-pick a smaller noise bound or treat this fixture result as
an overall speed pass. The confirmed short-fixture failures remain blocking.

The report is retained in the same evidence directory; Actions artifact
`13624417581`, SHA256
`e45ff96335967cd2449d8e7e6402c4fb7048840f6d5bf6c31872f6106375c9f2`.

The independent 102-change confirmation also passed completeness and oracles.
Uncached v1/v2 medians: 661.430 / 672.155 s; paired-median delta +9.964 s,
maximum absolute control delta 10.067 s. Cached medians: 14.238 / 11.487 s;
paired delta −2.683 s against 0.197 s control spread. Thus the long fully cached
case is faster in both cohorts; positive uncached differences accompany sizeable
control spread. This does not alter the short-workload blocker or establish a
new noise rule in place of retained P01b.

| Report | Actions artifact ID | SHA256 |
| --- | ---: | --- |
| jhipster-comparison.json | 13618097857 | 7198fbe3f35b20503984880acee6e53ce4bc4b8e938247381bc095009567ad7b |
| jhipster-confirmation.json | 13618730554 | 419fe9a2df2b595db53b7ea52df4d707ffccc0984f2b17b6a4d6084cca7aca99 |
| mobilitydata-user-comparison.json | 13618621974 | 6c180092af707c672a413369ca4470b1b1054ce8e12561400d19ffca58354429 |
| mobilitydata-user-confirmation.json | 13618184524 | f303d9a49e8ad825e230920054c1f013ac93cac2b27c41ef581b9e39639c976f |
| mobilitydata-main-comparison.json | 13624417581 | e45ff96335967cd2449d8e7e6402c4fb7048840f6d5bf6c31872f6106375c9f2 |
| mobilitydata-main-confirmation.json | 13624749957 | 616e649a01eab32e58024a16d9a0f019fa87716b227041edc89fb0152c81e14a |

Ordinary CI at the product/test checkpoint (`44f8e28`, run `42481604984`) passes every test/doc job;
only the unchanged aggregate coverage gate is red under the explicit #89/#174
exception. That exception does not waive this independently confirmed timing
regression. Do not bypass repository protection or mark the feature complete.
