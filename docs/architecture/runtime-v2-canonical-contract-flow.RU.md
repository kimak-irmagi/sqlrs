# Canonical v1 contract Runtime v2: взаимодействие компонентов

Статус: согласовано для issues #130/#131 2026-09-27 после уточнения требований.

Дополнение внешнего conformance facade: согласовано для issue #138 2026-09-29.

## Flow внешнего conformance facade

Facade не добавляет runtime или production execution path. Чистый consumer
использует те же production builders через фиксированную schema capability:

```text
consumer declaration
  -> role-specific declaration helper из schemas/conformancev1
  -> фиксированная factory/transform/extension schema из schemas/conformancev1
  -> schema-bound builder runtimev2
       |-> типизированные IdentityField values
       |-> typed observation helper из schemas/conformancev1
       `-> role-complete resolved extension bindings
  -> canonical identity / envelope / explanation
  -> consumer assertions
```

Только реализация facade импортирует `schemaauthor`. Consumer видит root
semantic types и фиксированный, versioned для canonical-v1 facade. Declaration
spelling и observations могут изменяться независимо: declaration helpers
принимают одну reference string, а observation helpers валидируют фиксированный
operational vocabulary и связывают его скрытую schema capability. Identity
schema гарантирует, что только locator, plan и credential входят в semantic
channel. СУБД, filesystem, registry, container, network client и command runner
не участвуют.

Repository production files не могут импортировать facade. Tests и внешний
clean-consumer module используют его так:

```text
fixed facade builder + typed fields + optional observation/extensions
  -> Build
  -> ordinary canonical-v1 identity/envelope/explanation
  -> проверка equality, sensitivity, redaction и diagnostic isolation
