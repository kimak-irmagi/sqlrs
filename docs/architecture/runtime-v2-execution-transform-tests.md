# Execution-transform test inventory

Status: ET01–ET09 separately approved by @evilguest on 2026-10-19 after the #156
[API/structure approval](runtime-v2-execution-transform.md).
Tests and implementation pass locally; CI, merge and publication remain gates.
Publication acceptance is #157.
Revised after review: isolated consumer execution is mandatory, old production
fingerprints have a pre-change baseline, observations must survive, and errors
have explicit expected categories/paths below. Approval is not execution evidence.

## New checks

1. **ET01 — public successful construction.** External-package tests construct
   valid OCI, host and database-container execution transforms with zero and
   multiple inputs, using existing types and no dummy SQL. Assert the exact
   provider/kind/schema and binding field set. Compare binding values with
   independently computed public extension fingerprints, not facade output
   reused as the expected answer. This is pure construction, not tool execution.
2. **ET02 — closed declarations.** Reject wrong/empty kind, missing/zero/foreign
   execution declaration, invalid ordinary declaration fields, nonempty
   arguments/attributes, and invalid or excessive input declarations. Include
   paired accepted controls, reference-length boundaries and legal non-identity
   metadata. A changed locator alone does not change the resolved identity.
3. **ET03 — complete execution validation.** Reject missing/zero/foreign-schema
   execution identity and a structurally accepted identity with matching
   owner/kind but invalid nested semantics or noncanonical form. Reuse existing
   public Build/Parse validation, not a weaker owner-name check. Test-only forged
   values may use schemaauthor; the standalone consumer must not import it.
4. **ET04 — positional input completeness.** Reject missing/extra/zero inputs,
   mismatched owner/kind and a reorder that violates declarations. Separately
   accept two distinct same-owner/kind inputs in either declared order and
   require different fingerprints; accept equal identities in two legal
   positions. Verify exact `extension.input.<index>` and mandatory execution
   bindings without sorting inputs, deduplication or another binding API.
5. **ET05 — identity and lineage sensitivity.** Change valid execution command,
   environment, runtime image/platform, or input identity one at a time; both
   transform fingerprint and derived child change. Changing parent alone changes
   only the child. Changing job ID, reference or valid non-identity metadata
   changes neither. Round-trip full transform/child envelopes; reject tampering
   without recomputing commitments. No private fingerprint algorithm or fixture
   copied from the facade serves as the oracle.
6. **ET06 — observations, bounds and safe failures.** Accept absent and valid
   job observations, including exactly MaxResolvedFields; reject one extra,
   zero observation, foreign owner/kind/schema and unknown fields. Assert the
   exact returned observation values, count and order, including repeated legal
   observations, so dropping or deduplicating job metadata cannot pass. Changing
   only job metadata preserves identity but changes the returned observations.
   Include a protected marker in invalid caller-controlled keys/values; check
   error categories/paths against the table below, not just a non-nil error.
   Every failure returns empty identity/observations, including a failure in
   the LAST observation after valid inputs/observations have been processed.
   A valid job ID is returned as operational data; this API does not claim to
   redact caller-supplied metadata. Existing declaration/input limits are tested
   at their boundary and one beyond, not relaxed.
7. **ET07 — atomic ownership and concurrency.** Mutating caller-owned slices/maps
   after a successful call cannot change the result; modifying returned views
   cannot change a subsequent view. An invalid call cannot contaminate a valid
   retry. Concurrent independent calls on the same immutable inputs give the
   same canonical result under the race detector, with no shared mutable state.
8. **ET08 — real standalone consumers.** Add self-contained
   `test/runtime-v2-consumer/execution_transform_test.go`, with no schemaauthor,
   conformance or internal imports and no helper dependency on another source
   file. In the SAME directory, add `execution_transform_isolation_test.go`:
   its mandatory test runs `go test -count=1 execution_transform_test.go` as a
   child command in the already staged external module, propagating any failure.
   The selected file does not contain the wrapper, so the child cannot recurse.
   Check this isolation runner with a positive self-contained fixture and a
   negative fixture that compiles only when a second helper file is included;
   a runner silently changed to `go test ./...` must fail that negative control.
   The positive control and actual selected-file test must succeed; an unavailable
   compiler or a failed dependency download cannot count as the negative proof.
   No skip/environment opt-in is permitted. Existing CI and release gates copy
   `*_test.go` and run the whole suite, automatically invoking the wrapper and
   then all old tests. Old source S without the new files keeps its old behavior;
   automation H must not substitute its newer tests for historical S.
   The actual consumer constructs a non-PSQL transform with two typed inputs,
   round-trips the existing execution resolver result/cache codec ONCE, and uses
   that decoded execution identity to build/decode transform and derived lineage.
   Do not reimplement resolver dispatch, freshness, storage or cache fault tests;
   their existing suites remain regression gates. Staging uses the exact branch
   module through the file proxy, without replace or sibling sources. #157 runs
   the same source-owned wrapper against immutable RC/GA via public proxy/sumdb.
   No workflow changes, new job or version-only source PR are needed.
