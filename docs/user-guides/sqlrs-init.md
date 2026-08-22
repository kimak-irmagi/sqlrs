# sqlrs init — Workspace Initialization

## Overview

`sqlrs init` initializes or validates a **sqlrs workspace** and optionally
configures how the CLI connects to an engine.

There are two init modes:

- `local` — configure local engine + snapshot store (default).
- `remote` — configure a remote engine endpoint + token.

Running `sqlrs init` with no subcommand is equivalent to `sqlrs init local`.

A workspace is identified by the presence of a `.sqlrs/` directory.
Git repositories are **explicitly ignored**; git integration is the user's responsibility.

The command follows the _principle of least surprise_:

- it never silently creates nested workspaces,
- it never changes global state unless explicitly requested,
- it fails loudly on ambiguity.

---

## Workspace Detection Rules

### What counts as a workspace

A directory is considered a sqlrs workspace if:

- a `.sqlrs/` directory exists **in that directory**.

Inside `.sqlrs/`, the presence of a config file is validated, but **the directory
itself is the primary marker**.

Expected layout (minimal):

```text
.sqlrs/
  config.yaml
```

Additional subdirectories MAY exist but are not required at init time.

### Parent workspace detection

Before initializing a workspace, `sqlrs init` scans **upwards** from the target
directory:

- if a `.sqlrs/` directory is found in any parent,
- and the target directory itself does NOT contain `.sqlrs/`,

then a **parent workspace conflict** is detected.

---

## Command Syntax

```text
sqlrs init [local] [flags]
sqlrs init remote --url <url> --token <token> [flags]
```

---

## Default Behavior

```bash
sqlrs init
```

Equivalent to:

```bash
sqlrs init local --workspace <current working directory>
```

Behavior:

1. If `<workspace>/.sqlrs/` exists:
   - validate structure
   - validate config (if present)
   - exit successfully (idempotent)
2. If `<workspace>/.sqlrs/` does NOT exist:
   - if a parent workspace exists → **error**
   - otherwise:
     - create `<workspace>/.sqlrs/`
     - create `<workspace>/.sqlrs/config.yaml` with defaults
     - apply local or remote configuration (if requested)
     - exit successfully

---

## Configuration Format

- Local workspace config format: **YAML**
- Default file name: `.sqlrs/config.yaml`
- `sqlrs init` may write workspace config when flags require it
- If config exists but cannot be parsed → treated as corruption

Global config is NOT created or modified by default.

---

## Flags

### Global flags (local and remote)

### `--workspace <path>`

Explicitly specify the workspace root.

All detection and initialization logic applies relative to this path.

### `--force`

Allow creation of a **nested workspace** even if a parent workspace exists.

This flag MUST be explicit.

### `--dry-run`

Do not create or modify any files.
Print intended actions.

### `--update`

Allow updating an existing workspace configuration.

- If `.sqlrs/` already exists, config updates are applied.
- Without `--update`, an existing workspace remains unchanged.
- If `.sqlrs/` does not exist, `--update` behaves like a normal init (creates workspace).
- If `config.yaml` is missing or corrupted, `--update` recreates it.
- If `--update` is used and local/remote init fails, the workspace config is left unchanged.

---

### Local flags (`sqlrs init local`)

### `--snapshot <auto|btrfs|overlay|copy>`

Select the snapshot backend for the local engine.

- `auto` (default): choose the best available backend.
- `btrfs`: require btrfs snapshots.
- `overlay`: require OverlayFS (Linux only).
- `copy`: force full copy snapshots (works everywhere).

### `--store <dir|device|image> [path]`

Configure how the local **state store** is provisioned.

- `dir` — use a directory as the store root.
- `device` — use a block device (format + mount as btrfs unless it is already
  btrfs and `--reinit` is not set).
- `image` — use a disk image (create + format + mount as btrfs).

If `[path]` is omitted, `sqlrs init` selects a default path per platform.

### `--store-size <N>GB`

Size for `--store image` when creating a new image.

- Required suffix: `GB`
- Default: `100GB`

### `--reinit`

Recreate the store from scratch (destructive).

Valid only when the resolved backend is btrfs (for example, `--snapshot btrfs`
or `--snapshot auto` with `--store image|device`).

### `--engine <path>`

Set the native host engine binary path in workspace config.

- Overrides environment, bundle-relative, and `PATH` discovery
- Stored in `.sqlrs/config.yaml`
- If a **relative path** is provided **and** the current working directory is inside the workspace, the path is stored **relative to `.sqlrs/config.yaml`**.
- Otherwise (absolute path, or `sqlrs init` run outside the workspace via `--workspace`), the path is stored as an **absolute path**.

