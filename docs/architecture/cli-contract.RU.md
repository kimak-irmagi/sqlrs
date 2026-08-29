# Контракт CLI sqlrs (черновик)

Этот документ определяет **предварительный пользовательский контракт CLI** для `sqlrs`.
Он намеренно неполный и эволюционный, по духу близок к ранним версиям CLI `git`
или `docker`.

Цели:

- закрепить стабильную _ментальную модель_ для пользователя,
- определить пространства команд и ответственности,
- направлять внутренние решения по API и UX.

---

## 0. Принципы дизайна

1. **Каноническое имя CLI**: `sqlrs`
2. **Интерфейс на подкомандах** (стиль git/docker)
3. **Явное состояние вместо неявной магии**
4. **Композиция команд** (plan -> prepare -> run -> inspect)
5. **По умолчанию машинно-дружественный вывод** (JSON где уместно)
6. **Человеко-читаемые сводки** при интерактивном запуске

---

## 1. Высокоуровневая ментальная модель

С точки зрения пользователя `sqlrs` управляет:

- **states**: неизменяемые состояния БД, получаемые детерминированным процессом
  prepare
- **instances**: изменяемые копии states; все модификации БД происходят здесь
- **plans**: упорядоченные наборы изменений (например, Liquibase changesets)
- **runs**: исполнение планов, скриптов или команд
- в remote/shared деплойментах — **users** и **organizations**, которые задают
  аутентифицированный principal и tenant context для будущих shared workflows

```text
state  --(materialize)-->  instance  --(run)-->  new state
```

---

## Конвенция формы команд

Во всех командах sqlrs используется следующая форма:

```text
sqlrs <verb>[:<kind>] [subject] [options] [-- <command>...]
```

- `<verb>` - основная команда (`prepare`, `run`, `ls`, ...).
- `:<kind>` - опциональный селектор исполнителя/адаптера (например, `prepare:psql`,
  `run:pgbench`).
- `subject` - опциональный аргумент, зависит от команды (например, id инстанса,
  имя и т.п.).
- `-- <command>...` используется только для команд, выполняющих внешнюю команду
  (в основном `run`), и опционален для `run`-видов с дефолтной командой.

`sqlrs ls` не использует `:<kind>` и не принимает `-- <command>...`.

Исключение: `sqlrs diff` — meta-команда: сначала область diff, затем
оборачиваемая команда. **Сейчас:** один токен — `plan:psql`, `plan:lb`,
`prepare:psql`, `prepare:lb`. **Цель дизайна:** также двухстадийный composite
`prepare ... run`:

```text
sqlrs diff <diff-scope> <wrapped-command>            # поддержано (один токен)
sqlrs diff <diff-scope> <prepare-stage> <run-stage>  # пока не реализовано
```

Синтаксис остаётся совместимым с основным CLI вместо отдельного `diff`-DSL.

Внутреннее архитектурное правило: kind-specific file-bearing semantics
принадлежат общим CLI-side компонентам `internal/inputset` и переиспользуются
в execution, `sqlrs diff` и `sqlrs alias check`.

## Правила префиксов id

Там, где CLI ожидает **id состояния или инстанса**, пользователь может передать
hex-префикс (минимум 8 символов). CLI разрешает префикс без учета регистра и
сообщает об ошибке при неоднозначности.

Job id считаются **непрозрачными и должны задаваться целиком**; префиксы для job
не поддерживаются.

## 2. Группы команд (неймспейсы)

```text
sqlrs
  init
  auth
  user
  org
  status
  cache
  ls
  rm
  discover
  alias
  prepare
  watch
  plan
  run
  diff
```

Не все группы требуются в MVP.

---

## 3. Основные команды (MVP)

### 3.1 `sqlrs init`

Актуальная семантика команды описана в user guide:

- [`docs/user-guides/sqlrs-init.md`](../user-guides/sqlrs-init.md)

---

### 3.2 `sqlrs status`

См. user guide с авторитетным и актуальным описанием команды:

