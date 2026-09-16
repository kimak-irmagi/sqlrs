# SQLite fixtures for orchestration and HTTP tests

Conversation timestamp: 2026-09-16 14:24 Asia/Novosibirsk (07:24 UTC).
GitHub user ID: @evilguest. Agent: Codex (GPT-6).
Status: accepted; the user requested this optimization within PR #106.

## Question

How can coverage tests finish faster without reducing storage guarantees?
The Windows CI run reached Go's ten-minute package timeout while HTTP tests
were seeding SQLite and prepare tests were still making progress. A configuration
validation test created eleven separate queue databases and took 26.7 seconds.

## Alternatives

1. Increase the timeout only: permits completion but retains repeated disk writes.
2. Use isolated in-memory SQLite for generic orchestration/HTTP fixtures, retaining
   file-backed storage and recovery tests.
3. Disable synchronization or journaling on file-backed fixtures: faster, but
   obscures which tests actually exercise durable storage semantics.
4. Replace SQLite with mocks: removes SQL constraints from these tests.

## Decision and rationale

Choose option 2 for prepare's `newQueueStore` and HTTP's common server/route
fixtures. Each fixture gets a separate SQLite database and runs the same SQL,
schema constraints and assertions. HTTP store and queue share the same single
connection, matching [ADR 0014](0014-shared-sqlite-connection.md). Fixture handles
are closed through test cleanup even when construction fails.

File-backed queue/store migration and reopen tests, managed identity/access
fixtures, secret ACL tests and native PostgreSQL recovery tests stay on real
files. No production durability settings change. This does not supersede
[ADR 0011](0011-task-executor-and-queue.md): production jobs remain persistent.

Re-measure package coverage and CI duration after the change. Cache publication
between the initial lookup and the build lock gets an explicit fixture boundary,
so its test does not depend on slow file writes to exercise the second lookup.
Keep a bounded twenty-minute limit for the Windows slow coverage group to absorb
host variability; it is a ceiling, not an execution delay.
