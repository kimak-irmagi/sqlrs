# Runtime v2 alias composition: module and component structure

Status: approved by @evilguest for issue #109 after critical design review,
2026-09-27.

## Public module boundary

Add an opt-in subpackage to the independent Runtime v2 module:

```text
backend/libs/runtime-go/
  composition/
    document.go       immutable alias document and JSON contract
    catalog.go        explicit multi-document namespace construction
    expand.go         deterministic recipe and transform expansion
    trace.go          non-semantic source and expansion diagnostics
    errors.go         stable composition error envelope
    validation.go     names, bounds, and closed-union validation
```

Package `composition` imports the root
`github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go` package. The root
`runtimev2` package does not import `composition`, preventing a dependency
cycle and keeping existing identity code unchanged. The package uses only the
Go standard library and imports no CLI, YAML, resolver implementation,
local-engine, Docker, DBMS, SQLite, StateFS, or execution package.

The API is additive in the next available Runtime module minor release. The
design does not reserve a version number before the preceding approved release
line has been published.

## Versioned document model

The document schema is `sqlrs.runtime.v2.aliases.v1`. Runtime v2 and the alias
document revision are independently visible in the name: a future compatible
Runtime v2 alias transport can select `aliases.v2` without reinterpreting v1.

The normative JSON shape is:

```json
{
  "schema_version": "sqlrs.runtime.v2.aliases.v1",
  "aliases": {
    "main-migrations": {
      "type": "transform",
      "declaration": {
        "kind": "psql",
        "reference": "migrations/main.sql",
        "arguments": [],
        "attributes": {}
      }
    },
    "base-db": {
      "type": "recipe",
      "base": {
        "factory": {
          "kind": "postgres",
          "reference": "postgres:17",
          "arguments": [],
          "attributes": {}
        }
      },
      "steps": []
    },
    "project-db": {
      "type": "recipe",
      "base": {"recipe": "base-db"},
      "steps": [
        {"use": "main-migrations"},
        {
          "transform": {
            "kind": "psql",
            "reference": "seeds/project.sql",
            "arguments": [],
            "attributes": {}
          }
        }
      ]
    }
  }
}
```

Nested factory and transform declarations reuse their existing diagnostic wire
shape. The alias document supplies the required outer schema version; it does
not embed standalone declaration-document wrappers.

Every alias value is a closed union selected by `type`:

- `transform` requires only `declaration`;
- `recipe` requires only `base` and `steps`;
- `base` contains exactly one of `factory` or `recipe`;
- a step contains exactly one of `use` or `transform`.

Unknown, duplicate, missing, extra-for-variant, and `null` members fail strict
decoding. Failed decoding is atomic. Object-member order has no meaning;
marshalling emits aliases in unsigned bytewise name order while preserving step
order exactly.

## Public types and operations

The intended public surface is:

