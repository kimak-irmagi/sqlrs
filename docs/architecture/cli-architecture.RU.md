# Архитектура CLI (локально и удаленно)

Этот документ описывает, как CLI `sqlrs` разрешает входы и общается с engine/runner в локальном и shared-деплойментах, включая обработку путей/URL и загрузку источников.

Примечание: ссылки на `POST /runs` в этом документе описывают **runner API**
для shared-деплоймента (целевой дизайн). Текущий локальный engine использует
`POST /v1/prepare-jobs` для prepare/plan и `POST /v1/runs` для `sqlrs run`.

## 1. Цели

- Поддержать одинаковый UX CLI для локальных и удаленных целей.
- Разрешать входы как локальные файлы или публичные URL везде, где ожидается "файл".
- Избегать больших тел запросов в `POST /runs`.
- Обеспечить возобновляемые, контент-адресуемые загрузки для удаленного выполнения.

## 2. Ключевые понятия

- **Target**: endpoint движка (локальный loopback или удаленный gateway).
- **Source**: содержимое проекта (скрипты, changelog, конфиги).
- **Source ref**: локальный путь, публичный URL или серверный `source_id`.
- **Source storage**: хранилище контента на стороне сервиса, ключи по хешам и `source_id`.
- **Effective bearer token**: token, который CLI отправляет на защищенные
  remote API endpoint-ы после применения `SQLRS_TOKEN`, refresh stored OIDC
  session или legacy static bearer profile rules.

## 3. Правила разрешения

Для любого флага CLI, который ожидает файл или директорию, CLI принимает:

- **Локальный путь** (файл или директория).
- **Публичный URL** (HTTP/HTTPS).
- **Серверный source ID** (ранее загруженный bundle).

Матрица решений:

| Target | Input | Действие CLI |
|---|---|---|
| Local engine | Local path | передать путь в engine |
| Local engine | Public URL | передать URL в engine |
| Remote engine | Public URL | передать URL в engine |
| Remote engine | Local path | загрузить в source storage, затем передать `source_id` |

## 4. Потоки

### 4.1 Локальная цель, локальные файлы

```mermaid
sequenceDiagram
  participant CLI as CLI
  participant ENG as Local Engine

  CLI->>ENG: POST /v1/prepare-jobs (prepare_kind, args)
  ENG->>ENG: resolve/validate local file paths
  ENG-->>CLI: prepare events + terminal status
```

### 4.2 Удаленная цель, локальные файлы (сначала загрузка)

```mermaid
sequenceDiagram
  participant CLI as CLI
  participant GW as Gateway
  participant SS as Source Storage
  participant RUN as Runner

  CLI->>GW: POST /sources (create session)
  GW-->>CLI: source_id + chunk size
  loop for each chunk
    CLI->>GW: PUT /sources/{id}/chunks/{n}
  end
  CLI->>GW: POST /sources/{id}/finalize (manifest)
  GW->>SS: store bundle
  CLI->>GW: POST /runs (source_id)
  GW->>RUN: enqueue run
  RUN-->>CLI: stream status/results
```

### 4.3 Удаленная цель, публичный URL

```mermaid
sequenceDiagram
  participant CLI as CLI
  participant GW as Gateway
  participant RUN as Runner

  CLI->>GW: POST /runs (source_url=https://...)
  GW->>RUN: enqueue run
  RUN->>RUN: fetch URL into source cache
  RUN-->>CLI: stream status/results
```

### 4.4 Список (sqlrs ls)

```mermaid
sequenceDiagram
  participant CLI as CLI
  participant FS as "State Dir"
  participant ENG as Engine

  alt Local target
    CLI->>FS: read engine.json (endpoint + auth token)
    alt missing or stale
      CLI->>ENG: spawn local engine
      ENG-->>FS: write engine.json
      CLI->>FS: read engine.json
    end
  end

  CLI->>ENG: GET /v1/names (Authorization, Accept)
  ENG-->>CLI: JSON array или NDJSON
  CLI->>ENG: GET /v1/instances (Authorization, Accept)
  ENG-->>CLI: JSON array или NDJSON
  opt states requested
    CLI->>ENG: GET /v1/states (Authorization, Accept)
    ENG-->>CLI: JSON array или NDJSON
  end
  opt jobs requested
    CLI->>ENG: GET /v1/prepare-jobs (Authorization, Accept)
    ENG-->>CLI: JSON array или NDJSON
  end
  opt tasks requested
    CLI->>ENG: GET /v1/tasks?job=... (Authorization, Accept)
    ENG-->>CLI: JSON array или NDJSON
  end
  CLI->>CLI: render tables or JSON
```

Примечания:

