# sqlrs discover

## Overview

**Status: implemented in the current local CLI.**

The stable analyzer set is:

- `--aliases`
- `--gitignore`
- `--vscode`
- `--prepare-shaping`

Bare `discover` runs all four in canonical order. The command is local-only,
advisory, and read-only.

---

## Command Shape

The public syntax is:

```text
sqlrs discover [--aliases] [--gitignore] [--vscode] [--prepare-shaping]
```

Selection rules:

- analyzer flags are additive;
- if no analyzer flags are provided, `discover` runs **all stable analyzers**;
- if one or more analyzer flags are provided, `discover` runs exactly that
  subset;
- duplicate analyzer flags are ignored;
- output order is canonical and does not depend on flag order.

Canonical analyzer order:

1. `--aliases`
2. `--gitignore`
3. `--vscode`
4. `--prepare-shaping`

Bare `discover` therefore means "run all stable analyzers".

---

## Shared Command Rules

- `discover` stays a top-level verb, not `alias discover`.
- The command accepts analyzer selectors only; it does not accept positional
  subjects.
- Global CLI options such as `--workspace`, `--output json`, `--verbose`, and
  `--help` continue to work through the existing root/app surface.
- The command remains incompatible with execution commands in the same
  invocation.
- This slice does **not** introduce `--apply`, `--write`, `--fix`, or
  `--update` behavior.
- Execution commands never depend on prior `discover` output.

The generic analyzer surface is intentionally advisory only. It improves
repository hygiene and workflow shaping without introducing mutation semantics
that would need a second review surface.

---

## Analyzer Contracts

### `--aliases`

This analyzer:

- scan the workspace for likely prepare/run workflow roots;
- suppress suggestions already covered by repo-tracked alias files;
- emit copy-pasteable `sqlrs alias create ...` commands.

The emitted command uses the detected shell family and materializes a
repo-tracked alias file. The analyzer does not run the command.

### `--gitignore`

This analyzer checks two kinds of existing local artifacts:

- a workspace-root `.sqlrs/` directory;
- files named `coverage-current` below the workspace, excluding `.git`,
  `.sqlrs`, `node_modules`, and `vendor` directory trees.

For `.sqlrs/`, the target is the workspace-root `.gitignore` and the suggested
entry is `.sqlrs/`. For `coverage-current`, the target is a `.gitignore` in the
artifact's own directory and the suggested entry is `coverage-current`.

The analyzer prints:

- the suggested ignore entries;
- the target `.gitignore` path;
- a copy-paste follow-up command that appends the missing entries.

The follow-up command is rendered for the current shell family when shell syntax
matters:

- PowerShell on Windows shells;
- POSIX shell otherwise.

The command checks for the exact entry before appending it. `discover` itself
does not edit `.gitignore` files.

### `--vscode`

This analyzer examines `.vscode/settings.json` and ensures that `yaml.schemas`
contains this mapping:

```json
{
  "yaml.schemas": {
    "./.vscode/sqlrs-workspace-config.schema.json": [
      "**/.sqlrs/config.yaml"
    ]
  }
}
```

The analyzer prints:

- the target `.vscode/settings.json` path;
- the complete merged JSON payload;
- a copy-paste follow-up command that writes that payload.

When shell syntax matters, the follow-up command is rendered for the current
shell family:

- PowerShell on Windows shells;
- POSIX shell otherwise.

When the file contains a JSON object, the analyzer preserves unrelated
top-level settings while adding the missing schema mapping. A missing or empty
file is treated as an empty object. Invalid JSON produces an advisory finding
that asks the user to inspect the file manually. `discover` itself does not
write `.vscode/settings.json`.

### `--prepare-shaping`

This analyzer walks `.sql`, `.xml`, `.yaml`, `.yml`, and `.json` files, excluding
`.git`, `.sqlrs`, `node_modules`, and `vendor` directory trees. It groups files
by directory and classifies their base names with two token sets:

- stable: `schema`, `init`, `ddl`, `base`;
- volatile: `seed`, `demo`, `sample`, `data`.

When one directory contains at least one file from each class, the analyzer
suggests splitting stable schema/bootstrap preparation from volatile
seed/demo inputs. It does not parse include or changelog graphs and does not
create or inspect alias layouts.

---

## Output Model

Human output is block-oriented rather than table-oriented. It starts with the
selected analyzers and aggregate counters. Findings are then grouped under
`[aliases]`, `[gitignore]`, `[vscode]`, and `[prepare-shaping]` headings in
canonical analyzer order.

Rendering rules:

- findings are grouped by analyzer in canonical analyzer order;
- alias findings include detected type, ref, kind, file, alias path, score,
  reason or error, and create command;
- generic findings include analyzer, target, action, and any available reason,
  entries, JSON payload, follow-up command, shell family, or error;
- when there are no findings, output ends with
  `no advisory discover findings`.

Examples of analyzer-specific actions:

- `--aliases`: a copy-paste `sqlrs alias create ...` command;
- `--gitignore`: one or more ignore lines plus a copy-paste shell command that
  appends them to a specific `.gitignore`;
- `--vscode`: the merged settings payload plus a copy-paste shell command that
  writes `.vscode/settings.json`;
- `--prepare-shaping`: a suggestion to separate stable and volatile files in
  the reported directory.

With `--output json`, sqlrs serializes the discover report with:

- selected analyzers;
- per-analyzer summary counts;
- a stable `analyzer` field on every finding;
- a stable follow-up command field when the analyzer emits a ready-to-copy
  command;
- the shell family for shell-native follow-up commands when relevant;
- analyzer-specific payload fields only where shared fields would lose meaning.

---

## Failure Handling

- invalid analyzer flags are usage errors;
- an analyzer-level error is converted into an invalid finding for that
  analyzer;
- unrelated selected analyzers continue to run;
- candidate-specific validation errors remain findings where the analyzer can
  identify the affected candidate.

This keeps `discover` useful as a broad advisory pass even when one repository
area is malformed.

---

## Examples

Run all stable analyzers:

```bash
sqlrs discover
```

Run only repository-hygiene checks:

```bash
sqlrs discover --gitignore --vscode
```

Run only workflow-oriented checks:

```bash
sqlrs discover --aliases --prepare-shaping
```

Render machine-readable output for one analyzer:

```bash
sqlrs --output json discover --gitignore
```

Example human follow-up shapes:

```text
[gitignore]
1. ADVISORY gitignore
   Target        : .gitignore
   Action        : add missing ignore entries
   Entries       : .sqlrs/
   Follow-up command: <shell-native append command>
```

```text
[vscode]
1. ADVISORY vscode
   Target        : .vscode/settings.json
   Action        : add missing VS Code yaml schema guidance
   Payload       : <merged settings JSON>
   Follow-up command: <shell-native write command>
```

---

## Rationale Summary

This CLI shape keeps `discover` simple:

- one verb;
- additive analyzer selectors;
- stable default behavior;
- no mutation modes;
- clear separation between discovery findings and execution semantics.

Bare `discover` runs every stable analyzer; explicit analyzer flags provide a
focused subset without changing output order.
