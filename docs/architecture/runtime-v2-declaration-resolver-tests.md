# Runtime v2 declarations, resolver, and module-release test plan

Status: approved by @evilguest for issues #108, #123, and #124, 2026-09-24.

This plan verifies the approved declaration, resolver, cache, acquisition, and
release contracts. Test IDs are stable requirement references. Implementation
starts only after this list is approved and existing tests are reviewed for
contradictions.

## 1. Versioned declarations and extension identity

- **D01 — document round trips:** valid factory, transform, recipe, input,
  execution-environment, and deployment documents round-trip semantically;
  exact byte goldens apply only where the wire contract specifies canonical
  member order, while canonical field-array ordering is always byte-stable.
- **D02 — strict decoding:** missing/wrong schema versions, unknown or duplicate
  members at every nesting level, nulls, invalid UTF-8, trailing tokens, and
  exceeded bounds fail without mutating the receiver. Every count, value, total
  size, and nesting limit is tested at `limit-1`, `limit`, and `limit+1`.
- **D03 — recipe forms:** factory-only, one-transform, and ordered multi-transform
  declarations remain distinct from compiled `Recipe` values.
- **D04 — role safety:** role-specific constructors/types prevent implicit
  cross-role use; compile-time examples prove intended assignments.
- **D05 — opaque extension declarations:** file, OCI-style input, execution, and
  deployment examples preserve only their role's validated payload.
- **D06 — extension canonicalization:** field permutations encode identically;
  duplicate names, invalid names/values, and caller mutation are rejected or
  isolated by defensive copies.
- **D07 — resolved extension transport:** strict round trips, bounds, required
  qualification, and immutable accessors are covered.
- **D08 — extension fingerprint:** an independent verifier and golden vectors
  cover the domain tag, schema, owner, kind, identity schema, and every canonical
  field; changing each identity input changes the fingerprint.
- **D09 — diagnostic exclusion:** capability, portability, and resolver
  observations do not change extension or state identity.
- **D10 — explicit composition:** every supplied binding becomes a deterministic
  reserved `extension.<binding>` field; duplicate bindings, reserved-prefix base
  collisions, and invalid bindings fail; input slices remain unmodified.
  The module ships a reusable adapter conformance suite; no test claims that
  nonexistent or external adapters use the helper until they run that suite.
- **D11 — downstream identity:** composed extension changes propagate through
  factory/transform fingerprints and StateIDs, while diagnostic-only changes do
  not.
- **D12 — #107 compatibility:** all existing semantic-core public types, JSON,
  independent verifier output, and golden vectors remain byte-for-byte unchanged;
  a compile-only external consumer exercises the complete promised #107 API.

## 2. Resolver registry and manager

- **R01 — full-tuple dispatch:** role, owner, kind, and specification schema all
  participate in selection; partial matches and unsupported declarations fail.
- **R02 — registry integrity:** duplicate registrations fail, construction takes
  defensive copies, and concurrent read-only dispatch is race-free.
- **R03 — phase separation:** normalize, resolve, validate-resolution,
  revalidate, and acquire are independently invoked and observable.
- **R04 — validation boundary:** provider evidence is validated after both fresh
  resolution and cache load; malformed or unknown evidence is never trusted.
- **R05 — current fast path:** `CURRENT` returns the cached identity without
  calling resolve or acquire, persists refreshed freshness before reporting
  restart-safe success, and exposes store failure instead of hiding it.
- **R06 — stale/unknown behavior:** `STALE` and `UNKNOWN` remain distinct,
  observable outcomes and both force resolution when current identity is needed.
- **R07 — failure context:** a failed re-resolution preserves the preceding
  stale/unknown reason in a structured error.
- **R08 — cancellation and limits:** cancellation reaches normalization,
  hashing/resolution, revalidation, and acquisition; all bounded inputs and
  evidence enforce their limits.
