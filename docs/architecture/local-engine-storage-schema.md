# Local Engine Storage Schema (SQLite)

Update, 2026-09-14: the approved [managed-identity flow](managed-database-identity.md)
replaces fixed SQL-name assumptions. The [component/schema design](managed-database-identity-internals.md)
and test plan are approved; implementation details below describe the current generation,
not permission to infer or restore a fixed administrative role.

This document defines the SQLite schema for persistent names/instances/states
used by the local engine and exposed via `sqlrs ls`.

## 1. Goals

- Persist names/instances/states in SQLite under `<StateDir>`.
- Support list filters for `sqlrs ls`.
- Keep derived fields (status, refcount) computed from stored data.

## 2. Storage location

- SQLite database file: `<StateDir>/state.db`
- One writer (engine), concurrent readers (CLI).

## 3. Core tables

Schema source: [`schema.sql`](../../backend/local-engine-go/internal/store/sqlite/schema.sql).
Run `pnpm run docs:schemas` to refresh embedded snippets.

### 3.1 `states`

<!--ref:sql -->
[`schema.sql`](../../backend/local-engine-go/internal/store/sqlite/schema.sql#L3-L15)
<!--ref:body-->
```sql
CREATE TABLE IF NOT EXISTS states (
  state_id TEXT PRIMARY KEY,
  parent_state_id TEXT,
  state_fingerprint TEXT,
  image_id TEXT NOT NULL,
  prepare_kind TEXT NOT NULL,
  prepare_args_normalized TEXT NOT NULL,
  created_at TEXT NOT NULL,
  size_bytes INTEGER,
  last_used_at TEXT,
  use_count INTEGER,
  min_retention_until TEXT,
  evicted_at TEXT,
```
<!--ref:end-->

Notes:

- `state_id` is a UUID (see state-cache design).
- This is the current local-engine storage format. Runtime v2 uses content-derived
  `sha256:` StateIDs in its standalone semantic module, but issue #107 does not
  change this table or reinterpret its rows. Adoption requires a separately
  designed schema/integration transition.
- `parent_state_id` is nullable and models state ancestry for recursive deletes.
- `state_fingerprint` may equal `state_id`; it is stored for name bindings even
  if the state is evicted.
- `size_bytes` stores the persisted snapshot size in bytes for the materialized state directory; it may be NULL for legacy rows created before size tracking was enabled.
- `status` is reserved for future use (e.g., `success`, `failed`).

### 3.2 `instances`

<!--ref:sql -->
[`schema.sql`](../../backend/local-engine-go/internal/store/sqlite/schema.sql#L17-L28)
<!--ref:body-->
```sql
  status TEXT
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_states_fingerprint ON states(state_fingerprint);
CREATE INDEX IF NOT EXISTS idx_states_parent ON states(parent_state_id);
CREATE INDEX IF NOT EXISTS idx_states_image ON states(image_id);
CREATE INDEX IF NOT EXISTS idx_states_kind ON states(prepare_kind);

CREATE TABLE IF NOT EXISTS instances (
  instance_id TEXT PRIMARY KEY,
  state_id TEXT NOT NULL,
  image_id TEXT NOT NULL,
  created_at TEXT NOT NULL,
```
<!--ref:end-->

Notes:

- `instance_id` uses the engine-defined id format.
- `runtime_id` stores the exact runtime/container identifier used to stop or inspect the instance;
  managed access also verifies its durable physical binding. Missing containers fail closed.
- `runtime_dir` stores the absolute path to the per-job runtime data directory for cleanup;
  it does not authorize adopting or recreating a managed container.
- Managed publication writes `status=active` only after native access verification.

### 3.3 `names`

<!--ref:sql -->
[`schema.sql`](../../backend/local-engine-go/internal/store/sqlite/schema.sql#L30-L42)
<!--ref:body-->
```sql
  runtime_id TEXT,
  runtime_dir TEXT,
  status TEXT,
  FOREIGN KEY(state_id) REFERENCES states(state_id)
);
CREATE INDEX IF NOT EXISTS idx_instances_state ON instances(state_id);
CREATE INDEX IF NOT EXISTS idx_instances_image ON instances(image_id);
CREATE INDEX IF NOT EXISTS idx_instances_expires ON instances(expires_at);

CREATE TABLE IF NOT EXISTS names (
  name TEXT PRIMARY KEY,
  instance_id TEXT,
  state_id TEXT,
```
<!--ref:end-->

Notes:

- Multiple names may alias the same instance.
- `is_primary` marks the preferred name for instance listings.
- `state_id` is nullable to allow a name to outlive a missing instance/state.

## 3.4 Connection tracking

Connection counts are tracked in memory and are not stored in SQLite.

## 4. Derived fields

- **Instance status**:
  - `expired` if `expires_at` < now.
  - `orphaned` if no names reference the instance.
  - otherwise `active`.
- **Name status**:
  - `missing` if `instance_id` is NULL or instance is missing.
  - `expired` if the referenced instance is expired.
  - otherwise `active`.
- **State refcount**: count of instances with `state_id`.

## 5. ID format and naming rules

- `state_id` uses UUID format (see state-cache design).
- `instance_id` uses the engine-defined id format.
- Names must not match the instance id format. Enforcement is in application
  logic (see `internal/id` and `internal/registry`).

## 6. Relationships

```mermaid
erDiagram
  STATES ||--o{ INSTANCES : "state_id"
  INSTANCES ||--o{ NAMES : "instance_id"
  STATES ||--o{ NAMES : "state_id"
  STATES ||--o{ STATES : "parent_state_id"
```
