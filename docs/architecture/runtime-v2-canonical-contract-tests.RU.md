# Runtime v2 canonical v1: проект тестов

Статус: согласовано для issues #130/#131 2026-09-27 в 23:31
Asia/Novosibirsk (16:31 UTC), после критического review и переработки.

Дополнение test plan внешнего conformance facade согласовано 2026-09-29 для
issues #138/#139 после критического review и переработки.

План проверяет согласованные
[поток canonical-v1](runtime-v2-canonical-contract-flow.RU.md),
[структуру компонентов](runtime-v2-canonical-contract-structure.RU.md) и
[схему conformance bundle](runtime-v2-conformance-bundle-schema.RU.md). Stable ID
связывают требования с executable evidence. Suites v0.2.0 остаются immutable
legacy-v2 evidence и не перегенерируются кодом canonical-v1.

## 1. Evidence model и oracles

Evidence выполняется на стадии, где доступны его зависимости:

| Стадия | Доступно | Gate result |
| --- | --- | --- |
| PR | source, tests, draft embedded bundle, merge base | deterministic unit/public/API, architecture, compatibility и bundle-policy checks |
| PR fuzz | bounded-time fuzz jobs и checked-in seeds | поиск дефектов, но не единственное доказательство resource bound |
| RC | immutable RC tag и public proxy artifact | clean consumer, proxy zip/metadata, bundle и attestation-candidate checks |
| GA | immutable GA tag, checksum DB, release asset | same-commit, checksum, final attestation и consumer checks |
| post-GA | успешные GA evidence | только release closure #133, не PR test |

Oracle policy:

- **OR01 — locked known-answer vectors (PR):** reviewed input и точные preimage,
  digest, StateID, endpoint, entry digest и detached digest checked in; tests не
  могут их обновлять.
- **OR02 — independent reference verifier (PR/RC):** read-only Node и Go
  независимо recompute одни vectors без shared generated encoder или
  expected-output generator.
- **OR03 — vector-change policy (PR):** output offline generator — только review
  material. Принятие требует human review, новой bundle version и immutable
  policy check; CI не переписывает expected files.
- **OR04 — platform matrix (PR):** deterministic tests идут на Linux/Windows;
  integer conversion/compile включают `GOARCH=386`, race tests — поддерживаемый
  64-bit runner.
- **OR05 — traceability (PR):** каждый implementation test/vector указывает ID
  ниже. ID завершён только при наличии всех его assertions; coverage tag сам по
  себе недостаточен.

## 2. Canonical values и измеримые budgets

- **CV01 — primitive known answers (PR):** null и representative UTF-8 strings
  проверяют exact node bytes, framed identity-value preimage, digest и token.
- **CV02 — composite known answers (PR):** nested list/map/set проверяют точный
  count/length framing и полные canonical bytes, не только digest.
- **CV03 — deterministic order (PR):** exhaust всех permutations до пяти
  distinct map/set members плюс seeded property tests для больших collections;
  bytes равны. Ordered lists остаются order-sensitive.
- **CV04 — raw Unicode (PR):** NFC/NFD-equivalent strings byte-distinct, implicit
  normalization отсутствует, valid NUL/non-ASCII round-trip, invalid UTF-8
  rejected без replacement.
- **CV05 — duplicates (PR):** duplicate source map key rejected до sorting;
  duplicate encoded set member rejected. Отдельного normalization-collision
  класса нет, потому что keys unnormalized.
- **CV06 — token grammar/boundary (PR):** exact lowercase SHA-256 принимается;
  uppercase, whitespace, truncation, лишние separators/suffix и unsupported
  algorithm rejected. Parsed token пригоден для comparison/display, но один не
  строит verified identity field.
- **CV07 — malformed binary tree (PR):** unknown tag, invalid null payload,
  overflow, truncation, trailing bytes, wrong count, duplicate/non-canonical
  order и malformed nesting дают stable code/path и no value.
- **CV08 — zero/copy (PR):** zero `CanonicalValue`/token invalid; constructors и
  byte/tree accessors изолируют caller mutation.

