// Package sqlrs exposes approved schema-specific canonical-v1 builders without
// leaking the generic schemaauthor trust boundary into production callers.
package sqlrs

import (
	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
	"github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/schemaauthor"
)

// NewDockerFactoryBuilder creates the supported sqlrs Docker factory builder.
// Semantic image and platform values must be supplied as text identity fields;
// job/container IDs and paths are operational observations only.
func NewDockerFactoryBuilder(declaration runtimev2.FactoryDeclaration) (*runtimev2.FactoryIdentityBuilder, error) {
	schema, err := schemaauthor.NewFactorySchema(schemaauthor.SchemaInput{
		Provider: "sqlrs", SemanticKind: "docker", IdentitySchema: "sqlrs.docker.v1",
		ObservationSchema: "sqlrs.docker.observation.v1",
		Fields: []schemaauthor.FieldDefinition{
			{Name: "image", Role: schemaauthor.FieldRoleSemantic, Kind: runtimev2.IdentityFieldText, Required: true, Disclosure: schemaauthor.DisclosurePublic},
			{Name: "platform", Role: schemaauthor.FieldRoleSemantic, Kind: runtimev2.IdentityFieldText, Required: false, Disclosure: schemaauthor.DisclosurePublic},
			{Name: "container_id", Role: schemaauthor.FieldRoleOperational, Kind: runtimev2.IdentityFieldText, Disclosure: schemaauthor.DisclosureProtected},
			{Name: "materialization_path", Role: schemaauthor.FieldRoleOperational, Kind: runtimev2.IdentityFieldText, Disclosure: schemaauthor.DisclosureProtected},
		},
	})
	if err != nil {
		return nil, err
	}
	return runtimev2.NewFactoryIdentityBuilder(schema, declaration)
}

// NewPSQLTransformBuilder creates the supported sqlrs psql transform builder.
func NewPSQLTransformBuilder(declaration runtimev2.TransformDeclaration) (*runtimev2.TransformIdentityBuilder, error) {
	schema, err := schemaauthor.NewTransformSchema(schemaauthor.SchemaInput{
		Provider: "sqlrs", SemanticKind: "psql", IdentitySchema: "sqlrs.psql.v1",
		ObservationSchema: "sqlrs.psql.observation.v1",
		Fields: []schemaauthor.FieldDefinition{
			{Name: "sql", Role: schemaauthor.FieldRoleSemantic, Kind: runtimev2.IdentityFieldCanonicalValue, Required: true, Disclosure: schemaauthor.DisclosureProtected},
			{Name: "job_id", Role: schemaauthor.FieldRoleOperational, Kind: runtimev2.IdentityFieldText, Disclosure: schemaauthor.DisclosureProtected},
		},
	})
	if err != nil {
		return nil, err
	}
	return runtimev2.NewTransformIdentityBuilder(schema, declaration)
}
