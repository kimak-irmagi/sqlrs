# Runtime v2 persistence decisions

Conversation timestamp: 2026-09-27 18:22:55 Asia/Novosibirsk
(2026-09-27 11:22:55 UTC). GitHub user ID: @evilguest (41718235).
Agent: OpenAI Codex (GPT-5). Approval timestamp: 2026-09-27 18:22:55
Asia/Novosibirsk (2026-09-27 11:22:55 UTC).

Status: accepted for issue #110 after critical design review.

## Decision 1: side-by-side legacy and Runtime v2 persistence

Question: how should Runtime v2 records coexist with rc.6 state and cache data?

Alternatives: reinterpret or migrate legacy rows in place; replace the legacy
schema and runtime path; add an explicitly versioned Runtime v2 namespace beside
the existing records.

Decision: add reserved `runtime_v2_*` SQLite objects with a singleton format and
semantic-version marker. Runtime v2 getters read only those objects. Legacy rows
remain on their existing path and can only be reported by explicit diagnostic
classification; they never become Runtime v2 cache hits or States implicitly.
The default prepare/runtime path remains legacy until a separate cutover.

Rationale: development can proceed against populated rc.6 stores without
changing supported behavior, silently changing identity, or requiring reverse
migration when the dormant v2 path is disabled.

## Decision 2: separate identity, observations, cache, and materialization

Question: which Runtime v2 concepts should share a persistent record?

Alternatives: one mutable state/cache row; one State row containing physical
checkpoint fields; separate records for immutable logical identity, provenance
observations, replaceable resolution evidence, and physical materializations.

Decision: store each logical State and its resolved identity immutably; store
zero or more immutable, deduplicated provenance observations separately; store
resolution evidence as replaceable cache data keyed by the complete versioned
cache key; and associate zero or more immutable materializations with a State.
A materialization can contain a canonical name-keyed set of components. Physical
paths, backend/runtime/job identifiers, timestamps, and sizes never affect the
StateID.

Rationale: replay and explain retain semantic evidence while cache refresh and
physical checkpoint multiplicity cannot mutate logical identity.

## Decision 3: public cache record and engine-owned store boundary

Question: where should persistence validation and engine storage APIs live?

Alternatives: duplicate resolver serialization inside the engine; expose mutable
cache DTO fields; put SQLite concerns in the public runtime module; share an
opaque public cache record while keeping engine persistence internal.

Decision: `runtime-go/resolver` adds an opaque validated `CacheRecord` retaining
the complete `CacheKey` context and `Resolution`, using the existing
`sqlrs.resolution-cache.v1` wire format. `DirectoryCache` and the SQLite adapter
share that codec. The local engine owns a closed `internal/runtimev2store.Store`,
implemented by the existing SQLite store without extending legacy DTOs. The
additive public API targets immutable runtime-go v0.3.0 RC/GA tags; v0.2.0 tags
remain untouched.

Rationale: one public trust boundary prevents codec drift and keeps StateID logic
inside the semantic module, while SQLite and legacy engine details stay out of
the reusable module.

## Decision 4: guarded transactional schema installation

Question: how should fresh stores, direct SQLite users, managed startup, and
populated rc.6 stores receive the Runtime v2 schema?

Alternatives: separate DDL implementations for each entry point; lazy per-table
creation; one installer/verifier called from every supported construction path.

Decision: use one low-level transactional installer/verifier after enabling
foreign keys. It inspects every reserved name before mutation, creates the whole
supported format atomically when absent, verifies a complete supported format
idempotently, and rejects collisions, partial shapes, corruption, and unknown
versions. Managed startup invokes it in its existing schema transaction; direct
`sqlite.Open`/`New` paths invoke an explicit transactional wrapper.

Rationale: every constructor sees the same complete schema, failed upgrades leave
no partial namespace, and existing legacy or unrelated data is neither adopted
nor rewritten.

## Decision 5: immutable writes, bounded reconstruction, and deferred lifecycle

Question: what consistency and lifecycle guarantees belong to issue #110?

Alternatives: best-effort multi-row writes; update records in place; add
checkpoint selection and garbage collection now; use atomic insert-only records
and defer lifecycle policy.

Decision: schema installation, each lineage write, and each materialization with
its components are separate atomic transactions; cache replacement is atomic per
key. Duplicate immutable values succeed only when canonically equivalent.
Provenance digest collisions additionally require byte equality. Reads strictly
decode and recompute public semantic relationships, validate indexed copies, use
bounded lineage traversal, detect cycles and missing parents, and return no
partial values. UPDATE and DELETE triggers keep logical, provenance, and
materialization records immutable until a separately approved lifecycle design
replaces that policy.

Rationale: restart and concurrent retry behavior is deterministic, corrupt rows
fail closed, and issue #110 does not accidentally define checkpoint selection or
garbage collection.

The detailed contracts are specified by the
[interaction flow](../architecture/runtime-v2-persistence-flow.md) and
[component/schema design](../architecture/runtime-v2-persistence-structure.md).
No earlier ADR is obsolete: this decision is additive and retains the existing
SQLite ownership, canonical schema-source, shared-connection, and external-lock
fail-fast decisions.

## Decision 6: bounded materialization pagination

Refinement timestamp: 2026-09-27 19:15:47 Asia/Novosibirsk
(2026-09-27 12:15:47 UTC). GitHub user ID: @evilguest (41718235).
Agent: OpenAI Codex (GPT-5).

