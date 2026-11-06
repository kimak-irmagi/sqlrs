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

## Delivery separation

[#89](https://github.com/kimak-irmagi/sqlrs/issues/89) retains the normal container
path, tag/digest/JDBC binding, correct cache results, reuse and normal cleanup.
Advanced restart/replay belongs to [#170](https://github.com/kimak-irmagi/sqlrs/issues/170),
cache/deletion/finalization recovery to [#171](https://github.com/kimak-irmagi/sqlrs/issues/171),
SQLite ownership/migration qualification to [#172](https://github.com/kimak-irmagi/sqlrs/issues/172),
expanded search compatibility to [#173](https://github.com/kimak-irmagi/sqlrs/issues/173),
and broad coverage work to [#174](https://github.com/kimak-irmagi/sqlrs/issues/174).
Shared correctness prerequisites are not waived by this separation.

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

Coverage thresholds, complete current-branch regression, native multi-step
execution/cache/cleanup and the agreed v1 performance comparison remain release
gates. No roadmap item or issue is marked complete by this checkpoint.
