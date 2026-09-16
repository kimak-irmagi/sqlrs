package prepare

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"log"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sqlrs/engine-local/internal/config"
	"github.com/sqlrs/engine-local/internal/deletion"
	"github.com/sqlrs/engine-local/internal/prepare/queue"
	engineRuntime "github.com/sqlrs/engine-local/internal/runtime"
	"github.com/sqlrs/engine-local/internal/store"
)

type fakeStore struct {
	createStateErr    error
	createInstanceErr error
	getStateErr       error
	listStatesErr     error
	statesByID        map[string]store.StateEntry
	listStates        []store.StateEntry
	states            []store.StateCreate
	instances         []store.InstanceCreate
	deletedStates     []string
}

func (f *fakeStore) ListNames(ctx context.Context, filters store.NameFilters) ([]store.NameEntry, error) {
	return nil, nil
}

func (f *fakeStore) GetName(ctx context.Context, name string) (store.NameEntry, bool, error) {
	return store.NameEntry{}, false, nil
}

func (f *fakeStore) ListInstances(ctx context.Context, filters store.InstanceFilters) ([]store.InstanceEntry, error) {
	return nil, nil
}

func (f *fakeStore) GetInstance(ctx context.Context, instanceID string) (store.InstanceEntry, bool, error) {
	return store.InstanceEntry{}, false, nil
}

func (f *fakeStore) ListStates(ctx context.Context, filters store.StateFilters) ([]store.StateEntry, error) {
	if f.listStatesErr != nil {
		return nil, f.listStatesErr
	}
	source := f.listStates
	if len(source) == 0 && len(f.statesByID) > 0 {
		source = make([]store.StateEntry, 0, len(f.statesByID))
		for _, entry := range f.statesByID {
			source = append(source, entry)
		}
	}
	if len(source) == 0 {
		return nil, nil
	}
	out := make([]store.StateEntry, 0, len(source))
	for _, entry := range source {
		if filters.Kind != "" && entry.PrepareKind != filters.Kind {
			continue
		}
		if filters.ImageID != "" && entry.ImageID != filters.ImageID {
			continue
		}
		if filters.ParentID != "" {
			if entry.ParentStateID == nil || *entry.ParentStateID != filters.ParentID {
				continue
			}
		}
		if filters.IDPrefix != "" && !strings.HasPrefix(strings.ToLower(entry.StateID), strings.ToLower(filters.IDPrefix)) {
			continue
		}
		out = append(out, entry)
	}
	return out, nil
}

func (f *fakeStore) GetState(ctx context.Context, stateID string) (store.StateEntry, bool, error) {
	if f.getStateErr != nil {
		return store.StateEntry{}, false, f.getStateErr
	}
	if f.statesByID == nil {
		return store.StateEntry{}, false, nil
	}
	entry, ok := f.statesByID[stateID]
	return entry, ok, nil
}

func (f *fakeStore) CreateState(ctx context.Context, entry store.StateCreate) error {
	if f.createStateErr != nil {
		return f.createStateErr
	}
	f.states = append(f.states, entry)
	if f.statesByID == nil {
		f.statesByID = map[string]store.StateEntry{}
	}
	f.statesByID[entry.StateID] = store.StateEntry{
		StateID:       entry.StateID,
		ParentStateID: entry.ParentStateID,
		ImageID:       entry.ImageID,
		PrepareKind:   entry.PrepareKind,
		PrepareArgs:   entry.PrepareArgsNormalized,
		CreatedAt:     entry.CreatedAt,
		SizeBytes:     entry.SizeBytes,
		RefCount:      0,
	}
	return nil
}

func (f *fakeStore) UpdateStateSize(ctx context.Context, stateID string, sizeBytes int64) error {
	entry, ok := f.statesByID[stateID]
	if !ok {
		return nil
	}
	if entry.SizeBytes != nil && *entry.SizeBytes > 0 {
		return nil
	}
	value := sizeBytes
	entry.SizeBytes = &value
	f.statesByID[stateID] = entry
	return nil
}

func (f *fakeStore) CreateInstance(ctx context.Context, entry store.InstanceCreate) error {
	if f.createInstanceErr != nil {
		return f.createInstanceErr
	}
	f.instances = append(f.instances, entry)
	return nil
}

func (f *fakeStore) DeleteInstance(ctx context.Context, instanceID string) error {
	return nil
}

func (f *fakeStore) DeleteState(ctx context.Context, stateID string) error {
	f.deletedStates = append(f.deletedStates, stateID)
	if f.statesByID != nil {
		delete(f.statesByID, stateID)
	}
	return nil
}

func (f *fakeStore) Close() error {
	return nil
}

type blockingStore struct {
	fakeStore
	started chan struct{}
}

func (b *blockingStore) CreateState(ctx context.Context, entry store.StateCreate) error {
	select {
	case <-b.started:
	default:
		close(b.started)
	}
	<-ctx.Done()
	return ctx.Err()
}

type cancelStore struct {
	fakeStore
}

func (c *cancelStore) CreateInstance(ctx context.Context, entry store.InstanceCreate) error {
	return ctx.Err()
}

type cancelStateStore struct {
	fakeStore
}

func (c *cancelStateStore) CreateState(ctx context.Context, entry store.StateCreate) error {
	return ctx.Err()
}

type fakeRuntime struct {
	instance      engineRuntime.Instance
	initCalls     []engineRuntime.StartRequest
	startCalls    []engineRuntime.StartRequest
	stopCalls     []string
	execCalls     []engineRuntime.ExecRequest
	waitCalls     []time.Duration
	resolveCalls  []string
	noDefaults    bool
	initErr       error
	resolveErr    error
	startErr      error
	stopErr       error
	execErr       error
	waitErr       error
	execOutput    string
	initCreated   bool
	resolvedImage string
}

func (f *fakeRuntime) InitBase(ctx context.Context, imageID string, dataDir string) error {
	f.initCalls = append(f.initCalls, engineRuntime.StartRequest{ImageID: imageID, DataDir: dataDir})
	if f.initErr != nil {
		return f.initErr
	}
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return err
	}
	f.initCreated = true
	return os.WriteFile(filepath.Join(dataDir, "PG_VERSION"), []byte("17"), 0o600)
}

type loggingRuntime struct {
	fakeRuntime
}

func (l *loggingRuntime) ResolveImage(ctx context.Context, imageID string) (string, error) {
	if sink := engineRuntime.LogSinkFromContext(ctx); sink != nil {
		sink("pulling layers")
	}
	return l.fakeRuntime.ResolveImage(ctx, imageID)
}

func (f *fakeRuntime) Start(ctx context.Context, req engineRuntime.StartRequest) (engineRuntime.Instance, error) {
	f.startCalls = append(f.startCalls, req)
	if f.startErr != nil {
		return engineRuntime.Instance{}, f.startErr
	}
	instance := f.instance
	if !f.noDefaults {
		if instance.ID == "" {
			instance.ID = "container-1"
		}
		if instance.Host == "" {
			instance.Host = "127.0.0.1"
		}
		if instance.Port == 0 {
			instance.Port = 5432
		}
	}
	return instance, nil
}

func (f *fakeRuntime) ResolveImage(ctx context.Context, imageID string) (string, error) {
	f.resolveCalls = append(f.resolveCalls, imageID)
	if f.resolveErr != nil {
		return "", f.resolveErr
	}
	if f.resolvedImage != "" {
		return f.resolvedImage, nil
	}
	return imageID + "@sha256:resolved", nil
}

func (f *fakeRuntime) Stop(ctx context.Context, id string) error {
	f.stopCalls = append(f.stopCalls, id)
	if f.stopErr != nil {
		return f.stopErr
	}
	return nil
}

func (f *fakeRuntime) Exec(ctx context.Context, id string, req engineRuntime.ExecRequest) (string, error) {
	f.execCalls = append(f.execCalls, req)
	if f.execErr != nil {
		return f.execOutput, f.execErr
	}
	return f.execOutput, nil
}

func (f *fakeRuntime) WaitForReady(ctx context.Context, id string, timeout time.Duration) error {
	f.waitCalls = append(f.waitCalls, timeout)
	if f.waitErr != nil {
		return f.waitErr
	}
	return nil
}

type cancelRuntime struct {
	started chan struct{}
}

func (b *cancelRuntime) InitBase(ctx context.Context, imageID string, dataDir string) error {
	return nil
}

func (b *cancelRuntime) ResolveImage(ctx context.Context, imageID string) (string, error) {
	return imageID + "@sha256:resolved", nil
}

func (b *cancelRuntime) Start(ctx context.Context, req engineRuntime.StartRequest) (engineRuntime.Instance, error) {
	select {
	case <-b.started:
	default:
		close(b.started)
	}
	<-ctx.Done()
	return engineRuntime.Instance{}, ctx.Err()
}

func (b *cancelRuntime) Stop(ctx context.Context, id string) error {
	return nil
}

func (b *cancelRuntime) Exec(ctx context.Context, id string, req engineRuntime.ExecRequest) (string, error) {
	return "", nil
}

func (b *cancelRuntime) WaitForReady(ctx context.Context, id string, timeout time.Duration) error {
	return nil
}

type fakeDBMS struct {
	prepareCalls int
	resumeCalls  int
	prepareErr   error
	resumeErr    error
}

func (f *fakeDBMS) PrepareSnapshot(ctx context.Context, instance engineRuntime.Instance) error {
	f.prepareCalls++
	return f.prepareErr
}

func (f *fakeDBMS) ResumeSnapshot(ctx context.Context, instance engineRuntime.Instance) error {
	f.resumeCalls++
	return f.resumeErr
}

type fakePsqlRunner struct {
	runs      []PsqlRunRequest
	instances []engineRuntime.Instance
	output    string
	err       error
}

func (f *fakePsqlRunner) Run(ctx context.Context, instance engineRuntime.Instance, req PsqlRunRequest) (string, error) {
	f.runs = append(f.runs, req)
	f.instances = append(f.instances, instance)
	if f.err != nil {
		return f.output, f.err
	}
	return f.output, nil
}

type streamingPsqlRunner struct {
	output string
	err    error
}

func (s *streamingPsqlRunner) Run(ctx context.Context, instance engineRuntime.Instance, req PsqlRunRequest) (string, error) {
	if sink := engineRuntime.LogSinkFromContext(ctx); sink != nil {
		sink("streamed line")
	}
	if s.err != nil {
		return s.output, s.err
	}
	return s.output, nil
}

type fakeLiquibaseRunner struct {
	runs   []LiquibaseRunRequest
	output string
	err    error
}

func (f *fakeLiquibaseRunner) Run(ctx context.Context, req LiquibaseRunRequest) (string, error) {
	f.runs = append(f.runs, req)
	if f.err != nil {
		return f.output, f.err
	}
	return f.output, nil
}

type streamingLiquibaseRunner struct {
	output string
	err    error
}

func (s *streamingLiquibaseRunner) Run(ctx context.Context, req LiquibaseRunRequest) (string, error) {
	if sink := engineRuntime.LogSinkFromContext(ctx); sink != nil {
		sink("liquibase streamed")
	}
	if s.err != nil {
		return s.output, s.err
	}
	return s.output, nil
}

type faultQueueStore struct {
	queue.Store

	createJob        func(context.Context, queue.JobRecord) error
	updateJob        func(context.Context, string, queue.JobUpdate) error
	getJob           func(context.Context, string) (queue.JobRecord, bool, error)
	listJobs         func(context.Context, string) ([]queue.JobRecord, error)
	listJobsByStatus func(context.Context, []string) ([]queue.JobRecord, error)
	deleteJob        func(context.Context, string) error

	replaceTasks func(context.Context, string, []queue.TaskRecord) error
	listTasks    func(context.Context, string) ([]queue.TaskRecord, error)
	updateTask   func(context.Context, string, string, queue.TaskUpdate) error

	appendEvent     func(context.Context, queue.EventRecord) (int64, error)
	listEventsSince func(context.Context, string, int) ([]queue.EventRecord, error)
	countEvents     func(context.Context, string) (int, error)
}

func (f *faultQueueStore) CreateJob(ctx context.Context, job queue.JobRecord) error {
	if f.createJob != nil {
		return f.createJob(ctx, job)
	}
	return f.Store.CreateJob(ctx, job)
}

func (f *faultQueueStore) UpdateJob(ctx context.Context, jobID string, update queue.JobUpdate) error {
	if f.updateJob != nil {
		return f.updateJob(ctx, jobID, update)
	}
	return f.Store.UpdateJob(ctx, jobID, update)
}

func (f *faultQueueStore) GetJob(ctx context.Context, jobID string) (queue.JobRecord, bool, error) {
	if f.getJob != nil {
		return f.getJob(ctx, jobID)
	}
	return f.Store.GetJob(ctx, jobID)
}

func (f *faultQueueStore) ListJobs(ctx context.Context, jobID string) ([]queue.JobRecord, error) {
	if f.listJobs != nil {
		return f.listJobs(ctx, jobID)
	}
	return f.Store.ListJobs(ctx, jobID)
}

func (f *faultQueueStore) ListJobsByStatus(ctx context.Context, statuses []string) ([]queue.JobRecord, error) {
	if f.listJobsByStatus != nil {
		return f.listJobsByStatus(ctx, statuses)
	}
	return f.Store.ListJobsByStatus(ctx, statuses)
}

func (f *faultQueueStore) DeleteJob(ctx context.Context, jobID string) error {
	if f.deleteJob != nil {
		return f.deleteJob(ctx, jobID)
	}
	return f.Store.DeleteJob(ctx, jobID)
}

func (f *faultQueueStore) ReplaceTasks(ctx context.Context, jobID string, tasks []queue.TaskRecord) error {
	if f.replaceTasks != nil {
		return f.replaceTasks(ctx, jobID, tasks)
	}
	return f.Store.ReplaceTasks(ctx, jobID, tasks)
}

func (f *faultQueueStore) ListTasks(ctx context.Context, jobID string) ([]queue.TaskRecord, error) {
	if f.listTasks != nil {
		return f.listTasks(ctx, jobID)
	}
	return f.Store.ListTasks(ctx, jobID)
}

func (f *faultQueueStore) UpdateTask(ctx context.Context, jobID string, taskID string, update queue.TaskUpdate) error {
	if f.updateTask != nil {
		return f.updateTask(ctx, jobID, taskID, update)
	}
	return f.Store.UpdateTask(ctx, jobID, taskID, update)
}

func (f *faultQueueStore) AppendEvent(ctx context.Context, event queue.EventRecord) (int64, error) {
	if f.appendEvent != nil {
		return f.appendEvent(ctx, event)
	}
	return f.Store.AppendEvent(ctx, event)
}

func (f *faultQueueStore) ListEventsSince(ctx context.Context, jobID string, offset int) ([]queue.EventRecord, error) {
	if f.listEventsSince != nil {
		return f.listEventsSince(ctx, jobID, offset)
	}
	return f.Store.ListEventsSince(ctx, jobID, offset)
}

func (f *faultQueueStore) CountEvents(ctx context.Context, jobID string) (int, error) {
	if f.countEvents != nil {
		return f.countEvents(ctx, jobID)
	}
	return f.Store.CountEvents(ctx, jobID)
}

func (f *faultQueueStore) Close() error {
	return f.Store.Close()
}

func TestNewManagerRequiresStore(t *testing.T) {
	queueStore := newQueueStore(t)
	if _, err := NewPrepareService(Options{Queue: queueStore}); err == nil {
		t.Fatalf("expected error when store is nil")
	}
}

func TestNewManagerRequiresQueue(t *testing.T) {
	if _, err := NewPrepareService(Options{Store: &fakeStore{}}); err == nil {
		t.Fatalf("expected error when queue is nil")
	}
}

func TestNewManagerRequiresRuntime(t *testing.T) {
	queueStore := newQueueStore(t)
	stateRoot := filepath.Join(t.TempDir(), "state-store")
	if _, err := NewPrepareService(Options{
		Store:          &fakeStore{},
		Queue:          queueStore,
		StateFS:        &fakeStateFS{},
		DBMS:           &fakeDBMS{},
		StateStoreRoot: stateRoot,
	}); err == nil {
		t.Fatalf("expected error when runtime is nil")
	}
}

func TestNewManagerRequiresStateFS(t *testing.T) {
	queueStore := newQueueStore(t)
	stateRoot := filepath.Join(t.TempDir(), "state-store")
	if _, err := NewPrepareService(Options{
		Store:          &fakeStore{},
		Queue:          queueStore,
		Runtime:        &fakeRuntime{},
		DBMS:           &fakeDBMS{},
		StateStoreRoot: stateRoot,
	}); err == nil {
		t.Fatalf("expected error when statefs is nil")
	}
}

func TestNewManagerRequiresDBMS(t *testing.T) {
	queueStore := newQueueStore(t)
	stateRoot := filepath.Join(t.TempDir(), "state-store")
	if _, err := NewPrepareService(Options{
		Store:          &fakeStore{},
		Queue:          queueStore,
		Runtime:        &fakeRuntime{},
		StateFS:        &fakeStateFS{},
		StateStoreRoot: stateRoot,
	}); err == nil {
		t.Fatalf("expected error when dbms connector is nil")
	}
}

func TestNewManagerRequiresStateStoreRoot(t *testing.T) {
	queueStore := newQueueStore(t)
	if _, err := NewPrepareService(Options{
		Store:   &fakeStore{},
		Queue:   queueStore,
		Runtime: &fakeRuntime{},
		StateFS: &fakeStateFS{},
		DBMS:    &fakeDBMS{},
	}); err == nil {
		t.Fatalf("expected error when state store root is empty")
	}
}

func TestSubmitRejectsInvalidKind(t *testing.T) {
	mgr := newManager(t, &fakeStore{})

	_, err := mgr.Submit(context.Background(), Request{PrepareKind: "", ImageID: "img"})
	expectValidationError(t, err, "prepare_kind is required")

	_, err = mgr.Submit(context.Background(), Request{PrepareKind: "lb", ImageID: "img", LiquibaseArgs: []string{"update"}})
	if err != nil {
		t.Fatalf("expected lb to be accepted, got %v", err)
	}

	_, err = mgr.Submit(context.Background(), Request{PrepareKind: "unknown", ImageID: "img"})
	expectValidationError(t, err, "unsupported prepare_kind")
}

func TestSubmitRejectsMissingImageID(t *testing.T) {
	mgr := newManager(t, &fakeStore{})

	_, err := mgr.Submit(context.Background(), Request{PrepareKind: "psql"})
	expectValidationError(t, err, "image_id is required")
}

