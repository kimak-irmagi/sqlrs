# Композиция aliases Runtime v2: структура модуля и компонентов

Статус: согласовано @evilguest для issue #109 после критического анализа
дизайна, 2026-09-27.

## Граница публичного модуля

В независимый модуль Runtime v2 добавляется opt-in subpackage:

```text
backend/libs/runtime-go/
  composition/
    document.go       immutable alias document и JSON contract
    catalog.go        построение явного multi-document namespace
    expand.go         детерминированный recipe/transform expansion
    trace.go          non-semantic source и expansion diagnostics
    errors.go         стабильный composition error envelope
    validation.go     names, bounds и closed-union validation
```

Package `composition` импортирует корневой package
`github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go`. Корневой `runtimev2`
не импортирует `composition`, что предотвращает dependency cycle и сохраняет
существующий identity code неизменным. Package использует только стандартную
библиотеку Go и не импортирует CLI, YAML, resolver implementation, local-engine,
Docker, DBMS, SQLite, StateFS или execution packages.

API добавляется аддитивно в следующем свободном minor release Runtime-модуля.
Дизайн не резервирует номер версии до публикации предыдущей согласованной
release-линии.

## Модель versioned document

Версия document schema — `sqlrs.runtime.v2.aliases.v1`. Runtime v2 и revision
alias document независимо видны в имени: будущий совместимый transport aliases
Runtime v2 сможет выбрать `aliases.v2`, не переосмысливая v1.

Нормативная JSON shape:

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

Nested factory и transform declarations переиспользуют существующую diagnostic
wire shape. Alias document задаёт обязательную outer schema version и не
встраивает standalone declaration-document wrappers.

Каждое alias value является closed union, выбранным через `type`:

- `transform` требует только `declaration`;
- `recipe` требует только `base` и `steps`;
- `base` содержит ровно одно из `factory` или `recipe`;
- step содержит ровно одно из `use` или `transform`.

Unknown, duplicate, missing, extra-for-variant и `null` members отклоняются при
strict decoding. Неуспешное декодирование атомарно. Порядок object members не
имеет смысла; marshalling выводит aliases в unsigned bytewise name order и
сохраняет точный порядок steps.

## Публичные типы и операции

Предполагаемая public surface:

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

Closed unions, opaque immutable values, точные declaration/trace accessors,
defensive-copy behavior и отдельные typed recipe/transform results нормативны.
Constructors принимают slices вместо maps там, где требуется duplicate
detection. Accessors возвращают defensive copies внутренних declarations
`runtimev2` и trace collections. Zero-value documents и traces нельзя
marshal-ить; JSON unmarshalling и dedicated decode helpers имеют одинаковое
strict, bounded и atomic поведение.

## Имена, limits и владение

Alias names используют существующую lower-case Runtime identifier grammar
`[a-z][a-z0-9._-]{0,127}`. Это lookup keys, а не identity fields. Catalog
содержит не больше `MaxAliases` alias definitions суммарно из не более чем
`MaxSourceDocuments` явных source documents. Один document и одно expanded
declaration используют существующий Runtime JSON limit 4 MiB. И
`NewAliasDocument`, и `DecodeAliasDocumentJSON` измеряют canonical encoded
document, поэтому constructor input не может обойти transport bound. Сумма
canonical document bytes и source IDs в одном catalog не превышает
`MaxCatalogBytes`; один только document count поэтому не допускает gigabytes of
input. Serialized trace не превышает `MaxTraceJSONBytes`.

Alias references используют alias-name grammar. Source IDs — уникальные внутри
catalog non-empty UTF-8 strings длиной до `MaxSourceIDBytes`. CLI callers
используют slash-separated workspace-relative IDs, а не absolute host paths.
Поскольку source IDs видны в errors и traces, callers не должны помещать в них
credentials или другие secrets.

Empty object `aliases` валиден, а nil constructor slice канонизируется в такой
empty object. `NewCatalog(nil)` возвращает валидный empty immutable catalog;
любая expansion из него возвращает `missing_reference`. Zero-value
`AliasDocument` и `Catalog` остаются невалидными и не позволяют обойти
construction.

Expansion создаёт не больше `runtimev2.MaxTransforms`. Catalog accounting,
declaration transport size, trace transport size и collection growth используют
checked arithmetic до allocation или append. Implementation итеративна, поэтому
максимально длинная допустимая parent chain не исчерпывает Go stack. Declaration
и его trace проходят все size checks до возврата любого из них.

