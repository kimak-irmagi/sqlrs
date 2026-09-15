# Local Engine Component Structure

Update, 2026-09-14: the approved [managed-identity flow](managed-database-identity.md)
replaces fixed SQL-name assumptions. The [component/schema design](managed-database-identity-internals.md)
and test plan are approved; implementation details below describe the current generation,
not permission to infer or restore a fixed administrative role.

This document defines the current internal component layout of the local `sqlrs-engine`.

## 1. Goals

- Keep HTTP/API concerns separate from execution and persistence concerns.
- Keep state and queue metadata persistent in SQLite.
- Make filesystem snapshot responsibilities explicit.

## 2. Packages and responsibilities

- `cmd/sqlrs-engine`
  - Parses flags and wires dependencies.
  - Opens SQLite state DB, config manager, runtime, statefs, prepare/run/delete managers.
  - Resolves runtime mode from engine config: `container.runtime` (`auto|docker|podman`).
  - Allows operational override via `SQLRS_CONTAINER_RUNTIME`.
  - In `auto` mode, probes `docker` then `podman`.
  - For Podman, can set `CONTAINER_HOST` from the default podman connection when not already set.
  - Starts HTTP server and writes/removes `engine.json`.
- `internal/httpapi`
  - HTTP routing for `/v1/*` endpoints.
  - JSON and NDJSON responses (prepare events, run stream).
  - Delegates to `prepare`, `run`, `deletion`, `registry`, `config`.
- `internal/config`
  - Default config + persisted override management (`config.json`).
  - Schema exposure and path-based get/set/remove.
- `internal/prepare`
  - `prepare.PrepareService` is the public facade used by HTTP handlers.
  - Internal split:
    - `jobCoordinator`: job lifecycle, planning, queue/event transitions.
    - `taskExecutor`: runtime/task execution and instance creation.
    - `snapshotOrchestrator`: base state init, dirty cache invalidation, snapshot guard logic.
  - End-to-end prepare orchestration remains in this package: validation, planning, execution, snapshotting, state/instance creation.
  - Supports both `psql` and `lb` prepare kinds.
  - Handles `plan_only`, event streaming, and per-job lifecycle.
- `internal/prepare/queue`
  - Persistent job/task/event queue in SQLite.
  - Recovery for queued/running jobs and retention trimming by signature.
- `internal/run`
  - Executes `run:psql` and `run:pgbench` against existing instances.
  - Recreates missing runtime containers from `runtime_dir` when possible.
- `internal/deletion`
  - Builds and executes deletion trees for instances/states.
  - Applies `recurse`, `force`, `dry_run` rules.
- `internal/registry`
  - Name/id resolution and list/get operations for names/instances/states.
- `internal/statefs`
  - State store path layout + StateFS interface.
  - Wraps snapshot backend operations with cleanup and validation.
- `internal/snapshot`
  - Snapshot backend implementations and selection (`auto`, `overlay`, `btrfs`, `copy`).
- `internal/runtime`
  - Container runtime adapter (Docker/Podman CLI; init base, start/stop, exec, run container).
- `internal/dbms`
  - DBMS-specific snapshot hooks (Postgres stop/resume with `pg_ctl`).
- `internal/conntrack`
  - Connection tracking abstraction (current local wiring uses `conntrack.Noop`).
- `internal/auth`
  - Bearer token validation for protected endpoints.
- `internal/id`
  - ID format validation helpers.
- `internal/store`
  - Storage interfaces and filter models for names/instances/states.
- `internal/store/sqlite`
  - SQLite implementation of `internal/store`.
- `internal/stream`
  - List/NDJSON stream helpers for HTTP responses.

## 3. Key types and interfaces

- `prepare.PrepareService`
  - Public prepare facade for submit/status/events/delete.
- `prepare.jobCoordinator` (internal)
  - Coordinates job lifecycle, planning, task transitions, and queue writes.
- `prepare.taskExecutor` (internal)
  - Executes tasks and runtime operations.
- `prepare.snapshotOrchestrator` (internal)
  - Owns snapshot/cache hygiene and base-state init guards.
- `prepare.Request`, `prepare.Status`, `prepare.PlanTask`
  - Prepare API request/status payloads and planned task model.
