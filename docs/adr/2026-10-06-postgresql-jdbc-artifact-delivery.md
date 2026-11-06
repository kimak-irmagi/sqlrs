# PostgreSQL JDBC artifact delivery for container Liquibase

- Conversation timestamp: 2026-10-06T16:40:51+07:00
- GitHub user id: @evilguest
- Agent name/version: Codex / GPT-5
- Status: Accepted

## Question discussed

How should the engine obtain and ship the verified PostgreSQL JDBC driver needed
by Liquibase images that do not contain one?

## Alternatives considered

1. Commit the JAR binary to the repository.
2. Download the JAR while executing a Liquibase job.
3. Fetch a fixed Maven Central artifact through the project's existing locked
   external-artifact manifest, verify its digest, and include it in engine
   release bundles.
4. Build and publish a custom Liquibase image containing the driver.

## Decision

Use option 3. The engine's build/release process resolves
`org.postgresql:postgresql:42.7.13` from Maven Central through
`scripts/external/manifest.yaml` and verifies its pinned SHA-256 before
packaging. The initial artifact hash is
`6e0e4cc2d8cae902084f8a2b18728b073a6fd9d1f87c9d8bff8f298c18185b93`.
The released engine verifies the packaged JAR again before mounting it into a
Liquibase container. Job execution never downloads or updates the JAR. The
driver artifact digest is part of the immutable execution binding; old
artifacts remain available while referenced by recoverable jobs.

The Windows CLI transfers the verified release JAR with the Linux engine into
WSL, checks its SHA-256 there, and validates the installed copy on subsequent
startup. A separately supplied Linux engine must carry the same adjacent
verified bundle; a missing or changed JAR is not silently replaced at runtime.

The engine must still inspect/select a supported image/Java/driver combination
and reject an image with a conflicting embedded driver; merely having this
artifact does not make every Liquibase tag supported. The trusted repository
remains `liquibase/liquibase`, not a derived image.

## Rationale

This reuses the repository's existing checksum-locked external-file mechanism
without requiring Maven or Gradle in the Go engine, keeps releases usable
offline, and prevents a floating dependency from changing during a job.

## Contradiction check

This fills in the previously deferred driver-packaging detail in
[container execution](../architecture/liquibase-container-component-structure.md). It does
not supersede that decision. No earlier ADR requires a different JAR source.
