# Композиция aliases Runtime v2: дизайн тестов

Статус: согласовано @evilguest по issue #109, 2026-09-28 18:28:21 +07:00.
Review существующих тестов на противоречия завершён 2026-09-28 18:28:21
+07:00; противоречия не найдены.

Этот план проверяет согласованные
[interaction flow](runtime-v2-composition-flow.RU.md) и
[component structure](runtime-v2-composition-structure.RU.md). Test IDs являются
стабильными ссылками на requirements. Production implementation следует после
обязательного review существующих тестов на противоречия.

## 1. Уровни тестов и fixtures

Suite содержит четыре уровня:

1. white-box package tests для validation, bounds, traversal, immutability и
   точных error locations;
2. external-package conformance tests только через exported Go и JSON API;
3. проверенные JSON fixtures для одного nested recipe trace и одного standalone
   transform trace;
4. CLI-package tests для strict YAML adaptation и legacy classification без
   переключения текущего execution на Runtime v2.

Golden JSON проверяет документированный wire shape, а не случайный layout Go
fields. Declarations сравниваются структурно; порядок JSON object проверяется
только для явно canonical порядка alias names. Test helpers не должны сами
генерировать ожидаемые traversal, origins, error pointers или compatibility
result.

## 2. Alias document и public API

- **D01 — valid variants:** создать и round-trip-нуть empty document, transform,
  factory-only recipe, recipe-prefix reference, named-transform step,
  inline-transform step и полный документированный пример.
- **D02 — closed unions:** отклонить zero/multiple alias, base и step variants, а
  также missing, extra-for-variant, unknown и `null` members на каждом уровне.
- **D03 — strict JSON:** отклонить wrong/missing schema version, duplicate object
  members, trailing tokens, wrong JSON types, invalid UTF-8 и nil decode target.
  Failed decoding не меняет non-zero receiver.
- **D04 — names:** принять граничные valid lower-case alias names и отклонить
  empty, upper-case, leading non-letter, slash, control, non-ASCII и overlong
  names как в definitions, так и в references.
- **D05 — duplicate constructor names:** duplicate `NamedAliasInput` names дают
  ошибку до возврата value; duplicate JSON keys отклоняются до materialization в
  map.
- **D06 — canonical transport:** semantic round trips сохраняют step order и
  выводят aliases в unsigned bytewise name order независимо от порядка
  constructor input или object members.
- **D07 — immutable snapshots:** mutation constructor slices, nested Runtime
  declarations, arguments, attributes и values из accessors не меняет document,
  expanded value, trace или последующий JSON output.
- **D08 — zero and empty values:** constructed empty documents/catalogs валидны;
  zero-value documents/catalogs/traces дают только документированный empty
  accessor result и не marshal-ятся как valid data.
- **D09 — document bounds:** alias count и canonical JSON size проверяются на
  `limit-1`, `limit`, `limit+1` через constructors и decoders. Constructor
  accounting атомарно отклоняет oversize input; allocation profiling является
  advisory evidence, а не platform-sensitive correctness assertion.
- **D10 — nested Runtime validation:** каждая invalid factory/transform
  declaration доступна как `runtimev2.ValidationError`, получает
  `invalid_document`, сохраняет stable validation code/path и использует
  composition pointer enclosing declaration.

## 3. Построение catalog

- **C01 — empty and single-source catalogs:** empty constructed catalog даёт
  `missing_reference`; один valid source разрешает все содержащиеся aliases.
- **C02 — source IDs:** покрыть empty, invalid UTF-8, byte-length boundaries,
  duplicate IDs, bounded opaque Unicode IDs и сохранение diagnostics. Public
  package не интерпретирует IDs как paths; CLI-relative-path enforcement
  отдельно покрывает L04.
- **C03 — permutation determinism:** каждая permutation фиксированного
  three-source fixture и deterministic shuffled larger fixtures даёт equal
  expansion/trace либо одинаковые error code, primary location и contexts.
- **C04 — ambiguity:** duplicate alias definitions eagerly дают
  `ambiguous_reference`; candidates полные, bounded, отсортированы по `SourceID`,
  указывают на definitions и не зависят от source order.
- **C05 — precedence:** одновременные source-count, source-ID, zero-document,
  aggregate-size и alias-ambiguity defects выбирают документированный canonical
  error precedence.
- **C06 — aggregate bounds:** source-document count, total definition count,
  source-ID bytes и `MaxCatalogBytes` используют checked arithmetic и покрывают
  `limit-1`, `limit`, `limit+1` без partial catalogs.
