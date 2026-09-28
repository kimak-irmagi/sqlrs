# Схема conformance bundle canonical v1 Runtime v2

Статус: согласовано для issues #130/#131 2026-09-27 вместе с canonical-v1
contract.

## Независимые версии и layout

- bundle schema: `sqlrs.runtime.conformance.bundle-schema.v1`;
- bundle version: `runtime-v2-canonical-v1.1`;
- semantic schema: `sqlrs.runtime.v2.canonical.v1`;
- proposed module: `v0.3.0`.

`conformance/canonical-v1/` содержит `manifest.json`, `manifest.sha256` и strict
`vectors/*.json`. Published bundle version immutable; изменение bytes/cases/files
требует нового bundle version/directory. Current module embeds один current
bundle; historical bundle доступен через immutable old module release.

## Manifest и detached digest

Manifest содержит три версии и entries `{path,size,sha256}`. Path — portable
lowercase ASCII subset UTF-8; каждый segment matches
`^[a-z0-9][a-z0-9._-]*$`, separator `/`, sort по raw UTF-8 bytes. Empty/`.`/`..`,
uppercase/non-ASCII, absolute, backslash и drive prefix запрещены; тем самым нет
platform-dependent Unicode case-folding. In-memory verifier не видит filesystem
objects. Отдельный no-follow walk запрещает symlink/junction/reparse/non-regular.

Fixed domain: `sqlrs.runtime.conformance.bundle.v1`. Digest preimage точно:

```text
encodeString(bundle-domain)
|| encodeString(bundle-schema-version)
|| u32be(entry-count)
|| repeated(
     encodeString(path)
     || u64be(size)
     || raw-32-byte-SHA256(content)
   )
```

`encodeString` использует u64 big-endian byte length. `manifest.sha256` содержит
`sha256:<64-lowercase-hex>\n`. Проверка идёт path/order → size/content digest →
manifest digest → semantic vectors. Release evidence связывает bundle version,
bundle-schema, semantic schema, digest и source SHA.

Public parser принимает expected descriptor с bundle-schema, bundle и semantic
versions и проверяет exact equality. `LoadConformanceBundle` использует compiled
current constants; RC/GA attestation связывает их и digest с source SHA. Сам
detached digest не объявляется authentication manifest metadata.

## Vector files

Каждый file имеет `vector_schema`, sorted globally unique cases и relations.
Required coverage: legacy non-reinterpretation, canonical value/limits/fuzz,
factory/transform/extension/root/derived, extension composition, recipe/relative,
same resolution, identity changes, diagnostics-only, secret references,
safe/internal disclosure, tampering и supported-builder operational metadata.

Operations strict: canonical value, canonical token, binary canonical-value
envelope, typed field, identities/states, composition, lineage, fingerprint
envelope decode, explain profiles и legacy decode. Error expected всегда
содержит exact structured code/path без rejected value. Validation phase order:
document budget/syntax, shape/discriminator, limits, descriptor mapping,
payload commitment, identity digest, lineage/endpoint. Negative case обычно
содержит ровно один fault.
Envelope/bundle cases используют stable error taxonomy/path grammar из
canonical-v1 flow.

Canonical-value AST использует array map entries, сохраняя duplicates/order.
Success содержит node hex и `civ1` token. Negative/fuzz cases покрывают UTF-8,
duplicates, depth/nodes/count/string/key/byte/envelope budgets, overflow, token,
canonical order и trailing data.

Typed-field vectors явно выбирают text/canonical-value/secret-reference, сверяют
payload/commitment/accessors и доказывают, что text `civ1:` не интерпретируется.
Canonical-value identity всегда содержит полный AST; token-only fixture допустим
только для parse/comparison и rejected как verified identity subject. Secret
fixtures содержат synthetic non-secret references; raw credentials отсутствуют.

Identity/composition/lineage vectors содержат exact preimages, commitments,
digests, envelopes, parent links и endpoint. Tampering меняет по одному элементу
и ожидает integrity error. Legacy bytes принимаются только legacy decoder;
cross-revision decode и rehash всегда rejected.

Safe explain vectors содержат redaction markers, но не protected payload, opaque
secret ID или их per-field commitments. Они сохраняют ранее verified aggregate
digest/StateID/links, но не декодируются как proof. Internal explain требует
non-nil allow authorizer и live context; nil/deny/error/cancel возвращают no
projection. Runtime-only canary, не checked-in vector, проверяет отсутствие raw
secret во всех sinks.

Relations сравнивают projections без копий expected hashes: permutation,
ordering sensitivity, mutable-reference same resolution, identity-changing,
diagnostics invariance и secret-version changes.

## Verification

Public loader возвращает defensive files и versions/digest, не используя module
cache paths. Node verifier — read-only reference implementation без shared
generated encoder/expected generator с Go. CI запускает обе реализации на
reviewed locked known-answer vectors; ни одна не переписывает expected data.
Offline generation требует human review, новой bundle version и policy check.
Pre-merge policy сравнивает merge base, release preflight — latest published tag.
