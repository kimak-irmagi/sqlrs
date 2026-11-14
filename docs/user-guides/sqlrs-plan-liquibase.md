# sqlrs plan (Liquibase)

```text
sqlrs plan:lb [--liquibase-version <tag>] [--image <db-image>] [--provenance-path <path>] [--ref <git-ref>] [--ref-mode worktree|blob] [--ref-keep-worktree] -- <liquibase-args...>
```

The approved target uses the same version selection, admitted immutable inputs,
engine-owned connection settings and digest-bound container as
[prepare:lb](sqlrs-prepare-liquibase.md). Omission selects latest; --image still
selects the database image. No host Liquibase or Java installation is selected.

Planning invokes Liquibase updateSQL against a managed temporary PostgreSQL copy
and returns prospective changesets without applying the preparation chain or
publishing a prepared instance. Prospective state_execute entries are public
plan information, not executable queue tasks of the plan-only job. Only the
actual plan task is queued. See [common plan behavior](sqlrs-plan.md).

The same driver/input/configuration provenance contributes to the plan and later
execution identity. User connection, driver and classpath overrides are rejected;
unsupported dependencies are not silently omitted. Normal cleanup removes the
planning environment and private output without deleting published cache states.
Arbitrary changelog logic can still have external effects; updateSQL is not a
security sandbox.

Container integration and release qualification for #89 remain in progress.
The [component status](../architecture/liquibase-container-component-structure.md)
distinguishes passing component checks from a finished engine. Extended recovery
and search-path compatibility remain separate deliveries, not promises inferred
from this command description.
