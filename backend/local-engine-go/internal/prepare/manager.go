package prepare

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/sqlrs/engine-local/internal/config"
	"github.com/sqlrs/engine-local/internal/dbms"
	"github.com/sqlrs/engine-local/internal/deletion"
	"github.com/sqlrs/engine-local/internal/instanceaccess"
	"github.com/sqlrs/engine-local/internal/managedidentity"
	"github.com/sqlrs/engine-local/internal/prepare/queue"
	"github.com/sqlrs/engine-local/internal/runtime"
	"github.com/sqlrs/engine-local/internal/statefs"
	"github.com/sqlrs/engine-local/internal/store"
)

const (
	StatusQueued    = "queued"
	StatusRunning   = "running"
	StatusSucceeded = "succeeded"
	StatusFailed    = "failed"
)

type Options struct {
	Identity       *managedidentity.Owner
	Access         *instanceaccess.Service
	Store          store.Store
	Queue          queue.Store
	Runtime        runtime.Runtime
	StateFS        statefs.StateFS
	ValidateStore  func(root string) error
	DBMS           dbms.Connector
	StateStoreRoot string
	Config         config.Store
	Psql           psqlRunner
	Liquibase      liquibaseRunner
	Version        string
	Now            func() time.Time
	IDGen          func() (string, error)
	Async          bool
	HeartbeatEvery time.Duration
}

type PrepareService struct {
	access      *instanceaccess.Service
	managedMu   sync.Mutex
	managedRoot string
	// Populated by the coordinated managed-store startup integration.
	identity       *managedidentity.Owner
	store          store.Store
	queue          queue.Store
	runtime        runtime.Runtime
	statefs        statefs.StateFS
	dbms           dbms.Connector
	stateStoreRoot string
	config         config.Store
	psql           psqlRunner
	liquibase      liquibaseRunner
	version        string
	validateStore  func(root string) error
	now            func() time.Time
	idGen          func() (string, error)
	async          bool
	heartbeatEvery time.Duration
	lastEviction   *CacheEvictionSummary

	mu      sync.Mutex
	running map[string]*jobRunner
	events  *eventBus
	beats   map[string]*heartbeatState

	coordinator jobCoordinatorAPI
	executor    taskExecutorAPI
	snapshot    snapshotOrchestratorAPI
}

type jobRunner struct {
	cancel context.CancelFunc
	done   chan struct{}
	mu     sync.Mutex
	rt     *jobRuntime
}

type jobRuntime struct {
	operation   instanceaccess.Operation
	bootstrap   *instanceaccess.SecretBinding
	mu          sync.Mutex
	instance    runtime.Instance
	dataDir     string
	runtimeDir  string
	cleanup     func() error
	scriptMount *scriptMount
}

type preparedRequest struct {
	managed              managedidentity.BaseSelection
	request              Request
	normalizedArgs       []string
	argsNormalized       string
	filePaths            []string
	liquibaseMounts      []runtime.Mount
	resolvedImageID      string
	psqlInputs           []psqlInput
	psqlSteps            []psqlStep
	psqlWorkDir          string
	liquibaseLockPaths   []string
	liquibaseSearchPaths []string
	liquibaseWorkDir     string
}

func NewPrepareService(opts Options) (*PrepareService, error) {
	if opts.Store == nil {
		return nil, fmt.Errorf("store is required")
	}
	if opts.Queue == nil {
		return nil, fmt.Errorf("queue is required")
	}
	if opts.Runtime == nil {
		return nil, fmt.Errorf("runtime is required")
	}
	if opts.StateFS == nil {
		return nil, fmt.Errorf("statefs is required")
	}
	if opts.DBMS == nil {
		return nil, fmt.Errorf("dbms connector is required")
	}
	if strings.TrimSpace(opts.StateStoreRoot) == "" {
		return nil, fmt.Errorf("state store root is required")
	}
	now := opts.Now
	if now == nil {
		now = time.Now
	}
	idGen := opts.IDGen
	if idGen == nil {
		idGen = func() (string, error) {
			return randomHex(16)
		}
	}
	psql := opts.Psql
	if psql == nil {
		psql = containerPsqlRunner{runtime: opts.Runtime}
	}
	liquibase := opts.Liquibase
	if liquibase == nil {
		liquibase = hostLiquibaseRunner{}
	}
	validateStore := opts.ValidateStore
	if validateStore == nil {
		validateStore = func(root string) error {
			return opts.StateFS.Validate(root)
		}
	}
	m := &PrepareService{
		identity:       opts.Identity,
		access:         opts.Access,
		managedRoot:    opts.StateStoreRoot,
		store:          opts.Store,
		queue:          opts.Queue,
		runtime:        opts.Runtime,
		statefs:        opts.StateFS,
		dbms:           opts.DBMS,
		stateStoreRoot: opts.StateStoreRoot,
		config:         opts.Config,
		psql:           psql,
		liquibase:      liquibase,
		version:        opts.Version,
		validateStore:  validateStore,
		now:            now,
		idGen:          idGen,
		async:          opts.Async,
		heartbeatEvery: normalizeHeartbeat(opts.HeartbeatEvery),
		running:        map[string]*jobRunner{},
		events:         newEventBus(),
		beats:          map[string]*heartbeatState{},
	}
	m.snapshot = &snapshotOrchestrator{m: m}
	m.executor = &taskExecutor{m: m, snapshot: m.snapshot}
	m.coordinator = &jobCoordinator{m: m, executor: m.executor}
	return m, nil
}

func (m *PrepareService) Recover(ctx context.Context) error {
	if errResp := m.ensureCacheCapacity(ctx, "", "startup_recovery"); errResp != nil {
		m.logJob("", "startup cache check failed code=%s message=%s details=%s", errResp.Code, errResp.Message, summarizeLogDetails(errResp.Details))
	}
	jobs, err := m.queue.ListJobsByStatus(ctx, []string{StatusQueued, StatusRunning})
	if err != nil {
		return err
	}
	for _, job := range jobs {
		m.logJob(job.JobID, "recover status=%s", job.Status)
		prepared, err := m.prepareFromJob(job)
		if err != nil {
			errResp := errorResponse("internal_error", "cannot restore job request", err.Error())
			_ = m.failJob(job.JobID, errResp)
			continue
		}
		if m.async {
			go m.runJob(prepared, job.JobID)
		} else {
			m.runJob(prepared, job.JobID)
		}
	}
	return nil
}

func (m *PrepareService) Submit(ctx context.Context, req Request) (Accepted, error) {
	prepared, err := m.prepareRequest(req)
	if err != nil {
		return Accepted{}, err
	}
	jobID, err := m.idGen()
	if err != nil {
		return Accepted{}, err
	}
	if m.identity != nil {
		if errResp := m.ensureResolvedImageID(ctx, jobID, &prepared, nil); errResp != nil {
			return Accepted{}, errorFromExplainResponse(errResp)
		}
		if err := m.bindManagedRequest(ctx, &prepared, nil); err != nil {
			return Accepted{}, err
		}
	}
	now := m.now().UTC().Format(time.RFC3339Nano)
	reqJSON, err := json.Marshal(prepared.request)
	if err != nil {
		return Accepted{}, err
	}

	argsNormalized := prepared.argsNormalized
	job := queue.JobRecord{
		JobID:                 jobID,
		Status:                StatusQueued,
		PrepareKind:           prepared.request.PrepareKind,
		ImageID:               prepared.request.ImageID,
		PlanOnly:              prepared.request.PlanOnly,
		SnapshotMode:          "always",
		PrepareArgsNormalized: &argsNormalized,
		RequestJSON:           strPtr(string(reqJSON)),
		CreatedAt:             now,
		ResolvedImageID:       prepared.resolvedImageID,
		LineageRef:            prepared.managed.LineageRef,
		IdentityDigest:        prepared.managed.IdentityDigest,
	}
	if err := m.queue.CreateJob(ctx, job); err != nil {
		return Accepted{}, err
	}
	m.logJob(jobID, "created kind=%s image=%s plan_only=%t", prepared.request.PrepareKind, prepared.request.ImageID, prepared.request.PlanOnly)
	_ = m.appendEvent(jobID, Event{
		Type:   "status",
		Ts:     now,
		Status: StatusQueued,
	})

	if m.async {
		go m.runJob(prepared, jobID)
	} else {
		m.runJob(prepared, jobID)
	}

	base := "/v1/prepare-jobs/" + jobID
	return Accepted{
		JobID:     jobID,
		StatusURL: base,
		EventsURL: base + "/events",
		Status:    StatusQueued,
	}, nil
}

func (m *PrepareService) Get(jobID string) (Status, bool) {
	job, ok, err := m.queue.GetJob(context.Background(), jobID)
	if err != nil {
		m.logJob(jobID, "lookup failed error=%v", err)
		return Status{}, false
	}
	if !ok {
		m.logJob(jobID, "lookup missing")
		return Status{}, false
	}
	tasks, err := m.queue.ListTasks(context.Background(), jobID)
	if err != nil {
		m.logJob(jobID, "task list failed error=%v", err)
		tasks = nil
	}
	status := Status{
		JobID:                 job.JobID,
		Status:                job.Status,
		PrepareKind:           job.PrepareKind,
		ImageID:               job.ImageID,
		PlanOnly:              job.PlanOnly,
		PrepareArgsNormalized: valueOrEmpty(job.PrepareArgsNormalized),
		CreatedAt:             strPtr(job.CreatedAt),
		StartedAt:             job.StartedAt,
		FinishedAt:            job.FinishedAt,
		Tasks:                 planTasksFromRecords(tasks),
	}
	if job.ResultJSON != nil {
		var result Result
		if err := json.Unmarshal([]byte(*job.ResultJSON), &result); err == nil {
			if m.access == nil || m.hydrateManagedResult(context.Background(), &result) == nil {
				status.Result = &result
			}
		}
	}
	if job.ErrorJSON != nil {
		var errResp ErrorResponse
		if err := json.Unmarshal([]byte(*job.ErrorJSON), &errResp); err == nil {
			status.Error = &errResp
		}
	}
	return status, true
}

