package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"time"
)

// CapturePruneAuthorization is the public-identity-only projection of the
// coordinator's committed retirement/slot join. It is neither client JSON nor
// standalone authority: the queue must first validate publication and pins, and
// only the pinned root helper may consume it. No path/shell/credential fields
// exist. Requirements: private-capture storage schema §4.1 and PC10/PC20-N01.
type CapturePruneAuthorization struct {
	RetirementRef       string `json:"retirement_ref"`
	SessionID           string `json:"session_id"`
	WorkerGeneration    int    `json:"worker_generation"`
	RunnerID            string `json:"runner_id"`
	PriorInvocationRef  string `json:"prior_invocation_ref"`
	PriorManifestDigest string `json:"prior_manifest_digest"`
	NextInvocationRef   string `json:"next_invocation_ref"`
	HelperDigest        string `json:"helper_digest"`
}

// VerifyLiquibaseCapturePruned observes one already-authorized prior directory
// without initialization, deletion or SQL. The caller owns session exclusion.
// All commands share one deadline; exact running/idle parent is checked around
// the fixed bounded helper command. Requirements: ownership §9, MP03.
func (r *DockerRuntime) VerifyLiquibaseCapturePruned(ctx context.Context, name string, identity StepCaptureIdentity) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if r == nil || !validStepCaptureIdentity(identity) {
		return fmt.Errorf("invalid exact Liquibase prune observation")
	}
	prune, err := serializeStepPrune(identity)
	if err != nil || prune == "" {
		return fmt.Errorf("saved Liquibase prune authority unavailable")
	}
	runner, ok := r.runner.(boundedBinaryRunner)
	if !ok {
		return fmt.Errorf("bounded Liquibase prune observation unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	inspect := func() error {
		view, found, err := r.InspectNamedContainer(ctx, name)
		if err != nil || !found || view.ID != identity.RunnerID || view.Status != "running" {
			return fmt.Errorf("exact Liquibase prune parent is not running")
		}
		return nil
	}
	bounded := func(args []string, want string) error {
		data, err := runner.RunBounded(ctx, r.binary, args, 64, 4096)
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if err != nil || string(data) != want {
			return fmt.Errorf("exact Liquibase prune absence is unproved")
		}
		return nil
	}
	idle := []string{"exec", "-u", "0", identity.RunnerID, "/bin/sh", "-c", stepIdleProbe}
	if err := inspect(); err != nil {
		return err
	}
	if err := bounded(idle, "sqlrs-step-idle-v1\n"); err != nil {
		return err
	}
	args := stepCaptureExecEnvironment(identity, 1, prune)
	args = append(args, identity.RunnerID, "/bin/sh", "/sqlrs/helper/step-capture.sh", "probe-prune")
	if err := bounded(args, "sqlrs-capture-prune-absent-v1\n"); err != nil {
		return err
	}
	if err := bounded(idle, "sqlrs-step-idle-v1\n"); err != nil {
		return err
	}
	return inspect()
}

// stepCaptureExecEnvironment supplies every per-invocation binding explicitly.
// An empty saved authority overrides any value inherited from the image; it is
// not equivalent to omitting the Docker environment override. MP03.
func stepCaptureExecEnvironment(identity StepCaptureIdentity, maxStreamBytes int64, prune string) []string {
	args := []string{"exec", "-u", "0"}
	for _, value := range []string{
		"SQLRS_CAPTURE_DIR=/sqlrs/capture/" + identity.InvocationRef,
		"SQLRS_JOB_ID=" + identity.JobID, "SQLRS_TASK_ID=" + identity.TaskID,
		"SQLRS_SESSION_ID=" + identity.SessionID, "SQLRS_INVOCATION_REF=" + identity.InvocationRef,
		"SQLRS_ATTEMPT_NO=" + strconv.Itoa(identity.AttemptNo),
		"SQLRS_WORKER_GENERATION=" + strconv.Itoa(identity.WorkerGeneration),
		"SQLRS_RUNNER_ID=" + identity.RunnerID, "SQLRS_EXECUTION_DIGEST=" + identity.ExecutionDigest,
		"SQLRS_HELPER_DIGEST=" + identity.HelperDigest,
		"SQLRS_CAPTURE_MAX_BYTES=" + strconv.FormatInt(maxStreamBytes, 10),
		"SQLRS_CAPTURE_PRUNE_AUTHORIZATION=" + prune,
	} {
		args = append(args, "-e", value)
	}
	return args
}

// serializeStepPrune refuses incomplete or foreign bindings before any Docker
// operation. A saved nullable prior link is required even when the deletion was
// separately confirmed; reissuing exec_requested is still forbidden by journal
// reconciliation, not authorized by the presence of this value.
func serializeStepPrune(identity StepCaptureIdentity) (string, error) {
	if identity.Prune == nil && identity.PriorRetirementRef == nil {
		return "", nil
	}
	authority := identity.Prune
	if authority == nil || identity.PriorRetirementRef == nil || authority.RetirementRef != *identity.PriorRetirementRef ||
		!stepCaptureToken.MatchString(authority.RetirementRef) || authority.SessionID != identity.SessionID ||
		authority.WorkerGeneration != identity.WorkerGeneration || authority.RunnerID != identity.RunnerID ||
		authority.HelperDigest != identity.HelperDigest || authority.NextInvocationRef != identity.InvocationRef ||
		!stepCaptureToken.MatchString(authority.PriorInvocationRef) || authority.PriorInvocationRef == authority.NextInvocationRef ||
		!validCaptureDigest(authority.PriorManifestDigest) {
		return "", fmt.Errorf("invalid exact Liquibase prune authorization")
	}
	// Closed primitive fields cannot fail JSON encoding. The bounded token,
	// digest and integer grammar keeps the whole record below the 4096-byte
	// wire cap; no unbounded strings or custom marshalers participate.
	wire := struct {
		Protocol string `json:"protocol"`
		CapturePruneAuthorization
	}{Protocol: "lb-capture-prune-v1", CapturePruneAuthorization: *authority}
	encoded, _ := json.Marshal(wire)
	return string(encoded), nil
}
