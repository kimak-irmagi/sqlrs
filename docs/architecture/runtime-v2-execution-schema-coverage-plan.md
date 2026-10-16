# Execution schema: first coverage follow-up

Status: **one follow-up pass approved** by @evilguest, 2026-10-16,
conversation timestamp 2026-10-16T15:08:21+07:00. Approval is not a
claim that #150 is complete. The [test inventory](runtime-v2-execution-schema-tests.md)
remains approved; this plan adds missing evidence for those same requirements.

## Measured baseline

The complete nested-module suite passed on Windows with count=1 and a fresh
coverage profile. Exact merged statement counts (not rounded block/line counts):

| Scope | Covered/total | Coverage |
| --- | --- | --- |
| Whole module | 4888/5182 | 94.33% |
| All four new Go implementation files | 588/659 | 89.23% |
| execution_validation.go | 266/305 | 87.21% |
| execution_value.go | 251/279 | 89.96% |
| execution.go | 34/37 | 91.89% |
| canonical_value_inspect_v1.go | 37/38 | 97.37% |

The local profile is backend/libs/runtime-go/coverage-runtime-all.out; it is a
generated artifact, not a committed fixture. The coverage checker merges
duplicate blocks, weights their statement counts, and reports uncovered source
ranges. Other packages have older uncovered statements, but the first pass
concentrates on the new files with the largest gaps. No existing fixture/test
weakening or unrelated module refactoring is proposed.

## Ordered work and requirements

1. **execution_validation.go: 39 uncovered statements.** Add public Build/Parse
   cases for ES07–ES19: malformed/oversize raw values through every collection
   and typed-path/reference channel; duplicate/invalid/unused bindings;
   reordered multiple sources/services; missing database-container target;
   host executable/working-directory and mount target rules; invalid Windows
   image roots and service socket/file roots; incompatible service transport
   and empty selected database/user; wrong-variant text fields; literal merging
   that exceeds the string limit. Each fixture isolates the relevant failure
   and asserts its error/zero-result/privacy contract. No private validator is
   called solely to execute an otherwise unreachable branch.
2. **execution_value.go: 28 uncovered statements.** Add ES15–ES20 cases for
   missing/wrong-type nested scalar members; file yes/no flag and forbidden tree
   executable member; invalid mount access and variant members; auth versus
   semantic secret member separation and invalid reference reconstruction;
   unknown top-level member with the same field count; and bounded generated
   node/byte overflows. Distinguish per-value public-helper errors from summed
   forest limits. For Parse, generic-valid oversized forests must fail budget
   checks before semantic traversal; canonical bytes/checksums remain valid.
   Exercise these via public Build/Parse, not injected encoder errors.
3. **execution.go: three uncovered statements.** The generated-limit tests must
   exercise propagation of executionFields failures. Audit the AddIdentityField
   and final Build failure branches against the fixed seven-field schema and
   validated constructed values. Only if those failures are proven impossible
   under the documented invariant, simplify their handling with explicit
   invariant comments; do not add production hooks or forged private state.
   Likewise audit duplicate reconstruction of an already valid immutable secret
   reference in the validator before proposing removal of an impossible failure.
4. **canonical_value_inspect_v1.go: one uncovered statement.** Already above the
   minimum. Inspect the unknown-tag fallthrough against the opaque validated-node
   invariant; keep it as a documented defensive path or simplify only if no
   reachable requirement exists. Do not mutate internals to manufacture coverage.
5. Re-run the full suite, exact per-file/new-code/module coverage checks, fixed
   independent vectors, clean staged consumer and the three fuzz targets.
   Run the approved race gate on Linux CI; local Windows -race currently fails
   while starting cgo's configured compiler, not during a race assertion.
   Obtain a fresh detailed uncovered-range report before proposing any further
   iteration. If any required threshold is still below 95%, ask for approval
   again rather than extending this pass silently.

No new semantic requirement has yet been inferred from these gaps. If the audit
finds one, document it and obtain design approval before asserting it in tests.
Dead-code removal requires the invariant proof above; absence of coverage alone
is not such proof. Existing canonical encodings/domains and publication #151 do
not change. Do not mark the roadmap/feature complete before these gates pass.

