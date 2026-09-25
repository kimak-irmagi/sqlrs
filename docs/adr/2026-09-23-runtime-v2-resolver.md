# Runtime v2 resolver decisions

Conversation timestamp: 2026-09-23 22:01 Asia/Novosibirsk (15:01 UTC).
GitHub user ID: @evilguest (41718235). Agent: Codex (GPT-5).
Approval timestamp: 2026-09-24 00:18 Asia/Novosibirsk (2026-09-23 17:18 UTC).
Status: accepted for issue #108.

## Decision 1: package and semantic-core boundary

Question: where should the resolver framework live, and should a resolved file
pretend to be a factory or transform?

Alternatives: local-engine internals; resolution I/O in package `runtimev2`; a
sibling package with neutral resource identity; an opaque caller-computed digest.

Decision: add `runtime-go/resolver`. It accepts the typed
`runtimev2.InputDeclaration` from #124 and returns a provider-qualified
`runtimev2.ResolvedExtensionIdentity`; only an explicit adapter composes that
extension into an existing Runtime v2 factory or transform identity.

Rationale: resolvers remain reusable, the semantic package stays free of I/O,
and a resource receives no false semantic role before its consumer is known.

## Decision 2: orchestration and observable revalidation

Question: should cache lookup, revalidation, and acquisition be one operation?

Alternatives: one eager resolve/download call; low-level methods only; separate
methods plus cache-aware `ResolveCurrent`.

Decision: keep normalization, resolution, revalidation, and acquisition
independent, and add a manager that performs cache lookup/re-resolution without
acquisition while preserving `CURRENT`, `STALE`, and `UNKNOWN` in its outcome.

Rationale: exact State-cache hits avoid artifact I/O and callers can distinguish
proven invalidity from inability to prove continuity.

## Decision 3: persistent cache format

Question: where should restart-safe resolution metadata live?

Alternatives: legacy SQLite/cache rows; new DB tables; a cache interface with a
versioned atomic directory implementation.

Decision: ship a strict bounded-JSON directory cache. Keys use a
normative binary encoding and include workspace-scope digest, normalized
declaration, cache schema, and resolver semantic version. Records include both
schema versions. Incompatible/corrupt records fail closed.

Rationale: #108 stays independently mergeable and dormant, cannot reinterpret
legacy records, and does not pre-empt the separate v2 persistence issue.

## Decision 4: deterministic symlink policy

Question: should workspace-file resolution follow, preserve, or reject links?

Alternatives: follow checked final paths; hash link targets; reject all
links/reparse points beneath a canonical workspace root.

Decision: resolve the workspace root once and open declarations beneath
Go 1.25 `os.Root`, then use pre/post checks to reject every symlink or Windows
reparse point below it; accept regular files.

Rationale: the rule is restart-stable, prevents workspace escape, and avoids
host-specific link targets in replay semantics.

## Decision 5: safe cheap file revalidation

Question: which metadata may establish that cached content is current?

Alternatives: size/mtime; always rehash; use strong platform identity/change
evidence and otherwise return `UNKNOWN` and rehash.

Decision: `CURRENT` requires matching strong evidence on an explicitly
recognized local filesystem class. Network, virtual, unknown, coarse-timestamp,
or incomplete filesystems always downgrade to `UNKNOWN`. Deletion/non-file/size
change is `STALE`; replacement, changed metadata, unavailable evidence, and
transient observation failure are `UNKNOWN`.

Rationale: unchanged files are cheap, preserved weak timestamps cannot reuse
stale identity, and metadata-only changes retain identity after rehashing.

## Decision 6: acquisition representation

Question: should a resolution contain a local artifact path?

Alternatives: path in identity; path in every cached resolution; provider-owned
closeable artifacts only from `Acquire`.

Decision: identity and cached logical resolution exclude acquisition
locations. The file provider never returns the mutable workspace handle;
`Acquire` streams it into a trusted content-addressed store, verifies the
expected digest, and returns the immutable published artifact.

Rationale: identity is location-independent and future OCI/Git/package providers
can expose physical artifacts without changing the generic identity contract.

## Decision 7: provider evidence validation

Question: how can a generic cache safely decode provider-specific evidence?

Alternatives: trust opaque `json.RawMessage`; standardize every provider's
evidence in the core; require the selected resolver to validate its resolution.

Decision: the generic cache validates and bounds the common envelope,
then the manager calls `Resolver.ValidateResolution` both after cache load and
after fresh resolution. Only validated evidence reaches revalidation/storage.

Rationale: providers keep ownership of their closed schemas without letting
unknown or malformed evidence bypass a trust boundary.

## Decision 8: cache and artifact trust boundary

Question: may a cached digest be reused from a caller-controlled directory?

Alternatives: trust any path; cryptographically authenticate every record with a
managed secret; restrict the reference store to trusted engine-owned directories
and defer multi-tenant authorization to #110.

