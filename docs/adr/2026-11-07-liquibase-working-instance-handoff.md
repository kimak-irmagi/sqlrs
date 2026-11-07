# Publish the existing Liquibase working database as the result

- Conversation date: 2026-11-07, Asia/Novosibirsk; exact reply timestamp unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Status: Component design accepted; the separately recorded schema decision
  below is accepted. Test inventory approval is still pending.

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

## Separate decision: handoff storage

- Conversation date: 2026-11-07, Asia/Novosibirsk; exact reply timestamp unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Status: accepted by explicit reply, "Утвердить схему передачи".
- Question: how to persist the exact transfer without another owner table or
  indefinite retention of a completed instance/state?
- Alternatives: a new transfer table, permanent instance/state foreign keys,
  or extending the existing session-disposal identity with bounded protection.
- Decision: approve the [storage amendment](../architecture/issue89-working-instance-handoff-schema.md):
  `instance_transferred`, six all-or-none identity fields, deferred outcome FK,
  atomic publication/transfer/pin, immutable operation Source, and private-only
  cleanup while the database stays live. Advance the session format to 3;
  helper protocols remain private/v2. Do not migrate the unreleased prototype.
- Rationale: a transfer must never commit alone, but historical references must
  not prevent ordinary deletion after completion. The existing publication pin
  bounds active protection without inventing database-absence evidence.

This amends only the successful working-suffix retirement branch and current
installation target. Earlier installation and private/v2 decisions still apply
elsewhere. Test inventory and exact old-test changes need separate approval;
the schema decision does not authorize them or waive the performance gate.
