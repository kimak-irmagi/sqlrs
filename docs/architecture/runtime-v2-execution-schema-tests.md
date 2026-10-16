# Runtime v2 execution schema: test inventory

Status: **approved** by @evilguest, 2026-10-16, #150. The
[API/component structure](runtime-v2-execution-schema-structure.md) and refined
[flow](runtime-v2-execution-schema-flow.md) are approved. Approval includes the
narrow architecture-test exception and additive CI/test infrastructure changes.
Implementation/execution evidence is tracked separately; approval is not a
passing test result. Publication gates belong separately to #151.
Revised after critical review: cache outcomes, profile compatibility, protected
explanations, allocation evidence and mandatory CI execution are specified below.

## Scope and evidence rules

Tests exercise the existing Runtime Go module, not an executor. Use public
construction/read APIs in package-external consumer tests. Schema-author test
fixtures may construct malformed semantic trees, but ordinary consumers must
not import `schemaauthor`, `internal`, or `schemas/conformancev1`.

Use small deterministic fixtures and one relevant change per negative case.
Every Build/Parse rejection checks ErrInvalid/ValidationError and the documented
code/path class, and returns a zero result, not merely any error or a partial
object. Resolver failures check resolver.Error stage/code and zero Outcome;
explanation authorization failures check CodeAuthorizationDenied and an invalid
projection. A cache fallback is not a failed request. Compilation/architecture
checks use their own diagnostics, not these runtime error classes.
Cases distinguish generic wire/schema integrity from provider semantics: a
semantic mutant must have valid canonical encoding/commitments/checksums so that
the intended semantic check, rather than an unrelated checksum failure, rejects it.

Pin expected values independently from the implementation under test. Round trips
and calls to the same builder for both actual/expected are not sufficient proof.
No real Docker, registry, database, filesystem materializer or external network
is required. Temporary files for a clean-module consumer are test infrastructure,
not execution-provider I/O.

## A. Public interface and normal form

| ID | Required evidence |
| --- | --- |
| ES01 | An external package obtains the fixed schema, builds/parses an identity and uses root inspection without generic schema authoring. Provider/kind/schema, all seven required fields, kinds/disclosures and exact exported signatures match the approved contract. |
| ES02 | Valid minimal and representative identities cover host-executable, host-bundle, OCI and database-container, POSIX and Windows path/platform forms, with and without inputs/services/secrets. Each parses and rebuilds to the exact original canonical bytes/fingerprint. The compatibility matrix below tests all 27 known runtime/identity/network combinations through both Build and Parse. Target-platform fixtures produce the same fixed bytes on every CI host; they do not substitute the runner's OS/path rules. |
| ES03 | Independently reviewed fixed fixtures pin normalized typed output, canonical tree shape, extension bytes and fingerprint for representative OCI, partial host and secret-bearing profiles. Existing root canonical helpers may construct expected tree nodes, but expected execution-schema mapping is not produced by Build/Parse. At least one byte/fingerprint fixture is independently derived from the normative canonical grammar. |
| ES04 | Permuting keyed inputs/env/services/secrets, nil versus empty keyed collections, adjacent literal merging and redundant empty literals give the same normal form. Windows environment names normalize before duplicate checks; POSIX case remains significant. Ordered argv, text parts and mount order are not sorted away. |
| ES05 | Paired valid fixtures change one semantic dimension and show a changed canonical identity, excluding documented normalizations. Include runtime identity extent/digest/platform, argv/env/workdir/user, qualified source parts/profile/executable, mounts/access/order, network/service/transport/authentication profile and semantic secret reference parts. Where a change requires dependent references or a compatible invocation, update them coherently and prove both fixtures valid before comparing fingerprints. Invalid mutations belong in rejection tests, not identity-difference evidence. |

## B. Invocation, paths and sources

