# Персистентность Runtime v2: компоненты и схема

Статус: согласовано пользователем для issue #110, 2026-09-27.

## Границы и владение

Публичный resolver-модуль получает непрозрачное валидируемое persistence-
представление `CacheKey` вместе с `Resolution`. В модуле локального движка
появляется `internal/runtimev2store` с DTO и интерфейсами движка. Существующий
`internal/store/sqlite.Store` реализует эти интерфейсы в отдельных файлах
`runtime_v2_*.go`, сохраняя реализацию legacy-интерфейса `internal/store.Store`.
Один SQLite connection и одна граница транзакций обслуживают оба интерфейса, но
их семантика не смешивается.

```text
backend/libs/runtime-go/resolver/
  cache_record.go           непрозрачный versioned CacheRecord и строгий codec

backend/local-engine-go/internal/
  runtimev2store/
    store.go                 интерфейсы, закрытые записи, ограниченные результаты
    materialization.go       DTO физических связей и валидация
  store/sqlite/
    runtime_v2_schema.go     защищённая транзакционная установка схемы
    runtime_v2_state.go      state identity и provenance observations
    runtime_v2_resolution.go реализация resolver.Cache
    runtime_v2_material.go   сохранение материализаций и компонентов
    runtime_v2_classify.go   явная диагностика legacy/v2
```

`runtimev2store` импортирует публичные пакеты `runtimev2` и `resolver`, а они не
импортируют движок. Новый пакет не расширяет legacy DTO состояний и не выводит ID
самостоятельно.

Логические State records и их resolved identities — неизменяемые постоянные
данные. Provenance observations — неизменяемые insert-only записи; несколько
диагностических observations могут относиться к одной resolved identity.
Resolution evidence — заменяемые данные кеша. Материализации и их компоненты —
неизменяемые физические записи, жизненным циклом которых будет владеть будущая
checkpoint policy. Эта задача даёт операции связи и чтения, но не выбирает, не
обновляет, не вытесняет и не удаляет checkpoints.

## Поверхность хранилища

На каждой границе identity интерфейс использует публичные семантические типы:

```go
type Store interface {
    PutRecipeLineage(context.Context, runtimev2.RecipeLineage) error
    PutRelativeLineage(context.Context, runtimev2.RelativeLineage) error
    GetLogicalState(context.Context, runtimev2.StateID) (StateRecord, bool, error)
    TraceLineage(context.Context, runtimev2.StateID) ([]StateRecord, error)
    ListProvenance(context.Context, runtimev2.StateID, ProvenancePageRequest) (ProvenancePage, error)

    PutMaterialization(context.Context, Materialization) error
    ListMaterializations(context.Context, runtimev2.StateID, MaterializationPageRequest) (MaterializationPage, error)
    ClassifyState(context.Context, string) (StateClassification, error)
}
```

Имя `GetLogicalState` намеренно отличается от существующего legacy-метода
`sqlite.Store.GetState(context.Context, string)`: Go не поддерживает перегрузку
методов, а SQLite-тип реализует обе границы store без обёртки и без смешения их
результирующих DTO.

`StateRecord` — закрытое read-only значение с accessors для record version,
публичного `runtimev2.State`, ровно одной публичной resolved factory или
transform identity. Provenance observations читаются через отдельный bounded
page API. У него нет exported fields или доступного caller
некорректного конструктора. Адаптер оборачивает identity в публичный provenance
без diagnostics и проверяет через `FactoryState` или `Derive`, не повторяя
формулу хеша. Каждый decoded observation обязан содержать ту же resolved identity,
но может иметь другие declaration или resolver diagnostics. `TraceLineage`
возвращает порядок от корня к endpoint, ограничен `runtimev2.MaxTransforms + 1`
и 32 MiB canonical state/identity data и проверяет каждое ребро до возврата
любого значения.

`ProvenancePageRequest` использует те же обязательный row limit 1..100 и page
budget 32 MiB, что и materialization pages. Opaque cursor связывает StateID,
high-water mark insertion sequence первой страницы и последнюю возвращённую
insertion sequence. Observations используют детерминированный first-observed-first
порядок `insertion_seq ASC` без раскрытия sequence; later observations видны
только новому traversal.

