# Git-aware semantics: passive features (CLI)

Status: **mixed**. Scenarios P1, P4, and P6 capture the delivered client-side ref,
provenance, and cache-explain surfaces. Standalone `run --ref` is also
implemented and documented in
[`../user-guides/sqlrs-run-ref.md`](../user-guides/sqlrs-run-ref.md), with the
implemented interaction flow in [`run-ref-flow.md`](run-ref-flow.md) and
internal structure in
[`run-ref-component-structure.md`](run-ref-component-structure.md); the rest of
this document remains future design. Today, the public MVP CLI still relies on
commands such as `sqlrs prepare:psql ... run:psql ...`.

Goal: add git-aware capabilities **without changing the user's work habits**. All functions in this document are activated **only by explicit user commands/flags** and do not require repository setup "for Taidon".

## Design principles

- **Do not guess intent from the repo.** The user defines context with explicit
  command shapes such as `sqlrs plan|prepare --ref ...`, `sqlrs run --ref ...`,
  and `sqlrs diff ...`.
- **Minimum side effects.** Prefer Git-object reads when they preserve command semantics, but keep compatibility first. Temporary `worktree` checkouts remain the default where full filesystem semantics matter and leave minimal traces under `.git/worktrees` that can be cleaned up.
- **Fast path first.** Try to find a ready state in the Taidon cache by hashes of the involved files. If not found, build it.
- **Prepare-oriented execution is reproducible.** Single-stage `plan` and
  `prepare` can save a provenance manifest; run-side provenance is not part of
  the current surface.
- **Remote mode keeps Git client-side.** The CLI resolves `--ref`, binds the
  selected input graph, and transfers required source blobs through
  [remote source sync](../user-guides/remote-source-input-sync.md). The backend
  does not need a server-side mirror or VCS credentials.

---

## Scenario P1. Repository-backed plan/prepare by git ref: `--ref`

Current public-slice note: this scenario describes the already-landed bounded
`plan` / `prepare --ref` baseline. Later follow-ups split out
provenance/cache explanation and now standalone `run --ref` as separate slices.

### Motivation

User wants to evaluate a prepare-oriented workflow **as in
commit/branch/tag** without touching the current working directory (dirty
state, open IDEs, parallel tasks).

### UX / CLI

The current public `plan` / `prepare` command shapes expose one explicit
stage-local flag family.

```bash
sqlrs plan --ref <git-ref> <prepare-alias>
sqlrs plan:psql --ref <git-ref> -- -f ./prepare.sql
sqlrs prepare --ref <git-ref> <prepare-alias>
sqlrs prepare:lb --ref <git-ref> -- update --changelog-file db/changelog.xml
```

Where `<git-ref>` can be: `HEAD`, `origin/main`, `abc1234`, `v1.2.3`, `refs/pull/123/head` (if available locally).

Important boundary for the current public surface: Git resolution and revision
projection happen in the CLI. Both local and remote profiles are supported;
remote profiles receive the selected input closure through source sync.

Additional guardrail for the first public slice: `prepare --ref` remains
watch-only, so `prepare --ref --no-watch` is rejected.

Behavior options:

- `--ref-mode worktree|blob` (default `worktree`)
  - `worktree`: create a temporary `git worktree` and remove it after the
    command
  - `blob`: read needed files directly from git objects (no full checkout)
- `--ref-keep-worktree` (debugging: do not remove the temporary worktree)

### Implementation sketch

1. Detect repo root (if missing, error `not a git repo`).
2. Resolve `<git-ref>` to `commit/tree`.
3. Resolve the caller's projected cwd inside that selected revision.
4. Bind alias-backed or raw `plan` / `prepare` inputs in that ref-backed
   context.
5. Collect file-bearing inputs through the shared kind-specific inputset layer.
6. Continue through the normal `plan` or `prepare` flow.
7. Remove the temporary worktree unless `--ref-keep-worktree` was requested.

Code-quality constraint for this slice: generic repo/ref/worktree handling
belongs to `internal/refctx`, alias-target resolution belongs to
`internal/alias`, and per-kind closure semantics belong to `internal/inputset`.
`internal/app` should only orchestrate these shared components.

---

