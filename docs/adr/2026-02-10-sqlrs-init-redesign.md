# ADR: Redesign `sqlrs init` (local/remote + snapshot/store)

Status: Partially obsolete. The local-init design remains accepted; the remote
`--url` plus `--token` syntax is superseded by
[2026-09-24 Remote Connection Bootstrap](2026-09-24-remote-connection-bootstrap.md).

## Conversation timestamp

2026-02-10

## GitHub user id

@evilguest

## Agent name/version

Codex (GPT-5)

## Question discussed

How should `sqlrs init` be redesigned to remove the `--wsl` flag, make local vs
remote setup explicit, and expose snapshot/store choices consistently across platforms?

## Alternatives considered

1. Keep the existing `sqlrs init --wsl` flag and add more WSL-specific options.
2. Keep a single `sqlrs init` command with a new `--engine local|remote` selector.
3. Introduce explicit subcommands: `sqlrs init local` and `sqlrs init remote`,
   with snapshot/store flags for local and URL/token for remote.

## Chosen solution

Adopt explicit subcommands and new flags:

- `sqlrs init local` for local engine setup.
- `sqlrs init remote --url <url> --token <token>` for remote engine configuration.
- Local snapshot backend selection via `--snapshot <auto|btrfs|overlay|copy>`.
- Local store provisioning via `--store <dir|device|image> [path]` and `--store-size`.
- WSL+btrfs setup is triggered by `sqlrs init local --snapshot btrfs` on Windows.

## Rationale

This removes platform-specific flags from the main CLI, makes intent explicit,
and aligns init behavior with snapshot backend capabilities. It also cleanly
separates remote configuration from local storage provisioning, simplifies help
output, and provides a clearer path for future extensions without overloading a
single command.
