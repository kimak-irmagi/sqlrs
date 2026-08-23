# Release-Gated Happy Path E2E

## Scope

This document defines MVP release validation for the local profile:

- release bundle contract;
- happy-path E2E gating in GitHub Actions;
- component interaction flow for RC to GA promotion;
- internal component structure for CLI, engine, and CI services.

The design goal is to ensure users receive the same binaries that passed E2E.

---

## Constraints and Principles

- Validate artifacts, not source tree state.
- Use deterministic comparisons with normalization of unstable fields.
- Keep happy-path scenarios short and reproducible.
- Validate both fallback and CoW snapshot paths in release-gated Linux E2E.
- Preserve cross-platform release bundles.
- Prefer `build once -> test -> promote` over rebuild-on-release.

---

## Release Bundle Contract

Current release bundles follow:

- `linux/amd64` (`.tar.gz`)
- `linux/arm64` (`.tar.gz`)
- `windows/amd64` (`.zip`)
- `darwin/amd64` (`.tar.gz`)
- `darwin/arm64` (`.tar.gz`)

Per target, release output includes:

- archive: `sqlrs_<version>_<os>_<arch>.<ext>`
- checksum: `sqlrs_<version>_<os>_<arch>.<ext>.sha256`

An additional manifest artifact is required:

- `release-manifest.json` with target list, checksums, workflow run id,
  and commit SHA.

---

## Happy-Path Scenario Catalog

Happy-path scenarios are derived from `examples/`.

Release-blocking scenarios for MVP:

- `hp-psql-chinook`: prepare from `examples/chinook/prepare.sql`, then run `examples/chinook/queries.sql`.
- `hp-psql-sakila`: prepare from `examples/sakila/prepare.sql`, then run `examples/sakila/queries.sql`.

Extended non-blocking scenarios:

- `hp-psql-flights-smoke`: minimal prepare+query for flights dataset.
- `hp-lb-jhipster`: Liquibase-based flow from `examples/liquibase/jhipster-sample-app`
  (runner/tooling dependent).

Scenario metadata is declared in:

- `test/e2e/release/scenarios.json`

---

## Component Interaction Flow

1. Maintainer pushes RC tag `vX.Y.Z-rc.N`.
2. `build_rc` compiles `sqlrs` and `sqlrs-engine` for all targets and packages bundles.
   The Windows archive is self-contained: it includes the native Windows engine
   and the matching Linux engine payload under `libexec/linux-<arch>/`.
3. `build_rc` publishes archives, checksums, and `release-manifest.json` as
   workflow artifacts.
4. `e2e_happy_path` downloads RC artifacts and runs release-gated matrix in
   clean runners with axes `platform x scenario x snapshot_backend`.
   Current blocking cells are:
   - Linux: `hp-psql-chinook`/`hp-psql-sakila` with `copy` and `btrfs`.
   - Windows: `hp-psql-chinook` with `copy` (native host engine connected to
     the Linux Docker daemon in WSL) and `btrfs` (host `sqlrs.exe` + WSL
     runtime).
     Both cells extract only the published Windows archive; the WSL cell must
     use its bundled Linux payload rather than a separately downloaded Linux
     release.
   - macOS podman probe: `hp-psql-chinook` with `copy`, runtime forced by
     `container.runtime=podman`, with double `prepare+run` pass in one workspace.
     Podman machine startup in this cell is release-blocking.
5. Each scenario run normalizes output and compares against golden snapshots.
6. `publish_rc` creates/updates pre-release and attaches validated artifacts if
   all required E2E jobs passed.
7. Maintainer pushes GA tag `vX.Y.Z`.
8. `promote_ga` fetches RC assets, validates manifest provenance
   (`release-manifest.json` `source_sha` equals the GA tag commit SHA), verifies
   checksums, and publishes the final release without rebuilding.

Failure at any validation step must block promotion.

---

## Internal Component Structure

### CLI deployment unit (`frontend/cli-go`)

