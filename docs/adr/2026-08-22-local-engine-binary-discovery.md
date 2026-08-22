# ADR: Local engine binary discovery and Windows bundle composition

Status: Proposed
Date: 2026-08-22

## Decision Record 1: Resolve local engines without mandatory configuration

- Timestamp: 2026-08-22T16:38:14+07:00
- User: @evilguest
- Agent: Codex (GPT-5)
- Question: How should `sqlrs` find a local engine when `sqlrs init local` is
  invoked without an explicit engine reference?
- Alternatives:
  - Require a new `SQLRS_HOME` variable and resolve binaries below it.
  - Require an engine environment variable such as `SQLRS_ENGINE`.
  - Search only `PATH` for `sqlrs-engine`.
  - Resolve explicit overrides first, then discover binaries relative to the
    running CLI release bundle, with `PATH` as a native-host fallback.
- Decision: Keep explicit configuration optional. Resolve the native host engine
  from `--engine`/config, then `SQLRS_DAEMON_PATH`, then the bundle containing
  the running CLI, and finally `PATH`. Resolve the Windows WSL payload
  independently from `--wsl-engine`/the provisioned config,
  `SQLRS_WSL_ENGINE_PATH`, and then the CLI bundle. Do not introduce
  `SQLRS_HOME` for executable discovery.
- Rationale: A release bundle should work after extraction without per-workspace
  paths, while explicit overrides remain available for development and custom
  installations. Separating the host and WSL paths removes the existing
  ambiguity where one daemon path could not describe both binaries.

## Decision Record 2: Ship both engines in the Windows release

- Timestamp: 2026-08-22T16:38:14+07:00
- User: @evilguest
- Agent: Codex (GPT-5)
- Question: Does the Windows release need both a native Windows engine and a
  Linux engine?
- Alternatives:
  - Ship only the Linux engine and require WSL2 for every local workflow.
  - Ship only the Windows engine and remove WSL2/btrfs support.
  - Ship both engines and select the runtime from the resolved snapshot backend.
- Decision: Ship `sqlrs.exe`, `sqlrs-engine.exe`, and a same-architecture Linux
  engine under `libexec/linux-<arch>/sqlrs-engine`. Use the Linux engine for
  WSL2/btrfs and the Windows engine for copy snapshots or automatic host
  fallback. During WSL init, copy the Linux payload into the selected distro and
  record its installed Linux path.
- Rationale: Docker Desktop exposes its engine to Windows clients even when its
  own backend uses WSL2, so the native engine remains useful without requiring a
  user-managed WSL distribution. Shipping both binaries also preserves the
  documented copy fallback while enabling the preferred WSL2/btrfs path.

## Decision Record 3: Validate before writing and repair derived provisioning

- Timestamp: 2026-08-22T16:38:14+07:00
- User: @evilguest
- Agent: Codex (GPT-5)
- Question: How should init behave when engine discovery or a previous WSL
  provisioning attempt is incomplete?
- Alternatives:
  - Write workspace configuration first and report missing engines later.
  - Require users to delete `.sqlrs` and initialize again.
  - Validate required bundle inputs before configuration writes and allow a
    repeated init to repair the derived WSL engine installation.
- Decision: Validate all binaries required by the selected runtime before
  creating or updating workspace configuration. Keep the existing configuration
  unchanged on validation or provisioning failure. A repeated
  `sqlrs init local` may restore a missing provisioned Linux engine without
  `--update`, because that copy is derived runtime state rather than user-owned
  configuration.
- Rationale: Initialization must not leave a workspace marker that prevents a
  user from completing setup after correcting permissions or elevation. Safe,
  idempotent repair removes the need for manual workspace deletion.
