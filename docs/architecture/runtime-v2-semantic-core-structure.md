# Runtime v2 semantic core: module and component structure

Status: approved by @evilguest for issue #107, 2026-09-23. The critical-review
corrections were approved in the same conversation before test design.

## Public module boundary

Create `backend/libs/runtime-go` as an independent nested Go module:

```text
module github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go
package runtimev2
```

The path follows the renamed canonical repository. Add the module to root
`go.work`. Releases of this nested module use repository tags prefixed with its
subdirectory, for example `backend/libs/runtime-go/v0.1.0`. Runtime v2 is the
semantic schema name; it does not force a Go module `/v2` suffix before the Go
module itself reaches major version 2.

The module uses only the Go standard library and imports neither local-engine
internals nor Docker, DBMS, StateFS, Liquibase, SQLite, or service packages.
Consumers may import it without the repository workspace.

```text
backend/libs/runtime-go/
  go.mod
  doc.go                 package contract and schema constants
  declaration.go         mutable unresolved/diagnostic transport DTOs
  identity.go            immutable resolved factory/transform identities
  provenance.go          declaration plus resolved identity observations
  state.go               fingerprints and immutable logical states
  canonical.go           normative binary encoder and SHA-256 helpers
  lineage.go             recipe, steps, Build, and Extend
  validation.go          bounded decoding and structured errors
  testdata/golden/*.json transport values and expected identities
```

## Public model

Declaration DTOs are mutable transport inputs and never hash inputs:

- `FactoryDeclaration` and `TransformDeclaration` preserve unresolved kind,
  reference, arguments, and bounded diagnostic attributes;
- `ResolverObservation` may retain resolver implementation/build information.

Identity-bearing semantic values are immutable:

- `ResolvedField{Name, Value}` represents one provider-defined immutable input;
- `ResolvedFactoryIdentity` and `ResolvedTransformIdentity` contain schema
  version, provider, kind, identity schema, and resolved fields;
- `State` contains its ID and either factory-root or parent/transform provenance;
- `LineageStep` contains `TransformProvenance`, its verified fingerprint, and the
  resulting derived `State`;
- `RecipeLineage` contains factory provenance, its root state, and ordered steps;
- `RelativeLineage` contains an external anchor and ordered new steps.

`FactoryProvenance` and `TransformProvenance` keep identity and optional diagnostic
declaration/observation layers separate. Consequently, two provenance records may
carry different declaration spelling while exposing equal resolved identities.

Opaque semantic types use private fields. Constructors and validated JSON
decoders copy slices and maps; accessors return values or defensive copies.
Custom `MarshalJSON` and `UnmarshalJSON` provide the public persistence/transport
form without allowing callers to mutate a validated value in place.

## JSON transport contract

The following field names and shapes are normative. Object member order is not
significant. `MarshalJSON` emits resolved fields in canonical name order.

Resolved factory and transform identities share this shape:

```json
{
  "schema_version": "sqlrs.runtime.v2",
  "provider": "example",
  "kind": "psql",
  "identity_schema": "example.psql.v1",
  "fields": [{"name": "content.digest", "value": "sha256:..."}]
}
```

`fields` is required and may be empty. Declaration and resolver diagnostics are:

```json
{
  "kind": "psql",
  "reference": "migrations/main.sql",
  "arguments": [],
  "attributes": {}
}
```

```json
{"implementation": "example-resolver", "version": "1.2.3"}
```

Factory and transform provenance use the same envelope. `declaration` and
`resolver` are optional and are omitted, never encoded as `null`, when absent:

```json
{
  "identity": {},
  "declaration": {},
  "resolver": {}
}
```

A factory state and a derived state are distinct shapes selected by `state_kind`:

```json
{
  "schema_version": "sqlrs.runtime.v2",
  "state_kind": "factory",
  "id": "sha256:...",
  "factory_fingerprint": "sha256:..."
}
```

```json
{
  "schema_version": "sqlrs.runtime.v2",
  "state_kind": "derived",
  "id": "sha256:...",
  "parent_id": "sha256:...",
  "transform_fingerprint": "sha256:..."
}
```

The remaining public containers are:

```json
{
  "transform": {},
  "transform_fingerprint": "sha256:...",
  "state": {}
}
```