func (m *PrepareService) ListJobs(jobID string) []JobEntry {
	jobs, err := m.queue.ListJobs(context.Background(), jobID)
	if err != nil {
		return []JobEntry{}
	}
	resolvedByJobID := map[string]string{}
	if tasks, err := m.queue.ListTasks(context.Background(), jobID); err == nil {
		for _, task := range tasks {
			resolved := strings.TrimSpace(valueOrEmpty(task.ResolvedImageID))
			if resolved == "" {
				continue
			}
			if _, ok := resolvedByJobID[task.JobID]; !ok {
				resolvedByJobID[task.JobID] = resolved
			}
		}
	}
	entries := make([]JobEntry, 0, len(jobs))
	for _, job := range jobs {
		entry := JobEntry{
			JobID:                 job.JobID,
			Status:                job.Status,
			PrepareKind:           job.PrepareKind,
			ImageID:               job.ImageID,
			ResolvedImageID:       resolvedByJobID[job.JobID],
			PrepareArgsNormalized: valueOrEmpty(job.PrepareArgsNormalized),
			Signature:             valueOrEmpty(job.Signature),
			PlanOnly:              job.PlanOnly,
			CreatedAt:             strPtr(job.CreatedAt),
			StartedAt:             job.StartedAt,
			FinishedAt:            job.FinishedAt,
		}
		entries = append(entries, entry)
	}
	return entries
}

func (m *PrepareService) ListTasks(jobID string) []TaskEntry {
	tasks, err := m.queue.ListTasks(context.Background(), jobID)
	if err != nil || len(tasks) == 0 {
		return []TaskEntry{}
	}
	jobRecords, err := m.queue.ListJobs(context.Background(), jobID)
	if err != nil {
		jobRecords = nil
	}
	jobRequests := make(map[string]*Request, len(jobRecords))
	for _, job := range jobRecords {
		jobRequests[job.JobID] = decodeJobRequest(job)
	}
	entries := make([]TaskEntry, 0, len(tasks))
	for _, task := range tasks {
		entries = append(entries, taskEntryFromRecord(task, jobRequests[task.JobID]))
	}
	return entries
}

func (m *PrepareService) Delete(jobID string, opts deletion.DeleteOptions) (deletion.DeleteResult, bool) {
	_, ok, err := m.queue.GetJob(context.Background(), jobID)
	if err != nil || !ok {
		return deletion.DeleteResult{}, false
	}
	tasks, err := m.queue.ListTasks(context.Background(), jobID)
	if err != nil {
		return deletion.DeleteResult{}, false
	}

	blocked := hasRunningTasks(tasks)
	node := deletion.DeleteNode{
		Kind: "job",
		ID:   jobID,
	}
	if blocked && !opts.Force {
		m.logJob(jobID, "delete blocked active_tasks=true")
		node.Blocked = deletion.BlockActiveTasks
		return deletion.DeleteResult{
			DryRun:  opts.DryRun,
			Outcome: deletion.OutcomeBlocked,
			Root:    node,
		}, true
	}

	result := deletion.DeleteResult{
		DryRun:  opts.DryRun,
		Outcome: deletion.OutcomeDeleted,
		Root:    node,
	}
	if opts.DryRun {
		result.Outcome = deletion.OutcomeWouldDelete
		return result, true
	}

	if blocked && opts.Force {
		m.logJob(jobID, "delete force cancel")
		runner := m.getRunner(jobID)
		if runner != nil {
			runner.cancel()
			<-runner.done
		}
	}

	if err := m.queue.DeleteJob(context.Background(), jobID); err != nil {
		return deletion.DeleteResult{}, false
	}
	if err := m.removeJobDir(jobID); err != nil {
		m.logJob(jobID, "delete cleanup failed: %v", err)
		return deletion.DeleteResult{}, false
	}
	m.logJob(jobID, "deleted")
	return result, true
}

func (m *PrepareService) Cancel(jobID string) (Status, bool, bool, error) {
	job, ok, err := m.queue.GetJob(context.Background(), jobID)
	if err != nil {
		return Status{}, false, false, err
	}
	if !ok {
		return Status{}, false, false, nil
	}

	switch job.Status {
	case StatusSucceeded, StatusFailed:
		status, ok := m.Get(jobID)
		if !ok {
			return Status{}, true, false, nil
		}
		return status, true, false, nil
	}

	if runner := m.getRunner(jobID); runner != nil {
		m.logJob(jobID, "cancel requested")
		runner.cancel()
		status, ok := m.Get(jobID)
		if !ok {
			return Status{}, true, true, nil
		}
		return status, true, true, nil
	}

	m.logJob(jobID, "cancel requested without active runner")
	if err := m.failJob(jobID, errorResponse("cancelled", "job cancelled", "")); err != nil {
		return Status{}, true, false, err
	}
	status, ok := m.Get(jobID)
	if !ok {
		return Status{}, true, true, nil
	}
	return status, true, true, nil
}

func (m *PrepareService) EventsSince(jobID string, index int) ([]Event, bool, bool, error) {
	job, ok, err := m.queue.GetJob(context.Background(), jobID)
	if err != nil {
		return nil, false, false, err
	}
	if !ok {
		return nil, false, false, nil
	}
	events, err := m.queue.ListEventsSince(context.Background(), jobID, index)
	if err != nil {
		return nil, true, false, err
	}
	out := make([]Event, 0, len(events))
	for _, event := range events {
		decoded := eventFromRecord(event)
		if m.access != nil && decoded.Result != nil {
			if err := m.hydrateManagedResult(context.Background(), decoded.Result); err != nil {
				return nil, true, false, err
			}
		}
		out = append(out, decoded)
	}
	done := job.Status == StatusSucceeded || job.Status == StatusFailed
	return out, true, done, nil
}

func (m *PrepareService) WaitForEvent(ctx context.Context, jobID string, index int) error {
	ch := m.events.subscribe(jobID)
	defer m.events.unsubscribe(jobID, ch)
	for {
		count, err := m.queue.CountEvents(ctx, jobID)
		if err != nil {
			return err
		}
		if count > index {
			return nil
		}
		job, ok, err := m.queue.GetJob(ctx, jobID)
		if err != nil {
			return err
		}
		if !ok {
			return errJobNotFound
		}
		if job.Status == StatusSucceeded || job.Status == StatusFailed {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ch:
		}
	}
}

func (m *PrepareService) prepareFromJob(job queue.JobRecord) (preparedRequest, error) {
	if job.RequestJSON == nil {
		return preparedRequest{}, fmt.Errorf("request_json is empty")
	}
	var req Request
	if err := json.Unmarshal([]byte(*job.RequestJSON), &req); err != nil {
		return preparedRequest{}, err
	}
	prepared, err := m.prepareRequest(req)
	if err != nil {
		return preparedRequest{}, err
	}
	if m.identity != nil {
		prepared.resolvedImageID = job.ResolvedImageID
		if err := m.bindManagedRequest(context.Background(), &prepared, &job); err != nil {
			return preparedRequest{}, err
		}
	}
	return prepared, nil
}

