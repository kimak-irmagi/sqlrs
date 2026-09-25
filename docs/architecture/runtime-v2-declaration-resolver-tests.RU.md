# План тестов declarations, resolver и module release Runtime v2

Статус: согласовано @evilguest для issues #108, #123 и #124, 2026-09-24.

План проверяет согласованные contracts declarations, resolver, cache, acquisition
и release. ID тестов служат стабильными ссылками на требования. Реализация
начинается только после согласования списка и проверки существующих тестов на
противоречия.

## 1. Versioned declarations и extension identity

- **D01 — round trip документов:** валидные factory, transform, recipe, input,
  execution-environment и deployment документы проходят semantic round trip;
  exact-byte goldens применяются только там, где wire contract задаёт member
  order, а canonical field-array order всегда byte-stable.
- **D02 — строгий decoder:** отсутствующие/неверные schema versions, неизвестные
  или повторные members на каждом уровне, null, invalid UTF-8, trailing tokens и
  превышение bounds дают ошибку без изменения receiver. Count/value/total-size/
  nesting limits проверяются в точках `limit-1`, `limit`, `limit+1`.
- **D03 — формы recipe:** factory-only, один и упорядоченные несколько transforms
  отделены от скомпилированного `Recipe`.
- **D04 — безопасность ролей:** role-specific constructors/types не допускают
  неявное смешение ролей; compile-time examples фиксируют допустимые назначения.
- **D05 — opaque declarations:** примеры file, OCI-style input, execution и
  deployment сохраняют только проверенный payload своей роли.
- **D06 — canonicalization extensions:** перестановки полей равнозначны;
  дубликаты/невалидные значения отклоняются, caller mutation изолирована.
- **D07 — transport resolved extension:** проверяются strict round trip, bounds,
  обязательная квалификация и immutable accessors.
- **D08 — fingerprint extension:** независимый verifier и golden vectors покрывают
  domain tag, schema, owner, kind, identity schema и каждое поле; изменение любого
  identity input меняет fingerprint.
- **D09 — исключение diagnostics:** capability, portability и resolver observations
  не меняют extension/state identity.
- **D10 — явная composition:** каждый binding становится детерминированным
  `extension.<binding>` field; дубликаты, collisions и невалидные bindings дают
  ошибку; входные slices не меняются. Module предоставляет reusable adapter
  conformance suite; до её запуска не утверждается, что внешний adapter применил helper.
- **D11 — downstream identity:** composed extension меняет factory/transform
  fingerprints и StateIDs, diagnostic-only изменения — нет.
- **D12 — совместимость #107:** public types, JSON, independent verifier и golden
  vectors semantic core остаются byte-for-byte прежними; compile-only внешний
  consumer использует весь обещанный public API #107.

## 2. Registry и manager resolver-а

- **R01:** dispatch использует полный tuple role/owner/kind/specification schema;
  partial match и unsupported declaration дают ошибку.
- **R02:** duplicate registration отклоняется, registry копирует входы и безопасен
  для конкурентного read-only dispatch.
- **R03:** normalize, resolve, validate-resolution, revalidate и acquire вызываются
  раздельно и наблюдаемы.
- **R04:** provider evidence валидируется после fresh resolve и cache load.
- **R05:** `CURRENT` возвращает cached identity без resolve/acquire, сохраняет
  refreshed freshness до restart-safe success и не скрывает store failure.
- **R06:** `STALE` и `UNKNOWN` различимы и оба запускают resolve при запросе current.
- **R07:** ошибка повторного resolve сохраняет исходную stale/unknown причину.
- **R08:** cancellation достигает всех фаз; inputs/evidence соблюдают bounds.
- **R09:** table-driven orchestration matrix пересекает cache miss/corrupt/
  incompatible/permission/cancellation/I/O с каждым revalidation status,
  validation/resolve/store failure. Каждая строка проверяет calls, outcome,
  stable error code, prior status, cache contents и отсутствие partial values.
  Только miss/corrupt/incompatible допускают fallback; остальные ошибки abort.
- **R10:** каждая операция проверяет stable operation/code/descriptor,
  `errors.Is` для context/wrapped system errors, отсутствие утечки source content
  и закрытие/removal handles и temporary files на всех injected failure paths.

## 3. Persistent resolution cache

- **C01:** independent key golden покрывает workspace digest, role, owner, kind,
  spec schema, semantic resolver version и canonical declaration. Aliases одного
  physical root имеют общий scope digest, разные roots — разные. Raw absolute
  paths не сохраняются; JSON/map order не влияет на key.
- **C02:** разные semantic versions имеют разные сосуществующие keys.
- **C03:** envelope строго проверяет schema/key/scope/declaration/resolution,
  evidence/provenance/bounds/checksum, unknown/duplicate fields и trailing data.