9. **ET09 — regressions and measured gates.** Run the complete runtime-go suite,
   go vet, existing schema-import/API architecture checks, frozen PSQL/Docker
   vectors and independent canonical/conformance verifiers. BEFORE adding tests
   that reference the missing API or changing production code, record canonical
   bytes and fingerprints for a Docker factory and PSQL transforms both without
   and with an execution binding from baseline main source
   `27936cb4d97e8acb4437ff76b92f6b4ae0354bb5`. Keep their complete fixed inputs
   and literal expected bytes/digests in a new regression fixture, with that
   source SHA. Never regenerate expected values using the changed code during
   tests or rewrite existing vectors. This is distinct from legacy/conformance
   vector preservation. Preserve every existing assertion. Run existing CI
   OS/race/fuzz and consumer
   gates; distinguish local checks from actual CI. Obtain per-line coverage:
   target 100%, minimum 95% for new production code and the module. Additional
   coverage iterations require approval under repository rules. Do not close
   #156 on documentation approval or #157 on implementation merge alone.

## Fixed error expectations

Every listed case changes only ONE condition on otherwise valid input; no
validation precedence is prescribed for multiple simultaneous defects.
Codes below are existing `runtimev2` canonical-v1 ValidationCode constants;
paths are closed facade paths, not caller-defined names or underlying error text.
Shape/value/limit failures are not collapsed into a generic error.

| Condition | Expected code | Exact safe path |
| --- | --- | --- |
| Missing/zero execution declaration | CodeShapeInvalid | `declaration.execution_environment` |
| Zero typed input/capability/portability member in the declaration | CodeShapeInvalid | `declaration` |
| Wrong declaration kind/owner or nonempty arguments/attributes; invalid ordinary declaration value | CodeValueInvalid | `declaration` |
| Ordinary declaration byte/count limit exceeded | CodeLimitExceeded | `declaration` |
| Missing/zero resolved execution identity | CodeShapeInvalid | `extensions.execution_environment` |
| Execution identity rejected by the existing Parse API | The existing Parse's canonical code for that specific fixture, recorded explicitly in the test case | `extensions.execution_environment` |
| Input result count mismatch or zero input identity | CodeShapeInvalid | `extensions.inputs` |
| Input owner/kind mismatch | CodeValueInvalid | `extensions.inputs` |
| Zero operational observation | CodeShapeInvalid | `observations` |
| Foreign observation owner/kind/schema or unknown field | CodeValueInvalid | `observations` |
| Observation count above MaxResolvedFields | CodeLimitExceeded | `observations` |

Existing declaration-validation legacy codes, if encountered internally, map to
their already defined canonical categories: invalid_shape -> shape_invalid,
invalid_value -> value_invalid, too_large -> limit_exceeded. Do not add speculative
branches for errors unreachable through the validated typed declaration API.
Do not expose the underlying validators' potentially caller-derived
paths. Execution-Parse cases each pin a literal expected code rather than
learning it from the facade under test. Error()/Code/Path and formatted error
all exclude the protected marker. The error table refines the approved API's
safe-failure requirement and was approved with this inventory.

## Review and execution order

After inventory approval, inspect existing tests for contradictions before
editing them; request a choice if one is found. Capture the unchanged production
baseline above first. Then add tests and demonstrate the missing API fails,
implement only the approved facade, and rerun.
Tests are not skipped or weakened to obtain green results. Inspect current
release/consumer scripts and their source S versus automation H behavior before
claiming standalone or public distribution evidence. A staged local proxy is
not a published RC, and neither construction nor a green CI run proves native
tool execution or State reuse eligibility.

## Local execution evidence (2026-10-19)

Existing production/architecture tests were inspected before additions; no
contradiction required changing an existing assertion. The three ET09 baselines
were captured from the fixed source SHA above, then pinned as literal bytes and
digests BEFORE adding missing-API tests. Both the module tests and a clean staged
consumer then failed specifically because BuildExecutionTransform was absent.

After implementation, the full module and `go vet ./...` passed on Windows.
The existing independent canonical-bundle/extension-golden verifiers and seven
coverage-checker controls passed. The complete source-owned external consumer
passed through a newly staged file-proxy version with GOWORK=off and no replace;
its mandatory wrapper passed the positive/negative controls and the selected-file
production test. This proves local package isolation, not public publication.

The statement-weighted per-line profile reports **4991/5208 (95.83%)** for the
module and **33/33 (100%)** for the sole new production file,
`schemas/sqlrs/execution_transform.go`; it has no uncovered blocks. Existing
production Go files, vectors and workflows are unchanged. Initial PowerShell
coverage commands did not retain a profile because unquoted dotted arguments
were split; only the subsequent complete run with quoted flags and an inspected
profile supplies coverage evidence. Linux race/fuzz, other OS runners, actual
GitHub CI and #157 RC/GA/public-proxy qualification are NOT claimed by these
local results. No additional coverage iteration was required.
