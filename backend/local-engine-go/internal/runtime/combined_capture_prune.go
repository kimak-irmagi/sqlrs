package runtime

import (
	"context"
	"fmt"

	"github.com/sqlrs/engine-local/internal/preparecost"
)

// combinedStepPruneProbe performs the existing idle/absence/idle observations
// in one read-only root exec. A sentinel preserves every helper-response byte,
// including trailing newlines, and a nonzero helper status cannot be hidden by
// a later printf. Requirements: issue89-cost-attribution-and-trimming.md §6, CPR08.
const combinedStepPruneProbe = "idle() {\n" + stepIdleProbe + "\n}\n" + `
idle >/dev/null
reply=$(/bin/sh /sqlrs/helper/step-capture.sh probe-prune && printf '.') || exit 91
[ "$reply" = 'sqlrs-capture-prune-absent-v1
.' ] || exit 91
idle >/dev/null
printf 'sqlrs-step-idle-pruned-v1\n'
`

// stepPruneObservation is constructible only by this package, after native
// completion/absence and the enclosing post-inspection. No mutable caller
// pointers, private bytes or paths are retained; it grants no SQL authority.
type stepPruneObservation struct {
	identity                   StepCaptureIdentity
	authority                  CapturePruneAuthorization
	manifestDigest             string
	manifestBytes              int64
	exitCode                   int
	stdoutDigest, stderrDigest string
}

// observationIdentity retains only comparable identity values. Nullable prior
// links and prune authority are checked separately against the saved value copy.
func observationIdentity(identity StepCaptureIdentity) StepCaptureIdentity {
	identity.PriorRetirementRef, identity.Prune = nil, nil
	return identity
}

// MatchesPruneObservation matches a native receipt against both exact current
// completion and the queue's committed prior authority. Zero/manual/local-file
// captures cannot prove absence. It performs no I/O, SQL, retry or mutation.
// Requirements: issue89-cost-attribution-and-trimming.md §6, CPR02/CPR05.
func (c CompleteStepCapture) MatchesPruneObservation(identity StepCaptureIdentity, authority CapturePruneAuthorization) bool {
	p := c.pruneObservation
	return p != nil && identity.PriorRetirementRef != nil && *identity.PriorRetirementRef == authority.RetirementRef &&
		identity.Prune != nil && *identity.Prune == authority && p.authority == authority && p.identity == observationIdentity(identity) &&
		p.manifestDigest == c.ManifestDigest && p.manifestBytes == c.ManifestBytes && p.exitCode == c.ExitCode &&
		p.stdoutDigest == c.StdoutDigest && p.stderrDigest == c.StderrDigest
}

// readCombinedStepPrune performs only the inner bounded command. The caller
// owns the common deadline, archive validation and both exact parent checks;
// success here alone cannot construct or expose a completed native receipt.
func (r *DockerRuntime) readCombinedStepPrune(ctx context.Context, runner boundedBinaryRunner, identity StepCaptureIdentity, authorization string, limit int64) error {
	args := stepCaptureExecEnvironment(identity, limit, authorization)
	args = append(args, "-e", "SQLRS_CAPTURE_COST=0", identity.RunnerID, "/bin/sh", "-c", combinedStepPruneProbe)
	output, err := preparecost.Observe(ctx, preparecost.DockerCommand, func(child context.Context) ([]byte, error) {
		return runner.RunBounded(child, r.binary, args, 64, 4096)
	})
	if contextErr := ctx.Err(); contextErr != nil {
		return contextErr
	}
	if err != nil || string(output) != "sqlrs-step-idle-pruned-v1\n" {
		return fmt.Errorf("combined Liquibase idle/prune observation is unproved")
	}
	return nil
}