func TestSubmitIDGenFails(t *testing.T) {
	queueStore := newQueueStore(t)
	mgr, err := NewPrepareService(Options{
		Store:          &fakeStore{},
		Queue:          queueStore,
		Runtime:        &fakeRuntime{},
		StateFS:        &fakeStateFS{},
		DBMS:           &fakeDBMS{},
		StateStoreRoot: filepath.Join(t.TempDir(), "state-store"),
		Version:        "v1",
		IDGen: func() (string, error) {
			return "", errors.New("boom")
		},
		Psql: &fakePsqlRunner{},
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}

	if _, err := mgr.Submit(context.Background(), Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestSubmitCreateJobFails(t *testing.T) {
	queueStore := newQueueStore(t)
	faulty := &faultQueueStore{
		Store: queueStore,
		createJob: func(context.Context, queue.JobRecord) error {
			return errors.New("boom")
		},
	}
	mgr := newManagerWithQueue(t, &fakeStore{}, faulty)

	if _, err := mgr.Submit(context.Background(), Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestSubmitStoresStateAndInstance(t *testing.T) {
	store := &fakeStore{}
	mgr := newManager(t, store)

	accepted, err := mgr.Submit(context.Background(), Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if accepted.JobID == "" || accepted.StatusURL == "" || accepted.EventsURL == "" {
		t.Fatalf("unexpected accepted: %+v", accepted)
	}
	if len(store.states) != 1 || len(store.instances) != 1 {
		t.Fatalf("expected state and instance to be stored")
	}

	status, ok := mgr.Get(accepted.JobID)
	if !ok {
		t.Fatalf("expected job to exist")
	}
	if status.Status != StatusSucceeded || status.Result == nil {
		t.Fatalf("unexpected status: %+v", status)
	}

	tasks := mgr.ListTasks(accepted.JobID)
	if len(tasks) != 4 {
		t.Fatalf("expected 4 tasks, got %d", len(tasks))
	}
	for _, task := range tasks {
		if task.Status != StatusSucceeded {
			t.Fatalf("expected succeeded tasks, got %+v", tasks)
		}
	}

	events, ok, done, err := mgr.EventsSince(accepted.JobID, 0)
	if err != nil || !ok || !done || len(events) == 0 {
		t.Fatalf("unexpected events: ok=%v done=%v err=%v len=%d", ok, done, err, len(events))
	}
	foundTask := false
	for _, event := range events {
		if event.Type == "task" && event.TaskID != "" {
			foundTask = true
		}
	}
	if !foundTask {
		t.Fatalf("expected task events, got %+v", events)
	}
}

func TestSubmitFailsWhenStoreNotReady(t *testing.T) {
	queueStore := newQueueStore(t)
	deps := &testDeps{
		validate: func(root string) error {
			return errors.New("missing mount")
		},
	}
	mgr := newManagerWithDeps(t, &fakeStore{}, queueStore, deps)

	accepted, err := mgr.Submit(context.Background(), Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	status, ok := mgr.Get(accepted.JobID)
	if !ok {
		t.Fatalf("expected job to exist")
	}
	if status.Status != StatusFailed {
		t.Fatalf("expected failed status, got %s", status.Status)
	}
	if status.Error == nil || status.Error.Message != "state store not ready" {
		t.Fatalf("expected store not ready error, got %+v", status.Error)
	}
}

func TestSubmitEmitsPsqlOutputLog(t *testing.T) {
	store := &fakeStore{}
	queueStore := newQueueStore(t)
	psql := &fakePsqlRunner{output: "psql output line"}
	mgr := newManagerWithDeps(t, store, queueStore, &testDeps{psql: psql})

	accepted, err := mgr.Submit(context.Background(), Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	events, ok, done, err := mgr.EventsSince(accepted.JobID, 0)
	if err != nil || !ok || !done {
		t.Fatalf("unexpected events: ok=%v done=%v err=%v", ok, done, err)
	}
	found := false
	for _, event := range events {
		if event.Type == "log" && strings.Contains(event.Message, "psql: psql output line") {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("expected psql log event, got %+v", events)
	}
}

func TestSubmitSkipsPsqlOutputReplayWhenLogSinkStreams(t *testing.T) {
	store := &fakeStore{}
	queueStore := newQueueStore(t)
	psql := &streamingPsqlRunner{output: "psql output line"}
	mgr := newManagerWithDeps(t, store, queueStore, &testDeps{psql: psql})

	accepted, err := mgr.Submit(context.Background(), Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	events, ok, done, err := mgr.EventsSince(accepted.JobID, 0)
	if err != nil || !ok || !done {
		t.Fatalf("unexpected events: ok=%v done=%v err=%v", ok, done, err)
	}
	foundStream := false
	foundOutput := false
	for _, event := range events {
		if event.Type != "log" {
			continue
		}
		if strings.Contains(event.Message, "psql: streamed line") {
			foundStream = true
		}
		if strings.Contains(event.Message, "psql: psql output line") {
			foundOutput = true
		}
	}
	if !foundStream {
		t.Fatalf("expected streamed psql log event, got %+v", events)
	}
	if foundOutput {
		t.Fatalf("expected output replay to be skipped, got %+v", events)
	}
}

func TestSubmitPlanOnlyBuildsTasks(t *testing.T) {
	store := &fakeStore{}
	mgr := newManager(t, store)

	accepted, err := mgr.Submit(context.Background(), Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
		PlanOnly:    true,
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if len(store.states) != 0 || len(store.instances) != 0 {
		t.Fatalf("expected no state or instance creation")
	}

	status, ok := mgr.Get(accepted.JobID)
	if !ok {
		t.Fatalf("expected job to exist")
	}
	if status.Status != StatusSucceeded || !status.PlanOnly {
		t.Fatalf("unexpected status: %+v", status)
	}
	if status.Result != nil {
		t.Fatalf("expected no result for plan-only, got %+v", status.Result)
	}
	if status.PrepareArgsNormalized == "" {
		t.Fatalf("expected normalized args")
	}
	if len(status.Tasks) != 4 {
		t.Fatalf("expected 4 tasks, got %d", len(status.Tasks))
	}

	tasks := mgr.ListTasks(accepted.JobID)
	for _, task := range tasks {
		if task.Status != StatusSucceeded {
			t.Fatalf("expected succeeded tasks, got %+v", tasks)
		}
	}
}

func TestSubmitPlanOnlyRunsLiquibasePlanner(t *testing.T) {
	store := &fakeStore{}
	queueStore := newQueueStore(t)
	liquibase := &fakeLiquibaseRunner{}
	mgr := newManagerWithDeps(t, store, queueStore, &testDeps{liquibase: liquibase})

	accepted, err := mgr.Submit(context.Background(), Request{
		PrepareKind:   "lb",
		ImageID:       "image-1",
		LiquibaseArgs: []string{"update"},
		PlanOnly:      true,
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if accepted.JobID == "" {
		t.Fatalf("expected job id")
	}
	if len(liquibase.runs) != 1 {
		t.Fatalf("expected liquibase runner to be used for planning, got %+v", liquibase.runs)
	}
	if !containsArg(liquibase.runs[0].Args, "updateSQL") {
		t.Fatalf("expected updateSQL for planning, got %+v", liquibase.runs[0].Args)
	}
	status, ok := mgr.Get(accepted.JobID)
	if !ok || status.Status != StatusSucceeded || !status.PlanOnly {
		t.Fatalf("unexpected status: %+v", status)
	}
	if len(status.Tasks) == 0 {
		t.Fatalf("expected tasks for plan-only")
	}
}

func TestSubmitPlanOnlyCachedFlag(t *testing.T) {
	fake := &fakeStore{statesByID: map[string]store.StateEntry{}}
	mgr := newManager(t, fake)

	prepared, err := mgr.prepareRequest(Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	})
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	if errResp := mgr.ensureResolvedImageID(context.Background(), "job-1", &prepared, nil); errResp != nil {
		t.Fatalf("ensureResolvedImageID: %+v", errResp)
	}
	taskHash, errResp := mgr.computeTaskHash(prepared)
	if errResp != nil {
		t.Fatalf("computeTaskHash: %+v", errResp)
	}
	stateID, errResp := mgr.computeOutputStateID("image", prepared.effectiveImageID(), taskHash)
	if errResp != nil {
		t.Fatalf("computeOutputStateID: %+v", errResp)
	}
	fake.statesByID[stateID] = store.StateEntry{StateID: stateID}

	if _, err := mgr.Submit(context.Background(), Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
		PlanOnly:    true,
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	status, ok := mgr.Get("job-1")
	if !ok {
		t.Fatalf("expected job to exist")
	}
	if len(status.Tasks) < 3 || status.Tasks[2].Cached == nil || !*status.Tasks[2].Cached {
		t.Fatalf("expected cached true, got %+v", status.Tasks)
	}
}

func TestPrepareResolvesImageDigestRequired(t *testing.T) {
	runtime := &fakeRuntime{}
	store := &fakeStore{}
	mgr := newManagerWithDeps(t, store, newQueueStore(t), &testDeps{runtime: runtime})

	if _, err := mgr.Submit(context.Background(), Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
		PlanOnly:    true,
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if len(runtime.resolveCalls) != 1 || runtime.resolveCalls[0] != "image-1" {
		t.Fatalf("expected resolve call for image-1, got %+v", runtime.resolveCalls)
	}
}

func TestPrepareResolveImageFailureFailsJob(t *testing.T) {
	runtime := &fakeRuntime{resolveErr: errors.New("boom")}
	store := &fakeStore{}
	mgr := newManagerWithDeps(t, store, newQueueStore(t), &testDeps{runtime: runtime})

	if _, err := mgr.Submit(context.Background(), Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	status, ok := mgr.Get("job-1")
	if !ok || status.Status != StatusFailed || status.Error == nil || status.Error.Message != "cannot resolve image" {
		t.Fatalf("unexpected status: %+v", status)
	}
	if len(store.states) != 0 || len(store.instances) != 0 {
		t.Fatalf("expected no state or instance creation")
	}
	if tasks := mgr.ListTasks("job-1"); len(tasks) != 0 {
		t.Fatalf("expected no tasks, got %+v", tasks)
	}
}

func TestPlanIncludesResolveImageTask(t *testing.T) {
	mgr := newManagerWithDeps(t, &fakeStore{}, newQueueStore(t), &testDeps{runtime: &fakeRuntime{}})

	if _, err := mgr.Submit(context.Background(), Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
		PlanOnly:    true,
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	tasks := mgr.ListTasks("job-1")
	if len(tasks) != 4 {
		t.Fatalf("expected 4 tasks, got %d", len(tasks))
	}
	found := false
	for _, task := range tasks {
		if task.Type == "resolve_image" {
			found = true
			if task.ImageID != "image-1" || task.ResolvedImageID == "" {
				t.Fatalf("unexpected resolve task: %+v", task)
			}
		}
	}
	if !found {
		t.Fatalf("expected resolve_image task, got %+v", tasks)
	}
}

func TestPlanSkipsResolveImageTaskWhenDigestProvided(t *testing.T) {
	runtime := &fakeRuntime{}
	mgr := newManagerWithDeps(t, &fakeStore{}, newQueueStore(t), &testDeps{runtime: runtime})
	imageID := "image-1@sha256:abc"

	if _, err := mgr.Submit(context.Background(), Request{
		PrepareKind: "psql",
		ImageID:     imageID,
		PsqlArgs:    []string{"-c", "select 1"},
		PlanOnly:    true,
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	tasks := mgr.ListTasks("job-1")
	if len(tasks) != 3 {
		t.Fatalf("expected 3 tasks, got %d", len(tasks))
	}
	for _, task := range tasks {
		if task.Type == "resolve_image" {
			t.Fatalf("unexpected resolve_image task: %+v", tasks)
		}
	}
	if len(runtime.resolveCalls) != 0 {
		t.Fatalf("expected no resolve calls, got %+v", runtime.resolveCalls)
	}
}

func TestStateIDUsesResolvedImageID(t *testing.T) {
	runtime := &fakeRuntime{resolvedImage: "image-1@sha256:resolved"}
	store := &fakeStore{}
	mgr := newManagerWithDeps(t, store, newQueueStore(t), &testDeps{runtime: runtime})

	req := Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	}
	prepared, err := mgr.prepareRequest(req)
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	if errResp := mgr.ensureResolvedImageID(context.Background(), "job-1", &prepared, nil); errResp != nil {
		t.Fatalf("ensureResolvedImageID: %+v", errResp)
	}
	taskHash, errResp := mgr.computeTaskHash(prepared)
	if errResp != nil {
		t.Fatalf("computeTaskHash: %+v", errResp)
	}
	expectedStateID, errResp := mgr.computeOutputStateID("image", prepared.effectiveImageID(), taskHash)
	if errResp != nil {
		t.Fatalf("computeOutputStateID: %+v", errResp)
	}

	if _, err := mgr.Submit(context.Background(), req); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	if len(store.states) != 1 {
		t.Fatalf("expected one state, got %+v", store.states)
	}
	if store.states[0].StateID != expectedStateID {
		t.Fatalf("unexpected state id: %s", store.states[0].StateID)
	}
	if store.states[0].ImageID != prepared.effectiveImageID() {
		t.Fatalf("unexpected image id: %s", store.states[0].ImageID)
	}
}

func TestResolveImageTaskStatusReported(t *testing.T) {
	mgr := newManagerWithDeps(t, &fakeStore{}, newQueueStore(t), &testDeps{runtime: &fakeRuntime{}})

	if _, err := mgr.Submit(context.Background(), Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
		PlanOnly:    true,
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}
	tasks := mgr.ListTasks("job-1")
	for _, task := range tasks {
		if task.Type == "resolve_image" {
			if task.Status != StatusSucceeded {
				t.Fatalf("expected resolve_image succeeded, got %+v", task)
			}
			if task.ImageID != "image-1" || task.ResolvedImageID == "" {
				t.Fatalf("unexpected resolve task data: %+v", task)
			}
			return
		}
	}
	t.Fatalf("resolve_image task missing: %+v", tasks)
}

func TestListJobsAndTasks(t *testing.T) {
	mgr := newManager(t, &fakeStore{})

	_, err := mgr.Submit(context.Background(), Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
		PlanOnly:    true,
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	jobs := mgr.ListJobs("")
	if len(jobs) != 1 || jobs[0].JobID != "job-1" {
		t.Fatalf("unexpected jobs: %+v", jobs)
	}

	tasks := mgr.ListTasks("job-1")
	if len(tasks) != 4 {
		t.Fatalf("expected 4 tasks, got %d", len(tasks))
	}
}

func TestListJobsErrorReturnsEmpty(t *testing.T) {
	queueStore := newQueueStore(t)
	faulty := &faultQueueStore{
		Store: queueStore,
		listJobs: func(context.Context, string) ([]queue.JobRecord, error) {
			return nil, errors.New("boom")
		},
	}
	mgr := newManagerWithQueue(t, &fakeStore{}, faulty)

	if jobs := mgr.ListJobs(""); len(jobs) != 0 {
		t.Fatalf("expected empty jobs, got %+v", jobs)
	}
}

func TestListTasksErrorReturnsEmpty(t *testing.T) {
	queueStore := newQueueStore(t)
	faulty := &faultQueueStore{
		Store: queueStore,
		listTasks: func(context.Context, string) ([]queue.TaskRecord, error) {
			return nil, errors.New("boom")
		},
	}
	mgr := newManagerWithQueue(t, &fakeStore{}, faulty)

	if tasks := mgr.ListTasks("job-1"); len(tasks) != 0 {
		t.Fatalf("expected empty tasks, got %+v", tasks)
	}
}

func TestGetListTasksErrorReturnsJob(t *testing.T) {
	queueStore := newQueueStore(t)
	faulty := &faultQueueStore{
		Store: queueStore,
		listTasks: func(context.Context, string) ([]queue.TaskRecord, error) {
			return nil, errors.New("boom")
		},
	}
	mgr := newManagerWithQueue(t, &fakeStore{}, faulty)
	req := Request{PrepareKind: "psql", ImageID: "image-1"}
	createJobRecord(t, queueStore, "job-1", req, StatusQueued)

	status, ok := mgr.Get("job-1")
	if !ok {
		t.Fatalf("expected job to exist")
	}
	if status.JobID != "job-1" || status.Status != StatusQueued {
		t.Fatalf("unexpected status: %+v", status)
	}
	if len(status.Tasks) != 0 {
		t.Fatalf("expected empty tasks, got %+v", status.Tasks)
	}
}

func TestDeleteJobRemovesEntry(t *testing.T) {
	mgr := newManager(t, &fakeStore{})

	_, err := mgr.Submit(context.Background(), Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
		PlanOnly:    true,
	})
	if err != nil {
		t.Fatalf("Submit: %v", err)
	}

	jobDir := filepath.Join(mgr.stateStoreRoot, "jobs", "job-1")
	if err := os.MkdirAll(jobDir, 0o700); err != nil {
		t.Fatalf("mkdir job dir: %v", err)
	}

	result, ok := mgr.Delete("job-1", deletion.DeleteOptions{})
	if !ok || result.Outcome != deletion.OutcomeDeleted {
		t.Fatalf("unexpected delete result: ok=%v result=%+v", ok, result)
	}
	if len(mgr.ListJobs("")) != 0 {
		t.Fatalf("expected jobs to be removed")
	}
	if _, err := os.Stat(jobDir); !os.IsNotExist(err) {
		t.Fatalf("expected job dir to be removed")
	}
}

func TestDeleteJobBlockedWithoutForce(t *testing.T) {
	queueStore := newQueueStore(t)
	mgr := newManagerWithQueue(t, &fakeStore{}, queueStore)

	if _, err := mgr.Submit(context.Background(), Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
		PlanOnly:    true,
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	status := StatusRunning
	if err := queueStore.UpdateTask(context.Background(), "job-1", "plan", queue.TaskUpdate{
		Status: &status,
	}); err != nil {
		t.Fatalf("UpdateTask: %v", err)
	}

	result, ok := mgr.Delete("job-1", deletion.DeleteOptions{})
	if !ok || result.Outcome != deletion.OutcomeBlocked || result.Root.Blocked != deletion.BlockActiveTasks {
		t.Fatalf("unexpected blocked result: ok=%v result=%+v", ok, result)
	}
}

func TestJobSignatureDiffersForPlanOnly(t *testing.T) {
	mgr := newManager(t, &fakeStore{})

	req := Request{
		PrepareKind: "psql",
		ImageID:     "image-1@sha256:abc",
		PsqlArgs:    []string{"-c", "select 1"},
	}
	prepared, err := mgr.prepareRequest(req)
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	signatureExecute, errResp := mgr.computeJobSignature(prepared)
	if errResp != nil {
		t.Fatalf("computeJobSignature: %+v", errResp)
	}

	req.PlanOnly = true
	preparedPlan, err := mgr.prepareRequest(req)
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	signaturePlan, errResp := mgr.computeJobSignature(preparedPlan)
	if errResp != nil {
		t.Fatalf("computeJobSignature: %+v", errResp)
	}

	if signatureExecute == signaturePlan {
		t.Fatalf("expected different signatures for plan_only")
	}
}

func TestUpdateJobSignatureError(t *testing.T) {
	queueStore := newQueueStore(t)
	faulty := &faultQueueStore{
		Store: queueStore,
		updateJob: func(context.Context, string, queue.JobUpdate) error {
			return errors.New("boom")
		},
	}
	mgr := newManagerWithQueue(t, &fakeStore{}, faulty)
	prepared, err := mgr.prepareRequest(Request{
		PrepareKind: "psql",
		ImageID:     "image-1@sha256:abc",
		PsqlArgs:    []string{"-c", "select 1"},
	})
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	if errResp := mgr.updateJobSignature(context.Background(), "job-1", prepared); errResp == nil {
		t.Fatalf("expected update error")
	}
}

func TestTrimCompletedJobsBySignature(t *testing.T) {
	queueStore := newQueueStore(t)
	mgr := newManagerWithQueue(t, &fakeStore{}, queueStore)

	req := Request{
		PrepareKind: "psql",
		ImageID:     "image-1@sha256:abc",
		PsqlArgs:    []string{"-c", "select 1"},
	}
	prepared, err := mgr.prepareRequest(req)
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	signature, errResp := mgr.computeJobSignature(prepared)
	if errResp != nil {
		t.Fatalf("computeJobSignature: %+v", errResp)
	}

	olderFinished := "2026-01-19T00:01:00Z"
	newerFinished := "2026-01-19T00:02:00Z"
	newestFinished := "2026-01-19T00:03:00Z"
	jobs := []queue.JobRecord{
		{JobID: "job-old", Status: StatusSucceeded, PrepareKind: "psql", ImageID: req.ImageID, Signature: &signature, CreatedAt: "2026-01-19T00:00:00Z", FinishedAt: &olderFinished},
		{JobID: "job-mid", Status: StatusSucceeded, PrepareKind: "psql", ImageID: req.ImageID, Signature: &signature, CreatedAt: "2026-01-19T00:00:00Z", FinishedAt: &newerFinished},
		{JobID: "job-new", Status: StatusSucceeded, PrepareKind: "psql", ImageID: req.ImageID, Signature: &signature, CreatedAt: "2026-01-19T00:00:00Z", FinishedAt: &newestFinished},
	}
	for _, job := range jobs {
		if err := queueStore.CreateJob(context.Background(), job); err != nil {
			t.Fatalf("CreateJob %s: %v", job.JobID, err)
		}
		dir := filepath.Join(mgr.stateStoreRoot, "jobs", job.JobID)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("mkdir job dir: %v", err)
		}
	}

	mgr.trimCompletedJobs(context.Background(), prepared)

	remaining, err := queueStore.ListJobsBySignature(context.Background(), signature, []string{StatusSucceeded})
	if err != nil {
		t.Fatalf("ListJobsBySignature: %v", err)
	}
	if len(remaining) != 2 || remaining[0].JobID != "job-new" || remaining[1].JobID != "job-mid" {
		t.Fatalf("unexpected remaining jobs: %+v", remaining)
	}
	if _, err := os.Stat(filepath.Join(mgr.stateStoreRoot, "jobs", "job-old")); !os.IsNotExist(err) {
		t.Fatalf("expected old job dir to be removed")
	}
}

func TestTrimCompletedJobsForJob(t *testing.T) {
	queueStore := newQueueStore(t)
	mgr := newManagerWithQueue(t, &fakeStore{}, queueStore)

	req := Request{
		PrepareKind: "psql",
		ImageID:     "image-1@sha256:abc",
		PsqlArgs:    []string{"-c", "select 1"},
	}
	prepared, err := mgr.prepareRequest(req)
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	signature, errResp := mgr.computeJobSignature(prepared)
	if errResp != nil {
		t.Fatalf("computeJobSignature: %+v", errResp)
	}

	olderFinished := "2026-01-19T00:01:00Z"
	newerFinished := "2026-01-19T00:02:00Z"
	newestFinished := "2026-01-19T00:03:00Z"
	jobs := []queue.JobRecord{
		{JobID: "job-old", Status: StatusSucceeded, PrepareKind: "psql", ImageID: req.ImageID, Signature: &signature, CreatedAt: "2026-01-19T00:00:00Z", FinishedAt: &olderFinished},
		{JobID: "job-mid", Status: StatusSucceeded, PrepareKind: "psql", ImageID: req.ImageID, Signature: &signature, CreatedAt: "2026-01-19T00:00:00Z", FinishedAt: &newerFinished},
		{JobID: "job-new", Status: StatusSucceeded, PrepareKind: "psql", ImageID: req.ImageID, Signature: &signature, CreatedAt: "2026-01-19T00:00:00Z", FinishedAt: &newestFinished},
	}
	for _, job := range jobs {
		if err := queueStore.CreateJob(context.Background(), job); err != nil {
			t.Fatalf("CreateJob %s: %v", job.JobID, err)
		}
		dir := filepath.Join(mgr.stateStoreRoot, "jobs", job.JobID)
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatalf("mkdir job dir: %v", err)
		}
	}

	mgr.trimCompletedJobsForJob(context.Background(), "job-new")

	remaining, err := queueStore.ListJobsBySignature(context.Background(), signature, []string{StatusSucceeded})
	if err != nil {
		t.Fatalf("ListJobsBySignature: %v", err)
	}
	if len(remaining) != 2 || remaining[0].JobID != "job-new" || remaining[1].JobID != "job-mid" {
		t.Fatalf("unexpected remaining jobs: %+v", remaining)
	}
	if _, err := os.Stat(filepath.Join(mgr.stateStoreRoot, "jobs", "job-old")); !os.IsNotExist(err) {
		t.Fatalf("expected old job dir to be removed")
	}
}

func TestTrimCompletedJobsForJobNoopCases(t *testing.T) {
	queueStore := newQueueStore(t)
	mgr := newManagerWithQueue(t, &fakeStore{}, queueStore)

	mgr.trimCompletedJobsForJob(context.Background(), "")

	req := Request{
		PrepareKind: "psql",
		ImageID:     "image-1@sha256:abc",
		PsqlArgs:    []string{"-c", "select 1"},
	}
	reqJSON, err := jsonMarshal(req)
	if err != nil {
		t.Fatalf("jsonMarshal: %v", err)
	}
	if err := queueStore.CreateJob(context.Background(), queue.JobRecord{
		JobID:       "job-1",
		Status:      StatusSucceeded,
		PrepareKind: req.PrepareKind,
		ImageID:     req.ImageID,
		PlanOnly:    req.PlanOnly,
		RequestJSON: &reqJSON,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		Signature:   nil,
	}); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	mgr.trimCompletedJobsForJob(context.Background(), "missing")
	mgr.trimCompletedJobsForJob(context.Background(), "job-1")
}

func TestTrimCompletedJobsBySignatureNoop(t *testing.T) {
	queueStore := newQueueStore(t)
	mgr := newManagerWithQueue(t, &fakeStore{}, queueStore)

	mgr.trimCompletedJobsBySignature(context.Background(), "")
	mgr.trimCompletedJobsBySignature(context.Background(), "   ")
}

func TestTrimCompletedJobsFallbackToCreatedAt(t *testing.T) {
	queueStore := newQueueStore(t)
	mgr := newManagerWithDeps(t, &fakeStore{}, queueStore, &testDeps{
		config: &fakeConfigStore{value: 1},
	})

	req := Request{
		PrepareKind: "psql",
		ImageID:     "image-1@sha256:abc",
		PsqlArgs:    []string{"-c", "select 1"},
	}
	prepared, err := mgr.prepareRequest(req)
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	signature, errResp := mgr.computeJobSignature(prepared)
	if errResp != nil {
		t.Fatalf("computeJobSignature: %+v", errResp)
	}

	jobOld := queue.JobRecord{
		JobID:       "job-old",
		Status:      StatusSucceeded,
		PrepareKind: "psql",
		ImageID:     req.ImageID,
		Signature:   &signature,
		CreatedAt:   "2026-01-19T00:00:00Z",
	}
	jobNew := queue.JobRecord{
		JobID:       "job-new",
		Status:      StatusSucceeded,
		PrepareKind: "psql",
		ImageID:     req.ImageID,
		Signature:   &signature,
		CreatedAt:   "2026-01-19T00:01:00Z",
	}
	if err := queueStore.CreateJob(context.Background(), jobOld); err != nil {
		t.Fatalf("CreateJob old: %v", err)
	}
	if err := queueStore.CreateJob(context.Background(), jobNew); err != nil {
		t.Fatalf("CreateJob new: %v", err)
	}

	mgr.trimCompletedJobs(context.Background(), prepared)

	remaining, err := queueStore.ListJobsBySignature(context.Background(), signature, []string{StatusSucceeded})
	if err != nil {
		t.Fatalf("ListJobsBySignature: %v", err)
	}
	if len(remaining) != 1 || remaining[0].JobID != "job-new" {
		t.Fatalf("unexpected remaining jobs: %+v", remaining)
	}
}

func TestTrimCompletedJobsNoopWhenLimitDisabled(t *testing.T) {
	queueStore := newQueueStore(t)
	mgr := newManagerWithDeps(t, &fakeStore{}, queueStore, &testDeps{
		config: &fakeConfigStore{value: 0},
	})
	req := Request{PrepareKind: "psql", ImageID: "image-1", PsqlArgs: []string{"-c", "select 1"}}
	createJobRecord(t, queueStore, "job-1", req, StatusSucceeded)

	prepared, err := mgr.prepareRequest(req)
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	mgr.trimCompletedJobs(context.Background(), prepared)

	if jobs := mgr.ListJobs(""); len(jobs) != 1 {
		t.Fatalf("expected jobs untouched, got %+v", jobs)
	}
}

func TestTrimCompletedJobsNoopBelowLimit(t *testing.T) {
	queueStore := newQueueStore(t)
	mgr := newManagerWithDeps(t, &fakeStore{}, queueStore, &testDeps{
		config: &fakeConfigStore{value: 3},
	})
	req := Request{PrepareKind: "psql", ImageID: "image-1", PsqlArgs: []string{"-c", "select 1"}}

	prepared, err := mgr.prepareRequest(req)
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	signature, errResp := mgr.computeJobSignature(prepared)
	if errResp != nil {
		t.Fatalf("computeJobSignature: %+v", errResp)
	}
	for _, jobID := range []string{"job-1", "job-2"} {
		if err := queueStore.CreateJob(context.Background(), queue.JobRecord{
			JobID:       jobID,
			Status:      StatusSucceeded,
			PrepareKind: req.PrepareKind,
			ImageID:     req.ImageID,
			Signature:   &signature,
			CreatedAt:   time.Now().UTC().Format(time.RFC3339Nano),
		}); err != nil {
			t.Fatalf("CreateJob %s: %v", jobID, err)
		}
	}
	mgr.trimCompletedJobs(context.Background(), prepared)

	remaining, err := queueStore.ListJobsBySignature(context.Background(), signature, []string{StatusSucceeded})
	if err != nil {
		t.Fatalf("ListJobsBySignature: %v", err)
	}
	if len(remaining) != 2 {
		t.Fatalf("expected jobs retained, got %+v", remaining)
	}
}

func TestHasImageDigest(t *testing.T) {
	if hasImageDigest("") {
		t.Fatalf("expected empty image to be false")
	}
	if hasImageDigest("postgres:15") {
		t.Fatalf("expected tag-only image to be false")
	}
	if hasImageDigest("postgres@") {
		t.Fatalf("expected trailing @ to be false")
	}
	if !hasImageDigest("postgres:15@sha256:abc") {
		t.Fatalf("expected digest to be true")
	}
}

func TestTrimCompletedJobsSkipsOnSignatureError(t *testing.T) {
	queueStore := newQueueStore(t)
	mgr := newManagerWithQueue(t, &fakeStore{}, queueStore)
	req := Request{PrepareKind: "psql", ImageID: "image-1", PsqlArgs: []string{"-c", "select 1"}}
	createJobRecord(t, queueStore, "job-1", req, StatusSucceeded)

	prepared := preparedRequest{request: Request{PrepareKind: "psql", ImageID: ""}}
	mgr.trimCompletedJobs(context.Background(), prepared)

	if jobs := mgr.ListJobs(""); len(jobs) != 1 {
		t.Fatalf("expected jobs untouched, got %+v", jobs)
	}
}

func TestTrimCompletedJobsListJobsError(t *testing.T) {
	queueStore := newQueueStore(t)
	wrapped := &signatureQueueStore{Store: queueStore, listErr: errors.New("boom")}
	mgr := newManagerWithQueue(t, &fakeStore{}, wrapped)

	prepared, err := mgr.prepareRequest(Request{
		PrepareKind: "psql",
		ImageID:     "image-1@sha256:abc",
		PsqlArgs:    []string{"-c", "select 1"},
	})
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}

	mgr.trimCompletedJobs(context.Background(), prepared)
}

func TestTrimCompletedJobsDeleteError(t *testing.T) {
	queueStore := newQueueStore(t)
	wrapped := &signatureQueueStore{Store: queueStore, deleteErr: errors.New("boom")}
	mgr := newManagerWithDeps(t, &fakeStore{}, wrapped, &testDeps{
		config: &fakeConfigStore{value: 1},
	})

	req := Request{
		PrepareKind: "psql",
		ImageID:     "image-1@sha256:abc",
		PsqlArgs:    []string{"-c", "select 1"},
	}
	prepared, err := mgr.prepareRequest(req)
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	signature, errResp := mgr.computeJobSignature(prepared)
	if errResp != nil {
		t.Fatalf("computeJobSignature: %+v", errResp)
	}

	jobOld := queue.JobRecord{
		JobID:       "job-old",
		Status:      StatusSucceeded,
		PrepareKind: "psql",
		ImageID:     req.ImageID,
		Signature:   &signature,
		CreatedAt:   "2026-01-19T00:00:00Z",
	}
	jobNew := queue.JobRecord{
		JobID:       "job-new",
		Status:      StatusSucceeded,
		PrepareKind: "psql",
		ImageID:     req.ImageID,
		Signature:   &signature,
		CreatedAt:   "2026-01-19T00:01:00Z",
	}
	if err := queueStore.CreateJob(context.Background(), jobOld); err != nil {
		t.Fatalf("CreateJob old: %v", err)
	}
	if err := queueStore.CreateJob(context.Background(), jobNew); err != nil {
		t.Fatalf("CreateJob new: %v", err)
	}

	mgr.trimCompletedJobs(context.Background(), prepared)

	remaining, err := queueStore.ListJobsBySignature(context.Background(), signature, []string{StatusSucceeded})
	if err != nil {
		t.Fatalf("ListJobsBySignature: %v", err)
	}
	if len(remaining) != 2 {
		t.Fatalf("expected delete failure to keep jobs, got %+v", remaining)
	}
}

func TestDeleteListTasksError(t *testing.T) {
	queueStore := newQueueStore(t)
	faulty := &faultQueueStore{
		Store: queueStore,
		listTasks: func(context.Context, string) ([]queue.TaskRecord, error) {
			return nil, errors.New("boom")
		},
	}
	mgr := newManagerWithQueue(t, &fakeStore{}, faulty)
	createJobRecord(t, queueStore, "job-1", Request{PrepareKind: "psql", ImageID: "image-1"}, StatusQueued)

	if _, ok := mgr.Delete("job-1", deletion.DeleteOptions{}); ok {
		t.Fatalf("expected delete to fail")
	}
}

func TestDeleteJobForceCancels(t *testing.T) {
	queueStore := newQueueStore(t)
	blocker := &cancelRuntime{started: make(chan struct{})}
	mgr := newManagerWithQueue(t, &fakeStore{}, queueStore)
	mgr.runtime = blocker
	mgr.async = true

	if _, err := mgr.Submit(context.Background(), Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	select {
	case <-blocker.started:
	case <-time.After(20 * time.Second):
		tasks, _ := queueStore.ListTasks(context.Background(), "job-1")
		t.Fatalf("timeout waiting for execute runtime start, tasks=%+v", tasks)
	}

	result, ok := mgr.Delete("job-1", deletion.DeleteOptions{Force: true})
	if !ok || result.Outcome != deletion.OutcomeDeleted {
		t.Fatalf("unexpected delete result: ok=%v result=%+v", ok, result)
	}
	if len(mgr.ListJobs("")) != 0 {
		t.Fatalf("expected job removed")
	}
}

func TestRecoverQueuedJob(t *testing.T) {
	queueStore := newQueueStore(t)
	req := Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	}
	reqJSON, err := jsonMarshal(req)
	if err != nil {
		t.Fatalf("jsonMarshal: %v", err)
	}
	if err := queueStore.CreateJob(context.Background(), queue.JobRecord{
		JobID:       "job-1",
		Status:      StatusQueued,
		PrepareKind: "psql",
		ImageID:     "image-1",
		PlanOnly:    false,
		RequestJSON: &reqJSON,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	store := &fakeStore{}
	mgr := newManagerWithQueue(t, store, queueStore)
	mgr.async = false

	if err := mgr.Recover(context.Background()); err != nil {
		t.Fatalf("Recover: %v", err)
	}
	status, ok := mgr.Get("job-1")
	if !ok || status.Status != StatusSucceeded || status.Result == nil {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestRecoverListJobsError(t *testing.T) {
	queueStore := newQueueStore(t)
	faulty := &faultQueueStore{
		Store: queueStore,
		listJobsByStatus: func(context.Context, []string) ([]queue.JobRecord, error) {
			return nil, errors.New("boom")
		},
	}
	mgr := newManagerWithQueue(t, &fakeStore{}, faulty)

	if err := mgr.Recover(context.Background()); err == nil {
		t.Fatalf("expected error")
	}
}

func TestRecoverMissingRequestMarksFailed(t *testing.T) {
	queueStore := newQueueStore(t)
	if err := queueStore.CreateJob(context.Background(), queue.JobRecord{
		JobID:       "job-1",
		Status:      StatusQueued,
		PrepareKind: "psql",
		ImageID:     "image-1",
		CreatedAt:   time.Now().UTC().Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	mgr := newManagerWithQueue(t, &fakeStore{}, queueStore)
	if err := mgr.Recover(context.Background()); err != nil {
		t.Fatalf("Recover: %v", err)
	}
	status, ok := mgr.Get("job-1")
	if !ok || status.Status != StatusFailed || status.Error == nil {
		t.Fatalf("expected failed job, got %+v", status)
	}
}

func TestPrepareFromJobInvalidJSON(t *testing.T) {
	mgr := newManager(t, &fakeStore{})

	if _, err := mgr.prepareFromJob(queue.JobRecord{RequestJSON: strPtr("{")}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestLoadOrPlanTasksPromotesRunningTasks(t *testing.T) {
	queueStore := newQueueStore(t)
	req := Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	}
	reqJSON, err := jsonMarshal(req)
	if err != nil {
		t.Fatalf("jsonMarshal: %v", err)
	}
	if err := queueStore.CreateJob(context.Background(), queue.JobRecord{
		JobID:       "job-1",
		Status:      StatusRunning,
		PrepareKind: "psql",
		ImageID:     "image-1",
		RequestJSON: &reqJSON,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	stateStore := &fakeStore{statesByID: map[string]store.StateEntry{}}
	mgr := newManagerWithQueue(t, stateStore, queueStore)
	tasks := []queue.TaskRecord{
		{
			JobID:    "job-1",
			TaskID:   "execute-0",
			Position: 0,
			Type:     "state_execute",
			Status:   StatusRunning,
		},
	}

	prepared, err := mgr.prepareRequest(req)
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	outputID := psqlOutputStateID(t, mgr, prepared, TaskInput{Kind: "image", ID: "image-1"})
	stateStore.statesByID[outputID] = store.StateEntry{StateID: outputID}
	tasks[0].OutputStateID = strPtr(outputID)
	if err := queueStore.ReplaceTasks(context.Background(), "job-1", tasks); err != nil {
		t.Fatalf("ReplaceTasks: %v", err)
	}
	states, _, errResp := mgr.loadOrPlanTasks(context.Background(), "job-1", prepared)
	if errResp != nil {
		t.Fatalf("loadOrPlanTasks: %+v", errResp)
	}
	if len(states) != 1 || states[0].Status != StatusSucceeded {
		t.Fatalf("unexpected task states: %+v", states)
	}
}

func TestLoadOrPlanTasksListError(t *testing.T) {
	queueStore := newQueueStore(t)
	faulty := &faultQueueStore{
		Store: queueStore,
		listTasks: func(context.Context, string) ([]queue.TaskRecord, error) {
			return nil, errors.New("boom")
		},
	}
	mgr := newManagerWithQueue(t, &fakeStore{}, faulty)

	prepared, err := mgr.prepareRequest(Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	})
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	if _, _, errResp := mgr.loadOrPlanTasks(context.Background(), "job-1", prepared); errResp == nil {
		t.Fatalf("expected error response")
	}
}

func TestLoadOrPlanTasksReplaceError(t *testing.T) {
	queueStore := newQueueStore(t)
	faulty := &faultQueueStore{
		Store: queueStore,
		replaceTasks: func(context.Context, string, []queue.TaskRecord) error {
			return errors.New("boom")
		},
	}
	mgr := newManagerWithQueue(t, &fakeStore{}, faulty)

	prepared, err := mgr.prepareRequest(Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	})
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	if _, _, errResp := mgr.loadOrPlanTasks(context.Background(), "job-1", prepared); errResp == nil {
		t.Fatalf("expected error response")
	}
}

func TestExecuteStateTaskCachedSkipsCreate(t *testing.T) {
	stateStore := &fakeStore{statesByID: map[string]store.StateEntry{}}
	mgr := newManager(t, stateStore)

	prepared, err := mgr.prepareRequest(Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	})
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	outputID := psqlOutputStateID(t, mgr, prepared, TaskInput{Kind: "image", ID: "image-1"})
	stateStore.statesByID[outputID] = store.StateEntry{StateID: outputID}
	paths, err := resolveStatePaths(mgr.stateStoreRoot, "image-1", outputID, mgr.statefs)
	if err != nil {
		t.Fatalf("resolveStatePaths: %v", err)
	}
	if err := os.MkdirAll(paths.stateDir, 0o700); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(paths.stateDir, "PG_VERSION"), []byte("17"), 0o600); err != nil {
		t.Fatalf("write PG_VERSION: %v", err)
	}
	task := taskState{
		PlanTask: PlanTask{
			TaskID:        "execute-0",
			Type:          "state_execute",
			OutputStateID: outputID,
			Input:         &TaskInput{Kind: "image", ID: "image-1"},
		},
	}
	if _, errResp := mgr.executeStateTask(context.Background(), "job-1", prepared, task); errResp != nil {
		t.Fatalf("executeStateTask: %+v", errResp)
	}
	if len(stateStore.states) != 0 {
		t.Fatalf("expected no state creation")
	}
}

func TestExecuteStateTaskCreateStateError(t *testing.T) {
	store := &fakeStore{createStateErr: errors.New("boom")}
	mgr := newManager(t, store)

	prepared, err := mgr.prepareRequest(Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	})
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	outputID := psqlOutputStateID(t, mgr, prepared, TaskInput{Kind: "image", ID: "image-1"})
	task := taskState{
		PlanTask: PlanTask{
			TaskID:        "execute-0",
			Type:          "state_execute",
			OutputStateID: outputID,
			Input:         &TaskInput{Kind: "image", ID: "image-1"},
		},
	}
	_, errResp := mgr.executeStateTask(context.Background(), "job-1", prepared, task)
	if errResp == nil || errResp.Code != "internal_error" {
		t.Fatalf("expected internal error, got %+v", errResp)
	}
}

func TestExecuteStateTaskCancelled(t *testing.T) {
	store := &cancelStateStore{}
	mgr := newManager(t, store)

	prepared, err := mgr.prepareRequest(Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	})
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	outputID := psqlOutputStateID(t, mgr, prepared, TaskInput{Kind: "state", ID: "parent-1"})
	task := taskState{
		PlanTask: PlanTask{
			TaskID:        "execute-0",
			Type:          "state_execute",
			OutputStateID: outputID,
			Input:         &TaskInput{Kind: "state", ID: "parent-1"},
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, errResp := mgr.executeStateTask(ctx, "job-1", prepared, task)
	if errResp == nil || errResp.Code != "cancelled" {
		t.Fatalf("expected cancelled error, got %+v", errResp)
	}
}

func TestExecuteStateTaskPsqlError(t *testing.T) {
	store := &fakeStore{}
	psql := &fakePsqlRunner{err: errors.New("boom"), output: "psql failed"}
	mgr := newManagerWithDeps(t, store, newQueueStore(t), &testDeps{psql: psql})

	prepared, err := mgr.prepareRequest(Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	})
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	outputID := psqlOutputStateID(t, mgr, prepared, TaskInput{Kind: "image", ID: "image-1"})
	task := taskState{
		PlanTask: PlanTask{
			TaskID:        "execute-0",
			Type:          "state_execute",
			OutputStateID: outputID,
			Input:         &TaskInput{Kind: "image", ID: "image-1"},
		},
	}
	_, errResp := mgr.executeStateTask(context.Background(), "job-1", prepared, task)
	if errResp == nil || errResp.Code != "internal_error" {
		t.Fatalf("expected internal error, got %+v", errResp)
	}
}

func TestExecuteStateTaskSnapshotError(t *testing.T) {
	store := &fakeStore{}
	snap := &fakeStateFS{snapshotErr: errors.New("boom")}
	mgr := newManagerWithDeps(t, store, newQueueStore(t), &testDeps{statefs: snap})

	prepared, err := mgr.prepareRequest(Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	})
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	outputID := psqlOutputStateID(t, mgr, prepared, TaskInput{Kind: "image", ID: "image-1"})
	task := taskState{
		PlanTask: PlanTask{
			TaskID:        "execute-0",
			Type:          "state_execute",
			OutputStateID: outputID,
			Input:         &TaskInput{Kind: "image", ID: "image-1"},
		},
	}
	_, errResp := mgr.executeStateTask(context.Background(), "job-1", prepared, task)
	if errResp == nil || errResp.Code != "internal_error" {
		t.Fatalf("expected internal error, got %+v", errResp)
	}
}

func TestExecuteStateTaskMissingOutputState(t *testing.T) {
	store := &fakeStore{}
	mgr := newManager(t, store)
	prepared, err := mgr.prepareRequest(Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	})
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	task := taskState{
		PlanTask: PlanTask{
			TaskID: "execute-0",
			Type:   "state_execute",
			Input:  &TaskInput{Kind: "image", ID: "image-1"},
		},
	}
	outputID, errResp := mgr.executeStateTask(context.Background(), "job-1", prepared, task)
	if errResp != nil {
		t.Fatalf("executeStateTask: %+v", errResp)
	}
	if strings.TrimSpace(outputID) == "" {
		t.Fatalf("expected output state id")
	}
	if len(store.states) != 1 {
		t.Fatalf("expected state to be stored, got %+v", store.states)
	}
}

func TestExecuteStateTaskMissingInput(t *testing.T) {
	store := &fakeStore{}
	mgr := newManager(t, store)
	prepared, err := mgr.prepareRequest(Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	})
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	task := taskState{
		PlanTask: PlanTask{
			TaskID:        "execute-0",
			Type:          "state_execute",
			OutputStateID: "state-1",
		},
	}
	_, errResp := mgr.executeStateTask(context.Background(), "job-1", prepared, task)
	if errResp == nil || errResp.Code != "internal_error" {
		t.Fatalf("expected internal error, got %+v", errResp)
	}
}

func TestExecuteStateTaskPrepareSnapshotError(t *testing.T) {
	store := &fakeStore{}
	connector := &fakeDBMS{prepareErr: errors.New("boom")}
	mgr := newManagerWithDeps(t, store, newQueueStore(t), &testDeps{dbms: connector})

	prepared, err := mgr.prepareRequest(Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	})
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	outputID := psqlOutputStateID(t, mgr, prepared, TaskInput{Kind: "image", ID: "image-1"})
	task := taskState{
		PlanTask: PlanTask{
			TaskID:        "execute-0",
			Type:          "state_execute",
			OutputStateID: outputID,
			Input:         &TaskInput{Kind: "image", ID: "image-1"},
		},
	}
	_, errResp := mgr.executeStateTask(context.Background(), "job-1", prepared, task)
	if errResp == nil || errResp.Code != "internal_error" {
		t.Fatalf("expected internal error, got %+v", errResp)
	}
}

func TestExecuteStateTaskResumeSnapshotError(t *testing.T) {
	store := &fakeStore{}
	connector := &fakeDBMS{resumeErr: errors.New("boom")}
	mgr := newManagerWithDeps(t, store, newQueueStore(t), &testDeps{dbms: connector})

	prepared, err := mgr.prepareRequest(Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	})
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	outputID := psqlOutputStateID(t, mgr, prepared, TaskInput{Kind: "image", ID: "image-1"})
	task := taskState{
		PlanTask: PlanTask{
			TaskID:        "execute-0",
			Type:          "state_execute",
			OutputStateID: outputID,
			Input:         &TaskInput{Kind: "image", ID: "image-1"},
		},
	}
	_, errResp := mgr.executeStateTask(context.Background(), "job-1", prepared, task)
	if errResp == nil || errResp.Code != "internal_error" {
		t.Fatalf("expected internal error, got %+v", errResp)
	}
}

func TestCreateInstanceMissingState(t *testing.T) {
	store := &fakeStore{}
	mgr := newManager(t, store)
	prepared, err := mgr.prepareRequest(Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	})
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	if _, errResp := mgr.createInstance(context.Background(), "job-1", prepared, "state-missing"); errResp == nil || errResp.Code != "internal_error" {
		t.Fatalf("expected internal error, got %+v", errResp)
	}
}

func TestCreateInstanceMissingStateID(t *testing.T) {
	store := &fakeStore{}
	mgr := newManager(t, store)
	prepared, err := mgr.prepareRequest(Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	})
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	if _, errResp := mgr.createInstance(context.Background(), "job-1", prepared, ""); errResp == nil || errResp.Code != "internal_error" {
		t.Fatalf("expected internal error, got %+v", errResp)
	}
}

func TestStartRuntimeFailsWhenStateDirty(t *testing.T) {
	store := &fakeStore{statesByID: map[string]store.StateEntry{
		"state-1": {StateID: "state-1", ImageID: "image-1"},
	}}
	mgr := newManagerWithStateFS(t, store, &fakeStateFS{})
	paths, err := resolveStatePaths(mgr.stateStoreRoot, "image-1", "state-1", mgr.statefs)
	if err != nil {
		t.Fatalf("resolveStatePaths: %v", err)
	}
	if err := os.MkdirAll(paths.stateDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(paths.stateDir, "postmaster.pid"), []byte("123"), 0o600); err != nil {
		t.Fatalf("write postmaster.pid: %v", err)
	}
	_, errResp := mgr.startRuntime(context.Background(), "job-1", preparedRequest{
		request: Request{
			PrepareKind: "psql",
			ImageID:     "image-1",
		},
	}, &TaskInput{Kind: "state", ID: "state-1"})
	if errResp == nil {
		t.Fatalf("expected error")
	}
	if errResp.Code != "internal_error" {
		t.Fatalf("expected internal_error, got %+v", errResp)
	}
}

func TestStartRuntimeFailsWhenStateMissingPGVersion(t *testing.T) {
	store := &fakeStore{statesByID: map[string]store.StateEntry{
		"state-1": {StateID: "state-1", ImageID: "image-1"},
	}}
	mgr := newManagerWithStateFS(t, store, &fakeStateFS{})
	paths, err := resolveStatePaths(mgr.stateStoreRoot, "image-1", "state-1", mgr.statefs)
	if err != nil {
		t.Fatalf("resolveStatePaths: %v", err)
	}
	if err := os.MkdirAll(paths.stateDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_, errResp := mgr.startRuntime(context.Background(), "job-1", preparedRequest{
		request: Request{
			PrepareKind: "psql",
			ImageID:     "image-1",
		},
	}, &TaskInput{Kind: "state", ID: "state-1"})
	if errResp == nil {
		t.Fatalf("expected error")
	}
	if errResp.Code != "internal_error" {
		t.Fatalf("expected internal_error, got %+v", errResp)
	}
}

func TestStartRuntimeFailsWhenRuntimeDirDirty(t *testing.T) {
	store := &fakeStore{statesByID: map[string]store.StateEntry{
		"state-1": {StateID: "state-1", ImageID: "image-1"},
	}}
	mountDir := filepath.Join(t.TempDir(), "mount")
	if err := os.MkdirAll(mountDir, 0o700); err != nil {
		t.Fatalf("mkdir mount: %v", err)
	}
	if err := os.WriteFile(filepath.Join(mountDir, "postmaster.pid"), []byte("123"), 0o600); err != nil {
		t.Fatalf("write postmaster.pid: %v", err)
	}
	mgr := newManagerWithStateFS(t, store, &fakeStateFS{mountDir: mountDir})
	paths, err := resolveStatePaths(mgr.stateStoreRoot, "image-1", "state-1", mgr.statefs)
	if err != nil {
		t.Fatalf("resolveStatePaths: %v", err)
	}
	if err := os.MkdirAll(paths.stateDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	_, errResp := mgr.startRuntime(context.Background(), "job-1", preparedRequest{
		request: Request{
			PrepareKind: "psql",
			ImageID:     "image-1",
		},
	}, &TaskInput{Kind: "state", ID: "state-1"})
	if errResp == nil {
		t.Fatalf("expected error")
	}
	if errResp.Code != "internal_error" {
		t.Fatalf("expected internal_error, got %+v", errResp)
	}
}

func TestStartRuntimeFailsWhenRuntimeDirMissingPGVersion(t *testing.T) {
	store := &fakeStore{statesByID: map[string]store.StateEntry{
		"state-1": {StateID: "state-1", ImageID: "image-1"},
	}}
	mountDir := filepath.Join(t.TempDir(), "mount")
	if err := os.MkdirAll(mountDir, 0o700); err != nil {
		t.Fatalf("mkdir mount: %v", err)
	}
	mgr := newManagerWithStateFS(t, store, &fakeStateFS{mountDir: mountDir})
	paths, err := resolveStatePaths(mgr.stateStoreRoot, "image-1", "state-1", mgr.statefs)
	if err != nil {
		t.Fatalf("resolveStatePaths: %v", err)
	}
	if err := os.MkdirAll(paths.stateDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(paths.stateDir, "PG_VERSION"), []byte("17"), 0o600); err != nil {
		t.Fatalf("write PG_VERSION: %v", err)
	}
	_, errResp := mgr.startRuntime(context.Background(), "job-1", preparedRequest{
		request: Request{
			PrepareKind: "psql",
			ImageID:     "image-1",
		},
	}, &TaskInput{Kind: "state", ID: "state-1"})
	if errResp == nil {
		t.Fatalf("expected error")
	}
	if errResp.Code != "internal_error" {
		t.Fatalf("expected internal_error, got %+v", errResp)
	}
}

func TestExecuteStateTaskDirtyCachedStateReturnsCachedWithoutInvalidation(t *testing.T) {
	stateStore := &fakeStore{statesByID: map[string]store.StateEntry{}}
	mgr := newManagerWithStateFS(t, stateStore, &fakeStateFS{})

	prepared, err := mgr.prepareRequest(Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	})
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	outputID := psqlOutputStateID(t, mgr, prepared, TaskInput{Kind: "image", ID: "image-1"})
	stateStore.statesByID[outputID] = store.StateEntry{StateID: outputID, ImageID: "image-1"}

	paths, err := resolveStatePaths(mgr.stateStoreRoot, "image-1", outputID, mgr.statefs)
	if err != nil {
		t.Fatalf("resolveStatePaths: %v", err)
	}
	if err := os.MkdirAll(paths.stateDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(paths.stateDir, "postmaster.pid"), []byte("123"), 0o600); err != nil {
		t.Fatalf("write postmaster.pid: %v", err)
	}

	task := taskState{
		PlanTask: PlanTask{
			TaskID:        "execute-0",
			Type:          "state_execute",
			OutputStateID: outputID,
			Input:         &TaskInput{Kind: "image", ID: "image-1"},
		},
	}

	if _, errResp := mgr.executeStateTask(context.Background(), "job-1", prepared, task); errResp != nil {
		t.Fatalf("executeStateTask: %+v", errResp)
	}
	if len(stateStore.deletedStates) != 0 {
		t.Fatalf("expected no cached state deletion during execute task, got %+v", stateStore.deletedStates)
	}
	if len(stateStore.states) != 0 {
		t.Fatalf("expected no rebuild for cached state, got %+v", stateStore.states)
	}
}

func TestExecuteStateTaskCachedStateMissingPGVersionReturnsCachedWithoutInvalidationManager(t *testing.T) {
	stateStore := &fakeStore{statesByID: map[string]store.StateEntry{}}
	mgr := newManagerWithStateFS(t, stateStore, &fakeStateFS{})

	prepared, err := mgr.prepareRequest(Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	})
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	outputID := psqlOutputStateID(t, mgr, prepared, TaskInput{Kind: "image", ID: "image-1"})
	stateStore.statesByID[outputID] = store.StateEntry{StateID: outputID, ImageID: "image-1"}

	paths, err := resolveStatePaths(mgr.stateStoreRoot, "image-1", outputID, mgr.statefs)
	if err != nil {
		t.Fatalf("resolveStatePaths: %v", err)
	}
	if err := os.MkdirAll(paths.stateDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	task := taskState{
		PlanTask: PlanTask{
			TaskID:        "execute-0",
			Type:          "state_execute",
			OutputStateID: outputID,
			Input:         &TaskInput{Kind: "image", ID: "image-1"},
		},
	}

	if _, errResp := mgr.executeStateTask(context.Background(), "job-1", prepared, task); errResp != nil {
		t.Fatalf("executeStateTask: %+v", errResp)
	}
	if len(stateStore.deletedStates) != 0 {
		t.Fatalf("expected no cached state deletion during execute task, got %+v", stateStore.deletedStates)
	}
	if len(stateStore.states) != 0 {
		t.Fatalf("expected no rebuild for cached state, got %+v", stateStore.states)
	}
}

func TestCreateInstanceMissingConnectionInfo(t *testing.T) {
	store := &fakeStore{statesByID: map[string]store.StateEntry{"state-1": {StateID: "state-1", ImageID: "image-1"}}}
	runtime := &fakeRuntime{instance: engineRuntime.Instance{ID: "container-1"}, noDefaults: true}
	mgr := newManagerWithDeps(t, store, newQueueStore(t), &testDeps{runtime: runtime})
	prepared, err := mgr.prepareRequest(Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	})
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	if _, errResp := mgr.createInstance(context.Background(), "job-1", prepared, "state-1"); errResp == nil || errResp.Code != "internal_error" {
		t.Fatalf("expected internal error, got %+v", errResp)
	}
}

func TestStartRuntimeRejectsNilInput(t *testing.T) {
	store := &fakeStore{}
	mgr := newManager(t, store)
	prepared, err := mgr.prepareRequest(Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	})
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	if _, errResp := mgr.startRuntime(context.Background(), "job-1", prepared, nil); errResp == nil || errResp.Code != "internal_error" {
		t.Fatalf("expected internal error, got %+v", errResp)
	}
}

func TestStartRuntimeUnknownInputKind(t *testing.T) {
	store := &fakeStore{}
	mgr := newManager(t, store)
	prepared, err := mgr.prepareRequest(Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	})
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	if _, errResp := mgr.startRuntime(context.Background(), "job-1", prepared, &TaskInput{Kind: "other", ID: "x"}); errResp == nil || errResp.Code != "internal_error" {
		t.Fatalf("expected internal error, got %+v", errResp)
	}
}

func TestStartRuntimeMissingState(t *testing.T) {
	store := &fakeStore{}
	mgr := newManager(t, store)
	prepared, err := mgr.prepareRequest(Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	})
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	if _, errResp := mgr.startRuntime(context.Background(), "job-1", prepared, &TaskInput{Kind: "state", ID: "missing"}); errResp == nil || errResp.Code != "internal_error" {
		t.Fatalf("expected internal error, got %+v", errResp)
	}
}

func TestStartRuntimeMissingStateID(t *testing.T) {
	store := &fakeStore{}
	mgr := newManager(t, store)
	prepared, err := mgr.prepareRequest(Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	})
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	if _, errResp := mgr.startRuntime(context.Background(), "job-1", prepared, &TaskInput{Kind: "state", ID: ""}); errResp == nil || errResp.Code != "internal_error" {
		t.Fatalf("expected internal error, got %+v", errResp)
	}
}

func TestStartRuntimeGetStateError(t *testing.T) {
	store := &fakeStore{getStateErr: errors.New("boom")}
	mgr := newManager(t, store)
	prepared, err := mgr.prepareRequest(Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	})
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	if _, errResp := mgr.startRuntime(context.Background(), "job-1", prepared, &TaskInput{Kind: "state", ID: "state-1"}); errResp == nil || errResp.Code != "internal_error" {
		t.Fatalf("expected internal error, got %+v", errResp)
	}
}

func TestStartRuntimeFromImageSuccess(t *testing.T) {
	runtime := &fakeRuntime{}
	snap := &fakeStateFS{}
	store := &fakeStore{}
	mgr := newManagerWithDeps(t, store, newQueueStore(t), &testDeps{runtime: runtime, statefs: snap})
	prepared, err := mgr.prepareRequest(Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	})
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	rt, errResp := mgr.startRuntime(context.Background(), "job-1", prepared, &TaskInput{Kind: "image", ID: "image-1"})
	if errResp != nil {
		t.Fatalf("startRuntime: %+v", errResp)
	}
	if rt == nil || rt.instance.ID == "" {
		t.Fatalf("expected runtime instance")
	}
	if len(runtime.startCalls) != 1 {
		t.Fatalf("expected runtime start call")
	}
	if len(snap.cloneCalls) != 1 {
		t.Fatalf("expected clone call")
	}
}

func TestStartRuntimeRejectsRuntimeDirNestedInStateDir(t *testing.T) {
	stateRoot := t.TempDir()
	store := &fakeStore{
		statesByID: map[string]store.StateEntry{
			"state-1": {StateID: "state-1", ImageID: "image-1"},
		},
	}
	mgr := newManagerWithDeps(t, store, newQueueStore(t), &testDeps{stateRoot: stateRoot})
	prepared, err := mgr.prepareRequest(Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	})
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	paths, err := resolveStatePaths(stateRoot, "image-1", "state-1", mgr.statefs)
	if err != nil {
		t.Fatalf("resolveStatePaths: %v", err)
	}
	if err := os.MkdirAll(paths.stateDir, 0o700); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(paths.stateDir, "PG_VERSION"), []byte("17"), 0o600); err != nil {
		t.Fatalf("write PG_VERSION: %v", err)
	}

	jobID := filepath.Join("..", "engines", "image-1", "latest", "states", "state-1", "nested")
	_, errResp := mgr.startRuntime(context.Background(), jobID, prepared, &TaskInput{Kind: "state", ID: "state-1"})
	if errResp == nil || !strings.Contains(errResp.Message, "runtime dir is nested inside state dir") {
		t.Fatalf("expected nested runtime dir error, got %+v", errResp)
	}
}

func TestExecuteStateTaskLiquibaseRequiresPendingChangesets(t *testing.T) {
	store := &fakeStore{}
	liquibase := &fakeLiquibaseRunner{output: ""}
	mgr := newManagerWithDeps(t, store, newQueueStore(t), &testDeps{
		runtime:   &fakeRuntime{},
		liquibase: liquibase,
	})
	prepared, err := mgr.prepareRequest(Request{
		PrepareKind:   "lb",
		ImageID:       "image-1",
		LiquibaseArgs: []string{"update"},
	})
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	task := taskState{
		PlanTask: PlanTask{
			TaskID:        "execute-0",
			Type:          "state_execute",
			OutputStateID: "state-1",
			Input:         &TaskInput{Kind: "image", ID: "image-1"},
		},
		Status: StatusQueued,
	}

	_, errResp := mgr.executeStateTask(context.Background(), "job-1", prepared, task)
	if errResp == nil || !strings.Contains(errResp.Message, "no pending changesets") {
		t.Fatalf("expected no pending changesets error, got %+v", errResp)
	}
	if len(liquibase.runs) == 0 {
		t.Fatalf("expected liquibase planning call")
	}
}

func TestExecuteStateTaskLiquibaseUsesPlannedHashForCachedState(t *testing.T) {
	stateStore := &fakeStore{statesByID: map[string]store.StateEntry{}}
	liquibase := &fakeLiquibaseRunner{output: ""}
	mgr := newManagerWithDeps(t, stateStore, newQueueStore(t), &testDeps{
		runtime:   &fakeRuntime{},
		liquibase: liquibase,
	})
	prepared, err := mgr.prepareRequest(Request{
		PrepareKind:   "lb",
		ImageID:       "image-1",
		LiquibaseArgs: []string{"update"},
	})
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}

	taskHash := "planned-task-hash"
	outputID, errResp := mgr.computeOutputStateID("image", "image-1", taskHash)
	if errResp != nil {
		t.Fatalf("computeOutputStateID: %+v", errResp)
	}
	stateStore.statesByID[outputID] = store.StateEntry{StateID: outputID, ImageID: "image-1"}
	paths, err := resolveStatePaths(mgr.stateStoreRoot, "image-1", outputID, mgr.statefs)
	if err != nil {
		t.Fatalf("resolveStatePaths: %v", err)
	}
	if err := os.MkdirAll(paths.stateDir, 0o700); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(paths.stateDir, "PG_VERSION"), []byte("17"), 0o600); err != nil {
		t.Fatalf("write PG_VERSION: %v", err)
	}

	task := taskState{
		PlanTask: PlanTask{
			TaskID:        "execute-0",
			Type:          "state_execute",
			OutputStateID: outputID,
			TaskHash:      taskHash,
			Input:         &TaskInput{Kind: "image", ID: "image-1"},
		},
		Status: StatusQueued,
	}

	got, errResp := mgr.executeStateTask(context.Background(), "job-1", prepared, task)
	if errResp != nil {
		t.Fatalf("executeStateTask: %+v", errResp)
	}
	if got != outputID {
		t.Fatalf("expected output %q, got %q", outputID, got)
	}
	if len(liquibase.runs) != 0 {
		t.Fatalf("expected no liquibase updateSQL calls for cached task hash, got %+v", liquibase.runs)
	}
}

func TestEnsureRuntimeRequiresRunner(t *testing.T) {
	store := &fakeStore{}
	mgr := newManager(t, store)
	prepared, err := mgr.prepareRequest(Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	})
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	if _, errResp := mgr.ensureRuntime(context.Background(), "job-1", prepared, &TaskInput{Kind: "image", ID: "image-1"}, nil); errResp == nil || errResp.Code != "internal_error" {
		t.Fatalf("expected internal error, got %+v", errResp)
	}
}

func TestRunnerForJobEphemeral(t *testing.T) {
	mgr := newManager(t, &fakeStore{})
	runner, ephemeral := mgr.runnerForJob("")
	if runner == nil || !ephemeral {
		t.Fatalf("expected ephemeral runner")
	}
}

func TestEnsureBaseStateSkipsInit(t *testing.T) {
	runtime := &fakeRuntime{}
	store := &fakeStore{}
	mgr := newManagerWithDeps(t, store, newQueueStore(t), &testDeps{runtime: runtime})
	baseDir := filepath.Join(t.TempDir(), "base")
	if err := os.MkdirAll(baseDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(baseDir, "PG_VERSION"), []byte("17"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := mgr.ensureBaseState(context.Background(), "image-1", baseDir); err != nil {
		t.Fatalf("ensureBaseState: %v", err)
	}
	if len(runtime.initCalls) != 0 {
		t.Fatalf("expected no init calls, got %d", len(runtime.initCalls))
	}
}

func TestEnsureBaseStateInitError(t *testing.T) {
	runtime := &fakeRuntime{initErr: errors.New("boom")}
	store := &fakeStore{}
	mgr := newManagerWithDeps(t, store, newQueueStore(t), &testDeps{runtime: runtime})
	baseDir := filepath.Join(t.TempDir(), "base")
	if err := mgr.ensureBaseState(context.Background(), "image-1", baseDir); err == nil {
		t.Fatalf("expected init error")
	}
}

func TestEnsureBaseStateRequiresDir(t *testing.T) {
	mgr := newManager(t, &fakeStore{})
	if err := mgr.ensureBaseState(context.Background(), "image-1", ""); err == nil {
		t.Fatalf("expected error for empty base dir")
	}
}

func TestCreateInstanceErrors(t *testing.T) {
	store := &fakeStore{
		createInstanceErr: errors.New("boom"),
		statesByID: map[string]store.StateEntry{
			"state-1": {StateID: "state-1", ImageID: "image-1"},
		},
	}
	mgr := newManager(t, store)
	prepared, err := mgr.prepareRequest(Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	})
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	if _, errResp := mgr.createInstance(context.Background(), "job-1", prepared, "state-1"); errResp == nil || errResp.Code != "internal_error" {
		t.Fatalf("expected internal error, got %+v", errResp)
	}
}

func TestCreateInstanceCancelled(t *testing.T) {
	store := &cancelStore{}
	mgr := newManager(t, store)
	prepared, err := mgr.prepareRequest(Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	})
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, errResp := mgr.createInstance(ctx, "job-1", prepared, "state-1"); errResp == nil || errResp.Code != "cancelled" {
		t.Fatalf("expected cancelled error, got %+v", errResp)
	}
}

func TestCreateInstanceStoresRuntimeDir(t *testing.T) {
	stateRoot := filepath.Join(t.TempDir(), "state-store")
	store := &fakeStore{
		statesByID: map[string]store.StateEntry{
			"state-1": {StateID: "state-1", ImageID: "image-1"},
		},
	}
	mgr := newManagerWithDeps(t, store, newQueueStore(t), &testDeps{stateRoot: stateRoot})
	prepared, err := mgr.prepareRequest(Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	})
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	paths, err := resolveStatePaths(stateRoot, "image-1", "state-1", mgr.statefs)
	if err != nil {
		t.Fatalf("resolveStatePaths: %v", err)
	}
	if err := os.MkdirAll(paths.stateDir, 0o700); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(paths.stateDir, "PG_VERSION"), []byte("17"), 0o600); err != nil {
		t.Fatalf("write PG_VERSION: %v", err)
	}
	if _, errResp := mgr.createInstance(context.Background(), "job-1", prepared, "state-1"); errResp != nil {
		t.Fatalf("createInstance: %+v", errResp)
	}
	if len(store.instances) != 1 {
		t.Fatalf("expected instance create, got %+v", store.instances)
	}
	expected := filepath.Join(stateRoot, "jobs", "job-1", "runtime")
	if store.instances[0].RuntimeDir == nil || *store.instances[0].RuntimeDir != expected {
		t.Fatalf("unexpected runtime dir: %+v", store.instances[0].RuntimeDir)
	}
}

