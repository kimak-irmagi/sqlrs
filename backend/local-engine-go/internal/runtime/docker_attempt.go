package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

var createdContainerID = regexp.MustCompile(`^[0-9a-f]{64}$`)
var plannedContainerName = regexp.MustCompile(`^sqlrs-(lb|step)-[a-zA-Z0-9_.-]+$`)
var plannedDatabaseContainerName = regexp.MustCompile(`^sqlrs-db-[a-zA-Z0-9_.-]+$`)

// ContainerInspection is a verified Docker view of one saved transformation
// attempt, used only to prove non-start or to block unsafe replay. See A02
// in docs/architecture/transformation-attempt-recovery-tests.md.
type ContainerInspection struct {
	ID                                                                   string
	Status                                                               string
	ExitCode                                                             int
	SessionID, RunnerConfigDigest, WorkerGeneration, CaptureHelperDigest string
}

// InspectNamedContainer lists by exact planned name and then validates the
// full inspect identity. An unavailable Docker daemon is an error, not proof
// that the container does not exist.
func (r *DockerRuntime) InspectNamedContainer(ctx context.Context, name string) (ContainerInspection, bool, error) {
	if !plannedContainerName.MatchString(name) {
		return ContainerInspection{}, false, fmt.Errorf("invalid planned container name")
	}
	return r.inspectExactNamedContainer(ctx, name)
}

// InspectNamedDatabaseContainer verifies the journal's separate DB runtime
// name. Session snapshots require this container to remain present while its
// PostgreSQL process is stopped. Executor-name validation stays unchanged.
func (r *DockerRuntime) InspectNamedDatabaseContainer(ctx context.Context, name string) (ContainerInspection, bool, error) {
	if !plannedDatabaseContainerName.MatchString(name) {
		return ContainerInspection{}, false, fmt.Errorf("invalid planned database container name")
	}
	return r.inspectExactNamedContainer(ctx, name)
}

func (r *DockerRuntime) inspectExactNamedContainer(ctx context.Context, name string) (ContainerInspection, bool, error) {
	output, err := r.run(ctx, []string{"container", "ls", "-a", "--no-trunc", "--filter", "name=^/" + name + "$", "--format", "{{.ID}}"}, nil)
	if err != nil {
		return ContainerInspection{}, false, fmt.Errorf("docker container list failed: %w", err)
	}
	id := strings.TrimSpace(output)
	if id == "" {
		return ContainerInspection{}, false, nil
	}
	if !createdContainerID.MatchString(id) {
		return ContainerInspection{}, false, fmt.Errorf("docker container list returned an invalid ID")
	}
	output, err = r.run(ctx, []string{"container", "inspect", "--format", "{{json .}}", id}, nil)
	if err != nil {
		return ContainerInspection{}, false, fmt.Errorf("docker container inspect failed: %w", err)
	}
	inspection, err := decodeContainerInspection(output, id, name)
	return inspection, err == nil, err
}

// inspectPresentContainer observes an already attributed allocation in one
// command. Unlike name discovery it never treats an error as absence. Callers
// still check their required phase before/after each protected operation.
// O1/CT02: docs/architecture/issue89-cost-attribution-and-trimming.md §2.
func (r *DockerRuntime) inspectPresentContainer(ctx context.Context, name, id string) (ContainerInspection, error) {
	if r == nil || !plannedContainerName.MatchString(name) || !createdContainerID.MatchString(id) {
		return ContainerInspection{}, fmt.Errorf("invalid saved container identity")
	}
	if err := ctx.Err(); err != nil {
		return ContainerInspection{}, err
	}
	output, err := r.run(ctx, []string{"container", "inspect", "--format", "{{json .}}", id}, nil)
	if contextErr := ctx.Err(); contextErr != nil {
		return ContainerInspection{}, contextErr
	}
	if err != nil {
		return ContainerInspection{}, fmt.Errorf("docker container inspect failed: %w", err)
	}
	return decodeContainerInspection(output, id, name)
}

// decodeContainerInspection keeps discovery and known-ID observations on the
// same native identity/label decoder. No cached view grants a later authority.
func decodeContainerInspection(output, id, name string) (ContainerInspection, error) {
	var details struct {
		ID     string `json:"Id"`
		Name   string `json:"Name"`
		Config struct {
			Labels map[string]string `json:"Labels"`
		} `json:"Config"`
		State struct {
			Status   string `json:"Status"`
			ExitCode int    `json:"ExitCode"`
		} `json:"State"`
	}
	if err := json.Unmarshal([]byte(strings.TrimSpace(output)), &details); err != nil {
		return ContainerInspection{}, fmt.Errorf("docker container inspect returned invalid JSON: %w", err)
	}
	if details.ID != id || details.Name != "/"+name || details.State.Status == "" {
		return ContainerInspection{}, fmt.Errorf("docker container identity or state does not match saved attempt")
	}
	return ContainerInspection{ID: id, Status: details.State.Status, ExitCode: details.State.ExitCode,
		SessionID: details.Config.Labels["sqlrs.liquibase.session"], RunnerConfigDigest: details.Config.Labels["sqlrs.liquibase.config"],
		WorkerGeneration: details.Config.Labels["sqlrs.liquibase.generation"], CaptureHelperDigest: details.Config.Labels["sqlrs.liquibase.helper"]}, nil
}

