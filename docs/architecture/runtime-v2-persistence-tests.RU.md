# Персистентность Runtime v2: дизайн тестов

Статус: одобрено пользователем для issue #110 после второго критического ревью,
2026-09-27.

Примечание о версии: проверки старого формата записи ниже относятся к выпуску
задачи #110. Для канонической записи и адаптера SQLite версии 0.5.0 нужен
[новый перечень проверок](runtime-v2-canonical-resolver-tests.RU.md).

План проверяет согласованные
[поток взаимодействий](runtime-v2-persistence-flow.RU.md) и
[дизайн компонентов/схемы](runtime-v2-persistence-structure.RU.md). Тесты
проверяют публичные семантические контракты и persistent invariants, а не layout
private helpers. Ни один тест не переключает default prepare/runtime path на
Runtime v2.

## 1. Уровни тестов и fixtures

- Тесты публичного `runtime-go/resolver` проверяют opaque codec `CacheRecord` и
  сохраняют опубликованный cache wire format v0.2.0.
- Unit tests local engine проверяют закрытые DTO и validation
  `runtimev2store`.
- SQLite integration tests используют и изолированные in-memory DB, и file-backed
  DB, повторно открываемые новым store instance. Restart, upgrade, transaction и
  durability assertions выполняются file-backed.
- Checked-in representative rc.6 database fixture создаётся pinned historical
  rc.6 revision, а не вручную текущим кодом. Он содержит legacy states,
  instances, names, queue data, settings/unrelated objects и не содержит v2
  marker; revision generator и checksum зафиксированы.
- Independent golden persistence rows создаются из reviewed public Runtime v2
  JSON/identity vectors без вызова проверяемого SQLite adapter.
- Corruption tests меняют disposable fixture только через явно test-only raw
  connection с отключёнными foreign keys/check constraints, после чего БД
  открывается обычным store. Production API этот bypass не предоставляет.
- Failure injection покрывает transaction begin/statement/commit, cancellation и
  schema-installation boundaries. Pre-commit failure обязан выполнить rollback;
  ambiguous commit показывает только полную старую или новую generation, а
  идемпотентный retry сходится к новой. Acceptance criteria о SQLite constraints
  или restart behavior не проверяются только mocks.
- Timestamps и materialization IDs используют injected deterministic sources;
  тесты не зависят от wall-clock ordering.

## 2. Публичный контракт cache record

- **CR-01 — wire compatibility v0.2.0:** декодировать checked-in cache records,
  записанные runtime-go v0.2.0, сопоставить полные ключи и повторно получить
  byte-identical encoding с тем же checksum и schema
  `sqlrs.resolution-cache.v1`.
- **CR-02 — construction и round trip:** построить `CacheRecord` для каждой
  поддерживаемой declaration role, выполнить marshal/decode и сохранить key,
  descriptor, normalized declaration, resolution identity и provider evidence.
- **CR-03 — full-key matching:** отдельно изменить key digest, workspace scope,
  role, owner, kind, specification schema, resolver semantic version и normalized
  declaration; `Matches` должен отклонить каждое изменение.
- **CR-04a..j — matrix strict trust boundary:** `a` unknown member, `b` duplicate
  member, `c` null required member, `d` missing required member, `e` trailing
  token, `f` oversized input, `g` invalid checksum, `h` unsupported schema, `i`
  malformed identity и `j` invalid zero value проверяют документированную
  resolver error.
- **CR-05 — defensive copies:** изменение constructor inputs и значений из
  `Resolution()` не меняет record или его последующую serialization.
- **CR-06 — adapter conformance:** `DirectoryCache` и in-memory reference adapter
  на `CacheRecord` возвращают одинаковые hit/miss/corrupt/incompatible outcomes
  и сохраняют существующие atomic-publication tests.
- **CR-07 — consumption публичного модуля:** standalone `GOWORK=off` consumer
  компилируется с additive API в обычном PR suite.
- **RG-01 — release-only verification:** verification v0.3.0 RC/GA отдельно
  повторяет gates clean-consumer, public-proxy, checksum, race, fuzz-smoke,
  dependencies и coverage без перемещения tags v0.2.0; это не обычный
  unit/integration test.

