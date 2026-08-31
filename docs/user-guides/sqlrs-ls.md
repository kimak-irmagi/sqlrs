# sqlrs ls

## Overview

`sqlrs ls` lists objects managed by sqlrs.

sqlrs manages five object types:

- **states** - immutable database states produced by `prepare:psql`
- **instances** - mutable database instances derived from states
- **names** - stable user-defined handles pointing to instances (and bound to a
  state fingerprint)
- **jobs** - prepare job executions and their lifecycle
- **tasks** - queued or running steps belonging to jobs

`ls` is a pure introspection command: it does not create, modify, or delete anything.

---

## Command Syntax

```text
sqlrs ls [OPTIONS]
```

### Object selectors (what to list)

```bash
--instances, -i   List instances
--states,    -s   List states
--names,     -n   List names
--jobs,     -j   List jobs
--tasks,    -t   List tasks
--all            Equivalent to -i -s -n --jobs --tasks
```

### Filtering and formatting

```text
--quiet          Suppress headers and explanatory text (table still printed)
--no-header      Do not print table header (human output)
--long           Show full ids in human output (default is 12 chars)
--wide           Disable truncation of wide text columns in human output
--cache-details  Show additional cache metadata for state rows
```

### Optional filters

```text
--name <name>            Filter by name (names/instances)
--instance <instance_id> Filter by instance id (full or hex prefix)
--state <state_id>       Filter by state id (full or hex prefix)
--job <job_id>           Filter by job id (jobs/tasks)
--kind <prepare_kind>    Filter by prepare kind (states)
--image <image id>       Filter by the base image
```

> Note: filters apply after object selection. If no selector flags are provided,
> defaults apply (see below).

### ID matching

`--instance` and `--state` accept full ids or hex prefixes (8+ characters).
Prefix matching is case-insensitive and resolves to a single id:

- If the prefix matches exactly one id, that id is used.
- If the prefix matches multiple ids, `ls` fails with an error.
- If the prefix matches nothing, the result is an empty list.

`--job` requires a full job id (no prefix matching).

---

## Default Behavior

If no object selector flags are given, `sqlrs ls` lists:

- `--names` and `--instances`

Rationale: these are the objects users interact with most often. `--states` can
be large (cache), and jobs/tasks are operational details, so they are shown explicitly.

---

## Output (Human-Readable)

By default, `sqlrs ls` prints a table per selected object type, in this order:

1. Names
2. Instances
3. States
4. Jobs
5. Tasks

Each section begins with a one-line title (suppressed by `--quiet`).
When `--quiet` is set and multiple sections are printed, sections are separated
by a blank line.

IDs are printed in lowercase. By default, human output shortens ids to 12
characters. Use `--long` to print full ids and absolute timestamps.

### Human table width and wrapping

Human-readable tables are rendered as **one row per object**. `sqlrs` emits
explicit newline characters between rows and does not insert hard line wraps
inside a table cell.

When stdout is attached to a TTY, `sqlrs` uses the current terminal width to
budget wide text columns. This sizing happens at render time only; if the user
resizes the terminal after the command exits, previously printed output is not
reflowed.

When stdout is not a TTY (for example, when redirecting to a file), `sqlrs` does
not try to guess the width of the eventual viewer or editor. In that case,
default human output keeps the compact column budget, while `--wide` prints the
full value for wide text columns so the result can be inspected with horizontal
scrolling.

### IMAGE_ID formatting (human output)

In human-readable output, `IMAGE_ID` can be displayed in a compact form to keep
tables readable:

- For digest references like `postgres@sha256:<64-hex>`, the short form is `postgres@<12-hex>`.
- For raw digests like `sha256:<64-hex>`, the short form is `<12-hex>`.
- For non-digest references (e.g. `postgres:16`), the value is printed as-is.

Use `--long` to print the full `IMAGE_ID` values as returned by the engine.

#### `IMAGE_ID` vs Docker identifiers

`sqlrs ls` prints `IMAGE_ID` as a digest-based image reference (the value used
by the engine for caching).
This is typically comparable to Docker `RepoDigests`, not to the Docker `IMAGE ID`
column.

Examples (show both kinds of ids):

```bash
# Repo digests (comparable to sqlrs IMAGE_ID)
docker image ls --digests postgres
docker image inspect postgres --format '{{json .RepoDigests}}'

# Local image id (config digest; often different)
docker image inspect postgres --format '{{.Id}}'
```

### Names table

Columns:

- `NAME`
- `INSTANCE_ID`
- `IMAGE_ID`
- `STATE_ID` (or fingerprint-derived id)
- `STATUS` (active / expired / missing)
- `LAST_USED` (optional; if tracked)

Meaning:

- `STATUS=missing` indicates the name exists but its instance was purged/removed.
  The binding (name → state fingerprint) still exists.

### Instances table

Columns:

- `INSTANCE_ID`
- `IMAGE_ID`
- `STATE_ID`
- `NAME` (empty if ephemeral)
- `CREATED`
- `EXPIRES`
- `STATUS` (active / expired / orphaned)

Meaning:

- `STATUS=orphaned` indicates an instance exists without an attached name.

### States table

Columns:

- `STATE_ID`
- `IMAGE_ID`
- `KIND` (human-readable header; values use short sqlrs kind aliases such as `psql` and `lb`)
- `PREPARE_ARGS` (normalized; compact by default in human output)
- `CREATED`
- `SIZE` (persisted snapshot size in bytes; may be empty for legacy states that were created before size tracking)
- `REFCOUNT` (number of instances referencing this state)

In human-readable output, `CREATED` uses two display modes:

- default compact mode uses a relative form such as `3d ago`, `5h ago`, or
  `12m ago`;
- `--long` switches `CREATED` to an absolute UTC timestamp with second
  precision (RFC3339 without fractional seconds).

In default human output, `PREPARE_ARGS` is rendered as a compact preview to
keep the table readable.

- The compact states table uses a **single-space** gap between columns instead
  of the usual two-space visual padding.
- On a TTY, `sqlrs` assigns `PREPARE_ARGS` the remaining table width after the
  fixed columns are rendered, with a minimum budget of 16 characters and a
  maximum budget of 48 characters.
- If the value does not fit, it is truncated in the middle using the form
  `prefix ... suffix`.
- If the terminal is still too narrow even with the minimum budget, `sqlrs`
  keeps the row single-line and leaves any extra clipping to the terminal.
- When stdout is not a TTY, the compact renderer uses a stable 96-character
  fallback budget without inspecting the eventual viewer width.

Use `--wide` to print the full `PREPARE_ARGS` values in human output. `--wide`
does not change id or time formatting; combine it with `--long` when full ids,
absolute timestamps, and full `PREPARE_ARGS` are needed. JSON output always
returns the full `prepare_args_normalized` value, the machine field `prepare_kind`, and the original absolute
`created_at` timestamp.

Use `--signature` to add a diagnostic `SIGNATURE` column to the human-readable
jobs table. This flag is intended for retention/drift troubleshooting and is not
enabled by `--long`. `--signature` is valid only together with `--jobs` (or
`--all`).

`SIZE` is reported from persisted state metadata, not from a live recursive disk walk performed by the CLI. This keeps `ls` fast and deterministic. When size metadata is still missing for an older state, the column is left empty.

Optional columns with `--cache-details`:

- `LAST_USED`
- `USE_COUNT`
- `MIN_RETENTION_UNTIL`

`--cache-details` is valid only together with `--states` (or `--all`).
It is intended for bounded-cache diagnostics rather than the default compact
listing. `--cache-details` can be combined with `--wide` when full `PREPARE_ARGS`
visibility is needed in a human-readable table.

#### States hierarchy (human output)

The `STATE_ID` column is rendered as a compact tree. The prefix uses
**one character per depth level**:

- `+` means “this node has following siblings” (not last)
- `` ` `` means “last sibling”
- `|` means “there are further siblings at this ancestor depth”
- space means “no further siblings at this ancestor depth”

When `ParentStateID` information is available, `sqlrs ls --states` renders
`STATE_ID` as a tree to make
parent/child relationships visible (similar to `sqlrs rm --recurse` output).
For example:

```text
STATE_ID         IMAGE_ID             KIND          PREPARE_ARGS  CREATED  SIZE  REFCOUNT
aaaaaaaaaaaa     postgres@7352e0c4d62b psql         ...          ...      ...   ...
+bbbbbbbbbbbb   postgres@7352e0c4d62b psql         ...          ...      ...   ...
`cccccccccccc   postgres@7352e0c4d62b psql         ...          ...      ...   ...
```

For machine-readable processing, use `--output json`.

### Jobs table

Columns:

- `JOB_ID`
- `STATUS` (queued / running / succeeded / failed)
- `KIND` (human-readable header; values use short sqlrs kind aliases such as `psql` and `lb`)
- `IMAGE_ID`
- `PREPARE_ARGS` (normalized; compact by default in human output)
- `PLAN_ONLY` (true / false)
- `CREATED`
- `STARTED`
- `FINISHED`

In human-readable output, `jobs` follow the same compact formatting rules as
`states` for the overlapping fields, with one exception for the wide text
column:

- `IMAGE_ID` prefers the resolved image digest when available and falls back to
  the original requested image reference when resolution metadata is missing;
- `PREPARE_ARGS` is rendered as a width-budgeted wide column in compact human
  output; on a TTY it uses the remaining table width after the fixed columns are
  rendered, and `--wide` disables truncation for it;
- `CREATED`, `STARTED`, and `FINISHED` use relative forms in compact output and
  switch to absolute UTC timestamps with second precision under `--long`;
- compact `jobs` tables also use a one-character inter-column gap.

In JSON output, `jobs` expose both:

- `image_id` as the original requested image reference;
- `resolved_image_id` as the resolved digest-based image reference when
  available.
- `signature` as the engine-computed retention/drift signature when available.

### Tasks table

Columns:

- `TASK_ID`
- `JOB_ID`
- `TYPE`
- `STATUS` (queued / running / succeeded / failed)
- `INPUT` (compact kind:id in human output)
- `ARGS` (task-specific summary; compact by default in human output)
- `OUTPUT_ID` (state_execute only)
- `CACHED` (state_execute only)

For human-readable task tables:

- `INPUT` uses kind-aware shortening rules:
  - state inputs use the same id-shortening rules as other state/id fields;
  - image inputs use the same digest-aware compact formatting as `IMAGE_ID`
    columns, so distinct resolved digests remain distinguishable in compact
    output;
- in compact output, the `INPUT` kind prefixes may be shortened to one-letter
  aliases such as `i:` for image and `s:` for state;
- `ARGS` is a task-specific summary field exposed by the engine:
  - for psql `state_execute` tasks it identifies the corresponding `-f` or `-c`
    step;
  - for Liquibase `state_execute` tasks it identifies the corresponding
    changeset;
  - for task kinds without a meaningful argument summary, the column is empty;
- `ARGS` is rendered as a width-budgeted wide column in compact human output;
  `--wide` disables truncation for it just as it does for `PREPARE_ARGS`;
- `OUTPUT_ID` is a human-readable header rename only; JSON remains
  `output_state_id`.

In JSON output, `tasks` additionally expose engine-level metadata used for
consistent rendering and tooling:

- `image_id` (requested image ref, when relevant to the task)
- `resolved_image_id` (resolved digest, when available)
- `args_summary` (task-specific one-line summary for human-oriented listings)
- Liquibase-specific `changeset_id`, `changeset_author`, `changeset_path` when
  applicable

---

## Output (JSON)

With the global `--output json` option, `sqlrs ls` prints a single JSON object:

```json
{
  "names": [ ... ],
  "instances": [ ... ],
  "states": [ ... ],
  "jobs": [ ... ],
  "tasks": [ ... ]
}
```

- Arrays are present only for the selected object types.
- Each element is a stable schema suitable for CI tooling.
- JSON output always uses full ids (no truncation).

Recommended fields:

### Name object

- `name`
- `instance_id` (nullable)
- `image_id`
- `state_id`
- `state_fingerprint`
- `status` (`active` | `missing` | `expired`)
- `last_used_at` (optional)

### Instance object

- `instance_id`
- `image_id`
- `state_id`
- `name` (nullable)
- `created_at`
- `expires_at` (nullable)
- `status` (`active` | `expired` | `orphaned`)

### State object

- `state_id`
- `parent_state_id` (nullable)
- `image_id`
- `prepare_kind`
- `prepare_args_normalized`
- `created_at`
- `size_bytes` (optional)
- `last_used_at` (nullable)
- `use_count` (nullable)
- `min_retention_until` (nullable)
- `refcount`

### Job object

- `job_id`
- `status` (`queued` | `running` | `succeeded` | `failed`)
- `prepare_kind`
- `image_id`
- `resolved_image_id` (optional)
- `prepare_args_normalized` (optional)
- `plan_only`
- `created_at`
- `started_at` (nullable)
- `finished_at` (nullable)

### Task object

- `task_id`
- `job_id`
- `type`
- `status` (`queued` | `running` | `succeeded` | `failed`)
- `input` (object with `kind` and `id`)
- `image_id` (optional)
- `resolved_image_id` (optional)
- `args_summary` (optional)
- `task_hash` (optional)
- `output_state_id` (optional)
- `cached` (optional)
- `instance_mode` (optional)
- `changeset_id` (optional)
- `changeset_author` (optional)
- `changeset_path` (optional)

---

## Exit Codes

- `0` — success (even if no objects match; empty lists are valid)
- `2` — invalid flags or invalid filter combinations
- `3` — internal error (storage backend unavailable, corrupted metadata, etc.)

---

## Examples

List active names and instances (default):

```bash
sqlrs ls
```

List everything:

```bash
sqlrs ls --all
```

List states only:

```bash
sqlrs ls -s
```

List states with cache metadata:

```bash
sqlrs ls --states --cache-details
```

List states with full prepare arguments in human output:

```bash
sqlrs ls --states --wide
```

List instances derived from a specific state:

```bash
sqlrs ls -i --state <state_id>
```

List instances by state id prefix:

```bash
sqlrs ls -i --state deadbeef
```

List a single name entry:

```bash
sqlrs ls -n --name devdb
```

List jobs:

```bash
sqlrs ls --jobs
```

List tasks for a job:

```bash
sqlrs ls --tasks --job <job_id>
```

Machine-readable output for CI:

```bash
sqlrs --output json ls --all
```

---

## Notes and Design Rationale

### Why separate `names` from `instances`?

- Names are the _stable handles_ users remember.
- Instances are the _actual mutable databases_ that may be ephemeral, expiring,
  or renamed.
- Keeping both visible makes lifecycle debugging straightforward.

### Why not list states by default?

States are a cache of build artifacts. In shared mode, the cache can be large and
not directly user-relevant. Showing it explicitly avoids surprising output volume.

### Missing instance behind a name

`ls` intentionally reports `STATUS=missing` for names whose instance was purged.
