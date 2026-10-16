# Runtime Go publication: approved checks

Status: this inventory and the detailed [contract](runtime-go-module-publication.md)
were approved by @evilguest on 2026-10-16, including narrow replacement of the
obsolete existing-test expectations below. Local execution is recorded below;
live publication is pending. Existing Go tests/vectors and coverage gates stay intact.

| ID | Evidence |
| --- | --- |
| MP01 | Modes/defaults and exact version/S/candidate grammar; reject refs, abbreviations, malformed notes, non-main dispatch and wrong input combinations before writes. Test UTF-8 boundaries for 16 KiB notes, 20 KiB complete body and 128 KiB serialized payload, including escapes, control and multibyte characters, generated footer, retries and inherited final body. Shell/workflow syntax stays inert data. |
| MP02 | Exact source ancestry/post-merge ci, missing/failed/running/cancelled/foreign evidence, pagination and errors. H != S works without new scripts/notes in S. A broken validator in S and corrected H demonstrate harness-only recovery: algorithms and Git-root discovery use H and explicit S respectively; Go code/tests, consumer fixtures, manifest and expected data stay at S. Wrong-tree fixtures fail. |
| MP03 | Tag protection rejects absent/disabled/wrong-namespace/insufficient rules. Only GITHUB_TOKEN receives contents:write; no PAT/App fallback or legacy-event recursion. Ref-create/release permission denials stop explicitly; no alternate target/token and no assumption that validate can dry-run writes. Privileged jobs never execute source scripts; source jobs never receive publication credentials. Preserve product/historical boundaries. Mocks do not qualify actual platform permissions. |
| MP04 | Source/candidate/body/size/existing-state gates precede the public tag, then draft, matching assets, publication, public verification. Draft status does not hide a Go tag. Independent expected body/attestation examples avoid a shared generator oracle. Explicit make_latest=false preserves the product latest release. |
| MP05 | Paginated authenticated release/asset discovery includes drafts; public get-by-tag 404 is insufficient. Reject hidden/duplicate releases, insufficient visibility, conflicting S/body/metadata, unknown/malformed/conflicting assets and auth/rate/transport errors. Write 404 is an error. No force/deletion/rewrite/clobber or fallback creation after an ambiguous read. |
| MP06 | Retry every committed-write/lost-response boundary: no objects, tag-only, matching draft without/with tag, each missing asset, published complete/incomplete output and public-verification failure. Drafts resume; published output is only verified; conflicts block before further writes. Concurrent conflicts trigger full reread, not blind retry. Never cancel an active same-version run; pending replacement is not a FIFO guarantee. |
| MP07 | In-run verify-candidate fully checks the published same-S/same-family RC, prerelease flag, body, tag, assets, public checksums, retraction and clean consumer. Missing/draft/wrong/failing RC blocks final writes. Final publish has a mandatory successful current-run dependency; a plausible old report, successful past run or skipped/cancelled current gate cannot bypass it. No artifact download/expiry protocol. |
| MP08 | Final body is the exact verified RC body; reject independent notes and invalid/missing candidate_version. Tag and title differ, S does not. RC and final checksums are separately verified, not equated. H may be newer than S; no version-specific source file/new commit is required. Both final validate and verify also recheck the RC. |
| MP09 | verify has no publication credential/write adapter and never creates/edits tag/release/assets; it checks public output. Harness-only recovery uses corrected H and the unchanged S/publication, without past artifact availability or tag movement. validate runs source gates and, for final versions, RC checks without publishing a nonexistent target. |
| MP10 | Public distribution enforces exact version/tag/source/module, valid nonempty module/go.mod sums, no replace/go.work/neighbor source, bounded retry and existing consumer suite. Checksum/attestation/body/consumer failures block acceptance/promotion even after the tag became public; no success claim or rollback. |
| MP11 | Reports bind operation/version/tag, S/H, run/attempt, body digest, sums and gate results. Failure reports identify failed gates; only successful gates produce success results. Full-run success is required for acceptance, but reports never authorize another run. Attempt-specific diagnostic artifacts survive failures; no tokens/notes in error logs, no full-body JSON duplication or retention-dependent release gate. |
| MP12 | Narrowly replace obsolete source-note/verify_published/tag-push expectations, preserving all other quality gates. Behavioral workflow tests reject wrong checkout/root, privileged source execution, skipped mandatory dependency, wrong permission, implicit release/tag creation, product-latest changes and wrong write order; merely finding command names in YAML is insufficient. |

Policy/state/API tests use local Git fixtures and injected GitHub responses,
not live publication or real credentials. Mocked success is not evidence that a
module was published. Detailed per-line coverage of new helper code must reach
95% minimum; preserve full Runtime Go new-file/module gates. Any coverage
follow-up remains subject to repository approval rules.

After reviewed implementation/CI, live #151 acceptance still requires two real
successful publication runs for v0.6.0-rc.1 and v0.6.0 at
the approved S (bf161c7cb6bf54b1ae6f8614e452963a5ddad36f; explicitly approved
after the initial source's ref-create permission denial),
using GITHUB_TOKEN, including public-proxy/sumdb and clean
consumer proof. No test creates/deletes a throwaway public tag. Do not close #151
when the automation PR merges.

The first approved real RC run qualifies platform permissions; no throwaway tag
or API permission dry-run is assumed. Explicit ref creation precedes creation
of the release by existing tag, without target_commitish. Tests reject implicit
tag creation and permission fallbacks; a failure reports exact partial state.

The user approved replacing the existing source-owned RELEASE-version.md and
verify_published/tag-push expectations with this contract. That narrow update
is implemented; other quality requirements and historical notes are preserved.

## Local implementation evidence (2026-10-16)

### Approved read-after-write regression checks

Use detached API snapshots. Independently delay release-list visibility, asset
lists, the draft-to-published transition and published lookup. Assert exactly
one draft, one upload per asset and one publication; pin the returned release ID.
Also cover a resumed draft, repeated disappearance of previously observed assets,
conflicting metadata/IDs during lag, malformed successful creation responses,
lost write responses with a stale list, and permanently stale state. Each must
either converge with exact bytes or stop without duplicate/replacement writes.
409/422 without observed progress must not retry the same write. Exercise the
actual CLI publish path so its final report cannot accidentally start another
unpinned lookup. Existing lost-response and conflict tests stay intact.

The policy, CLI, semantic workflow wiring and existing coverage/attestation/
bundle suites ran together: 43 tests, 42 passed, 0 failed, 1 existing Windows
symbolic-link test skipped because the host cannot create those links. Linux
CI retains that test unchanged. New publication tooling line coverage is 99.66%
(policy 99.58%, CLI 100%); function coverage is 100%. The only uncovered line
is the bounded-pagination exhaustion error. An LCOV report was generated;
ordinary CI enforces >=95% and retains that detailed report.

This proves local automation behavior, not a new module release or a successful
GitHub write. No tag/release/setting was changed; no runtime Go code was changed.
Real RC/GA source, Go/race/fuzz/staged/public-consumer gates remain required.

### Read-after-write correction (2026-10-17)

All six new regression groups failed against the prior implementation before
the fix. The complete suite now has 49 tests: 48 passed, 0 failed, the same
existing Windows symlink skip. Detailed LCOV: policy 99.64%, CLI 100%, combined
99.70% lines and 100% functions. The only uncovered line remains pagination
exhaustion, not a new recovery requirement. No workflow, Go code or dependency
was changed. #153's real CI passed; the partial RC run is not release acceptance.
