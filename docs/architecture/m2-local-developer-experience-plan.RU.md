# План M2: Local Developer Experience

Статус: завершённый public/local baseline (2026-08-31); более широкие follow-up
задачи явно перечислены ниже.

Этот документ определяет план реализации для **публичной/локальной** части
roadmap milestone M2 после отказа от implicit repo-layout guessing в пользу
**явных versioned alias files** и advisory workspace discovery.

План остается в публичной/open части roadmap. Он не возвращает private
Team/Shared sequencing или внутренние детали rollout backend.

## 1. Результат

M2 должен уменьшить трение при локальном onboarding и улучшить инструменты
воспроизводимости для пользователей `sqlrs`, работающих из репозитория на
рабочей станции разработчика.

Ожидаемый публичный результат:

- repo-tracked workflow recipes для типовых `plan`, `prepare` и `run` flow;
- явное разделение между versioned workflow definitions и local-only
  workspace configuration;
- advisory discovery tooling, предлагающий улучшения, но не участвующий в
  execution semantics;
- детерминированные shared CLI-side inputset building blocks для последующих
  execution, `diff`, Git-ref, provenance и cache explanation возможностей.

## 2. Ограничения

- Ранние M2-срезы должны оставаться local-first и CLI-led.
- Нельзя полагаться на execution-time guesswork.
- Versioned workflow definitions должны жить отдельно от `.sqlrs/config.yaml`.
- Aliases и runtime names должны оставаться разными сущностями.
- Пока engine API явно не нужен, предпочтение отдается CLI-only изменениям.
- Command syntax должен оставаться additive и explicit.
- Для file-bearing semantics каждого tool kind должен существовать один shared
  CLI-side источник истины, общий для execution, `diff` и `alias check`.

## 3. Утвержденный Порядок Срезов

Утвержден следующий порядок реализации:

1. file-based prepare aliases
2. run aliases, alias inspection и mixed `prepare ... run` composition
3. `discover --aliases`
4. generic discover analyzers
5. shared local inputset layer
6. `sqlrs diff` в path mode
7. Git ref execution baseline
8. provenance и cache explain

Такой порядок дает раннюю публичную ценность и при этом держит будущую
Git-aware часть ограниченной и тестируемой.

## 4. PR-срезы

### 4.1 PR1: Базовый Срез File-Based Prepare Aliases

**Статус: завершено.**

**Цель**: сделать repo-tracked prepare recipes исполняемыми без смешивания их с
local workspace config.

**Основной результат**:

- `sqlrs plan <prepare-ref>` резолвит `<prepare-ref>.prep.s9s.yaml` от
  текущего рабочего каталога
- `sqlrs prepare <prepare-ref>` резолвит тот же класс alias files
- поддерживается exact-file escape через trailing `.`
- runtime names остаются отдельными от alias refs

**Ожидаемая работа**:

- определить формат prepare alias files
- реализовать alias-ref resolution от текущего рабочего каталога
- резолвить file-bearing alias arguments относительно директории alias file
- добавить alias-mode dispatch для `plan` и `prepare`
- задокументировать взаимодействие с `--name`

**Ожидаемые тесты**:

- тесты alias-ref resolution
- тесты exact-file escape
- тесты валидации prepare aliases
- negative tests для missing files и kind/schema errors

**Вне scope**:

- run aliases
- discover
- diff
- Git refs

### 4.2 PR2: Run Aliases, Alias Inspection и Mixed Composition

**Статус: завершено.**

**Цель**: завершить явную alias execution surface и заставить обычные
`prepare ... run` pipeline работать через raw и alias modes.

**Основной результат**:

- standalone `sqlrs run <run-ref> --instance <id|id-prefix|name>` резолвит
  `<run-ref>.run.s9s.yaml` от текущего рабочего каталога
- `prepare ... run ...` принимает смешанные raw/alias комбинации
- `sqlrs alias ls [--prepare] [--run] [--from <workspace|cwd|path>] [--depth <self|children|recursive>]`
- `sqlrs alias create <ref> <wrapped-command> [-- <command>...]`
- `sqlrs alias check [--prepare] [--run] [--from <workspace|cwd|path>]`
  `[--depth <self|children|recursive>] [<ref>]`

**Ожидаемая работа**:

- определить формат run alias files
- добавить run alias-mode dispatch
- разрешить смешивание raw/alias стадий в composite `prepare ... run`
- запретить `--instance`, если target instance уже выбран предшествующим
  `prepare`
- реализовать alias listing/check команды с ограниченной настройкой scan scope
- реализовать alias creation из wrapped execution commands
- сохранить raw `run:<kind>` mode рядом с alias mode

**Ожидаемые тесты**:

- тесты run alias resolution
- тесты composite grammar для смешанных raw/alias стадий
- тесты alias inspection commands
- тесты alias create validation и write-path
- тесты scan root и scan depth для `alias ls` / `alias check`
- тесты single-alias `check <ref>`, включая ambiguity и exact-file escape
- тесты валидации kind-specific constraints
- negative tests для missing `--instance`, conflicting composite `--instance`
  и wrong alias type

