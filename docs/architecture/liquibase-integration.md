# Taidon–Liquibase Integration Design

This document describes how Taidon interoperates with Liquibase without re-implementing Liquibase internals (changelog parsing, filtering, ordering). It focuses on:

- obtaining a "plan" (pending changesets)
- obtaining stable per-step hashes (checksums) when possible
- maximising cache hits via rewind (jumping between cached states)
- executing changesets step-by-step and snapshotting safely

---

## Table of Contents

- [1. Scope](#1-scope)
- [2. Integration Modes](#2-integration-modes)
- [3. Observability via Structured Logs](#3-observability-via-structured-logs)
- [4. Planning](#4-planning)
- [5. Hash Horizon and Volatility](#5-hash-horizon-and-volatility)
- [6. Cache Rewind Algorithm](#6-cache-rewind-algorithm)
- [7. Step Execution and Snapshotting](#7-step-execution-and-snapshotting)
- [8. Failure Handling and Compatibility Modes](#8-failure-handling-and-compatibility-modes)
- [9. Cache Keys and Parameters](#9-cache-keys-and-parameters)
- [10. Operational Considerations](#10-operational-considerations)
- [11. Open Questions](#11-open-questions)

---

## 1. Scope

### In scope

- Liquibase CLI orchestration by Taidon ("master" / wrapper)
- Step-wise application (`update-count 1`) and snapshotting after each applied changeset
- Predictive cache lookup using changeset checksum when available
- Handling volatile/unknown steps by materialising state and resuming

### Out of scope (MVP)

- Direct DB inspection by Taidon (e.g., reading `DATABASECHANGELOG`)
- Implementing Liquibase changelog parsing (XML/YAML/JSON/SQL)
- Mid-transaction snapshotting

---

## 2. Integration Modes

### 2.1 Liquibase Master (recommended)

User runs Taidon CLI. Taidon orchestrates Liquibase.

- Pros: best cache hit rate, deterministic orchestration, easiest step-wise execution
- Cons: requires users to adopt `taidon migrate` (or similar)

### 2.2 Liquibase Wrapper (compatibility)

User runs a `taidon-liquibase` wrapper that forwards most flags to Liquibase.

- Pros: minimal workflow change for existing teams
- Cons: still requires using wrapper for full caching benefits

### 2.3 Drop-in DB Proxy (later)

A DB proxy/driver observes commits and snapshots. Predictive caching is limited unless the client provides explicit block markers.

### 2.4 Execution Environments

Liquibase execution is abstracted so the engine can run it in different environments.

**Current (local Windows + WSL engine):**

- Engine runs Liquibase as a **host Windows executable** via WSL interop.
- Paths are translated from WSL (`/mnt/c/...`) to Windows (`C:\...`) before launch.
- `liquibase.exec` config (if set) overrides PATH lookup.

**Future (container execution):**

- Engine runs Liquibase in a **separate container**.
- Paths are translated from WSL to container mount paths.
- The execution interface remains the same; only the runner and path mapper change.

Path mapping is implemented behind a **PathMapper interface** with per-environment
implementations (WSL->Windows, WSL->Container).

---

## 3. Observability via Structured Logs

Taidon treats Liquibase as the source of truth and observes execution through structured JSON logs.

### 3.1 Required log properties

- `logFormat`: `JSON` or `JSON_PRETTY`
- `logLevel`: at least `INFO` (tune as needed)
- stable correlation fields:
  - operation id / run id
  - changeset identifiers (if present)
  - checksum (if present)

### 3.2 Minimum events Taidon needs

- `plan.pending`: pending changeset list (or ability to reconstruct it)
- `step.start`: changeset about to run
- `step.applied`: changeset applied successfully
- `step.failed`: changeset failed

Taidon must not depend on human-readable log messages.

---

## 4. Planning

Taidon obtains a pending changeset plan by invoking Liquibase commands with the same filters and parameters that will be used for execution.

### 4.3 Content locking (atomicity)

To prevent plan drift, Taidon acquires **read-locks** on all Liquibase inputs
for the duration of each task:

- planning (`updateSQL`): lock changelog + referenced files while building the plan
- execution (`update-count --count=1`): lock the same set while applying a step

If any lock cannot be acquired (file is being modified), the task fails.

### 4.1 Inputs that affect the plan

- changelog file path
- contexts / labels filters
- DBMS filters
- changelog parameters (property substitution)
- Liquibase version

### 4.2 Plan output

A plan is an ordered list of steps:

- `step_index` (0..n-1)
- `changeset_ref` (opaque reference for logs/diagnostics)
- `checksum` (optional; may be unknown initially)
- `is_volatile` (optional; may be unknown initially)

---

## 5. Hash Horizon and Volatility

### 5.1 Goal

Compute hashes for as many upcoming steps as possible _without executing them_, then rewind through cache to the most recent cached state.

### 5.2 Horizon definition

The **hash horizon** is the longest prefix of the pending plan for which Taidon can obtain stable `block_hash` values.

Horizon ends when:

- a checksum is not available without executing earlier steps
- Liquibase indicates a change is volatile (SQL not statically determinable)
- plan data is incomplete or inconsistent

### 5.3 Practical horizon strategy (MVP)

- Attempt to obtain checksum per step during planning.
- If checksum is missing for step `k`, treat `k` as horizon end.
- If Liquibase later reveals checksum after materialisation, extend the horizon and continue rewinding.

---

## 6. Cache Rewind Algorithm

### 6.1 Intent

Find the most advanced cached State reachable from `original_base_id` by repeatedly applying known step hashes (without executing anything).

### 6.2 High-level flow

```mermaid
flowchart TD
  A[Start: original_base_id] --> B[Plan pending changesets via Liquibase]
  B --> C[Compute hashes until horizon]
  C --> D[Rewind: walk cache forward by hashes]
  D -->|miss or horizon end| E[Materialize instance from last cached state]
  E --> F[Execute steps (update-count 1) + snapshot]
  F --> G[Try to extend horizon and rewind again]
  G -->|no pending| H[Done: final_state_id]
```

### 6.3 Rewind details

Given:

- `base_id` (initially `original_base_id`)
- `hashes[]` for steps `i..j` (within horizon)

Rewind loop:

1. Compute `key = H(engine, base_id, hashes[next], exec_params)`.
2. If `key` exists in cache:
   - set `base_id = cached_state_id`
   - advance to next step hash
3. Else:
   - stop. The next step must be executed.

No instance is created during rewind.

---

## 7. Step Execution and Snapshotting

### 7.1 Materialisation rule

When rewind stops (cache miss or horizon end), Taidon materialises a instance from the last known `base_id`.

### 7.2 Step-wise execution

Taidon runs Liquibase in step mode:

- `update-count 1` repeatedly until:
  - no pending changesets remain, or
  - execution fails

### 7.3 Snapshot trigger

After each successful `update-count 1`:

- create a new State snapshot
- store `key(base_id, block_hash, exec_params) -> new_state_id`
- update `current_base_id = new_state_id`

### 7.4 Instance switching on cache hit

If a cache hit is discovered for upcoming steps:

- stop using current instance
- create a new instance from the cached State
- resume Liquibase execution

This keeps the user experience logically continuous while physically switching bases.

### 7.5 Plan drift check

After materialisation, Taidon must confirm that Liquibase is applying the expected next step:

- compare the next-step reference observed in logs with the predicted step
- if mismatch occurs, abort with a "plan drift" error

---

## 8. Failure Handling and Compatibility Modes

### 8.1 Failure categories

- **Transactional changeset failure** (`runInTransaction=true`): should rollback, leaving DB unchanged for that step.
- **Non-transactional changeset failure** (`runInTransaction=false`): may partially apply changes.

### 8.2 Diagnostic snapshot (optional)

On failure, Taidon may snapshot the failed instance state for investigation.

- `status=failed`
- excluded from automatic cache lookup
- can be pinned/tagged explicitly by the user

### 8.3 Compatibility modes for `current_base_id`

- **Conservative (last-success)**:
  - set `current_base_id` to last successful snapshot
  - treat failed non-transactional step as not persisted for future runs
- **Investigative (failed-base)**:
  - set `current_base_id` to diagnostic failed snapshot
  - allow users to reproduce and iterate from the failure state

---

## 9. Cache Keys and Parameters

This is the current Liquibase-specific MVP key. It is not the Runtime v2 logical
state contract and remains unchanged by issue #107. Future adoption maps the
resolved Liquibase plan to `ResolvedTransformIdentity` values and derives states
through the engine-neutral Runtime v2 parent/transform formula; checkpoint and
planning policy stay outside that semantic core.

### 9.1 Canonical key

```code
key = H(
  engine_id,
  engine_version,
  base_state_id,
  block_hash,
  execution_params
)
```

### 9.2 Execution parameters

`execution_params` must include any inputs that change results:

- contexts / labels
- dbms filter
- changelog parameters (property substitution)
- Liquibase version (until proven safe to omit)
- environment flags influencing execution

### 9.3 Block hash source

The per-step `block_hash` is **content-based**:

- preferred: Liquibase checksum (when available without execution)
- fallback: hash of the SQL emitted for that changeset (`updateSQL`)

Changeset identity (author/id/path) is recorded for diagnostics/UX and plan
drift detection but **does not** participate in the cache key.

---

## 10. Operational Considerations

### 10.1 Log volume

JSON structured logs can become very large for big SQL payloads.

- prefer writing logs to file and streaming/processing incrementally
- support log level tuning
- store only required keys/fields for long-term retention

### 10.2 Concurrency

- Each instance is isolated; Liquibase runs should not share a DB instance.
- Cache index must be concurrency-safe (atomic insert for key->state).

### 10.3 Security

- ensure secrets in parameters are redacted from stored logs
- avoid persisting raw SQL unless explicitly enabled

---

## 11. Open Questions

- What is the most reliable way to obtain per-step checksums pre-execution using only Liquibase invocations?
- How should Taidon detect volatile steps from structured logs across Liquibase versions?
- Should Taidon implement a maximum rewind depth or stop early based on replay-cost heuristics?