func (c *jobCoordinator) runJob(prepared preparedRequest, jobID string) {
	m := c.m
	ctx, cancel := context.WithCancel(context.Background())
	runner := m.registerRunner(jobID, cancel)
	jobSucceeded := false
	defer func() {
		if !jobSucceeded {
			m.cleanupRuntime(ctx, runner)
		}
		close(runner.done)
		m.unregisterRunner(jobID)
	}()

	job, ok, err := m.queue.GetJob(context.Background(), jobID)
	if err != nil {
		_ = m.failJob(jobID, errorResponse("internal_error", "cannot load job", err.Error()))
		return
	}
	if !ok {
		return
	}
	if job.Status == StatusSucceeded || job.Status == StatusFailed {
		m.logJob(jobID, "skip execution for terminal job status=%s", job.Status)
		return
	}
	if ctx.Err() != nil {
		_ = m.failJob(jobID, errorResponse("cancelled", "job cancelled", ""))
		return
	}

	now := m.now().UTC()
	startedAt := now.Format(time.RFC3339Nano)
	_ = m.queue.UpdateJob(ctx, jobID, queue.JobUpdate{
		Status:    strPtr(StatusRunning),
		StartedAt: &startedAt,
	})
	_ = m.appendEvent(jobID, Event{
		Type:   "status",
		Ts:     startedAt,
		Status: StatusRunning,
	})
	_ = m.queue.UpdateJob(ctx, jobID, queue.JobUpdate{
		PrepareArgsNormalized: &prepared.argsNormalized,
	})
	m.logJob(jobID, "running")

	if err := m.validateStore(m.stateStoreRoot); err != nil {
		_ = m.failJob(jobID, errorResponse("internal_error", "state store not ready", err.Error()))
		return
	}

	tasks, stateID, errResp := c.loadOrPlanTasks(ctx, jobID, prepared)
	if errResp != nil {
		_ = m.failJob(jobID, errResp)
		return
	}

	if prepared.request.PlanOnly {
		if err := m.markTasksSucceeded(ctx, jobID, tasks); err != nil {
			_ = m.failJob(jobID, errorResponse("internal_error", "cannot update task status", err.Error()))
			return
		}
		if err := m.succeedPlan(jobID); err == nil {
			jobSucceeded = true
		}
		return
	}

	for _, task := range tasks {
		if ctx.Err() != nil {
			_ = m.failJob(jobID, errorResponse("cancelled", "job cancelled", ""))
			return
		}
		if task.Status == StatusSucceeded {
			continue
		}
		if task.Status == StatusFailed {
			_ = m.failJob(jobID, errorResponse("internal_error", "task failed", task.TaskID))
			return
		}
		if task.Type == "state_execute" && task.Cached != nil && *task.Cached && strings.TrimSpace(task.OutputStateID) != "" {
			cached, err := m.isManagedStateCached(task.OutputStateID, prepared)
			if err != nil {
				_ = m.failJob(jobID, errorResponse("internal_error", "cannot check state cache", err.Error()))
				return
			}
			if cached {
				stateID = task.OutputStateID
				if err := m.updateTaskStatus(ctx, jobID, task.TaskID, StatusSucceeded, nil, strPtr(m.now().UTC().Format(time.RFC3339Nano)), nil); err != nil {
					_ = m.failJob(jobID, errorResponse("internal_error", "cannot update task status", err.Error()))
					return
				}
				continue
			}
		}
		if err := m.updateTaskStatus(ctx, jobID, task.TaskID, StatusRunning, strPtr(m.now().UTC().Format(time.RFC3339Nano)), nil, nil); err != nil {
			_ = m.failJob(jobID, errorResponse("internal_error", "cannot update task status", err.Error()))
			return
		}
		switch task.Type {
		case "plan":
		case "resolve_image":
			if strings.TrimSpace(task.ResolvedImageID) != "" && strings.TrimSpace(prepared.resolvedImageID) == "" {
				prepared.resolvedImageID = task.ResolvedImageID
			}
			if errResp := m.ensureResolvedImageID(ctx, jobID, &prepared, nil); errResp != nil {
				_ = m.updateTaskStatus(ctx, jobID, task.TaskID, StatusFailed, nil, strPtr(m.now().UTC().Format(time.RFC3339Nano)), errResp)
				_ = m.failJob(jobID, errResp)
				return
			}
		case "state_execute":
			outputID, errResp := c.executor.executeStateTask(ctx, jobID, prepared, task)
			if errResp != nil {
				_ = m.updateTaskStatus(ctx, jobID, task.TaskID, StatusFailed, nil, strPtr(m.now().UTC().Format(time.RFC3339Nano)), errResp)
				_ = m.failJob(jobID, errResp)
				return
			}
			stateID = outputID
		case "prepare_instance":
			result, errResp := c.executor.createInstance(ctx, jobID, prepared, stateID)
			if errResp != nil {
				_ = m.updateTaskStatus(ctx, jobID, task.TaskID, StatusFailed, nil, strPtr(m.now().UTC().Format(time.RFC3339Nano)), errResp)
				_ = m.failJob(jobID, errResp)
				return
			}
			if err := m.updateTaskStatus(ctx, jobID, task.TaskID, StatusSucceeded, nil, strPtr(m.now().UTC().Format(time.RFC3339Nano)), nil); err != nil {
				_ = m.failJob(jobID, errorResponse("internal_error", "cannot update task status", err.Error()))
				return
			}
			if err := m.succeed(jobID, *result); err == nil {
				jobSucceeded = true
			}
			return
		}
		if err := m.updateTaskStatus(ctx, jobID, task.TaskID, StatusSucceeded, nil, strPtr(m.now().UTC().Format(time.RFC3339Nano)), nil); err != nil {
			_ = m.failJob(jobID, errorResponse("internal_error", "cannot update task status", err.Error()))
			return
		}
	}

	if stateID == "" {
		_ = m.failJob(jobID, errorResponse("internal_error", "missing output state", ""))
		return
	}
	result, errResp := c.executor.createInstance(ctx, jobID, prepared, stateID)
	if errResp != nil {
		_ = m.failJob(jobID, errResp)
		return
	}
	if err := m.succeed(jobID, *result); err == nil {
		jobSucceeded = true
	}
}

func (c *jobCoordinator) loadOrPlanTasks(ctx context.Context, jobID string, prepared preparedRequest) ([]taskState, string, *ErrorResponse) {
	m := c.m
	taskRecords, err := m.queue.ListTasks(ctx, jobID)
	if err != nil {
		return nil, "", errorResponse("internal_error", "cannot load tasks", err.Error())
	}
	if errResp := m.ensureResolvedImageID(ctx, jobID, &prepared, taskRecords); errResp != nil {
		return nil, "", errResp
	}
	if len(taskRecords) == 0 {
		if prepared.request.PrepareKind != "lb" {
			if errResp := m.updateJobSignature(ctx, jobID, prepared); errResp != nil {
				return nil, "", errResp
			}
		}
		tasks, stateID, errResp := c.buildPlan(ctx, jobID, prepared)
		if errResp != nil {
			return nil, "", errResp
		}
		if prepared.request.PrepareKind == "lb" {
			if errResp := m.updateJobSignatureFromPlan(ctx, jobID, prepared, tasks); errResp != nil {
				return nil, "", errResp
			}
		}
		m.logJob(jobID, "planned tasks count=%d state_id=%s", len(tasks), stateID)
		records := taskRecordsFromPlan(jobID, tasks)
		if err := m.queue.ReplaceTasks(ctx, jobID, records); err != nil {
			return nil, "", errorResponse("internal_error", "cannot store tasks", err.Error())
		}
		m.logJob(jobID, "stored tasks count=%d", len(tasks))
		m.trimCompletedJobs(ctx, prepared)
		return taskStatesFromPlan(tasks), stateID, nil
	}
	if err := m.validateManagedTasks(prepared, taskRecords); err != nil {
		return nil, "", errorResponse("internal_error", "invalid managed task chain", err.Error())
	}
	replanned, tasks, replannedStateID, errResp := c.replanTasksOnDrift(ctx, jobID, prepared, taskRecords)
	if errResp != nil {
		return nil, "", errResp
	}
	if replanned {
		return taskStatesFromPlan(tasks), replannedStateID, nil
	}
	states := taskStatesFromRecords(taskRecords)
	stateID := findOutputStateID(states)
	for i := range states {
		task := &states[i]
		if task.Status != StatusRunning {
			continue
		}
		if task.Type == "state_execute" {
			exists, err := m.isManagedStateCached(task.OutputStateID, prepared)
			if err != nil {
				return nil, "", errorResponse("internal_error", "cannot check recovered state cache", err.Error())
			}
			if exists {
				finishedAt := m.now().UTC().Format(time.RFC3339Nano)
				_ = m.updateTaskStatus(ctx, jobID, task.TaskID, StatusSucceeded, nil, &finishedAt, nil)
				task.Status = StatusSucceeded
			} else {
				_ = m.updateTaskStatus(ctx, jobID, task.TaskID, StatusQueued, nil, nil, nil)
				task.Status = StatusQueued
			}
		}
	}
	return states, stateID, nil
}

func (c *jobCoordinator) replanTasksOnDrift(ctx context.Context, jobID string, prepared preparedRequest, taskRecords []queue.TaskRecord) (bool, []PlanTask, string, *ErrorResponse) {
	m := c.m
	if len(taskRecords) == 0 {
		return false, nil, "", nil
	}
	job, ok, err := m.queue.GetJob(ctx, jobID)
	if err != nil {
		return false, nil, "", errorResponse("internal_error", "cannot load job", err.Error())
	}
	if !ok {
		return false, nil, "", errorResponse("internal_error", "cannot load job", jobID)
	}

	signatureDrift, errResp := m.hasPlanSignatureDrift(prepared, job, taskRecords)
	if errResp != nil {
		return false, nil, "", errResp
	}
	shapeDrift := hasPsqlExecuteShapeDrift(prepared, taskRecords)
	if !signatureDrift && !shapeDrift {
		return false, nil, "", nil
	}

	tasks, stateID, buildErr := c.buildPlan(ctx, jobID, prepared)
	if buildErr != nil {
		return false, nil, "", buildErr
	}
	if prepared.request.PrepareKind == "lb" {
		if errResp := m.updateJobSignatureFromPlan(ctx, jobID, prepared, tasks); errResp != nil {
			return false, nil, "", errResp
		}
	} else {
		if errResp := m.updateJobSignature(ctx, jobID, prepared); errResp != nil {
			return false, nil, "", errResp
		}
	}
	records := taskRecordsFromPlan(jobID, tasks)
	if err := m.queue.ReplaceTasks(ctx, jobID, records); err != nil {
		return false, nil, "", errorResponse("internal_error", "cannot store tasks", err.Error())
	}
	m.logJob(jobID, "replanned tasks due to plan drift signature=%t shape=%t count=%d state_id=%s", signatureDrift, shapeDrift, len(tasks), stateID)
	return true, tasks, stateID, nil
}

func (m *PrepareService) hasPlanSignatureDrift(prepared preparedRequest, job queue.JobRecord, taskRecords []queue.TaskRecord) (bool, *ErrorResponse) {
	storedSignature := strings.TrimSpace(valueOrEmpty(job.Signature))
	if storedSignature == "" {
		return false, nil
	}

	var expectedSignature string
	var errResp *ErrorResponse
	if prepared.request.PrepareKind == "lb" {
		expectedSignature, errResp = m.computeJobSignatureFromPlan(prepared, planTasksFromRecords(taskRecords))
	} else {
		expectedSignature, errResp = m.computeJobSignature(prepared)
	}
	if errResp != nil {
		return false, errResp
	}
	return storedSignature != expectedSignature, nil
}

func hasPsqlExecuteShapeDrift(prepared preparedRequest, taskRecords []queue.TaskRecord) bool {
	if prepared.request.PrepareKind != "psql" {
		return false
	}
	expectedExecTasks := len(prepared.psqlSteps)
	if expectedExecTasks == 0 {
		expectedExecTasks = 1
	}
	if expectedExecTasks <= 1 {
		return false
	}
	actualExecTasks := 0
	seen := map[string]bool{}
	for _, task := range taskRecords {
		if task.Type != "state_execute" {
			continue
		}
		actualExecTasks++
		seen[task.TaskID] = true
	}
	if actualExecTasks > 0 && actualExecTasks < expectedExecTasks {
		return true
	}
	if actualExecTasks != expectedExecTasks {
		return false
	}
	for i := 0; i < expectedExecTasks; i++ {
		if !seen[fmt.Sprintf("execute-%d", i)] {
			return true
		}
	}
	return false
}