```go
const (
    AliasSchemaVersion          = "sqlrs.runtime.v2.aliases.v1"
    ExpansionTraceSchemaVersion = "sqlrs.runtime.v2.alias-expansion-trace.v1"
    MaxAliases                  = runtimev2.MaxTransforms
    MaxSourceDocuments          = 1_024
    MaxSourceIDBytes            = runtimev2.MaxResolvedValueBytes
    MaxCatalogBytes             = 32 << 20
    MaxTraceNodes               = MaxAliases + runtimev2.MaxTransforms
    MaxDiagnosticReferences     = MaxAliases + 1
    MaxPointerBytes             = runtimev2.MaxResolvedValueBytes
    MaxErrorTextBytes           = 16 << 10
    MaxTraceJSONBytes           = 32 << 20
)

type AliasDocument struct { /* opaque */ }
type AliasKind string
const (
    AliasKindTransform AliasKind = "transform"
    AliasKindRecipe    AliasKind = "recipe"
)
type DocumentInput struct {
    Aliases []NamedAliasInput
}
type NamedAliasInput struct {
    Name      string
    Transform *runtimev2.TransformDeclaration
    Recipe    *RecipeAliasInput
}
type RecipeAliasInput struct {
    Base  RecipeBaseInput
    Steps []RecipeStepInput
}
type RecipeBaseInput struct {
    Factory *runtimev2.FactoryDeclaration
    Recipe  string
}
type RecipeStepInput struct {
    Use       string
    Transform *runtimev2.TransformDeclaration
}

func NewAliasDocument(DocumentInput) (AliasDocument, error)
func DecodeAliasDocumentJSON([]byte, *AliasDocument) error
func (AliasDocument) MarshalJSON() ([]byte, error)
func (*AliasDocument) UnmarshalJSON([]byte) error

type SourceDocument struct {
    SourceID string
    Document AliasDocument
}
type Catalog struct { /* opaque immutable snapshot */ }
func NewCatalog([]SourceDocument) (Catalog, error)

type ExpansionTrace struct { /* opaque versioned diagnostic value */ }
type TraceNode struct { /* opaque */ }
type FactoryOrigin struct { /* opaque */ }
type TransformOrigin struct { /* opaque */ }
type ExpandedRecipe struct { /* opaque */ }
type ExpandedTransform struct { /* opaque */ }
func (Catalog) ExpandRecipe(string) (ExpandedRecipe, error)
func (Catalog) ExpandTransform(string) (ExpandedTransform, error)

func (ExpandedRecipe) Declaration() runtimev2.RecipeDeclaration
func (ExpandedRecipe) Trace() ExpansionTrace
func (ExpandedTransform) Declaration() runtimev2.TransformDeclarationDocument
func (ExpandedTransform) Trace() ExpansionTrace
func (ExpansionTrace) TargetNodeID() int
func (ExpansionTrace) Nodes() []TraceNode
func (ExpansionTrace) Factory() (FactoryOrigin, bool)
func (ExpansionTrace) Transforms() []TransformOrigin
func (ExpansionTrace) MarshalJSON() ([]byte, error)
func (*ExpansionTrace) UnmarshalJSON([]byte) error
func DecodeExpansionTraceJSON([]byte, *ExpansionTrace) error

func (TraceNode) ID() int
func (TraceNode) ParentID() (int, bool)
func (TraceNode) ReferencePointer() string
func (TraceNode) Kind() AliasKind
func (TraceNode) Alias() string
func (TraceNode) SourceID() string
func (TraceNode) Pointer() string
func (FactoryOrigin) NodeID() int
func (FactoryOrigin) Pointer() string
func (TransformOrigin) OutputIndex() int
func (TransformOrigin) NodeID() int
func (TransformOrigin) Pointer() string

var ErrInvalid = errors.New("runtime v2 composition invalid")
```

The closed unions, opaque immutable values, exact declaration/trace accessors,
defensive-copy behavior, and separate typed recipe/transform results are
normative. Constructors accept slices instead of maps where duplicate detection
matters. Accessors return defensive copies of the underlying `runtimev2`
declarations and trace collections. Zero-value documents and traces cannot be
marshalled; JSON unmarshalling and the dedicated decode helpers share the same
strict, bounded, atomic behavior.

## Names, limits, and ownership

Alias names use the existing lower-case Runtime identifier grammar
`[a-z][a-z0-9._-]{0,127}`. They are lookup keys, not identity fields. A catalog
contains at most `MaxAliases` alias definitions in total across at most
`MaxSourceDocuments` explicit source documents. One document and one expanded
declaration use the existing 4-MiB Runtime JSON limit. Both
`NewAliasDocument` and `DecodeAliasDocumentJSON` measure the canonical encoded
document, so constructor input cannot bypass the transport bound. The sum of
canonical document bytes plus source IDs in one catalog is at most
`MaxCatalogBytes`; document count alone therefore cannot admit gigabytes of
input. A serialized trace is at most `MaxTraceJSONBytes`.

Alias references use the alias-name grammar. Source IDs are non-empty UTF-8 of
at most `MaxSourceIDBytes`, unique within a catalog, and diagnostic. CLI callers
use slash-separated workspace-relative IDs and never absolute host paths.
Because source IDs are exposed in errors and traces, callers must not place
credentials or other secrets in them.

An empty `aliases` object is valid, and a nil constructor slice canonicalizes
to that empty object. `NewCatalog(nil)` returns a valid empty immutable catalog;
any expansion from it returns `missing_reference`. Zero-value `AliasDocument`
and `Catalog` values remain invalid and cannot bypass construction.

Expansion may produce at most `runtimev2.MaxTransforms`. Catalog accounting,
declaration transport size, trace transport size, and collection growth use
checked arithmetic before allocation or append. The implementation is iterative
so a maximal valid parent chain cannot exhaust the Go stack. A declaration and
its trace pass all size checks before either is returned.

