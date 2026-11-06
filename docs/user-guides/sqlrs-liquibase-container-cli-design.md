# Liquibase container execution: approved user contract

Status: approved on 2026-10-05; main delivery #89 remains in progress. This is
an approved target contract, not a claim that the container service is integrated.
See [prepare:lb](sqlrs-prepare-liquibase.md) and the
[bounded component design](../architecture/liquibase-container-component-structure.md).

## Direct CLI commands

Keep the direct commands for the primary Liquibase recipe:

```text
sqlrs prepare:lb [--liquibase-version <release|latest>] [existing prepare options] -- <liquibase-args...>
sqlrs plan:lb [--liquibase-version <release|latest>] [existing plan options] -- <liquibase-args...>
sqlrs cache explain prepare:lb [--liquibase-version <release|latest>] [existing explain options] -- <liquibase-args...>
```

`--liquibase-version` is a sqlrs option before `--`; it is not forwarded to
Liquibase. It is a user-facing tag selector, not an immutable execution
fingerprint: even a numeric tag is resolved to a concrete image digest for
each job. When omitted, it means `latest`. There is no host-Liquibase
fallback. The `--image` option, where present, still selects
the **database base image**, not the Liquibase runner image. Other existing
CLI options and the Liquibase argument separator keep their current meaning.

Examples:

```text
sqlrs prepare:lb --liquibase-version 5.0.3 --image postgres:17 -- update --changelog-file config/liquibase/master.xml --context-filter=dev,faker
sqlrs prepare:lb --image postgres:17 -- update --changelog-file liquibase/changelog.xml
```

The CLI constructs a transient Liquibase transform declaration from the direct
command. Its version has the same meaning as a version written in a repository
transform description; it is not a separate host-runner selector. The
declaration remains user-authored intent; the engine subsequently details it
into fixed execution bindings and a reproducible fingerprint. Direct
commands continue to work in both local and remote profiles.

## File-backed transform descriptions

A repository-authored Liquibase transform declares the version explicitly in
its string attributes. The field is `liquibase_version`:

```yaml
kind: lb
reference: config/liquibase/master.xml
arguments:
  - update
  - --context-filter=dev,faker
attributes:
  liquibase_version: "5.0.3"
```

`"latest"` is also valid. Omitting `liquibase_version` from a file-backed
Liquibase transform has the same meaning as `"latest"`. A direct CLI invocation
and a file-backed declaration
that describe the same changelog, arguments, database base, and version must
resolve to the same execution semantics. The exact source-file format and
discovery rules remain those of the separately approved Runtime v2 composition
contract; this proposal adds a Liquibase-specific attribute, not a new
composition format.

## Container selection and repeatability

In local and remote modes, the engine runs Liquibase in a container. With no
version selection, it uses `liquibase/liquibase:latest`; a fixed release uses
`liquibase/liquibase:<release>`. This interprets the requested unversioned
image as MobilityData's `liquibase/liquibase`, not the distinct literal Docker
reference `liquibase:liquibase`. The project selects only the Liquibase release
or the floating `latest` tag; the engine selects the trusted image repository
and supplies database
connectivity and the database driver. Host executable paths, Windows batch
mode, and client `JAVA_HOME` are not Liquibase transform settings.

For `latest`, the engine refreshes and resolves the tag before planning. It
binds the resolved platform-specific image digest to the job and uses that
same digest for planning, execution, retries, and cache identity. A later job
may resolve a newer digest. If the registry cannot be checked, a new `latest`
job fails rather than silently using a stale local image. This is the agreed
floating-version behavior, not a promise that two jobs run the same binary.

Versioned releases are also recorded with their resolved image digest for
diagnostics and reproducibility. The engine does not substitute a different
Liquibase version when the requested image cannot be obtained. How custom
Liquibase extensions are packaged is outside this CLI-syntax proposal.

## Compatibility boundary

This contract does not retain host-runner compatibility. The old workspace
`liquibase.exec`, `liquibase.exec_mode`, and client `JAVA_HOME` values do not
select the runner for local or remote Liquibase transforms. The API and
component designs will specify request admission, source mounting, and the
transition of existing aliases in subsequent design stages.
