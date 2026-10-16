# Execution schema: corrections and repeat design review

Date: 2026-10-16. Scope: the proposed #150 API/structure, not implementation.
The reviewer is the same Codex agent, not an independent reviewer. No unit,
integration or coverage result is claimed. These scenarios are design checks,
not an approved implementation test inventory.

## Original findings and corrections

| Finding | Revised contract | Scenario checked by reading the design |
| --- | --- | --- |
| Missing typed input paths | ExecutionPath and path text parts; late native/forward-slash rendering | A changelog argument is built from a literal prefix and mount:project coordinate; Windows staging paths never enter identity. |
| Untyped input hash ignores file/tree semantics | Qualified source reference plus regular-file/tree.v1 profiles | A non-executable and executable wrapper differ in identity; a tree requires complete manifest/type/content/mode evidence, not a plain directory hash. |
| Nil secret version treated as harmless | Authentication/semantic purpose with closed allowed uses | A semantic seed cannot omit its version; an auth password is tied to one logical user/service and a known secure profile. |
| Host requires a complete dependency bundle | host-executable versus host-bundle resource extent | A local installed tool may execute without claiming full dependency capture; its result and descendants cannot be reusable States. |
| Compose defaulting incomplete | Resolve image/project rules before Build into effective argv/env | Image CMD is not accidentally appended after an explicit Compose entrypoint; removed and empty environment variables remain different. |
| Schema hard-codes provider limitations | Ordered nested mounts, explicit transport, separate network policy | Parent/child mount order is retained; a Unix-socket DB target under none can be represented, but unsupported physical isolation is not silently granted. |

## Second pass: additional edge cases found and addressed

1. A cached-State hit can bypass an execution-only gate. Qualification is now
   checked before any reusable-State selection, again before publication/replay.
2. A complete suffix after a local incomplete step could launder an unqualified
   parent into a reusable endpoint. Ineligibility now propagates through lineage
   and survives owner restart; only a verified complete baseline can reset it.
3. Tagged Go views could carry an ignored wrong-variant field. Both construction
   and parsing reject non-applicable fields; the mapping never silently drops
   them. The former draft's nullable command lifecycle is removed.
4. Image environment defaults could be copied as raw known secrets. They require
   the same classification as project values before constructing the effective
   map; no source is automatically a safe literal.
5. Exact canonical-size preflight could lead to copied framing logic. Raw input
   budgets precede allocation; bounded public node construction and existing
   NodeCount/EncodedSize enforce final aggregate limits without another encoder.
6. A local installed build could otherwise inherit unknown network use while
   appearing confined. The explicit local-context network mode is restricted to
   host-executable and cannot authorize complete/shared/reusable execution.

## Boundary checks and remaining implementation obligations

- The three operations still return/use the existing canonical identity. Seven
  fixed fields and new provider schema do not modify published schemas or cache
  wire records. The read-only tree API changes no bytes or hash domains.
- Build/Parse have no source, registry, process, filesystem or database I/O.
  Source references are not represented as proof; complete manifest verification
  and source schema support are mandatory at the source-owner boundary.
- Authentication profiles and OS-specific path/isolation behavior must be known
  and admitted by a provider. Unknown implementations fail before effects.
- Consumer reuse gates, lineage qualification and recovery must be implemented
  before a production profile is enabled. They are not claimed as code delivered
  by this pure library issue. Code/tests must verify the module contract after
  the API and then test inventory are approved.

Result of this repeat design review: no further blocking finding identified
within the revised, explicitly stated boundary. This is not feature acceptance,
proof of a running provider or an assertion that design review eliminates risk.
Subsequently, @evilguest approved the revised API/structure on 2026-10-16.
The [test inventory](runtime-v2-execution-schema-tests.md) was then separately
approved on 2026-10-16. Tests precede implementation; this is no test result.
