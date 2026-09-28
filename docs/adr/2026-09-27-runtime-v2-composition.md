# Runtime v2 alias composition decisions

Conversation timestamp: 2026-09-27 23:37:03 Asia/Novosibirsk
(2026-09-27 16:37:03 UTC). GitHub user ID: @evilguest (41718235).
Agent: OpenAI Codex (GPT-5). Approval followed by critical design review in the
same conversation.

Status: accepted for issue #109.

## Decision 1: separate engine-neutral composition package

Question: where should Runtime v2 alias expansion live?

Alternatives: current CLI `internal/alias`; local-engine internals; the root
semantic package; an opt-in public subpackage importing the root Runtime v2
model.

Decision: add `backend/libs/runtime-go/composition`. It depends only on the root
`runtimev2` package and the Go standard library. The root package does not import
it. Expansion produces existing `RecipeDeclaration` and
`TransformDeclarationDocument` values before resolution.

Rationale: local, shared, and external callers can share one deterministic graph
contract without coupling semantic identity to CLI files, YAML, storage, or
execution.

## Decision 2: independently versioned strict document and YAML boundary

Question: how should the user-facing alias schema be versioned and decoded?

Alternatives: reuse `sqlrs.runtime.v2` without a document revision; decode YAML
inside the public module; define `sqlrs.runtime.v2.aliases.v1` with strict JSON
and constructors, leaving YAML to an adapter.

Decision: use `schema_version: sqlrs.runtime.v2.aliases.v1`. The public package
owns strict bounded JSON and immutable constructors. A future CLI adapter maps a
closed YAML subset into the same constructors and rejects duplicate keys,
anchors, aliases, merges, custom tags, multiple documents, and unknown members.

Rationale: alias transport can evolve without reinterpreting the Runtime v2
identity schema, and the public module retains one minimal trust boundary with
no YAML dependency.

## Decision 3: flat explicit catalog without precedence

Question: how are references resolved across one or more alias documents?

Alternatives: filesystem discovery with nearest-file precedence; explicit
imports and relative references; an explicit caller-supplied catalog with one
flat exact-name namespace.

Decision: callers supply source documents explicitly. Names use lower-case
Runtime identifier syntax and resolve exactly in one flat case-sensitive
namespace. There is no source precedence. Duplicate names across documents are
`ambiguous_reference`; missing names are `missing_reference`. File discovery,
imports, and workspace lookup are deferred.

Rationale: expansion is deterministic and portable without embedding an
unapproved CLI discovery policy or allowing input-order-dependent shadowing.

## Decision 4: recipe nesting only through the base prefix

Question: where may a recipe reference another recipe?

Alternatives: allow recipes in any step and reconcile factories; allow
factory-less recipe fragments; permit one recipe only as `base.recipe` and allow
only transforms in steps.

Decision: a recipe base contains exactly one of an inline factory or a recipe
reference. A nested recipe contributes its complete factory and ordered
transform sequence as a prefix. Steps contain transform references or inline
transforms only. A recipe used as a step is `wrong_reference_kind`.

Rationale: the rule naturally represents factory-only recipes and reusable
ordered prefixes, requires no unresolved factory-equivalence comparison, and
prevents exponential graph fan-out.

## Decision 5: alias names and traces remain non-semantic

Question: how can expansion remain explainable without alias spelling affecting
logical identity?

Alternatives: copy alias/source names into declaration references or attributes;
drop source information; return an expanded declaration plus a separate
diagnostic trace.

Decision: expansion returns the existing declaration and a separate immutable
trace mapping output positions to source IDs, alias chains, and structural
paths. Alias and source data are never injected into declarations, resolver
keys, resolved identities, or fingerprints. Structural equality of expanded
declarations defines equal expanded semantic form.

Rationale: explain can show how a recipe was assembled while renaming aliases
without changing the fully expanded declaration cannot alter State identity.

## Decision 6: no parameters or implicit inheritance in schema v1

Question: should alias references support parameters, overrides, or config
defaults?

Alternatives: free-form inheritance; typed parameters with override precedence;
an explicit first schema with defaults materialized before expansion.

Decision: v1 has no parameters, overrides, imports, or implicit inheritance. A
compatibility adapter must first apply existing legacy precedence and supply the
selected explicit declaration input. The composition package never reads config
or environment state.

