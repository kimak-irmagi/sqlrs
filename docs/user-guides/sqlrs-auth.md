# sqlrs auth

## Overview

`sqlrs auth` manages local CLI authentication sessions for shared or cloud
deployments. A remote installation advertises its enabled login providers; the
first configured provider is Google, using the generic OIDC adapter.

The server and gateway continue to accept only short-lived Google ID tokens as
`Authorization: Bearer <id-token>`. Refresh tokens are client-only secrets. The
CLI stores them in the operating-system credential store, refreshes ID tokens
locally, and never sends refresh tokens to any sqlrs API.

Manual bearer-token use through `SQLRS_TOKEN` remains a debug and smoke-test
override. If `SQLRS_TOKEN` is set, protected API commands use it before any
stored auth session.

## Command syntax

```text
sqlrs auth login google [--login-hint <email>] [--no-browser]
sqlrs auth status
sqlrs auth logout [--no-revoke]
```

Global flags continue to work:

```text
sqlrs --profile remote-dev auth login google
sqlrs --profile remote-dev auth status
sqlrs --profile remote-dev --output json auth status
sqlrs --profile remote-dev auth logout
```

`auth` is a remote-profile command group. Local profiles do not need remote
login because local engine requests use the local daemon token from
`engine.json`.

## Remote profile and provider discovery

A normal remote profile stores stable installation and routing metadata, not
provider-specific login configuration:

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
  set, it overrides the stored Google session.
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

## Google OAuth client

The shared-deployment operator creates and configures the Google OAuth client:

1. In Google Cloud Console, create an OAuth client with application type
   `Desktop app`.
2. Publish the non-secret client ID and provider endpoints through the
   installation's Google provider configuration route.
3. Configure the sqlrs gateway to accept Google ID tokens with:
   - issuer: `https://accounts.google.com`;
   - audience: the same Google client ID.

The CLI uses a loopback redirect URI at runtime:

```text
http://127.0.0.1:<random-port>
```

The random port is selected for each login. Do not configure a fixed redirect
port in sqlrs config. A provider that requires a confidential client secret is
not compatible with this direct native-CLI flow; no such secret may be returned
by the public provider API.

## `sqlrs auth login google`

Logs the selected remote profile into Google and stores a refresh-capable local
session.

```text
sqlrs auth login google [--login-hint <email>] [--no-browser]
```

Flow:

1. Validate that the selected profile is `mode: remote` and
   `auth.mode: remoteSession`.
2. Request the current Google OIDC configuration from
   `<profile endpoint>/v1/auth/providers/google`. The same installation-owned
   configuration is available below root, existing-organization, and
   nonexistent candidate prefixes without disclosing organization existence.
3. Require the provider response to contain the public client ID, issuer,
   authorization endpoint (the login-form base URL), token endpoint, scopes,
   token endpoint authentication method `none`, RFC 9207 authorization-response
   issuer support, an `openid` scope, flow identifier, and configuration version
   expected by the OIDC adapter. The returned provider ID must match the
   requested provider path. The provider collection is not a prerequisite for
   login; an unknown or disabled provider returns `404`.
4. Generate:
   - a high-entropy PKCE verifier;
   - a SHA-256 PKCE challenge using method `S256`;
   - a high-entropy `state`;
   - a high-entropy OIDC `nonce`.
5. Start an HTTP listener on `127.0.0.1:<random-port>`.
6. Build the Google authorization URL from the advertised
   `authorizationEndpoint`; the CLI does not hard-code the Google login URL.
   Add:
   - `response_type=code`;
   - `client_id=<provider client ID>`;
   - `redirect_uri=http://127.0.0.1:<random-port>`;
   - `scope=<provider-advertised scopes>`;
   - `code_challenge=<challenge>`;
   - `code_challenge_method=S256`;
   - `state=<state>`;
   - `nonce=<nonce>`;
   - provider-advertised static authorization parameters. For Google these are
     `access_type=offline` and `prompt=consent`.
7. Open the system browser unless `--no-browser` is set. With explicit
   `--no-browser`, immediately print the authorization URL to stderr for the
   user to open manually. Browser mode does not print the URL.
8. Accept exactly one callback on the loopback listener.
9. Reject the callback if:
   - `state` does not match;
   - RFC 9207 `iss` is missing or does not exactly match the issuer bound to
     this login attempt;
   - Google returns `error`;
   - `code` is missing.
10. Exchange the authorization code at the advertised token endpoint using the
    PKCE verifier.
11. Require an ID token and refresh token in the token response.
12. Decode the ID token claims for local validation and metadata:
    - `iss` must match the configured issuer;
    - `sub` must be present and non-empty;
    - configuration version 1 requires the client ID to be the sole `aud`;
    - `azp`, when present, must match the advertised client ID;
    - `exp` and `iat` must both be present;
    - `exp` must be in the future;
    - `iat` must not be more than five minutes in the future;
    - `nonce` must match the login nonce;
    - `email` is captured for status output when present.
13. Store the refresh token and optional cached ID token in the OS credential
    store.
14. Store only safe metadata for status and diagnostics and make `google` the
    active provider for this profile.
