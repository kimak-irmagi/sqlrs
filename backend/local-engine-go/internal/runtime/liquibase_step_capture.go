package runtime

import (
	"context"
	_ "embed"
	"fmt"
	"regexp"
	"time"
)

const LiquibaseStepCaptureProtocol = "lb-step-capture-v2"

var stepCaptureToken = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$`)

//go:embed assets/liquibase-step-capture-v2.sh
var liquibaseStepCaptureHelper []byte

// LiquibaseStepCaptureHelper returns immutable engine-owned helper bytes and
// their content-addressed identity for the session's retained helper lease.
func LiquibaseStepCaptureHelper() ([]byte, string) {
	return append([]byte(nil), liquibaseStepCaptureHelper...), captureDigest(liquibaseStepCaptureHelper)
}

// StepCaptureIdentity binds a child command to the durable session and common
// attempt. None of these fields contain credentials, SQL, or host paths. See
// docs/architecture/liquibase-reusable-container-storage-schema.md.
type StepCaptureIdentity struct {
	JobID, TaskID, SessionID, InvocationRef string
	AttemptNo, WorkerGeneration             int
	RunnerID, ExecutionDigest, HelperDigest string
	// PriorRetirementRef is the exact nullable link from the saved slot, not
	// an output-derived authorization or permission to run another child.
	PriorRetirementRef *string
	// Prune is derived by the coordinator from the exact committed slot/intent
	// join. Only invocation consumes it; reading completion needs no authority
	// to delete bytes or run a child.
	Prune *CapturePruneAuthorization
}

// CompleteStepCapture proves the child exit and both complete streams while
// the reusable runner remains alive. Its manifest hash can be persisted
// without persisting the protected output bytes.
type CompleteStepCapture struct {
	CompleteCapture
	ExitCode       int
	ManifestDigest string
	ManifestBytes  int64
}

type liquibaseStepCaptureManifest struct {
	Protocol           string  `json:"protocol"`
	JobID              string  `json:"job_id"`
	TaskID             string  `json:"task_id"`
	SessionID          string  `json:"session_id"`
	InvocationRef      string  `json:"invocation_ref"`
	AttemptNo          int     `json:"attempt_no"`
	WorkerGeneration   int     `json:"worker_generation"`
	RunnerID           string  `json:"runner_id"`
	ExecutionDigest    string  `json:"execution_digest"`
	HelperDigest       string  `json:"helper_digest"`
	ChildExitCode      *int    `json:"child_exit_code"`
	StdoutEOF          bool    `json:"stdout_eof"`
	StderrEOF          bool    `json:"stderr_eof"`
	StdoutBytes        int64   `json:"stdout_bytes"`
	StdoutSHA256       string  `json:"stdout_sha256"`
	StderrBytes        int64   `json:"stderr_bytes"`
	StderrSHA256       string  `json:"stderr_sha256"`
	PriorRetirementRef *string `json:"prior_retirement_ref"`
}

func validStepCaptureIdentity(identity StepCaptureIdentity) bool {
	return stepCaptureToken.MatchString(identity.JobID) && stepCaptureToken.MatchString(identity.TaskID) &&
		stepCaptureToken.MatchString(identity.SessionID) && stepCaptureToken.MatchString(identity.InvocationRef) &&
		identity.AttemptNo > 0 && identity.WorkerGeneration > 0 && createdContainerID.MatchString(identity.RunnerID) &&
		validCaptureDigest(identity.ExecutionDigest) && validCaptureDigest(identity.HelperDigest) &&
		(identity.PriorRetirementRef == nil || stepCaptureToken.MatchString(*identity.PriorRetirementRef))
}

// ReadCompleteLiquibaseStepCapture checks the child manifest independently of
// a Docker CLI exit code. The caller additionally proves exact runner identity
// and absence of surviving untrusted children before recording completion.
// Missing or partial files remain unknown outcomes; no output is truncated.
func ReadCompleteLiquibaseStepCapture(dir string, identity StepCaptureIdentity, maxStreamBytes int64) (CompleteStepCapture, error) {
	if dir == "" {
		return CompleteStepCapture{}, fmt.Errorf("invalid step capture identity or limit")
	}
	return readCompleteLiquibaseStepCapture(filesystemCaptureFiles{dir: dir}, identity, maxStreamBytes)
}

// readCompleteLiquibaseStepCapture binds the nullable prior link and complete
// streams independently of local files or a container's diagnostic exit code.
func readCompleteLiquibaseStepCapture(files captureFileReader, identity StepCaptureIdentity, maxStreamBytes int64) (CompleteStepCapture, error) {
	if !validStepCaptureIdentity(identity) || maxStreamBytes <= 0 {
		return CompleteStepCapture{}, fmt.Errorf("invalid step capture identity or limit")
	}
	data, err := files.readCapture("manifest.complete", 8192)
	if err != nil {
		return CompleteStepCapture{}, fmt.Errorf("complete step manifest unavailable")
	}
	var manifest liquibaseStepCaptureManifest
	if !decodeCaptureObject(data, &manifest, captureRequiredFields+" session_id invocation_ref worker_generation runner_id prior_retirement_ref", "prior_retirement_ref") {
		return CompleteStepCapture{}, fmt.Errorf("invalid step completion manifest")
	}
	priorMatches := manifest.PriorRetirementRef == nil && identity.PriorRetirementRef == nil
	if manifest.PriorRetirementRef != nil && identity.PriorRetirementRef != nil {
		priorMatches = *manifest.PriorRetirementRef == *identity.PriorRetirementRef
	}
	if manifest.Protocol != LiquibaseStepCaptureProtocol || manifest.JobID != identity.JobID ||
		manifest.TaskID != identity.TaskID || manifest.SessionID != identity.SessionID ||
		manifest.InvocationRef != identity.InvocationRef || manifest.AttemptNo != identity.AttemptNo ||
		manifest.WorkerGeneration != identity.WorkerGeneration || manifest.RunnerID != identity.RunnerID ||
		manifest.ExecutionDigest != identity.ExecutionDigest || manifest.HelperDigest != identity.HelperDigest || !priorMatches ||
		manifest.ChildExitCode == nil || *manifest.ChildExitCode < 0 || *manifest.ChildExitCode > 255 ||
		!manifest.StdoutEOF || !manifest.StderrEOF || manifest.StdoutBytes < 0 || manifest.StderrBytes < 0 ||
		!validCaptureHex(manifest.StdoutSHA256) || !validCaptureHex(manifest.StderrSHA256) {
		return CompleteStepCapture{}, fmt.Errorf("step manifest identity or completion differs")
	}
	stdout, err := files.readCapture("stdout.complete", maxStreamBytes)
	if err != nil {
		return CompleteStepCapture{}, fmt.Errorf("complete step stdout unavailable")
	}
	stderr, err := files.readCapture("stderr.complete", maxStreamBytes)
	if err != nil {
		return CompleteStepCapture{}, fmt.Errorf("complete step stderr unavailable")
	}
	if int64(len(stdout)) != manifest.StdoutBytes || int64(len(stderr)) != manifest.StderrBytes ||
		captureHex(stdout) != manifest.StdoutSHA256 || captureHex(stderr) != manifest.StderrSHA256 {
		return CompleteStepCapture{}, fmt.Errorf("step stream bytes differ from completion manifest")
	}
	return CompleteStepCapture{CompleteCapture: CompleteCapture{Stdout: stdout, Stderr: stderr,
		StdoutDigest: captureDigest(stdout), StderrDigest: captureDigest(stderr)},
		ExitCode: *manifest.ChildExitCode, ManifestDigest: captureDigest(data), ManifestBytes: int64(len(data))}, nil
}

// stepIdleProbe inspects the fixed untrusted uid used by the pinned capture
// helper. A daemonized child that closed its capture pipes must still prevent
// publication. Failure to read procfs is an inconclusive probe, not success.
const stepIdleProbe = `set -eu
for status in /proc/[0-9]*/status; do
  [ -e "$status" ] || continue
  if ! uid=$(sed -n 's/^Uid:[[:space:]]*\([0-9]*\).*/\1/p' "$status" 2>/dev/null); then
    [ ! -e "$status" ] && continue
    exit 90
  fi
  if [ -z "$uid" ]; then
    [ ! -e "$status" ] && continue
    exit 90
  fi
  [ "$uid" != 1001 ] || exit 91