| ID | Required evidence |
| --- | --- |
| ES06 | Effective argv/env preserve order, an empty argument, a removed variable versus an empty variable, explicit user and workdir. Resolved image/project examples model Compose entrypoint/CMD outcomes without implementing or invoking Compose. Build performs no shell insertion, splitting, variable expansion, image defaulting or rewriting of literal paths. |
| ES07 | Reject empty argv, invalid first-word kinds/emptiness, a host executable not anchored at tool in native format, zero/unknown enums and non-applicable tagged fields. Semantic secrets cannot expand in argv. Valid prefixed path arguments and environment templates retain part order. |
| ES08 | Path tables cover every approved root and compatible runtime: mount, tool, workspace, POSIX/Windows image and service socket/directory/credential file. Check empty/nonempty Relative and native/forward-slash format. Reject malformed roots, missing roles/mounts, absolute or unclean Relative, backslash/NUL/dot traversal, incompatible runtime roots, forbidden mount destinations and known file-only workdirs. Unknown physical member existence/aliases are not inferred by the pure schema. |
| ES09 | Qualified sources validate provider/kind/schema/fingerprint and preserve every reference component. File mode 0644 versus 0755 differs through Executable; tree bindings require Executable=false and their explicit profile. Reject unknown/zero profiles, malformed references, duplicate names and unused inputs. Parse success asserts a source reference, never manifest existence or source acquisition authority. |

## C. Mounts, services and secrets

| ID | Required evidence |
| --- | --- |
| ES10 | Input/scratch/semantic-secret mount variants accept exactly their applicable fields and access policy. Reject wrong input/slot, inconsistent read-only flags, duplicate names/exact logical targets and target-reference cycles. Ordered nested targets are accepted; reversing overlapping layers changes identity. A mount-root path denotes the final visible namespace, not guaranteed provenance from a shadowed parent. No mount application is claimed. |
| ES11 | Service tables cover TCP host/port and Unix socket paths, database/user selection, database-container target binding, none/services network and declared network-target usage. Reject absent/duplicate/unused roles, unsupported selection/transport combinations, mismatched target runtime and wrong variant fields. A Unix target under none is structurally representable without asserting physical isolation. |
| ES12 | local-context is accepted only with host-executable and rejected for complete/image profiles. The partial profile remains identifiable after Build/Parse/cache round trip, independently of portability observations. Tests explicitly do not claim that this library implements State-cache admission. |
| ES13 | Semantic secrets require valid immutable non-secret provider/identifier/version references. Reject missing/zero/invalid references and unused/duplicate slots; reference-component changes alter identity. Secret environment parts and read-only file mounts work; file argv uses a path, not secret expansion. |
| ES14 | Authentication requires the exact service backlink, nonempty logical user and versioned profile, with one service per slot and no IdentityReference. Reject dangling/crossed bindings, missing profile/version, auth slots in generic text/mounts, semantic ServiceRole and other non-applicable fields. A syntactically valid but unknown provider profile is representable, not falsely admitted by the library. |

## D. Validation, resource bounds and ownership

| ID | Required evidence |
| --- | --- |
| ES15 | Parse rejects zero identity, wrong owner/kind/schema, missing/extra top-level fields, wrong kinds/disclosure and malformed closed nested shapes. Mutate every map/list level and tagged variant: missing/extra keys, wrong node kinds, unknown enum, duplicate keyed name and cross-reference failure. Both Build and Parse reject equivalent invalid semantic inputs. |
| ES16 | Construct otherwise valid generic identities containing non-normalized semantic trees: unsorted keyed data where representable, adjacent redundant literals, unnormalized Windows environment names and alternate scalar/profile forms. Parse rejects rather than repairs them. Generic canonical map order is already enforced by the root codec and is not confused with execution-specific normalization. |
| ES17 | Boundary tables cover per-string/key size, each raw collection limit, aggregate raw text and generated node/byte budgets. Test the exact maximum where a valid fixture can reach it, plus one, and combinations individually below limits but collectively over. Raw redundant literals cannot evade input limits by merging. Fixed schema shape may hit an aggregate limit before a per-field maximum; tests identify the actual applicable constraint rather than weakening another bound. Required preflight/allocation review evidence below supplements error-result tests; late rejection alone does not prove bounded allocation. |
| ES18 | Invalid UTF-8, NUL, invalid name/identifier/platform/digest grammar, duplicate normalized env names and invalid reference atoms fail without panic. Errors expose only static paths/indices and never caller names, secret-reference identifiers, private host paths or sentinel sensitive text; result values are zero on failure. |
| ES19 | Mutating caller slices/path pointers/reference pointers after Build cannot change identity. Mutating Parse output cannot alter the original or a later read. Parallel read/build/parse operations are deterministic and race-free. Nil/empty normalization does not alias retained caller buffers. |
| ES20 | Root Kind/StringValue/Elements/Entries/EncodedSize cover all five canonical node kinds, zero values, wrong-kind queries and empty/nonempty collections. Returned slices are defensive; map/set order is canonical; children remain immutable; EncodedSize equals existing CanonicalBytes length and is zero for invalid values. A separate warmed-up AllocsPerRun check requires zero allocations for EncodedSize on prebuilt zero/small/large values; no construction or CanonicalBytes call is inside the measured closure. Existing bytes/tokens/domains/depth/node counts remain unchanged. |