Rationale: the same document and explicit catalog always produce the same
declaration, independent of machine or invocation context.

## Decision 7: conservative legacy compatibility and no cutover

Question: how should existing simple prepare aliases participate before
provider-specific Runtime v2 declaration adapters are complete?

Alternatives: approximate `kind`/`image`/`args` conversion; silently run expanded
declarations through the legacy executor; translate only complete provider-aware
inputs and otherwise report legacy-only status.

Decision: generic composition never understands legacy fields. A CLI adapter may
translate only a fully bound alias for which provider adapters can construct a
complete deterministic Runtime v2 declaration. Otherwise it returns structured
`legacy_only` diagnostics with an actionable reason. Existing legacy commands
continue unchanged, and neither their records nor execution are reinterpreted as
Runtime v2.

Rationale: a superficially valid declaration must not mix Runtime v2 identity
with legacy physical execution invariants.

## Decision 8: bounded atomic expansion and additive delivery

Question: what resource, error, and delivery guarantees apply?

Alternatives: recursive unbounded traversal with free-form errors; partial
output; iterative bounded all-or-nothing expansion delivered beside the current
runtime.

Decision: use iterative visit states, checked limits, complete cycle chains,
stable error codes, defensive copies, and no partial result. The public package
lands as an opt-in additive API in the next available minor release. A later CLI
adapter is independently mergeable and does not connect composition to the
default prepare/runtime path. No release number is reserved before the preceding
approved line is published.

Rationale: adversarial graphs cannot exhaust the stack or produce partially
trusted recipes, while every merged PR remains buildable and supported.

## Decision 9: versioned linear-size expansion trace

Refinement timestamp: 2026-09-27 23:52:59 Asia/Novosibirsk
(2026-09-27 16:52:59 UTC). GitHub user ID: @evilguest (41718235).
Agent: OpenAI Codex (GPT-5).

Question: how should explain diagnostics be serialized without making alias
names semantic or causing quadratic growth?

Alternatives: leave the trace format unversioned; repeat a complete alias chain
for every transform; keep trace in memory only; define a versioned dense node
table with parent links and indexed origins.

Decision: define strict schema
`sqlrs.runtime.v2.alias-expansion-trace.v1`. A dense node table stores each
visited alias occurrence with a parent-node ID. Factory and ordered transform
origins reference nodes and RFC 6901 JSON Pointers. Non-target nodes also retain
the reference-site pointer in their parent source. Nodes are allocated in a
normative target/base-chain/output-transform order, and repeated references are
distinct occurrences. Dense IDs, parent ordering, origin indexes, variant
shape, and a 32-MiB transport limit are validated. Opaque node and origin
values expose defensive-copy accessors, so callers do not need to parse the
wire form to render an explanation.

Rationale: callers receive a restart-safe explain format whose size is linear in
visited aliases plus output transforms, while trace bytes remain outside every
resolver, identity, and fingerprint input.

## Decision 10: provider-owned path bases

Refinement timestamp: 2026-09-27 23:52:59 Asia/Novosibirsk
(2026-09-27 16:52:59 UTC). GitHub user ID: @evilguest (41718235).
Agent: OpenAI Codex (GPT-5).

Question: should an alias document location implicitly rebase declaration paths?

Alternatives: rebase every apparent path against the alias source; make all
paths alias-file-relative; treat declarations as opaque and retain each provider
contract's path base.

Decision: composition never interprets or rebases declaration fields from a
source file or `SourceID`. Provider specifications own their bases; the #108
workspace-file path remains workspace-root-relative. Provider-aware authoring
adapters materialize fields before document construction. The legacy adapter
applies existing alias-file-relative binding and default precedence before
constructing an explicit declaration.

Rationale: the generic layer cannot safely identify provider path fields, and
moving a diagnostic source must not silently alter semantic input.

## Decision 11: deterministic source identity, errors, and aggregate limits

Refinement timestamp: 2026-09-27 23:52:59 Asia/Novosibirsk
(2026-09-27 16:52:59 UTC). GitHub user ID: @evilguest (41718235).
Agent: OpenAI Codex (GPT-5).

Question: what additional constraints make multi-document diagnostics and
resource use independently reproducible?

