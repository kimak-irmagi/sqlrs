# Семантическое ядро Runtime v2: структура модуля и компонентов

Статус: согласовано @evilguest для issue #107, 2026-09-23. Исправления по
критическому анализу согласованы в той же беседе до проектирования тестов.

## Граница публичного модуля

Создать `backend/libs/runtime-go` как самостоятельный вложенный Go-модуль:

```text
module github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go
package runtimev2
```

Путь соответствует переименованному каноническому репозиторию. Модуль добавляется
в корневой `go.work`. Release tags используют префикс каталога module. Tag
`backend/libs/runtime-go/v0.1.0` указывает на merge commit #107 `52bb255`, но был
опубликован до полной declaration boundary и заменяется планируемым release
#108/#124 `backend/libs/runtime-go/v0.2.0`. Runtime v2 — имя семантической схемы;
оно не требует Go module suffix `/v2`, пока сам Go-модуль не достиг major v2.

Модуль использует только стандартную библиотеку Go и не импортирует внутренности
local engine, Docker, СУБД, StateFS, Liquibase, SQLite или пакеты сервисов. Его
можно импортировать без workspace репозитория.

```text
backend/libs/runtime-go/
  go.mod
  doc.go                 контракт пакета и константы схемы
  declaration.go         mutable unresolved/diagnostic transport DTOs
  identity.go            immutable resolved factory/transform identities
  provenance.go          declaration плюс resolved identity observations
  state.go               fingerprints и immutable logical states
  canonical.go           нормативный binary encoder и SHA-256 helpers
  lineage.go             recipe, steps, Build и Extend
  validation.go          bounded decoding и structured errors
  testdata/golden/*.json transport values и ожидаемые identities
```

## Публичная модель

Declaration DTOs — изменяемые transport inputs, которые не участвуют в hash:

- `FactoryDeclaration` и `TransformDeclaration` сохраняют unresolved kind,
  reference, arguments и ограниченные diagnostic attributes;
- `ResolverObservation` может сохранять данные реализации/сборки resolver-а.

Identity-bearing semantic values неизменяемы:

- `ResolvedField{Name, Value}` представляет один provider-defined immutable input;
- `ResolvedFactoryIdentity` и `ResolvedTransformIdentity` содержат schema version,
  provider, kind, identity schema и resolved fields;
- `State` содержит ID и provenance корня либо родителя/преобразования;
- `LineageStep` содержит `TransformProvenance`, проверенный fingerprint и
  полученное производное `State`;
- `RecipeLineage` содержит provenance фабрики, корень и упорядоченные steps;
- `RelativeLineage` содержит внешнюю опору и упорядоченные новые steps.

`FactoryProvenance` и `TransformProvenance` разделяют identity и необязательные
diagnostic declaration/observation. Поэтому у двух provenance records может быть
разное написание declaration при равной resolved identity.

Эти declaration DTO из #107 и их вложенный JSON остаются compatibility/golden
baseline. Новые standalone inputs используют документы с обязательной версией и
typed extension roles из
[структуры declarations](runtime-v2-declaration-structure.RU.md); поле версии не
добавляется задним числом в legacy nested shape.

Opaque semantic types используют закрытые поля. Constructors и проверяемые JSON
decoders копируют slices и maps; accessors возвращают values или защитные копии.
Custom `MarshalJSON` и `UnmarshalJSON` дают публичную форму хранения/передачи, не
позволяя изменить уже проверенное значение на месте.

## Контракт JSON transport

Следующие имена полей и shapes нормативны. Порядок members object незначим.
`MarshalJSON` выдаёт resolved fields в каноническом порядке имён.

Resolved factory и transform identities имеют общую форму:

```json
{
  "schema_version": "sqlrs.runtime.v2",
  "provider": "example",
  "kind": "psql",
  "identity_schema": "example.psql.v1",
  "fields": [{"name": "content.digest", "value": "sha256:..."}]
}
```

`fields` обязателен и может быть пустым. Сохранённые для совместимости #107 legacy
nested declaration и resolver diagnostics:

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

Factory и transform provenance используют общий envelope. `declaration` и
`resolver` необязательны; при отсутствии они пропускаются, но не равны `null`:

```json
{
  "identity": {},
  "declaration": {},
  "resolver": {}
}
```

Factory state и derived state — разные shapes с discriminator `state_kind`:

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

Остальные публичные containers:

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

Это `LineageStep`, `Recipe`, `RecipeLineage` и `RelativeLineage`. Каждый `{}`
placeholder содержит полную соответствующую shape, а не открытый extension object.