## E. Resolver integration and composition

| ID | Required evidence |
| --- | --- |
| ES21 | A fake provider registers the approved schema in the existing registry/manager, validates with Parse and resolves through the existing canonical result. A new manager/cache instance reads the same record, invokes semantic validation and preserves the exact identity. No parallel registry/result/cache format is introduced. |
| ES22 | Fresh and cached generic-valid but semantically invalid identities fail provider ValidateResolution. The cache case matrix below distinguishes request failure from discard-and-resolve fallback, and asserts exact returned/stored identity and ordered calls. Cached fixtures have valid key/checksum/commitments and matching descriptor, isolating nested semantic rejection. Acquire is never called by ResolveCurrent, even on success: its zero counter is only a boundary check, not evidence of rejecting a bad identity. |
| ES23 | Bind the identity through the existing transform composition helper; changing execution semantics changes the transform/derived StateID. Changing separate operational observations such as temporary physical paths, container IDs and endpoint addresses does not change canonical execution or transform identity. This is identity evidence, not approval for reusable-State publication. |
| ES24 | Canonical identity JSON/envelope and CacheRecord round trips preserve the seven-field shape, protected disclosures, normalized values and fingerprint. The protected-explanation cases below exercise the new schema through existing ExplainSafe/ExplainInternal. Existing published fixtures and canonical domains stay unchanged; no new result/wire version, schema migration or hidden normalization on decode is accepted. |

## F. Architecture, consumer and robustness gates

| ID | Required evidence |
| --- | --- |
| ES25 | Extend the architecture test's fixed capability exception only for zero-argument schemas/sqlrs.ExecutionEnvironmentSchema returning the existing immutable ExtensionIdentitySchema. Keep the existing workspace-file exception and all generic constructor/type/import restrictions. Negative cases still reject arbitrary schema parameters, generic re-exports and disallowed schemaauthor/conformance imports. No new third-party dependencies or execution I/O enter the facade. |
| ES26 | A staged local module/file-backed proxy consumer outside go.work, without replace/internal/schemaauthor/conformance imports, builds/parses, inspects and serializes the production identity, and composes it into a transform. This verifies PR source packaging/public accessibility, not publication of an immutable version. |
| ES27 | Seeded fuzz/property checks cover bounded typed inputs and decoded canonical identities: no panic, deterministic repeated results, Build→Parse→Build equality for accepted values and rejection of noncanonical semantic mutations. Root inspection fuzzing checks kind-consistent access and EncodedSize against existing bytes. Valid-profile and generic-valid/semantic-invalid seeds ensure that validation is reached rather than only early wire rejection. Fixed negative examples remain explicit tests, not dependent on random discovery. Named targets and CI commands below make fuzzing mandatory. |
| ES28 | Run the full nested-module suite and existing conformance/architecture/consumer gates, preserving existing tests and published expected values. Enforce the exact coverage thresholds and CI matrix below; publish detailed profiles and per-file diagnostics even on failure. New-code and module results are separate gates, not just displayed totals. Any additional coverage iteration follows the repository approval rules. |

## Compatibility cases (ES02, ES07, ES12, ES15)

The following nine known runtime/identity pairs, crossed with three network
modes, give 27 cases. "Accept" is structural validation, not provider admission.

| RuntimeKind | IdentityKind | none | services | local-context |
| --- | --- | --- | --- | --- |
| host | host-executable | Accept | Accept | Accept |
| host | host-bundle | Accept | Accept | Reject |
| host | image-manifest | Reject | Reject | Reject |
| oci | host-executable | Reject | Reject | Reject |
| oci | host-bundle | Reject | Reject | Reject |
| oci | image-manifest | Accept | Accept | Reject |
| database-container | host-executable | Reject | Reject | Reject |
| database-container | host-bundle | Reject | Reject | Reject |
| database-container | image-manifest | Accept | Accept | Reject |