done
printf 'sqlrs-step-idle-v1\n'`

// ReadCompleteLiquibaseStepOutput reads private bytes from the exact parent,
// ignoring the former host-directory argument. Running parents also require an
// idle child probe; exited parents never receive exec/restart. Both branches
// require unchanged pre/post identity and state, and share one 30-second read
// deadline. See liquibase-private-capture-ownership.md live/stopped read rules.
func (r *DockerRuntime) ReadCompleteLiquibaseStepOutput(ctx context.Context, name, id, _ string,
	identity StepCaptureIdentity, maxStreamBytes int64) (CompleteStepCapture, error) {
	if r == nil || id != identity.RunnerID || !validStepCaptureIdentity(identity) {
		return CompleteStepCapture{}, fmt.Errorf("invalid exact Liquibase session runner")
	}
	phase := ""
	inspect := func() error {
		if err := ctx.Err(); err != nil {
			return err
		}
		view, found, err := r.InspectNamedContainer(ctx, name)
		if err != nil || !found || view.ID != id || (view.Status != "running" && view.Status != "exited") || (phase != "" && view.Status != phase) {
			return fmt.Errorf("exact Liquibase session runner state is not stable")
		}
		phase = view.Status
		return nil
	}
	if err := inspect(); err != nil {
		return CompleteStepCapture{}, err
	}
	ctx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	capture, err := readCompleteLiquibaseStepCapture(containerCaptureFiles{ctx: ctx, runtime: r, id: id, invocation: identity.InvocationRef}, identity, maxStreamBytes)
	if err != nil {
		return CompleteStepCapture{}, err
	}
	if phase == "running" {
		output, err := r.run(ctx, []string{"exec", "-u", "0", id, "/bin/sh", "-c", stepIdleProbe}, nil)
		if err != nil || output != "sqlrs-step-idle-v1\n" {
			return CompleteStepCapture{}, fmt.Errorf("Liquibase session child-process state is not proved idle")
		}
	}
	if err := inspect(); err != nil {
		return CompleteStepCapture{}, err
	}
	if err := ctx.Err(); err != nil {
		return CompleteStepCapture{}, err
	}
	return capture, nil
}

// InvokeTrackedLiquibaseStep runs the stock CLI through the pinned helper in
// the exact live runner. The coordinator must commit exec_requested first.
// Docker's return code is diagnostic: only a complete, idle-checked capture
// returns a known child result. See N02/N03 in the reusable-container tests.
func (r *DockerRuntime) InvokeTrackedLiquibaseStep(ctx context.Context, name, captureDir string,
	identity StepCaptureIdentity, liquibaseArgs []string, maxStreamBytes int64) (CompleteStepCapture, error) {
	if r == nil || !validStepCaptureIdentity(identity) || maxStreamBytes < 1 || maxStreamBytes > 64<<20 || len(liquibaseArgs) == 0 {
		return CompleteStepCapture{}, fmt.Errorf("invalid tracked Liquibase invocation")
	}
	prune, err := serializeStepPrune(identity)
	if err != nil {
		return CompleteStepCapture{}, err
	}
	view, found, err := r.InspectNamedContainer(ctx, name)
	if err != nil || !found || view.ID != identity.RunnerID || view.Status != "running" {
		return CompleteStepCapture{}, fmt.Errorf("exact Liquibase session runner is not running")
	}
	args := stepCaptureExecEnvironment(identity, maxStreamBytes, prune)
	args = append(args, identity.RunnerID, "/bin/sh", "/sqlrs/helper/step-capture.sh", "liquibase")
	args = append(args, liquibaseArgs...)
	// Do not surface either Docker output or its arbitrary error text. The
	// protected streams and manifest carry the recoverable child result.
	_, _ = r.run(ctx, args, nil)
	return r.ReadCompleteLiquibaseStepOutput(ctx, name, identity.RunnerID, captureDir, identity, maxStreamBytes)
}
