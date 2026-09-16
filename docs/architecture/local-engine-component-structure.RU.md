# Компонентная структура локального engine

Уточнение от 2026-09-14: согласованный [порядок работы с идентичностью](managed-database-identity.RU.md)
заменяет предположения о фиксированном SQL-имени. [Проект компонентов и схемы](managed-database-identity-internals.RU.md)
и план тестов согласованы; описание реализации ниже относится к текущему поколению,
а не разрешает угадывать или восстанавливать фиксированную административную роль.

Документ описывает текущую внутреннюю структуру локального `sqlrs-engine`.

## 1. Цели

- Разделить HTTP/API слой и слои исполнения/персистентности.
- Хранить метаданные state и очереди в SQLite.
- Явно зафиксировать ответственность за snapshot-на-уровне файловой системы.

## 2. Пакеты и ответственность

- `cmd/sqlrs-engine`
  - Парсит флаги и связывает зависимости.
  - Открывает SQLite state DB, config manager, runtime, statefs, prepare/run/delete managers.
  - Определяет режим runtime из engine config: `container.runtime` (`auto|docker|podman`).
  - Поддерживает operational override через `SQLRS_CONTAINER_RUNTIME`.
  - В режиме `auto` пробует `docker`, затем `podman`.
  - Для Podman может выставлять `CONTAINER_HOST` из default podman connection, если переменная не задана.
  - Запускает HTTP сервер и пишет/удаляет `engine.json`.
- `internal/httpapi`
  - HTTP роутинг для `/v1/*`.
  - JSON и NDJSON ответы (prepare events, run stream).
  - Делегирует в `prepare`, `run`, `deletion`, `registry`, `config`.
- `internal/config`
  - Управляет дефолтами и persisted overrides (`config.json`).
  - Отдает схему и поддерживает path-based get/set/remove.
- `internal/prepare`
  - `prepare.PrepareService` — публичный фасад для HTTP handlers.
  - Внутреннее разделение:
    - `jobCoordinator`: жизненный цикл job, planning, переходы queue/event.
    - `taskExecutor`: runtime/task execution и создание instance.
    - `snapshotOrchestrator`: инициализация base state, invalidation dirty cache, snapshot guard-логика.
  - Полная оркестрация prepare остаётся в этом пакете: валидация, план, выполнение, snapshot, создание state/instance.
  - Поддерживает prepare kind `psql` и `lb`.
  - Обрабатывает `plan_only`, стрим событий и жизненный цикл job.
- `internal/prepare/queue`
  - Персистентная очередь jobs/tasks/events в SQLite.
  - Recovery для queued/running jobs и retention trimming по сигнатуре.
- `internal/run`
  - Выполняет `run:psql` и `run:pgbench` по существующим instances.
  - Проверяет managed-связи runtime/доступа; при потере контейнера закрывает доступ без принятия каталога `runtime_dir`.
- `internal/deletion`
  - Строит и применяет delete tree для instances/states.
  - Учитывает `recurse`, `force`, `dry_run`.
- `internal/registry`
  - Разрешение name/id и list/get операции для names/instances/states.
- `internal/statefs`
  - Path layout state store + интерфейс StateFS.
  - Оборачивает операции snapshot backend-а (clone/snapshot/remove/validate).
- `internal/snapshot`
  - Реализации и выбор backend-а (`auto`, `overlay`, `btrfs`, `copy`).
- `internal/runtime`
  - Адаптер container runtime (Docker/Podman CLI; init base, start/stop, exec, run container).
- `internal/dbms`
  - DBMS-специфичные snapshot hooks (Postgres stop/resume через `pg_ctl`).
- `internal/conntrack`
  - Абстракция трекинга подключений (в текущей локальной wiring используется `conntrack.Noop`).
- `internal/auth`
  - Bearer-token проверка для защищенных endpoint-ов.
- `internal/id`
  - Хелперы валидации формата ID.
- `internal/store`
  - Интерфейсы хранения и фильтры для names/instances/states.
- `internal/store/sqlite`
  - Реализация `internal/store` на SQLite.
- `internal/stream`
  - Хелперы list/NDJSON стриминга для HTTP-ответов.

## 3. Ключевые типы и интерфейсы

- `prepare.PrepareService`
  - Публичный prepare фасад для submit/status/events/delete.
- `prepare.jobCoordinator` (internal)
  - Координирует жизненный цикл job, planning, переходы task и queue writes.
- `prepare.taskExecutor` (internal)
  - Выполняет задачи и runtime-операции.
- `prepare.snapshotOrchestrator` (internal)
  - Владеет snapshot/cache hygiene и guard-логикой base-state init.
