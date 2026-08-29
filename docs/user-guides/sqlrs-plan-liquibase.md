# sqlrs plan (liquibase)

This document describes `sqlrs plan:lb`.

For common `plan` behavior and output formats, see [`sqlrs-plan.md`](sqlrs-plan.md).

---

## Command Syntax

```text
sqlrs plan:lb [--provenance-path <path>] [--ref <git-ref>] [--ref-mode worktree|blob] [--ref-keep-worktree] [--image <db-image-id>] -- <liquibase-args...>
```

Where:

- `--image <db-image-id>` overrides the DB base image (same as `prepare:psql`).
- `--provenance-path <path>` writes a JSON provenance artifact.
- `--ref`, `--ref-mode`, and `--ref-keep-worktree` select a Git revision
  resolved by the CLI and its projection mode; see
  [`sqlrs-ref.md`](sqlrs-ref.md).
- `liquibase-args...` are passed to Liquibase CLI after `--`.

---

## Planning Model

`plan:lb` delegates changelog parsing and SQL generation to Liquibase:

1. The engine starts a fresh DB instance (same as `prepare:lb`).
2. The engine runs Liquibase **plan** via `updateSQL` (no DB changes).
3. The engine parses the resulting changeset list and SQL, and builds a plan.
4. The plan is returned without executing any changesets.

The resulting plan reflects the ordered changesets Liquibase would apply.

---

## Argument Filtering

sqlrs filters Liquibase arguments for determinism and safety:

- **Blocked commands**: anything outside the `update*` family.
- **Blocked connection args**: any CLI flags that set `url/username/password`.
- **Blocked runtime args**: flags that change classpath or external driver behavior.

All other arguments are passed through as-is.

---

## Path Handling

Liquibase runs as a host executable. Native mode uses paths for the engine's
operating system. When a WSL engine invokes a Windows `.bat` or `.cmd`
Liquibase launcher, sqlrs uses Windows-host mode:

- `--changelog-file`, `--defaults-file`, and `--searchPath` are translated to
  Windows paths before execution.
- If the file is under the workspace root, sqlrs prefers **relative paths** to
  allow Liquibase to resolve them against `--searchPath`.
- For alias-backed `plan:lb` invocations that declare a local `--searchPath`,
  sqlrs runs Liquibase from the first local search-path entry so the changelog
  path and include graph stay aligned.

Container-based Liquibase execution is not implemented.

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

During planning, sqlrs acquires read-locks on all Liquibase inputs involved in
`updateSQL` (changelog + referenced files). If any file cannot be locked,
`plan:lb` fails.

---

## Examples

```bash
sqlrs plan:lb -- update \
  --changelog-file examples/liquibase/jhipster-sample-app/master.xml
```
