# Git-aware semantics: пассивные функции (CLI)

Статус: **смешанный**. Сценарии P1, P4 и P6 фиксируют уже реализованные client-side
ref, provenance и cache-explain surfaces. Standalone `run --ref` также
реализован и описан в
[`../user-guides/sqlrs-run-ref.md`](../user-guides/sqlrs-run-ref.md), с
реализованным interaction flow в [`run-ref-flow.RU.md`](run-ref-flow.RU.md) и
внутренней структурой в
[`run-ref-component-structure.RU.md`](run-ref-component-structure.RU.md);
остальная часть документа остается future design. Сейчас публичный MVP CLI
по-прежнему опирается на вызовы вида `sqlrs prepare:psql ... run:psql ...`.

Цель: добавить git-aware возможности **без вмешательства в привычный процесс работы**.
Все функции в этом документе активируются **только по явной команде/флагу пользователя**
и не требуют настройки репозитория "под sqlrs".

## Принципы дизайна

- **Не угадываем намерения по репозиторию.** Контекст задает пользователь
  явными формами команд вроде `sqlrs plan|prepare --ref ...`,
  `sqlrs run --ref ...` и `sqlrs diff ...`.
- **Минимум побочных эффектов.** Где семантика команды сохраняется, предпочтительны
  чтения из Git-объектов, но совместимость важнее. В сценариях, где нужна полная
  файловая семантика, по умолчанию остаются временные `worktree`, оставляющие
  минимальные следы в `.git/worktrees`.
- **Сначала быстрый путь.** Сначала пытаемся найти готовое состояние в кэше sqlrs
  по хешам задействованных файлов. Если не нашли — строим.
- **Prepare-oriented execution воспроизводим.** Single-stage `plan` и
  `prepare` могут сохранить provenance manifest; run-side provenance не входит
  в текущий surface.
- **В remote-режиме Git остается на стороне клиента.** CLI разрешает `--ref`,
  связывает выбранный input graph и передает необходимые source blobs через
  [remote source sync](../user-guides/remote-source-input-sync.md). Backend не
  нужен server-side mirror или VCS credentials.

---

## Сценарий P1. Repository-backed plan/prepare по git ref: `--ref`

Примечание по текущему публичному срезу: этот сценарий описывает уже
реализованный bounded baseline для `plan` / `prepare --ref`. Более поздние
follow-up были вынесены в отдельные slice: сначала provenance/cache
explanation, а теперь и standalone `run --ref`.

### Мотивация

Пользователь хочет вычислить prepare-oriented workflow **как в
коммите/ветке/теге**, не портя текущий рабочий каталог (грязное состояние,
открытые IDE, параллельные задачи).

### UX / CLI

Текущие публичные command shapes `plan` / `prepare` предоставляют одно
явное семейство stage-local флагов.

```bash
sqlrs plan --ref <git-ref> <prepare-alias>
sqlrs plan:psql --ref <git-ref> -- -f ./prepare.sql
sqlrs prepare --ref <git-ref> <prepare-alias>
sqlrs prepare:lb --ref <git-ref> -- update --changelog-file db/changelog.xml
```

Где `<git-ref>`: `HEAD`, `origin/main`, `abc1234`, `v1.2.3`, `refs/pull/123/head`
(если доступно локально).

Важная граница текущего публичного surface: Git resolution и revision
projection выполняются в CLI. Поддерживаются local и remote profiles; для
remote profile выбранный input closure передается через source sync.

Дополнительный guardrail первого публичного slice: `prepare --ref` остается
только в watch mode, поэтому `prepare --ref --no-watch` отклоняется.

Опции поведения:

- `--ref-mode worktree|blob` (по умолчанию `worktree`)
  - `worktree`: создать временный `git worktree` и удалить после завершения
    команды
  - `blob`: читать нужные файлы прямо из git-объектов (без извлечения всего репо)
- `--ref-keep-worktree` (для отладки: не удалять временный worktree)

### Алгоритм реализации (набросок)

1. Определить корень репозитория (если нет — ошибка `not a git repo`).
2. Разрешить `<git-ref>` в `commit/tree`.
3. Разрешить projected cwd вызывающего процесса внутри выбранной ревизии.
4. Привязать alias-backed или raw `plan` / `prepare` inputs в этом ref-backed
   context.
5. Собрать file-bearing inputs через shared kind-specific inputset layer.
6. Продолжить обычный flow `plan` или `prepare`.
7. Удалить временный worktree, если не задан `--ref-keep-worktree`.

Code-quality constraint для этого slice: generic repo/ref/worktree handling
принадлежит `internal/refctx`, alias-target resolution принадлежит
`internal/alias`, а per-kind closure semantics принадлежат `internal/inputset`.
`internal/app` должен только оркестрировать эти общие компоненты.