func TestEventFromRecordParsesPayloads(t *testing.T) {
	result := Result{DSN: "dsn", InstanceID: "i", StateID: "s", ImageID: "img", PrepareKind: "psql", PrepareArgsNormalized: "args"}
	resultJSON, err := json.Marshal(result)
	if err != nil {
		t.Fatalf("marshal result: %v", err)
	}
	errResp := ErrorResponse{Code: "boom", Message: "fail"}
	errJSON, err := json.Marshal(errResp)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	event := eventFromRecord(queue.EventRecord{
		Type:       "result",
		Ts:         "2026-01-19T00:00:00Z",
		ResultJSON: strPtr(string(resultJSON)),
		ErrorJSON:  strPtr(string(errJSON)),
	})
	if event.Result == nil || event.Result.DSN != "dsn" {
		t.Fatalf("unexpected result: %+v", event.Result)
	}
	if event.Error == nil || event.Error.Code != "boom" {
		t.Fatalf("unexpected error: %+v", event.Error)
	}
}

func TestFindOutputStateID(t *testing.T) {
	stateID := findOutputStateID([]taskState{
		{PlanTask: PlanTask{Type: "plan"}},
		{PlanTask: PlanTask{Type: "state_execute", OutputStateID: "state-1"}},
	})
	if stateID != "state-1" {
		t.Fatalf("unexpected state id: %s", stateID)
	}
}

