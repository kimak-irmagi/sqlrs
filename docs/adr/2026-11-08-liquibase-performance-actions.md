# Run the retained Liquibase performance protocol away from the laptop

- Conversation date: 2026-11-08, Asia/Novosibirsk; exact reply timestamp unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Status: Actions direction and Ubuntu/component design approved on 2026-11-08;
  PA01–PA12 separately approved on the same date after critical and repeat review.

## Question and alternatives

The approved v1/v2 paired/noise/confirmation series for three public fixtures is
a multi-hour workload. Continue on the user's Windows laptop, shorten/waive the
protocol, or move execution to GitHub Actions while retaining the protocol?

## Decision and rationale

The user prefers GitHub Actions if feasible, retaining and executing the full
protocol. Do not tie up the laptop or expose measurements to its user workload.
This does not grant a speed pass, change thresholds or shorten required series.
The local multi-pair scheduler was stopped; the already active collector may
finish its own exact cleanup, without launching more laptop series. An incomplete
third pair is not acceptance evidence.

Published Linux/amd64 RC7 is available. The approved component design selects standard
Ubuntu Actions runners and the same released source, rather than pretending the
Windows artifact runs unchanged there. Same-host pairing, isolated confirmation,
six-hour feasibility, opt-in launch and private evidence handling are recorded in
[issue89-performance-actions-design.md](../architecture/issue89-performance-actions-design.md).
The user explicitly approved this Ubuntu design in the subsequent reply
“Утвердить проект переноса на Ubuntu”. No paid/self-hosted runner is provisioned;
implementation still follows the separate test-inventory approval.

This changes the execution venue, not the earlier accepted actual-v1
comparison (conversation of 2026-10-16, retained in the original worktree's
2026-10-16-liquibase-performance-baseline.md). Historical Windows
observations remain diagnostics and cannot be pooled into Linux timings.

## Acquisition reliability within the approved PA02 boundary

- Conversation/implementation date: 2026-11-08, Asia/Novosibirsk; exact user
  reply timestamp unavailable. This is implementation of the approved inventory,
  not a new approval attributed to the user.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Question: repeated preparation-only HTTP/transport failures prevent obtaining
  the already pinned release on fresh hosted VMs. Retry whole jobs, use a
  different reference, or retry acquisition through the system transport?
- Decision: at most three transient Node acquisition attempts, then one bounded
  system curl attempt against the **same public release URL**, with the same
  mandatory SHA256 before extraction/execution. A received checksum mismatch
  fails immediately, without trying another reference. No measured sample is
  retried/replaced by this policy; all acquisition precedes measurement.
- Rationale: preserve immutable artifact identity while accommodating transient
  transport failure. No private access, paid runner, alternate v1 build, product
  compatibility change or speed waiver is introduced. Exhaustion remains failure.

## Private test-store disposal within PA09

- Conversation/implementation date: 2026-11-08, Asia/Novosibirsk; exact user reply
  time unavailable. Implementation choice within the separately approved PA09,
  not a new approval attributed to the user.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Question: the native collector reaches oracle verification but reports private
  cleanup failure. Container-owned PostgreSQL files can prevent an ordinary
  hosted UID from traversing the temporary store during Go TempDir cleanup.
- Alternatives: weaken file permissions, elevate the measured engine, or confine
  a pinned Docker helper to disposal of the exact private test root after owner
  containers are absent.
- Decision: use the last option only in external-test teardown. Require a
  canonical absolute non-root directory and exact unpredictable owner marker;
  do not follow symlinks. Delete contents through the already pinned fixture
  image, preserving the marker until Go removes the empty owned directory.
  Retire the helper by its full ID, independently bound the cleanup and confirm
  absence. Any unproved owner/removal remains an incomplete run.
- Rationale: preserve closed permissions and identical measured engine users.
  No production cleanup, public API, persistent schema, published instance or
  laptop resource is changed. Teardown is outside measured intervals and inside
  the overall hosted budget. The inherited prototype retain-store mode is not
  part of this new collector; pilot versus cohort is owned by the scheduler.
