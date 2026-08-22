# sqlrs CLI (Go) — project specification

Path in monorepo: `frontend/cli-go`  
Legacy prototype: `frontend/cli` (mjs)

Goal: implement a cross-platform, fast-starting, low-dependency CLI for `sqlrs`
  that can:

- work with a remote `sqlrs service` via HTTP(S)+JSON
- work with a local _ephemeral_ `sqlrs service` (connect-or-start orchestration)
- stream logs/progress as NDJSON (JSON Lines)

Non-goals (for now):

- dynamic plugin loading
- gRPC / custom binary protocols
- complex interactive TUI

---

## 1. Tech constraints

- Language: Go 1.25+ (latest stable supported by CI)
- Dependencies: keep minimal; prefer stdlib for HTTP/JSON.
- Config file format: YAML (requires a YAML lib; choose one and use only it)
- Cross-platform: Windows, Linux, macOS
- Fast startup: avoid heavy frameworks; no reflection-heavy patterns.

---

## 2. Repository layout

`frontend/cli-go/` structure:

- `go.mod`
- `cmd/sqlrs/`
  - `main.go` (entry)
- `internal/app/`
  - `app.go` (root wiring)
- `internal/cli/`
  - `root.go` (command router / arg parsing)
  - `commands_*.go`
- `internal/config/`
  - `config.go` (types)
  - `load.go` (load + merge)
  - `expand.go` (env var expansion like ${SQLRSROOT})
- `internal/client/`
  - `http_client.go` (REST + NDJSON)
  - `types.go` (request/response/event DTOs)
- `internal/daemon/`
  - `orchestrator.go` (connect-or-start logic)
  - `state.go` (engine.json read/write, stale detection)
  - `lock.go` (cross-platform lock)
- `internal/paths/`
  - `paths.go` (SQLRSROOT, project config discovery)
- `internal/util/`
  - `io.go` (line reader, safe writes, etc.)
  - `errors.go`
- `README.md`
- `SPEC.md` (this file)

Notes:

- Use `internal/...` to prevent external imports.
- Keep commands small and testable (pure funcs where possible).

---

## 3. Build & run

From repo root:

```bash
cd frontend/cli-go
go build ./...
go test ./...
```

Binary:

```bash
go build -o ../../dist/bin/sqlrs ./cmd/sqlrs
```

Release build recommended flags:

```bash
go build -trimpath -ldflags="-s -w" -o ../../dist/bin/sqlrs ./cmd/sqlrs
```

Cross-compile examples:

- Windows: GOOS=windows GOARCH=amd64 go build -o ../../dist/bin/sqlrs.exe ./cmd/sqlrs
- Linux: GOOS=linux GOARCH=amd64 go build -o ../../dist/bin/sqlrs ./cmd/sqlrs
- macOS arm64: GOOS=darwin GOARCH=arm64 go build -o ../../dist/bin/sqlrs ./cmd/sqlrs

## 4. CLI UX

### 4.1 Top-level command

Executable name: sqlrs

General syntax:

```bash
sqlrs [global flags] <command> [command flags] [-- passthrough args...]
```

Global flags (keep minimal):

- `--profile <name>`: choose profile from config (default from config)
- `--endpoint <url|auto>`: override profile endpoint
- `--mode <local|remote|auto>`: override profile mode (auto = use profile)
- `--workspace <path>`: override workspace (optional)
- `--output <human|json>`: output format (default human)
- `--timeout <duration>`: request timeout override (e.g. 30s)
- `-v, --verbose`: more logs to stderr

4.2 Commands (MVP)

1. `sqlrs status`
   - Checks connection to the selected service (local or remote)
   - Prints health info in human/json

2. See [cli-contract.md](/docs/architecture/cli-contract.md) for more commands.
   (We can add more commands later; keep structure extendable.)

### 4.3 Output rules

- Human output goes to stdout; diagnostics to stderr.
- --`output json`:
  - command result is one JSON object to stdout
  - streaming uses NDJSON objects, one per line
- Do not print random logs to stdout in json mode.

## 5. Configuration system

### 5.1 SQLRSROOT and directory layout

By default, `sqlrs` follows the XDG Base Directory Specification on Unix-like systems.

#### Linux / Unix (XDG-compliant)

- Config:
  - `$XDG_CONFIG_HOME/sqlrs/`
  - default: `~/.config/sqlrs/`

- State (runtime, daemon state, locks, logs):
  - `$XDG_STATE_HOME/sqlrs/`
  - default: `~/.local/state/sqlrs/`

