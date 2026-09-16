package run

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/sqlrs/engine-local/internal/instanceaccess"
	"github.com/sqlrs/engine-local/internal/registry"
	engineRuntime "github.com/sqlrs/engine-local/internal/runtime"
	"github.com/sqlrs/engine-local/internal/store"
)

const (
	kindPsql    = "psql"
	kindPgbench = "pgbench"
)

type Options struct {
	Access   *instanceaccess.Service
	Registry *registry.Registry
	Runtime  engineRuntime.Runtime
}

type Manager struct {
	access   *instanceaccess.Service
	registry *registry.Registry
	runtime  engineRuntime.Runtime
}

type Request struct {
	InstanceRef string   `json:"instance_ref"`
	Kind        string   `json:"kind"`
	Command     *string  `json:"command,omitempty"`
	Args        []string `json:"args"`
	Stdin       *string  `json:"stdin,omitempty"`
	Steps       []Step   `json:"steps,omitempty"`
}

type Step struct {
	Args  []string `json:"args"`
	Stdin *string  `json:"stdin,omitempty"`
}

type Result struct {
	InstanceID string
	Stdout     string
	Stderr     string
	ExitCode   int
	Events     []Event
}

func NewManager(opts Options) (*Manager, error) {
	if opts.Registry == nil {
		return nil, fmt.Errorf("registry is required")
	}
	if opts.Runtime == nil {
		return nil, fmt.Errorf("runtime is required")
	}
	return &Manager{
		access:   opts.Access,
		registry: opts.Registry,
		runtime:  opts.Runtime,
	}, nil
}

func (m *Manager) Run(ctx context.Context, req Request) (Result, error) {
	if strings.TrimSpace(req.InstanceRef) == "" {
		return Result{}, ValidationError{Message: "instance_ref is required"}
	}
	kind := strings.ToLower(strings.TrimSpace(req.Kind))
	if !isKnownKind(kind) {
		return Result{}, ValidationError{Message: "unknown run kind"}
	}
	entry, ok, _, err := m.registry.GetInstance(ctx, req.InstanceRef)
	if err != nil {
		return Result{}, err
	}
	if !ok {
		return Result{}, NotFoundError{Message: "instance not found"}
	}
	runtimeID := ""
	if entry.RuntimeID != nil {
		runtimeID = strings.TrimSpace(*entry.RuntimeID)
	}
	if runtimeID == "" {
		return Result{}, ConflictError{Message: "instance runtime id is missing"}
	}
	events := []Event{}

	args := append([]string{}, req.Args...)
	if kind == kindPsql && hasPsqlConnectionArgs(args) {
		return Result{}, ConflictError{Message: "conflicting psql connection arguments"}
	}
	if kind == kindPgbench && hasPgbenchConnectionArgs(args) {
		return Result{}, ConflictError{Message: "conflicting pgbench connection arguments"}
	}

	command := ""
	if req.Command != nil {
		command = strings.TrimSpace(*req.Command)
	}
	if command == "" {
		command = defaultCommand(kind)
	}

	if len(req.Steps) > 0 {
		if kind != kindPsql {
			return Result{}, ValidationError{Message: "steps are only supported for run:psql"}
		}
		if len(args) > 0 {
			return Result{}, ValidationError{Message: "steps cannot be combined with args"}
		}
		if req.Stdin != nil {
			return Result{}, ValidationError{Message: "steps cannot be combined with stdin"}
		}

		var output strings.Builder
		for _, step := range req.Steps {
			stepArgs := append([]string{}, step.Args...)
			if hasPsqlConnectionArgs(stepArgs) {
				return Result{}, ConflictError{Message: "conflicting psql connection arguments"}
			}
			execArgs := buildExecArgs(kind, command, stepArgs)
			stepOutput, err := m.execWithRecovery(ctx, entry, &runtimeID, execArgs, step.Stdin, &events)
			if err != nil {
				return Result{}, fmt.Errorf("exec failed: %w", err)
			}
			output.WriteString(stepOutput)
		}
		return Result{
			InstanceID: entry.InstanceID,
			Stdout:     output.String(),
			ExitCode:   0,
			Events:     events,
		}, nil
	}

	execArgs := buildExecArgs(kind, command, args)
	output, err := m.execWithRecovery(ctx, entry, &runtimeID, execArgs, req.Stdin, &events)
	if err != nil {
		return Result{}, fmt.Errorf("exec failed: %w", err)
	}

	return Result{
		InstanceID: entry.InstanceID,
		Stdout:     output,
		ExitCode:   0,
		Events:     events,
	}, nil
}

