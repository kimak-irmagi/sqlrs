# Liquibase container components: main delivery

This is the bounded #89 view of the previously approved container design,
following the delivery split accepted by @evilguest on 2026-11-06. It does not
introduce a new recovery policy or storage schema.

## Ownership and boundaries

| Component | Responsibility | Owned data |
| --- | --- | --- |
| CLI | Select a Liquibase tag, defaulting to latest; never select a host executable or environment. | User declaration and invocation-local source discovery. |
| prepare/inputbundle | Capture, hash and verify immutable source bytes; materialize read-only container inputs. BundleRef identifies content, not a client or worker path. | Content-addressed manifests/blobs and disposable verified views. The coordinator must retain a durable lease before use. |
| prepare/driverbundle | Select a compatible embedded driver or the checksum-locked release artifact; reject duplicates and unsupported image/Java/database combinations. | Verified driver binding and disposable read-only materialization. |
| prepare/effectiveconfig | Parse admitted project properties, reject connection/classpath overrides, and create private engine-owned connection settings. | Ordinary properties in memory; a temporary protected file, immediately owned by the caller for cleanup. Passwords are not execution identity. |
| runtime | Resolve a fresh trusted tag for the Docker daemon platform, ensure the exact manifest digest, and operate on exact container identities. | Registry/Docker observations; no durable job decisions. |
| prepare and queue | Persist the admitted source and resolved execution before planning; serialize step execution, publication and cleanup. | Job binding, resource ownership, cursor, outcomes and artifact protection. Their integration is not included in the foundation checkpoint. |

The selected repository is liquibase/liquibase. A user tag remains user intent;
resolution produces a platform-specific immutable manifest. Planning, execution
and cache identity must use the same resolved image, verified driver and input
bundle. There is no host-runner fallback in the target delivery. Cache keys are
domain-separated from former host execution.

The runner and managed PostgreSQL container are reused through ordinary state
steps. Each step starts the stock Liquibase CLI for at most one change; it does
not retain a live JVM. Snapshot preparation may stop/restart PostgreSQL inside
the same database container. Private output belongs to the exact runner layer,
not a host capture directory; completion and permission to discard output are
separate facts. A failed observation never authorizes a blind SQL retry.

The [JDBC artifact decision](../adr/2026-10-06-postgresql-jdbc-artifact-delivery.md)
uses the existing external dependency resolver. Build/release verifies the
pinned Maven artifact before placing it under lib/postgresql-42.7.13.jar.
The engine verifies bytes again before use. WSL transfer/installation must carry
and recheck the same JAR; implementing that integration remains pending here.

## Admission contract

Both prepare-shaped API routes use the same strict decoder before admission.
Client executable, mode and environment keys are forbidden by presence, even
when empty/null or case-varied/escaped. Duplicate and unknown fields fail;
plan-only and source metadata do not grant runner authority. The optional
liquibase_version is a tag only for lb, with omission meaning latest. Typed
service calls apply the same ownership and selector rules. This extracts the
already approved admission contract, without enabling replay or new storage.

The public configuration namespace cannot select a runner either. Editor hints
and configuration documentation reject the obsolete executable/mode/environment
settings. The resolved-input fingerprint builder rejects unpinned values and
separates container identity from former host states. Its hash is not permission
to replay; #170 remains a separate delivery.

The complete engine regression passed after admission integration. The strict
HTTP, typed-service and public-configuration controls passed, as did the
resolved-input fingerprint matrix. These results do not prove that production
planning/execution already uses the new binding: that wiring is still pending.

The session coordinator and its record types have also been extracted, without
installing tables or connecting a production worker. Its unchanged tests pass
for three calls/one runner, exact predecessor-disposal dispatch, denial before
execution, and retention after an ambiguous response. Those tests use journal
doubles; they do not qualify SQLite slot ownership or the complete prepare flow.

## Delivery separation

[#89](https://github.com/kimak-irmagi/sqlrs/issues/89) retains the normal container
path, tag/digest/JDBC binding, correct cache results, reuse and normal cleanup.
Advanced restart/replay belongs to [#170](https://github.com/kimak-irmagi/sqlrs/issues/170),
cache/deletion/finalization recovery to [#171](https://github.com/kimak-irmagi/sqlrs/issues/171),
SQLite ownership/migration qualification to [#172](https://github.com/kimak-irmagi/sqlrs/issues/172),
expanded search compatibility to [#173](https://github.com/kimak-irmagi/sqlrs/issues/173),
and broad coverage work to [#174](https://github.com/kimak-irmagi/sqlrs/issues/174).
Shared correctness prerequisites are not waived by this separation.

On 2026-11-07 the [installation boundary](issue89-normal-path-storage-extraction.md)
was approved: retain the exact referenced catalog, install final definitions
directly, keep only the bounded released-baseline bridge, and do not activate
deferred recovery or the common psql executor. NI01–NI08 were separately approved
and the direct installer is implemented. Their Windows and Linux UID 1001 runs
pass, including real COMMIT BUSY and rollback visibility. This establishes the
installation boundary, not production planning/session integration or the 95%
coverage gate.

## Foundation checkpoint, 2026-11-06

The independent input/driver/settings packages, JDBC packaging and runtime
primitives have been extracted. No new database schema is installed by this
checkpoint. The production engine still has its previous host-runner wiring;
the new CLI alone is not a complete or releasable container implementation.

Full client tests passed. The full engine module passed before the last runtime
transfer, and the transferred runtime's unit tests/vet passed separately.
The real moving-registry-tag test passed. Native private-output, initialization,
child and pruning checks passed on Windows and Linux UID 1001. The older
three-JVM fixture still fails before its intended boundaries and awaits the
separately proposed OT-MP02. A subsequent full module run also exposed first-start
deadline sensitivity in TestCaptureAccessBoundedBinaryRunner/binary (one failure,
then four passes in a five-repeat diagnostic); OT-MP03 proposes separating its
ordinary-start and cancellation budgets. The test has not been changed and
the latest full module run is not green.
These are component results, not end-to-end acceptance of #89.

### Approved follow-up: OT-MP02 and OT-MP03

@evilguest approved both limited test changes on 2026-11-06; see the
[fixture decision](../adr/2026-11-06-liquibase-main-path-test-fixtures.md).
The updated three-JVM/incomplete-manifest/surviving-child test passed on
Windows (30.83 s) and Linux UID 1001 (87.19 s). All 11 bounded-process cases
passed three times on Windows and once on Linux, retaining the original
cancellation, quota and cleanup constraints. The subsequent complete engine
module passed. Neither production code nor production timeouts changed.

These results supersede the two open fixture failures recorded at the
foundation checkpoint, not the remaining integration/release gates.

Coverage thresholds, complete current-branch regression, native multi-step
execution/cache/cleanup and the agreed v1 performance comparison remain release
gates. No roadmap item or issue is marked complete by this checkpoint.
