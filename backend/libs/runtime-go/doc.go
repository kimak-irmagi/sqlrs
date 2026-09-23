// Package runtimev2 defines the engine-neutral Runtime v2 semantic model.
//
// The package derives logical state identity exclusively from resolved factory
// and transform provenance. Execution, persistence, and physical materialization
// are deliberately outside this module. See
// docs/architecture/runtime-v2-semantic-core-structure.md.
package runtimev2

const (
	// SchemaVersion is the only semantic schema accepted by this package.
	SchemaVersion = "sqlrs.runtime.v2"

	// MaxIdentifierBytes bounds provider, kind, schema, and field identifiers.
	MaxIdentifierBytes = 128
	// MaxResolvedValueBytes bounds one resolved or diagnostic string value.
	MaxResolvedValueBytes = 4096
	// MaxResolvedFields bounds one resolved identity field set.
	MaxResolvedFields = 256
	// MaxTransforms bounds one recipe or relative lineage suffix.
	MaxTransforms = 10_000
	// MaxJSONBytes bounds one decoded public semantic JSON value.
	MaxJSONBytes = 4 << 20
	// MaxArguments bounds one diagnostic declaration argument list.
	MaxArguments = 256
	// MaxAttributes bounds one diagnostic declaration attribute map.
	MaxAttributes = 256
)
