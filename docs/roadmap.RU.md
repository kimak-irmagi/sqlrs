# Roadmap Taidon

Этот roadmap приоритизирует сценарии, use cases и компоненты, чтобы дать раннюю
продуктовую ценность при сохранении четкого пути к team (on-prem) и
cloud/education предложениям.

---

## Цели и не-цели

### Цели

- Дать быстрый, воспроизводимый database instance для разработчиков (local-first)
- Обеспечить единое инвариантное ядро (Engine + API) для всех профилей деплоймента
- Дать CI/CD интеграцию для team adoption
- Сохранить чистый путь апгрейда к public cloud sharing и образовательным сценариям

### Не-цели (на старте)

- Полный multi-tenant биллинг и платежи
- Поддержка множества DB engines одновременно
- Полный browser-based IDE hosting (VS Code-in-browser)

---

## Обзор дорожной карты

```mermaid
gantt
    title Дорожная карта Taidon (высокий уровень)
    dateFormat  YYYY-MM-DD
    axisFormat  %b %Y

    section Основы
    Каркас Core API + Engine                  :done, a1, 2026-01-01, 30d
    Локальный runtime + lifecycle экземпляра   :done, a2, after a1, 45d
    Бэкенд snapshot ФС (overlayfs)            :done, a2fs, after a2, 20d
    Бэкенд snapshot ФС (Btrfs)                :done, a2b, after a2fs, 30d
    Бэкенд snapshot ФС (ZFS)                  :a2z, after a2b, 30d
    Адаптер Liquibase (применение миграций)   :done, a3, after a2b, 30d

    section Продуктовый MVP (локальный)
    CLI UX + детерминированные запуски        :done, b2, after a2, 45d
    Кэш состояний v1 (reuse states)           :done, b3, after a3, 45d
    Ограничения кэша (размер + eviction)      :done, b3g, after b3, 30d
    Git-aware CLI (ref/diff/provenance)       :done, b4, after b3, 30d

    section Team (On-Prem)
    Базовый shared control plane + policy      :c1, after b2, 45d
    Team connectivity и workflow adoption      :c0, after c1, 45d
    Базовый team deployment gateway            :c6, after c1, 30d
    Базовая работа с artefacts и audit         :c2, after c1, 45d
    Шаблоны интеграции CI/CD                  :active, c3, after c1, 30d
    Client contract slice users/orgs           :done, c4a, after c1, 20d
    Google OIDC CLI auth session slice         :done, c4b, after c4a, 20d
    Client/API slice remote source input sync  :done, c4c, after c4b, 15d
    Базовый auth + tenant access               :c4, after c1, 45d
    Масштабирование shared capacity            :c5, after c2, 30d

    section Облако (Sharing)
    Шаринг артефактов (immutable runs)        :d1, after c2, 45d
    Публичные read-only страницы              :d2, after d1, 30d
    Anti-abuse ограничения (rate/quota)       :d3, after d1, 30d

    section Исследования / Опционально
    Git-backed интеграция workspace            :o1, after d2, 45d
    PR automation и warmup workflows           :o2, after o1, 45d

    section Образование
    Модель курс/задание/сдача                 :e1, after d2, 45d
    Раннер автопроверки                       :e2, after e1, 45d
```

> Даты — placeholder-ы для визуализации порядка. Roadmap построен вокруг milestones.

---

## Статус (на 2026-07-06)

- **Сделано**: локальная поверхность API (health, config, names, instances, runs,
  states, prepare jobs, tasks), локальный runtime и lifecycle, end-to-end pipeline
  init/prepare/run, хранение job/task и события, абстракция StateFS, базовая часть
  state cache и ретеншн, локальная CLI-поверхность (`sqlrs init`, `sqlrs config`,
  `sqlrs ls`, `sqlrs status`, `sqlrs plan:psql`, `sqlrs plan:lb`, `sqlrs prepare:psql`,
  `sqlrs prepare:lb`, `sqlrs run:psql`, `sqlrs run:pgbench`, `sqlrs rm`), WSL
  WSL init flow (включая установку nsenter), логирование instance-delete.
- **Отложено (names follow-up)**: более глубокие workflows вокруг runtime names
  и любая alias-to-name связка намеренно вынесены за пределы M2 local DX; текущие
  list/get names уже входят в local engine baseline, но более богатая оркестрация
  names отложена.