Alternatives: accept repeated source IDs and dotted paths; limit each document
only; require unique bounded logical source IDs, JSON Pointers, and aggregate
catalog/output budgets.

Decision: source IDs are unique per catalog, non-secret bounded UTF-8; the CLI
uses workspace-relative slash-separated IDs rather than absolute host paths.
Errors expose stable code, optional source ID, RFC 6901 pointer, and bounded
cycle/candidate accessors. Public constants bound aliases, source documents,
source-ID bytes, aggregate catalog bytes, declaration JSON, and trace JSON.
Both decoded and constructor-created documents are measured in canonical JSON,
so constructors cannot bypass the per-document transport limit. Checked
accounting completes before returning either declaration or trace.

Rationale: ambiguity ordering has no input-order tie, dotted alias names cannot
make locations ambiguous, and many individually valid documents cannot amplify
into unbounded catalog memory or transport output.

## Decision 12: compatibility classification is required for issue closure

Refinement timestamp: 2026-09-27 23:52:59 Asia/Novosibirsk
(2026-09-27 16:52:59 UTC). GitHub user ID: @evilguest (41718235).
Agent: OpenAI Codex (GPT-5).

Question: may legacy compatibility remain an optional follow-up after the public
composition package lands?

Alternatives: defer it outside #109; approximate missing provider semantics;
require a separate mergeable compatibility slice before closing #109.

Decision: #109 remains open until the CLI compatibility slice deterministically
translates a fully supported legacy alias or returns an actionable structured
`legacy_only` reason. Missing provider support yields
`runtime_v2_provider_unavailable`; it never authorizes approximate identity or a
Runtime-v2/legacy-executor hybrid. The slice depends on a published immutable
composition-module version and does not alter current execution. Its first
version translates prepare aliases only; run presets are classified as
`legacy_semantics_unsupported`, because legacy run execution is not necessarily
an identity-bearing Runtime v2 transform. The result API enforces an exclusive
translated-declaration or legacy-only-reason state.

Rationale: the acceptance criterion is satisfied honestly while PR and release
sequencing remain independently buildable.

## Decision 13: trace nodes represent reference occurrences

Refinement timestamp: 2026-09-28 00:12:41 Asia/Novosibirsk
(2026-09-27 17:12:41 UTC). GitHub user ID: @evilguest (41718235).
Agent: OpenAI Codex (GPT-5).

Question: how can a persisted trace identify both a reused definition and the
specific edge that selected it?

Alternatives: deduplicate nodes by alias definition and retain no use site;
repeat complete chains at every output; allocate one compact node per reference
occurrence with definition and parent-reference pointers.

Decision: nodes are occurrence records. The target is node zero, recipe-prefix
nodes follow down to the factory, and named transform nodes follow in output
order. Every non-target node carries an RFC 6901 `reference_pointer` in its
parent source as well as its own definition `pointer`. Repeated uses therefore
remain distinct without repeating whole chains.

Rationale: explain output can locate the exact authoring edge, remains
deterministic, and retains linear size.

## Decision 14: bound definitions and canonicalize catalog failures

Refinement timestamp: 2026-09-28 00:12:41 Asia/Novosibirsk
(2026-09-27 17:12:41 UTC). GitHub user ID: @evilguest (41718235).
Agent: OpenAI Codex (GPT-5).

Question: what must the catalog count and in what order should it reject
multiple simultaneous defects?

Alternatives: bound only distinct names and report the first input-order error;
rely only on encoded bytes; bound total definitions and validate a canonical
source/name order with explicit precedence.

Decision: `MaxAliases` bounds all definitions, including competing definitions
of one name. Source count is checked first; sources are then sorted by raw
unsigned bytewise source ID before ID validity/duplication, document validity,
and aggregate definition/byte accounting. Alias ambiguity is checked last in
unsigned bytewise alias-name/source-ID order. Trace nodes and diagnostic
references have explicit public count bounds. Constructed empty documents and
catalogs are valid, while zero values remain invalid.

Rationale: repeated definitions cannot amplify decoded memory, and equivalent
catalog sets select the same error regardless of caller slice order.

## Decision 15: executable prepare-only compatibility contract

Refinement timestamp: 2026-09-28 00:12:41 Asia/Novosibirsk
(2026-09-27 17:12:41 UTC). GitHub user ID: @evilguest (41718235).
Agent: OpenAI Codex (GPT-5).

