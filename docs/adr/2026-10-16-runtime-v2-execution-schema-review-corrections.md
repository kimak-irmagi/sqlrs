# Execution-schema review corrections

Each decision below has conversation timestamp 2026-10-16T12:59:09+07:00,
GitHub user @evilguest (41718235), agent Codex / GPT-5. Initially prepared at
the user's request to fix review findings. All six revised decisions and the
concrete API/component structure were approved by the same user at conversation
timestamp 2026-10-16T13:32:32+07:00. The "Proposed choice" wording below records
what was presented and is now accepted. Decision 7 records subsequent test-list
approval; no completed implementation or module publication is claimed.

## Decision 7: reviewed test inventory and narrow existing-test adjustments

- Conversation timestamp: 2026-10-16T14:21:43+07:00.
- GitHub user: @evilguest (41718235); agent: Codex / GPT-5.
- Status: accepted by the user after corrections and repeat inventory review.
- Question: which evidence is required before accepting the new facade?
- Alternatives: rely on round trips and aggregate coverage alone; or add
  independent fixtures, complete profile compatibility, cache outcome traces,
  protected explanation checks, allocation evidence and explicit CI gates.
- Decision: approve ES01–ES28 in the linked test inventory, including the narrow
  fixed-schema architecture exception and additive workflow-contract assertions.
  Existing canonical fixtures/restrictions and checks otherwise remain intact.
- Rationale: a checksum failure, unused Acquire counter or rounded module total
  cannot stand in for semantic rejection, cache recovery or coverage of new code.
  Source I/O, executor behavior and State-cache eligibility remain consumer tests;
  immutable publication remains #151. Tests precede production implementation.

## Decision 1: resolve authoring defaults before atomic construction

- Question: should the facade preserve source-default requests or commit to
  effective process configuration?
- Alternatives: independent nullable Entrypoint/Command/env-mode fields; or
  resolution outside the facade into one argv and explicit env/workdir/user.
- Proposed choice: the latter. Compose interactions and variable removal happen
  before Build; declaration absent/empty distinctions remain at that stage.
- Rationale: the previous generic inheritance rules could misrepresent Compose
  and made a resolved identity behave like another mutable declaration.
  Public source: https://docs.docker.com/reference/compose-file/services/.

## Decision 2: typed path coordinates and qualified source references

- Question: how are physical paths relocated without parsing arbitrary strings,
  and what does an input reference actually identify?
- Alternatives: literal paths plus tool-specific rewriting; untyped content
  digests; or named path roots and qualified canonical source references with
  explicit versioned file/tree materialization profiles.
- Proposed choice: named roots and qualified references. Profiles cover bytes,
  kinds, names, executable flags and normalized metadata; source owners must
  verify their complete manifests. The facade does not add source I/O/hash code.
- Rationale: path relocation and executable-file metadata affect behavior.
  A source reference is not its envelope, manifest or acquisition authority.

## Decision 3: secret purpose, version requirements and controlled authentication

- Question: may a nil version reference automatically make any secret irrelevant
  to identity?
- Alternatives: the old nil/non-nil classification; mandatory versions even for
  technical authentication; or explicit authentication versus semantic purpose.
- Proposed choice: explicit purpose. Semantic inputs require a version; unknown
  purpose fails. Authentication binds one declared logical user/service and an
  admitted versioned injection profile, not generic argv/env/file substitution.
- Rationale: a changing data secret must not produce an unnoticed cache hit.
  No raw secret enters identity and no field-name heuristic proves irrelevance.
  This supersedes decision 4's concrete variants in the 2026-10-15 proposal,
  while retaining the protected map of validated non-secret references.

## Decision 4: ordinary local tools without a false complete-bundle claim

- Question: must every host invocation prove an immutable dependency closure?
- Alternatives: exclude ordinary installed tools; label an incomplete bundle as
  reproducible; or distinguish host-executable from host-bundle resource extent.
- Proposed choice: distinguish extent. Host-executable is local-only and may
  execute, but is barred from reusable-State selection/publication and shared
  replay. Opaque ambient context is not copied into canonical/diagnostic data.
- Rationale: local execution remains useful without pretending to know all OS
  libraries/environment. Provider capability observations cannot override this
  profile's mandatory safety gate or turn a false content assertion into truth.

## Decision 5: represent topology; admit it at the responsible provider

- Question: should a provider-neutral semantic schema ban all nested mounts and
  equate no network attachments with absence of a database target?
- Alternatives: those global restrictions; ignore topology/order; or represent
  ordered overlays, explicit transport and a separate network policy.
- Proposed choice: the latter. Exact duplicate targets and invalid references
  remain errors; physical aliases and unsupported isolation are admission errors.
- Rationale: a Unix-socket target need not use a network attachment. Mount order
  is semantic, not an excuse to forbid all overlays or discard ordering.
  Public source: https://github.com/opencontainers/runtime-spec/blob/main/config.md#mounts.

## Decision 6: reuse qualification precedes lookup and survives descendants

- Question: is an execution-time/publication-only check sufficient after an
  incomplete local step?
- Alternatives: check only on a cache miss; reset qualification at each complete
  suffix step; or gate before lookup and propagate ineligibility through lineage.
- Proposed choice: the last alternative, durably retained by the consumer's
  workspace/attempt owner. Only a verified complete baseline reestablishes reuse.
- Rationale: otherwise a cache hit bypasses admission, or a complete suffix
  publishes data whose parent omitted ambient dependencies. A valid StateID is
  not proof of physical reproducibility. The pure facade owns no runtime store.

## Existing decisions and scope

Reviewed the existing public canonical contract, canonical resolver and
2026-10-15 facade records. No published canonical domain, scalar reference tag,
generic result/cache format or accepted public/private ownership boundary is
superseded. Only the unapproved detailed proposal is revised. The tree inspection
API remains read-only and standard-library-only. Publication is still #151.

See the [revised API](../architecture/runtime-v2-execution-schema-structure.md)
and [repeat review](../architecture/runtime-v2-execution-schema-review.md).

## Decision 8: approved first coverage pass and invariant-only simplification

- Conversation timestamp: 2026-10-16T15:08:21+07:00.
- GitHub user: @evilguest (41718235); agent: Codex / GPT-5.
- Status: one pass accepted by the user; implementation audit and results are
  recorded in the [coverage follow-up](../architecture/runtime-v2-execution-schema-coverage-plan.md).
- Question: should uncovered requirements be tested through the public API, or
  should test hooks/forged opaque state manufacture otherwise impossible errors?
- Alternatives: weaken tests/thresholds; add production hooks; or add public
  requirement cases and simplify only branches whose preconditions prove them
  unreachable. The root's defensive unknown-tag fallback may remain.
- Decision: choose the last alternative, as conditionally approved in the plan.
  Valid immutable secret handles are copied, not redundantly reconstructed;
  fixed-name, fixed-kind fields satisfy fresh-builder preconditions by construction.
  Caller validation, closed shapes and all budget failures remain checked.
- Rationale: these redundancies added no reachable requirement. Public input,
  parser, independent-vector and complete-schema tests remain intact. All new
  files and the module passed the 95% floor; no second iteration was performed.
  Remote CI/race and publication are not claimed by this local measurement.
