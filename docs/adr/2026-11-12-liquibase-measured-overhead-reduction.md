# Proceed with the measured #89 overhead candidates

- Conversation date: 2026-11-12, Asia/Novosibirsk; exact reply time unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Question: leave the measured v2 overhead in place, or implement the identified
  reductions, adjust tests coupled to the removed actions, and repeat timing?
- Alternatives: keep diagnosing without implementation; implement all suspected
  changes without preserving ownership/provenance; or design bounded reductions,
  retain safety assertions and repeat the same controlled measurements.
- Decision: the user authorised proceeding with the reductions, correcting
  coupled tests where needed and remeasuring. Detailed components, O5 storage
  and the final test inventory still follow their distinct AGENTS.md approvals.
- Rationale: completed CT13 43061874236 confirms ordinary v2 is still +14.99%
  fresh / +18.16% cached versus actual v1, with concrete source costs.

## Retain the exact planning DB for the first working session

- Conversation date: 2026-11-12, Asia/Novosibirsk; exact proposal time unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Question: how to remove the measured 2628 ms second DB startup without fake
  absence evidence or a crash gap in ownership?
- Alternatives: retain destruction/recreation; reuse only an in-memory pointer;
  or explicitly save a retained-plan owner and atomically transfer the exact DB
  to the first source-compatible working session.
- Decision: O5 approved at component level by “Утвердить проект компонентов”.
  Storage and exact guards require separate approval. This extends the scope
  previously excluded by the working-instance handoff; it does not rewrite its
  historical evidence or approve DDL/implementation.
- Rationale: preserve exact runtime/path/operation Source and allow only a true
  first application, without applying an incompatible cached prefix again.

## Use a bounded per-service inventory cache

- Conversation date: 2026-11-12, Asia/Novosibirsk; exact proposal time unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Question: avoid repeating image inventory without changing driver selection?
- Alternatives: remove compatibility checks/always mount JDBC; ship known-digest
  metadata; introduce persistent inventory storage; or cache validated successful
  inventory in the engine process by exact digest/platform/rule revision.
- Decision: O6 approved by “Утвердить проект компонентов”: bounded 32-entry cache
  with copied values, cancellable coalescing, no cached failures or disk/schema.
- Rationale: preserve real driver provenance and keep cold first-use cost visible;
  do not warm a new cache solely to improve benchmark results.

## Return results on the same helper exec

- Conversation date: 2026-11-12, Asia/Novosibirsk; exact proposal time unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Question: remove repeated result acquisition without losing private durable
  capture, exact parent, child-idle or prior-output absence checks?
- Alternatives: drop the checks; retain all separate Docker reads; add a live
  JVM callback integration; or return a bounded verified result envelope through
  the same stock-CLI helper exec while retaining independent reads.
- Decision: O7 approved by “Утвердить проект компонентов”: separate transport/child outcomes and
  closed result framing, no automatic SQL retry or error-to-success fallback.
- Rationale: remove command round trips, not completion evidence or ownership;
  account for the new real boundaries rather than retain fictional cost scopes.

## Propose the retained-planning ownership representation

- Conversation date: 2026-11-12, Asia/Novosibirsk; exact proposal time unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Question: how to persist the approved O5 live-preview ownership/transfer?
- Alternatives: duplicate physical clone/operation identities; overload absence
  facts; or add one relational ownership record referencing exact append-only
  retention/disposal facts and one deferred-bound working session.
- Decision (schema approved on 2026-11-12): one 13-column table and a partial
  live-owner index, existing journal facts, exact transition/cleanup guards and
  session store format 3→4; no prototype migration, new API or SQL replay.
- Rationale: avoid duplicate destructive owners, fictitious absence, permanent
  historical resource pins and uncertain-COMMIT cleanup races.

## Approve the bounded inventory tests and coupled observations

- Conversation date: 2026-11-12, Asia/Novosibirsk; exact reply time unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Question: how to test O6 without accepting arbitrary observed probe counts?
- Alternatives: relax counters to accept either zero or one; prewarm the cache;
  or predeclare exact artifact-bound cold/hit expectations and preserve legacy
  controls while testing isolation, cancellation, failures and finite capacity.
- Decision: approve IC01–IC08 and only OT-IC01–OT-IC03 in the inventory test plan.
- Rationale: distinguish a genuine avoided probe from missing compatibility work;
  preserve the original timing window, workload and independent data oracles.

## Approve the same-exec framing and its exact test changes

- Conversation date: 2026-11-12, Asia/Novosibirsk; exact reply time unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Question: how to frame the approved same-exec result and preserve completion
  checks without another result acquisition?
- Alternatives: delimit arbitrary streams as text; add another archive read;
  or fixed ordered binary-length members, a bounded optional detail and a final
  root-helper idle/prune footer, with exact pre/post parent inspection.
- Decision: approve IR01–IR10, the wire/quota definition and only OT-IR01–OT-IR08
  in issue89-same-exec-result-tests.md. Transport zero and manifest child exit
  remain distinct; quota/transport failures cannot trigger a hidden success read.
  Legacy independent reads/parsers remain strict; new measurements are explicitly
  selected by exact artifact before observation.
- Rationale: avoid successful-path Docker round trips while retaining durable
  output, identity/hash/EOF checks and the original API timing/speed criterion.
