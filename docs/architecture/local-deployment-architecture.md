# Local Deployment Architecture (sqlrs)

This document describes how `sqlrs` is deployed and executed on a **developer workstation** in the MVP.

It focuses on:

- thin CLI design
- ephemeral engine process
- interaction with the container runtime and psql

This document intentionally avoids repeating script-runner details; Liquibase integration
is planned and covered in [`liquibase-integration.md`](liquibase-integration.md).
Engine internals are detailed in [`engine-internals.md`](engine-internals.md).
Team/Cloud variant is covered in [`shared-deployment-architecture.md`](shared-deployment-architecture.md).

---

## 1. Goals

- Fast CLI startup
- Minimal permanent footprint on the host system
- Cross-platform operation (Linux, macOS, Windows via WSL2)
- Clear separation between user-facing CLI and heavy runtime logic
- Easy evolution toward a persistent daemon or team/cloud deployment

---

## 2. High-Level Topology (MVP)

```mermaid
flowchart LR
  U[User]
  CLI["sqlrs CLI"]
  ENG["sqlrs engine process"]
  DOCKER["Docker Engine"]
  DB["DB Container"]
  STATE["State dir (engine.json)"]
  STORE["state store"]

  U --> CLI
  CLI -->|spawn / connect| ENG
  ENG --> DOCKER
  DOCKER --> DB
  ENG --> STORE
  CLI -. discovery .-> STATE
```

---

## 3. sqlrs CLI

### 3.1 Responsibilities

- Parse user commands and flags
- Interact with local filesystem (project config, paths)
- Discover or spawn a local engine process
- Resolve the native engine from explicit configuration, environment, the CLI
  release bundle, or `PATH`; on Windows, resolve and provision the bundled Linux
  companion independently for WSL2/btrfs
- Communicate with engine via HTTP over loopback or Unix socket
- Execute `run` commands locally against a prepared instance/instance
- Reject remote-only user and organization management commands before local
  engine discovery or autostart
- Exit immediately after command completion
- Optional: pre-flight checks (container runtime reachable, state store writable), show engine endpoint/version for diagnostics

The CLI is intentionally **thin** and stateless.

### 3.2 Non-Responsibilities

- No container runtime orchestration logic
- No snapshotting logic
- No direct script execution
- No user profile, external identity, organization, or membership management

---

## 4. Engine Process (Ephemeral)

### 4.1 Characteristics

- Started on-demand by the CLI
- Runs as a child process (not a system daemon)
- Listens on a local endpoint (loopback or socket)
- Manages runtime state while active

### 4.2 Responsibilities

- Container runtime orchestration
- Snapshotting and state management
- Cache rewind and eviction
- Script execution via container `psql`
- Connection / proxy layer (when needed)
- Connection tracking for TTL and safe deletion
- IPC/API for CLI and future IDE integrations
- Prepare planning/execution; does not execute `run` commands
- Ephemeral instance creation for prepare

### 4.3 Lifecycle

- Spawned when required
- May persist for a short TTL after last request
- Terminates automatically when idle
- Writes its endpoint/lock and auth token into `<StateDir>/engine.json` to allow subsequent CLI invocations to reuse the same process

This avoids permanent background services in MVP.

---

## 5. IPC: CLI <-> Engine

- **Transport/Protocol**: REST over HTTP; loopback-only. Unix domain socket on Linux/macOS; TCP loopback on Windows host with WSL forwarding. No TLS in local mode.
- **Endpoint discovery**:
  - CLI checks `TAIDON_ENGINE_ADDR` env var.
  - Else reads `<StateDir>/engine.json` (contains endpoint, socket path / TCP port, PID, instanceId, auth token).
  - If not found or stale, CLI spawns a new engine; the engine writes `engine.json` when ready.
- **Security**: deny non-loopback binds; require auth token for non-health endpoints; rely on file perms (UDS) or loopback firewall; engine refuses connections from non-local addresses.
- **Versioning**: CLI sends its version; engine rejects incompatible major; CLI may suggest upgrade.

Key engine endpoints (logical):

