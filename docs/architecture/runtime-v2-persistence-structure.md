# Runtime v2 persistence: component and schema design

Status: approved by the user for issue #110, 2026-09-27.

## Boundaries and ownership

The public resolver module adds an opaque, validated persistence representation
for `CacheKey` plus `Resolution`. The local-engine module adds
`internal/runtimev2store` for engine-owned DTOs and interfaces. The existing
`internal/store/sqlite.Store` implements those interfaces in separate
`runtime_v2_*.go` files while retaining its legacy `internal/store.Store`
implementation. This lets one SQLite connection and transaction boundary serve
both interfaces without merging their semantics.

```text
backend/libs/runtime-go/resolver/
  cache_record.go           opaque versioned CacheRecord and strict codec

backend/local-engine-go/internal/
  runtimev2store/
    store.go                 interfaces, closed records, bounded query results
    materialization.go       physical association DTOs and validation
  store/sqlite/
    runtime_v2_schema.go     guarded transactional schema installation
    runtime_v2_state.go      state identity and provenance observations
    runtime_v2_resolution.go resolver.Cache implementation
    runtime_v2_material.go   materialization/component persistence
    runtime_v2_classify.go   explicit legacy/v2 diagnostic classification
```

`runtimev2store` imports the public `runtimev2` and `resolver` packages. Those
public packages do not import the engine. The new package does not extend the
legacy state DTOs and never derives an ID locally.

Logical State records and their resolved identities are immutable persistent
data. Provenance observations are immutable, insert-only records; multiple
diagnostic observations may refer to the same resolved identity. Resolution
evidence is replaceable cache data. Materializations and their components are
immutable physical records with a lifecycle owned by a later checkpoint policy.
This issue provides association reads/writes but does not choose, update, evict,
or garbage-collect checkpoints.

## Store surface

The interface uses public semantic types at every identity boundary:

```go
type Store interface {
    PutRecipeLineage(context.Context, runtimev2.RecipeLineage) error
    PutRelativeLineage(context.Context, runtimev2.RelativeLineage) error
    GetLogicalState(context.Context, runtimev2.StateID) (StateRecord, bool, error)
    TraceLineage(context.Context, runtimev2.StateID) ([]StateRecord, error)
    ListProvenance(context.Context, runtimev2.StateID, ProvenancePageRequest) (ProvenancePage, error)

    PutMaterialization(context.Context, Materialization) error
    ListMaterializations(context.Context, runtimev2.StateID, MaterializationPageRequest) (MaterializationPage, error)
    ClassifyState(context.Context, string) (StateClassification, error)
}
```

`GetLogicalState` is intentionally distinct from the existing legacy
`sqlite.Store.GetState(context.Context, string)`: Go has no method overloading,
and the SQLite type implements both store boundaries without wrapping or
conflating their result DTOs.

`StateRecord` is a closed read-only value with accessors for record version,
public `runtimev2.State`, exactly one public resolved factory or transform
identity. Provenance observations are read through the separate bounded page API.
It has no exported fields or caller-accessible invalid constructor. The adapter
wraps an identity in diagnostic-free public provenance and verifies it with
`FactoryState` or `Derive`; it does not reproduce the hash formula. Each decoded
observation must contain the same resolved identity but may carry different
declaration or resolver diagnostics. `TraceLineage` returns root-to-endpoint
order, is limited to `runtimev2.MaxTransforms + 1` and 32 MiB of canonical
state/identity data, and validates every edge before returning anything.

`ProvenancePageRequest` has the same required 1..100 row limit and 32 MiB page
budget as materialization pages. Its opaque cursor binds the StateID, a
first-page provenance insertion high-water mark, and the last returned insertion
sequence. Observations use deterministic first-observed-first
`insertion_seq ASC` order without exposing the sequence; later observations are
visible only to a new traversal.

`MaterializationPageRequest` requires a limit from 1 through 100 and accepts an
optional opaque cursor returned by the previous page. The versioned cursor binds
the StateID, first-page insertion-sequence high-water mark, and the last returned
insertion sequence, so it cannot be reused for another State. Inserts after the
first page are excluded from that traversal. `MaterializationPage` contains at
most the requested number of values and an optional next cursor. Results use
stable newest-persisted-first `insertion_seq DESC` keyset order; the sequence is
not exposed in the returned DTO. Offset pagination and an unbounded list
operation are not exposed. A materialization or provenance page
also stops before its next item would exceed 32 MiB of canonical returned data;
because one valid item is smaller than that budget, every nonterminal page makes
progress. The cursor resumes at the first item not returned.

