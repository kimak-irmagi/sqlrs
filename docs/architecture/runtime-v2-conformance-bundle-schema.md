# Runtime v2 canonical v1 conformance bundle schema

Status: approved with the canonical-v1 contract for issues #130 and #131 on
2026-09-27.

## Versions and directory layout

The bundle schema, bundle content version, semantic schema, and Go module version
are independent:

- bundle schema: `sqlrs.runtime.conformance.bundle-schema.v1`;
- bundle version: `runtime-v2-canonical-v1.1`;
- semantic schema: `sqlrs.runtime.v2.canonical.v1`;
- first proposed module release: `v0.3.0`.

```text
conformance/canonical-v1/
  manifest.json
  manifest.sha256
  vectors/
    canonical-values.json
    compatibility.json
    composition.json
    fingerprints.json
    integrity.json
    lineage.json
    limits.json
    secrets-and-disclosure.json
```

A published bundle version is immutable. Any vector byte change, file addition,
removal, rename, or expected-result change requires a new bundle version and
directory. Historical bundles remain available through their immutable module
versions; the current module embeds exactly one current bundle.

All files are UTF-8 JSON except `manifest.sha256`, use LF, and end with one
newline. JSON decoding rejects duplicate/unknown members and trailing tokens.

## Manifest

```json
{
  "bundle_schema_version": "sqlrs.runtime.conformance.bundle-schema.v1",
  "bundle_version": "runtime-v2-canonical-v1.1",
  "semantic_schema": "sqlrs.runtime.v2.canonical.v1",
  "entries": [
    {
      "path": "vectors/canonical-values.json",
      "size": 1234,
      "sha256": "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
    }
  ]
}
```

Entry paths are a portable lowercase ASCII subset of UTF-8 and are ordered
lexicographically by their raw UTF-8 bytes. Each segment matches
`^[a-z0-9][a-z0-9._-]*$`; separators are `/`. Paths are relative, contain no
empty, `.`, or `..` segments, and cannot name `manifest.json` or
`manifest.sha256`. The verifier rejects non-ASCII/uppercase characters, absolute
paths, backslashes, drive prefixes, unlisted files, and missing files. This
removes platform/locale-dependent Unicode case-folding from the contract. `size`
is the exact unsigned byte length. Entry SHA-256 is lowercase hex without an
algorithm prefix.

The in-memory public verifier receives byte entries and therefore cannot observe
symlinks. A separate repository/package filesystem check walks the source bundle
without following links and rejects symlinks, junctions/reparse points, and all
other non-regular files before embedding.

## Detached manifest digest

The fixed bundle digest domain is
`sqlrs.runtime.conformance.bundle.v1`. `encodeString` is unsigned 64-bit
big-endian byte-length framing. The digest preimage is exactly:

```text
encodeString("sqlrs.runtime.conformance.bundle.v1")
|| encodeString(bundle-schema-version)
|| u32be(entry-count)
|| for each manifest entry in listed order:
     encodeString(path)
     || u64be(size)
     || raw-32-byte-SHA256(content)
```

The manifest file and detached digest file are excluded. `manifest.sha256`
contains exactly `sha256:<64-lowercase-hex>\n`. The verifier validates manifest
shape and path order, checks every size/content digest, recomputes this preimage,
and only then parses semantic vectors.

Because the mandated preimage does not contain `bundle_version` or semantic
schema, release evidence binds the tuple `(bundle version, bundle-schema
version, semantic schema, detached digest, source SHA)`. Changing vector content
always changes both bundle version and detached digest.

The public parser also receives an expected descriptor containing the first
three tuple fields and requires exact equality before accepting entries.
`LoadConformanceBundle` supplies compiled constants for the current bundle. The
RC/GA attestation binds those constants and the detached digest to source SHA;
there is no claim that the detached digest alone authenticates manifest metadata.

## Vector file envelope

Every listed vector file has:

```json
{
  "vector_schema": "sqlrs.runtime.v2.canonical.conformance-vector.v1",
  "cases": [],
  "relations": []
}
```

Case and relation IDs are globally unique lowercase strings matching
`^[a-z][a-z0-9._/-]{0,127}$`. Arrays are lexicographically ordered by ID. Tags
are unique and sorted. The bundle verifier requires these coverage tags:

```text
legacy-non-reinterpretation
canonical-value
factory
transform
resolved-extension
root-state
derived-state
extension-composition
ordered-recipe
relative-lineage
mutable-reference-same-resolution
identity-changing
diagnostics-only
secret-reference
safe-redaction
internal-disclosure
integrity-tampering
supported-builder-operational-metadata
limits
fuzz-seed
```

Tags count only when the operation/projection matches their meaning.

## Case envelope and errors

```json
{
  "id": "canonical-value/map-order",
  "tags": ["canonical-value"],
  "operation": "canonical-value",
  "input": {},
  "expected": {"status":"ok"}
}
```

Operations are `canonical-value`, `canonical-token`,
`canonical-value-envelope`, `identity-field`, `factory`, `transform`,
`resolved-extension`, `root-state`, `derived-state`, `compose-factory`,
`compose-transform`, `recipe`, `relative-lineage`, `decode-envelope`,
`explain-safe`, `explain-internal`, and `legacy-decode`.

Success objects are operation-specific. Failure is always:

```json
{
  "status": "error",
  "error": {
    "code": "digest_mismatch",
    "path": "steps[1].state.id"
  }
}
```

