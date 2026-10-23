# CLI auth-provider discovery flow

Status: component interaction flow approved by the product owner on 2026-10-23.
The approved CLI syntax is in
[`../user-guides/sqlrs-auth-providers-proposal.md`](../user-guides/sqlrs-auth-providers-proposal.md),
and the choice is recorded in
[`../adr/2026-10-23-cli-auth-provider-list-discovery.md`](../adr/2026-10-23-cli-auth-provider-list-discovery.md).

## Remote init

1. The user runs `sqlrs init remote <endpoint>` without a legacy token.
2. The CLI sends unauthenticated `GET <endpoint>/v1/connection-info` under the
   existing safe-redirect and endpoint-validation rules. It validates the
   installation, current/control endpoints, and nonempty provider summaries.
3. The CLI writes the remote profile atomically. Only after successful commit
   does human output show the returned provider IDs/display names and a login
   hint. A dry run reports what would change but does not present a persisted
   connection as ready for login.
4. Neither provider choice nor the catalogue is saved in workspace config.

## Repeatable discovery

1. The user runs `sqlrs auth providers`, optionally selecting a profile with
   the global `--profile` flag.
2. The CLI resolves that profile and any invocation-local global `--endpoint`
   override, then rejects local mode or a missing/invalid effective remote
   endpoint before network access.
3. It sends an unauthenticated `GET <selected profile endpoint>/v1/auth/providers`
   using the existing public transport rules. It does not attach a bearer token,
   cookie, ID token, or refresh token. An organization-prefixed endpoint remains
   prefixed; the CLI does not infer organization existence or membership.
4. The response is bounded at 1 MiB and validated as a public provider-summary
   list. Human output
   sorts by provider ID and displays ID/display name and a login hint; JSON
   output has the public `providers` array. Nothing is cached or persisted.

## Failure and login boundary

- Empty, malformed, or unavailable lists fail clearly, without stale fallback.
- A local profile fails before HTTP, engine startup, browser, or credential-store
  access.
- `sqlrs auth login` without a provider remains an error and points to
  `sqlrs auth providers`.
- `sqlrs auth login <provider-id>` continues to fetch that provider's current
  detail directly. List display is not login authorization and does not grant
  organization membership.
- No server API, database schema, gateway trust, or IdP enablement changes.