`AliasDocument`, `Catalog` и expanded results владеют immutable snapshots своих
данных. Declaration slices/maps копируются через существующие Runtime v2
constructors и accessors. Package-global registry, filesystem state, environment
variables, workspace config и mutable cache не участвуют в expansion.

## Catalog и ambiguity

`NewCatalog` не нормализует написание имён и не применяет source precedence. Его
validation precedence не зависит от порядка caller slice. Сначала он проверяет
source count, сортирует sources по raw unsigned bytewise `SourceID`, валидирует
IDs и отклоняет duplicate IDs в этом порядке, затем отклоняет zero-value
documents и с checked arithmetic суммирует total definition/canonical-byte
limits. После этого он индексирует aliases по unsigned bytewise name и
`SourceID`. Имя, присутствующее в нескольких source documents,
отклоняется как `ambiguous_reference`; candidate diagnostics сортируются по
ставшему уникальным `SourceID`. Один alias document не может содержать одинаковый
JSON member дважды, потому что strict decoder отклоняет duplicate members.

Package намеренно не определяет file discovery, imports, relative document
references и workspace lookup. Эти политики требуют отдельно согласованной CLI
или service boundary.

## Алгоритм expansion

Recipe expansion хранит явные состояния `unvisited`, `visiting` и `complete`, а
также ordered stack. `base.recipe` семантически рекурсивно, но итеративно в
реализации разворачивает один prefix. После определения factory expander
добавляет transforms этого prefix, затем обходит steps запрошенного recipe в
исходном порядке.

Transform references разрешаются в immutable declaration copies. Inline
transforms копируются в точной позиции. Completed recipe expansion можно
memoize-ить внутри одной операции, но mutable result не разделяется между
вызовами.

Expansion error precedence следует тому же deterministic traversal. Operation
сначала валидирует requested name и target kind, проходит всю base chain до
любых steps, затем emits steps от deepest recipe к target с сохранением source
order внутри каждого recipe. На каждом edge он разрешает и валидирует текущий
reference до проверки projected output/trace bounds этого occurrence. Первая
failure останавливает traversal, поэтому limit failure может предшествовать
defect в более позднем unvisited step. Гарантия validity всех references
относится к successful result, а не к unreachable work после error.

Поддерживаются следующие invariants:

- result содержит ровно одну factory;
- nested recipe добавляет полный ordered prefix;
- recipe никогда не принимается как step;
- automatic parallelization и order normalization отсутствуют;
- каждый reference, посещённый successful expansion, валидируется, хотя alias
  names исключены из output;
- при ошибке не возвращаются ни declaration, ни partial trace.

## Trace и модель ошибок

`ExpansionTrace` immutable, non-semantic и сериализуется с обязательным
`schema_version: sqlrs.runtime.v2.alias-expansion-trace.v1`. Он использует dense
node table с parent-node IDs, чтобы полные alias chains можно было восстановить
без копирования chain в каждый output origin:

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

Trace standalone transform использует тот же envelope без `factory`:

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

Node IDs — zero-based dense array indexes, а `target_node` всегда равен нулю.
Nodes представляют alias occurrences, а не deduplicated definitions. Expander
выделяет их детерминированно: сначала target, затем recipe-base occurrences до
factory, после них named transform occurrences в output order. Inline
transforms не добавляют node и ссылаются на node содержащего их recipe.
Каждый node alias удовлетворяет alias-name grammar; каждый node source ID —
non-empty valid UTF-8, bounded через `MaxSourceIDBytes`.

Pointers являются canonical RFC 6901 paths, а не произвольными pointer-shaped
strings, и не превышают `MaxPointerBytes`. Node definition pointer имеет форму
`/aliases/<escaped-alias>`. Recipe child использует
`/aliases/<escaped-parent>/base/recipe` parent recipe; named transform occurrence
использует `/aliases/<escaped-parent>/steps/<index>/use`. Factory,
named-transform и inline-transform origins используют соответствующие paths
`/base/factory`, `/declaration` и `/steps/<index>/transform`.

Recipe trace имеет closed grammar:

- node zero — recipe target и единственный не содержит `parent_node_id` и
  `reference_pointer`;
- recipe nodes образуют один non-empty contiguous prefix; каждый recipe после
  node zero имеет предыдущий recipe node как parent;
