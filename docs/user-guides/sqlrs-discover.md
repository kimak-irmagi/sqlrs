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

This analyzer keeps the existing meaning:

- scan the workspace for likely prepare/run workflow roots;
- suppress suggestions already covered by repo-tracked alias files;
- emit copy-pasteable `sqlrs alias create ...` commands.

It remains the only analyzer that suggests an explicit follow-up command.

### `--gitignore`

This analyzer reports repository-hygiene suggestions related to local-only or
generated workspace artifacts.

Initial focus:

- missing ignore coverage for `.sqlrs/`;
- missing ignore coverage for other local-only sqlrs workspace artifacts that
  should not be committed;
- placement issues where a nested `.gitignore` would express the rule more
  narrowly than the workspace root.

The analyzer prints:

- the suggested ignore entries;
- the target `.gitignore` path;
- a copy-paste follow-up command that appends the missing entries.

The follow-up command is rendered for the current shell family when shell syntax
matters:

- PowerShell on Windows shells;
- POSIX shell otherwise.

The command should be idempotent where practical so that rerunning it does not
blindly duplicate ignore lines. `discover` itself still does not edit files.

### `--vscode`

This analyzer reports editor-integration suggestions for repositories that are
already using or likely to benefit from sqlrs-oriented workspace conventions.

Initial focus:

- presence and shape of `.vscode/settings.json` entries relevant to
  `.sqlrs/config.yaml`;
- optional `.vscode/extensions.json` recommendations when the repository lacks
  obvious editor guidance for SQL/YAML-focused workflows;
- consistency between existing VS Code workspace settings and documented sqlrs
  workspace conventions.

The analyzer prints:

- the target `.vscode/*.json` path;
- the suggested settings or extensions payload;
- a copy-paste follow-up command for creating or updating that file.

When shell syntax matters, the follow-up command is rendered for the current
shell family:

- PowerShell on Windows shells;
- POSIX shell otherwise.

When the target JSON file already exists, the suggested command should merge the
missing sqlrs-related entries without overwriting unrelated user settings. When
the file does not exist, the command may create it from scratch. `discover`
itself still does not write `.vscode/*` files.

### `--prepare-shaping`

This analyzer reports workflow-shaping suggestions intended to improve local
prepare reuse and cache friendliness.

Initial focus:

- likely prepare roots that combine stable and volatile inputs in one large
  workflow;
- repeated include/changelog fan-in patterns that suggest a reusable shared
  base;
- alias-layout opportunities where a repository would benefit from splitting a
  single implicit prepare flow into multiple explicit repo-tracked aliases.

The analyzer is advisory only. It may point to candidate split points or alias
layouts, but it does not rewrite workflows or create aliases automatically.

---

## Output Model

Human output should stay block-oriented rather than table-oriented.

Rendering rules:

- findings are grouped by analyzer in canonical analyzer order;
- each finding block starts with the analyzer name;
- each finding states the target path or workflow root under discussion;
- each finding includes one actionable suggestion;
- analyzers may add analyzer-specific detail fields when needed, but the common
  "what was found" and "what to do next" shape stays consistent.

Examples of analyzer-specific actions:

- `--aliases`: a copy-paste `sqlrs alias create ...` command;
- `--gitignore`: one or more ignore lines plus a copy-paste shell command that
  appends them to a specific `.gitignore`;
- `--vscode`: a suggested settings or extensions payload plus a copy-paste
  shell command that creates or merges the target `.vscode/*.json` file;
- `--prepare-shaping`: a suggested alias split or root-selection change.

JSON output should preserve:

- selected analyzers;
- per-analyzer summary counts;
- a stable `analyzer` field on every finding;
- a stable follow-up command field when the analyzer emits a ready-to-copy
  command;
- the shell family for shell-native follow-up commands when relevant;
- analyzer-specific payload fields only where shared fields would lose meaning.

---

## Failure Handling

- workspace resolution errors remain hard failures;
- invalid analyzer flags remain usage errors;
- analyzer-internal validation problems should become findings where practical
  instead of terminating the whole command;
- one analyzer failing to validate a candidate should not discard unrelated
  findings from other selected analyzers.

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
Analyzer: gitignore
Target: .gitignore
Suggested entries:
  .sqlrs/
Suggested command:
  Add-Content ...               # PowerShell example on Windows
```

```text
Analyzer: vscode
Target: .vscode/settings.json
Suggested command:
  <shell-native merge/create command for the current platform>
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
