# sqlrs `run --ref`

## Overview

**Status: implemented in the current CLI.**

This document describes the bounded Git-aware client-side surface that followed
`plan` / `prepare --ref`, provenance, and `cache explain`: allow standalone
`run` commands to read repository-backed alias files and file-bearing run inputs
from a selected Git revision without changing the caller's working tree.

The current surface stays intentionally narrow:

- supported: standalone `run <run-ref> --instance ...`
- supported: standalone raw `run:psql` and `run:pgbench`
- supported: local and remote profiles with client-side `--ref` resolution
- not supported yet: `prepare ... run ...` when the run stage carries `--ref`
- not supported yet: `prepare --ref ... run ...`
- not supported yet: provenance for `run`
- not supported yet: `cache explain run ...`
- not supported yet: server-side Git fetching or hosted-repository access

The surface makes repository-tracked query and benchmark workflows runnable
from another revision while leaving composite multi-stage revision semantics
out of scope.

---

## Command Shape

Public syntax:

```text
sqlrs run [--ref <git-ref>] [--ref-mode worktree|blob] [--ref-keep-worktree] <run-ref> --instance <id|name>
sqlrs run:<kind> [--ref <git-ref>] [--ref-mode worktree|blob] [--ref-keep-worktree] [--instance <id|name>] [-- <command>] [args...]
```

Selection rules:

- omitting `--ref` keeps live-filesystem behavior unchanged;
- `--ref` is resolved by the CLI and requires a local Git repository context;
- `--ref-mode` and `--ref-keep-worktree` are valid only when `--ref` is set;
- `--ref-mode` defaults to `worktree`;
- `--ref-keep-worktree` is valid only with `--ref-mode worktree`;
- standalone alias mode still requires `--instance <id|name>`;
- `run --ref` is standalone only.

The flag belongs to the `run` stage itself, not to global CLI options.

---

## Scope

### Supported

- `sqlrs run --ref <ref> <run-alias> --instance <id|name>`
- `sqlrs run:psql --ref <ref> --instance <id|name> -- -f ...`
- `sqlrs run:psql --ref <ref> --instance <id|name> -- -c ...`
- `sqlrs run:pgbench --ref <ref> --instance <id|name> -- -f ...`
- `sqlrs run:pgbench --ref <ref> --instance <id|name> -- -c ...`

These shapes work with local and remote profiles. The CLI always resolves the
ref and materializes file-bearing run inputs; the remote backend does not need
repository access.

### Explicitly out of scope

- `sqlrs prepare ... run --ref ...`
- `sqlrs prepare --ref ... run ...`
- `sqlrs run --provenance-path ...`
- `sqlrs cache explain run ...`
- `sqlrs diff` syntax changes
- server-side Git fetching or hosted-repository access

Composite shapes remain deferred to avoid mixing one revision-sensitive `run`
stage with instance hand-off, detach/no-watch rules, and prepare-stage ref
propagation.

---

## Revision Context Rules

When `--ref` is present, sqlrs evaluates the `run` stage inside a projected
repository context:

1. Find the repository root from the caller's current working directory.
2. Resolve `--ref <git-ref>` locally.
3. Project the caller's current working directory into that revision.
4. Resolve the run alias file or raw file-bearing inputs inside that projected
   revision context.

This means the current working directory still matters.

Example:

- repo root: `/repo`
- caller cwd: `/repo/examples/chinook`
- command: `sqlrs run --ref origin/main smoke --instance dev`

Then run-alias resolution starts from the projected directory
`<origin/main>:/examples/chinook`, not from the repository root.

If that projected cwd does not exist at the selected ref, the command fails.

---

## Alias Mode Under `--ref`

Alias mode keeps the same logical rules as live-filesystem `run`; only the filesystem
backing changes from the live working tree to the selected revision.

For:

```text
sqlrs run --ref <git-ref> <run-ref> --instance <id|name>
```

rules are:

- `<run-ref>` remains a cwd-relative logical stem;
- exact-file escape via trailing `.` still works;
- the alias file is resolved inside the projected ref context;
- file-bearing paths read from that alias file remain relative to the alias file
  directory inside the same ref context;
- non-file-bearing run args keep their current behavior unchanged.

Example:

- caller cwd: `<repo>/examples`
- command: `sqlrs run --ref HEAD~1 smoke --instance dev`
- resolved alias file: `<HEAD~1>:/examples/smoke.run.s9s.yaml`

If the alias file exists in the current working tree but not at the selected
ref, the command fails explicitly.

---

## Raw Mode Under `--ref`