Question: what exact API prevents the compatibility slice from returning an
ambiguous or approximate outcome?

Alternatives: leave translation as prose; return a declaration plus optional
reason fields; expose an opaque exclusive result with checked constructors and
limit the first translation slice to semantically compatible prepare aliases.

Decision: `TranslateLegacy` and each provider adapter return an opaque `Result`.
Checked constructors create either a valid recipe declaration or a non-empty
legacy-only reason and remedy. Ordinary lack of support is a result, not an
error. The first slice translates prepare aliases only; run presets receive
`legacy_semantics_unsupported` and remain on the legacy execution path. The
adapter exposes its supported kind for an exact handshake, and effective-image
provenance uses a closed diagnostic enum. The subpackage is named
`aliasruntimev2`, and its parent does not import it.

Rationale: callers must handle one explicit state, unsupported workflows remain
actionable, and the package layout does not introduce an import cycle.

## Decision 16: closed, self-validating trace grammar

Refinement timestamp: 2026-09-28 18:04:32 Asia/Novosibirsk
(2026-09-28 11:04:32 UTC). GitHub user ID: @evilguest (41718235).
Agent: OpenAI Codex (GPT-5).

Question: which occurrence graphs and origin relationships may a decoded trace
accept without access to the source alias documents?

Alternatives: validate only dense IDs and RFC 6901 syntax; accept arbitrary
parent trees; define a canonical recipe-chain/transform-leaf grammar whose
pointers and output order are self-validating.

Decision: recipe nodes are one contiguous target-to-factory chain, followed by
named-transform leaves in output order. The deepest recipe owns the factory;
each transform leaf is consumed exactly once; inline origins point to recipe
nodes; step indexes are contiguous within each recipe from deepest to target.
Definition, reference, factory, named-transform, and inline-transform pointers
have canonical schema-derived forms. A standalone trace contains exactly one
transform node and one origin. Unused nodes and alternate pointer spellings are
invalid.

Rationale: strict persisted traces cannot claim impossible provenance, while a
decoder can validate them without loading mutable source files.

## Decision 17: first failure follows expansion traversal

Refinement timestamp: 2026-09-28 18:04:32 Asia/Novosibirsk
(2026-09-28 11:04:32 UTC). GitHub user ID: @evilguest (41718235).
Agent: OpenAI Codex (GPT-5).

Question: which error wins when one graph contains cycles, bad references, and
projected output-limit violations?

Alternatives: prevalidate the whole graph in a separate pass; assign a global
error-code priority; return the first failure in the normative expansion walk.

Decision: validate the requested target, walk the full base chain, then visit
steps from deepest recipe to target in source order. Resolve and validate the
current reference before checking its projected output/trace bounds. Stop at the
first failure; later unvisited defects do not replace it.

Rationale: one bounded pass remains deterministic, matches output order, and
does not require building or validating unreachable partial results.

## Decision 18: bound diagnostic text and the YAML trust boundary

Refinement timestamp: 2026-09-28 18:04:32 Asia/Novosibirsk
(2026-09-28 11:04:32 UTC). GitHub user ID: @evilguest (41718235).
Agent: OpenAI Codex (GPT-5).

Question: what prevents diagnostic strings and pre-constructor YAML parsing from
bypassing aggregate resource limits or coercing unintended values?

Alternatives: rely only on total JSON bytes and YAML-library defaults; allow YAML
scalar coercion; add explicit pointer/error/message and raw-YAML byte/node/depth
bounds with a string-only logical scalar subset.

Decision: pointers, rendered error text, and compatibility messages receive
explicit byte limits. The CLI checks raw YAML bytes before parsing, then walks
the parsed node tree iteratively with fixed node/depth limits. Only map, sequence,
and string tags required by the logical schema are accepted; directives,
coercible non-string scalars, graph/merge features, custom tags, and multiple
documents fail. The canonical Runtime document limit is checked independently.

Rationale: both transport stages are bounded and YAML presentation cannot alter
the strict JSON/constructor semantics.

## Decision 19: defensive adapter boundary and provider conformance

Refinement timestamp: 2026-09-28 18:04:32 Asia/Novosibirsk
(2026-09-28 11:04:32 UTC). GitHub user ID: @evilguest (41718235).
Agent: OpenAI Codex (GPT-5).

