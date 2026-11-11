# #89 O5: retained planning DB and first-session transfer — storage

Status: proposed schema after critical/repeat review; approval requested.
[O5 components](issue89-measured-overhead-reduction.md) are approved. No DDL,
canonical artifact, product writer or old schema test has been changed yet.

## 1. Representation and data ownership

Add one table `prepare_liquibase_planning_db_ownership`; do not make another
physical clone, rename a DB or copy the operation into a fictional new owner.
Reuse the existing append-only `prepare_attempt_evidence` for exact observations.

| Field | Constraint / meaning |
| --- | --- |
| job_id | TEXT PK, FK prepare_jobs(job_id) ON DELETE CASCADE. |
| task_id | TEXT NOT NULL CHECK = 'plan'. |
| attempt_no | Positive integer; composite job/task/attempt FK to prepare_step_attempts ON DELETE CASCADE. |
| clone_id | TEXT NOT NULL UNIQUE; FK prepare_clones(clone_id) ON DELETE CASCADE. |
| retained_evidence_id | Positive UNIQUE FK prepare_attempt_evidence(evidence_id) ON DELETE CASCADE. |
| plan_digest | NOT NULL canonical SHA256; immutable saved plan identity. |
| session_id | Nullable UNIQUE TEXT; FK prepare_liquibase_sessions(session_id) ON DELETE CASCADE DEFERRABLE INITIALLY DEFERRED. |
| first_task_id | Nullable TEXT; composite job/task FK to prepare_tasks. |
| first_execution_digest | Nullable FK prepare_execution_bindings(execution_digest). |
| transferred_at | Nullable validated UTC, single assignment. |
| disposal_requested_evidence_id | Nullable UNIQUE FK prepare_attempt_evidence(evidence_id) ON DELETE CASCADE. |
| disposal_evidence_id | Nullable UNIQUE FK prepare_attempt_evidence(evidence_id) ON DELETE CASCADE. |
| disposed_at | Nullable validated UTC, single assignment. |

The four transfer fields are all NULL or all non-NULL. Transfer excludes any
disposal request/evidence/time. Disposal evidence and disposed_at are both NULL
or non-NULL, and require a prior disposal request. A request with no completion
is explicitly pending; it is not proof of absence. No mutable phase column is
needed: retained / transfer / disposal-pending / disposed derive from these
all-or-none fields. Base identity fields never change; transition fields cannot
be cleared, replaced or redirected. A no-op update does not grant new authority.

Full DB ID, runtime name, operation_ref, target_path, source and identity are
already pinned by the immutable retention evidence plus clone/managed binding.
Do not duplicate these values in this table. In particular do not add a permanent
FK from historical evidence to managed_runtime_operations, states or instances;
normal terminal instance/state deletion must still work.

Indexes: the job PK and clone/session/evidence uniqueness support exact lookups;
add a partial job index for session_id IS NULL AND disposed_at IS NULL to enumerate
active retained owners without scanning history. No new retention TTL or worker.

## 2. Exact facts in the existing journal

Three closed observation kinds extend the existing bounded/hash-checked facts:

- `planning_db_retained`: exact job/task/attempt/clone, worker generation,
  planning execution digest/fingerprint, full runtime ID/name, operation_ref,
  target_path, managed identity and original input kind/ID. It records successful
  read-only observations of the same live ready DB/verified operation after the
  exact planner exited successfully. It is **not** db_quiescence/runtime_absent.
- `planning_db_disposal_requested`: links that retention evidence and immutable
  plan, with one reason/authority: source_mismatch, full_cache, accepted_cancel
  or selected_failure. Source mismatch names the actual first task/binding/input;
  full-cache proof uses real completed cached task/state bindings; cancellation
  and failure reference their existing saved authority. An arbitrary reason
  string is not sufficient. Zero-change plans never enter retention.
- `planning_db_disposed`: links exact retention and request evidence and records
  fresh confirmed full-ID absence, retired managed operation and absent private
  work directory. There is no transfer/publication in this branch.

Each fact must share the owning planning attempt's exact scope/generation and
canonical digest. Preserve existing 8192-byte and append-only guards. The runtime
does external observations before the write transaction through the trusted
verifier, with typed expected IDs; SQL rechecks the current exact owner/identity.
No fact containing a flag supplied by CLI/API can replace native observation.

## 3. Complete planning while retaining a live DB

A new narrow `CompleteRetainedPlanning` writer uses the existing owned-connection
settlement mechanism. Only normal running prepare, non-empty verified plan,
private/v2 successful stopped planner, matching active preview and no prior
working history/cancellation/finalization qualify. This normal-path amendment
accepts planning attempt 1 only; it does not admit a recovered/replayed second
attempt. The plan's output and ordered
evidence remain exact: successful executor inspection < planning_db_retained <
planning_output. The existing saved-plan bytes/digest must match the capture.

One transaction inserts the ownership row, records the complete_planning decision
with exactly these three evidence links, closes the exact planning attempt as
planning_result_saved and completes its stable task. The existing resolution
guard gains this **separate exact alternative** to the old absence-based proof.
The absence branch remains unchanged. Clone phase stays active with the same ID;
neither db_quiescence nor released is fabricated for the live DB.

A failed/ambiguous commit returns success only after full owned-connection
settlement of row, all facts/decision links, full plan, attempt/task and managed
operation/clone. Partial readback is not enough. Before this transaction, the
original active planning clone still protects the runtime even if a plan or
retention observation was already saved. If cancellation/failure wins first,
retained completion is refused and original planning cleanup remains responsible.

