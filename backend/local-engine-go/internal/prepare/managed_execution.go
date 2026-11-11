package prepare

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/sqlrs/engine-local/internal/dbms"
	"github.com/sqlrs/engine-local/internal/instanceaccess"
	"github.com/sqlrs/engine-local/internal/managedidentity"
	"github.com/sqlrs/engine-local/internal/prepare/queue"
	"github.com/sqlrs/engine-local/internal/preparecost"
	engineRuntime "github.com/sqlrs/engine-local/internal/runtime"
	"github.com/sqlrs/engine-local/internal/statefs"
	"github.com/sqlrs/engine-local/internal/store"
)

func managedOperationRef(parts ...string) string {
	h := sha256.New()
	for _, part := range parts {
		h.Write([]byte(part))
		h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))
}

// startManagedRuntime uses persisted assignment, never PG_VERSION alone, as
// provenance. Partial/unowned targets fail closed without destructive reset.
// See managed-database-identity-internals.md, atomicity and recovery.
func (e *taskExecutor) startManagedRuntime(ctx context.Context, jobID string, prepared preparedRequest, input *TaskInput) (*jobRuntime, *ErrorResponse) {
	return e.startManagedRuntimeWithPlan(ctx, jobID, prepared, input, nil)
}

// startManagedRuntimeWithPlan honors the already committed clone/name intent.
// The nil-plan path retains the existing psql behavior. A tracked start failure
// retains its files for exact reconciliation instead of deleting a possible DB.
func (e *taskExecutor) startManagedRuntimeWithPlan(ctx context.Context, jobID string, prepared preparedRequest, input *TaskInput, plan *queue.ClonePlan) (*jobRuntime, *ErrorResponse) {
	child, finish := preparecost.Start(ctx, preparecost.DBStartup)
	outcome := preparecost.Panicked
	defer func() { finish(outcome) }()
	owned, problem := e.observeStartManagedRuntimeWithPlan(child, jobID, prepared, input, plan)
	outcome = preparecost.Succeeded
	if problem != nil {
		outcome = preparecost.Failed
		if problem.Code == "cancelled" {
			outcome = preparecost.Cancelled
		}
	}
	return owned, problem
}