`MaterializationPageRequest` требует limit от 1 до 100 и принимает optional
opaque cursor из предыдущей страницы. Versioned cursor связывает StateID и
high-water mark insertion sequence первой страницы и последнюю возвращённую
insertion sequence, поэтому его нельзя применить к другому State. Inserts после
первой страницы исключаются из traversal. `MaterializationPage` содержит не
больше запрошенного числа значений и optional next cursor. Результаты используют
стабильный newest-persisted-first keyset-порядок `insertion_seq DESC`; sequence не
возвращается в DTO. Offset pagination и unbounded list API не предоставляются.
Materialization или provenance page также останавливается до
item, который превысил бы 32 MiB canonical returned data. Один valid item меньше
budget, поэтому каждая nonterminal page продвигается; cursor продолжает с первого
не возвращённого item.

Первая page получает committed `sqlite_sequence` таблицы (ноль при отсутствии) в
той же read transaction, что и page query. Continuation pages используют только
cursor high-water mark и last sequence, поэтому не вычисляют `MAX` по растущему
State и не удерживают database transaction между calls.

Два cursor type являются разными закрытыми values `runtimev2store` с private
fields и без text/JSON persistence contract. Они действуют только как
continuation tokens внутри запущенного engine; после restart caller начинает
новый snapshot. Private version discriminator допускает внутреннюю эволюцию и
защитно валидируется. Использование valid cursor с другим State — ошибка.

Byte budget имеет storage-independent точное определение. Provenance item стоит
сумму UTF-8 byte lengths возвращаемых record version, observation digest и
timestamp плюс canonical `provenance_json`. Materialization стоит сумму UTF-8
byte lengths всех возвращаемых string fields и metadata blobs, а для каждого
component — его string fields и metadata; присутствующее `int64` добавляет
восемь байт. Null fields добавляют ноль. Lineage cost равен сумме byte lengths
canonical `state_json` и `resolved_identity_json`. Cursor bytes и Go allocation
overhead не входят в protocol limit.

`StateClassification` содержит `LegacyPresent`, `RuntimeV2Present` и точный
`RuntimeV2RecordVersion`, если запись существует. Raw string аргумент намеренно
принимает legacy identifiers, которые не являются корректными Runtime v2
StateID. Классификация не декодирует и не принимает legacy row за новую.

SQLite store также напрямую реализует `resolver.Cache`:

```go
Load(context.Context, resolver.CacheKey) (resolver.CacheLoad, error)
Store(context.Context, resolver.CacheKey, resolver.Resolution) error
```

Замена кеша меняет evidence или resolved identity только внутри точного
версионированного ключа. Уже построенные из предыдущего resolution логические
состояния неизменяемы и не обновляются или удаляются при замене кеша.

Resolver package добавляет следующую публичную поверхность, не раскрывая поля
ключа для изменения:

```go
func NewCacheRecord(CacheKey, Resolution) (CacheRecord, error)
func DecodeCacheRecordJSON([]byte) (CacheRecord, error)
func (CacheRecord) Matches(CacheKey) bool
func (CacheRecord) Resolution() Resolution
func (CacheRecord) MarshalJSON() ([]byte, error)
```

`CacheRecord` использует существующий конверт `sqlrs.resolution-cache.v1` и
содержит digest ключа, workspace scope, descriptor resolver, normalized
declaration, resolution и checksum. `Matches` сравнивает все составляющие ключа.
Accessors возвращают defensive copies. Существующий `DirectoryCache` и новый
SQLite cache используют один codec, предотвращая расхождение формата и валидации.

### Граница релиза публичного модуля

`backend/libs/runtime-go/v0.2.0` уже опубликован и неизменяем. Экспорт
`CacheRecord` является additive-изменением публичного API и предназначен для
`backend/libs/runtime-go/v0.3.0`, которому предшествуют неизменяемые теги
`v0.3.0-rc.N` и существующие gates clean-consumer, public-proxy,
checksum-database, race, fuzz-smoke и coverage. Существующие теги не перемещаются
и не перезаписываются.

Рефакторинг `DirectoryCache` обязан сохранить побайтовую совместимость с
записями `sqlrs.resolution-cache.v1`, созданными v0.2.0. Существующие записи
читаются без перезаписи, сохраняют прежнее определение checksum и повторно
кодируются идентично. Новый публичный тип не меняет identity domains Runtime v2
или семантику StateID. Repository workspace может собирать local-engine adapter
с sibling module до публикации v0.3.0, но внешние consumers должны использовать
tagged release, а не production `replace`.

`Materialization` использует `runtimev2.StateID` и содержит несемантические ID
материализации, backend, необязательные runtime/job ID, время, общий размер и
ограниченные metadata, а также множество `MaterializationComponent` с ключом по
имени. Компонент имеет уникальное внутри материализации имя, kind, locator,
размер и ограниченные metadata. Порядок caller отбрасывается; валидация и
каноническое хранение сортируют компоненты по имени. Идемпотентный дубликат
должен быть канонически идентичен; изменение policy создаёт новый ID.

