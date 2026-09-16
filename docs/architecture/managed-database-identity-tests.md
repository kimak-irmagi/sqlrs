# Managed database identity: test plan

Status: test plan and proposed existing-test resolutions approved, 2026-09-14, @evilguest. The
[component/schema design](managed-database-identity-internals.md) is approved;
the user authorized writing these tests and updating conflicting expectations.
Both repositories carry the same acceptance requirements, with their own harnesses.

### Local acceptance, 2026-09-16

Image-alias review fix: regression tests reproduce the conflict before the fix
and pass afterward. They cover short/qualified repository references, a renamed
repository with the same digest, the stored snapshot location, bare-digest job
restoration, different-digest rejection and a missing capture seal. After removing
the sandbox restrictions, the full Windows prepare suite and full Linux suite
with native PostgreSQL pass, including Sakila, Chinook, Liquibase and recovery.
Fresh combined coverage is 3778/3966 statements (95.260%); `managedImageDigest`,
`bindManagedRequest`, `isManagedStateCached` and `loadManagedState` have 100%.
Profiles, HTML and uncovered-block reports use `coverage/image-alias-*`.
The earlier restricted run's secret-fixture failures also reproduced on unchanged
HEAD and are resolved by the unrestricted run; its partial profile is not merged.

Review regression plan (2026-09-16; approved by the user):

- Cancel a job owning a managed runtime; assert bounded, uncancelled stop,
  operation retirement and clone cleanup. Preserve uncertain-stop retry coverage.
- Fail capture before state publication, retire the owner and retry with a new
  physical operation for the same state key. Require successful resealing; reject
  replacement of published seals, active owners or another identity. Reopen the
  access service to prove that eligibility is durable.
- Hold an authorized command open on instance A while accessing, activating and
  retiring B and reading physical metadata. B must progress independently.
- Retirement of A waits for its command; cancellation releases a waiting caller
  without running its callback. Retired access remains unavailable.

Existing physical-capture conflict and uncertain-stop tests retain their current
assertions: active/published seals remain immutable and unconfirmed live clones
must never be erased.

Review regression results: all three defects reproduced before the fixes and
passed afterward. Full Windows `instanceaccess`/`prepare` suites and run/deletion
regressions pass; concurrency tests passed 20 consecutive repetitions. The full
Linux prepare suite, including native PostgreSQL and a failed-capture/retry/run
scenario, passes. Its coverage combines with the current Windows profile to
cover 3773/3963 statements (95.206%). Initial cross-compiled harness failures were
resolved with an absolute executable path, a subprocess `GOCOVERDIR`, and removal
of inherited WSL path-mapping flags; no test assertions were weakened or skipped.
Windows `-race` could not build with the installed CGo toolchain; repetitions are
not claimed as a race-detector run. Fresh profiles, HTML and uncovered-block
reports use the `coverage/review-*` prefix. The remaining fence gap is cancellation
concurrent with acquiring an available gate; its defensive recheck is retained.

The user also approved accelerating coverage fixtures in this PR. Generic prepare
queue and HTTP server/route tests use isolated in-memory SQLite with the same
schema and assertions. Persistence, migration, managed access, ACL and native
recovery fixtures remain file-backed. See the [fixture ADR](../adr/2026-09-16-memory-sqlite-test-fixtures.md).
Cache publication before the build lock has an explicit test boundary, without
depending on disk latency to reach the second cache lookup.

The local production path is wired through startup, planning, prepare, run and
deletion. Guarded transactional cutover, immutable lineage/state/job bindings,
protected bootstrap/instance secrets, native SCRAM proof and retirement fences
are implemented. [PR #106](https://github.com/kimak-irmagi/sqlrs/pull/106) contains
the implementation and separate review-fix commits. Shared izess acceptance must
be confirmed by its separate session before merge; local results do not prove it.

The live suite uses official PostgreSQL 17.7,
`postgres@sha256:7352e0c4d62bbac8aa69d95e40220a60967c4a19f9c4f65b4d118175f7ce9e3b`.
Sakila (1000 films), Chinook (3503 tracks) and Liquibase (12 changesets) passed,
including repeated cache reuse and distinct instance passwords. It also proves
interrupted publication resumes with the same runtime/secret, run and deletion
after engine restart, and physical base eviction/rebuild preserves lineage.
Application postgres/sqlrs role lifecycles remain unchanged. Damaged managed
privileges fail before publication. SQL and replication SCRAM, wrong/absent
credentials, IPv4/IPv6 loopback and cancellation during password activation pass
through native pgx against PostgreSQL. Passwords are absent from control metadata
and the checked container logs.

Per-package statement coverage with normal Go caches:

| Package | Coverage | Evidence |
| --- | ---: | --- |
| managedidentity | 100% | Reservation, restoration, scope and cancellation |
| store/sqlite | 96.1% | Real SQLite preflight, corruption, immutable bindings |
| prepare/queue | 96.7% | Durable job binding and reopen |
| managedstore | 98.7% | Atomic cutover, rollback, cancellation, schema refusal |
| instanceaccess | 95.6% | Protected files/ACLs, intents, recovery, retirement, per-instance fences |
| runtime | 95.1% | Inventory, protected mounts, exact runtime ownership |
| dbms | 95.8% | Unit and native PostgreSQL integration |
| prepare | 95.3% | Current Windows unit, Linux filesystem and live PostgreSQL suites, including the image-alias fix |
| run | 98.4% | Access and runtime binding fences |
| deletion | 98.4% | Retirement and uncertain cleanup |
| cmd/sqlrs-engine | 95.1% | Startup refusal and managed dependency wiring |

Profiles and per-line HTML reports are local artifacts under
`backend/local-engine-go/coverage/`. The prepare profile combines successful
runs of identical production sources; test-only StateFS/psql helpers were moved
to `_test.go`. Platform-specific secret code is measured separately, without
merging incompatible Windows/Unix source ranges. Native integration tests require
`-tags managedintegration`; normal cross-platform CI runs the unit suites.

The approved coverage iterations covered failure/publication paths first, then
startup/schema/filesystem failures and real cancellation boundaries. Remaining
uncovered lines include defensive driver/scan/close errors with fixed SQL, the
registered SQLite driver's fixed in-memory open, and cancellation between narrow
stages. They remain requirements-backed fail-closed checks: no synthetic SQL
results were added merely to exercise them. The 95% minimum is met; 100% is not
claimed. Partial initialization, lost runtimes and uncertain cleanup remain
closed to access and preserve evidence; automatic runtime replacement is outside
the implemented recovery path. See [recovery limits](managed-database-identity-internals.md#local-recovery-limits).

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
