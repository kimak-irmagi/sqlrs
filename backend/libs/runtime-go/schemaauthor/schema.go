// Package schemaauthor is the explicit low-level trust boundary for defining
// canonical-v1 identity schemas. Most production code should use an approved
// schema facade instead of importing this package directly.
package schemaauthor

import (
	"fmt"
	"regexp"
	"sort"
	"strings"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
	"github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/internal/canonicalv1"
)

type FieldRole = canonicalv1.FieldRole
type DisclosureClass = canonicalv1.DisclosureClass

const (
	FieldRoleSemantic    = canonicalv1.FieldRoleSemantic
	FieldRoleOperational = canonicalv1.FieldRoleOperational
	DisclosurePublic     = canonicalv1.DisclosurePublic
	DisclosureProtected  = canonicalv1.DisclosureProtected
)

// FieldDefinition declares one accepted schema-bound input channel.
type FieldDefinition struct {
	Name       string
	Role       FieldRole
	Kind       runtimev2.IdentityFieldKind
	Required   bool
	Disclosure DisclosureClass
}

// SchemaInput names a provider-owned semantic and observation schema.
type SchemaInput struct {
	Provider, SemanticKind, IdentitySchema, ObservationSchema string
	Fields                                                    []FieldDefinition
}

var identifierPattern = regexp.MustCompile(`^[a-z][a-z0-9._-]{0,127}$`)

func validate(input SchemaInput) ([]canonicalv1.FieldDefinition, error) {
	for name, value := range map[string]string{
		"provider": input.Provider, "semantic kind": input.SemanticKind,
		"identity schema": input.IdentitySchema,
	} {
		if !identifierPattern.MatchString(value) {
			return nil, fmt.Errorf("canonical schema invalid: %s", name)
		}
	}
	fields := append([]FieldDefinition(nil), input.Fields...)
	sort.Slice(fields, func(i, j int) bool { return fields[i].Name < fields[j].Name })
	result := make([]canonicalv1.FieldDefinition, len(fields))
	hasOperational := false
	for index, field := range fields {
		if !identifierPattern.MatchString(field.Name) {
			return nil, fmt.Errorf("canonical schema invalid: fields[%d].name", index)
		}
		if strings.HasPrefix(field.Name, "extension.") {
			return nil, fmt.Errorf("canonical schema invalid: reserved field namespace")
		}
		if index > 0 && fields[index-1].Name == field.Name {
			return nil, fmt.Errorf("canonical schema invalid: duplicate field")
		}
		if field.Role != FieldRoleSemantic && field.Role != FieldRoleOperational {
			return nil, fmt.Errorf("canonical schema invalid: fields[%d].role", index)
		}
		if field.Kind != runtimev2.IdentityFieldText && field.Kind != runtimev2.IdentityFieldCanonicalValue && field.Kind != runtimev2.IdentityFieldSecretReference {
			return nil, fmt.Errorf("canonical schema invalid: fields[%d].kind", index)
		}
		if field.Disclosure != DisclosurePublic && field.Disclosure != DisclosureProtected {
			return nil, fmt.Errorf("canonical schema invalid: fields[%d].disclosure", index)
		}
		if field.Role == FieldRoleOperational {
			hasOperational = true
			if field.Required || field.Kind != runtimev2.IdentityFieldText {
				return nil, fmt.Errorf("canonical schema invalid: operational field constraints")
			}
		}
		result[index] = canonicalv1.NewFieldDefinition(field.Name, field.Role, string(field.Kind), field.Required, field.Disclosure)
	}
	if hasOperational && !identifierPattern.MatchString(input.ObservationSchema) {
		return nil, fmt.Errorf("canonical schema invalid: observation schema")
	}
	if !hasOperational && input.ObservationSchema != "" && !identifierPattern.MatchString(input.ObservationSchema) {
		return nil, fmt.Errorf("canonical schema invalid: observation schema")
	}
	return result, nil
}

func NewFactorySchema(input SchemaInput) (runtimev2.FactoryIdentitySchema, error) {
	fields, err := validate(input)
	if err != nil {
		return runtimev2.FactoryIdentitySchema{}, err
	}
	return canonicalv1.NewFactorySchema(input.Provider, input.SemanticKind, input.IdentitySchema, input.ObservationSchema, fields), nil
}

func NewTransformSchema(input SchemaInput) (runtimev2.TransformIdentitySchema, error) {
	fields, err := validate(input)
	if err != nil {
		return runtimev2.TransformIdentitySchema{}, err
	}
	return canonicalv1.NewTransformSchema(input.Provider, input.SemanticKind, input.IdentitySchema, input.ObservationSchema, fields), nil
}

func NewExtensionSchema(input SchemaInput) (runtimev2.ExtensionIdentitySchema, error) {
	fields, err := validate(input)
	if err != nil {
		return runtimev2.ExtensionIdentitySchema{}, err
	}
	return canonicalv1.NewExtensionSchema(input.Provider, input.SemanticKind, input.IdentitySchema, input.ObservationSchema, fields), nil
}
