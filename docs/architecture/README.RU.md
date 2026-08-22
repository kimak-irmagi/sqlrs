# Документы по архитектуре

Точки входа по архитектуре и дизайну сервисов Taidon.

- [`mvp-architecture.RU.md`](mvp-architecture.RU.md) - состав MVP-сервисов и цели.
- [`diagrams.RU.md`](diagrams.RU.md) - высокоуровневые диаграммы компонентов и
    k8s топологии.
- [`local-deployment-architecture.RU.md`](local-deployment-architecture.RU.md) -
    локальный профиль (тонкий CLI, эфемерный engine).
- [`shared-deployment-architecture.RU.md`][sda] -
  team/cloud профиль (gateway/orchestrator, multi-tenant).
- [`engine-internals.RU.md`](engine-internals.RU.md) - внутренняя структура sqlrs
  engine.
- [`runtime-snapshotting.RU.md`](runtime-snapshotting.RU.md) - модель хранения,
  snapshot/backends (OverlayFS/копирование/etc).
- [`state-cache-design.RU.md`](state-cache-design.RU.md) - cache keys, триггеры,
  retention, локальный layout.
- [`state-cache-capacity-control.RU.md`](state-cache-capacity-control.RU.md) -
  bounded cache-политика (лимиты емкости, watermark-ы, семантика eviction).
- [`sql-runner-api.RU.md`](sql-runner-api.RU.md) - поверхность API runner,
  таймауты, отмена, стриминг.
- [`liquibase-integration.RU.md`](liquibase-integration.RU.md) - стратегия
  Liquibase провайдера и структурированные логи.
- [`k8s-architecture.RU.md`](k8s-architecture.RU.md) - Kubernetes архитектура с
  единой точкой входа через gateway.
- [`cli-contract.RU.md`](cli-contract.RU.md) - CLI контракт и команды.
- [`cli-architecture.RU.md`](cli-architecture.RU.md) - CLI флоу для local vs
  remote и загрузки исходников.
- [`cli-auth-flow.RU.md`](cli-auth-flow.RU.md) - CLI flow для Google OIDC
  login, status, logout и token resolution для защищенных запросов.
- [`cli-auth-component-structure.RU.md`](cli-auth-component-structure.RU.md) -
  границы CLI auth packages, credential storage и владение данными.
- [`cli-component-structure.RU.md`](cli-component-structure.RU.md) - внутренняя
  структура CLI.
- [`local-engine-binary-discovery-flow.RU.md`](local-engine-binary-discovery-flow.RU.md) -
  discovery локального engine, Windows dual-engine bundle и WSL provisioning.
- [`local-engine-binary-discovery-component-structure.RU.md`](local-engine-binary-discovery-component-structure.RU.md) -
  package boundaries и владение данными для discovery engine binaries.
- [`inputset-component-structure.RU.md`](inputset-component-structure.RU.md) -
  общий CLI-side слой input set для file-bearing семантики команд.
- [`local-engine-component-structure.RU.md`][lecs] -
  внутренняя структура локального engine.
- [`prepare-job-events-flow.RU.md`][pjef] - events-first
  флоу мониторинга prepare job.
- [`prepare-job-events-component-structure.RU.md`][pjecs] -
  компонентная структура стриминга prepare events и watch-контролов.
- [`diff-component-structure.RU.md`](diff-component-structure.RU.md) - структура
  компонентов и поток вызовов для `sqlrs diff` (после контракта CLI).
- [`ref-flow.RU.md`](ref-flow.RU.md) - поток взаимодействия для bounded local
  `plan` / `prepare --ref`.
- [`ref-component-structure.RU.md`](ref-component-structure.RU.md) - внутренняя
  компонентная структура bounded local ref-backed `plan` / `prepare`.
- [`run-ref-flow.RU.md`](run-ref-flow.RU.md) - поток взаимодействия для bounded
  local standalone `run --ref`.
- [`run-ref-component-structure.RU.md`](run-ref-component-structure.RU.md) -
  внутренняя компонентная структура bounded local standalone `run --ref`.