// observeStartManagedRuntimeWithPlan retains the original ownership/error
// ordering; subintervals describe actual calls, not inferred Docker internals.
func (e *taskExecutor) observeStartManagedRuntimeWithPlan(ctx context.Context, jobID string, prepared preparedRequest, input *TaskInput, plan *queue.ClonePlan) (*jobRuntime, *ErrorResponse) {
	m := e.m
	fail := func(err error) (*jobRuntime, *ErrorResponse) {
		return nil, errorResponse("internal_error", "managed runtime unavailable", err.Error())
	}
	if ctx.Err() != nil {
		return nil, errorResponse("cancelled", "job cancelled", "")
	}
	if input == nil || prepared.managed.Validate() != nil {
		return fail(instanceaccess.ErrInvalid)
	}
	rt, ok := m.runtime.(engineRuntime.ManagedRuntime)
	if !ok {
		return fail(instanceaccess.ErrUnavailable)
	}
	_ = preparecost.ObserveCall(ctx, preparecost.DBLockWait, func(context.Context) error { m.managedMu.Lock(); return nil })
	defer m.managedMu.Unlock()
	image := prepared.effectiveImageID()
	paths, err := resolveStatePaths(m.managedRoot, image, "", m.statefs)
	if err != nil {
		return fail(err)
	}
	var source string
	var bootstrap instanceaccess.SecretBinding
	err = preparecost.ObserveCall(ctx, preparecost.DBSource, func(ctx context.Context) error {
		switch input.Kind {
		case "image":
			if input.ID != prepared.managed.Key {
				return instanceaccess.ErrConflict
			}
			source = filepath.Join(paths.baseDir, prepared.managed.Key)
			info, statErr := os.Lstat(source)
			if statErr != nil && !os.IsNotExist(statErr) {
				return statErr
			}
			if statErr == nil && (!info.IsDir() || info.Mode()&os.ModeSymlink != 0) {
				return instanceaccess.ErrConflict
			}
			op, err := m.access.BaseOperation(ctx, prepared.managed.Key, source, prepared.managed.IdentityBinding, statErr == nil)
			if err != nil {
				return err
			}
			bootstrap, _, err = m.access.Bootstrap(ctx, prepared.managed.IdentityBinding, op.Stage == "assigned")
			if err != nil {
				return err
			}
			if op.Stage != "ready" {
				if _, err := os.Lstat(source); err == nil || !os.IsNotExist(err) {
					return instanceaccess.ErrConflict
				}
				if err := m.statefs.EnsureBaseDir(ctx, source); err != nil {
					return err
				}
				if err := preparecost.ObserveCall(ctx, preparecost.DBBaseInit, func(child context.Context) error {
					return rt.InitManagedBase(child, engineRuntime.ManagedInitRequest{ImageID: image, DataDir: source, Identity: prepared.managed.IdentityBinding, Bootstrap: bootstrap})
				}); err != nil {
					return err
				}
				if err := m.access.BaseReady(ctx, op); err != nil {
					return err
				}
			}
		case "state":
			entry, found, err := m.loadManagedState(ctx, input.ID, prepared)
			if err != nil {
				return err
			}
			if !found {
				return instanceaccess.ErrNotFound
			}
			paths, err := resolveStatePaths(m.managedRoot, entry.ImageID, input.ID, m.statefs)
			if err != nil {
				return err
			}
			source = paths.stateDir
			bootstrap, _, err = m.access.Bootstrap(ctx, prepared.managed.IdentityBinding, false)
			if err != nil {
				return err
			}
		default:
			return instanceaccess.ErrInvalid
		}
		return nil
	})
	if err != nil {
		return fail(err)
	}
	opRef := "runtime_" + managedOperationRef(m.stateStoreRoot, jobID, input.Kind, input.ID)
	target := filepath.Join(m.stateStoreRoot, "jobs", jobID, opRef)
	if plan != nil {
		if plan.Ref != filepath.ToSlash(filepath.Join("jobs", jobID, opRef)) || plan.ID == "" || plan.PlannedRuntimeName == "" || plan.SourceKind != input.Kind || input.Kind == "image" && plan.SourceImageID != input.ID || input.Kind == "state" && plan.SourceStateID != input.ID {
			return fail(instanceaccess.ErrConflict)
		}
	}
	op, err := preparecost.Observe(ctx, preparecost.DBAssign, func(child context.Context) (instanceaccess.Operation, error) {
		return m.access.Assign(child, opRef, target, input.ID, prepared.managed.IdentityBinding)
	})
	if err != nil {
		return fail(err)
	}
	if op.Stage != "assigned" {
		return fail(instanceaccess.ErrConflict)
	}
	if _, err := os.Lstat(target); err == nil || !os.IsNotExist(err) {
		return fail(instanceaccess.ErrConflict)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0700); err != nil {
		return fail(err)
	}
	clone, err := preparecost.Observe(ctx, preparecost.DBClone, func(child context.Context) (statefs.CloneResult, error) {
		return m.statefs.Clone(child, source, target)
	})
	if err != nil {
		return fail(err)
	}
	mount, err := scriptMountForFiles(prepared.filePaths)
	if err != nil {
		_ = clone.Cleanup()
		return fail(err)
	}
	runtimeName := "sqlrs-managed-" + op.PhysicalIdentity
	if plan != nil {
		runtimeName = plan.PlannedRuntimeName
	}
	instance, err := preparecost.Observe(ctx, preparecost.DBStart, func(child context.Context) (engineRuntime.Instance, error) {
		return rt.StartManaged(child, engineRuntime.ManagedStartRequest{StartRequest: engineRuntime.StartRequest{ImageID: image, DataDir: clone.MountDir, Name: runtimeName, Mounts: runtimeMountsFrom(mount)}, Identity: prepared.managed.IdentityBinding, PhysicalIdentity: op.PhysicalIdentity})
	})
	if err != nil {
		if plan == nil {
			_ = clone.Cleanup()
		}
		return fail(err)
	}
	owned := &jobRuntime{instance: instance, dataDir: clone.MountDir, runtimeDir: target, cleanup: clone.Cleanup, scriptMount: mount, operation: op, bootstrap: &bootstrap}
	if input.Kind == "state" {
		owned.stateID = input.ID
	}
	if err := preparecost.ObserveCall(ctx, preparecost.DBAttach, func(child context.Context) error { return m.access.Attach(child, op, instance.Binding) }); err != nil {
		_ = m.runtime.Stop(context.WithoutCancel(ctx), instance.ID)
		return fail(err)
	}
	if err := preparecost.ObserveCall(ctx, preparecost.DBVerifyStartup, func(child context.Context) error { return m.verifyManagedRuntime(child, owned) }); err != nil {
		_ = m.runtime.Stop(context.WithoutCancel(ctx), instance.ID)
		_ = m.access.RetireOperation(context.WithoutCancel(ctx), op)
		return fail(err)
	}
	return owned, nil
}

func (m *PrepareService) managedConnection(ctx context.Context, rt *jobRuntime) (dbms.ManagedConnection, error) {
	_, secret, err := m.access.Bootstrap(ctx, rt.instance.Binding.IdentityBinding, false)
	if err != nil {
		return dbms.ManagedConnection{}, err
	}
	return dbms.ManagedConnection{Binding: rt.instance.Binding, Host: rt.instance.Host, Port: uint16(rt.instance.Port), Password: secret.Password}, nil
}

func (m *PrepareService) verifyManagedRuntime(ctx context.Context, rt *jobRuntime) error {
	connection, err := m.managedConnection(ctx, rt)
	if err != nil {
		return err
	}
	proof, err := dbms.VerifyManagedAccess(ctx, connection)
	if err != nil {
		return err
	}
	if proof.Binding != rt.instance.Binding {
		return instanceaccess.ErrConflict
	}
	return nil
}