## 3. Установка схемы и безопасность upgrade

- **SC-01 — fresh install:** создать exact marker, tables, indexes, foreign keys,
  CHECK constraints и UPDATE/DELETE triggers в одной transaction.
- **SC-02 — canonical DDL и parity constructors:** schema-sync check доказывает,
  что `schema.sql` — единственный Runtime v2 DDL source и architecture block
  генерируется из него; `sqlite.Open`, `sqlite.New` и explicit ensure wrapper
  вызывают один installer и создают один normalized object manifest без
  сравнения несущественного whitespace `sqlite_master`.
- **SC-03 — parity managed startup:** fresh и already-managed paths
  `managedstore.Initialize` устанавливают/проверяют ту же v2 schema; следующий
  `sqlite.New` выполняет идемпотентную проверку.
- **SC-04 — restart idempotence:** многократный close/reopen не меняет marker,
  normalized object manifest, records или timestamps.
- **SC-05 — upgrade заполненного rc.6:** начать с checked-in fixture, добавить v2
  schema и доказать неизменность каждой legacy/unrelated row и object.
- **SC-06 — реальная rc.6 rollback/disable compatibility:** после установки v2
  запустить pinned historical rc.6 compatibility harness на копии, выполнить
  representative legacy CRUD, повторно открыть текущим кодом и доказать, что
  старый binary игнорировал reserved tables без reinterpretation или повреждения
  обоих namespace.
- **SC-07a..i — matrix отказа collision/shape:** `a` wrong object kind, `b`
  case-insensitive reserved-name collision, `c` missing object, `d`
  extra/reordered column, `e` altered index columns/order, `f` altered
  foreign-key action/check, `g` altered trigger body, `h` malformed marker и `i`
  unknown version завершаются до mutation.
- **SC-08 — atomic/ambiguous failure:** inject cancellation и каждую достижимую
  DDL/commit failure. Pre-commit failures оставляют старую schema; ambiguous
  commit после reopen показывает только полную старую или полную v2 schema, а
  retry сходится к verified v2.
- **SC-09 — constraints/connections:** доказать `foreign_keys=ON` и
  `busy_timeout=0` для каждого constructor, затем fail-closed foreign keys,
  size/version checks, immutable UPDATE и out-of-scope DELETE triggers.

## 4. Logical states, identities и provenance

- **ST-01 — recipe restart round trip:** сохранить factory-only, one-step и
  multi-step `RecipeLineage`; после reopen получить те же StateID, fingerprints,
  resolved identities, parent links и endpoint.
- **ST-02 — relative lineage:** сохранить suffix от известного anchor и
  восстановить traversal root-to-endpoint; atomically отклонить отсутствующий
  anchor.
- **ST-03 — logical-only state:** сохранить и прочитать states с нулём
  materialization rows.
- **ST-04 — idempotent identity:** повтор той же lineage и canonical identity
  успешен без изменения `stored_at` или дублирования observations; первый
  `observed_at` сохраняется при observation deduplication.
- **ST-05 — multiple diagnostics:** два provenance с одной resolved identity и
  разными declaration spelling или resolver observation дают один State и два
  recoverable observations через page API, а не conflict.
- **ST-06 — observation deduplication:** equal canonical provenance хранится один
  раз; injected digest collision с другими canonical bytes завершается ошибкой и
  сохраняет original observation.
- **ST-07 — identity conflict:** injected row с другой resolved identity под
  существующим StateID считается corruption и не перезаписывается.
- **ST-08a..g — matrix strict reconstruction:** test-only corruption harness
  независимо меняет record version, state kind/JSON/ID, parent copy, resolved
  identity, observation type/identity и timestamps; каждый stable case ID не
  возвращает partial record.
- **ST-09a..d — matrix lineage corruption:** fixtures обходят constraints только
  для создания missing parents, cycles, discontinuous edges и traversal больше
  `runtimev2.MaxTransforms + 1`; каждый case fail closed.
- **ST-10 — transaction rollback/ambiguity:** failure после любого insert
  parent/state/observation откатывает lineage до commit. Ambiguous commit после
  reopen показывает одну полную generation, а identical retry сходится.
