# Decision: show the selected server's login providers during init and on demand

- Conversation time: 2026-10-23 11:02 Europe/Moscow
- GitHub user ID: `evilguest` (authenticated account for this repository)
- Agent: Codex (GPT-5)
- Issue: [#162](https://github.com/kimak-irmagi/sqlrs/issues/162)
- Status: approved design and tests; implemented in branch `feat/cli-auth-provider-discovery`

## Question

How can a user choose a server-advertised login provider during onboarding and
recover the list after the original `sqlrs init remote` output is lost?

## Alternatives

1. Print providers only after init: no repeatable discovery.
2. Add only `sqlrs auth providers`: repeatable but poor first-run discoverability.
3. Print after init and add `sqlrs auth providers`: first-run guidance and a
   repeatable query against the selected server.

## Decision and rationale

Choose option 3. The user explicitly approved a repeatable `sqlrs auth providers`
command for an already initialized workspace, including explicit profile
selection when needed. The CLI must query the selected server's current public
catalogue instead of persisting a copy or assuming Google. `auth login` still
requires an explicit provider ID. This preserves server ownership of enabled
providers and avoids stale onboarding instructions.

No earlier ADR contradicted this decision. The existing remote-bootstrap ADR
already assigns the provider catalogue to the installation and its public API.

## Decision: effective endpoint for repeated discovery

- Conversation time: 2026-10-23 11:18 Europe/Moscow
- GitHub user ID: `evilguest` (authenticated account for this repository)
- Agent: Codex (GPT-5)
- Status: approved

Question: When `sqlrs auth providers` receives both a selected profile and a
global `--endpoint`, which server supplies the catalogue?

Alternatives: ignore the override; reject the combination; or use the CLI's
existing effective endpoint resolution. Choose the effective endpoint. The
override is invocation-local and never persists a provider list or endpoint
change. This matches established global-flag behavior and makes the request
target explicit to the caller.

## Decision: bound the public provider-list response

- Conversation time: 2026-10-23 11:18 Europe/Moscow
- GitHub user ID: `evilguest` (authenticated account for this repository)
- Agent: Codex (GPT-5)
- Status: approved

Question: Should the new public catalogue read accept an unbounded JSON body?

Alternatives: rely on the current unbounded public JSON decoder; change every
public discovery route; or bound this new list response to 1 MiB before
decoding. Choose the last option as a scoped client-side safety limit. A body
larger than 1 MiB fails without partial results. This does not claim a server
or OpenAPI size limit and does not alter the existing connection-info route.