func (m *PrepareService) updateJobSignature(ctx context.Context, jobID string, prepared preparedRequest) *ErrorResponse {
	signature, errResp := m.computeJobSignature(prepared)
	if errResp != nil {
		return errResp
	}
	if err := m.queue.UpdateJob(ctx, jobID, queue.JobUpdate{Signature: &signature}); err != nil {
		return errorResponse("internal_error", "cannot update job signature", err.Error())
	}
	return nil
}

func (m *PrepareService) updateJobSignatureFromPlan(ctx context.Context, jobID string, prepared preparedRequest, tasks []PlanTask) *ErrorResponse {
	signature, errResp := m.computeJobSignatureFromPlan(prepared, tasks)
	if errResp != nil {
		return errResp
	}
	if err := m.queue.UpdateJob(ctx, jobID, queue.JobUpdate{Signature: &signature}); err != nil {
		return errorResponse("internal_error", "cannot update job signature", err.Error())
	}
	return nil
}

func (m *PrepareService) computeJobSignature(prepared preparedRequest) (string, *ErrorResponse) {
	taskHash, errResp := m.computeTaskHash(prepared)
	if errResp != nil {
		return "", errResp
	}
	imageID := prepared.effectiveImageID()
	if strings.TrimSpace(imageID) == "" {
		return "", errorResponse("internal_error", "resolved image id is required", "")
	}
	hasher := newStateHasher()
	hasher.write("task_hash", taskHash)
	hasher.write("image_id", imageID)
	if prepared.managed.Key != "" {
		hasher.write("managed_base_key", prepared.managed.Key)
	}
	hasher.write("plan_only", fmt.Sprintf("%t", prepared.request.PlanOnly))
	signature := hasher.sum()
	if signature == "" {
		return "", errorResponse("internal_error", "cannot compute job signature", "")
	}
	return signature, nil
}

func (m *PrepareService) computeJobSignatureFromPlan(prepared preparedRequest, tasks []PlanTask) (string, *ErrorResponse) {
	imageID := prepared.effectiveImageID()
	if strings.TrimSpace(imageID) == "" {
		return "", errorResponse("internal_error", "resolved image id is required", "")
	}
	hasher := newStateHasher()
	hasher.write("image_id", imageID)
	if prepared.managed.Key != "" {
		hasher.write("managed_base_key", prepared.managed.Key)
	}
	hasher.write("plan_only", fmt.Sprintf("%t", prepared.request.PlanOnly))
	hasher.write("engine_version", m.version)
	for _, task := range tasks {
		hasher.write("task_id", task.TaskID)
		hasher.write("task_type", task.Type)
		if task.Input != nil {
			hasher.write("input_kind", task.Input.Kind)
			hasher.write("input_id", task.Input.ID)
		}
		if task.TaskHash != "" {
			hasher.write("task_hash", task.TaskHash)
		}
		if task.ChangesetID != "" {
			hasher.write("changeset_id", task.ChangesetID)
		}
		if task.ChangesetAuthor != "" {
			hasher.write("changeset_author", task.ChangesetAuthor)
		}
		if task.ChangesetPath != "" {
			hasher.write("changeset_path", task.ChangesetPath)
		}
	}
	signature := hasher.sum()
	if signature == "" {
		return "", errorResponse("internal_error", "cannot compute job signature", "")
	}
	return signature, nil
}

func (m *PrepareService) trimCompletedJobs(ctx context.Context, prepared preparedRequest) {
	limit := maxIdenticalJobs(m.config)
	if limit <= 0 {
		return
	}
	signature, errResp := m.computeJobSignature(prepared)
	if errResp != nil {
		m.logJob("", "job retention skipped: %s", errResp.Message)
		return
	}
	m.trimCompletedJobsBySignature(ctx, signature)
}

func (m *PrepareService) trimCompletedJobsForJob(ctx context.Context, jobID string) {
	if strings.TrimSpace(jobID) == "" {
		return
	}
	job, ok, err := m.queue.GetJob(ctx, jobID)
	if err != nil {
		m.logJob(jobID, "job retention lookup failed: %v", err)
		return
	}
	if !ok {
		return
	}
	signature := valueOrEmpty(job.Signature)
	if strings.TrimSpace(signature) == "" {
		return
	}
	m.trimCompletedJobsBySignature(ctx, signature)
}

func (m *PrepareService) trimCompletedJobsBySignature(ctx context.Context, signature string) {
	limit := maxIdenticalJobs(m.config)
	if limit <= 0 {
		return
	}
	if strings.TrimSpace(signature) == "" {
		return
	}
	jobs, err := m.queue.ListJobsBySignature(ctx, signature, []string{StatusSucceeded, StatusFailed})
	if err != nil {
		m.logJob("", "job retention failed: %v", err)
		return
	}
	if len(jobs) <= limit {
		return
	}
	for i := limit; i < len(jobs); i++ {
		jobID := jobs[i].JobID
		if err := m.queue.DeleteJob(ctx, jobID); err != nil {
			m.logJob(jobID, "job retention delete failed: %v", err)
			continue
		}
		if err := m.removeJobDir(jobID); err != nil {
			m.logJob(jobID, "job retention cleanup failed: %v", err)
		}
		m.logJob(jobID, "retention deleted")
	}
}

func (m *PrepareService) prepareRequest(req Request) (preparedRequest, error) {
	kind := strings.TrimSpace(req.PrepareKind)
	if kind == "" {
		return preparedRequest{}, ValidationError{Code: "invalid_argument", Message: "prepare_kind is required"}
	}
	switch kind {
	case "psql", "lb":
	default:
		return preparedRequest{}, ValidationError{Code: "invalid_argument", Message: "unsupported prepare_kind", Details: kind}
	}
	imageID := strings.TrimSpace(req.ImageID)
	if imageID == "" {
		return preparedRequest{}, ValidationError{Code: "invalid_argument", Message: "image_id is required"}
	}
	req.PrepareKind = kind
	req.ImageID = imageID
	var prepared preparedRequest
	switch kind {
	case "psql":
		psqlPrepared, err := preparePsqlArgs(req.PsqlArgs, req.Stdin)
		if err != nil {
			return preparedRequest{}, err
		}
		prepared = preparedRequest{
			request:        req,
			normalizedArgs: psqlPrepared.normalizedArgs,
			argsNormalized: psqlPrepared.argsNormalized,
			filePaths:      psqlPrepared.filePaths,
			psqlInputs:     psqlPrepared.inputs,
			psqlSteps:      psqlPrepared.steps,
			psqlWorkDir:    psqlPrepared.workDir,
		}
	case "lb":
		cwd, _ := os.Getwd()
		execMode := normalizeExecMode(req.LiquibaseExecMode)
		execPath := strings.TrimSpace(req.LiquibaseExec)
		windowsMode := shouldUseWindowsBat(execPath, execMode)
		lbPrepared, err := prepareLiquibaseArgs(req.LiquibaseArgs, cwd, windowsMode, usesContainerLiquibaseRunner(m.liquibase))
		if err != nil {
			return preparedRequest{}, err
		}
		prepared = preparedRequest{
			request:              req,
			normalizedArgs:       lbPrepared.normalizedArgs,
			argsNormalized:       lbPrepared.argsNormalized,
			liquibaseMounts:      lbPrepared.mounts,
			liquibaseLockPaths:   lbPrepared.lockPaths,
			liquibaseSearchPaths: lbPrepared.searchPaths,
			liquibaseWorkDir:     lbPrepared.workDir,
		}
	}
	resolvedImageID := ""
	if hasImageDigest(imageID) {
		resolvedImageID = imageID
	}
	prepared.resolvedImageID = resolvedImageID
	return prepared, nil
}

func usesContainerLiquibaseRunner(r liquibaseRunner) bool {
	switch r.(type) {
	case containerLiquibaseRunner, *containerLiquibaseRunner:
		return true
	default:
		return false
	}
}

func (c *jobCoordinator) buildPlan(ctx context.Context, jobID string, prepared preparedRequest) ([]PlanTask, string, *ErrorResponse) {
	switch prepared.request.PrepareKind {
	case "psql":
		return c.buildPlanPsql(prepared)
	case "lb":
		return c.buildPlanLiquibase(ctx, jobID, prepared)
	default:
		return nil, "", errorResponse("internal_error", "unsupported prepare kind", prepared.request.PrepareKind)
	}
}

