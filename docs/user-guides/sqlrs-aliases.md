# sqlrs aliases

## Overview

**Status: implemented for the current local CLI, except for the name-binding
follow-ups called out below.**

Current local CLI support includes:

- `sqlrs plan <prepare-ref>`
- `sqlrs prepare <prepare-ref>`
- `sqlrs run <run-ref> [--instance <id|id-prefix|name>]`
- `*.prep.s9s.yaml` prepare alias files
- `*.run.s9s.yaml` run alias files
- exact-file escape via a trailing `.`
- cwd-relative alias-ref resolution
- alias-file-relative resolution for file-bearing paths
- mixed `prepare ... run ...` composite invocations across raw and alias modes
- explicit `sqlrs alias create`, `sqlrs alias ls`, and `sqlrs alias check`
- advisory `sqlrs discover` analysis for aliases, gitignore hygiene, VS Code
  guidance, and prepare shaping

Remaining follow-ups:

- alias-driven name binding flags such as `--name`

This document defines how sqlrs should use **repo-tracked alias files** for
common `plan`, `prepare`, and `run` workflows without mixing those definitions
into local-only workspace configuration.

The core principle is explicitness:

- alias files are versioned together with the scripts or changelogs they refer to;
- local workspace settings stay in `.sqlrs/config.yaml`;
- execution commands resolve only the alias ref the user asked for;
- discovery tooling may suggest improvements, but execution never depends on
  guesswork.

---

## Entities

sqlrs should keep these concepts separate:

- **prepare alias** - a repo-tracked recipe describing how to build a state
- **run alias** - a repo-tracked preset describing how to run a tool against an
  existing instance
- **name** - a runtime handle bound to a materialized instance

Aliases are part of the repository contract. Names are part of runtime state.

Aliases and names must not share resolution semantics or a common implicit
namespace.

---

## Alias Files

Two versioned alias-file classes are defined:

- `*.prep.s9s.yaml` - prepare/plan alias definition
- `*.run.s9s.yaml` - run alias definition

Examples:

```text
chinook.prep.s9s.yaml
app/main.prep.s9s.yaml
smoke.run.s9s.yaml
perf/pgbench.run.s9s.yaml
```

These files are expected to be committed to the repository. They are distinct
from `.sqlrs/config.yaml`, which remains local-only and developer-specific.

---

## Alias Reference Resolution

Alias refs are **current-working-directory-relative logical stems**.

Workspace discovery still matters, but only as a **boundary and config root**:

- alias refs are resolved from the caller's current working directory;
- the resolved alias file must still stay within the active workspace;
- execution does not silently rebase alias refs against the workspace root.

### Prepare / plan resolution

```text
sqlrs plan <ref>
sqlrs prepare <ref>
```

Resolution rule:

- `sqlrs prepare chinook` -> `<cwd>/chinook.prep.s9s.yaml`
- `sqlrs prepare path/chinook` -> `<cwd>/path/chinook.prep.s9s.yaml`
- if `pwd = <workspace>/examples/chinook`, then
  `sqlrs prepare ../chinook` -> `<workspace>/examples/chinook.prep.s9s.yaml`

### Run resolution

```text
sqlrs run <ref> [--instance <id|id-prefix|name>]
```

Resolution rule:

- `sqlrs run smoke` -> `<cwd>/smoke.run.s9s.yaml`
- `sqlrs run path/smoke` -> `<cwd>/path/smoke.run.s9s.yaml`

### Exact-file escape

If the ref ends with a trailing `.`, sqlrs disables suffix inference and uses
the path literally after stripping the final dot:

- `sqlrs prepare chinook.txt.` -> `<cwd>/chinook.txt`

This is an escape hatch for explicit files. It is not the primary happy path.

### Paths inside alias files

Alias-file payloads are resolved relative to the alias file itself.

Examples:

- if `examples/chinook.prep.s9s.yaml` contains `- -f` / `- chinook/prepare.sql`,
  then `chinook/prepare.sql` resolves from `examples/`, not from the caller's
  current working directory;
- if `db/app.prep.s9s.yaml` contains `- --changelog-file` / `- changelog.xml`,
  then `changelog.xml` resolves to `db/changelog.xml`.

This rule applies only to paths read from the alias file. Raw command stages
such as `prepare:psql -- -f queries.sql` or `run:psql -- -f queries.sql` keep
their normal current-working-directory-relative path semantics.

### Non-goals of alias resolution

Alias resolution must not:

- scan the workspace recursively looking for a matching basename;
- silently reinterpret the requested ref as workspace-root relative;
- fall back to heuristics when the requested ref is missing.