- **R09 — orchestration matrix:** table-driven fakes cover cache miss, corrupt,
  incompatible, permission, cancellation, and I/O outcomes crossed with each
  revalidation status, validation failure, resolve failure, and store failure.
  Every row asserts calls made/not made, outcome, stable error code, prior status,
  cache contents, and absence of partial values. Only miss/corrupt/incompatible
  may fall back; permission/cancellation/general I/O abort without overwrite.
- **R10 — error and cleanup contract:** every operation asserts stable operation/
  code/descriptor fields, `errors.Is` for context and wrapped system errors, no
  source-content leakage, and closure/removal of handles and temporary files on
  every injected failure path.

## 3. Persistent resolution cache

- **C01 — normative key golden:** an independent verifier covers workspace-root
  digest, role, owner, kind, specification schema, resolver semantic version,
  and canonical declaration. Aliases resolving to the same physical root share
  a scope digest; distinct physical roots do not. Raw absolute paths never appear
  in persisted bytes. JSON/map order does not influence the key.
- **C02 — version coexistence:** different semantic resolver versions use
  different keys and may coexist without overwrite.
- **C03 — strict envelope:** schema, key, workspace scope, declaration,
  resolution, evidence, provenance, bounds, checksum, unknown fields, duplicate
  fields, truncation, and trailing bytes are validated atomically.
- **C04 — checksum scope:** the checksum covers the normative envelope excluding
  only the checksum member; controlled corruption is detected.
- **C05 — restart persistence:** a new process/cache instance reads a valid entry
  and can take the provider-approved fast path.
- **C06 — atomic publication and commit point:** fault injection before and after
  temp write, file sync, replacement, and directory durability proves that
  replacement is the visibility commit. Pre-commit failure leaves the old entry;
  post-commit durability failure may return an error with a complete new entry
  visible, which retry/load accepts. Partial entries are never hits and orphan
  temporary files are harmless across restart.
- **C07 — concurrency:** goroutine and subprocess tests cover same-key and
  different-key readers/writers, a process killed at every publication barrier,
  and Windows/Unix replacement behavior; readers yield only complete old or new
  entries. `go test -race` supplements but does not replace subprocess evidence.
- **C08 — trusted root:** symlink/reparse cache roots or entries are rejected.
  Unix roots/files have owner-only modes. Windows objects inherit an engine-owned
  root ACL without broadening it. Store construction fails when the deployment
  cannot establish the required trusted root; security assertions are not skipped.
- **C09 — explicit pruning:** namespace, age, and count policies remove only
  eligible complete entries, ignore active temporaries safely, reject links, and
  never run implicitly on reads. Subprocess tests run prune concurrently with an
  active writer; stale-temp cleanup uses an age/ownership rule and cannot remove
  a live writer's temporary generation.
- **C10 — adversary boundary demonstration:** controlled corruption without a
  new checksum fails, while a fixture acting as the directory owner can rewrite
  both record and checksum. The latter documents the ownership assumption and is
  not reported as cryptographic tamper resistance.

## 4. Workspace-file provider

- **F01 — portable normalization:** empty, absolute, parent-traversal, separator,
  volume-qualified, ADS, reserved-device, and trailing-dot/space cases follow the
  documented Windows/POSIX rules; accepted case is preserved.
- **F02 — declaration schema:** exactly one `path` field is required under the
  approved owner/kind/specification tuple; aliases and extras fail.
- **F03 — containment:** symlinks/reparse points in every component and at the
  leaf, non-regular files, and detected mount/filesystem transitions are rejected.
- **F04 — namespace races:** deterministic barriers inject component/leaf
  rename, link replacement, and detected mount transition before open, after
  inspection, and during hashing. Each case either hashes bytes from the safely
  rooted opened descriptor or returns a safe error; probabilistic stress alone
  is insufficient.
- **F05 — content identity:** exact bytes produce the required lowercase SHA-256
  identity; path, case spelling, timestamps, and file IDs never enter identity.
- **F06 — mutation during hash:** size/content replacement while hashing retries
  safely or returns a structured non-current error, never a mixed identity.
