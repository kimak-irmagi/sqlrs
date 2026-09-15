# Managed database identity: test plan

Status: test plan and proposed existing-test resolutions approved, 2026-09-14, @evilguest. The
[component/schema design](managed-database-identity-internals.md) is approved;
the user authorized writing these tests and updating conflicting expectations.
Both repositories carry the same acceptance requirements, with their own harnesses.

Implementation checkpoint: both initial identity modules passed at 100%, confirmed
by the user. The metadata coordinator now passes in both repositories at 97.1%.
Tests prove winner selection, no regeneration after failed/corrupt reads, scope
checks, cancellation, and bounded diagnostics. Test execution outside the sandbox
with the normal cache now works; cache paths were not overridden.

The local SQLite adapter also passes real temporary-database reservation, reopen,
corruption and failure tests. Full SQLite regression passes at 95.8%; all new
lineage.go blocks are covered. Its new schema is deliberately not installed by
normal startup before the upgrade preflight exists. PostgreSQL persistence,
runtime wiring and deployment remain unimplemented; this is not end-to-end acceptance.

Update 2026-09-15: the user approved both cancellation cases. Deterministic tests
now cancel after the missing lookup (no entropy read or write) and after generation
(no write); both return cancellation without a partial identity. Both complete
coordinator suites pass at 100% statement coverage with no uncovered blocks.
Production coordinator code was unchanged in this iteration.

The shared PostgreSQL adapter now implements validated Find/Reserve/Get and bounded
errors, using the existing state-store pool. Reservation uses INSERT ON CONFLICT
DO NOTHING followed by a separate read of the committed winner. Its complete
package suite passes at 97.1%, with every new lineage.go block covered; go vet passes.
These adapter tests use a fake driver boundary, not a real PostgreSQL concurrency
proof. That proof, PostgreSQL schema installation/preflight and runtime wiring are
still pending. A Docker read-only availability check was denied by the sandbox;
this does not establish that Docker itself is unavailable. No deployments changed.

### Local checkpoint, 2026-09-15: read-only metadata preflight

`sqlite.CheckEmptyManagedMetadata` and its tests now exist. The focused suite
passed after first failing on the missing API. Each of states, instances, names,
prepare_jobs, prepare_tasks, prepare_events and instance_access independently
blocks cutover when populated. Tests compare database bytes before/after and use
SQLite query-only mode; empty databases/tables and unrelated settings are preserved.
Views/indexes under an expected table name fail closed, SQLite name casing cannot
hide rows, nil/closed databases produce bounded errors, and cancellation is retained.
Fixtures open `database/sql` directly, without Store.New migrations.

The user approved the additional corruption/cancellation coverage plan. Tests now
corrupt real catalog/table b-tree pages, cancel a blocked connection acquisition,
and verify bounded wrapped cancellation. All preflight and lineage blocks are
covered. SQLite also has a transaction-owned format/DomainRef reservation helper
with rollback, reopen, unsupported/corrupt marker and uncertain-inventory tests.
The complete SQLite suite passes at 96.2%; the new format helper is at 97.1%.
The managedidentity suite, including RuntimeBinding and BaseKey, passes at 100%.

Physical inventory is implemented separately in the Docker adapter: platform
resource directories and all container mounts, including stopped containers.
Unrelated files survive; unknown Docker inventory fails closed. The runtime suite
passes at 96.2%. The approved additional tests cover tmpfs/unknown mounts, opaque
WSL paths, invalid directories and cancellation. Review found and fixed accidental
case folding of Linux `/mnt/Uppercase` paths; drive-letter paths still normalize.

Native pgx verification passed on a disposable official PostgreSQL 17 container,
resolved to `postgres@sha256:7352e0c4d62bbac8aa69d95e40220a60967c4a19f9c4f65b4d118175f7ce9e3b`.
It rejects trust-only access, proves SCRAM plus wrong/absent-password rejection,
allows application postgres/sqlrs role lifecycles, and rejects reachable loss of
managed SUPERUSER privileges without repairing the role. The secret canary was
absent from container logs. The dbms suite with this integration test passes at
96.2%. The approved additional tests prove that trust and transport failure never
count as password rejection, cancellation is preserved, and the negative test
credential always differs from the actual password. Remaining uncovered defensive
branches are recorded in the per-line profiles; another iteration was requested.