- [`docs/user-guides/sqlrs-status.md`](../user-guides/sqlrs-status.md)

Текущее направление дизайна:

- `status` остаётся командой health/engine-диагностики и по умолчанию
  включает компактную cache summary;
- `status --cache` разворачивает эту summary в полные bounded-cache diagnostics.

---

### 3.3 `sqlrs ls`

Актуальная семантика команды описана в user guide:

- [`docs/user-guides/sqlrs-ls.md`](../user-guides/sqlrs-ls.md)

Текущее направление дизайна:

- `ls --states` остаётся основной командой инвентаризации состояний;
- human-readable вывод `ls --states` сохраняет одну строку на состояние и не
  делает жёстких переносов внутри ячеек таблицы;
- в compact human output поле `CREATED` использует относительное представление,
  а `--long` переключает его на абсолютный UTC timestamp с точностью до секунд;
- в human-readable таблице состояний колонка kind использует компактный
  заголовок `KIND` и budget под существующие short sqlrs kind aliases;
- compact human-readable таблица состояний использует межколоночный gap в один
  символ, чтобы уменьшить pressure от глубокого дерева состояний;
- compact human-readable таблица `jobs` использует те же правила для `KIND`,
  `IMAGE_ID` и timestamps, что и `states`; при наличии и requested, и resolved
  image reference human-readable `jobs` предпочитает resolved image id, а
  `PREPARE_ARGS` ведёт себя как width-budgeted wide column по аналогии с task
  `ARGS`;
- диагностический job `signature` доступен в JSON как `signature` и попадает в
  human-readable таблицу `jobs` только по явному флагу `--signature`;
- compact human-readable таблица `tasks` сокращает префиксы kinds в `INPUT` и
  использует более короткий заголовок `OUTPUT_ID`, а также получает
  API-backed колонку task summary `ARGS`;
- для `INPUT` в `tasks` state id сокращаются по обычным правилам id, а image
  inputs используют тот же digest-aware compact formatter, что и колонки
  `IMAGE_ID`;
- на TTY поле `PREPARE_ARGS` получает budget относительно текущей ширины
  терминала и при необходимости усекается по середине;
- когда stdout не является TTY, широкие human-readable колонки используют
  стабильный fallback budget вместо попытки угадать ширину внешнего viewer;
- `--wide` отключает усечение широких текстовых колонок в human output
  (сейчас это `PREPARE_ARGS` и task `ARGS`), а `--long` независимо от `--wide`
  разворачивает id и timestamps;
- `ls --states --cache-details` добавляет операторские cache-метаданные к
  строкам состояний без введения новой top-level cache-команды в MVP-поверхность.

---

### 3.4 `sqlrs rm`

Актуальная семантика команды описана в user guide:

- [`docs/user-guides/sqlrs-rm.md`](../user-guides/sqlrs-rm.md)

---

### 3.5 `sqlrs prepare`

Актуальная семантика команды описана в user guide:

- [`docs/user-guides/sqlrs-prepare.md`](../user-guides/sqlrs-prepare.md)
- [`docs/user-guides/sqlrs-provenance.md`](../user-guides/sqlrs-provenance.md)
- [`docs/user-guides/sqlrs-watch.md`](../user-guides/sqlrs-watch.md)

Текущее поведение:

- `prepare <prepare-ref>` резолвит repo-tracked `*.prep.s9s.yaml` file от
  текущего рабочего каталога.
- bounded client-side `prepare --ref <git-ref>` поддерживает local и remote
  profiles только для single-stage prepare; он сохраняет
  cwd-relative semantics для alias и raw paths, проецируя caller cwd в
  context выбранного ref.
- `prepare --ref-mode worktree|blob` и `--ref-keep-worktree` используют ту же
  vocabulary и те же defaults, что уже приняты для `sqlrs diff`:
  `worktree` по умолчанию, `blob` как явный opt-in, а
  `--ref-keep-worktree` допустим только с `worktree`.
