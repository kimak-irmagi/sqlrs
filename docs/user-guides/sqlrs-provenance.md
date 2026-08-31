# sqlrs provenance for prepare flows

## Overview

**Status: implemented in the current CLI.**

This document describes the provenance baseline for repository-aware
`sqlrs` workflows after the bounded `plan` / `prepare --ref` slice.

The goal is to let a user persist a reproducible execution manifest for the
prepare-oriented command they actually ran, without changing the command's main
stdout/stderr contract.

The current surface stays intentionally narrow:

- supported: single-stage `plan` and `prepare`
- supported: raw and alias-backed prepare flows
- supported: local and remote profiles, with optional client-side `--ref`
- not supported yet: standalone `run`
- not supported yet: composite `prepare ... run ...`
- not supported yet: provenance print-to-stdout modes

---

## Command Shape

Public syntax:

```text
sqlrs plan [--ref <git-ref>] [--ref-mode worktree|blob] [--ref-keep-worktree] [--provenance-path <path>] <prepare-ref>
sqlrs plan:<kind> [--ref <git-ref>] [--ref-mode worktree|blob] [--ref-keep-worktree] [--provenance-path <path>] [--image <image-id>] [--] [tool-args...]

sqlrs prepare [--ref <git-ref>] [--ref-mode worktree|blob] [--ref-keep-worktree] [--watch|--no-watch] [--provenance-path <path>] <prepare-ref>
sqlrs prepare:<kind> [--ref <git-ref>] [--ref-mode worktree|blob] [--ref-keep-worktree] [--watch|--no-watch] [--provenance-path <path>] [--image <image-id>] [--] [tool-args...]
```

Where:

- omitting `--provenance-path` keeps normal command behavior unchanged;
- `--provenance-path <path>` requests that sqlrs write one JSON provenance
  artifact for this invocation;
- the path is resolved from the caller's current working directory, not from
  the alias file directory;
- provenance inherits the same client-side `--ref` rules already accepted for
  `plan` / `prepare`;
- `prepare --ref --no-watch` stays invalid because that guardrail belongs to the
  bounded client-side `--ref` slice itself.

---

## Scope

### Supported

- `sqlrs plan --provenance-path ./artifacts/chinook-plan.json chinook`
- `sqlrs plan --ref origin/main --provenance-path ./artifacts/chinook-plan.json chinook`
- `sqlrs prepare:psql --provenance-path ./artifacts/prepare.json -- -f ./prepare.sql`
- `sqlrs prepare --ref HEAD~1 --provenance-path ./artifacts/chinook-prepare.json chinook`

The same forms work with remote profiles. The CLI writes the artifact locally
after collecting the engine's cache explanation and command outcome.

### Explicitly out of scope

- `sqlrs run --provenance-path ...`
- `sqlrs prepare ... run ... --provenance-path ...`
- automatic provenance emission without an explicit flag
- human/JSON provenance printing as part of the main command result
- server-side artifact persistence or automatic upload

---

## Output Contract

Provenance does not change the primary command result shape.

That means:

- `plan` keeps its current human and JSON output;
- `prepare` keeps its current DSN output in watch mode and job-reference output
  in non-ref `--no-watch` mode;
- provenance is written to the requested file as a side artifact, not appended
  to the main stdout payload.

If provenance writing fails after the main command succeeded, the command should
fail and report that write error explicitly instead of silently dropping the
artifact.

---

## Provenance Artifact

sqlrs writes one JSON document with enough data to answer:

1. What command shape was executed?
2. Which local input graph and hashes were used?
3. Was a Git ref involved, and if so which resolved revision?
4. Why did sqlrs reuse cache or build new state?
5. What outcome did the CLI observe before it returned?

Artifact fields:

- command family and kind (`plan`, `prepare`, `psql`, `lb`, alias vs raw)
- invocation timestamp
- workspace root, caller cwd, and selected alias path when applicable
- selected Git ref metadata when `--ref` is used:
  - requested ref
  - resolved commit
  - ref mode (`worktree` or `blob`)
- normalized prepare input args
- collected input entries with stable content hashes
- cache decision summary:
  - cache key / signature
  - hit vs miss
  - matched state id when present
  - miss reason code when known
- observed command outcome summary:
  - `succeeded` or `failed`
  - plan-only vs prepare execution
  - resulting state id / job id when available

For watched `plan` and `prepare` commands, `succeeded` means the command reached
its successful result. For `prepare --no-watch`, the current artifact is written
after job acceptance and also uses `succeeded`; the presence of `jobId` identifies
that submission-only case. It does not mean that the asynchronous prepare job
has reached a terminal state.

Distinct accepted and canceled outcome semantics are not implemented yet and
are tracked in [#93](https://github.com/kimak-irmagi/sqlrs/issues/93).

The artifact avoids ephemeral runtime credentials such as DSNs or auth
tokens. The point is reproducibility and explanation, not secret capture.

---

## Failure Semantics

sqlrs writes provenance only after it has enough bound command
context to describe the intended prepare flow.

That means:

- early usage errors do not emit provenance;
- missing repo / bad ref / missing file errors may emit provenance only if the
  command has already resolved enough context to identify the attempted flow and
  input set;
- execution-time failure after binding writes provenance with
  `outcome.status = failed`.

This keeps the feature useful for debugging real workflow failures without
forcing a provenance file for trivial parse errors.

---

## Examples

Write provenance for an alias-backed local plan:

```bash
sqlrs plan --provenance-path ./artifacts/chinook-plan.json chinook
```

Write provenance for a ref-backed prepare:

```bash
sqlrs prepare --ref origin/main --provenance-path ./artifacts/chinook-prepare.json chinook
```

Write provenance for a raw SQL prepare:

```bash
sqlrs prepare:psql --provenance-path ./artifacts/prepare.json -- -f ./prepare.sql
```

Not supported:

```bash
# not supported
sqlrs run --provenance-path ./artifacts/run.json smoke --instance dev
```

```bash
# not supported
sqlrs prepare --provenance-path ./artifacts/composite.json chinook run:psql -- -f ./queries.sql
```

---

## Rationale Summary

This shape keeps the first provenance slice narrow and low-risk:

- one additive file-output flag instead of a new result envelope;
- no change to current stdout JSON/human contracts;
- the same prepare-oriented scope already accepted for client-side `--ref`;
- an artifact shape aligned with `sqlrs cache explain`.