- **C04:** checksum покрывает normative envelope кроме самого checksum и выявляет
  контролируемое повреждение.
- **C05:** новый process/cache instance читает entry и использует разрешённый fast path.
- **C06:** fault injection до/после temp write, file sync, replacement и directory
  durability фиксирует replacement как visibility commit. До commit остаётся old
  entry; после commit durability error может сосуществовать с полной видимой new
  entry, принимаемой retry/load. Partial entry не бывает hit; orphan temp безопасен.
- **C07:** goroutine и subprocess tests покрывают readers/writers одного/разных
  keys, kill process на каждом publication barrier и Windows/Unix replacement;
  видны только complete old/new entries. Race detector не заменяет subprocess.
- **C08:** symlink/reparse roots/entries отклоняются. Unix roots/files имеют
  owner-only modes; Windows objects наследуют engine-owned root ACL без его
  расширения. Невозможность создать trusted root приводит к construction error.
- **C09:** явный prune по namespace/age/count не трогает неeligible/active entries,
  отклоняет links и никогда не запускается при read. Subprocess test запускает
  prune с active writer; age/ownership rule не удаляет live temporary generation.
- **C10:** corruption без нового checksum отклоняется; fixture с правами владельца
  переписывает и record, и checksum, демонстрируя ownership assumption, а не
  cryptographic tamper resistance.

## 4. Workspace-file provider

- **F01:** empty/absolute/parent paths, separators, volumes, ADS, device names и
  trailing dot/space следуют Windows/POSIX rules; принятый case сохраняется.
- **F02:** tuple owner/kind/spec принимает ровно один `path`, extras отклоняются.
- **F03:** symlink/reparse на каждом component/leaf, non-regular files и обнаруженные
  mount/filesystem transitions отклоняются.
- **F04:** deterministic barriers внедряют component/leaf rename, link replacement
  и detected mount transition до open, после inspection и во время hash. Результат
  — bytes безопасного rooted descriptor либо safe error; одного stress недостаточно.
- **F05:** identity — lowercase SHA-256 точных bytes; path/case/time/file ID не входят.
- **F06:** mutation во время hash даёт безопасный retry/error, но не mixed identity.
- **F07:** долгий hash быстро прекращается по context cancellation.
- **F08:** candidate NTFS/APFS/ext4/XFS/Btrfs дают `CURRENT` без чтения bytes только
  после mandatory native live gate для точного class/evidence revision. Иначе
  production classifier возвращает `UNKNOWN` независимо от fake units.
- **F09:** network/virtual/coarse/unknown и неподтверждённый OverlayFS дают
  `UNKNOWN` и rehash.
- **F10:** deletion/type/size дают stale; replacement/file-ID или ambiguous metadata
  дают unknown; same-size/same-mtime content change не считается current.
- **F11:** metadata-only change может дать `UNKNOWN`, затем прежнюю identity.
- **F12:** допустимые case variants могут иметь разные cache entries, но равную identity.

## 5. Immutable artifact acquisition

- **A01:** acquire возвращает только immutable artifact, не workspace handle/path.
- **A02:** stream hash обязан совпасть с expected digest; mutation/mismatch дают error.
- **A03:** fsync/atomic publish не оставляют partial object; valid existing digest
  не перезаписывается, published object не writable через returned handle.
- **A04:** существующий CAS object повторно хешируется; deterministic replacement
  между validation и return доказывает, что handle относится к той же verified
  generation, а не unchecked reopen. Repair использует только новый verified stream.
- **A05:** goroutine и subprocess acquisition одинакового digest дедуплицируется,
  включая killed writers на barriers; handles закрываются, bytes целы и immutable.
- **A06:** linked roots/objects отклоняются; проверяется тот же обязательный
  Unix-mode/Windows-inherited-ACL contract, что в C08.

## 6. Cross-platform, fuzz, race и release gates

- **X01:** fake capability probes покрывают все filesystem classes на каждом CI host.
- **X02:** native NTFS/APFS/ext4/XFS/Btrfs jobs проверяют overwrite с восстановлением
  size/mtime, rapid changes около timestamp granularity, atomic replacement,
  reopen/new process и native tokens. Отсутствующий live job оставляет production
  class disabled (`UNKNOWN`), а не accepted skip; для OverlayFS правило то же.
- **X03:** checked-in capability table связывает production-enabled FS class с
  evidence revision/native job. CI отклоняет entry без evidence; unlisted — `UNKNOWN`.
- **X04:** fuzz declaration/cache JSON, paths, evidence, fields и composition не
  паникует, не принимает trailing data и не мутирует receiver при ошибке.