func (m *Manager) execWithRecovery(ctx context.Context, entry store.InstanceEntry, runtimeID *string, args []string, stdin *string, events *[]Event) (string, error) {
	if m.access != nil {
		return m.execManaged(ctx, entry, *runtimeID, args, stdin)
	}
	output, err := m.runtime.Exec(ctx, *runtimeID, engineRuntime.ExecRequest{
		User:  "postgres",
		Args:  args,
		Stdin: stdin,
	})
	if err == nil {
		return output, nil
	}
	if !isContainerMissing(err) {
		return output, err
	}
	newRuntimeID, err := m.recreateContainer(ctx, entry, events)
	if err != nil {
		return output, err
	}
	*runtimeID = newRuntimeID
	return m.runtime.Exec(ctx, *runtimeID, engineRuntime.ExecRequest{
		User:  "postgres",
		Args:  args,
		Stdin: stdin,
	})
}

func (m *Manager) recreateContainer(ctx context.Context, entry store.InstanceEntry, events *[]Event) (string, error) {
	appendLogEvent(events, "run: container missing - recreating")
	runtimeDir := ""
	if entry.RuntimeDir != nil {
		runtimeDir = strings.TrimSpace(*entry.RuntimeDir)
	}
	if runtimeDir == "" {
		return "", ConflictError{Message: "instance runtime_dir is missing"}
	}
	info, err := os.Stat(runtimeDir)
	if err != nil || !info.IsDir() {
		return "", fmt.Errorf("runtime_dir is missing: %w", err)
	}
	dataDir := runtimeDir
	merged := filepath.Join(runtimeDir, "merged")
	if info, err := os.Stat(merged); err == nil && info.IsDir() {
		dataDir = merged
	}
	appendLogEvent(events, "run: restoring runtime")
	instance, err := m.runtime.Start(ctx, engineRuntime.StartRequest{
		ImageID:     entry.ImageID,
		DataDir:     dataDir,
		Name:        "sqlrs-run-" + entry.InstanceID,
		AllowInitdb: false,
	})
	if err != nil {
		return "", fmt.Errorf("runtime start failed: %w", err)
	}
	appendLogEvent(events, "run: container started")
	if err := m.registry.UpdateInstanceRuntime(ctx, entry.InstanceID, strPtr(instance.ID)); err != nil {
		_ = m.runtime.Stop(ctx, instance.ID)
		return "", err
	}
	return instance.ID, nil
}

func appendLogEvent(events *[]Event, message string) {
	if events == nil || strings.TrimSpace(message) == "" {
		return
	}
	*events = append(*events, Event{
		Type: "log",
		Ts:   time.Now().UTC().Format(time.RFC3339Nano),
		Data: message,
	})
}

func isContainerMissing(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "no such container") {
		return true
	}
	if strings.Contains(msg, "container") && strings.Contains(msg, "is not running") {
		return true
	}
	return false
}

func strPtr(value string) *string {
	return &value
}

func isKnownKind(kind string) bool {
	switch kind {
	case kindPsql, kindPgbench:
		return true
	default:
		return false
	}
}

func defaultCommand(kind string) string {
	if kind == kindPsql {
		return "psql"
	}
	return "pgbench"
}

func buildExecArgs(kind string, command string, args []string) []string {
	execArgs := []string{command}
	if kind == kindPsql {
		execArgs = append(execArgs, args...)
		execArgs = append(execArgs, defaultDSN())
		return execArgs
	}
	execArgs = append(execArgs, "-h", "127.0.0.1", "-p", "5432", "-U", "sqlrs", "-d", "postgres")
	execArgs = append(execArgs, args...)
	return execArgs
}

func defaultDSN() string {
	return "postgres://sqlrs@127.0.0.1:5432/postgres"
}
