# Remote connection bootstrap and OIDC: test matrix

Status: fully approved on 2026-09-25 by @evilguest after critical and final
review. The review policy resolutions and the explicit `--no-browser` output
boundary are recorded in the bootstrap ADR.

This matrix covers the sqlrs CLI side of issue #125. Server-side bootstrap,
anti-enumeration, and gateway acceptance remain tracked by
[izess#122](https://github.com/kimak-irmagi/izess/issues/122). Tests use injected
clocks, entropy, browser/listener, credential stores, filesystem writers, and
`httptest` OIDC/sqlrs services. A real Google account is not an acceptance
dependency.

Normative sources:

- [remote init guide](../user-guides/sqlrs-init.md)
- [auth guide](../user-guides/sqlrs-auth.md)
- [user and organization guide](../user-guides/sqlrs-users-orgs.md)
- [auth flow](cli-auth-flow.md)
- [auth component structure](cli-auth-component-structure.md)
- [user and organization flow](user-org-flow.md)
- [OpenAPI contract](../api-guides/sqlrs-engine.openapi.yaml)

Requirement-family traceability:

| Test IDs | Normative section |
| --- | --- |
| `SURF-*`, `URL-*`, `INIT-*` | Remote flags, discovery/persistence, migration, and error behavior in the remote init guide |
| `DISC-*`, `PROV-*` | Bootstrap/provider types and validation in the auth component structure and OpenAPI |
| `OIDC-*`, `CLAIM-*` | Interactive login flow and security invariants in the auth guide/flow |
| `SESS-*`, `REF-*`, `STAT-*`, `LOGOUT-*` | Token resolution, refresh, status, logout, and failure table in the auth guide/flow |
| `MIG-*` | RC profile/credential migration in the init guide and auth component structure |
| `REC-*`, `UORG-*` | Endpoint reconciliation and authenticated onboarding in the auth and user/org flows |
| `OUT-*`, `E2E-*` | Output/security invariants and the three documented onboarding journeys |

Each row is a test family. Every listed case must have an independent assertion
or named table entry. “Unchanged” means byte-identical workspace config and no
credential-store mutation unless the row says otherwise.

## 1. CLI surface and URL trust

| ID | Level / owner | Given and action | Required observations |
| --- | --- | --- | --- |
| SURF-01 | parser unit / `internal/app` | Parse positional endpoint; parse compatibility `--url`; parse either with `--update`, `--dry-run`, and profile selection. | Both endpoint spellings produce the same normalized option; no token is required. |
| SURF-02 | parser unit / `internal/app` | Missing endpoint, positional plus `--url`, two positional endpoints, or remote command with local-only flags. | Argument error, exit `64`, no network/filesystem/store calls. |
| SURF-03 | parser + command / `internal/app` | Use explicit deprecated `--token`. | Select legacy `bearer` mode, do not perform bootstrap discovery, emit deprecation guidance, and preserve the existing compatibility config shape. |
| SURF-04 | parser/render / `internal/app`, `internal/cli` | Parse `auth login <provider>` and `user create` with valid and syntactically invalid provider IDs. | Provider is a required stable provider ID matching `AuthProviderID`; `user create` requires `--identity-provider`; `oidc` is never inferred from the adapter as a default provider; help shows positional remote init and generic login syntax. |
| SURF-05 | command unit / `internal/app` | Select a local profile for auth login/status/logout or a user/org command. | Reject before daemon lookup, engine discovery/autostart, provider/user HTTP, browser, or credential-store access. |
| URL-01 | table unit / `internal/client` | Validate HTTPS root, valid one-slug candidate, trailing slash, explicit port, and literal IPv4/IPv6 loopback HTTP. | Accepted inputs have one deterministic normalized representation; root and candidate remain distinct. |
| URL-02 | table unit / `internal/client` | Validate non-loopback HTTP, userinfo, query, fragment, dot segments, encoded dot or slash/backslash, empty/double/additional segments, bad percent escapes, and invalid slug length/hyphens/characters. | Reject before HTTP or persistence. |
| URL-03 | table unit / `internal/client` | Validate discovered control/current pairs: root/root, root/candidate, different scheme/authority/port, nested current, candidate control, and current outside control. | Accept only same-origin root control plus root-or-one-slug current; use parsed URL components rather than string prefixes. |
| URL-04 | HTTP unit / `internal/client` | Connection-info or provider discovery responds with relative same-origin redirect, absolute same-origin redirect, cross-origin redirect, HTTPS-to-HTTP redirect, loop, or excessive chain. | Follow only the explicitly supported safe redirects; reject cross-origin, downgrade, loop, and limit breach without forwarding authorization. |
| URL-05 | table unit / `internal/client` | Validate issuer and OIDC endpoints containing HTTP, userinfo, fragment, allowed query, or query keys colliding with CLI-owned OAuth parameters. | Issuer and endpoints satisfy their distinct rules; allowed endpoint query is preserved; collisions are rejected. |
| URL-06 | table unit / endpoint reconciler | Validate canonical organization endpoints on/off the control origin, root, one valid slug, nested path, userinfo/query/fragment, and custom domain. | Only the same-control-origin one-slug endpoint is trusted; no request is sent to an untrusted returned endpoint. |

## 2. Bootstrap discovery and remote init

| ID | Level / owner | Given and action | Required observations |
| --- | --- | --- | --- |
| DISC-01 | HTTP unit / `internal/client` | Discover from root and candidate input. | Send unauthenticated `GET <exact-normalized-base>/v1/connection-info`; never strip the candidate prefix and never attach bearer/cookie credentials. |
| DISC-02 | table unit / `internal/client` | Decode valid connection info plus missing/empty installation ID, missing endpoints, empty providers, duplicate/invalid provider IDs, and invalid cross-field URLs. | Valid DTO returned only after all invariants; empty providers is a typed “interactive login unavailable” result; malformed data is never persisted. |
| DISC-03 | HTTP unit / `internal/client` | Fetch one provider for login. | Call the fixed detail path directly and unauthenticated; do not require or call the provider collection first. |
| DISC-04 | HTTP unit / `internal/client` | Connection/provider routes return `304`, `404`, `405`, `429`, `5xx`, malformed JSON, wrong content/body, timeout, or cancellation. | Map each supported response to a deterministic typed/actionable error; never mutate config/store or open the browser. Provider-detail `404` is “unknown or disabled provider”; bootstrap absence is an upgrade error. |
| INIT-01 | command integration / `internal/app` | Fresh root init with valid discovery. | Write one remote profile with installation ID, control endpoint, current endpoint, `remoteSession`, and token-env name; select it; persist no provider config or secret. |
| INIT-02 | command integration / `internal/app` | Fresh valid candidate init. | Request candidate bootstrap and persist candidate as current endpoint while retaining root control endpoint; do not claim the organization is verified. |
| INIT-03 | command integration / `internal/app` | `--update` a workspace containing local profile, unrelated profiles/settings, and selected-profile state. | Replace only approved remote metadata, preserve unrelated data, and select the remote profile atomically. |
| INIT-04 | command integration / `internal/app` | Repeat the same successful init; run a different endpoint without `--update`. | Same normalized endpoint is an idempotent success; a different endpoint is an actionable conflict with exit `64`; neither case mutates the existing workspace without `--update`. |
| INIT-05 | failure injection / `internal/app` | Fresh or update discovery/validation/config-write fails at each boundary. | Fresh failure leaves no partial workspace; update failure leaves config unchanged; no credential mutation except an explicitly tested migration stage. |
| INIT-06 | command integration / `internal/app` | Run `--dry-run` for fresh init, update, and legacy migration. | Perform the same parsing, network discovery, and validation; render intended actions; do not create files or mutate either credential key. |
| INIT-07 | command integration / `internal/app` | Server lacks `/v1/connection-info`; provider list is empty; installation/provider data is inconsistent (`503`). | Normal init fails with upgrade/configuration guidance and no insecure fallback; explicit `--token` remains the deliberate bypass. |

## 3. Provider configuration and interactive login

| ID | Level / owner | Given and action | Required observations |
| --- | --- | --- | --- |
| PROV-01 | table unit / `internal/client` | Validate a complete version-1 OIDC detail and variants with mismatched ID, unsupported adapter/version/flow/auth method, missing `openid`, duplicate/empty scopes, false RFC 9207 support, or missing required endpoint/client/issuer. | Only the approved public-client configuration reaches the adapter; optional revocation endpoint is allowed to be absent. |
| PROV-02 | table unit / `internal/client` | Supply static authorization parameters, including each reserved key (`client_id`, redirect/scope/state/nonce/PKCE, `login_hint`, `response_mode`, `request`, `request_uri`) and benign provider keys. | Reject every collision; retain benign keys exactly. |
| OIDC-01 | unit / `internal/authsession` | Dispatch provider config with adapter `oidc`, unknown adapter, and known adapter with unknown configuration version. | Generic OIDC adapter handles the first; unsupported values fail before entropy, listener, browser, token HTTP, or store access. |
| OIDC-02 | unit / `internal/authsession` | Build authorization URL with fixed entropy/listener, existing allowed endpoint query, provider parameters, optional login hint, and scopes. | Preserve allowed query; add exactly one copy of every CLI-owned parameter; use PKCE S256, state, nonce, advertised client ID/scopes, and the advertised authorization endpoint. |
| OIDC-03 | failure-order unit / `internal/authsession` | Provider validation, entropy, listener creation, or URL construction fails. | Browser is not opened, token endpoint is not called, and existing credential is unchanged. |
| OIDC-04 | table unit / callback | Receive matching callback, wrong/missing state, wrong/missing RFC 9207 issuer, OAuth error, missing/duplicate code, wrong host/path, and a second callback. | Accept exactly one bound loopback callback; all invalid cases fail before token exchange and do not expose callback data. |
| OIDC-05 | HTTP unit / OAuth client | Exchange code against advertised token endpoint. | POST public-client PKCE fields; omit `client_secret` and HTTP client-auth credentials; never follow redirect; classify transport, status, malformed body, missing ID token, and missing refresh token without storing a session. |
| CLAIM-01 | table unit / claims | Validate login claims over exact issuer, nonempty `sub`, sole audience, optional matching/mismatching `azp`, required `iat`/`exp`, expired token, future `iat` boundary, matching nonce, and optional email. | Each invalid required claim rejects login; boundary clock behavior is deterministic; no signature-verification claim is made locally. |
| OIDC-06 | store unit / `internal/authsession` | Successful login with no prior session, a prior provider/account session, and injected atomic replacement failure. | Store complete new session only after validation; replace provider/account atomically; on failure retain the previous complete session and render no success. |
| OIDC-07 | command integration / `internal/app` | `SQLRS_TOKEN` is set while interactive login succeeds. | Login itself stores the new session; post-login `/v1/users/me` uses the new login ID token, not the environment override. |

## 4. Session lookup, refresh, status, and logout

| ID | Level / owner | Given and action | Required observations |
| --- | --- | --- | --- |
| SESS-01 | table unit / credential key | Vary profile, installation ID, normalized control endpoint, mutable current endpoint, and equivalent control spelling. | Profile/ID/control distinguish keys; an org endpoint switch does not; equivalent normalized control endpoints produce one key. |
| SESS-02 | unit / token resolver | Resolve with `SQLRS_TOKEN`, fresh cached ID token, expiring token, missing session, legacy bearer profile, and unsupported mode. | Enforce documented precedence; make token source explicit to the reconciler; do not read/refresh a stored session when the environment override wins. |
| REF-01 | unit / token resolver | Cached ID token is outside/inside the five-minute refresh window at fixed times. | Reuse or refresh at the exact documented boundary. |
| REF-02 | HTTP/store unit / token resolver | Before refresh, provider detail is unavailable or changes provider ID, adapter, issuer, or client ID. | Require a new login; retain session; never send the refresh token to any token endpoint. |
| REF-03 | table HTTP unit / token resolver | Refresh gets redirect, transport error, `429`, each `5xx`, `invalid_grant`, another `4xx`, malformed success, or missing ID token. | Never follow the redirect. Configuration version 1 deletes the session only for OAuth `invalid_grant`; every other failure retains it and returns the appropriate retry/configuration error; never call the protected sqlrs API with an invalid result. |
| REF-04 | claims/store unit / token resolver | Refreshed claims change issuer, subject, audience/`azp`; omit/break `iat`/`exp`; return absent, matching, or mismatching nonce; rotate or omit refresh token. | Reject invalid claims while retaining prior refresh credential; accept allowed nonce behavior; atomically store valid ID token plus rotated-or-existing refresh token. |
| REF-05 | failure injection / token resolver | Atomic storage of a valid rotated refresh result fails. | Do not expose the new ID token to the protected command; leave the prior local session byte-identical, report that re-login may be required because provider-side rotation cannot be rolled back, and never claim refresh success. |
| STAT-01 | renderer/store unit | Status with no session, active session, expired metadata, environment override, legacy profile, and store error in human/JSON/verbose modes. | Report documented safe fields and override source; never refresh merely for status; never print refresh token, ID token, nonce, or raw subject. |
| LOGOUT-01 | table unit / auth session | Logout with revocation endpoint, without it, with `--no-revoke`, provider discovery failure or changed provider binding, redirect, network/4xx/5xx revocation failure, no session, and local delete failure. | Never send a token under changed/untrusted configuration or follow revoke redirect; attempt revocation only when applicable; always attempt local deletion after remote/config failure; accurately report whether local credentials remain. |

## 5. RC credential migration

The migration harness records ordered store/config operations and injects a
failure before and after each operation. It verifies states, not a claim that a
process crash itself was simulated.

| ID | Level / owner | Given and action | Required observations |
| --- | --- | --- | --- |
| MIG-01 | config/store unit | Load `oidcSession` and run existing auth commands without update. | Resolve the legacy key and fields, warn, and do not rewrite config or credentials implicitly. |
| MIG-02 | command unit / init | Update when legacy endpoint origin differs from discovered control origin or legacy metadata is insufficient. | Do not copy/delete the legacy credential; write only safe new profile metadata and require a new login. |
| MIG-03 | command unit / init | Destination credential already exists. | Treat it as authoritative, never overwrite it, commit approved profile metadata, and leave the legacy credential untouched because no verified copy was performed in this invocation. |
| MIG-04 | ordered failure injection / init | Destination absent; migration succeeds. | `Put destination -> read/verify destination -> atomic config commit -> best-effort legacy delete`; final config uses `remoteSession` with no legacy provider fields or secret. |
| MIG-05 | ordered failure injection / init | Fail destination put or verification. | Config and legacy credential remain unchanged; an unverified destination is not selected as active. |
| MIG-06 | ordered failure injection / init | Fail atomic config commit after verified destination write. | Legacy credential and old config remain; destination may also remain; neither credential is lost and retry is safe. |
| MIG-07 | ordered failure injection / init | Fail legacy delete after config commit. | New config/destination remain authoritative; legacy may remain; command warns without rolling back to an unsafe state; retry is idempotent. |
| MIG-08 | session/claims unit | A migrated session lacks login nonce; refresh returns no nonce or a nonce. | No-nonce refreshed token may pass other checks; returned nonce without stored comparison requires a new login and is not sent to sqlrs. |

## 6. Endpoint reconciliation and user/organization commands

| ID | Level / owner | Given and action | Required observations |
| --- | --- | --- | --- |
| REC-01 | table unit / endpoint reconciler | Root profile and authenticated user has zero, exactly one, or multiple memberships. | Zero/multiple leave endpoint unchanged; exactly one trusted canonical endpoint is atomically selected. |
| REC-02 | command integration / login | Root `/v1/users/me` returns `404`. | Login/session succeeds with exit `0`; suggest `user register`; no profile switch. |
| REC-03 | table unit / endpoint reconciler | Candidate response names the same canonical endpoint, a different organization endpoint, or malformed/untrusted endpoint. | Same endpoint binds metadata without switch warning; different/untrusted result is never persisted or contacted, retains the login session, and returns partial-success exit `1` with recovery. |
| REC-04 | command integration / login | Candidate current-user returns `404`. | Keep valid session, emit complete login stdout, recovery on stderr, and exit `1`. |
| REC-05 | table command / login | Reconciliation times out, returns network/`5xx`, or profile write fails. | Keep valid session; complete success stdout; stderr recovery; exit `1`; never claim switch. |
| REC-06 | concurrency/failure unit / config | Config differs from the snapshot before compare-before-write. | Refuse overwrite, preserve concurrent edit, and return partial success with explicit recovery. |
| REC-07 | table unit / endpoint reconciler | Token source is `StoredRemoteSession`, `EnvironmentOverride`, or `LegacyBearer`. | Persist only for `StoredRemoteSession`; environment and legacy bearer sources print canonical endpoint and init/update recovery, exit `0`, and do not change config. |
| REC-08 | renderer unit | Successful automatic switch in human and JSON modes. | Write the exact old/new/profile/org warning only to stderr; stdout remains the complete normal result and JSON is one valid document. |
| UORG-01 | command integration / user register | Create returns `201`; create-only returns `412` followed by successful current-user read. | Render created/existing result respectively and run the same reconciliation matrix for both. |
| UORG-02 | command integration / org create | Creation succeeds with canonical endpoint; remote succeeds but reconciliation fails; token override is active. | Switch and warn for stored session; partial exit `1` after local failure; suppress switch and exit `0` with recovery for override. Remote success is never reported as failed. |
| UORG-03 | parser/client unit / user create | Provider is absent, `oidc`, or an explicit valid service provider ID; request is sent. | Absence is argument error; explicit value is transmitted unchanged; no adapter default is inserted. Existing conditional request semantics remain. |
| UORG-04 | regression / protected commands | Exercise user/org reads and writes after endpoint switch. | Credential lookup still uses installation control key while HTTP uses the new current endpoint; bearer tokens and canonical URLs are not logged. |

## 7. Output, security canaries, and end-to-end acceptance

| ID | Level / owner | Given and action | Required observations |
| --- | --- | --- | --- |
| OUT-01 | table renderer/command | Put unique canaries in refresh token, ID token, authorization code, PKCE verifier, environment token, and full subject; trigger success and every error family in normal/JSON/verbose output. | No secret canary appears in stdout, stderr, returned errors, config, or diagnostic serialization; only documented masked/safe fields appear. |
| OUT-02 | command integration | Exercise normal success, argument failure, root unregistered login, candidate/reconciliation partial success, and remote-write/local-update partial success. | Assert exact exit classes (`0`, `1`, `64` as specified), stdout/stderr separation, and one valid JSON stdout document. |
| OUT-03 | command integration / login | Run login in human and JSON modes with and without explicit `--no-browser`, using canaries for URL, state, nonce, PKCE challenge, and verifier; then trigger callback and later errors. | `--no-browser` writes the complete authorization URL immediately and exactly once to stderr; that URL contains state, nonce, and challenge but not verifier. Browser mode prints no URL. The URL and its transient canaries do not recur in final output, errors, verbose diagnostics, or logs; stdout contains only the final result and JSON stdout is one valid document. |
| E2E-01 | fake-service acceptance | Fresh root init -> login -> root user `404` -> register with no membership -> org create. | No manual client ID/token; session persists; org create switches profile with warning; subsequent protected command reuses the same credential under the new request endpoint. |
| E2E-02 | fake-service acceptance | Second user: fresh root init -> login -> current user with exactly one membership. | Automatic switch to server-returned endpoint with stderr warning and stable credential lookup. |
| E2E-03 | fake-service acceptance | Candidate init -> login with matching organization; repeat with protected candidate `404`. | Matching candidate binds without switch warning; nonexistent/invisible candidate retains session and exits `1` with recovery. Public discovery behavior itself is an izess acceptance concern. |

## 8. Execution and acceptance gates

- First add requirement-named tests that fail for the documented missing
  behavior. Do not weaken old regression assertions unrelated to this design.
- After matrix approval, repeat the existing-test conflict audit and obtain an
  explicit resolution before editing contradictory tests.
- Run focused packages while developing, then `go test ./...` for
  `frontend/cli-go`. Run applicable platform credential-store tests on their
  native OS; mocks do not prove Windows Credential Manager, Keychain, or Secret
  Service integration.
- Run `go test -race` for portable affected packages on a supported race build.
  If a platform/toolchain cannot build it, report that limitation rather than
  calling repeated ordinary runs a race result.
- Measure fresh per-package and aggregate statement coverage for affected CLI
  production sources. Target 100%, minimum 95%. Inspect the per-line report from
  the files with most uncovered lines and obtain approval for any follow-up
  coverage plan required by the repository process.
- Regenerate and lint OpenAPI documentation. The client fake fixtures must use
  payloads valid under the approved schema, but sqlrs CLI tests do not substitute
  for izess route/gateway contract tests.

## Preliminary existing-test conflicts

This inventory is evidence for the post-approval audit; this document changes no
test yet.

| Existing test | Conflict to resolve after approval |
| --- | --- |
| `internal/app/init_test.go: TestInitRemoteRequiresToken`, `TestInitRemoteWritesProfile` | Replace normal static-token expectations with discovery-backed `remoteSession`; retain separate deprecated `--token` compatibility coverage. |
| `internal/app/user_org_test.go: TestParseUserArgsCreateDefaultsOIDCProvider` | Replace the `oidc` default with required `--identity-provider`; preserve all other create parsing assertions. |
| `internal/app/app_auth_test.go: TestParseAuthArgs` | Replace parser rejection of `login github` with acceptance of any syntactically valid provider ID; provider detail/adapter support decides availability later. |
| `internal/app/auth_runner_test.go` login fixtures and fake manager | Stop loading/passing workspace `clientID`/`clientSecret` and hard-coded Google URL; inject provider discovery and retain URL-before-success/output-order assertions. |
| `internal/authsession/google_http_test.go: TestGoogleOAuthClientExchangeRefreshAndRevoke` and manager client-secret assertions | Replace confidential-client assertions with generic public OIDC requests that contain neither form `client_secret` nor HTTP client authentication; preserve exchange/refresh/revoke method and response coverage. |
| `internal/authsession/pkce_test.go` Google URL and callback tests | Build from advertised endpoint/scopes/parameters and require RFC 9207 `iss`; preserve PKCE/state/OAuth-error/single-callback regressions. |
| `internal/authsession/claims_test.go` and manager token fixtures | Add required `iat`, nonempty `sub`, sole audience and `azp` cases; old “valid” fixtures missing `iat` must be repaired rather than weakening validation. |
| `internal/authsession/manager_test.go: TestResolveBearerTokenDeletesSessionOnRefreshFailure` and `manager_errors_test.go: TestResolveBearerTokenRefreshRejectsMissingOrInvalidIDToken` | Split by error taxonomy: only `invalid_grant` deletes; transport/status/claim/malformed-token failures retain the prior refresh credential and never call a protected API. |
| Credential-store tests in `store*_test.go` and `testCredentialKey` | Replace endpoint/provider/issuer/client-ID keying with profile + installation ID + normalized control endpoint; keep provider/account in the session value and preserve every platform round-trip/error assertion. Limit the empty-provider-to-Google fallback, if retained, to explicit legacy decoding. |
| `internal/config/config_test.go: TestAuthConfigLoadsOIDCSessionFields` | Preserve it as legacy read-compatibility evidence, but assert a read-only migration view; add normal `remoteSession` config tests proving provider metadata/secrets are absent. |
| `internal/client/users_orgs_test.go` and `internal/cli/commands_user_org_test.go` fixtures | Use a service provider ID such as `google` instead of adapter `oidc`, and include required canonical `Organization.endpoint`; preserve HTTP precondition, status/error, list/get, and renderer assertions. |
| Normal-flow tests using `auth.mode: oidcSession` or string token-source values | Move normal paths to `remoteSession` and the closed `TokenSource` values; keep dedicated legacy-alias tests for `oidcSession`. |

Approved resolution (2026-09-25, @evilguest): update the old expectations listed
above to the approved contract while preserving their unrelated regression
assertions. Do not change the newly approved matrix to accommodate obsolete RC
behavior.