func (c *jobCoordinator) buildPlanPsql(prepared preparedRequest) ([]PlanTask, string, *ErrorResponse) {
	m := c.m
	imageID := prepared.effectiveImageID()
	if strings.TrimSpace(imageID) == "" {
		return nil, "", errorResponse("internal_error", "resolved image id is required", "")
	}

	steps := prepared.psqlSteps
	if len(steps) == 0 {
		steps = []psqlStep{{
			args:   append([]string{}, prepared.normalizedArgs...),
			inputs: append([]psqlInput{}, prepared.psqlInputs...),
			stdin:  prepared.request.Stdin,
		}}
	}

	tasks := make([]PlanTask, 0, 3+len(steps))
	tasks = append(tasks, PlanTask{
		TaskID:      "plan",
		Type:        "plan",
		PlannerKind: prepared.request.PrepareKind,
	})
	if needsImageResolve(prepared.request.ImageID) {
		tasks = append(tasks, PlanTask{
			TaskID:          "resolve-image",
			Type:            "resolve_image",
			ImageID:         prepared.request.ImageID,
			ResolvedImageID: imageID,
		})
	}

	inputKind := "image"
	inputID := imageID
	if prepared.managed.Key != "" {
		inputID = prepared.managed.Key
	}
	stateID := ""
	for i, step := range steps {
		digest, err := computePsqlContentDigest(step.inputs, prepared.psqlWorkDir)
		if err != nil {
			return nil, "", errorResponse("invalid_argument", "cannot compute psql content hash", err.Error())
		}
		taskHash := psqlTaskHash(prepared.request.PrepareKind, digest.hash, m.version)
		outputStateID, errResp := m.computeOutputStateID(inputKind, inputID, taskHash)
		if errResp != nil {
			return nil, "", errResp
		}
		cached, err := m.isManagedStateCached(outputStateID, prepared)
		if err != nil {
			return nil, "", errorResponse("internal_error", "cannot check state cache", err.Error())
		}
		cachedFlag := cached
		tasks = append(tasks, PlanTask{
			TaskID: fmt.Sprintf("execute-%d", i),
			Type:   "state_execute",
			Input: &TaskInput{
				Kind: inputKind,
				ID:   inputID,
			},
			TaskHash:      taskHash,
			OutputStateID: outputStateID,
			Cached:        &cachedFlag,
		})
		inputKind = "state"
		inputID = outputStateID
		stateID = outputStateID
	}
	if strings.TrimSpace(stateID) == "" {
		return nil, "", errorResponse("internal_error", "missing output state", "")
	}
	tasks = append(tasks, PlanTask{
		TaskID: "prepare-instance",
		Type:   "prepare_instance",
		Input: &TaskInput{
			Kind: "state",
			ID:   stateID,
		},
		InstanceMode: "ephemeral",
	})
	return tasks, stateID, nil
}

func (c *jobCoordinator) buildPlanLiquibase(ctx context.Context, jobID string, prepared preparedRequest) ([]PlanTask, string, *ErrorResponse) {
	m := c.m
	imageID := prepared.effectiveImageID()
	if strings.TrimSpace(imageID) == "" {
		return nil, "", errorResponse("internal_error", "resolved image id is required", "")
	}
	changesets, errResp := c.planLiquibaseChangesets(ctx, jobID, prepared)
	if errResp != nil {
		return nil, "", errResp
	}

	tasks := make([]PlanTask, 0, 3+len(changesets))
	tasks = append(tasks, PlanTask{
		TaskID:      "plan",
		Type:        "plan",
		PlannerKind: prepared.request.PrepareKind,
	})
	if needsImageResolve(prepared.request.ImageID) {
		tasks = append(tasks, PlanTask{
			TaskID:          "resolve-image",
			Type:            "resolve_image",
			ImageID:         prepared.request.ImageID,
			ResolvedImageID: imageID,
		})
	}

	inputKind := "image"
	inputID := imageID
	if prepared.managed.Key != "" {
		inputID = prepared.managed.Key
	}
	prevFingerprintID := inputID
	stateID := ""

	if len(changesets) == 0 {
		taskHash := liquibaseFingerprint(prevFingerprintID, nil)
		outputStateID, errResp := m.computeOutputStateID(inputKind, inputID, taskHash)
		if errResp != nil {
			return nil, "", errResp
		}
		cached, err := m.isManagedStateCached(outputStateID, prepared)
		if err != nil {
			return nil, "", errorResponse("internal_error", "cannot check state cache", err.Error())
		}
		cachedFlag := cached
		tasks = append(tasks, PlanTask{
			TaskID: "execute-0",
			Type:   "state_execute",
			Input: &TaskInput{
				Kind: inputKind,
				ID:   inputID,
			},
			TaskHash:      taskHash,
			OutputStateID: outputStateID,
			Cached:        &cachedFlag,
		})
		stateID = outputStateID
	} else {
		for i, changeset := range changesets {
			taskHash := liquibaseFingerprint(prevFingerprintID, []LiquibaseChangeset{changeset})
			outputStateID, errResp := m.computeOutputStateID(inputKind, inputID, taskHash)
			if errResp != nil {
				return nil, "", errResp
			}
			cached, err := m.isManagedStateCached(outputStateID, prepared)
			if err != nil {
				return nil, "", errorResponse("internal_error", "cannot check state cache", err.Error())
			}
			cachedFlag := cached
			tasks = append(tasks, PlanTask{
				TaskID: fmt.Sprintf("execute-%d", i),
				Type:   "state_execute",
				Input: &TaskInput{
					Kind: inputKind,
					ID:   inputID,
				},
				TaskHash:        taskHash,
				OutputStateID:   outputStateID,
				Cached:          &cachedFlag,
				ChangesetID:     changeset.ID,
				ChangesetAuthor: changeset.Author,
				ChangesetPath:   changeset.Path,
			})
			inputKind = "state"
			inputID = outputStateID
			prevFingerprintID = outputStateID
			stateID = outputStateID
		}
	}

	if strings.TrimSpace(stateID) == "" {
		return nil, "", errorResponse("internal_error", "missing output state", "")
	}
	tasks = append(tasks, PlanTask{
		TaskID: "prepare-instance",
		Type:   "prepare_instance",
		Input: &TaskInput{
			Kind: "state",
			ID:   stateID,
		},
		InstanceMode: "ephemeral",
	})
	return tasks, stateID, nil
}

func (c *jobCoordinator) planLiquibaseChangesets(ctx context.Context, jobID string, prepared preparedRequest) ([]LiquibaseChangeset, *ErrorResponse) {
	m := c.m
	if m.liquibase == nil {
		return nil, errorResponse("internal_error", "liquibase runner is required", "")
	}
	imageID := prepared.effectiveImageID()
	if strings.TrimSpace(imageID) == "" {
		return nil, errorResponse("internal_error", "resolved image id is required", "")
	}

	var rt *jobRuntime
	baseInputID := imageID
	if prepared.managed.Key != "" {
		baseInputID = prepared.managed.Key
	}
	runner := m.getRunner(jobID)
	if !prepared.request.PlanOnly && runner != nil {
		planned, errResp := c.executor.ensureRuntime(ctx, jobID, prepared, &TaskInput{Kind: "image", ID: baseInputID}, runner)
		if errResp != nil {
			return nil, errResp
		}
		rt = planned
	} else {
		temp := &jobRunner{}
		planned, errResp := c.executor.startRuntime(ctx, jobID, prepared, &TaskInput{Kind: "image", ID: baseInputID})
		if errResp != nil {
			return nil, errResp
		}
		temp.setRuntime(planned)
		defer m.cleanupRuntime(context.Background(), temp)
		rt = planned
	}
	lock, errResp := ensureLiquibaseContentLock(prepared, "")
	if errResp != nil {
		return nil, errResp
	}
	defer lock.Close()
	return c.executor.runLiquibaseUpdateSQL(ctx, jobID, prepared, rt)
}

func (e *taskExecutor) runLiquibaseUpdateSQL(ctx context.Context, jobID string, prepared preparedRequest, rt *jobRuntime) ([]LiquibaseChangeset, *ErrorResponse) {
	m := e.m
	if m.liquibase == nil {
		return nil, errorResponse("internal_error", "liquibase runner is required", "")
	}
	if rt == nil {
		return nil, errorResponse("internal_error", "runtime instance is required", "")
	}
	if strings.TrimSpace(rt.instance.Host) == "" || rt.instance.Port == 0 {
		return nil, errorResponse("internal_error", "runtime instance is missing connection info", "")
	}

	execMode := normalizeExecMode(prepared.request.LiquibaseExecMode)
	rawExecPath := strings.TrimSpace(prepared.request.LiquibaseExec)
	windowsMode := shouldUseWindowsBat(rawExecPath, execMode)
	execPath, err := normalizeLiquibaseExecPath(rawExecPath, windowsMode)
	if err != nil {
		return nil, errorResponse("internal_error", "cannot resolve liquibase executable", err.Error())
	}
	var mapper PathMapper
	if windowsMode && isWSL() {
		mapper = wslPathMapper{}
	}
	args, err := mapLiquibaseArgs(prepared.normalizedArgs, mapper)
	if err != nil {
		return nil, errorResponse("internal_error", "cannot map liquibase arguments", err.Error())
	}
	workDir := strings.TrimSpace(prepared.request.WorkDir)
	if windowsMode && workDir == "" {
		workDir = deriveLiquibaseWorkDir(args)
	}
	if workDir != "" && mapper != nil {
		mappedDir, mapErr := mapper.MapPath(workDir)
		if mapErr != nil {
			return nil, errorResponse("internal_error", "cannot map liquibase workdir", mapErr.Error())
		}
		workDir = mappedDir
	}
	if !windowsMode {
		args = relativizeLiquibaseHostFileArgs(args, workDir)
	}
	args = replaceLiquibaseCommand(args, "updateSQL")
	args = prependLiquibaseConnectionArgs(args, rt.instance, windowsMode)
	var credential *instanceaccess.Secret
	if m.access != nil {
		connection, err := m.managedConnection(ctx, rt)
		if err != nil {
			return nil, errorResponse("internal_error", "managed recipe access unavailable", err.Error())
		}
		credential = &instanceaccess.Secret{Password: connection.Password}
	}
	env, err := mapLiquibaseEnv(prepared.request.LiquibaseEnv, windowsMode)
	if err != nil {
		return nil, errorResponse("internal_error", "cannot map liquibase env", err.Error())
	}

	execLine := formatExecLine(execPath, args)
	m.appendLog(jobID, fmt.Sprintf("liquibase: exec %s", execLine))
	m.logJob(jobID, "liquibase exec %s", execLine)
	m.appendLog(jobID, "liquibase: start")
	lbCtx := runtime.WithLogSink(ctx, func(line string) {
		m.appendLog(jobID, "liquibase: "+line)
	})
	output, err := m.liquibase.Run(lbCtx, LiquibaseRunRequest{
		Credential: credential,
		ExecPath:   execPath,
		ExecMode:   execMode,
		Args:       args,
		Env:        env,
		WorkDir:    workDir,
		Mounts:     prepared.liquibaseMounts,
		Network:    "",
	})
	if err != nil {
		if ctx.Err() != nil {
			return nil, errorResponse("cancelled", "task cancelled", "")
		}
		details := strings.TrimSpace(output)
		if details == "" {
			details = err.Error()
		}
		return nil, errorResponse("internal_error", "liquibase execution failed", details)
	}
	changesets, err := parseLiquibaseUpdateSQL(output)
	if err != nil {
		return nil, errorResponse("invalid_argument", "cannot parse liquibase changesets", err.Error())
	}
	return changesets, nil
}