## Scenario P2. Zero-copy cache hit (speed-up without file extraction)

### Motivation

In large repositories, checkout is expensive, while Taidon may already have the required state by migration hashes.

### UX / CLI

Enabled automatically in ref-backed prepare-oriented flows using
`--ref-mode blob` (or via an explicit flag):

```bash
sqlrs prepare --ref <ref> --ref-mode blob <prepare-alias>
```

Optional:

- `--zero-copy=auto|off` disable optimization or keep auto mode

### Implementation

1. In blob mode, get file list and blob hashes via `git ls-tree`.
2. Compute cache keys from hashes.
3. Check Taidon cache **before** extracting files.
4. If cache hit, do not extract files to disk, return the environment immediately.

Note: if required blob objects are missing locally (`partial clone`/LFS), a `git fetch` is needed or switch to `worktree` mode.

---

## Scenario P3. Taidon-aware diff by wrapping an existing sqlrs command: `diff`

### Motivation

`git diff` shows text, but the user wants "what changed **from the perspective of
an actual sqlrs command**" while staying syntactically close to the main CLI.

### UX / CLI

Instead of inventing a separate `--prepare <path>` mini-DSL, `diff` wraps one
existing content-aware sqlrs command and evaluates it in two contexts.

```bash
sqlrs diff --from-ref <refA> --to-ref <refB> plan:psql -- -f ./prepare.sql
sqlrs diff --from-ref <refA> --to-ref <refB> prepare:lb -- update --changelog-file db/changelog.xml
sqlrs diff --from-path <pathA> --to-path <pathB> prepare:psql -- -f ./prepare.sql
```

Rules:

- diff-specific options come before the wrapped command;
- the wrapped command keeps its current syntax unchanged;
- global `-v` stays the verbose control and global `--output` stays the text/json selector;
- wrapped-command file-bearing semantics come from the shared CLI-side
  `inputset` component for the selected kind;
- **Current CLI slice** (`frontend/cli-go`): exactly **one** wrapped token among
  `plan:psql`, `plan:lb`, `prepare:psql`, `prepare:lb`; compares **file-list
  closures** (hashes) only—no engine. **Ref mode** defaults to **`worktree`**
  for full filesystem semantics; explicit **`blob`** reads use `git show` /
  `git ls-tree`.
- **Design / later**: two-stage `prepare ... run` composites and alias-backed
  `prepare <ref>`; full derived representations (task plans, prepare payloads).
- Future standalone `run:*` support is possible only for file-backed inputs,
  because inline-only invocations may have no revision-dependent payload.

Examples of composite shapes (**design target**; not all parsed by the CLI yet):

```bash
sqlrs diff --from-ref <refA> --to-ref <refB> prepare chinook run:psql -- -f ./queries.sql
sqlrs diff --from-path <pathA> --to-path <pathB> prepare:psql -- -f ./prepare.sql run smoke
```

What `diff` compares:

- **Today:** same **resolved file graph + content** for `plan:*` and `prepare:*`
  of a given kind (psql vs lb).
- **Design target:** `plan:*` -> derived task plan; `prepare:*` -> bodies + graph;
  `run:*` -> file-backed inputs only.

### Implementation

1. Parse the diff scope (`from/to ref` or `from/to path`).
2. Parse the wrapped command using the same CLI grammar as the main invocation
   (today: single `plan:*` or `prepare:*` token only).
3. For each side independently, bind the wrapped command through the shared
   `inputset` component for that kind and collect the resulting file set using
   that side's own revision or path context.
4. **Today:** build file-list closures and compare Added / Modified / Removed.
   **Later:** richer representations and per-phase output for `prepare ... run`.
5. Render human or JSON.

---

## Scenario P4. Provenance side artifact for single-stage plan/prepare

### Motivation

In real commands you quickly need to answer "what exactly did you run?" A
portable artifact helps reproduce issues months later without changing the main
CLI result contract.

### UX / CLI

Enabled explicitly via an additive path flag:

```bash
sqlrs plan --provenance-path ./artifacts/chinook-plan.json chinook
sqlrs prepare --ref origin/main --provenance-path ./artifacts/chinook-prepare.json chinook
sqlrs prepare:psql --provenance-path ./artifacts/prepare.json -- -f ./prepare.sql
```

