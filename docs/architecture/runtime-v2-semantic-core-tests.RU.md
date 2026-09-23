# Семантическое ядро Runtime v2: проект тестов

Статус: предложение для issue #107 после критического анализа 2026-09-23;
ожидает согласования пользователя до создания тестов или реализации.

Документ определяет conformance evidence для согласованных
[потока взаимодействия](runtime-v2-semantic-core-flow.RU.md) и
[структуры компонентов](runtime-v2-semantic-core-structure.RU.md). Тесты проверяют
публичный identity contract Runtime v2, а не внутреннее разбиение helpers.

## 1. Уровни тестов и эталоны

Suite состоит из четырёх уровней:

1. white-box unit tests точных canonical bytes, parsing, validation и defensive
   copying;
2. public-package conformance tests только через exported API и JSON;
3. checked-in golden vectors, общие с реализациями не на Go;
4. module-boundary tests из local engine и независимого Go-модуля.

Golden expected values нельзя генерировать или переписывать тестируемым пакетом.
Fixture — conformance envelope с раздельными объектами `input` и `expected`;
вложенный `input.value` имеет точную публичную JSON shape. `expected` содержит
каждый canonical hash preimage в lowercase hex, полученные digest, все
intermediate states и endpoint.

Test-only envelope имеет точную форму:

```json
{
  "case": "G01",
  "comparison_group": "optional-name",
  "input": {"type": "recipe", "value": {}},
  "expected": {
    "canonical": {
      "factory": "optional-lowercase-hex",
      "transforms": [],
      "derived_states": []
    },
    "transform_fingerprints": [],
    "states": [],
    "endpoint": "sha256:..."
  }
}
```

`input.type` равен `recipe` или `relative`. `comparison_group` и
`expected.canonical.factory` пропускаются, когда неприменимы; `null` запрещён.
Arrays сохраняют порядок transforms/states. `{}` placeholder содержит полный
public value, не extension data. Verifier отклоняет unknown/duplicate members.

Начальные значения проверяются по нормативной грамматике и независимым
`scripts/maintenance/verify-runtime-v2-golden.mjs`. Команда:

```text
node scripts/maintenance/verify-runtime-v2-golden.mjs backend/libs/runtime-go/testdata/golden
```

Verifier читает только `input` при пересчёте bytes/hashes, затем сравнивает с
`expected`. Он не использует encoder code, generated constants или imports
expected values Go-модуля. Команда обязательна в PR и release gate. Изменение
fixtures требует явного schema/ADR review; auto-update flag отсутствует.

Порядок members JSON object не является контрактом. Round-trip tests сравнивают
проверенные semantic values и пересчитанные identities, а не сырые JSON bytes.

## 2. Golden vectors

Checked-in fixtures в `testdata/golden`:

| ID | Fixture | Обязательное evidence |
| --- | --- | --- |
| G01 | `factory-only.json` | Canonical bytes фабрики, root fingerprint/StateID, ноль steps, endpoint. |
| G02 | `one-step.json` | Factory/root, transform bytes/fingerprint, derived state и endpoint. |
| G03 | `multi-step.json` | Три разных transform, все intermediate states, ordered endpoint. |
| G04 | `relative.json` | External anchor, два новых steps, без factory/root record, endpoint. |
| G05 | `same-endpoint-recipe.json` | Recipe history для сравнения равных endpoints. |
| G06 | `same-endpoint-relative.json` | Relative history с теми же root anchor, steps и endpoint, что G05. |

Каждый fixture проверяется в текущем процессе, свежем helper subprocess и
независимым verifier. Для каждого factory, transform и derived-state hash в
`expected` есть точные canonical preimage bytes. Linux и Windows CI используют
одни checked-in fixtures.

## 3. Тесты canonical encoding

- **C01 — integer encoding:** граничные значения big-endian `u16`, `u32`, `u64`.
- **C02 — length framing:** framing пустых/непустых payload и различие
  `("ab", "c")` от `("a", "bc")`.
- **C03 — record fields:** проверка output encoder-а: numeric tags возрастают,
  field count точен, каждый required tag встречается один раз. Canonical-byte
  decoder в пакете не создаётся.
- **C04 — field-set ordering:** все перестановки трёх полей дают одинаковые bytes
  и fingerprint; повтор ASCII names запрещён.
