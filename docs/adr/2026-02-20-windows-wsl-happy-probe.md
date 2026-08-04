# ADR: windows-latest WSL happy-path probe workflow

Status: Accepted
Date: 2026-02-20

## Decision Record 1: validate Windows hosted-runner happy-path via standalone workflow

- Status: Obsolete (superseded by [Decision Record 5](#decision-record-5-run-probe-via-windows-host-sqlrs-with-btrfs-init))
- Timestamp: 2026-02-20T22:53:14.6043817+07:00
- User: @evilguest
- Agent: Codex (GPT-5)
- Question: How should we validate `hp-psql-chinook` happy-path on standard
  `windows-latest` runners before integrating it into release-candidate gating?
- Alternatives:
  - Keep Windows smoke-only and postpone happy-path validation.
  - Add Windows happy-path directly into `release-local.yml` immediately.
  - Create a separate probe workflow on `windows-latest` that sets up WSL and
    runs the chinook happy-path end to end.
- Decision: Implement a standalone probe workflow
  (`e2e-windows-wsl-probe.yml`) that:
  - provisions WSL (`Vampire/setup-wsl`);
  - runs `hp-psql-chinook` inside WSL with `snapshot=copy`;
  - validates output against existing golden file;
  - uploads diagnostics for iteration.
- Rationale: This de-risks hosted Windows environment/tooling assumptions
  (WSL+Docker path) without breaking current release gating, and provides a
  direct path to later merge into `release-local.yml` once stable.

## Decision Record 2: include host docker setup action as additional bootstrap

- Timestamp: 2026-02-20T22:57:34.3032848+07:00
- User: @evilguest
- Agent: Codex (GPT-5)
- Question: Should the Windows WSL probe include `docker/setup-docker-action`
  bootstrap?
- Alternatives:
  - Keep only in-WSL Docker bootstrap (`docker.io` + `dockerd`).
  - Add host Docker bootstrap action and keep in-WSL bootstrap fallback.
- Decision: Add `docker/setup-docker-action@v4` in the probe workflow before WSL
  execution, while retaining in-WSL daemon bootstrap logic.
- Rationale: This improves startup robustness on hosted runners and preserves the
  existing WSL-local fallback path if host-level setup is insufficient.

## Decision Record 3: add temporary push trigger for branch iteration

- Timestamp: 2026-02-20T23:05:03.4995942+07:00
- User: @evilguest
- Agent: Codex (GPT-5)
- Question: How should the probe workflow be triggered before merge so it is
  visible and runnable during branch work?
- Alternatives:
  - Keep `workflow_dispatch` only.
  - Add temporary `push` trigger for non-main branches plus keep
    `workflow_dispatch`.
- Decision: Add temporary `push` trigger with `branches-ignore: [main]` while
  keeping `workflow_dispatch`.
- Rationale: This allows immediate branch-level validation before merge and
  avoids extra noise on `main`.

## Decision Record 4: bootstrap Chinook SQL asset explicitly in probe workflow

- Timestamp: 2026-02-20T23:08:45.7211390+07:00
- User: @evilguest
- Agent: Codex (GPT-5)
- Question: How should the probe guarantee `prepare.sql` dependencies for
  `hp-psql-chinook` on clean runners?
- Alternatives:
  - Assume `Chinook_PostgreSql.sql` is already present in repository checkout.
  - Run repo-wide `pnpm fetch:sql --lock` in probe.
  - Add explicit chinook-only download with sha256 verification in probe.
- Decision: Add an explicit locked bootstrap step in the workflow that downloads
  `examples/chinook/Chinook_PostgreSql.sql` and verifies the expected sha256
  before WSL execution.
- Rationale: This directly satisfies the dependency of `prepare.sql` in a clean
  runner and avoids extra tooling assumptions in the probe path.

## Decision Record 5: run probe via Windows-host sqlrs with strict btrfs init

- Timestamp: 2026-02-21T00:20:00+07:00
- User: @evilguest
- Agent: Codex (GPT-5)
- Question: Should probe execution emulate the real user path where `sqlrs` is
  a Windows-host application and `btrfs` snapshot mode is required?
- Alternatives:
  - Keep Linux-style run inside WSL and `snapshot=copy`.
  - Run host `sqlrs.exe` but keep non-strict snapshot fallback.
  - Run host `sqlrs.exe` and require strict `snapshot=btrfs` initialization with
    WSL+Docker prerequisites.
- Decision: Update probe workflow to run `sqlrs.exe` on Windows host, execute
  `init local --snapshot btrfs --store image ... --distro Ubuntu-24.04`, and
  fail the job if prerequisite checks (WSL distro, Docker availability, init)
  fail.
- Rationale: This validates the real user contract and prevents false-green
  results that bypass actual `btrfs` behavior.

## Decision Record 6: integrate Windows WSL happy-path into release-local matrix

- Timestamp: 2026-02-21T04:15:00+07:00
- User: @evilguest
- Agent: Codex (GPT-5)
- Question: How should the validated Windows WSL happy-path flow be integrated
  into release gating?
- Alternatives:
  - Keep probe-only validation and leave `release-local.yml` unchanged.
  - Add separate Windows happy-path job outside existing E2E matrix.
  - Extend release E2E matrix with a `platform` axis and include Windows WSL
    blocking cells there.
- Decision: Extend `release-local.yml` E2E job to a matrix with
  `platform x scenario x snapshot_backend`, keeping Linux full matrix and adding
  Windows blocking cell `hp-psql-chinook + btrfs` (host `sqlrs.exe`, WSL
  runtime prerequisites, diagnostics upload).
- Rationale: This moves the validated path under release gate control while
  preserving phased rollout by limiting Windows scope to the proven scenario.

## Decision Record 7: enable Windows copy backend cell in release matrix

- Timestamp: 2026-02-21T05:10:00+07:00
- User: @evilguest
- Agent: Codex (GPT-5)
- Question: Should Windows release E2E also validate `snapshot=copy` and how
  should engine binary selection be handled?
- Alternatives:
  - Keep only Windows `btrfs` cell and skip `copy`.
  - Enable Windows `copy` but keep WSL-only engine contract.
  - Enable Windows `copy` and route engine per backend:
    host Windows engine for `copy`, Linux engine in WSL for `btrfs`.
- Decision: Enable Windows `copy` for `hp-psql-chinook` in release matrix and
  branch engine selection by backend:
  - `copy` -> Windows `sqlrs-engine.exe` on host;
  - `btrfs` -> Linux `sqlrs-engine` via WSL.
- Rationale: This broadens blocking coverage on Windows while preserving correct
  runtime contract for each backend and avoiding platform-mismatched engine
  execution.

## Decision Record 8: keep probe workflow as manual diagnostic path only

Status: Obsolete; superseded by [Decision Record 9](#decision-record-9-remove-the-obsolete-standalone-probe-test).

- Timestamp: 2026-02-21T05:25:00+07:00
- User: @evilguest
- Agent: Codex (GPT-5)
- Question: Should `.github/workflows/e2e-windows-wsl-probe.yml` be removed or
  still kept after integration into `release-local.yml`?
- Alternatives:
  - Delete probe workflow entirely.
  - Keep probe workflow but disable automatic triggers.
  - Keep current automatic push trigger.
- Decision: Keep probe workflow, remove automatic `push` trigger, and leave
  `workflow_dispatch` only.
- Rationale: Release gating is now covered by `release-local.yml`; manual probe
  remains useful for focused troubleshooting without extra CI noise.

## Decision Record 9: remove the obsolete standalone probe test

- Timestamp: 2026-08-04T16:59:06+07:00
- User: @evilguest
- Agent: Codex (GPT-5)
- Question: Should the test and documentation for the removed standalone
  Windows WSL probe remain after its coverage moved into `release-local.yml`?
- Alternatives:
  - Keep the orphaned test and historical documentation unchanged.
  - Restore the standalone probe workflow.
  - Remove the orphaned test and update the documentation to describe the
    release-gated matrix.
- Decision: Remove `windows-wsl-probe-workflow.test.mjs` and update the user
  guide to reference the Windows cells in `release-local.yml`.
- Rationale: The standalone workflow was removed in commit `ab0bedf`, so its
  test could only fail on a missing file. The release workflow and its tests
  now own the same Windows/WSL contract.
