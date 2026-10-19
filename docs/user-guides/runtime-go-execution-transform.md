# Constructing a tool-neutral execution transform

Implemented for #156 and verified locally; not yet merged or published. See the
[contract](../architecture/runtime-v2-execution-transform.md) for approval status.

An execution-environment identity describes the resolved invocation: primitive,
image/tool identity, command, environment and typed resource references. A
transform commits that description and ordered inputs to a State transition.
It is not a second editable command. Non-PSQL tools must not supply dummy SQL
to the existing PSQL transform constructor.

The approved construction sequence uses only existing public types:

```go
composition, err := sqlrs.BuildExecutionTransform(
    declaration, // Kind: "execution"; mandatory execution declaration
    runtimev2.ResolvedTransformExtensions{
        Inputs: resolvedInputs,
        ExecutionEnvironment: &executionIdentity,
    },
    nil, // optional, bounded non-identity operational observations
)
// Check err before accessing the result.
transform, err := runtimev2.NewTransformEnvelope(composition.Identity())
// Check err before deriving the child.
child, err := runtimev2.NewDerivedStateEnvelope(parentStateID, transform)
```

Arguments and attributes of `declaration` are empty; the existing validated
execution identity is the sole command description. Resolve input declarations
in their declared order and retain full identity payloads behind the transform's
fingerprint references. Optional `job_id` observations use owner `sqlrs`, kind
`execution`, schema `sqlrs.execution-transform.observation.v1`; they never change
the transform or State fingerprint. Their exact values and order are retained
in the composition's detached observation views, including legal repeats.
Observations are not a secret-storage or automatic-redaction API. Invalid input
returns an empty composition and the closed validation category/path specified
in the design's linked error table, without echoing caller text.

This construction performs no execution, I/O, cache lookup or reuse admission.
Existing PSQL/Docker constructors remain available with unchanged semantics.
Consume only an immutable release containing #156, published separately by #157;
no existing released version is claimed to contain this approved function yet.
