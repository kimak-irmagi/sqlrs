# Minor CLI release for the replacement authentication API

- Conversation timestamp: 2026-10-29T14:30:28+07:00.
- GitHub user id: @evilguest (41718235).
- Agent name/version: Codex / GPT-6.
- Status: Accepted, proposed by the user.

## Question discussed

Which CLI release candidate should deliver installation-owned token exchange
and refresh after the incompatible authentication API replacement?

## Alternatives considered

1. Continue the v0.1.1 release-candidate sequence after v0.1.1-rc.10.
2. Increment the minor version and start a new release-candidate sequence.

## Chosen solution or decision made

Target **v0.2.0-rc.1** for the matched CLI release tracked by
[#168](https://github.com/kimak-irmagi/sqlrs/issues/168), including
[PR #169](https://github.com/kimak-irmagi/sqlrs/pull/169).
Release notes must identify the incompatible replacement of authentication
discovery and token operations, the matching-server requirement, and the lack
of legacy fallback. Publish through the existing release workflow after its
required gates pass. This record selects the version; it does not create a tag
or publish release artifacts.

The CLI release version, discovery schema revision 2, HTTP /v1 route prefix and
independently released runtime-go module retain their separate meanings.

## Brief rationale

The new CLI requires installation-owned code exchange and refresh instead of
performing those operations directly with the provider. A minor release clearly
marks this pre-1.0 compatibility boundary and resets the RC sequence.

## Contradiction check

This refines the matched-release requirement in the
[authentication ADR](2026-10-28-installation-token-exchange.md).
Earlier runtime-go version decisions apply to independently prefixed module tags
and are unaffected.