- последний recipe node владеет required factory origin;
- все оставшиеся nodes — transform leaves, каждый parented by recipe node и
  используется ровно одним transform origin;
- transform origins идут в output order: от deepest recipe к target, с
  contiguous zero-based step indexes внутри каждого recipe; inline origin
  указывает прямо на recipe node, named origin — на свой unique transform node;
- subsequence transform nodes совпадает с порядком их появления в origin array;
  unused nodes отсутствуют.

Standalone-transform trace содержит ровно один node: transform target zero без
parent/reference pointer, без factory и с одним output-zero transform origin,
указывающим на этот node. `output_index` всегда равен array position origin.
Эти invariants, canonical pointers, kinds, links и variant shape строго и
атомарно проверяются при trace decoding.

Число nodes/origins растёт линейно от visited alias occurrences и output
transforms. Число nodes проверяется по `MaxTraceNodes`, число transform origins —
по `runtimev2.MaxTransforms`, а recipe добавляет ровно один factory origin.
Checked arithmetic предшествует каждой allocation или append, способной выйти
за bound. Trace никогда не принимается
`runtimev2.NewRecipeDeclaration`,
resolver cache keys, identity constructors или fingerprint functions.

Composition failures соответствуют package sentinel и предоставляют стабильный
envelope:

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

Созданные package error strings содержат только stable code и bounded primary
diagnostic location, никогда не содержат contexts или declaration contents и не превышают
`MaxErrorTextBytes`. Cycle chains и ambiguity
candidates содержат не больше `MaxDiagnosticReferences` entries и возвращаются
в deterministic order через defensive-copy accessors. `Cycle()` начинается с
первого повторно посещённого active recipe и повторяет его последней closing
entry; его entries указывают на alias definitions, а primary error — на closing
reference. `Candidates()` сортируется по `SourceID` и также указывает на
definitions. Expansion operations
сначала валидируют alias-name grammar запрошенного target. Nested missing или
wrong-kind error указывает на referencing field; missing top-level target имеет
empty source/root pointer, а wrong-kind top-level target указывает на выбранную
definition. Cycle указывает на closing reference, а invalid document — на
rejected field, когда он существует. Для nested `runtimev2` declaration
validation composition pointer указывает на enclosing factory или transform
declaration; unwrapped `runtimev2.ValidationError.Path` сохраняет nested field
location без неоднозначной конвертации dotted path в pointer. Catalog-wide
ambiguity и aggregate-limit
errors используют empty primary source и root pointer. Expansion-output limit
errors указывают на requested target.
Errors из composition constructors, dedicated decoders, catalog и expansion
operations соответствуют `ErrInvalid` через `errors.Is`. Как и в root Runtime
package, `encoding/json` может отклонить malformed syntax до вызова
`UnmarshalJSON`; такой native syntax error не является composition error.
Underlying `runtimev2.ValidationError`, включая scalar field/count violations,
классифицируется как `invalid_document`, но сохраняет stable validation
code/path через error unwrapping. `expansion_too_large` предназначена для
aggregate document/catalog, graph, declaration-output и trace transport/count
bounds самого composition layer. Empty error `Pointer` обозначает root source
document.

## Владение path binding

Composition package валидирует и копирует declarations, но рассматривает их
provider-owned contents как opaque. Он никогда не rebases `reference`, argument,
attribute или extension field из `SourceID` либо source file location. Каждая
provider specification определяет собственный base; workspace-file resolver из
#108 по-прежнему интерпретирует field `path` относительно переданного workspace
root.

Provider-aware authoring adapter материализует declaration values до вызова
`NewAliasDocument`. Legacy adapter сначала выполняет текущее alias-file-relative
path binding и existing default selection, затем передаёт получившийся explicit
declaration. Поэтому само перемещение source не может неявно изменить path base.

## Обязательный CLI compatibility slice

Для закрытия issue #109 после release public module требуется более поздний
independently mergeable CLI slice:

```text
frontend/cli-go/internal/alias/runtimev2/
  yaml.go            strict bounded YAML-to-document adapter
  compatibility.go   legacy translation/classification boundary
```

Имя Go package — `aliasruntimev2`, что исключает collision с импортируемым root
package `runtimev2`. Application layer может импортировать и этот subpackage, и
его parent `internal/alias`; parent package не импортирует subpackage, поэтому
dependency graph остаётся acyclic.

Compatibility boundary владеет следующими key contracts:

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