- `prepare` поддерживает `--watch` (по умолчанию) и `--no-watch`.
- `prepare --no-watch` возвращает `job_id` и ссылки на status/events, если
  команда не несет `--ref`.
- первый bounded slice для `prepare --ref` остается watch-only;
  `prepare --ref --no-watch` отклоняется, чтобы асинхронная ref-backed
  semantics не расширяла этот шаг.
- `prepare ... run ...` принимает обычную двухстадийную composite-форму, где
  каждая стадия может быть raw- или alias-mode.
- bounded `--ref` slice пока **не** расширяется на `prepare ... run ...`;
  prepare-stage с `--ref` пока остается только single-stage.
- `--provenance-path <path>` реализован для single-stage `prepare` с local и
  remote profiles, не меняя основной
  stdout/stderr контракт команды; JSON-артефакт записывается как side file,
  путь к которому резолвится от caller cwd.
- file-bearing paths, прочитанные из prepare alias, резолвятся относительно
  самого alias file, а raw-stage пути сохраняют обычную базу текущего рабочего
  каталога.
- В watch-режиме `Ctrl+C` открывает control prompt:
  - `[s] stop` (с подтверждением),
  - `[d] detach`,
  - `[Esc/Enter] continue`.
- Для composite-вызова `prepare ... run ...` действие `detach` отключает
  наблюдение за `prepare` и пропускает последующую фазу `run` в текущем
  CLI-процессе независимо от того, является ли эта фаза raw- или alias-backed.

- Добавить именованные экземпляры и флаги привязки (`--name`, `--reuse`, `--fresh`,
  `--rebind`).

### 3.6 `sqlrs plan`

Актуальная семантика команды описана в user guide:

- [`docs/user-guides/sqlrs-plan.md`](../user-guides/sqlrs-plan.md)
- [`docs/user-guides/sqlrs-plan-psql.md`](../user-guides/sqlrs-plan-psql.md)
- [`docs/user-guides/sqlrs-plan-liquibase.md`](../user-guides/sqlrs-plan-liquibase.md)
- [`docs/user-guides/sqlrs-provenance.md`](../user-guides/sqlrs-provenance.md)

CLI должен предоставлять `plan:<kind>` для каждого поддерживаемого `prepare:<kind>`.

Текущий alias mode:

- `sqlrs plan <prepare-ref>` резолвит repo-tracked prepare alias file от
  текущего рабочего каталога.
- bounded client-side `plan --ref <git-ref>` поддерживает local и remote
  profiles только для single-stage plan; он переиспользует те же правила
  projected-cwd, `worktree` и явного `blob`, что и bounded `prepare --ref`.
- `--provenance-path <path>` реализован для single-stage `plan` с local и remote
  profiles; он записывает один
  JSON side artifact без изменения основного human/JSON result payload.

---

### 3.7 `sqlrs run`

Актуальная семантика команды описана в user guide:

- [`docs/user-guides/sqlrs-run.md`](../user-guides/sqlrs-run.md)
- [`docs/user-guides/sqlrs-run-ref.md`](../user-guides/sqlrs-run-ref.md)

Документы реализованных ref-aware flow и component structure:

- [`run-ref-flow.RU.md`](run-ref-flow.RU.md)
- [`run-ref-component-structure.RU.md`](run-ref-component-structure.RU.md)

Текущее поведение:

- standalone `sqlrs run <run-ref> --instance <id|name>` резолвит repo-tracked
  `*.run.s9s.yaml` file от текущего рабочего каталога, сохраняя явный выбор
  runtime instance;
- bounded client-side `run --ref <git-ref>` поддерживает local и remote
  profiles для single-stage `run`; он применяется к alias-backed
  `run <run-ref>` и raw `run:psql` / `run:pgbench`, проецируя caller cwd в
  context выбранного ref;
- `run --ref-mode worktree|blob` и `--ref-keep-worktree` переиспользуют ту же
  vocabulary и defaults, что уже приняты для `plan` / `prepare` / `diff`:
  `worktree` по умолчанию, `blob` как явный opt-in, а
  `--ref-keep-worktree` допустим только с `worktree`;
