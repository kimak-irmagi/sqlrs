# Versioned declarations Runtime v2: структура компонентов

Статус: согласовано @evilguest для issues #108, #123 и #124, 2026-09-24.

## Файлы и владение

Существующий package `runtimev2` расширяется без изменения canonical identity:

```text
backend/libs/runtime-go/
  declaration.go             существующие nested diagnostic declarations
  declaration_document.go    versioned standalone factory/transform wrappers
  recipe_declaration.go      versioned unresolved recipe aggregate
  extension_declaration.go   typed input/environment/deployment specifications
  extension_identity.go      provider-owned resolved extension identity
  diagnostics.go             capability и portability observations
```

Новые document, typed-extension, resolved-extension и diagnostic semantic values
opaque после construction, копируют slices и используют strict bounded JSON.
Существующие declaration DTO остаются mutable authoring inputs ради source
compatibility; document constructors валидируют и deep-copy их. Смысл provider
fields остаётся вне core. Resolver, filesystem, Docker, DBMS, storage и execution
dependencies не добавляются.

## Public model

Существующие nested diagnostic declarations получают optional typed-extension и
diagnostic fields, не меняя прежний JSON при отсутствии этих значений.
`FactoryDeclaration` может содержать inputs, одну execution environment, один
deployment и capability/portability observations. `TransformDeclaration` может
содержать inputs, одну execution environment и capability/portability
observations. Standalone persistence использует:

```go
type FactoryDeclarationDocument struct { /* opaque */ }
type TransformDeclarationDocument struct { /* opaque */ }
type RecipeDeclaration struct { /* opaque factory + ordered transforms */ }

func NewFactoryDeclarationDocument(FactoryDeclaration) (FactoryDeclarationDocument, error)
func NewTransformDeclarationDocument(TransformDeclaration) (TransformDeclarationDocument, error)
func NewRecipeDeclaration(FactoryDeclaration, []TransformDeclaration) (RecipeDeclaration, error)
```

`RecipeDeclaration` нельзя передать в `Build`, который по-прежнему требует
resolved `Recipe`. Accessors возвращают defensive copies. Zero-transform recipe
валиден.

Provider-owned specifications используют общие fields, но разные Go types:

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

Role-specific constructors принимают `ExtensionSpecificationInput`; implicit
conversion между roles отсутствует. Provider определяется tuple
`(role, owner, kind, specification_schema)`. Names используют существующие
identifier rules; field values — существующие limits и unique name sorting.

Resolved provider возвращает:

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

Identity immutable, но не является State transform. Core предоставляет:

```go
type ExtensionBinding struct {
    Name     string
    Identity ResolvedExtensionIdentity
}

func ExtensionFingerprint(ResolvedExtensionIdentity) (Fingerprint, error)
func ComposeResolvedFields(base []ResolvedField, bindings []ExtensionBinding) ([]ResolvedField, error)
```

`ExtensionFingerprint` использует domain
`sqlrs.runtime.v2/resolved-extension` и существующую tagged length-delimited
grammar: tag 1 schema version, tag 2 owner, tag 3 kind, tag 4 identity schema,
tag 5 canonical field set. Это новый value type/domain без изменения existing
factory, transform или State bytes.

`ComposeResolvedFields` создаёт для каждого binding field
`extension.<binding-name>` со значением extension fingerprint. Binding names —
bounded unique identifiers и сортируются по field name. Helper запрещает base
fields в reserved namespace `extension.*` и не пропускает supplied bindings.
Provider adapters обязаны использовать его при наличии resolved extensions;
conformance tests проверяют полноту binding.

`CapabilityObservation` и `PortabilityObservation` — отдельные versioned
diagnostic types с owner/kind/schema и bounded fields. Identity constructors их
не принимают.

## Wire shapes

Standalone factory/transform document:

```json
{
  "schema_version": "sqlrs.runtime.v2",
  "declaration": {}
}
```

Recipe declaration:

```json
{
  "schema_version": "sqlrs.runtime.v2",
  "factory": {},
  "transforms": []
}
```

Typed extension specification:

```json
{
  "schema_version": "sqlrs.runtime.v2",
  "owner": "sqlrs.workspace",
  "kind": "file",
  "specification_schema": "sqlrs.workspace-file.declaration.v1",
  "fields": [{"name": "path", "value": "migrations/main.sql"}]
}
```

Каждый typed extension independently versioned и безопасен при persistence вне
recipe document. Owner/kind/spec schema версионируют provider vocabulary.
`ResolvedExtensionIdentity` также содержит собственный `schema_version`, owner,
kind, identity schema и resolved fields.

Unknown/duplicate members и trailing tokens запрещены. Provider fields — closed
array, а не map, поэтому duplicate names отклоняются и serialization не зависит
от map order. Failed unmarshal atomic.

## Release и совместимость

Существующие #107 identity bytes, StateIDs, JSON golden files и public resolved
types не меняются. Уже опубликованный tag `backend/libs/runtime-go/v0.1.0`
immutable, но не содержит полной declaration boundary; следующий `go.mod`
retracts его с actionable reason.

Первая рекомендуемая external version — `backend/libs/runtime-go/v0.2.0`.
Exact intended commit сначала публикуется как
`backend/libs/runtime-go/v0.2.0-rc.1`; GA использует тот же commit только после
tests #108/#124 и external-consumer gates.
Отдельный workflow имеет два режима:

- `workflow_dispatch` preflight проверяет proposed version/commit, clean tree,
  module path, unit/conformance/golden/race/fuzz-smoke/coverage/dependency gates и
  standalone `GOWORK=off` consumer до создания tag maintainer-ом;
- push `backend/libs/runtime-go/v*` проверяет prefix, module path и exact tag
  commit, затем строит clean consumer против immutable tagged version без
  `replace` и с fresh module cache;
- post-publication verification выполняет `go list -m` и `go mod download` через
  public proxy с bounded retry для propagation и проверяет release notes со
  schema `sqlrs.runtime.v2` и source commit.

Acceptance выполняется по этапам без цикла: PR/preflight доказывает локальный
clean consumer; immutable RC tag — public proxy и checksum database; GA создаётся
из того же commit только после успешного RC; финальная проверка GA внешним
consumer закрывает issue #123. Поэтому GA-only check не требуется до merge или
до появления самого GA tag.

Workflow никогда не перемещает и не перезаписывает tag. Issue #123 закрывается
только после успешных post-publication checks.

Implementation остаётся reviewable как четыре buildable commits в одном PR:
versioned declarations/extensions, resolver framework, workspace-file/cache/
artifact implementation и release automation. Каждый commit сохраняет legacy
runtime behavior; final tag создаётся только из merged commit.