- **C07 — snapshot isolation:** mutation source slices или замена input documents
  после `NewCatalog` не влияет на catalog; concurrent read-only expansion
  race-free.
- **C08 — no hidden lookup:** catalog creation и expansion не выполняют directory
  scan, environment/config lookup, implicit import, case normalization или source
  precedence fallback.

## 4. Детерминированный expansion

- **E01 — factory-only recipe:** expansion возвращает ровно одну copied factory,
  zero transforms и никакого synthetic step.
- **E02 — ordered steps:** named и inline transforms сохраняют буквальный source
  order; repeated references остаются repeated output transforms.
- **E03 — recipe prefix:** one- и multi-level `base.recipe` chains добавляют
  deepest factory и каждый prefix transform перед child steps.
- **E04 — multiple migration roots:** разные named migration transforms одного
  recipe остаются последовательными и никогда не сортируются/распараллеливаются.
- **E05 — standalone transform:** `ExpandTransform` возвращает versioned
  `TransformDeclarationDocument` без обязательного recipe membership.
- **E06 — top-level errors:** invalid target names, missing targets и wrong-kind
  targets возвращают документированные code и primary source/pointer.
- **E07 — nested reference errors:** missing и wrong-kind base/step references
  указывают на точный escaped RFC 6901 reference location.
- **E08 — cycles:** self и multi-node prefix cycles возвращают exact closed cycle
  от первого revisited recipe до repeated closing entry; primary error указывает
  на closing reference.
- **E09 — deep iterative chain:** максимальная valid recipe chain проходит без
  recursion/stack growth; одно definition/count сверх bound падает
  детерминированно.
- **E10 — output bounds:** transform count, expanded-declaration JSON size, trace
  node/origin counts и trace JSON size покрывают boundary matrices и атомарно
  возвращают `expansion_too_large`.
- **E11 — semantic exclusion:** alias/source renaming с обновлением references
  может изменить только trace diagnostics, а source-document reordering не меняет
  ни declaration, ни trace. Equal expanded declarations не получают alias
  names/source paths от composition.
- **E12 — declaration sensitivity:** изменение referenced validated declaration
  или step order меняет expanded declaration; trace-only metadata не меняет.
- **E13 — provider opacity:** перемещение composition source или изменение только
  `SourceID` не rebases `reference`, argument, attribute или extension fields.
  Package не вызывает resolver/provider.
- **E14 — atomic failure:** каждая expansion failure возвращает zero results, без
  declaration и partial trace; следующий valid call того же catalog не затронут.

## 5. Expansion trace и errors

- **T01 — recipe golden:** nested recipe fixture проверяет target/base/transform
  node order, parent IDs, definition pointers, reference pointers, factory
  origin, transform indexes и exact schema version.
- **T02 — standalone golden:** standalone fixture содержит один transform target,
  не содержит factory member и имеет ровно один transform origin.
- **T03 — occurrence model:** repeated use одного alias создаёт distinct nodes с
  distinct reference pointers; inline transforms не создают extra node и
  указывают на node содержащего recipe.
- **T04 — strict trace decoding:** отклонить unknown/duplicate/missing/null
  members, wrong schema/kind, trailing tokens, invalid UTF-8 и mutation при
  failed decode.
- **T05 — graph integrity:** отклонить non-dense IDs, non-zero target, parent у
  target, missing non-target parent/reference pointer, non-contiguous recipe
  prefixes, non-chain recipe parents, transform nodes как parents,
  forward/dangling links, unused nodes и standalone traces не из одного target
  node.
- **T06 — origin and pointer integrity:** отклонить factory не у последнего recipe
  node, transform nodes с zero/multiple uses, несовпадение named-node/origin order,
  invalid recipe/step output order, non-contiguous step indexes, wrong/missing
  output indexes, invalid node aliases/source IDs и non-canonical/overlong
  definition, reference, factory, named-transform и inline-transform pointers.
- **T07 — accessors:** node, factory-origin, transform-origin и trace accessors
  возвращают точные values и defensive copies; absent parent/factory используют
  документированный boolean result.
- **T08 — trace size:** максимальные valid trace counts/JSON bytes проходят;
  `MaxTraceNodes+1`, `MaxTransforms+1` origins и `MaxTraceJSONBytes+1` падают без
  mutation receiver.
