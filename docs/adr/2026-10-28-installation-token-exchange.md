# Installation-owned token exchange

- Conversation timestamp: 2026-10-28T22:04:24+07:00.
- GitHub user id: @evilguest (41718235).
- Agent name/version: Codex / GPT-6.
- Status: Accepted; implementation tracked by [#167](https://github.com/kimak-irmagi/sqlrs/issues/167).

## Question discussed

Where should independently configured installations keep application credentials,
and how should CLI login and refresh obtain user tokens?

## Alternatives considered

1. Publish provider credentials and exchange directly from the CLI.
2. Receive browser callbacks and retain user tokens on the server for later pickup.
3. Keep the callback local and synchronously exchange through the installation API.

## Chosen solution or decision made

Use option 3. The CLI owns PKCE, state, nonce, local callback and OS credentials.
The installation owns client ID/secret and provider exchange/refresh. Discovery
schema revision 2 replaces revision 1 in `/v1`, without negotiation or fallback.
Only public metadata and installation operation URLs reach the CLI.

Every failed refresh leaves the current credential untouched, including invalid_grant
and concurrent refresh success. A compatible configured OIDC instance requires no
new CLI build. The same installation-scoped OS account address is used for reads,
writes and deletion, regardless of provider selection.

## Brief rationale

A local callback returns the code directly to the waiting client. Synchronous
exchange avoids server token-pickup state while keeping application secrets on
their installation. Failed processes must not undo another process's successful
refresh. Provider/account data belongs to the session value, not its lookup key.

## Contradiction check

This supersedes direct token exchange, schema negotiation and invalid_grant deletion
in the authentication portions of the [remote bootstrap ADR](2026-09-24-remote-connection-bootstrap.md).
Unrelated routing and admission decisions remain unchanged. A matched CLI release
is tracked separately in [#168](https://github.com/kimak-irmagi/sqlrs/issues/168).
