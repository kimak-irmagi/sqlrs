# sqlrs auth

The CLI owns the browser, local callback and OS credential store. The selected
installation owns application client credentials and exchanges user codes and
refresh tokens with the provider. The CLI never receives client_secret.
The fixed discovery schema revision 2 replaces the old `/v1` authentication
contract; matching server/CLI versions are required, without compatibility fallback.

## Command syntax

```text
sqlrs auth providers

sqlrs auth login <provider> [--login-hint <email>] [--no-browser]
sqlrs auth status
sqlrs auth logout [--no-revoke]
```

Global flags continue to work:

```text
sqlrs --profile remote-dev auth providers
sqlrs --profile remote-dev --output json auth providers
sqlrs --profile remote-dev auth login <provider-id>
sqlrs --profile remote-dev auth status
sqlrs --profile remote-dev --output json auth status
sqlrs --profile remote-dev auth logout
```

`auth` is a remote-profile command group. Local profiles do not need remote
login because local engine requests use the local daemon token from
`engine.json`.

For example, `sqlrs --profile remote-dev auth login google`. Provider IDs select
configured instances, not protocols; the first adapter is `oidc`.

`sqlrs auth providers` fetches the current public provider list from the
selected profile endpoint. It works even if the output from `init remote` was
lost, does not require a stored login, and does not change workspace config.
Use global `--profile` to select another profile or `--endpoint` to query a
different endpoint for this invocation only. The result is a list of login
choices, not proof of organization membership. If no providers are enabled,
the command reports that condition instead of suggesting a provider.

## Remote profile and provider discovery

Initialize a remote profile with `sqlrs init remote <endpoint>`. The remoteSession
profile retains stable installation ID, trusted control endpoint and current
organization endpoint. It contains no provider secret or user tokens.
`SQLRS_TOKEN`, when set, remains the explicit highest-priority bearer override.

```yaml
profiles:
  remote-dev:
    mode: remote
    installationID: taidon-production
    installationEndpoint: "https://api.taidon.dev"
    endpoint: "https://api.taidon.dev/nsu"
    auth:
      mode: remoteSession
      tokenEnv: SQLRS_TOKEN
```

Rules:

- `auth.mode: remoteSession` tells protected commands to use the active local
  session for one service-advertised provider.
- `installationID` and `installationEndpoint` remain stable when the profile's
  request `endpoint` changes from the installation root to an organization URL.
- Provider choice is user-local session state. It is not committed to workspace
  config.
- `GET <base>/v1/auth/providers` lists the providers enabled by the selected
  installation.
- `GET <base>/v1/auth/providers/<provider>` returns the current, versioned
  configuration consumed by the corresponding CLI adapter.
- Both provider routes and `GET <base>/v1/connection-info` are public before
  login and are available at the installation root and below every candidate
  organization prefix, including a nonexistent slug. Bootstrap success does
  not confirm organization existence.
- `auth.tokenEnv` defaults to `SQLRS_TOKEN`; when that environment variable is
  set, it overrides the stored session for the active provider.
- Existing `auth.mode: bearer` profiles keep their current behavior and read a
  caller-supplied bearer token from `tokenEnv` or `token`.

Provider configuration may contain public OAuth/OIDC metadata such as client
ID, issuer, authorization endpoint, token endpoint, scopes, token endpoint
authentication method, and permitted authorization parameters. The generic
OIDC adapter supports Authorization Code with PKCE S256, a loopback redirect,
ID and refresh tokens, and token endpoint authentication method `none`. The
configuration must never expose a confidential client secret or user token. Do
not store provider client configuration, refresh tokens, or raw ID tokens in
`.sqlrs/config.yaml`.

Service bootstrap bases use HTTPS, except development HTTP on literal loopback,
and contain no userinfo, query, fragment, dot segments, or encoded separators.
OIDC issuer and provider endpoints use HTTPS and contain no userinfo or
fragment; the issuer also contains no query. Existing endpoint query parameters
must not duplicate parameters owned by the CLI. URL joining is structural, and
bootstrap redirects may not cross origin or downgrade. The CLI rejects invalid
configuration before opening a browser or sending a token request.

Public `/v1/connection-info` and `/v1/auth/providers/{provider}` work at the root
and valid candidate organization prefixes without disclosing organization
existence. Explicit login does not require a successful provider-list request.
Detail must match the requested ID and the supported revision-2 OIDC capabilities.
It exposes authorization metadata and installation exchange/refresh URLs, without
an upstream token endpoint, client-authentication method or secret. Operation URLs
must remain under the trusted installation control origin/base. Unsafe parameters
or incompatible metadata fail before a browser opens.

