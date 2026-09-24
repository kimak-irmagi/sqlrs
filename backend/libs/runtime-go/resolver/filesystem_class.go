package resolver

// enabledFilesystemEvidence is deliberately empty for v0.2.0. A filesystem may
// enter this table only with a matching mandatory native CI job and evidence
// revision; unlisted classes rehash and return UNKNOWN.
var enabledFilesystemEvidence = map[string]string{}

func cheapRevalidationEnabled(class, evidenceRevision string) bool {
	return enabledFilesystemEvidence[class] == evidenceRevision && evidenceRevision != ""
}
