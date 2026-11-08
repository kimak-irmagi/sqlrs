# #89: move performance collection to GitHub Actions

Status: proposed, reviewed and corrected; component/platform approval pending.
On 2026-11-08 @evilguest requested off-laptop execution while retaining the full
performance protocol if Actions can support it. No workflow or collector code is
authorized by this proposal alone; the test inventory follows separately.

## Boundary and platform

Use standard GitHub-hosted `ubuntu-24.04` x64 VMs for the public sqlrs repository.
Each measured comparison runs both engines sequentially on the same VM, not one
engine per matrix job. The released reference is still actual v0.1.1-rc.7, now
its published Linux/amd64 artifact, with host Liquibase versus container Liquibase
as before. Do not combine these measurements with laptop Windows observations.
Windows functional qualification remains separate and is not removed.

The published Linux archive checksum is
`da6132c95185f2a11561628458d34b22e7cb928c574d8ecec9f06d44fc8d0f01`;
verify both this pinned value and release provenance before executing anything.
Tag/source must match `cab99c4c7ced5b18f4dc43a65b002d84d8fef02d`.
Record the extracted engine hash/build metadata too. A missing or incompatible
Linux baseline is a failure, not permission to build a different reference.

Official constraints checked on 2026-11-08:

- [Standard public-repository runners](https://docs.github.com/en/actions/reference/runners/github-hosted-runners)
  provide a fresh VM; standard use is free for a public repository. No larger or
  self-hosted runner is provisioned, billed or registered automatically.
- [Docker container support](https://docs.github.com/en/actions/tutorials/use-containerized-services/use-docker-service-containers)
  uses Linux/Ubuntu. The current Windows-only collector/executable is not simply
  copied into a Linux job; host launch and paths need a separately tested port.
- [Hosted job limit](https://docs.github.com/en/actions/reference/limits) is six
  hours. Keep an entire paired cohort and its same-host noise controls together.
  Independent confirmation may use another fresh VM with its own controls.
  A timeout or insufficient budget leaves qualification incomplete; never split
  one cohort across machines and relabel pooled timings as same-host evidence.

## Flow

1. Explicit opt-in only. Support manual dispatch once the workflow exists on the
   default branch, and a same-repository PR label trigger for bootstrapping #175.
   Do not merge the unfinished feature merely to register a dispatch workflow.
   No automatic expensive run for every ordinary push or fork PR.
2. Read-only repository permissions. Build the candidate from the exact selected
   PR head SHA, not an unspecified merge ref; download the pinned released Linux
   reference. Resolve inputs, JDBC, Liquibase images and Java before measurement.
   Public fixtures only: jhipster, mobilitydata-user and mobilitydata-main (102).
3. Run a short Linux compatibility pilot using both real external engine artifacts
   and independent DB oracles. Establish the collector works and estimate whether
   the largest full cohort fits the hosted limit before starting that cohort.
   Explicitly select the same supported copy-snapshot backend for both engines;
   do not accidentally compare a different filesystem/backend. A pilot is not a
   qualifying performance series. If either release cannot use that setup, fail
   compatibility rather than adding a new product behavior.
4. For each fixture, run the unchanged approved P01b alternating pairs and noise
   controls serially on one VM. Warm pinned images outside the timed intervals;
   use fresh state stores for uncached passes and the same store for their cached
   follow-up. No builds or other workloads run beside the measured commands.
5. Run the independent confirmation series on an independent fresh VM with the
   same declared artifacts/settings and its own local noise controls. Compare
   within each VM, never the absolute laptop/VM or cross-VM timings.
6. Upload redacted samples, immutable identities, runner/image/CPU/memory/Docker/
   Java provenance, container/CLI counts and oracle results. Always perform exact
   test-owned cleanup. Do not upload DB stores, engine.json, DSNs, credentials,
   protected SQL captures or unfiltered engine logs.
7. Inspect the completed artifacts under the existing P01b decision rule. A green
   collection job means valid complete collection, not automatic speed acceptance.
   Missing, timed-out or inconclusive evidence cannot close #89. The protocol and
   performance requirement are not shortened by moving the executor.

If the pilot predicts that a full cohort plus its controls cannot fit six hours,
report that constraint before a long futile run. A dedicated off-laptop runner
or an amended protocol then needs a separate decision; neither is assumed.

A fresh VM removes laptop user workload, not every possible source of cloud
noise. Controls remain mandatory; noisy/inconclusive measurements are not a pass.

## Internal components and data ownership

- A separate performance workflow owns opt-in, read-only permissions, concurrency,
  artifact preparation, per-fixture VM jobs, bounded time and artifact retention.
- A repository-owned external collector under `test/performance/liquibase` owns
  real engine startup/API requests, fixture/oracle evaluation, isolated stores,
  monotonic timing, exact Docker event observation and cleanup. Port the existing
  Windows collector's checked behavior; do not depend on the prototype worktree.
  No product package gets a new injection hook or experimental execution path.
- Small preparation/report helpers under `scripts/performance` own checksum/ref
  validation, sample/provenance serialization and completeness checks. They cannot
  grant a performance pass merely because test data was collected successfully.
- Native DB/config/output stays private to its job and is destroyed there. Only
  redacted evidence becomes a workflow artifact. No shared DB or API/schema change.

Existing product/collector tests are not rewritten by this approval. New port,
workflow guards and evidence validation require a separate reviewed test plan.

## Critical and repeat review

- P1: Windows binary/launcher cannot qualify Linux. Pin the real Linux release;
  prove compatibility before timed collection and keep platform evidence separate.
- P1: one-engine-per-job comparisons measure different hosts. Keep each full
  paired cohort and its controls on one VM; confirmation has its own controls.
- P1: hosted timeout could silently truncate the 102-change experiment. Pilot
  first, reserve cleanup/upload time and report incomplete instead of passing.
- P1: workflow success could be mistaken for the speed gate. Separate valid data
  collection from P01b evaluation and preserve the existing unresolved/fail states.
- P1: public artifacts could disclose connection settings or SQL. Allowlist report
  data; keep raw stores/captures/logs private and clean only exact job-owned IDs.
- P2: workflow_dispatch alone cannot bootstrap a branch-only workflow. Provide an
  explicitly requested same-repository PR-label route without premature merging.
- P2: Linux could silently change snapshot backend or be treated as noise-free.
  Verify the same copy backend in the pilot and retain same-VM noise controls.

Repeat review found no further P0-P2 contradiction in this transport/component
proposal. Linux port behavior, timing budget and executed qualification remain
unproven until the approved tests and remote pilot actually run.
