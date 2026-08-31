# sqlrs CLI Contract (Draft)

This document defines a **preliminary user-facing CLI contract** for `sqlrs`.
It is intentionally incomplete and iterative, similar in spirit to early `git`
or `docker` CLI designs.

The goal is to:

- establish a stable _mental model_ for users,
- define command namespaces and responsibilities,
- guide internal API and UX decisions.

---

## 0. Design Principles

1. **Canonical CLI name**: `sqlrs`
2. **Subcommand-based interface** (git/docker style)
3. **Explicit state over implicit magic**
4. **Composable commands** (plan → prepare → run → inspect)
5. **Machine-friendly output by default** (JSON where applicable)
6. **Human-friendly summaries** when run interactively

---

## 1. High-Level Mental Model

From the user’s point of view, `sqlrs` manages:

- **states**: immutable database states produced by a deterministic preparation
  process
- **instances**: mutable copies of states; all database modifications happen here
- **plans**: ordered sets of changes to apply (e.g., Liquibase changesets)
- **runs**: executions of plans, scripts, or commands
- in remote/shared deployments, **users** and **organizations** that define the
  authenticated principal and tenant context for future shared workflows

```text
state  --(materialize)-->  instance  --(run)-->  new state
```

---

## Command Shape Convention

Across sqlrs, commands generally follow this shape:

```text
sqlrs <verb>[:<kind>] [subject] [options] [-- <command>...]
```

- `<verb>` is the main command (`prepare`, `run`, `ls`, ...).
- `:<kind>` is an optional executor/adaptor selector (e.g., `prepare:psql`, `run:pgbench`).
- `subject` is optional and verb-specific (e.g., an instance id, a name, etc.).
- `-- <command>...` appears only for verbs that execute an external command (primarily
  `run`) and is optional for `run` kinds with default commands.

`sqlrs ls` itself does not use `:<kind>` and does not accept `-- <command>...`.

Exception: `sqlrs diff` is a meta-command with a diff scope, then a wrapped
command. **Implemented today:** one wrapped token among `plan:psql`, `plan:lb`,
`prepare:psql`, `prepare:lb`. **Design target:** also a normal two-stage
`prepare ... run` composite:

```text
sqlrs diff <diff-scope> <wrapped-command>            # supported (single token)
sqlrs diff <diff-scope> <prepare-stage> <run-stage>  # not implemented yet
```

This keeps nested syntax aligned with the main CLI instead of a separate
`diff`-specific input DSL.

Internal architecture rule: kind-specific file-bearing semantics are owned by
shared CLI-side `internal/inputset` components and are reused by execution,
`sqlrs diff`, and `sqlrs alias check`.

## ID Prefix Rules

Where the CLI expects a **state or instance id**, users may supply a hex prefix
(minimum 8 chars). The CLI resolves the prefix case-insensitively and fails on
ambiguity.

Job ids are **opaque and must be provided in full**; prefix matching is not
supported for jobs.

## 2. Command Groups (Namespaces)

```text
sqlrs
  init
  auth
  user
  org
  status
  cache
  ls
  rm
  discover
  alias
  prepare
  watch
  plan
  run
  diff
```

Not all groups are required in MVP.

---

## 3. Core Commands (MVP)

### 3.1 `sqlrs init`

See the user guide for the authoritative, up-to-date command semantics:

- [`docs/user-guides/sqlrs-init.md`](../user-guides/sqlrs-init.md)

---

### 3.2 `sqlrs status`

See the user guide for the authoritative, up-to-date command semantics:

- [`docs/user-guides/sqlrs-status.md`](../user-guides/sqlrs-status.md)

Current design direction:

- `status` remains the engine health command and includes a compact cache
  summary by default;
- `status --cache` expands that summary into full bounded-cache diagnostics.

---

### 3.3 `sqlrs ls`

See the user guide for the authoritative, up-to-date command semantics:

- [`docs/user-guides/sqlrs-ls.md`](../user-guides/sqlrs-ls.md)

Current design direction:

- `ls --states` stays the primary state inventory command;
- human-readable `ls --states` output remains one row per state and does not
  hard-wrap table cells;
- in compact human output, `CREATED` uses a relative representation, while
  `--long` switches it to an absolute UTC timestamp with second precision;
- in human-readable state tables, the kind column uses the compact header `KIND`
  and budgets for the existing short sqlrs kind aliases;
- compact human-readable state tables use a one-character inter-column gap to
  reduce width pressure from deep state trees;