For raw `run:<kind>` invocations:

- file-bearing arguments keep their live-filesystem per-kind grammar;
- relative paths are resolved from the projected cwd at the selected ref;
- the shared `internal/inputset` semantics remain the source of truth for file
  discovery and runtime materialization.

Examples:

```bash
sqlrs run:psql --ref origin/main --instance dev -- -f ./queries.sql
sqlrs run:pgbench --ref HEAD~1 --instance perf -- -f ./bench.sql -T 30
```

Ref-backed execution reuses the normal run-kind materialization rules:

- `run:psql` still turns file-backed `-f` inputs into the same step/stdin
  projection it already uses for live-filesystem runs;
- `run:pgbench` still turns file-backed `-f` inputs into `/dev/stdin`
  materialization in the same way as live-filesystem execution.

The slice does not introduce a second run-only path parser or a new engine-side
Git input mode.

---

## Ref Modes

The mode flags match the existing `sqlrs diff`, `plan --ref`, and
`prepare --ref` vocabulary.

### `--ref-mode worktree` (default)

- materialize the selected revision as a detached temporary worktree;
- project the caller's cwd into that worktree;
- evaluate run-alias and raw file-bearing inputs against normal filesystem
  semantics;
- remove the temporary worktree after the command unless
  `--ref-keep-worktree` is set.

This remains the default because it most closely preserves
local filesystem execution.

### `--ref-mode blob`

- read files directly from Git objects without creating a detached worktree;
- preserve the same projected-cwd model logically;
- rely on the shared Git-backed filesystem layer for file reads and directory
  traversal.

This mode is lighter but remains opt-in because `worktree` is the safer default
when full filesystem behavior matters.

---

## Instance Resolution and Output

Ref-backed execution does not change `run` instance-selection rules:

- standalone `run --ref ...` still requires `--instance <id|name>`;
- conflicting instance selection remains an error;
- `run` keeps forwarding stdout, stderr, and exit code of the executed command;
- `run` itself still produces no additional success output.

`--ref` only changes how repository-backed inputs are read before the normal
run request is sent to the engine.

---

## Validation and Errors

New validation rules:

- `--ref` requires a Git repository context;
- `--ref-mode` without `--ref` is a usage error;
- `--ref-keep-worktree` without `--ref` is a usage error;
- `--ref-keep-worktree` with `--ref-mode blob` is a usage error;
- standalone alias mode under `--ref` still requires `--instance`;
- the selected ref must resolve locally;
- the caller's projected cwd must exist at that ref;
- the selected run alias file or raw file-bearing entrypoint must exist at that
  ref;
- `prepare ... run ...` with a run stage carrying `--ref` is rejected in this
  slice.

Examples of user-facing failures:

- not a Git repository
- unknown ref
- projected cwd missing at ref
- run alias file missing at ref
- `-f` target missing at ref
- `--ref-mode blob --ref-keep-worktree`
- `run alias requires --instance`
- `run --ref` used in a composite `prepare ... run ...` command

These are regular command errors, not discover findings.

---

## Examples

Run a repo-tracked alias from another revision:

```bash
sqlrs run --ref origin/main smoke --instance dev
```

Run raw SQL from another revision without touching the current working tree:

```bash
sqlrs run:psql --ref HEAD~1 --instance dev -- -f ./queries.sql
```

Run a pgbench script from another revision:

```bash
sqlrs run:pgbench --ref feature/new-bench --instance perf -- -f ./bench.sql -T 30
```

Keep the temporary worktree for debugging:

```bash
sqlrs run --ref origin/main --ref-keep-worktree smoke --instance dev
```

Use direct Git-object reads:

```bash
sqlrs run:psql --ref origin/main --ref-mode blob --instance dev -- -f ./queries.sql
```

Not supported:

```bash
# rejected
sqlrs prepare chinook run --ref origin/main smoke
```

```bash
# rejected
sqlrs prepare --ref origin/main chinook run smoke
```

---

## Rationale Summary

This CLI shape keeps `run`-aware Git execution bounded:

- one explicit `--ref` flag family reused by `run` as-is;
- the same `worktree` vs `blob` vocabulary already established by other
  repository-aware commands;
- no new engine API or hosted Git dependency;
- no mixed-stage revision semantics in the current surface;
- raw mode and alias mode keep the same path-base rules they already have
  in live-filesystem execution.

Implemented flow and component documents:

- [`../architecture/run-ref-flow.md`](../architecture/run-ref-flow.md)
- [`../architecture/run-ref-component-structure.md`](../architecture/run-ref-component-structure.md)
