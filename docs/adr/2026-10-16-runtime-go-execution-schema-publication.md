# Execution-schema module publication (#151)

- Conversation timestamp: 2026-10-16T17:49:58+07:00.
- GitHub user id: @evilguest (41718235).
- Agent name/version: Codex / GPT-5.
- Status: direct-source direction accepted on 2026-10-16T18:35:36+07:00;
  detailed workflow/test inventory subsequently approved (record below).
  Implemented locally; no tag or release is claimed.

## Decision 1: additive minor version

Which immutable module version should publish the execution-environment facade
merged by PR #152, and what separates implementation from release acceptance?

## Alternatives

1. v0.5.1: treat the added public facade and inspection API as a patch.
2. v0.6.0: publish the additive public API as the next minor release, using the
   existing release-candidate and final-release process.
3. v1.0.0: declare a stable major contract, broadening this issue into a separate
   compatibility/stability decision.

## Version proposal

Use v0.6.0 for the additive API and v0.6.0-rc.1 as its candidate, at the same
reviewed library source S. The requested target is PR #152's existing merge
892a5007adf4d499fbfbd7861569861ff11c43a8, not a new release-metadata descendant.
The token path and remaining live-permission risk are clarified in Decision 4.

Never move/reuse an existing tag or overwrite immutable assets. A source fix
requires a new reviewed commit and a new candidate; harness failures must be
diagnosed without silently changing the already tagged source. Close #151 only
after the final release gates and publication report pass.

## Rationale and existing decisions

The change adds production API, not just a bug fix. It does not alter existing
canonical bytes/domains, resolver/cache wire contracts, or Go dependencies.
The reviewed module release machinery already requires an RC at the exact final
source commit. A stable major release is unnecessary for this scoped task.

## Decision 2: publish an existing merge without release-readiness PRs

- Conversation timestamp: 2026-10-16T18:35:36+07:00.
- GitHub user id: @evilguest (41718235); agent: Codex / GPT-5.
- Status: direction accepted; detailed contract and MP01-MP12 checks proposed.
- Question: should every release require a new source commit merely to carry
  a version-specific note file?
- Alternatives: retain that metadata PR; manually bypass the release workflow;
  or evolve the shared workflow to publish explicit S using trusted automation H
  and externally supplied release notes.
- Decision: choose the last alternative. The one-time automation change is
  reviewed normally; future versions need no readiness PR, source note file,
  new library commit or version-specific roadmap edit. The former unapproved
  metadata-PR plan is withdrawn, not an additional task to complete.
- Proposed concrete structure: manual validate/publish/verify modes, separate
  H/S checkouts, bounded Markdown input, exact successful source CI, protected
  create-only tags, resumable matching drafts, immutable published output,
  public consumer verification and attempt-specific diagnostic reports.
  Final promotion fully reverifies the published candidate in the current run,
  not merely its tag or a previous run's artifact. Tooling from H targets S.
  Decision 4 clarifies the GITHUB_TOKEN route without changing S; no live
  publication or successful write-permission check is claimed.
- Rationale: release metadata is not library source. Separating automation from
  S also permits harness corrections without changing a published module.
  Failures/conflicts must not become permission to retag or overwrite assets.

This replaces the invocation/file-location portion of Decision 6 and the
verify_published input in Decision 11 of the
[original release ADR](2026-09-23-runtime-v2-declarations-and-release.md).
It preserves immutable tags, successful same-source RC/final promotion,
public-proxy/sumdb consumption, least privilege and the historical product
boundary. No new runtime identity/cache/DB contract is introduced.

The previous v0.6.0 alias-publication proposal is already obsolete; no such tag
exists. Reusing the next unused version number for #151 does not revive that
proposal, publish a YAML adapter, or supersede the accepted
[client alias boundary](2026-10-03-runtime-v2-client-alias-boundary.md).
The #150 [facade decision](2026-10-15-runtime-v2-execution-schema-facade.md),
its [approved corrections](2026-10-16-runtime-v2-execution-schema-review-corrections.md),
and existing immutable-release safety rules remain in force. The invocation
supersessions above are explicit; historical published source is not edited.

