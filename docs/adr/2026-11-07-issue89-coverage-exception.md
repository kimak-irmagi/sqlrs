# Explicit coverage exception for the #89 delivery

- Conversation date: 2026-11-07, Asia/Novosibirsk; exact reply timestamp unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Status: Accepted, limited to #89 / PR #175.

Performance/merge provisions are partially obsolete: the
[2026-11-14 decision](2026-11-14-issue89-performance-acceptance-and-merge.md)
accepts the measured overhead and authorizes merge. The coverage exception,
honest reporting and unchanged shared threshold remain in force.

## Question and alternatives

After separating the broad quality program into #174, must #89 still reach
95% aggregate coverage before merging? Alternatives: retain that blocking gate
and start another approved coverage iteration, or explicitly grant #89 an
exception while keeping all functional, regression and performance criteria.

## Decision

The user explicitly chose the exception for #89. Aggregate coverage remediation
belongs to #174, retaining the repository's 100% target and 95% minimum there.
Green regressions and fulfilled functional/performance criteria remain mandatory
for #89. The exception does not waive safety, authorize weakening tests, claim
95% coverage, or authorize changing the shared CI threshold. Report the red
coverage check transparently; merge authorization is separate from this decision.

## Rationale and consistency

The original executable trust-boundary fix must not absorb the entire cross-project
coverage program again. Current ordinary CI coverage is engine 74.3% / CLI 94.8%;
the distinct all-platform CI/local/native union is 84.35% for the engine. Neither
measurement is represented as meeting 95%.

Earlier #89 inventory statements that 95% was not waived are superseded only for
this delivery by this explicit later decision. Historical measurements remain
unchanged. No earlier ADR is generally obsolete: installation, private capture,
ownership, correctness and test-approval requirements are unchanged. In particular,
the preliminary slowdown versus real v1 is not waived and still needs resolution.