- `POST /v1/prepare-jobs` - start prepare job (plan/execute steps, snapshot states, create instance); `plan_only` computes tasks only
- `GET /v1/prepare-jobs/{jobId}` - job status
- `GET /v1/prepare-jobs/{jobId}/events` - job event stream (NDJSON)
- `GET /v1/config` / `PATCH /v1/config` - server configuration read/write
- `GET /v1/config/schema` - configuration schema
- list names/instances/states (JSON array or NDJSON via `Accept`)
- `GET /v1/names/{name}` - read name binding
- `GET /v1/instances/{instanceId}` - read instance (supports name alias with 307 redirect to the canonical id URL when resolved by name)
- `DELETE /v1/instances/{instanceId}` - delete instance (idempotent; supports dry-run)
- `GET /v1/states/{stateId}` - read state
- `DELETE /v1/states/{stateId}` - delete state (idempotent; supports recurse/force/dry-run)
- `POST /snapshots` - manual snapshot
- `GET /cache/{key}` - cache lookup
- `POST /engine/shutdown` - optional graceful stop

Local engine does not expose `/v1/users*` or `/v1/organizations*`. Those
resources belong to shared/team/cloud deployments behind the Gateway.

### 5.1 Long-running operations: async jobs, sync CLI

- Engine handles prepare as asynchronous jobs; `POST /v1/prepare-jobs` returns `201 Created` with a job id.
- CLI supports:
  - `prepare --watch` (default): watch status/events until terminal status.
  - `prepare --no-watch`: submit and return `job_id`, `status_url`, `events_url`.
  - `watch <job_id>`: attach to an existing prepare job.
- In interactive watch mode, `Ctrl+C` opens a control prompt:
  - `stop` (with confirmation) requests cancellation,
  - `detach` disconnects while leaving the job running.
- For composite `prepare ... run ...`, `detach` skips the subsequent `run` phase
  in the same CLI process.

---

## 6. Interaction with psql

The engine executes `psql` inside the DB container via runtime `exec`.
When file inputs are present, it bind-mounts the scripts root read-only and
rewrites file arguments to the container path.

Liquibase integration is planned; provider selection details live in
[`liquibase-integration.md`](liquibase-integration.md).

---

## 6. Interaction with Container Runtime

- The local engine supports Docker and Podman CLIs.
- Runtime mode is configured via engine config `container.runtime` with values `auto|docker|podman`.
- `SQLRS_CONTAINER_RUNTIME` is an operational override for CI/debug sessions.
- In `auto` mode, runtime probe order is `docker`, then `podman`.
- For Podman on macOS, when `CONTAINER_HOST` is unset, the engine reads the default Podman connection and exports it for child runtime commands.
- Engine controls DB containers and executes `psql` via runtime `exec`
- All persistent data directories are mounted from host-managed storage
- Engine validates runtime availability on start; CLI surfaces actionable errors if missing/denied

On Windows (when btrfs is selected):

- Docker runs inside WSL2
- State store lives inside the Linux filesystem (btrfs loopback volume)
- `sqlrs init local --snapshot btrfs` installs a systemd mount unit inside the selected distro
  so the btrfs volume is visible to Docker and all child processes
- Engine verifies the mount is active before touching the store

---

## 7. Windows / WSL2 Considerations

- When btrfs is selected, engine and StateFS run inside WSL2
- When btrfs is not selected, the engine may run on the Windows host (copy backend)
- CLI may run on Windows host or inside WSL2
- Communication via localhost forwarding when the engine runs in WSL2
- Engine writes `engine.json` to the Windows state directory and receives that path via `/mnt/...` (WSL path translation)
- Engine verifies the systemd mount for `SQLRS_STATE_STORE` at startup (WSL + btrfs only)
- StateFS backend may fall back to copy-based strategy
- The Windows release contains both `sqlrs-engine.exe` for native copy mode and
  a Linux companion under `libexec/linux-<arch>/` for WSL2/btrfs. Discovery and
  provisioning are defined in
  [`local-engine-binary-discovery-flow.md`](local-engine-binary-discovery-flow.md).

---

## 8. Evolution Path

### Phase 1 (MVP)

- Ephemeral engine process
- Thin CLI
- Local-only deployment

### Phase 2

- Optional persistent local daemon (`sqlrsd`)
- Warm instance reuse
- IDE integrations

### Phase 3

- Team-shared engine
- Remote cache
- Cloud-hosted control plane

---

## 9. Non-Goals

- System-wide background service by default
- OS-specific installers or service managers
- Deep Liquibase embedding

---

## 10. Open Questions

- Unix socket vs TCP loopback as default IPC?
- Default engine TTL after last command?
- Should CLI auto-upgrade engine binary?
