# 2026-09-24 Remote Connection Bootstrap

- Conversation timestamp: 2026-09-24T00:22:00.7856181+07:00
- GitHub user id: @evilguest
- Agent name/version: Codex / GPT-5
- Status: Accepted

## Decision Record 1: initialize remote profiles through service discovery

- Conversation timestamp: 2026-09-24T00:22:00.7856181+07:00
- GitHub user id: @evilguest
- Agent name/version: Codex / GPT-5

### Question discussed

How should `sqlrs init remote` obtain the installation and authentication
metadata needed for normal shared-service onboarding without requiring a user to
know a provider client ID or paste a bearer token?

### Alternatives considered

1. Keep requiring `--url` plus a static `--token`.
2. Require provider-specific flags such as `--client-id` during init.
3. Accept one remote base URL and discover non-secret installation, routing,
   and login-provider metadata from the service.
4. Compile one public deployment's provider metadata into the CLI.

### Chosen solution

Adopt option 3.

The canonical syntax is:

```text
sqlrs init remote <endpoint>
```

`--url <endpoint>` remains a compatibility spelling and cannot be combined with
the positional form. The existing `--token` path remains accepted but is
deprecated and creates a legacy bearer profile.

Normal init requests `<endpoint>/v1/connection-info` without authentication.
It stores the stable installation ID and control endpoint plus the normalized
current bootstrap endpoint. It does not store provider client configuration or
user credentials. A path-prefixed input URL is used as supplied; the CLI does
not remove the prefix, interpret it as a confirmed organization, or construct
organization URLs from slugs.

### Brief rationale

The provider registration belongs to the shared installation, not to an end
user. Service discovery makes the public onboarding command usable while
keeping installation-specific data server-owned and avoiding a deployment
special case in the CLI binary.

## Decision Record 2: select the provider at login and fetch adapter configuration

- Conversation timestamp: 2026-09-24T00:22:00.7856181+07:00
- GitHub user id: @evilguest
- Agent name/version: Codex / GPT-5

### Question discussed

Should init select one identity provider and persist its OAuth/OIDC parameters,
or should each `auth login <provider>` obtain the current provider configuration
from the selected installation?

### Alternatives considered

1. Select one provider during init and write its client ID and endpoints into
   workspace config.
2. Hard-code the Google authorization and token endpoints in the Google CLI
   adapter.
3. Let the installation advertise multiple provider IDs, then fetch a
   versioned provider-specific configuration when login is requested.
4. Define one unrestricted map of arbitrary OAuth parameters for every future
   provider now.

### Chosen solution

Adopt option 3 and reject option 4 for the current slice.

`GET <base>/v1/auth/providers` lists enabled provider IDs for discovery and UI.
`sqlrs auth login <provider>` retrieves the fixed
`<base>/v1/auth/providers/<provider>` detail and passes it to the corresponding
built-in CLI adapter.
This slice implements a generic `oidc` adapter and initially uses it for the
`google` provider. A future provider can reuse the adapter when it supports the
same public native-client profile; non-OIDC protocols or incompatible flows get
separate adapter designs when they are added.

The OIDC configuration includes, at minimum, a configuration version, public
client ID, issuer, authorization endpoint, token endpoint, scopes, flow, token
endpoint authentication method, and permitted provider authorization
parameters. The supported profile is Authorization Code with PKCE S256, a
loopback redirect, an ID token, refresh-token operation, and no confidential
client authentication. The CLI builds the login URL from the advertised
authorization endpoint rather than hard-coding a provider login URL. The CLI
still owns `state`, OIDC `nonce`, loopback `redirect_uri`, PKCE
verifier/challenge, and other per-attempt security values. Google-specific
values such as `access_type=offline` and `prompt=consent` are configuration
data, not adapter behavior.

The public provider API must not return confidential client secrets or user
tokens. A provider that requires a confidential native-client secret needs a
separately designed server-side broker and is not supported by this direct flow.

### Brief rationale

An installation may enable several login partners and rotate their public
configuration independently of CLI releases. A versioned protocol adapter
reuses standard OIDC behavior without treating a provider name as a protocol,
while retaining CLI ownership of login-attempt security controls.

## Decision Record 3: expose bootstrap routes at installation and organization bases

- Conversation timestamp: 2026-09-24T01:05:08.8015586+07:00
- GitHub user id: @evilguest
- Agent name/version: Codex / GPT-5

### Question discussed

How can both `sqlrs init remote https://api.example` and
`sqlrs init remote https://api.example/acme` complete discovery and login before
the user has an authenticated session?

### Alternatives considered

1. Support connection and provider discovery only at the installation root and
   make the CLI infer that root from an organization URL.
2. Redirect organization-prefixed discovery to an inferred root URL.
3. Expose connection-info, provider collection, and provider detail routes at
   both the installation root and every existing organization prefix.
4. Expose the bootstrap routes at the installation root and every candidate
   organization prefix without checking whether the candidate slug exists.

### Chosen solution

Adopt option 4.

The following unauthenticated routes are available relative to the installation
root or any candidate path prefix, including a nonexistent organization slug:

```text
GET <base>/v1/connection-info
GET <base>/v1/auth/providers
GET <base>/v1/auth/providers/<provider>
```

Provider configuration is installation-owned and equivalent through root and
candidate-prefixed routes. Connection info returns no organization ID, slug,
display name, existence flag, or membership data. It returns the stable control
base and a normalized current bootstrap base corresponding to the supplied
URL. A prefixed current base is an unverified routing candidate until a later
authenticated operation returns organization data. Protected requests return
`404` when the candidate organization does not exist or is not visible under
the protected endpoint's disclosure rules.

### Brief rationale

The caller's URL remains authoritative, path prefixes are not guessed, and a
new user can start directly from an organization URL without already knowing
the installation-root routing convention. Treating all candidate prefixes
identically prevents the public bootstrap surface from becoming an
organization-discovery oracle.

## Decision Record 4: reconcile one unambiguous organization automatically

- Conversation timestamp: 2026-09-24T01:05:08.8015586+07:00
- GitHub user id: @evilguest
- Agent name/version: Codex / GPT-5

### Question discussed

What should happen when a profile initialized at the installation root belongs
to an existing user in one organization, or when the first user creates an
organization?

### Alternatives considered

1. Always require the user to edit or reinitialize the profile manually.
2. Create a second profile and require another login.
3. Atomically change the selected profile to the canonical organization
   endpoint while retaining stable installation identity and auth session.
4. Construct the organization URL locally by appending its slug.

### Chosen solution

Adopt option 3.

One reconciliation component runs after successful login/current-user lookup,
self-registration, and organization creation. It switches an
installation-scoped profile only when the service returns exactly one
unambiguous organization and its canonical endpoint. It does not choose among
multiple memberships, does not construct URLs, and does not silently replace a
different organization endpoint explicitly selected by the user.

After a successful change, the CLI writes a warning to stderr in human and JSON
modes:

```text
warning: profile "remote" switched to organization "nsu": https://api.taidon.dev -> https://api.taidon.dev/nsu
```

The config write is atomic and must not overwrite a concurrently changed file.
If the remote operation succeeded but local reconciliation failed, the CLI
reports partial-success exit `1` and an explicit recovery command.

Credential lookup uses stable installation identity rather than the mutable
organization endpoint, so switching does not require another login.

### Brief rationale

The same behavior completes onboarding for both the first organization admin
and an existing member who started from the common URL. Keeping one profile and
one session avoids exposing routing mechanics as routine user work.

## Decision Record 5: constrain the reusable OIDC adapter profile

- Conversation timestamp: 2026-09-24T01:05:08.8015586+07:00
- GitHub user id: @evilguest
- Agent name/version: Codex / GPT-5

### Question discussed

Which protocol and security invariants must a provider satisfy to use the
generic `oidc` CLI adapter safely when an installation advertises multiple
login providers?

### Alternatives considered

1. Treat any URI-and-parameter payload labelled `oidc` as compatible.
2. Keep a provider-specific Google adapter.
3. Define a strict reusable public native-client profile and require a separate
   adapter for incompatible providers.

### Chosen solution

Adopt option 3. The `oidc` adapter requires Authorization Code, PKCE S256, an
external browser, an IP-literal loopback redirect, an ID token, refresh-token
operation, token endpoint authentication method `none`, and RFC 9207 `iss` in
the authorization response. The CLI binds the expected issuer and redirect URI
to each login attempt and rejects a missing or mismatched callback issuer.

OIDC issuer, authorization, token, and optional revocation endpoints use HTTPS;
the issuer has no query or fragment. Provider scopes contain `openid`.
Provider-owned authorization parameters cannot override CLI-owned security or
user-choice parameters, including `state`, `nonce`, PKCE fields, redirect URI,
scope, response mode, request objects, or `login_hint`.

Public bootstrap metadata is revalidated using `Cache-Control: no-cache` and an
ETag. Provider IDs are unique, provider detail identity matches its request
path, and an empty provider catalogue makes normal remote-session init fail
with an actionable error. Login fetches the fixed provider-detail path
directly; the collection remains useful for discovery and UI.

### Brief rationale

The strict profile keeps provider names out of protocol code while preventing
the generic adapter boundary from accepting unsafe or only partially compatible
OAuth/OIDC servers. In particular, callback issuer validation prevents
authorization-server mix-up when more than one login partner is enabled.

## Decision Record 6: close session, trust-boundary, and RC migration gaps

- Conversation timestamp: 2026-09-24T13:59:43.4702921+07:00
- GitHub user id: @evilguest
- Agent name/version: Codex / GPT-5

### Question discussed

Which additional invariants are needed so the revised bootstrap and OIDC
contract remains locatable after restart, cannot redirect credentials across a
trust boundary, and upgrades existing public-RC profiles predictably?

### Alternatives considered

1. Keep provider identity in the credential key and add an active-session
   pointer; accept advertised URLs with ordinary string-prefix validation; make
   RC users log in again unconditionally.
2. Store exactly one active session under stable profile/installation identity,
   structurally validate all URL roles and canonical endpoints, and migrate a
   derivable legacy credential during explicit profile update.
