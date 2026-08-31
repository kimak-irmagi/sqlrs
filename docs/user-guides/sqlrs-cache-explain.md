# sqlrs cache explain

## Overview

**Status: implemented in the current CLI.**

This document describes the user-facing `cache explain` command for
repository-aware prepare flows.

The goal is to answer a narrow operator/developer question before execution:

> If I run this prepare-oriented workflow now, will sqlrs reuse cached state or
> rebuild it, and why?

The command is intentionally read-only:

- it does not execute the prepare flow;
- it does not create an instance;
- it does not mutate cache;
- it only explains the cache decision sqlrs would make for one bounded
  prepare-oriented invocation.

---

## Command Shape

Public syntax:

```text
sqlrs cache explain prepare [--ref <git-ref>] [--ref-mode worktree|blob] [--ref-keep-worktree] <prepare-ref>
sqlrs cache explain prepare:<kind> [--ref <git-ref>] [--ref-mode worktree|blob] [--ref-keep-worktree] [--image <image-id>] [--] [tool-args...]
```

Where:

- `cache` is a top-level command group;
- `explain` is the first subcommand in that group;
- the wrapped prepare stage reuses the same raw and alias-backed grammar already
  accepted for single-stage `prepare`;
- `--ref`, `--ref-mode`, and `--ref-keep-worktree` keep the same meaning as in
  client-side `prepare --ref`;
- `--watch` and `--no-watch` are not accepted here because `cache explain` does
  not execute anything.

The current command does not accept wrapped `plan` or `run` stages. It stays
focused on the cacheability of one prepare-oriented input graph.

---

## Scope

### Supported

- `sqlrs cache explain prepare chinook`
- `sqlrs cache explain prepare --ref origin/main chinook`
- `sqlrs cache explain prepare:psql -- -f ./prepare.sql`
- `sqlrs cache explain prepare:lb -- update --changelog-file db/changelog.xml`

These forms work with local and remote profiles. For remote file-bearing
commands, source sync supplies the same bound input graph used by execution.

### Explicitly out of scope

- `sqlrs cache explain run ...`
- `sqlrs cache explain plan ...`
- composite `prepare ... run ...`
- cache eviction advice or storage-capacity diagnostics

Operational cache-capacity diagnostics remain under the existing commands:

- `sqlrs status --cache`
- `sqlrs ls --states --cache-details`

`cache explain` is for one prepare-oriented decision, not for store-health
overview.

---

## Binding Semantics

`cache explain` binds inputs exactly the same way as the corresponding
single-stage `prepare` command would:

- alias refs stay current-working-directory-relative;
- raw file-bearing args keep the same kind-specific semantics;
- alias-file paths stay relative to the alias file directory;
- `--ref` keeps projected-cwd semantics and the same `worktree` vs `blob`
  vocabulary.

The command reuses the same shared layers already introduced for
repository-aware execution:

- `internal/refctx` for repo/ref/worktree context
- `internal/alias` for alias-target resolution
- `internal/inputset` for per-kind closure and hashing semantics

The whole point of the command is to explain the real prepare decision, not an
approximation.

---

## Output

### Human output

Human output is line-oriented and explicit, for example:

```text
decision: hit
reasonCode: exact_state_match
prepare.kind: psql
prepare.image: postgres:17
cache.signature: sha256:...
cache.stateId: state-123
ref.requested: origin/main
ref.resolvedCommit: abcdef123456
input.count: 3
input[0]: examples/chinook.prep.s9s.yaml sha256:...
input[1]: examples/chinook/prepare.sql sha256:...
input[2]: examples/chinook/include.sql sha256:...
```

For a miss, the current implementation returns:

- `reasonCode: no_matching_state`

More specific miss classification, including image, input-hash, and lookup
causes, is not implemented yet and is tracked in
[#92](https://github.com/kimak-irmagi/sqlrs/issues/92).

### JSON output

With `--output json`, the command emits one stable object:

```json
{
  "decision": "hit",
  "reasonCode": "exact_state_match",
  "prepare": {
    "class": "alias",
    "kind": "psql",
    "image": "postgres:17"
  },
  "refContext": {
    "requested": "origin/main",
    "resolvedCommit": "abcdef123456",
    "mode": "worktree"
  },
  "cache": {
    "signature": "sha256:...",
    "matchedStateId": "state-123"
  },
  "inputs": [
    {"path": "examples/chinook.prep.s9s.yaml", "hash": "sha256:..."},
    {"path": "examples/chinook/prepare.sql", "hash": "sha256:..."}
  ]
}
```

The JSON shape is stable enough for scripting and aligns with the provenance
artifact written by `--provenance-path`.

---

## Validation and Errors

The command rejects:

- missing wrapped prepare stage
- unsupported wrapped stage kinds
- `--ref-mode` or `--ref-keep-worktree` without `--ref`
- `--ref-keep-worktree` with `--ref-mode blob`
- `--watch` or `--no-watch`
- non-Git caller context when `--ref` is requested
- missing alias file or raw file-bearing entrypoint

These failures are normal command errors, not partial explain output.

---

## Examples

Explain an alias-backed local prepare:

```bash
sqlrs cache explain prepare chinook
```

Explain a ref-backed alias prepare:

```bash
sqlrs cache explain prepare --ref origin/main chinook
```

Explain a raw SQL prepare:

```bash
sqlrs cache explain prepare:psql -- -f ./prepare.sql
```

Explain a Liquibase prepare against a selected revision:

```bash
sqlrs cache explain prepare:lb --ref HEAD~1 -- update --changelog-file db/changelog.xml
```

Not supported:

```bash
# not supported
sqlrs cache explain plan chinook
```

```bash
# not supported
sqlrs cache explain prepare --no-watch chinook
```

---

## Rationale Summary

This command keeps cache explanation narrow and coherent:

- it explains one real prepare-oriented cache decision;
- it reuses the same path/ref/input semantics as `prepare`;
- it stays separate from store-health diagnostics already covered by
  `status --cache` and `ls --states --cache-details`;
- it creates one clear bridge between the landed client-side `--ref` slice
  and the provenance / reproducibility surface.