See the reusable [publication contract](../architecture/runtime-go-module-publication.md),
[automation checks](../architecture/runtime-go-module-publication-tests.md)
and [feature publication checks](../architecture/runtime-v2-execution-schema-tests.md#publication-checks-151-proposed).

## Decision 3: review corrections and the remaining publication prerequisite

- Conversation timestamp: 2026-10-16T21:20:31+07:00.
- GitHub user id: @evilguest (41718235); agent: Codex / GPT-5.
- Status: review corrections requested; detailed contract/tests proposed.
  No credential configuration, new source selection or publication approved.
  The credential/prerequisite conclusion below is superseded by Decision 4;
  the other corrections remain applicable.
- Question: how to retain exact-source publication without artifact-retention
  coupling, frozen tooling, partial-write ambiguity or assumed token capability?
- Alternatives: authorize promotion from an archived run receipt or freshly
  verify the candidate; execute validators from S or H; assume GITHUB_TOKEN
  supports older workflow-changing commits or expose its permission/event gap.
- Proposed decision: use candidate_version and mandatory current-run full RC
  verification. Reports remain diagnostics, not a cross-run authorization
  protocol. H owns algorithms with explicit S roots; S owns source/tests and
  expected data. Validate UTF-8/serialized sizes and complete draft-visible
  state before the public tag; resume only matching partial drafts, never
  rewrite published output. Preserve the product latest-release pointer.
- Remaining choice: old S requires a concrete supported credential/event
  isolation route. A separate token is not automatically authorized and may
  start S's legacy workflow. Using the automation PR's reviewed merge as S
  avoids that legacy workflow without an extra version-specific PR, but changes
  the requested SHA and needs approval. Do not silently choose either route.
- Rationale: a current full check is simpler than archived-proof validation;
  tooling fixes must not change immutable module source. A draft does not hide
  a public Go tag, and mocks do not establish GitHub token capabilities. Record
  the unresolved prerequisite instead of claiming a publishable design.

## Decision 4: retain the chosen source and qualify the tag-first token path

- Conversation timestamp: 2026-10-16T22:09:28+07:00.
- GitHub user id: @evilguest (41718235); agent: Codex / GPT-5.
- Status: user acknowledged the correction and retained-source direction;
  detailed workflow/test approval and actual publication are still pending.
- Question: does a commit preceding the automation merge necessarily require
  different credentials or a different release source?
- Alternatives: replace S with the automation merge; add a PAT/App token with
  legacy-event isolation; or retain S and use GITHUB_TOKEN with an explicit
  tag-first path, treating actual permission failures as failures.
- Proposed decision: retain S=892a5007adf4d499fbfbd7861569861ff11c43a8 and
  contents:write on GITHUB_TOKEN. Create the exact protected tag through Git
  Refs, verify S, then create the release by existing tag with no
  target_commitish. No implicit tag creation, alternate target/token, test tag,
  repository setting change or unconditional write-path blocker is introduced.
  The first approved real RC run qualifies platform behavior. A refusal stops
  with accurate partial-state evidence; changing credentials or S needs approval.
- Rationale: "older" describes history, not elapsed time or a blanket HEAD
  restriction. GitHub's original permission announcement includes the absence
  of an existing ref, while release parameters say target_commitish is unused
  for existing tags. Our route already separates those operations; this
  supports implementation, not a guarantee that a real write has succeeded.
  A mocked/read-only permission dry-run cannot replace live acceptance.
- Sources: [permission announcement](https://github.blog/changelog/2023-11-02-github-actions-enforcing-workflow-scope-when-creating-a-release/),
  [release API](https://docs.github.com/en/rest/releases/releases),
  [Git Refs API](https://docs.github.com/en/rest/git/refs).

## Approval of the detailed design and test inventory

- Conversation record timestamp: 2026-10-16T22:44:17+07:00.
- GitHub user id: @evilguest (41718235); agent: Codex / GPT-5.
- Question: approve MP01-MP12 and narrowly replace obsolete source-note and
  tag-trigger expectations while retaining the original source SHA?
- Alternatives: keep those contradictory expectations, revise the design,
  or implement the reviewed manual contract and its tests.
- Decision: the user approved the last alternative. Tests were constructed
  before implementation; existing Go/vector/quality requirements are retained.
  The helper/CLI and read-only composite action implement the approved H/S
  split. Detailed local evidence is in the publication test inventory.
- Rationale: no version-specific readiness PR is needed, while real RC/final
  publication remains a separately verifiable #151 acceptance step. Approval
  does not imply that a tag, release or GitHub setting has been changed.

## Decision 5: approved source adjustment and read-after-write recovery

- Conversation record timestamp: 2026-10-16T23:55:25+07:00.
- GitHub user id: @evilguest (41718235); agent: Codex / GPT-5.
- Question: how to continue after a real ref-create permission denial and
  subsequently duplicated drafts caused by delayed GitHub list visibility?
- Alternatives: change credentials; explicitly use the reviewed automation
  merge without changing Go code; or abandon publication. For consistency,
  retry creation from lists, add durable receipts, or pin IDs and remember
  attempted/observed progress within one invocation.
- Decision: the user explicitly approved S=bf161c7cb6bf54b1ae6f8614e452963a5ddad36f.
  Only module README differs from the originally requested #152 source;
  Decisions 1/4's exact-source selection is superseded, not tag immutability.
  The RC tag already points to this S. Fix automation through a real reviewed
  PR, using pinned release IDs and bounded read-only convergence, with the
  approved stale-list/asset/state and lost-response regression checks.
  Do not repeat the same write on uncertainty. There is no durable exactly-once
  draft guarantee across invocations; reconcile ambiguous creation first.
  The user authorized deletion only of empty duplicate draft 415176820 after
  fresh identity/draft/asset checks, retaining 415176803 and the existing tag.
  Retry the RC and publish GA only after its complete successful qualification.
- Rationale: successful write responses must not be forgotten because a list
  lags. This is a genuine automation correction, not a release-readiness PR.
  No credentials, source code, immutable coordinate or release-body bytes change.
