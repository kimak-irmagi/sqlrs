# #89: extract installation without recovery migrations

Status: component amendment approved by @evilguest on 2026-11-07;
NI01–NI08 were separately approved on the same date and the direct installer is
implemented. Production session wiring is connected; see the
[execution checkpoint](issue89-main-path-execution.md). Release qualification remains pending. This is a
delivery-boundary decision, not a change to the public CLI/API, resource-ownership
rules, private/v2 protocol, or approved physical table/trigger definitions.

## Evidence and problem

The accumulated implementation opens the container journal through
`managedstore/common.go`. Its installation sequence includes common-attempt,
session, finalization, cache-selection and private-output migrations.
`VerifyLiquibasePrivateCaptureV2` in turn requires the finalization and
cache-selection catalogs. Copying only the session writer does not remove
those SQL dependencies. Copying the whole opener would also bring historical
upgrades and recovery infrastructure back from #170–#172.

The extracted session coordinator already passes its unchanged dispatch/reuse
tests. It has not been connected to production storage. Neither a test double
nor an in-memory substitute may stand in for its durable journal.

## Approved internal boundary

1. Keep the exact approved current catalog needed by the normal writers,
   including definitions referenced by its guards. Do not weaken triggers or
   remove foreign keys to make an incomplete installation work.
2. Extract those final definitions mechanically as embedded installation SQL
   and an immutable expected-object catalog. They come from the approved
   composed schema, not a new handwritten reduced schema. Record provenance
   and compare the extraction against that source before accepting it.
3. Add one queue installer/verifier consumed by managed-store startup. It
   accepts the existing released managed baseline without the new namespace,
   or the exact installed target. A partial/altered/unknown namespace is an
   error, never a request to fill missing objects.
   Fresh stores create the approved current job table directly. Opening the
   exact released baseline still needs the existing bounded conversion of
   prepare_jobs (purpose and cache_explain_result_json with their checks).
   Preserve every old column/value and assign historical purpose=prepare;
   invent no explanation result. This baseline bridge is retained, not
   misrepresented as a migration-free upgrade. Reject every other source
   format. Any required foreign-key suspension belongs to the exact startup
   connection and must end before it is made available to workers.
4. Before any new DDL or inventory callback, reject unsupported unfinished
   legacy Liquibase work. Preserve existing state, instance, psql and terminal
   history records. This does not add compatibility for unfinished old jobs.
5. Install all required definitions and markers in one caller-owned SQLite
   transaction. Verify the actual catalog, markers and foreign keys again on
   opening. Cache no validation result for a real database. Installation
   failure must not publish a partially usable store.
6. Transfer only the writers/readers actually needed by normal admission,
   binding, planning, three-step execution, publication, bounded output reuse,
   ordinary failure/cancellation and cleanup. Persist resource intent before
   Docker actions. Keep observations outside SQL transactions/eviction locks.
   Keep the existing exact-connection commit checks where required; a failed
   COMMIT cannot be accepted from uncommitted readback.
7. Do not connect automatic replay, cache-handoff recovery, historical format
   migration, or expanded #173 search decoding. Their dormant SQL definitions
   confer no runtime authority. An ambiguous attempt remains recorded and
   nonterminal; a hash or absent reply never authorizes SQL repetition.
   Do not enable the common-attempt executor for psql as a side effect. Its
   existing normal path must continue to work with no new attempt rows; verify
   that the retained guards do not require one. If they do, stop and report a
   scope conflict rather than changing those guards or porting psql recovery.
8. Preserve the approved MP01–MP07 requirements and old protective assertions.
   Installation tests and any additional old-fixture changes require their
   own approval before implementation. This amendment does not approve them.

## Alternatives and rationale

- Copy the composed opener and all migrations: closest to the accumulated
  prototype, but imports deferred behavior and qualification into #89.
- Redesign a smaller journal or use memory/request JSON instead: potentially
  fewer definitions, but replaces the approved ownership/atomicity contract.
- **Chosen:** keep the approved physical catalog and normal transitions,
  separating fresh installation/verification from historical migration code.
  This changes how definitions are packaged, not what they permit.

This decision does not promise a smaller mandatory dependency count before
measuring the extracted closure. If a normal writer still requires a deferred
behavior, report that dependency; do not silently add it or replace it with
a success-returning stub.

## Critical review and corrections

- P1, incomplete guard dependencies: retain the complete referenced catalog,
  not just the session tables; check actual FKs and native normal execution.
- P1, implicit repair of corrupt stores: distinguish absent from partial;
  validation precedes all writes/inventory and never caches a real-store PASS.
- P1, hidden recovery activation: normal writer whitelist and explicit refusal
  of ambiguous repetition; no migration/replay startup call sites.
- P1, false commit success: retain exact-connection ownership and independent
  committed evidence; schema packaging cannot waive the MP04 checks.
- P2, two sources drifting: embedded definitions/catalog must be mechanically
  derived together with recorded provenance and checked against the approved
  composed source; ordinary code changes must not regenerate history silently.
- P1, confusing fresh installation with baseline upgrade: explicitly retain
  the known prepare_jobs conversion and its row/FK checks; no broad migration
  dispatcher or unsupported historical bridge is introduced.
- P1, accidental psql cutover: no activation flag or new psql attempt rows;
  unchanged psql regressions and actual guard applicability are prerequisites.
- P2, scope confusion: exact table reuse does not claim #170–#173 complete or
  binary compatibility with the accumulated unreleased prototype.

Repeat review found no remaining P0–P2 in this component proposal. It is not
release acceptance, a new schema approval, or permission to change old tests.

Decision record: [installation boundary](../adr/2026-11-07-liquibase-normal-installation-boundary.md).
The separately approved [installation test inventory](issue89-normal-path-storage-tests.md)
records the installation evidence; a metadata PASS is not complete prepare acceptance.