`AliasDocument`, `Catalog`, and expanded results own immutable snapshots of
their data. Declaration slices/maps are copied through existing Runtime v2
constructors and accessors. No package-global registry, filesystem state,
environment variable, workspace config, or mutable cache participates in
expansion.

## Catalog and ambiguity

`NewCatalog` normalizes no name spelling and applies no source precedence. Its
validation precedence is independent of caller slice order. It first checks the
source count, sorts sources by raw unsigned bytewise `SourceID`, validates IDs
and rejects duplicate IDs in that order, then rejects zero-value documents and
accumulates total definition/canonical-byte limits with checked arithmetic.
Finally it indexes aliases in unsigned bytewise name and `SourceID` order. A
name present in multiple source documents is
rejected as `ambiguous_reference`; candidate diagnostics are sorted by the
now-unique `SourceID`. An alias document cannot contain the same JSON member
twice because strict decoding rejects duplicate members.

The package deliberately does not define file discovery, imports, relative
document references, or workspace lookup. Those policies require a separately
approved CLI or service boundary.

## Expansion algorithm

Recipe expansion maintains explicit `unvisited`, `visiting`, and `complete`
states and an ordered stack. `base.recipe` recursively in semantic terms but
iteratively in implementation expands one prefix. Once the factory is known,
the expander appends that prefix's transforms and then visits the requested
recipe's steps in source order.

Transform references resolve to immutable declaration copies. Inline transforms
are copied at their exact position. A completed recipe expansion may be memoized
inside one operation, but no mutable result is shared between calls.

Expansion error precedence follows that same deterministic traversal. The
operation validates the requested name and target kind first, walks the complete
base chain before any steps, then emits steps from the deepest recipe back to
the target, preserving source order within each recipe. At each edge it resolves
and validates the current reference before checking the projected output/trace
bounds for that occurrence. The first failure stops traversal, so a limit failure
may precede a defect in a later unvisited step. The guarantee that all references
are valid applies to successful results, not to unreachable work after an error.

The following invariants hold:

- a result has exactly one factory;
- a nested recipe contributes its full ordered prefix;
- a recipe is never accepted as a step;
- no automatic parallelization or order normalization occurs;
- every reference visited by a successful expansion is validated even though
  alias names are omitted from the output;
- an error returns neither a declaration nor a partial trace.

## Trace and error model

`ExpansionTrace` is immutable, non-semantic, and serialized with required
`schema_version: sqlrs.runtime.v2.alias-expansion-trace.v1`. It uses a dense
node table with parent-node IDs so complete alias chains are reconstructable
without copying a chain into every output origin:

```json
{
  "schema_version": "sqlrs.runtime.v2.alias-expansion-trace.v1",
  "target_node": 0,
  "nodes": [
    {
      "node_id": 0,
      "kind": "recipe",
      "alias": "project-db",
      "source_id": "db/aliases.yaml",
      "pointer": "/aliases/project-db"
    },
    {
      "node_id": 1,
      "parent_node_id": 0,
      "reference_pointer": "/aliases/project-db/base/recipe",
      "kind": "recipe",
      "alias": "base-db",
      "source_id": "db/aliases.yaml",
      "pointer": "/aliases/base-db"
    },
    {
      "node_id": 2,
      "parent_node_id": 0,
      "reference_pointer": "/aliases/project-db/steps/0/use",
      "kind": "transform",
      "alias": "main-migrations",
      "source_id": "db/aliases.yaml",
      "pointer": "/aliases/main-migrations"
    }
  ],
  "factory": {
    "node_id": 1,
    "pointer": "/aliases/base-db/base/factory"
  },
  "transforms": [
    {
      "output_index": 0,
      "node_id": 2,
      "pointer": "/aliases/main-migrations/declaration"
    }
  ]
}
```

A standalone-transform trace uses the same envelope without `factory`:

```json
{
  "schema_version": "sqlrs.runtime.v2.alias-expansion-trace.v1",
  "target_node": 0,
  "nodes": [
    {
      "node_id": 0,
      "kind": "transform",
      "alias": "seed-data",
      "source_id": "db/aliases.yaml",
      "pointer": "/aliases/seed-data"
    }
  ],
  "transforms": [
    {
      "output_index": 0,
      "node_id": 0,
      "pointer": "/aliases/seed-data/declaration"
    }
  ]
}
```

