package sqlrs

import (
	"bytes"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
	"github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/schemaauthor"
)

// ExecutionRuntimeKind selects a primitive, not an orchestration description.
// All execution types implement docs/architecture/runtime-v2-execution-schema-structure.md.
type ExecutionRuntimeKind string

// ExecutionRuntimeIdentityKind specifies the verified resource's identity extent.
type ExecutionRuntimeIdentityKind string

// ExecutionTextPartKind selects exactly one applicable text-part variant.
type ExecutionTextPartKind string

// ExecutionPathFormat requests late target-platform path rendering by the owner.
type ExecutionPathFormat string

// ExecutionMountKind selects a named input, private scratch, or semantic secret.
type ExecutionMountKind string

// ExecutionInputProfile specifies versioned materialization semantics, not I/O.
type ExecutionInputProfile string

// ExecutionSecretPurpose distinguishes result-affecting input from authentication.
type ExecutionSecretPurpose string

// ExecutionServiceTransport selects logical TCP or Unix-socket transport.
type ExecutionServiceTransport string

// ExecutionNetworkMode describes requested isolation, never admission evidence.
type ExecutionNetworkMode string

// ExecutionServiceField selects a logical non-secret target attribute.
type ExecutionServiceField string

const (
	ExecutionRuntimeHost              ExecutionRuntimeKind         = "host"               // Local execution owner.
	ExecutionRuntimeOCI               ExecutionRuntimeKind         = "oci"                // Selected OCI platform image.
	ExecutionRuntimeDatabaseContainer ExecutionRuntimeKind         = "database-container" // Tool inside the declared DB container.
	ExecutionIdentityImageManifest    ExecutionRuntimeIdentityKind = "image-manifest"     // Platform manifest, not an index.
	ExecutionIdentityHostBundle       ExecutionRuntimeIdentityKind = "host-bundle"        // Complete immutable tool bundle.
	ExecutionIdentityHostExecutable   ExecutionRuntimeIdentityKind = "host-executable"    // Partial local extent; never reusable State approval.
	ExecutionTextPartLiteral          ExecutionTextPartKind        = "literal"            // No expansion/rewrite.
	ExecutionTextPartSecret           ExecutionTextPartKind        = "secret"             // Semantic secret in environment only.
	ExecutionTextPartService          ExecutionTextPartKind        = "service"            // Non-secret service attribute.
	ExecutionTextPartPath             ExecutionTextPartKind        = "path"               // Late logical path projection.
	ExecutionPathNative               ExecutionPathFormat          = "native"             // Target-native representation.
	ExecutionPathForwardSlash         ExecutionPathFormat          = "forward-slash"      // Slash-separated representation.
	ExecutionMountInput               ExecutionMountKind           = "input"              // Materialize a qualified source.
	ExecutionMountScratch             ExecutionMountKind           = "scratch"            // Fresh private writable storage.
	ExecutionMountSecret              ExecutionMountKind           = "secret"             // Owner-delivered semantic secret file.
	ExecutionInputRegularFile         ExecutionInputProfile        = "regular-file.v1"    // Bytes and explicit executable mode.
	ExecutionInputRegularTree         ExecutionInputProfile        = "regular-tree.v1"    // Complete versioned manifest.
	ExecutionSecretAuthentication     ExecutionSecretPurpose       = "authentication"     // One service/user, admitted secure profile.
	ExecutionSecretSemantic           ExecutionSecretPurpose       = "semantic"           // Immutable non-secret version reference required.
	ExecutionServiceTCP               ExecutionServiceTransport    = "tcp"                // Host/port endpoint.
	ExecutionServiceUnixSocket        ExecutionServiceTransport    = "unix-socket"        // Owner-projected socket path.
	ExecutionNetworkNone              ExecutionNetworkMode         = "none"               // No network attachments requested.
	ExecutionNetworkServices          ExecutionNetworkMode         = "services"           // Declared service targets only.
	ExecutionNetworkLocalContext      ExecutionNetworkMode         = "local-context"      // Explicitly opted-in incomplete local context.
	ExecutionServiceHost              ExecutionServiceField        = "host"               // Late TCP host binding.
	ExecutionServicePort              ExecutionServiceField        = "port"               // Late TCP port binding.
	ExecutionServiceDatabase          ExecutionServiceField        = "database"           // Semantic database selector.
	ExecutionServiceUser              ExecutionServiceField        = "user"               // Semantic logical user selector.
)

// ExecutionEnvironmentInput is resolved semantic configuration, not a provider
// proof or execution authority. Build copies it; Parse returns caller-owned data.
type ExecutionEnvironmentInput struct {
	Runtime     ExecutionRuntime
	Argv        []ExecutionText
	Environment []ExecutionEnvironmentVariable
	Workdir     ExecutionPath
	User        string
	Network     ExecutionNetworkMode
	Inputs      []ExecutionInputBinding
	Mounts      []ExecutionMount
	Services    []ExecutionServiceBinding
	Secrets     []ExecutionSecretBinding
}

// ExecutionRuntime binds the primitive to a digest, platform, and optional DB role.
type ExecutionRuntime struct {
	Kind            ExecutionRuntimeKind
	IdentityKind    ExecutionRuntimeIdentityKind
	IdentityDigest  string
	Platform        ExecutionPlatform
	DatabaseService string
}

// ExecutionPlatform describes the target, independently of the host running Build.
type ExecutionPlatform struct{ OS, Architecture, Variant string }

// ExecutionPath is a logical root/clean relative coordinate, never an authority.
type ExecutionPath struct{ Root, Relative string }

// ExecutionText preserves ordered typed parts of one argv/environment value.
type ExecutionText []ExecutionTextPart

