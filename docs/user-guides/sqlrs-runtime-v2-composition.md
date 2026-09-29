# Runtime v2 alias composition

## Status

This document describes the approved design for issues #109 and #137. The composition
schema and library are not implemented or connected to the current `sqlrs`
commands yet.

Existing `*.prep.s9s.yaml` and `*.run.s9s.yaml` files continue to use the legacy
alias behavior documented in [sqlrs aliases](sqlrs-aliases.md). In particular,
`sqlrs plan <ref>` and `sqlrs prepare <ref>` do not yet select a Runtime v2
composition entry.

## Purpose

Runtime v2 composition lets repository data describe:

- reusable standalone transforms;
- a factory-only database recipe;
- ordered transforms applied to one factory;
- a recipe that reuses another complete recipe as its prefix;
- multiple migration roots applied sequentially to one logical database.

Composition is expressed in data rather than through a multi-step CLI grammar.
It produces the existing engine-neutral Runtime v2 declarations before input
resolution, fingerprinting, planning, or execution.

## Schema

The schema version is `sqlrs.runtime.v2.aliases.v1`:

```yaml
schema_version: sqlrs.runtime.v2.aliases.v1
aliases:
  main-migrations:
    type: transform
    declaration:
      kind: psql
      reference: migrations/main.sql
      arguments: []
      attributes: {}

  user-migrations:
    type: transform
    declaration:
      kind: psql
      reference: migrations/users.sql
      arguments: []
      attributes: {}

  base-db:
    type: recipe
    base:
      factory:
        kind: postgres
        reference: postgres:17
        arguments: []
        attributes: {}
    steps: []

  project-db:
    type: recipe
    base:
      recipe: base-db
    steps:
      - use: main-migrations
      - use: user-migrations
```

The YAML example is a presentation of the logical schema. The public Runtime v2
composition library uses strict JSON and constructor inputs. A future CLI YAML
adapter will accept only one bounded document and reject duplicate keys,
anchors, aliases, merge keys, custom tags, multiple documents, and unknown
members. It also rejects directives, YAML scalar coercion, and input beyond its
documented byte, node, and depth limits. These YAML transport limits may reject
a very large document accepted by strict JSON; accepted values have identical
constructor/JSON meaning.

The schema does not yet assign a filename or define directory discovery,
imports, or precedence between files. A caller supplies an explicit set of
documents to one catalog. Defining product-level discovery requires a separate
CLI design and approval.

Each supplied document receives a unique logical `SourceID`. Future CLI tooling
uses a slash-separated workspace-relative ID, not an absolute host path. Source
IDs appear in diagnostics, so they must not contain credentials or secrets.

## Alias kinds

### Transform

A transform alias contains one complete `TransformDeclaration`:

```yaml
seed-data:
  type: transform
  declaration:
    kind: psql
    reference: seeds/data.sql
    arguments: []
    attributes: {}
```

It can be used by a recipe or expanded independently for application relative
to an arbitrary existing Runtime v2 State.

### Recipe with a factory

A recipe may declare its factory directly:

```yaml
preloaded-db:
  type: recipe
  base:
    factory:
      kind: postgres
      reference: registry.example/postgres-preloaded:17
      arguments: []
      attributes: {}
  steps: []
```

An empty `steps` list is valid and does not create a synthetic transform.

### Recipe with a recipe prefix

A recipe may use another recipe as its complete prefix:

```yaml
application-db:
  type: recipe
  base:
    recipe: preloaded-db
  steps:
    - use: seed-data
```

The parent recipe contributes its factory and every transform in its existing
order. The child steps are appended afterward.

A recipe cannot be placed in `steps`. Allowing that would require comparing or
merging two factories before provider resolution. Such a reference is rejected
as `wrong_reference_kind`.

### Inline transform

A one-off transform may be embedded without creating a named alias:

```yaml
smoke-db:
  type: recipe
  base:
    recipe: application-db
  steps:
    - transform:
        kind: psql
        reference: smoke/setup.sql
        arguments: []
        attributes: {}
```

Each step contains exactly one of `use` or `transform`.

## Deterministic behavior

