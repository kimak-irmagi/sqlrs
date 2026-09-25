package httpapi

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sqlrs/engine-local/internal/prepare"
	"github.com/sqlrs/engine-local/internal/prepare/queue"
	engineRuntime "github.com/sqlrs/engine-local/internal/runtime"
	"github.com/sqlrs/engine-local/internal/statefs"
	"github.com/sqlrs/engine-local/internal/store"
)

type fakeRuntime struct{}

func (f *fakeRuntime) InitBase(ctx context.Context, imageID string, dataDir string) error {
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dataDir, "PG_VERSION"), []byte("17"), 0o600)
}

func (f *fakeRuntime) ResolveImage(ctx context.Context, imageID string) (string, error) {
	return imageID + "@sha256:resolved", nil
}

func (f *fakeRuntime) Start(ctx context.Context, req engineRuntime.StartRequest) (engineRuntime.Instance, error) {
	return engineRuntime.Instance{ID: "container-1", Host: "127.0.0.1", Port: 5432}, nil
}

func (f *fakeRuntime) Stop(ctx context.Context, id string) error {
	return nil
}

func (f *fakeRuntime) Exec(ctx context.Context, id string, req engineRuntime.ExecRequest) (string, error) {
	return "", nil
}

func (f *fakeRuntime) WaitForReady(ctx context.Context, id string, timeout time.Duration) error {
	return nil
}

type fakeStateFS struct{}

var httpapiTestLayoutFS = statefs.NewManager(statefs.Options{Backend: "copy"})

func (f *fakeStateFS) Kind() string {
	return "fake"
}

func (f *fakeStateFS) Capabilities() statefs.Capabilities {
	return statefs.Capabilities{
		RequiresDBStop:        true,
		SupportsWritableClone: true,
		SupportsSendReceive:   false,
	}
}

func (f *fakeStateFS) Validate(root string) error {
	return nil
}

func (f *fakeStateFS) BaseDir(root, imageID string) (string, error) {
	return httpapiTestLayoutFS.BaseDir(root, imageID)
}

func (f *fakeStateFS) StatesDir(root, imageID string) (string, error) {
	return httpapiTestLayoutFS.StatesDir(root, imageID)
}

func (f *fakeStateFS) StateDir(root, imageID, stateID string) (string, error) {
	return httpapiTestLayoutFS.StateDir(root, imageID, stateID)
}

func (f *fakeStateFS) JobRuntimeDir(root, jobID string) (string, error) {
	return httpapiTestLayoutFS.JobRuntimeDir(root, jobID)
}

func (f *fakeStateFS) EnsureBaseDir(ctx context.Context, baseDir string) error {
	return os.MkdirAll(baseDir, 0o700)
}

func (f *fakeStateFS) EnsureStateDir(ctx context.Context, stateDir string) error {
	return os.MkdirAll(stateDir, 0o700)
}

func (f *fakeStateFS) Clone(ctx context.Context, srcDir string, destDir string) (statefs.CloneResult, error) {
	if err := os.MkdirAll(destDir, 0o700); err != nil {
		return statefs.CloneResult{}, err
	}
	return statefs.CloneResult{
		MountDir: destDir,
		Cleanup: func() error {
			return os.RemoveAll(destDir)
		},
	}, nil
}

func (f *fakeStateFS) Snapshot(ctx context.Context, srcDir string, destDir string) error {
	return os.MkdirAll(destDir, 0o700)
}

func (f *fakeStateFS) RemovePath(ctx context.Context, dir string) error {
	return os.RemoveAll(dir)
}

type fakeDBMS struct{}

func (f *fakeDBMS) PrepareSnapshot(ctx context.Context, instance engineRuntime.Instance) error {
	return nil
}

func (f *fakeDBMS) ResumeSnapshot(ctx context.Context, instance engineRuntime.Instance) error {
	return nil
}

type fakePsqlRunner struct{}

func (f *fakePsqlRunner) Run(ctx context.Context, instance engineRuntime.Instance, req prepare.PsqlRunRequest) (string, error) {
	return "", nil
}

func newPrepareManager(t *testing.T, store store.Store, queueStore queue.Store, opts ...func(*prepare.Options)) *prepare.PrepareService {
	t.Helper()
	stateRoot := filepath.Join(t.TempDir(), "state-store")
	options := prepare.Options{
		Store:          store,
		Queue:          queueStore,
		Runtime:        &fakeRuntime{},
		StateFS:        &fakeStateFS{},
		DBMS:           &fakeDBMS{},
		StateStoreRoot: stateRoot,
		Psql:           &fakePsqlRunner{},
		Version:        "test",
		Async:          false,
	}
	for _, opt := range opts {
		opt(&options)
	}
	mgr, err := prepare.NewPrepareService(options)
	if err != nil {
		t.Fatalf("prepare service: %v", err)
	}
	return mgr
}

func TestFakeStateFSJobRuntimeDir(t *testing.T) {
	root := t.TempDir()
	fs := &fakeStateFS{}
	dir, err := fs.JobRuntimeDir(root, "job-1")
	if err != nil {
		t.Fatalf("JobRuntimeDir: %v", err)
	}
	expected := filepath.Join(root, "jobs", "job-1", "runtime")
	if dir != expected {
		t.Fatalf("expected job runtime dir %s, got %s", expected, dir)
	}
}
