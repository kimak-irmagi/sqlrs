# Runtime v2 alias consumer publication decisions

Status: **Obsolete**. Superseded by the
[2026-10-03 client-alias-boundary ADR](2026-10-03-runtime-v2-client-alias-boundary.md).
These #140 proposals were not implemented or published; sqlrs#140 was closed
as not planned. This record is retained only as decision history.

## Decision 1: publish in the Runtime Go release cycle

Conversation timestamp: 2026-10-02 23:19:38 Asia/Novosibirsk
(2026-10-02 16:19:38 UTC). GitHub user ID: @evilguest (41718235).
Agent: OpenAI Codex (GPT-5).

Question: should the YAML and legacy-alias consumer boundary from issue #137 be
published as a separate nested module or in the existing Runtime Go module?

Alternatives considered:

1. Publish a dedicated `frontend/libs/alias-go` nested module with its own tags,
   workflow, checksums, and compatibility lifecycle.
2. Publish a public package in `backend/libs/runtime-go` and release it through
   the existing Runtime Go RC/GA lifecycle.
3. Split the standard-library compatibility policy into `runtime-go` and place
   only the YAML transport in a second facade module.

Decision: publish the complete boundary in
`github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go` beginning with v0.6.0.
The existing protected nested tags, same-commit RC/GA promotion, public proxy,
checksum database, release notes, and provenance workflow remain the single
release lifecycle.

Rationale: one release version and checksum identity are more important than
preserving a separate dependency lifecycle. A split artifact would add a second
compatibility matrix, while publishing the complete CLI module would expose an
unnecessarily broad process-oriented API.

## Decision 2: retain the `aliasruntimev2` package name

Conversation timestamp: 2026-10-02 23:19:38 Asia/Novosibirsk
(2026-10-02 16:19:38 UTC). GitHub user ID: @evilguest (41718235).
Agent: OpenAI Codex (GPT-5).

Question: should the public package be named `legacyaliasv1` or retain the
existing #137 name `aliasruntimev2`?

Alternatives considered: `legacyaliasv1`, `aliasruntimev2`, and an unversioned
generic `aliascompat` name.

Decision: use package and directory name `aliasruntimev2`, with import path
`github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/aliasruntimev2`.

Rationale: the name matches the reviewed #137 boundary and describes its Runtime
v2 purpose. `legacyaliasv1` would imply a package API version that does not match
the actual independently versioned alias-document schema and would suggest an
unwanted family of `legacyaliasvN` packages.

## Decision 3: narrow the Runtime Go dependency gate

Conversation timestamp: 2026-10-02 23:19:38 Asia/Novosibirsk
(2026-10-02 16:19:38 UTC). GitHub user ID: @evilguest (41718235).
Agent: OpenAI Codex (GPT-5).

Question: how should Runtime Go host the strict YAML decoder while its existing
architecture and CI require standard-library-only dependencies?

Alternatives considered: preserve the standard-library-only rule and publish a
separate module; implement or vendor a private YAML parser; permit all external
dependencies; or allow exactly the already reviewed YAML implementation.

Decision: replace the module-wide standard-library-only rule in v0.6.0 with a
closed allowlist containing packages from Runtime Go itself, the Go standard
library, and exactly `gopkg.in/yaml.v3 v3.0.1`. Only `aliasruntimev2` may import
YAML. Existing semantic, composition, resolver, schema, and conformance packages
remain free of external imports. CI and release gates must reject every other
non-standard package.

Rationale: strict YAML parsing is part of the required public contract, and a
private parser would add security and conformance risk. A one-entry allowlist
keeps the dependency expansion explicit and bounded while preserving the chosen
single release cycle.

