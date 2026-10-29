# CLI authentication component structure

The [flow](cli-auth-flow.md) and [OpenAPI](../api-guides/sqlrs-engine.openapi.yaml)
define one installation-exchange contract, discovery schema revision 2. This
replaces the former direct-provider exchange design without compatibility fallback.

## Deployment units and ownership

| Unit/module | Responsibility |
| --- | --- |
| CLI `internal/app/auth.go` | Resolve profiles, validate public discovery and trusted operation URLs, pass command timeouts, reconcile organization endpoints, render safe output. |
| CLI `internal/app/auth_exchange.go` | Implement the domain TokenExchanger port using installation HTTP operations; map safe API refusals. |
| CLI `internal/client` | Public discovery DTOs and anonymous bounded exchange/refresh JSON requests; no unrelated Authorization header, redirect or automatic replay. |
| CLI `internal/authsession` | State/nonce/PKCE, local browser/callback, identity checks, cached bearer selection, refresh decisions and OS credential writes. |
| Server | Own application client credentials, provider HTTP adapters and signature/JWKS verification. Return token results synchronously; no persistent user-token state. |
| Local engine | Retain its independent local authentication; no external provider exchange responsibility. |

## Ports and data types

`TokenExchanger.ExchangeCode` and `.Refresh` receive a `LoginProvider` containing
an `InstallationBinding` (installation ID, control endpoint, provider ID, issuer,
client ID), operation URLs and timeout. Requests contain only user/attempt artifacts:
code/verifier/redirect/nonce, or refresh token/original subject/nonce.
`TokenResponse` contains ID token, optional rotated refresh token and optional
access token. No domain/request/store type contains the application client secret.

`ProviderRevoker` is a separate optional capability for direct secret-free
revocation. BrowserOpener, LoopbackProvider, Clock, random source and CredentialStore
remain injectable seams. The production manager always receives an explicit
installation exchanger; it cannot construct a direct upstream exchanger.

Public provider detail owns display data and protocol metadata only. It exposes
installation exchange/refresh URLs, not upstream token endpoints or client-auth
method. `configuration_version: 2` is a fixed schema marker, not negotiation.
The supported adapter is OIDC authorizationCodePKCE. Authorization URL parameters
owned by the CLI cannot be overridden by endpoint query or configuration.

## Persistent and in-memory state

Workspace configuration owns profile/routing information and auth mode; it stores
no provider secret, refresh token or ID token. An OS credential is keyed by profile,
stable installation ID and normalized control base, independent of organization
route, provider and account. All platforms must use the same address for reads,
writes and deletion. Provider, issuer, client ID, subject, nonce, token expiry and
user tokens belong to the credential value. A successful login replaces that one
active session atomically. There is no separate active-provider pointer.

One invocation owns discovery snapshots, PKCE verifier/state, callback listener,
nonce and transient response data. The server owns its immutable configuration and
partitioned JWKS caches; it never persists user refresh tokens. No schema migration
or server-side token-pickup mechanism is introduced.

## Validation and failure invariants

The CLI validates signed-result claim structure and original identity; the server
validates signatures, exact issuer/client audience and operation constraints.
Both require exp strictly in the future for returned token results. The CLI
refresh-ahead window does not reject short-lived valid tokens.

Only successful login/refresh may Put credentials. Every failed refresh retains
the current store record without Delete, Put or restoring an earlier read, even
when another process succeeds concurrently. No cross-process refresh lock is
required. Explicit logout retains its independent deletion semantics.

Legacy configuration readers may still support init's explicit profile update,
but incomplete credentials require fresh login. They cannot enable schema 1,
direct exchange, secret prompts or a fallback token endpoint. Failed discovery
requires an actionable configuration/server upgrade error.

`SQLRS_TOKEN` remains an explicit override. Auth status reads safe metadata only.
Raw user/provider credentials and upstream error descriptions are not rendered.
HTTP token responses are bounded to 1 MiB, operation timeout defaults to 30 seconds,
and POST redirects/retries are forbidden. See the [guide](../user-guides/sqlrs-auth.md)
for syntax, local storage setup and routing outcomes.