If the resolved file does not exist, the command fails explicitly.

---

## Execution Commands

Alias mode and raw mode are intentionally separate.

### Raw mode

Raw mode keeps the existing command families:

```text
sqlrs plan:psql ...
sqlrs prepare:lb ...
sqlrs run:pgbench ...
```

### Alias mode

Alias mode uses the bare verb plus an alias ref as the subject:

```text
sqlrs plan <prepare-ref>
sqlrs prepare <prepare-ref>
sqlrs run <run-ref> [--instance <id|id-prefix|name>]
```

Examples:

```bash
sqlrs plan chinook
sqlrs prepare chinook
sqlrs run smoke --instance dev
```

### Alias mode rules

- `plan <prepare-ref>` and `prepare <prepare-ref>` load a prepare alias file.
- `run <run-ref>` loads a run alias file.
- In alias mode, the tool kind and tool arguments come from the alias file.
- An alias stage does **not** accept inline tool arguments of its own.
- Only orchestration flags remain valid in alias mode:
  - for `prepare`: `--watch`, `--no-watch`
  - future follow-up for `prepare`: `--name`, later `--reuse`, `--fresh`,
    `--rebind`
  - for `run`: `--instance`

This keeps the alias file as the canonical recipe instead of turning it into a
partial preset layered with ad-hoc command-line overrides.

### Mixed composite `prepare ... run`

The normal two-stage `prepare ... run` invocation may mix raw and alias modes
freely:

```text
sqlrs prepare <prepare-ref> run <run-ref>
sqlrs prepare <prepare-ref> run:<kind> [--instance <id|id-prefix|name>] [-- <command>] [args...]
sqlrs prepare:<kind> ... run <run-ref>
sqlrs prepare:<kind> ... run:<kind> [--instance <id|id-prefix|name>] [-- <command>] [args...]
```

Examples:

```bash
sqlrs prepare chinook run:psql -- -f queries.sql
sqlrs prepare:psql -- -f prepare.sql run smoke
sqlrs prepare chinook run smoke
```

Composite rules:

- A composite invocation still contains exactly two stages: one `prepare`
  stage and one `run` stage.
- The `prepare` stage may use alias mode or raw mode.
- The `run` stage may use alias mode or raw mode.
- If a `run` stage follows `prepare` in the same invocation, the target
  instance is the instance produced by that `prepare`.
- In that composite case, `--instance` must not be used on the `run` stage;
  providing it is an explicit ambiguity error.
- `--no-watch` or interactive `detach` still skip the subsequent `run` stage,
  regardless of whether that stage is raw or alias-backed.

---

## Alias File Shape

The initial alias files should remain intentionally small and explicit.

### Prepare alias example

```yaml
kind: lb
image: postgres:17
args:
  - update
  - --changelog-file
  - config/liquibase/master.xml
```

### Run alias example

```yaml
kind: psql
args:
  - -f
  - queries/smoke.sql
```

Initial requirements:

- `kind` is required
- `args` is required and ordered
- `image` is allowed only for prepare aliases

Future metadata may be added, but the initial format should stay easy to read,
review, and generate.

---

## Alias Management Commands

sqlrs should expose explicit alias-management commands that operate on the
repo-tracked alias files:

```text
sqlrs alias create <ref> <wrapped-command> [-- <command>...]
sqlrs alias ls [--prepare] [--run] [--from <workspace|cwd|path>] [--depth <self|children|recursive>]
sqlrs alias check [--prepare] [--run] [--from <workspace|cwd|path>] [--depth <self|children|recursive>] [<ref>]
```

Purpose:

- `create` - materialize a repo-tracked alias file from a wrapped execution
  command
- `ls` - list discovered alias refs from the workspace
- `check` - validate schema and command-specific constraints

These commands inspect, validate, or create alias files. They do not create
runtime instances and they do not replace `names`.

### Common inspection rules

- `alias ls` and `alias check` without `<ref>` run in **scan mode**.
- Scan mode searches for files ending in
  `.prep.s9s.yaml` or `.run.s9s.yaml`.
- Discovery is exact-suffix-based only. These commands do not apply heuristics
  and do not depend on `discover`.
- The local-only `.sqlrs/` directory is out of scope for alias inspection.
- Scan results are sorted deterministically by workspace-relative file path.
- `--prepare` limits the selection to prepare aliases.
- `--run` limits the selection to run aliases.
- For `ls` and `check` without `<ref>`, omitting both selectors means
  "inspect both alias classes".