On Windows this path identifies `sqlrs-engine.exe`, which is used for native
host execution and the `copy` snapshot fallback. It does not identify the Linux
engine used inside WSL2.

### `--wsl-engine <path>`

Windows only. Set the Linux engine payload that `sqlrs init local` provisions
inside the selected WSL2 distribution.

- Overrides `SQLRS_WSL_ENGINE_PATH` and bundle-relative discovery
- Must identify a Linux ELF binary for the host CPU architecture
- The source path is not used as the long-lived runtime path: init copies the
  binary into the WSL distribution and records the installed Linux path in
  `engine.wsl.enginePath`

### `--shared-cache`

Enable usage of the global state cache for this workspace.

This flag only writes local configuration.
Global cache initialization is deferred.

### `--no-start`

Do not start the local engine after init.

### `--distro <name>`

Windows only. Select the WSL distro to use when btrfs requires WSL2.

---

### Remote flags (`sqlrs init remote`)

### `--url <url>`

Remote engine endpoint (base URL).

### `--token <token>`

Bearer token used by the CLI to authenticate with the remote engine.

---

## Snapshot and Store Selection

This section defines how `sqlrs init local` resolves snapshot backend and store
placement when `--snapshot` and `--store` are not fully specified.

### Compatibility rules

- `--store device|image` is valid only when the resolved backend is btrfs
  (for example, `--snapshot btrfs` or `--snapshot auto` with `--store image|device`).
- `--snapshot overlay` is valid only on Linux and requires OverlayFS support.
- OverlayFS uses a `dir` store on the host filesystem (e.g., ext4/xfs) and does
  not require a dedicated device or image.
- `--snapshot btrfs` is valid on Linux and Windows (via WSL2). It is unsupported on macOS.

### Store type resolution (when `--store` is omitted)

The store type is selected from `dir|image` based on platform and snapshot backend:

| Platform | `--snapshot auto`                              | `--snapshot btrfs`                                    | `--snapshot overlay` | `--snapshot copy` |
| -------- | ---------------------------------------------- | ----------------------------------------------------- | -------------------- | ----------------- |
| Windows  | `image` if WSL2+btrfs is available, else `dir` | `image`                                               | error                | `dir`             |
| Linux    | `dir`                                          | `dir` if default store path is on btrfs, else `image` | `dir`                | `dir`             |
| macOS    | `dir`                                          | error                                                 | error                | `dir`             |

### Store path resolution (when `[path]` is omitted)

- `dir`: use the platform default for `SQLRS_STATE_STORE`.
- `image`:
  - Windows: `%LOCALAPPDATA%\\sqlrs\\store\\btrfs.vhdx`
  - Linux: `${XDG_STATE_HOME:-~/.local/state}/sqlrs/store/btrfs.img`
- `device`: path is **required** (no default).

### Backend resolution (when `--snapshot auto`)

If `--snapshot auto` is in effect and the store type is `dir`, the backend is
chosen by filesystem and platform:

1. btrfs (if the store path is on btrfs)
2. overlay (Linux only, if available)
3. copy (fallback)

If `--snapshot auto` is in effect and the store type is `image` or `device`,
the backend resolves to **btrfs**.

---

## Platform Notes

### Windows

When btrfs is required, the local engine runs inside WSL2:

- `sqlrs init local` provisions the btrfs store inside the selected distro,
  using a host VHDX by default.
- The Windows release bundle contains both the native Windows engine and the
  matching Linux engine payload.
- Init copies the Linux payload into the selected WSL distribution before it
  writes a WSL-backed workspace configuration.
- The CLI records WSL mount metadata in workspace config for validation.

### Engine binary discovery

Explicit configuration remains optional for a normal release installation.
The CLI resolves the native host engine in this order:

1. `--engine` during init.
2. `SQLRS_DAEMON_PATH`.
3. `orchestrator.daemonPath` from merged configuration.
4. The platform-native engine in the release bundle containing the running
   `sqlrs` executable.
5. `sqlrs-engine` (or `sqlrs-engine.exe`) on `PATH`.

On Windows, the Linux WSL engine payload is resolved independently:

1. `--wsl-engine` during init.
2. `SQLRS_WSL_ENGINE_PATH`.
3. `engine.wsl.enginePath` after provisioning.
4. `libexec/linux-<arch>/sqlrs-engine` in the release bundle containing the
   running `sqlrs.exe`.

`SQLRS_HOME` is not used for binary discovery. SQLRS home/state directories own
mutable configuration and runtime state, while release binaries are discovered
from explicit overrides or the installation layout.

