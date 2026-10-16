# Production execution-environment schema facade

- Conversation timestamp: 2026-10-15T17:11:15+07:00.
- GitHub user id: @evilguest (41718235).
- Agent name/version: Codex / GPT-5.
- Status: public-facade direction and interaction flow accepted; concrete
  API/component structure subsequently approved on 2026-10-16 as revised in
  [the review corrections](2026-10-16-runtime-v2-execution-schema-review-corrections.md).
  The reviewed test inventory and narrow existing-test/CI adjustments were
  separately approved on 2026-10-16; approval is not execution evidence.

## Question

How should an external provider construct a production canonical execution
extension without acquiring generic schema-authoring authority?

## Alternatives

1. Use the conformance fixture as a production schema.
2. Require consumers to author and duplicate the schema themselves.
3. Add a reviewed schema-specific facade to the existing Runtime Go module.
4. Publish a separate execution service/module or a second resolver API.

## Chosen direction and proposed details

Choose alternative 3 for issue #150; immutable module publication is #151.
Propose adding the pure schema-specific path to `schemas/sqlrs`, reusing the
existing canonical extension type, structured values, typed non-secret
references, resolver result/cache and transform composition. No registry,
filesystem, Docker, process or database I/O belongs in the facade.

The approved interaction boundary is in the
[flow document](../architecture/runtime-v2-execution-schema-flow.md). Exact
exported types and validation rules were subsequently approved after review
on 2026-10-16. Starting the independent branch was not implementation or test
approval; the test inventory was subsequently approved separately on 2026-10-16.

## Rationale and compatibility

Only the missing production schema boundary needs extending. Fixtures cannot
define product semantics, duplicated schemas drift, and new orchestration or
resolution-result formats solve a different problem. Existing canonical domains
and schemas retain their meaning; a new provider schema is explicitly versioned.

Reviewed the 2026-09-27 public canonical contract ADR and canonical contract/
resolver architecture for conflicts. Their public schema-authoring boundary,
fixture-only conformance facade and single current resolver result remain
unchanged. No earlier ADR is made obsolete by this proposal.

## Decision 2: atomic typed construction and semantic cache validation

- Conversation timestamp: 2026-10-15T22:55:14+07:00.
- GitHub user id: @evilguest (41718235).
- Agent name/version: Codex / GPT-5.
- Status: Approved on 2026-10-16 as refined by the linked review corrections.
- Question: should the facade expose a raw schema-bound field builder, another
  result format, or typed atomic construction and checked reading of the
  existing canonical identity?
- Alternatives: expose only the root generic builder with a field schema;
  serialize an opaque provider JSON value; add another mutable builder/result;
  or use Build/Parse plus a schema accessor and typed construction/read views.
- Proposed decision: the last alternative. Generic codec/schema validation is
  structural; providers must also call Parse for fresh and cached identities.
  Build/Parse share validation and existing public canonical construction.
- Rationale: ordered commands, inheritance, secret slots and cross-references
  need semantic rules beyond field-name/kind checks. Atomic construction avoids
  another mutable lifecycle or resolution-result structure.

## Decision 3: inspect canonical values without another byte decoder

- Conversation timestamp: 2026-10-15T22:55:14+07:00.
- GitHub user id: @evilguest (41718235).
- Agent name/version: Codex / GPT-5.
- Status: Approved on 2026-10-16; read-only accessors change no canonical bytes.
- Question: how can the schema validate nested content after a generic cache
  decode when CanonicalValue currently exposes bytes but not children?
- Alternatives: trust the initial build forever; parse framing independently
  inside the facade; or add read-only root Kind/StringValue/Elements/Entries
  accessors plus no-copy EncodedSize for aggregate preflight.
- Proposed decision: the read-only root accessors, reusing existing validated
  framing and child-decoding helpers. Defensive slices expose immutable values.
- Rationale: malformed nested content must not become trusted after restart.
  This is supporting API within #150, not a second encoding, new wire revision,
  hash change or runtime dependency. Schema code must not copy the byte decoder.

## Decision 4: a typed collection of non-secret revision references

- Conversation timestamp: 2026-10-15T22:55:14+07:00.
- GitHub user id: @evilguest (41718235).
- Agent name/version: Codex / GPT-5.
- Status: Superseded in its concrete variants by accepted decision 3 in
  [the review corrections](2026-10-16-runtime-v2-execution-schema-review-corrections.md).
  The protected map remains accepted; execution-only/versioned variants do not.
- Revision note: the execution-only/versioned variants in this initial proposal
  are superseded by the reviewed authentication/semantic variants in
  [the 2026-10-16 corrections](2026-10-16-runtime-v2-execution-schema-review-corrections.md).
  Neither concrete representation had been approved or published before review.
- Question: how can a fixed resolver schema represent several named secret
  revision references and execution-only slots?
- Alternatives: a fixed number of numbered scalar fields; a schema selected
  from per-call slot names; a new canonical secret-reference node; or one
  protected canonical map constructed from validated SecretReference values.
- Proposed decision: the protected map with explicit versioned/execution-only
  entry variants. Parse reconstructs non-secret references through the public
  constructor. No raw value or unkeyed raw-secret hash is accepted.
- Rationale: named map semantics avoid ordinal flattening and per-call resolver
  schemas; a new canonical node would expand the core format unnecessarily.
  The new provider schema owns the collection representation. The existing
  scalar IdentityFieldSecretReference encoding is unchanged; byte equivalence
  with it is not claimed. The revised collection was approved on 2026-10-16.

The concrete proposal is in the
[API and component structure](../architecture/runtime-v2-execution-schema-structure.md).
