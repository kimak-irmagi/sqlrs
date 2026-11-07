# Publish the existing Liquibase working database as the result

- Conversation date: 2026-11-07, Asia/Novosibirsk; exact reply timestamp unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Status: Component design accepted; schema/tests require separate approval.

## Question and alternatives

The normal worker deletes its completed working database and starts another
clone for the final instance, unlike real v1. Options: keep this replacement,
reuse the runtime with only an in-memory flag, or atomically transfer exact
durable runtime/directory/operation ownership with instance publication.

## Decision and rationale

The user approved the narrow third option in
[the component amendment](../architecture/issue89-working-instance-handoff-design.md).
Stop the exact Liquibase parent before activation. Publish the instance, immutable
outcome, publication pin and ownership transfer together; then dispose of only
the private parent/output and projections before terminal completion. The live
instance database and its directory must never be falsely marked absent or
deleted by job cleanup. Unknown effects retain visible ownership, without replay.

This removes an avoidable allocation while preserving crash-safe ownership. No
promise is made that it alone meets the entire performance gate. Pure hits,
preview disposal, #104, image-inventory caching, JVM integration and psql remain
unchanged. The runtime operation keeps its original Source provenance; final
state identity is verified separately.

## Consistency

This narrowly amends the earlier normal-path retirement sequence only for a
successfully completed working suffix. Failure/cancellation disposal still
requires real database absence. Previous component decisions remain applicable
elsewhere, and no general recovery or isolation guarantee is introduced. Exact
storage guards and tests must be approved before implementation or old-test edits.
