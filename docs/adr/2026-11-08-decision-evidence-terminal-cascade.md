# Decision-evidence retention follows its parent job

- Conversation date: 2026-11-08, Asia/Novosibirsk; exact reply timestamp unavailable.
- GitHub participant: @evilguest (41718235).
- Agent: Codex / GPT-5.
- Status: accepted, including the exact current catalog/hash/provenance update.

## Question and alternatives

WH06's real queue and Docker tests showed that terminal job deletion fails with
`decision evidence link is retained`. During the job cascade, an evidence row
can be deleted before its sibling decision. Checking only decision existence
therefore rejects legitimate cascading history removal. Options: leave the
terminal job undeletable, disable/reorder guards during deletion, or retain
links according to the decision's parent job lifetime.

## Decision and rationale

Use the third option: prepare_decision_evidence_delete_guard joins its decision
to prepare_jobs. A direct link deletion while the job exists remains forbidden;
deleting an active/protected job remains forbidden by the existing job guards.
Once the parent job has been deleted, cascade order cannot falsely retain the
link. No table, FK, public API, authorization or SQL replay change is introduced.

Update only this shared definition and the corresponding current target/hash/
amendment provenance. Preserve the frozen released/composed history and other
old assertions. The approved WH06 terminal-deletion requirement already covers
the failure; no additional feature or broad recovery matrix is authorized.

This clarifies terminal cascade behavior without obsoleting the earlier
handoff/storage decisions. The schema amendment records critical/repeat review.
