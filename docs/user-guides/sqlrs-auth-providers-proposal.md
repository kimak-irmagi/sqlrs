# CLI login-provider discovery proposal

Status: CLI syntax, flow, component structure, and tests approved by the product
owner on 2026-10-23; implemented in branch `feat/cli-auth-provider-discovery`.

## User-facing workflow

```text
sqlrs init remote <endpoint>
sqlrs auth providers
sqlrs auth login <provider-id>
```

After a successful token-free `init remote`, human output shows the provider IDs and display
names from that endpoint's connection-info response, followed by a short example of
`sqlrs auth login <provider-id>`. It does not silently pick a provider. The result remains
bound to the selected server, not a hard-coded provider or Taidon URL.

`sqlrs auth providers` is a read-only command for an existing remote profile. It fetches
the current `GET /v1/auth/providers` catalogue from the selected profile endpoint on each
invocation, including organization-prefixed endpoints. It lists each provider's `id` and
`display_name` in human mode; `sqlrs --output json auth providers` emits the public
`providers` array with `id`, `display_name`, and `adapter`. Human ordering is by provider ID
for deterministic output. The CLI does not persist or cache the catalogue in workspace
configuration.

`sqlrs auth login` with no provider remains an error but points to `sqlrs auth providers`.
`sqlrs auth login <provider-id>` retains its existing behavior and verifies the selected
provider against the server's current provider-detail endpoint. The list is guidance, not
a grant of organization membership or proof that a user is authorized.

## Errors and boundaries

- Local profile: identify that a remote profile is required; do not contact a server.
- Empty catalogue: say that the selected server advertises no login providers.
- Unavailable or malformed catalogue: return a clear discovery error without falling
  back to a stale or hard-coded list.
- Output only public summary fields; never print or store tokens, credentials, provider
  client configuration, or organization membership data.
- Preserve normal endpoint validation and safe redirect rules. No new server API is needed:
  connection-info and `/v1/auth/providers` already exist in the OpenAPI contract.

## Alternatives to decide

1. Show providers only after `init remote`: simple first-run experience, but no way to
   refresh the list later. Not preferred.
2. Provide only `auth providers`: refreshable, but first-run users still need to discover
   the command. Not preferred.
3. Show the list after remote init and provide `auth providers` for refresh: preferred.

This proposal does not enable additional identity providers or change gateway trust. Tests,
architecture documents, implementation, and release claims are separate approval stages.