- **F07 — cancellable hash:** long reads stop promptly on context cancellation.
- **F08 — strong current evidence:** candidate NTFS, APFS, ext4, XFS, and Btrfs
  classes may return `CURRENT` without reading content only after the exact class
  and evidence revision passes its mandatory native live gate. Otherwise the
  production classifier returns `UNKNOWN` regardless of fake-probe unit coverage.
- **F09 — conservative evidence:** network, virtual, coarse-metadata, unknown,
  and unproven OverlayFS classes return `UNKNOWN` and rehash.
- **F10 — changes:** deletion/type/size changes are stale; replacement/file-ID or
  ambiguous metadata changes are unknown; same-size/same-mtime content changes
  are never accepted as current.
- **F11 — same identity after rehash:** metadata-only changes may produce
  `UNKNOWN` followed by the original content identity.
- **F12 — duplicate spelling:** accepted case variants may create separate cache
  entries but resolve to equal content identity.

## 5. Immutable artifact acquisition

- **A01 — no workspace handle:** acquisition returns only an immutable artifact,
  never a mutable source handle or workspace path as identity.
- **A02 — verified stream:** the copied stream is hashed and must match the
  expected resolved digest; concurrent source mutation or mismatch fails.
- **A03 — atomic CAS publication:** fsync and atomic publish expose no partial
  object after injected write, sync, rename, or cancellation failures; a valid
  existing digest object is never overwritten and published objects are not
  writable through returned handles.
- **A04 — existing object validation:** an existing content-addressed object is
  rehashed before use; deterministic replacement between validation and return
  proves the returned handle names the same validated generation, never an
  unchecked reopen. Corruption is repaired only from a newly verified stream.
- **A05 — deduplication/concurrency:** goroutine and subprocess acquisition of
  equal digests deduplicates, including killed writers at publication barriers;
  handles close correctly and readers see immutable complete bytes.
- **A06 — trusted artifact root:** linked roots/objects are rejected and the same
  mandatory Unix-mode/Windows-inherited-ACL contract as C08 is verified.

## 6. Cross-platform, fuzz, race, and release gates

- **X01 — platform units:** fake capability probes deterministically cover every
  supported and unsupported filesystem class on all CI hosts.
- **X02 — live filesystem enablement:** native Windows/NTFS, macOS/APFS, and Linux
  ext4/XFS/Btrfs jobs test overwrite with restored size/mtime, rapid changes near
  timestamp granularity, atomic replacement, reopen/new-process behavior, and
  observable IDs/tokens. A missing live job keeps that production class disabled
  (`UNKNOWN`); it is not an accepted skip. OverlayFS follows the same rule.
- **X03 — capability manifest:** a checked-in table maps each production-enabled
  filesystem class to its evidence revision and required native job. CI fails for
  an enabled class without matching evidence; unlisted classes are `UNKNOWN`.
- **X04 — fuzzing:** declaration/cache JSON, normalized paths, provider evidence,
  canonical fields, and composition fuzz targets never panic, accept trailing
  data, or mutate a receiver after failure.
- **X05 — race suite:** registry, cache, prune, and artifact acquisition tests pass
  `go test -race` on supported CI platforms.
- **X06 — dependency boundary:** the nested module remains standard-library-only
  unless a separately approved dependency is introduced.
- **P01 — retraction:** `go.mod` retracts immutable `v0.1.0` with the approved
  reason. PR tests validate the directive locally; after stable `v0.2.0` exists,
  a proxy-backed `go list -m -u` from a `v0.1.0` consumer must expose the warning.
- **P02 — trigger isolation:** product `v*` tags cannot trigger the nested-module
  publisher, and `backend/libs/runtime-go/v*` cannot trigger product binaries.
- **P03 — PR/preflight gate:** manual version/commit input verifies clean tree, module
  path, tests, conformance/goldens, race, fuzz smoke, coverage, dependencies, and
  a standalone `GOWORK=off` consumer before tagging.
- **P04 — tag verification:** tag mode verifies prefix, exact commit, module path,
  and required release-note schema/source metadata.
