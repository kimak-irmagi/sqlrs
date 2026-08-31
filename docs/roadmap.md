# Taidon Roadmap

This roadmap prioritises scenarios, use cases, and components to maximise early
product value while keeping a clear path to team (on-prem) and cloud/education offerings.

---

## Goals and Non-Goals

### Goals

- Deliver a fast, reproducible database instance for developers (local-first)
- Provide a single invariant core (Engine + API) across all deployment profiles
- Enable CI/CD integration for team adoption
- Preserve a clean upgrade path to public cloud sharing and education workflows

### Non-Goals (initially)

- Full multi-tenant billing and payments
- Support for many database engines at once
- Full browser-based IDE hosting (VS Code-in-browser)

---

## Roadmap Overview

```mermaid
gantt
    title Taidon Roadmap (High Level)
    dateFormat  YYYY-MM-DD
    axisFormat  %b %Y

    section Foundation
    Core API + Engine skeleton                 :done, a1, 2026-01-01, 30d
    Local runtime + instance lifecycle         :done, a2, after a1, 45d
    Filesystem snapshot backend (overlayfs)    :done, a2fs, after a2, 20d
    Filesystem snapshot backend (Btrfs)        :done, a2b, after a2fs, 30d
    Filesystem snapshot backend (ZFS)          :a2z, after a2b, 30d
    Liquibase adapter (apply migrations)       :done, a3, after a2b, 30d

    section Product MVP (Local)
    CLI UX + deterministic runs                :done, b2, after a2, 45d
    State cache v1 (reuse states)              :done, b3, after a3, 45d
    State cache guardrails (size + eviction)   :done, b3g, after b3, 30d
    Git-aware CLI (ref/diff/provenance)        :done, b4, after b3, 30d

    section Team (On-Prem)
    Shared control plane + policy baseline     :c1, after b2, 45d
    Team connectivity and workflow adoption    :c0, after c1, 45d
    Team deployment gateway baseline           :c6, after c1, 30d
    Artifact handling + audit baseline         :c2, after c1, 45d
    CI/CD integration templates                :active, c3, after c1, 30d
    Users/orgs client contract slice           :done, c4a, after c1, 20d
    Google OIDC CLI auth session slice         :done, c4b, after c4a, 20d
    Remote source input sync client/API slice  :done, c4c, after c4b, 15d
    Auth + tenant access baseline              :c4, after c1, 45d
    Shared capacity scaling                    :c5, after c2, 30d

    section Cloud (Sharing)
    Sharing artefacts (immutable runs)         :d1, after c2, 45d
    Public read-only pages                     :d2, after d1, 30d
    Anti-abuse limits (rate/quota)             :d3, after d1, 30d

    section Research / Optional
    Git-backed workspace integration           :o1, after d2, 45d
    PR automation and warmup workflows         :o2, after o1, 45d

    section Education
    Course/Assignment/Submission model         :e1, after d2, 45d
    Autograding runner                         :e2, after e1, 45d
```

> Dates are placeholders to visualise ordering. The roadmap is milestone-driven.

---

## Status (as of 2026-08-31)

- **Done**: local engine API surface (health, config, names, instances, runs,
  states, prepare jobs, tasks), local runtime and lifecycle, end-to-end
  init/prepare/run pipeline, job/task persistence and events, StateFS abstraction,
  state cache foundations and retention rules, CLI local surface (`sqlrs init`,
  `sqlrs config`, `sqlrs ls`, `sqlrs status`, `sqlrs plan:psql`, `sqlrs plan:lb`,
  `sqlrs prepare:psql`, `sqlrs prepare:lb`, `sqlrs run:psql`, `sqlrs run:pgbench`,
  `sqlrs rm`), WSL init flow (incl. nsenter install), instance-delete logging.
- **Deferred (names follow-up)**: deeper runtime-name workflows and any
  alias-to-name linkage remain intentionally out of M2 local DX scope; the
  current names list/get API is part of the local engine baseline, but richer
  name orchestration is postponed.
