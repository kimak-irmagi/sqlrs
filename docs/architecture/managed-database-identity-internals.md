# Managed database identity: components and persistence

Status: structure, schema and upgrade conditions approved, 2026-09-14, @evilguest. Implements the approved
[common flow](managed-database-identity.md). This document is mirrored in taidon
and izess. Code and the [test plan](managed-database-identity-tests.md) are also
approved, reconfirmed 2026-09-15. No data deletion is authorized.

The sections below record the approved local/shared design. Local implementation
and live acceptance are recorded in the [test report](managed-database-identity-tests.md);
shared acceptance remains a separate merge gate.

## Ownership and module boundaries

| Owner | Durable responsibility | In-memory responsibility |
| --- | --- | --- |
| Local `internal/managedidentity` (new), using `internal/store/sqlite` | Base-lineage reservations and state bindings | Validation and canonical key construction |
| Shared snapshot-cache `internal/lineage` (new) | Organization-scoped reservations and snapshot/catalog bindings | Resolve/reserve and publication checks |
| Local `internal/instanceaccess` (new) | Activation intents and protected secret references | Coordinate maintenance, proof and publication |
| Shared env-manager `instanceaccess` / `accesssecrets` | Existing intents/secrets, extended identity binding | Existing lease/access coordinator |
| Local `internal/dbms` and `internal/runtime` | No authoritative identity registry | PostgreSQL init, SQL connection, maintenance and proof |
| Shared node-runtime-agent | Bound execution/runtime journal and non-secret evidence | Physical initialization, seal, maintenance, HBA and native proof |
| Local prepare / shared sql-runner and orchestrator | Job/task association with selected lineage | Resolve before hashing, propagate without generating names |

The cache owner generates names, using the PostgreSQL adapter's naming policy.
The adapter validates the expected identity and actual database, not an arbitrary
administrator supplied by a recipe. Shared authorization derives organization and
actor from the trusted request context; lineage is organization cache metadata,
not the actor who owns a job or instance. Local domain is one engine state store.
No global registry and no new network service are introduced.

## Types and semantic interfaces

Approved non-secret value types:

- `BaseSelector{DomainRef, EngineKind, ImageDigest, InitSpecDigest, PolicyVersion}`.
- `ManagedIdentity{LineageRef, EngineKind, PolicyVersion, Username}`.
- `IdentityBinding{ManagedIdentity, IdentityDigest}`; digest covers all fields
  with a versioned, length-delimited canonical encoding.
- `RuntimeBinding{RuntimeRef, PhysicalIdentity, IdentityBinding}`.
- `AccessBinding{RuntimeBinding, InstanceRef, AccessVersion, SecretRef}`.

For PostgreSQL v1, Username must match `^sqlrs_admin_[0-9a-f]{32}$`; callers cannot
substitute a different name with the same lineage reference. The identity is not
a secret, but its provenance is security-relevant. Enforce bounds and exact
equality, not prefix discovery. The default database remains `postgres`; it is
not a role and is not renamed. OS process user remains `postgres`.

Semantic ports (Go interfaces/Java DTOs adapt these same operations):

- `ResolveOrReserveBase(ctx, selector) -> IdentityBinding`: atomic winner, durable
  before return; does not initialize PostgreSQL or reserve an instance password.
- `GetIdentity(ctx, authorizedDomain, lineageRef) -> IdentityBinding`: read-only,
  missing/corrupt data is an error; never regenerate.
- `InitializeBase(ctx, runtimeTarget, identity, bootstrapSecretRef)`: requires an
  empty assigned target or an exact matching resumable operation.
- `VerifyManagedAccess(ctx, runtimeBinding, accessRef) -> ManagedAccessProof`:
  fresh connection, exact role and administrative capability; no arbitrary SQL.
- `SealVerifiedState(ctx, runtimeBinding, accessRef) -> SealProof`: hold the same
  runtime exclusion across verification and DB stop; bind seal to physical
  identity, identity digest and operation fingerprint. Capture requires this seal.
- `EnsureInstanceAccess(ctx, accessBinding) -> AccessProof`: apply only to the
  assigned writable clone; preserve missing-role/lost-privilege failure semantics.
- `ResolveConnection(ctx, authorizedInstance) -> sensitive connection value`:
  assemble only in memory, never persist a DSN or use it for generic diagnostics.

Local Runtime.InitBase becomes a typed request carrying the identity and a private
bootstrap-secret reference; Start and readiness carry RuntimeBinding instead of
assuming a SQL username. Existing ExecRequest.User remains the OS user. Keep
snapshot stop/resume in the DBMS connector; add a separate managed-access port,
rather than making generic Exec the public administrative contract. psql,
Liquibase and connection construction consume the same resolved identity.

