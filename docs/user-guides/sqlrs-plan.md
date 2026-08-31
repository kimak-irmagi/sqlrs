# sqlrs plan

## Overview

`sqlrs plan` computes the execution plan for a deterministic preparation without
creating an instance or queueing a job. It reports which steps are already
cached and which steps would run during `prepare`.

`plan:<kind>` is always paired with `prepare:<kind>` and accepts the same
arguments; the only difference is that plan never executes the tasks it
produces.

---

## Command Syntax

```text
sqlrs plan [--provenance-path <path>] [--ref <git-ref>] [--ref-mode worktree|blob] [--ref-keep-worktree] <ref>
sqlrs plan:<kind> [--provenance-path <path>] [--ref <git-ref>] [--ref-mode worktree|blob] [--ref-keep-worktree] [--image <image-id>] [--] [tool-args...]
```

Where:

- bare `plan <ref>` resolves a repo-tracked prepare alias file
  from the current working directory (`<cwd>/<ref>.prep.s9s.yaml`);
- `:<kind>` selects the preparation variant (for example, `psql`, `lb`).
- `--provenance-path <path>` writes a JSON provenance artifact for this plan.
- `--ref <git-ref>` reads plan inputs from a Git revision resolved by the CLI.
- `--ref-mode` chooses `worktree` (default) or `blob`.
- `--ref-keep-worktree` keeps the detached worktree after exit in
  `worktree` mode.
- `--image <image-id>` overrides the base container image.
- `tool-args` are forwarded to the underlying tool for the selected kind.

For alias mode, paths read from the alias file itself are resolved relative to
the alias file directory.

If `--` is omitted, all remaining arguments are treated as `tool-args`.
To pass tool flags that would clash with sqlrs flags (for example `-v`),
use `--` explicitly.

Alias-mode details are described in [`sqlrs-aliases.md`](sqlrs-aliases.md).
Ref-backed details are described in [`sqlrs-ref.md`](sqlrs-ref.md).

Client-side `plan --ref` notes:

- ref-backed plan is single-stage only and works with local and remote
  profiles;
- the caller's current working directory is projected into the selected
  revision, matching `sqlrs diff` ref-mode behavior;
- remote profiles receive the selected files through source sync; the backend
  does not fetch the Git repository;
- `worktree` remains the default ref mode, with `blob` as an explicit opt-in;
- successful plan output stays unchanged.

---

## Variant docs

- `psql`: [`sqlrs-plan-psql.md`](sqlrs-plan-psql.md)
- `lb`: [`sqlrs-plan-liquibase.md`](sqlrs-plan-liquibase.md)

---

## Base Image Selection (Common)

The base container image id is resolved in this order:

1. `--image <image-id>` command-line flag
2. Workspace config (`.sqlrs/config.yaml`, `dbms.image`)
3. Global config (`$XDG_CONFIG_HOME/sqlrs/config.yaml`, `dbms.image`)

If the resolved image id does not include a digest, the engine resolves it to a
canonical digest (for example `postgres:15@sha256:...`) before planning. If the
digest cannot be resolved, `plan` fails.

When a digest resolution is required, the plan includes a `resolve_image` task
before any planning or execution.

Config key:

```yaml
dbms:
  image: postgres:17
```

When `-v/--verbose` is set, sqlrs prints the resolved image id and its source.

---

## Plan Tasks (Common)

`plan` returns an ordered list of tasks. The current task types are:

- `plan`: build the plan for the requested kind.
- `resolve_image`: resolve a non-digest image reference to a canonical digest.
- `state_execute`: apply a change block on a base image or state.
- `prepare_instance`: create an ephemeral instance from the final state.

The `resolve_image` task is only present when the requested image id does not
already include a digest.

Each `state_execute` task includes:

- `task_hash`: hash of the change block.
- `output_state_id`: deterministic id derived from input id and `task_hash`.
- `cached`: whether the output state already exists.

If `cached` is true, the `prepare` executor can skip the execution step and
reuse the existing state.

Snapshotting is part of `state_execute`.

---

## Output

Use the global `--output` flag to select the format.

### JSON output

```json
{
  "prepare_kind": "psql",
  "image_id": "postgres:17",
  "prepare_args_normalized": "-X -v ON_ERROR_STOP=1 -f /abs/init.sql",
  "tasks": [
    {
      "task_id": "plan",
      "type": "plan",
      "planner_kind": "psql"
    },
    {
      "task_id": "resolve-image",
      "type": "resolve_image",
      "image_id": "postgres:17",
      "resolved_image_id": "postgres:17@sha256:deadbeef"
    },
    {
      "task_id": "execute-0",
      "type": "state_execute",
      "input": { "kind": "image", "id": "postgres:17@sha256:deadbeef" },
      "task_hash": "2f9c...e18",
      "output_state_id": "8a1c...b42",
      "cached": false
    },
    {
      "task_id": "prepare-instance",
      "type": "prepare_instance",
      "input": { "kind": "state", "id": "8a1c...b42" },
      "instance_mode": "ephemeral"
    }
  ]
}
```

### Human output

Human output prints a summary with the final state id and a task list.

---

## Guarantees

- `plan` never creates instances or states.
- `plan` is deterministic with respect to its inputs.