Boundary tests используют root depth 1, members per collection, whole-tree
nodes/bytes и stricter key limit:

- **LM01 — каждый scalar boundary (PR):** depth, nodes, list/map/set members,
  string bytes, key bytes, canonical bytes и decoded-envelope bytes проверяются
  на `limit-1`, `limit`, `limit+1` с exact code/path.
- **LM02 — combined budgets (PR):** fixtures независимо исчерпывают nodes до
  bytes, bytes до nodes, members внутри nested tree и key против ordinary string;
  побеждает первая документированная validation phase.
- **LM03 — pre-allocation plan (PR):** same-package tests вызывают production
  non-allocating `allocationPlan` с hostile u32/u64, `MaxInt`/32-bit edges,
  arithmetic overflow и sort cardinality. Reject происходит до proportional
  `make`, recursion/sort, без injected allocator или alternate path.
- **LM04 — real decoder corpus (PR):** public decoder обрабатывает maximum-valid
  и compact over-limit wide/deep/duplicate-heavy input внутри 4 MiB без panic и
  с exact expected value/code/path. Plan counters доказывают, что rejected plan
  не входит в materialization/sort. Exact allocations/timing не обещаются.
- **FZ01 — canonical fuzz (PR fuzz):** token/tree/framing seeds проверяют no
  panic, atomic failure и decode→encode byte equality. Fuzzing не доказывает
  memory/time limits.

## 3. Typed fields, identities и lineage

- **ID01 — kind separation (PR):** text/canonical-value/secret-reference
  проверяют kind tags и distinct contributions; kind входит в contribution и
  field-set preimages.
- **ID02 — no token sniffing (PR):** text `civ1:` остаётся text;
  canonical-value field требует полный tree.
- **ID03 — immutable opaque values (PR):** zero values fail; mutations всех
  constructor inputs/accessor results не меняют fields, references, identities,
  descriptors, envelopes, observations и lineages.
- **ID04 — exact identity answers (PR):** factory, transform, extension, root и
  derived state проверяют tags/lengths, contributions, preimage, domain, digest,
  StateID и endpoint.
- **ID05 — sensitivity matrix (PR):** schema, provider/owner, semantic kind,
  identity schema, field name/kind/payload, parent, transform/order меняют нужную
  projection; field permutations и operational observations — нет.
- **ID06 — five-domain separation (PR):** один payload в пяти domains даёт пять
  разных digests; wrong kind/domain pair rejected.
- **ID07 — secret-reference semantics (PR):** provider, opaque ID и immutable
  version обязательны и независимо identity-bearing. Supported builders не имеют
  raw-secret channel. Tests гарантируют только typed channels/enumerated sinks:
  generic trust boundary не классифицирует arbitrary text как secret.
- **ID08 — revision isolation (PR):** legacy принимает только
  `sqlrs.runtime.v2`, canonical — `sqlrs.runtime.v2.canonical.v1`; cross-revision
  decode, descriptor, anchor и rehash fail explicitly.
- **LN01 — recipe lineage (PR):** factory-only, one/many ordered transforms
  recompute root, intermediate/final StateID и endpoint; reorder меняет lineage.
- **LN02 — relative lineage (PR):** verified factory/derived anchor и zero/one/
  many transforms recompute links. Bare, token-only, unverified, legacy или
  mismatched anchor rejected до construction.

## 4. Composition, schema trust и architecture

- **CO01 — canonical extension (PR):** exact vectors покрывают owner, semantic
  kind, identity schema, typed fields и domain.
- **CO02 — role completeness (PR):** declaration count/position/role/owner/kind
  enforced; missing, extra, duplicate, reordered, unknown, cross-role и
  transform-deployment fail atomically.
- **CO03 — legacy API freeze (PR):** known answers v0.2.0 для
  `ExtensionFingerprint`/`ComposeResolvedFields` unchanged; canonical functions
  принимают только distinct new types.
- **BU01 — schema validation (PR):** duplicate/unstable names, invalid role/kind/
  disclosure, impossible requiredness и inconsistent observation schema fail;
  valid schema/accessors immutable.
