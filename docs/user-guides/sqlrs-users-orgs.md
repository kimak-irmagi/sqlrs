# sqlrs users and organizations

## Overview

This guide describes the implemented CLI surface for user and organization
management.

The slice is designed for a shared or cloud sqlrs deployment where the API sees
an authenticated external OAuth/OIDC identity in the bearer token and can create
or find a sqlrs user profile for that identity.

Local engine mode does not support users or organizations. The CLI must detect
that case before starting or contacting a local engine and fail with an
actionable remote-profile message.

## Authentication prerequisite

Normal onboarding starts by discovering a shared installation and logging in
through one provider advertised by that installation:

```text
sqlrs init remote https://api.taidon.dev
sqlrs auth login google
```

The workspace profile stores stable installation/routing metadata rather than
provider client configuration:

```yaml
profiles:
  remote-dev:
    mode: remote
    installationID: taidon-production
    installationEndpoint: "https://api.taidon.dev"
    endpoint: "https://api.taidon.dev"
    auth:
      mode: remoteSession
      tokenEnv: SQLRS_TOKEN
```

The token can come from the explicit `SQLRS_TOKEN` override, from a legacy
static bearer profile, or from the active provider session managed by
[`sqlrs auth`](sqlrs-auth.md). The CLI sends the effective token as
`Authorization: Bearer <token>` and never prints it.

## Command syntax

```text
sqlrs user register [--display-name <name>] [--email <email>]
sqlrs user create --identity-provider <provider> --identity-issuer <issuer> --identity-subject <subject> [--display-name <name>] [--email <email>]
sqlrs user me

sqlrs org create <slug> [--name <display-name>]
sqlrs org ls
sqlrs org get <org-ref>
```

Global flags continue to work:

```text
sqlrs --profile remote-dev user register
sqlrs --profile remote-dev org create acme --name "Acme"
sqlrs --output json org ls
```

## `sqlrs user register`

Creates a sqlrs user profile for the currently authenticated external identity.

```text
sqlrs user register [--display-name <name>] [--email <email>]
```

Rules:

- The command requires a remote profile.
- The command requires a bearer token.
- The server derives the external identity from validated token claims.
- The gateway maps the token's trusted issuer/client-ID pair to the stable
  service provider ID, such as `google`; `oidc` is an adapter name, not an
  identity provider ID.
- If the external identity is already linked to a user profile, the command is
  idempotent and returns the existing profile.
- If the external identity is not linked yet and server-side self-registration
  is disabled, the server rejects the request and does not create a profile.
- `--display-name` and `--email` are profile hints. The server may normalize or
  ignore them if trusted identity-provider claims are authoritative.
- The command does not create an organization.
- If registration returns exactly one membership while the selected profile is
  still installation-scoped, the CLI atomically switches that profile to the
  canonical organization endpoint returned by the service and warns on stderr.

Human output:

```text
user: usr_01J...
status: created
displayName: Anton Zlygostev
email: zlygostev@example.com
organizations: 0
```

JSON output:

```json
{
  "status": "created",
  "user": {
    "id": "usr_01J...",
    "display_name": "Anton Zlygostev",
    "email": "zlygostev@example.com",
    "created_at": "2026-06-19T09:00:00Z",
    "updated_at": "2026-06-19T09:00:00Z"
  },
  "identities": [
    {
      "provider": "google",
      "issuer": "https://accounts.google.com",
      "subject": "248289761001"
    }
  ],
  "memberships": []
}
```

## `sqlrs user create`

Creates a sqlrs user profile for another external identity.

```text
sqlrs user create --identity-provider <provider> --identity-issuer <issuer> --identity-subject <subject> [--display-name <name>] [--email <email>]
```

Rules:

- The command requires a remote profile.
- The command requires a bearer token with administrative permission.
- `--identity-provider` is required and names a stable service provider ID such
  as `google`, never an adapter name such as `oidc`. The server rejects a
  provider that is not enabled and mapped by installation gateway trust.
- `--identity-issuer` and `--identity-subject` are required and identify the
  external OAuth/OIDC identity that will be allowed to use the created profile.
- The server creates the user profile and links that external identity in one
  atomic, identity-keyed operation.