- compact human-readable `jobs` tables follow the same `KIND`, `IMAGE_ID`, and
  timestamp rules as `states`; when both requested and resolved image
  references are available, human-readable `jobs` prefer the resolved image id,
  while `PREPARE_ARGS` behaves as a width-budgeted wide column like task
  `ARGS`;
- diagnostic job `signature` is exposed in JSON as `signature` and is added to
  the human-readable jobs table only with an explicit `--signature` flag;
- compact human-readable `tasks` tables shorten `INPUT` kind prefixes, use the
  shorter header `OUTPUT_ID`, and expose an API-backed task summary column
  `ARGS`;
- for task `INPUT`, state ids use regular id shortening, while image inputs use
  the same digest-aware compact formatter as `IMAGE_ID` columns;
- on a TTY, `PREPARE_ARGS` is width-budgeted against the current terminal and
  truncated in the middle when needed;
- when stdout is not a TTY, wide human-readable columns use a stable fallback
  budget instead of guessing the eventual viewer width;
- `--wide` disables truncation of wide text columns in human output (currently
  `PREPARE_ARGS` and task `ARGS`), while `--long` expands ids and timestamps
  independently of `--wide`;
- `ls --states --cache-details` adds operator-facing cache metadata for state
  rows without introducing a new top-level cache command in the MVP surface.

---

### 3.4 `sqlrs rm`

See the user guide for the authoritative, up-to-date command semantics:

- [`docs/user-guides/sqlrs-rm.md`](../user-guides/sqlrs-rm.md)
ID prefix support (implemented):

- Full ids are hex strings (instances: 32 chars, states: 64 chars).
- Any state/instance id argument accepts 8+ hex characters as a case-insensitive
  prefix.
- Job ids must be specified in full (no prefix matching).
- If multiple matches are found within a kind, the command fails with an ambiguity
  error.
- If a job id matches and a state/instance prefix matches, the command fails with
  a cross-kind ambiguity error.
- Human output shortens ids to 12 characters by default; `--long` prints full ids.
- JSON output always uses full ids.

---

### 3.5 `sqlrs prepare`

See the user guide for the authoritative, up-to-date command semantics:

- [`docs/user-guides/sqlrs-prepare.md`](../user-guides/sqlrs-prepare.md)
- [`docs/user-guides/sqlrs-provenance.md`](../user-guides/sqlrs-provenance.md)
- [`docs/user-guides/sqlrs-watch.md`](../user-guides/sqlrs-watch.md)

Current behavior:

- `prepare <prepare-ref>` resolves a repo-tracked `*.prep.s9s.yaml` file from
  the current working directory.
- bounded client-side `prepare --ref <git-ref>` supports local and remote
  profiles for single-stage prepare only; it keeps cwd-relative alias and raw
  path semantics by projecting the caller cwd into the selected ref context.
- `prepare --ref-mode worktree|blob` and `--ref-keep-worktree` follow the same
  vocabulary and defaults already accepted for `sqlrs diff`:
  `worktree` by default, `blob` as an explicit opt-in, and
  `--ref-keep-worktree` only with `worktree`.
- `prepare` supports `--watch` (default) and `--no-watch`.
- `prepare --no-watch` returns `job_id` and stream/status references when the
  command does not carry `--ref`.
- the first bounded `prepare --ref` slice remains watch-only;
  `prepare --ref --no-watch` is rejected so asynchronous ref-backed prepare
  semantics stay out of scope.
- `prepare ... run ...` accepts the normal two-stage composite shape with raw
  or alias mode on each stage.
- the bounded `--ref` slice does **not** yet extend to `prepare ... run ...`;
  a prepare stage carrying `--ref` remains single-stage only for now.
- `--provenance-path <path>` is implemented for single-stage `prepare` with
  local and remote profiles without changing
  the command's primary stdout/stderr contract; the JSON artifact is written as
  a side file resolved from the caller's current working directory.
- file-bearing paths read from a prepare alias resolve relative to that alias
  file, while raw-stage file paths keep their normal current-working-directory
  base.
- In watch mode, `Ctrl+C` opens a control prompt:
  - `[s] stop` (with confirmation),
  - `[d] detach`,
  - `[Esc/Enter] continue`.
- For composite `prepare ... run ...`, `detach` detaches from `prepare` and
  skips the subsequent `run` phase in the current CLI process, regardless of
  whether that phase is raw or alias-backed.

- Add named instances and name binding flags (`--name`, `--reuse`, `--fresh`, `--rebind`).

