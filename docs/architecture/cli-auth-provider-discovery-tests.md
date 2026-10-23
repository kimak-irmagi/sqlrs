# CLI auth-provider discovery test plan

Status: approved by the product owner on 2026-10-23 after critical review.
Covers the approved
[CLI syntax](../user-guides/sqlrs-auth-providers-proposal.md),
[flow](cli-auth-provider-discovery-flow.md), and
[component structure](cli-auth-provider-discovery-structure.md).

## Contract clarifications proposed with this test plan

- For repeatable discovery, the effective endpoint resolved by the existing
  command context is authoritative: the selected `--profile` supplies it, and
  an explicit global `--endpoint` overrides it for that invocation. Neither
  override is persisted by `auth providers`.
- Both human and JSON provider lists are sorted by provider ID. JSON has exactly
  one top-level `providers` array; each entry contains only `id`, `display_name`,
  and `adapter`.
- The public provider-list response, including HTTP error bodies, is capped at
  1 MiB before JSON decoding or error parsing.
  This is a proposed client-side safety bound, not an OpenAPI/server limit.
- Human rendering neutralizes CR, LF, and terminal escape/control characters in
  server-supplied display names; JSON retains the original value with normal
  JSON escaping.
- Error outcomes are distinguished by cause: `no enabled login providers`,
  `invalid provider catalogue`, and `cannot fetch provider catalogue`.
  Tests assert these categories rather than incidental full sentences.

## Tests to write before implementation

| ID | Layer | Scenario and required result |
| --- | --- | --- |
| P01 | `internal/client` | Fetch the root catalogue with one and two providers. Assert the exact GET path, public three-field decoding, and absence of `Authorization`/`Cookie` with a nonempty client auth token. |
| P02 | `internal/client` | Fetch through an organization-prefixed endpoint without stripping the prefix. A same-origin redirect succeeds; cross-origin and HTTPS-to-HTTP redirects fail without credential forwarding. |
| P03 | `internal/client` | Reject missing/null `providers`, unknown fields, malformed/trailing JSON, invalid or duplicate IDs, blank names/adapters, wrong content type, and a success or HTTP-error body over 1 MiB; test the exact size boundary. Empty `providers: []` remains a valid wire response. |
| P04 | `internal/client` | HTTP 503, timeout, and malformed response remain distinguishable from a valid empty catalogue; no cached or hard-coded fallback. |
| P05 | CLI process integration | Initialize a remote workspace, discard the init output, start a **new OS process** from that saved workspace, and run `auth providers` without reinitializing. Change the server catalogue and repeat in another process: the output changes and config bytes stay unchanged. This is the primary lost-init-output scenario. |
| P06 | `internal/app` | Default profile, explicit `--profile`, organization-prefixed endpoint, and explicit global `--endpoint` each contact only the effective selected server. Override is invocation-local. |
| P07 | renderer | Human output sorts IDs, shows each display name and a syntactically safe login example. Server-supplied CR/LF/escape controls cannot inject extra terminal lines or commands. JSON is valid, sorted, has exactly the three public fields, and contains no human hint, token, provider detail, or membership. |
| P08 | `internal/app` | Local mode and missing/invalid effective remote endpoint fail before HTTP, daemon/engine startup, browser, or credential-store access. |
| P09 | `internal/app` | Empty list, invalid catalogue, and unreachable server return the three distinct error categories above, with no previous-list or Google fallback and no partial success output. |
| P10 | `internal/app` init | Fresh token-free init with one/two providers and `init remote --update` to a changed endpoint display only that discovery result **after** successful config commit. Neither catalogue nor choice is persisted; unrelated settings survive update. |
| P11 | `internal/app` init | Discovery failure, config-write failure, dry-run, and deprecated bearer-token init never print a ready-to-login provider list. |
| P12 | parser/help | `auth providers` accepts no extra positional arguments or unsupported flags; `sqlrs auth --help` and `sqlrs auth providers --help` show it. `auth login` without provider remains an error and points to `sqlrs auth providers`. |
| P13 | regression | Explicit `auth login <provider-id>` still fetches provider detail directly without a preceding list call. Run existing `auth status`/`auth logout` tests rather than duplicate their full behavior here. |

After approval, compare existing auth/init/client tests with these expectations
and report any contradictions before changing tests. Implement the new tests
before production code. Run focused and full relevant CLI suites and measure
changed-code coverage (target 100%, minimum 95%); follow the repository's
approval process for any coverage-remediation iteration.
