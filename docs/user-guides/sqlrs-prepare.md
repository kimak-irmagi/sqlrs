# sqlrs prepare

## Overview

`sqlrs prepare` is the only command that can **deterministically construct or restore**
a database state in sqlrs.

A `prepare` invocation:

1. Identifies an immutable **state** based on its arguments and inputs.
2. Ensures this state exists (by reusing or building it).
3. Creates a mutable **instance** derived from this state.
4. Returns a DSN pointing to that instance.

All reproducibility guarantees in sqlrs rely on `prepare`.

---

## Terminology

- **State** - an immutable database state produced by a deterministic preparation process.
- **Instance** - a mutable copy of a state; all database modifications happen here.

---

## Command Syntax

```text
sqlrs prepare [--provenance-path <path>] [--ref <git-ref>] [--ref-mode worktree|blob] [--ref-keep-worktree] [--watch|--no-watch] <ref>
sqlrs prepare:<kind> [--provenance-path <path>] [--ref <git-ref>] [--ref-mode worktree|blob] [--ref-keep-worktree] [--watch|--no-watch] [--image <image-id>] [--] [tool-args...]
```

Where:

- bare `prepare <ref>` resolves a repo-tracked prepare alias file
  from the current working directory (`<cwd>/<ref>.prep.s9s.yaml`);
- `:<kind>` selects the preparation variant (for example, `psql`, `lb`).
- `--provenance-path <path>` writes a JSON provenance artifact for this
  single-stage invocation.
- `--ref <git-ref>` reads prepare inputs from a Git revision resolved by the
  CLI.
- `--ref-mode` chooses `worktree` (default) or `blob`.
- `--ref-keep-worktree` keeps the detached worktree after exit in
  `worktree` mode.
- `--watch` keeps the CLI attached to the job until terminal status (default).
- `--no-watch` submits the job and exits immediately with job references.
- `--image <image-id>` overrides the base DB image.
- `tool-args` are forwarded to the underlying tool for the selected kind.

For alias mode, paths read from the alias file itself are resolved relative to
the alias file directory, not relative to the caller's current working
directory.

Alias-mode details are described in [`sqlrs-aliases.md`](sqlrs-aliases.md).
Ref-backed details are described in [`sqlrs-ref.md`](sqlrs-ref.md).

Current boundaries:

- standalone alias preparation and mixed raw/alias `prepare ... run ...` forms
  are implemented;
- `prepare --ref` is single-stage only, works with local and remote profiles,
  and preserves the caller's projected cwd inside the selected revision;
- remote profiles receive the selected files through source sync; the backend
  does not fetch the Git repository;
- `prepare --ref` keeps `worktree` as the default ref mode and offers `blob` as
  an explicit opt-in;
- `prepare --ref --no-watch` is rejected, so ref-backed prepare
  remains watch-only for now.

### Composite `prepare ... run`

`prepare` may be the first stage of a normal two-stage composite invocation:

```text
sqlrs prepare <prepare-ref> run <run-ref>
sqlrs prepare <prepare-ref> run:<kind> [--instance <id|name>] [-- <command>] [args...]
sqlrs prepare:<kind> [--watch|--no-watch] [--image <image-id>] [--] [tool-args...] run <run-ref>
sqlrs prepare:<kind> [--watch|--no-watch] [--image <image-id>] [--] [tool-args...] run:<kind> [--instance <id|name>] [-- <command>] [args...]
```

Rules:

- the `prepare` stage may use alias mode or raw mode;
- the `run` stage may use alias mode or raw mode;
- the composite still has exactly one `prepare` stage and one `run` stage;
- if the `run` stage follows `prepare` in the same invocation, it targets the
  prepared instance and must not also specify `--instance`;
- if the `prepare` stage carries `--ref`, the composite form is rejected in the
  current bounded ref slice;
- alias stages keep their kind and tool args inside the alias file and do not
  accept per-stage inline tool arguments.
- raw-stage file paths keep their normal current-working-directory-relative
  semantics, while alias-stage file paths are resolved relative to the alias
  file used by that stage.

### Variant docs

- `psql`: [`sqlrs-prepare-psql.md`](sqlrs-prepare-psql.md)
- `lb`: [`sqlrs-prepare-liquibase.md`](sqlrs-prepare-liquibase.md)

---

## Base Image Selection (Common)

The base container image id is resolved in this order:

1. `--image <image-id>` command-line flag
2. Workspace config (`.sqlrs/config.yaml`, `dbms.image`)
3. Global config (`$XDG_CONFIG_HOME/sqlrs/config.yaml`, `dbms.image`)

