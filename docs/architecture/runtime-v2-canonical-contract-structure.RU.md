# Canonical v1 contract Runtime v2: структура компонентов

Статус: согласовано для issues #130/#131 2026-09-27.

## Module boundary

Существующие `canonical.go`/`identity.go` остаются frozen legacy-v2. Новая
реализация разделена на `canonical_value_v1.go`, `identity_field_v1.go`,
`secret_reference_v1.go`, `canonical_identity_v1.go`,
`fingerprint_envelope_v1.go`, `explain_v1.go`, `builder_v1.go`, package
`schemaauthor`, internal core и многофайловый `conformance/canonical-v1/`.
Module остаётся standard-library-only.

`SchemaVersion` остаётся alias `sqlrs.runtime.v2` для source compatibility.
Новый `CanonicalSchemaVersion = sqlrs.runtime.v2.canonical.v1`. Экспортируются
пять domain constants и limits: depth 32, nodes 4096, collection members 256,
string 4096 bytes, map key 1024 bytes, canonical value 1 MiB, envelope 4 MiB.

## Canonical values и fields

Opaque `CanonicalValue` создаётся конструкторами null/string/list/map/set. Map
принимает `[]CanonicalMapEntry`, поэтому duplicate source keys видны до sort.
`Token()` возвращает opaque `CanonicalValueToken` с representation
`civ1:sha256:...`; zero values invalid, byte accessors defensive.

Production constructor/decoder используют non-allocating `allocationPlan`
helpers из `internal/canonicalv1`: u64-to-int conversion, remaining budgets и
sort cardinality проверяются до proportional `make`, recursion или sort. Tests
вызывают те же helpers без behavior-changing hooks или alternate allocator.

`IdentityField` имеет kind `text`, `canonical-value` или `secret-reference` и
только read-only accessors. Constructors не auto-parse text `civ1:...`.
Parsed `CanonicalValueToken` используется только для comparison/display и не
строит identity field: canonical-value field сохраняет полный immutable tree.
`SecretReference` создаётся только из provider, opaque non-secret identifier и
immutable version. Raw-secret constructor отсутствует.

Field contribution commitment включает framed name, kind tag и payload.
Canonical identity field set содержит name/kind/raw commitment и сортируется по
UTF-8 name. Full envelope проверяет commitment из payload. Safe explain скрывает
protected payload и per-field commitment, сохраняет только aggregate digest и
не является independently verifiable proof.

## Раздельные canonical-v1 types

`CanonicalFactoryIdentity`, `CanonicalTransformIdentity`,
`CanonicalResolvedExtensionIdentity`, `CanonicalState`, canonical recipe/relative
lineages и `CanonicalStateID` не совместимы по типам с legacy values. Их strict
decoders принимают только canonical discriminator. Derived-state encoder требует
verified transform envelope.

## Schema authoring и builders

Generic constructors находятся в `runtime-go/schemaauthor`; общий opaque state —
в `internal/canonicalv1`. AST/package-graph checker запрещает direct imports,
aliases, constructor references, generic type exposure и re-exports вне exact
allowlist. Transitive dependency production package допустима только через
approved schema facade: любой graph path сначала входит в allowlist. Production
adapters импортируют facade, а не trust-boundary package напрямую.

Schema field definition фиксирует name, semantic/operational role, allowed kind,
requiredness и public/protected disclosure. Schema-bound single-use builders
принимают `IdentityField`, `OperationalObservation` и role-specific resolved
extensions через разные channels. Rejected operation transactional; successful
Build seals builder. Observation-to-identity conversion отсутствует.

Factory/transform builders сверяют input count/order, owner/kind и presence
environment/deployment с declaration. Canonical binding names fixed.
`CanonicalExtensionFingerprint`/`ComposeCanonicalResolvedFields` принимают
только new types; текущие одноимённые legacy functions сохраняют old signatures
и bytes.

## Diagnostics, integrity и explain

`OperationalObservation` имеет owner/kind/schema, sorted fields и disclosure
class. Job IDs, handles, timestamps, physical sizes, paths и backends находятся
только здесь. Execution-only credentials остаются в private executor channel и
не входят в Runtime v2 objects.

`FingerprintEnvelope` содержит immutable descriptor и typed subject, достаточный
для recomputation tokens, commitments и fingerprint. Recipe/relative envelopes
проверяют parent links, StateIDs и endpoint; relative envelope требует verified
anchor envelope.

Пять descriptor kinds имеют fixed schema/domain/algorithm mapping. Provider
metadata required для factory/transform/extension и forbidden для canonical
value/derived state. Standalone trusted decoder отсутствует: descriptor
декодируется только внутри subject-bearing envelope.

`ExplainSafe` redacts protected payloads, opaque secret identifiers и их
per-field commitments. Projection строится только из verified envelope, не
имеет trust-producing decoder и не принимается как identity/envelope.
`ExplainInternal` проверяет non-nil authorizer и live context до construction;
nil/denial/error/cancel возвращает zero projection. Оба профиля исключают raw
secrets. Strict decode возвращает stable code/path из flow taxonomy.

## Bundle и release

`ConformanceBundle` загружает manifest, detached digest и immutable file map.
Loader сначала проверяет paths, lexicographic order, sizes, content digests и
manifest digest, затем semantic vectors. Public API возвращает bundle/schema
versions, digest и defensive file copies; external consumer не читает module
cache layout.

In-memory parser принимает expected descriptor с bundle-schema, bundle и
semantic versions и не делает symlink claims. Отдельный filesystem check не
follow links и запрещает symlinks/non-regular files. Pre-merge policy сравнивает
bundle с merge base; release preflight — с последним published module tag.

Checked-in notes содержат static schema/bundle facts без собственного SHA.
Release workflow создаёт content-addressed attestation asset из tagged commit с
exact source SHA, module/tag, schema versions и digest и отказывается overwrite
existing asset. Verifier всё равно сравнивает asset с protected tag/proxy, не
считая release storage intrinsically immutable. RC проверяет proxy/bundle/clean
consumer; GA — same commit, checksum DB, attestation и legacy boundary.

Attestation JSON имеет fixed member order `module`, `tag`, `source_sha`,
`canonical_schema`, `bundle_schema_version`, `bundle_version`,
`manifest_digest`, UTF-8/LF и один terminal newline без extra members. Companion
`.sha256` содержит digest exact bytes как `sha256:<lowercase-hex>\n`; digest также
входит в asset name. Trust всё равно задаёт comparison с tag/proxy.