- raw ref-backed file-bearing inputs сохраняют ту же shared семантику
  `internal/inputset`, которая уже используется live-filesystem `run` для
  `psql` и `pgbench`;
- в `prepare ... run <run-ref>` run alias использует instance, полученный из
  предшествующего `prepare`;
- в такой composite-форме `--instance` запрещён как явная ambiguity.
- первый bounded slice для `run --ref` пока **не** расширяется на
  `prepare ... run ...`; run-stage с `--ref` пока остается только
  single-stage;
- file-bearing paths, прочитанные из run alias, резолвятся относительно самого
  alias file.

---

### 3.8 `sqlrs watch`

Подключается к уже запущенной prepare job и стримит прогресс/события.

```bash
sqlrs watch <job_id>
```

См.:

- [`docs/user-guides/sqlrs-watch.md`](../user-guides/sqlrs-watch.md)

---

### 3.9 `sqlrs diff`

`sqlrs diff` — **группа составных команд** (дизайн): область diff вставляется
между `sqlrs` и content-aware командой, с тем же синтаксисом, что в основном CLI.
**Первый срез** (`frontend/cli-go`): сравниваются **closures файлов** (пути и
хеши) ровно для одного `plan:psql`, `plan:lb`, `prepare:psql` или `prepare:lb`
— **без вызова engine**. Долгосрочный источник истины для file semantics
вложенной команды - shared CLI-side слой `internal/inputset`; любые builders,
которые пока лежат в `internal/diff`, считаются переходными.

Цели на следующих срезах:

- `sqlrs diff ... plan ...` — **планы задач** prepare.
- `sqlrs diff ... prepare ...` — **тела задач** prepare.
- `sqlrs diff ... run ...` — файловые входы run.

Diff-опции задают область сравнения; синтаксис вложенной команды — обычный.

**Уже есть**

- scope: `--from-path`/`--to-path` или `--from-ref`/`--to-ref`
- ref: по умолчанию **`worktree`** (detached checkout); опционально **`blob`**
  читает объекты Git без checkout
- вложенные команды: только четыре токена выше
- глобальные `-v` и `--output`

**Дизайн (не всё сделано)**

- `prepare <alias>`, composite `prepare ... run`, позже `run:*`
- per-side bases для alias, как в основном CLI
- shared per-kind file semantics с execution и alias inspection через
  `internal/inputset`

См.:

- [`docs/user-guides/sqlrs-diff.md`](../user-guides/sqlrs-diff.md)
- [`docs/architecture/git-aware-passive.RU.md`](git-aware-passive.RU.md) (сценарий P3)

---

### 3.10 `sqlrs discover`

В post-MVP local design команда `discover` вводится как **advisory verb для
анализа workspace**:

```text
sqlrs discover [--aliases] [--gitignore] [--vscode] [--prepare-shaping]
```

Правила дизайна:

- `discover` read-only по умолчанию;
- `discover` не предоставляет флаг `--apply` в этом срезе;
- execution commands никогда не зависят от предыдущего discovery output;
- analyzer flags являются additive;
- если analyzer flags не указаны, `discover` запускает все stable analyzers в
  canonical order;
- первый stable analyzer set: `--aliases`, `--gitignore`, `--vscode` и
  `--prepare-shaping`;
- `--aliases` использует cheap prefilter,
  более глубокую специфичную для kind валидацию, ранжирование по топологии и
  отбрасывание уже покрытого существующими псевдонимами;
- `discover --aliases` предлагает вероятные кандидаты `*.prep.s9s.yaml` /
  `*.run.s9s.yaml` для поддерживаемых SQL и Liquibase workflows и печатает готовую
  к копированию и запуску команду `sqlrs alias create ...` для каждого сильного
  кандидата;
- `discover --gitignore` сообщает об отсутствии ignore coverage для local-only
  workspace artifacts и может печатать shell-native follow-up command для
  добавления недостающих ignore entries;