State and queue binding persistence now passes real SQLite tests: parent lineage,
immutable identity, duplicate-ID conflicts, foreign-key protection, rollback and
job reopen before task creation. Public state JSON is unchanged. SQLite coverage
is 96.1% (state installation helper: 100%); queue coverage is 96.7%.
The engine regression passed with `go test -p 1 ./... -timeout 2m`. The initial
parallel run failed one HTTP job-completion test with SQLITE_BUSY; its focused
rerun and full sequential rerun passed. The initial failure remains recorded.

Subsequent review regressions reject a mismatched state image, malformed job image
suffixes and silent fingerprint collisions. A real PostgreSQL regression first
demonstrated that SQL-only proof accepted a physical-replication trust bypass.
Native proof now also authenticates a replication connection, executes
IDENTIFY_SYSTEM and rejects wrong/absent replication credentials. The dbms suite
with the live integration test passes at 95.7%; IPv6 publication remains to verify.

Environment restrictions are cleared: Go tests, Docker and GitHub are accessible.
These helpers are not wired into startup or prepare/run. Complete schema
installation, instanceaccess, secret storage, recovery, sealing and publication
remain pending. Native adapter acceptance is not Sakila/Chinook/CLI acceptance;
no PR, CI success, cross-project acceptance or merge is claimed.

## 1. Identity, scope and durable reservation

- Valid generated PostgreSQL name, exact 128-bit source consumption and no fixed-name
  fallback. Inject entropy failure and require no accepted reservation or init.
  Do not use probabilistic uniqueness sampling as evidence of security.
- Same selector across repeated, concurrent and restarted requests returns one
  committed identity. Prove this against SQLite and PostgreSQL stores, not only mocks.
- Different organization/local-store, image, effective initialization settings or
  policy cannot accidentally retrieve the other selector's reservation.
- Reject malformed names, changed username under the same lineage, foreign scope,
  corrupt/missing identity and digest mismatch before runtime mutation.
- Physical base eviction preserves identity; recreating the base and restoring
  a descendant do not generate a new name or require the old base snapshot.

## 2. Cache, planning and job recovery

- Repeated plan/prepare use the same identity-bound keys and report a real cache hit.
  Changed lineage/policy cannot reuse the old key; password changes do not affect it.
- First plan may reserve lineage metadata, but creates no DBMS, job, state, instance
  or instance secret. Subsequent plan and prepare reuse that reservation.
- Parent/child binding survives serialization, job recovery and process restart;
  conflicting recovered bindings fail without recomputation under another policy.
- Include Java sql-runner hashing/catalog lookup and local prepare hashing, not
  just the new identity utility. Image resolution and initialization inputs must
  reach the same selector used by execution.

## 3. Adapter and private protocol

- Thread a non-default generated identity through every initialization path,
  readiness, psql, Liquibase, connection construction, HBA and native verification.
  SQL username must be independent of Linux user and database name.
- Required identity fields, bounds, digest/fingerprint and organization/owner
  bindings are checked by actual private handlers. Unsupported versions fail
  before mutation; no fallback to postgres/sqlrs or a prefix-matched role.
- Materialization inherits the owner snapshot's identity and rejects a mismatched
  caller expectation. Wrong-role evidence remains rejected even when privileged.
- Public JSON schemas and CLI syntax remain unchanged; authorized DSNs contain the
  effective login, while inventory, jobs and diagnostics contain no secret.

## 4. Real PostgreSQL recipe parity

Run the official postgres:17 image resolved to a digest in both profiles:

- Unmodified Sakila prepare plus its existing query fixture/golden output.
  Verify imported objects/ownership, not only exit status; repeat with a cache hit.
- Create/use/drop ordinary application roles postgres and sqlrs without damaging
  managed access. Preserve application grants, memberships and intentional passwords.
- Preserve Chinook reconnect and a representative Liquibase path.
- A script may end under an application role; independent managed-access validation
  must not mistake that session's current_user for the managed identity.
- A reachable recipe-induced loss of administrative access prevents state and
  instance publication. Test metadata-missing/renamed-role handling separately at
  the adapter boundary when PostgreSQL itself forbids the attempted SQL mutation.
  Do not require impossible bootstrap-role deletion just to exercise a branch.
- Prove verification/stop/capture serialization and reject a stale seal or a seal
  for a different physical runtime; SQL exit zero alone cannot publish a snapshot.

## 5. Instance credentials, HBA and preservation

- Two clones of one state share its managed identity but receive different working
  passwords. Each rejects the other's password and missing/wrong passwords.
- Verify fresh connections through the actual managed client path using the native
  driver, administrative capability, IPv4/IPv6 and applicable replication policy.
- Inherited trust cannot bypass the managed role's SCRAM rules. Rules for application
  roles and recipe configuration remain preserved within the approved policy.