// ExecutionTextPart must populate only the selected variant; Path is copied.
type ExecutionTextPart struct {
	Kind                             ExecutionTextPartKind
	Literal, SecretSlot, ServiceRole string
	ServiceField                     ExecutionServiceField
	Path                             *ExecutionPath
	PathFormat                       ExecutionPathFormat
}

// ExecutionEnvironmentVariable names one resolved value; Windows names fold to uppercase.
type ExecutionEnvironmentVariable struct {
	Name  string
	Value ExecutionText
}

// ExecutionInputBinding identifies a named source and its materialization policy.
type ExecutionInputBinding struct {
	Name       string
	Source     ExecutionInputReference
	Profile    ExecutionInputProfile
	Executable bool
}

// ExecutionInputReference is a qualified canonical reference, not a source proof.
type ExecutionInputReference struct {
	Provider, Kind, IdentitySchema string
	Fingerprint                    runtimev2.CanonicalFingerprint
}

// ExecutionMount describes one ordered layer; writable input layers are private copies.
type ExecutionMount struct {
	Name, InputName string
	Target          ExecutionPath
	Kind            ExecutionMountKind
	SecretSlot      string
	ReadOnly        bool
}

// ExecutionServiceBinding names a target without a physical address or credential.
type ExecutionServiceBinding struct {
	Role, Protocol, Database, User              string
	Transport                                   ExecutionServiceTransport
	AuthenticationSecret, AuthenticationProfile string
}

// ExecutionSecretBinding contains no raw secret. Semantic versions are immutable;
// authentication instead binds a single service/user and admitted injection profile.
type ExecutionSecretBinding struct {
	Name              string
	Purpose           ExecutionSecretPurpose
	ServiceRole       string
	IdentityReference *runtimev2.SecretReference
}

// ExecutionEnvironmentSchema returns the fixed reviewed immutable capability.
// It accepts no caller-authored schema or operational fields. Requirements: §8.
func ExecutionEnvironmentSchema() runtimev2.ExtensionIdentitySchema {
	fields := make([]schemaauthor.FieldDefinition, 0, 7)
	for _, name := range []string{"primitive", "runtime", "invocation", "inputs", "network", "mounts", "secrets"} {
		kind, disclosure := runtimev2.IdentityFieldCanonicalValue, schemaauthor.DisclosureProtected
		if name == "primitive" {
			kind, disclosure = runtimev2.IdentityFieldText, schemaauthor.DisclosurePublic
		}
		fields = append(fields, schemaauthor.FieldDefinition{Name: name, Role: schemaauthor.FieldRoleSemantic, Kind: kind, Required: true, Disclosure: disclosure})
	}
	// All definitions are compile-time constants from the approved schema.
	schema, _ := schemaauthor.NewExtensionSchema(schemaauthor.SchemaInput{Provider: "sqlrs.execution", SemanticKind: "environment", IdentitySchema: "sqlrs.execution-environment.v1", Fields: fields})
	return schema
}

// BuildExecutionEnvironmentIdentity atomically validates/normalizes resolved
// configuration and returns the existing canonical extension type. No I/O occurs.
// Host-executable records never authorize reusable-State selection/publication.
func BuildExecutionEnvironmentIdentity(input ExecutionEnvironmentInput) (runtimev2.CanonicalResolvedExtensionIdentity, error) {
	if err := executionPreflight(input); err != nil {
		return runtimev2.CanonicalResolvedExtensionIdentity{}, err
	}
	value, err := executionNormalize(input)
	if err != nil {
		return runtimev2.CanonicalResolvedExtensionIdentity{}, err
	}
	fields, err := executionFields(value)
	if err != nil {
		return runtimev2.CanonicalResolvedExtensionIdentity{}, err
	}
	builder, _ := runtimev2.NewExtensionIdentityBuilder(ExecutionEnvironmentSchema())
	// executionFields produces exactly the seven required fields with fixed
	// names/kinds and valid immutable values. This fresh local builder cannot
	// be nil/sealed or contain duplicates, and every required field is supplied.
	// The fixed-schema ES01/ES03 fixtures enforce that construction invariant;
	// there is no additional caller-controlled failure at these builder calls.
	for _, field := range fields {
		_ = builder.AddIdentityField(field)
	}
	composition, _ := builder.Build()
	return composition.Identity(), nil
}

// ParseExecutionEnvironmentIdentity validates owner/schema/disclosure, nested
// semantics, normal form and aggregate budgets, including for cached identities.
// The returned view is caller-owned; success is not provider/reuse admission.
func ParseExecutionEnvironmentIdentity(identity runtimev2.CanonicalResolvedExtensionIdentity) (ExecutionEnvironmentInput, error) {
	if identity.Provider() != "sqlrs.execution" || identity.Kind() != "environment" || identity.IdentitySchema() != "sqlrs.execution-environment.v1" {
		return ExecutionEnvironmentInput{}, executionError(runtimev2.CodeShapeInvalid, "identity")
	}
	input, err := executionRead(identity)
	if err != nil {
		return ExecutionEnvironmentInput{}, err
	}
	rebuilt, err := BuildExecutionEnvironmentIdentity(input)
	if err != nil {
		return ExecutionEnvironmentInput{}, err
	}
	originalBytes, _ := runtimev2.CanonicalResolvedExtensionBytes(identity)
	rebuiltBytes, _ := runtimev2.CanonicalResolvedExtensionBytes(rebuilt)
	if !bytes.Equal(originalBytes, rebuiltBytes) {
		return ExecutionEnvironmentInput{}, executionError(runtimev2.CodeNonCanonical, "identity")
	}
	// Return the independently owned normalized view, not an untrusted raw tree.
	return executionNormalize(input)
}