- `discover --vscode` сообщает об отсутствующих или неполных `.vscode/*.json`
  guidance files и может печатать shell-native follow-up command для создания
  или merge недостающих entries без перезаписи unrelated settings;
- `discover --prepare-shaping` сообщает advisory workflow-shaping opportunities
  для лучшего prepare reuse и cache friendliness;
- human output рендерится как numbered multi-line blocks с target, rationale и
  при наличии follow-up command на отдельных строках;
- `discover` пишет progress в `stderr`: delayed spinner в обычном режиме и
  line-based milestones в verbose mode;
- verbose progress использует analyzer/stage/candidate granularity и не
  трассирует каждый просмотренный файл;
- если важен shell syntax, follow-up commands рендерятся для текущей shell
  family;
- JSON output должен сохранять selected analyzers, стабильные per-analyzer
  summary counts и любые follow-up command strings в стабильной форме.

См.:

- [`docs/user-guides/sqlrs-discover.md`](../user-guides/sqlrs-discover.md)
- [`docs/user-guides/sqlrs-aliases.md`](../user-guides/sqlrs-aliases.md)
- [`alias-create-flow.RU.md`](alias-create-flow.RU.md)
- [`alias-create-component-structure.RU.md`](alias-create-component-structure.RU.md)
- [`discover-flow.RU.md`](discover-flow.RU.md)
- [`discover-component-structure.RU.md`](discover-component-structure.RU.md)

---

### 3.11 `sqlrs alias`

В post-MVP local design также вводятся явные alias-management команды для
repo-tracked workflow recipes:

```text
sqlrs alias create <ref> <wrapped-command> [-- <command>...]
sqlrs alias ls [--prepare] [--run] [--from <workspace|cwd|path>] [--depth <self|children|recursive>]
sqlrs alias check [--prepare] [--run] [--from <workspace|cwd|path>] [--depth <self|children|recursive>] [<ref>]
```

Эти команды inspect-ят, проверяют или создают alias files; они не заменяют
runtime `names`.

Примечания к дизайну:

- `create` materialize-ит repo-tracked alias file из wrapped `prepare:<kind>`
  или `run:<kind>` command;
- `create` переиспользует тот же wrapped-command parsing и file-bearing
  semantics, что и execution commands;
- `discover` печатает `alias create` command shape, но никогда не пишет файлы;
- scan mode по умолчанию использует `--from cwd --depth recursive`
- `check <ref>` переиспользует те же правила alias-ref resolution, что и
  execution-команды
- kind-specific file-bearing validation и closure semantics являются общими
  с execution и `diff` через одни и те же компоненты `internal/inputset`
- semantic-команда `show` сознательно не входит в текущий срез, потому что
  сами alias files остаются основным human-readable источником истины

См.:

- [`docs/user-guides/sqlrs-aliases.md`](../user-guides/sqlrs-aliases.md)
- [`alias-create-flow.RU.md`](alias-create-flow.RU.md)
- [`alias-create-component-structure.RU.md`](alias-create-component-structure.RU.md)
- [`alias-inspection-flow.RU.md`](alias-inspection-flow.RU.md)

---

### 3.12 `sqlrs cache`

См. user guide с авторитетным и актуальным описанием команды:

- [`docs/user-guides/sqlrs-cache-explain.md`](../user-guides/sqlrs-cache-explain.md)

Текущее направление дизайна:

- `status --cache` остается глобальной operator-facing командой для cache
  health;
- `ls --states --cache-details` остается surface-ом для per-state cache
  metadata;
- `cache explain prepare ...` - это реализованная read-only команда
  cache-diagnostics для одного single-stage prepare-oriented решения;
- `cache explain` переиспользует те же raw, alias-backed и client-side
  `--ref` binding semantics, что и single-stage `prepare`;
- первый slice пока **не** поддерживает wrapped `plan`, wrapped `run` или
  composite `prepare ... run ...`.

---

### 3.13 `sqlrs auth`

См. user guide с авторитетным описанием семантики команды:

- [`docs/user-guides/sqlrs-auth.md`](../user-guides/sqlrs-auth.md)