- **P05 — RC public gate:** after the immutable RC tag exists, a clean consumer
  with no `replace`, fresh module cache, and `GOWORK=off` resolves it through the
  explicitly configured public proxy and checksum database with bounded retries,
  verifies the selected version, source commit metadata, module zip, and sums.
- **P06 — GA provenance and closure gate:** GA creation verifies that
  `v0.2.0-rc.1` and `v0.2.0` point to the same tested commit. A second clean
  public-consumer check verifies GA; only that post-tag result closes issue #123.
  Neither RC nor GA public checks are represented as pre-merge PR requirements.
- **P07 — tag-policy and workflow authority:** a repository-policy check proves
  that the nested tag namespace rejects update/deletion and that release jobs use
  least-privilege permissions. Failure blocks RC/GA and is reported as an
  external policy failure, not misrepresented as a code-test success. Repository
  administrators remain an explicit governance boundary; public proxy/checksum
  proofs independently detect content changes after observation.

## 7. Coverage and acceptance

Package coverage is measured per platform with per-line reports and an explicit
merged report for common code; platform-only files must meet their native job's
threshold and are not excused as unreachable elsewhere. The target is 100%; 95%
is the minimum permitted by project policy. Any shortfall follows the separately
approved remediation process.

Acceptance is staged: PR acceptance requires unit, golden, independent
conformance, fuzz-smoke, race, required native-filesystem enablement for every
class left enabled, local clean-consumer, and workflow-contract tests. RC
acceptance requires its public-consumer gate. GA/issue-#123 closure requires the
same-commit proof and final GA public-consumer gate.

## 8. Proposed coverage-remediation iteration

Measured on Windows after the four buildable implementation commits: core 83.0%,
resolver 70.1%. The approved requirements are present, but their negative and
failure branches are not yet sufficiently exercised. No undocumented production
behavior is proposed. Address files in uncovered-statement order:

1. `extension_identity.go` (54), `diagnostics.go` (47), and
   `extension_declaration.go` (41): strict round trips for every role, zero/nil
   values, atomic failed decode, all accessors, duplicate/boundary inputs, and
   composition mutation isolation.
2. `workspace_file.go` (36), `directory_cache.go` (34), and `framework.go` (24):
   complete the approved path/type/cancellation matrix, cache corruption and
   size/version/key failures, and every manager orchestration row.
3. `artifact_store.go` (23), `cache_prune.go` (14), and `errors.go` (10): inject
   read/write/cancel/digest/existing-object failures, age/version/link pruning,
   cleanup, stable codes, and `errors.Is`/`errors.As` behavior.
4. Declaration documents/recipe and existing semantic files (65 combined): test
   the documented defensive-copy, nil receiver, strict decoder, integrity, and
   legacy compatibility branches. Remove a branch only if line review proves it
   has no corresponding requirement.
5. Re-measure per platform. Target 100%; acceptance requires at least 95% for
   core and resolver separately. Windows replacement error paths and native-only
   Unix paths are measured in their native jobs and merged, not fabricated.

## 9. Coverage and critical-review outcome

The approved remediation reached 95.5% for the core package. Resolver coverage
is measured separately on every supported platform with a 95% floor. The review
also requires deterministic killed-writer publication barriers, closed
role-complete declaration dispatch, atomic digest/evidence snapshots, native
Windows ACL validation, native Unix continuity gates, resolver fuzz targets,
and per-package CI/release
thresholds, bounded cache reads/writes, strict canonical evidence, closed
revalidation statuses, validated refreshed `CURRENT` evidence, NTFS reparse
handling, cross-filesystem detection where native device IDs exist, and
same-size/restored-mtime continuity tests. Enabled revisions are `ntfs-usn-v1`,
the approved Linux ext4/XFS/Btrfs revisions, and the approved macOS APFS
revision; all unlisted classes remain `UNKNOWN`.

The repository ruleset prohibiting nested-tag update/deletion is active. It
remains an external publication prerequisite: the release job checks the live
GitHub policy on every run and fails closed if that protection changes.