- **X05:** registry/cache/prune/acquire проходят `go test -race`.
- **X06:** nested module остаётся standard-library-only без отдельного согласования.
- **P01:** `go.mod` retract-ит immutable `v0.1.0`; PR локально проверяет directive,
  а после stable `v0.2.0` proxy-backed `go list -m -u` из consumer на `v0.1.0`
  обязан показать retraction warning.
- **P02:** product `v*` и nested `backend/libs/runtime-go/v*` workflows не запускают
  друг друга.
- **P03 — PR/preflight:** manual preflight проверяет commit/version, clean tree, module path, unit,
  conformance/golden, race, fuzz smoke, coverage, dependencies и standalone
  `GOWORK=off` consumer.
- **P04:** tag mode проверяет prefix, exact commit, module path и release-note metadata.
- **P05 — RC public gate:** после immutable RC tag clean consumer без `replace`,
  с fresh cache и `GOWORK=off` получает RC через явно заданные public proxy и
  checksum database, проверяя version, source metadata, module zip и sums.
- **P06 — GA/closure:** GA creation доказывает, что RC и `v0.2.0` указывают на один
  tested commit; затем второй clean public consumer проверяет GA и закрывает #123.
  RC/GA public checks не выдаются за pre-merge PR requirements.
- **P07:** repository-policy check доказывает запрет update/delete nested tags и
  least-privilege release jobs; failure блокирует RC/GA как external policy error.
  Repository admins остаются governance boundary; proxy/checksum proofs отдельно
  выявляют изменение уже наблюдавшегося content.

## 7. Coverage и приёмка

Per-line coverage измеряется отдельно на каждой платформе и объединяется для
common code; platform-only files обязаны пройти threshold в native job и не
исключаются как unreachable. Target 100%, допустимый минимум 95%; недостаток
исправляется по отдельно согласованному плану.

Acceptance staged: PR требует unit, golden, independent conformance, fuzz-smoke,
race, native-FS gate для каждого оставленного enabled class, local clean consumer
и workflow-contract tests. RC требует public-consumer gate. GA/закрытие #123
требует same-commit proof и финальный GA public-consumer gate.

## 8. Предлагаемая итерация исправления coverage

После четырёх buildable implementation commits на Windows измерено: core 83,0%,
resolver 70,1%. Требования реализованы, но negative/failure branches покрыты
недостаточно. Новое undocumented behavior не добавляется. Порядок по числу
uncovered statements:

1. `extension_identity.go` (54), `diagnostics.go` (47),
   `extension_declaration.go` (41): strict round trips всех ролей, zero/nil,
   atomic failed decode, accessors, duplicate/boundary inputs и mutation isolation.
2. `workspace_file.go` (36), `directory_cache.go` (34), `framework.go` (24):
   полный path/type/cancellation matrix, corruption/size/version/key cache и все
   строки manager orchestration matrix.
3. `artifact_store.go` (23), `cache_prune.go` (14), `errors.go` (10): injected
   read/write/cancel/digest/existing-object failures, age/version/link pruning,
   cleanup, stable codes и `errors.Is`/`errors.As`.
4. Declaration documents/recipe и прежние semantic files (65 вместе):
   документированные defensive-copy, nil receiver, strict decoder, integrity и
   legacy compatibility branches. Ветка удаляется только при отсутствии требования.
5. Повторное per-platform измерение. Target 100%; минимум 95% отдельно для core
   и resolver. Windows replacement и native Unix paths объединяются из native
   jobs, а не имитируются.

## 9. Результат coverage и критического ревью

Согласованная remediation достигла 95,5% для package core. Coverage resolver
измеряется отдельно на каждой supported platform с floor 95%. Ревью также
требует deterministic killed-writer publication barriers, закрытый
role-complete declaration dispatch, atomic digest/evidence snapshots, native
Windows ACL validation, native Unix continuity gates, resolver fuzz targets и отдельные CI/release
thresholds по package, bounded cache reads/writes, strict canonical evidence,
закрытый набор revalidation statuses, валидацию refreshed evidence для `CURRENT`,
обработку NTFS reparse points, cross-filesystem detection там, где доступны
native device IDs и tests для same-size overwrite с восстановленным mtime.
Включённые revisions: `ntfs-usn-v1`, утверждённые Linux revisions для
ext4/XFS/Btrfs и утверждённая macOS APFS revision; все unlisted classes остаются
`UNKNOWN`.

Repository ruleset, запрещающий update/delete nested tags, активен. Он остаётся
внешней предпосылкой публикации: release job проверяет live GitHub policy при
каждом запуске и fail-closed завершается, если protection изменится.