- If the external identity is already linked to any user profile, the server
  rejects the create-only request with a precondition failure and must not
  create a second user entity. The CLI may read the existing identity-keyed
  resource afterward when it needs to show the current owner.
- The command does not create an organization and does not add the user to an
  organization.

Human output:

```text
user: usr_01J...
status: created
displayName: New User
email: new.user@example.com
identity: google https://accounts.google.com 248289761001
```

JSON output:

```json
{
  "status": "created",
  "user": {
    "id": "usr_01J...",
    "display_name": "New User",
    "email": "new.user@example.com",
    "created_at": "2026-06-19T09:00:00Z",
    "updated_at": "2026-06-19T09:00:00Z"
  },
  "identities": [
    {
      "provider": "google",
      "issuer": "https://accounts.google.com",
      "subject": "248289761001"
    }
  ],
  "memberships": []
}
```

## `sqlrs user me`

Returns the current user profile, linked external identities, and organization
memberships.

```text
sqlrs user me
```

Rules:

- The command requires a remote profile.
- If the token is valid but no sqlrs user profile exists yet, the server returns
  `404`; the current CLI reports `current user is not registered`.
- Memberships are read-only in this slice.

An actionable `sqlrs user register` suggestion is not implemented yet and is
tracked in [#94](https://github.com/kimak-irmagi/sqlrs/issues/94).

Human output:

```text
user: usr_01J...
displayName: Anton Zlygostev
email: zlygostev@example.com
organizations:
  acme admin
```

## `sqlrs org create`

Creates an organization and makes the current registered user its admin.

```text
sqlrs org create <slug> [--name <display-name>]
```

Rules:

- The command requires a remote profile.
- The command requires an existing sqlrs user profile.
- In the first slice, the user must not already belong to an organization.
- `<slug>` is the stable human-readable organization reference.
- Slugs are lowercase ASCII identifiers: `a-z`, `0-9`, and `-`; 3 to 63
  characters; no leading, trailing, or repeated hyphen.
- If `--name` is omitted, the server uses the slug as the display name.
- The created membership role is `admin`.
- The response includes the canonical organization endpoint.
- After successful creation, the CLI atomically switches the selected profile
  from the installation root to that endpoint and writes the same profile-switch
  warning to stderr in human and JSON modes.

Human output:

```text
organization: acme
id: org_01J...
name: Acme
endpoint: https://api.taidon.dev/acme
role: admin
```

JSON output:

```json
{
  "organization": {
    "id": "org_01J...",
    "slug": "acme",
    "display_name": "Acme",
    "endpoint": "https://api.taidon.dev/acme",
    "created_at": "2026-06-19T09:00:00Z",
    "updated_at": "2026-06-19T09:00:00Z"
  },
  "membership": {
    "user_id": "usr_01J...",
    "organization_id": "org_01J...",
    "role": "admin",
    "created_at": "2026-06-19T09:00:00Z"
  }
}
```

## `sqlrs org ls`

Lists organizations the current user belongs to.

```text
sqlrs org ls
```

Rules:

- The command requires a remote profile.
- The command requires an existing sqlrs user profile.
- The result is scoped to the current authenticated user.

Human output:

```text
SLUG  ROLE   NAME
acme  admin  Acme
```

## `sqlrs org get`

Returns one organization visible to the current user.

```text
sqlrs org get <org-ref>
```

`<org-ref>` may be an organization id or slug.

Rules:

- The command requires a remote profile.
- The command requires an existing sqlrs user profile.
- Non-members receive `404` so callers do not learn whether an inaccessible
  organization exists.

## Local mode behavior

All `user` and `org` commands are remote-only. In local mode the CLI must fail
before local engine discovery or autostart.

Example:

```text
user and organization management commands require remote mode
```

The current error does not include the selected profile name or a retry command.
That actionable guidance is tracked in
[#94](https://github.com/kimak-irmagi/sqlrs/issues/94).

## Error handling

- `401` means the remote profile has no valid bearer token or the token was
  rejected.
- `403` from `user register` with code `self_registration_disabled` means the
  authenticated external identity is not linked yet and the server currently
  disallows self-registration.
- `403` from `user create` means the current user is not allowed to create other
  users.
- `404` from `user me`, `org ls`, or `org get` means the user profile or visible
  organization is missing.
- `412` from `user register` or `user create` means the create-only
  conditional PUT did not create a new profile because the target identity is
  already linked. The CLI may follow up with the corresponding `GET` request to
  show the existing profile when the current actor is allowed to see it.
- `409` from `org create` means the slug is already taken or the current first
  slice policy forbids organization creation for a user that already has an
  organization membership.
- If a remote registration or organization creation succeeds but the local
  profile cannot be updated, the CLI reports partial-success exit code `1`,
  a warning, and an explicit `sqlrs init remote <organization-endpoint>
  --update` recovery command. It must not claim that the remote operation
  failed or silently overwrite a concurrently changed config file.
  Human and JSON stdout retain the complete successful remote result; warnings
  and recovery remain on stderr, so JSON stdout is one valid document.
- When the effective bearer token comes from `SQLRS_TOKEN` or another explicit
  override, `user register` and `org create` do not persist an automatic profile
  switch. They print the canonical endpoint and an explicit `sqlrs init remote
  <organization-endpoint> --update` command instead and exit successfully, so a
  temporary identity cannot reroute the stored session's profile.

## Onboarding flows

### First user of an organization

```text
sqlrs init remote https://api.taidon.dev
sqlrs auth login google
sqlrs user register
sqlrs org create nsu --name "NSU"
```

The first three commands use the installation control endpoint. Organization
creation returns the canonical `https://api.taidon.dev/nsu` endpoint and the CLI
switches the same profile to it. Before switching, the CLI requires the control
origin, exactly one valid slug segment, and no userinfo, query, or fragment; an
untrusted endpoint is never persisted or sent a bearer token. The auth session
remains valid because its credential key is scoped to the stable installation,
including its canonical control endpoint, not the mutable request endpoint.

### Existing member starting from the installation root

```text
sqlrs init remote https://api.taidon.dev
sqlrs auth login google
```

After login, the CLI reads the current user through the control endpoint. When
that identity is already registered with exactly one organization membership,
the CLI switches the profile to the canonical organization endpoint. Re-running
`sqlrs user register` remains safe and returns the existing profile.

### User starting from an organization URL

```text
sqlrs init remote https://api.taidon.dev/nsu
sqlrs auth login google
```

The candidate-prefixed connection-info and provider routes are public before
login even when `/nsu` does not exist. Discovery returns the normalized current
bootstrap base and stable installation control base, but no organization
identity or existence signal. Login does not strip or guess the `/nsu` prefix.

After login, the first protected current-user request verifies the candidate
prefix and membership. When `/nsu` exists and is visible to the user, the CLI
binds the returned authenticated organization metadata to the profile. The URL
does not change, so no switch warning is printed. A nonexistent or non-visible
candidate produces the protected endpoint's `404`, retains the valid session,
and exits with partial-success code `1` and an actionable message to reinitialize with the
installation root or the correct organization URL. A root current-user `404`
instead exits successfully and invites the user to register.

If an installation-root profile has no memberships, it stays at the root. If
it has multiple memberships, the CLI does not select one implicitly; explicit
organization selection is a later CLI slice.

## Identity uniqueness

The server, not the CLI, owns identity uniqueness. For every linked external
identity, the persistent store must enforce a unique key over:

- `provider`
- `issuer`
- `subject`

`sqlrs user register` is idempotent for an already linked current identity and
returns the existing profile instead of creating a new one. At the API layer the
CLI can implement this by issuing create-only `PUT /v1/users/me` and reading
`GET /v1/users/me` after a precondition failure.

`sqlrs user create` is identity-keyed by the supplied external identity. At the
API layer the CLI uses create-only conditional `PUT` against that natural key.
If the identity is already linked, the server returns a precondition failure so
an administrator can inspect the existing account before taking manual action.

## Non-goals for this slice

- Local engine support for users or organizations.
- Email-only invitations, member invitation, role changes, organization
  deletion, or user deletion.
- Billing, quotas, or organization-scoped prepare/run authorization changes.
- Changes to the CLI login/session-management flow. `sqlrs auth` is a separate
  CLI slice; the user and organization commands only consume the effective
  bearer token selected for the remote profile.
