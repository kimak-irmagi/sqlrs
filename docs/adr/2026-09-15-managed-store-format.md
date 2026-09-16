# Local managed-store provenance in the schema transaction

Conversation timestamp: 2026-09-15 17:46 Asia/Novosibirsk (10:46 UTC).
GitHub user ID: @evilguest. Agent: Codex (GPT-6); exact model build unavailable.

Status: implementation detail of the previously approved durable DomainRef,
format-version and fail-before-mutation cutover conditions; not a completed rollout.

Question: how should local store provenance be committed with the new schema?

Alternatives considered:

- Infer the domain from the filesystem path: moving a store would change identity.
- Write a separate version/domain file: a crash could commit only one side.
- Store a singleton row in the same SQLite transaction as schema installation.

Decision: use `managed_store_format(slot, format_version, domain_ref)`, with slot
1, format `sqlrs-managed-store.v1`, and a random `store_` identifier. The startup
caller owns the transaction covering metadata checks, physical inventory, format
reservation and complete schema installation. Existing valid format records are
read without an empty-legacy check; corruption and unsupported versions are errors.
No marker or schema is installed by normal startup in this implementation checkpoint.

Rationale: preserve a stable domain through moves/restarts and prevent a failed
migration from leaving a committed marker that permits later checks to be bypassed.
The transaction does not replace admission exclusion or physical inventory.

See [components and persistence](../architecture/managed-database-identity-internals.md).
