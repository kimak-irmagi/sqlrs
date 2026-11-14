# Separate normal Liquibase installation from prototype migrations

- Conversation date: 2026-11-07, Asia/Novosibirsk; exact reply timestamp unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Status: Accepted for the main #89 delivery; tests require separate approval.

## Question

How can the normal private/v2 Liquibase journal be installed without importing
the unreleased prototype's entire migration and recovery chain from #170–#172?

## Alternatives

1. Transfer the composed opener, historical upgrades and recovery infrastructure.
2. Replace the approved journal with a smaller schema or in-memory/request-JSON state.
3. Keep the exact approved physical definitions and normal transitions, but
   separate direct installation/verification from historical migration code.

## Decision

Choose option 3, as specified in the approved
[component amendment](../architecture/issue89-normal-path-storage-extraction.md).
Mechanically retain the final SQL definitions and immutable expected-object
catalog, including every dependency of retained guards. Validate actual stores
on every open; never repair a partial/altered/unknown namespace implicitly.

Preserve the bounded conversion from the exact released prepare_jobs baseline,
including all existing values and foreign-key checks. Fresh stores create the
approved target directly. Unsupported unfinished old Liquibase work is rejected
before new DDL or inventory. No compatibility for unfinished old jobs is added.

Only normal admission, binding, planning, execution, publication, bounded
capture reuse, ordinary error/cancellation and cleanup are connected. Automatic
replay, cache-handoff recovery and historical prototype upgrades stay deferred.
Do not switch psql to the common-attempt executor. Exact-connection COMMIT
correctness remains a prerequisite, not a deferrable qualification detail.

## Rationale and consequences

The normal coordinator needs durable ownership; test doubles or memory cannot
replace it. Retaining the complete referenced catalog preserves the approved
guards without importing every historical migration or enabling recovery.
This is not a promise of a particular dependency size or readiness to merge.
New tests and changes to existing tests still follow AGENTS.md approval gates.

## Consistency review

ADR 0003 remains valid: executable SQL has one embedded canonical source,
and documentation must not become a hand-maintained DDL duplicate. The expected
catalog is a mechanically derived verification artifact, not a second authored
schema. The Runtime v2 persistence decision of 2026-09-27 remains unchanged:
its namespace, installer and semantic revisions are not replaced by this
Liquibase-only boundary. No earlier ADR in the delivery branch mandates the
unreleased prototype's historical Liquibase migration chain; none is made
obsolete by this decision.