3. Put provider configuration and refresh credentials back into workspace
   config so all lookup inputs remain available.

### Chosen solution

Adopt option 2. The active credential key is `{profileName, installationID,
installationEndpoint}`;
the value owns provider, issuer, client ID, subject, and tokens. Login atomically
replaces the active session. `oidcSession` remains a deprecated read alias, and
existing auth commands retain read-only compatibility with its credential and
warn until explicit `init remote --update` rewrites the profile, removes legacy
provider fields, and migrates a derivable OS credential without copying secrets
into config. An already-present destination credential is authoritative; the
migration never overwrites it.

A service base is the root or exactly one valid slug segment, uses HTTPS except
literal-loopback development HTTP, and has no userinfo, query, fragment, dot
segments, or encoded separators. Bootstrap redirects cannot change origin or
downgrade. Canonical organization endpoints must stay on the control origin and
contain exactly one valid slug; custom domains require an explicit future
allowlist. OIDC endpoint query parameters may not collide with CLI-owned
parameters.

The service advertises a provider only when gateway trust accepts its exact
issuer/client-ID pair. Startup inconsistency yields `503`. Configuration version
1 requires the client ID as the sole ID-token audience and validates issuer,
`azp`, issued-at, and expiry; login additionally enforces nonce, while refresh
enforces stable subject, compares a returned nonce with the retained login
nonce, and atomically accepts refresh-token rotation.

Root and candidate bootstrap use one installation-owned handler without an
organization lookup, preserving equal status/body/cache semantics apart from
URL-derived current fields. A root current-user `404` after login exits zero and
suggests registration. Candidate `404`, reconciliation network/5xx, and local
profile-update failure retain the session but return partial-success exit `1`.

### Brief rationale

The selected model is restart-safe without duplicating provider state in the
profile, prevents bearer-token exfiltration through advertised routing, gives
the public RC a recoverable migration path, and makes anti-enumeration and
partial-success behavior testable rather than implicit.

## Decision Record 7: bind sessions to installation trust and close final review

- Conversation timestamp: 2026-09-24T15:43:46.2549798+07:00
- GitHub user id: @evilguest
- Agent name/version: Codex / GPT-5

### Question discussed

Which final constraints are required before freezing the design for tests and
implementation?

### Alternatives considered

1. Treat installation ID as globally trustworthy, keep `provider: oidc`, and
   leave migration/reconciliation failure details to implementation.
2. Bind credentials to the canonical control endpoint, distinguish service
   provider ID from adapter name, and specify crash-safe migration, override,
   refresh, identity-claim, and partial-success behavior.
3. Remove automatic reconciliation and RC credential migration entirely.

### Chosen solution

Adopt option 2. The active credential key includes profile name, installation
ID, and normalized installation control endpoint. Legacy credentials migrate
only across a matching origin through write/verify, atomic config update, and
best-effort legacy deletion; an existing destination is never overwritten and
dry-run never mutates credentials.

External identity `provider` is the stable service provider ID such as
`google`; `oidc` remains only the adapter name. Gateway trust maps each accepted
issuer/client-ID pair to that provider ID. `user create` therefore requires an
explicit provider ID.

Post-login reconciliation uses the ID token issued by that login, regardless
of `SQLRS_TOKEN`. User registration and organization creation do not persist a
profile switch when an explicit token override is active. Successful remote
work followed by reconciliation failure retains its result on stdout, writes
recovery to stderr, and exits `1`, including JSON mode.

OIDC login requires non-empty `sub` plus present `iat` and `exp`. Refresh binds
provider ID, adapter, issuer, client ID, and subject to the stored session.
Token/revocation POSTs do not follow redirects. Transient network, `429`, and
`5xx` failures retain the session; definitive grant rejection deletes it.
Logout still deletes locally when provider lookup or revocation fails.

### Brief rationale

These rules prevent cross-origin credential collisions, keep multi-provider
identity stable, make RC migration crash-safe, avoid persistent routing changes
from temporary token overrides, and give tests deterministic output and failure
semantics.

## Relationship to existing decisions

This ADR supersedes the static-token-only remote syntax in
[`2026-02-10-sqlrs-init-redesign.md`](2026-02-10-sqlrs-init-redesign.md), the
workspace-owned Google provider metadata and client-secret compatibility path in
[`2026-07-01-google-oidc-cli-auth.md`](2026-07-01-google-oidc-cli-auth.md), and
the mutable endpoint portion of the credential key in
[`2026-07-02-cli-auth-component-boundary.md`](2026-07-02-cli-auth-component-boundary.md).
It preserves the accepted loopback Authorization Code with PKCE flow and OS
credential-store boundary.

Decision Record 7 also supersedes the optional `--identity-provider` default in
[`2026-06-20-user-registration-command-split.md`](2026-06-20-user-registration-command-split.md): multi-provider identities require an explicit stable service provider ID.

## Contradiction check

The superseded portions above are marked accordingly. No other accepted ADR
defines service-discovered provider configuration or organization endpoint
reconciliation.
