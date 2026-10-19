package sqlrs

import (
	"errors"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
	"github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/schemaauthor"
)

// BuildExecutionTransform atomically binds one validated execution identity and
// the declaration's ordered resolved inputs to sqlrs.execution-transform.v1.
// Kind must be execution, its execution declaration must name sqlrs.execution /
// environment, and Arguments/Attributes must be empty. Reference, capabilities,
// portability and optional job_id observations are non-identity metadata.
//
// The result uses existing canonical-v1 composition/envelope types. Observations
// retain their values/order in detached views; they are not automatically redacted.
// Failure returns an empty composition and a closed ValidationError path, never
// caller-defined keys or values. No resources are resolved or executed, no I/O
// occurs, and no shared mutable state is retained. Requirements:
// docs/architecture/runtime-v2-execution-transform.md.
func BuildExecutionTransform(declaration runtimev2.TransformDeclaration, extensions runtimev2.ResolvedTransformExtensions, observations []runtimev2.OperationalObservation) (runtimev2.TransformIdentityComposition, error) {
	empty := runtimev2.TransformIdentityComposition{}
	environment := declaration.ExecutionEnvironment
	if environment == nil || environment.Owner() == "" {
		return empty, executionError(runtimev2.CodeShapeInvalid, "declaration.execution_environment")
	}
	if environment.Owner() != "sqlrs.execution" || environment.Kind() != "environment" {
		return empty, executionError(runtimev2.CodeValueInvalid, "declaration")
	}
	// Only constants author this closed schema. The existing fixed execution
	// and workspace-file capabilities use the same construction convention.
	schema, _ := schemaauthor.NewTransformSchema(schemaauthor.SchemaInput{
		Provider: "sqlrs", SemanticKind: "execution", IdentitySchema: "sqlrs.execution-transform.v1",
		ObservationSchema: "sqlrs.execution-transform.observation.v1",
		Fields:            []schemaauthor.FieldDefinition{{Name: "job_id", Role: schemaauthor.FieldRoleOperational, Kind: runtimev2.IdentityFieldText, Disclosure: schemaauthor.DisclosureProtected}},
	})
	builder, err := runtimev2.NewTransformIdentityBuilder(schema, declaration)
	if err != nil {
		return empty, executionTransformValidation(err, "declaration")
	}
	if len(declaration.Arguments) != 0 || len(declaration.Attributes) != 0 {
		return empty, executionError(runtimev2.CodeValueInvalid, "declaration")
	}
	if extensions.ExecutionEnvironment == nil {
		return empty, executionError(runtimev2.CodeShapeInvalid, "extensions.execution_environment")
	}
	if _, err := ParseExecutionEnvironmentIdentity(*extensions.ExecutionEnvironment); err != nil {
		return empty, executionTransformValidation(err, "extensions.execution_environment")
	}
	if err := builder.BindExtensions(extensions); err != nil {
		// Execution was fully validated above; remaining binding errors concern
		// ordered inputs. Never expose a validator's caller-derived nested path.
		return empty, executionTransformValidation(err, "extensions.inputs")
	}
	if len(observations) > runtimev2.MaxResolvedFields {
		return empty, executionError(runtimev2.CodeLimitExceeded, "observations")
	}
	for _, observation := range observations {
		if err := builder.AddObservation(observation); err != nil {
			return empty, executionTransformValidation(err, "observations")
		}
	}
	return builder.Build()
}

// executionTransformValidation preserves the existing canonical taxonomy while
// replacing potentially caller-derived paths. Typed declarations still report
// the older categories internally; only their reachable categories are mapped.
func executionTransformValidation(err error, path string) error {
	code := runtimev2.CodeValueInvalid
	var validation *runtimev2.ValidationError
	if errors.As(err, &validation) {
		code = validation.Code
	}
	switch code {
	case runtimev2.CodeInvalidShape:
		code = runtimev2.CodeShapeInvalid
	case runtimev2.CodeInvalidValue:
		code = runtimev2.CodeValueInvalid
	case runtimev2.CodeTooLarge:
		code = runtimev2.CodeLimitExceeded
	}
	return executionError(code, path)
}