- Activation and restart do not mutate the canonical snapshot or replace an active
  password with bootstrap credentials. Retry uses the same access version.
- Use secret canaries to check platform/container logs, error responses, argv,
  metadata, job payloads and journals. Do not call PostgreSQL's own catalogs/WAL
  or the protected secret store a forbidden diagnostic sink.
- Local file/ACL confinement and shared read-only secret delivery reject wrong
  bindings, traversal and symlink escape. Keep prior permission/CHOWN regressions.

## 6. Lifecycle, failures and storage

- Inject failure before/after reservation, secret publication, activation intent,
  physical mutation, seal, capture and state publication. Reopen real stores and
  prove no duplicate identity/secret version, premature publication or lost owner.
- A secret orphan is removed only after proving absence of durable/runtime owners.
  A referenced secret survives delayed activation and restart.
- Activation versus deletion, late success, exact-runtime reuse and concurrent
  retries preserve retirement and existing route-generation fencing.
- Transaction constraints prevent partial state/binding publication; corrupt or
  unsupported versions fail unchanged. Real filesystem/network/persistence failures
  are valid scenarios; arbitrary impossible mock states are not coverage targets.

## 7. Upgrade preflight

- Empty-runtime cutover admits the new schema/protocol generation.
- Independently seed each kind of old resource (state, instance, job, lease,
  snapshot/journal) and prove preflight stops before schema/data mutation.
- Preserve unrelated sources, users, organizations, configuration and secrets.
  No guessed backfill, automatic archive, cleanup or hidden legacy namespace.
- Verify rollback's matching metadata/snapshot/secret set in a disposable fixture;
  never use real user data or execute a cloud reset as a unit-test step.

## 8. Execution and acceptance gates

Start with focused new tests that fail for the documented missing behavior, then
implement. Repeat review after fixes. Run local unit/contract suites and real
PostgreSQL parity before publishing/deploying. Extend existing CI jobs/harnesses
rather than adding redundant full pipelines; retain existing regression suites.

Local release already includes hp-psql-sakila. Shared cloud smoke currently uses
SELECT 1/metacommand/cache: add meaningful Sakila coverage to candidate acceptance,
without removing the existing smoke/fencing checks. Team and cloud verification
must exercise the candidate build and verify prepare/run/cache/delete results.
Environment-unavailable skips are not successful real-engine acceptance.

Obtain per-line coverage after implementation: target 100%, minimum 95% per
affected package/module where measured, without combining unrelated code to mask
gaps. Review files with most uncovered lines against requirements, identify dead
or unreachable scenarios, and request approval for any additional coverage plan.
Do not remove safeguards or alter tests merely to increase a percentage.

## Preliminary existing-test conflict review

This is an initial inventory; repeat the review after test-plan approval and
before modifying tests. Proposed resolution: update old assumptions to the new
contract, not weaken the approved design.

| Existing evidence | Proposed treatment |
| --- | --- |
| taidon runtime/docker_test.go: TestDockerRuntimeInitBaseSuccess requires --username=sqlrs | Assert the supplied identity and that OS postgres remains independent |
| Same file: TestEnsureHostAuthAddsNewlineBeforeAppendedEntries requires host all all ... trust | Replace managed-access policy expectations with role-specific SCRAM, preserve newline/idempotence and inherited application-rule tests |
| Same file: missing/permission-denied HBA helpers may return success | Such best-effort behavior cannot certify published access; retain helper semantics only if used outside the publication gate, require activation failure without proof |
| taidon prepare/ensure_base_state_test.go: marker/PG_VERSION-only reuse and reset of nonempty directory | Require matching managed metadata and exact resumable target; add unchanged-data rejection for legacy/unowned directories, retain initialization concurrency/error coverage |
| shared execution/access_activation* tests and noderuntime/instance_access_test.go use Username postgres | Bind fixtures to generated identities; preserve wrong-username, ownership, timeout and recovery assertions |
| shared docker/instance_access_integration_test.go initializes SQL role postgres | Parameterize the managed SQL identity; retain OS postgres, database postgres, real authentication and no-leak proof |
| Existing schema-version/hash expectations and Runtime fakes | Update only where the approved signature/schema/key changes require it; preserve corruption, migration rejection and lifecycle assertions |

The two roles of the word postgres must not be mechanically replaced everywhere.
A generic adapter proof with an explicit postgres fixture is not automatically a
product-contract defect; update the fixtures that are meant to exercise the new
managed policy. No existing test is changed by this document.