func relativizeLiquibaseHostFileArgs(args []string, workDir string) []string {
	base := strings.TrimSpace(workDir)
	if base == "" {
		return args
	}
	base = filepath.Clean(base)
	if resolved, err := filepath.EvalSymlinks(base); err == nil {
		base = resolved
	}
	normalized := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		switch {
		case arg == "--changelog-file" || arg == "--defaults-file":
			if i+1 >= len(args) {
				normalized = append(normalized, arg)
				continue
			}
			normalized = append(normalized, arg, relativizeLiquibaseHostPath(args[i+1], base))
			i++
		case strings.HasPrefix(arg, "--changelog-file="):
			value := strings.TrimPrefix(arg, "--changelog-file=")
			normalized = append(normalized, "--changelog-file="+relativizeLiquibaseHostPath(value, base))
		case strings.HasPrefix(arg, "--defaults-file="):
			value := strings.TrimPrefix(arg, "--defaults-file=")
			normalized = append(normalized, "--defaults-file="+relativizeLiquibaseHostPath(value, base))
		default:
			normalized = append(normalized, arg)
		}
	}
	return normalized
}

func relativizeLiquibaseHostPath(value string, base string) string {
	path := strings.TrimSpace(value)
	if path == "" {
		return value
	}
	if looksLikeRemoteRef(path) || looksLikeWindowsPath(path) {
		return value
	}
	if !filepath.IsAbs(path) {
		return value
	}
	path = filepath.Clean(path)
	base = filepath.Clean(base)
	if resolved, err := filepath.EvalSymlinks(base); err == nil {
		base = resolved
	}
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	rel, err := filepath.Rel(base, path)
	if err != nil {
		return value
	}
	rel = filepath.Clean(rel)
	if rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return value
	}
	return rel
}

func (m *PrepareService) computeTaskHash(prepared preparedRequest) (string, *ErrorResponse) {
	if prepared.request.PrepareKind == "psql" {
		digest, err := computePsqlContentDigest(prepared.psqlInputs, prepared.psqlWorkDir)
		if err != nil {
			return "", errorResponse("invalid_argument", "cannot compute psql content hash", err.Error())
		}
		taskHash := psqlTaskHash(prepared.request.PrepareKind, digest.hash, m.version)
		if taskHash == "" {
			return "", errorResponse("internal_error", "cannot compute task hash", "")
		}
		return taskHash, nil
	}
	hasher := newStateHasher()
	hasher.write("prepare_kind", prepared.request.PrepareKind)
	for i, arg := range prepared.normalizedArgs {
		hasher.write(fmt.Sprintf("arg:%d", i), arg)
	}
	hasher.write("engine_version", m.version)
	taskHash := hasher.sum()
	if taskHash == "" {
		return "", errorResponse("internal_error", "cannot compute task hash", "")
	}
	return taskHash, nil
}

func psqlTaskHash(kind string, contentHash string, engineVersion string) string {
	return psqlTaskHashWithSchema(kind, contentHash, engineVersion, psqlTaskHashSchema)
}

const psqlTaskHashSchema = "psql-task-hash-v2"

func psqlTaskHashWithSchema(kind string, contentHash string, engineVersion string, schema string) string {
	hasher := newStateHasher()
	hasher.write("prepare_kind", kind)
	hasher.write("content_hash", contentHash)
	hasher.write("engine_version", engineVersion)
	hasher.write("psql_task_hash_schema", schema)
	return hasher.sum()
}

func (m *PrepareService) computeOutputStateID(inputKind, inputID, taskHash string) (string, *ErrorResponse) {
	hasher := newStateHasher()
	hasher.write("input_kind", inputKind)
	hasher.write("input_id", inputID)
	hasher.write("task_hash", taskHash)
	stateID := hasher.sum()
	if stateID == "" {
		return "", errorResponse("internal_error", "cannot compute state id", "")
	}
	return stateID, nil
}

func (m *PrepareService) isStateCached(stateID string) (bool, error) {
	if strings.TrimSpace(stateID) == "" {
		return false, nil
	}
	_, ok, err := m.store.GetState(context.Background(), stateID)
	if err != nil {
		return false, err
	}
	return ok, nil
}

func (m *PrepareService) markTasksSucceeded(ctx context.Context, jobID string, tasks []taskState) error {
	for _, task := range tasks {
		if err := m.updateTaskStatus(ctx, jobID, task.TaskID, StatusSucceeded, nil, strPtr(m.now().UTC().Format(time.RFC3339Nano)), nil); err != nil {
			return err
		}
	}
	return nil
}

func (m *PrepareService) updateTaskStatus(ctx context.Context, jobID string, taskID string, status string, startedAt *string, finishedAt *string, errResp *ErrorResponse) error {
	var errJSON *string
	if errResp != nil {
		payload, err := json.Marshal(errResp)
		if err != nil {
			return err
		}
		errJSON = strPtr(string(payload))
	}
	update := queue.TaskUpdate{
		Status:     &status,
		StartedAt:  startedAt,
		FinishedAt: finishedAt,
		ErrorJSON:  errJSON,
	}
	if err := m.queue.UpdateTask(ctx, jobID, taskID, update); err != nil {
		return err
	}
	m.logTask(jobID, taskID, "status=%s", status)
	event := Event{
		Type:   "task",
		Ts:     m.now().UTC().Format(time.RFC3339Nano),
		Status: status,
		TaskID: taskID,
	}
	if errResp != nil {
		event.Message = errResp.Message
		event.Error = errResp
	}
	return m.appendEvent(jobID, event)
}

func (m *PrepareService) succeed(jobID string, result Result) error {
	now := m.now().UTC().Format(time.RFC3339Nano)
	if m.access != nil {
		result.DSN = ""
	}
	payload, err := json.Marshal(result)
	if err != nil {
		return err
	}
	if err := m.queue.UpdateJob(context.Background(), jobID, queue.JobUpdate{
		Status:     strPtr(StatusSucceeded),
		FinishedAt: &now,
		ResultJSON: strPtr(string(payload)),
	}); err != nil {
		return err
	}
	if err := m.appendEvent(jobID, Event{
		Type:   "status",
		Ts:     now,
		Status: StatusSucceeded,
	}); err != nil {
		return err
	}
	m.logJob(jobID, "succeeded instance=%s state=%s", result.InstanceID, result.StateID)
	if err := m.appendEvent(jobID, Event{
		Type:   "result",
		Ts:     now,
		Result: &result,
	}); err != nil {
		return err
	}
	m.trimCompletedJobsForJob(context.Background(), jobID)
	return nil
}

func (m *PrepareService) succeedPlan(jobID string) error {
	now := m.now().UTC().Format(time.RFC3339Nano)
	if err := m.queue.UpdateJob(context.Background(), jobID, queue.JobUpdate{
		Status:     strPtr(StatusSucceeded),
		FinishedAt: &now,
	}); err != nil {
		return err
	}
	m.logJob(jobID, "succeeded plan_only=true")
	if err := m.appendEvent(jobID, Event{
		Type:   "status",
		Ts:     now,
		Status: StatusSucceeded,
	}); err != nil {
		return err
	}
	m.trimCompletedJobsForJob(context.Background(), jobID)
	return nil
}

func (m *PrepareService) failJob(jobID string, errResp *ErrorResponse) error {
	now := m.now().UTC().Format(time.RFC3339Nano)
	payload, err := json.Marshal(errResp)
	if err != nil {
		return err
	}
	if err := m.queue.UpdateJob(context.Background(), jobID, queue.JobUpdate{
		Status:     strPtr(StatusFailed),
		FinishedAt: &now,
		ErrorJSON:  strPtr(string(payload)),
	}); err != nil {
		return err
	}
	if err := m.appendEvent(jobID, Event{
		Type:   "status",
		Ts:     now,
		Status: StatusFailed,
	}); err != nil {
		return err
	}
	if errResp != nil {
		if details := summarizeLogDetails(errResp.Details); details != "" {
			m.logJob(jobID, "failed code=%s message=%s details=%s", errResp.Code, errResp.Message, details)
		} else {
			m.logJob(jobID, "failed code=%s message=%s", errResp.Code, errResp.Message)
		}
	} else {
		m.logJob(jobID, "failed")
	}
	if err := m.appendEvent(jobID, Event{
		Type:  "error",
		Ts:    now,
		Error: errResp,
	}); err != nil {
		return err
	}
	m.trimCompletedJobsForJob(context.Background(), jobID)
	return nil
}

func (m *PrepareService) appendEvent(jobID string, event Event) error {
	if m.access != nil && event.Result != nil {
		projection := *event.Result
		projection.DSN = ""
		event.Result = &projection
	}
	record := eventRecordFromEvent(jobID, event)
	if _, err := m.queue.AppendEvent(context.Background(), record); err != nil {
		return err
	}
	m.updateHeartbeat(jobID, event)
	m.events.notify(jobID)
	return nil
}

func (m *PrepareService) registerRunner(jobID string, cancel context.CancelFunc) *jobRunner {
	runner := &jobRunner{
		cancel: cancel,
		done:   make(chan struct{}),
	}
	m.mu.Lock()
	m.running[jobID] = runner
	m.mu.Unlock()
	return runner
}

func (m *PrepareService) unregisterRunner(jobID string) {
	m.mu.Lock()
	delete(m.running, jobID)
	m.mu.Unlock()
}

func (m *PrepareService) getRunner(jobID string) *jobRunner {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.running[jobID]
}