Build each case with otherwise valid kind-specific argv/path/user, platform and
target settings; database-container has a declared DatabaseService, other kinds
have none. Services mode uses declared targets; none includes a representable
Unix-socket target where needed. For rejected pairs, select the invocation by
RuntimeKind so rejection is not caused by an unrelated path/command error.
Exercise Parse with generic-canonical identities independently assembled for
every case, since Build cannot produce the rejected cases. Add zero/unknown
values on each axis separately; preserve the other axes as valid.

## Resolver outcome cases (ES21–ES22)

Each case starts from isolated provider/cache state. Record ordered Load,
ValidateResolution, Revalidate, Resolve and Store events and the identity passed
to each. The cache wrapper must delegate JSON/key/checksum verification to the
existing codec; it must not inject an object that bypasses record decoding.
Observe Store calls separately from the preloaded record contents.

| Case | Expected result and effects after Normalize/Load |
| --- | --- |
| Cache miss, fresh semantic-invalid identity | Resolve → Validate rejects; resolver.Error operation validate_resolution/code invalid_resolution; zero Outcome; no Store. |
| Semantic-invalid cached identity, fresh valid identity | Validate cached rejects → Resolve → Validate fresh accepts → Store exactly the fresh identity once. Success has CacheHit=false, empty PriorStatus and reason invalid_cached_resolution. No Revalidate of the rejected record. |
| Semantic-invalid cached identity, fresh semantic-invalid identity | Validate cached rejects → Resolve → Validate fresh rejects; validation error and zero Outcome; no Revalidate/Store. The old physical record may remain but is never returned as usable. |
| Valid cached identity, CURRENT with valid updated evidence | Validate cached → Revalidate → Validate updated evidence → Store updated resolution. Return the cached identity with new evidence, CacheHit=true and PriorStatus=CURRENT; no Resolve. |
| Valid cached identity, CURRENT with invalid updated evidence | Second Validate rejects; validation error, zero Outcome and no Store/Resolve. This confirms evidence validation is separate from successful Parse. |
| Valid cached identity, STALE or UNKNOWN, fresh valid identity | Two distinct cases: Validate cached → Revalidate → Resolve → Validate fresh → Store fresh once. Return fresh identity, CacheHit=true and the original prior status/reason; never return the old identity as CURRENT. |

Provider validation calls Parse first and independently validates evidence.
Assert the specific nested ValidationError cause for semantic rejection, not an
unrelated decode/evidence error. Revalidate does not change an immutable identity;
only its returned evidence is substituted. Acquire stays zero in every case.
Do not alter the existing manager/cache contract to satisfy these tests.

## Protected explanation cases (ES18, ES24)

Construct an envelope for the production execution identity, directly through
the existing extension envelope API. Use distinct non-secret sentinel values in
the six protected fields, including qualified source and semantic-secret
reference components. Compute expected field names/visibility independently.

- ExplainSafe: primitive is public; all six protected fields have Redacted=true
  and no text, canonical_hex, secret_reference or field commitment payload.
  Inspect the parsed JSON fields, not just string absence: canonical trees are
  represented as hex, so searching for plaintext alone can miss disclosure.
- Authorized ExplainInternal: hex-decode canonical_hex and compare it to the
  CanonicalBytes of independently constructed expected trees/reference components;
  check exact field commitments too. No additional canonical byte decoder or
  root API is needed. Do not expect a scalar secret_reference for
  this schema's protected canonical map. No credential value exists in the API.
- Nil authorizer, explicit denial, authorizer error, canceled-before and
  canceled-during authorization: CodeAuthorizationDenied, invalid projection and
  no partial protected result. Confirm the expected authorizer call count/order.
- Full identity/envelope/cache serialization intentionally carries protected
  non-secret data; do not apply safe-diagnostic redaction rules to those internal
  round trips or mistake a reference identifier for the raw credential itself.

## Allocation/preflight evidence (ES17, ES20)