The first page captures the table's committed `sqlite_sequence` value (zero when
absent) in the same read transaction as the page query. Continuation pages use
only the cursor's high-water mark and last sequence, so they neither compute
`MAX` over a growing State nor retain a database transaction between calls.

The two cursor types are distinct closed `runtimev2store` values with private
fields and no text/JSON persistence contract. They are valid only as continuation
tokens within the running engine; after restart a caller starts a new snapshot.
The private version discriminator permits internal evolution and is still
validated defensively. Reusing a valid cursor with another State is an error.

The byte budget has a storage-independent exact definition. A provenance item
costs the UTF-8 byte lengths of its returned record version, observation digest,
and timestamp plus its canonical `provenance_json`. A materialization costs
the UTF-8 byte lengths of all returned string fields plus metadata blobs and,
for every component, its string fields plus metadata; present `int64` values add
eight bytes. Null fields add zero. Lineage cost is the sum of canonical
`state_json` and `resolved_identity_json` byte lengths. Cursor bytes and Go
allocation overhead are not part of the protocol limit.

`StateClassification` contains `LegacyPresent`, `RuntimeV2Present`, and the exact
`RuntimeV2RecordVersion` when present. Its raw string argument intentionally
accepts legacy identifiers that are not valid Runtime v2 StateIDs. Classification
never decodes or adopts a legacy row.

The SQLite store also implements `resolver.Cache` directly:

```go
Load(context.Context, resolver.CacheKey) (resolver.CacheLoad, error)
Store(context.Context, resolver.CacheKey, resolver.Resolution) error
```

Cache replacement changes evidence or a resolved identity only within the exact
versioned cache key. Logical states already built from an earlier resolution are
immutable and are not updated or deleted by cache replacement.

The resolver package adds this public surface without exposing key fields for
mutation:

```go
func NewCacheRecord(CacheKey, Resolution) (CacheRecord, error)
func DecodeCacheRecordJSON([]byte) (CacheRecord, error)
func (CacheRecord) Matches(CacheKey) bool
func (CacheRecord) Resolution() Resolution
func (CacheRecord) MarshalJSON() ([]byte, error)
```

`CacheRecord` uses the existing `sqlrs.resolution-cache.v1` envelope and contains
the key digest, workspace scope, resolver descriptor, normalized declaration,
resolution, and checksum. `Matches` compares every key constituent. Accessors
return defensive copies. The existing `DirectoryCache` and the new SQLite cache
share this codec, preventing format and validation drift.

### Public module release boundary

`backend/libs/runtime-go/v0.2.0` is already immutable and published. Exporting
`CacheRecord` is an additive public API change targeted at
`backend/libs/runtime-go/v0.3.0`, preceded by immutable `v0.3.0-rc.N` tags and the
existing clean-consumer, public-proxy, checksum-database, race, fuzz-smoke, and
coverage gates. No existing tag is moved or overwritten.

The refactor of `DirectoryCache` must remain byte-compatible with persisted
`sqlrs.resolution-cache.v1` records written by v0.2.0. Existing records load
without rewrite, retain the same checksum definition, and re-encode identically.
The new public type changes neither Runtime v2 identity domains nor StateID
semantics. The repository workspace can build the local-engine adapter against
the sibling module before v0.3.0 is published, but external consumers must use
the tagged release rather than a production `replace`.

`Materialization` uses `runtimev2.StateID` and contains a non-semantic
materialization ID, backend, optional runtime/job IDs, timestamp, optional total
size and bounded metadata, plus a name-keyed set of `MaterializationComponent`
values. A component has a unique name within its materialization, kind, locator,
optional size, and bounded metadata. Caller order is discarded; validation and
canonical persistence sort components by name. An idempotent duplicate must be
canonically identical; policy updates use a new materialization ID.