Каждый required member присутствует и не равен null. Необязательные diagnostic
members могут отсутствовать, но при наличии они non-null и valid. Decoder
отклоняет unknown members, duplicate member names на каждом уровне, invalid UTF-8
и trailing JSON tokens. При ошибке `UnmarshalJSON` проверяет temporary value и
оставляет существующий non-zero receiver неизменным. Успешное decoding атомарно
заменяет receiver.

## Публичные операции

Пакет предоставляет чистые операции без глобального реестра и I/O:

```go
func NewFactoryIdentity(input FactoryIdentityInput) (ResolvedFactoryIdentity, error)
func NewTransformIdentity(input TransformIdentityInput) (ResolvedTransformIdentity, error)
func FactoryState(factory FactoryProvenance) (State, error)
func TransformFingerprint(transform ResolvedTransformIdentity) (Fingerprint, error)
func Derive(parent StateID, transform TransformProvenance) (LineageStep, error)
func Build(recipe Recipe) (RecipeLineage, error)
func Extend(anchor StateID, transforms []TransformProvenance) (RelativeLineage, error)
func DecodeJSON(data []byte, target json.Unmarshaler) error
```

`Recipe` — упорядоченный input из provenance фабрики и преобразований. Для
factory-only recipe `Build` возвращает корень и пустой список steps. `Extend`
принимает любой корректный StateID и упорядоченный suffix; пустой suffix означает
неизменённую опору без выдуманного состояния или преобразования.

`RecipeLineage.Endpoint()` возвращает корень или состояние последнего step.
`RelativeLineage.EndpointID()` возвращает опору или StateID последнего step.
Проверяемый JSON decoder пересчитывает каждый вложенный fingerprint и StateID и
останавливается при первом несовпадении.

Read-only accessor-ы открывают все scalar-поля resolved identity и defensive
copies полей, declarations, resolver observations, recipe transforms и lineage
steps. `Recipe.Factory()`, `RecipeLineage.Factory()` и
`RelativeLineage.Anchor()` предоставляют immutable inputs, необходимые внешним
реализациям engine, не раскрывая внутреннее хранение пакета.

## Контракт проверки

```go
type ValidationCode string

type ValidationError struct {
    Code ValidationCode
    Path string
}
```

`Path` использует стабильную запись поля/индекса, например `steps[2].state.id`.
Ошибка не повторяет значения полей. `errors.Is(err, ErrInvalid)` совпадает со
всеми validation errors; для кода и пути используется `errors.As`. Пакет
экспортирует limits и константу схемы, чтобы adapters заранее отклоняли лишнюю
работу.

JSON decoder отклоняет неизвестные поля. Добавление diagnostics требует релиза
схемы/API и не игнорируется молча. Canonical hash encoding не зависит от порядка
JSON members и представления diagnostics.

На trust boundaries используется `DecodeJSON`: он вызывает package decoder
напрямую и возвращает `ValidationError` даже для malformed syntax и trailing
tokens. Обычный `encoding/json.Unmarshal` поддержан для valid JSON, но Go может
вернуть собственный syntax error до вызова `UnmarshalJSON` на malformed input.

## Владение и граница интеграции

Модуль владеет immutable semantic values, canonical encoding, проверкой и
детерминированным построением lineage в памяти. Постоянных данных у него нет.
Движки владеют resolver implementations, storage transactions, авторизацией,
сохранением provenance и связью logical state с нулём или несколькими
материализациями.

Issue #107 не меняет схему БД, HTTP или CLI. Прежние ID локальных состояний не
мигрируют и не переосмысливаются молча. Будущий adapter может отобразить managed
database identity в документированную factory identity schema, но внедрение в
local persistence/cache остаётся отдельным изменением.

## Совместимость и conformance

`SchemaVersion` строго равна `sqlrs.runtime.v2`; неизвестные версии запрещены.
Изменение identity fields или canonical bytes требует нового namespace, доменов
и golden vectors. Diagnostic transport развивается по semver Go-модуля независимо
от namespace Runtime identity.

Golden JSON fixtures покрывают factory-only, one-step, multi-step, relative и
recipe/relative histories с одинаковым endpoint. Fixture содержит declarations,
resolved identities, ожидаемые fingerprints, все states/steps и endpoint. Тесты
декодируют, повторно кодируют и пересчитывают каждую identity.

Import conformance содержит две compile-проверки:

- external-package test в local-engine импортирует публичный модуль и доказывает
  отсутствие обратной/циклической зависимости;
- независимый consumer test module вне root `go.work` получает путь через явный
  test-only `replace` и не импортирует local-engine. Release verification повторяет
  проверку без `replace` для опубликованного tag вложенного модуля.