func TestWaitForEvent(t *testing.T) {
	queueStore := newQueueStore(t)
	mgr := newManagerWithQueue(t, &fakeStore{}, queueStore)

	if _, err := mgr.Submit(context.Background(), Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
		PlanOnly:    true,
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	if err := mgr.WaitForEvent(context.Background(), "job-1", 0); err != nil {
		t.Fatalf("WaitForEvent: %v", err)
	}

	count, err := queueStore.CountEvents(context.Background(), "job-1")
	if err != nil {
		t.Fatalf("CountEvents: %v", err)
	}
	if err := mgr.WaitForEvent(context.Background(), "job-1", count); err != nil {
		t.Fatalf("WaitForEvent after done: %v", err)
	}
}

func TestWaitForEventDoneNoEvents(t *testing.T) {
	queueStore := newQueueStore(t)
	reqJSON, err := jsonMarshal(Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	})
	if err != nil {
		t.Fatalf("jsonMarshal: %v", err)
	}
	if err := queueStore.CreateJob(context.Background(), queue.JobRecord{
		JobID:       "job-1",
		Status:      StatusSucceeded,
		PrepareKind: "psql",
		ImageID:     "image-1",
		RequestJSON: &reqJSON,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	mgr := newManagerWithQueue(t, &fakeStore{}, queueStore)

	if err := mgr.WaitForEvent(context.Background(), "job-1", 0); err != nil {
		t.Fatalf("WaitForEvent: %v", err)
	}
}

func TestWaitForEventCountError(t *testing.T) {
	queueStore := newQueueStore(t)
	faulty := &faultQueueStore{
		Store: queueStore,
		countEvents: func(context.Context, string) (int, error) {
			return 0, errors.New("boom")
		},
	}
	mgr := newManagerWithQueue(t, &fakeStore{}, faulty)

	if err := mgr.WaitForEvent(context.Background(), "job-1", 0); err == nil {
		t.Fatalf("expected error")
	}
}

func TestWaitForEventNotifies(t *testing.T) {
	queueStore := newQueueStore(t)
	req := Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	}
	createJobRecord(t, queueStore, "job-1", req, StatusRunning)
	mgr := newManagerWithQueue(t, &fakeStore{}, queueStore)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	waitErr := make(chan error, 1)
	go func() {
		waitErr <- mgr.WaitForEvent(ctx, "job-1", 0)
	}()

	deadline := time.Now().Add(2 * time.Second)
	for {
		mgr.events.mu.Lock()
		_, ok := mgr.events.subs["job-1"]
		mgr.events.mu.Unlock()
		if ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timeout waiting for subscription")
		}
		time.Sleep(5 * time.Millisecond)
	}

	if err := mgr.appendEvent("job-1", Event{
		Type:   "status",
		Ts:     time.Now().UTC().Format(time.RFC3339Nano),
		Status: StatusRunning,
	}); err != nil {
		t.Fatalf("appendEvent: %v", err)
	}
	if err := <-waitErr; err != nil {
		t.Fatalf("WaitForEvent: %v", err)
	}
}

