# Runtime v2 production execution schema: interaction flow

Status: interaction flow **approved** by @evilguest on 2026-10-15, issue #150.
The concrete [API and component structure](runtime-v2-execution-schema-structure.md)
were approved on 2026-10-16; the test inventory was separately approved that day. Publication is tracked
separately by [#151](https://github.com/kimak-irmagi/sqlrs/issues/151).

Review clarification, approved 2026-10-16: the concrete facade receives resolved
argv/env/path bindings, not authoring inherit/clear flags. The revised structure
also defines qualified source materialization and an explicit local-executable
profile that cannot authorize reusable States. These details are approved;
the approved separation of resolution, identity and execution remains unchanged.

## Purpose

An external provider must be able to identify a resolved execution environment
without importing `schemaauthor` or using `schemas/conformancev1` in production.
The existing v0.5.0 public declarations, canonical extension identities,
structured values and resolver result/cache are sufficient generic contracts.
The missing part is a reviewed production schema-specific construction path.

Add that path to `backend/libs/runtime-go/schemas/sqlrs`. Do not add an execution
service, separate module, second resolver result, new StateID algorithm or
alternate canonical encoding. The nested module remains standard-library-only.
No CLI, HTTP API, database schema or active engine path changes in this issue.

## Flow and trust boundary

```text
provider-owned, explicitly validated declaration
  -> provider resolution outside the semantic library
  -> exact semantic execution content + separate operational observations
  -> schema-specific production facade
  -> existing CanonicalResolvedExtensionIdentity
  -> existing resolver Resolution and schema-validated CacheRecord
  -> existing execution-environment binding in a transform builder
  -> verified transform envelope and derived-state lineage
```

The facade owns classification and validation of supported identity fields.
The provider owns resolution, source admission and the truth of its assertions.
A fingerprint proves which values were committed, not that an image was pulled,
a host tool is hermetic, or a job succeeded. The facade performs no I/O.

Use typed semantic inputs rather than an unclassified map of runtime metadata.
Reuse the root immutable canonical-value and secret-reference types, and return
the existing canonical extension type. Only the facade implementation may
import the generic schema-authoring package. Consumers need a schema accessor
for the existing resolver contract and a schema-specific construction path;
they must not copy the field schema or import a conformance fixture.

## Semantic content

The concrete API must preserve these categories and distinctions:

- Primitive: host, OCI or execution inside the database container. Compose
  authoring normalizes to a primitive; YAML spelling and service names are not
  additional identity inputs.
- Exact immutable image/tool identity and explicit platform. Image index,
  platform-specific manifest and local daemon ID must not be conflated. Host
  execution does not become remotely reproducible merely by naming a tool.
- Effective argv and ordered arguments. Declaration inherit/absent versus empty
  differences are resolved using the source's rules before identity construction.
  No implicit splitting, shell insertion or reordering occurs in the facade.
- Explicit non-secret environment and separately typed non-secret references
  for behavior-affecting secret revisions. Raw credentials and their unkeyed
  hashes are outside the API contract.
- Logical workdir and content-bound mount descriptions, including destination,
  access mode and isolated writable scratch semantics. A logical container path
  may affect behavior; an acquisition path or caller host path is not identity.
  Typed path coordinates render only after binding; qualified source identities
  and materialization profiles account for file/tree kinds and executable modes.
- Logical service/target roles and behavior-affecting selection, not the current
  physical address, workspace/container reference, job or owner generation.

Structured content uses the existing canonical constructors, with bounded
collections and duplicate rejection. The approved component/API design makes
these categories mechanically distinct, including typed secret slots, rather
than pack everything into an arbitrary JSON string or a single unchecked map.

## Observations, records and compatibility

Keep original mutable references, resolver evidence/version, acquisition handles,
timestamps, process IDs, capabilities and portability outside semantic content.
They cannot be accepted through an operational type and converted into identity.
The schema cannot detect a caller deliberately disguising a secret as ordinary
text; this trust limitation must be explicit rather than replaced by a name
denylist or a claim of arbitrary-secret detection.

The new provider schema has its own explicit version. Existing canonical-v1
domains, other provider schemas and published vectors retain their meaning.
Do not migrate or reinterpret older records. Existing public envelope and
resolver codecs validate new identities against the selected schema; no new
cache wire format is necessary merely to add a provider schema.

`Resolve`, `Revalidate` and `Acquire` remain outside the facade. Resolver
implementations register this schema using the existing framework. A serialized
identity preserves the committed content but is not by itself an executable
replay bundle or permission to access sources, secrets or a database.

A host-executable identity describes only the verified executable, not complete
ambient dependencies. Its consumer must deny reusable-State lookup/publication
and shared replay before planning, and propagate that qualification through all
descendants. A later complete tool cannot repair an incomplete ancestor. This
is distinct from retaining a generic resolution record; the facade owns no
runtime eligibility store or cache-policy implementation.

## Approval sequence

The interaction boundary and exact API/component structure are approved.
The architecture index, canonical/resolver documents and module README reflect
that approval. The [test inventory](runtime-v2-execution-schema-tests.md)
was separately approved on 2026-10-16. Write tests before implementation;
approval is not evidence of a completed feature.
Individual decision statuses are recorded in the
[ADR](../adr/2026-10-15-runtime-v2-execution-schema-facade.md).