Decision: directory cache/artifact stores must be outside the mutable
workspace, use restrictive permissions, reject link roots, validate envelope
checksums, and document that a party controlling the directory can forge data.
Shared/multi-tenant stores use #110 authorization rather than this adapter.

Rationale: checksums handle corruption, while an explicit ownership boundary
avoids pretending an unauthenticated local file is secure against its owner.

## Decision 9: mount, Windows path, and cancellation policy

Question: which path/filesystem edge cases must fail closed?

Alternatives: rely only on lexical cleaning; support all host path forms; use a
rooted path API plus explicit platform rejection/downgrade rules.

Decision: use `os.Root`, reject detected mount/filesystem transitions,
links/reparse points, special files, Windows ADS/device/ambiguous components, and
check context cancellation between hash chunks. Same-device bind-mount changes
require trusted mount-namespace control and are outside the attacker model.

Rationale: normal untrusted declarations cannot escape or select alternate data,
while the remaining non-portable host-administrator boundary is explicit.

## Decision 10: bounded stale-cache retention and fallback errors

Question: how are inert semantic-version entries and failed re-resolution
observed?

Alternatives: retain entries forever and return only the final error; implicit
pruning during reads; explicit bounded pruning plus prior-status errors.

Decision: `DirectoryCache` implements an explicit `PrunableCache`
policy by namespace/age/count. Read paths never prune. A failed fallback carries
the prior `STALE` or `UNKNOWN` status/reason in the structured error.

Rationale: disk use is bounded without adding latency/races to exact-hit reads,
and failure diagnostics retain the required revalidation distinction.

## Decision 11: cache fallback and visibility commit

Conversation refinement timestamp: 2026-09-24 00:59 Asia/Novosibirsk
(2026-09-23 17:59 UTC).

Question: which cache failures may fall back, and when is an atomic write visible?

Alternatives: treat every load failure as a miss; fail every invalid record;
fallback only for semantic invalidation and define replacement as the commit point.

Decision: miss, corrupt record, and incompatible version may fall back to fresh
resolution. Permission, cancellation, and general I/O failures abort without
overwrite. Atomic replacement is the visibility commit point; a following
directory-durability failure returns an error even though a complete new entry
may already be visible and accepted by retry/load.

Rationale: availability does not hide environmental failures, and injected
post-rename failures have a precise, testable outcome without partial records.

## Decision 12: evidence-gated filesystem enablement

Question: may a candidate filesystem class use cheap `CURRENT` after unit tests?

Alternatives: enable by OS/name; enable from fake capability tests; require native
live evidence for each class and evidence revision.

Decision: NTFS, APFS, ext4, XFS, Btrfs, and any future class remain `UNKNOWN`
until a mandatory native job proves replacement, rapid-change, restored-metadata,
and reopen/new-process behavior for the exact evidence revision. A checked-in
capability table binds enabled classes to those revisions; CI rejects unsupported
entries and unlisted classes default to `UNKNOWN`.

Rationale: mocked metadata cannot establish the real filesystem guarantees on
which skipping content hashing depends.

## Decision 13: cross-process and handle-generation proof

Question: are goroutine/race tests sufficient for cache and artifact atomicity?

Alternatives: goroutine tests only; probabilistic process stress; deterministic
publication barriers plus subprocess crash and replacement tests.

Decision: subprocess tests kill writers at each publication barrier and exercise
same-key readers/writers on Windows and Unix. CAS validation returns a handle to
the same opened object generation that was hashed; unchecked reopen is forbidden.

Rationale: the OS replacement and descriptor semantics are not covered by Go's
race detector, and path validation followed by reopen has a TOCTOU gap.

## Decision 14: concrete trusted-root permissions

Question: what does restrictive permissions mean on supported platforms?

Alternatives: best effort everywhere; platform-specific mandatory behavior;
cryptographic authentication of local records.

Decision: Unix-created roots/files use owner-only modes. Windows objects inherit
the ACL of a caller-established engine-owned root, which the package never
broadens, and reparse roots are rejected. Construction fails if the deployment
cannot establish the required trust boundary.

Rationale: this removes vacuous security-test skips while keeping authentication
ownership-based and within the standard-library module boundary.

## Decision 15: first enabled cheap-revalidation evidence revision

Conversation refinement timestamp: 2026-09-24 Asia/Novosibirsk.

GitHub user: `@evilguest`. Agent: OpenAI Codex (GPT-5).

Question: which native evidence can satisfy #108's cheap `CURRENT` acceptance
criterion without trusting only size and timestamps?

Alternatives: leave every filesystem at `UNKNOWN`; trust portable stat metadata;
enable a revisioned NTFS proof based on volume/file identity and per-file USN.

Decision: enable only `ntfs-usn` revision `ntfs-usn-v1`. Evidence is collected
from one opened handle, requires an actual NTFS volume, and combines volume/file
identity, the V2 per-file USN, size, and mtime. Native Windows tests require an
unchanged file to return `CURRENT` and a same-size overwrite with restored mtime
not to return `CURRENT`. Every other filesystem class remains `UNKNOWN`.

