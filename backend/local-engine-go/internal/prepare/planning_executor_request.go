package prepare

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"github.com/sqlrs/engine-local/internal/prepare/effectiveconfig"
	"github.com/sqlrs/engine-local/internal/prepare/inputbundle"
	"github.com/sqlrs/engine-local/internal/prepare/queue"
	engineRuntime "github.com/sqlrs/engine-local/internal/runtime"
)

var planningJobIDPattern = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9_.-]{0,127}$`)
var planningCloneIDPattern = regexp.MustCompile(`^[0-9a-f]{24}$`)

// planningCapturePath maps the journal's opaque refs to one job-private
// physical directory. No client-supplied path or mutable project file is
// consulted. See C02/F03a in the planning recovery storage design.
func planningCapturePath(stateRoot, jobID, cloneID string) (string, error) {
	if !filepath.IsAbs(stateRoot) || !planningJobIDPattern.MatchString(jobID) ||
		!planningCloneIDPattern.MatchString(cloneID) {
		return "", fmt.Errorf("invalid planning capture identity")
	}
	return filepath.Join(filepath.Clean(stateRoot), "jobs", jobID, "planning-capture", cloneID), nil
}

// buildPlanningExecutorRequest creates only the journal-named protected
// resources and constructs a root-owned helper launch. The helper drops to
// uid/gid 1001 before running Liquibase; neither stream enters Docker logs.
// A failure after any creation leaves the exact resources for recovery.
// Capture bytes belong only to the container layer; see PC04 in
// docs/architecture/liquibase-private-capture-ownership.md and its test amendment.
func (m *PrepareService) buildPlanningExecutorRequest(ctx context.Context,
	prepared preparedRequest, saved queue.PlanningRecoverySnapshot,
	instance engineRuntime.Instance, password string, artifacts inputbundle.Store) (
	engineRuntime.RunRequest, *effectiveconfig.ProtectedConfig, string, error) {
	if m == nil || saved.Attempt.TaskID != "plan" || saved.Attempt.Phase != "planned" ||
		saved.Attempt.ExecutorID != "" || saved.ClonePhase != "active" ||
		saved.CloneRuntimeID == "" || saved.CloneRuntimeID != instance.ID ||
		saved.Clone.ID != saved.Attempt.CloneID ||
		prepared.liquibaseFingerprint != saved.Binding.AdapterBindingDigest ||
		prepared.liquibaseImageID != "liquibase/liquibase@"+saved.Binding.ExecutorImageDigest ||
		!projectionProtectedRefPattern.MatchString(saved.Capture.ProtectedInput) ||
		!strings.HasSuffix(saved.Capture.ProtectedInput, "-plan-"+saved.Clone.ID+"-input") ||
		saved.Capture.Stdout != "plan-"+saved.Clone.ID+"-stdout" ||
		saved.Capture.Stderr != "plan-"+saved.Clone.ID+"-stderr" ||
		saved.Capture.Manifest != "plan-"+saved.Clone.ID+"-manifest" ||
		saved.Capture.ProtocolRevision != engineRuntime.LiquibaseCaptureProtocol {
		return engineRuntime.RunRequest{}, nil, "", fmt.Errorf("saved planning executor identity differs")
	}
	captureDir, err := planningCapturePath(m.stateStoreRoot, saved.Attempt.JobID, saved.Clone.ID)
	if err != nil {
		return engineRuntime.RunRequest{}, nil, "", err
	}
	// Keep the deterministic locator for the reconciliation interface, but
	// never create or mount a host capture directory. Private/v2 reads resolve
	// the exact journal-bound container, not this host pathname. PC04/OT-PC04.
	helperPath := filepath.Join(m.stateStoreRoot, "jobs", saved.Attempt.JobID, "planning-helpers", saved.Clone.ID, "capture.sh")
	if err := materializeSavedPlanningHelper(ctx, artifacts, saved.Capture.HelperDigest, helperPath); err != nil {
		return engineRuntime.RunRequest{}, nil, captureDir, err
	}
	credentialRoot, err := m.liquibaseProjectionSettingsRoot(saved.Attempt.JobID, saved.Capture.ProtectedInput)
	if err != nil {
		return engineRuntime.RunRequest{}, nil, captureDir, err
	}
	if prepared.liquibaseProjection == nil || prepared.liquibaseProjection.job.jobID != saved.Attempt.JobID || credentialRoot != prepared.liquibaseProjection.Settings {
		return engineRuntime.RunRequest{}, nil, captureDir, errLiquibaseProjection
	}
	args := replaceLiquibaseCommand(prepared.normalizedArgs, "updateSQL")
	request, protected, err := m.liquibaseContainerRequestWithConfig(prepared, instance, args, password,
		func(options effectiveconfig.ProtectedOptions) (*effectiveconfig.ProtectedConfig, error) {
			options.Root = credentialRoot
			return effectiveconfig.CreatePlannedProtected(options, saved.Capture.ProtectedInput)
		})
	if err != nil {
		return engineRuntime.RunRequest{}, nil, captureDir, err
	}
	containerArgs := append([]string{"/sqlrs/helper/capture.sh", "liquibase"}, request.Args...)
	mounts := append([]engineRuntime.Mount(nil), request.Mounts...)
	mounts = append(mounts,
		engineRuntime.Mount{HostPath: helperPath, ContainerPath: "/sqlrs/helper/capture.sh", ReadOnly: true})
	env := map[string]string{
		"SQLRS_CAPTURE_DIR":       "/sqlrs/capture",
		"SQLRS_CAPTURE_MAX_BYTES": strconv.FormatInt(planningCaptureMaxBytes, 10),
		"SQLRS_JOB_ID":            saved.Attempt.JobID,
		"SQLRS_TASK_ID":           "plan",
		"SQLRS_ATTEMPT_NO":        strconv.Itoa(saved.Attempt.AttemptNo),
		"SQLRS_EXECUTION_DIGEST":  saved.Attempt.ExecutionDigest,
		"SQLRS_HELPER_DIGEST":     saved.Capture.HelperDigest,
	}
	for key, value := range request.Env {
		if _, reserved := env[key]; reserved || strings.TrimSpace(key) == "" {
			return engineRuntime.RunRequest{}, protected, captureDir, fmt.Errorf("reserved planning helper environment")
		}
		env[key] = value
	}
	return engineRuntime.RunRequest{
		ImageID: request.ImageID, EntryPoint: "/bin/sh", User: "0",
		Name: saved.Attempt.PlannedExecutorName, Args: containerArgs, Env: env,
		Dir: request.WorkDir, Network: request.Network, Mounts: mounts,
	}, protected, captureDir, nil
}