Materialization IDs are 1..256-byte UTF-8 opaque values without control
characters. Backend, component name, and component kind use the Runtime v2
identifier grammar. Runtime/job IDs are at most 256 UTF-8 bytes, locators at most
4096 UTF-8 bytes, and one materialization contains at most 256 components.
Metadata is a JSON object, defaults to `{}`, rejects duplicate keys and trailing
tokens, and is limited to 64 KiB per record. Metadata interpretation is governed
by the persistence record version; changing its contract requires a new version.
Secrets must not be stored in metadata or locators.

Every persisted timestamp is normalized before comparison and storage to UTC
with exactly nine fractional-second digits (`2006-01-02T15:04:05.000000000Z`).
The store receives an injected clock for `stored_at` and `observed_at`.
Idempotent duplicates preserve the first timestamp; equivalent instants written
with another RFC 3339 offset or precision canonicalize to the same value.

## Versioned SQLite schema

All names prefixed `runtime_v2_` are reserved by this format. The marker version
is `sqlrs.runtime-persistence.v1`; semantic JSON must identify
`sqlrs.runtime.v2`. Each data table repeats `record_version` so exported rows and
diagnostics remain self-classifying outside the database marker.

<!--ref:sql -->
[Canonical Runtime v2 schema](../../backend/local-engine-go/internal/store/sqlite/schema.sql#region=SQLRS%20RUNTIME%20V2%20SCHEMA)
<!--ref:body-->
```sql
CREATE TABLE runtime_v2_store_format (
  slot INTEGER PRIMARY KEY CHECK (slot = 1),
  format_version TEXT NOT NULL CHECK (format_version = 'sqlrs.runtime-persistence.v1'),
  semantic_version TEXT NOT NULL CHECK (semantic_version = 'sqlrs.runtime.v2')
);

CREATE TABLE runtime_v2_states (
  state_id TEXT PRIMARY KEY CHECK (length(state_id) = 71),
  record_version TEXT NOT NULL CHECK (record_version = 'sqlrs.runtime-persistence.v1'),
  state_kind TEXT NOT NULL CHECK (state_kind IN ('factory', 'derived')),
  parent_state_id TEXT CHECK (parent_state_id IS NULL OR length(parent_state_id) = 71),
  state_json BLOB NOT NULL CHECK (length(state_json) <= 4194304),
  resolved_identity_json BLOB NOT NULL CHECK (length(resolved_identity_json) <= 4194304),
  stored_at TEXT NOT NULL,
  FOREIGN KEY (parent_state_id) REFERENCES runtime_v2_states(state_id),
  CHECK ((state_kind = 'factory' AND parent_state_id IS NULL) OR
         (state_kind = 'derived' AND parent_state_id IS NOT NULL))
);
CREATE INDEX runtime_v2_states_parent ON runtime_v2_states(parent_state_id);

CREATE TABLE runtime_v2_provenance_observations (
  insertion_seq INTEGER PRIMARY KEY AUTOINCREMENT,
  state_id TEXT NOT NULL,
  observation_digest TEXT NOT NULL CHECK (length(observation_digest) = 71),
  record_version TEXT NOT NULL CHECK (record_version = 'sqlrs.runtime-persistence.v1'),
  provenance_json BLOB NOT NULL CHECK (length(provenance_json) <= 4194304),
  observed_at TEXT NOT NULL,
  UNIQUE (state_id, observation_digest),
  FOREIGN KEY (state_id) REFERENCES runtime_v2_states(state_id) ON DELETE CASCADE
);
CREATE INDEX runtime_v2_provenance_state_page
  ON runtime_v2_provenance_observations(state_id, insertion_seq ASC);

CREATE TABLE runtime_v2_resolutions (
  cache_key TEXT PRIMARY KEY CHECK (length(cache_key) = 64),
  record_version TEXT NOT NULL CHECK (record_version = 'sqlrs.runtime-persistence.v1'),
  cache_record_json BLOB NOT NULL CHECK (length(cache_record_json) <= 4194304),
  stored_at TEXT NOT NULL
);

CREATE TABLE runtime_v2_materializations (
  insertion_seq INTEGER PRIMARY KEY AUTOINCREMENT,
  materialization_id TEXT NOT NULL UNIQUE
    CHECK (length(CAST(materialization_id AS BLOB)) BETWEEN 1 AND 256),
  record_version TEXT NOT NULL CHECK (record_version = 'sqlrs.runtime-persistence.v1'),
  state_id TEXT NOT NULL,
  backend TEXT NOT NULL CHECK (length(CAST(backend AS BLOB)) BETWEEN 1 AND 128),
  runtime_id TEXT CHECK (runtime_id IS NULL OR length(CAST(runtime_id AS BLOB)) BETWEEN 1 AND 256),
  job_id TEXT CHECK (job_id IS NULL OR length(CAST(job_id AS BLOB)) BETWEEN 1 AND 256),
  created_at TEXT NOT NULL,
  size_bytes INTEGER CHECK (size_bytes IS NULL OR size_bytes >= 0),
  metadata_json BLOB NOT NULL CHECK (length(metadata_json) BETWEEN 2 AND 65536),
  FOREIGN KEY (state_id) REFERENCES runtime_v2_states(state_id)
);
CREATE INDEX runtime_v2_materializations_state_page
  ON runtime_v2_materializations(state_id, insertion_seq DESC);

CREATE TABLE runtime_v2_materialization_components (
  materialization_id TEXT NOT NULL,
  record_version TEXT NOT NULL CHECK (record_version = 'sqlrs.runtime-persistence.v1'),
  name TEXT NOT NULL CHECK (length(CAST(name AS BLOB)) BETWEEN 1 AND 128),
  kind TEXT NOT NULL CHECK (length(CAST(kind AS BLOB)) BETWEEN 1 AND 128),
  locator TEXT NOT NULL CHECK (length(CAST(locator AS BLOB)) BETWEEN 1 AND 4096),
  size_bytes INTEGER CHECK (size_bytes IS NULL OR size_bytes >= 0),
  metadata_json BLOB NOT NULL CHECK (length(metadata_json) BETWEEN 2 AND 65536),
  PRIMARY KEY (materialization_id, name),
  FOREIGN KEY (materialization_id)
    REFERENCES runtime_v2_materializations(materialization_id) ON DELETE CASCADE
);

CREATE TRIGGER runtime_v2_states_immutable BEFORE UPDATE ON runtime_v2_states
BEGIN SELECT RAISE(ABORT, 'runtime v2 state is immutable'); END;
CREATE TRIGGER runtime_v2_provenance_immutable
BEFORE UPDATE ON runtime_v2_provenance_observations
BEGIN SELECT RAISE(ABORT, 'runtime v2 provenance is immutable'); END;
CREATE TRIGGER runtime_v2_materializations_immutable
BEFORE UPDATE ON runtime_v2_materializations
BEGIN SELECT RAISE(ABORT, 'runtime v2 materialization is immutable'); END;
CREATE TRIGGER runtime_v2_materialization_components_immutable
BEFORE UPDATE ON runtime_v2_materialization_components
BEGIN SELECT RAISE(ABORT, 'runtime v2 materialization component is immutable'); END;
CREATE TRIGGER runtime_v2_states_no_delete BEFORE DELETE ON runtime_v2_states
BEGIN SELECT RAISE(ABORT, 'runtime v2 state deletion is not enabled'); END;
CREATE TRIGGER runtime_v2_provenance_no_delete
BEFORE DELETE ON runtime_v2_provenance_observations
BEGIN SELECT RAISE(ABORT, 'runtime v2 provenance deletion is not enabled'); END;
CREATE TRIGGER runtime_v2_materializations_no_delete
BEFORE DELETE ON runtime_v2_materializations
BEGIN SELECT RAISE(ABORT, 'runtime v2 materialization deletion is not enabled'); END;
CREATE TRIGGER runtime_v2_materialization_components_no_delete
BEFORE DELETE ON runtime_v2_materialization_components
BEGIN SELECT RAISE(ABORT, 'runtime v2 materialization component deletion is not enabled'); END;
```
<!--ref:end-->

The semantic JSON columns use the public strict codecs. `state_id`,
`state_kind`, and `parent_state_id` are indexed integrity copies and must match
decoded `state_json`. `resolved_identity_json` contains exactly the matching
public resolved factory or transform identity. Each `provenance_json` contains a
public provenance value with that same identity; diagnostics may differ between
observations. `cache_record_json` is the public closed `resolver.CacheRecord`
envelope, while `cache_key` is its indexed integrity copy.

`observation_digest` is `sha256:` plus the lowercase SHA-256 of the exact
canonical public provenance JSON bytes. It is a storage deduplication key, not a
Runtime v2 identity. Insert verifies canonical-byte equality after a digest
conflict. Application validation enforces identifier grammar, UTF-8, JSON shape,
component-count bounds, and canonical fixed-width UTC timestamps in addition to
SQL size checks.

Timestamps and all physical fields are deliberately absent from uniqueness and
semantic validation. Each `insertion_seq` is a storage-only pagination watermark
and never enters a DTO, identity, observation digest, or duplicate comparison.
No foreign key connects these tables to legacy `states`.
UPDATE triggers enforce immutable records. DELETE triggers deliberately keep
logical-state, provenance, and physical-record GC out of this issue; a later
approved lifecycle schema must replace them before exposing deletion APIs.

## Installation and compatibility

`sqlite.InstallRuntimeV2Schema(ctx, tx)` is the one low-level installer and
verifier. It runs after `PRAGMA foreign_keys = ON`, inspects `sqlite_master` for
all reserved names before changing v2 objects, and either creates the complete
format, verifies the already complete supported format, or returns an error that
lets its caller roll back. Table/index/trigger shape checks reject shadowing,
partial installation, and unknown formats.

The existing `backend/local-engine-go/internal/store/sqlite/schema.sql` remains
the canonical DDL source required by ADR 0003. Delimited Runtime v2 statements
are embedded once and consumed by the installer; `runtime_v2_schema.go` contains
orchestration and shape verification, not a second SQL copy. The architecture
schema block is synchronized from that source by the documentation schema tool.

Production `managedstore.Initialize` invokes the low-level installer in its
existing startup transaction for both fresh and already managed stores; its
in-memory expected-schema builder invokes the same function. The idempotent
`sqlite.EnsureRuntimeV2Schema(ctx, db)` wrapper opens a transaction around the
same installer and is called by `sqlite.initDB`, covering direct `Open`/`New`
users and rc.6 upgrade fixtures. In production the later `sqlite.New` call only
re-verifies the schema installed by `managedstore.Initialize`. No second DDL
implementation exists.

The installer does not change connection locking policy or add retries. Every
supported construction path enables foreign keys on its single SQLite
connection and keeps `PRAGMA busy_timeout = 0`; an external lock is returned as
`SQLITE_BUSY` without an internal wait, as required by ADR 0015.

The installation does not require the legacy tables to be empty and does not
call the stricter managed-identity migration gate, because no legacy record is
adopted. Existing rc.6 tables and rows remain byte-for-byte under their current
code path. Runtime v2 getters include the record-version predicate even though
the table has a constraint; they never fall back to an unversioned query.

## Consistency and transactions

Recipe/relative lineage, one materialization plus all its components, and schema
installation are separate atomic transaction units. Resolution cache entries are
atomic per key. State insertion is parent-first and immutable. For duplicate
state identities, stored JSON is strictly decoded and canonically re-encoded;
canonical identity bytes must match. Alternate matching provenance is inserted
as another observation rather than treated as a conflict. Observation digest
collisions require canonical-byte equality. Materialization duplicates compare a
canonical DTO with components sorted by name.

Cancellation or a failure before commit rolls the transaction back. If `Commit`
returns an error after SQLite may have committed, the operation reports the
error and makes no claim about which complete generation is visible. After
reopen, callers may observe only the complete old or complete new generation;
because every write is idempotent, retrying the same operation must converge to
the complete new generation. A partial schema, lineage, cache record, or
materialization is never a permitted result.

Reads validate stored JSON, reconstruct diagnostic-free provenance from the
stored resolved identity, recompute the State relationship through the public
semantic core, compare indexed copies, and return no partial value. Observation
validation against that identity occurs page by page;
a corrupt item fails its whole page without affecting State lookup or another
page. Lineage traversal detects missing parents
and cycles and enforces its byte budget. Provenance and materialization reads
validate all bounds and use the bounded snapshot-page contracts above;
components use canonical name order. Exact
State, parent, provenance, cache-key, and materialization-page queries use their
declared indexes; lineage performs at most one indexed parent lookup per bounded
State.