```

## Compatibility boundary

Новый контракт имеет discriminator `sqlrs.runtime.v2.canonical.v1`; typed fields
намеренно меняют canonical bytes. Значения `v0.2.0` с discriminator
`sqlrs.runtime.v2` остаются immutable legacy-v2.

Legacy и canonical-v1 используют разные Go types и decoders. Каждый decoder
принимает только свой discriminator; rehash/guess/implicit upgrade запрещены.
Side-by-side persistence выбирает decoder по discriminator. Migration означает
повторный resolve source declaration под явно выбранной canonical-v1 schema, а
не переписывание legacy value. CLI, HTTP API, DB schema, providers, execution,
snapshot policy и default cutover вне scope.

## Пять canonical domains и framing

`u16/u32/u64` — unsigned big-endian. `encodeBytes(x)` — `u64(len(x)) || x`,
`encodeString` применяет framing к неизменённым UTF-8 bytes. Canonical-v1 domains:

| Identity | Domain |
| --- | --- |
| structured value | `sqlrs.runtime.v2/identity-value` |
| factory/root | `sqlrs.runtime.v2.canonical.v1/factory-state` |
| transform | `sqlrs.runtime.v2.canonical.v1/transform` |
| resolved extension | `sqlrs.runtime.v2.canonical.v1/resolved-extension` |
| derived state | `sqlrs.runtime.v2.canonical.v1/state` |

Legacy domains не являются aliases. Digest format — lowercase
`sha256:<64-hex>`.

Factory/transform records используют tags 1–5: canonical semantic schema,
provider, semantic kind, provider identity schema, typed field set. Extension
использует owner в tag 2. Field set: `u32(count)` и sorted entries
`encodeString(name) || u16(kind) || raw-32-byte-commitment`. Factory/root StateID
равен factory digest. Derived-state record имеет raw parent canonical StateID в
tag 1 и raw verified transform digest в tag 2; legacy IDs/untyped fingerprints
отклоняются.

## Canonical values

Tree node: `u16(type-tag) || encodeBytes(payload)`: null=0, string=1, list=2,
map=3, set=4. List сохраняет порядок. Map принимает entry slice, сортируется по
полному encoded string-key node и видит duplicate source keys. Set сортируется по
encoded member nodes. Canonical duplicates отклоняются.

Digest preimage:

```text
encodeString("sqlrs.runtime.v2/identity-value") || encodeBytes(root-node)
```

Token: `civ1:sha256:<lowercase-hex>`. Это digest reference, не Base64 и не
reversible serialization. Envelope может нести tree рядом с token для
recomputation.

Экспортируются limits: depth 32, total nodes 4096, members per collection 256,
UTF-8 string 4096 bytes, map key 1024 bytes, canonical value 1 MiB, decoded
envelope 4 MiB. Budgets проверяются до allocation/sorting; обязательны fuzz и
adversarial vectors.

Root имеет depth 1, child добавляет 1. Members limit применяется к каждой
collection; total nodes/bytes — ко всему tree. Map entry — один member, а key
node и value subtree входят в total nodes; key также ограничен stricter key
byte limit.

## Typed fields и secrets

Canonical-v1 `IdentityField` immutable и имеет kind:

- `text` (tag 1, UTF-8 payload);
- `canonical-value` (tag 2, raw digest из `civ1` token);
- `secret-reference` (tag 3, framed provider/id/version).

Text `civ1:...` остаётся text. Field contribution:
`SHA-256(encodeString(name) || u16(kind) || encodeBytes(payload))`. Identity field
set сортируется по UTF-8 name и кодирует name, kind и raw commitment. Kind входит
в canonical preimage; safe explanation может redacted payload заменить
commitment без потери проверки root identity.

Raw credentials/tokens/private keys/connection strings не принимаются supported
builders и не попадают в identity, provenance, vectors, logs или explain. Raw
secret нельзя заменять unkeyed hash. `SecretReference` содержит только provider,
opaque stable identifier и immutable version. Если стабильной non-secret version
reference нет, credential execution-only и не входит в identity.

## Builders, extensions и diagnostics

Generic construction находится в публичном package `schemaauthor` — явной trust
boundary. Architecture check разрешает его только approved schema packages и
conformance fixtures. Schema объявляет name, allowed kind, requiredness,
semantic/operational role и public/protected disclosure, затем предоставляет
schema-bound builders.

Identity fields, canonical values, secret references, resolved extensions и
`OperationalObservation` идут по разным typed channels; diagnostic-to-identity
conversion отсутствует. Builder сверяет declaration с role-specific resolved
extensions и отклоняет missing/extra/reordered/mismatched values. Binding names:
`extension.input.<index>`, `extension.execution_environment`,
`extension.deployment`.

Текущие `ExtensionFingerprint`/`ComposeResolvedFields` frozen как legacy-v2.
Canonical-v1 использует отдельные typed функции; значения revisions несовместимы
на уровне типов.

## Integrity и disclosure

Persisted/transport surface — strict `FingerprintEnvelope` с descriptor и typed
subject, достаточным для recomputation. Decoder проверяет schema/kind/domain/
algorithm, canonical value tokens, field commitments, fingerprints, parent
links, StateIDs и endpoints; tampering даёт structured validation или
stable error code/path из taxonomy ниже.

Kinds `canonical-value`, `factory-state`, `transform`, `resolved-extension`,
`derived-state` имеют fixed domain mapping. Descriptor не имеет самостоятельного
trust-producing decoder: он декодируется только внутри subject-bearing envelope
и возвращается как verified read-only value.

Explain profiles:

- `safe` по умолчанию redacts protected payloads, opaque secret identifiers и
  per-field commitments, сохраняя уже verified aggregate digest/StateID/links;
- `internal` требует явного authorization decision и может показать protected
  non-secret values и полный secret reference;
- raw secret material запрещён в обоих.

Full envelope проверяет commitment из payload и recompute identity/lineage;
safe view — one-way projection, а не independently verified proof.

Token `civ1` можно parse для сравнения и отображения, но supported identity
builder принимает только полный `CanonicalValue` tree. Integrity envelope несёт
tree и recompute token; token-only subject не может получить verified status.

JSON strict, но не byte-canonical: whitespace и object member order незначимы;
duplicate/unknown members, invalid UTF-8, trailing values и invalid shape
запрещены. Validation идёт по фазам: document budget/syntax, shape/discriminator,
limits, descriptor mapping, payload commitments, identity digest, lineage и
endpoint. Public error содержит stable code/path, но не rejected value.

Stable codes: `document_too_large`, `syntax_invalid`, `unknown_member`,
`duplicate_member`, `revision_mismatch`, `shape_invalid`, `value_invalid`,
`limit_exceeded`, `non_canonical`, `descriptor_invalid`, `commitment_mismatch`, `digest_mismatch`,
`lineage_mismatch`, `endpoint_mismatch`, `authorization_denied`. Path использует
public member names и zero-based `[index]`, например `steps[1].state.id`; для
document-wide error path пустой.

Bundle parser дополнительно использует `path_invalid`, `file_missing`,
`file_unlisted`, `size_mismatch`, `file_digest_mismatch`,
`manifest_digest_mismatch`, `bundle_metadata_mismatch`, `vector_invalid` и
`relation_mismatch`; dynamic IDs используют JSON-quoted brackets, например
`files["vectors/limits.json"]` и `cases["limits/depth-over"]`.

Safe explain скрывает protected payload, opaque secret identifier и его
per-field commitment, чтобы не давать oracle для offline guessing. Он сохраняет
только уже verified aggregate digest/StateID/links. Explain — one-way projection
без trust-producing decoder и не является proof redacted payload. Internal
authorization выполняется до construction; nil, denial, error или canceled
context возвращает no projection.

Aggregate fingerprint/StateID публичен и всё ещё может быть oracle для полного
low-entropy identity. Safe explain убирает дополнительный per-field oracle, но
не обещает confidentiality content-derived ID. Sensitive providers используют
opaque high-entropy non-secret identifiers.

## Bundle и release

Conformance artifact — versioned directory с strict manifest, detached digest и
vector files; точная схема задана в
`runtime-v2-conformance-bundle-schema.md`. Одни embedded files проверяют Go,
Node, fresh process и clean public-proxy consumer.

Предлагаемая module version — `v0.3.0`. Checked-in notes содержат статические
schema/compatibility/migration/bundle сведения, но не собственный commit SHA.
После merge exact completion SHA на `main` проходит preflight; RC и GA tags
указывают на него. Workflow создаёт content-addressed non-overwriting
release-attestation asset с SHA,
tag/module, semantic schema, bundle-schema version, bundle version и detached
manifest digest. Verifier связывает asset с protected tag/proxy; storage сам по
себе не trust root.

Gates разделены: pre-merge tests/policy; RC proxy и clean consumer; GA proxy,
checksum и attestation; post-release closure. Более поздний gate не считается
PR unit test. PR #135 закрыл implementation issues #130/#131; evidence публикации
и post-GA closure завершены в issue #133.
