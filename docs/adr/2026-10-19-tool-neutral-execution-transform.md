# Tool-neutral execution transform facade

- Conversation timestamp: 2026-10-19T21:34:41+03:00.
- GitHub user id: @evilguest (41718235).
- Agent name/version: Codex / GPT-5.
- Status: API, interaction and component structure approved by @evilguest on
  2026-10-19 (approval recorded at 21:37:20 +03:00). Test inventory requires
  separate approval (subsequently granted on 2026-10-19). This record does not
  claim implementation or publication.

## Question and alternatives

How can a provider construct a canonical State transform for a native command
that has no PSQL SQL payload?

1. Reuse the PSQL schema with a synthetic SQL value.
2. Require the consumer to use generic schema authoring or conformance fixtures.
3. Add a tool-neutral fixed production facade, reusing existing execution and
   canonical composition types.
4. Add another executor, module, command representation or result contract.

## Decision

Choose 3: atomic `schemas/sqlrs.BuildExecutionTransform` returns the existing
`TransformIdentityComposition`. Its fixed `sqlrs.execution-transform.v1` schema
requires the existing execution-environment binding and ordered typed inputs.
It adds no editable semantic fields. PSQL/Docker schemas and fingerprints remain
unchanged. The complete contract is in the
[design](../architecture/runtime-v2-execution-transform.md).

The existing manual release workflow publishes #157 from the verified merged
source, with a qualified RC and same-source GA; it needs no separate worktree or
version-only PR. No release version is fixed before reviewing current releases.

## Rationale and consistency

Atomic construction cannot leak a partially validated command or require a new
mutable builder/result. A new explicit provider schema avoids reinterpreting an
existing identity. The 2026-09-27 public canonical-contract and 2026-10-15/16
execution-schema ADRs were checked: their schema-authoring boundary, single
execution description and immutable published identities are preserved. None
is obsolete. The existing publication ADR remains in force.

## Review refinement: explicit consumer gate and observable failure contract

- Conversation timestamp: 2026-10-19T21:57:42+03:00.
- GitHub user id: @evilguest (41718235).
- Agent name/version: Codex / GPT-5.
- Status: review corrections requested by the user; revised inventory and error
  details subsequently approved on 2026-10-19 before tests/implementation.
  API signature unchanged.
- Question: how can isolation be mandatory without breaking historical source
  validation or adding another workflow, and what concrete outcomes must tests pin?
- Alternatives: rely on a manual isolated run; amend both workflows with source
  capability branching; or add a source-owned test wrapper automatically run by
  existing whole-consumer steps. Choose the wrapper, with a nonrecursive selected
  file command and a helper-dependent negative control. Historical S retains its
  own tests and cannot receive H's new facade fixture.
- The revised inventory additionally fixes PSQL/Docker bytes/fingerprints from
  the unchanged baseline main SHA before new API tests, checks exact retained
  observations/order/copies, and specifies safe error categories/paths and empty
  results after late validation failure. Resolver/cache testing is one connecting
  round trip, not a duplicate implementation test suite.
- Rationale: a green full package does not prove isolated construction; unchanged
  conformance vectors alone do not pin production schemas, and discarding all
  observations must not satisfy non-identity checks. These refinements implement
  the existing ownership/compatibility boundaries; no earlier ADR is obsolete.
