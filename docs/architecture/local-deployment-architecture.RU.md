# Архитектура локального деплоймента (sqlrs)

Этот документ описывает, как `sqlrs` разворачивается и выполняется на **рабочей станции разработчика** в MVP.

Фокус:

- тонкий CLI
- эфемерный процесс engine
- взаимодействие с container runtime и psql

Этот документ намеренно не повторяет детали запуска скриптов; интеграция Liquibase планируется
и описана в [`liquibase-integration.RU.md`](liquibase-integration.RU.md).
Внутренности engine описаны в [`engine-internals.RU.md`](engine-internals.RU.md).
Team/Cloud вариант описан в [`shared-deployment-architecture.RU.md`](shared-deployment-architecture.RU.md).

---

## 1. Цели

- Быстрый запуск CLI
- Минимальный постоянный след на хосте
- Кросс-платформенность (Linux, macOS, Windows через WSL2)
- Четкое разделение CLI и тяжелой runtime-логики
- Простая эволюция в daemon или team/cloud deployment

---

## 2. Высокоуровневая топология (MVP)

```mermaid
flowchart LR
  U[User]
  CLI["sqlrs CLI"]
  ENG["sqlrs engine process"]
  DOCKER["Docker Engine"]
  DB["DB Container"]
  STATE["State dir (engine.json)"]
  STORE["state store"]

  U --> CLI
  CLI -->|spawn / connect| ENG
  ENG --> DOCKER
  DOCKER --> DB
  ENG --> STORE
  CLI -. discovery .-> STATE
```

---

## 3. sqlrs CLI

### 3.1 Ответственности

- Парсить команды и флаги пользователя
- Работать с локальной файловой системой (конфиг проекта, пути)
- Находить или запускать локальный процесс engine
- Разрешать native engine из explicit config, environment, CLI release bundle
  или `PATH`; на Windows отдельно разрешать и устанавливать bundled Linux
  companion для WSL2/btrfs
- Общаться с engine по HTTP через loopback или Unix socket
- Выполнять `run` команды локально против подготовленного экземпляра
- Отклонять remote-only команды управления пользователями и организациями до
  discovery или autostart локального engine
- Завершаться сразу после выполнения команды
- Опционально: pre-flight проверки (container runtime доступен, state store доступен для записи), вывод endpoint/version для диагностики

CLI намеренно **тонкий** и без состояния.

### 3.2 Не-ответственности

- Нет логики оркестрации container runtime
- Нет логики snapshotting
- Нет прямого выполнения скриптов
- Нет управления user profiles, external identities, organizations или
  memberships

---

## 4. Процесс Engine (эфемерный)

### 4.1 Характеристики

- Запускается по требованию CLI
- Работает как дочерний процесс (не системный daemon)
- Слушает локальный endpoint (loopback или socket)
- Управляет runtime-состоянием пока активен

### 4.2 Ответственности

- Оркестрация контейнеров через container runtime
- Snapshotting и управление состояниями
- Cache rewind и eviction
- Выполнение скриптов через `psql` внутри контейнера
- Connection / proxy слой (если нужен)
- Трекинг подключений для TTL и безопасного удаления
- IPC/API для CLI и будущих IDE-интеграций
- Планирование/исполнение prepare; `run` команды не выполняет
- Создание эфемерных экземпляров для prepare

### 4.3 Жизненный цикл

- Спавнится по требованию
- Может жить короткий TTL после последнего запроса
- Автоматически завершается в простое
- Пишет endpoint/lock и auth token в `<StateDir>/engine.json`, чтобы последующие CLI могли переиспользовать процесс

Это избегает постоянных background-сервисов в MVP.

---

## 5. IPC: CLI <-> Engine

- **Транспорт/протокол**: REST по HTTP; только loopback. Unix domain socket на Linux/macOS; TCP loopback на Windows хосте с WSL forwarding. В локальном режиме TLS нет.
- **Discovery endpoint**:
  - CLI проверяет env var `TAIDON_ENGINE_ADDR`.
  - Иначе читает `<StateDir>/engine.json` (endpoint, socket path / TCP port, PID, instanceId, auth token).
  - Если не найден или устарел, CLI запускает новый engine; engine пишет `engine.json` при готовности.
- **Security**: запрет bind на не-loopback; auth token обязателен для non-health endpoint-ов; опора на права файлов (UDS) или loopback firewall; engine отказывает в соединениях с нелокальных адресов.
- **Versioning**: CLI отправляет свою версию; engine отклоняет несовместимую major; CLI может предложить апгрейд.

Ключевые endpoint-ы engine (логически):