- [`user-org-flow.RU.md`](user-org-flow.RU.md) - поток взаимодействия для
  remote-only управления пользователями и организациями.
- [`user-org-component-structure.RU.md`](user-org-component-structure.RU.md) -
  внутренняя компонентная структура remote-only управления пользователями и
  организациями.
- [`provenance-cache-flow.RU.md`](provenance-cache-flow.RU.md) - поток
  взаимодействия для `--provenance-path` и `sqlrs cache explain` в
  single-stage local prepare-oriented workflows.
- [`provenance-cache-component-structure.RU.md`](provenance-cache-component-structure.RU.md) -
  внутренняя компонентная структура baseline-среза для provenance и
  cache-explain.
- [`alias-inspection-flow.RU.md`](alias-inspection-flow.RU.md) - поток
  взаимодействия для `sqlrs alias ls` и `sqlrs alias check`.
- [`alias-inspection-component-structure.RU.md`](alias-inspection-component-structure.RU.md) -
  внутренняя компонентная структура CLI-среза alias inspection.
- [`alias-create-flow.RU.md`](alias-create-flow.RU.md) - поток
  взаимодействия для `sqlrs alias create` и discover-подсказок в виде
  copy-paste команд.
- [`alias-create-component-structure.RU.md`](alias-create-component-structure.RU.md) -
  внутренняя компонентная структура CLI-среза alias creation.
- [`discover-flow.RU.md`](discover-flow.RU.md) - поток взаимодействия для
  `sqlrs discover` с aliases-анализатором и copy-paste alias-create output.
- [`discover-component-structure.RU.md`](discover-component-structure.RU.md) -
  внутренняя компонентная структура pipeline discover.
- [`prepare-manager-refactor.RU.md`](prepare-manager-refactor.RU.md) - разбиение
  prepare manager на coordinator/executor/snapshot роли.
- [`local-engine-cli-maintainability-refactor.RU.md`][lecmr] -
  предложенный следующий проход cleanup внутренних границ prepare/httpapi/CLI.
- [`cli-maintainability-refactor.RU.md`](cli-maintainability-refactor.RU.md) -
  staged cleanup plan для текущего CLI-only maintainability долга.
- [`statefs-component-structure.RU.md`](statefs-component-structure.RU.md) - контракт
  StateFS и изоляция ФС-логики в engine.
- [`local-engine-storage-schema.RU.md`](local-engine-storage-schema.RU.md) - схема
  SQLite для локального engine.
- [`release-happy-path-e2e.RU.md`](release-happy-path-e2e.RU.md) - release-gated
  happy-path E2E flow и внутренняя структура компонентов.
- [`../api-guides/sqlrs-engine.openapi.yaml`][openAPI] - спецификация OpenAPI
  3.1 для sqlrs HTTP API. Локальный engine реализует local runtime subset;
  shared deployment также реализует remote-only endpoint-ы пользователей и
  организаций.
- [`query-analysis-workflow-review.RU.md`][qawr] -
  заметки по workflow анализа запросов.
- [`git-aware-passive.RU.md`](git-aware-passive.RU.md) - сценарии работы с git,
  инициируемые локальным CLI.
- [`m2-local-developer-experience-plan.RU.md`](m2-local-developer-experience-plan.RU.md) -
  утвержденные реализационные срезы для публичного/local M2 по developer experience.
- [`git-aware-active.RU.md`](git-aware-active.RU.md) - сценарии github-интеграции,
  требующие сервисного API.

[pjecs]: prepare-job-events-component-structure.RU.md
[lecs]: local-engine-component-structure.RU.md
[pjef]: prepare-job-events-flow.RU.md
[sda]: shared-deployment-architecture.RU.md
[openAPI]: ../api-guides/sqlrs-engine.openapi.yaml
[qawr]: query-analysis-workflow-review.RU.md
[lecmr]: local-engine-cli-maintainability-refactor.RU.md