- `prepare.Request`, `prepare.Status`, `prepare.PlanTask`
  - API payload prepare и модель задач плана.
- `queue.Store`
  - Персистентный API jobs/tasks/events для `prepare.PrepareService`.
- `run.Manager`, `run.Request`, `run.Event`
  - Менеджер исполнения run и stream runtime-событий.
- `deletion.Manager`, `deletion.DeleteResult`
  - Планировщик/исполнитель удаления и модель ответа-дерева.
- `config.Store` (`config.Manager`)
  - Runtime config API для `/v1/config*`.
- `store.Store`
  - Интерфейс персистентного хранения names/instances/states.
- `statefs.StateFS`
  - Файловая абстракция для clone/snapshot/remove + деривации путей.

## 4. Владение данными

- Metadata DB: `<state-store-root>/state.db` (names/instances/states + таблицы prepare queue).
- Snapshot store: `<state-store-root>/engines/<engine>/<version>/base|states/<state_id>`.
- Runtime-директории job: `<state-store-root>/jobs/<job_id>/runtime`.
- Конфиг engine: `<state-store-root>/config.json`.
- Discovery-файл для CLI: `<state-dir>/sqlrs/engine.json` (путь задается через `--write-engine-json`).

## 5. Диаграмма зависимостей

```mermaid
flowchart TD
  CMD["cmd/sqlrs-engine"]
  HTTP["internal/httpapi"]
  PREP["internal/prepare (PrepareService facade)"]
  PREP_COORD["prepare jobCoordinator (internal)"]
  PREP_EXEC["prepare taskExecutor (internal)"]
  PREP_SNAP["prepare snapshotOrchestrator (internal)"]
  QUEUE["internal/prepare/queue"]
  RUN["internal/run"]
  DEL["internal/deletion"]
  REG["internal/registry"]
  AUTH["internal/auth"]
  CFG["internal/config"]
  STORE["internal/store (interfaces)"]
  SQLITE["internal/store/sqlite"]
  STATEFS["internal/statefs"]
  SNAPSHOT["internal/snapshot"]
  RT["internal/runtime"]
  DBMS["internal/dbms"]
  CONN["internal/conntrack"]
  ID["internal/id"]
  STREAM["internal/stream"]

  CMD --> HTTP
  CMD --> CFG
  CMD --> SQLITE
  CMD --> STATEFS
  CMD --> RT

  HTTP --> AUTH
  HTTP --> PREP
  HTTP --> RUN
  HTTP --> DEL
  HTTP --> REG
  HTTP --> CFG
  HTTP --> STREAM

  PREP --> PREP_COORD
  PREP --> PREP_EXEC
  PREP --> PREP_SNAP
  PREP_COORD --> QUEUE
  PREP_COORD --> CFG
  PREP_COORD --> STORE
  PREP_COORD --> PREP_EXEC
  PREP_EXEC --> RT
  PREP_EXEC --> DBMS
  PREP_EXEC --> STATEFS
  PREP_EXEC --> STORE
  PREP_EXEC --> PREP_SNAP
  PREP_SNAP --> STATEFS
  PREP_SNAP --> STORE

  RUN --> RT
  RUN --> REG
  DEL --> STORE
  DEL --> CONN
  DEL --> RT
  DEL --> STATEFS
  REG --> ID
  REG --> STORE

  STATEFS --> SNAPSHOT
  SQLITE --> STORE
```

## 6. Требования к helper-функциям и зона ответственности

Технические helper-функции являются частью контрактов компонентов, когда они
влияют на корректность, детерминизм, безопасность или наблюдаемое API-поведение.

- helper-функции `internal/prepare`
  - Отвечают за lock/marker-семантику build base/state, dirty-state detection,
    cache invalidation, normalisation/path mapping аргументов Liquibase и
    семантику host-exec режима Liquibase (`native` vs `windows-bat`).
- helper-функции `internal/runtime`
  - Отвечают за классификацию docker-ошибок, startup diagnostics, идемпотентное
    обновление host auth в `pg_hba.conf` и гарантии парсинга host-port.
- helper-функции `internal/run`
  - Отвечают за guard-проверки конфликтующих connection args и триггеры recovery
    при отсутствии контейнера.
- helper-функции `internal/deletion`
  - Отвечают за cleanup fallback-правила (`StateFS` first, filesystem fallback)
    и нефатальную обработку docker-unavailable при остановке runtime в удалении.
- helper-функции `internal/httpapi`
  - Отвечают за поведение цикла NDJSON prepare-event stream (упорядоченная выдача,
    ожидание новых событий, завершение при terminal status).

Эти helper-контракты должны быть явно задокументированы и покрыты тестами по
ожидаемому поведению, а не только по текущей реализации.