Rationale: NTFS exposes a native change token that closes the known portable-stat
false-negative, while a revisioned allowlist keeps unproved platforms fail-safe.

## Decision 16: role-complete resolver declaration contract

Conversation refinement timestamp: 2026-09-25 14:20 Asia/Novosibirsk
(2026-09-25 07:20 UTC).

GitHub user: `@evilguest`. Agent: OpenAI Codex (GPT-5).

Question: should the public resolver contract remain specific to input
declarations or accept every typed Runtime v2 extension-declaration role?

Alternatives: keep `InputDeclaration` and add parallel managers later; accept an
open structural interface; introduce a closed common interface implemented only
by the three Runtime v2 declaration types.

Decision: introduce a closed `ExtensionDeclaration` interface implemented by
`InputDeclaration`, `ExecutionEnvironmentDeclaration`, and
`DeploymentDeclaration`. Resolver normalization, normalized declarations, cache
keys, and manager dispatch use that interface while retaining the concrete role
in the resolver descriptor and cache key.

Rationale: the tagged module can accommodate input, OCI/environment, and
deployment resolvers without a later breaking interface change, while external
packages cannot forge an unsupported declaration implementation.

## Decision 17: atomic content-and-continuity snapshot

Conversation refinement timestamp: 2026-09-25 14:20 Asia/Novosibirsk
(2026-09-25 07:20 UTC).

GitHub user: `@evilguest`. Agent: OpenAI Codex (GPT-5).

Question: when may filesystem continuity evidence be paired with a content
digest?

Alternatives: collect evidence only after hashing; trust size/mtime around two
reads; collect native evidence before and after the stable double read.

Decision: collect native file identity/change evidence before hashing and again
after the confirming read. Strong evidence is published only when both samples
match exactly and the final stat metadata matches the hashed snapshot; a change
returns `ErrChanged` rather than caching a mixed generation.

Rationale: a write between digest confirmation and evidence collection must not
associate the old digest with the new generation's change token.

## Decision 18: native Unix cheap-revalidation revisions

Conversation refinement timestamp: 2026-09-25 14:20 Asia/Novosibirsk
(2026-09-25 07:20 UTC).

GitHub user: `@evilguest`. Agent: OpenAI Codex (GPT-5).

Question: which common Unix filesystems may satisfy #108's cheap unchanged-file
path?

Alternatives: keep every non-Windows filesystem `UNKNOWN`; trust portable
size/mtime; enable revisioned native proofs for recognized local filesystems.

Decision: enable revisioned evidence for Linux ext4, XFS, and Btrfs and macOS
APFS. Evidence combines filesystem class, device, inode, ctime, size, and mtime
from the same opened handle. Overlay, network, virtual, unknown, incomplete, and
unrecognized filesystems remain `UNKNOWN`. Each enabled platform/class must pass
a native same-size overwrite with restored-mtime test.

Rationale: inode and ctime close the known portable-stat false-negative on the
explicitly tested local filesystems without extending trust to unproved mounts.

## Decision 19: enforce the Windows trusted-root ACL boundary

Conversation refinement timestamp: 2026-09-25 14:20 Asia/Novosibirsk
(2026-09-25 07:20 UTC).

GitHub user: `@evilguest`. Agent: OpenAI Codex (GPT-5).

Question: how does store construction prove that a Windows cache/artifact root
is engine-owned rather than broadly writable?

Alternatives: rely on `os.Chmod`; document an unchecked caller precondition;
require an existing root and validate its owner and DACL with WinAPI.

Decision: Windows constructors require an existing non-reparse directory, verify
that its owner is the current process user, and reject DACLs granting write-like
access to broad identities such as Everyone, Authenticated Users, or Builtin
Users. Children inherit the validated ACL. Unix continues to create and enforce
owner-only roots.

Rationale: cache checksums are not authentication, and Windows `os.Chmod` does
not establish the ownership boundary on which cached identities rely.

## Decision 20: deterministic crash-barrier conformance

Conversation refinement timestamp: 2026-09-25 14:20 Asia/Novosibirsk
(2026-09-25 07:20 UTC).

GitHub user: `@evilguest`. Agent: OpenAI Codex (GPT-5).

Question: how should restart safety be demonstrated at publication boundaries?

Alternatives: normal concurrent subprocess completion; injected in-process
errors; subprocess writers paused and killed at named durability barriers.

Decision: cache and CAS publication expose package-private test barriers after
temporary creation/write, file sync, replacement, and directory sync. Subprocess
tests pause writers at each reachable barrier, terminate them, reopen the store,
and require either the complete old generation or complete new generation.

Rationale: deterministic process death validates OS visibility and durability
semantics that goroutine tests and injected return errors cannot establish.
