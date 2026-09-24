# 2026-06-20 User Registration Command Split

> **Partially superseded (2026-09-24):** the optional
> `--identity-provider` default is superseded by
> [`2026-09-24-remote-connection-bootstrap.md`](2026-09-24-remote-connection-bootstrap.md).
> `user create` now requires the stable service provider ID explicitly; `oidc`
> is an adapter name, not an external identity provider ID.

- Conversation timestamp: 2026-06-20T09:47:44.4613773+07:00
- GitHub user id: @evilguest
- Agent name/version: Codex / GPT-5
- Status: Accepted

## Decision Record 1: split self-registration and administrator provisioning

### Question discussed

Should the first users/organizations CLI slice expose separate
`sqlrs user register` and `sqlrs user create` commands, or one command whose
flags select whether the target user is the current authenticated identity or
another external identity?

### Alternatives considered

1. Use one overloaded command, for example
   `sqlrs user register [--identity-issuer ... --identity-subject ...]`, where
   the absence of explicit identity flags means self-registration and their
   presence means administrator-created user provisioning.
2. Use separate commands:
   - `sqlrs user register` for the current authenticated external identity.
   - `sqlrs user create` for administrator-created profiles linked to an
     explicitly provided external identity.
3. Delay administrator-created users until a later invitation or organization
   membership slice.

### Chosen solution

Adopt option 2.

The accepted CLI surface keeps:

```text
sqlrs user register [--display-name <name>] [--email <email>]
sqlrs user create --identity-issuer <issuer> --identity-subject <subject> [--identity-provider <provider>] [--display-name <name>] [--email <email>]
```

`sqlrs user register` uses the current bearer token as both actor and target
identity. The server derives the target external identity from validated
OAuth/OIDC claims. If that identity is already linked, the command is
idempotent and returns the existing profile. If it is not linked and
self-registration is disabled, the server rejects the request without creating
a profile.

`sqlrs user create` uses the current bearer token as the administrator actor and
the supplied external identity as the target. It requires administrative
permission. The server creates the user profile and links the external identity
atomically. If the target identity is already linked to any user profile, the
server rejects the request as an already-linked identity and must not create
another user entity.

### Brief rationale

The two operations differ in security semantics, target identity source, and
duplicate handling. Keeping them separate prevents a normal user from
accidentally or intentionally turning self-registration into "register someone
else" by passing identity flags, and keeps administrator duplicate handling
explicit instead of hiding it behind idempotent self-registration behavior.

## Related documents

- `docs/user-guides/sqlrs-users-orgs.md`

## Contradiction check

No existing ADR was marked obsolete. Existing ADRs do not define a user or
organization registration command surface.
