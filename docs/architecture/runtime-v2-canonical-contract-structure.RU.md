# Canonical v1 contract Runtime v2: структура компонентов

Статус: согласовано для issues #130/#131 2026-09-27.

Дополнение внешнего conformance facade: согласовано для issue #138 2026-09-29.

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

### Schema-safe внешний conformance facade

Чистому внешнему consumer требуется проверять все canonical-v1 builder channels
без импорта generic trust boundary `schemaauthor` и без копирования provider
schema. Поэтому package `runtime-go/schemas/conformancev1` владеет одним
фиксированным семейством schemas для проверки контракта:

```go
const Provider = "runtime-conformance"
const FactoryKind = "fixture-factory"
const TransformKind = "fixture-transform"
const ExtensionKind = "fixture-extension"

const FactoryIdentitySchema = "runtime-conformance.factory.v1"
const TransformIdentitySchema = "runtime-conformance.transform.v1"
const ExtensionIdentitySchema = "runtime-conformance.extension.v1"
const FactoryObservationSchema = "runtime-conformance.factory-observation.v1"
const TransformObservationSchema = "runtime-conformance.transform-observation.v1"
const ExtensionObservationSchema = "runtime-conformance.extension-observation.v1"
const ExtensionSpecificationSchema = "runtime-conformance.extension-specification.v1"

const FieldLocator = "locator"       // обязательный public text
const FieldPlan = "plan"             // необязательное protected canonical value
const FieldCredential = "credential" // необязательная protected secret reference

const ObservationJobID = "job_id"
const ObservationContainerID = "container_id"
const ObservationTimestamp = "timestamp"
const ObservationPhysicalSize = "physical_size"
const ObservationMaterializationPath = "materialization_path"
const ObservationCheckpointBackend = "checkpoint_backend"
const DeclarationReference = "reference"

func NewFactoryBuilder(runtimev2.FactoryDeclaration) (*runtimev2.FactoryIdentityBuilder, error)
func NewTransformBuilder(runtimev2.TransformDeclaration) (*runtimev2.TransformIdentityBuilder, error)
func NewExtensionBuilder() (*runtimev2.ExtensionIdentityBuilder, error)

func NewFactoryObservation(map[string]string) (runtimev2.OperationalObservation, error)
func NewTransformObservation(map[string]string) (runtimev2.OperationalObservation, error)
func NewExtensionObservation(map[string]string) (runtimev2.OperationalObservation, error)

func NewInputDeclaration(reference string) (runtimev2.InputDeclaration, error)
func NewExecutionEnvironmentDeclaration(reference string) (runtimev2.ExecutionEnvironmentDeclaration, error)
func NewDeploymentDeclaration(reference string) (runtimev2.DeploymentDeclaration, error)
```

Три identity schemas используют один фиксированный словарь semantic fields,
чтобы внешний contract test мог проверить text, structured values, secret
references, disclosure и diagnostic isolation для каждого builder kind.
Helpers operational observations связывают правильные provider, kind и
observation schema, принимают только фиксированные names выше, задают protected
disclosure и оставляют значения вне identity. Helpers extension declarations
фиксируют owner, kind, specification schema и одно непустое UTF-8 поле
`reference`; caller может менять declaration spelling без передачи map или
authoring identity schema.

Declaration helpers используют существующий immutable discriminator declarations
`runtimev2.SchemaVersion`: canonical-v1 меняет resolved identity, а не уже
опубликованный declaration wire contract. Их specification schema равна
`ExtensionSpecificationSchema`.

Observation helpers делают defensive copy map и сортируют names по raw bytes Go
string. Они проверяют все names в этом порядке до проверки values в том же
порядке. Name с invalid UTF-8 или не соответствующий grammar identifiers Runtime
возвращает `ValidationError{Code: CodeValueInvalid, Path: "fields.name"}`;
unsafe name не включается в path. Первый well-formed, но неизвестный identifier
возвращает `ValidationError{Code: CodeUnknownMember, Path: "fields." + name}`.
`nil` и empty maps допустимы, как и empty operational values. Invalid UTF-8
observation values возвращают `CodeValueInvalid`; значения длиннее
`MaxCanonicalStringBytes` bytes возвращают `CodeLimitExceeded`, оба с path
`"fields." + name`.

Declaration helpers отклоняют invalid UTF-8 или empty references с
`CodeValueInvalid`, а references длиннее `MaxResolvedValueBytes` bytes — с
`CodeLimitExceeded`, оба с path `reference`. Каждая validation error facade
поддерживает `errors.Is(err, runtimev2.ErrInvalid)`, обнаруживается как
`*runtimev2.ValidationError` через `errors.As`, возвращает zero result и не
включает reference/observation payloads или unsafe names в сообщение.

Facade не экспортирует schema type, field definition, произвольные
provider/kind, generic schema constructor или mutable variable. Constants и
functions из блока signatures выше являются точным allowlist exported API;
каждый symbol имеет Go documentation comment. Это API проверки контракта, а не
production provider schema. Production packages продолжают использовать свои
approved facades в `schemas/**`. Package path и identifiers навсегда связаны с
canonical-v1 и никогда не перенаправляются на следующую semantic revision.

Repository architecture checks сохраняют direct imports `schemaauthor` только
внутри реализаций schema facade и module-owned conformance fixtures. Отдельное
AST import rule отклоняет `schemas/conformancev1` из любого non-test Go file вне
самого facade и явно разрешённого fixture `test/runtime-v2-consumer`. Module
tests и этот clean external-consumer fixture могут его импортировать; repository
production code — нет. Так classification contract-verification-only становится
исполнимой внутри этого репозитория.

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
