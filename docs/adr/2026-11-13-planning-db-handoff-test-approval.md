# Approve O5 tests and bounded IC06 fixture corrections

## O5 test inventory

- Conversation date: 2026-11-13, Asia/Novosibirsk; exact reply time unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Question: proceed with the critically/repeatedly reviewed planning-DB handoff
  tests and their explicitly bounded old-test changes?
- Alternatives: omit ownership/failure controls to shorten delivery; expand full
  recovery matrices; or verify the approved normal-path schema and its three new
  writers while preserving recovery restrictions and the existing speed gate.
- Decision: user accepted PG01–PG10 and OT-PG01/OT-PG02 after the review summary.
  Tests precede O5 implementation; other old-test contradictions need approval.
- Rationale: prove real DB reuse and single destructive ownership without
  broadening #89 into the deferred recovery programme.

## IC06 independent-request fixture

- Conversation date: 2026-11-13, Asia/Novosibirsk; exact reply time unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Question: repair the three demonstrated new-fixture failures or weaken product
  binding checks to satisfy their original preparations?
- Alternatives: alter product semantics; remove failed cases; or correct only
  the trigger target, second-job signature scope and shared request mount slice.
- Decision: accept only those three fixture corrections. Retain per-job work,
  failure/no-progress and first-job immutability requirements.
- Rationale: tests must reach the real binding boundary and model independent
  requests, not mistake first-job success or aliased fixture data for a defect.

## IC06 safe error classification and positive control

- Conversation date: 2026-11-13, Asia/Novosibirsk; exact reply time unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Question: how to prove the intended binding refusal without requiring the
  deliberately hidden SQLite error text?
- Alternatives: expose SQLite text; accept an arbitrary failure; or retain
  `ErrLiquibaseBinding` and prove the exact same binding succeeds after removing
  only the fixture's trigger.
- Decision: user approved safe `errors.Is` classification and that positive
  metadata control after the unchanged negative and first-job assertions.
- Rationale: preserve privacy while independently establishing that the intended
  boundary, rather than an unrelated early refusal, caused the failure.
- Evidence: all four IC06 cases and the complete focused inventory suite passed
  on 2026-11-13. This is not native Actions or speed acceptance.

## PG02 cache-explanation flag correction

- Conversation date: 2026-11-13, Asia/Novosibirsk; exact reply time unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-6.
- Question: should ordinary retained planning require the existing
  `PlanningRecoverySnapshot.DecisionSaved` cache-explanation flag?
- Alternatives: change the product meaning of the flag; retain the erroneous
  expectation; or correct only the new PG02 expectation and matching PG04 oracle.
- Decision: user approved the bounded correction to require `DecisionSaved=false`.
  Preserve complete_planning, exactly three evidence links, saved plan, task and
  attempt assertions. Existing product semantics and old tests stay unchanged.
- Rationale: the passing legacy-writer control proves complete_planning exists
  with this flag false; decision_saved_at belongs to cache_explain.

## IC07 full-cache fixture retains the planner

- Conversation date: 2026-11-13, Asia/Novosibirsk; exact reply time unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-6.
- Question: how to keep the old inventory-negative control after the approved
  O5 complete-boundary qualifier requires actual retained planning?
- Alternatives: weaken the qualifier; leave IC07 failing at an unrelated missing
  boundary; or add only its mandatory plan.retain span to the full-cache fixture.
- Decision: user approved only adding plan.retain in inventory-boundaries.test.mjs.
  Preserve two DB allocations, plan.dispose=1, plan.release=1 and refusal of an
  extra runner.inventory observation.
- Rationale: let IC07 reach its inventory boundary without relaxing O5 ownership,
  cached physical allocation counts or the original inventory requirement.

## First O5 coverage iteration CP-PG01–CP-PG05

- Conversation timestamp: 2026-11-13, Asia/Novosibirsk; approval received before
  implementation of this iteration (exact reply time unavailable).
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-6.
- Question: approve the documented requirement-based coverage increase after the
  initial O5 per-line measurement fell below the required minimum?
- Alternatives: accept insufficient coverage; change the target; or add the
  local coordinator, disposal, journal readback, publication and source-reader
  controls described in CP-PG01–CP-PG05.
- Decision: user approved CP-PG01–CP-PG05. Keep target 100%, minimum 95%, the
  existing aggregate exception, all previous test limits and Actions-only native
  workloads. Remeasure before requesting any further coverage iteration.
- Rationale: cover the actual ownership/effect requirements exposed by zero-hit
  blocks without weakening existing tests or extending deferred recovery scope.

## Second O5 coverage iteration CP2-H/D/T/R

- Conversation timestamp: 2026-11-13, Asia/Novosibirsk; exact reply time unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-6.
- Question: approve the second requirement-based coverage iteration after CP-PG
  left the three new journal files below the minimum?
- Alternatives: stop below the target; weaken the threshold; or implement the
  documented CP2 retention/readback, disposal, real state-source transfer and
  proven duplicate-Close-check review.
- Decision: user approved CP2-H/D/T/R. Preserve 100% target / 95% minimum,
  existing old-test approvals and the Actions-only native/workload boundary.
- Rationale: qualify the remaining ownership/observation boundaries and the real
  state-source positive path without expanding recovery or the three-writer
  BUSY/lost-reply matrix. Remeasure before a further coverage iteration.