The EncodedSize allocation test uses prebuilt values and a result sink so the
call cannot be optimized away. Include a large legal string/tree. Replacing the
accessor with len(CanonicalBytes()) must fail the zero-allocation check. Source
review must additionally establish direct size access without a stack copy or
re-decoding; allocation counts alone do not prove absence of all copying.

For Build/Parse, a limit_exceeded result does not prove rejection preceded
allocation. Required code-review evidence traces each raw count/UTF-8 byte sum
and checked addition before proportional copy/sort/merge, then bounded public
node construction and aggregate metrics before final identity construction.
Parse must check top-level aggregate metrics before child traversal/copying.
Use bounded over-limit fixtures, including repeated small literals and shared
large strings whose aggregate exceeds the limit; do not allocate unbounded test
inputs. Record allocation diagnostics outside fixture construction, but impose
no arbitrary latency/heap threshold or zero-allocation claim on Build/Parse.
If the required ordering is not established, the review fails even if ES17
returns the expected error. No production hooks or copied encoder are added
solely to observe private implementation steps.

## Mandatory execution and coverage gates (ES27–ES28)

These are approved planned changes to the existing runtime-v2 CI job and release workflow,
not changes made or runs completed by this document. Keep all existing gates.

- Linux, macOS and Windows: run the full nested-module suite with a 10-minute
  per-package timeout and coverage profile. Run the same target-platform fixed
  fixtures on all three hosts, not platform-specific substitutes.
- Linux: run the full race detector with a 10-minute per-package timeout, plus
  existing dependency/conformance/architecture/staged-consumer checks.
- Linux PR/post-merge CI and the release workflow: add the three exact fuzz
  targets below alongside existing targets. Verify each target exists in its
  package before execution; a command with no matching fuzz test is not a pass.
  Existing release-workflow contract tests must assert these new invocations.

From backend/libs/runtime-go (GOWORK=off for the module checks):

```text
go test -count=1 -timeout=10m -coverpkg=./... -covermode=count -coverprofile=coverage-runtime-all.out ./...
go test -race -count=1 -timeout=10m ./...
go test -run '^$' -fuzz '^FuzzExecutionEnvironmentInput$' -fuzztime=2s -timeout=30s -parallel=2 ./schemas/sqlrs
go test -run '^$' -fuzz '^FuzzExecutionEnvironmentIdentity$' -fuzztime=2s -timeout=30s -parallel=2 ./schemas/sqlrs
go test -run '^$' -fuzz '^FuzzCanonicalValueInspect$' -fuzztime=2s -timeout=30s -parallel=2 .
```

The first fuzz target maps bounded bytes to typed input; the second exercises
schema-bound decoded/mutated canonical identities through Parse; the third
inspects root canonical values. Each has fixed valid and malformed seeds;
pre-filtering must not leave only early parse failures. Fuzz execution supplements,
not replaces, deterministic compatibility and rejection tests.

Coverage gates use exact covered/total instrumented statement counts from the
profile, not rounded go tool cover percentages: module >=95%; all new
implementation code together >=95%; and each new implementation file >=95%.
Target remains 100%. Expected new files are canonical_value_inspect_v1.go and
schemas/sqlrs/execution{,_validation,_value}.go; include any additional new
implementation files too. Report touched-package totals and per-line/block
uncovered locations separately. Go coverage measures instrumented statements,
not a literal percentage of text lines.

The coverage check must fail for a missing expected file, zero/absent measurable
data, or an exact value below 95% even if the displayed rounded value is 95.0%.
Its own deterministic checks cover those cases and the exact passing boundary.
Detailed profiles/diagnostics are retained with an always-run artifact step;
failing tests, fuzz/race jobs or coverage checks remain failures. Gate approval
is not permission to skip existing tests or to
perform an unapproved coverage iteration.

## Approved existing-test adjustments

`TestSchemaFacadesDoNotExposeGenericSchemaTypes` currently permits only
WorkspaceFileExtensionSchema to return a fixed schema capability. The new
approved accessor would otherwise fail this test. ES25 proposes the narrow
additional exception, with negative cases retaining the original trust boundary.
No current expected canonical bytes change. Existing workflow contract tests
also gain additive assertions for the new fuzz invocations and coverage gates;
their prior assertions remain. The fixed-schema exception is the only proposed
relaxation of an existing restriction.
After approval, perform the required full contradiction review
of existing tests before implementing new tests; ask again if another conflict
requires a choice.