- **ST-11 — concurrent writers:** same-lineage writers сходятся идемпотентно;
  разные valid observations сохраняются; conflicting injected identities дают
  одного winner и bounded corruption error.
- **ST-12 — independent persistence oracle:** independently authored golden rows
  восстанавливаются в reviewed public States, StateID, fingerprints, identities,
  provenance и indexed copies; dependency/static check запрещает adapter
  дублировать derivation StateID или fingerprint.
- **ST-13 — timestamps/order observations:** offsets и fractional precision
  нормализуются в fixed-width UTC nanoseconds; invalid/out-of-range values
  отклоняются; observations возвращаются first-persisted по `insertion_seq ASC`
  без раскрытия sequence. Pages применяют row/32-MiB budgets, отклоняют
  invalid/wrong-state cursors, исключают post-high-water inserts из active
  traversal и показывают их в новом traversal. Exact below/equal/above byte
  budget cases используют документированную payload-size formula.

## 5. SQLite adapter resolution cache

- **RC-01 — miss/store/load/restart:** miss становится hit после `Store` и
  остаётся идентичным validated hit после close/reopen engine DB.
- **RC-02 — strict v2 isolation:** representative legacy cache/state rows,
  включая похожие identifiers, никогда не удовлетворяют `Load`.
- **RC-03 — agreement index/envelope:** mismatch indexed cache key и embedded
  record или любого incoming/embedded key constituent возвращает
  `ErrCorruptCache`, а не hit.
- **RC-04 — replacement semantics:** exact-key replacement атомарно обновляет
  resolution/evidence, а ранее сохранённые logical State records не меняются.
- **RC-05 — invalidation classification:** missing, corrupt, incompatible,
  permission/storage и cancellation outcomes соответствуют public resolver
  cache contract; только разрешённые invalidations допускают fallback в Manager.
- **RC-06 — provider validation boundary:** generic load валидирует envelope и
  identity, а Manager по-прежнему вызывает `ValidateResolution` выбранного
  provider до использования evidence.
- **RC-07 — failure atomicity/concurrency:** pre-commit failures сохраняют старую
  запись; ambiguous commit и same-key concurrent writers показывают полную старую
  или новую запись без смешения key, identity или evidence. Reopen и identical
  retry сходятся к требуемой новой записи.

## 6. Materialization associations

- **MT-01 — cardinality/pages:** один State имеет ноль, одну и больше 100 разных
  materializations без изменения identity. Страница не превышает запрошенный
  limit 1..100, не имеет duplicates/gaps, завершает обход empty cursor и
  отклоняет zero/over-limit, malformed, wrong-state и unsupported-version cursor.
  Insertion-sequence high-water mark первой страницы исключает concurrent later
  inserts, включая backdated rows, из текущего traversal; новый traversal их
  видит. Byte budget 32 MiB может сократить page, всегда возвращает хотя бы один
  valid item и продолжает без gaps. Exact below/equal/above cases используют
  документированную payload-size formula.
- **MT-02 — named components:** сохранить ноль, один и несколько named components
  и восстановить все metadata после restart.
- **MT-03 — order independence:** permutations одного component set являются
  одной canonical materialization и возвращают components по имени.
- **MT-04 — idempotence/conflict:** identical duplicate успешен без изменения
  timestamps; любое отличие metadata/component под тем же materialization ID
  завершается ошибкой без replacement.
- **MT-05 — state boundary:** отклонить malformed/unknown Runtime v2 StateID и не
  связывать materialization с legacy-only state.
- **MT-06a..k — matrix bounds/JSON:** `a` materialization ID, `b` identifier
  field, `c` runtime/job ID, `d` locator, `e` component count, `f` signed size,
  `g` UTF-8/control characters, `h` metadata byte size, `i` duplicate key, `j`
  non-object JSON и `k` trailing token покрывают min/max/over-limit. `c`
  различает empty/absent; `f` включает 64-bit boundaries.
- **MT-07 — immutable lifecycle:** UPDATE и DELETE отклоняются отсутствием store
  API и SQLite triggers; тесты не придумывают будущий GC behavior.
- **MT-08a..d — matrix corrupt reads:** test-only bypass создаёт malformed
  metadata, orphan component, version mismatch и invalid component; каждый
  stable case ID не возвращает partial materialization.
