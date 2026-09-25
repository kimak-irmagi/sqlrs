package resolver

type publicationStage string

const (
	publicationTemporaryCreated publicationStage = "temporary-created"
	publicationContentWritten   publicationStage = "content-written"
	publicationFileSynced       publicationStage = "file-synced"
	publicationReplaced         publicationStage = "replaced"
	publicationDirectorySynced  publicationStage = "directory-synced"
)

// publicationBarrier is a package-private test seam for observing publication
// boundaries. Production execution leaves it as a no-op.
var publicationBarrier = func(string, publicationStage) {}