Node IDs are zero-based dense array indexes and `target_node` is always zero.
Nodes represent alias occurrences, not deduplicated definitions. The expander
allocates them deterministically: target first, recipe-base occurrences down to
the factory, then named transform occurrences in output order. Inline
transforms add no node and point to their containing recipe node.
Every node alias satisfies the alias-name grammar; every node source ID is
non-empty valid UTF-8 bounded by `MaxSourceIDBytes`.

Pointers are canonical RFC 6901 paths, not arbitrary pointer-shaped strings, and
are at most `MaxPointerBytes`. A node definition pointer is
`/aliases/<escaped-alias>`. A recipe child uses its parent recipe's
`/aliases/<escaped-parent>/base/recipe`; a named transform occurrence uses
`/aliases/<escaped-parent>/steps/<index>/use`. Factory, named-transform, and
inline-transform origins use the corresponding `/base/factory`, `/declaration`,
and `/steps/<index>/transform` paths.

A recipe trace has this closed grammar:

- node zero is the recipe target and alone omits `parent_node_id` and
  `reference_pointer`;
- recipe nodes form one non-empty contiguous prefix; each recipe after node zero
  has the preceding recipe node as parent;
- the last recipe node owns the required factory origin;
- all remaining nodes are transform leaves, each parented by a recipe node and
  consumed by exactly one transform origin;
- transform origins are in output order: deepest recipe through target, with
  contiguous zero-based step indexes inside each recipe; an inline origin points
  directly to its recipe node, while a named origin points to its unique
  transform node;
- the transform-node subsequence is the same order in which those nodes appear
  in the origin array; there are no unused nodes.

A standalone-transform trace has exactly one node: transform target zero with no
parent/reference pointer, no factory, and one output-zero transform origin that
points to that node. `output_index` always equals the origin's array position.
These invariants, canonical pointers, kinds, links, and variant shape are
strictly and atomically validated during trace decoding.

Node/origin counts grow linearly in visited alias occurrences plus output
transforms. Node count is checked against `MaxTraceNodes`; transform-origin
count is checked against `runtimev2.MaxTransforms`, and a recipe adds exactly
one factory origin. Checked arithmetic precedes every allocation or append that
could cross a bound. The trace is never accepted by
`runtimev2.NewRecipeDeclaration`, resolver cache keys, identity constructors,
or fingerprint functions.

Composition failures match a package sentinel and expose a stable envelope:

```go
type ErrorCode string
const (
    CodeInvalidDocument     ErrorCode = "invalid_document"
    CodeMissingReference    ErrorCode = "missing_reference"
    CodeAmbiguousReference  ErrorCode = "ambiguous_reference"
    CodeWrongReferenceKind  ErrorCode = "wrong_reference_kind"
    CodeCycle               ErrorCode = "cycle"
    CodeExpansionTooLarge   ErrorCode = "expansion_too_large"
)

type Error struct {
    Code     ErrorCode
    SourceID string // optional diagnostic source
    Pointer  string // RFC 6901 JSON Pointer
}

type DiagnosticReference struct {
    Alias    string
    SourceID string
    Pointer  string
}

func (e *Error) Cycle() []DiagnosticReference
func (e *Error) Candidates() []DiagnosticReference
```

Package-produced error strings include only the stable code and bounded primary
diagnostic location, never contexts or declaration contents, and are at most
`MaxErrorTextBytes`. Cycle chains and ambiguity
candidates contain at most `MaxDiagnosticReferences` entries and are returned
in deterministic order through defensive-copy accessors. `Cycle()` starts with
the first revisited active recipe and repeats it as the final closing entry;
its entries point to alias definitions, while the primary error points to the
closing reference. `Candidates()` is sorted by `SourceID` and also points to
definitions. Expansion operations
first validate the requested alias-name grammar. A nested missing or wrong-kind
error points to its referencing field; a missing top-level target has empty
source/root pointer, while a wrong-kind top-level target points to the selected
definition. A cycle points to the closing reference, and an invalid document
points to the rejected field when one exists. For nested `runtimev2` declaration
validation, the composition pointer identifies the enclosing factory or
transform declaration; the unwrapped `runtimev2.ValidationError.Path` retains
the nested field location without attempting an ambiguous dotted-path-to-pointer
conversion. Catalog-wide ambiguity and
aggregate-limit errors use an empty primary source and root pointer.
Expansion-output limit errors point to the requested target.
Errors returned by composition constructors, dedicated decoders, catalog and
expansion operations match `ErrInvalid` through `errors.Is`. As with the root
Runtime package, `encoding/json` may reject malformed syntax before invoking
`UnmarshalJSON`; that native syntax error is not a composition error. Underlying
`runtimev2.ValidationError` values, including scalar field/count violations,
are classified as `invalid_document` without losing their stable validation
code/path through error unwrapping. `expansion_too_large` is reserved for the
composition layer's aggregate document/catalog, graph, declaration-output, and
trace transport/count bounds. An empty error `Pointer` denotes the
source-document root.