func TestWaitForEventGetJobError(t *testing.T) {
	queueStore := newQueueStore(t)
	faulty := &faultQueueStore{
		Store: queueStore,
		countEvents: func(context.Context, string) (int, error) {
			return 0, nil
		},
		getJob: func(context.Context, string) (queue.JobRecord, bool, error) {
			return queue.JobRecord{}, false, errors.New("boom")
		},
	}
	mgr := newManagerWithQueue(t, &fakeStore{}, faulty)

	if err := mgr.WaitForEvent(context.Background(), "job-1", 0); err == nil {
		t.Fatalf("expected error")
	}
}

func TestWaitForEventCancel(t *testing.T) {
	queueStore := newQueueStore(t)
	reqJSON, err := jsonMarshal(Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	})
	if err != nil {
		t.Fatalf("jsonMarshal: %v", err)
	}
	if err := queueStore.CreateJob(context.Background(), queue.JobRecord{
		JobID:       "job-1",
		Status:      StatusRunning,
		PrepareKind: "psql",
		ImageID:     "image-1",
		RequestJSON: &reqJSON,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	mgr := newManagerWithQueue(t, &fakeStore{}, queueStore)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := mgr.WaitForEvent(ctx, "job-1", 0); err == nil {
		t.Fatalf("expected cancel error")
	}
}

func TestHeartbeatRepeatsRunningTask(t *testing.T) {
	const heartbeatEvery = 200 * time.Millisecond
	const waitTimeout = 10 * time.Second
	queueStore := newQueueStore(t)
	mgr, err := NewPrepareService(Options{
		Store:          &fakeStore{},
		Queue:          queueStore,
		Runtime:        &fakeRuntime{},
		StateFS:        &fakeStateFS{},
		DBMS:           &fakeDBMS{},
		StateStoreRoot: t.TempDir(),
		Config:         &fakeConfigStore{value: 2},
		Psql:           &fakePsqlRunner{},
		Version:        "v1",
		Now:            time.Now,
		IDGen:          func() (string, error) { return "job-1", nil },
		Async:          false,
		HeartbeatEvery: heartbeatEvery,
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	req := Request{PrepareKind: "psql", ImageID: "image-1"}
	createJobRecord(t, queueStore, "job-1", req, StatusRunning)
	if err := queueStore.ReplaceTasks(context.Background(), "job-1", []queue.TaskRecord{
		{JobID: "job-1", TaskID: "prepare-instance", Position: 0, Type: "prepare_instance", Status: StatusQueued},
	}); err != nil {
		t.Fatalf("ReplaceTasks: %v", err)
	}
	startedAt := time.Now().UTC().Format(time.RFC3339Nano)
	if err := mgr.updateTaskStatus(context.Background(), "job-1", "prepare-instance", StatusRunning, &startedAt, nil, nil); err != nil {
		t.Fatalf("updateTaskStatus: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), waitTimeout)
	defer cancel()
	offset := 0
	runningCount := 0
	for runningCount < 1 {
		events, ok, _, err := mgr.EventsSince("job-1", offset)
		if err != nil || !ok {
			t.Fatalf("EventsSince: ok=%v err=%v", ok, err)
		}
		offset += len(events)
		for _, event := range events {
			if event.Type == "task" && event.TaskID == "prepare-instance" && event.Status == StatusRunning {
				runningCount++
			}
		}
		if runningCount >= 1 {
			break
		}
		if err := mgr.WaitForEvent(ctx, "job-1", offset); err != nil {
			t.Fatalf("WaitForEvent: %v", err)
		}
	}

	finishedAt := time.Now().UTC().Format(time.RFC3339Nano)
	if err := mgr.updateTaskStatus(context.Background(), "job-1", "prepare-instance", StatusSucceeded, nil, &finishedAt, nil); err != nil {
		t.Fatalf("updateTaskStatus: %v", err)
	}
}

func TestHeartbeatRepeatsLogEvent(t *testing.T) {
	const heartbeatEvery = 200 * time.Millisecond
	const waitTimeout = 10 * time.Second
	queueStore := newQueueStore(t)
	mgr, err := NewPrepareService(Options{
		Store:          &fakeStore{},
		Queue:          queueStore,
		Runtime:        &fakeRuntime{},
		StateFS:        &fakeStateFS{},
		DBMS:           &fakeDBMS{},
		StateStoreRoot: t.TempDir(),
		Config:         &fakeConfigStore{value: 2},
		Psql:           &fakePsqlRunner{},
		Version:        "v1",
		Now:            time.Now,
		IDGen:          func() (string, error) { return "job-1", nil },
		Async:          false,
		HeartbeatEvery: heartbeatEvery,
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	req := Request{PrepareKind: "psql", ImageID: "image-1"}
	createJobRecord(t, queueStore, "job-1", req, StatusRunning)
	if err := queueStore.ReplaceTasks(context.Background(), "job-1", []queue.TaskRecord{
		{JobID: "job-1", TaskID: "execute-0", Position: 0, Type: "state_execute", Status: StatusQueued},
	}); err != nil {
		t.Fatalf("ReplaceTasks: %v", err)
	}
	startedAt := time.Now().UTC().Format(time.RFC3339Nano)
	if err := mgr.updateTaskStatus(context.Background(), "job-1", "execute-0", StatusRunning, &startedAt, nil, nil); err != nil {
		t.Fatalf("updateTaskStatus: %v", err)
	}
	mgr.appendLog("job-1", "docker: pulling layers")
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	offset := 0
	logCount := 0
	for logCount < 2 {
		events, ok, _, err := mgr.EventsSince("job-1", offset)
		if err != nil || !ok {
			t.Fatalf("EventsSince: ok=%v err=%v", ok, err)
		}
		offset += len(events)
		for _, event := range events {
			if event.Type == "log" && event.Message == "docker: pulling layers" {
				logCount++
			}
		}
		if logCount >= 2 {
			break
		}
		if err := mgr.WaitForEvent(ctx, "job-1", offset); err != nil {
			t.Fatalf("WaitForEvent: %v", err)
		}
	}
}

func TestHeartbeatStopsAfterTaskComplete(t *testing.T) {
	queueStore := newQueueStore(t)
	mgr, err := NewPrepareService(Options{
		Store:          &fakeStore{},
		Queue:          queueStore,
		Runtime:        &fakeRuntime{},
		StateFS:        &fakeStateFS{},
		DBMS:           &fakeDBMS{},
		StateStoreRoot: t.TempDir(),
		Config:         &fakeConfigStore{value: 2},
		Psql:           &fakePsqlRunner{},
		Version:        "v1",
		Now:            time.Now,
		IDGen:          func() (string, error) { return "job-1", nil },
		Async:          false,
		HeartbeatEvery: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	req := Request{PrepareKind: "psql", ImageID: "image-1"}
	createJobRecord(t, queueStore, "job-1", req, StatusRunning)
	if err := queueStore.ReplaceTasks(context.Background(), "job-1", []queue.TaskRecord{
		{JobID: "job-1", TaskID: "execute-0", Position: 0, Type: "state_execute", Status: StatusQueued},
	}); err != nil {
		t.Fatalf("ReplaceTasks: %v", err)
	}
	startedAt := time.Now().UTC().Format(time.RFC3339Nano)
	if err := mgr.updateTaskStatus(context.Background(), "job-1", "execute-0", StatusRunning, &startedAt, nil, nil); err != nil {
		t.Fatalf("updateTaskStatus: %v", err)
	}
	mgr.appendLog("job-1", "docker: pulling layers")
	events, ok, _, err := mgr.EventsSince("job-1", 0)
	if err != nil || !ok {
		t.Fatalf("EventsSince: ok=%v err=%v", ok, err)
	}
	offset := len(events)
	runningCount := 0
	for _, event := range events {
		if event.Type == "task" && event.TaskID == "execute-0" && event.Status == StatusRunning {
			runningCount++
		}
	}
	if runningCount == 0 {
		t.Fatalf("expected running task event, got %+v", events)
	}
	finishedAt := time.Now().UTC().Format(time.RFC3339Nano)
	if err := mgr.updateTaskStatus(context.Background(), "job-1", "execute-0", StatusSucceeded, nil, &finishedAt, nil); err != nil {
		t.Fatalf("updateTaskStatus: %v", err)
	}
	ctxAfter, cancelAfter := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancelAfter()
	waitErr := mgr.WaitForEvent(ctxAfter, "job-1", offset)
	if waitErr != nil && !errors.Is(waitErr, context.DeadlineExceeded) {
		t.Fatalf("WaitForEvent after completion: %v", waitErr)
	}
	eventsAfter, ok, _, err := mgr.EventsSince("job-1", offset)
	if err != nil || !ok {
		t.Fatalf("EventsSince: ok=%v err=%v", ok, err)
	}
	finishedTs, err := time.Parse(time.RFC3339Nano, finishedAt)
	if err != nil {
		t.Fatalf("parse finishedAt: %v", err)
	}
	for _, event := range eventsAfter {
		if event.Type != "task" || event.TaskID != "execute-0" || event.Status != StatusRunning {
			continue
		}
		ts, err := time.Parse(time.RFC3339Nano, event.Ts)
		if err != nil {
			t.Fatalf("parse event ts: %v", err)
		}
		if ts.After(finishedTs) {
			t.Fatalf("expected no heartbeat after completion, got %+v", event)
		}
	}
}

func TestUpdateTaskStatusIncludesErrorEventPayload(t *testing.T) {
	queueStore := newQueueStore(t)
	mgr := newManagerWithQueue(t, &fakeStore{}, queueStore)
	req := Request{PrepareKind: "psql", ImageID: "image-1"}
	createJobRecord(t, queueStore, "job-1", req, StatusRunning)
	if err := queueStore.ReplaceTasks(context.Background(), "job-1", []queue.TaskRecord{
		{JobID: "job-1", TaskID: "execute-0", Position: 0, Type: "state_execute", Status: StatusQueued},
	}); err != nil {
		t.Fatalf("ReplaceTasks: %v", err)
	}

	finishedAt := time.Now().UTC().Format(time.RFC3339Nano)
	errResp := errorResponse("internal_error", "psql execution failed", "exit status 3")
	if err := mgr.updateTaskStatus(context.Background(), "job-1", "execute-0", StatusFailed, nil, &finishedAt, errResp); err != nil {
		t.Fatalf("updateTaskStatus: %v", err)
	}

	events, ok, _, err := mgr.EventsSince("job-1", 0)
	if err != nil || !ok {
		t.Fatalf("EventsSince: ok=%v err=%v", ok, err)
	}
	var failedTask *Event
	for i := range events {
		if events[i].Type == "task" && events[i].TaskID == "execute-0" && events[i].Status == StatusFailed {
			failedTask = &events[i]
			break
		}
	}
	if failedTask == nil {
		t.Fatalf("expected failed task event, got %+v", events)
	}
	if failedTask.Message != "psql execution failed" {
		t.Fatalf("expected task message, got %+v", failedTask)
	}
	if failedTask.Error == nil || failedTask.Error.Details != "exit status 3" {
		t.Fatalf("expected task error payload, got %+v", failedTask)
	}
}

func TestAppendLogIgnoresEmpty(t *testing.T) {
	queueStore := newQueueStore(t)
	mgr := newManagerWithQueue(t, &fakeStore{}, queueStore)
	req := Request{PrepareKind: "psql", ImageID: "image-1"}
	createJobRecord(t, queueStore, "job-1", req, StatusRunning)

	mgr.appendLog("", "message")
	mgr.appendLog("job-1", "")

	events, ok, _, err := mgr.EventsSince("job-1", 0)
	if err != nil || !ok {
		t.Fatalf("EventsSince: ok=%v err=%v", ok, err)
	}
	if len(events) != 0 {
		t.Fatalf("expected no events, got %d", len(events))
	}

	mgr.appendLog("job-1", "hello")
	events, ok, _, err = mgr.EventsSince("job-1", 0)
	if err != nil || !ok {
		t.Fatalf("EventsSince: ok=%v err=%v", ok, err)
	}
	if len(events) != 1 || events[0].Message != "hello" {
		t.Fatalf("unexpected log events: %+v", events)
	}
}

func TestAppendLogLines(t *testing.T) {
	queueStore := newQueueStore(t)
	mgr := newManagerWithQueue(t, &fakeStore{}, queueStore)
	req := Request{PrepareKind: "psql", ImageID: "image-1"}
	createJobRecord(t, queueStore, "job-1", req, StatusRunning)

	mgr.appendLogLines("job-1", "docker", " line-1\n\n line-2 ")
	mgr.appendLogLines("job-1", "", "line-3")
	events, ok, _, err := mgr.EventsSince("job-1", 0)
	if err != nil || !ok {
		t.Fatalf("EventsSince: ok=%v err=%v", ok, err)
	}
	if len(events) != 3 {
		t.Fatalf("expected 3 log events, got %d", len(events))
	}
	if events[0].Message != "docker: line-1" || events[1].Message != "docker: line-2" || events[2].Message != "line-3" {
		t.Fatalf("unexpected log messages: %+v", events)
	}
}

func TestNormalizeHeartbeat(t *testing.T) {
	if got := normalizeHeartbeat(0); got != 500*time.Millisecond {
		t.Fatalf("expected default heartbeat, got %v", got)
	}
	if got := normalizeHeartbeat(50 * time.Millisecond); got != 200*time.Millisecond {
		t.Fatalf("expected min heartbeat, got %v", got)
	}
	if got := normalizeHeartbeat(1500 * time.Millisecond); got != time.Second {
		t.Fatalf("expected max heartbeat, got %v", got)
	}
	if got := normalizeHeartbeat(300 * time.Millisecond); got != 300*time.Millisecond {
		t.Fatalf("expected passthrough heartbeat, got %v", got)
	}
}

func TestSummarizeLogDetails(t *testing.T) {
	if got := summarizeLogDetails(""); got != "" {
		t.Fatalf("expected empty summary, got %q", got)
	}
	if got := summarizeLogDetails("line1\nline2"); got != "line1 | line2" {
		t.Fatalf("unexpected newline summary: %q", got)
	}
	long := strings.Repeat("x", 600)
	got := summarizeLogDetails(long)
	if !strings.Contains(got, "(truncated)") {
		t.Fatalf("expected truncation marker, got %q", got)
	}
}

func TestHeartbeatStopsOnStatusEvent(t *testing.T) {
	queueStore := newQueueStore(t)
	mgr, err := NewPrepareService(Options{
		Store:          &fakeStore{},
		Queue:          queueStore,
		Runtime:        &fakeRuntime{},
		StateFS:        &fakeStateFS{},
		DBMS:           &fakeDBMS{},
		StateStoreRoot: t.TempDir(),
		Config:         &fakeConfigStore{value: 2},
		Psql:           &fakePsqlRunner{},
		Version:        "v1",
		Now:            time.Now,
		IDGen:          func() (string, error) { return "job-1", nil },
		Async:          false,
		HeartbeatEvery: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	req := Request{PrepareKind: "psql", ImageID: "image-1"}
	createJobRecord(t, queueStore, "job-1", req, StatusRunning)

	mgr.updateHeartbeat("job-1", Event{Type: "task", Status: StatusRunning, TaskID: "execute-0"})
	mgr.updateHeartbeat("job-1", Event{Type: "status", Status: StatusSucceeded})

	mgr.mu.Lock()
	state := mgr.beats["job-1"]
	mgr.mu.Unlock()
	if state != nil {
		t.Fatalf("expected heartbeat state removed, got %+v", state)
	}
}

func TestHeartbeatDoesNotRecreateAfterTerminalStatus(t *testing.T) {
	mgr := newManager(t, &fakeStore{})

	mgr.updateHeartbeat("job-1", Event{Type: "task", Status: StatusRunning, TaskID: "execute-0"})
	mgr.updateHeartbeat("job-1", Event{Type: "status", Status: StatusSucceeded})
	mgr.updateHeartbeat("job-1", Event{
		Type:   "result",
		Result: &Result{InstanceID: "i", StateID: "s", ImageID: "img"},
	})

	mgr.mu.Lock()
	state := mgr.beats["job-1"]
	mgr.mu.Unlock()
	if state != nil {
		t.Fatalf("expected no heartbeat state after terminal status, got %+v", state)
	}
}

func TestStartHeartbeatNoopWhenCancelSet(t *testing.T) {
	mgr := newManager(t, &fakeStore{})
	called := false
	mgr.mu.Lock()
	mgr.beats["job-1"] = &heartbeatState{
		cancel: func() { called = true },
	}
	mgr.mu.Unlock()

	mgr.startHeartbeat("job-1")
	if called {
		t.Fatalf("expected startHeartbeat to noop when cancel set")
	}
}

func TestStartHeartbeatNoEvent(t *testing.T) {
	mgr, err := NewPrepareService(Options{
		Store:          &fakeStore{},
		Queue:          newQueueStore(t),
		Runtime:        &fakeRuntime{},
		StateFS:        &fakeStateFS{},
		DBMS:           &fakeDBMS{},
		StateStoreRoot: t.TempDir(),
		Config:         &fakeConfigStore{value: 2},
		Psql:           &fakePsqlRunner{},
		Version:        "v1",
		Now:            time.Now,
		IDGen:          func() (string, error) { return "job-1", nil },
		Async:          false,
		HeartbeatEvery: 50 * time.Millisecond,
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	mgr.mu.Lock()
	mgr.beats["job-1"] = &heartbeatState{
		runningTask: "execute-0",
	}
	mgr.mu.Unlock()
	mgr.startHeartbeat("job-1")
	time.Sleep(100 * time.Millisecond)
}

func TestUpdateHeartbeatEmptyJobID(t *testing.T) {
	mgr := newManager(t, &fakeStore{})
	mgr.updateHeartbeat("", Event{Type: "task", Status: StatusRunning, TaskID: "execute-0"})
}

func TestEventBusNotify(t *testing.T) {
	bus := newEventBus()
	ch := bus.subscribe("job-1")
	bus.notify("job-1")
	select {
	case <-ch:
	default:
		t.Fatalf("expected notification")
	}
	bus.unsubscribe("job-1", ch)
}

func TestEventsSinceUnknown(t *testing.T) {
	mgr := newManager(t, &fakeStore{})

	events, ok, done, err := mgr.EventsSince("missing", 0)
	if err != nil || ok || done || events != nil {
		t.Fatalf("expected missing job, got ok=%v done=%v err=%v events=%v", ok, done, err, events)
	}
}

func TestEventsSinceListError(t *testing.T) {
	queueStore := newQueueStore(t)
	reqJSON, err := jsonMarshal(Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	})
	if err != nil {
		t.Fatalf("jsonMarshal: %v", err)
	}
	if err := queueStore.CreateJob(context.Background(), queue.JobRecord{
		JobID:       "job-1",
		Status:      StatusQueued,
		PrepareKind: "psql",
		ImageID:     "image-1",
		RequestJSON: &reqJSON,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	faulty := &faultQueueStore{
		Store: queueStore,
		listEventsSince: func(context.Context, string, int) ([]queue.EventRecord, error) {
			return nil, errors.New("boom")
		},
	}
	mgr := newManagerWithQueue(t, &fakeStore{}, faulty)

	if _, ok, done, err := mgr.EventsSince("job-1", 0); err == nil || !ok || done {
		t.Fatalf("expected list error, ok=%v done=%v err=%v", ok, done, err)
	}
}

func TestGetUnknownJob(t *testing.T) {
	mgr := newManager(t, &fakeStore{})

	if _, ok := mgr.Get("missing"); ok {
		t.Fatalf("expected missing job")
	}
}

func TestGetIncludesErrorPayload(t *testing.T) {
	queueStore := newQueueStore(t)
	req := Request{PrepareKind: "psql", ImageID: "image-1"}
	reqJSON, err := jsonMarshal(req)
	if err != nil {
		t.Fatalf("jsonMarshal: %v", err)
	}
	errResp := ErrorResponse{Code: "boom", Message: "fail"}
	errJSON, err := json.Marshal(errResp)
	if err != nil {
		t.Fatalf("marshal error: %v", err)
	}
	if err := queueStore.CreateJob(context.Background(), queue.JobRecord{
		JobID:       "job-1",
		Status:      StatusFailed,
		PrepareKind: "psql",
		ImageID:     "image-1",
		RequestJSON: &reqJSON,
		ErrorJSON:   strPtr(string(errJSON)),
		CreatedAt:   time.Now().UTC().Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}

	mgr := newManagerWithQueue(t, &fakeStore{}, queueStore)
	status, ok := mgr.Get("job-1")
	if !ok || status.Error == nil || status.Error.Code != "boom" {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestPrepareRequestPsqlError(t *testing.T) {
	mgr := newManager(t, &fakeStore{})

	if _, err := mgr.prepareRequest(Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-f"},
	}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestIsStateCachedEmpty(t *testing.T) {
	mgr := newManager(t, &fakeStore{})

	cached, err := mgr.isStateCached("")
	if err != nil || cached {
		t.Fatalf("expected empty cache result, cached=%v err=%v", cached, err)
	}
}

func TestIsStateCachedError(t *testing.T) {
	store := &fakeStore{getStateErr: errors.New("boom")}
	mgr := newManager(t, store)

	if _, err := mgr.isStateCached("state-1"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestRandomHex(t *testing.T) {
	value, err := randomHex(4)
	if err != nil {
		t.Fatalf("randomHex: %v", err)
	}
	if len(value) != 8 {
		t.Fatalf("expected 8 chars, got %d", len(value))
	}
	if _, err := hex.DecodeString(value); err != nil {
		t.Fatalf("expected hex string, got %q", value)
	}
}

func TestRandomHexError(t *testing.T) {
	prevReader := randReader
	randReader = errorReader{}
	t.Cleanup(func() { randReader = prevReader })

	if _, err := randomHex(4); err == nil {
		t.Fatalf("expected error")
	}
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) {
	return 0, errors.New("boom")
}

func TestNewManagerDefaultIDGen(t *testing.T) {
	queueStore := newQueueStore(t)
	mgr, err := NewPrepareService(Options{
		Store:          &fakeStore{},
		Queue:          queueStore,
		Runtime:        &fakeRuntime{},
		StateFS:        &fakeStateFS{},
		DBMS:           &fakeDBMS{},
		StateStoreRoot: filepath.Join(t.TempDir(), "state-store"),
		Psql:           &fakePsqlRunner{},
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	if mgr.idGen == nil {
		t.Fatalf("expected id generator")
	}
	if mgr.now == nil {
		t.Fatalf("expected now function")
	}
}

func TestSubmitAsyncRunsJob(t *testing.T) {
	mgr := newManager(t, &fakeStore{})
	mgr.async = true

	if _, err := mgr.Submit(context.Background(), Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
		PlanOnly:    true,
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	index := 0
	for {
		_, ok, done, err := mgr.EventsSince("job-1", index)
		if err != nil {
			t.Fatalf("EventsSince: %v", err)
		}
		if ok && done {
			return
		}
		count, err := mgr.queue.CountEvents(ctx, "job-1")
		if err != nil {
			t.Fatalf("CountEvents: %v", err)
		}
		index = count
		if err := mgr.WaitForEvent(ctx, "job-1", index); err != nil {
			t.Fatalf("job did not finish: %v", err)
		}
	}
}

func TestDeleteUnknownJob(t *testing.T) {
	mgr := newManager(t, &fakeStore{})

	if _, ok := mgr.Delete("missing", deletion.DeleteOptions{}); ok {
		t.Fatalf("expected delete to miss")
	}
}

func TestDeleteJobDryRun(t *testing.T) {
	mgr := newManager(t, &fakeStore{})

	if _, err := mgr.Submit(context.Background(), Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
		PlanOnly:    true,
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	result, ok := mgr.Delete("job-1", deletion.DeleteOptions{DryRun: true})
	if !ok || result.Outcome != deletion.OutcomeWouldDelete {
		t.Fatalf("unexpected delete result: ok=%v result=%+v", ok, result)
	}
}

func TestListJobsMissingJob(t *testing.T) {
	mgr := newManager(t, &fakeStore{})

	if jobs := mgr.ListJobs("missing"); len(jobs) != 0 {
		t.Fatalf("expected no jobs, got %+v", jobs)
	}
}

func TestSubmitCreateStateFails(t *testing.T) {
	store := &fakeStore{createStateErr: errors.New("boom")}
	mgr := newManager(t, store)

	if _, err := mgr.Submit(context.Background(), Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	status, ok := mgr.Get("job-1")
	if !ok || status.Status != StatusFailed || status.Error == nil {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestSubmitCreateInstanceFails(t *testing.T) {
	store := &fakeStore{createInstanceErr: errors.New("boom")}
	mgr := newManager(t, store)

	if _, err := mgr.Submit(context.Background(), Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	status, ok := mgr.Get("job-1")
	if !ok || status.Status != StatusFailed || status.Error == nil {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestSubmitInstanceIDFails(t *testing.T) {
	store := &fakeStore{}
	mgr := newManager(t, store)

	prevReader := randReader
	randReader = errorReader{}
	t.Cleanup(func() { randReader = prevReader })

	if _, err := mgr.Submit(context.Background(), Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	}); err != nil {
		t.Fatalf("Submit: %v", err)
	}

	status, ok := mgr.Get("job-1")
	if !ok || status.Status != StatusFailed || status.Error == nil || status.Error.Message != "cannot generate instance id" {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestRecoverWithFailedTask(t *testing.T) {
	queueStore := newQueueStore(t)
	reqJSON, err := jsonMarshal(Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	})
	if err != nil {
		t.Fatalf("jsonMarshal: %v", err)
	}
	if err := queueStore.CreateJob(context.Background(), queue.JobRecord{
		JobID:       "job-1",
		Status:      StatusRunning,
		PrepareKind: "psql",
		ImageID:     "image-1",
		RequestJSON: &reqJSON,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if err := queueStore.ReplaceTasks(context.Background(), "job-1", []queue.TaskRecord{
		{
			JobID:    "job-1",
			TaskID:   "execute-0",
			Position: 0,
			Type:     "state_execute",
			Status:   StatusFailed,
		},
	}); err != nil {
		t.Fatalf("ReplaceTasks: %v", err)
	}

	mgr := newManagerWithQueue(t, &fakeStore{}, queueStore)
	if err := mgr.Recover(context.Background()); err != nil {
		t.Fatalf("Recover: %v", err)
	}
	status, ok := mgr.Get("job-1")
	if !ok || status.Status != StatusFailed {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestRecoverMissingStateIDFailsJob(t *testing.T) {
	queueStore := newQueueStore(t)
	reqJSON, err := jsonMarshal(Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	})
	if err != nil {
		t.Fatalf("jsonMarshal: %v", err)
	}
	if err := queueStore.CreateJob(context.Background(), queue.JobRecord{
		JobID:       "job-1",
		Status:      StatusRunning,
		PrepareKind: "psql",
		ImageID:     "image-1",
		RequestJSON: &reqJSON,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
	if err := queueStore.ReplaceTasks(context.Background(), "job-1", []queue.TaskRecord{
		{
			JobID:    "job-1",
			TaskID:   "plan",
			Position: 0,
			Type:     "plan",
			Status:   StatusQueued,
		},
	}); err != nil {
		t.Fatalf("ReplaceTasks: %v", err)
	}

	mgr := newManagerWithQueue(t, &fakeStore{}, queueStore)
	if err := mgr.Recover(context.Background()); err != nil {
		t.Fatalf("Recover: %v", err)
	}
	status, ok := mgr.Get("job-1")
	if !ok || status.Status != StatusFailed {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestBuildPlanCacheError(t *testing.T) {
	store := &fakeStore{getStateErr: errors.New("boom")}
	mgr := newManager(t, store)
	prepared, err := mgr.prepareRequest(Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	})
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	if errResp := mgr.ensureResolvedImageID(context.Background(), "job-1", &prepared, nil); errResp != nil {
		t.Fatalf("ensureResolvedImageID: %+v", errResp)
	}
	if _, _, errResp := mgr.buildPlan(context.Background(), "job-1", prepared); errResp == nil {
		t.Fatalf("expected error")
	}
}

func TestBuildPlanLiquibaseUsesChangesets(t *testing.T) {
	temp := t.TempDir()
	changelog := filepath.Join(temp, "changelog.xml")
	writeTempFile(t, changelog, "<databaseChangeLog></databaseChangeLog>")

	liquibaseOutput := strings.Join([]string{
		"-- Changeset changelog.xml::1::dev",
		"CREATE TABLE test(id INT);",
		"-- Changeset changelog.xml::2::dev",
		"ALTER TABLE test ADD COLUMN name TEXT;",
	}, "\n")
	liquibase := &fakeLiquibaseRunner{output: liquibaseOutput}
	runtime := &fakeRuntime{}
	mgr := newManagerWithDeps(t, &fakeStore{}, newQueueStore(t), &testDeps{
		runtime:   runtime,
		liquibase: liquibase,
	})

	prepared, err := mgr.prepareRequest(Request{
		PrepareKind:   "lb",
		ImageID:       "image-1@sha256:resolved",
		LiquibaseArgs: []string{"update", "--changelog-file", changelog},
	})
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	if errResp := mgr.ensureResolvedImageID(context.Background(), "job-1", &prepared, nil); errResp != nil {
		t.Fatalf("ensureResolvedImageID: %+v", errResp)
	}

	tasks, stateID, errResp := mgr.buildPlan(context.Background(), "job-1", prepared)
	if errResp != nil {
		t.Fatalf("buildPlan: %+v", errResp)
	}
	if len(liquibase.runs) != 1 {
		t.Fatalf("expected liquibase run, got %+v", liquibase.runs)
	}
	if !containsArg(liquibase.runs[0].Args, "updateSQL") {
		t.Fatalf("expected updateSQL command, got %+v", liquibase.runs[0].Args)
	}
	if len(tasks) != 4 {
		t.Fatalf("expected 4 tasks, got %d", len(tasks))
	}
	first := tasks[1]
	second := tasks[2]
	if first.Type != "state_execute" || second.Type != "state_execute" {
		t.Fatalf("expected state_execute tasks, got %+v", tasks)
	}
	if first.ChangesetID == "" || second.ChangesetID == "" {
		t.Fatalf("expected changeset metadata, got %+v", tasks)
	}
	if first.Input == nil || first.Input.Kind != "image" {
		t.Fatalf("expected first input to be image, got %+v", first.Input)
	}
	if second.Input == nil || second.Input.Kind != "state" || second.Input.ID != first.OutputStateID {
		t.Fatalf("expected second input to match first output, got %+v", second.Input)
	}
	if stateID != second.OutputStateID {
		t.Fatalf("expected final state id to match last output, got %s", stateID)
	}
}

func TestRunJobTaskFailed(t *testing.T) {
	queueStore := newQueueStore(t)
	req := Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	}
	createJobRecord(t, queueStore, "job-1", req, StatusQueued)
	if err := queueStore.ReplaceTasks(context.Background(), "job-1", []queue.TaskRecord{
		{
			JobID:    "job-1",
			TaskID:   "execute-0",
			Position: 0,
			Type:     "state_execute",
			Status:   StatusFailed,
		},
	}); err != nil {
		t.Fatalf("ReplaceTasks: %v", err)
	}

	mgr := newManagerWithQueue(t, &fakeStore{}, queueStore)
	prepared, err := mgr.prepareRequest(req)
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	mgr.runJob(prepared, "job-1")

	status, ok := mgr.Get("job-1")
	if !ok || status.Status != StatusFailed {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestRunJobMissingOutputState(t *testing.T) {
	queueStore := newQueueStore(t)
	req := Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	}
	createJobRecord(t, queueStore, "job-1", req, StatusQueued)
	if err := queueStore.ReplaceTasks(context.Background(), "job-1", []queue.TaskRecord{
		{
			JobID:    "job-1",
			TaskID:   "plan",
			Position: 0,
			Type:     "plan",
			Status:   StatusQueued,
		},
	}); err != nil {
		t.Fatalf("ReplaceTasks: %v", err)
	}

	mgr := newManagerWithQueue(t, &fakeStore{}, queueStore)
	prepared, err := mgr.prepareRequest(req)
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	mgr.runJob(prepared, "job-1")

	status, ok := mgr.Get("job-1")
	if !ok || status.Status != StatusFailed {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestRunJobPlanOnlyMarkTasksFail(t *testing.T) {
	queueStore := newQueueStore(t)
	faulty := &faultQueueStore{
		Store: queueStore,
		updateTask: func(context.Context, string, string, queue.TaskUpdate) error {
			return errors.New("boom")
		},
	}
	req := Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
		PlanOnly:    true,
	}
	createJobRecord(t, queueStore, "job-1", req, StatusQueued)

	mgr := newManagerWithQueue(t, &fakeStore{}, faulty)
	prepared, err := mgr.prepareRequest(req)
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	mgr.runJob(prepared, "job-1")

	status, ok := mgr.Get("job-1")
	if !ok || status.Status != StatusFailed {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestRunJobUpdateTaskStatusFails(t *testing.T) {
	queueStore := newQueueStore(t)
	faulty := &faultQueueStore{
		Store: queueStore,
		updateTask: func(context.Context, string, string, queue.TaskUpdate) error {
			return errors.New("boom")
		},
	}
	req := Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	}
	createJobRecord(t, queueStore, "job-1", req, StatusQueued)

	mgr := newManagerWithQueue(t, &fakeStore{}, faulty)
	prepared, err := mgr.prepareRequest(req)
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	mgr.runJob(prepared, "job-1")

	status, ok := mgr.Get("job-1")
	if !ok || status.Status != StatusFailed {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestRunJobStateExecuteMissingOutputState(t *testing.T) {
	queueStore := newQueueStore(t)
	req := Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	}
	createJobRecord(t, queueStore, "job-1", req, StatusQueued)
	if err := queueStore.ReplaceTasks(context.Background(), "job-1", []queue.TaskRecord{
		{
			JobID:    "job-1",
			TaskID:   "execute-0",
			Position: 0,
			Type:     "state_execute",
			Status:   StatusQueued,
		},
	}); err != nil {
		t.Fatalf("ReplaceTasks: %v", err)
	}

	mgr := newManagerWithQueue(t, &fakeStore{}, queueStore)
	prepared, err := mgr.prepareRequest(req)
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	mgr.runJob(prepared, "job-1")

	status, ok := mgr.Get("job-1")
	if !ok || status.Status != StatusFailed {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestRunJobSkipsSucceededTask(t *testing.T) {
	queueStore := newQueueStore(t)
	req := Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	}
	createJobRecord(t, queueStore, "job-1", req, StatusQueued)
	if err := queueStore.ReplaceTasks(context.Background(), "job-1", []queue.TaskRecord{
		{
			JobID:    "job-1",
			TaskID:   "plan",
			Position: 0,
			Type:     "plan",
			Status:   StatusSucceeded,
		},
		{
			JobID:         "job-1",
			TaskID:        "execute-0",
			Position:      1,
			Type:          "state_execute",
			Status:        StatusSucceeded,
			OutputStateID: strPtr("state-1"),
		},
		{
			JobID:    "job-1",
			TaskID:   "prepare-instance",
			Position: 2,
			Type:     "prepare_instance",
			Status:   StatusQueued,
		},
	}); err != nil {
		t.Fatalf("ReplaceTasks: %v", err)
	}

	store := &fakeStore{statesByID: map[string]store.StateEntry{"state-1": {StateID: "state-1", ImageID: "image-1"}}}
	mgr := newManagerWithQueue(t, store, queueStore)
	prepared, err := mgr.prepareRequest(req)
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	paths, err := resolveStatePaths(mgr.stateStoreRoot, "image-1", "state-1", mgr.statefs)
	if err != nil {
		t.Fatalf("resolveStatePaths: %v", err)
	}
	if err := os.MkdirAll(paths.stateDir, 0o700); err != nil {
		t.Fatalf("mkdir state dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(paths.stateDir, "PG_VERSION"), []byte("17"), 0o600); err != nil {
		t.Fatalf("write PG_VERSION: %v", err)
	}
	mgr.runJob(prepared, "job-1")

	status, ok := mgr.Get("job-1")
	if !ok || status.Status != StatusSucceeded || status.Result == nil {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestRunJobCreateInstanceAfterLoopFails(t *testing.T) {
	queueStore := newQueueStore(t)
	req := Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	}
	createJobRecord(t, queueStore, "job-1", req, StatusQueued)
	if err := queueStore.ReplaceTasks(context.Background(), "job-1", []queue.TaskRecord{
		{
			JobID:    "job-1",
			TaskID:   "plan",
			Position: 0,
			Type:     "plan",
			Status:   StatusQueued,
		},
		{
			JobID:         "job-1",
			TaskID:        "execute-0",
			Position:      1,
			Type:          "state_execute",
			Status:        StatusQueued,
			OutputStateID: strPtr("state-1"),
		},
	}); err != nil {
		t.Fatalf("ReplaceTasks: %v", err)
	}

	store := &fakeStore{createInstanceErr: errors.New("boom")}
	mgr := newManagerWithQueue(t, store, queueStore)
	prepared, err := mgr.prepareRequest(req)
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	mgr.runJob(prepared, "job-1")

	status, ok := mgr.Get("job-1")
	if !ok || status.Status != StatusFailed {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestRunJobPrepareInstanceUpdateTaskError(t *testing.T) {
	queueStore := newQueueStore(t)
	req := Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	}
	createJobRecord(t, queueStore, "job-1", req, StatusQueued)
	if err := queueStore.ReplaceTasks(context.Background(), "job-1", []queue.TaskRecord{
		{
			JobID:    "job-1",
			TaskID:   "plan",
			Position: 0,
			Type:     "plan",
			Status:   StatusQueued,
		},
		{
			JobID:    "job-1",
			TaskID:   "prepare-instance",
			Position: 1,
			Type:     "prepare_instance",
			Status:   StatusQueued,
		},
	}); err != nil {
		t.Fatalf("ReplaceTasks: %v", err)
	}
	updateCalls := 0
	faulty := &faultQueueStore{
		Store: queueStore,
		updateTask: func(ctx context.Context, jobID string, taskID string, update queue.TaskUpdate) error {
			updateCalls++
			if updateCalls == 4 {
				return errors.New("boom")
			}
			return queueStore.UpdateTask(ctx, jobID, taskID, update)
		},
	}
	mgr := newManagerWithQueue(t, &fakeStore{}, faulty)
	prepared, err := mgr.prepareRequest(req)
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	mgr.runJob(prepared, "job-1")

	status, ok := mgr.Get("job-1")
	if !ok || status.Status != StatusFailed {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestRunJobCancelBeforeInitialGetJob(t *testing.T) {
	queueStore := newQueueStore(t)
	req := Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-c", "select 1"},
	}
	createJobRecord(t, queueStore, "job-1", req, StatusQueued)

	enterGetJob := make(chan struct{})
	releaseGetJob := make(chan struct{})
	var firstGet sync.Once
	faulty := &faultQueueStore{
		Store: queueStore,
		getJob: func(ctx context.Context, jobID string) (queue.JobRecord, bool, error) {
			firstGet.Do(func() {
				close(enterGetJob)
				<-releaseGetJob
			})
			if err := ctx.Err(); err != nil {
				return queue.JobRecord{}, false, err
			}
			return queueStore.GetJob(ctx, jobID)
		},
	}

	mgr := newManagerWithQueue(t, &fakeStore{}, faulty)
	prepared, err := mgr.prepareRequest(req)
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}

	done := make(chan struct{})
	go func() {
		mgr.runJob(prepared, "job-1")
		close(done)
	}()

	select {
	case <-enterGetJob:
	case <-time.After(5 * time.Second):
		t.Fatalf("timeout waiting for initial GetJob")
	}

	runner := mgr.getRunner("job-1")
	if runner == nil {
		t.Fatalf("runner was not registered")
	}
	runner.cancel()
	close(releaseGetJob)

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatalf("timeout waiting for runJob completion")
	}

	status, ok := mgr.Get("job-1")
	if !ok {
		t.Fatalf("expected job status")
	}
	if status.Status != StatusFailed || status.Error == nil {
		t.Fatalf("unexpected status: %+v", status)
	}
	if status.Error.Code != "cancelled" {
		t.Fatalf("expected cancelled error code, got %+v", status.Error)
	}
}

func TestWaitForEventNotFound(t *testing.T) {
	mgr := newManager(t, &fakeStore{})
	if err := mgr.WaitForEvent(context.Background(), "missing", 0); err == nil {
		t.Fatalf("expected error")
	}
}

func TestEventsSinceError(t *testing.T) {
	queueStore := newQueueStore(t)
	if err := queueStore.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	mgr := newManagerWithQueue(t, &fakeStore{}, queueStore)
	if _, _, _, err := mgr.EventsSince("job-1", 0); err == nil {
		t.Fatalf("expected error")
	}
}

func TestFormatTimeHelpers(t *testing.T) {
	if formatTime(time.Time{}) != nil {
		t.Fatalf("expected nil for zero time")
	}
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	if formatTime(now) == nil {
		t.Fatalf("expected formatted time")
	}
	if formatTimePtr(nil) != nil {
		t.Fatalf("expected nil for nil pointer")
	}
	if formatTimePtr(&now) == nil {
		t.Fatalf("expected formatted time for pointer")
	}
}

func TestSucceedUpdateJobError(t *testing.T) {
	queueStore := newQueueStore(t)
	faulty := &faultQueueStore{
		Store: queueStore,
		updateJob: func(context.Context, string, queue.JobUpdate) error {
			return errors.New("boom")
		},
	}
	mgr := newManagerWithQueue(t, &fakeStore{}, faulty)
	req := Request{PrepareKind: "psql", ImageID: "image-1"}
	createJobRecord(t, queueStore, "job-1", req, StatusRunning)

	result := Result{
		DSN:                   "dsn",
		InstanceID:            "inst",
		StateID:               "state-1",
		ImageID:               "image-1",
		PrepareKind:           "psql",
		PrepareArgsNormalized: "args",
	}
	if err := mgr.succeed("job-1", result); err == nil {
		t.Fatalf("expected error")
	}
}

func TestSucceedAppendEventError(t *testing.T) {
	queueStore := newQueueStore(t)
	faulty := &faultQueueStore{
		Store: queueStore,
		appendEvent: func(context.Context, queue.EventRecord) (int64, error) {
			return 0, errors.New("boom")
		},
	}
	mgr := newManagerWithQueue(t, &fakeStore{}, faulty)
	req := Request{PrepareKind: "psql", ImageID: "image-1"}
	createJobRecord(t, queueStore, "job-1", req, StatusRunning)

	result := Result{
		DSN:                   "dsn",
		InstanceID:            "inst",
		StateID:               "state-1",
		ImageID:               "image-1",
		PrepareKind:           "psql",
		PrepareArgsNormalized: "args",
	}
	if err := mgr.succeed("job-1", result); err == nil {
		t.Fatalf("expected error")
	}
}

func TestSucceedPlanUpdateJobError(t *testing.T) {
	queueStore := newQueueStore(t)
	faulty := &faultQueueStore{
		Store: queueStore,
		updateJob: func(context.Context, string, queue.JobUpdate) error {
			return errors.New("boom")
		},
	}
	mgr := newManagerWithQueue(t, &fakeStore{}, faulty)
	req := Request{PrepareKind: "psql", ImageID: "image-1"}
	createJobRecord(t, queueStore, "job-1", req, StatusRunning)

	if err := mgr.succeedPlan("job-1"); err == nil {
		t.Fatalf("expected error")
	}
}

func TestFailJobAppendEventError(t *testing.T) {
	queueStore := newQueueStore(t)
	faulty := &faultQueueStore{
		Store: queueStore,
		appendEvent: func(context.Context, queue.EventRecord) (int64, error) {
			return 0, errors.New("boom")
		},
	}
	mgr := newManagerWithQueue(t, &fakeStore{}, faulty)
	req := Request{PrepareKind: "psql", ImageID: "image-1"}
	createJobRecord(t, queueStore, "job-1", req, StatusRunning)

	if err := mgr.failJob("job-1", errorResponse("boom", "fail", "")); err == nil {
		t.Fatalf("expected error")
	}
}

func TestFailJobUpdateJobError(t *testing.T) {
	queueStore := newQueueStore(t)
	faulty := &faultQueueStore{
		Store: queueStore,
		updateJob: func(context.Context, string, queue.JobUpdate) error {
			return errors.New("boom")
		},
	}
	mgr := newManagerWithQueue(t, &fakeStore{}, faulty)
	req := Request{PrepareKind: "psql", ImageID: "image-1"}
	createJobRecord(t, queueStore, "job-1", req, StatusRunning)

	if err := mgr.failJob("job-1", errorResponse("boom", "fail", "")); err == nil {
		t.Fatalf("expected error")
	}
}

type testDeps struct {
	runtime   *fakeRuntime
	statefs   *fakeStateFS
	dbms      *fakeDBMS
	psql      psqlRunner
	liquibase liquibaseRunner
	stateRoot string
	config    config.Store
	validate  func(root string) error
}

type fakeConfigStore struct {
	value  any
	values map[string]any
	err    error
}

func (f *fakeConfigStore) Get(path string, effective bool) (any, error) {
	if f.err != nil {
		return nil, f.err
	}
	if f.values != nil {
		value, ok := f.values[path]
		if !ok {
			return nil, errors.New("missing value")
		}
		return value, nil
	}
	switch path {
	case "orchestrator.jobs.maxIdentical", "log.level":
		return f.value, nil
	default:
		return nil, errors.New("missing value")
	}
}

func (f *fakeConfigStore) Set(path string, value any) (any, error) {
	return nil, nil
}

func (f *fakeConfigStore) Remove(path string) (any, error) {
	return nil, nil
}

func (f *fakeConfigStore) Schema() any {
	return map[string]any{}
}

type signatureQueueStore struct {
	queue.Store
	listErr   error
	deleteErr error
}

func (s *signatureQueueStore) ListJobsBySignature(ctx context.Context, signature string, statuses []string) ([]queue.JobRecord, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.Store.ListJobsBySignature(ctx, signature, statuses)
}

func (s *signatureQueueStore) DeleteJob(ctx context.Context, jobID string) error {
	if s.deleteErr != nil {
		return s.deleteErr
	}
	return s.Store.DeleteJob(ctx, jobID)
}

func newManager(t *testing.T, store store.Store) *PrepareService {
	t.Helper()
	return newManagerWithQueue(t, store, newQueueStore(t))
}

func newManagerWithQueue(t *testing.T, store store.Store, queueStore queue.Store) *PrepareService {
	t.Helper()
	return newManagerWithDeps(t, store, queueStore, nil)
}

func newManagerWithDeps(t *testing.T, store store.Store, queueStore queue.Store, deps *testDeps) *PrepareService {
	t.Helper()
	now := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	if deps == nil {
		deps = &testDeps{}
	}
	if deps.runtime == nil {
		deps.runtime = &fakeRuntime{}
	}
	if deps.statefs == nil {
		deps.statefs = &fakeStateFS{}
	}
	if deps.dbms == nil {
		deps.dbms = &fakeDBMS{}
	}
	if deps.psql == nil {
		deps.psql = &fakePsqlRunner{}
	}
	if deps.liquibase == nil {
		deps.liquibase = &fakeLiquibaseRunner{}
	}
	if deps.config == nil {
		deps.config = &fakeConfigStore{
			values: map[string]any{
				"orchestrator.jobs.maxIdentical": 2,
				"log.level":                      "debug",
				"cache.capacity.maxBytes":        int64(0),
				"cache.capacity.reserveBytes":    int64(0),
				"cache.capacity.highWatermark":   0.90,
				"cache.capacity.lowWatermark":    0.80,
				"cache.capacity.minStateAge":     "10m",
			},
		}
	}
	stateRoot := deps.stateRoot
	if stateRoot == "" {
		stateRoot = filepath.Join(t.TempDir(), "state-store")
	}
	mgr, err := NewPrepareService(Options{
		Store:          store,
		Queue:          queueStore,
		Runtime:        deps.runtime,
		StateFS:        deps.statefs,
		ValidateStore:  deps.validate,
		DBMS:           deps.dbms,
		StateStoreRoot: stateRoot,
		Config:         deps.config,
		Psql:           deps.psql,
		Liquibase:      deps.liquibase,
		Version:        "v1",
		Now:            func() time.Time { return now },
		IDGen:          func() (string, error) { return "job-1", nil },
		Async:          false,
	})
	if err != nil {
		t.Fatalf("NewManager: %v", err)
	}
	return mgr
}

func newQueueStore(t *testing.T) queue.Store {
	t.Helper()
	// Orchestration tests need real SQL constraints, not disk durability. Queue
	// reopen/migration tests and managed recovery fixtures use file-backed stores.
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open queue database: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	store, err := queue.New(db)
	if err != nil {
		t.Fatalf("open queue: %v", err)
	}
	return store
}

func jsonMarshal(req Request) (string, error) {
	data, err := json.Marshal(req)
	if err != nil {
		return "", err
	}
	return string(data), nil
}

func TestMaxIdenticalJobsDefaults(t *testing.T) {
	if got := maxIdenticalJobs(nil); got != defaultMaxIdenticalJobs {
		t.Fatalf("expected default %d, got %d", defaultMaxIdenticalJobs, got)
	}
	if got := maxIdenticalJobs(&fakeConfigStore{value: nil}); got != defaultMaxIdenticalJobs {
		t.Fatalf("expected default for nil value, got %d", got)
	}
	if got := maxIdenticalJobs(&fakeConfigStore{value: "bad"}); got != defaultMaxIdenticalJobs {
		t.Fatalf("expected default for invalid value, got %d", got)
	}
	if got := maxIdenticalJobs(&fakeConfigStore{err: errors.New("boom")}); got != defaultMaxIdenticalJobs {
		t.Fatalf("expected default for config error, got %d", got)
	}
}

func TestMaxIdenticalJobsFromConfig(t *testing.T) {
	if got := maxIdenticalJobs(&fakeConfigStore{value: 4}); got != 4 {
		t.Fatalf("expected 4, got %d", got)
	}
	if got := maxIdenticalJobs(&fakeConfigStore{value: float64(3)}); got != 3 {
		t.Fatalf("expected 3, got %d", got)
	}
	if got := maxIdenticalJobs(&fakeConfigStore{value: json.Number("5")}); got != 5 {
		t.Fatalf("expected 5, got %d", got)
	}
}

func TestConfigValueToInt(t *testing.T) {
	cases := []struct {
		value  any
		want   int
		wantOK bool
	}{
		{value: int(1), want: 1, wantOK: true},
		{value: int32(2), want: 2, wantOK: true},
		{value: int64(3), want: 3, wantOK: true},
		{value: float32(4), want: 4, wantOK: true},
		{value: float32(4.5), wantOK: false},
		{value: float64(5), want: 5, wantOK: true},
		{value: float64(5.5), wantOK: false},
		{value: json.Number("6"), want: 6, wantOK: true},
		{value: json.Number("1e2"), wantOK: false},
		{value: json.Number("999999999999999999999999"), wantOK: false},
		{value: "nope", wantOK: false},
	}
	for _, tc := range cases {
		got, ok := configValueToInt(tc.value)
		if ok != tc.wantOK || (ok && got != tc.want) {
			t.Fatalf("configValueToInt(%#v)=%d,%v; want %d,%v", tc.value, got, ok, tc.want, tc.wantOK)
		}
	}
}

func TestRemoveJobDirNoopAndDelete(t *testing.T) {
	mgr := &PrepareService{}
	if err := mgr.removeJobDir("job-1"); err != nil {
		t.Fatalf("expected empty state store root to be ignored: %v", err)
	}
	mgr.stateStoreRoot = t.TempDir()
	if err := mgr.removeJobDir(""); err != nil {
		t.Fatalf("expected empty job id to be ignored: %v", err)
	}
	jobDir := filepath.Join(mgr.stateStoreRoot, "jobs", "job-1")
	if err := os.MkdirAll(jobDir, 0o700); err != nil {
		t.Fatalf("mkdir job dir: %v", err)
	}
	if err := mgr.removeJobDir("job-1"); err != nil {
		t.Fatalf("removeJobDir: %v", err)
	}
	if _, err := os.Stat(jobDir); !os.IsNotExist(err) {
		t.Fatalf("expected job dir removed")
	}
}

func TestRemoveJobDirBtrfsRuntimeSubvolume(t *testing.T) {
	snap := &fakeStateFS{kind: "btrfs"}
	mgr := &PrepareService{
		stateStoreRoot: t.TempDir(),
		statefs:        snap,
	}
	jobDir := filepath.Join(mgr.stateStoreRoot, "jobs", "job-1")
	runtimeDir := filepath.Join(jobDir, "runtime")
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		t.Fatalf("mkdir runtime dir: %v", err)
	}
	if err := mgr.removeJobDir("job-1"); err != nil {
		t.Fatalf("removeJobDir: %v", err)
	}
	if len(snap.removeCalls) != 1 || snap.removeCalls[0] != runtimeDir {
		t.Fatalf("expected remove on runtime dir, got %+v", snap.removeCalls)
	}
	if _, err := os.Stat(jobDir); !os.IsNotExist(err) {
		t.Fatalf("expected job dir removed")
	}
}

func TestEnsureResolvedImageIDUsesTasks(t *testing.T) {
	queueStore := newQueueStore(t)
	mgr := newManagerWithQueue(t, &fakeStore{}, queueStore)
	req := Request{PrepareKind: "psql", ImageID: "image-1", PsqlArgs: []string{"-c", "select 1"}}
	prepared, err := mgr.prepareRequest(req)
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	tasks := []queue.TaskRecord{
		{Type: "resolve_image", ResolvedImageID: strPtr("image-1@sha256:resolved")},
	}
	if errResp := mgr.ensureResolvedImageID(context.Background(), "job-1", &prepared, tasks); errResp != nil {
		t.Fatalf("ensureResolvedImageID: %+v", errResp)
	}
	if prepared.resolvedImageID != "image-1@sha256:resolved" {
		t.Fatalf("unexpected resolved image: %s", prepared.resolvedImageID)
	}
}

func TestEnsureResolvedImageIDNilPrepared(t *testing.T) {
	mgr := newManager(t, &fakeStore{})
	if errResp := mgr.ensureResolvedImageID(context.Background(), "job-1", nil, nil); errResp == nil {
		t.Fatalf("expected error for nil prepared request")
	}
}

func TestEnsureResolvedImageIDUsesRequestWhenTasksPresent(t *testing.T) {
	queueStore := newQueueStore(t)
	mgr := newManagerWithQueue(t, &fakeStore{}, queueStore)
	req := Request{PrepareKind: "psql", ImageID: "image-1", PsqlArgs: []string{"-c", "select 1"}}
	prepared, err := mgr.prepareRequest(req)
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	tasks := []queue.TaskRecord{
		{Type: "state_execute"},
	}
	if errResp := mgr.ensureResolvedImageID(context.Background(), "job-1", &prepared, tasks); errResp != nil {
		t.Fatalf("ensureResolvedImageID: %+v", errResp)
	}
	if prepared.resolvedImageID != "image-1" {
		t.Fatalf("unexpected resolved image: %s", prepared.resolvedImageID)
	}
}

func TestEnsureResolvedImageIDSkipsResolveForDigest(t *testing.T) {
	queueStore := newQueueStore(t)
	mgr := newManagerWithQueue(t, &fakeStore{}, queueStore)
	req := Request{PrepareKind: "psql", ImageID: "image-1@sha256:abc", PsqlArgs: []string{"-c", "select 1"}}
	prepared, err := mgr.prepareRequest(req)
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	prepared.resolvedImageID = ""
	if errResp := mgr.ensureResolvedImageID(context.Background(), "job-1", &prepared, nil); errResp != nil {
		t.Fatalf("ensureResolvedImageID: %+v", errResp)
	}
	if prepared.resolvedImageID != req.ImageID {
		t.Fatalf("expected resolved image to match digest input")
	}
}

func TestEnsureResolvedImageIDResolveErrors(t *testing.T) {
	queueStore := newQueueStore(t)
	rt := &fakeRuntime{resolveErr: errors.New("boom")}
	mgr := newManagerWithDeps(t, &fakeStore{}, queueStore, &testDeps{runtime: rt})
	req := Request{PrepareKind: "psql", ImageID: "image-1", PsqlArgs: []string{"-c", "select 1"}}
	prepared, err := mgr.prepareRequest(req)
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	if errResp := mgr.ensureResolvedImageID(context.Background(), "job-1", &prepared, nil); errResp == nil {
		t.Fatalf("expected resolve error")
	}
}

func TestEnsureResolvedImageIDResolveEmpty(t *testing.T) {
	queueStore := newQueueStore(t)
	rt := &fakeRuntime{resolvedImage: " "}
	mgr := newManagerWithDeps(t, &fakeStore{}, queueStore, &testDeps{runtime: rt})
	req := Request{PrepareKind: "psql", ImageID: "image-1", PsqlArgs: []string{"-c", "select 1"}}
	prepared, err := mgr.prepareRequest(req)
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	if errResp := mgr.ensureResolvedImageID(context.Background(), "job-1", &prepared, nil); errResp == nil {
		t.Fatalf("expected empty resolved error")
	}
}

func TestEnsureResolvedImageIDResolvesImage(t *testing.T) {
	queueStore := newQueueStore(t)
	rt := &loggingRuntime{fakeRuntime: fakeRuntime{resolvedImage: "image-1@sha256:resolved"}}
	mgr := newManagerWithQueue(t, &fakeStore{}, queueStore)
	mgr.runtime = rt
	req := Request{PrepareKind: "psql", ImageID: "image-1", PsqlArgs: []string{"-c", "select 1"}}
	prepared, err := mgr.prepareRequest(req)
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	createJobRecord(t, queueStore, "job-1", req, StatusRunning)
	if errResp := mgr.ensureResolvedImageID(context.Background(), "job-1", &prepared, nil); errResp != nil {
		t.Fatalf("ensureResolvedImageID: %+v", errResp)
	}
	if prepared.resolvedImageID != "image-1@sha256:resolved" {
		t.Fatalf("unexpected resolved image: %s", prepared.resolvedImageID)
	}
	events, ok, _, err := mgr.EventsSince("job-1", 0)
	if err != nil || !ok {
		t.Fatalf("EventsSince: ok=%v err=%v", ok, err)
	}
	foundResolve := false
	foundResolved := false
	for _, event := range events {
		if event.Message == "resolve image image-1" {
			foundResolve = true
		}
		if event.Message == "resolved image image-1@sha256:resolved" {
			foundResolved = true
		}
	}
	if !foundResolve || !foundResolved {
		t.Fatalf("expected resolve log events, got %+v", events)
	}
}

func TestEnsureResolvedImageIDAlreadyResolved(t *testing.T) {
	queueStore := newQueueStore(t)
	mgr := newManagerWithQueue(t, &fakeStore{}, queueStore)
	req := Request{PrepareKind: "psql", ImageID: "image-1", PsqlArgs: []string{"-c", "select 1"}}
	prepared, err := mgr.prepareRequest(req)
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	prepared.resolvedImageID = "image-1@sha256:resolved"
	if errResp := mgr.ensureResolvedImageID(context.Background(), "job-1", &prepared, nil); errResp != nil {
		t.Fatalf("ensureResolvedImageID: %+v", errResp)
	}
	if prepared.resolvedImageID != "image-1@sha256:resolved" {
		t.Fatalf("expected resolved image to be preserved")
	}
}

func createJobRecord(t *testing.T, queueStore queue.Store, jobID string, req Request, status string) {
	t.Helper()
	reqJSON, err := jsonMarshal(req)
	if err != nil {
		t.Fatalf("jsonMarshal: %v", err)
	}
	if err := queueStore.CreateJob(context.Background(), queue.JobRecord{
		JobID:       jobID,
		Status:      status,
		PrepareKind: req.PrepareKind,
		ImageID:     req.ImageID,
		PlanOnly:    req.PlanOnly,
		RequestJSON: &reqJSON,
		CreatedAt:   time.Now().UTC().Format(time.RFC3339Nano),
	}); err != nil {
		t.Fatalf("CreateJob: %v", err)
	}
}

func TestManagerLogHelpers(t *testing.T) {
	prev := log.Writer()
	log.SetOutput(io.Discard)
	t.Cleanup(func() { log.SetOutput(prev) })

	mgr := &PrepareService{}
	mgr.logJob("job-1", "created")
	mgr.logJob("", "created")
	mgr.logTask("job-1", "task-1", "status=%s", StatusQueued)
	mgr.logTask("", "", "noop")
}