```json
{
  "schema_version": "sqlrs.runtime.v2",
  "factory": {},
  "transforms": []
}
```

```json
{
  "schema_version": "sqlrs.runtime.v2",
  "factory": {},
  "root": {},
  "steps": []
}
```

```json
{
  "schema_version": "sqlrs.runtime.v2",
  "anchor": "sha256:...",
  "steps": []
}
```

These are, respectively, `LineageStep`, `Recipe`, `RecipeLineage`, and
`RelativeLineage`. Each `{}` placeholder above contains the complete corresponding
shape, not an open extension object.

Every required member must be present and non-null. Optional diagnostic members
may be absent but, when present, must be non-null and valid. Decoders reject
unknown members, duplicate member names at every nesting level, invalid UTF-8,
and trailing JSON tokens. Failed `UnmarshalJSON` validates into a temporary value
and leaves an existing non-zero receiver unchanged. Successful decoding replaces
the receiver atomically.

## Public operations

The package exposes pure operations with no global registry or I/O:

```go
func NewFactoryIdentity(input FactoryIdentityInput) (ResolvedFactoryIdentity, error)
func NewTransformIdentity(input TransformIdentityInput) (ResolvedTransformIdentity, error)
func FactoryState(factory FactoryProvenance) (State, error)
func TransformFingerprint(transform ResolvedTransformIdentity) (Fingerprint, error)
func Derive(parent StateID, transform TransformProvenance) (LineageStep, error)
func Build(recipe Recipe) (RecipeLineage, error)
func Extend(anchor StateID, transforms []TransformProvenance) (RelativeLineage, error)
```

`Recipe` is an ordered input consisting of factory provenance and transform
provenance. `Build` returns a root and zero steps for a factory-only recipe.
`Extend` accepts any valid StateID and an ordered suffix; an empty suffix is valid
and represents the unchanged anchor without inventing a state or transform.

`RecipeLineage.Endpoint()` returns its root or final step state.
`RelativeLineage.EndpointID()` returns its anchor or final step StateID. Validated
JSON decoding recomputes every embedded fingerprint and StateID, and rejects a
record at the first mismatch.

## Validation contract

```go
type ValidationCode string

type ValidationError struct {
    Code ValidationCode
    Path string
}
```

`Path` uses a stable field/index notation such as `steps[2].state.id`. Errors do
not echo field values. `errors.Is(err, ErrInvalid)` matches all validation errors;
callers use `errors.As` when they need code and path. The package exports the
limits and supported schema constant so adapters can reject oversized work before
building DTOs.

JSON decoding rejects unknown fields. Additive diagnostics therefore require a
schema/API release rather than being silently ignored. Canonical hash encoding is
independent of JSON member ordering and diagnostic representation.

## Ownership and integration boundary

The module owns immutable semantic values, canonical encoding, verification, and
deterministic lineage derivation in memory. It owns no persistent data. Engines
own resolver implementations, storage transactions, authorization, provenance
retention, and mappings from logical states to zero or more materializations.

No database schema or HTTP/CLI contract changes are part of issue #107. Existing
local state IDs are not migrated or silently reinterpreted. A later adapter may
map existing managed database identity into a documented factory identity schema,
but local persistence and cache adoption are separate changes.

## Compatibility and conformance

`SchemaVersion` is exactly `sqlrs.runtime.v2`; unknown versions fail closed.
Changing identity fields or canonical bytes requires a new schema namespace,
domains, and golden vectors. Diagnostic transport evolution follows Go module
semantic versioning independently from the Runtime identity namespace.

Golden JSON fixtures cover factory-only, one-step, multi-step, relative, and the
same-endpoint recipe/relative histories. Each fixture contains declarations,
resolved identities, expected fingerprints, every expected state and step, and
the endpoint. Tests decode and re-encode fixtures and recompute every identity.

Import conformance has two compile checks:

- an external-package test in the local-engine module imports the public module,
  proving there is no reverse/circular dependency;
- an independent consumer test module, not relying on root `go.work`, resolves
  the module path through an explicit test-only `replace` and imports no
  local-engine package. Release verification repeats this without `replace`
  against the published nested-module tag.