- `queue.Store`
  - Persistent jobs/tasks/events API used by `prepare.PrepareService`.
- `run.Manager`, `run.Request`, `run.Event`
  - Run execution manager with streamed runtime events.
- `deletion.Manager`, `deletion.DeleteResult`
  - Deletion planner/executor and response tree model.
- `config.Store` (`config.Manager`)
  - Runtime config API for `/v1/config*`.
- `store.Store`
  - Persistent names/instances/states interface.
- `statefs.StateFS`
  - Filesystem abstraction for clone/snapshot/remove and path derivation.

## 4. Data ownership

- Metadata DB: `<state-store-root>/state.db` (names/instances/states + prepare queue tables).
- Snapshot store: `<state-store-root>/engines/<engine>/<version>/base|states/<state_id>`.
- Per-job runtime dirs: `<state-store-root>/jobs/<job_id>/runtime`.
- Engine config: `<state-store-root>/config.json`.
- Engine discovery file for CLI: `<state-dir>/sqlrs/engine.json` (path passed via `--write-engine-json`).

## 5. Dependency diagram

```mermaid
flowchart TD
  CMD["cmd/sqlrs-engine"]
  HTTP["internal/httpapi"]
  PREP["internal/prepare (PrepareService facade)"]
  PREP_COORD["prepare jobCoordinator (internal)"]
  PREP_EXEC["prepare taskExecutor (internal)"]
  PREP_SNAP["prepare snapshotOrchestrator (internal)"]
  QUEUE["internal/prepare/queue"]
  RUN["internal/run"]
  DEL["internal/deletion"]
  REG["internal/registry"]
  AUTH["internal/auth"]
  CFG["internal/config"]
  STORE["internal/store (interfaces)"]
  SQLITE["internal/store/sqlite"]
  STATEFS["internal/statefs"]
  SNAPSHOT["internal/snapshot"]
  RT["internal/runtime"]
  DBMS["internal/dbms"]
  CONN["internal/conntrack"]
  ID["internal/id"]
  STREAM["internal/stream"]

  CMD --> HTTP
  CMD --> CFG
  CMD --> SQLITE
  CMD --> STATEFS
  CMD --> RT

  HTTP --> AUTH
  HTTP --> PREP
  HTTP --> RUN
  HTTP --> DEL
  HTTP --> REG
  HTTP --> CFG
  HTTP --> STREAM

  PREP --> PREP_COORD
  PREP --> PREP_EXEC
  PREP --> PREP_SNAP
  PREP_COORD --> QUEUE
  PREP_COORD --> CFG
  PREP_COORD --> STORE
  PREP_COORD --> PREP_EXEC
  PREP_EXEC --> RT
  PREP_EXEC --> DBMS
  PREP_EXEC --> STATEFS
  PREP_EXEC --> STORE
  PREP_EXEC --> PREP_SNAP
  PREP_SNAP --> STATEFS
  PREP_SNAP --> STORE

  RUN --> RT
  RUN --> REG
  DEL --> STORE
  DEL --> CONN
  DEL --> RT
  DEL --> STATEFS
  REG --> ID
  REG --> STORE

  STATEFS --> SNAPSHOT
  SQLITE --> STORE
```

## 6. Helper Requirements and Ownership

Technical helpers are part of component contracts when they influence correctness,
determinism, safety, or observable API behavior.

- `internal/prepare` helpers
  - Own lock/marker semantics for base/state build, dirty-state detection, cache
    invalidation, Liquibase argument/path normalization, and host Liquibase
    execution mode (`native` vs `windows-bat`) semantics.
- `internal/runtime` helpers
  - Own docker error classification, startup diagnostics, `pg_hba.conf` host auth
    patching, and host-port parsing guarantees.
- `internal/run` helpers
  - Own connection-argument conflict guards and container-missing recovery triggers.
- `internal/deletion` helpers
  - Own cleanup fallback rules (`StateFS` first, filesystem fallback) and
    non-fatal handling of docker-unavailable runtime stop during deletion.
- `internal/httpapi` helpers
  - Own NDJSON prepare-event stream loop behavior (ordered emit, wait-until-event,
    terminal completion).

These helper contracts must be documented and tested against expected behavior,
not only against current implementation details.