- Cache:
  - `$XDG_CACHE_HOME/sqlrs/`
  - default: `~/.cache/sqlrs/`

---

#### macOS

Use a single application directory (Apple convention):

- `~/Library/Application Support/sqlrs/`

Subdirectories:

- `config/`
- `state/`
- `cache/`

#### Windows

- Config:
  - `%APPDATA%\sqlrs\`

- State and cache:
  - `%LOCALAPPDATA%\sqlrs\`

---

#### Derived paths (used by the application)

The application MUST internally resolve and expose:

- ConfigDir
- StateDir
- CacheDir

All other paths (runDir, logs, engine.json, lock files) MUST be derived from
  these directories.

### 5.2 Project-local config override

Local override file: `./.sqlrs/config.yaml`

Discovery algorithm:

- Starting from current working directory, walk up parents until filesystem root.
- If a directory `.sqlrs` is found and contains `config.yaml`, use it as project
  override.
- Only one override (the nearest up the tree).

### 5.3 Merge rules

Precedence (highest wins):

1. CLI flags
2. project-local `.sqlrs/config.yaml`
3. global `${ConfigDir}/config.yaml`
4. built-in defaults

Merge strategy:

- deep-merge by keys for maps/structs
- scalars in higher precedence overwrite lower
- arrays are replaced entirely (no concat)

YAML placeholders:

- allow ${SQLRSROOT} expansion inside YAML string values
- allow ${ENV_VAR} expansion

### 5.4 Config schema (YAML)

Example:

```yaml
defaultProfile: local

client:
  timeout: 30s
  retries: 1
  output: human

orchestrator:
  startupTimeout: 5s
  idleTimeout: 120s
  runDir: "${StateDir}/run"

profiles:
  local:
    mode: local
    endpoint: auto     # auto => use engine.json or start daemon
    autostart: true
    auth:
      mode: fileToken  # fileToken reads token from engine.json
  remote-dev:
    mode: remote
    endpoint: "https://sqlrs.example.org"
    autostart: false
    auth:
      mode: bearer
      tokenEnv: SQLRS_TOKEN