This decision partially supersedes the no-YAML-dependency portion of
[Runtime v2 composition Decision 2](./2026-09-27-runtime-v2-composition.md#decision-2-independently-versioned-strict-document-and-yaml-boundary).
The `composition` package itself remains standard-library-only and continues to
own only strict JSON and immutable constructors.

## Decision 4: public compatibility API and opaque result

Status: partially superseded by
[Decision 6](#decision-6-separate-filesystem-binding-provider-output-and-final-result).

Conversation timestamp: 2026-10-03 10:34:20 Asia/Novosibirsk
(2026-10-03 03:34:20 UTC). GitHub user ID: @evilguest (41718235).
Agent: OpenAI Codex (GPT-5).

Question: which public types should replace the CLI-internal alias model, and
should the existing opaque `Result` name and contract be retained?

Alternatives considered: expose CLI-internal types; flatten all legacy fields
into `TranslationInput`; add explicit `LegacyClass` and `LegacyDefinition`
values; rename `Result` to `TranslationResult`; or return a declaration plus an
error and encode ordinary lack of support as an error.

Decision: expose `LegacyClass`, `LegacyDefinition`, `TranslationInput`,
`ImageSource`, `ProviderAdapter`, `Status`, `ReasonCode`, and the opaque custom
type `Result` from `aliasruntimev2`. `Result` retains checked constructors and
read-only accessors and represents exactly one of `translated` or `legacy_only`.
Ordinary lack of Runtime v2 support remains a result rather than an error. The
later review retains the name and final two-state result but removes
`ImageSource` and final-result constructors from the provider-facing surface.

Rationale: the dedicated legacy DTO prevents imports of CLI internals while
keeping the provider contract explicit. The package-qualified name
`aliasruntimev2.Result` is concise and preserves the reviewed #137 vocabulary;
checked construction prevents mixed or incomplete outcomes.

## Decision 5: CLI facade, data ownership, and publication bootstrap

Status: partially superseded by
[Decision 6](#decision-6-separate-filesystem-binding-provider-output-and-final-result)
and [Decision 8](#decision-8-stage-checksums-and-release-evidence-in-their-observable-order).

Conversation timestamp: 2026-10-03 10:34:20 Asia/Novosibirsk
(2026-10-03 03:34:20 UTC). GitHub user ID: @evilguest (41718235).
Agent: OpenAI Codex (GPT-5).

Question: how should the CLI preserve its internal boundary while the public
implementation and the CLI dependency on runtime-go v0.6.0 are published from
the same repository commit?

Alternatives considered: delete the CLI package and expose public DTOs
throughout the application; duplicate the implementation; publish Runtime Go
first and migrate the CLI in a later source commit; or retain a thin internal
facade and use the workspace during RC validation followed by standalone CLI
verification after GA publication.

Decision: retain `frontend/cli-go/internal/alias/runtimev2` as a conversion and
adapter facade over the public package. It owns no duplicate YAML, validation,
classification, or result logic. All package state remains request-scoped and
in memory. PR and RC source checks use `go.work`; after the same commit is tagged
as runtime-go v0.6.0 GA, the release gate verifies the CLI and clean consumer
with `GOWORK=off`, no sibling checkout, and no `replace`.

Rationale: one implementation remains authoritative, existing CLI-internal
callers keep their boundary, and the single-release-cycle decision does not
require a temporary duplicate implementation or a second public artifact.

## Decision 6: separate filesystem binding, provider output, and final result

Conversation timestamp: 2026-10-03 12:38:03 Asia/Novosibirsk
(2026-10-03 05:38:03 UTC). GitHub user ID: @evilguest (41718235).
Agent: OpenAI Codex (GPT-5).

Question: how should the reviewed API avoid filesystem I/O in Runtime Go,
machine-dependent provider inputs, and providers forging common compatibility
classifications?

Alternatives considered:

1. Move the CLI `pathutil` behavior, including `filepath.EvalSymlinks`, into
   Runtime Go and relax the no-filesystem boundary.
2. Keep absolute paths and `ImageSource` in the public input but rely on
   documentation and adapter conformance tests to prevent their use in a
   declaration.
3. Keep filesystem and configuration binding in the CLI, expose only logical
   `SourceID` and effective image publicly, give adapters a smaller
   `ProviderInput`, and separate `ProviderResult` from final `Result`.

Decision: choose alternative 3. Public `TranslationInput` contains
`LegacyDefinition`, `SourceID`, and `EffectiveImage`. The adapter receives only
copied arguments, `SourceID`, and effective image; its `Kind()` already owns the
handshake. CLI path
canonicalization, symbolic-link handling, workspace containment, and image
source precedence stay in the CLI facade. Providers construct only a translated
or unsupported `ProviderResult`; only `TranslateLegacy` constructs final
`Result` values and owns the three stable final reason codes.

Rationale: this preserves CLI behavior without filesystem access in Runtime Go,
removes machine-local data from the provider's deterministic input, and makes
classification ownership enforceable by the Go type system rather than by a
postcondition that accepts provider-forged common reasons.

## Decision 7: publish structured errors and state YAML parser limits precisely

Conversation timestamp: 2026-10-03 12:38:03 Asia/Novosibirsk
(2026-10-03 05:38:03 UTC). GitHub user ID: @evilguest (41718235).
Agent: OpenAI Codex (GPT-5).

Question: should external consumers parse error strings, and do YAML depth/node
limits bound `yaml.v3` before it constructs a node tree?

Alternatives considered:

1. Leave all errors as package-owned strings and describe every YAML limit as a
   parser resource bound.
2. Expose implementation-specific YAML errors and provider text directly.
3. Publish a small stable `ErrorCode`/field taxonomy and sentinels, preserve
   nested causes without echoing their values, and distinguish the pre-parse
   byte limit from post-parse depth/node acceptance limits.

Decision: choose alternative 3. Package validation errors match `ErrInvalid`
and use `invalid_yaml`, `limit_exceeded`, `invalid_input`, `adapter_mismatch`,
or `adapter_contract`. Provider operational failures match `ErrAdapter` and
their original cause, with bounded package-visible text. `MaxYAMLBytes` is the
pre-parser bound; depth and node limits apply to the constructed tree and are
not claimed as `yaml.v3` memory ceilings.

Rationale: consumers gain machine-readable failure classes without depending
on prose, diagnostics remain bounded and value-free, and the security claim
matches the actual parser boundary rather than overstating post-parse checks.

## Decision 8: stage checksums and release evidence in their observable order

Conversation timestamp: 2026-10-03 12:38:03 Asia/Novosibirsk
(2026-10-03 05:38:03 UTC). GitHub user ID: @evilguest (41718235).
Agent: OpenAI Codex (GPT-5).

Question: how can a synthetic staged module retain dependency checksum
verification, and when can an attestation truthfully bind public module sums?

Alternatives considered:

1. Continue using `GOSUMDB=off` for the entire staged dependency graph and
   generate the attestation before public-proxy download.
2. Stage every external dependency and assert locally computed values as public
   proxy sums.
3. Use a public-proxy fallback and a checksum exception scoped only to the
   synthetic first-party module; verify a public RC before GA authorization;
   create an explicitly versioned v2 attestation only after public download.

Decision: choose alternative 3. The tag ruleset rejects direct human creation,
update, and deletion and allows only the release automation identity to create
nested Runtime Go tags. Pre-tag checks cause that automation to create RC. A
publicly verified same-commit RC authorizes it to create GA. Staged consumers use
`GOPROXY=file://<staged>,https://proxy.golang.org`, the public checksum database,
and `GONOSUMDB` limited to the synthetic Runtime Go module. RC and GA tag jobs
obtain public sums first and only then create
`sqlrs.runtime-go.release-attestation.v2` completion evidence.

Rationale: YAML remains checksum-verified, evidence contains values actually
observed from the public service, and workflow language no longer claims that a
job triggered by a tag can prevent that already-created tag. Immutable content
defects require a new version; external or harness failures may be explicitly
re-verified.

## Decision 9: enforce the public surface and observable boundaries directly

Conversation timestamp: 2026-10-03 12:38:03 Asia/Novosibirsk
(2026-10-03 05:38:03 UTC). GitHub user ID: @evilguest (41718235).
Agent: OpenAI Codex (GPT-5).

Question: which tests can reliably enforce an immutable API, a thin CLI facade,
and concurrency behavior without asserting unprovable implementation details?

Alternatives considered:

1. Rely on a compile-only consumer, syntax heuristics for duplicate logic, and
   an unconditional concurrency-safety claim.
2. Freeze the entire implementation and compare source files byte-for-byte.
3. Pin exports with `go/types`, enforce imports and observable delegation,
   review semantic duplication as code, and test concurrency only with distinct
   outputs and a concurrency-safe adapter.

Decision: choose alternative 3. Same-package tests are allowed for private
invalid states and exact node accounting; the test design says so explicitly.
The staged and public consumer use the same checked-in sources, while unit tests
retain the exhaustive rejection matrix.

Rationale: the resulting checks fail on actual contract drift, avoid brittle
source-shape assertions, and do not promise safety for shared mutable targets or
consumer adapters outside the package's ownership.
