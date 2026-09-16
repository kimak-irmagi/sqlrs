# Scope of metadata-only managed identity reservation

Conversation timestamp: 2026-09-15 18:03 Asia/Novosibirsk (11:03 UTC).
GitHub user ID: @evilguest. Agent: Codex (GPT-6).
Status: clarification of accepted Decision 5; continued implementation requested
2026-09-16 by @evilguest ("закончи PR"). Existing planning behavior is retained.

## Question

Does the prohibition on runtime allocation apply to identity reservation itself,
or to the complete existing Liquibase plan operation?

Decision 5 in [the original ADR](2026-09-14-managed-database-identity.md) says no
runtime/job/state/instance is allocated **by that reservation**. The test-plan
wording instead applies this to the first plan. Existing `prepare.Submit` persists
a job for plan-only requests; `planLiquibaseChangesets` starts PostgreSQL to obtain
`updateSQL`, including during cache explanation. These are separate operations
from lineage reservation.

## Alternatives

1. Preserve existing plan semantics. Identity reservation is metadata-only;
   Liquibase may use a private temporary managed PostgreSQL runtime with protected
   bootstrap access. It publishes no user instance or instance credential.
   Existing plan-only jobs remain visible under the current API.
2. Require the entire plan to create no DBMS/job. This needs a separate planning
   change, including replacement of the current queued plan-only API semantics.
   Offline Liquibase is not equivalent: it does not support preconditions.
   See [Liquibase offline documentation](https://docs.liquibase.com/community/user-guide-5-0-2/how-do-i-manage-an-offline-database).

## Decision and rationale

Choose option 1 and clarify the test plan. It matches the original reservation
decision, preserves existing Liquibase semantics and avoids a public API redesign
inside the managed-identity fix. Add separate assertions that reservation creates
no runtime/job/secret and that Liquibase planning never publishes an instance or
instance password; temporary managed runtime cleanup remains required.

This clarifies reservation scope and does not supersede the original ADR.