### 3.6 `sqlrs plan`

See the user guide for the authoritative, up-to-date command semantics:

- [`docs/user-guides/sqlrs-plan.md`](../user-guides/sqlrs-plan.md)
- [`docs/user-guides/sqlrs-plan-psql.md`](../user-guides/sqlrs-plan-psql.md)
- [`docs/user-guides/sqlrs-plan-liquibase.md`](../user-guides/sqlrs-plan-liquibase.md)
- [`docs/user-guides/sqlrs-provenance.md`](../user-guides/sqlrs-provenance.md)

The CLI must expose `plan:<kind>` for every supported `prepare:<kind>`.

Current alias mode:

- `sqlrs plan <prepare-ref>` resolves a repo-tracked prepare alias file from
  the current working directory.
- bounded client-side `plan --ref <git-ref>` supports local and remote profiles
  for single-stage plan only; it reuses the same projected-cwd, `worktree`,
  and explicit `blob` rules as bounded `prepare --ref`.
- `--provenance-path <path>` is implemented for single-stage `plan` with local
  and remote profiles; it writes one JSON
  side artifact without changing the main human/JSON result payload.

---

### 3.7 `sqlrs run`

See the user guide for the authoritative, up-to-date command semantics:

- [`docs/user-guides/sqlrs-run.md`](../user-guides/sqlrs-run.md)
- [`docs/user-guides/sqlrs-run-ref.md`](../user-guides/sqlrs-run-ref.md)

Implemented ref-aware flow and component documents:

- [`run-ref-flow.md`](run-ref-flow.md)
- [`run-ref-component-structure.md`](run-ref-component-structure.md)

Current behavior:

- standalone `sqlrs run <run-ref> --instance <id|name>` resolves a repo-tracked
  `*.run.s9s.yaml` file from the current working directory while keeping runtime
  instance selection explicit;
- bounded client-side `run --ref <git-ref>` supports local and remote profiles
  for single-stage `run`; it applies to alias-backed `run <run-ref>` and raw
  `run:psql` / `run:pgbench`, projecting the caller cwd into the selected ref
  context;
- `run --ref-mode worktree|blob` and `--ref-keep-worktree` reuse the same
  vocabulary and defaults already accepted for `plan` / `prepare` / `diff`:
  `worktree` by default, `blob` as an explicit opt-in, and
  `--ref-keep-worktree` only with `worktree`;
- raw ref-backed file-bearing inputs keep the same shared `internal/inputset`
  semantics already used by live-filesystem `run` for `psql` and `pgbench`;
- in `prepare ... run <run-ref>`, the run alias consumes the instance produced
  by the preceding `prepare`;
- in that composite form, `--instance` is forbidden as an explicit ambiguity.
- the first bounded `run --ref` slice does **not** yet extend to
  `prepare ... run ...`; a run stage carrying `--ref` remains single-stage only
  for now;
- file-bearing paths read from a run alias resolve relative to that alias file.

---

### 3.8 `sqlrs watch`

Attach to an existing prepare job and stream progress/events.

```bash
sqlrs watch <job_id>
```

See:

- [`docs/user-guides/sqlrs-watch.md`](../user-guides/sqlrs-watch.md)

---

### 3.9 `sqlrs diff`

`sqlrs diff` is a **group of composite commands** (design): the user inserts the
diff scope between `sqlrs` and one content-aware command (`plan`, `prepare`, or
`run`), reusing the main CLI syntax. **First implementation slice** (in
`frontend/cli-go`): compares **file-list closures** (paths + content hashes) for
exactly one wrapped `plan:psql`, `plan:lb`, `prepare:psql`, or `prepare:lb`
invocation—**no engine calls**. The long-term source of truth for wrapped
command file semantics is the shared CLI-side `internal/inputset` layer; any
builders that still live in `internal/diff` are transitional.

Design targets for later slices:

- `sqlrs diff ... plan ...` — difference in **task plans** for instance preparation.
- `sqlrs diff ... prepare ...` — difference in **task bodies** for preparation.
- `sqlrs diff ... run ...` — difference in **task bodies** for query execution (file-backed inputs).

Diff-specific options define the comparison scope; the wrapped command keeps its
normal syntax.

**Implemented today**

- scope: `--from-path`/`--to-path` or `--from-ref`/`--to-ref`
- ref mode: **`worktree` by default** (`git worktree add --detach`); optional
  **`blob`** reads Git objects without checkout
