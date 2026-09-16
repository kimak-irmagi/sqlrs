# Managed database identity and local/shared parity

Conversation date: 2026-09-14, Asia/Novosibirsk; exact message time unavailable.
GitHub user ID: @evilguest. Agent: Codex (GPT-5); exact model build unavailable.

## Decision 1: generated managed administrator

Status: accepted direction.
Question: use an application administrator, a fixed name, or a generated identity?
Alternatives: no separate managed role; fixed sqlrs/postgres; generated name;
mandatory restrictions on administrative SQL.
Chosen: adapter-managed PostgreSQL identity named sqlrs_admin_ plus 128 random
bits in lowercase hexadecimal, stable across a base cluster's states and clones.
Published instances have independent passwords. Do not silently repair damaged
roles or privileges. Rationale: minimize accidental application-role collisions
without claiming protection against superuser SQL or breaking Sakila.

## Decision 2: coordinated profiles

Status: accepted direction.
Question: rewrite Sakila, fix shared alone, or align both profiles?
Chosen: coordinated taidon/izess PRs, preserving application scripts and applying
the same identity and authenticated instance-access guarantees. No shared
microservice deployment is required locally.
Rationale: local examples must exercise shared database-role assumptions.
The historical passwordless-host assumption in ADR 0012 is not the new target.

## Decision 3: persistent lineage and cache binding

Status: common flow/cache policy approved in the user's follow-up.
Question: regenerate identities, omit them from keys, derive them from image IDs,
or persist the base lineage and include its identity in keys?
Chosen: persistent lineage, retained across physical base eviction; child keys
inherit the parent's binding. Passwords/verifiers are not cache-key inputs.
Rationale: preserve cache reuse without equating different SQL-visible inputs.
See the [common flow](../architecture/managed-database-identity.md).
Exact interfaces/schema, planning metadata reservation and upgrade conditions were
subsequently approved in Decisions 4–6 and the
[internals](../architecture/managed-database-identity-internals.md). The test plan
and implementation are also approved, reconfirmed by @evilguest on 2026-09-15.
No local data deletion or legacy-data conversion is authorized.

## Decision 4: component ownership and persistent identity binding

Conversation date: 2026-09-14, Asia/Novosibirsk; exact message time unavailable.
GitHub user ID: @evilguest. Agent: Codex (GPT-5); exact model build unavailable.
Status: approved in the user's follow-up to the component/schema proposal.
Question: where should lineage identity and instance credentials be owned?
Alternatives: infer identity from database contents; duplicate mutable usernames;
give the cache owner durable lineage and the access subsystem instance secrets.
Chosen: the last alternative, with the types, schema fields, private protocol
versions and recovery rules in
[components and persistence](../architecture/managed-database-identity-internals.md).
Local modules implement the same responsibilities without new microservices.
Rationale: preserve one authority per lifecycle and bind every physical operation
to the expected identity. This approves the design, not completed implementation.

## Decision 5: metadata reservation during planning

Conversation date: 2026-09-14, Asia/Novosibirsk; exact message time unavailable.
GitHub user ID: @evilguest. Agent: Codex (GPT-5); exact model build unavailable.
Status: approved with the explicit planning side effect presented to the user.
Question: how can plan and prepare select identical identity-aware keys?
Alternatives: an ephemeral random plan identity; omit identity; reserve the durable
lineage without creating runtime resources.
Chosen: metadata-only reservation, reused by later plan/prepare. No DBMS, job,
state, instance or instance password is allocated by that reservation.
Rationale: stable predicted keys without executing the preparation.

## Decision 6: fail-before-mutation upgrade preflight

Conversation date: 2026-09-14, Asia/Novosibirsk; exact message time unavailable.
GitHub user ID: @evilguest. Agent: Codex (GPT-5); exact model build unavailable.
Status: upgrade conditions approved; no concrete cleanup operation authorized.
Question: convert fixed-role data, delete automatically, or require an explicit cutover?
Chosen: preflight blocks schema change if old runtime data/resources remain;
operator-approved disposition or a separately designed migration is required.
Alternatives: permanent fixed-role compatibility, guessed role renaming/backfill,
automatic reset. Rationale: avoid silent data loss and lifecycle ambiguity.
The test plan is subsequently approved; any actual archive/reset remains a
separate approval gate.