- **BU02 — transactional builders (PR):** required fields/roles enforced;
  rejected method не меняет state; successful build seals single-use builder;
  identity, extension, observation и executor-private channels не конвертируются.
- **BU03 — supported builders (PR):** для каждой checked-in schema semantic input
  меняет identity, а timestamp/path/runtime/job ID, cache/checkpoint, acquisition
  observation и execution-only credential не входят в canonical bytes.
- **BU04 — AST/package graph (PR policy):** exact allowlist и production path
  через approved schema facade проходят; direct/bypassing graph path, alias,
  constructor reference, generic exported type, wrapper и re-export rejected.
  Проверяются все production packages и graph paths, не sample.

## 5. Integrity, errors и disclosure

- **IN01 — descriptor matrix (PR):** каждый kind имеет одну schema/domain/
  algorithm и metadata shape; wrong pairs/unknown rejected.
- **IN02 — full recomputation (PR):** value/factory/transform/extension/recipe/
  relative envelopes recompute tree token, contributions, digests, links и
  endpoint. Missing AST fails; token alone не получает verified status.
- **IN03 — single-fault tampering (PR):** отдельные fixtures меняют каждый
  descriptor field, public/protected payload, contribution, digest, parent,
  step, StateID, endpoint или discriminator и проверяют code/path.
- **IN04 — validation precedence (PR):** multi-fault проверяет только порядок:
  byte budget/syntax → shape/discriminator → limits → descriptor → payload
  commitment → identity digest → lineage/endpoint.
- **IN05 — strict semantic JSON (PR):** duplicate/unknown members, invalid UTF-8,
  forbidden null, trailing value и invalid shape fail atomically; whitespace и
  member permutations succeed. Canonical JSON order не выдумывается.
- **IN06 — no standalone trust (PR):** descriptor, parsed token, explanation и
  bare anchor не имеют decoder/conversion в trusted envelope/identity/StateID.
- **EX01 — safe one-way projection (PR):** protected payload, opaque secret ID и
  per-field commitment отсутствуют; typed markers есть, aggregate digest/StateID/
  links равны verified source. Output не декодируется как proof.
- **EX02 — internal auth (PR):** allow/live context показывает только разрешённые
  protected non-secret values/reference parts. Nil, deny, error и canceled
  context возвращают zero projection без partial serialization.
- **EX03 — runtime canary sinks (PR):** случайный non-checked-in canary подаётся
  через executor-private credential input и wrong-kind request supported schema;
  builder errors, их `%v`/`%+v`, captured logs, produced envelope/explain и bundle
  output его не содержат. Generic text вне claim: public trust boundary не умеет
  классифицировать arbitrary text как secret.

## 6. Conformance bundle и repository policy

- **CB01 — detached digest (PR):** exact domain/schema framing, UTF-8 order, u32
  count, u64 sizes, raw entry hashes и final digest independently recomputed;
  manifest/digest excluded.
- **CB02 — in-memory paths (PR):** empty, `.`, `..`, repeated separator,
  absolute/UNC, drive, backslash, control, invalid UTF-8, uppercase, non-ASCII,
  invalid segment, duplicate, unsorted, reserved и
  missing/extra/unlisted paths fail до vectors.
- **CB03 — filesystem objects (PR policy):** no-follow walk принимает regular
  files и запрещает symlink, junction/reparse point, directory в entry и прочие
  non-regular objects. In-memory parser не делает symlink assertion.
- **CB04 — file bytes/integrity (PR):** UTF-8, LF и один terminal newline;
  wrong size, uppercase/malformed digest, changed bytes, CRLF, missing newline,
  missing/extra file rejected. Detached digest file — ровно одна строка
  `sha256:<lowercase-hex>\n`.
- **CB05 — metadata binding (PR):** изменение bundle-schema, bundle или semantic
  version против expected descriptor rejected даже при прежнем entries digest;
  current loader использует exact compiled constants.
- **CB06 — vector schema (PR):** vector/case/relation IDs grammar/global unique/
  sorted; tags unique/sorted; operation, expected/error, comparison/projection и
  referenced IDs valid. Mis-tagged case не закрывает coverage.