- wrapped commands: `plan:psql`, `plan:lb`, `prepare:psql`, `prepare:lb` only
- global `-v` and `--output` as elsewhere

**Design scope (not all implemented)**

- wrapped alias-mode commands such as `prepare <prepare-ref>`
- wrapped two-stage `prepare ... run` composites
- per-side alias path bases as in the main CLI; longer command chains out of scope
- shared per-kind file semantics with execution and alias inspection via
  `internal/inputset`

See:

- [`docs/user-guides/sqlrs-diff.md`](../user-guides/sqlrs-diff.md)
- [`docs/architecture/git-aware-passive.md`](git-aware-passive.md) (scenario P3)

---

### 3.10 `sqlrs discover`

`discover` is an implemented **local advisory workspace-analysis verb**:

```text
sqlrs discover [--aliases] [--gitignore] [--vscode] [--prepare-shaping]
```

Current behavior:

- `discover` is read-only and does not expose an `--apply` flag;
- execution commands never depend on prior discovery output;
- analyzer flags are additive;
- if no analyzer flags are supplied, `discover` runs all stable analyzers in
  canonical order;
- the stable analyzer set is `--aliases`, `--gitignore`, `--vscode`, and
  `--prepare-shaping`;
- `--aliases` uses a cheap
  prefilter, deeper kind-specific validation, topology ranking, and existing
  alias-coverage suppression;
- `discover --aliases` suggests likely `*.prep.s9s.yaml` / `*.run.s9s.yaml`
  candidates for supported SQL and Liquibase workflows and prints a
  copy-paste `sqlrs alias create ...` command for each strong suggestion;
- `discover --gitignore` checks an existing workspace-root `.sqlrs/` directory
  and files named `coverage-current`, then reports missing exact ignore entries
  at the corresponding `.gitignore` path;
- `discover --vscode` ensures `.vscode/settings.json` maps the sqlrs workspace
  schema to `**/.sqlrs/config.yaml` and prints the merged payload plus a
  shell-native write command;
- `discover --prepare-shaping` groups supported SQL/XML/YAML/JSON files by
  directory and reports directories whose file names contain both stable
  (`schema`, `init`, `ddl`, `base`) and volatile (`seed`, `demo`, `sample`,
  `data`) tokens;
- human output starts with aggregate counters and renders numbered findings in
  analyzer groups with analyzer-specific fields;
- `discover` writes progress to `stderr`: a delayed spinner in normal mode and
  line-based milestones in verbose mode;
- verbose progress uses analyzer/stage/candidate granularity and does not trace
  every scanned file;
- when shell syntax matters, follow-up commands are rendered for the current
  shell family;
- JSON output contains selected analyzers, per-analyzer summary counts,
  findings, and any emitted follow-up command strings.

See:

- [`docs/user-guides/sqlrs-discover.md`](../user-guides/sqlrs-discover.md)
- [`docs/user-guides/sqlrs-aliases.md`](../user-guides/sqlrs-aliases.md)
- [`alias-create-flow.md`](alias-create-flow.md)
- [`alias-create-component-structure.md`](alias-create-component-structure.md)
- [`discover-flow.md`](discover-flow.md)
- [`discover-component-structure.md`](discover-component-structure.md)

---

### 3.11 `sqlrs alias`

Post-MVP local design also introduces explicit alias-management commands for
repo-tracked workflow recipes:

```text
sqlrs alias create <ref> <wrapped-command> [-- <command>...]
sqlrs alias ls [--prepare] [--run] [--from <workspace|cwd|path>] [--depth <self|children|recursive>]
sqlrs alias check [--prepare] [--run] [--from <workspace|cwd|path>] [--depth <self|children|recursive>] [<ref>]
```

These commands inspect, validate, or create alias files; they do not replace
runtime `names`.

Design notes:

- `create` materializes a repo-tracked alias file from a wrapped
  `prepare:<kind>` or `run:<kind>` command;
- `create` reuses the same wrapped-command parsing and file-bearing semantics
  as execution commands;
- `discover` prints the `alias create` command shape but never writes files;
- scan mode defaults to `--from cwd --depth recursive`
- `check <ref>` reuses the same alias-ref rules as execution commands
- kind-specific file-bearing validation and closure semantics are shared with
  execution and `diff` through the same `internal/inputset` components
- the current slice intentionally omits a semantic `show` command because the
  alias files themselves remain the primary human-readable source of truth

See:

