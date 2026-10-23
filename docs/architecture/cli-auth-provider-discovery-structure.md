# CLI auth-provider discovery component structure

Status: component structure approved by the product owner on 2026-10-23. The approved
syntax and flow are in
[`../user-guides/sqlrs-auth-providers-proposal.md`](../user-guides/sqlrs-auth-providers-proposal.md)
and [`cli-auth-provider-discovery-flow.md`](cli-auth-provider-discovery-flow.md).

## Deployment units and ownership

Only `frontend/cli-go` changes. The existing shared-service endpoints
`GET /v1/connection-info` and `GET /v1/auth/providers` are reused. The gateway,
local engine, database, and credential store have no new responsibilities.

| Module | Responsibility |
| --- | --- |
| `internal/client` | Decode and validate a public `AuthProviderList{Providers []AuthProviderSummary}` from the selected endpoint using the existing unauthenticated transport and safe redirects. Reuse provider-summary validation and expose `GetAuthProviders(ctx)` without session/token access. |
| `internal/app` init | After token-free discovery and successful config commit, render the already validated `ConnectionInfo.AuthProviders`. Do not fetch a second list or print a success hint before the commit. Legacy bearer init and dry-run do not claim the session is ready. |
| `internal/app` auth | Parse `auth providers`, resolve the selected profile and output mode, reject local/invalid remote profiles, call `GetAuthProviders`, and render deterministic human or public JSON output. Add a discovery hint to the missing-provider login error. Leave explicit login/session behavior unchanged. |
| `internal/cli` | Keep root/global `--profile` and `--output` parsing unchanged. No new global option or persistent state. |
| `internal/config` | Continue to own the selected endpoint and stable installation metadata only. Provider catalogue and selection are never written to `.sqlrs/config.yaml`. |

`AuthProviderSummary` already exists in `internal/client` with `ID`,
`DisplayName`, and `Adapter`. A new `AuthProviderList` is a wire DTO, not a
persisted model. A shared summary validator should enforce the same ID grammar,
nonblank display name/adapter, and uniqueness for connection-info and the
standalone provider-list route. Empty lists are structurally valid API responses
but result in a distinct user-facing no-enabled-providers error for the command.

## Data flow and invariants

1. The effective `cmdCtx.profile.Endpoint` is the sole source of the repeatable
   request base. Global `--endpoint` may override the selected `--profile` for
   this invocation only. Use the existing normalized root-or-one-slug endpoint
   contract; do not replace it with the installation root or a hard-coded URL.
2. `client.GetAuthProviders` uses the same public HTTP path as bootstrap. It
   builds a request without auth headers/cookies, caps this list response at
   1 MiB before decoding, and validates JSON before returning summaries. It
   must not contact OAuth endpoints.
3. Rendering sorts a copy by ID so neither the wire response nor profile state
   is mutated. Human output includes ID and display name plus a short login
   hint, neutralizing terminal control characters in server-supplied names.
   JSON output exposes the public `providers` array only; no raw provider
   detail or credential-bearing fields.
4. The command performs no credential-store read, browser launch, engine
   startup, workspace write, or provider selection. Login still fetches the
   chosen provider detail directly, so a list observed earlier cannot authorize
   a later login.
5. Transport failure, malformed JSON/summary, and an empty list are distinct
   failures. Never substitute a cached, configured, or hard-coded provider.

## Compatibility

Existing `auth login <provider>`, `auth status`, and `auth logout` syntax and
behavior remain unchanged. `auth providers` is additive. Remote init's new
human output is additive; scripts should use the dedicated JSON command for
machine-readable catalogue data. No OpenAPI or schema change is required.