func (r *jobRunner) setRuntime(rt *jobRuntime) {
	r.mu.Lock()
	r.rt = rt
	r.mu.Unlock()
}

func (r *jobRunner) getRuntime() *jobRuntime {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.rt
}

func (m *PrepareService) logJob(jobID string, format string, args ...any) {
	if strings.TrimSpace(jobID) == "" {
		log.Printf("prepare "+format, args...)
		return
	}
	args = append([]any{jobID}, args...)
	log.Printf("prepare job=%s "+format, args...)
}

func (m *PrepareService) logTask(jobID string, taskID string, format string, args ...any) {
	if strings.TrimSpace(jobID) == "" || strings.TrimSpace(taskID) == "" {
		log.Printf("prepare task "+format, args...)
		return
	}
	args = append([]any{jobID, taskID}, args...)
	log.Printf("prepare job=%s task=%s "+format, args...)
}

func (m *PrepareService) logInfoJob(jobID string, format string, args ...any) {
	if !logLevelAllowsInfo(logLevelFromConfig(m.config)) {
		return
	}
	m.logJob(jobID, format, args...)
}

func (m *PrepareService) appendLog(jobID string, message string) {
	if strings.TrimSpace(jobID) == "" || strings.TrimSpace(message) == "" {
		return
	}
	_ = m.appendEvent(jobID, Event{
		Type:    "log",
		Ts:      m.now().UTC().Format(time.RFC3339Nano),
		Message: message,
	})
}

func (m *PrepareService) appendLogLines(jobID string, prefix string, content string) {
	content = strings.TrimSpace(content)
	if content == "" {
		return
	}
	for _, line := range strings.Split(content, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		if prefix != "" {
			m.appendLog(jobID, prefix+": "+line)
		} else {
			m.appendLog(jobID, line)
		}
	}
}

func normalizeHeartbeat(value time.Duration) time.Duration {
	if value <= 0 {
		return 500 * time.Millisecond
	}
	if value < 200*time.Millisecond {
		return 200 * time.Millisecond
	}
	if value > time.Second {
		return time.Second
	}
	return value
}

type heartbeatState struct {
	lastEventAt time.Time
	lastEvent   *Event
	runningTask string
	cancel      context.CancelFunc
}

func (m *PrepareService) updateHeartbeat(jobID string, event Event) {
	if strings.TrimSpace(jobID) == "" {
		return
	}
	m.mu.Lock()
	state := m.beats[jobID]
	if state == nil {
		if !(event.Type == "task" && event.Status == StatusRunning && strings.TrimSpace(event.TaskID) != "") {
			m.mu.Unlock()
			return
		}
		state = &heartbeatState{}
		m.beats[jobID] = state
	}
	state.lastEventAt = m.now().UTC()
	shouldStart := false
	if event.Type == "task" && event.Status == StatusRunning && strings.TrimSpace(event.TaskID) != "" {
		copy := event
		state.runningTask = event.TaskID
		state.lastEvent = &copy
		if state.cancel == nil {
			shouldStart = true
		}
	}
	if event.Type == "log" && state.runningTask != "" {
		copy := event
		state.lastEvent = &copy
	}
	if event.Type == "task" && event.Status != StatusRunning && state.runningTask != "" && state.runningTask == event.TaskID {
		state.runningTask = ""
		state.lastEvent = nil
		if state.cancel != nil {
			state.cancel()
			state.cancel = nil
		}
	}
	if event.Type == "status" && (event.Status == StatusSucceeded || event.Status == StatusFailed) {
		state.runningTask = ""
		state.lastEvent = nil
		if state.cancel != nil {
			state.cancel()
			state.cancel = nil
		}
		delete(m.beats, jobID)
	}
	m.mu.Unlock()
	if shouldStart {
		m.startHeartbeat(jobID)
	}
}

func (m *PrepareService) startHeartbeat(jobID string) {
	m.mu.Lock()
	state := m.beats[jobID]
	if state == nil || state.cancel != nil {
		m.mu.Unlock()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	state.cancel = cancel
	m.mu.Unlock()

	go func() {
		ticker := time.NewTicker(m.heartbeatEvery)
		defer ticker.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
			}
			var emit *Event
			var lastEventAt time.Time
			m.mu.Lock()
			state := m.beats[jobID]
			if state != nil {
				lastEventAt = state.lastEventAt
				if state.lastEvent != nil && state.runningTask != "" {
					copy := *state.lastEvent
					emit = &copy
				}
			}
			m.mu.Unlock()
			if emit == nil {
				return
			}
			if m.now().UTC().Sub(lastEventAt) < m.heartbeatEvery {
				continue
			}
			emit.Ts = m.now().UTC().Format(time.RFC3339Nano)
			_ = m.appendEvent(jobID, *emit)
		}
	}()
}

type taskState struct {
	PlanTask
	Status string
}

func taskStatesFromPlan(tasks []PlanTask) []taskState {
	states := make([]taskState, 0, len(tasks))
	for _, task := range tasks {
		states = append(states, taskState{
			PlanTask: task,
			Status:   StatusQueued,
		})
	}
	return states
}

func taskStatesFromRecords(records []queue.TaskRecord) []taskState {
	states := make([]taskState, 0, len(records))
	for _, task := range records {
		states = append(states, taskState{
			PlanTask: planTaskFromRecord(task),
			Status:   task.Status,
		})
	}
	return states
}

func taskRecordsFromPlan(jobID string, tasks []PlanTask) []queue.TaskRecord {
	records := make([]queue.TaskRecord, 0, len(tasks))
	for i, task := range tasks {
		records = append(records, queue.TaskRecord{
			JobID:           jobID,
			TaskID:          task.TaskID,
			Position:        i,
			Type:            task.Type,
			Status:          StatusQueued,
			PlannerKind:     nullableString(task.PlannerKind),
			InputKind:       nullableString(taskInputKind(task.Input)),
			InputID:         nullableString(taskInputID(task.Input)),
			ImageID:         nullableString(task.ImageID),
			ResolvedImageID: nullableString(task.ResolvedImageID),
			TaskHash:        nullableString(task.TaskHash),
			OutputStateID:   nullableString(task.OutputStateID),
			Cached:          task.Cached,
			InstanceMode:    nullableString(task.InstanceMode),
			ChangesetID:     nullableString(task.ChangesetID),
			ChangesetAuthor: nullableString(task.ChangesetAuthor),
			ChangesetPath:   nullableString(task.ChangesetPath),
		})
	}
	return records
}

func planTasksFromRecords(records []queue.TaskRecord) []PlanTask {
	tasks := make([]PlanTask, 0, len(records))
	for _, task := range records {
		tasks = append(tasks, planTaskFromRecord(task))
	}
	return tasks
}

func planTaskFromRecord(task queue.TaskRecord) PlanTask {
	var input *TaskInput
	if task.InputKind != nil && task.InputID != nil {
		input = &TaskInput{
			Kind: *task.InputKind,
			ID:   *task.InputID,
		}
	}
	return PlanTask{
		TaskID:          task.TaskID,
		Type:            task.Type,
		PlannerKind:     valueOrEmpty(task.PlannerKind),
		Input:           input,
		ImageID:         valueOrEmpty(task.ImageID),
		ResolvedImageID: valueOrEmpty(task.ResolvedImageID),
		TaskHash:        valueOrEmpty(task.TaskHash),
		OutputStateID:   valueOrEmpty(task.OutputStateID),
		Cached:          task.Cached,
		InstanceMode:    valueOrEmpty(task.InstanceMode),
		ChangesetID:     valueOrEmpty(task.ChangesetID),
		ChangesetAuthor: valueOrEmpty(task.ChangesetAuthor),
		ChangesetPath:   valueOrEmpty(task.ChangesetPath),
	}
}

// decodeJobRequest reconstructs list-surface summaries from the persisted job request.
// See docs/user-guides/sqlrs-ls.md and docs/architecture/cli-contract.md.
func decodeJobRequest(job queue.JobRecord) *Request {
	if job.RequestJSON == nil || strings.TrimSpace(*job.RequestJSON) == "" {
		return nil
	}
	var req Request
	if err := json.Unmarshal([]byte(*job.RequestJSON), &req); err != nil {
		return nil
	}
	return &req
}

func taskEntryFromRecord(task queue.TaskRecord, req *Request) TaskEntry {
	var input *TaskInput
	if task.InputKind != nil && task.InputID != nil {
		input = &TaskInput{
			Kind: *task.InputKind,
			ID:   *task.InputID,
		}
	}
	return TaskEntry{
		TaskID:          task.TaskID,
		JobID:           task.JobID,
		Type:            task.Type,
		Status:          task.Status,
		PlannerKind:     valueOrEmpty(task.PlannerKind),
		Input:           input,
		ImageID:         valueOrEmpty(task.ImageID),
		ResolvedImageID: valueOrEmpty(task.ResolvedImageID),
		ArgsSummary:     taskArgsSummary(task, req),
		TaskHash:        valueOrEmpty(task.TaskHash),
		OutputStateID:   valueOrEmpty(task.OutputStateID),
		Cached:          task.Cached,
		InstanceMode:    valueOrEmpty(task.InstanceMode),
		ChangesetID:     valueOrEmpty(task.ChangesetID),
		ChangesetAuthor: valueOrEmpty(task.ChangesetAuthor),
		ChangesetPath:   valueOrEmpty(task.ChangesetPath),
	}
}

func taskArgsSummary(task queue.TaskRecord, req *Request) string {
	if strings.TrimSpace(task.Type) != "state_execute" {
		return ""
	}

	if summary := liquibaseTaskArgsSummary(task); summary != "" {
		return summary
	}
	return psqlTaskArgsSummary(task.TaskID, req)
}