- **CB07 — semantic coverage (PR):** vectors покрывают values/limits, пять
  domains, composition, recipe/relative, same-resolution, identity changes,
  diagnostics-only, secret revision, redaction, tampering, builder metadata и
  legacy non-reinterpretation.
- **CB08 — relations (PR):** required equality/inequality recomputed из cases;
  recorded digest не сравнивается только со своей копией.
- **CB09 — immutability (PR policy/RC):** PR сравнивает merge base и требует
  новую directory/version при изменении existing content/path; RC повторяет с
  latest published tag. Для initial unpublished content нет fake baseline.
- **CB10 — ownership/concurrency (PR):** files/manifest/accessors defensive;
  repeated/concurrent read/verify deterministic и race-free под `go test -race`.

## 7. Legacy compatibility и migration boundary

- **CP01 — legacy golden lock (PR):** binary/JSON goldens, fingerprints, StateID,
  endpoints и public tests v0.2.0 проходят byte-for-byte без изменений.
- **CP02 — side-by-side consumer (PR):** один external module держит обе revisions
  и dispatch по explicit discriminator в consumer code; decoder не guess/upgrade/
  fallback.
- **CP03 — re-resolution (PR):** existing declaration/resolver flow плюс явно
  выбранная supported canonical schema строят новый value. Новый generic
  migration API не вводится; legacy fingerprint/StateID недостаточен как input.

## 8. Release gates

- **RL01 — static release material (PR):** checked-in notes называют schema,
  immutable legacy separation, side-by-side/re-resolution, bundle versions и
  digest, без placeholder/self-referential source SHA.
- **RL02 — source-tree consumer (PR):** temporary module вне `go.work`, без
  `replace`, использует staged module zip через temporary file-backed `GOPROXY`
  и проверяет values, builder, envelope, explain и bundle. Это не internet
  public-proxy test.
- **RL03 — RC public consumer (RC):** immutable `v0.3.0-rc.N` разрешается через
  proxy; zip/metadata/checksum, embedded bundle и tagged commit совпадают; clean
  consumer проходит.
- **RL04 — generated attestation (RC/GA):** automation строит content-addressed
  non-overwriting asset из tag без source mutation; связывает module/tag, exact
  SHA, schema/bundle versions и digest. Tests проверяют exact member order,
  UTF-8/LF/newline, companion digest, digest в asset name, no extras, tag/proxy
  comparison и tampering каждого field.
- **RL05 — same-commit GA (GA):** exact `backend/libs/runtime-go/v0.3.0` и
  verified RC tag указывают на один completion commit; proxy/checksum DB serve
  matching content и final attestation.
- **RL06 — closure (post-GA):** #133 закрывается только после RL03–RL05;
  implementation issues #130/#131 закрыты PR #135. Failure до tag ничего не
  публикует. Failure после RC/GA tag оставляет его immutable и
  блокирует promotion/closure; transient verification можно rerun только для
  того же tag, без move.

## 9. Existing-test contradiction и coverage review

Review завершён 2026-09-27 после approval. Existing module, resolver,
architecture, workflow, golden, external-consumer и release tests не
противоречат плану. Их constants/domains/bytes `sqlrs.runtime.v2` и assertions
release v0.2.0 — обязательные legacy evidence CP01/CO03 и остаются unchanged.

Найдены additive harness gaps, а не conflicts: local consumer использует
`replace`, workflows проверяют только legacy goldens/API, canonical-v1 bundle и
reference verifier отсутствуют. RL02 заменяет local-only path временным
file-backed proxy; legacy assertions сохраняются, canonical добавляются.
Существующие Go suites, оба Node legacy verifier и workflow-contract test прошли
до реализации новых тестов.

После реализации coverage измеряется по repository policy. Uncovered branch
связывается с approved requirement или рассматривается как candidate dead code;
tests undocumented implementation details не считаются целью сами по себе.

## 10. Дополнение внешнего conformance facade

Дополнение переиспользует существующие canonical-v1 oracles и не меняет bundle
или его locked vectors. Все behavioral tests facade используют внешний package
`conformancev1_test`; source/API architecture policy может проверять syntax, но
не вызывает private implementation helpers. Clean-consumer fixture компилируется
вне `go.work` и не импортирует `schemaauthor`.