// CreateContainer creates, but never starts, a named container. Its returned
// ID must be committed to the attempt journal before StartContainer. See the
// ordering in docs/architecture/liquibase-container-storage-schema.md.
func (r *DockerRuntime) CreateContainer(ctx context.Context, req RunRequest) (string, error) {
	if strings.TrimSpace(req.ImageID) == "" || strings.TrimSpace(req.Name) == "" {
		return "", fmt.Errorf("image and planned container name are required")
	}
	args := []string{"create", "--name", req.Name}
	keys := make([]string, 0, len(req.Labels))
	for key := range req.Labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		args = append(args, "--label", key+"="+req.Labels[key])
	}
	if req.EntryPoint != "" {
		if req.EntryPoint != "/bin/sh" {
			return "", fmt.Errorf("unsupported planned executor entrypoint")
		}
		args = append(args, "--entrypoint", req.EntryPoint)
	}
	if req.User != "" {
		args = append(args, "-u", req.User)
	}
	if req.Dir != "" {
		args = append(args, "-w", req.Dir)
	}
	if req.Network != "" {
		args = append(args, "--network", req.Network)
	}
	for key, value := range req.Env {
		if strings.TrimSpace(key) != "" {
			args = append(args, "-e", key+"="+value)
		}
	}
	for _, mount := range req.Mounts {
		if mount.HostPath != "" && mount.ContainerPath != "" {
			args = append(args, "-v", dockerBindSpec(mount.HostPath, mount.ContainerPath, mount.ReadOnly))
		}
	}
	args = append(args, req.ImageID)
	args = append(args, req.Args...)
	output, err := r.run(ctx, args, nil)
	if err != nil {
		return "", fmt.Errorf("docker create failed: %w", err)
	}
	id := strings.TrimSpace(output)
	if !createdContainerID.MatchString(id) {
		return "", fmt.Errorf("docker create returned an invalid container ID")
	}
	return id, nil
}

// StartContainer returns after Docker has requested start, not after command
// completion. The caller records its running transition before waiting.
func (r *DockerRuntime) StartContainer(ctx context.Context, id string) error {
	if !createdContainerID.MatchString(id) {
		return fmt.Errorf("invalid container ID")
	}
	if _, err := r.run(ctx, []string{"start", id}, nil); err != nil {
		return fmt.Errorf("docker start failed: %w", err)
	}
	return nil
}

// SignalContainer requests immediate termination of one already inspected,
// exact Docker ID. A lost or altered Docker response is not acknowledgement;
// the coordinator must inspect the same ID again before any replay. See R04e–g
// in docs/architecture/liquibase-planning-recovery-tests.md.
func (r *DockerRuntime) SignalContainer(ctx context.Context, id string) error {
	if !createdContainerID.MatchString(id) {
		return fmt.Errorf("invalid container ID")
	}
	output, err := r.run(ctx, []string{"kill", "--signal=SIGKILL", id}, nil)
	if err != nil {
		return fmt.Errorf("docker signal failed: %w", err)
	}
	if strings.TrimSpace(output) != id {
		return fmt.Errorf("docker signal acknowledgement did not match exact ID")
	}
	return nil
}

// WaitContainer returns best-effort logs and a proved Docker exit code. A
// wait transport error leaves the attempt open; diagnostic log loss after a
// valid wait does not erase the exit proof. See A03 in the recovery tests.
func (r *DockerRuntime) WaitContainer(ctx context.Context, id string) (string, int, error) {
	if !createdContainerID.MatchString(id) {
		return "", 0, fmt.Errorf("invalid container ID")
	}
	output, err := r.run(ctx, []string{"wait", id}, nil)
	if err != nil {
		return "", 0, fmt.Errorf("docker wait failed: %w", err)
	}
	exitCode, err := strconv.Atoi(strings.TrimSpace(output))
	if err != nil || exitCode < 0 || exitCode > 255 {
		return "", 0, fmt.Errorf("docker wait returned an invalid exit code")
	}
	logs, err := r.run(ctx, []string{"logs", id}, nil)
	if err != nil {
		// Logs are diagnostic only. Docker wait has already proved the exit;
		// discarding it would strand a recoverable database-changing attempt.
		return "", exitCode, nil
	}
	return logs, exitCode, nil
}

// RemoveStoppedContainer does not force removal; a still-running container
// must remain inspectable to recovery instead of being silently destroyed.
func (r *DockerRuntime) RemoveStoppedContainer(ctx context.Context, id string) error {
	if !createdContainerID.MatchString(id) {
		return fmt.Errorf("invalid container ID")
	}
	if _, err := r.run(ctx, []string{"rm", id}, nil); err != nil {
		return fmt.Errorf("docker remove failed: %w", err)
	}
	return nil
}
