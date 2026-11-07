# #89: reuse the working database as the result instance

Status: component amendment approved by @evilguest on 2026-11-07 after critical/
repeat review. This is not an approved schema or test inventory.

## Evidence and scope

The current normal worker retires the complete working database and starts
another clone of the final snapshot in publishNormalLiquibaseInstance. Real v1
publishes its existing working database instead. In a diagnostic jhipster run,
the interval after the last successful checkpoint was 18.185 s new / 0.751 s v1.
The first working task also pays for a separate database after preview; that is
a distinct opportunity, not included in this amendment. Neither observation is
a completed statistical speed qualification or a promised optimization gain.

Propose removing only the unnecessary final database replacement after an
uncached suffix. Pure cache hits still start from the verified cached state.
Plan-only and cache explain keep their existing preview disposal. No preview
adoption, #104 planning shortcut, image-inventory cache, persistent JVM/callback,
psql change, public CLI/API change or automatic replay is included.

Alternatives: keep destroy/recreate; reuse the database using only an in-memory
flag; transfer exact durable ownership atomically with instance publication.
The proposed third option retains visible ownership on every failure boundary.

## Packages, responsibilities and data ownership

- prepare owns the in-memory working handle and sequences final checkpoint,
  exact runner stop, database resume/verification, existing access activation,
  publication, private-output cleanup and terminal completion.
- prepare/queue owns the persistent transfer and instance publication transaction.
  A new narrow writer consumes the exact session/clone/last invocation, managed
  runtime operation and final state; it returns a typed publication/transfer
  receipt only after known commit or complete existing settlement verification.
  The schema representation and SQL guards require separate design approval.
  The runtime operation's immutable Source remains the session's original input;
  the final output state is linked separately, never by rewriting that provenance.
- instanceaccess retains existing persistent credential provisioning/activation.
  The result uses an instance credential, not the working bootstrap password.
- runtime performs exact stop/read/absence operations outside SQL transactions.
  It does not decide logical ownership. The live database becomes instance-owned;
  the Liquibase parent and its private bytes remain disposal-owned until removed.
- The existing finalizer retains the publication pin through private/projection
  cleanup and terminal completion. No new public job state is introduced.
  Keep the existing runtime pathname even if it is below jobs/<job>; no live
  directory rename/copy is required. Instance/runtime-operation ownership must
  protect that path from job/private cleanup until ordinary instance deletion.

## Normal sequence

1. Require the exact final published cursor, successful complete last invocation,
   final checkpoint/managed seal and matching execution identity. No SQL remains.
2. Stop the exact Liquibase parent and persist the existing stop proof. Do not
   remove its retained private output or the database yet.
3. Resume the same database if snapshotting left it stopped; verify its managed
   identity and use the existing access activation protocol for the result.
4. In one transaction publish the instance, immutable success intent and pin,
   and transfer this exact runtime/directory/operation out of job ownership.
   Accepted cancellation or a previously selected failure prevents this choice.
5. Remove only the stopped Liquibase parent/private output under the committed
   transfer receipt, observing exact absence. This branch proves durable instance
   ownership instead of falsely asserting database absence. It cannot call the
   ordinary database-stop/delete or working-directory removal path.
6. Retire the session's execution/private owners, remove disposable projections,
   then complete the existing terminal transaction and release the pin. Keep
   the instance, its runtime directory and every checkpoint intact.

## Failure and concurrency boundaries

Before known transfer commit, the runtime stays job-owned, including after
credential activation. Unknown commit, failed verification, failed stop or
failed cleanup retains exact visible ownership/recovery_required; no new SQL,
alternate instance, blind retry or premature directory deletion is permitted.
After known commit, the instance and its pin protect the runtime while private
cleanup is incomplete. Existing outcome ordering remains immutable. Crash
recovery execution is still #170, not added here; safe ownership is mandatory.

Deletion must use the same persistent publication protection as existing
publication. The transfer receipt is queue-derived, not a recipe flag or a
caller-supplied assertion. Pure hits have no working session and cannot consume it.

## Critical review and repeat review

Corrected P1 risks: a transferred live DB must not be called absent; stop proof
must precede activation; activation/COMMIT failure must not fall through to
destructive cleanup; all four ownership/publication records must commit together.
Also corrected the P1 path hazard: a job-shaped pathname does not grant job
cleanup ownership over a transferred, instance-referenced runtime directory.
Corrected P2 risks: cache hits cannot adopt a preview as the final state; this
amendment must not silently import preview handoff, #104 or recovery. No remaining
P0–P2 contradiction was found at component level. Exact storage and tests remain
separate approval gates; no implementation or old-test changes are authorized.