- **Done (filesystem)**: overlayfs-based copy stub and Btrfs snapshot backend.
- **Done (PR #37-#41 hardening)**: release happy-path e2e scenarios landed for
  Chinook/Sakila with matrix expansion (incl. Btrfs), `init --btrfs` behaviour
  unified across Linux/Windows, Windows WSL/docker probe moved into release
  verification, and deterministic output/workspace handling was tightened.
- **Done (release identity)**: RC bundles embed the full source tag in the CLI,
  native engine, and bundled WSL engine while non-release builds remain `dev`.
- **Done (MVP command surface)**: local MVP command set is stable around
  `init/config/ls/status/plan/prepare/run/rm`; legacy command naming is
  deprecated in docs.
- **Done (bounded cache core)**: the local engine already supports
  `cache.capacity.*` configuration, strict enforcement before/after snapshot
  phases, deterministic eviction of unreferenced leaf states, and structured
  errors when space cannot be reclaimed.
- **Done (bounded cache hardening)**: local operator-facing diagnostics now ship
  in the public CLI surface via `sqlrs status`, `sqlrs status --cache`, and
  `sqlrs ls --states --cache-details`; persisted `size_bytes` metadata and a
  dedicated release cache-pressure scenario are also in place for the local
  profile.
- **Done (M2 alias execution baseline)**: repo-tracked prepare and run aliases
  now back `sqlrs plan <ref>`, `sqlrs prepare <ref>`, and standalone
  `sqlrs run <ref> --instance ...` through `*.prep.s9s.yaml` and
  `*.run.s9s.yaml` files, with exact-file escape via trailing `.`, cwd-relative
  alias-ref resolution, alias-file-relative file-bearing paths, and mixed raw
  and alias `prepare ... run` composition with `--instance` guardrails.
- **Done (M2 alias inspection baseline)**: the CLI now ships
  `sqlrs alias ls` and `sqlrs alias check [<ref>]` with bounded scan scope,
  default `cwd + recursive`, explicit `--from` / `--depth` controls, static
  validation for both scan and single-alias modes, and invalid-entry reporting
  for malformed alias files.
- **Done (M2 alias authoring + advisory discovery baseline)**: the CLI now also
  ships `sqlrs alias create` plus `sqlrs discover --aliases`, including
  copy-paste alias creation commands, bounded workspace traversal with progress
  reporting, JSON/human rendering, and shared alias/discover heuristics over the
  current repository slice.
- **Done (M2 generic discover analyzers)**: `sqlrs discover` now runs as a
  registry-driven multi-analyzer advisory surface, with additive
  `--aliases` / `--gitignore` / `--vscode` / `--prepare-shaping` selection,
  stable default execution over all shipped analyzers, analyzer-grouped
  human/JSON output, failure isolation, and shell-aware follow-up commands for
  `.gitignore` and the sqlrs YAML schema mapping in `.vscode/settings.json`.
  Extension recommendations and deeper input-graph shaping remain tracked in
  [#90](https://github.com/kimak-irmagi/sqlrs/issues/90) and
  [#91](https://github.com/kimak-irmagi/sqlrs/issues/91).
- **Done (CLI maintainability PR4 stage pipeline)**: direct and alias-backed
  `plan` / `prepare` flows in `internal/app` now share one package-local stage
  pipeline for image resolution, kind-specific binding, terminal invocation,
  and cleanup handling, reducing duplicated orchestration without changing the
  public CLI contract.
- **Done (CLI maintainability PR5 WSL init split)**: WSL-backed local init in
  `internal/app` is now split into bootstrap, storage, and mount-finalization
  helpers, shared WSL path conversion moved out of `app.go`, and the existing
  `init local`/`resolveWSLSettings`/cleanup-spinner behavior remains covered by
  targeted regression tests.
- **Done (M2 shared inputset + diff path baseline)**: the CLI now ships
  `sqlrs diff --from-path/--to-path` for `prepare:psql` and `prepare:lb`, while
  `prepare`, `plan`, `run`, `diff`, and `alias check` now share one
  kind-specific `internal/inputset` layer for file-bearing semantics and local
  closure collection across `psql`, `liquibase`, and `pgbench`.
- **Done (M2 diff ref-mode hardening)**: ref-mode worktree resolution now keeps
  the caller cwd stable across symlinked paths, and `psql` include semantics
  stay aligned with `psql` (`\i` / `\include` from command cwd, `\ir` /
  `\include_relative` from the including file directory). The CLI also keeps
  `worktree` as the default ref mode until explicit `blob` reads match those
  filesystem semantics safely.
- **Done (M2 bounded local ref execution baseline)**: `sqlrs plan` and
  `sqlrs prepare` now support bounded local `--ref` reads across raw and
  alias-backed flows, with projected-cwd semantics aligned with `sqlrs diff`,
  shared `internal/refctx` ownership for repo/ref/worktree handling, and shared
  filesystem-aware alias-target resolution in `internal/alias`. In this first
  public slice, `worktree` remains the default ref mode and `prepare --ref`
  stays watch-only.
- **Done (M2 provenance + cache explain baseline)**: single-stage local
  `plan` / `prepare` now support `--provenance-path` JSON side artifacts across
  raw, alias-backed, and bounded `--ref` flows; the CLI also ships read-only
  `sqlrs cache explain prepare ...` with matching raw/alias/ref binding
  semantics, plus a dedicated local engine `POST /v1/cache/explain/prepare`
  endpoint and regression coverage for hit/miss, rendering, and binding parity.
  More specific cache-miss reasons and distinct accepted/canceled provenance
  outcomes remain tracked in
  [#92](https://github.com/kimak-irmagi/sqlrs/issues/92) and
  [#93](https://github.com/kimak-irmagi/sqlrs/issues/93).
- **Done (M2 standalone `run --ref` baseline)**: standalone local
  `run`, `run:psql`, and `run:pgbench` now support bounded `--ref` reads across
  raw and alias-backed flows, reusing the existing projected-cwd ref context and
  inputset semantics; the CLI also rejects ref-backed `prepare ... run`
  composites at the boundary until a dedicated composite design is accepted.
- **Done (release alias/workspace coverage)**: release/e2e scenarios now
  exercise repo-tracked prepare aliases for Chinook, Sakila, and
  Liquibase/JHipster examples, keeping alias/workspace conventions under
  verification.
- **Done (CLI alias coverage hardening)**: `internal/alias` regression tests now
  cover the remaining create, resolve, and scan edge branches, bringing the CLI
  alias package above the minimum coverage threshold in `go test --cover ./...`.
- **Done (remote users/orgs client/API contract slice)**: the CLI now has
  remote-only `sqlrs user me`, `sqlrs user register`, `sqlrs user create`,
  `sqlrs org create`, `sqlrs org ls`, and `sqlrs org get` flows backed by the
  documented users/organizations OpenAPI contract. The slice deliberately
  excludes local-engine support and server-side persistence/provider
  implementation. Actionable registration and remote-profile error guidance is
  tracked in [#94](https://github.com/kimak-irmagi/sqlrs/issues/94).
- **Done (Google OIDC CLI auth session slice)**: the CLI now has
  `sqlrs auth login google`, `sqlrs auth status`, and `sqlrs auth logout`
  backed by Google Authorization Code Flow with PKCE, loopback redirect
  handling, OS credential-store refresh-token storage, cached ID-token refresh,
  `SQLRS_TOKEN` debug override precedence, and temporary Google Desktop
  `client_secret` forwarding to the Google token endpoint when configured. The
  gateway still receives only short-lived Google ID tokens; accepting the CLI
  OAuth client audience remains a gateway configuration/follow-up concern where
  needed.
- **Done (remote source input sync client/API slice)**: the OpenAPI contract and
  CLI now support remote source-input synchronization for remote `prepare`,
  `plan`, and `cache explain prepare` flows. The client attaches a bounded
  `source_manifest`, handles recoverable `source_inputs_missing` responses,
  uploads requested blobs through `/v1/source-blobs/sha256/{digest}`, emits
  typed sync events rendered as verbose stderr lines or a delayed interactive
  spinner (with normal non-TTY output kept silent), honors per-profile
  `sourceSync` policy, and uses the
  selected ref filesystem for ref-backed prepare stages. Local profile behavior
  is unchanged. The production handshake now also sends absolute client
  `root_path`/`work_dir` coordinates, retains absolute public prepare arguments,
  uses a dedicated 15-minute source-transfer timeout, and interoperates with
  the gateway-owned admission projection without changing the CLI syntax.
  Remote path binding is isolated from local WSL configuration, preventing
  Windows host paths from being rewritten into `/mnt/...` coordinates.
  Upload progress reports actual streamed bytes at bounded checkpoints, stable
  unique-upload ordinals, and a final transferred-byte total, and identifies
  files only by workspace-relative path and shortened digest. The remote
  Liquibase executable trust boundary remains tracked as a defect in
  [#89](https://github.com/kimak-irmagi/sqlrs/issues/89).
- **In progress (CI templates baseline)**: GitHub Actions-based release/e2e flows
  are active; broader team templates (e.g., GitLab and on-prem deployment variants)
  are still pending.
- **Tracked unfinished implementation work**: remote Liquibase executable
  ownership (#89), richer discover analyzers (#90/#91), specific cache-miss
  explanations (#92), asynchronous provenance outcomes (#93), and actionable
  users/orgs error guidance (#94). Direct links are collected below.
- **Planned**: ZFS snapshot backend, optional VS Code integration, team on-prem
  baseline, cloud sharing, education.

---

## Tracked Unfinished Work

- [#89](https://github.com/kimak-irmagi/sqlrs/issues/89): make Liquibase runner
  selection backend-owned at the remote API trust boundary.
- [#90](https://github.com/kimak-irmagi/sqlrs/issues/90): extend
  `discover --vscode` with extension recommendations.
- [#91](https://github.com/kimak-irmagi/sqlrs/issues/91): deepen
  `discover --prepare-shaping` with input-graph and alias-layout analysis.
- [#92](https://github.com/kimak-irmagi/sqlrs/issues/92): explain concrete
  cache-miss causes beyond `no_matching_state`.
- [#93](https://github.com/kimak-irmagi/sqlrs/issues/93): distinguish accepted
  asynchronous jobs and cancellation in provenance artifacts.
- [#94](https://github.com/kimak-irmagi/sqlrs/issues/94): add actionable
  registration and remote-profile guidance to users/orgs CLI errors.

Additional unscheduled directions without dedicated issues remain composite
ref-backed `prepare ... run`, run-side provenance/cache-explain parity, and
zero-copy/cache-hit reuse. Hosted workflow expansion, Git mutation flows, and
broader team/cloud repository features remain deferred beyond the next local
slice.

---

## Milestones

### M0. Architecture Baseline

**Outcome**: stable concepts and contracts before heavy implementation.

- Freeze canonical entities: project, instance, run, artefact, share
- Freeze core API surface (create/prepare/run/remove + status/events)
- Decide runtime isolation approach for MVP (local containers vs other)

**Status**: Done (architecture captured via ADRs and the local engine OpenAPI spec).

**Completed baseline documents**:

- [`sqlrs-engine.openapi.yaml`](api-guides/sqlrs-engine.openapi.yaml)
- [`local-deployment-architecture.md`](architecture/local-deployment-architecture.md)
- [`state-cache-design.md`](architecture/state-cache-design.md)

---

### M1. Local MVP (Scenario A1)

**Primary scenario**: A1 local development with Liquibase.

**Target use cases**:

- UC-1 Provision isolated database instance
- UC-2 Apply migrations (Liquibase / SQL)
- UC-3 Run tests / queries / scripts
- UC-4 Cache and reuse database states

**Deliverables**:

- Taidon Engine + API (local mode) — **Done** (local OpenAPI spec)
- Local runtime (containers) with instance lifecycle — **Done**
- CLI surface (local): `sqlrs init`, `sqlrs config`, `sqlrs ls`, `sqlrs status`,
  `sqlrs plan:psql`, `sqlrs plan:lb`, `sqlrs prepare:psql`, `sqlrs prepare:lb`,
  `sqlrs run:psql`, `sqlrs run:pgbench`, `sqlrs rm` — **Done**
- Cache v1 (prepare jobs + state reuse + retention) — **Done (core)**
- Cache capacity guardrails (size limits + eviction) — **Done** (core
  enforcement, CLI diagnostics, persisted size metadata, release
  cache-pressure coverage for the local profile)
- Filesystem snapshot backends — **Done** (overlayfs copy stub, Btrfs),
  **Planned** (ZFS)
- Liquibase adapter (apply changelog) — **Done (MVP scope)** (local flow via
  `prepare:lb`/`plan:lb` is implemented)
- Release happy-path e2e gate — **Done** (Linux/macOS/Windows WSL coverage for
  Chinook, Sakila, and Liquibase/JHipster alias-driven scenarios, with Btrfs in
  matrix validation and a dedicated local cache-pressure scenario in release
  checks)
- Self-contained local release discovery — **Done** (native engines are found
  beside the CLI or on `PATH`; Windows archives also carry and atomically
  provision the matching Linux engine for WSL, including repeat-init repair)

**Optional (nice-to-have)**:

- VS Code extension v0:
  - list instances
  - apply migrations
  - show logs and run results

**Exit criteria**:

- A cold start produces a working instance
- A warm start reuses cached state and is significantly faster
- Migrations are deterministic and reproducible

**Status**: Done (MVP baseline). Remaining public local follow-up now sits
primarily in M2 developer experience and optional runtime extensions such as ZFS.

---

### M2. Developer Experience (Post-MVP Local)

**Purpose**: reduce local onboarding friction and improve reproducibility tooling.

**Target use cases**:

- UC-1, UC-2, UC-3 (with minimal config)

**Deliverables**:

- Project-tracked workflow aliases:
  - `*.prep.s9s.yaml` and `*.run.s9s.yaml`
  - explicit cwd-relative alias-ref resolution for `plan`, `prepare`, and `run`
  - alias-file-relative resolution for file-bearing paths stored inside alias
    files
  - normal `prepare ... run` composition across raw and alias modes
  - explicit alias inspection via `sqlrs alias ls/check`
  - explicit alias creation via `sqlrs alias create`
- Advisory discovery tooling:
  - `sqlrs discover` with `--aliases`, `--gitignore`, `--vscode`, and
    `--prepare-shaping`
  - copy-paste follow-up commands for alias creation plus `.gitignore` and
    `.vscode` remediation suggestions
- Git-aware CLI (passive):
  - `diff` path mode
  - bounded local `--ref` for `plan` / `prepare` / standalone `run` plus
    ref-aware `diff` semantics (`worktree|blob`)
  - provenance and cache explain
- VS Code extension v1 (optional):
  - one-click copy DSN
  - open SQL editor (via existing VS Code DB tooling)

**Exit criteria**:

- A developer can run common workflows using explicit repo-tracked recipes with
  low local setup friction and clear cache provenance diagnostics.

**Status**: Done (public/local baseline). Alias execution, inspection,
authoring, generic advisory discovery, shared `internal/inputset` semantics,
`sqlrs diff`, bounded local `plan` / `prepare` / standalone `run` `--ref`, and
provenance/cache explain are landed. Tracked implementation follow-ups are
[#90](https://github.com/kimak-irmagi/sqlrs/issues/90),
[#91](https://github.com/kimak-irmagi/sqlrs/issues/91),
[#92](https://github.com/kimak-irmagi/sqlrs/issues/92), and
[#93](https://github.com/kimak-irmagi/sqlrs/issues/93). Broader ref-backed
composites, run-side diagnostics, and zero-copy/cache-hit reuse remain outside
the accepted M2 baseline and do not yet have dedicated issues.

---

### M3. Team On-Prem (Scenario A2)

**Primary scenario**: shared Taidon for a team or department.

**Target use cases**:

- UC-5 Integrate with CI/CD
- UC-1..UC-4 in a shared authenticated deployment

**Deliverables**:

- Shared control-plane baseline for authenticated multi-user deployments
- Remote users/orgs API contract and CLI client surface — **Done (client/API
  contract slice)**; server-side persistence/provider implementation remains in
  the shared control-plane/auth baseline. CLI error-guidance follow-up:
  [#94](https://github.com/kimak-irmagi/sqlrs/issues/94).
- Google OIDC CLI auth sessions — **Done (CLI slice)**; gateway accepted
  audiences and server-side auth/tenant policy remain in the shared
  control-plane/auth baseline.
- Remote source input sync — **Done (client/API contract slice)** for remote
  prepare/plan/cache-explain retries and source blob uploads; server-side source
  resolution and storage implementation remain part of the shared control-plane
  baseline. Remote Liquibase trust-boundary defect:
  [#89](https://github.com/kimak-irmagi/sqlrs/issues/89).
- Team deployment gateway and service entrypoint baseline
- Shared state, artifact, and audit handling with retention controls
- Basic auth, tenant access, quotas, and policy enforcement
- CI/CD integration templates and operator deployment path
- Team workflow compatibility and capacity scaling baseline

**Exit criteria**:

- Multiple developers can run isolated instances concurrently
- Quotas prevent resource exhaustion
- CI pipelines can provision and teardown instances reliably

---

### M4. Public Cloud Sharing (Scenario B3)

**Primary scenario**: quick experiments and public sharing.

**Target use cases**:

- UC-6 Share experiment results

**Deliverables**:

- Immutable run snapshots:
  - shareable artefact bundles
  - redaction of secrets
- Public read-only pages:
  - view results
  - reproduce button (clone into user workspace)
- Anti-abuse controls:
  - rate limiting
  - instance limits
  - TTL enforcement

**Exit criteria**:

- A user can share a run via link
- Another user can reproduce it in a controlled environment

---

### R1. Hosted Git Integration (Optional / Research)

**Purpose**: connect hosted or shared environments to repository revisions
without making local workflows dependent on hosted infrastructure.

**Deliverables**:

- Git-backed project integration for hosted/shared environments
- Start or refresh workloads from a selected repository revision
- Secure repository access and provenance-aware refresh flow

**Exit criteria**:

- User can attach a Git repo and start a instance from a chosen branch/commit
- Repo updates are available in the instance without manual re-import

---

### R2. PR Automation (Optional / Research)

**Purpose**: automate warmup and diff workflows around PRs without exposing
credentials or runtime connection details.

**Deliverables**:

- Hosted automation path for PR-triggered warmup and diff workflows
- Integration surface for checks, callbacks, or bot-driven review signals
- Reproducible PR summaries without exposing credentials or DSNs
- Lifecycle hooks for refreshing or retiring prepared backend state

**Exit criteria**:

- PR label or slash command triggers warmup in a controlled runner
- Check Run shows diff/warmup summary without exposing secrets

---

### M5. Education (Scenarios C4a/C4b)

**Primary scenario**: assignments, submissions, and grading.

**Target use cases**:

- UC-7 Prepare assignments
- UC-8 Submit and review results

**Deliverables**:

- Course/assignment/submission model
- Autograding runner:
  - instructor-defined checks (SQL/tests)
  - structured grading report
- Instructor dashboard:
  - list submissions
  - compare results

**Exit criteria**:

- Instructor can publish assignment template
- Students submit runs
- Instructor can evaluate consistently

---

## Prioritisation Rationale

1. Local MVP (A1) yields immediate product value and validates core performance
2. Drop-in adoption reduces friction and improves feedback velocity
3. Team on-prem (A2) unlocks the enterprise path (quotas, auth, CI)
4. Sharing (B3) becomes safe once artefacts, auth, and quotas exist
5. Education (C4) builds naturally on sharing plus role-based access

---

## Scope Controls and Decisions to Lock Early

- Database engines for MVP (start with one, expand later)
- Runtime isolation mechanism (container/k8s) and snapshot approach
- Cache semantics and invalidation rules
- Security boundaries (network egress, time limits, resource limits)
- API stability policy (versioning strategy)

---

## Risks and Mitigations

- Performance regressions due to heavy isolation
  - Mitigation: cache-first architecture and benchmark gates
- Non-deterministic migrations and flaky states
  - Mitigation: strict hashing, pinned images, reproducible seed strategy
- Cloud abuse vectors (untrusted SQL)
  - Mitigation: strong sandboxing, quotas, network policies, TTL

---

## Detailed Architecture Documents

- [`sqlrs-engine.openapi.yaml`](api-guides/sqlrs-engine.openapi.yaml) (HTTP API + events)
- [`sql-runner-api.md`](architecture/sql-runner-api.md) (timeouts, cancel,
  streaming, cache-aware planning)
- [`local-deployment-architecture.md`](architecture/local-deployment-architecture.md)
  and [`k8s-architecture.md`](architecture/k8s-architecture.md) (local + k8s)
- [`liquibase-integration.md`](architecture/liquibase-integration.md) (modes,
  config discovery)
- [`state-cache-design.md`](architecture/state-cache-design.md) (snapshotting,
  hashing, retention)
- [`cli-contract.md`](architecture/cli-contract.md) (commands and exit codes)
- [`runtime-snapshotting.md`](architecture/runtime-snapshotting.md) (details of
  the snapshot mechanics)

Still unfinished without a dedicated GitHub issue: a consolidated security
model covering cloud hardening, redaction, and audit.
- [`git-aware-passive.md`](architecture/git-aware-passive.md) (CLI by ref,
  zero-copy, provenance)
- [`git-aware-active.md`](architecture/git-aware-active.md) (PR automation,
  warmup/diff checks)
- [`k8s-architecture.md`](architecture/k8s-architecture.md) (single entry gateway
  in k8s)
