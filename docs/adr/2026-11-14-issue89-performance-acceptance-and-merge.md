# Accept measured #89 overhead and merge PR #175

- Conversation date: 2026-11-14, Asia/Novosibirsk; recorded at 17:35
  (10:35 UTC), exact user-message timestamp unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-6.
- Status: Accepted by the user's explicit instruction to merge and proceed to
  the next feature.

## Question and alternatives

After O5–O7, CT13 on production revision `28cfc25` measures fresh/cached means
of 51.512/9.113 seconds versus released v1 48.711/8.451 and the fixed old v2
57.615/10.120. Continue optimizing until the original no-regression criterion
passes, or accept the remaining architecture overhead for this delivery?

## Decision

The user accepts the measured **5.75% fresh / 7.84% cached** overhead against
released v1 and explicitly authorizes merging PR #175 into `main`. For #89,
further performance optimization and the outstanding full P01b qualification
are no longer merge prerequisites. This supersedes the earlier no-regression
merge gate for this delivery only; it establishes no general regression budget.

The original measurements, timers, statistical limitations, functional tests
and cleanup requirements remain intact. CT13 remains a diagnostic two-repeat
comparison, not a successful no-regression test or full P01b qualification.
Native run 43566256871 passed real allocation/identity, cache, failure,
uncertain-transfer and cleanup checks. Current-head CI 43568065037 passed all
15 test/documentation jobs; aggregate coverage alone failed.

The separately approved #89 aggregate-coverage exception remains in force:
remediation belongs to #174 and the shared 95% threshold is unchanged. This
decision adds the previously separate merge authorization; no branch protection
or CI rule is weakened. Deferred #170–#174 scopes remain separate deliveries.

## Rationale and supersession

The user considers the observed overhead acceptable in exchange for the new
engine-owned execution boundary. The implemented reductions save 10.59% fresh
and 9.95% cached against the fixed old container engine, with all 16 CT13 passes
verifying data and cleanup.

Earlier ADR provisions retaining performance as a #89 merge blocker are
partially obsolete under this decision. Their designs, test approvals,
measurement procedures, historical results and unrelated requirements remain
valid. See the [measured results](../architecture/issue89-measured-overhead-reduction.md)
and the [coverage exception](2026-11-07-issue89-coverage-exception.md).
