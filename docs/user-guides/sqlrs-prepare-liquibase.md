# sqlrs prepare (Liquibase)

The approved #89 target runs Liquibase in an engine-owned container in both
local and remote profiles. Container service integration is still in progress;
CLI/admission/component tests alone do not establish end-to-end availability.
See the [approved CLI contract](sqlrs-liquibase-container-cli-design.md) and
[component status](../architecture/liquibase-container-component-structure.md).

## Syntax

```text
sqlrs prepare:lb [--liquibase-version <tag>] [--image <db-image>] [--provenance-path <path>] [--ref <git-ref>] [--ref-mode worktree|blob] [--ref-keep-worktree] [--watch|--no-watch] -- <liquibase-args...>
```

- `--image` selects the PostgreSQL base image, not the Liquibase image.
- `--liquibase-version` selects a tag in liquibase/liquibase; omission means
  latest. It is user intent, not a reproducible fingerprint. The engine resolves
  it to a platform-specific immutable manifest before planning or cache lookup.
- Provenance, Git references and waiting retain their normal meaning; see
  [Git references](sqlrs-ref.md). Ref-backed preparation remains watch-only.
- An update-family command is required after `--`. Connection, driver and
  classpath overrides are rejected.

```bash
sqlrs prepare:lb --liquibase-version 5.0.4 --image postgres:17 -- update \
  --changelog-file examples/liquibase/jhipster-sample-app/config/liquibase/master.xml
```

## Execution and cache contract

Planning uses Liquibase updateSQL against a managed PostgreSQL copy. Preparation
then uses update-count --count=1 and publishes a snapshot after each changeset.
Ordinary working steps reuse the same Liquibase and database containers; JVM
startup still occurs for each CLI invocation. Planning has its own lifetime.
PostgreSQL may stop/restart inside its container while a snapshot is prepared.

Exact cache hits and cached prefixes must retain the same database contents as
execution. Cache identity includes the resolved runner image, platform, verified
JDBC driver, admitted source contents, effective non-secret settings and managed
database identity, as well as the changeset chain. The old host-runner identity
is not reusable for container execution. A mutable tag is resolved afresh for
an independent job but cannot change within a bound job.

Inputs are captured and verified before use, then mounted read-only. The initial
supported input collector must reject dependencies it cannot bind completely.
The engine supplies database connectivity in a protected settings file and uses
the verified release JDBC artifact when required by the image. Raw credentials
are not command arguments, public configuration or cache identity. Closed
Liquibase output is not forwarded line-by-line into public job events.

## No host-runner settings

Host Liquibase, JAVA_HOME, Windows batch mode and PATH do not select the runner.
Remove obsolete workspace liquibase.exec, liquibase.exec_mode and liquibase.env
settings; the CLI rejects them even when empty or null. Public server configuration
is not an executable-selection control plane. Use the version option or the
approved file-backed liquibase_version attribute instead. Engine installation in
WSL is separate from Liquibase selection and must carry the same verified JDBC JAR.

For remote inputs, see [source synchronization](remote-source-input-sync.md).
Extended search-path compatibility is tracked separately in
[#173](https://github.com/kimak-irmagi/sqlrs/issues/173); it is not implied by
passing a simple changelog or by the presence of a version option.

## Completion and failures

Normal completion removes exact owned temporary containers, private output and
working files while preserving the published result. Known failures must not
be reported as success; an unproved result must not trigger a blind SQL repeat.
Extended restart/replay is a separate delivery,
[#170](https://github.com/kimak-irmagi/sqlrs/issues/170), not an availability
claim of this intermediate branch. Changelog logic may cause external effects;
strict isolation remains a future improvement rather than a determinism guarantee.