---

## Сценарий P2. Zero-copy cache hit (ускорение без извлечения файлов)

### Мотивация

В больших репозиториях извлечение/checkout дорого, хотя sqlrs может уже иметь
нужное состояние по хешам миграций.

### UX / CLI

Включается автоматически в ref-backed prepare-oriented flow с
`--ref-mode blob` (или отдельным флагом):

```bash
sqlrs prepare --ref <ref> --ref-mode blob <prepare-alias>
```

Опционально:

- `--zero-copy=auto|off` отключить оптимизацию или оставить авто-режим

### Алгоритм реализации

1. В blob-mode получить список файлов и их blob-хеши через `git ls-tree`.
2. Посчитать ключи кэша по хешам.
3. Проверить кэш sqlrs **до** извлечения файлов.
4. Если попадание в кэш — не извлекать файлы на диск, сразу выдавать окружение.

Примечание: если нужные blob-объекты отсутствуют локально (`partial clone`/LFS),
потребуется `git fetch` или переход в `worktree` режим.

---

## Сценарий P3. Taidon-aware diff через оборачивание существующей sqlrs-команды: `diff`

### Мотивация

Обычный `git diff` показывает текст, но пользователю нужен ответ “что изменилось
**с точки зрения реальной sqlrs-команды**”, оставаясь синтаксически близким к
основному CLI.

### UX / CLI

Вместо отдельного mini-DSL вида `--prepare <path>` команда `diff` оборачивает
одну существующую content-aware команду sqlrs и вычисляет её в двух контекстах.

```bash
sqlrs diff --from-ref <refA> --to-ref <refB> plan:psql -- -f ./prepare.sql
sqlrs diff --from-ref <refA> --to-ref <refB> prepare:lb -- update --changelog-file db/changelog.xml
sqlrs diff --from-path <pathA> --to-path <pathB> prepare:psql -- -f ./prepare.sql
```

Правила:

- diff-опции идут до оборачиваемой команды;
- оборачиваемая команда сохраняет свой текущий синтаксис без изменений;
- глобальный `-v` остаётся флагом подробного вывода, а глобальный `--output` -
  переключателем text/json;
- file-bearing semantics оборачиваемой команды приходят из shared CLI-side
  компонента `inputset`, выбранного по kind;
- **Текущий CLI** (`frontend/cli-go`): ровно **один** wrapped-токен из
  `plan:psql`, `plan:lb`, `prepare:psql`, `prepare:lb`; сравниваются **closures
  файлов** (хеши), без engine. **Режим ref** по умолчанию — **`worktree`**
  ради полной семантики ФС; явный **`blob`** использует `git show` /
  `git ls-tree`.
- **Дизайн / потом**: composite `prepare ... run`, alias `prepare <ref>`; полные
  производные представления (планы, payload prepare).
- будущая standalone-поддержка `run:*` возможна только для file-backed входов,
  потому что inline-only вызовы могут не иметь payload, зависящего от ревизии.

Примеры composite-формы (**цель дизайна**; парсер CLI пока не всё поддерживает):

```bash
sqlrs diff --from-ref <refA> --to-ref <refB> prepare chinook run:psql -- -f ./queries.sql
sqlrs diff --from-path <pathA> --to-path <pathB> prepare:psql -- -f ./prepare.sql run smoke
```

Что именно сравнивает `diff`:

- **Сейчас:** один и тот же **resolved file graph + контент** для `plan:*` и
  `prepare:*` данного kind (psql vs lb).
- **Целевое:** `plan:*` → task plan; `prepare:*` → тела + граф; `run:*` → только
  файловые входы.

### Алгоритм реализации

1. Разобрать diff-scope (`from/to ref` или `from/to path`).
2. Разобрать оборачиваемую команду (сейчас: один `plan:*` или `prepare:*` токен).
3. Для каждой стороны независимо привязать оборачиваемую команду через shared
   компонент `inputset` для ее kind и собрать resulting file set в соответствии
   с ревизией или path-контекстом этой стороны.
4. **Сейчас:** построить списки файлов (closures) и сравнить Added / Modified /
   Removed.
   **Потом:** richer representations и per-phase вывод для `prepare ... run`.
5. Вывести human или JSON.

---

## Сценарий P4. Provenance side artifact для single-stage plan/prepare

### Мотивация

В реальных командах быстро возникает вопрос “что именно ты запускал?”. Нужен
артефакт, который можно приложить к багрепорту или повторить через месяц, не
меняя основной контракт результата CLI-команды.

### UX / CLI

Включается явно через добавочный path-флаг:

```bash
sqlrs plan --provenance-path ./artifacts/chinook-plan.json chinook
sqlrs prepare --ref origin/main --provenance-path ./artifacts/chinook-prepare.json chinook
sqlrs prepare:psql --provenance-path ./artifacts/prepare.json -- -f ./prepare.sql
```

