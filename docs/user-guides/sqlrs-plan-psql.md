# sqlrs plan (psql)

This document describes `sqlrs plan:psql`.

For common `plan` behavior and output formats, see [`sqlrs-plan.md`](sqlrs-plan.md).

---

## Command Syntax

```text
sqlrs plan:psql [--provenance-path <path>] [--ref <git-ref>] [--ref-mode worktree|blob] [--ref-keep-worktree] [--image <image-id>] [--] [psql-args...]
```

Where:

- `--image <image-id>` overrides the base container image.
- `--provenance-path <path>` writes a JSON provenance artifact.
- `--ref`, `--ref-mode`, and `--ref-keep-worktree` select a Git revision
  resolved by the CLI and its projection mode; see
  [`sqlrs-ref.md`](sqlrs-ref.md).
- `psql-args` are passed to `psql` and fully describe how the state is produced.

If `--` is omitted, all remaining arguments are treated as `psql-args`.
To pass `psql` flags that would clash with sqlrs flags (for example `-v`),
use `--` explicitly.

---

## `plan:psql` Concept

`plan:psql` defines:

- how the database state would be constructed,
- which **content inputs** participate in state identification,
- which execution steps would be reused from cache.

---

## psql Argument Handling

`plan:psql` aims to match `psql` semantics closely. All `psql-args` are passed
verbatim to `psql` with two enforced defaults for determinism:

- `-X` (ignore `~/.psqlrc`)
- `-v ON_ERROR_STOP=1`

If a user-provided argument conflicts with these enforced defaults, `plan`
fails with an error.

Connection arguments are rejected because sqlrs supplies the connection for
preparation:

- `-h`, `--host`
- `-p`, `--port`
- `-U`, `--username`
- `-d`, `--dbname`, `--database`

### SQL input sources

- `-f`, `--file <path>`: SQL script file. Relative paths are resolved from the CLI working
  directory and sent as absolute paths; files must live under the workspace root.
- `-c`, `--command <sql>`: inline SQL string.
- `-f -`: read SQL from stdin; sqlrs reads stdin and passes it to the engine.

All inputs above participate in plan identification via **normalized SQL
content** (expanded includes).

## Content locking (atomicity)

During planning, sqlrs acquires read-locks on all files discovered via include
expansion. If any file cannot be locked, `plan:psql` fails.

---

## Examples

### Plan a psql prepare

```bash
sqlrs plan:psql -- -f ./init.sql
```

---

### Override base image

```bash
sqlrs plan:psql --image postgres:17 -- -f ./init.sql
```

---

### Use stdin

```bash
cat ./init.sql | sqlrs plan:psql -- -f -
```
