# Resolver Runtime v2: структура модуля и компонентов

Статус: согласовано @evilguest для issue #108, 2026-09-24.

## Граница модуля

В существующий независимый модуль добавляется package `resolver`:

```text
backend/libs/runtime-go/resolver/
  doc.go                 package contract и public limits
  declaration.go         generic и normalized declarations
  binding.go             resolved-extension fingerprint bindings
  resolution.go          resolution, provenance, freshness и outcomes
  registry.go            immutable dispatch по kind
  manager.go             cache/revalidation orchestration
  cache.go               cache interface и strict record envelope
  directory_cache.go     restart-safe atomic directory implementation
  cache_prune.go         bounded namespace/age/count pruning
  artifact_store.go      trusted content-addressed acquired artifacts
  replace_unix.go        atomic cache publication на Unix
  replace_windows.go     atomic cache publication на Windows
  errors.go              общая модель operations/errors
  workspace_file.go      reference file resolver
  filesystem_class.go   cheap-revalidation capability classification
  filemeta_unix.go       Unix continuity evidence
  filemeta_windows.go    Windows continuity evidence
  filemeta_fallback.go   безопасное UNKNOWN-поведение на других ОС
```

Package принимает versioned `runtimev2.InputDeclaration` и возвращает общий
`runtimev2.ResolvedExtensionIdentity` из issue #124. Engine adapter явно включает
resolved extension в factory или transform identity. В остальном используется
Go standard library, включая traversal-resistant Go 1.25 `os.Root`; нет импортов
engine, Docker, Liquibase, DBMS, SQLite или StateFS.

## Public contracts

```go
type Resolver interface {
    Descriptor() Descriptor
    Normalize(context.Context, Workspace, runtimev2.InputDeclaration) (NormalizedDeclaration, error)
    Resolve(context.Context, Workspace, NormalizedDeclaration) (Resolution, error)
    ValidateResolution(Resolution) error
    Revalidate(context.Context, Workspace, Resolution) (Revalidation, error)
    Acquire(context.Context, Workspace, Resolution) (Artifact, error)
}

type Cache interface {
    Load(context.Context, CacheKey) (CacheLoad, error)
    Store(context.Context, CacheKey, Resolution) error
}

type PrunableCache interface {
    Prune(context.Context, PrunePolicy) (PruneResult, error)
}

type ArtifactStore interface {
    PublishVerified(context.Context, io.Reader, ContentDigest) (Artifact, error)
}

func NewRegistry(resolvers ...Resolver) (Registry, error)
func NewManager(registry Registry, cache Cache) (Manager, error)
func NewWorkspaceFileResolver(artifacts ArtifactStore) (Resolver, error)
func (m Manager) ResolveCurrent(ctx context.Context, workspace Workspace, declaration runtimev2.InputDeclaration) (Outcome, error)
```

Точное Go-написание может уточняться при реализации одобренных тестов, но
границы нормативны:

- dispatch выполняется по validated role/owner/kind/specification-schema tuple,
  duplicate registration запрещён;
- normalization, resolution, revalidation и acquisition вызываются независимо;
- manager вызывает `ValidateResolution` после cache load и после `Resolve`, до
  использования или persistence provider evidence;
- `Resolution` отделяет identity от declaration, provenance, freshness и evidence;
- `Artifact` содержит physical handle/location вне identity;
- file acquisition возвращает только digest-verified immutable artifact из
  trusted `ArtifactStore`, но не mutable workspace handle;
- `Outcome` показывает cache outcome и точный revalidation status;
- constructors и strict decoders копируют inputs, запрещают unknown/duplicate
  members и не меняют receiver при ошибке.

## Совместимость identity

`runtimev2.ResolvedExtensionIdentity` содержит `Owner`, `Kind`,
`IdentitySchema` и canonical `[]runtimev2.ResolvedField`. Он не является factory
или transform. Consuming adapter явно map/compose его fields в Runtime v2
constructor input; helper one-resource-to-one-transform отсутствует. Поэтому
файл нельзя ошибочно принять за полный execution transform.

`runtimev2.ExtensionFingerprint` использует новый domain-separated canonical
record из extension schema, owner, kind, identity schema и всех resolved fields.
`runtimev2.ComposeResolvedFields` принимает uniquely named `ExtensionBinding` и
возвращает обычные identity-bearing `ResolvedField` с reserved names
`extension.*`. Duplicate bindings запрещены, ни один supplied extension не может
быть пропущен. Adapter обязан использовать helper; conformance tests перечисляют
extensions declaration и доказывают влияние каждого на factory/transform fingerprint.

Workspace-file identity:

