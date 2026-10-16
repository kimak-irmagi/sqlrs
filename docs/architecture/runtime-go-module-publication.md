# Runtime Go: publish an existing source commit

Status: direction, detailed structure and MP01-MP12 checks approved by @evilguest
on 2026-10-16 for #151. Automation PR #153 and its post-merge CI passed;
RC publication exposed a read-after-write defect, corrected below. Final
publication remains pending. This replaces the unapproved release-readiness
PR proposal, not the module's API or immutable-release policy.

## Operator contract

Run release-runtime-go.yml from main. The workflow revision H is pinned at
dispatch; the published library source is a separate, explicit commit S.

| Input | Contract |
| --- | --- |
| operation | validate (default, no publication), publish, or verify (existing publication, no release/tag writes). |
| version | Exact v0.MINOR.PATCH or v0.MINOR.PATCH-rc.N; no leading zeroes, rc.N starts at 1. |
| commit | Exact lowercase 40-hex S; no branch, tag, abbreviation or implicit latest. Must be in main with successful post-merge ci evidence for S. |
| notes | Markdown required by candidate validate/publish, at most 16 KiB UTF-8; empty for verify and all final operations. Data, never shell code. No RELEASE-version.md is required in S. |
| candidate_version | Exact RC in the final version's family, required for every final operation; empty for RC operations. Its publication supplies the final body. No run-id or artifact input. |

Candidate and final publication are two explicit invocations, with no PR, commit
or local checkout between them. Example: publish v0.6.0-rc.1 from S; after that
run succeeds, publish v0.6.0 from S with candidate_version=v0.6.0-rc.1. verify
checks an existing immutable publication after a harness-only failure. Final
operations reverify the RC in this run; past CI artifacts are not permits.

## Interaction and authority

1. Check out trusted automation at H and source at S in separate directories.
   Reject non-main dispatch, invalid inputs, unqualified source CI, missing tag
   protection, unsupported source layout or incompatible module contract before
   any publication. Only explicit endpoint-specific not-found responses mean
   absence; ambiguous API/auth/rate/transport errors do not mean absent objects
   or successful CI.
2. Run Go tests from S and exact coverage gates, race/fuzz/conformance,
   dependency and staged-consumer checks against S using tooling from H.
   Run automation tests from H, without publication credentials in source jobs.
   Never test one tree while packaging another. The old-source case H != S,
   including S without the new helper or release-note file, is mandatory.
3. For a final version, the read-only verify-candidate job fully verifies the
   published RC: family, prerelease flag, exact S, protected tag, body/footer,
   deterministic assets, public-proxy/sumdb download, retraction/contract checks
   and clean consumer. Final publication needs this job's success in this run,
   not a tag or old report. A missing, draft, conflicting or failing RC blocks
   promotion. RC and final module checksums are checked separately, not equated.
4. Only publish has contents:write on GITHUB_TOKEN; no PAT/App fallback.
   Validate existing state and serialized payload sizes before
   creating a previously unused exact nested tag at S; never force/update/delete
   it. Create a draft release,
   attach and verify the existing deterministic source attestation and checksum,
   then publish it. Serialize same-version runs without cancelling active runs.
   Conflicting target, notes, release state or asset contents fail closed.
   Pending runs can be replaced by GitHub concurrency; no unlimited/FIFO queue
   is promised. Re-dispatching uses the same safe state checks.
5. Retry can resume a matching incomplete draft and upload missing assets only.
   Matching published output is verified without rewriting it. Changed notes,
   conflicting assets or another S must never be "repaired" by overwrite.
6. Verify the exact public module with GOWORK=off, public GOPROXY/GOSUMDB, no
   replace/sibling source, and the source-owned external-consumer suite. Check
   checksums, immutable tag target, attestation, release body, prior retraction
   and the existing contract markers. Keep bounded propagation retries. A failed
   check leaves the coordinate unchanged and does not produce success evidence.
7. After all gates, upload an attempt-specific diagnostic report and summarize
   version, S, H, checksums and run link. Overall workflow success is required
   for acceptance. Reports are evidence, never authorization or required input
   to another run. Preserve detailed diagnostic artifacts on failure.

The release body is supplied Markdown plus a generated, reserved provenance/
compatibility footer naming S and the existing historical/current contracts.
User notes cannot impersonate that footer. Final publication inherits the exact
verified candidate body; version/title/tag remain separate GitHub metadata.
Check UTF-8 byte counts, not JavaScript character counts: notes <=16 KiB, complete
body including the reserved footer <=20 KiB, and each serialized release-write
JSON payload <=128 KiB. Compute these before the first write, including retries
and final promotion. Escaped/control characters and multibyte text cannot turn
accepted input into a post-tag size failure. Reports store the body digest,
not another escaped full copy; report expiry never blocks promotion.
Notes and dynamic release evidence live in GitHub Releases/Actions, not a new
source commit. Future releases require no version-specific PR or roadmap edit.

## Token policy and platform risk

The new workflow becomes manual-only. S may contain an earlier tag-push workflow,
but GITHUB_TOKEN-created events do not start it. Use that token with explicit
contents:write, not a PAT/App token. There is no token fallback, test tag or
repository-setting change in this design. Source jobs and verify stay read-only.

GitHub documents extra workflow permissions for some release creation paths;
its original announcement specifies a workflow-changing target SHA without an
existing branch/tag ref. Our path creates the exact tag first through Git Refs,
checks it resolves to S, then creates a release for that existing tag, omitting
target_commitish (documented as unused for an existing tag). Never let release
creation implicitly create a tag, and never use a different target to bypass a
permission failure. Do not infer an age/HEAD prohibition from this restriction.

