package runtimev2

import "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/internal/canonicalv1"

// Canonical-v1 schema types are opaque capabilities created by schemaauthor.
// Requirements: runtime-v2-canonical-contract-structure.md.
type FactoryIdentitySchema = canonicalv1.FactorySchema
type TransformIdentitySchema = canonicalv1.TransformSchema
type ExtensionIdentitySchema = canonicalv1.ExtensionSchema
type CanonicalFieldDefinition = canonicalv1.FieldDefinition

// DisclosureClass controls payload visibility in explanations.
type DisclosureClass = canonicalv1.DisclosureClass

const (
	DisclosurePublic    = canonicalv1.DisclosurePublic
	DisclosureProtected = canonicalv1.DisclosureProtected
)