## Verification snapshot before PR publication

- All nested-module tests and both Node test groups passed (six tests each).
- Independent published conformance-bundle verification passed.
- The clean external consumer passed through a local file proxy with a unique
  synthetic prerelease, no replace/internal/schemaauthor imports. Reusing the
  old fixed local v0.0.0-pr.0 first selected a previously cached archive; that
  attempt is not counted as evidence for current source.
- All three new fuzz targets passed with baseline execution and mutations; the
  input target was rerun for five seconds because the first two-second Windows
  invocation ended before worker initialization completed.
- No GitHub PR, immutable module release or remote CI result is claimed.

## Invariant audit performed in the approved pass

The public SecretReference constructor is the only way to obtain a nonzero
opaque reference; its data is immutable. After Valid and facade raw/NUL checks,
reconstruction could not fail and added no requirement. Copying the handle into
a new pointer preserves ownership without the impossible reconstruction branch.

The encoder produces exactly primitive plus six protected values with fixed
valid names and kinds; all error-producing canonical construction and aggregate
limits are checked before fields are returned. A fresh local extension builder
therefore cannot be nil/sealed, receive unknown/duplicate/wrong-kind fields, or
miss a required field. Field wrapping and AddIdentityField/final Build failure
branches were simplified only under these established preconditions. The
schema/independent-vector tests remain unchanged and enforce the fixed set.

The root inspection unknown-tag fallthrough is retained as a defensive case;
opaque public values cannot reach it, and its file already exceeds 95%.

## Result of the approved pass (before PR publication)

The pass is complete; no second coverage iteration was performed. A fresh full
module run passed, followed by the exact coverage gate:

| Scope | Covered/total | Coverage |
| --- | --- | --- |
| Whole module | 4948/5175 | 95.61% |
| All new implementation files | 648/652 | 99.39% |
| execution_validation.go | 303/303 | 100% |
| execution_value.go | 274/277 | 98.92% |
| execution.go | 34/34 | 100% |
| canonical_value_inspect_v1.go | 37/38 | 97.37% |

Touched-package merged totals are 2731/2882 (94.76%) for the root and 617/624
(98.88%) for schemas/sqlrs. These are reported separately; the approved gates
apply to each new file, all new code and the whole module, not every old package.

Four new statements remain uncovered: the one defensive unknown-tag return and
three statements for node/byte rejection in canonical map construction. These
are retained, not hidden by exclusions or removed to force 100%. All required
95% thresholds pass without changing any existing expected canonical bytes.

The three fuzz targets passed again with all seed baselines and mutations
(five seconds each), as did the clean external consumer using a fresh local
synthetic version. Both Node groups and all three independent golden/bundle
verifiers passed. The roadmap reflects local implementation, not a release or
completed GitHub acceptance. Linux race and remote cross-platform CI remain
unverified at this measurement; no PR/merge/module publication had yet occurred.

## PR review follow-up

The implementation was subsequently published in
[PR #152](https://github.com/kimak-irmagi/sqlrs/pull/152). Its initial commit
`fc808d0` passed [all 15 CI jobs](https://github.com/kimak-irmagi/sqlrs/actions/runs/38878043249),
including Linux race detection, fuzzing, the clean consumer and all three OSes.
This supersedes the local-only caveat above, not the separate publication #151.

The subsequent code review found two evidence gaps: STALE/UNKNOWN used identical
old/new identities, and ES05 did not explicitly pin all semantic dimensions.
The correction uses distinct fingerprints and evidence generations and checks
the returned resolution, Store argument and decoded stored record. Added paired
valid inputs pin the missing runtime, invocation, source/mount, service and
secret dimensions; both identities must parse and have different canonical
bytes/fingerprints. No production API/encoding changes or weaker tests are used.
Documentation now distinguishes implemented PR source from accepted/released code.
Final acceptance and CI evidence always apply to the latest PR commit.

The corrected tests passed the full local Go suite and go vet. All 12 Node
checks and the three independent golden/bundle verifiers also passed. A fresh
profile remains 648/652 (99.39%) for new production code and 4948/5175 (95.61%)
for the module; every new production file still exceeds 95%. These are local
results, not an assertion that the follow-up commit has already passed CI.