Boundaries of the first public slice:

- single-stage `plan` and `prepare` only;
- raw and alias-backed prepare flows;
- local and remote profiles, using live inputs or client-side `--ref`;
- no standalone `run`;
- no composite `prepare ... run ...`;
- no automatic emission without the explicit flag;
- no provenance printed as part of the main stdout payload.

Content (minimum):

- command family / kind / class (raw vs alias)
- invocation timestamp
- workspace root, caller cwd, and selected alias path when applicable
- selected ref metadata when `--ref` is used
- normalized prepare args
- collected input entries with deterministic hashes
- pre-execution cache decision snapshot:
  - signature
  - hit vs miss
  - matched state id when present
  - miss reason code when known
- terminal outcome summary (success/failure/canceled plus state/job identifiers
  when available)

### Implementation

1. Parse and validate the normal `plan` / `prepare` command first.
2. Bind the selected stage using the same raw/alias/ref semantics as normal
   execution.
3. Collect the deterministic local input graph through the shared
   `internal/inputset` layer.
4. When `--provenance-path` is present, ask the engine for a read-only
   pre-execution cache explanation for that bound stage.
5. Run the normal `plan` or `prepare` flow.
6. Serialize one JSON side artifact to the requested caller-cwd-relative path.

Important behavior note: the artifact records the cache decision observed
immediately before execution. If another process mutates cache state between
that explain step and the actual prepare execution, the recorded explanation is
still the correct pre-execution diagnostic snapshot, not a post-facto guarantee.

---

## Scenario P5. Compare: one query on two states

### Motivation

QA/dev needs to compare results/errors between two schema versions (e.g., base vs PR).

### UX / CLI

```bash
sqlrs compare \
  --from-ref <refA> --from-prepare <path> \
  --to-ref <refB> --to-prepare <path> \
  -- psql -c "select * from flights limit 10"
```

Output:

- exit codes
- stderr/stdout (with limits)
- (optional) diff of result sets in table format

Options:

- `--diff text|json|table`
- `--timeout-ms 5000`
- `--max-rows 1000`

### Implementation

1. Bring up environments for `from-ref` and `to-ref` (with cache).
2. Execute the same command in both.
3. Collect results and compare.

---

## Scenario P6. `cache explain` for one prepare-oriented decision

### Motivation

User wants to know whether a single-stage prepare-oriented workflow would reuse
an existing state now, and if not, why.

### UX / CLI

The current public command is read-only and prepare-oriented:

```bash
sqlrs cache explain prepare <prepare-alias>
sqlrs cache explain prepare --ref <ref> <prepare-alias>
sqlrs cache explain prepare:psql -- -f ./prepare.sql
sqlrs cache explain prepare:lb -- update --changelog-file db/changelog.xml
```

Output:

- hit vs miss for the final prepare state lookup
- engine-computed signature
- matched state id when present
- best-known reason code when the final state is absent

This first slice does **not** accept wrapped `plan`, wrapped `run`, or
composite `prepare ... run ...`.

### Implementation

1. Parse the wrapped single-stage prepare grammar (`prepare <alias>` or
   `prepare:<kind> ...`).
2. Bind the stage through the same raw/alias/ref path as real `prepare`.
3. Collect the deterministic input graph through the shared `inputset` layer.
4. Submit one read-only engine request that computes the same final prepare
   signature and cache lookup as real execution, without creating a job or
   mutating cache.
5. Render human or JSON output, merging the engine decision with the local
   input/ref metadata already known to the CLI.

Store-health diagnostics remain separate:

- `sqlrs status --cache`
- `sqlrs ls --states --cache-details`

---

## Minimal MVP for passive features

1. bounded client-side `plan` / `prepare --ref` with `worktree` default and explicit
   `blob`
2. `sqlrs diff --from-ref/--to-ref <wrapped-command...>` for one `plan:*` or
   `prepare:*` command
3. provenance side artifact for bounded single-stage `plan` / `prepare`
4. `cache explain prepare ...` (simple version)
5. standalone `run --ref` for raw and alias-backed single-stage run flows
