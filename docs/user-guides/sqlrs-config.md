# sqlrs config

## Overview

`sqlrs config` manages **server-side** configuration.
The CLI reads and writes configuration via the engine API.

Local mode stores configuration in a JSON file next to the state store.
Remote mode applies to the server deployment.

Liquibase executable selection is not public server configuration. Writes to
the reserved liquibase namespace are rejected. Obsolete workspace liquibase.exec,
liquibase.exec_mode and liquibase.env keys are also rejected by the CLI, including
empty or null values; they do not select a runner in either deployment mode.
Use --liquibase-version or the file-backed liquibase_version attribute instead;
see the [approved container contract](sqlrs-liquibase-container-cli-design.md).

---

## Command Syntax

```text
sqlrs config get <path> [--effective]
sqlrs config set <path> <json_value>
sqlrs config rm <path>
sqlrs config schema
```

## Output formatting

`sqlrs config` honors the global `--output` flag:

- `--output human` (default) prints diagnostic lines and pretty-prints JSON.
- `--output json` prints compact JSON only, with no extra text.

---

## Path syntax

Paths use JavaScript-like notation:

- Dot access: `orchestrator.jobs.maxIdentical`
- Array index: `limits.jobs[0].maxIdentical`

Keys are case-sensitive. Array indices must be integers.

---

## Value encoding

`set` accepts JSON values:

```text
sqlrs config set orchestrator.jobs.maxIdentical 2
sqlrs config set auth.enabled true
sqlrs config set limits.tags ["ci","local"]
sqlrs config set limits.rules {"max":2,"ttl":"1h"}
sqlrs config set featureFlag null
sqlrs config set log.level info
sqlrs config set ui.title "\"Hello world\""
sqlrs config set ui.title="\"Hello world\""
sqlrs config set ui.title '"Hello world"'
```

`null` is a valid value. To remove a key, use `rm`.

---

## Snapshot backend selection

The local engine can select a snapshot backend via configuration.

Path: `snapshot.backend`

Allowed values:

- `"auto"` (default) - pick the best available backend.
- `"overlay"` - force OverlayFS (Linux only).
- `"btrfs"` - force btrfs subvolume snapshots (Linux/WSL2 only).
- `"copy"` - force full copy snapshots (works everywhere).

If the requested backend is unavailable, the engine falls back to `"copy"` and
emits a warning event in the prepare job logs.

Examples:

```text
sqlrs config set snapshot.backend "auto"
sqlrs config set snapshot.backend "btrfs"
sqlrs config set snapshot.backend "copy"
```

---

## Container runtime selection

The local engine can select the container runtime via configuration.

Path: `container.runtime`

Allowed values:

- `"auto"` (default) - probe `docker` first, then `podman`.
- `"docker"` - use Docker only.
- `"podman"` - use Podman only.

Operational override:

- `SQLRS_CONTAINER_RUNTIME` can override the configured mode for CI/debug runs.

Example:

```text
sqlrs config set container.runtime "auto"
sqlrs config set container.runtime "docker"
sqlrs config set container.runtime "podman"
```

---

## Commands

### 1) `get`

```text
sqlrs config get <path> [--effective]
```

- Returns the configured value at `<path>`.
- `--effective` returns the value after applying defaults.

### 2) `set`

```text
sqlrs config set <path> <json_value>
```

- Sets `<path>` to the provided JSON value.
- Applies schema validation and any semantic rules.

### 3) `rm`

```text
sqlrs config rm <path>
```

- Removes the key at `<path>`.
- The effective value may still be provided by defaults.

### 4) `schema`

```text
sqlrs config schema
```

- Prints the JSON Schema used for validation.

---

## Examples

```text
sqlrs config get orchestrator.jobs.maxIdentical
sqlrs config get orchestrator.jobs.maxIdentical --effective
sqlrs config set orchestrator.jobs.maxIdentical 2
sqlrs config rm orchestrator.jobs.maxIdentical
sqlrs config schema
```