- **CF01 — точный public API и документация (PR):** AST/API allowlist отклоняет
  любой exported symbol вне спроектированных constants и functions, включая
  exported variables, schema capabilities, generic helpers и type aliases.
  Compile-time и behavioral assertions покрывают каждую exported константу
  provider, kind, identity schema, observation schema, specification schema,
  semantic field, observation field и declaration field. `go doc`/source policy
  проверяет doc comment каждого symbol и указывает, что package предназначен для
  contract verification, а не production provider.
- **CF02 — role-specific construction builders (PR):** constructors factory,
  transform и extension строят identities с точными provider, kind и identity
  schema. Factory/transform declarations с неверным kind возвращают существующий
  stable builder code/path. Отсутствующий обязательный `locator` отклоняется;
  каждый valid builder закрывается после одного успешного build. До sealing
  rejected additions identity field и observation оставляют builder пригодным
  для следующей valid operation; premature build без `locator` также можно
  дополнить и повторить. Каждый constructor failure возвращает nil builder,
  поддерживает `errors.Is`/`errors.As` и имеет exact code/path. Каждый failure
  transactional.
- **CF03 — все kinds identity fields (PR):** каждый из трёх builders принимает
  `locator` как public text, `plan` как protected canonical value и `credential`
  как protected secret reference; неверные names и kinds отклоняются. Optional
  fields могут отсутствовать, mutation caller не меняет принятые values.
- **CF04 — role-specific observations и isolation (PR):** каждый опубликованный
  operational name принимается соответствующим observation constructor и
  builder. Cross-role observations отклоняются. Изменение только observation
  value или порядка не меняет canonical bytes, fingerprints и envelopes;
  observations остаются в composition result. Input maps и возвращаемые slices
  являются defensive copies. `nil` maps, empty maps, empty values и maps со всеми
  разрешёнными names явно принимаются.
- **CF05 — deterministic validation observations (PR):** names проверяются в
  raw-byte order до values, которые проверяются в том же порядке. Несколько
  неизвестных identifiers возвращают первый sorted path с `CodeUnknownMember`;
  invalid UTF-8, control-character и прочие invalid identifier names возвращают
  `CodeValueInvalid` с `fields.name` без включения name. Несколько invalid values
  выбирают первое sorted field. Invalid UTF-8 values возвращают
  `CodeValueInvalid`; ASCII и multibyte UTF-8 values на границах
  `MaxCanonicalStringBytes-1`, byte limit и byte-limit-plus-one проверяют
  acceptance и `CodeLimitExceeded`. Mixed invalid-name/invalid-value cases
  доказывают приоритет name phase. Каждый failure возвращает zero observation,
  удовлетворяет `errors.Is(..., runtimev2.ErrInvalid)`, обнаруживается как
  `*runtimev2.ValidationError` через `errors.As`, имеет exact code/path и не
  раскрывает rejected payload или unsafe name через `%v` или `%+v`.
- **CF06 — узкие extension declarations (PR):** helpers input,
  execution-environment и deployment возвращают точные role,
  `runtimev2.SchemaVersion`, owner, kind, specification schema и одно поле
  `reference`. References проверяются на empty, invalid UTF-8 и границах
  ASCII/multibyte `MaxResolvedValueBytes-1`/byte-limit/byte-limit-plus-one.
  Valid non-ASCII и NUL-bearing references round-trip без изменений. Каждый
  failure возвращает zero declaration соответствующей role, поддерживает
  `errors.Is`/`errors.As`, имеет canonical code и path `reference` и не раскрывает
  rejected value через `%v` или `%+v`.
- **CF07 — role-complete composition extensions (PR):** созданные facade
  extension identities связываются с ролями factory input/environment/deployment
  и transform input/environment. Position и role сохраняются: missing и extra
  identities атомарно отклоняются, а reorder двух valid input identities
  принимается и меняет position-sensitive identity factory/transform. Deployment
  доступен только в factory binding type; transform API не имеет deployment
  channel. Существующие root builder tests остаются oracle для rejection
  owner/kind mismatch. Rejected bind transactional и допускает последующий valid
  complete binding на том же builder.