## Deferred evidence: not delivered by these module tests

Source manifest verification/materialization, actual executable permissions,
image/Compose resolution, physical path projection, secure injection profile
admission, real network/mount isolation, tool execution and output handling
belong to consumers/providers. So do pre-lookup reuse gates, ineligibility
propagation, durable recovery and publication restrictions for partial hosts.
Consumers must test these before activation; a fake provider or stable StateID
in ES21–ES24 does not prove them. Immutable tag/proxy availability and a consumer
of that exact published version are #151 acceptance, not PR-stage evidence.

Implementation sequence after approval: contradiction review, test fixtures and
tests (expected missing-API failures), then code, full checks and coverage. No
existing test is skipped or weakened to make the implementation pass.

## Repeat review of this revision

This is a same-agent review of the test design, not independent review or an
execution result. The five findings are addressed by the detailed cache cases,
27-case compatibility matrix, protected explanation assertions, allocation plus
source-review evidence, and mandatory CI/coverage gates above.

The second pass also corrected three ambiguities: use error contracts specific
to each layer instead of requiring every failure to be ErrInvalid; permit
coherent dependent-reference updates in valid identity-difference pairs; and
explicitly include additive changes to existing workflow contract tests rather
than claiming that the architecture exception is the only existing-test edit.
Authorized explanation assertions compare existing canonical bytes without
adding a decoder or an implementation dependency to the facade.

No further blocking issue was identified in this revised inventory. Source-owner,
executor and State-reuse evidence remains explicitly outside the pure module;
those omissions are not claimed as tested. The inventory, narrow architecture
exception and additive CI/test infrastructure changes were subsequently approved
on 2026-10-16. Tests precede implementation; approval alone proves no execution.

## Publication checks (#151, proposed)

These reuse existing module checks without weakening ES01-ES28 or adding runtime
production code. The new automation has a separate
[test inventory](runtime-go-module-publication-tests.md). Both inventories were
approved on 2026-10-16 for #151's
[proposed release sequence](runtime-v2-execution-schema-structure.md#10-proposed-module-publication-151).
The #150 implementation and its merged CI results are not themselves evidence
that a tagged module can be consumed.

The requested S below is retained. Actual token permissions are qualified by
the first approved real RC run, not a read-only/mocked preflight. A denial
reports partial state without changing S or falling back to another token.

| ID | Required evidence |
| --- | --- |
| EP01 | Review the additive API difference from v0.5.0; v0.6.0 notes name the fixed schema and inspection methods, unchanged canonical/resolver/cache contracts, no I/O, and separate provider/reuse obligations. Existing release-contract markers remain documented with their historical/current meanings. No existing tag is reused. |
| EP02 | S is the reviewed merge commit 892a5007adf4d499fbfbd7861569861ff11c43a8 in main, independent of newer automation H. Its post-merge CI and current source validation pass: full Go suite, exact >=95% coverage, race, fuzz, conformance, dependency boundary and staged consumer. No new release-note source file/commit is required. Automation checks use H. |
| EP03 | New nested-module RC and final tags both resolve to S. Protected update/deletion rules remain active. Release attestations name the correct module, version/tag, S and unchanged conformance manifest; asset checksums validate and assets are not overwritten. |
| EP04 | For each published version, an external consumer with GOWORK=off, public GOPROXY and GOSUMDB downloads that exact immutable version without replace, sibling sources or a pseudo-version. Module and go.mod checksums are present. The existing public-facade fixture builds/parses, inspects, serializes and composes the execution identity. A staged-proxy run alone is insufficient. |
| EP05 | Both manual candidate and final runs finish successfully, with public-proxy and retraction/legacy checks; final promotion fully reverifies the published candidate in a mandatory read-only job of the current run, not from its tag or a past report alone. Distinct versioned zip sums do not imply different S. Source corrections need a new candidate; harness-only fixes use H and read-only verify, never retagging. |
| EP06 | GitHub release/run evidence and the final issue report record v0.6.0, full S/H, release/CI links and clean public-consumer proof. Only then close #151. A version-specific roadmap/source PR is not a gate; application adoption remains separate. |