- [`docs/user-guides/sqlrs-aliases.md`](../user-guides/sqlrs-aliases.md)
- [`alias-create-flow.md`](alias-create-flow.md)
- [`alias-create-component-structure.md`](alias-create-component-structure.md)
- [`alias-inspection-flow.md`](alias-inspection-flow.md)

---

### 3.12 `sqlrs cache`

See the user guide for the authoritative, up-to-date command semantics:

- [`docs/user-guides/sqlrs-cache-explain.md`](../user-guides/sqlrs-cache-explain.md)

Current behavior:

- `status --cache` remains the global operator-facing cache health command;
- `ls --states --cache-details` remains the per-state cache metadata surface;
- `cache explain prepare ...` is the implemented read-only cache-diagnostics
  command for one single-stage prepare-oriented decision;
- `cache explain` reuses the same raw, alias-backed, and client-side `--ref`
  binding semantics as single-stage `prepare`;
- the first slice does **not** yet support wrapped `plan`, wrapped `run`, or
  composite `prepare ... run ...`.

---

### 3.13 `sqlrs auth`

See the user guide for the authoritative command semantics:

- [`docs/user-guides/sqlrs-auth.md`](../user-guides/sqlrs-auth.md)

Current design direction:

- `auth` manages local CLI authentication sessions for remote/shared
  deployments.
- `auth login google` uses Google Authorization Code Flow with PKCE, a
  loopback redirect listener, and OS credential storage for the refresh token.
- `auth status` reports safe session metadata and never prints raw tokens.
- `auth logout` deletes local credentials and attempts refresh-token revocation.
- Protected remote API commands keep `SQLRS_TOKEN` as the highest-priority
  override, then use a stored OIDC session when the selected profile has
  `auth.mode: oidcSession`, then fall back to legacy static bearer profile
  behavior.
- The gateway still receives only a short-lived Google ID token as
  `Authorization: Bearer <id-token>`.

---

### 3.14 `sqlrs user`

See the user guide for the authoritative command semantics:

- [`docs/user-guides/sqlrs-users-orgs.md`](../user-guides/sqlrs-users-orgs.md)

Current design direction:

- `user` is a remote/shared deployment command group; local mode rejects it
  before local engine discovery or autostart.
- `user register` creates or returns the current authenticated user's profile by
  deriving the target external identity from validated OAuth/OIDC bearer-token
  claims.
- `user register` can be rejected when server-side self-registration is
  disabled.
- `user create` is a separate administrator command for provisioning a profile
  linked to an explicitly supplied external identity.
- External identity uniqueness is server-owned and enforced over
  `provider + issuer + subject`.

---

### 3.15 `sqlrs org`

See the user guide for the authoritative command semantics:

- [`docs/user-guides/sqlrs-users-orgs.md`](../user-guides/sqlrs-users-orgs.md)

Current design direction:

- `org` is a remote/shared deployment command group; local mode rejects it
  before local engine discovery or autostart.
- `org create <slug>` creates a new organization for a registered user who is
  not yet attached to an organization and makes that user the organization
  admin.
- `org ls` and `org get <org-ref>` are scoped to organizations visible to the
  current authenticated user.

---

## 4. Output and Scripting

- Default output: human-readable
- `--json`: machine-readable
- Stable schemas for JSON output

Designed for CI/CD usage.

---

## 5. Input Sources (Local Paths, URLs, Remote Uploads)

Wherever the CLI expects a file or directory, it accepts:

- local path (file or directory)
- public URL (HTTP/HTTPS)
- server-side `source_id` (previous upload)

Behavior depends on target:

- local engine + local path: pass path directly
- remote engine + public URL: pass URL directly
- remote engine + local path: upload to source storage (chunked) and pass `source_id`

This keeps `POST /runs` small and enables resumable uploads for large projects.

---

## 6. Compatibility and Extensibility

- Liquibase is treated as an external planner/executor
- CLI does not expose Liquibase internals directly
- Future backends (Flyway, raw SQL, custom planners) fit into the same contract

---

## 7. Non-Goals (for this CLI contract)

- Full parity with Liquibase CLI options
- Interactive TUI
- GUI bindings

---

## 8. Open Questions

- Should we allow multiple prepare/run steps per invocation? (see user guides)
- Should `plan` be implicit in `migrate` or always explicit?
- How much state history should be shown by default?
- Should destructive operations require confirmation flags?

---

## 9. Philosophy

`sqrls` (sic) is not a database.

It is a **state management and execution engine** for databases.

The CLI should make state transitions explicit, inspectable, and reproducible.
