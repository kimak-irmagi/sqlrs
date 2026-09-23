# Runtime v2 versioned declarations: component structure

Status: approved by @evilguest for issues #108, #123, and #124, 2026-09-24.

## Files and ownership

Extend the existing `runtimev2` package without changing canonical identity code:

```text
backend/libs/runtime-go/
  declaration.go             existing nested diagnostic declarations
  declaration_document.go    versioned standalone factory/transform wrappers
  recipe_declaration.go      versioned unresolved recipe aggregate
  extension_declaration.go   typed input/environment/deployment specifications
  extension_identity.go      provider-owned resolved extension identity
  diagnostics.go             capability and portability observations
```

New document, typed-extension, resolved-extension, and diagnostic semantic values
are opaque after construction, defensively copy slices, and use strict bounded
JSON decoding. Existing declaration DTOs remain mutable authoring inputs for
source compatibility; document constructors validate and deep-copy them.
Provider field meaning remains outside the core. No resolver, filesystem,
Docker, DBMS, storage, or execution dependency is introduced.

## Public model

The existing nested diagnostic declarations gain optional typed-extension and
diagnostic fields without changing their old JSON when those values are absent.
`FactoryDeclaration` may carry inputs, one execution environment, one deployment,
and capability/portability observations. `TransformDeclaration` may carry inputs,
one execution environment, and capability/portability observations. Standalone
persistence uses:

```go
type FactoryDeclarationDocument struct { /* opaque */ }
type TransformDeclarationDocument struct { /* opaque */ }
type RecipeDeclaration struct { /* opaque factory + ordered transforms */ }

func NewFactoryDeclarationDocument(FactoryDeclaration) (FactoryDeclarationDocument, error)
func NewTransformDeclarationDocument(TransformDeclaration) (TransformDeclarationDocument, error)
func NewRecipeDeclaration(FactoryDeclaration, []TransformDeclaration) (RecipeDeclaration, error)
```

`RecipeDeclaration` cannot be passed to `Build`, which continues to require the
resolved `Recipe` type. Accessors return defensive copies. A zero-transform
recipe is valid.

Provider-owned specifications share field vocabulary but not Go types:

```go
type DeclarationField struct { Name, Value string }
type ExtensionSpecificationInput struct {
    SchemaVersion       string
    Owner               string
    Kind                string
    SpecificationSchema string
    Fields              []DeclarationField
}

type InputDeclaration struct { /* opaque */ }
type ExecutionEnvironmentDeclaration struct { /* opaque */ }
type DeploymentDeclaration struct { /* opaque */ }
```

Role-specific constructors accept `ExtensionSpecificationInput`; there is no
implicit conversion between roles. Provider meaning is selected by the tuple
`(role, owner, kind, specification_schema)`. Names use existing identifier
rules; field values use existing value/count limits and are sorted uniquely by
name.

Resolved providers return:

```go
type ResolvedExtensionIdentityInput struct {
    SchemaVersion  string
    Owner          string
    Kind           string
    IdentitySchema string
    Fields         []ResolvedField
}
type ResolvedExtensionIdentity struct { /* opaque */ }
```

This identity is immutable but is not directly hashable as a State transform.
The core exposes:

```go
type ExtensionBinding struct {
    Name     string
    Identity ResolvedExtensionIdentity
}

func ExtensionFingerprint(ResolvedExtensionIdentity) (Fingerprint, error)
func ComposeResolvedFields(base []ResolvedField, bindings []ExtensionBinding) ([]ResolvedField, error)
```

`ExtensionFingerprint` uses the domain
`sqlrs.runtime.v2/resolved-extension` and the existing tagged,
length-delimited grammar: tag 1 schema version, tag 2 owner, tag 3 kind, tag 4
identity schema, and tag 5 the canonical field set. This introduces a new value
type/domain and does not change existing factory, transform, or State bytes.

`ComposeResolvedFields` creates one field per supplied binding named
`extension.<binding-name>` with the extension fingerprint as its value. Binding
names are bounded identifiers, must be unique, and are sorted by field name. The
helper rejects base fields in the reserved `extension.*` namespace and never
drops a supplied binding. Provider adapters must use it whenever declarations
contain resolved extensions; conformance tests verify complete binding.

`CapabilityObservation` and `PortabilityObservation` are separately typed,
versioned diagnostics with owner/kind/schema and bounded diagnostic fields.
They are never accepted by identity constructors.

## Wire shapes

Standalone factory/transform documents have this envelope:

```json
{
  "schema_version": "sqlrs.runtime.v2",
  "declaration": {}
}
```

The recipe declaration is:

```json
{
  "schema_version": "sqlrs.runtime.v2",
  "factory": {},
  "transforms": []
}
```

A typed extension specification is:

```json
{
  "schema_version": "sqlrs.runtime.v2",
  "owner": "sqlrs.workspace",
  "kind": "file",
  "specification_schema": "sqlrs.workspace-file.declaration.v1",
  "fields": [{"name": "path", "value": "migrations/main.sql"}]
}
```

Each typed extension is independently versioned so it remains safe if persisted
outside a recipe document. Owner/kind/specification schema version the opaque
provider vocabulary. `ResolvedExtensionIdentity` likewise carries its own
`schema_version` plus owner, kind, identity schema, and resolved fields.

Unknown/duplicate members and trailing tokens fail. Provider fields are a closed
array rather than a map, so duplicate names are rejected and deterministic
serialization does not depend on map order. Failed unmarshalling is atomic.

## Release and compatibility

Existing #107 identity bytes, StateIDs, JSON golden files, and public resolved
types remain unchanged. The previously published nested-module tag
`backend/libs/runtime-go/v0.1.0` is immutable but incomplete for the declaration
boundary and is retracted in the next module `go.mod` with an actionable reason.

The first recommended external version is
`backend/libs/runtime-go/v0.2.0`. The exact intended commit is first published as
`backend/libs/runtime-go/v0.2.0-rc.1`; GA uses the same commit only after the RC
passes #108/#124 tests and external-consumer gates. A dedicated workflow has two
modes:

- `workflow_dispatch` preflight verifies the proposed version/commit, clean tree,
  module path, unit/conformance/golden/race/fuzz-smoke/coverage/dependency gates,
  and a standalone `GOWORK=off` consumer before a maintainer creates the tag;
- a `backend/libs/runtime-go/v*` push verifies prefix, module path, and exact tag
  commit, then builds a clean consumer against the immutable tagged version with
  no `replace` and a fresh module cache;
- post-publication verification runs `go list -m` and `go mod download` through
  the public proxy with bounded retry for propagation, and checks release notes
  naming schema `sqlrs.runtime.v2` and the source commit.

Acceptance is staged rather than circular: PR/preflight proves the local clean
consumer; the immutable RC tag proves public-proxy and checksum-database
consumption; GA is created from that same commit only after RC succeeds; and a
final GA public-consumer check is the closure gate for issue #123. A GA-only
check is never required before merge or before its tag exists.

The workflow never moves or overwrites a tag. Issue #123 closes only after the
post-publication checks pass.

Implementation remains reviewable as four buildable commits in one PR:
versioned declarations/extensions, resolver framework, workspace-file/cache/
artifact implementation, and release automation. Every commit keeps legacy
runtime behavior unchanged; the final tag is created only from the merged commit.