Proof values are bounded internal evidence, not reusable bearer capabilities:
check the live operation, journal, runtime and deletion state again when consuming
them. No proof contains password/verifier/DSN. Driver-based credential checks are
required; this does not replace psql as the user SQL execution tool.

## Shared private protocol contract

Add `managed-identity.v1` capability with mandatory identityBinding on native
execution provision, prepared-runtime setup, verified seal, capture and activation.
Use `node-runtime.v2` for the changed node envelope and `psql-cow-state.v2` for
state requests/responses; cover every added field in operation fingerprints.
Materialize resolves identity from its snapshot owner and returns it; it must
reject a caller's incompatible expectation rather than adopt it.

Add owner-only `POST /internal/state/base-lineages:resolve`, accepting the
versioned BaseSelector without caller-selected username; organization comes from
the existing trusted context. Return the winning IdentityBinding. SQL-runner
resolves this before HashUtils state/cache lookup; orchestrator persists the
resolved binding with the job. Retries must not silently pick a new policy.

This is an internal wire change, not a new public endpoint. Rollout requires a
coherent private-protocol set while admission is closed. Unsupported peers must
reject before mutation; there is no fixed-role fallback. Unrelated source/identity
service contracts are unchanged.

## Approved logical database schema

Local format provenance uses `managed_store_format(slot, format_version,
domain_ref)`: exactly one row with slot 1, format `sqlrs-managed-store.v1` and
`store_` plus 16 cryptographically random bytes in lowercase hex. The path is
not the domain identifier. `sqlite.EnsureManagedStoreFormat` operates in the
startup caller's transaction: metadata reads, physical inventory, reservation
and all schema installation must commit together. Rolling back a later migration
must also roll back the format marker. Corrupt/unsupported markers and
unversioned lineage tables fail closed; an existing valid format retains its
DomainRef without rerunning empty-legacy checks. `internal/managedstore` now calls
the helper under OS-held store/database locks, installs every local schema in the
same transaction and checks expected DDL on reopen before adapters or Recover.

Both cache owners add `managed_base_lineages`:

- `domain_ref`, `lineage_ref`, `engine_kind`, `image_digest`, `init_spec_digest`,
  `policy_version`, `username`, `identity_digest`, `created_at`.
- Primary key `(domain_ref, lineage_ref)`; unique selector
  `(domain_ref, engine_kind, image_digest, init_spec_digest, policy_version)`.
- Non-empty bounded fields; PostgreSQL name-policy validation; immutable identity
  columns. Insert-on-conflict reads and validates the committed winner.
- Domain uses organization ID in shared, a durable engine-store ID locally.
  Preserve reservations across base eviction; deletion is explicit domain teardown,
  with references checked, never ordinary snapshot garbage collection.

Local SQLite states gain `lineage_ref` and `identity_digest`, with a foreign key
to the store's lineage. Private StateCreate/lookup DTOs carry these fields; public
StateEntry JSON stays unchanged. Write the state and its binding in one transaction.
Queue/job records retain the selected binding so recovery cannot choose again.

Local installation helpers now add these constraints within the cutover
transaction. A unique local lineage index supports the additive SQLite foreign
key; an insert trigger checks the singleton domain and exact identity digest.
State insertion also checks parent lineage and conflicting duplicate state IDs.
Identity columns are immutable; deleting a state leaves lineage reservations.
Jobs persist the resolved image ID, lineage reference and identity digest before
tasks exist; updates cannot substitute another binding. Planner integration now
uses the selected base key and validates recovered job references in tests.
Production startup now injects the owner and access service into prepare/run.
Deletion uses the same access service. Physical base eviction removes its seal
but preserves lineage; rebuilding allocates a new physical operation for that lineage.

Shared `state_snapshots` and `reusable_states` gain `lineage_ref` and
`identity_digest`, scoped by organization and validated against the owning
lineage table. Materializations inherit via snapshot_ref; their journal observations
repeat the digest and physical identity. No separate mutable username columns in
each state. State publication checks the exact physical seal and persisted binding.

Local adds `instance_access` keyed by instance ID, containing lineage/digest,
runtime/physical identity, access version, secret reference and stage. The local
physical operation is stored separately in `managed_runtime_operations` and
joined by physical identity; timestamps/failure codes are not access-table columns.
Instance listing becomes
active only after verified access; internal pending intents are not public active
instances. Shared extends existing access intents and leases with lineage/digest.
Both use stages reserved, applying, verified, retiring, retired, quarantined.
Retirement cannot return to verified.

