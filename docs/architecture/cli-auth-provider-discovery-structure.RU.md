# Структура компонентов обнаружения провайдеров входа в CLI

Статус: структура компонентов утверждена владельцем продукта 2026-10-23.
Согласованные синтаксис и поток описаны в
[`../user-guides/sqlrs-auth-providers-proposal.md`](../user-guides/sqlrs-auth-providers-proposal.md)
и [`cli-auth-provider-discovery-flow.RU.md`](cli-auth-provider-discovery-flow.RU.md).

## Deployment units и владение

Меняется только `frontend/cli-go`. Повторно используются существующие endpoints
`GET /v1/connection-info` и `GET /v1/auth/providers`. Gateway, local engine,
база данных и credential store не получают новых обязанностей.

| Модуль | Ответственность |
| --- | --- |
| `internal/client` | Декодировать и проверять публичный `AuthProviderList{Providers []AuthProviderSummary}` с выбранного endpoint через существующий unauthenticated transport и безопасные redirects. Повторно использовать проверку provider summary и предоставить `GetAuthProviders(ctx)` без доступа к session/token. |
| `internal/app` init | После token-free discovery и успешной записи config вывести уже проверенный `ConnectionInfo.AuthProviders`. Не делать второй запрос и не печатать подсказку об успешном подключении до записи. Legacy bearer init и dry-run не выдают session за готовую. |
| `internal/app` auth | Разобрать `auth providers`, разрешить выбранный профиль и output mode, отвергнуть local/некорректный remote profile, вызвать `GetAuthProviders` и вывести детерминированный human или публичный JSON. Добавить подсказку к ошибке login без провайдера. Явный login/session не менять. |
| `internal/cli` | Сохранить существующий разбор глобальных `--profile` и `--output`. Нового глобального флага и persistent state нет. |
| `internal/config` | По-прежнему хранить только выбранный endpoint и stable installation metadata. Каталог и выбор провайдера не записываются в `.sqlrs/config.yaml`. |

`AuthProviderSummary` уже существует в `internal/client` с `ID`,
`DisplayName` и `Adapter`. Новый `AuthProviderList` — wire DTO, а не
сохраняемая модель. Общий validator должен применять одинаковые правила ID,
непустого display name/adapter и уникальности к connection-info и отдельному
маршруту списка. Пустой список допустим структурно для API, но команда
возвращает отдельную понятную ошибку «нет включённых провайдеров».

## Поток данных и инварианты

1. Effective `cmdCtx.profile.Endpoint` — единственный источник base URL для
   повторного запроса. Глобальный `--endpoint` может временно переопределить
   выбранный `--profile` только на этот вызов. Используется существующий
   контракт нормализованного root-or-one-slug endpoint; installation root и
   жёстко заданный URL не подставляются.
2. `client.GetAuthProviders` использует существующий публичный HTTP transport.
   Запрос без auth headers/cookies; ответ ограничен 1 MiB до декодирования, JSON
   проверяется до возврата списка. OAuth endpoints не вызываются.
3. Renderer сортирует копию по ID, не меняя wire response и профиль. Human
   output содержит ID/имя и короткую подсказку для входа, обезвреживая
   терминальные управляющие символы в именах от сервера. JSON содержит только
   публичный массив `providers`, без деталей или секретов.
4. Команда не читает credential store, не открывает браузер, не запускает engine,
   не пишет workspace и не выбирает провайдера. Login по-прежнему напрямую
   получает детали выбранного провайдера; прежний список не авторизует вход.
5. Сбой транспорта, некорректный JSON/summary и пустой список различаются.
   Кэшированный, настроенный или жёстко заданный провайдер не подставляется.

## Совместимость

Синтаксис и поведение `auth login <provider>`, `auth status`, `auth logout` не
меняются. `auth providers` добавляется отдельно. Новый human output init
аддитивен; машинному клиенту следует использовать выделенную JSON-команду.
Изменения OpenAPI и схемы БД не нужны.