- **T09 — stable error envelope:** failures из composition constructors, catalog
  expansion, dedicated decoders и value-level JSON validation соответствуют
  `ErrInvalid`, поддерживают `errors.As(*Error)`, содержат ожидаемые code/source/
  pointer и zero irrelevant contexts. Dedicated decoders оборачивают malformed
  syntax; native `encoding/json` syntax error до `UnmarshalJSON` проверяется
  только как decode failure.
- **T10 — context bounds:** cycle/ambiguity contexts соблюдают
  `MaxDiagnosticReferences`; returned slices являются defensive copies.
- **T11 — error precedence:** table tests покрывают одновременно invalid target,
  missing/wrong-kind reference, cycle и output-limit conditions и проверяют
  первую failure в документированном target/base/deepest-to-target step traversal,
  включая resolve-before-projected-limit rule.
- **T12 — non-disclosure:** composition error strings могут содержать только code
  и caller-supplied bounded primary `SourceID`/pointer; они не содержат
  declaration references, arguments, attributes, contexts или другой payload.
  CLI tests отдельно доказывают, что generated source IDs не являются absolute
  paths.
- **T13 — diagnostic text bounds:** generated pointers canonical и не превышают
  `MaxPointerBytes`; decoded pointers сверх limit отклоняются атомарно. Каждая
  package-produced error string содержит valid UTF-8 и не превышает
  `MaxErrorTextBytes`, включая errors из maximum-size accepted source IDs и
  pointers.

## 6. CLI YAML и legacy compatibility

- **L01 — YAML equivalence:** документированный YAML отображается в тот же
  `AliasDocument`, что strict JSON/constructor input, без второго semantic model.
- **L02 — YAML rejection:** отклонить duplicate keys, anchors, aliases, merge
  keys, custom tags, directives, multiple documents, unknown fields, invalid
  UTF-8 и raw byte-limit overflow. Boundary cases покрывают `MaxYAMLDepth` и
  `MaxYAMLNodes`; boolean, integer, float, timestamp, binary и null scalars
  отклоняются, а не coerced в strings. Depth начинается с one для root mapping;
  node counts включают keys и исключают document wrapper. Walk итеративный.
- **L03 — YAML atomicity:** `DecodeAliasDocumentYAML` отклоняет nil target, а
  каждая adapter failure оставляет non-zero target неизменным и не возвращает
  partial document или constructor input.
- **L04 — translation input:** canonical absolute workspace/alias paths,
  contained alias location, exact derived workspace-relative slash `SourceID`,
  effective image и closed `ImageSource` values валидируются до adapter
  invocation. Path cases покрывают non-canonical, equal-root, outside-root,
  absolute/mismatched source IDs, backslashes, dot segments и byte overflow.
  Image matrix покрывает explicit-image/alias-source, inherited-image/workspace-
  or-global-source, empty-image/zero-source и все inconsistent combinations.
- **L05 — result invariants:** checked constructors принимают ровно один valid
  translated recipe либо non-empty legacy-only reason/remedy и отклоняют mixed,
  empty, unknown-status и zero-value states. Legacy-only messages считают UTF-8
  bytes, принимают `limit-1` и `limit`, атомарно отклоняют `limit+1` и сохраняют
  valid UTF-8.
- **L06 — prepare translation:** deterministic fake provider получает unchanged
  prepare definition и fully materialized defaults; его complete recipe
  возвращается как `translated` с immutable declaration.
- **L07 — run classification:** каждый legacy run alias возвращает
  `legacy_semantics_unsupported` без вызова provider adapter.
- **L08 — unavailable/default classifications:** nil provider и unavailable
  effective default дают `runtime_v2_provider_unavailable` и
  `legacy_default_unavailable` с actionable bounded messages; run classification,
  provider absence, default absence и kind mismatch соблюдают documented
  precedence.
- **L09 — kind handshake:** lower-case exact adapter kind проходит; empty,
  differently cased и mismatched kinds являются contract errors и не дают
  approximate declaration.
- **L10 — provider-declared unsupported input:** adapter может вернуть точный
  `legacy_only` для unsupported arguments; обычное отсутствие поддержки не
  представляется operational error.
- **L11 — provider conformance:** reusable suite требует от каждого production
  adapter покрыть все supported file-bearing arguments, materialize
  alias-relative paths в explicit provider/workspace form ровно один раз,
  применить defaults, отклонить unsupported forms и вернуть immutable
  declarations. Equivalent fully materialized inputs дают equal declarations
  независимо от diagnostic `SourceID`/`ImageSource`; adapters не копируют эту
  provenance в semantic declaration fields. Fake доказывает сам suite; provider
  не считается supported, пока не пройдёт его.