```json
{
  "schema_version": "sqlrs.runtime.v2",
  "owner": "sqlrs.workspace",
  "kind": "file",
  "identity_schema": "sqlrs.workspace-file.v1",
  "fields": [
    {"name": "content.digest", "value": "sha256:..."}
  ]
}
```

Normalized/absolute paths, timestamps, file ID, cache key и acquisition location
исключены из identity. Provenance хранит original/normalized declarations,
resolver implementation/semantic version и resolution time.

## Revalidation и artifact

`RevalidationStatus` — закрытый enum `CURRENT`, `STALE`, `UNKNOWN`.
`Revalidation` содержит stable reason и refreshed freshness, но не replacement
identity: её создаёт только `Resolve`.

`Freshness` хранит observation time и provider evidence. File evidence — private
versioned strict-JSON value с size, mtime, stable file identity, change token и
filesystem class и признаком strong continuity. Weak fields никогда не входят в identity. Generic
cache валидирует и ограничивает envelope; `ValidateResolution` применяет закрытую
provider schema и отклоняет missing/unknown evidence до revalidation.

`Artifact` — небольшой interface с artifact kind и `Close`; concrete values
принадлежат provider. File resolver возвращает opened immutable content-addressed
artifact и diagnostic store location. Store stream/hash-ит temporary file,
проверяет expected digest, sync-ит, atomic publish-ит и проверяет existing object
перед reuse. Возвращаемый artifact handle относится к той же открытой generation
object, bytes которой проверены; unchecked reopen по path после validation
запрещён. OCI/Git/package providers смогут вернуть свои handles.

## Модель ошибок

Operations возвращают `*resolver.Error` с operation, stable code и resolver descriptor. System
errors доступны через `Unwrap`, contents файлов не раскрываются. Codes включают
`invalid_declaration`, `unsupported_kind`, `duplicate_kind`, `not_found`,
`unsafe_path`, `not_regular`, `changed_during_resolution`, `invalid_resolution`,
`incompatible_cache`, `corrupt_cache`, `permission_denied`, `unavailable`, `io`.
Partial resolution/artifact не возвращается; context errors остаются matchable.
Если fallback resolution падает после revalidation, `Error` сохраняет предыдущий
`RevalidationStatus` и stable reason без partial new resolution.

## Владение данными и concurrency

`Registry`, `Manager`, `Resolution`, `ResolvedExtensionIdentity` и normalized declarations
immutable и безопасны для concurrent reads. Global registry нет. Cache владеет
только своей directory. Writes сериализуются per-key внутри process; atomic
replacement показывает reader старую или новую полную record. Cross-process
last-writer-wins безопасен благодаря self-validation key и semantic version.
Same-directory temporary files и platform-specific replacement публикуют полные
generations; readers игнорируют temporary files и считают truncated record
invalidation, а не hit.
Это доказывают subprocess conformance tests, а не только goroutine race tests.

Workspace files, cache records и content-addressed acquired artifacts persistent;
registry, locks, manager и opened handles находятся in memory. Trusted
cache/artifact directories расположены вне mutable workspace и отклоняют links.
На Unix новые roots/files получают owner-only modes. На Windows objects наследуют
ACL engine-owned root; package не расширяет ACL и отклоняет reparse roots. Если
deployment не может создать trusted root, store construction завершается ошибкой,
а не silent downgrade. Checksums сохраняются, но ownership directory — граница
authentication. Logical states/materializations не сохраняются — это #110.

`DirectoryCache` также реализует `PrunableCache`. Policies ограничивают inert
schema/semantic-version namespaces по age и entry count. Pruning использует
rooted no-link discipline, пропускает active temporary files и не выполняется
неявно на read path.

## Граница совместимости

Package — dormant library code. Он не меняет legacy IDs/cache, команды, HTTP или
execution. Record содержит cache-schema и resolver semantic versions; unknown
versions fail closed и не переинтерпретируют старые данные.

## Версионирование module release

Go версионирует nested module, а не отдельные packages. Tag
`backend/libs/runtime-go/v0.1.0` существует на commit `52bb255`, но был
опубликован до полной declaration boundary из issue #124. Он immutable и будет
retracted в следующем `go.mod`; external consumers не должны выбирать его для
новых зависимостей.

Совместный release #108/#124 сначала публикует
`backend/libs/runtime-go/v0.2.0-rc.1`. После clean-consumer/public-proxy gates на
exact RC commit immutable GA tag нацелен на:

```text
backend/libs/runtime-go/v0.2.0
```

После публикации consumer фиксирует его так:

```text
go get github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go@v0.2.0
```

Package `resolver` не получает отдельный tag. Module release version, resolver
semantic version, cache schema и identity schema независимы и не
переинтерпретируют друг друга автоматически.