If the resolved image id does not include a digest, the engine resolves it to a
canonical digest (for example `postgres:15@sha256:...`) before planning or
execution. If the digest cannot be resolved, `prepare` fails.

When a digest resolution is required, the job includes a `resolve_image` task
before any planning or execution.

Config key:

```yaml
dbms:
  image: postgres:17
```

When `-v/--verbose` is set, sqlrs prints the resolved image id and its source.

---

## Local Execution (MVP)

For local profiles, the engine performs real execution:

- Requires a local container runtime (`docker` or `podman`); the selected tool runs inside a container.
- Runtime mode is configured via engine config key `container.runtime` (`auto|docker|podman`).
- `SQLRS_CONTAINER_RUNTIME` is an operational override (for CI/debug scenarios).
- In `auto` mode, probe order is `docker`, then `podman`.
- On macOS with Podman, if `CONTAINER_HOST` is unset, the engine reads it from the default Podman connection.
- State data is stored under `<StateDir>/state-store` (outside containers).
- Each task snapshots the DB state; the engine prefers OverlayFS on Linux, can use
  btrfs subvolume snapshots when configured, and falls back to full copy.
- The prepare container stays running after the job and the instance is recorded
  as warm. Automatic connection-count cleanup is not implemented yet.

---

## State Identification

State identification depends on the **prepare kind** and is documented in each
variant guide.

---

## Error Conditions

`sqlrs prepare` may fail with the following errors:

- **Invalid inputs**
  - Missing files required by the selected variant.
  - Invalid arguments for the selected `prepare:<kind>`.

- **Executor failure**
  - Underlying tool exited with non-zero status.

- **Engine errors**
  - Storage backend unavailable.
  - Insufficient permissions.

All errors are reported before any mutable instance is exposed.

---

## Output

On success, `prepare` prints the DSN of the selected instance to stdout.
With `-v/--verbose`, extra details (including image source) are printed to stderr.

Example:

```text
DSN=postgres://...
```

This DSN uniquely identifies the instance and can be consumed by `sqlrs run`
or external applications.

When `--no-watch` is used without `--ref`, `prepare` prints job references
instead of a DSN:

```text
JOB_ID=<job-id>
STATUS_URL=/v1/prepare-jobs/<job-id>
EVENTS_URL=/v1/prepare-jobs/<job-id>/events
```

Use [`sqlrs-watch.md`](sqlrs-watch.md) to attach later.

---

## Job Monitoring (Events-First)

`sqlrs prepare` monitors engine progress through the job events stream rather
than periodic status polling.

### Event Stream Rules

- The CLI uses the `events_url` returned by `POST /v1/prepare-jobs`.
- The events stream is newline-delimited JSON (`application/x-ndjson`).
- The engine emits task/status events plus log events from tool execution.
- During long-running tasks, the engine repeats the last task event with a new
  timestamp when no new events appear for ~500ms, so the CLI can keep showing
  progress even if the underlying system is quiet.
- The CLI reads events until the stream is exhausted:
  - If the response status is 200 and `Content-Length` is present, the stream is
    considered complete after the declared byte length is fully read.
  - If the response is any 4xx status, the stream is considered complete and
    the command fails.
- If the stream ends without a definitive job outcome (`succeeded` or `failed`),
  the command fails with an error.

### Status Validation

When a status event is received (queued, running, succeeded, failed), the CLI
re-fetches the job status via `GET /v1/prepare-jobs/{jobId}` to confirm the
final result. The job is considered complete only when the status endpoint
returns `succeeded` or `failed`.
Cancellation is represented as `failed` with `error.code=cancelled`.

### Progress UX

Progress events are printed to stderr. By default, the CLI rewrites the same
line and shows a spinner when events repeat. It also includes the `message` from
the latest event if present.

With `--verbose`, each event is printed on a new line (no overwriting).

During interactive watch mode, `Ctrl+C` opens the control prompt:

```text
[s] stop  [d] detach  [Esc/Enter] continue
```

Rules:

- `s` asks for explicit confirmation (`Cancel job? [y/N]`) and then requests cancellation.
- `d` detaches from the job and leaves it running.
- `Esc` or `Enter` resumes watch.
- Repeated `Ctrl+C` while prompt is open is treated as `continue`.

For composite invocation `prepare ... run ...`, `detach` means:

- disconnect from `prepare`,
- skip the subsequent `run` phase in this CLI process, whether that phase is
  raw or alias-backed,
- exit successfully after printing the `job_id` and a message that `run` was skipped.

---

## Guarantees

- `prepare` is idempotent with respect to state identification.
- State objects are immutable once created.