**Вне scope**:

- discover analyzers
- diff
- Git refs

### 4.3 PR3: `discover --aliases`

**Статус: завершено.**

**Цель**: помочь авторам репозитория bootstrap-ить explicit alias files без
зависимости execution path от эвристик.

**Основной результат**:

- `sqlrs discover --aliases` анализирует workspace и сообщает candidate alias
  files как ready-to-copy `sqlrs alias create ...` commands
- команда advisory и read-only по умолчанию

**Ожидаемая работа**:

- добавить семейство команд `discover`
- реализовать analyzer `--aliases`
- определить стабильный human и JSON output для findings и create-command strings
- сохранить жесткое разделение между discovery и execution semantics

**Ожидаемые тесты**:

- тесты выбора analyzers
- тесты candidate detection
- тесты JSON finding shape
- regression tests, подтверждающие, что `plan/prepare/run` не fallback-ятся к discovery

**Вне scope**:

- generic discover analyzers
- write mode для unrelated workspace files
- diff

### 4.4 PR4: Generic Discover Analyzers

**Статус: завершено для поставленного analyzer baseline.** Текущий analyzer
`--vscode` проверяет YAML schema mapping для sqlrs, а текущий
`--prepare-shaping` использует ограниченные filename heuristics.

**Отслеживаемые follow-up задачи**:

- [#90](https://github.com/kimak-irmagi/sqlrs/issues/90): рекомендации VS Code
  extensions и дополнительные editor checks
- [#91](https://github.com/kimak-irmagi/sqlrs/issues/91): анализ input graph и
  alias layout для prepare shaping

**Цель**: превратить `discover` в общий advisory workflow для local repository
hygiene и cache-friendly shaping.

**Основной результат**:

- базовый analyzer framework для нескольких selectors
- стартовые non-alias analyzers, например:
  - `--gitignore`
  - `--vscode`
  - `--prepare-shaping`

**Ожидаемая работа**:

- добавить analyzer registration и selection rules
- где это уместно, определить shared finding structure
- сохранить analyzer-specific semantics явными

**Ожидаемые тесты**:

- тесты multi-analyzer selection
- per-analyzer finding tests
- negative tests для incompatible write/update modes, если они появятся

**Вне scope**:

- Git-ref workflow
- provenance

### 4.5 PR5: Shared Local InputSet Layer

**Статус: завершено.**

**Цель**: зафиксировать единый shared CLI-side источник истины для
revision-sensitive local inputs.

**Основной результат**:

- у CLI появляется единый слой `inputset` для поддерживаемых file-bearing tool kind
- тот же слой переиспользуется в `prepare`, `plan`, `run`, `alias check`,
  `diff`, discover analyzers, Git-ref mode, provenance и cache explanation

**Ожидаемая работа**:

- определить CLI-side абстракции parse/bind/collect/project, а также общие
  типы resolver и filesystem
- реализовать общий `psql` inputset component
- реализовать общий Liquibase inputset component
- реализовать общий `pgbench` inputset component для file-bearing run inputs
- перевести execution и alias-check flow на shared layer
- определить стабильные правила hashing и ordering

**Ожидаемые тесты**:

- тесты parity для parser/binding у `psql`, Liquibase и `pgbench`
- тесты детерминированного порядка
- тесты обхода closure для `psql`
- тесты обхода changelog graph для Liquibase
- тесты стабильности hash на normalization cases
- тесты parity projection между execution и inspection consumers

### 4.6 PR6: `sqlrs diff` в Path Mode

**Цель**: поставить первый user-visible Git-aware workflow без обязательного
доступа к Git objects.

**Статус: завершено для принятого baseline сравнения input graph.** Path- и
ref-mode работают для одного wrapped `plan:psql` / `plan:lb` / `prepare:psql` /
`prepare:lb`; сравнение использует file-list closure и hashes через shared
inputset layer и не обращается к engine.

**Не реализовано из более широкого design target**: parsing двухстадийного
`prepare ... run`, alias-backed wrapped stages, сравнение engine task/payload и
per-phase output. Эти задачи находятся вне завершённого baseline и пока не имеют
отдельных GitHub issues.

**Основной результат** (цель):

- `sqlrs diff --from-path/--to-path ...` работает для одного wrapped `plan:*`
  или `prepare:*` command, а также для обычной двухстадийной grammar
  `prepare ... run`

**Ожидаемая работа**:

- добавить dispatch команды `diff`
- отдельно парсить diff scope и wrapped command
- переиспользовать shared inputset components из PR5 вместо diff-owned
  parsers/builders
- отдельно вычислять wrapped `prepare` и `run` фазы, если используется
  composite
- реализовать human и JSON rendering

**Ожидаемые тесты**:

- тесты парсинга аргументов
- path-mode compare tests для `plan:psql`
- path-mode compare tests для `prepare:lb`
- тесты parity, показывающие, что `diff` использует ту же per-kind input
  semantics, что и shared execution layer
- path-mode compare tests для mixed raw/alias `prepare ... run`
- тесты JSON shape

### 4.7 PR7: Git Ref Execution Baseline

**Статус: завершено.** Исходный baseline для single-stage `plan` / `prepare`
реализован; standalone `run --ref` был поставлен отдельным последующим срезом.

**Цель**: позволить пользователю выполнять repository-aware workflow из Git
revision без изменения working tree.

**Основной результат**:

- bounded local `--ref` support для single-stage `plan` и `prepare`
- raw и alias-backed prepare flows, работающие против выбранного ref
- режим по умолчанию `worktree` и явный `blob`, используя ту же vocabulary, что
  и `sqlrs diff`

**Ожидаемая работа**:

- Git ref resolution
- projected-cwd resolution внутри выбранного ref
- binding alias и raw-stage против ref-backed filesystem context
- blob-mode input access
- worktree lifecycle handling
- понятные user-facing errors для non-Git, missing-ref и missing-path случаев

**Явно вне scope**:

- standalone `run --ref`
- `prepare ... run ...` с ref-backed prepare-stage
- provenance и `cache explain`
- remote runner semantics

**Ожидаемые тесты**:

- тесты парсинга и разрешения refs
- тесты projected-cwd и alias-resolution под `--ref`
- тесты raw file-bearing paths под `--ref`
- worktree lifecycle tests
- failure tests для bad refs, missing projected cwd и missing files на ref

### 4.8 PR8: Provenance и Cache Explain

**Статус: завершено для принятого single-stage baseline.**

**Отслеживаемые follow-up задачи**:

- [#92](https://github.com/kimak-irmagi/sqlrs/issues/92): конкретная
  классификация причин cache miss сверх `no_matching_state`
- [#93](https://github.com/kimak-irmagi/sqlrs/issues/93): отдельная семантика
  asynchronous acceptance и cancellation в provenance artifacts

**Цель**: сделать repository-aware local workflows воспроизводимыми и объяснимыми.

**Основной результат**:

- `--provenance-path <path>` для single-stage local `plan` / `prepare`
  workflows, включая raw, alias-backed и bounded local `--ref` binding
- `sqlrs cache explain prepare ...` для read-only user-facing hit/miss
  diagnostics над одним single-stage prepare-oriented решением

**Ожидаемая работа**:

- определить provenance payload
- переиспользовать shared local binding path, чтобы собирать детерминированные
  input hashes и для provenance, и для cache explanation
- добавить read-only engine API для prepare-oriented cache explanation
- реализовать human и JSON output для cache explain без расширения текущих
  result envelopes у `plan` / `prepare`

**Ожидаемые тесты**:

- тесты provenance payload
- cache-explain tests для hit/miss
- тесты binding parity, доказывающие, что `cache explain` и provenance
  переиспользуют те же raw/alias/ref semantics, что и реальный `prepare`
- тесты text/JSON rendering

## 5. Сквозные Правила

- Каждый slice должен нести самостоятельную пользовательскую ценность.
- Discovery остается advisory, если только явно не выбран определенный write mode.
- Execution commands никогда не должны зависеть от предыдущего `discover`.
- Alias refs должны оставаться детерминированными и зависеть от текущего
  рабочего каталога.
- File-bearing paths внутри alias files должны резолвиться от директории самого
  alias file.
- Names остаются runtime handles и не заменяют aliases.
- Если slice меняет command semantics, в том же PR нужно обновлять релевантные
  user guide и architecture/contract docs.

## 6. Явные Не-Цели Этого Плана

- sequencing rollout для Team/Shared backend
- PR automation или hosted Git integrations
- сама поставка IDE extension
- `sqlrs compare`
- широкая переработка command surface за пределами alias/discover/Git-aware roadmap

## 7. Definition of Done для M2

M2 можно считать завершенным для публичного/local roadmap, когда:

- repo-tracked prepare/run aliases существуют и исполнимы;
- `discover` дает полезный advisory analysis для local repositories;
- `sqlrs diff` существует в path mode;
- Git-ref execution поддерживается в ограниченном local baseline;
- provenance и cache explanation доступны для repository-aware flow;
- итоговая документация описывает shipped public behavior без опоры на private
  deployment assumptions.

## 8. Ссылки

- [`../roadmap.RU.md`](../roadmap.RU.md)
- [`../user-guides/sqlrs-aliases.md`](../user-guides/sqlrs-aliases.md)
- [`cli-contract.RU.md`](cli-contract.RU.md)
- [`git-aware-passive.RU.md`](git-aware-passive.RU.md)
- [`diff-component-structure.RU.md`](diff-component-structure.RU.md)
