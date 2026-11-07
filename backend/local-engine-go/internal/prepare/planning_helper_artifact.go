package prepare

import (
	"context"
	"fmt"

	"github.com/sqlrs/engine-local/internal/prepare/inputbundle"
	engineRuntime "github.com/sqlrs/engine-local/internal/runtime"
)

const maxPlanningHelperBytes = 1 << 20

// publishPlanningHelper stores the exact verified helper bytes before the
// job's capture_helper lease and planning attempt are committed. Its digest,
// not the current engine binary, is the recovery identity. See C01a/P01.
func publishPlanningHelper(ctx context.Context, artifacts inputbundle.Store) (string, error) {
	data, digest := engineRuntime.LiquibaseCaptureHelper()
	if len(data) == 0 || len(data) > maxPlanningHelperBytes {
		return "", fmt.Errorf("planning capture helper size is unsupported")
	}
	stored, err := artifacts.PublishBlob(ctx, data)
	if err != nil {
		return "", err
	}
	if stored != digest {
		return "", fmt.Errorf("planning capture helper digest differs")
	}
	return digest, nil
}

// materializeSavedPlanningHelper reacquires the journal-bound helper even if
// the replacement engine embeds a newer protocol or newer helper bytes. The
// caller must verify the corresponding job lease before using this path.
func materializeSavedPlanningHelper(ctx context.Context, artifacts inputbundle.Store, savedDigest, destination string) error {
	if savedDigest == "" || destination == "" {
		return fmt.Errorf("saved planning helper identity and destination are required")
	}
	return artifacts.MaterializeBlob(ctx, savedDigest, destination, maxPlanningHelperBytes)
}