- `--from` selects the scan root:
  - `workspace` = active workspace root
  - `cwd` = caller's current working directory
  - any other value = explicit path, resolved from the caller's current working
    directory unless already absolute
- The resolved scan root must stay within the active workspace.
- `--depth` limits scan breadth from that root:
  - `self` = only the selected directory
  - `children` = the selected directory plus its immediate child directories
  - `recursive` = all descendants
- Scan mode defaults to `--from cwd --depth recursive`.
- `alias check <ref>` switches to **single-alias mode** and reuses the same ref
  rules as execution commands:
  current-working-directory-relative stems plus exact-file escape via a trailing
  `.`.
- In single-alias mode, `--from` and `--depth` are not allowed.
- For `check <ref>`, sqlrs must resolve exactly one alias. If both
  `<ref>.prep.s9s.yaml` and `<ref>.run.s9s.yaml` match, the command fails with
  an ambiguity error and asks the user to add `--prepare`, `--run`, or an
  exact-file escape.
- If an exact-file escape is used with a non-standard filename in single-alias
  mode, the caller must provide `--prepare` or `--run` so sqlrs knows which
  alias schema to apply.

### `sqlrs alias create`

`create` writes a repo-tracked alias file from a wrapped execution command.

Current slice shape:

```text
sqlrs alias create <ref> <wrapped-command> [-- <command>...]
```

Examples:

```bash
sqlrs alias create chinook prepare:psql -- -f chinook/prepare.sql
sqlrs alias create flights prepare:lb -- update --changelog-file db/changelog.xml
sqlrs alias create smoke run:pgbench -- -c 10 -T 30
```

Rules:

- `<ref>` is a cwd-relative logical stem; sqlrs writes `<ref>.prep.s9s.yaml`
  for `prepare:<kind>` and `<ref>.run.s9s.yaml` for `run:<kind>`.
- The wrapped command is validated with the same shared file-bearing semantics
  used by execution commands.
- The initial slice treats an existing target alias file as an error and does
  not overwrite it.
- The command does not depend on `discover`; discover only prints this command
  shape for strong candidates.

### `sqlrs alias ls`

`ls` is an inventory command. It does not validate aliases beyond the minimum
needed to recognize their class and, when possible, extract `kind`.

Human output should be copy-friendly and show at least:

- alias type (`prepare` or `run`)
- invocation ref relative to the caller's current working directory
- workspace-relative file path
- tool kind (`psql`, `lb`, `pgbench`, ...) when it can be read cheaply

Unreadable or malformed files still appear in `ls` when they match the alias
suffix, but `kind` may be empty or marked unknown.

`--output json` should return one entry per discovered alias file with stable
fields for:

- alias type
- invocation ref
- workspace-relative file path
- optional tool kind

### `sqlrs alias check`

`check` performs static checks only. It must not require engine availability,
network access, container startup, or Git-ref resolution.

Validation scope:

- YAML parses successfully
- the file matches the selected alias class
- required fields such as `kind` and `args` are present
- kind-specific constraints are satisfied
- file-bearing arguments resolve using the same alias-file-relative rules as
  execution commands
- each referenced local file required by the selected alias kind exists

With no `<ref>`, `check` validates all selected alias files in the scan scope.
With `<ref>`, it validates exactly one resolved alias.

Human output should report one result per alias plus a deterministic summary.

Suggested exit semantics:

- exit `0` - all selected aliases are valid
- exit `1` - validation completed, but at least one alias is invalid
- exit `2` - command usage or alias selection error (for example missing `<ref>`
  for `check`, ambiguous alias class, or missing exact-file selector)

`--output json` should expose:

- checked count
- valid count
- invalid count
- per-alias results with path, type, validity, and error message when present

### Examples

List all aliases in the current workspace:

```bash
sqlrs alias ls
```

List only run aliases under the current directory subtree:

```bash
sqlrs alias ls --run --from cwd
```

Validate every discovered alias file:

```bash
sqlrs alias check
```

Validate only the direct children of `examples/`:

```bash
sqlrs alias check --from examples --depth children
```

Validate one explicit file by exact path:

```bash
sqlrs alias check --run scripts/smoke.run.s9s.yaml.
```

---

## `sqlrs discover`

`discover` is a general advisory command family for workspace analysis.

It is broader than alias generation and should remain a **verb**, not a
subcommand under `alias`.

The complete analyzer CLI is documented in
[`sqlrs-discover.md`](sqlrs-discover.md). This section focuses on how discovery
interacts with aliases.

Current command shape:

```text
sqlrs discover [--aliases] [--gitignore] [--vscode] [--prepare-shaping]
```

`discover` is advisory and read-only. Bare `discover` runs all stable analyzers
in canonical order; passing analyzer flags selects exactly that subset.

### Current analyzer roles

- `--aliases`
  - detect candidate prepare/run workflows, rank likely start files, and
    emit copy-pasteable `sqlrs alias create ...` commands for repo-tracked
    alias files
- `--gitignore`
  - suggest repository hygiene changes such as ignoring `.sqlrs/`
- `--vscode`
  - check `.vscode/settings.json` and suggest the `yaml.schemas` mapping from
    the sqlrs workspace schema to `**/.sqlrs/config.yaml`
- `--prepare-shaping`
  - group supported files by directory and report a split opportunity when
    stable-name tokens (`schema`, `init`, `ddl`, `base`) coexist with
    volatile-name tokens (`seed`, `demo`, `sample`, `data`)

Broader analyzer behavior is tracked separately:

- [#90](https://github.com/kimak-irmagi/sqlrs/issues/90) covers optional VS Code
  extension recommendations and additional editor consistency checks;
- [#91](https://github.com/kimak-irmagi/sqlrs/issues/91) covers input-graph and
  alias-layout analysis beyond the current filename heuristic.

### Discover rules

- `discover` is advisory and read-only by default
- execution commands never depend on `discover`
- alias resolution never falls back to discovery results
- the aliases analyzer is a pipeline:
  - cheap path/content prefilter;
  - deeper validation and closure collection for supported kinds;
  - topology/root ranking for likely alias entrypoints;
  - suppression of suggestions already covered by existing repo-tracked aliases
- the analyzer currently focuses on supported SQL and Liquibase workflow roots
- analyzers may be added incrementally over time
- aliases findings are rendered as numbered multi-line blocks rather than a
  wide table; each alias block shows the ref, kind, source file, alias path,
  score, and copy-paste `sqlrs alias create ...` command;
- non-alias analyzers render their own finding details and suggested follow-up
  commands under the analyzer heading;
- `discover` writes progress to `stderr` so `stdout` can be redirected safely;
- in normal interactive mode, progress is shown with a delayed spinner;
- in verbose mode, progress is written as line-based stage/candidate milestones;
- progress does not trace every scanned file or folder;
- JSON output preserves analyzer-grouped findings and summary counts in a
  stable shape; alias findings include the suggested create command string

`discover --aliases` directly supports the alias-file workflow. It stays
read-only and never writes alias files; the user materializes a suggestion by
running the printed `alias create` command.

---

## Interaction with Names

Names remain runtime handles pointing to materialized instances.

Current implementation note:

- prepare alias mode does not yet expose `--name`
- alias/name interaction is therefore still limited to the documented future
  design

Examples:

```bash
sqlrs prepare chinook --name dev
sqlrs run smoke --instance dev
```

In this flow:

- `chinook` is a prepare alias ref
- `smoke` is a run alias ref
- `dev` is a runtime name

Rules:

- aliases do not automatically create or reuse names unless the command
  explicitly asks for it;
- names do not replace alias refs;
- aliases and names do not share implicit lookup;
- a name may later expose optional metadata such as `source_alias`, but only for
  introspection.

---

## Rationale Summary

This design optimizes for low surprise:

- repo-tracked workflows are explicit files;
- local-only settings stay local;
- alias refs are deterministic and cwd-relative;
- file-bearing paths inside alias files stay local to the alias file;
- `discover` helps authors improve a workspace by printing copy-paste
  `alias create` commands, but never becomes execution-time magic;
- runtime handles (`names`) stay distinct from repo-tracked recipes.

---

## Examples

Prepare from an alias file:

```bash
sqlrs prepare chinook
```

Run a preset against a named instance:

```bash
sqlrs run smoke --instance dev
```

Inspect workspace alias files:

```bash
sqlrs alias ls
sqlrs alias ls --from cwd
sqlrs alias check
```

Materialize a suggested alias:

```bash
sqlrs alias create chinook prepare:psql -- -f chinook/prepare.sql
```

Advisory discovery:

```bash
sqlrs discover
sqlrs discover --aliases
```

Example discover output for a strong candidate should be copy-pasteable:

```bash
sqlrs alias create chinook prepare:psql -- -f chinook/prepare.sql
```

Select repository-hygiene or workflow-shaping analyzers explicitly when a
focused report is preferable:

```bash
sqlrs discover --gitignore --vscode
sqlrs discover --prepare-shaping
```
