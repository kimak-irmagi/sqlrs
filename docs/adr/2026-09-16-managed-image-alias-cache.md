# Managed image aliases in cache reads

Conversation timestamp: 2026-09-16 20:25 Asia/Novosibirsk (13:25 UTC).
GitHub user ID: @evilguest. Agent: Codex (GPT-6).
Status: accepted; the user requested the image-alias review fix.

Question: how should an existing cached state be read through another image
reference containing the same immutable digest?

Alternatives: compare complete image-reference strings; compare digests and
derive the snapshot path from the new request; compare digests and retain the
published state's recorded storage location.

Decision: use the same digest extraction for lineage selection and cache
validation. Resolve an existing snapshot's path from its stored image reference.
Keep lineage, identity-digest and seal checks; do not rewrite metadata or move
snapshots. No schema or public API change is needed.

Rationale: aliases select the same lineage and state key. They must not cause a
false identity conflict or redirect a cache read to an empty directory.