Before creating a new workspace or changing its configuration, local init
validates every engine binary required by the selected runtime. If validation or
WSL provisioning fails, `.sqlrs/config.yaml` remains unchanged. Re-running
`sqlrs init local` for an existing WSL-backed workspace repairs a missing
provisioned Linux engine from the bundle without requiring `--update`; this
repairs a derived runtime artifact and does not change user configuration.

### macOS

btrfs is currently unsupported. Requests for `--snapshot btrfs` fail.

---

## Global State Interaction

`sqlrs init`:

- does NOT create global config
- does NOT repair global cache
- does NOT modify global engine state

If global resources are missing, a **hint** MAY be printed.

---

## Error Conditions and Exit Codes

| Condition                         | Message (summary)                   | Exit Code |
| --------------------------------- | ----------------------------------- | --------- |
| Parent workspace detected         | Refusing to create nested workspace | 2         |
| Config parse error                | Workspace config is corrupted       | 3         |
| Filesystem error                  | Cannot create .sqlrs directory      | 4         |
| Invalid flags / unsupported combo | Invalid arguments                   | 64        |
| Unsupported platform capability   | Snapshot backend unsupported        | 64        |
| Unknown error                     | Internal error                      | 1         |

---

## Examples

### Initialize a new local workspace

```bash
sqlrs init
```

### Initialize local workspace explicitly at path

```bash
sqlrs init local --workspace ./db-env
```

### Force local snapshot backend (copy)

```bash
sqlrs init local --snapshot copy
```

### Request btrfs on Linux (auto dir or image)

```bash
sqlrs init local --snapshot btrfs
```

### Create a btrfs image store with explicit size

```bash
sqlrs init local --snapshot btrfs --store image --store-size 200GB
```

### Use a specific WSL distro for btrfs (Windows)

```bash
sqlrs init local --snapshot btrfs --distro Ubuntu-22.04
```

### Update an existing workspace config

```bash
sqlrs init local --update --snapshot auto
```

### Configure remote engine

```bash
sqlrs init remote --url https://engine.example.com --token $SQLRS_TOKEN
```

---

## Design Rationale

- `.sqlrs/` directory is a strong, explicit workspace marker
- Parent workspace detection prevents accidental environment splits
- Subcommands make local vs remote intent explicit and keep flags relevant
- `--snapshot` and `--store` separate backend choice from storage placement

---

## State Machine / Decision Table

This section defines the **formal decision logic** for workspace creation.
It is intended to be normative for implementation.

### Definitions

- **Target path (T)** — path resolved from `--workspace` or `cwd`
- **Local marker** — directory `T/.sqlrs/`
- **Parent marker** — any `.sqlrs/` found strictly above `T`
- **Valid config** — `T/.sqlrs/config.yaml` exists and is parseable YAML

### High-level State Machine

```text
START
  |
  v
Resolve target path (T)
  |
  v
Check for parent marker
  |
  +-- found parent marker AND no local marker?
  |        |
  |        +-- --force? --> continue
  |        |
  |        +-- otherwise --> ERROR (nested workspace)
  |
  v
Check for local marker
  |
  +-- exists?
  |      |
  |      +-- validate config
  |      |       |
  |      |       +-- valid --> SUCCESS (idempotent)
  |      |       |
  |      |       +-- invalid --> ERROR (corrupt config)
  |
  +-- does not exist
         |
         +-- create .sqlrs/
         |
         +-- create config.yaml
         |
         +-- SUCCESS
```

### Decision Table

| #   | Parent `.sqlrs/` | Local `.sqlrs/` | `--force` | Config state | Result                         |
| --- | ---------------- | --------------- | --------- | ------------ | ------------------------------ |
| 1   | no               | no              | n/a       | n/a          | Create new workspace           |
| 2   | no               | yes             | n/a       | valid        | Success (idempotent)           |
| 3   | no               | yes             | n/a       | invalid      | Error: corrupt config          |
| 4   | yes              | no              | no        | n/a          | Error: nested workspace        |
| 5   | yes              | no              | yes       | n/a          | Create nested workspace        |
| 6   | yes              | yes             | n/a       | valid        | Success (local workspace wins) |
| 7   | yes              | yes             | n/a       | invalid      | Error: corrupt local config    |

### Notes and Invariants

- Local workspace **always has priority** over parent workspace.
- Nested workspaces are allowed **only** with `--force`.
- `sqlrs init` never silently switches target to a parent workspace.
- Any YAML parse error is treated as configuration corruption.
- No global state is created or modified in any state.

---

## Implementation Notes (Non-normative)

- Parent marker detection MUST stop at filesystem root.
- Symlinks SHOULD be resolved before marker detection.
- All filesystem mutations SHOULD be atomic where possible.
- `--dry-run` follows the same state machine but skips mutations.
