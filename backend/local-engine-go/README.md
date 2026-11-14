# sqlrs-engine (local, Go)

The approved #89 overhead reduction is described in
[O5–O7](../../docs/architecture/issue89-measured-overhead-reduction.md): exact
planning-DB handoff, per-service inventory reuse, and same-exec private results.
O5 planning-DB handoff, O6 inventory reuse and O7 same-exec results are implemented
and undergoing qualification. New O5 files meet the local coverage minimum
(98.95% combined; each file at least 95%).
No API or SQL-retry permission is added. Hosted native/speed results for these
changes are not yet available; the prior measured regression is still open.

Local engine used by `sqlrs` in the local profile.

Current API surface includes:

- `/v1/health`
- `/v1/config*`
- `/v1/prepare-jobs*` and `/v1/tasks`
- `/v1/runs`
- `/v1/names*`, `/v1/instances*`, `/v1/states*`

The engine owns local prepare planning/execution, state snapshotting, instance
lifecycle, config persistence, and deletion flows.

Liquibase production jobs use the engine-owned Docker runner, not a host Java
installation. Build the complete bundle with `node scripts/build-engine-go.mjs`
from the repository root to include the checksum-locked JDBC dependency under
`lib/` beside the executable. A bare `go build` does not package that dependency.
See [normal execution](../../docs/architecture/issue89-main-path-execution.md)
for supported inputs, cleanup and the deferred automatic-recovery boundary.
Ordinary Liquibase steps reuse their database and runner containers. The final
working database is handed to the published instance; deleting its completed
prepare job does not delete that instance's database or runtime directory.

See also:

- `../../docs/architecture/engine-internals.md`
- `../../docs/architecture/local-engine-component-structure.md`
- `../../docs/api-guides/sqlrs-engine.openapi.yaml`

## Build

From repository root:

```bash
go build -o dist/bin/sqlrs-engine ./backend/local-engine-go/cmd/sqlrs-engine
```

From this module directory:

```bash
go build -o ../../dist/bin/sqlrs-engine ./cmd/sqlrs-engine
```

## Run

```bash
./dist/bin/sqlrs-engine \
  --listen 127.0.0.1:0 \
  --run-dir /tmp/sqlrs-run \
  --write-engine-json /tmp/sqlrs-state/engine.json \
  --idle-timeout 30s
```

## Flags

- `--listen` (required): host:port to bind; use `127.0.0.1:0` for a random port.
- `--run-dir`: runtime directory for engine process state.
- `--write-engine-json` (required): path to discovery metadata written on startup.
- `--idle-timeout`: shutdown after idle period.
- `--version`: engine version string.

## Test

```bash
go test ./...
```
