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

The first CLI slice reuses the existing remote-profile bearer-token model:

```yaml
profiles:
  remote-dev:
    mode: remote
    endpoint: "https://sqlrs.example.org"
    auth:
      mode: bearer
      tokenEnv: SQLRS_TOKEN
```

The token can come from the explicit `SQLRS_TOKEN` override, from a legacy
static bearer profile, or from the Google OIDC session managed by
[`sqlrs auth`](sqlrs-auth.md). The CLI sends the effective token as
`Authorization: Bearer <token>` and never prints it.

## Command syntax

```text
sqlrs user register [--display-name <name>] [--email <email>]
sqlrs user create --identity-issuer <issuer> --identity-subject <subject> [--identity-provider <provider>] [--display-name <name>] [--email <email>]
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
- If the external identity is already linked to a user profile, the command is
  idempotent and returns the existing profile.
- If the external identity is not linked yet and server-side self-registration
  is disabled, the server rejects the request and does not create a profile.
- `--display-name` and `--email` are profile hints. The server may normalize or
  ignore them if trusted identity-provider claims are authoritative.
- The command does not create an organization.

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
      "provider": "oidc",
      "issuer": "https://idp.example.org",
      "subject": "248289761001"
    }
  ],
  "memberships": []
}
```

## `sqlrs user create`

Creates a sqlrs user profile for another external identity.

```text
sqlrs user create --identity-issuer <issuer> --identity-subject <subject> [--identity-provider <provider>] [--display-name <name>] [--email <email>]
```

Rules:

- The command requires a remote profile.
- The command requires a bearer token with administrative permission.
- `--identity-provider` defaults to `oidc`.
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
identity: oidc https://idp.example.org 248289761001
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
      "provider": "oidc",
      "issuer": "https://idp.example.org",
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

Human output:

```text
organization: acme
id: org_01J...
name: Acme
role: admin
```

JSON output:

```json
{
  "organization": {
    "id": "org_01J...",
    "slug": "acme",
    "display_name": "Acme",
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