## Google OAuth client

The installation operator configures an independent Google **Desktop** client ID
and secret on the server. It must permit PKCE S256, a random IPv4 loopback callback,
response issuer identification, RS256 ID tokens and refresh returning a fresh ID
token. Configure `openid email profile` and Google's `access_type=offline` and
`prompt=consent` explicitly on that provider instance. No Google defaults are
inherited by other instances. Verify the actual provider/client capabilities;
sharing a brand or issuer alone does not establish compatibility.

The secret remains in protected server configuration. Users do not enter it in
CLI options, profiles or authorization URLs. A provider needing a different
protocol/client-authentication method needs a corresponding server adapter.

## `sqlrs auth login google`

1. Fetch and validate the selected provider detail and trusted operation URLs.
2. Generate state, nonce and PKCE S256 material; start a temporary listener at
   `http://127.0.0.1:<random-port>/` and construct the configured authorization URL.
3. Open the browser. With `--no-browser`, print the URL immediately to stderr for
   manual opening. The URL includes public client ID and attempt parameters only.
4. Receive the authorization code locally; reject callback errors or mismatched
   state/issuer. Send code, exact redirect URI, verifier, nonce and expected
   installation/issuer/client binding to the installation exchange endpoint.
5. The server uses its client secret for provider exchange and verifies the signed
   result before returning it synchronously, without storing a user session.
6. The CLI checks exact issuer, sole audience, optional matching azp, subject,
   nonce and exp strictly in the future. Login requires both ID and refresh tokens.
   Access tokens cannot replace ID tokens. Only then atomically replace the local
   OS credential. Failed login preserves the previous session.
7. Run the existing organization endpoint reconciliation described below.

### Organization endpoint reconciliation

After a successful login, the CLI asks the profile's current request endpoint
for the current registered user. For an installation-root profile with exactly
one organization membership, it atomically changes the request endpoint to the
canonical organization endpoint returned by the service. It preserves
`installationID` and `installationEndpoint`, so provider discovery and
credential lookup remain stable.

For a candidate-prefixed profile, this protected request is the first existence
and access check. A successful matching response binds authenticated
organization metadata without changing the URL or printing a switch warning. A
`404` leaves the valid login session intact and reports how to reinitialize the
profile with the installation root or a correct organization URL.

If the authenticated response instead names another organization endpoint or
an invalid/untrusted canonical endpoint, the CLI never persists or contacts
that endpoint. It retains the successful login session, returns partial-success
exit `1`, and prints routing recovery on stderr.

At the installation root, a current-user `404` instead means that authentication
succeeded but this identity has not registered a sqlrs user yet. Login succeeds
and suggests `sqlrs user register`.

The CLI never constructs an organization endpoint from a slug. A returned
canonical endpoint is accepted only when it has the installation control
origin, exactly one valid organization-slug segment, and no userinfo, query, or
fragment. A future custom-domain feature requires an explicit allowlist; an
untrusted endpoint is never persisted or sent a bearer token. If there are no
memberships the CLI leaves the bootstrap profile unchanged. If there are
multiple memberships it does not choose one implicitly. An explicitly supplied
organization endpoint is not silently replaced by a different organization.

After a successful automatic change, the CLI writes this warning to stderr in
both human and JSON output modes:

```text
warning: profile "remote" switched to organization "nsu": https://api.taidon.dev -> https://api.taidon.dev/nsu
```

Root login followed by current-user `404` exits successfully because the user
can continue with registration. A valid candidate login also exits
successfully. Candidate `404`, reconciliation network/5xx failure, or a failed
local profile update returns partial-success exit code `1` while retaining the
valid session. Human and JSON modes emit the successful login result to stdout,
then the diagnostic and recovery command to stderr; JSON stdout remains one
valid document. The CLI never claims that the profile was switched when its
atomic update failed.

## Token selection for protected API commands

The CLI uses an explicit SQLRS_TOKEN override first. Otherwise it uses a complete
installation-scoped local session. An eligible cached ID token is reused; a token
missing or due for refresh triggers anonymous current provider discovery and a
request to the installation refresh endpoint. The request includes the stored
subject/nonce and expected installation/issuer/client binding. Changed trust
requires login before submitting the refresh credential. No browser opens.

A valid result is independently checked before storing a rotated refresh token.
Omission retains the submitted token; an explicit empty replacement is invalid.
Tokens expiring in less than five minutes remain acceptable: five minutes is the
refresh-ahead threshold, not a minimum token lifetime.