- **CF08 — sensitivity и disclosure (PR):** изменение locator, plan или любого
  компонента secret reference меняет identity; изменение только observation —
  нет. Safe explanation показывает text/commitment public locator и typed
  redaction markers, но не protected plan payload, opaque secret identifier или
  commitment protected field. Авторизованный
  internal explanation показывает разрешённые protected values/reference parts
  и никогда raw secret. Nil authorizer, deny/authorizer error и cancellation до
  или во время authorization возвращают документированный authorization error и
  zero projection без утечки partial protected result.
- **CF09 — исполнимая import boundary (PR policy):** repository-wide AST tests
  разрешают `schemaauthor` только approved schema packages/fixtures, а
  `schemas/conformancev1` — только собственной реализации, Go test files и явно
  указанному fixture `test/runtime-v2-consumer`. Direct, aliased, wrapped и
  re-exported production imports отклоняются. Facade не экспортирует generic
  schema type или constructor. `go.mod` остаётся unchanged, facade не добавляет
  third-party module dependency. Unversioned forwarding facade или alias package
  не добавляется.
- **CF10 — чистый внешний consumer (PR):** staged-module fixture через
  file-backed proxy, вне `go.work` и без `replace`, импортирует
  `schemas/conformancev1`, но не `schemaauthor`; он строит все три identities,
  все field kinds, все extension roles, observations, envelopes и safe/internal
  explanations. Проверяются sensitivity secret version и isolation observations.
- **CF11 — concurrency и ownership (PR/race):** concurrent constructor use
  deterministic и race-free. Package-owned schema state immutable; input maps,
  declarations, fields и возвращаемые observations не могут изменить другой
  builder или последующую construction.
- **CF12 — compatibility и bundle immutability (PR):** все существующие tests и
  goldens legacy-v2/canonical-v1 проходят без изменений. Существующие vector
  files canonical-v1, bundle version, manifest и detached digest остаются
  byte-identical без отдельно согласованной bundle revision.
- **RF01 — release material для #139 (PR/RC):** checked-in release notes называют
  facade contract-verification-only, сохраняют обе опубликованные semantic
  revisions и фиксируют неизменённые bundle metadata без self-referential source
  SHA. Generated provenance/attestation связывает exact module tag и source
  commit. После fetch `origin/main` и remote tags release automation выбирает
  следующий свободный minor release модуля по существующей version policy для
  additive API и доказывает отсутствие его RC/GA tags до создания. License,
  Workflow и его contract tests проверяют generic syntax semver/RC Runtime
  module и динамически выбирают соответствующий release-notes file; следующий
  release не требует изменения version literals в tests. Module zip, checksum,
  license, conformance-bundle и
  provenance/attestation gates выполняются для одного immutable candidate.
- **RF02 — public-proxy consumer для #139 (RC/GA):** после создания immutable
  candidate и GA tag тот же source CF10 разрешается через public Go proxy и
  checksum database без `replace`, с clean module cache и explicit public
  proxy/checksum settings. Availability может использовать bounded retry;
  mismatch metadata, checksum, zip или source является terminal. Proxy zip
  совпадает с reviewed tag. Failure не перемещает существующий tag и блокирует
  закрытие issue.

Review существующих tests на противоречия завершён 2026-09-29 после согласования.
Assertions canonical builders, disclosure, architecture, bundle, legacy goldens
и external consumer совместимы и остаются additive evidence. Review нашёл одно
устаревшее ограничение harness: release workflow, его contract test и staged
consumer были закреплены за уже опубликованным циклом `v0.3.0`. Пользователь
согласовал замену version-specific assertions на reusable gates semver, dynamic
release notes и temporary staged consumer с сохранением `v0.3.0` как immutable
historical compatibility evidence. Противоречий требований больше нет.

Coverage измеряется per package с target 100% и минимумом 95%; deficit
публикуется с per-line evidence и отдельным планом исправления для согласования.
