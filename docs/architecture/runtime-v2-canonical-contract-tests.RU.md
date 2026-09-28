# Runtime v2 canonical v1: проект тестов

Статус: согласовано для issues #130/#131 2026-09-27 в 23:31
Asia/Novosibirsk (16:31 UTC), после критического review и переработки.

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
| post-GA | успешные GA evidence | только closure #131, не PR test |

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
- **RL06 — closure (post-GA):** #131 закрывается только после RL03–RL05; failure
  до tag ничего не публикует. Failure после RC/GA tag оставляет его immutable и
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