- Для remote target используются те же list endpoint-ы. Перед защищенным
  remote request CLI разрешает effective bearer token: сначала `SQLRS_TOKEN`
  override, затем refreshed stored OIDC session для `auth.mode: oidcSession`,
  затем legacy static bearer profile configuration.
- По умолчанию CLI запрашивает names и instances; states, jobs и tasks запрашиваются явно.
- Вывод tasks можно отфильтровать по job id (`--job`).

### 4.5 Удаление (sqlrs rm)

```mermaid
sequenceDiagram
  participant CLI as CLI
  participant ENG as Engine

  CLI->>ENG: GET /v1/instances?id_prefix=...
  ENG-->>CLI: instance list
  CLI->>ENG: GET /v1/states?id_prefix=...
  ENG-->>CLI: state list
  CLI->>ENG: GET /v1/prepare-jobs?job=...
  ENG-->>CLI: job list
  alt ambiguous or not found
    CLI->>CLI: error or noop
  else job resolved
    CLI->>ENG: DELETE /v1/prepare-jobs/{id}?force&dry_run
    ENG-->>CLI: DeleteResult
  else instance resolved
    CLI->>ENG: DELETE /v1/instances/{id}?force&dry_run
    ENG-->>CLI: DeleteResult
  else state resolved
    CLI->>ENG: DELETE /v1/states/{id}?recurse&force&dry_run
    ENG-->>CLI: DeleteResult
  end
  CLI->>CLI: render deletion tree
```

### 4.6 Run (sqlrs run)

```mermaid
sequenceDiagram
  participant CLI as CLI
  participant ENG as Engine
  participant RT as Runtime

  alt Composite prepare present
    CLI->>ENG: start prepare job
    ENG-->>CLI: job_id + events_url + status_url
    CLI->>ENG: watch events (events_url)
    CLI->>ENG: GET /v1/prepare-jobs/{jobId} (on status events)
    ENG-->>CLI: terminal status + DSN
  else Explicit instance
    alt Значение в форме префикса (8+ hex-символов)
      CLI->>ENG: GET /v1/instances/{idOrName}
      ENG-->>CLI: точный instance или 404
      opt Точное совпадение отсутствует
        CLI->>ENG: GET /v1/instances?id_prefix={prefix}
        ENG-->>CLI: ноль, один или несколько instances
      end
    end
    CLI->>CLI: требует один instance и сохраняет его полный id
  end

  CLI->>ENG: POST /v1/runs (полный instance id, kind, command or default, args)
  opt Missing instance container
    ENG-->>CLI: event "run: container missing - recreating"
    ENG-->>CLI: event "run: restoring runtime"
    ENG->>RT: start container with runtime_dir mount
    RT-->>ENG: container ready
    ENG-->>CLI: event "run: container started"
  end
  ENG->>RT: exec in instance container (inject DSN)
  RT-->>ENG: stdout/stderr/exit
  ENG-->>CLI: stream output + exit

  opt Composite invocation
    CLI->>ENG: delete instance
  end
```

Примечания:

- `run:psql` передает DSN как позиционный connection string; `run:pgbench`
  использует `-h/-p/-U/-d`.
- Команды выполняются внутри контейнера инстанса (тот же runtime, что и
  `prepare:psql`).
- Для явно заданного шестнадцатеричного `--instance` длиной 8+ символов CLI
  сохраняет точное совпадение по id/name, а иначе разрешает значение через
  существующий prefix filter списка instances. Ноль совпадений дает not-found,
  несколько — ambiguity; `POST /v1/runs` получает канонический полный id.
- Если контейнер инстанса отсутствует и `runtime_dir` существует, engine
  пересоздает контейнер и пишет run-события до выполнения команды.
- Если `--instance` задан вместе с предыдущим `prepare`, CLI завершает работу с
  явной ошибкой неоднозначности.
- Мониторинг prepare работает через события; CLI проверяет терминальный статус
  через `GET /v1/prepare-jobs/{jobId}` при получении событий статуса.

## 5. Детали загрузки (remote)

- CLI режет файлы на чанки, считает хеши и загружает только отсутствующие чанки.
- Manifest сопоставляет пути файлов и хеши чанков; это дает rsync-style delta.
- `source_id` контент-адресуемый и может переиспользоваться между запусками.
- Большие загрузки возобновляемые; сбойные чанки можно повторить без рестарта.

## 6. Поддержка Liquibase

- CLI поддерживает `plan:lb` и `prepare:lb`, передавая Liquibase-аргументы в `POST /v1/prepare-jobs`.
- CLI не строит локальный fallback step-план для Liquibase; планирование/выполнение принадлежат engine.
- В Windows + WSL local mode CLI переписывает Liquibase path-аргументы/workdir для совместимости с host execution.
