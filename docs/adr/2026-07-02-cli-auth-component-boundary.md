# 2026-07-02 CLI Auth Component Boundary

- Conversation timestamp: 2026-07-02T01:06:12.1196914+07:00
- GitHub user id: @evilguest
- Agent name/version: Codex / GPT-5
- Status: Accepted

## Decision Record 1: isolate CLI auth sessions in `internal/authsession`

### Question discussed

Where should Google OAuth/OIDC login, refresh-token storage, cached ID-token
refresh, and protected-command bearer-token selection live inside the CLI?

### Alternatives considered

1. Put OAuth and session logic in `internal/app`, next to command dispatch and
   profile resolution.
2. Put OAuth and session logic in `internal/client`, next to sqlrs HTTP API
   calls.
3. Put OAuth and session logic in `internal/config`, because profile auth
   settings are loaded there.
4. Add a dedicated `internal/authsession` package that owns OAuth/session logic
   and exposes a small manager API to `internal/app`.

### Chosen solution

Adopt option 4.

`internal/authsession` owns PKCE, state/nonce generation, Google authorization
URL construction, loopback callback validation, token exchange/refresh/revoke,
ID-token claim decoding, credential-store access, and effective bearer-token
selection. `internal/app` keeps command dispatch and calls the auth session
manager. `internal/client` receives an already resolved bearer token for sqlrs
API calls. `internal/config` loads only non-secret auth settings.

### Brief rationale

OAuth/session behavior has different dependencies and safety rules from sqlrs
API transport and config loading. A dedicated package keeps secret handling,
token refresh, and provider-specific OAuth behavior testable and isolated while
preserving the existing command and HTTP client boundaries.

## Decision Record 2: store one active OIDC session per remote profile and OAuth client

Status: Partially superseded by
[2026-09-24 Remote Connection Bootstrap](2026-09-24-remote-connection-bootstrap.md).
The one-active-session behavior remains accepted, but credential lookup uses
stable installation identity rather than the mutable request endpoint.

### Question discussed

Should the credential-store lookup key include the Google subject, or should
the CLI store one active session per remote profile/client ID and keep the
subject in session metadata?

### Alternatives considered

1. Include the Google subject in the credential lookup key.
2. Store a separate plaintext or config-file pointer from profile to subject,
   then use that subject in the credential lookup key.
3. Use profile, endpoint, provider, issuer, and client ID as the credential
   lookup key; store subject and email inside the credential value.

### Chosen solution

Adopt option 3.

The active credential key is scoped to the selected profile, endpoint, provider,
issuer, and client ID. The stored session contains the Google subject and email.
A successful login overwrites the active session for that key, which switches
the Google account used by that profile.

### Brief rationale

`auth status` and protected remote commands need to find the active session
before they can decode a cached ID token or know the current subject. Putting
subject in the lookup key would require a second persistent index. Keeping one
active session per remote profile/client ID matches the CLI profile mental
model and avoids additional local state.

## Related documents

- `docs/architecture/cli-auth-component-structure.md`
- `docs/architecture/cli-auth-flow.md`
- `docs/user-guides/sqlrs-auth.md`
- `docs/adr/2026-07-01-google-oidc-cli-auth.md`

## Contradiction check

No existing ADR was marked obsolete. Existing accepted auth ADRs choose the
Google loopback PKCE flow and OS credential storage, but do not define the CLI
package boundary or credential lookup key shape.