- `POST /v1/prepare-jobs` - старт prepare job (plan/execute steps, snapshot states, создание экземпляра); `plan_only` считает только задачи
- `GET /v1/prepare-jobs/{jobId}` - статус job
- `GET /v1/prepare-jobs/{jobId}/events` - стрим событий job (NDJSON)
- `GET /v1/config` / `PATCH /v1/config` - чтение/запись server config
- `GET /v1/config/schema` - схема конфигурации
- список names/instances/states (JSON array или NDJSON через `Accept`)
- `GET /v1/names/{name}` - чтение name binding
- `GET /v1/instances/{instanceId}` - чтение экземпляра (если найдено по имени, engine отвечает 307 redirect на канонический URL по id)
- `DELETE /v1/instances/{instanceId}` - удаление экземпляра (идемпотентно; поддерживает dry-run)
- `GET /v1/states/{stateId}` - чтение state
- `DELETE /v1/states/{stateId}` - удаление state (идемпотентно; поддерживает recurse/force/dry-run)
- `POST /snapshots` - ручной snapshot
- `GET /cache/{key}` - cache lookup
- `POST /engine/shutdown` - опциональная мягкая остановка

Локальный engine не экспортирует `/v1/users*` или `/v1/organizations*`. Эти
ресурсы принадлежат shared/team/cloud деплойментам за Gateway.

### 5.1 Долгие операции: async jobs, sync CLI

- Engine выполняет prepare как асинхронные job; `POST /v1/prepare-jobs` отвечает `201 Created` с job id.
- CLI поддерживает:
  - `prepare --watch` (по умолчанию): наблюдение за status/events до терминального статуса.
  - `prepare --no-watch`: отправить job и вернуть `job_id`, `status_url`, `events_url`.
  - `watch <job_id>`: подключиться к уже запущенной prepare job.
- В интерактивном watch-режиме `Ctrl+C` открывает control prompt:
  - `stop` (с подтверждением) запрашивает отмену,
  - `detach` отключает наблюдение, оставляя job выполняться.
- Для composite-вызова `prepare ... run ...` действие `detach` пропускает
  последующую фазу `run` в том же CLI-процессе.

---

## 6. Взаимодействие с psql

Engine выполняет `psql` внутри DB-контейнера через runtime `exec`.
При наличии файловых входов он монтирует корень скриптов read-only и
переписывает `-f` аргументы на путь внутри контейнера.

Интеграция Liquibase планируется; детали провайдера описаны в
[`liquibase-integration.RU.md`](liquibase-integration.RU.md).

---

## 6. Взаимодействие с Container Runtime

- Локальный engine поддерживает Docker и Podman CLI.
- Режим runtime задается через engine config `container.runtime` со значениями `auto|docker|podman`.
- `SQLRS_CONTAINER_RUNTIME` используется как operational override для CI/debug-сценариев.
- В режиме `auto` порядок проб: `docker`, затем `podman`.
- Для Podman на macOS при пустом `CONTAINER_HOST` engine читает default Podman connection и экспортирует её для дочерних runtime-команд.
- Engine управляет DB-контейнерами и выполняет `psql` через runtime `exec`
- Все persistent data directories монтируются из host-managed хранилища
- Engine проверяет доступность runtime на старте; CLI выводит понятные ошибки, если runtime недоступен

На Windows (когда выбран btrfs):

- Docker работает внутри WSL2
- State store живет внутри Linux файловой системы (btrfs loopback)
- `sqlrs init local --snapshot btrfs` устанавливает systemd mount unit внутри выбранного дистрибутива,
  чтобы btrfs-том был виден Docker и всем дочерним процессам
- Engine проверяет активность маунта перед работой со state store

---

## 7. Особенности Windows / WSL2

- Когда выбран btrfs, engine и StateFS работают внутри WSL2
- Когда btrfs не выбран, engine может работать на Windows host (copy backend)
- CLI может работать на Windows host или внутри WSL2
- Коммуникация через localhost forwarding, когда engine работает в WSL2
- Engine пишет `engine.json` в Windows state dir и получает путь через `/mnt/...` (WSL path translation)
- Engine проверяет systemd-маунт `SQLRS_STATE_STORE` при старте (только WSL + btrfs)
- StateFS backend может откатываться на copy-based стратегию
- Windows release содержит `sqlrs-engine.exe` для native copy mode и Linux
  companion в `libexec/linux-<arch>/` для WSL2/btrfs. Discovery и provisioning
  определены в
  [`local-engine-binary-discovery-flow.RU.md`](local-engine-binary-discovery-flow.RU.md).

---

## 8. Путь эволюции

### Phase 1 (MVP)

- Эфемерный engine процесс
- Тонкий CLI
- Только локальный деплоймент

### Phase 2

- Опциональный постоянный локальный daemon (`sqlrsd`)
- Переиспользование warm instance
- IDE интеграции

### Phase 3

- Team-shared engine
- Remote cache
- Cloud-hosted control plane

---

## 9. Не-цели

- System-wide background service по умолчанию
- OS-specific installers или service managers
- Глубокий embedding Liquibase

---

## 10. Открытые вопросы

- Unix socket vs TCP loopback как дефолт IPC?
- Дефолтный TTL engine после последней команды?
- Нужно ли CLI авто-обновлять engine binary?