`DecodeAliasDocumentYAML` проверяет `MaxYAMLBytes`, UTF-8 validity и directive
lines до parsing и при любой failure оставляет non-nil destination неизменным.
Собственный 10,000-level scanner guard pinned YAML parser служит defense in
depth; после parsing adapter итеративно применяет `MaxYAMLDepth` и `MaxYAMLNodes`
до construction любого public value. YAML document node исключён из обеих
метрик: logical root mapping имеет depth one, а node count включает каждый
reachable mapping, sequence и scalar node, включая mapping-key scalars. Alias
nodes отклоняются, а не traversed. Adapter принимает только map, sequence и
string tags, требуемые logical schema. Boolean, integer, float, timestamp,
binary и null scalars не coerced в strings. Anchors, aliases, merge keys, custom
tags, directives, duplicate keys и multiple documents отклоняются. Получившийся
constructor input независимо должен удовлетворять canonical Runtime JSON-size
limit. Эти transport limits могут отклонить очень большое value, представимое
strict JSON; каждое принятое YAML value имеет точно constructor/JSON meaning и
не создаёт вторую semantic model.

Compatibility component получает fully bound legacy alias вместе
с его location, explicit effective defaults и provider adapter. `WorkspaceRoot`
и `AliasPath` — canonical absolute paths, уже проверенные existing alias
resolver, а `AliasPath` находится строго внутри `WorkspaceRoot`. `SourceID`
должен совпадать с non-empty `filepath.ToSlash` form canonical relative alias
path: он не absolute, не содержит backslash, empty, `.` или `..` segment и
соблюдает `composition.MaxSourceIDBytes`. `TranslateLegacy` повторно проверяет
эти relationships до adapter invocation, а не доверяет direct callers.
Machine-stable `ImageSource` диагностический и не попадает в declaration.
Provider владеет
конвертацией текущих alias-file-relative inputs в explicit Runtime v2
workspace/provider form. `TranslateLegacy` требует, чтобы lower-case
`Kind()` adapter совпадал с `Definition.Kind`. Первый compatibility slice
принимает только legacy
prepare aliases; run alias получает `legacy_semantics_unsupported`, потому что
run preset не является identity-bearing Runtime v2 recipe transform. Result
возвращает либо полный Runtime v2 recipe declaration (`StatusTranslated`,
`Declaration` present, empty reason/message), либо structured classification
`legacy_only` (без declaration, с non-empty reason и actionable message), но не
оба. Result constructors поддерживают эти invariants, а accessors возвращают
immutable snapshots. Legacy-only messages — valid UTF-8, non-empty и не длиннее
`MaxCompatibilityMessageBytes`; принимаются только closed reason codes.

После structural input validation `TranslateLegacy` использует такой
precedence: run aliases возвращают `legacy_semantics_unsupported`; nil и
typed-nil adapters возвращают `runtime_v2_provider_unavailable`; prepare alias
без effective image возвращает `legacy_default_unavailable`; затем проверяется
adapter kind и вызывается adapter. Explicit legacy image требует идентичный
effective image с `ImageSourceAlias`; inherited non-empty image требует ровно
один workspace/global source; empty image требует zero source.

Adapter error всегда возвращает zero `Result`, даже если adapter одновременно
вернул value. При nil error `TranslateLegacy` повторно валидирует opaque result и
считает zero или другой invalid result contract error. Выбранный provider adapter
обрабатывает kind-specific arguments и может вернуть точный legacy-only result.
Non-nil errors предназначены только для invalid inputs либо adapter
contract/operational failures, но не для обычного отсутствия поддержки. Каждый
production adapter обязан пройти reusable conformance suite: все поддержанные
file-bearing arguments, ровно одно alias-relative to workspace/provider binding,
defaults, unsupported forms, diagnostic-provenance exclusion и declaration
immutability. До этого production
translation claim отсутствует. Component не имеет
права создавать approximate declaration: до появления complete provider adapter
обязательна actionable classification
`runtime_v2_provider_unavailable`. Slice не меняет текущий alias execution,
`alias create`, `alias check` и default prepare/runtime path.

## Границы persistence и integration

Alias documents, catalogs и traces не добавляются в SQLite schema Runtime v2.
Callers могут хранить source files в существующем repository context; local
engine сохраняет только resolved logical values, согласованные в #110. HTTP
endpoint не добавляется. Resolution, acquisition, provider adaptation, planning,
execution, materialization и cutover остаются отдельной работой.
