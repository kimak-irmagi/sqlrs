# Preserve regression assertions while updating approved fixtures

- Conversation date: 2026-11-07, Asia/Novosibirsk; exact reply timestamp unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Status: Accepted; limited to the three explicit approvals.

## Decision 1: physical macOS temporary roots

Question: how should tests reach input validation when macOS's system temporary
directory itself is an alias? Alternatives: weaken product link rejection, change
assertions, or resolve only the test root. Choose filepath.EvalSymlinks(t.TempDir())
in driverbundle/driver_test.go, inputbundle/bundle_test.go, inputbundle/verify_view_test.go
and only TestLiquibaseSecondPassSearchAndTrackerBranches in the client.
Deliberately created links and all assertions remain unchanged. This tests real
input-link rejection rather than incidental platform directory spelling.

## Decision 2: publication-pin dependency in the managed fixture

Question: how should the two retirement controls reach their intended boundaries?
Alternatives: bypass pin checks or supply the same dependency as real startup.
Choose only NewServiceWithPublicationPins(..., q) in managedExecutionFixture.
Keep all old success/refusal assertions; no product authorization changes.

## Decision 3: JDBC delivery in three WSL fixtures

Question: how should synthetic release executables satisfy the verified-driver
reader boundary? Alternatives: permit a missing driver, weaken installer assertions,
or supply verified-reader fixture bytes. Choose the latter only for
TestInstallWSLEngineWritesAtomically,
TestRunInitRepairsMissingDerivedWSLEngineWithoutUpdate and
TestRunInitRepairsExecutableButInvalidWSLEngine. Restore the reader after each
test. Separate missing/invalid-driver checks remain unchanged. This preserves the
old installation/repair requirements without pretending fixture bytes are a JAR.

## Review and consistency

The amendments change preparations, not expected behavior. Existing ADRs retain
link rejection, owned publication and checksum-verified JDBC delivery; no earlier
decision is superseded. Both full Windows module runs pass after these changes.
macOS/Linux CI checks also pass; overall PR readiness and coverage remain separate.