func (e *taskExecutor) createManagedInstance(ctx context.Context, jobID string, prepared preparedRequest, stateID string) (*Result, *ErrorResponse) {
	m := e.m
	fail := func(err error) (*Result, *ErrorResponse) {
		return nil, errorResponse("internal_error", "managed instance publication failed", err.Error())
	}
	if stateID == "" {
		return fail(instanceaccess.ErrInvalid)
	}
	instanceID := managedOperationRef("instance", jobID)[:32]
	// Stable instance reference makes a retried final publication idempotent.
	if entry, found, err := m.store.GetInstance(ctx, instanceID); err != nil {
		return fail(err)
	} else if found {
		if entry.StateID != stateID {
			return fail(instanceaccess.ErrConflict)
		}
		result := Result{InstanceID: instanceID, StateID: stateID, ImageID: prepared.effectiveImageID(), PrepareKind: prepared.request.PrepareKind, PrepareArgsNormalized: prepared.argsNormalized}
		if err := m.hydrateManagedResult(ctx, &result); err != nil {
			return fail(err)
		}
		return &result, nil
	}
	runner, ephemeral := m.runnerForJob(jobID)
	if ephemeral {
		defer m.cleanupRuntime(context.Background(), runner)
	}
	rt := runner.getRuntime()
	if rt == nil {
		binding, op, err := m.access.ResumePublication(ctx, instanceID)
		if err == nil {
			// Only this job's durable physical assignment may resume activation.
			if filepath.Dir(op.Target) != filepath.Join(m.stateStoreRoot, "jobs", jobID) || op.Identity != prepared.managed.IdentityBinding {
				return fail(instanceaccess.ErrConflict)
			}
			inspector, ok := m.runtime.(interface {
				InspectManaged(context.Context, managedidentity.RuntimeBinding) (engineRuntime.Instance, error)
			})
			if !ok {
				return fail(instanceaccess.ErrUnavailable)
			}
			instance, err := inspector.InspectManaged(ctx, binding.RuntimeBinding)
			if err != nil {
				return fail(err)
			}
			rt = &jobRuntime{stateID: stateID, instance: instance, runtimeDir: op.Target, operation: op, cleanup: func() error { return m.statefs.RemovePath(context.Background(), op.Target) }}
			runner.setRuntime(rt)
		} else if !errors.Is(err, instanceaccess.ErrNotFound) {
			return fail(err)
		}
	}
	if rt != nil && rt.stateID != stateID {
		if err := m.cleanupRuntime(context.WithoutCancel(ctx), runner); err != nil {
			return fail(err)
		}
		rt = nil
	}
	if rt == nil {
		var resp *ErrorResponse
		rt, resp = e.startManagedRuntime(ctx, jobID, prepared, &TaskInput{Kind: "state", ID: stateID})
		if resp != nil {
			return nil, resp
		}
		runner.setRuntime(rt)
	}
	connection, err := m.managedConnection(ctx, rt)
	if err != nil {
		return fail(err)
	}
	var dsn string
	err = m.access.Activate(ctx, instanceID, rt.instance.Binding, func(secret instanceaccess.Secret) error {
		if err := dbms.EnsureManagedPassword(ctx, connection, secret.Password); err != nil {
			return err
		}
		dsn = managedDSN(rt.instance, secret.Password)
		return nil
	}, func() error {
		status := store.InstanceStatusActive
		return m.store.CreateInstance(ctx, store.InstanceCreate{InstanceID: instanceID, StateID: stateID, ImageID: prepared.effectiveImageID(), CreatedAt: m.now().UTC().Format(time.RFC3339Nano), RuntimeID: strPtr(rt.instance.ID), RuntimeDir: strPtr(rt.runtimeDir), Status: &status})
	})
	if err != nil {
		return fail(err)
	}
	return &Result{DSN: dsn, InstanceID: instanceID, StateID: stateID, ImageID: prepared.effectiveImageID(), PrepareKind: prepared.request.PrepareKind, PrepareArgsNormalized: prepared.argsNormalized}, nil
}

func managedDSN(instance engineRuntime.Instance, password string) string {
	return (&url.URL{Scheme: "postgres", User: url.UserPassword(instance.Binding.Username, password), Host: net.JoinHostPort(instance.Host, strconv.Itoa(instance.Port)), Path: "/postgres"}).String()
}

// hydrateManagedResult is called only from authorized result reads. Persisted
// result/event projections retain instance metadata but never the credential.
func (m *PrepareService) hydrateManagedResult(ctx context.Context, result *Result) error {
	binding, err := m.access.Lookup(ctx, result.InstanceID)
	if err != nil {
		return err
	}
	inspector, ok := m.runtime.(interface {
		InspectManaged(context.Context, managedidentity.RuntimeBinding) (engineRuntime.Instance, error)
	})
	if !ok {
		return instanceaccess.ErrUnavailable
	}
	instance, err := inspector.InspectManaged(ctx, binding.RuntimeBinding)
	if err != nil {
		return err
	}
	secret, err := m.access.Resolve(ctx, result.InstanceID, binding.RuntimeBinding)
	if err != nil {
		return err
	}
	result.DSN = managedDSN(instance, secret.Password)
	return nil
}
