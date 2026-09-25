# Bootstrap remote-подключения и OIDC: матрица тестов

Статус: полностью согласована 2026-09-25 пользователем @evilguest после
критического и финального ревью. Policy resolution из ревью и output boundary
для явного `--no-browser` записаны в bootstrap ADR.

Матрица покрывает CLI-часть issue #125. Серверный bootstrap, anti-enumeration и
gateway acceptance отслеживаются в
[izess#122](https://github.com/kimak-irmagi/izess/issues/122). Тесты используют
инъецируемые clock, entropy, browser/listener, credential store, filesystem writer
и `httptest`-сервисы OIDC/sqlrs. Настоящая учётная запись Google не нужна для
приёмки.

Нормативные источники:

- [руководство remote init](../user-guides/sqlrs-init.md)
- [руководство auth](../user-guides/sqlrs-auth.md)
- [руководство users/organizations](../user-guides/sqlrs-users-orgs.md)
- [auth flow](cli-auth-flow.RU.md)
- [структура auth-компонентов](cli-auth-component-structure.RU.md)
- [user/organization flow](user-org-flow.RU.md)
- [OpenAPI contract](../api-guides/sqlrs-engine.openapi.yaml)

Трассировка семейств требований:

| Test IDs | Нормативный раздел |
| --- | --- |
| `SURF-*`, `URL-*`, `INIT-*` | Remote flags, discovery/persistence, migration и error behavior в руководстве init |
| `DISC-*`, `PROV-*` | Bootstrap/provider types и validation в auth component structure и OpenAPI |
| `OIDC-*`, `CLAIM-*` | Interactive login flow и security invariants в auth guide/flow |
| `SESS-*`, `REF-*`, `STAT-*`, `LOGOUT-*` | Token resolution, refresh, status, logout и failure table в auth guide/flow |
| `MIG-*` | RC profile/credential migration в init guide и auth component structure |
| `REC-*`, `UORG-*` | Endpoint reconciliation и authenticated onboarding в auth и user/org flows |
| `OUT-*`, `E2E-*` | Output/security invariants и три документированных onboarding journey |

Каждая строка — семейство тестов. У каждого перечисленного случая должна быть
отдельная assertion или именованная строка table-driven test. «Без изменений»
означает byte-identical workspace config и отсутствие credential-store mutation,
если строка явно не говорит обратного.

## 1. CLI surface и URL trust

| ID | Уровень / владелец | Условие и действие | Обязательные наблюдения |
| --- | --- | --- | --- |
| SURF-01 | parser unit / `internal/app` | Разобрать positional endpoint и compatibility `--url`, каждый с `--update`, `--dry-run` и выбором profile. | Обе формы дают одинаковый normalized option; token не требуется. |
| SURF-02 | parser unit / `internal/app` | Нет endpoint; positional вместе с `--url`; два positional endpoint; remote-команда с local-only flags. | Argument error, exit `64`, нет network/filesystem/store calls. |
| SURF-03 | parser + command / `internal/app` | Явный deprecated `--token`. | Выбран legacy `bearer`, bootstrap discovery не выполняется, показано deprecation guidance, сохранена прежняя compatibility config shape. |
| SURF-04 | parser/render / `internal/app`, `internal/cli` | Разобрать `auth login <provider>` и `user create` с valid и syntactically invalid provider IDs. | Provider — обязательный stable ID по `AuthProviderID`; `user create` требует `--identity-provider`; `oidc` не выводится из adapter как default provider; help показывает positional init и generic login. |
| SURF-05 | command unit / `internal/app` | Выбрать local profile для auth login/status/logout или user/org command. | Отказ до daemon lookup, engine discovery/autostart, provider/user HTTP, browser и credential-store access. |
| URL-01 | table unit / `internal/client` | Проверить HTTPS root, valid one-slug candidate, trailing slash, port и literal IPv4/IPv6 loopback HTTP. | У принятых значений одно deterministic normalized representation; root и candidate не смешиваются. |
| URL-02 | table unit / `internal/client` | Проверить non-loopback HTTP, userinfo, query, fragment, dot segments, encoded dot/slash/backslash, empty/double/additional segments, bad percent escapes и invalid slug. | Отказ до HTTP или persistence. |
| URL-03 | table unit / `internal/client` | Проверить пары control/current: root/root, root/candidate, другой scheme/authority/port, nested current, candidate control, current вне control. | Принимается только same-origin root control и root-or-one-slug current; сравнение по URL components, не string prefix. |
| URL-04 | HTTP unit / `internal/client` | Connection-info или provider discovery возвращает relative/absolute same-origin redirect, cross-origin, HTTPS-to-HTTP, loop или слишком длинную цепочку. | Следовать только явно поддерживаемым безопасным redirects; reject cross-origin/downgrade/loop/limit без передачи authorization. |
| URL-05 | table unit / `internal/client` | Проверить issuer и OIDC endpoints с HTTP, userinfo, fragment, допустимым query или collision с CLI-owned OAuth parameter. | Соблюдены разные правила issuer/endpoint; допустимый query сохранён; collision отклонён. |
| URL-06 | table unit / endpoint reconciler | Проверить canonical org endpoint на/вне control origin, root, valid slug, nested path, userinfo/query/fragment и custom domain. | Trusted только same-control-origin one-slug endpoint; к untrusted returned endpoint запрос не отправляется. |

## 2. Bootstrap discovery и remote init

| ID | Уровень / владелец | Условие и действие | Обязательные наблюдения |
| --- | --- | --- | --- |
| DISC-01 | HTTP unit / `internal/client` | Discovery с root и candidate input. | Unauthenticated `GET <exact-normalized-base>/v1/connection-info`; candidate prefix не удаляется; bearer/cookie credentials не добавляются. |
| DISC-02 | table unit / `internal/client` | Decode valid connection info, missing/empty installation ID, missing endpoints, empty providers, duplicate/invalid provider IDs и invalid cross-field URLs. | DTO возвращается только после всех invariants; empty providers — typed «interactive login unavailable»; malformed data не сохраняется. |
| DISC-03 | HTTP unit / `internal/client` | Получить provider для login. | Fixed detail path вызывается напрямую и без auth; provider collection перед ним не вызывается. |
| DISC-04 | HTTP unit / `internal/client` | Connection/provider route возвращает `304`, `404`, `405`, `429`, `5xx`, malformed JSON, wrong body/content, timeout или cancellation. | Каждый поддерживаемый ответ даёт deterministic typed/actionable error; config/store/browser неизменны. Provider `404` означает unknown/disabled; отсутствие bootstrap — upgrade error. |
| INIT-01 | command integration / `internal/app` | Fresh root init с valid discovery. | Записан remote profile с installation ID, control/current endpoint, `remoteSession`, token-env; он выбран; provider config и secret не сохранены. |
| INIT-02 | command integration / `internal/app` | Fresh valid candidate init. | Запрошен candidate bootstrap; candidate сохранён как current, root — как control; организация не объявлена verified. |
| INIT-03 | command integration / `internal/app` | `--update` workspace с local/unrelated profiles/settings и selected-profile state. | Атомарно заменена только разрешённая remote metadata; остальное сохранено; remote выбран. |
| INIT-04 | command integration / `internal/app` | Повторить тот же init; вызвать другой endpoint без `--update`. | Тот же normalized endpoint — idempotent success; другой endpoint — actionable conflict с exit `64`; без `--update` workspace не меняется. |
| INIT-05 | failure injection / `internal/app` | Fresh/update discovery, validation или config write падает на каждой boundary. | Fresh не оставляет partial workspace; update сохраняет config; credential mutation нет, кроме отдельной migration stage. |
| INIT-06 | command integration / `internal/app` | `--dry-run` для fresh, update и legacy migration. | Выполняются те же parsing/network discovery/validation; показаны intended actions; files и оба credential key не меняются. |
| INIT-07 | command integration / `internal/app` | Нет `/v1/connection-info`; provider list пуст; bootstrap/provider inconsistent (`503`). | Normal init падает с upgrade/config guidance без fallback; explicit `--token` остаётся deliberate bypass. |

## 3. Provider configuration и interactive login

| ID | Уровень / владелец | Условие и действие | Обязательные наблюдения |
| --- | --- | --- | --- |
| PROV-01 | table unit / `internal/client` | Complete v1 OIDC detail и варианты: mismatched ID, unsupported adapter/version/flow/auth method, нет `openid`, duplicate/empty scopes, false RFC 9207, нет required endpoint/client/issuer. | Только approved public-client config попадает в adapter; revocation endpoint optional. |
| PROV-02 | table unit / `internal/client` | Static authorization parameters, включая каждый reserved key и benign provider keys. | Каждый collision отклонён; benign keys сохранены точно. |
| OIDC-01 | unit / `internal/authsession` | Dispatch `oidc`, unknown adapter и unknown config version. | Generic OIDC обрабатывает первый; остальные падают до entropy/listener/browser/token/store. |
| OIDC-02 | unit / `internal/authsession` | Построить authorization URL с fixed entropy/listener, allowed endpoint query, provider params, login hint и scopes. | Query сохранён; CLI-owned параметры по одному; PKCE S256/state/nonce и advertised endpoint/client/scopes использованы. |
| OIDC-03 | failure-order unit / `internal/authsession` | Provider validation, entropy, listener или URL construction падает. | Browser/token endpoint не вызываются; existing credential не меняется. |
| OIDC-04 | table unit / callback | Matching callback, wrong/missing state, wrong/missing RFC 9207 issuer, OAuth error, missing/duplicate code, wrong host/path и второй callback. | Принят ровно один bound loopback callback; invalid cases падают до token exchange и не раскрывают callback data. |
| OIDC-05 | HTTP unit / OAuth client | Exchange code через advertised token endpoint. | POST public-client PKCE fields; нет `client_secret` и HTTP client-auth credentials; redirect не follow; transport/status/malformed/missing tokens классифицированы без сохранения session. |
| CLAIM-01 | table unit / claims | Exact issuer, nonempty `sub`, sole audience, optional `azp`, required `iat`/`exp`, expiry, future-iat boundary, nonce и optional email. | Каждый invalid claim отклоняет login; clock boundary deterministic; local signature verification не заявляется. |
| OIDC-06 | store unit / `internal/authsession` | Successful login без prior session, с prior provider/account и с atomic replacement failure. | Полная новая session сохраняется после validation; provider/account заменяется atomically; failure сохраняет прежнюю session и не рендерит success. |
| OIDC-07 | command integration / `internal/app` | `SQLRS_TOKEN` задан во время успешного interactive login. | Новая session сохранена; post-login `/v1/users/me` использует новый login ID token, не env override. |

## 4. Session lookup, refresh, status и logout

| ID | Уровень / владелец | Условие и действие | Обязательные наблюдения |
| --- | --- | --- | --- |
| SESS-01 | table unit / credential key | Менять profile, installation ID, normalized control, mutable current и equivalent control spelling. | Profile/ID/control различают keys; org switch не различает; normalized equivalents дают один key. |
| SESS-02 | unit / token resolver | `SQLRS_TOKEN`, fresh/expiring cached token, missing session, legacy bearer и unsupported mode. | Соблюдён precedence; token source передан reconciler; при env override store не читается и refresh нет. |
| REF-01 | unit / token resolver | Cached token снаружи/внутри five-minute window при fixed clock. | Reuse/refresh на точной boundary. |
| REF-02 | HTTP/store unit / token resolver | До refresh provider detail недоступен или меняет provider ID, adapter, issuer, client ID. | Требуется login; session сохранена; refresh token никуда не отправлен. |
| REF-03 | table HTTP unit / token resolver | Redirect, transport, `429`, каждый `5xx`, `invalid_grant`, другой `4xx`, malformed success, missing ID token. | Redirect не follow. Configuration version 1 удаляет session только для OAuth `invalid_grant`; остальные failures сохраняют её и возвращают retry/configuration error; invalid result не попадает в protected API. |
| REF-04 | claims/store unit / token resolver | Refresh меняет issuer/subject/audience/`azp`, ломает claims, возвращает absent/matching/mismatching nonce, rotating/missing refresh token. | Invalid claims сохраняют prior refresh credential; nonce rules соблюдены; valid ID + rotated-or-old refresh сохранены atomically. |
| REF-05 | failure injection / token resolver | Atomic storage valid rotated result падает. | Новый ID token не используется; prior local session остаётся byte-identical; сообщается, что из-за необратимой provider-side rotation может понадобиться login; ложного success нет. |
| STAT-01 | renderer/store unit | Status без/с session, expired metadata, env override, legacy profile и store error в human/JSON/verbose. | Только safe fields и override source; status не refresh-ит; raw refresh/ID/nonce/subject не печатаются. |
| LOGOUT-01 | table unit / auth session | Logout с/без revocation endpoint, `--no-revoke`, discovery failure или changed provider binding, redirect, revoke network/4xx/5xx, no session и local delete failure. | Token не отправляется при changed/untrusted config; revoke redirect не follow; revoke только когда нужен; local delete всегда attempted; вывод точно говорит, остались ли local credentials. |

## 5. Миграция RC credentials

Harness записывает порядок store/config operations и вносит отказ до/после
каждой операции. Проверяются состояния, а не заявляется симуляция настоящего
падения процесса.

| ID | Уровень / владелец | Условие и действие | Обязательные наблюдения |
| --- | --- | --- | --- |
| MIG-01 | config/store unit | Загрузить `oidcSession` и вызвать auth-команды без update. | Legacy key/fields разрешаются с warning; config/credentials не переписываются implicit. |
| MIG-02 | command unit / init | Legacy origin отличается от discovered control либо metadata недостаточна. | Legacy не копируется/удаляется; записывается только safe profile metadata; требуется новый login. |
| MIG-03 | command unit / init | Destination credential уже существует. | Он authoritative и не overwrite; approved profile metadata commit; legacy остаётся untouched, потому что эта invocation не выполняла verified copy. |
| MIG-04 | ordered failure injection / init | Destination отсутствует; success. | `Put -> read/verify -> atomic config commit -> best-effort legacy delete`; final config — `remoteSession` без legacy provider fields/secret. |
| MIG-05 | ordered failure injection / init | Destination put или verify падает. | Config и legacy неизменны; unverified destination не выбран active. |
| MIG-06 | ordered failure injection / init | Config commit падает после verified destination. | Legacy и old config сохранены; destination может остаться; ни один credential не потерян; retry safe. |
| MIG-07 | ordered failure injection / init | Legacy delete падает после config commit. | New config/destination authoritative; legacy может остаться; warning без unsafe rollback; retry idempotent. |
| MIG-08 | session/claims unit | У migrated session нет login nonce; refresh возвращает no nonce или nonce. | No-nonce token может пройти другие checks; nonce без comparison требует login и не отправляется в sqlrs. |

## 6. Endpoint reconciliation и user/org commands

| ID | Уровень / владелец | Условие и действие | Обязательные наблюдения |
| --- | --- | --- | --- |
| REC-01 | table unit / reconciler | Root profile, у user zero/one/multiple memberships. | Zero/multiple не меняют endpoint; one trusted endpoint выбирается atomically. |
| REC-02 | command integration / login | Root `/v1/users/me` возвращает `404`. | Login/session success, exit `0`, hint `user register`, switch нет. |
| REC-03 | table unit / reconciler | Candidate response возвращает тот же endpoint, другой org или malformed/untrusted endpoint. | Same bind без warning; different/untrusted не persist/contact, сохраняет login session и возвращает partial-success exit `1` с recovery. |
| REC-04 | command integration / login | Candidate current-user `404`. | Valid session сохранена; complete login stdout, recovery stderr, exit `1`. |
| REC-05 | table command / login | Reconciliation timeout/network/`5xx` или profile write failure. | Session сохранена; success stdout; recovery stderr; exit `1`; ложного switch нет. |
| REC-06 | concurrency/failure unit / config | Config изменён после snapshot до compare-before-write. | Concurrent edit сохранён; overwrite отказан; partial success + recovery. |
| REC-07 | table unit / reconciler | Token source — `StoredRemoteSession`, `EnvironmentOverride` или `LegacyBearer`. | Persist только для `StoredRemoteSession`; environment/legacy bearer не меняют config, печатают endpoint/recovery и exit `0`. |
| REC-08 | renderer unit | Successful switch в human/JSON. | Exact warning с old/new/profile/org только stderr; normal stdout полный; JSON — один document. |
| UORG-01 | command integration / register | Create `201`; create-only `412` + successful current-user read. | Created/existing result и одинаковая reconciliation matrix. |
| UORG-02 | command integration / org create | Success; remote success + local reconcile failure; token override. | Switch для stored session; partial `1` при local failure; override suppress + `0`; remote success не назван failure. |
| UORG-03 | parser/client unit / user create | Provider absent, `oidc` или explicit valid service provider ID; отправить request. | Absence — argument error; explicit value передан unchanged; adapter default не вставлен; conditional semantics сохранены. |
| UORG-04 | regression / protected commands | User/org commands после endpoint switch. | Credential lookup использует control key, HTTP — current endpoint; bearer и canonical URLs не логируются. |

## 7. Output, security canaries и end-to-end acceptance

| ID | Уровень / владелец | Условие и действие | Обязательные наблюдения |
| --- | --- | --- | --- |
| OUT-01 | table renderer/command | Уникальные canaries в refresh/ID token, code, verifier, env token и full subject; success и error families во всех output modes. | Secret canary отсутствует в stdout/stderr/errors/config/diagnostics; только documented masked/safe fields. |
| OUT-02 | command integration | Normal success, argument error, root-unregistered login, candidate/reconcile partial success и remote-write/local-update partial success. | Exact exit classes (`0`, `1`, `64`), stdout/stderr separation и один valid JSON stdout. |
| OUT-03 | command integration / login | Login в human и JSON modes с explicit `--no-browser` и без него; canaries для URL, state, nonce, PKCE challenge и verifier; затем callback и later errors. | `--no-browser` немедленно и ровно один раз пишет полный authorization URL в stderr; URL содержит state, nonce и challenge, но не verifier. Browser mode URL не печатает. URL и transient canaries не повторяются в final output, errors, verbose diagnostics или logs; stdout содержит только final result, JSON stdout — один valid document. |
| E2E-01 | fake-service acceptance | Fresh root init -> login -> root user `404` -> register без membership -> org create. | Нет manual client ID/token; session сохранена; org create переключает profile; следующий protected command использует тот же credential с новым request endpoint. |
| E2E-02 | fake-service acceptance | Second user: root init -> login -> current user с one membership. | Auto-switch на server endpoint с stderr warning и stable credential lookup. |
| E2E-03 | fake-service acceptance | Candidate init -> matching login; повтор с protected candidate `404`. | Matching bind без switch warning; nonexistent/invisible сохраняет session, exit `1` + recovery. Public anti-enumeration проверяет izess. |

## 8. Порядок исполнения и приёмки

- Сначала добавить requirement-named тесты, падающие из-за отсутствующего
  поведения. Не ослаблять unrelated regression assertions.
- После согласования матрицы повторить аудит конфликтующих тестов и получить
  явное решение до их редактирования.
- Во время разработки запускать focused packages, затем `go test ./...` в
  `frontend/cli-go`. Platform credential-store tests запускать на native OS;
  mocks не доказывают интеграцию с Windows Credential Manager, Keychain или
  Secret Service.
- Запустить `go test -race` для portable affected packages на поддерживаемой
  сборке. Ограничение toolchain/platform сообщить, а не называть обычные повторы
  race result.
- Измерить fresh per-package и aggregate statement coverage affected CLI sources:
  target 100%, minimum 95%. Проверить per-line report начиная с файлов с максимумом
  uncovered lines и согласовать дополнительный coverage plan при необходимости.
- Перегенерировать и lint-ить OpenAPI docs. Fake payloads CLI должны соответствовать
  schema, но они не заменяют izess route/gateway contract tests.

## Предварительные конфликты существующих тестов

Это данные для post-approval audit; документ пока не меняет тесты.

| Existing test | Конфликт после согласования |
| --- | --- |
| `internal/app/init_test.go: TestInitRemoteRequiresToken`, `TestInitRemoteWritesProfile` | Normal static-token expectations заменить discovery-backed `remoteSession`; сохранить отдельное deprecated `--token` compatibility coverage. |
| `internal/app/user_org_test.go: TestParseUserArgsCreateDefaultsOIDCProvider` | Default `oidc` заменить required `--identity-provider`; остальные parsing assertions сохранить. |
| `internal/app/app_auth_test.go: TestParseAuthArgs` | Вместо reject `login github` принимать любой syntactically valid provider ID; availability определяют provider detail/adapter. |
| Login fixtures и fake manager в `internal/app/auth_runner_test.go` | Не читать/передавать workspace `clientID`/`clientSecret` и hard-coded Google URL; inject provider discovery, сохранив URL-before-success/output-order assertions. |
| `internal/authsession/google_http_test.go: TestGoogleOAuthClientExchangeRefreshAndRevoke` и manager client-secret assertions | Confidential-client assertions заменить generic public OIDC requests без form `client_secret` и HTTP client auth; сохранить method/response coverage exchange/refresh/revoke. |
| Google URL/callback tests в `internal/authsession/pkce_test.go` | Строить URL из advertised endpoint/scopes/parameters и требовать RFC 9207 `iss`; сохранить PKCE/state/OAuth-error/single-callback regressions. |
| `internal/authsession/claims_test.go` и manager token fixtures | Добавить required `iat`, nonempty `sub`, sole audience и `azp`; старые valid fixtures без `iat` исправить, не ослабляя validation. |
| `internal/authsession/manager_test.go: TestResolveBearerTokenDeletesSessionOnRefreshFailure` и `manager_errors_test.go: TestResolveBearerTokenRefreshRejectsMissingOrInvalidIDToken` | Разделить taxonomy: удаляет только `invalid_grant`; transport/status/claim/malformed-token failures сохраняют prior refresh credential и не вызывают protected API. |
| Credential-store tests в `store*_test.go` и `testCredentialKey` | Key заменить на profile + installation ID + normalized control; provider/account держать в session value; все platform round-trip/error assertions сохранить. Empty-provider-to-Google fallback, если нужен, ограничить explicit legacy decoding. |
| `internal/config/config_test.go: TestAuthConfigLoadsOIDCSessionFields` | Сохранить как legacy read-compatibility evidence, но проверять read-only migration view; добавить normal `remoteSession` tests без provider metadata/secrets. |
| Fixtures в `internal/client/users_orgs_test.go` и `internal/cli/commands_user_org_test.go` | Использовать service provider ID вроде `google` вместо adapter `oidc` и required canonical `Organization.endpoint`; сохранить HTTP precondition/status/error/list/get/renderer assertions. |
| Normal-flow tests с `auth.mode: oidcSession` или string token-source | Перевести normal paths на `remoteSession` и closed `TokenSource`; dedicated legacy-alias tests для `oidcSession` сохранить. |

Согласованное решение (2026-09-25, @evilguest): обновить перечисленные старые
ожидания под утверждённый contract, сохранив unrelated regression assertions.
Не менять новую матрицу ради устаревшего RC behavior.