- **MT-09 — transaction rollback/ambiguity/concurrency:** component или
  pre-commit failure не оставляет header/orphan subset; ambiguous commit допускает
  одну полную generation и сходится после retry. Identical concurrent inserts
  сходятся, conflicting сохраняют одну полную immutable запись.
- **MT-10 — deterministic order/time:** components сортируются по name; pages
  используют newest-persisted `insertion_seq DESC` без раскрытия sequence;
  одинаковые RFC 3339 instants канонизируются в fixed-width UTC nanoseconds, а
  первый duplicate timestamp сохраняется.

## 7. Classification и legacy separation

- **CL-01 — presence matrix:** raw ID даёт classification `none`, `legacy`, `v2`
  и defensive `both` с точной сохранённой v2 record version.
- **CL-02 — raw legacy identifiers:** non-v2 legacy ID spelling принимается для
  classification, но отклоняется semantic getters Runtime v2.
- **CL-03 — no adoption:** classification не выполняет decode, write, migration
  или cache hit и не создаёт Runtime v2 State.
- **CL-04 — malformed storage:** missing/wrong legacy object shape, unsupported
  v2 record version и canceled classification fail closed без изменения
  namespace.

## 8. Query plans и resource bounds

- **QY-01 — indexed required queries:** `EXPLAIN QUERY PLAN` fixtures доказывают,
  что exact StateID, parent, provenance, cache key и materialization page lookup
  используют документированные индексы, включая insertion-sequence watermark,
  без full scan или `MAX` растущей data table. High water первой page берётся из
  `sqlite_sequence` в той же read transaction. Assertions проверяют referenced
  tables/indexes, а не version-dependent текст SQLite plan.
- **QY-02 — bounded work:** materialization request читает не более одной page и
  одной lookahead row; provenance соблюдает те же row/byte budgets; lineage
  выполняет не более одного indexed parent lookup на State и останавливается на
  semantic и 32-MiB limits; corrupt fan-out не создаёт unbounded returned
  collection.
- **QY-03 — cancellation:** cancellation при ожидании единственного connection,
  traversal lineage, загрузке observations или materialization page возвращает
  `context.Canceled`/`DeadlineExceeded` без partial values или writes.

## 9. Stability, fuzzing и coverage gates

- **GX-01 — default-path regression:** текущее поведение и output
  prepare/run/delete/ls/cache не меняются. Startup может установить/проверить
  dormant v2 schema, но instrumentation доказывает отсутствие v2 semantic read,
  write, cache hit или materialization selection в текущем product flow.
- **GX-02 — restart acceptance:** один file-backed scenario сохраняет resolution,
  recipe и relative lineage, multiple provenance observations и multiple
  materializations, затем выполняет reopen и проверяет полный graph.
- **GX-03 — rc.6 acceptance:** один fixture-driven scenario вместе доказывает
  upgrade, legacy diagnostics, v2 isolation, rollback-safe failure и сохранность
  unrelated data.
- **GX-04 — fuzzing:** value-returning decoders не приводят к panic, не принимают
  trailing data и не возвращают partial values для cache records, semantic
  storage envelopes, materialization metadata/cursors и classification ID.
  Receiver-based decoders дополнительно сохраняют существующее value при failure.
- **GX-05 — race suite:** in-process reads/writes state/provenance/cache/
  materialization проходят `go test -race`; это не заменяет process-lock tests.
- **GX-06 — coverage:** runtime-go resolver и local-engine packages измеряются
  отдельно с per-line reports. Target 100%; minimum 95% для каждого changed
  package. Любой deficiency проходит documented approval loop репозитория до
  дополнительных тестов или удаления dead code.
- **GX-07 — external lock fail-fast:** subprocess удерживает database write lock,
  пока выполняются direct/managed constructors и writes. Instrumented assertions
  доказывают `busy_timeout=0`, отсутствие retry/sleep, быстрый `SQLITE_BUSY` и
  отсутствие mutation; тест намеренно обнаруживает текущее противоречие timeout
  managed startup до приёмки implementation.

CLI или OpenAPI tests не добавляются, потому что #110 не создаёт такую
поверхность. Existing tests проверяются на противоречия только после согласования
этого списка.
