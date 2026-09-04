# ADR: Run instance prefix resolution

Status: Accepted
Date: 2026-09-04

## Decision Record 1

- Timestamp: 2026-09-04T16:06:08+07:00
- User: @evilguest
- Agent: Codex (GPT-5)
- Question: Where and in what order should `sqlrs run --instance` resolve an
  instance ID prefix while preserving the existing full-ID and name behavior?
- Alternatives:
  - Resolve an eligible prefix in the CLI through the existing instance-list
    `id_prefix` filter, then send the canonical full ID to `POST /v1/runs`.
  - Extend `POST /v1/runs` so the engine run manager resolves prefixes.
  - Extend `GET /v1/instances/{idOrName}` so path lookup accepts prefixes.
  - List every instance in the CLI and filter locally.
- Decision: The CLI preserves exact full-ID and exact-name matches first. If an
  exact lookup misses and the value is a case-insensitive hexadecimal prefix of
  at least 8 characters, the CLI resolves it through
  `GET /v1/instances?id_prefix=...`. Exactly one match is converted to its full
  instance ID before `POST /v1/runs`; zero matches fail as not found and
  multiple matches fail as ambiguous.
- Rationale: This implements issue #95 using the prefix-filter contract already
  selected by [ADR 0007](0007-id-prefix-resolution.md), keeps exact-name
  behavior backward compatible, avoids broadening path lookup or run API
  semantics, and avoids unbounded client-side listing.