Materialization ID — непрозрачное UTF-8 значение длиной 1..256 байт без control
characters. Backend, имя и kind компонента используют identifier grammar Runtime
v2. Runtime/job ID ограничены 256 UTF-8 байтами, locator — 4096 байтами, одна
материализация содержит не более 256 компонентов. Metadata является JSON object,
по умолчанию `{}`, отклоняет duplicate keys и trailing tokens и ограничена 64
KiB на запись. Интерпретация metadata задаётся persistence record version;
изменение контракта требует новой версии. Secrets нельзя хранить в metadata или
locator.

Каждый timestamp перед сравнением и хранением нормализуется в UTC ровно с
девятью знаками долей секунды (`2006-01-02T15:04:05.000000000Z`). Для
`stored_at` и `observed_at` store получает injected clock. Идемпотентные
дубликаты сохраняют первый timestamp; одинаковые моменты в другом RFC 3339
offset или precision канонизируются в одно значение.

## Версионированная схема SQLite

Все имена с префиксом `runtime_v2_` зарезервированы форматом. Версия маркера —
`sqlrs.runtime-persistence.v1`, а семантический JSON должен указывать
`sqlrs.runtime.v2`. Каждая таблица данных повторяет `record_version`, чтобы
экспортированные строки и диагностика сохраняли собственную классификацию.

