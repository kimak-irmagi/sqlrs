# Acquire the identical released baseline through GitHub Releases API

- Conversation date: 2026-11-09, Asia/Novosibirsk; exact reply time unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Status: component clarification explicitly approved; separate tests await approval.
- Question: three diagnostic preparations fail on HTTP 504 from the browser
  download URL, before any measurements. Wait indefinitely, rebuild/substitute
  v1, patch the measured checkout, or acquire identical bytes via official API?
- Alternatives: those four choices. API asset `583092383` independently returned
  16,798,404 bytes with the original archive SHA256
  `da6132c95185f2a11561628458d34b22e7cb928c574d8ecec9f06d44fc8d0f01`.
- Decision: add the official API transport only after acquisition failures;
  keep checksum mismatch final. Use observer-owned preparation tooling with an
  explicit fixed clean source checkout at `668d505`, not observer product sources.
  Both original executable hashes and all archive/build/JDBC/release checks stay
  mandatory before measurement. Reuse the setup action; do not intercept fetch
  globally, patch the measured checkout, rebuild v1 or add a user URL/token.
- Rationale: fix preparation availability without changing measured products,
  versions, compiler, permissions, timing protocol or acceptance criteria. The
  three failed attempts remain non-measurements. No cause or optimisation of the
  product regression has been established by a download failure.

This narrowly extends the acquisition policy in
[the 2026-11-08 Actions ADR](2026-11-08-liquibase-performance-actions.md#acquisition-reliability-within-the-approved-pa02-boundary)
and the preparation-tool location in
[the localisation design](../architecture/issue89-performance-diagnostic-design.md).
Other venue, privacy, immutable-artifact and performance decisions remain in force.
