# Runtime v2 alias ownership and issue #140

Conversation timestamp: 2026-10-03 15:21:41 Asia/Novosibirsk
(2026-10-03 08:21:41 UTC). GitHub user ID: @evilguest (41718235).
Agent: OpenAI Codex (GPT-5).

Status: accepted. This ADR supersedes the unimplemented
[alias-consumer publication proposal](2026-10-02-runtime-v2-alias-consumer-publication.md).
The existing public `runtime-go/composition` package from #109 remains in place.

## Decision 1: expand aliases in the CLI before engine submission

Question: should sqlrs-engine receive alias documents and recursively expand
composite aliases, or should the client submit only the expansion result?

Alternatives considered:

1. Move YAML decoding, catalog construction, recursive alias expansion, and
   legacy compatibility into sqlrs-engine or a shared server-facing package.
2. Keep all alias authoring and expansion in sqlrs CLI, and submit only a
   complete versioned `runtimev2.RecipeDeclaration` and the source inputs needed
   to resolve it.

Decision: choose alternative 2. The CLI owns alias discovery, YAML decoding,
catalog construction, recursive expansion, legacy translation, and
provider-aware authoring/binding. The engine receives no alias document,
catalog, name-to-definition lookup request, or expansion trace. It validates
the submitted declaration and source inputs, resolves them via providers, and
owns planning, execution, and StateID computation. The request transport and
command cutover need separate design and implementation; this ADR does not
claim they already exist.

Rationale: the first-generation server did not need alias syntax. Keeping the
same authoring boundary avoids duplicating client-specific path/default rules
in the engine and keeps semantic identity based on resolved declarations and
inputs rather than alias names or source files.

## Decision 2: do not publish the #137 CLI adapter as a Runtime Go package

Question: does the client-side YAML and legacy compatibility layer need the
public `runtime-go/aliasruntimev2` package proposed in sqlrs#140?

Alternatives considered:

1. Publish the adapter in a separate module with its own release cycle.
2. Publish it as `runtime-go/aliasruntimev2` in the Runtime Go release cycle.
3. Keep it in `frontend/cli-go/internal/alias/runtimev2` and use the already
   published `runtime-go/composition` package for deterministic expansion.

Decision: choose alternative 3 and close sqlrs#140 as not planned. The name
`aliasruntimev2` continues to identify only the CLI-internal Go package, not a
new public API. No Runtime Go v0.6.0 release, YAML dependency exception, or
public compatibility surface is required for this decision. The CLI and engine
still need a coordinated product integration and release, but that does not
require publishing the CLI adapter as a reusable library.

Rationale: the adapter handles sqlrs-specific YAML, legacy alias semantics,
defaults, and path binding. Its only required product consumer is the CLI.
Publishing it would freeze an unnecessary API and move the responsibility
boundary toward the engine. The public composition contract already provides
the reusable, syntax-neutral part.