- `cmd/sqlrs`: process entrypoint.
- `internal/cli`: command parsing/dispatch for `init`, `prepare`, `run`, `plan`,
  `rm`, `status`.
- `internal/app`: orchestration, local engine lifecycle, config and IO wiring.
- `internal/client`: HTTP transport to local engine.

Responsibilities for E2E:

- execute scenario commands exactly as users do;
- produce stable text/JSON outputs used by golden comparison.

Data ownership:

- in-memory: command execution state;
- persistent: workspace-local state files and generated outputs in E2E temp dirs.

### Engine deployment unit (`backend/local-engine-go`)

- `cmd/sqlrs-engine`: process bootstrap and dependency wiring.
- `internal/httpapi`: API layer used by CLI.
- `internal/prepare`, `internal/run`, `internal/deletion`: workflow services.
- `internal/store/sqlite`: persistent metadata store.
- `internal/runtime`, `internal/snapshot`, `internal/statefs`, `internal/dbms`:
  runtime and state handling.

Responsibilities for E2E:

- run real prepare/run lifecycle for happy-path scenarios;
- persist/emit logs and state transitions for assertions/debug artifacts.

Data ownership:

- in-memory: job/task coordination and runtime process state;
- persistent: sqlite metadata, state directories, snapshot/state files in runner
  workspace.

### CI service unit (GitHub Actions + scripts)

- `.github/workflows/release-local.yml`: release build and packaging pipeline.
- planned workflow split/extension:
  - `build_rc` and artifact publication;
  - `e2e_happy_path` matrix execution;
  - `publish_rc` pre-release publication;
  - `promote_ga` final release promotion.
- harness scripts:
  - `scripts/e2e-release/run-scenario.mjs`
  - `scripts/e2e-release/normalize-output.mjs`
  - `scripts/e2e-release/compare-golden.mjs`
  - `scripts/e2e-release/smoke-bundle.mjs`
  - `scripts/e2e-release/create-release-manifest.mjs`

Responsibilities for E2E:

- isolate scenario runs in clean runner workspace;
- pass snapshot backend profile (`copy`/`btrfs`) to scenario init command;
- collect logs, normalized outputs, and diffs as artifacts;
- enforce gating policy for release publication.

Data ownership:

- in-memory: per-job runtime state in workflow;
- persistent: workflow artifacts, release assets, and golden files committed in
  repository.

---

## Runner Strategy for 3 Platforms

Current blocking profile:

- Happy-path release matrix is blocking with platform axis:
  - Linux: scenarios `hp-psql-chinook`, `hp-psql-sakila`; backends `copy`, `btrfs`.
  - Windows: scenario `hp-psql-chinook`; backends `copy`, `btrfs`
    (`copy` uses the native host engine with a WSL Linux Docker daemon,
    `btrfs` uses the WSL-backed runtime).
- macOS runs bundle + command smoke checks, and an additional podman probe
  (`hp-psql-chinook`, `copy`, two consecutive `prepare+run` executions).

Target state:

- full happy-path E2E on Linux, Windows, and macOS before GA promotion.

This adds CoW-path confidence immediately while preserving the path to strict
3-platform gating.

---

## Comparison and Diagnostics

Golden comparison pipeline:

1. run scenario and capture raw outputs;
2. normalize unstable fields;
3. compare normalized outputs with committed golden snapshots.

Failure diagnostics must upload:

- raw outputs;
- normalized outputs;
- unified diffs;
- engine and workflow logs;
- scenario manifest and environment metadata.

---

## Approved Decisions

- Current task uses phased gating with platform-aware matrix:
  Linux full happy-path E2E is blocking, and Windows
  `hp-psql-chinook + (copy|btrfs)` is blocking in the same release workflow.
- Next release-gating improvement:
  extend blocking Windows matrix beyond `hp-psql-chinook`, then enable full
  happy-path blocking on macOS.
- MVP release-blocking scenarios:
  `hp-psql-chinook` and `hp-psql-sakila`.
- Liquibase happy-path:
  non-blocking in current task; promoted to blocking after 3-platform full
  blocking is enabled.