- **C05 — exact strings:** UTF-8 bytes не нормализуются; NFC и NFD различаются.
- **C06 — digest representation:** lowercase `sha256:` декодируется ровно в 32
  raw hash-input bytes.
- **C07 — domain separation:** один payload в factory-state, transform и
  derived-state domains даёт три разных digest.
- **C08 — fixed domains:** точные v2 domain bytes каждой public operation;
  unsupported schema version отклоняется в V01, а не хешируется в выбранном
  caller-ом domain.

## 4. Чувствительность identity и независимость diagnostics

Table-driven tests меняют ровно один допустимый resolved identity input:

- **I01:** provider;
- **I02:** kind;
- **I03:** identity schema;
- **I04:** имя resolved field;
- **I05:** значение resolved field;
- **I06:** добавление поля;
- **I07:** удаление поля.

Каждое допустимое изменение меняет соответствующее factory state или transform
fingerprint. Изменение transform меняет его derived StateID и всех последующих
потомков; изменение factory меняет root и всех потомков.

- **D01:** изменение написания declaration reference.
- **D02:** изменение declaration arguments и их порядка.
- **D03:** изменение diagnostic attributes.
- **D04:** изменение implementation/build metadata resolver-а.

Каждая diagnostic mutation меняет provenance JSON, где представлена, но не
resolved identity, canonical bytes, fingerprints, StateIDs или endpoint.

## 5. Построение и целостность lineage

- **L01:** factory-only `Build` возвращает root, ноль steps и не создаёт transform.
- **L02:** `Build(A,B)` и `Build(B,A)` используют разные A/B и получают разные
  intermediate states и endpoints.
- **L03:** один transform с двумя разными parent StateID даёт разные derived ID.
- **L04:** parent каждого step равен StateID предыдущего root/step.
- **L05:** пустой `Extend` возвращает прежний anchor и ноль states/steps.
- **L06:** recipe и relative histories с одинаковым endpoint остаются разными
  Go types и JSON shapes.
- **L07:** decoded lineage пересчитывает fingerprints, parent links, StateIDs и
  endpoint без исходного `Recipe`.
- **L08:** state/identity JSON не содержит runtime ID, job ID, timestamp, physical
  path, checkpoint backend или materialization metadata.

- **T01:** изменение factory identity под существующим root.
- **T02:** изменение root ID или factory fingerprint.
- **T03:** изменение relative anchor.
- **T04:** изменение identity transform в step.
- **T05:** изменение fingerprint transform в step.
- **T06:** изменение parent ID derived state.
- **T07:** изменение resulting StateID.
- **T08:** перестановка lineage steps.

Validated decoding отклоняет каждую mutation с `integrity_mismatch` или более
точным structural code и точным field/index path.

## 6. JSON и digest parsing

- **J01:** каждый public semantic value проходит JSON round-trip без изменения
  identity и provenance.
- **J02:** unknown JSON members запрещены на root и nested уровнях.
- **J03:** duplicate JSON member names запрещены, а не last-value-wins.
- **J04:** trailing JSON values/tokens запрещены.
- **J05:** missing, `null`, empty и wrong-type required members запрещены.
- **J06:** регистр field names точный.
- **J07:** digest parser запрещает неверный/отсутствующий prefix, uppercase hex,
  non-hex, truncated и oversized values.
- **J08:** invalid UTF-8 запрещается до canonicalization.
- **J09:** JSON совпадает с нормативным allowlist каждой public shape; physical
  metadata и поля другого варианта State запрещены как unknown.

Identifier и version validation заданы явно:

- **V01:** любая schema version кроме `sqlrs.runtime.v2` отклоняется и не
  fingerprinted.
- **V02:** пустые identifiers запрещены.
- **V03:** uppercase, leading digit, leading dot/hyphen/underscore запрещены.
- **V04:** whitespace, slash, colon, NUL/control bytes и non-ASCII identifiers
  запрещены.
- **V05:** valid identifier максимальной byte length принимается.
- **V06:** identity с обязательным пустым массивом `fields` допустима.

## 7. Limits и ошибки

Boundary matrices покрывают `limit-1`, `limit`, `limit+1`:

- **B01:** byte length identifier;
- **B02:** byte length resolved value, включая multi-byte UTF-8;
- **B03:** число resolved fields;
- **B04:** число transforms recipe/relative;
- **B05:** размер top-level semantic JSON.
- **B06:** limits строк, arguments и attributes diagnostic
  declaration/observation.

Constructor и JSON path возвращают одинаковый stable validation code для одного
нарушения. Limits считают bytes, не runes.

- **E01:** каждый failure совпадает с `errors.Is(err, ErrInvalid)` и
  `errors.As(err, *ValidationError)`.
- **E02:** каждый failure имеет ожидаемые `Code` и field/index `Path` и не
  включает rejected values в error string.
- **E03:** функции, возвращающие value, при ошибке возвращают zero result.
- **E04:** failed `UnmarshalJSON` не меняет существующий non-zero receiver;
  успешное decoding атомарно заменяет его.

Тест JSON limit 4 MiB доказывает отказ после передачи bytes в `UnmarshalJSON`, но
не обещает предотвратить выделение input самим caller или `encoding/json`.

## 8. Immutability

- **A01:** mutation input slices полей и transforms после construction.
- **A02:** mutation declaration arguments и diagnostic maps.
- **A03:** mutation fields/steps, возвращённых accessors.
- **A04:** mutation provenance inputs после `Build` или `Extend`.
- **A05:** mutation исходного byte slice после JSON decoding.

Ранее построенные semantic values, canonical bytes, fingerprints, StateIDs,
endpoint и последующий JSON output не меняются. Accessors возвращают defensive
copies для slices, maps и byte arrays.

## 9. Process, platform и fuzz tests

- **P01:** helper subprocess пересчитывает все golden values в новом процессе.
- **P02:** Linux и Windows CI получают checked-in golden results.
- **F01:** fuzz JSON decoding без panic, excessive recursion и принятия value,
  чьи embedded fingerprints/StateIDs не проходят немедленный пересчёт.
- **F02:** fuzz digest parsing и canonical scalar encoding.
- **F03:** fuzz допустимых field permutations с invariant fingerprints.
- **F04:** fuzz lineage JSON mutation: полная validation либо structured rejection
  без partial result.

Fuzz corpus включает golden vectors и boundary cases. PR gate выполняет seed
corpus как обычные tests. Nightly CI запускает каждый fuzz target отдельной
командой на 30 секунд; release verification — на 5 минут. "Excessive recursion"
означает, что nested input до 4 MiB получает structured error или valid result без
panic/stack exhaustion. Fuzzing не заменяет golden/boundary/cross-process tests.

## 10. Dependency и import conformance

- **M01:** `go.mod` не содержит `require`. Проверка `go list -deps -json .`
  допускает только standard library и packages с prefix import path модуля.
- **M02:** external-package test в local engine импортирует
  `github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go` и создаёт минимальный
  factory-only lineage, доказывая отсутствие reverse/circular dependency.
- **M03:** `test/runtime-v2-consumer/go.mod` требует модуль `v0.0.0`, использует
  `replace github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go =>
  ../../backend/libs/runtime-go` и проходит `go test ./...` с `GOWORK=off`.
  В dependency graph нет local-engine import.
- **M04:** после публикации первого nested-module tag release verification
  повторяет M03 без `replace`. M04 — release gate, а не pre-publication PR gate
  issue #107.

## 11. Coverage и execution gates

Deterministic module tests запускаются как `go test ./... -count=1`. M02 идёт
через package test local engine; M03 — из consumer directory с `GOWORK=off`.
Independent Node verifier запускается отдельно. Race detection работает на Linux
amd64; Linux и Windows amd64 выполняют deterministic/golden tests. Linux arm64 —
release conformance target при наличии project runner.

Coverage измеряется для каждого package с line report: цель 100%, минимум 95%.
Недостаток coverage проходит repository approval loop до добавления тестов или
удаления dead code.

Issue #107 готова к implementation review, когда проходят G01-G06, C01-C08,
I01-I07, D01-D04, L01-L08, T01-T08, J01-J09, V01-V06, B01-B06, E01-E04,
A01-A05, P01-P02, seed corpus fuzz, M01-M03 и independent verifier. Timed fuzz
campaigns — nightly/release gates. M04 становится обязательным при публикации.
