package resolver

// enabledFilesystemEvidence lists only evidence revisions with mandatory native
// integration coverage. Unlisted classes conservatively rehash and return UNKNOWN.
var enabledFilesystemEvidence = map[string]string{
	"ntfs-usn": "ntfs-usn-v1",
}

func cheapRevalidationEnabled(class, evidenceRevision string) bool {
	return enabledFilesystemEvidence[class] == evidenceRevision && evidenceRevision != ""
}