Question: how should compatibility handle nil/misbehaving adapters and prove
real provider path/default semantics rather than only wrapper behavior?

Alternatives: trust every adapter result; validate only a fake adapter; reject
nil/invalid outcomes at the wrapper and require a reusable conformance suite for
each production adapter.

Decision: run aliases, provider absence (including typed nil), default absence,
kind mismatch, and adapter invocation have explicit precedence. Image/source
combinations are closed. Adapter errors discard any accompanying result; nil
errors require a revalidated non-zero opaque result. Every production adapter
must pass the shared file-binding/default/unsupported/immutability conformance
suite before translation support is claimed.

Rationale: classification remains panic-free and atomic, while fake tests cannot
be mistaken for evidence that a real provider preserves legacy semantics.

## Decision 20: exact YAML boundary and nested validation location

Refinement timestamp: 2026-09-28 18:16:29 Asia/Novosibirsk
(2026-09-28 11:16:29 UTC). GitHub user ID: @evilguest (41718235).
Agent: OpenAI Codex (GPT-5).

Question: how should callers invoke the YAML adapter and interpret its resource
counts and nested Runtime validation locations without relying on implementation
accidents?

Alternatives: leave the adapter signature and YAML depth/node accounting
implicit; promise a lossy conversion from Runtime dotted paths to JSON Pointers;
define an atomic decode API, exact counting roots, and separate enclosing
composition pointers from unwrapped Runtime field paths.

Decision: `DecodeAliasDocumentYAML(raw, target)` is the adapter boundary and
leaves a non-nil target unchanged on failure. Raw size, UTF-8, and directive-line
checks precede parsing. Depth starts at the logical root mapping; node counts
exclude the document wrapper but include mapping keys, and aliases are rejected
rather than traversed. The YAML transport may be more restrictive than strict
JSON only by its published resource limits. A nested Runtime validation failure
uses the enclosing declaration as its composition pointer and preserves the
precise Runtime path through error unwrapping.

Rationale: boundary tests have unambiguous off-by-one and atomicity expectations,
while diagnostics do not fabricate an RFC 6901 path from a path grammar that may
contain identifier punctuation.

## Decision 21: derive compatibility source IDs exactly

Refinement timestamp: 2026-09-28 18:21:32 Asia/Novosibirsk
(2026-09-28 11:21:32 UTC). GitHub user ID: @evilguest (41718235).
Agent: OpenAI Codex (GPT-5).

Question: may a direct compatibility caller supply an arbitrary relative
`SourceID` beside otherwise valid workspace and alias paths?

Alternatives: trust any bounded relative diagnostic string; let provider
adapters derive it; require it to equal the slash-normalized canonical relative
path and revalidate that relationship before dispatch.

Decision: the compatibility `SourceID` is exactly the non-empty
`filepath.ToSlash` form of `AliasPath` relative to `WorkspaceRoot`. The alias is
strictly inside the root; the ID has no absolute form, backslash, empty, `.`, or
`..` segment and obeys the composition source-ID byte limit. `TranslateLegacy`
performs this check before invoking an adapter even when its caller bypasses the
existing alias resolver.

Rationale: diagnostics cannot disagree with the selected file or leak an
absolute host path, and every adapter observes the same location identity.

## Decision 22: approve the composition test strategy

Decision timestamp: 2026-09-28 18:28:21 Asia/Novosibirsk
(2026-09-28 11:28:21 UTC). GitHub user ID: @evilguest (41718235).
Agent: OpenAI Codex (GPT-5).

Question: is the revised issue #109 test strategy sufficient to proceed to the
existing-test contradiction review and test implementation?

Alternatives: revise the test inventory again; approve only the public
composition slice; approve the complete staged composition and compatibility
strategy.

Decision: approve the complete test strategy in
`runtime-v2-composition-tests.md`, including its separate composition-module and
later CLI-compatibility acceptance gates.

Rationale: the plan now covers strict transports, deterministic expansion and
errors, provenance exclusion, resource bounds, provider conformance, downstream
identity behavior, and independently mergeable release sequencing.

No prior ADR is obsolete. The decisions are additive to the Runtime v2 semantic
core, declaration, resolver, and persistence contracts, and keep all current
legacy alias ADRs valid for the existing CLI path.