Текущее направление дизайна:

- `auth` управляет локальными CLI authentication sessions для remote/shared
  деплойментов.
- `auth login google` использует Google Authorization Code Flow with PKCE,
  loopback redirect listener и OS credential storage для refresh token.
- `auth status` показывает безопасную session metadata и никогда не печатает
  raw token-ы.
- `auth logout` удаляет локальные credentials и пытается revoke-нуть refresh
  token.
- Protected remote API commands сохраняют `SQLRS_TOKEN` как самый приоритетный
  override, затем используют stored OIDC session, если выбранный профиль имеет
  `auth.mode: oidcSession`, затем fallback-ятся к legacy static bearer profile
  behavior.
- Gateway по-прежнему получает только short-lived Google ID token как
  `Authorization: Bearer <id-token>`.

---

### 3.14 `sqlrs user`

См. user guide с авторитетным описанием семантики команды:

- [`docs/user-guides/sqlrs-users-orgs.md`](../user-guides/sqlrs-users-orgs.md)

Текущее направление дизайна:

- `user` — command group для remote/shared деплойментов; local mode отклоняет
  её до discovery или autostart локального engine.
- `user register` создает или возвращает профиль текущего
  аутентифицированного пользователя, выводя целевую external identity из
  проверенных OAuth/OIDC bearer-token claims.
- `user register` может быть отклонена, когда server-side self-registration
  отключена.
- `user create` — отдельная администраторская команда для provisioning профиля,
  привязанного к явно переданной external identity.
- Уникальность external identity принадлежит серверу и enforced по
  `provider + issuer + subject`.

---

### 3.15 `sqlrs org`

См. user guide с авторитетным описанием семантики команды:

- [`docs/user-guides/sqlrs-users-orgs.md`](../user-guides/sqlrs-users-orgs.md)

Текущее направление дизайна:

- `org` — command group для remote/shared деплойментов; local mode отклоняет
  её до discovery или autostart локального engine.
- `org create <slug>` создает новую организацию для зарегистрированного
  пользователя, который еще не привязан к организации, и делает этого
  пользователя admin организации.
- `org ls` и `org get <org-ref>` scoped по организациям, видимым текущему
  аутентифицированному пользователю.

---

## 4. Вывод и скриптинг

- Вывод по умолчанию: человеко-читаемый
- `--json`: машинно-читаемый
- Стабильные схемы для JSON-вывода

Подходит для CI/CD.

---

## 5. Источники входа (локальные пути, URL, удаленные загрузки)

Везде, где CLI ожидает файл или директорию, он принимает:

- локальный путь (файл или директория)
- публичный URL (HTTP/HTTPS)
- серверный `source_id` (предыдущая загрузка)

Поведение зависит от цели:

- локальный engine + локальный путь: передать путь напрямую
- удаленный engine + публичный URL: передать URL напрямую
- удаленный engine + локальный путь: загрузить в source storage (чанками) и
  передать `source_id`

Это держит `POST /runs` компактным и дает возобновляемые загрузки для больших проектов.

---

## 6. Совместимость и расширяемость

- Liquibase рассматривается как внешний планировщик/исполнитель
- CLI не раскрывает Liquibase internals напрямую
- Будущие бэкенды (Flyway, raw SQL, кастомные планировщики) вписываются в тот же
  контракт

---

## 7. Не-цели (для этого CLI контракта)

- Полный паритет с опциями Liquibase CLI
- Интерактивный TUI
- GUI bindings

---

## 8. Открытые вопросы

- Разрешать ли несколько prepare/run шагов на invocation? (см. user guides)
- Должен ли `plan` быть неявным в `migrate` или всегда явным?
- Сколько истории состояний показывать по умолчанию?
- Нужны ли флаги подтверждения для destructive операций?

---

## 9. Философия

`sqrls` (sic) — это не база данных.

Это **движок управления состояниями и выполнения** для баз данных.

CLI должен делать переходы состояний явными, инспектируемыми и воспроизводимыми.
