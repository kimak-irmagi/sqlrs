# Run the retained Liquibase performance protocol away from the laptop

- Conversation date: 2026-11-08, Asia/Novosibirsk; exact reply timestamp unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Status: Actions direction accepted conditionally on feasibility; Ubuntu/component
  proposal and subsequent test inventory still require separate approval.

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

Published Linux/amd64 RC7 is available. A separate proposal recommends standard
Ubuntu Actions runners and the same released source, rather than pretending the
Windows artifact runs unchanged there. Same-host pairing, isolated confirmation,
six-hour feasibility, opt-in launch and private evidence handling are recorded in
issue89-performance-actions-design.md. No paid/self-hosted runner is provisioned
and no CI/collector implementation is approved merely by this direction choice.

This changes the proposed execution venue, not the earlier accepted actual-v1
comparison in 2026-10-16-liquibase-performance-baseline.md. Historical Windows
observations remain diagnostics and cannot be pooled into Linux timings.