- Alias references use exact lower-case names.
- All supplied documents form one flat namespace with no implicit precedence.
- Source IDs are unique within a catalog.
- Duplicate names across documents are ambiguous and fail.
- A missing name fails without fallback or filesystem guessing.
- Recipe-prefix cycles fail before resolution or execution and report the full
  closed cycle chain.
- Steps retain source order and are never sorted or run in parallel.
- Expansion is atomic: an error returns no partial recipe.
- The expanded recipe contains no alias names or source paths added solely for
  diagnostics.

Consequently, two differently named alias graphs that expand to equal factory
and transform declarations have equal expanded semantic form. Given otherwise
equal provider/resolver semantics and external inputs, they also resolve to the
same identity. Alias names and source locations remain available in a separate
versioned explanation trace,
`sqlrs.runtime.v2.alias-expansion-trace.v1`.

Changing a referenced validated declaration value changes the parent's
expanded declaration. Reordering equivalent YAML/JSON members does not.
Downstream StateID changes only when the provider resolves the semantic change
to a different identity-bearing value; comments and other diagnostics do not
become identity inputs.

The trace uses a compact parent-linked node table and origin indexes rather than
repeating the full alias chain for every step. Its source locations are RFC 6901
JSON Pointers for both definitions and reference sites. Trace nodes are ordered
deterministically: target, recipe-base chain, then named transforms in output
order. Repeated references remain separate occurrences. The trace remains
diagnostic and is never a resolver or fingerprint input.

## Path binding

The composition layer does not interpret or rebase provider-owned declaration
fields. A document's file location and `SourceID` never become an implicit path
base.

Each provider contract defines its own rules. For example, the Runtime v2
`sqlrs.workspace/file` declaration from #108 remains workspace-root-relative.
A provider-aware authoring adapter must materialize such values before creating
the composition document. A `reference` value such as `migrations/main.sql`
does not by itself opt into workspace-file semantics; its `kind` provider owns
that interpretation.

For legacy compatibility, the adapter first applies the existing
alias-file-relative path behavior and image-default precedence, then constructs
an explicit Runtime v2 declaration. Moving only a composition source file does
not silently rebase its semantic inputs.

## Parameters and defaults

Schema v1 has no alias parameters, overrides, imports, or implicit defaults.
Every composition value is explicit.

A future compatibility adapter may apply the existing legacy image-default
precedence before constructing a Runtime v2 declaration. The selected value must
then be materialized explicitly; the composition library never reads workspace
or global configuration.

## Errors

The stable error classes are:

- `invalid_document` — malformed, unknown, or invalid schema content;
- `missing_reference` — an exact alias name was not found;
- `ambiguous_reference` — more than one source defines the name;
- `wrong_reference_kind` — a recipe was used as a transform or vice versa;
- `cycle` — a recipe-prefix chain revisits an active recipe;
- `expansion_too_large` — aggregate document/catalog, graph, expanded recipe,
  or trace limits were exceeded. Invalid scalar declaration fields retain
  `invalid_document` and the underlying Runtime validation detail.

Diagnostics expose a stable code, optional `SourceID`, an RFC 6901 JSON Pointer,
and bounded deterministic alias/candidate context. They do not include
declaration contents. Failures from composition constructors, dedicated
decoders, catalog, and expansion operations match the package's `ErrInvalid`
sentinel. Duplicate source IDs are `invalid_document`.

## Legacy aliases

Legacy prepare aliases are not silently reinterpreted as Runtime v2 recipes.
Issue #137 supplies the separate compatibility slice that consumes the
published composition module. Its provider-aware adapter translates an alias only when it
can produce a complete deterministic factory and transform declaration after
the existing path and default rules have been applied.

The first compatibility slice accepts legacy prepare aliases only. Legacy run
presets continue on their existing execution path and are classified as
`legacy_semantics_unsupported`, because running a tool is not necessarily an
identity-bearing database transform.

Otherwise the adapter reports `legacy_only` with an actionable reason such as
`runtime_v2_provider_unavailable`. This classification does not stop the alias
from running through the currently supported legacy executor.

Runtime v2 composition, legacy execution, legacy cache records, and Runtime v2
logical persistence remain separate until an explicit cutover is designed and
approved.