This is the baseline implementation route, not proof of deployed permission
behavior. validate and mocked tests cannot dry-run authorization for real writes.
The first real RC publication, after approval and all checks, qualifies the route.
A ref-create denial stops before release creation; a later denial may leave a
public tag or draft and is reported as partial failure, never as success. Keep S
and all existing coordinates; require new approval for additional credentials
or a source change if an actual platform restriction occurs. No implementation
blocker is claimed solely from the earlier documentation inference. Historical
tags and product releases stay intact.

## Existing-state discovery and recovery

Before the tag write, the authorized publisher lists all release pages with
draft visibility and all assets for the matching release id. Public get-by-tag
cannot prove draft absence. Duplicate releases, insufficient visibility,
malformed responses and ambiguous errors block publication. A create/update
404 is an error, not evidence that the object was absent.

Distinguish: no tag/no release; matching tag only; matching draft with absent/
matching tag; matching published release; conflicts. A tag must resolve to S;
an untagged draft must explicitly target S. Before writes compare exact tag,
name, body, draft/prerelease state and every existing asset's bytes. Unknown
assets or incomplete published releases are conflicts. Creation/publication
sets make_latest=false, preserving the product's latest release. Matching drafts
may receive missing assets; only draft-to-published transition is allowed.
Never rewrite descriptions or uploaded assets. Published output is only verified.
Re-read affected state after a concurrent/conflict response; resume only after
the complete comparison succeeds. A lost response may have committed a write.
Faults after tag creation, draft creation, each upload, publication and public
verification must all retry through this same state plan.

GitHub responses are detached snapshots, not a consistent shared database view.
Pin a discovered/created release ID and read that ID directly; disappearance
from a later global list never permits another draft. Keep checking those lists
for conflicting IDs. Remember each observed tag/asset and each attempted write
within the invocation. Delayed lists or draft flags may require read-only
observations, never another create/upload/publish of that same step. At most
32 observations are allowed; lack of convergence fails without rollback.
Successful writes must not rely on a subsequent list to remember their identity.
An ambiguous failed write is not blindly retried, including 409/422: reread
and require progress before any different step. Reports preserve partial state.
Across invocations there is no durable idempotency key for draft creation: a
lost create response plus an invisible draft requires operator reconciliation,
not a claim of exactly-once creation from GitHub's eventually consistent lists.
Read-only verification refreshes discovered releases directly and can discover
published output by tag; absent/draft/incomplete output gets bounded read-only
rechecks, while conflicting metadata/bytes fail immediately. Publication returns
its fully verified result to the CLI rather than discarding it and starting a
second unpinned lookup. These are in-memory recovery rules, not report-based
authorization or a new public API.

A draft is not a Go publication barrier: the public tag itself exposes a module
coordinate. All source, candidate, body, payload and existing-state checks must
precede the tag. Later failure cannot justify rollback or tag reuse.

## Component structure and ownership

- release-runtime-go.yml owns explicit read-only/publish jobs, permissions,
  checkouts, source checks, bounded data transfer and dependency ordering.
- scripts/maintenance/runtime-go-release.mjs owns input/state policy,
  Markdown/provenance construction and a narrow GitHub API adapter. Policy
  decisions are testable without live writes; errors carry static context and
  do not print tokens or caller-supplied notes. Helpers are executed from H.
- runtime-go-release-cli.mjs adapts workflow environment/files to that policy;
  the local runtime-go-public-consumer composite action shares the read-only
  public-consumer steps between candidate and target jobs. Both belong to H.
- Validators, coverage tooling and attestation generator come from H with an
  explicit source root S; Git discovery must also use S, not the tool directory.
  H supplies algorithms; S supplies Go code/tests, consumer fixtures, manifest
  and expected conformance data. A harness fix in H must work without adding
  scripts to S or substituting H's library fixtures. The existing attestation
  format is unchanged; no new dependency or runtime wire format. Unsupported
  source layouts fail before writes.

Trusted repository administrators and automation remain the governance boundary.
Reports record content/run status; they do not cryptographically
protect against an administrator who changes the workflow or publication policy.

Diagnostic reports record module/version/tag, S/H, run/attempt, operation, body
digest, sums and gate results. No cross-run receipt schema/parser is introduced.
Artifact names include run_attempt. Source-job outputs are bounded data
revalidated by H, never executable commands or imported source code in the
privileged job. In-memory plans and temporary files hold only one bounded run;
GitHub tags/releases/assets and Actions artifacts are persistent records. No DB.

## Acceptance boundary

The one-time automation change requires a reviewed PR and CI. It need not be in
the library source S. After the old-source ref creation was denied, @evilguest
explicitly approved S=bf161c7cb6bf54b1ae6f8614e452963a5ddad36f (PR #153's
reviewed merge). Against PR #152's 892a5007adf4d499fbfbd7861569861ff11c43a8,
the module differs only in README, not Go source/tests, dependencies or vectors.
The RC tag already fixes S; automation corrections must keep it unchanged.
Publish v0.6.0-rc.1 and v0.6.0 at that same S. Close #151 only after final public gates
and its GitHub evidence report; merging automation does not close publication.
No automatic consumer upgrade, executor or application adoption is included.

Approved checks and local results are in the [test inventory](runtime-go-module-publication-tests.md).
Decision history: [ADR](../adr/2026-10-16-runtime-go-execution-schema-publication.md).
GitHub's [token event rules](https://docs.github.com/en/actions/concepts/security/github_token),
[release permissions and draft visibility](https://docs.github.com/en/rest/releases/releases)
and [Git ref permissions](https://docs.github.com/en/rest/git/refs) define the
external boundary; tests cannot grant missing token capabilities.
The [original workflow-permission announcement](https://github.blog/changelog/2023-11-02-github-actions-enforcing-workflow-scope-when-creating-a-release/)
also describes the existing-ref condition; it is not a live repository test.