Code/path exactly match public structured errors and never include rejected
values. Validation phase order is document budget/syntax, shape/discriminator,
value limits, descriptor mapping, payload commitment, identity digest, then
lineage/endpoint. Negative cases normally mutate one property and preserve
source order/duplicates instead of storing already-normalized Go values.
Envelope and bundle cases use the stable error taxonomy and path grammar from
the canonical-v1 flow.

## Canonical-value AST

```json
{"type":"null"}
{"type":"string","value":"text"}
{"type":"list","members":[{"type":"null"}]}
{"type":"map","entries":[{"key":"a","value":{"type":"null"}}]}
{"type":"set","members":[{"type":"string","value":"a"}]}
```

Map entries are an array so duplicate keys and permutations are representable.
Success contains canonical-node hex and `civ1` token. The verifier independently
recomputes both and round-trips a full canonical-value envelope. Negative and
fuzz seeds cover invalid UTF-8, duplicate key/member, depth, total nodes,
collection count, string/key bytes, total canonical bytes, decoded envelope
bytes, integer overflow, malformed tokens, non-canonical order, and trailing
data.

## Typed fields and secret references

Identity-field inputs explicitly choose `text`, `canonical-value`, or
`secret-reference`. Canonical-value identity input always carries the complete
AST; its expected token is recomputed. Token-only values are parsing/comparison
fixtures and are rejected wherever a verified identity subject is required.
Secret-reference input contains synthetic non-secret provider/identifier/version
values only.

Success contains field kind, canonical payload hex, contribution commitment,
and read-only accessor values. Vectors prove that text starting `civ1:` remains
text, kinds produce different commitments, secret version changes identity, and
empty/invalid reference components fail. No fixture contains credential-like raw
material.

Builder-negative cases attempt to pass text to a schema field declared
secret-reference and require rejection. Providers lacking a stable reference
are represented by an omitted identity field plus execution-only metadata that
does not enter any serialized Runtime v2 object.

## Identity, composition, and lineage cases

Factory, transform, extension, and state success records contain:

- exact canonical preimage hex;
- typed field commitments;
- compact digest/StateID;
- strict descriptor and full envelope JSON;
- provider/owner, semantic kind, and identity schema where applicable.

Composition inputs contain an authored schema, declaration, typed identity
fields, operational observations, and role-specific resolved extensions.
Negative cases cover field kind/role/requiredness violations, direct
observation-to-field conversion attempts, missing/extra/reordered extension
results, owner/kind mismatch, and deployment on a transform. Operational-only
changes alter observations but not canonical bytes or identity digest.

Recipe and relative-lineage expected values contain verified envelopes for root
or anchor, ordered transforms and derived states, parent links, and endpoint.
Tampering cases independently modify descriptor kind/domain/schema/algorithm,
canonical-value token, field commitment, compact digest, parent, state ID, step
order, and endpoint and require an exact integrity error.

Legacy compatibility cases feed frozen `sqlrs.runtime.v2` values to the legacy
decoder and compare existing bytes/IDs. The same bytes must be rejected by every
canonical-v1 decoder. Canonical-v1 bytes must be rejected by legacy decoders. No
case expects conversion or rehashing.

## Disclosure cases

Schema definitions mark fields/observations public or protected. Safe explain
success shows typed redaction markers and omits protected values, opaque secret
identifiers, and their per-field commitments. It retains aggregate descriptor
digest, StateIDs, links, and endpoint previously verified by the source envelope
but is not independently decoded as proof. Internal explain requires a non-nil
authorizer fixture that explicitly returns allow for a live context; nil,
deny/error, and canceled-context cases return no projection. Internal output may
reveal protected non-secret values and complete secret references. Runtime-only
secret canaries, never checked-in vector data, scan both profiles and other sinks.

## Cross-case relations

```json
{
  "id": "diagnostics/job-id-does-not-change-identity",
  "tags": ["diagnostics-only"],
  "comparison": "equal",
  "projection": "digest",
  "cases": ["compose/factory-a", "compose/factory-b"]
}
```

Comparisons are `equal`/`not-equal`. Projections are `canonical-bytes`,
`canonical-value-token`, `field-commitment`, `digest`, `state-ids`, `endpoint`,
`descriptor`, `observation`, and `explanation`. Required relations cover map/set
permutation, ordered list/recipe sensitivity, same immutable resolution for
mutable references, identity-changing values, diagnostics-only invariance, and
secret-reference version changes.

## Public and independent verification

`LoadConformanceBundle` embeds all listed files and detached digest. It returns
defensive copies and exposes bundle-schema version, bundle version, semantic
schema, and manifest digest. `Verify` performs filesystem-independent manifest
and semantic verification and reports a stable file/case/relation path.

The checked-in Node verifier is a read-only reference implementation of framing,
tree encoding, field commitments, five domains, manifest preimage, and relation
projections. It shares no generated encoder code or expected-output generator
with Go. CI runs both implementations against reviewed, locked known-answer
vectors; neither implementation rewrites expected data. Any offline vector
generation tool is outside the test path and its output requires human review,
a new bundle version, and the immutable-bundle policy check.

Pre-merge policy compares bundle paths/bytes with the merge base. Release
preflight independently compares them with the latest published module tag.
Changing an existing published bundle or changing current content without a new
directory/version fails before semantic verification.