Saved private planner output may then be disposed under plan_saved with a
separate exact retained-ownership alternative to DB absence. Observe the exact
stopped planner/output/input cleanup as before. Keep the retained DB, source,
directory and operation pinned; ReleasePlanningClone remains forbidden while
ownership is retained or transferred to a live session.

## 4. Atomic transfer to one first session

New `TransferPlanningDBToFirstSession` uses a single owned transaction, no Docker
or files inside it. It requires the retained row with no disposal request,
matching original source and first **uncached** actual running state_execute
task/binding, no prior session/application attempt, no cancellation/finalization,
and exact completed planning/task/output evidence. Job must still be running
ordinary prepare, not recovery_required, plan-only or cache explanation.

1. Assign the row's future session_id, first_task_id, first_execution_digest and
   transferred_at, checking the complete tuple. All are single assignment.
2. Insert one planned runner session pointing at the **existing** active clone,
   its same generation/physical runtime/source and initial cursor. Insert the
   usual helper lease/config binding; do not INSERT another clone or advance
   clone_generation solely to transfer ownership.
3. Verify the joined row/session/first binding/plan/clone/managed operation.
   Deferred FK prevents a committed transfer without its session. Matching
   insertion and transition guards prevent an unrelated session satisfying it.

Any session INSERT referencing a clone ever used by a planning attempt requires
this exact committed-in-the-same-transaction transfer branch. The generic
source-matching session guard alone must not permit adoption when the ownership
row is missing, still retained or points at another future session.

Only after known commit or full readback can the same runtime be passed to the
normal working runner. Normal session readiness and first-invocation admission
still happen before SQL. A second session or a new clone cannot substitute for
uncertain transfer. The old first-session recovery proof keeps its released-
preview requirement; it is **not** broadened to launch SQL from retained history.

All normal cursor/snapshot/success/failure/cancellation and working-instance
handoff operations continue to use the same session/clone. The transfer row is
historical once session ownership takes over. Planning history cannot reclaim
that DB for cleanup; owner enumeration must deduplicate this exact shared clone
rather than treat planning and session as two physical allocations.

## 5. Non-reuse, deletion and crash boundaries

For a cached prefix/different source or full cache, save the exact disposal
request before external destruction. A concurrent transfer and disposal request
cannot both commit. The planner owner then retires the exact managed operation,
deletes the exact DB and directory, and observes full absence outside SQLite.
One settled transaction links the disposal fact/time and releases the clone.
Only after that known completion can the existing new-source/session path start.

After a disposal request, cancellation/failure may choose the final job outcome
but cannot clear the request, redirect deletion or authorise SQL. Cleanup may
finish using that existing exact destructive basis. Never force a live transferred
session through this branch, even after it subsequently retires normally.

Job deletion/terminal success/failure refuse active retained or disposal-pending
owners and any existing live session/private owner. Source deletion/eviction and
operation mutation/retirement obey the live owner/pin rules. The exact disposal
path may retire an operation only under its committed matching request; this
exception does not permit changing Source, target or runtime identity.

Finalization's effect/fact guards permit only exact disposal-request/completion
facts for an already retained owner after saved cancellation/failure. They still
forbid creating a new retention, transfer, session or application attempt after
outcome selection. No broad whitelist for all new observation kinds is allowed.

Transferred history itself is not an eternal pin: after normal session retirement
and the existing instance handoff/finalization protocol, ordinary instance/state
deletion works. Direct history/evidence/link deletion while the parent job exists
remains blocked. Terminal job cascade is allowed after the parent job row is
gone; guards must not depend on the order of sibling FK cascades.

Crash before retention commit: original preview owner. After retention commit:
retained owner. During ambiguous transfer: one of retained/session owners after
settlement, with no speculative SQL or cleanup. After transfer: session owner.
After disposal request: pending planner cleanup. After physical deletion but
before its commit: retained pending owner, not a source eligible for SQL. No
automatic restart/replay/replacement or expanded recovery implementation is added.

## 6. Installation and bounded change surface

Session component format advances **3 → 4**; common attempt=3, Liquibase binding=2,
finalization=2, cache selection=1 and helper manifest protocols remain unchanged.
The exact canonical catalog includes the new table/index/guards and the narrowly
amended planning, session, disposal, terminal/deletion and active-owner predicates.
No change to generic psql semantics or generic clone identity immutability.

Fresh installation and the already-approved exact released-baseline bridge use
the final definitions directly. There is no migration from unreleased #89 format
3, implicit repair or upgrade of its unfinished jobs. Reject unsupported/partial/
altered catalogs before writes/workers/probes. Historical source hashes remain
historical; target hashes/version assertions change only through the subsequent
explicit test inventory. No product DDL is authorised until schema approval.

## 7. Critical and repeat review

Corrected: a retained live DB must not be marked quiescent/absent; a transfer
cannot commit without a matching session; plan/session references cannot become
two destructive owners; incompatible cursor and recovery_required cannot launch;
disposal intent precedes physical effects; COMMIT readback retains the physical
connection and full tuple. Historical operation FKs would pin deleted instances
forever, so reuse journal facts and bounded live guards instead. Cascade guards
check parent-job existence to avoid the already observed sibling-order failure.

Repeat review checked every all-or-none transition, cancellation/failure order,
fact scope/order, late/uncertain commits, source/path protection, private capture
retirement, normal published-instance deletion and unchanged released installation.
No remaining P0–P2 schema-level issue found. Tests and exact old-test corrections
are the next separate approval boundary, not implied by this proposal.