func liquibaseTaskArgsSummary(task queue.TaskRecord) string {
	id := strings.TrimSpace(valueOrEmpty(task.ChangesetID))
	author := strings.TrimSpace(valueOrEmpty(task.ChangesetAuthor))
	path := strings.TrimSpace(valueOrEmpty(task.ChangesetPath))
	if id == "" || author == "" || path == "" {
		return ""
	}
	return id + "::" + author + "::" + path
}

func psqlTaskArgsSummary(taskID string, req *Request) string {
	if req == nil || strings.TrimSpace(strings.ToLower(req.PrepareKind)) != "psql" {
		return ""
	}
	steps, err := buildPsqlSteps(req.PsqlArgs, req.Stdin)
	if err != nil {
		return ""
	}
	step, err := psqlStepForTask(steps, taskID)
	if err != nil || len(step.inputs) == 0 {
		return ""
	}
	input := step.inputs[0]
	switch input.kind {
	case "file":
		return "-f " + input.value
	case "stdin":
		return "-f -"
	case "command":
		return "-c " + input.value
	default:
		return ""
	}
}

func eventFromRecord(record queue.EventRecord) Event {
	event := Event{
		Type:    record.Type,
		Ts:      record.Ts,
		Status:  valueOrEmpty(record.Status),
		TaskID:  valueOrEmpty(record.TaskID),
		Message: valueOrEmpty(record.Message),
	}
	if record.ResultJSON != nil {
		var result Result
		if err := json.Unmarshal([]byte(*record.ResultJSON), &result); err == nil {
			event.Result = &result
		}
	}
	if record.ErrorJSON != nil {
		var errResp ErrorResponse
		if err := json.Unmarshal([]byte(*record.ErrorJSON), &errResp); err == nil {
			event.Error = &errResp
		}
	}
	return event
}

func eventRecordFromEvent(jobID string, event Event) queue.EventRecord {
	record := queue.EventRecord{
		JobID:   jobID,
		Type:    event.Type,
		Ts:      event.Ts,
		Status:  nullableString(event.Status),
		TaskID:  nullableString(event.TaskID),
		Message: nullableString(event.Message),
	}
	if event.Result != nil {
		if payload, err := json.Marshal(event.Result); err == nil {
			record.ResultJSON = strPtr(string(payload))
		}
	}
	if event.Error != nil {
		if payload, err := json.Marshal(event.Error); err == nil {
			record.ErrorJSON = strPtr(string(payload))
		}
	}
	return record
}

func findOutputStateID(tasks []taskState) string {
	for i := len(tasks) - 1; i >= 0; i-- {
		if tasks[i].Type == "state_execute" && tasks[i].OutputStateID != "" {
			return tasks[i].OutputStateID
		}
	}
	return ""
}

func hasRunningTasks(tasks []queue.TaskRecord) bool {
	for _, task := range tasks {
		if task.Status == StatusRunning {
			return true
		}
	}
	return false
}

func taskInputKind(input *TaskInput) string {
	if input == nil {
		return ""
	}
	return input.Kind
}

func taskInputID(input *TaskInput) string {
	if input == nil {
		return ""
	}
	return input.ID
}

func valueOrEmpty(value *string) string {
	if value == nil {
		return ""
	}
	return *value
}

func nullableString(value string) *string {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	return &value
}

func summarizeLogDetails(value string) string {
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return ""
	}
	trimmed = strings.ReplaceAll(trimmed, "\r\n", "\n")
	trimmed = strings.ReplaceAll(trimmed, "\n", " | ")
	if len(trimmed) <= 512 {
		return trimmed
	}
	return trimmed[:512] + "... (truncated)"
}

const defaultMaxIdenticalJobs = 2

func maxIdenticalJobs(cfg config.Store) int {
	if cfg == nil {
		return defaultMaxIdenticalJobs
	}
	value, err := cfg.Get("orchestrator.jobs.maxIdentical", true)
	if err != nil {
		return defaultMaxIdenticalJobs
	}
	if value == nil {
		return defaultMaxIdenticalJobs
	}
	if num, ok := configValueToInt(value); ok && num >= 0 {
		return num
	}
	return defaultMaxIdenticalJobs
}

func configValueToInt(value any) (int, bool) {
	switch v := value.(type) {
	case int:
		return v, true
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case float32:
		if v != float32(int(v)) {
			return 0, false
		}
		return int(v), true
	case float64:
		if v != float64(int(v)) {
			return 0, false
		}
		return int(v), true
	case json.Number:
		if strings.ContainsAny(string(v), ".eE") {
			return 0, false
		}
		parsed, err := v.Int64()
		if err != nil {
			return 0, false
		}
		return int(parsed), true
	default:
		return 0, false
	}
}

func configValueToString(value any) (string, bool) {
	switch v := value.(type) {
	case string:
		return v, true
	case fmt.Stringer:
		return v.String(), true
	case []byte:
		return string(v), true
	default:
		return "", false
	}
}

func logLevelFromConfig(cfg config.Store) string {
	if cfg == nil {
		return "debug"
	}
	value, err := cfg.Get("log.level", true)
	if err != nil || value == nil {
		return "debug"
	}
	level, ok := configValueToString(value)
	if !ok {
		return "debug"
	}
	level = strings.TrimSpace(strings.ToLower(level))
	if level == "" {
		return "debug"
	}
	return level
}

func logLevelAllowsInfo(level string) bool {
	switch strings.TrimSpace(strings.ToLower(level)) {
	case "", "debug", "info":
		return true
	default:
		return false
	}
}

func (m *PrepareService) removeJobDir(jobID string) error {
	if strings.TrimSpace(m.stateStoreRoot) == "" {
		return nil
	}
	if strings.TrimSpace(jobID) == "" {
		return nil
	}
	path := filepath.Join(m.stateStoreRoot, "jobs", jobID)
	if m.statefs != nil {
		runtimeDir := filepath.Join(path, "runtime")
		_ = m.statefs.RemovePath(context.Background(), runtimeDir)
	}
	return os.RemoveAll(path)
}

func strPtr(value string) *string {
	return &value
}

func (p preparedRequest) effectiveImageID() string {
	if strings.TrimSpace(p.resolvedImageID) != "" {
		return p.resolvedImageID
	}
	return p.request.ImageID
}

func hasImageDigest(imageID string) bool {
	imageID = strings.TrimSpace(imageID)
	if imageID == "" {
		return false
	}
	at := strings.LastIndex(imageID, "@")
	return at != -1 && at+1 < len(imageID)
}

func needsImageResolve(imageID string) bool {
	return !hasImageDigest(imageID)
}

func (m *PrepareService) ensureResolvedImageID(ctx context.Context, jobID string, prepared *preparedRequest, tasks []queue.TaskRecord) *ErrorResponse {
	if prepared == nil {
		return errorResponse("internal_error", "prepared request is required", "")
	}
	if strings.TrimSpace(prepared.resolvedImageID) != "" {
		return nil
	}
	if resolved := resolvedImageFromTasks(tasks); resolved != "" {
		prepared.resolvedImageID = resolved
		return nil
	}
	if len(tasks) > 0 {
		prepared.resolvedImageID = prepared.request.ImageID
		return nil
	}
	if !needsImageResolve(prepared.request.ImageID) {
		prepared.resolvedImageID = prepared.request.ImageID
		return nil
	}
	m.appendLog(jobID, fmt.Sprintf("resolve image %s", prepared.request.ImageID))
	ctx = runtime.WithLogSink(ctx, func(line string) {
		m.appendLog(jobID, "docker: "+line)
	})
	resolved, err := m.runtime.ResolveImage(ctx, prepared.request.ImageID)
	if err != nil {
		return errorResponse("internal_error", "cannot resolve image", err.Error())
	}
	resolved = strings.TrimSpace(resolved)
	if resolved == "" {
		return errorResponse("internal_error", "resolved image id is required", "")
	}
	m.appendLog(jobID, fmt.Sprintf("resolved image %s", resolved))
	prepared.resolvedImageID = resolved
	return nil
}

func resolvedImageFromTasks(tasks []queue.TaskRecord) string {
	for _, task := range tasks {
		if task.Type != "resolve_image" {
			continue
		}
		resolved := valueOrEmpty(task.ResolvedImageID)
		if strings.TrimSpace(resolved) != "" {
			return resolved
		}
	}
	return ""
}

var errJobNotFound = fmt.Errorf("job not found")

type eventBus struct {
	mu   sync.Mutex
	subs map[string]map[chan struct{}]struct{}
}

func newEventBus() *eventBus {
	return &eventBus{
		subs: map[string]map[chan struct{}]struct{}{},
	}
}

func (b *eventBus) subscribe(jobID string) chan struct{} {
	ch := make(chan struct{}, 1)
	b.mu.Lock()
	if b.subs[jobID] == nil {
		b.subs[jobID] = map[chan struct{}]struct{}{}
	}
	b.subs[jobID][ch] = struct{}{}
	b.mu.Unlock()
	return ch
}

func (b *eventBus) unsubscribe(jobID string, ch chan struct{}) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.subs[jobID] != nil {
		delete(b.subs[jobID], ch)
		if len(b.subs[jobID]) == 0 {
			delete(b.subs, jobID)
		}
	}
	close(ch)
}

func (b *eventBus) notify(jobID string) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for ch := range b.subs[jobID] {
		select {
		case ch <- struct{}{}:
		default:
		}
	}
}

func buildDSN(host string, port int) string {
	return fmt.Sprintf("postgres://sqlrs@%s:%d/postgres", host, port)
}

func formatTime(value time.Time) *string {
	if value.IsZero() {
		return nil
	}
	formatted := value.UTC().Format(time.RFC3339Nano)
	return &formatted
}

func formatTimePtr(value *time.Time) *string {
	if value == nil {
		return nil
	}
	return formatTime(*value)
}

var randReader = rand.Reader

func randomHex(bytes int) (string, error) {
	buf := make([]byte, bytes)
	if _, err := randReader.Read(buf); err != nil {
		return "", err
	}
	return hex.EncodeToString(buf), nil
}
