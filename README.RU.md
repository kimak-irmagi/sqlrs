# Taidon

[![CI](https://github.com/kimak-irmagi/sqlrs/actions/workflows/ci.yml/badge.svg?branch=main)](https://github.com/kimak-irmagi/sqlrs/actions/workflows/ci.yml)
[![release-local](https://github.com/kimak-irmagi/sqlrs/actions/workflows/release-local.yml/badge.svg)](https://github.com/kimak-irmagi/sqlrs/actions/workflows/release-local.yml)
[![Coverage Status](https://coveralls.io/repos/github/kimak-irmagi/sqlrs/badge.svg?branch=main)](https://coveralls.io/github/kimak-irmagi/sqlrs?branch=main)

Taidon — это open-source платформа для безопасных и воспроизводимых SQL‑экспериментов.  
Она предоставляет изолированные окружения, быстрые снапшотные базы и единый API для выполнения SQL‑нагрузок без побочных эффектов.

Этот репозиторий — **монорепозиторий**, содержащий все компоненты платформы:
frontend‑приложения, backend‑микросервисы, общие библиотеки, исследовательские материалы и документацию.

---

## Обзор

Taidon предназначен для:

- обучения SQL;
- прототипирования инструментов, связанных с БД;
- исследований в области выполнения запросов и поведения БД;
- воспроизводимых окружений для демонстраций и воркшопов.

Ключевые идеи:

- **Воспроизводимость** — каждая SQL‑сессия стартует из известного снапшота.
- **Изоляция** — действия пользователя не влияют на другие сессии.
- **Скорость** — окружения запускаются быстро и масштабируются горизонтально.
- **Расширяемость** — несколько DB‑бекендов, плагины, внутренние и внешние интеграции.

Платформа со временем будет включать:

- web UI,
- набор микросервисов для выполнения SQL,
- оркестрацию снапшотов и окружений,
- исследовательские датасеты и эксперименты производительности,
- CLI‑инструменты для разработчиков.

---

## Структура проекта

```plain
taidon/
  README.md
  CONTRIBUTING.md
  CODE_OF_CONDUCT.md
  LICENSE

  docs/                # Архитектура, дизайн‑доки, ADR, user guides
  research/            # LaTeX‑статьи, бенчмарки, датасеты, эксперименты

  frontend/
    main/              # Основное SPA приложение (React)
    editor/            # Компонент редактора запросов
    result-viewer/     # Табличный просмотр результатов
    plan-viewer/       # Визуализация плана запроса
    ...                # Прочие UI‑модули и shared libs

  backend/
    gateway/           # API gateway (BFF) для фронтенда
    services/
      vcs-sync/        # Интеграция с VCS / локальной ФС
      sql-runner/      # Быстрое выполнение цепочек SQL
      env-manager/     # Оркестрация БД‑контейнеров + снапшоты
      snapshot-cache/  # Жизненный цикл снапшотов и warm cache
      user-profile/    # Пользователи, организации, роли, квоты
      idp/             # Аутентификация / identity provider
      audit-log/       # Логирование действий/событий
      telemetry/       # Метрики и usage‑данные
      scheduler/       # Фоновые задачи (cleanup, prewarm и т.д.)
      # billing/       # (опционально) биллинг и квоты
    libs/              # Общие backend‑библиотеки (типы, utils, клиенты)

  infra/
    docker/            # Dockerfile‑ы для сервисов и БД
    k8s/               # Kubernetes‑манифесты / Helm charts
    terraform/         # Cloud‑инфраструктура
    local-dev/         # docker‑compose для локальной разработки

  scripts/
    dev/               # Скрипты для разработки
    maintenance/       # Миграции БД, инструменты обслуживания

  examples/
    sql/
    api/
    scenarios/
```

---

## Подпроекты

### **Frontend**

Расположен в `frontend/`.  
Основное приложение — `frontend/main`, отдельные компоненты (editor, viewers, widgets) — в соседних директориях.

У каждого подпроекта есть свой `README.md` с инструкциями.

---

### **Backend**

Backend‑сервисы расположены в `backend/services/`, а общий фасад — в `backend/gateway/`.

Общие библиотеки, API‑контракты и утилиты — в `backend/libs/`.

`backend/libs/runtime-go` — независимо версионируемый публичный Go-модуль с
семантическим контрактом Runtime v2. Опубликованный `v0.2.0` — неизменяемый
legacy-v2 contract с discriminator `sqlrs.runtime.v2`; его fingerprints и
StateIDs никогда не переосмысливаются. Persistence adapter #110 добавляет opaque,
v0.2-wire-compatible cache-record API без изменения этих identity. PR #135
завершил #130/#131, добавив рядом контракт
`sqlrs.runtime.v2.canonical.v1`. Additive release `v0.3.0` опубликован из commit
`6e60578c` после успешных same-commit RC/GA conformance и public-proxy gates.
Опубликованный `v0.1.0` остаётся superseded и не должен использоваться в новых
зависимостях.

Опубликованный v0.4.0 добавляет фиксированный внешний фасад проверки
canonical-v1. Опубликованный v0.5.0 переводит существующий результат resolver на
`CanonicalResolvedExtensionIdentity` и вводит новую версию cache record без
преобразования старых записей.

Чтение YAML, рекурсивное раскрытие псевдонимов и совместимость со старыми
псевдонимами остаются в CLI. При будущем подключении Runtime v2 движок получит
раскрытое объявление и необходимые исходные данные. Предложенный выпуск
`aliasruntimev2` [отменён](https://github.com/kimak-irmagi/sqlrs/issues/140);
см. [порядок композиции](docs/architecture/runtime-v2-composition-flow.RU.md).

У каждого сервиса есть своя документация и инструменты.

---

### **Документация**

Архитектура, спецификации, ADR и дизайн‑заметки находятся в `docs/`.

---

### **Исследования**

Экспериментальные данные, бенчмарки, LaTeX‑статьи и ноутбуки находятся в `research/`.

Этот раздел поддерживает разработку стратегий снапшотов, моделей выполнения SQL и анализа производительности.

---

## Contributing

Мы приветствуем вклад студентов, волонтёров и профессионалов.

Пожалуйста, ознакомьтесь с гайдлайнами:

- **[CONTRIBUTING.RU.md](./CONTRIBUTING.RU.md)**

---

## Code of Conduct

Мы стремимся к дружелюбной и инклюзивной среде.

Пожалуйста, ознакомьтесь с Code of Conduct:

- **[CODE_OF_CONDUCT.RU.md](./CODE_OF_CONDUCT.RU.md)**

---

## License

Проект распространяется под лицензией **Apache License 2.0**.

Полный текст:

- **[LICENSE](./LICENSE)**

---