## Path-binding ownership

The composition package validates and copies declarations but treats their
provider-owned contents as opaque. It never rebases a `reference`, argument,
attribute, or extension field from `SourceID` or a source file location. Each
provider specification defines its own base; the #108 workspace-file resolver
continues to interpret its `path` field relative to the supplied workspace root.

A provider-aware authoring adapter materializes declaration values before
calling `NewAliasDocument`. The legacy adapter first performs the current
alias-file-relative path binding and existing default selection, then supplies
the resulting explicit declaration. Source movement alone therefore cannot
silently change a path base.

## Required CLI compatibility slice

Issue #137 implements the independently mergeable CLI slice against the
published public module:

This slice remains inside `frontend/cli-go/internal`. Its package name
`aliasruntimev2` does not denote a public Runtime Go package. The CLI owns
source discovery, YAML parsing, recursive alias expansion, legacy translation,
and provider-aware authoring/binding. The engine imports neither this internal
package nor alias-document or catalog types. A future integration sends the
engine a complete expanded `runtimev2.RecipeDeclaration` plus source inputs;
the engine validates and resolves those inputs and owns planning, execution,
and StateID computation. The wire contract and command cutover are outside
#109/#137 and must be designed separately. See the
[client-alias-boundary ADR](../adr/2026-10-03-runtime-v2-client-alias-boundary.md).

```text
frontend/cli-go/internal/alias/runtimev2/
  yaml.go            strict bounded YAML-to-document adapter
  compatibility.go   legacy translation/classification boundary
```

The Go package name is `aliasruntimev2`, avoiding a collision with the imported
root package name `runtimev2`. The application layer may import both this
subpackage and its parent `internal/alias`; the parent package does not import
the subpackage, so the dependency graph remains acyclic.

The compatibility boundary owns these key contracts:

```go
const (
    MaxYAMLBytes                  = runtimev2.MaxJSONBytes
    MaxYAMLDepth                  = 32
    MaxYAMLNodes                  = 65_536
    MaxCompatibilityMessageBytes = runtimev2.MaxResolvedValueBytes
)

type Status string
const (
    StatusTranslated Status = "translated"
    StatusLegacyOnly Status = "legacy_only"
)

type ReasonCode string
const (
    ReasonProviderUnavailable ReasonCode = "runtime_v2_provider_unavailable"
    ReasonDefaultUnavailable  ReasonCode = "legacy_default_unavailable"
    ReasonUnsupported         ReasonCode = "legacy_semantics_unsupported"
)

type ImageSource string
const (
    ImageSourceAlias           ImageSource = "alias"
    ImageSourceWorkspaceConfig ImageSource = "workspace_config"
    ImageSourceGlobalConfig    ImageSource = "global_config"
)

type TranslationInput struct {
    Definition           legacyalias.Definition
    WorkspaceRoot        string
    AliasPath            string
    SourceID             string
    EffectiveImage       string
    EffectiveImageSource ImageSource
}

type ProviderAdapter interface {
    Kind() string
    Translate(TranslationInput) (Result, error)
}

type Result struct { /* opaque translated declaration or legacy-only reason */ }

func DecodeAliasDocumentYAML([]byte, *composition.AliasDocument) error
func TranslateLegacy(TranslationInput, ProviderAdapter) (Result, error)
func NewTranslatedResult(runtimev2.RecipeDeclaration) (Result, error)
func NewLegacyOnlyResult(ReasonCode, string) (Result, error)
func (Result) Status() Status
func (Result) Declaration() (runtimev2.RecipeDeclaration, bool)
func (Result) Reason() ReasonCode
func (Result) Message() string
```