Local secret records live under an engine-owned protected directory outside any
recipe mount, with owner-only permissions/ACLs. Shared retains its protected volume.
Records bind instance, domain, lineage/digest, exact runtime and access version.
Shared credential format becomes `instance-credential.v2`; access bucket becomes
`instance-access.v2`, leases `runtime-leases.v4`. Missing expected versions fail
closed. Bootstrap secrets remain private and separately referenced by base/runtime;
they are not returned as instance access or embedded in identity metadata.

Store no passwords or password-bearing DSNs in SQLite, PostgreSQL control tables,
job payloads or journals. Secret values necessarily exist in the protected store
and PostgreSQL's own data; filesystem permissions are not encryption at rest.

The local native probe uses pgx/pgconn, pinned to the same version as izess.
`dbms.ManagedConnection` is ephemeral and redacts Go formatting; its fields are
excluded from JSON. A fresh connection must negotiate SCRAM and prove the exact
session/current role with LOGIN and SUPERUSER. Wrong and absent credentials must
produce PostgreSQL password-authentication rejection; a network failure is not
rejection evidence. SQL and physical-replication connections both require SCRAM;
the replication connection must also execute IDENTIFY_SYSTEM successfully.
The local endpoint is an explicit loopback TCP address.
The engine must supply a clean PG* environment: ambient PostgreSQL configuration
fails closed before service/pass files can affect parsing. Prepare now verifies
access before recipes and under runtime exclusion before stopping for capture.
The local access service records physical assignments/seals and fences activation
against retirement. Native acceptance covers interrupted publication and access
after engine restart, with the same physical runtime and credential.

## Atomicity, cache and recovery

Persist lineage before init. Persist secret before the activation intent that
references it, then intent before any external mutation. A crash after secret
creation may leave an orphan: delete it only after proving no intent/runtime
references it, not simply because it is old. Snapshot capture and SQL metadata
commit are not one transaction: incomplete operations remain non-public and
reconcile the same physical ID, lineage and fingerprint.

The base key includes image/init inputs, identity digest and policy version;
descendants include the parent binding. Policy changes deliberately invalidate
old keys. Password changes do not. Planning/cache explanation must use the same
selector and winning reservation as execution; reserving metadata must not start
a DBMS, publish a state or allocate an instance. Document this bounded metadata
side effect in the planning implementation.

On missing/mismatched identity, do not inspect all superusers and choose one.
On altered managed privileges, do not repair. On uncertain cleanup or stale
success, keep access closed. HBA, SQL quoting and native observations all use the
bound name; restore inherited configuration only within existing isolation rules.

## Local recovery limits

Production startup always injects managed identity/access services. Earlier
unbound constructors/runtime methods remain internal test adapters; startup does
not admit legacy stores through them. A pending activation can resume only from
its durable intent, exact job target, running operation and inspected container.
The saved password is retried even if ALTER ROLE completed before the crash.

An engine restart with the same container supports result hydration, run and
delete. Missing/stopped containers, partial base initialization, unknown clone
targets, missing secrets and uncertain ownership fail closed. The local managed
path does not automatically replace a missing container or infer a new access
binding from its data directory. Failed cleanup retains the physical evidence;
it does not remove a directory still possibly mounted by an unconfirmed runtime.

## Approved upgrade conditions

Prefer an explicit empty-runtime cutover over permanent fixed-role compatibility
or SQL role renaming. Preflight inventories both schemas and managed resources
before schema mutation. If old states, instances, jobs, leases or snapshots exist,
stop the upgrade without modifying them; report the need for an operator-approved
archive/reset or a separately designed migration. This also applies to local data.

No startup deletion, automatic namespace hiding or guessed backfill. An operator
may choose a separately provisioned empty local store, retaining the old store
and configuration, but switching stores requires explicit user direction.
Snapshot/metadata/secret backups form one rollback unit. A previous cloud reset
does not authorize another one. Do not run an old binary against new-format stores.

## Contract and approval boundary

Upstream `sqlrs-engine.openapi.yaml` already defines an authorized prepare-result
DSN. No new public fields, status codes, media types or CLI switches are proposed.
Keep internal failures inside existing public error/job envelopes; safe detailed
diagnostics need no raw server-output forwarding. User guides will describe
generated usernames, password-bearing DSNs and readiness failures after this design
is approved, with matching EN/RU architecture and README updates.

The structure, schema, upgrade conditions and test plan are approved. Existing
tests with conflicting fixed-role assumptions may be updated to this contract;
new material deviations still require approval. Real local PostgreSQL parity
precedes cloud acceptance.