15. Using the new ID token from this login, ask the profile's current request
    API for the authenticated current user. `SQLRS_TOKEN` does not override this
    post-login reconciliation request.
    This either validates and binds a candidate-prefixed endpoint or reconciles
    an installation-root profile to its one unambiguous organization endpoint.

The manual authorization URL is the sole output exception for login-attempt
values: it necessarily contains `state`, `nonce`, and the PKCE challenge. It
must never contain the PKCE verifier and must not be repeated in final human or
JSON output, errors, verbose diagnostics, or logs. Authorization codes, PKCE
verifiers, refresh tokens, and ID tokens are never printed. Stdout remains
reserved for the final command result; in JSON mode it contains exactly one
valid JSON document.

Human success output:

```text
logged in
provider: google
email: alice@example.com
issuer: https://accounts.google.com
audience: 1234567890-abcdef.apps.googleusercontent.com
profile: remote-dev
endpoint: https://sqlrs.example.org
```

The final result never includes the manual authorization URL.

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

Protected remote API commands resolve a bearer token in this order:

1. If `SQLRS_TOKEN` is set, use it exactly as the bearer token source.
2. If the selected profile uses `auth.mode: remoteSession`, load its active
   provider session from the OS credential store.
3. If the selected profile uses the legacy `auth.mode: bearer`, keep the current
   `tokenEnv` or `token` behavior.
4. If no token source is available, fail before making the protected API
   request.

For Google sessions, the CLI decodes the cached ID token locally and checks
`exp` before each protected API request. If the token is missing, expired, or
will expire within five minutes, the CLI reads the current Google provider
configuration, refreshes through its advertised token endpoint, and then sends
only the fresh ID token as:

```text
Authorization: Bearer <id-token>
```

Before sending the refresh token, the CLI requires the current provider detail
to retain the session's provider ID, adapter, issuer, and client ID; otherwise
it requires a new login. Token and revocation POST requests never follow HTTP
redirects.

The refreshed ID token must have a non-empty subject, the exact issuer, sole
client-ID audience, valid `azp`, required `iat` and `exp`, `iat` no more than
five minutes in the future, a future expiry, and the same subject as the stored
session. Nonce is not required on refresh, but a returned nonce must match the
login nonce retained in the credential. If the token response
rotates the refresh token, the CLI atomically stores the replacement together
with the new ID token; the provider remains responsible for enforcing rotation
or sender-constraining policy for this public client.

Transient network failures, `429`, and `5xx` retain the session for retry and
report that the provider is temporarily unavailable. In configuration version
1, only the OAuth error `invalid_grant` deletes the session and asks for login;
all other `4xx` responses retain it and report a request/provider configuration
error. Provider-specific definitive-rejection codes require a future
configuration version. Invalid returned claims retain the refresh credential
for a later retry but are never sent to a sqlrs API. If refresh fails or the credential store cannot be read, the CLI
must not send the stale ID token. A definitive session failure prints:

```text
Google auth session is expired or unavailable; run `sqlrs auth login google`.
```

## `sqlrs auth status`

Shows whether the selected profile has a stored Google auth session. It does
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

Deletes the selected profile's local Google auth session.

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

`--no-revoke` skips the Google revocation request and only deletes local
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

### Token expired

Protected commands refresh cached ID tokens automatically. If a command still
reports an expired or unavailable session, run:

```text
sqlrs auth login google
```

### Refresh token not returned

The login URL includes `access_type=offline` and `prompt=consent` so Google can
return a refresh token. If the token response still lacks `refresh_token`, check
that the OAuth client is a Desktop app client, the scopes are exactly
`openid email profile`, and the user completed consent for the selected Google
account.

### Provider requires a confidential client secret

The public provider API and native CLI must not distribute a confidential
client secret. Configure a public/native-client flow that supports PKCE or use a
separately designed server-side auth broker; do not put the secret in workspace
config.

### Invalid audience

The ID token `aud` claim is the Google OAuth client ID used by the CLI.
Configure the gateway to trust the exact issuer/client-ID pair before the
service advertises the provider. Configuration version 1 requires it to be the
sole audience; `azp`, when present, must equal the client ID. Inconsistent
provider and gateway trust configuration
makes bootstrap/provider discovery unavailable with `503`.

### Credential store unavailable

On Linux, start or install a Secret Service provider such as GNOME Keyring or
KWallet with libsecret support. The CLI must not store refresh tokens in
`.sqlrs/config.yaml` as a fallback.

### Browser callback does not complete

Retry login. If the browser cannot be opened automatically, use:

```text
sqlrs auth login google --no-browser
```

Open the printed URL manually in a browser on the same machine where the CLI is
listening.

## Device code fallback

The first design target is the loopback redirect flow because sqlrs CLI runs on
developer workstations with a browser. Device code flow remains a fallback only
if loopback login proves impractical. Before switching, verify that the selected
Google OAuth client type and scopes return refresh tokens for device flow and
that the resulting ID token audience is accepted by the gateway.

## External references

- Google OAuth 2.0 for mobile and desktop apps:
  <https://developers.google.com/identity/protocols/oauth2/native-app>
- Google OAuth 2.0 for limited-input devices:
  <https://developers.google.com/identity/protocols/oauth2/limited-input-device>
