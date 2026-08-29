# sqlrs prepare (liquibase)

This document describes `sqlrs prepare:lb` and the Liquibase execution model.
The command works with local and remote profiles; local engine configuration is
shown explicitly below, while a remote backend owns its Liquibase executable
configuration and receives project files through
[`remote-source-input-sync.md`](remote-source-input-sync.md).

---

## Goals

- Delegate Liquibase changelog parsing/format handling to Liquibase itself
  (XML/YAML/JSON/formatted SQL).
- Keep prepare deterministic and cacheable.
- Avoid mounting the entire workspace when possible.
- Minimize new complexity in the CLI and engine.

## Non-goals

- Client-side control of the Liquibase executable used by a remote backend.
- Custom Java extensions or classpath injection beyond the configured
  Liquibase distribution.
- Container-based Liquibase execution (planned, not implemented yet).

---

## CLI Syntax

```text
sqlrs prepare:lb [--provenance-path <path>] [--ref <git-ref>] [--ref-mode worktree|blob] [--ref-keep-worktree] [--watch|--no-watch] [--image <db-image-id>] -- <liquibase-args...>
```

### Flags

- `--image <db-image-id>` (optional): overrides the DB base image (same as `prepare:psql`).
- `--provenance-path <path>` writes a JSON provenance artifact for a
  single-stage invocation.
- `--ref`, `--ref-mode`, and `--ref-keep-worktree` select a Git revision
  resolved by the CLI and its projection mode; see
  [`sqlrs-ref.md`](sqlrs-ref.md).
- `--watch` waits for terminal status (default); `--no-watch` submits the job
  and exits, except that ref-backed prepare remains watch-only.
- `liquibase-args...` (required): passed to Liquibase CLI after `--`.

### Config fallback

Liquibase executable path is resolved from config. Otherwise, `liquibase` is
resolved via PATH.

```yaml
liquibase:
  exec: C:\Program Files\Liquibase\liquibase.exe
  exec_mode: native
```

`liquibase.exec_mode` accepts `auto` (default), `native`, or `windows-bat`.
Use `windows-bat` when a WSL engine invokes a Windows `.bat`/`.cmd` launcher;
use `native` when Liquibase runs on the same operating system as the engine.

---

## Engine Execution Model

1. Engine creates (or reuses) a base state for the DB image (same as `prepare:psql`).
2. Engine launches **host Liquibase**. It normally uses native execution; a WSL
   engine may launch a Windows batch wrapper through interop.
3. User provides **Liquibase command line** after `--` (for example `update`).
4. Engine executes **plan** via `updateSQL`, inspects the resulting changesets and
   builds a fine-grained plan.
5. Engine executes the plan as a sequence of `update-count --count=1` steps,
   snapshotting after
   each changeset.
6. The prepared state is cached and a new instance is created.

### Planning vs execution tasks

- **Plan task**: run `liquibase updateSQL` and **parse its output** to compute the
  ordered list of changesets and the SQL for each changeset (no DB changes).
- **Prepare tasks**: execute changesets **one by one** with
  `liquibase update-count --count=1`,
  emitting events per task (stdout lines -> events, exit -> status event).

### Connection parameters

Connection arguments are **always** supplied by the engine and override defaults
file values.
User-provided `--url`, `--username`, `--password`, `--classpath`, etc. are rejected.

---

## Path mapping (host Liquibase)

In native mode, paths use the engine operating system's normal syntax. In
`windows-bat` mode, the engine translates relevant WSL paths to Windows paths:

- `--changelog-file`
- `--defaults-file`
- `--searchPath`

This mapping is applied before Liquibase is executed, so it can resolve files on
the host filesystem.

The path mapper is **abstracted** to support future container execution (WSL paths
will be mapped to container mount paths instead of Windows paths).

---

## File resolution behavior (Liquibase)

Liquibase CLI resolves files relative to its **current working directory**, plus
classpath paths and a configured search path. When `--search-path` is set, it
overrides the default search locations. By default, Liquibase looks for a
`liquibase.properties` file in the directory where it is run.

**Current behavior in sqlrs**:

- The Liquibase **working directory** is the CLI working directory, except for
  alias-backed `prepare:lb` / `plan:lb` invocations that set a local
  `--searchPath`; in that case sqlrs runs Liquibase from the first local
  search-path entry so the changelog path and include graph stay aligned.
- sqlrs does **not** attempt to resolve includes itself. Liquibase handles it.

This delegates include resolution to Liquibase.

---

## Deterministic fingerprint (state id)

State identification is based on the **ordered list of Liquibase changesets**
returned by planning, plus the **parent state id**. The parent id is the task
input: for the first step it is the base **image id**, and for subsequent steps
it is the previous **state id**. The fingerprint is **content-based** rather
than path-based:

- `prepare kind` = `lb`
- `prev_state_id` (from the task input)
- ordered list of changesets as reported by Liquibase:
  - `changeset_hash` (preferred: Liquibase checksum when available; fallback:
    hash of SQL emitted for that changeset by `updateSQL`)
  - `id/author/path` are recorded for diagnostics but **do not** affect the fingerprint

If two different argument sets produce the same ordered changesets (including
per-changeset content hashes), sqlrs reuses the cached state for that chain.

---

## Content locking (atomicity)

To avoid plan drift, sqlrs locks all Liquibase inputs during each task that
computes or applies content:

- planning (`updateSQL`): changelog + referenced files are opened with read-locks
  while the plan is computed.
- execution (`update-count --count=1`): the same set is re-locked while the step
  runs.

If any lock cannot be acquired (file is being modified), `prepare:lb` fails.

---

## Error conditions

- Missing changelog or defaults file
- Search-path directory missing
- Liquibase exits non-zero
- Liquibase tries to access files outside mounted paths
- Engine storage/snapshot errors

---

## Event/logging behavior

Engine does not have a "verbose" mode. It always emits events:

- each line from Liquibase stdout -> task event
- Liquibase exit -> task status event

CLI decides how much to display, same as `prepare:psql`.

---

## Planning database target

Liquibase `updateSQL` needs a live database connection. For Postgres, a freshly
initialized cluster includes a default `postgres` database alongside `template0`
and `template1`, so sqlrs can connect to `postgres` on a brand-new instance for
planning. If `POSTGRES_DB` (or `POSTGRES_USER`) is set, the default database name
may differ; sqlrs should use the effective default database for the container.

---

## Filtering Liquibase arguments

sqlrs **filters** Liquibase arguments for safety and determinism:

- **Blocked commands**: anything outside the `update*` family (no rollback).
- **Blocked connection args**: any CLI flags that set `url/username/password`.
- **Blocked runtime args**: flags that change classpath or external driver behavior.
- **Search path**:
  - If `--searchPath` is passed, sqlrs mounts each path and rewrites it to
    `/sqlrs/mnt/pathN`.
  - In the future, we must also handle `searchPath` coming from environment or
    properties files.

All other arguments are passed through as-is.

---

## Examples

```bash
sqlrs prepare:lb -- update \
  --changelog-file examples/liquibase/jhipster-sample-app/master.xml
```