<!--ref:sql -->
[Каноническая схема Runtime v2](../../backend/local-engine-go/internal/store/sqlite/schema.sql#region=SQLRS%20RUNTIME%20V2%20SCHEMA)
<!--ref:body-->
```sql
CREATE TABLE runtime_v2_store_format (
  slot INTEGER PRIMARY KEY CHECK (slot = 1),
  format_version TEXT NOT NULL CHECK (format_version = 'sqlrs.runtime-persistence.v1'),
  semantic_version TEXT NOT NULL CHECK (semantic_version = 'sqlrs.runtime.v2')
);

CREATE TABLE runtime_v2_states (
  state_id TEXT PRIMARY KEY CHECK (length(state_id) = 71),
  record_version TEXT NOT NULL CHECK (record_version = 'sqlrs.runtime-persistence.v1'),
  state_kind TEXT NOT NULL CHECK (state_kind IN ('factory', 'derived')),
  parent_state_id TEXT CHECK (parent_state_id IS NULL OR length(parent_state_id) = 71),
  state_json BLOB NOT NULL CHECK (length(state_json) <= 4194304),
  resolved_identity_json BLOB NOT NULL CHECK (length(resolved_identity_json) <= 4194304),
  stored_at TEXT NOT NULL,
  FOREIGN KEY (parent_state_id) REFERENCES runtime_v2_states(state_id),
  CHECK ((state_kind = 'factory' AND parent_state_id IS NULL) OR
         (state_kind = 'derived' AND parent_state_id IS NOT NULL))
);
CREATE INDEX runtime_v2_states_parent ON runtime_v2_states(parent_state_id);

CREATE TABLE runtime_v2_provenance_observations (
  insertion_seq INTEGER PRIMARY KEY AUTOINCREMENT,
  state_id TEXT NOT NULL,
  observation_digest TEXT NOT NULL CHECK (length(observation_digest) = 71),
  record_version TEXT NOT NULL CHECK (record_version = 'sqlrs.runtime-persistence.v1'),
  provenance_json BLOB NOT NULL CHECK (length(provenance_json) <= 4194304),
  observed_at TEXT NOT NULL,
  UNIQUE (state_id, observation_digest),
  FOREIGN KEY (state_id) REFERENCES runtime_v2_states(state_id) ON DELETE CASCADE
);
CREATE INDEX runtime_v2_provenance_state_page
  ON runtime_v2_provenance_observations(state_id, insertion_seq ASC);

CREATE TABLE runtime_v2_resolutions (
  cache_key TEXT PRIMARY KEY CHECK (length(cache_key) = 64),
  record_version TEXT NOT NULL CHECK (record_version = 'sqlrs.runtime-persistence.v1'),
  cache_record_json BLOB NOT NULL CHECK (length(cache_record_json) <= 4194304),
  stored_at TEXT NOT NULL
);

CREATE TABLE runtime_v2_materializations (
  insertion_seq INTEGER PRIMARY KEY AUTOINCREMENT,
  materialization_id TEXT NOT NULL UNIQUE
    CHECK (length(CAST(materialization_id AS BLOB)) BETWEEN 1 AND 256),
  record_version TEXT NOT NULL CHECK (record_version = 'sqlrs.runtime-persistence.v1'),
  state_id TEXT NOT NULL,
  backend TEXT NOT NULL CHECK (length(CAST(backend AS BLOB)) BETWEEN 1 AND 128),
  runtime_id TEXT CHECK (runtime_id IS NULL OR length(CAST(runtime_id AS BLOB)) BETWEEN 1 AND 256),
  job_id TEXT CHECK (job_id IS NULL OR length(CAST(job_id AS BLOB)) BETWEEN 1 AND 256),
  created_at TEXT NOT NULL,
  size_bytes INTEGER CHECK (size_bytes IS NULL OR size_bytes >= 0),
  metadata_json BLOB NOT NULL CHECK (length(metadata_json) BETWEEN 2 AND 65536),
  FOREIGN KEY (state_id) REFERENCES runtime_v2_states(state_id)
);
CREATE INDEX runtime_v2_materializations_state_page
  ON runtime_v2_materializations(state_id, insertion_seq DESC);

CREATE TABLE runtime_v2_materialization_components (
  materialization_id TEXT NOT NULL,
  record_version TEXT NOT NULL CHECK (record_version = 'sqlrs.runtime-persistence.v1'),
  name TEXT NOT NULL CHECK (length(CAST(name AS BLOB)) BETWEEN 1 AND 128),
  kind TEXT NOT NULL CHECK (length(CAST(kind AS BLOB)) BETWEEN 1 AND 128),
  locator TEXT NOT NULL CHECK (length(CAST(locator AS BLOB)) BETWEEN 1 AND 4096),
  size_bytes INTEGER CHECK (size_bytes IS NULL OR size_bytes >= 0),
  metadata_json BLOB NOT NULL CHECK (length(metadata_json) BETWEEN 2 AND 65536),
  PRIMARY KEY (materialization_id, name),
  FOREIGN KEY (materialization_id)
    REFERENCES runtime_v2_materializations(materialization_id) ON DELETE CASCADE
);

CREATE TRIGGER runtime_v2_states_immutable BEFORE UPDATE ON runtime_v2_states
BEGIN SELECT RAISE(ABORT, 'runtime v2 state is immutable'); END;
CREATE TRIGGER runtime_v2_provenance_immutable
BEFORE UPDATE ON runtime_v2_provenance_observations
BEGIN SELECT RAISE(ABORT, 'runtime v2 provenance is immutable'); END;
CREATE TRIGGER runtime_v2_materializations_immutable
BEFORE UPDATE ON runtime_v2_materializations
BEGIN SELECT RAISE(ABORT, 'runtime v2 materialization is immutable'); END;
CREATE TRIGGER runtime_v2_materialization_components_immutable
BEFORE UPDATE ON runtime_v2_materialization_components
BEGIN SELECT RAISE(ABORT, 'runtime v2 materialization component is immutable'); END;
CREATE TRIGGER runtime_v2_states_no_delete BEFORE DELETE ON runtime_v2_states
BEGIN SELECT RAISE(ABORT, 'runtime v2 state deletion is not enabled'); END;
CREATE TRIGGER runtime_v2_provenance_no_delete
BEFORE DELETE ON runtime_v2_provenance_observations
BEGIN SELECT RAISE(ABORT, 'runtime v2 provenance deletion is not enabled'); END;
CREATE TRIGGER runtime_v2_materializations_no_delete
BEFORE DELETE ON runtime_v2_materializations
BEGIN SELECT RAISE(ABORT, 'runtime v2 materialization deletion is not enabled'); END;
CREATE TRIGGER runtime_v2_materialization_components_no_delete
BEFORE DELETE ON runtime_v2_materialization_components
BEGIN SELECT RAISE(ABORT, 'runtime v2 materialization component deletion is not enabled'); END;
```
<!--ref:end-->

Семантические JSON-столбцы используют строгие публичные codecs. `state_id`,
`state_kind` и `parent_state_id` являются индексируемыми копиями для контроля
целостности и обязаны совпадать с декодированным `state_json`.
`resolved_identity_json` содержит точно соответствующую публичную resolved
factory или transform identity. Каждый `provenance_json` содержит публичное
provenance value с той же identity; diagnostics могут различаться между
observations. `cache_record_json` является публичным закрытым конвертом
`resolver.CacheRecord`, а `cache_key` — его индексируемой integrity copy.

`observation_digest` состоит из `sha256:` и lowercase SHA-256 точных канонических
байтов публичного provenance JSON. Это storage deduplication key, а не identity
Runtime v2. При конфликте digest вставка проверяет равенство канонических байтов.
Application validation дополнительно к SQL size checks обеспечивает identifier
grammar, UTF-8, JSON shape, bound количества компонентов и канонические
fixed-width UTC timestamps.

Временные метки и все физические поля намеренно отсутствуют в семантической
валидации и ограничениях уникальности. Каждый `insertion_seq` — storage-only
pagination watermark, который не входит в DTO, identity, observation digest или
duplicate comparison. Внешних ключей к legacy `states` нет.
UPDATE triggers обеспечивают неизменяемость записей. DELETE triggers намеренно
оставляют GC logical state, provenance и физических записей за пределами этой
задачи; будущая согласованная lifecycle schema должна заменить их до появления
delete APIs.

## Установка и совместимость

`sqlite.InstallRuntimeV2Schema(ctx, tx)` — единственный low-level installer и
verifier. Он запускается после `PRAGMA foreign_keys = ON`, до изменения v2-
объектов проверяет все зарезервированные имена в `sqlite_master` и либо создаёт
полный формат, либо проверяет существующий поддерживаемый формат, либо возвращает
ошибку для rollback caller. Проверки формы таблиц, индексов и triggers отклоняют
shadowing, частичную установку и неизвестные форматы.

Существующий `backend/local-engine-go/internal/store/sqlite/schema.sql` остаётся
каноническим источником DDL по ADR 0003. Выделенные delimiters statements Runtime
v2 встраиваются один раз и используются installer; `runtime_v2_schema.go`
содержит orchestration и shape verification, но не вторую копию SQL. Schema block
архитектурного документа синхронизируется из этого source документационным
schema tool.

Production `managedstore.Initialize` вызывает low-level installer в существующей
startup transaction и для fresh, и для уже managed stores; in-memory builder
эталонной схемы вызывает ту же функцию. Идемпотентная обёртка
`sqlite.EnsureRuntimeV2Schema(ctx, db)` открывает транзакцию вокруг того же
installer и вызывается из `sqlite.initDB`, покрывая прямые `Open`/`New` и rc.6
upgrade fixtures. В production последующий `sqlite.New` только повторно проверяет
схему, установленную `managedstore.Initialize`. Второй реализации DDL нет.

Installer не меняет connection locking policy и не добавляет retries. Каждый
поддерживаемый constructor включает foreign keys на единственном SQLite
connection и сохраняет `PRAGMA busy_timeout = 0`; внешний lock возвращается как
`SQLITE_BUSY` без внутреннего ожидания согласно ADR 0015.

Для установки не требуется пустота legacy-таблиц и не вызывается более строгий
gate миграции managed identity, поскольку ни одна legacy-запись не принимается за
новую. Таблицы и строки rc.6 остаются побайтно под текущим путём кода. Runtime v2
getters добавляют предикат версии записи даже при наличии CHECK constraint и не
делают fallback к неверсионированному запросу.

## Согласованность и транзакции

Recipe/relative lineage, одна материализация со всеми компонентами и установка
схемы являются отдельными атомарными транзакциями. Запись кеша resolution
атомарна для одного ключа. State вставляется от родителя к потомку и неизменяем.
Для duplicate state identity сохранённый JSON строго декодируется и канонически
кодируется заново; канонические байты identity должны совпасть. Альтернативный
соответствующий provenance вставляется как ещё один observation, а не считается
конфликтом. Коллизия observation digest дополнительно требует равенства
канонических байтов. Duplicate materialization сравнивается как канонический DTO
с компонентами, отсортированными по имени.

Cancellation или failure до commit откатывает transaction. Если `Commit`
возвращает ошибку после того, как SQLite мог зафиксировать изменения, операция
возвращает эту ошибку и не утверждает, какая полная generation видима. После
reopen caller может увидеть только полную старую или полную новую generation;
идемпотентность каждой записи гарантирует, что повтор той же операции сходится к
полной новой generation. Partial schema, lineage, cache record или
materialization недопустимы.

Чтение валидирует JSON, восстанавливает provenance без diagnostics из сохранённой
resolved identity, повторно проверяет связь State через публичное семантическое
ядро, сравнивает индексные копии и не возвращает частичное значение. Observations
валидируются page by page; corrupt item отклоняет всю свою page, не влияя на
State lookup или другую page. Обход lineage обнаруживает отсутствующих
родителей и циклы и применяет byte budget. Чтение provenance и materialization
проверяет bounds и использует описанные bounded snapshot-page contracts;
components возвращаются в каноническом порядке имён. Точные запросы State,
parent, provenance, cache key и materialization page используют объявленные
индексы; lineage выполняет не более одного
индексированного parent lookup на каждый ограниченный State.
