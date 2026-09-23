# Managed database identity: local/shared contract

Status: direction, interaction/cache flow, component interfaces, storage schema,
upgrade conditions and test plan approved by @evilguest, 2026-09-14, reconfirmed
2026-09-15. The local implementation passes live PostgreSQL 17 acceptance; see
the [test report](managed-database-identity-tests.md) for evidence and recovery limits.
This remains the shared target contract; izess acceptance is tracked separately.

Local sqlrs and shared izess must implement one managed-identity policy. The
current local SQL administrator `sqlrs` and shared administrator `postgres` are
implementation history, not application-role reservations. Keep Sakila unchanged.

## Identity and flow

1. Resolve the immutable image, initialization settings and authorized cache
   domain. Atomically reserve or retrieve its durable base lineage before state
   key computation and database initialization.
2. Generate the PostgreSQL managed name once: `sqlrs_admin_` plus 128 random bits
   in lowercase hexadecimal. Persist it before initdb. Concurrent requests and
   recovery reuse the winning reservation.
3. Initialize and verify the base; derived snapshots and writable clones inherit
   its identity through trusted metadata, not recipe-controlled PGDATA files.
4. Execute prepare with that default SQL login. Application-role switches remain
   valid. Independently verify the original administrative identity before the
   coordinated stop/capture/publication boundary.
5. Apply a strong instance-specific password on the writable clone using protected
   adapter maintenance. Verify fresh administrative authentication and rejection
   of missing/wrong passwords before publishing access. Preserve recipe accounts.
6. Retry/restart reuse the same instance secret version. Do not repair missing
   managed roles or lost privileges silently. Retire access before secret cleanup.

Role names are not secrets or platform ownership identities. Randomization
minimizes accidental collisions but cannot protect a role from superuser SQL.
Linux process user `postgres` is separate and need not be renamed.

## Cache and compatibility

The managed name is observable in SQL. Include lineage identity and policy version
in the base key; descendants inherit that binding through the parent. Keep the
small lineage reservation through physical base eviction, so rebuilding the base
reuses the name. A new lineage must not reuse old keys. Passwords/verifiers are
never key inputs. Do not widen the current cache authorization domain.

Local and shared share semantics, not necessarily names or state IDs. Local parity
includes authenticated published instances instead of the current passwordless
access. It does not require deploying shared microservices locally.

This statement describes the current managed-identity/cache integration. The
standalone Runtime v2 core is engine-neutral and deterministic for identical
resolved factory identities. Issues #124 and #108 provide the typed extension,
composition, and resolution primitives, but do not map managed identities into
that model. A managed-database adapter remains later integration work.

No new CLI option or public identity field is proposed. The existing authorized
DSN contains the actual username. Use the approved error envelope; introducing
public error codes requires separate OpenAPI approval.

Old states must not be guessed into the new identity policy or silently reset.
The approved upgrade requires preflight before mutation; prior cloud debug-data deletion does not
authorize deletion of local data. Review runtime, prepare, Liquibase, DBMS helpers,
state storage and connection construction as consumers of the same identity.

See [approved components and persistence](managed-database-identity-internals.md).