Question: how can a State expose arbitrarily many materializations and provenance
observations without an unbounded read result?

Alternatives: return every row; use offset pagination; use bounded keyset pages
with a closed cursor.

Decision: expose separate provenance and materialization page APIs with a row
limit from 1 through 100 and a 32-MiB canonical-data budget. Each versioned
opaque cursor binds the StateID, first-page insertion-sequence high-water mark,
and last returned insertion sequence. Materializations use newest-persisted-first
`insertion_seq DESC`; provenance uses first-observed-first `insertion_seq ASC`.
The storage sequence is not exposed in DTOs or identity. Later inserts are
excluded from an existing traversal. There is no unbounded observation or
materialization list. Whole-lineage results independently have a 32-MiB budget.

Rationale: work and memory are bounded by bytes as well as rows, concurrent
inserts cannot create offset gaps, and a cursor cannot silently continue another
State's query.

## Decision 7: ambiguous commit and idempotent retry

Refinement timestamp: 2026-09-27 19:15:47 Asia/Novosibirsk
(2026-09-27 12:15:47 UTC). GitHub user ID: @evilguest (41718235).
Agent: OpenAI Codex (GPT-5).

Question: what does a write guarantee if `Commit` reports an error after SQLite
may already have committed?

Alternatives: promise rollback; report success by probing immediately; report
the error, permit only a complete old/new generation, and rely on idempotent
retry.

Decision: pre-commit failure rolls back. An ambiguous commit returns its error;
after reopen only the complete old or complete new generation is valid. Repeating
the same operation must safely converge to the complete new generation.

Rationale: a driver cannot always prove commit outcome, while atomic records and
idempotence provide a truthful restart-safe recovery contract.

## Decision 8: canonical timestamps and deterministic result order

Refinement timestamp: 2026-09-27 19:15:47 Asia/Novosibirsk
(2026-09-27 12:15:47 UTC). GitHub user ID: @evilguest (41718235).
Agent: OpenAI Codex (GPT-5).

Question: how should equivalent timestamp spellings and unordered observations
or materializations behave?

Alternatives: retain caller spelling and unspecified order; store Unix integers;
normalize fixed-width UTC text and define stable tie breakers.

Decision: persist UTC timestamps with exactly nine fractional digits and preserve
the first timestamp for an idempotent duplicate. Page order is independent of
caller clocks: observations use insertion sequence ascending, materializations
use insertion sequence descending, and components use name order. Insertion
sequence remains storage-only.

Rationale: fixed-width UTC text has stable SQLite ordering, retries do not rewrite
history, and callers receive deterministic restart-independent results.

## Decision 9: one DDL source and inherited SQLite lock policy

Refinement timestamp: 2026-09-27 19:15:47 Asia/Novosibirsk
(2026-09-27 12:15:47 UTC). GitHub user ID: @evilguest (41718235).
Agent: OpenAI Codex (GPT-5).

Question: should the Runtime v2 installer copy DDL or change SQLite locking
behavior?

Alternatives: duplicate SQL in installer/docs; add a separate schema source;
retain canonical `schema.sql` and the existing fail-fast lock contract.

Decision: delimit Runtime v2 statements in the existing canonical `schema.sql`;
the installer embeds that source once and documentation is synchronized from it.
Every supported single-connection path enables foreign keys, keeps
`busy_timeout=0`, and adds no retry or sleep.

Rationale: this conforms to ADR 0003 and ADR 0015, prevents schema drift, and
makes external locking visible instead of hiding it behind startup latency.

## Decision 10: independent compatibility and corruption evidence

Refinement timestamp: 2026-09-27 19:15:47 Asia/Novosibirsk
(2026-09-27 12:15:47 UTC). GitHub user ID: @evilguest (41718235).
Agent: OpenAI Codex (GPT-5).

Question: what evidence proves old-version compatibility and strict corrupted-row
handling without using the implementation as its own oracle?

Alternatives: fixtures created by current code; mocks only; a pinned rc.6 harness,
independently authored golden rows, and an isolated raw corruption fixture.

Decision: generate the upgrade fixture with a pinned historical rc.6 revision
and record its checksum; reopen an upgraded copy with the historical harness for
legacy CRUD. Persistence golden rows come from reviewed public semantic vectors
without the SQLite adapter. Impossible corrupt states are created only in a
disposable test database through an explicit constraint-bypassing raw connection.

Rationale: the tests can expose a shared encoder bug, demonstrate real rollback
compatibility, and exercise defensive readers without adding a production bypass.

## Decision 11: distinct logical-state getter name

Refinement timestamp: 2026-09-27 20:43:02 Asia/Novosibirsk
(2026-09-27 13:43:02 UTC). GitHub user ID: @evilguest (41718235).
Agent: OpenAI Codex (GPT-5). Approved by the user in the issue #110 design
implementation conversation.

Question: how can the existing SQLite type implement both the legacy store and
the Runtime v2 store when both originally specified a `GetState` method with
different parameter and result types?

Alternatives: rename the Runtime v2 getter; add a wrapper/view solely to retain
the overloaded spelling; rename the widely used legacy getter.

Decision: name the Runtime v2 method `GetLogicalState(context.Context,
runtimev2.StateID)`. Keep the legacy `GetState(context.Context, string)` intact,
and let one SQLite type implement both interfaces.

Rationale: Go does not support method overloading. The distinct semantic name
avoids a broad legacy migration and an otherwise unnecessary wrapper while
keeping the two result DTOs separate.