- **L12 — current CLI regression:** `plan`, `prepare`, `run`, `alias create` и
  `alias check` сохраняют существующие parsing, execution path и records; один
  import или вызов classification не выбирает Runtime v2 execution.
- **L13 — hostile adapter contract:** nil и typed-nil adapters, zero result с nil
  error, result вместе с non-nil error, invalid/oversized legacy-only result и
  panic-free kind mismatch возвращают documented classification либо zero-result
  contract error без partial declaration.

## 7. Conformance, fuzz, dependency и sequencing gates

- **X01 — external API conformance:** external-package test создаёт, expands,
  inspect-ит, serializes и decodes values только через документированный exported
  API.
- **X02 — downstream identity check:** external integration test передаёт graphs
  одному explicit deterministic resolver test provider. По-разному названные
  graphs с equal expanded declarations дают equal resolved identity, а trace
  changes не входят в resolver inputs. Изменение identity-bearing child
  declaration или перестановка двух distinct transform steps меняет resolved
  recipe fingerprint и resulting StateID через existing semantic core.
- **X03 — fuzz document JSON:** seeds включают каждый valid variant и boundary;
  fuzzing не panic-ует, не принимает trailing/duplicate data, не меняет receiver
  при failure и не возвращает value, не проходящий immediate revalidation.
- **X04 — fuzz trace JSON:** seeds включают оба goldens и graph mutations; каждый
  accepted trace удовлетворяет dense-node, parent, variant, pointer и origin
  invariants.
- **X05 — fuzz graph expansion:** small bounded generated catalogs либо совпадают
  с deliberately recursive reference flattener с independent data model, либо
  возвращают structured error без production recursion, nondeterminism или
  partial output.
- **X06 — race/concurrency:** concurrent expansion/serialization одного immutable
  small catalog проходит `go test -race`; caller mutation не может race-ить
  через retained internal reference.
- **X07 — dependency boundary:** dependency checks Runtime module по-прежнему
  разрешают только standard library и packages собственного module; CLI/YAML,
  resolver implementation, engine, storage и execution не попадают в
  `composition`.
- **X08 — staged release boundary:** public composition package и external
  consumer проходят до pin опубликованной immutable version CLI module.
  Compatibility slice компилируется без parent/subpackage import cycle и не
  становится default-runtime cutover gate.
- **X09 — release success matrix:** чистый external consumer раскрывает
  factory-only, one-step, multi-step и nested recipes и проверяет точный порядок
  transforms, а также публичные схемы alias document и expansion trace.
- **X10 — release identity and failures:** тот же consumer доказывает, что
  переименование alias не влияет на identity, перестановка steps влияет, а
  missing, cycle и cross-source ambiguity failures сохраняют стабильные
  публичные codes и упорядоченную диагностику.
- **X11 — release cache compatibility:** тот же consumer создаёт и round-trip-ит
  `resolver.CacheRecord` только через exported API, затем декодирует и байт-в-байт
  кодирует опубликованный wire fixture v0.2.0.
- **X12 — один staged/public oracle:** staged file-proxy gate и public
  proxy/checksum gate запускают одни и те же checked-in исходники тестов чистого
  consumer. Public gate не подменяет их более узким inline smoke test.

## 8. Coverage и acceptance

Deterministic package tests выполняются через `go test ./... -count=1`; external
consumer — с `GOWORK=off`. Fuzz seeds входят в обычный PR suite, timed campaigns
следуют текущей Runtime v2 nightly/release policy. Race tests выполняются на
поддерживаемой race-enabled CI platform. Fixtures на 10,000 nodes и 32 MiB
выполняются один раз в deterministic resource suite, не под race или timed fuzz.

Coverage измеряется отдельно для `runtime-go/composition` и
`frontend/cli-go/internal/alias/runtimev2` с per-line reports. Target — 100%,
minimum — 95%. Любой shortfall следует отдельному approval loop репозитория.

Release slice composition module принимается после D01-D10, C01-C08, E01-E14,
T01-T13, X01-X12 и staged/public consumer gates. Issue #109 остаётся открытой,
пока later compatibility slice также не пройдёт L01-L13; исполняемая CLI-
классификация `legacy_only` не является gate публикации runtime-go v0.3.0.