Every failed refresh, including invalid_grant, cancellation, lost response and
invalid claims, leaves the current credential untouched. It performs no deletion,
write or restoration of a previously read value, because another process may
have saved a successful refresh. There is no cross-process refresh lock. The
command reports a safe error and the user chooses whether to retry or log in.
Explicit logout remains the operation that deletes local credentials.

Exchange/refresh POSTs do not redirect or retry automatically. They never attach
an unrelated cached bearer. Responses are limited to 1 MiB and the command timeout
applies (default 30 seconds). Incomplete legacy sessions require a fresh login;
old discovery or missing operations never enable direct-provider exchange.

## `sqlrs auth status`

Shows whether the selected profile has a stored provider session. It does
not contact the provider or prove that a refresh token has not been revoked.

```text
sqlrs auth status
```

Human output fields:

```text
status: logged in
provider: google
email: alice@example.com
issuer: https://accounts.google.com
audience: 1234567890-abcdef.apps.googleusercontent.com
tokenExpiry: 2026-07-01T12:45:00Z
profile: remote-dev
endpoint: https://sqlrs.example.org
override: none
```

If `SQLRS_TOKEN` is set, status must make the override visible:

```text
override: SQLRS_TOKEN
```

`auth status` never prints raw tokens. In verbose mode it may print only a safe
claim summary: `iss`, `aud`, masked `sub`, `email`, and `exp`.

JSON output uses the global `--output json` flag and must not include raw
tokens:

```json
{
  "status": "logged_in",
  "provider": "google",
  "email": "alice@example.com",
  "issuer": "https://accounts.google.com",
  "audience": "1234567890-abcdef.apps.googleusercontent.com",
  "token_expiry": "2026-07-01T12:45:00Z",
  "profile": "remote-dev",
  "endpoint": "https://sqlrs.example.org",
  "override": null
}
```

## `sqlrs auth logout`

Deletes the selected profile's local provider session.

```text
sqlrs auth logout [--no-revoke]
```

Default behavior:

1. Load the refresh token from the OS credential store.
2. Fetch the active provider's current configuration and attempt revocation
   when it advertises a revocation endpoint.
3. Delete the local credential store entry and cached ID token even if
   provider discovery or revocation fails. Token and revocation POSTs do not
   follow redirects.
4. Leave `SQLRS_TOKEN` untouched.

`--no-revoke` skips the provider revocation request and only deletes local
credentials.

Human output:

```text
logged out
provider: google
profile: remote-dev
revoked: true
```

## Credential storage

The CLI stores refresh tokens only in the OS credential store:

- Windows: Windows Credential Manager.
- macOS: Keychain.
- Linux: Secret Service/libsecret.

The active-session credential lookup is scoped to:

- sqlrs application name;
- selected profile name;
- stable installation ID;
- normalized installation control endpoint.

The credential value may contain a JSON session record with:

- refresh token;
- optional cached ID token;
- login nonce used only to validate a nonce returned during refresh;
- token expiry;
- issuer;
- audience/client ID;
- subject as credential metadata;
- email;
- granted scopes;
- creation and update timestamps.

A successful `sqlrs auth login <provider>` atomically replaces the one active
session for the selected profile and installation, including when the provider
or account changes. Provider, issuer, client ID, and subject live in the session
value, so status and protected commands can locate it after restart. A tenant
endpoint switch does not change the credential key or require another login.

If Linux Secret Service/libsecret is unavailable, `auth login google` fails with
a clear setup error. The CLI does not fall back to storing refresh tokens in
plain text. `SQLRS_TOKEN` remains available for smoke/debug runs.

## Troubleshooting

- For invalid_grant or changed provider trust, run `sqlrs auth login <provider>`.
  A failed refresh retains the existing record; do not assume it was deleted.
- A missing refresh token usually requires checking the configured client type,
  scopes and consent parameters. Provider-specific parameters are server settings.
- An invalid audience means the returned ID token targets a different client ID.
  Every installation must use its own registration and trust configuration.
- On Linux, install/configure Secret Service/libsecret (for example through the
  desktop keyring). An unavailable store produces a setup error without plaintext
  fallback. `SQLRS_TOKEN` remains an explicit debugging mechanism.
- If the browser callback cannot reach the local listener, check local networking
  and try `--no-browser`. Device code flow is not implemented as a fallback.

See the [interaction flow](../architecture/cli-auth-flow.md),
[component structure](../architecture/cli-auth-component-structure.md), and
[OpenAPI](../api-guides/sqlrs-engine.openapi.yaml).
