# sqlrs Windows WSL release happy path

## Purpose

The Windows release-gated E2E validates that `hp-psql-chinook` can run on a
standard `windows-latest` GitHub-hosted runner with host-side `sqlrs`
execution. The `btrfs` snapshot variant delegates the Linux engine and Docker
prerequisites through WSL2.

The former standalone `e2e-windows-wsl-probe.yml` workflow was removed after
this execution model moved into the release-gated E2E matrix.

## Workflow

Workflow file:

- `.github/workflows/release-local.yml`

Execution model:

1. Checkout repository on `windows-latest`.
2. Download the release-candidate Windows bundle and the Linux engine bundle
   used by the WSL runtime.
3. Initialize Docker on the Windows host via `docker/setup-docker-action`.
4. Install/setup WSL distro (`Ubuntu-24.04`) via `Vampire/setup-wsl`.
5. Ensure Docker daemon is available inside the WSL distro.
6. Assert the host session is elevated (Administrator), required for
   loopback-backed `btrfs` init.
7. Fetch the example SQL assets through the locked external-asset manifest.
8. On Windows host (not inside WSL), run:
   - `sqlrs init local --snapshot btrfs --store image ... --distro Ubuntu-24.04`;
   - `sqlrs prepare:psql` + `sqlrs run:psql` for `examples/chinook`.
9. Normalize stdout and compare with committed golden output:
   `test/e2e/release/hp-psql-chinook/golden.txt`.
10. Upload diagnostics artifacts for post-failure analysis.

This matches real user behavior where CLI runs as a Windows application and
delegates Linux runtime concerns through WSL.

## Trigger

The workflow runs for an explicitly supplied release-candidate version through
`workflow_dispatch`, or for a matching release tag. Windows cells are part of
the blocking `e2e-happy` matrix.

## Output Artifacts

Diagnostics are uploaded under:

- `e2e-windows-hp-psql-chinook-copy`
- `e2e-windows-hp-psql-chinook-btrfs`

Including:

- init and flow command logs;
- raw/normalized stdout/stderr;
- golden diff;
- Docker daemon log from WSL;
- engine state/log files when available.
