# #89 working-instance handoff: storage amendment

Status: approved by @evilguest on 2026-11-07 after critical/repeat review.
The [component flow](issue89-working-instance-handoff-design.md) is approved.
The separate [test inventory](issue89-working-instance-handoff-tests.md) and
OT-WH01–OT-WH03 are approved. Implementation/verification is in progress;
the shared-cascade correction in §4 was separately approved on 2026-11-08.

## 1. Smallest persistent representation

Extend prepare_liquibase_session_disposals instead of adding another table.
Add reason `instance_transferred` and six nullable TEXT columns:

| Column | Meaning |
| --- | --- |
| handoff_job_id | Exact session job; deferred FK to prepare_liquibase_job_finalizations(job_id), ON DELETE CASCADE. |
| handoff_instance_id | Immutable historical instance reference. |
| handoff_state_id | Exact last published output state, not the session's original input. |
| handoff_operation_ref | Existing managed runtime operation; its Source is unchanged. |
| handoff_runtime_dir | Exact existing operation target / instance runtime directory. |
| handoff_outcome_digest | Exact immutable prepare_success digest, also authority_ref. |

Reuse existing session_id, runner_id, db_runtime_id, requested_at and
runner_absent_at. The receipt derives clone/job/generation/execution identity
from the immutable session and clone rows; do not duplicate them in columns.

For the new reason, all six fields and both physical IDs are nonnull/nonempty,
the digest is canonical SHA256, and db_absent_at/nonstart fields are always NULL.
For every other reason all six fields are NULL and existing rules remain.
The runner_absent_at CHECK accepts either genuine db_absent_at or this fully
bound transfer reason. It is not a general exemption for a reason string.
Existing UTC and exact-ID checks remain. New identity fields are immutable;
only the existing single-assignment runner_absent_at can progress.

No permanent FK to instances, states or operations is added for historical
handoff values: it would block ordinary instance/state deletion after completion.
The existing instance publication pin protects the live transfer until terminal
completion; historical references are validated as history, not current owners.

## 2. Atomic selection/publication

The trusted writer uses the existing owned finalization connection/settlement
mechanism. One transaction performs the following without external I/O:

1. Revalidate the exact stopped parent, completed successful last invocation,
   final published cursor/seal, job/execution/image/identity and verified access
   binding for the same physical database. No pending SQL, other active owners,
   previous finalization, accepted cancellation or selected failure is allowed.
2. Insert the transfer disposal row, including future finalization job/digest.
   The deferred FK prevents committing this row without its matching outcome.
3. Insert the matching immutable prepare_success intent and existing instance pin,
   then the exact instance/use-count and prepare_instance completion, using the
   same ordering and deferred pin FK as existing publication.
4. Verify the full joined tuple before commit. The result instance runtime ID,
   target, physical identity and operation must be the same working allocation.
   The operation Source equals the original session input; the final state
   equals its independently verified final cursor. Do not rewrite Source.

Selection guards permit only this exact transferred session's still-pending
private owners; all other existing inventory exclusions remain. The intent
insert must match the transfer's job/instance/state/digest, and instance insertion
must match its runtime/target/operation. A transfer may only be inserted before
selection in this transaction; it cannot be attached to an old outcome later.
Lost-COMMIT settlement compares all six fields, physical IDs, full outcome/pin,
instance/access/operation, state count and task completion. Partial readback
cannot report success. Existing outcome ordering is not relaxed.

## 3. Resume, private disposal and retirement

Publication may need to resume the snapshot-stopped database after the parent
has stopped. Extend only the existing restart-request/ready guards for this
final-success, stopped-parent case; persist request before resume. The final
cursor/invocation and absence of cancellation/failure are required. This never
permits session_exec: a stopped parent still cannot accept another invocation.

Private retirement requires a known committed joined transfer and live matching
instance pin, plus fresh exact parent/output absence observed outside SQLite.
Do not write DB-absence facts and do not call database/file deletion. The private
absence record, capture-slot retirement, session runner retirement/cursor clearing
and logical clone release follow the same single-assignment/atomic boundaries
as current retirement, with this separate exact ownership proof. Released means
released from job ownership here, not physical database absence; the receipt
preserves the distinction. Immutable historical IDs remain available.

Terminal completion requires the transferred session's private owners retired
and all other normal prerequisites satisfied. It releases only the existing
publication pin; it does not clear or mutate the historical transfer. Job deletion
can cascade terminal history, but active transfer/private ownership blocks it.
No instance/state lifetime is extended after normal completion. Instance deletion
remains the sole ordinary owner of runtime/directory removal after handoff.

## 4. Format and installation

### Approved narrow cascade correction (WH06 evidence)

The new real queue/Docker tests reach successful handoff but terminal job DELETE
fails with `decision evidence link is retained`: the common
prepare_decision_evidence_delete_guard checks the still-present decision during
an evidence cascade, even though its parent job has already been removed.
Correction **approved by @evilguest on 2026-11-08**: retain the link while
its decision's job exists (join prepare_attempt_decisions to prepare_jobs),
rather than merely while the decision exists. Direct history deletion while
the job exists remains forbidden; the existing job-delete guards run before
any cascade and still reject active/protected work. No table/FK/API change.

Review: requiring the parent job avoids relying on sibling-cascade order.
It does not allow deleting a decision/evidence link from a live job, or crossing
job identity; existing exact-scope and append-only guards remain. Repeat review
found no further P0–P2 issue. This shared guard change and its exact artifact
expectation/provenance update received separate approval beyond OT-WH02; WH06's
already approved terminal-deletion test is the failing requirement, not a new
test-plan expansion. See the [decision record](../adr/2026-11-08-decision-evidence-terminal-cascade.md).

### Component format

Advance the session component's store format from 2 to 3; capture helper protocols
remain private/v2. Update canonical SQL and regenerate its exact catalog using
the existing approved single-source mechanism. Fresh/current-target installation
and the exact released-baseline bridge remain; no migration from the unreleased
prototype/PR checkpoint is introduced. Unsupported/partial/altered definitions
are rejected before writes, inventory or workers. Do not silently repair v2 or
claim unfinished-job compatibility. Updating old version expectations requires
the later explicit old-test review/approval, not this schema approval alone.

## 5. Review corrections and repeat review

- P1: permanent receipt FKs would pin deleted instances/states forever. Keep
  historical values and the existing bounded live publication pin instead.
- P1: a transfer row alone must not commit. Use a deferred finalization FK and
  exact insertion guards on both intent and instance, with full settlement.
- P1: original operation Source differs from final output. Validate both exact
  identities separately; never rewrite provenance or weaken the cache-hit path.
- P1: final publication needs a restart with a stopped runner. Add only the
  guarded publication-only restart route, not a new SQL allowance.
- P1: broad inventory/absence exemptions could delete a live DB. Require the
  full transfer tuple and pin; private absence never means database absence.
- P2: partial nullable variants and ambiguous versions. Use all-or-none checks,
  immutable identity, exact format 3 and fail-before-write classification.

Repeat review found no additional P0–P2 schema-level contradiction. Tests and
limited old-fixture changes are listed separately in the proposed test inventory.