`DecodeAliasDocumentYAML` checks `MaxYAMLBytes`, UTF-8 validity, and directive
lines before parsing and leaves a non-nil destination unchanged on every
failure. The pinned YAML parser's own 10,000-level scanner guard is defense in
depth; after parsing, the adapter iteratively enforces `MaxYAMLDepth` and
`MaxYAMLNodes` before constructing any public value. The YAML document node is
excluded from both measures: the logical root mapping has depth one, and node
count includes every reachable mapping, sequence, and scalar node, including
mapping-key scalars. Alias nodes are rejected rather than traversed. The adapter
accepts only map, sequence, and string tags required by the logical schema.
Boolean, integer, float, timestamp, binary, and null scalars are not coerced to
strings. Anchors, aliases, merge keys, custom tags, directives, duplicate keys,
and multiple documents are rejected. The resulting constructor input must
independently satisfy the canonical Runtime JSON-size limit. These transport
limits may reject a very large value that strict JSON can represent; every YAML
value that is accepted has exactly the constructor/JSON meaning and does not
form a second semantic model.

The compatibility component receives a fully bound legacy
alias location plus explicit effective defaults and a provider adapter.
`WorkspaceRoot` and `AliasPath` are canonical absolute paths already checked by
the existing alias resolver, and `AliasPath` is strictly within `WorkspaceRoot`.
`SourceID` must equal the non-empty `filepath.ToSlash` form of the canonical
relative alias path: it is not absolute, contains no backslash, empty, `.`, or
`..` segment, and satisfies `composition.MaxSourceIDBytes`. `TranslateLegacy`
revalidates these relationships before adapter invocation rather than trusting
direct callers. The machine-stable `ImageSource` is diagnostic and never enters
the declaration.
The provider owns conversion of current alias-file-relative inputs into its
explicit Runtime v2 workspace/provider form. `TranslateLegacy` requires the
adapter's lower-case `Kind()` to equal `Definition.Kind`. This first
compatibility slice accepts only
legacy prepare aliases; a run alias is `legacy_semantics_unsupported` because a
run preset is not an identity-bearing Runtime v2 recipe transform. The result
returns either a complete Runtime v2 recipe declaration (`StatusTranslated`,
`Declaration` present, empty reason/message) or a structured `legacy_only`
classification (no declaration, non-empty reason and actionable message), never
both. Result constructors enforce those invariants and accessors return
immutable snapshots. Legacy-only messages are valid UTF-8, non-empty, and at
most `MaxCompatibilityMessageBytes`; only the closed reason codes are accepted.

`TranslateLegacy` applies this precedence after structural input validation:
run aliases return `legacy_semantics_unsupported`; nil and typed-nil adapters
return `runtime_v2_provider_unavailable`; a prepare alias with no effective image
returns `legacy_default_unavailable`; then adapter kind is checked and the
adapter is invoked. An explicit legacy image requires the identical effective
image with `ImageSourceAlias`; an inherited non-empty image requires exactly one
workspace/global source; an empty image requires the zero source.

An adapter error always produces a zero `Result`, even if the adapter also
returned a value. With a nil error, `TranslateLegacy` revalidates the returned
opaque result and treats a zero or otherwise invalid result as a contract error.
The selected provider adapter handles its kind-specific arguments and may return
a precise legacy-only result. Non-nil errors are reserved for invalid inputs or
adapter contract/operational failures, never ordinary lack of support. A
reusable conformance suite is mandatory for every production adapter: it covers
every supported file-bearing argument, alias-relative to workspace/provider
binding exactly once, defaults, unsupported forms, diagnostic-provenance
exclusion, and declaration immutability.
No production translation claim is made until that adapter passes the suite.
Until a complete provider adapter exists, `runtime_v2_provider_unavailable` is
the required actionable classification rather than an approximate declaration.
This slice does not change current alias execution, `alias create`, `alias
check`, or the default prepare/runtime path.

## Persistence and integration boundaries

Alias documents, catalogs, and traces are not added to the Runtime v2 SQLite
schema. Callers may persist source files in their existing repository context;
the local engine persists only the resolved logical values already approved by
#110. No HTTP endpoint is added. Resolution, acquisition, provider adaptation,
planning, execution, materialization, and cutover remain separate work.
