# CLI authentication flow

The CLI uses the installation-owned exchange contract in the
[OpenAPI](../api-guides/sqlrs-engine.openapi.yaml). Discovery schema revision 2
replaces revision 1 inside `/v1`; there is no negotiation or direct-provider
exchange fallback. See the [decision](../adr/2026-10-28-installation-token-exchange.md).

## Login

1. `sqlrs auth login <provider>` resolves a remoteSession profile and obtains
   anonymous provider detail. The provider ID must match the request; adapter,
   capabilities, URLs and authorization parameters are validated before opening
   a browser. Exchange/refresh URLs must be inside the trusted installation
   control origin and base. Optional catalogue lookup is not a prerequisite.
2. The CLI generates state, nonce and PKCE S256 material, starts an IPv4
   loopback listener on a random port and constructs the advertised authorization
   URL. Provider-specific parameters come from configuration, without Google defaults.
3. The browser authenticates with the provider. The callback returns a code to
   `http://127.0.0.1:<port>/`. The CLI checks state, response issuer and errors.
   `--no-browser` immediately prints the URL to stderr for manual opening.
4. The CLI anonymously posts code, exact redirect URI, verifier, nonce and
   expected installation/issuer/client binding to
   `/v1/auth/providers/{provider}/exchange` on the installation control endpoint.
5. The server checks its configured instance, exchanges the code with its own
   client ID/secret, verifies the signed ID token and returns the selected tokens
   synchronously with `Cache-Control: no-store`. It stores no user session or token.
6. The CLI independently checks issuer, sole audience, optional azp, subject,
   nonce and strictly future expiry before atomically replacing its OS credential.
   Access tokens cannot substitute for an ID token. Any failed login preserves
   the previous credential. The CLI never sees the application client secret.
7. Existing current-user and organization endpoint reconciliation runs after
   successful storage. Routing/admission failure is distinct from authentication;
   partial-success errors retain the new session. See the [guide](../user-guides/sqlrs-auth.md).

## Protected requests and refresh

`SQLRS_TOKEN` remains the highest-priority explicit override. Otherwise a complete
remote session is located by profile, stable installation ID and normalized
control endpoint. The current organization endpoint is not part of that key.
Provider/account identity belongs to the value. Incomplete legacy credentials
require a new login; migration never enables the old exchange protocol.

An eligible cached ID token is reused. Missing or refresh-due cached tokens
trigger anonymous current provider discovery and
`POST /v1/auth/providers/{provider}/refresh`, with the original subject/nonce
and expected installation/issuer/client binding. Changed provider trust prevents
submission of the stored refresh credential. No browser opens during refresh.

The server authenticates to the provider with its own secret and verifies the
result before returning it. The CLI also validates identity and expiry before
storing a rotated refresh token; omission retains the submitted token. An explicit
empty replacement is invalid. The five-minute refresh window is a scheduling
threshold, not a minimum accepted token lifetime.

Every failed refresh performs no credential Delete, Put or restoration, including
invalid_grant, cancellation, response loss and validation failure. A concurrent
successful process may have saved a newer token. There is no refresh lock or
exactly-once guarantee; errors explain when login is required. Exchange/refresh
POSTs never redirect or automatically replay. Response bodies are bounded to
1 MiB; token operations use the command timeout (default 30 seconds).

## Status, logout and storage

`sqlrs auth status` reads local safe metadata without refreshing or proving
upstream validity; it reports an active SQLRS_TOKEN override without printing it.
`sqlrs auth logout [--no-revoke]` attempts advertised secret-free revocation unless
skipped, then explicitly deletes local credentials even if discovery/revocation
fails. It leaves the environment override unchanged.

Refresh/ID tokens persist only in Windows Credential Manager, macOS Keychain or
Linux Secret Service/libsecret. There is no plaintext storage fallback. Linux and
macOS account naming uses the same installation key as reads, independent of the
selected provider. Raw tokens, codes, verifiers and secrets are absent from normal
output, JSON errors and diagnostic messages.

## Provider extension

A configured provider ID is an instance; `oidc` is the protocol adapter. A new
compatible instance requires server configuration and restart, with unchanged CLI
and server executables. Compatibility requires PKCE S256, loopback redirect,
response iss, RS256/JWKS, sole client audience, stable subject, refresh tokens and
fresh ID tokens on refresh. Organization policy is separate from this flow.
