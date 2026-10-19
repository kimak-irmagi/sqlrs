# Tool-neutral execution transforms

Status: API, interaction and component structure approved by @evilguest on
2026-10-19 for #156. The [test inventory](runtime-v2-execution-transform-tests.md)
was subsequently approved separately on the same date. Publication is
separate in #157; approval is not implementation or release evidence.

## API and scope

Add one atomic production function to the existing `schemas/sqlrs` package:

```go
func BuildExecutionTransform(
    declaration runtimev2.TransformDeclaration,
    extensions runtimev2.ResolvedTransformExtensions,
    observations []runtimev2.OperationalObservation,
) (runtimev2.TransformIdentityComposition, error)
```

All argument and result types already exist. There is no new mutable builder,
result envelope, executor, public HTTP API, CLI or database. The module retains
its standard-library-only dependencies. Usage is described in the
[consumer guide](../user-guides/runtime-go-execution-transform.md).

## Contract

- The fixed transform schema is provider `sqlrs`, kind `execution`, identity
  schema `sqlrs.execution-transform.v1`. It uses the existing canonical-v1
  transform domain, encoding and fingerprint algorithm, not a new hash domain.
- `declaration.Kind` must be `execution`. A valid execution-environment
  declaration is mandatory and must name owner `sqlrs.execution`, kind
  `environment`. Ordinary declaration validation and size bounds still apply.
- Arguments and attributes must be empty: this facade accepts no alternative
  command configuration. Reference is an unresolved locator/label, not another
  semantic command. Capability/portability observations remain non-identity
  metadata; they grant no execution or reusable-State authority.
- `extensions.ExecutionEnvironment` is mandatory and must pass the existing
  `ParseExecutionEnvironmentIdentity`, including schema, nested semantics,
  canonical normalization and budgets. An owner/kind-only match is insufficient.
- Inputs use the existing ordered declaration/result matching and
  `extension.input.<index>` bindings. Missing, extra, invalid or mismatched
  results fail. Equal identities may legitimately occupy distinct declared
  positions; reordering distinct accepted inputs changes the transform.
- The execution identity is bound as `extension.execution_environment`. No
  `sql`, editable argv/environment field or caller-authored field is added.
- The sole optional operational field is `job_id`, under owner `sqlrs`, kind
  `execution`, schema `sqlrs.execution-transform.observation.v1`. It never enters
  the transform fingerprint. Unknown fields or wrong observation schemas fail;
  the observation count is bounded by `runtimev2.MaxResolvedFields`.
  Successful construction preserves the observations' exact values and order,
  including legal repeats, in detached existing result views. They are not
  redacted or discarded; callers still classify operational metadata safely.
- Failure returns an empty composition and a fixed validation code/path, never
  caller-supplied values or map keys. No partial mutable object escapes.
  The [error expectation table](runtime-v2-execution-transform-tests.md#fixed-error-expectations)
  refines shape/value/limit categories and closed paths; that review
  clarification was approved with the revised test inventory.
- Existing `NewPSQLTransformBuilder`, Docker factory and conformance schemas,
  canonical vectors, and published identities remain unchanged.

## Interaction and component ownership

The caller first resolves inputs and constructs/validates the existing execution
identity. `schemas/sqlrs/execution_transform.go` validates the closed facade
contract, creates a private fixed schema via `schemaauthor`, then delegates to
the existing `NewTransformIdentityBuilder`, `BindExtensions`, `AddObservation`
and `Build`. It does not copy canonical encoding or compute private hashes.
Only approved schema code imports `schemaauthor`; consumers do not need it.

The result is passed to `NewTransformEnvelope` and `NewDerivedStateEnvelope`
using the existing parent State. Consumers retain the complete execution/input
payloads behind their fingerprint references; a reference is not the payload.
The existing resolver result and cache contracts are unchanged. This facade
does not resolve resources, launch commands, inspect paths or authorize reuse.

All temporary validation/build state is function-local. There is no persistent
state, shared registry, lifecycle or schema migration.

## Publication boundary

#156 supplies the code, documentation and reviewed tests. After merge and
successful source CI, #157 uses the existing `release-runtime-go.yml` manual
workflow with the exact reviewed `main` source SHA. Select an unused module
version, qualify an RC, verify the clean external consumer's new transform and
lineage round trip, then publish GA from the same Go source. Do not rewrite an
existing tag, use a production `replace`, or introduce a version-only PR.
Publication requires no separate worktree. Changes to the existing publication
workflow are not planned. The source-owned consumer wrapper invokes only the
self-contained new test file as a mandatory child command during the existing
whole-suite step, with positive/negative isolation controls. Existing `*_test.go`
copying includes both files for staging, RC and GA without injecting H's tests
into historical S. See ET08 in the test inventory for the exact execution gate.