- **Сделано (ФС)**: заглушка snapshot на overlayfs (copy) и бэкенд снимков на Btrfs.
- **Сделано (PR #37-#41, hardening)**: добавлены release happy-path e2e сценарии
  для Chinook/Sakila с расширением матрицы (включая Btrfs), поведение
  `init --btrfs` выровнено между Linux и Windows, Windows WSL/docker probe
  встроен в release-проверки, усилена детерминированность output/workspace.
- **Сделано (release identity)**: RC-бандлы содержат полный исходный tag в CLI,
  native engine и встроенном WSL engine, а нерелизные сборки остаются `dev`.
- **Сделано (MVP command surface)**: локальный MVP-набор команд стабилен вокруг
  `init/config/ls/status/plan/prepare/run/rm`; legacy-нейминг команд в
  документации считается устаревшим.
- **Сделано (bounded cache core)**: локальный engine уже поддерживает
  `cache.capacity.*` конфиг, strict enforcement перед/после snapshot-фаз,
  детерминированный eviction неиспользуемых leaf-states и structured errors
  при нехватке места.
- **Сделано (bounded cache hardening)**: операторская диагностика для local
  профиля уже входит в публичную CLI-поверхность через `sqlrs status`,
  `sqlrs status --cache` и `sqlrs ls --states --cache-details`; persisted
  `size_bytes` metadata и отдельный release cache-pressure сценарий тоже уже
  на месте.
- **Сделано (M2 alias execution baseline)**: repo-tracked prepare и run aliases
  теперь обслуживают `sqlrs plan <ref>`, `sqlrs prepare <ref>` и standalone
  `sqlrs run <ref> --instance ...` через `*.prep.s9s.yaml` и
  `*.run.s9s.yaml` files, с exact-file escape через trailing `.`, cwd-relative
  alias-ref resolution, alias-file-relative file-bearing paths и смешанной raw
  и alias-композицией `prepare ... run` с guardrails вокруг `--instance`.
- **Сделано (M2 alias inspection baseline)**: CLI теперь включает
  `sqlrs alias ls` и `sqlrs alias check [<ref>]` с ограниченной scan scope,
  дефолтом `cwd + recursive`, явными `--from` / `--depth`, статической
  валидацией как для scan mode, так и для single-alias mode, и reporting-ом
  invalid entries для битых alias-файлов.
- **Сделано (M2 alias authoring + advisory discovery baseline)**: CLI теперь
  также включает `sqlrs alias create` и `sqlrs discover --aliases`, включая
  copy-paste команды создания alias, ограниченный обход workspace с progress
  reporting, human/JSON rendering и общие alias/discover heuristics для
  текущего среза репозитория.
- **Сделано (M2 generic discover analyzers)**: `sqlrs discover` теперь работает
  как registry-driven multi-analyzer advisory surface с additive-выбором
  `--aliases` / `--gitignore` / `--vscode` / `--prepare-shaping`, стабильным
  default-запуском по всем shipped analyzers, analyzer-grouped human/JSON
  output, failure isolation и shell-aware follow-up командами для исправлений в
  `.gitignore` и `.vscode`.
- **Сделано (CLI maintainability PR4 stage pipeline)**: direct и alias-backed
  flow для `plan` / `prepare` в `internal/app` теперь разделяют один
  package-local stage pipeline для image resolution, kind-specific binding,
  terminal invocation и cleanup handling, что убирает дублирование
  orchestration без изменения публичного CLI-контракта.
- **Сделано (CLI maintainability PR5 split WSL init)**: WSL-backed local init
  в `internal/app` теперь разрезан на helper-ы bootstrap, storage и
  mount-finalization, shared WSL path conversion вынесен из `app.go`, а
  прежнее поведение `init local`/`resolveWSLSettings`/cleanup spinner
  удерживается под targeted regression tests.
- **Сделано (M2 shared inputset + diff path baseline)**: CLI теперь включает
  `sqlrs diff --from-path/--to-path` для `prepare:psql` и `prepare:lb`, а
  `prepare`, `plan`, `run`, `diff` и `alias check` теперь разделяют единый
  kind-specific слой `internal/inputset` для file-bearing semantics и локального
  closure collection по `psql`, `liquibase` и `pgbench`.
- **Сделано (M2 diff ref-mode hardening)**: разрешение worktree в ref-mode
  теперь сохраняет caller cwd через symlinked paths, а `psql` include semantics
  остаются как в `psql` (`\i` / `\include` от cwd команды, `\ir` /
  `\include_relative` от директории включающего файла). CLI также сохраняет
  `worktree` как режим ref по умолчанию, пока явные чтения `blob` не сравняются
  безопасно с этой файловой семантикой.
- **Сделано (M2 bounded local `--ref` baseline)**: `sqlrs plan` и
  `sqlrs prepare` теперь поддерживают bounded local `--ref` чтение и для raw,
  и для alias-backed flow, с projected-cwd semantics, выровненной с
  `sqlrs diff`, общим владением repo/ref/worktree handling в `internal/refctx`
  и общим filesystem-aware alias-target resolution в `internal/alias`. В этом
  первом публичном slice `worktree` остается ref-mode по умолчанию, а
  `prepare --ref` пока остается только в watch mode.
- **Сделано (baseline M2 provenance + cache explain)**: single-stage local
  `plan` / `prepare` теперь поддерживают JSON side artifacts через
  `--provenance-path` для raw, alias-backed и bounded `--ref` flow; CLI также
  включает read-only `sqlrs cache explain prepare ...` с теми же
  raw/alias/ref binding semantics, отдельный local engine endpoint
  `POST /v1/cache/explain/prepare` и regression coverage для hit/miss,
  rendering и binding parity.
- **Сделано (baseline M2 standalone `run --ref`)**: standalone local
  `run`, `run:psql` и `run:pgbench` теперь поддерживают bounded `--ref`
  чтение и для raw, и для alias-backed flow, переиспользуя существующие
  projected-cwd ref context и inputset semantics; на CLI boundary также
  режется ref-backed `prepare ... run`, пока не будет принят отдельный
  composite design.
- **Сделано (release alias/workspace coverage)**: release/e2e сценарии теперь
  гоняют repo-tracked prepare aliases для примеров Chinook, Sakila и
  Liquibase/JHipster, удерживая alias/workspace conventions под валидацией.
- **Сделано (CLI alias coverage hardening)**: регрессионные тесты `internal/alias`
  закрыли оставшиеся ветки create, resolve и scan, доведя coverage CLI alias
  package выше минимального порога в `go test --cover ./...`.
- **Сделано (remote users/orgs client/API contract slice)**: CLI теперь имеет
  remote-only flow для `sqlrs user me`, `sqlrs user register`,
  `sqlrs user create`, `sqlrs org create`, `sqlrs org ls` и `sqlrs org get`,
  опирающийся на задокументированный OpenAPI contract для users/organizations.
  Slice сознательно не включает поддержку local engine и server-side
  persistence/provider implementation.
- **Сделано (Google OIDC CLI auth session slice)**: CLI теперь имеет
  `sqlrs auth login google`, `sqlrs auth status` и `sqlrs auth logout` на базе
  Google Authorization Code Flow with PKCE, loopback redirect handling,
  хранения refresh token в OS credential store, refresh cached ID token и
  приоритета debug override через `SQLRS_TOKEN`, а также временную отправку
  Google Desktop `client_secret` в Google token endpoint, если он задан в
  профиле. Gateway по-прежнему получает только короткоживущие Google ID token;
  принятие CLI OAuth client audience остается gateway config/follow-up задачей
  там, где это нужно.
- **Сделано (remote source input sync client/API slice)**: OpenAPI contract и
  CLI теперь поддерживают remote source-input synchronization для remote
  `prepare`, `plan` и `cache explain prepare` flow. Клиент добавляет bounded
  `source_manifest`, обрабатывает recoverable `source_inputs_missing`,
  загружает запрошенные blob-ы через `/v1/source-blobs/sha256/{digest}`,
  испускает typed sync events, которые отображаются verbose lines в stderr или
  delayed interactive spinner (normal non-TTY остаётся silent), учитывает
  per-profile `sourceSync` policy и использует выбранный ref filesystem для
  ref-backed prepare stages.
  Поведение local profile не меняется. Production handshake теперь также
  передает абсолютные client coordinates `root_path`/`work_dir`, сохраняет
  абсолютные public prepare arguments, использует отдельный 15-минутный
  source-transfer timeout и работает с gateway-owned admission projection без
  изменения CLI syntax. Remote path binding изолирован от local WSL config,
  поэтому Windows host paths не преобразуются ошибочно в `/mnt/...`.
  Upload progress сообщает фактически переданные bytes на bounded checkpoints,
  stable unique-upload ordinals и final transferred-byte total и идентифицирует
  files только workspace-relative path и shortened digest.
- **В работе (базовый CI-template слой)**: GitHub Actions release/e2e пайплайны
  уже активны; более широкие team-шаблоны (например, GitLab и on-prem варианты)
  ещё впереди.
- **Следующий публичный local-фокус (ожидает выбора)**: выбрать первый
  post-`run --ref` repository-aware follow-up, скорее всего между composite
  ref-backed `prepare ... run`, run-side diagnostics (`provenance` /
  `cache explain`) и zero-copy/cache-hit reuse.
- **Запланировано**: ZFS snapshot backend, опциональная VS Code интеграция,
  team on-prem baseline, облачный sharing, образование.

---

## Ближайший Следующий Шаг (Ожидает Выбора)

- **Направление**: продолжить repository-aware local CLI после уже
  реализованного standalone `run --ref` baseline, не раздувая scope без
  отдельного решения.
- **Текущая точка выбора**: определить первый post-`run --ref` follow-up PR.
- **Наиболее вероятные кандидаты**:
  - composite ref-backed semantics для `prepare ... run`, включая явный
    stage-boundary contract для run-side refs;
  - parity для run-side diagnostics, начиная с `--provenance-path` и/или
    `cache explain run ...` для raw и alias-backed flow;
  - более глубокая runtime/cache reuse работа, например zero-copy/cache-hit
    follow-up поверх уже существующего repository-aware binding path.
- **По-прежнему отложено дальше следующего slice**: hosted workflow expansion,
  Git mutation flow и более широкие team/cloud repository features.

---

## Milestones

### M0. Архитектурный baseline

**Результат**: стабильные концепты и контракты до тяжелой реализации.

- Зафиксировать канонические сущности: project, instance, run, artefact, share
- Зафиксировать core API surface (create/prepare/run/remove + status/events)
- Принять подход к runtime изоляции для MVP (локальные контейнеры vs альтернативы)

**Статус**: сделано (архитектура закреплена ADR и локальным OpenAPI для engine).

**Ключевые документы, которые нужно подготовить дальше**:

- [`api-contract.md`](api-contract.md)
- [`instance-lifecycle.md`](instance-lifecycle.md)
- [`state-cache-design.md`](architecture/state-cache-design.RU.md)

---

### M1. Local MVP (сценарий A1)

**Основной сценарий**: A1 локальная разработка с Liquibase.

**Целевые use cases**:

- UC-1 Поднять изолированный экземпляр БД
- UC-2 Применить миграции (Liquibase / SQL)
- UC-3 Запустить тесты / запросы / скрипты
- UC-4 Кэшировать и переиспользовать состояния БД

**Deliverables**:

- Taidon Engine + API (локальный режим) — **сделано** (локальный OpenAPI spec)
- Локальный runtime (контейнеры) с lifecycle экземпляра — **сделано**
- CLI (локальный): `sqlrs init`, `sqlrs config`, `sqlrs ls`, `sqlrs status`,
  `sqlrs plan:psql`, `sqlrs plan:lb`, `sqlrs prepare:psql`, `sqlrs prepare:lb`,
  `sqlrs run:psql`, `sqlrs run:pgbench`, `sqlrs rm` — **сделано**
- Cache v1 (prepare jobs + reuse state + retention) — **сделано (ядро)**
- Ограничения ёмкости кэша (лимиты размера + eviction) — **сделано**
  (core enforcement, CLI diagnostics, persisted size metadata, release
  cache-pressure coverage для локального профиля)
- Бэкенды snapshot ФС — **сделано** (заглушка overlayfs copy, Btrfs),
  **в планах** (ZFS)
- Liquibase адаптер (apply changelog) — **сделано (MVP scope)** (базовый
  локальный поток через `prepare:lb`/`plan:lb` реализован)
- Release happy-path e2e gate — **сделано** (покрытие Linux/macOS/Windows WSL
  для alias-driven сценариев Chinook, Sakila и Liquibase/JHipster, включая
  Btrfs в матрице валидации и отдельный local cache-pressure сценарий в
  release-проверках)
- Самодостаточное обнаружение движка в локальном релизе — **сделано** (native
  engine находится рядом с CLI или через `PATH`; Windows-архив также содержит
  совместимый Linux engine для WSL, атомарно устанавливает и восстанавливает
  его при повторном init)

**Опционально (nice-to-have)**:

- VS Code extension v0:
  - list instances
  - apply migrations
  - show logs and run results

**Exit criteria**:

- Cold start создает рабочий экземпляр
- Warm start переиспользует кэшированное состояние и значительно быстрее
- Миграции детерминированы и воспроизводимы

**Статус**: сделано (MVP baseline). Оставшийся публичный local follow-up теперь
в основном находится в M2 developer experience и в опциональных runtime
расширениях вроде ZFS.

---

### M2. Developer Experience (Post-MVP Local)

**Цель**: снизить трение при локальном onboarding и улучшить инструменты воспроизводимости.

**Целевые use cases**:

- UC-1, UC-2, UC-3 (с минимальной конфигурацией)

**Deliverables**:

- Project-tracked workflow aliases:
  - `*.prep.s9s.yaml` и `*.run.s9s.yaml`
  - явное cwd-relative alias-ref resolution для `plan`, `prepare` и `run`
  - alias-file-relative resolution для file-bearing paths, сохранённых внутри
    alias files
  - обычная композиция `prepare ... run` через raw и alias modes
  - явный alias inspection через `sqlrs alias ls/check`
  - явный alias creation через `sqlrs alias create`
- Advisory discovery tooling:
  - `sqlrs discover` с `--aliases`, `--gitignore`, `--vscode` и
    `--prepare-shaping`
  - copy-paste follow-up команды для создания alias, а также suggestions по
    исправлению `.gitignore` и `.vscode`
- Git-aware CLI (passive):
  - `diff` в path mode
  - bounded local `--ref` для `plan` / `prepare` / standalone `run` плюс
    ref-aware semantics в `diff` (`worktree|blob`)
  - provenance и cache explain
- VS Code extension v1 (optional):
  - one-click copy DSN
  - открыть SQL редактор (через существующие DB инструменты VS Code)

**Exit criteria**:

- Разработчик может выполнять типовые сценарии через явные repo-tracked recipes
  с низким локальным setup friction и понятной диагностикой происхождения/
  содержимого кэша.

**Статус**: сделано (public/local baseline). Alias execution, inspection,
authoring, generic advisory discovery, shared `internal/inputset` semantics,
`sqlrs diff`, bounded local `plan` / `prepare` / standalone `run` с `--ref` и
provenance/cache explain уже реализованы. Следующий repository-aware
follow-up ещё не выбран; более широкие ref-backed composite-семантики,
run-side diagnostics и zero-copy/cache-hit reuse теперь находятся за пределами
принятого M2 baseline.

---

### M3. Team On-Prem (сценарий A2)

**Основной сценарий**: общий Taidon для команды или подразделения.

**Целевые use cases**:

- UC-5 Интеграция с CI/CD
- UC-1..UC-4 в общем аутентифицированном deployment

**Deliverables**:

- Shared control-plane baseline для аутентифицированных multi-user deployment
- Remote users/orgs API contract и CLI client surface — **сделано
  (client/API contract slice)**; server-side persistence/provider
  implementation остается частью shared control-plane/auth baseline.
- Google OIDC CLI auth sessions — **сделано (CLI slice)**; accepted audiences
  в gateway и server-side auth/tenant policy остаются частью shared
  control-plane/auth baseline.
- Remote source input sync — **сделано (client/API contract slice)** для remote
  prepare/plan/cache-explain retry и upload source blob-ов; server-side source
  resolution и storage implementation остаются частью shared control-plane
  baseline.
- Team deployment gateway и service entrypoint baseline
- Shared state, artifact и audit handling с retention controls
- Базовый auth, tenant access, quotas и policy enforcement
- CI/CD integration templates и operator deployment path
- Baseline для team workflow compatibility и shared capacity scaling

**Exit criteria**:

- Несколько разработчиков могут параллельно запускать изолированные экземпляры
- Квоты предотвращают истощение ресурсов
- CI пайплайны стабильно поднимают и уничтожают экземпляры

---

### M4. Public Cloud Sharing (сценарий B3)

**Основной сценарий**: быстрые эксперименты и публичный шаринг.

**Целевые use cases**:

- UC-6 Делиться результатами экспериментов

**Deliverables**:

- Immutable run snapshots:
  - shareable bundles артефактов
  - redaction секретов
- Public read-only pages:
  - просмотр результатов
  - кнопка reproduce (clone в workspace пользователя)
- Anti-abuse controls:
  - rate limiting
  - лимиты экземпляров
  - enforcement TTL

**Exit criteria**:

- Пользователь может поделиться run ссылкой
- Другой пользователь может воспроизвести это в контролируемом окружении

---

### R1. Hosted Git Integration (опционально / research)

**Цель**: связать hosted/shared окружения с ревизиями репозитория, не делая
локальные открытые workflow зависимыми от hosted-инфраструктуры.

**Deliverables**:

- Git-backed project integration для hosted/shared environments
- Запуск или refresh workloads из выбранной ревизии репозитория
- Secure repository access и provenance-aware refresh flow

**Exit criteria**:

- Пользователь может привязать Git репозиторий и запустить экземпляр из выбранной
  ветки/коммита
- Обновления репозитория доступны в экземпляре без ручного ре-импорта

---

### R2. PR Automation (опционально / research)

**Цель**: автоматизировать warmup и diff workflow вокруг PR без раскрытия
credentials или runtime connection details.

**Deliverables**:

- Hosted automation path для PR-triggered warmup и diff workflows
- Integration surface для checks, callbacks или bot-driven review signals
- Reproducible PR summaries без раскрытия credentials или DSN
- Lifecycle hooks для refresh или retire подготовленного backend state

**Exit criteria**:

- PR label или slash command запускает warmup в контролируемом runner
- Check Run показывает summary diff/warmup без раскрытия секретов

---

### M5. Education (сценарии C4a/C4b)

**Основной сценарий**: задания, сдачи и оценивание.

**Целевые use cases**:

- UC-7 Подготовка заданий
- UC-8 Сдача и проверка результатов

**Deliverables**:

- Модель course/assignment/submission
- Autograding runner:
  - instructor-defined checks (SQL/tests)
  - структурированный отчет по оценке
- Instructor dashboard:
  - список сдач
  - сравнение результатов

**Exit criteria**:

- Преподаватель может опубликовать шаблон задания
- Студенты отправляют runs
- Преподаватель может оценивать консистентно

---

## Обоснование приоритизации

1. Local MVP (A1) дает немедленную продуктовую ценность и проверяет
   производительность ядра
2. Drop-in adoption снижает трение и ускоряет обратную связь
3. Team on-prem (A2) открывает enterprise-путь (квоты, auth, CI)
4. Sharing (B3) становится безопасным, когда есть артефакты, auth и квоты
5. Education (C4) естественно строится на sharing плюс role-based access

---

## Контроль области и решения, которые нужно зафиксировать рано

- DB engines для MVP (начать с одного, потом расширять)
- Механизм runtime-изоляции (container/k8s) и подход к snapshot
- Семантика кэша и правила инвалидации
- Границы безопасности (network egress, time limits, resource limits)
- Политика стабильности API (стратегия версионирования)

---

## Риски и смягчения

- Регрессии производительности из-за сильной изоляции
  - Смягчение: cache-first архитектура и benchmark gates
- Недетерминированные миграции и flaky states
  - Смягчение: строгий хэшинг, pinned images, воспроизводимые seeds
- Векторы абьюза в облаке (untrusted SQL)
  - Смягчение: сильная sandboxing, квоты, network policies, TTL

---

## Следующие документы для детализации

- [`api-contract.md`](api-contract.md) (REST/gRPC + events)
- [`sql-runner-api.md`](architecture/sql-runner-api.RU.md) (timeouts, cancel,
  streaming, cache-aware planning)
- [`runtime-and-isolation.md`](runtime-and-isolation.md) (local + k8s)
- [`liquibase-integration.md`](architecture/liquibase-integration.RU.md) (modes,
  config discovery)
- [`state-cache-design.md`](architecture/state-cache-design.RU.md) (snapshotting,
  hashing, retention)
- [`cli-spec.md`](cli-spec.md) (commands and exit codes)
- [`security-model.md`](security-model.md) (cloud-hardening, redaction, audit)
- [`runtime-snapshotting.md`](architecture/runtime-snapshotting.RU.md) (details
  of the snapshot mechanics)
- [`git-aware-passive.md`](architecture/git-aware-passive.RU.md) (CLI by ref,
  zero-copy, provenance)
- [`git-aware-active.md`](architecture/git-aware-active.RU.md) (PR automation,
  warmup/diff checks)
- [`k8s-architecture.md`](architecture/k8s-architecture.RU.md) (single entry
  gateway in k8s)