Границы первого публичного slice:

- только single-stage `plan` и `prepare`;
- raw- и alias-backed prepare flows;
- local и remote profiles, live inputs и client-side `--ref`;
- без standalone `run`;
- без composite `prepare ... run ...`;
- без автоматической эмиссии без явного флага;
- без печати provenance как части основного stdout payload.

Содержимое (минимум):

- family / kind / class команды (raw vs alias)
- timestamp invocation
- workspace root, caller cwd и selected alias path при наличии
- metadata выбранного ref при использовании `--ref`
- normalized prepare args
- собранные input entries с детерминированными hash-ами
- pre-execution cache decision snapshot:
  - signature
  - hit vs miss
  - matched state id при наличии
  - miss reason code, если он известен
- terminal outcome summary (success/failure/canceled плюс state/job
  identifiers, если они доступны)

### Алгоритм реализации

1. Сначала разобрать и провалидировать обычную `plan` / `prepare` команду.
2. Привязать выбранную stage через те же raw/alias/ref semantics, что и в
   обычном execution.
3. Собрать детерминированный local input graph через shared слой
   `internal/inputset`.
4. Если задан `--provenance-path`, запросить у engine read-only pre-execution
   cache explanation для этой уже привязанной stage.
5. Выполнить обычный flow `plan` или `prepare`.
6. Сериализовать один JSON side artifact по caller-cwd-relative пути.

Важное замечание по поведению: артефакт фиксирует cache decision, увиденное
непосредственно перед execution. Если другой процесс изменит состояние cache
между explain-шагом и реальным `prepare`, записанное объяснение всё равно
остается корректным pre-execution diagnostic snapshot, а не post-factum
гарантией.

---

## Сценарий P5. Compare: один запрос на двух состояниях

### Мотивация

QA/разработчику нужно быстро сравнить результат/ошибку на двух версиях схемы
(например, base и PR).

### UX / CLI

```bash
sqlrs compare \
  --from-ref <refA> --from-prepare <path> \
  --to-ref <refB> --to-prepare <path> \
  -- psql -c "select * from flights limit 10"
```

Вывод:

- exit codes
- stderr/stdout (с лимитами)
- (опционально) diff результатов в табличном формате

Опции:

- `--diff text|json|table`
- `--timeout-ms 5000`
- `--max-rows 1000`

### Алгоритм реализации

1. Для `from-ref` и `to-ref` поднять окружение (с кэшем).
2. Выполнить одну и ту же команду.
3. Собрать результаты и сформировать сравнение.

---

## Сценарий P6. `cache explain` для одного prepare-oriented решения

### Мотивация

Пользователь хочет понять, переиспользует ли sqlrs уже существующее состояние
для одного single-stage prepare-oriented workflow прямо сейчас, и если нет, то
почему.

### UX / CLI

Текущая публичная команда остается read-only и prepare-oriented:

```bash
sqlrs cache explain prepare <prepare-alias>
sqlrs cache explain prepare --ref <ref> <prepare-alias>
sqlrs cache explain prepare:psql -- -f ./prepare.sql
sqlrs cache explain prepare:lb -- update --changelog-file db/changelog.xml
```

Вывод:

- hit vs miss для final prepare state lookup
- engine-computed signature
- matched state id при наличии
- best-known reason code, когда final state отсутствует

Этот первый slice **не** принимает wrapped `plan`, wrapped `run` или composite
`prepare ... run ...`.

### Алгоритм реализации

1. Разобрать wrapped single-stage prepare grammar (`prepare <alias>` или
   `prepare:<kind> ...`).
2. Привязать stage через тот же raw/alias/ref путь, что и у реального
   `prepare`.
3. Собрать детерминированный input graph через shared слой `inputset`.
4. Отправить один read-only запрос в engine, который считает ту же final
   prepare signature и делает тот же cache lookup, что и реальное execution,
   но не создает job и не мутирует cache.
5. Отрендерить human или JSON output, объединив engine decision с local
   input/ref metadata, уже известными CLI.

Store-health diagnostics остаются отдельными:

- `sqlrs status --cache`
- `sqlrs ls --states --cache-details`

---

## Минимальный MVP пассивных функций

1. bounded client-side `plan` / `prepare --ref` с `worktree` по умолчанию и явным
   `blob`
2. `sqlrs diff --from-ref/--to-ref <wrapped-command...>` для одной команды
   `plan:*` или `prepare:*`
3. provenance side artifact для bounded single-stage `plan` / `prepare`
4. `cache explain prepare ...` (простая версия)
5. standalone `run --ref` для raw и alias-backed single-stage run flow