```

Types:

- `mode`: `local|remote`
- `endpoint`: `auto` or URL
- auth modes:
  - `fileToken`: token comes from `engine.json` produced by local daemon
  - `bearer`: token from env var `tokenEnv`
  - `none`: no auth header

## 6. Local daemon orchestration

### 6.1 State file: engine.json

The local daemon publishes its runtime state via a JSON file.

#### Location

The `engine.json` file MUST be located under the **state directory**:

- Default path:
  - `<StateDir>/engine.json`

Where `StateDir` is resolved according to section 5.1.

---

#### Schema

```json
{
  "endpoint": "http://127.0.0.1:17654",
  "pid": 12345,
  "startedAt": "2026-01-11T12:34:56Z",
  "authToken": "base64-or-hex",
  "version": "0.1.0",
  "instanceId": "uuid"
}
```

Rules:

- Written atomically: write to temp + rename.
- Permissions:
  - best-effort restrict to current user (platform-specific; acceptable to note
    as TODO).
- CLI treats file as stale if:
  - endpoint not reachable or /v1/health fails within timeout
  - or instanceId mismatch (if available)
  - or PID clearly not running (best-effort check) AND health fails

#### Validity and staleness rules (client-side)

The CLI MUST treat engine.json as stale if ANY of the following is true:

- The `endpoint` is unreachable.
- `GET /v1/health` fails within the configured timeout.
- The returned `instanceId` does not match the value in `engine.json`.
- The recorded `pid` is not running AND health check fails.

If the file is stale:

- The CLI MAY delete or overwrite it.
- A new daemon MAY be started (subject to locking).

Stale files MUST NOT block startup.

### 6.2 Locking

Goal: avoid two CLI processes starting two daemons simultaneously.

Lock target: `${runDir}/daemon.lock`

Implementation:

- Use a cross-platform file lock abstraction.
- If lock is held:
  - wait with timeout (e.g. up to orchestrator.startupTimeout) OR exit with
    message “daemon start in progress”.
- Lock is held only during start attempt and initial health check.

### 6.3 Connect-or-start algorithm

Given selected profile in local mode:

1. If profile.endpoint != auto:
   - use it directly (still can check health)
2. Else:
   - try read `engine.json`:
     - if exists and health OK => connect
     - else => start daemon

Start daemon:

- Acquire lock
- Re-check engine.json + health (another process may have started it)
- If still not running:
  - spawn service process (see 6.4)
  - wait for health OK within startupTimeout
  - read `engine.json` (service should write it once ready)
- Release lock
- return endpoint + token

### 6.4 How to spawn the daemon

The native local engine is resolved from, in order:

1. `SQLRS_DAEMON_PATH`.
2. `orchestrator.daemonPath` in config.
3. The native engine in the release bundle containing the running CLI.
4. `sqlrs-engine` on `PATH`.

On Windows, the WSL Linux payload is resolved and provisioned independently as
defined in `docs/user-guides/sqlrs-init.md`. `orchestrator.daemonPath` always
means the native host engine.

Spawn:

- Start in background (do not attach stdin)
- Redirect stdout/stderr to files under ${SQLRSROOT}/logs/ or discard
- Pass args:
  - `--run-dir <runDir>`
  `--listen 127.0.0.1:0` (port 0 = auto) OR fixed port if desired
  `--write-engine-json <path>`
- Service is responsible for creating engine.json when ready.

If discovery finds no compatible engine, connect-or-start reports the required
runtime, attempted discovery sources, and a corrective init flag or environment
variable.

## 7. HTTP API contract (client side)

Base URL: endpoint from `profile/engine.json`.

### 7.1 Health

`GET /v1/health`

Response 200 JSON:

```json
{
  "ok": true,
  "version": "0.1.0",
  "instanceId": "uuid",
  "pid": 12345
}
```

No auth required for health.

### 7.2 Other commands

See [sql-runner-api.md](/docs/architecture/sql-runner-api.md)

### 7.3 Progress events (NDJSON stream)

`GET /v1/{objectType}/{objectId}/events`

Response 200 with NDJSON:
Each line is a JSON object:

Event types:

1. log

   ```json
   { "ts":"2026-01-11T12:00:00Z", "type":"log", "level":"info|warn|error", "message":"..." }
   ```

2. progress

   ```json
   { "ts":"...", "type":"progress", "phase":"prepare|run|snapshot", "current":3, "total":10 }
   ```

3. result (final)

   ```json
   { "ts":"...", "type":"result", "state":"succeeded|failed", "exitCode":0 }
   ```

Client behavior:

- Stream line-by-line.

- In human mode, render nicely.

- In json mode, pass through as-is (one line per event).

## 8. Error handling rules

- Any HTTP >= 400:
  - parse JSON error body if present:

    ```json
    { "error": { "code": "...", "message": "...", "details": {} } }
    ```

  - show message to user
  - exit non-zero

- Networking failure:
  - in local mode: attempt connect-or-start once (if autostart)
  - in remote mode: fail fast, suggest `--profile` / `--endpoint`
- NDJSON parsing:
  - tolerate blank lines
  - if a line is not valid JSON, treat as log line with level=warn (human) or
    emit a synthetic event (json) depending on simplicity (pick one and keep consistent)

## 9. Security notes (MVP)

- Local daemon must listen only on loopback.
- Auth token from engine.json:
  - included in Authorization header for non-health endpoints
- Avoid printing auth token in logs/output.
- If `--output json`, ensure stderr remains non-JSON diagnostics only.

## 10. Testing requirements

Unit tests:

- config load + merge + env expansion
- project config discovery walking up directories
- daemon`.json` atomic write/read + stale logic (with mocked health)
- NDJSON reader (handles partial lines, long lines)

Integration tests (optional for MVP):

- start a tiny mock HTTP server to emulate service endpoints
- verify sqlrs status and sqlrs run behave correctly

## 11. Documentation

`frontend/cli-go/README.md` must include:

- install/build instructions
- config locations
- example global config + local override
- example commands:
  - remote:

    ```bash  
    sqlrs --profile remote-dev status
    sqlrs --profile remote-dev run -- psql -c "select 1"
    ```

  - local:

    ```bash
    sqlrs status
    sqlrs run -- psql -c "select 1"
    ```

## 12. Implementation choices (explicit decisions)

- Use stdlib `net/http`, `encoding/json`
- Use a single YAML library for config:
  - preferred: `gopkg.in/yaml.v3` (small, widely used)
- CLI arg parsing:
  - prefer minimal; acceptable to use a small, stable library if it reduces code
    (but do not pull huge dependency trees)
  - If using a library, keep it limited to one (e.g., `spf13/cobra` is popular
    but heavier; prefer smaller if possible)

## 13. Milestones

MVP-0:

- sqlrs status (remote)
- config load/merge
- basic HTTP client

MVP-1:

- local orchestration: connect-or-start, engine.json + lock
- MVP-2:
- run with NDJSON events streaming

MVP-3:

- prepare job type
- polish output formats and exit codes
