package prepare

import (
	"context"
	"errors"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	engineRuntime "github.com/sqlrs/engine-local/internal/runtime"
	"github.com/sqlrs/engine-local/internal/statefs"
)

type errorStateFS struct {
	baseErr   error
	statesErr error
	stateErr  error
}

func (e *errorStateFS) Kind() string                       { return "err" }
func (e *errorStateFS) Capabilities() statefs.Capabilities { return statefs.Capabilities{} }
func (e *errorStateFS) Validate(root string) error         { return nil }
func (e *errorStateFS) BaseDir(root, imageID string) (string, error) {
	if e.baseErr != nil {
		return "", e.baseErr
	}
	return filepath.Join(root, "base"), nil
}
func (e *errorStateFS) StatesDir(root, imageID string) (string, error) {
	if e.statesErr != nil {
		return "", e.statesErr
	}
	return filepath.Join(root, "states"), nil
}
func (e *errorStateFS) StateDir(root, imageID, stateID string) (string, error) {
	if e.stateErr != nil {
		return "", e.stateErr
	}
	return filepath.Join(root, "states", stateID), nil
}
func (e *errorStateFS) JobRuntimeDir(root, jobID string) (string, error) {
	return filepath.Join(root, "jobs", jobID), nil
}
func (e *errorStateFS) EnsureBaseDir(ctx context.Context, baseDir string) error {
	return nil
}
func (e *errorStateFS) EnsureStateDir(ctx context.Context, stateDir string) error {
	return nil
}
func (e *errorStateFS) Clone(ctx context.Context, srcDir, destDir string) (statefs.CloneResult, error) {
	return statefs.CloneResult{}, nil
}
func (e *errorStateFS) Snapshot(ctx context.Context, srcDir, destDir string) error {
	return nil
}
func (e *errorStateFS) RemovePath(ctx context.Context, path string) error {
	return nil
}

type fakeContainerRuntime struct {
	fakeRuntime
	runCalls []engineRuntime.RunRequest
	runOut   string
	runErr   error
}

func (f *fakeContainerRuntime) RunContainer(ctx context.Context, req engineRuntime.RunRequest) (string, error) {
	f.runCalls = append(f.runCalls, req)
	if f.runErr != nil {
		return f.runOut, f.runErr
	}
	return f.runOut, nil
}

func TestContainerRunners(t *testing.T) {
	instance := engineRuntime.Instance{ID: "container-1"}
	if _, err := (containerPsqlRunner{}).Run(context.Background(), instance, PsqlRunRequest{}); err == nil {
		t.Fatalf("expected runtime error")
	}
	rt := &fakeRuntime{execErr: errors.New("boom")}
	if _, err := (containerPsqlRunner{runtime: rt}).Run(context.Background(), instance, PsqlRunRequest{Args: []string{"-c", "select 1"}}); err == nil {
		t.Fatalf("expected exec error")
	}

	if _, err := (containerLiquibaseRunner{}).Run(context.Background(), LiquibaseRunRequest{}); err == nil {
		t.Fatalf("expected runtime error")
	}
	if _, err := (containerLiquibaseRunner{runtime: &fakeRuntime{}}).Run(context.Background(), LiquibaseRunRequest{}); err == nil {
		t.Fatalf("expected container runner error")
	}
	cont := &fakeContainerRuntime{runOut: "ok"}
	out, err := (containerLiquibaseRunner{runtime: cont}).Run(context.Background(), LiquibaseRunRequest{ImageID: "img", Args: []string{"update"}})
	if err != nil || out != "ok" {
		t.Fatalf("expected run output, got %q err=%v", out, err)
	}
}

func TestResolveStatePathsErrors(t *testing.T) {
	if _, err := resolveStatePaths("root", "img", "state-1", nil); err == nil {
		t.Fatalf("expected statefs error")
	}
	_, err := resolveStatePaths("root", "img", "state-1", &errorStateFS{baseErr: errors.New("boom")})
	if err == nil {
		t.Fatalf("expected base dir error")
	}
	_, err = resolveStatePaths("root", "img", "state-1", &errorStateFS{statesErr: errors.New("boom")})
	if err == nil {
		t.Fatalf("expected states dir error")
	}
	_, err = resolveStatePaths("root", "img", "state-1", &errorStateFS{stateErr: errors.New("boom")})
	if err == nil {
		t.Fatalf("expected state dir error")
	}
}

func TestMergeEnvEmptyOverrides(t *testing.T) {
	base := []string{"FOO=bar"}
	out := mergeEnv(base, nil)
	if len(out) != 1 || out[0] != "FOO=bar" {
		t.Fatalf("unexpected merge result: %+v", out)
	}
}

func TestNormalizeEnvKey(t *testing.T) {
	if runtime.GOOS == "windows" {
		if normalizeEnvKey("Path") != "PATH" {
			t.Fatalf("expected uppercased key")
		}
	} else {
		if normalizeEnvKey("Path") != "Path" {
			t.Fatalf("expected unchanged key")
		}
	}
}

func TestExecFormattingHelpers(t *testing.T) {
	if formatExecArg("") != `""` {
		t.Fatalf("expected empty arg quoted")
	}
	if formatExecArg(`has space`) != `"has space"` {
		t.Fatalf("expected quoted arg")
	}
	if formatExecArg(`has"quote`) != `"has""quote"` {
		t.Fatalf("expected escaped quote")
	}
}

func TestLiquibaseWorkDirDerivation(t *testing.T) {
	if dir := deriveLiquibaseWorkDir([]string{"--changelog-file", "C:\\work\\changelog.xml"}); dir != "C:\\work" {
		t.Fatalf("unexpected workdir: %q", dir)
	}
	if dir := deriveLiquibaseWorkDir([]string{"--defaults-file=C:\\work\\lb.properties"}); dir != "C:\\work" {
		t.Fatalf("unexpected defaults workdir: %q", dir)
	}
	if dir := deriveLiquibaseWorkDir([]string{"--changelog-file", "/tmp/changelog.xml"}); dir != "" {
		t.Fatalf("expected empty workdir, got %q", dir)
	}
}

func TestWindowsPathDir(t *testing.T) {
	if windowsPathDir("  ") != "" {
		t.Fatalf("expected empty for blank path")
	}
	if windowsPathDir("C:\\") != "" {
		t.Fatalf("expected empty for root path")
	}
	if out := windowsPathDir("C:/work/file.xml"); out != "C:/work" {
		t.Fatalf("unexpected dir: %q", out)
	}
}

func TestPGVersionHelpers(t *testing.T) {
	dir := t.TempDir()
	if ok, err := hasPGVersion(dir); err != nil || ok {
		t.Fatalf("expected missing PG_VERSION, got ok=%v err=%v", ok, err)
	}
	if err := os.WriteFile(filepath.Join(dir, "PG_VERSION"), []byte("17"), 0o600); err != nil {
		t.Fatalf("write PG_VERSION: %v", err)
	}
	if ok, err := hasPGVersion(dir); err != nil || !ok {
		t.Fatalf("expected PG_VERSION true, got ok=%v err=%v", ok, err)
	}
	if _, err := hasPGVersion("bad\x00path"); err == nil {
		t.Fatalf("expected error for invalid path")
	}

	if paths := pgVersionPaths(" "); paths != nil {
		t.Fatalf("expected nil paths for empty base")
	}

	origRoot := postgresDataDirRoot
	origDir := postgresDataDir
	postgresDataDirRoot = "/tmp/data"
	postgresDataDir = "/tmp/data"
	if out := pgDataHostDir("/base"); out != "/base" {
		t.Fatalf("expected base dir, got %q", out)
	}
	postgresDataDirRoot = origRoot
	postgresDataDir = origDir
}

func TestPostmasterPIDPath(t *testing.T) {
	if postmasterPIDPath(" ") != "" {
		t.Fatalf("expected empty path for blank stateDir")
	}
	dir := t.TempDir()
	pgDir := pgDataHostDir(dir)
	if err := os.MkdirAll(pgDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	target := filepath.Join(pgDir, "postmaster.pid")
	if err := os.WriteFile(target, []byte("pid"), 0o600); err != nil {
		t.Fatalf("write postmaster.pid: %v", err)
	}
	if out := postmasterPIDPath(dir); out != target {
		t.Fatalf("expected postmaster path %q, got %q", target, out)
	}
}

func TestResetBaseDirContents(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	if err := resetBaseDirContents(missing); err != nil {
		t.Fatalf("resetBaseDirContents missing: %v", err)
	}
	dir := t.TempDir()
	lockPath := filepath.Join(dir, baseInitLockName)
	if err := os.WriteFile(lockPath, []byte("lock"), 0o600); err != nil {
		t.Fatalf("write lock: %v", err)
	}
	other := filepath.Join(dir, "other")
	if err := os.WriteFile(other, []byte("x"), 0o600); err != nil {
		t.Fatalf("write other: %v", err)
	}
	if err := resetBaseDirContents(dir); err != nil {
		t.Fatalf("resetBaseDirContents: %v", err)
	}
	if _, err := os.Stat(other); !os.IsNotExist(err) {
		t.Fatalf("expected other removed")
	}
}

func TestInitLockBranches(t *testing.T) {
	if err := withInitLock(context.Background(), t.TempDir(), nil); err == nil {
		t.Fatalf("expected error for nil callback")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := withInitLock(ctx, t.TempDir(), func() error { return nil }); err == nil {
		t.Fatalf("expected context error")
	}

	base := t.TempDir()
	if err := os.WriteFile(filepath.Join(base, baseInitMarkerName), []byte("ok"), 0o600); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	lockPath := filepath.Join(base, baseInitLockName)
	if err := os.WriteFile(lockPath, []byte("lock"), 0o600); err != nil {
		t.Fatalf("write lock: %v", err)
	}
	if err := withInitLock(context.Background(), base, func() error { return errors.New("should not run") }); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if _, err := os.Stat(lockPath); !os.IsNotExist(err) {
		t.Fatalf("expected lock removed")
	}
}

func TestStateBuildMarkerAndLock(t *testing.T) {
	filePath := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(filePath, []byte("x"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := writeStateBuildMarker(filePath, ""); err == nil {
		t.Fatalf("expected marker error")
	}
	if snapshotKind(nil) != "" {
		t.Fatalf("expected empty snapshot kind")
	}

	stateDir := t.TempDir()
	lockPath := filepath.Join(stateDir, stateBuildLockName)
	if err := os.WriteFile(lockPath, []byte("lock"), 0o600); err != nil {
		t.Fatalf("write lock: %v", err)
	}
	if err := writeStateBuildMarker(stateDir, ""); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	if err := withStateBuildLock(context.Background(), stateDir, lockPath, "", func() error { return nil }); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestStateDirHelpers(t *testing.T) {
	if err := ensureBaseDir(context.Background(), nil, ""); err == nil {
		t.Fatalf("expected statefs error")
	}
	if err := resetStateDir(context.Background(), nil, ""); err == nil {
		t.Fatalf("expected statefs error")
	}
	fs := &fakeStateFS{removeErr: errors.New("boom")}
	if err := resetStateDir(context.Background(), fs, t.TempDir()); err == nil {
		t.Fatalf("expected remove error")
	}
}

func TestCleanupRuntimeStopError(t *testing.T) {
	runtime := &fakeRuntime{stopErr: errors.New("boom")}
	mgr := newManagerWithDeps(t, &fakeStore{}, newQueueStore(t), &testDeps{runtime: runtime})
	runner := &jobRunner{}
	called := 0
	runner.setRuntime(&jobRuntime{
		instance: engineRuntime.Instance{ID: "container-1"},
		cleanup: func() error {
			called++
			return nil
		},
	})
	mgr.cleanupRuntime(context.Background(), runner)
	if len(runtime.stopCalls) != 2 {
		t.Fatalf("expected stop retry, got %d calls", len(runtime.stopCalls))
	}
	if called != 1 {
		t.Fatalf("expected cleanup called once, got %d", called)
	}
}

func TestParentStateID(t *testing.T) {
	if parentStateID(nil) != nil {
		t.Fatalf("expected nil for nil input")
	}
	if parentStateID(&TaskInput{Kind: "image", ID: "x"}) != nil {
		t.Fatalf("expected nil for non-state input")
	}
	if parentStateID(&TaskInput{Kind: "state", ID: " "}) != nil {
		t.Fatalf("expected nil for blank state id")
	}
	if out := parentStateID(&TaskInput{Kind: "state", ID: "state-1"}); out == nil || *out != "state-1" {
		t.Fatalf("unexpected parent state id: %v", out)
	}
}

func TestExecutePrepareStepUnsupported(t *testing.T) {
	mgr := newManagerWithDeps(t, &fakeStore{}, newQueueStore(t), &testDeps{})
	prepared := preparedRequest{request: Request{PrepareKind: "unknown"}}
	if errResp := mgr.executePrepareStep(context.Background(), "job-1", prepared, nil, taskState{}); errResp == nil {
		t.Fatalf("expected unsupported prepare kind error")
	}
}

func TestExecutePsqlStepErrors(t *testing.T) {
	mgr := newManagerWithDeps(t, &fakeStore{}, newQueueStore(t), &testDeps{psql: &fakePsqlRunner{}})
	prepared := preparedRequest{normalizedArgs: []string{"-f"}}
	rt := &jobRuntime{scriptMount: &scriptMount{HostRoot: t.TempDir(), ContainerRoot: containerScriptsRoot}}
	if errResp := mgr.executePsqlStep(context.Background(), "job-1", prepared, rt, taskState{}); errResp == nil {
		t.Fatalf("expected args error")
	}
	mgr.psql = nil
	prepared = preparedRequest{normalizedArgs: []string{"-c", "select 1"}}
	rt = &jobRuntime{}
	if errResp := mgr.executePsqlStep(context.Background(), "job-1", prepared, rt, taskState{}); errResp == nil {
		t.Fatalf("expected missing psql runner error")
	}
	mgr.psql = &fakePsqlRunner{output: " ", err: errors.New("boom")}
	if errResp := mgr.executePsqlStep(context.Background(), "job-1", prepared, rt, taskState{}); errResp == nil {
		t.Fatalf("expected execution error")
	}
	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()
	mgr.psql = &fakePsqlRunner{}
	if errResp := mgr.executePsqlStep(cancelCtx, "job-1", prepared, rt, taskState{}); errResp == nil {
		t.Fatalf("expected cancelled error")
	}
}

func TestExecuteLiquibaseStepMapArgsError(t *testing.T) {
	t.Setenv("WSL_INTEROP", "1")
	mgr := newManagerWithDeps(t, &fakeStore{}, newQueueStore(t), &testDeps{liquibase: &fakeLiquibaseRunner{}})
	prepared := preparedRequest{
		request:        Request{PrepareKind: "lb", LiquibaseExecMode: "windows-bat", LiquibaseExec: `C:\Tools\liquibase.bat`},
		normalizedArgs: []string{"--changelog-file"},
	}
	rt := &jobRuntime{instance: engineRuntime.Instance{ID: "container-1", Host: "127.0.0.1", Port: 5432}}
	if errResp := mgr.executeLiquibaseStep(context.Background(), "job-1", prepared, rt, taskState{}); errResp == nil {
		t.Fatalf("expected map args error")
	}
}

func TestWithStateBuildLockErrors(t *testing.T) {
	if err := withStateBuildLock(context.Background(), t.TempDir(), "", "", func() error { return nil }); err == nil {
		t.Fatalf("expected lock path error")
	}
	if err := withStateBuildLock(context.Background(), t.TempDir(), filepath.Join(t.TempDir(), "lock"), "", nil); err == nil {
		t.Fatalf("expected nil callback error")
	}
}

func TestEnsureBaseDirResetStateDirSuccess(t *testing.T) {
	fs := &fakeStateFS{}
	base := filepath.Join(t.TempDir(), "base")
	if err := ensureBaseDir(context.Background(), fs, base); err != nil {
		t.Fatalf("ensureBaseDir: %v", err)
	}
	stateDir := filepath.Join(t.TempDir(), "state")
	if err := resetStateDir(context.Background(), fs, stateDir); err != nil {
		t.Fatalf("resetStateDir: %v", err)
	}
}

func TestEnsureBaseStateEmptyBaseDir(t *testing.T) {
	mgr := newManagerWithStateFS(t, &fakeStore{}, &fakeStateFS{})
	if err := mgr.ensureBaseState(context.Background(), "image-1", ""); err == nil {
		t.Fatalf("expected base dir error")
	}
}

func TestExecutePsqlStepOutputDetails(t *testing.T) {
	psql := &fakePsqlRunner{output: "details", err: errors.New("boom")}
	mgr := newManagerWithDeps(t, &fakeStore{}, newQueueStore(t), &testDeps{psql: psql})
	prepared := preparedRequest{normalizedArgs: []string{"-c", "select 1"}}
	rt := &jobRuntime{}
	errResp := mgr.executePsqlStep(context.Background(), "job-1", prepared, rt, taskState{})
	if errResp == nil || !strings.Contains(errResp.Details, "boom") || !strings.Contains(errResp.Details, "details") {
		t.Fatalf("expected the runner error and command output in details, got %+v", errResp)
	}
}

func TestWithStateBuildLockContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := withStateBuildLock(ctx, t.TempDir(), filepath.Join(t.TempDir(), "lock"), "", func() error { return nil })
	if err == nil {
		t.Fatalf("expected context error")
	}
}

func TestExecutePsqlStepContextCancelAfterRun(t *testing.T) {
	psql := &fakePsqlRunner{}
	mgr := newManagerWithDeps(t, &fakeStore{}, newQueueStore(t), &testDeps{psql: psql})
	prepared := preparedRequest{normalizedArgs: []string{"-c", "select 1"}}
	rt := &jobRuntime{}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if errResp := mgr.executePsqlStep(ctx, "job-1", prepared, rt, taskState{}); errResp == nil {
		t.Fatalf("expected cancelled error")
	}
}

func TestWriteStateBuildMarkerSuccess(t *testing.T) {
	dir := t.TempDir()
	if err := writeStateBuildMarker(dir, ""); err != nil {
		t.Fatalf("writeStateBuildMarker: %v", err)
	}
}

func TestExecutePsqlStepOutputFallback(t *testing.T) {
	psql := &fakePsqlRunner{output: "", err: errors.New("boom")}
	mgr := newManagerWithDeps(t, &fakeStore{}, newQueueStore(t), &testDeps{psql: psql})
	prepared := preparedRequest{normalizedArgs: []string{"-c", "select 1"}}
	rt := &jobRuntime{}
	errResp := mgr.executePsqlStep(context.Background(), "job-1", prepared, rt, taskState{})
	if errResp == nil || !strings.Contains(errResp.Details, "boom") {
		t.Fatalf("expected fallback details, got %+v", errResp)
	}
}

func TestExecuteLiquibaseStepOutputFallback(t *testing.T) {
	liquibase := &fakeLiquibaseRunner{output: "", err: errors.New("boom")}
	mgr := newManagerWithDeps(t, &fakeStore{}, newQueueStore(t), &testDeps{liquibase: liquibase})
	prepared := preparedRequest{request: Request{PrepareKind: "lb"}, normalizedArgs: []string{"update"}}
	rt := &jobRuntime{instance: engineRuntime.Instance{ID: "container-1", Host: "127.0.0.1", Port: 5432}}
	errResp := mgr.executeLiquibaseStep(context.Background(), "job-1", prepared, rt, taskState{})
	if errResp == nil || !strings.Contains(errResp.Details, "boom") {
		t.Fatalf("expected fallback details, got %+v", errResp)
	}
}

func TestExecuteLiquibaseStepOutputLines(t *testing.T) {
	liquibase := &fakeLiquibaseRunner{output: "line1\nline2"}
	mgr := newManagerWithDeps(t, &fakeStore{}, newQueueStore(t), &testDeps{liquibase: liquibase})
	prepared := preparedRequest{request: Request{PrepareKind: "lb"}, normalizedArgs: []string{"update"}}
	rt := &jobRuntime{instance: engineRuntime.Instance{ID: "container-1", Host: "127.0.0.1", Port: 5432}}
	if errResp := mgr.executeLiquibaseStep(context.Background(), "job-1", prepared, rt, taskState{}); errResp != nil {
		t.Fatalf("executeLiquibaseStep: %+v", errResp)
	}
}

func TestEnsureBaseStateInitBaseError(t *testing.T) {
	rt := &fakeRuntime{initErr: errors.New("boom")}
	mgr := newManagerWithDeps(t, &fakeStore{}, newQueueStore(t), &testDeps{runtime: rt})
	base := t.TempDir()
	if err := mgr.ensureBaseState(context.Background(), "image-1", base); err == nil {
		t.Fatalf("expected init base error")
	}
}

func TestExecutePsqlStepRunsAndLogs(t *testing.T) {
	psql := &fakePsqlRunner{output: "ok"}
	mgr := newManagerWithDeps(t, &fakeStore{}, newQueueStore(t), &testDeps{psql: psql})
	prepared := preparedRequest{normalizedArgs: []string{"-c", "select 1"}}
	rt := &jobRuntime{}
	if errResp := mgr.executePsqlStep(context.Background(), "job-1", prepared, rt, taskState{}); errResp != nil {
		t.Fatalf("executePsqlStep: %+v", errResp)
	}
}

func TestExecuteLiquibaseStepMissingRunnerExtra(t *testing.T) {
	mgr := newManagerWithDeps(t, &fakeStore{}, newQueueStore(t), &testDeps{})
	mgr.liquibase = nil
	prepared := preparedRequest{request: Request{PrepareKind: "lb"}}
	rt := &jobRuntime{instance: engineRuntime.Instance{Host: "127.0.0.1", Port: 5432}}
	if errResp := mgr.executeLiquibaseStep(context.Background(), "job-1", prepared, rt, taskState{}); errResp == nil {
		t.Fatalf("expected missing runner error")
	}
}

func TestExecuteLiquibaseStepMissingConnectionInfo(t *testing.T) {
	mgr := newManagerWithDeps(t, &fakeStore{}, newQueueStore(t), &testDeps{liquibase: &fakeLiquibaseRunner{}})
	prepared := preparedRequest{request: Request{PrepareKind: "lb"}}
	rt := &jobRuntime{instance: engineRuntime.Instance{}}
	if errResp := mgr.executeLiquibaseStep(context.Background(), "job-1", prepared, rt, taskState{}); errResp == nil {
		t.Fatalf("expected missing connection info error")
	}
}

func TestExecuteLiquibaseStepCancelled(t *testing.T) {
	liquibase := &fakeLiquibaseRunner{output: "ok"}
	mgr := newManagerWithDeps(t, &fakeStore{}, newQueueStore(t), &testDeps{liquibase: liquibase})
	prepared := preparedRequest{request: Request{PrepareKind: "lb"}, normalizedArgs: []string{"update"}}
	rt := &jobRuntime{instance: engineRuntime.Instance{ID: "container-1", Host: "127.0.0.1", Port: 5432}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if errResp := mgr.executeLiquibaseStep(ctx, "job-1", prepared, rt, taskState{}); errResp == nil {
		t.Fatalf("expected cancelled error")
	}
}

func TestWithStateBuildLockUsesMarker(t *testing.T) {
	dir := t.TempDir()
	lockPath := filepath.Join(dir, stateBuildLockName)
	if err := os.WriteFile(lockPath, []byte("lock"), 0o600); err != nil {
		t.Fatalf("write lock: %v", err)
	}
	if err := writeStateBuildMarker(dir, ""); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	if err := withStateBuildLock(context.Background(), dir, lockPath, "", func() error { return nil }); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestEnsureBaseStateInitLockCancel(t *testing.T) {
	mgr := newManagerWithStateFS(t, &fakeStore{}, &fakeStateFS{})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := mgr.ensureBaseState(ctx, "image-1", filepath.Join(t.TempDir(), "base")); err == nil {
		t.Fatalf("expected cancel error")
	}
}

func TestHasPGVersionPermissionError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("permission bits are not enforced on windows")
	}
	dir := filepath.Join(t.TempDir(), "noaccess")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "PG_VERSION"), []byte("17"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Skipf("chmod not supported: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o700) })
	_, err := hasPGVersion(dir)
	if err == nil || !os.IsPermission(err) {
		t.Fatalf("expected permission error, got %v", err)
	}
}

func TestCleanupRuntimeNilRunner(t *testing.T) {
	mgr := newManagerWithDeps(t, &fakeStore{}, newQueueStore(t), &testDeps{runtime: &fakeRuntime{}})
	mgr.cleanupRuntime(context.Background(), nil)
}

func TestEnsureRuntimeExisting(t *testing.T) {
	mgr := newManagerWithDeps(t, &fakeStore{}, newQueueStore(t), &testDeps{})
	runner := &jobRunner{}
	runner.setRuntime(&jobRuntime{instance: engineRuntime.Instance{ID: "container-1"}})
	if rt, errResp := mgr.ensureRuntime(context.Background(), "job-1", preparedRequest{}, &TaskInput{}, runner); errResp != nil || rt == nil {
		t.Fatalf("expected existing runtime, err=%v", errResp)
	}
}

func TestExecuteLiquibaseStepSearchPathError(t *testing.T) {
	t.Setenv("WSL_INTEROP", "1")
	mgr := newManagerWithDeps(t, &fakeStore{}, newQueueStore(t), &testDeps{liquibase: &fakeLiquibaseRunner{}})
	prepared := preparedRequest{
		request:        Request{PrepareKind: "lb", LiquibaseExecMode: "windows-bat", LiquibaseExec: `C:\Tools\liquibase.bat`},
		normalizedArgs: []string{"update", "--searchPath", " "},
	}
	rt := &jobRuntime{instance: engineRuntime.Instance{ID: "container-1", Host: "127.0.0.1", Port: 5432}}
	if errResp := mgr.executeLiquibaseStep(context.Background(), "job-1", prepared, rt, taskState{}); errResp == nil {
		t.Fatalf("expected searchPath error")
	}
}

func TestExecutePsqlStepOutputLines(t *testing.T) {
	psql := &fakePsqlRunner{output: "line1\nline2"}
	mgr := newManagerWithDeps(t, &fakeStore{}, newQueueStore(t), &testDeps{psql: psql})
	prepared := preparedRequest{normalizedArgs: []string{"-c", "select 1"}}
	rt := &jobRuntime{}
	if errResp := mgr.executePsqlStep(context.Background(), "job-1", prepared, rt, taskState{}); errResp != nil {
		t.Fatalf("executePsqlStep: %+v", errResp)
	}
}

func TestEnsureBaseStateUsesInitMarkerExtra(t *testing.T) {
	mgr := newManagerWithStateFS(t, &fakeStore{}, &fakeStateFS{})
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, baseInitMarkerName), []byte("ok"), 0o600); err != nil {
		t.Fatalf("write marker: %v", err)
	}
	if err := mgr.ensureBaseState(context.Background(), "image-1", dir); err != nil {
		t.Fatalf("ensureBaseState: %v", err)
	}
}

func TestFormatExecLineDefaultPath(t *testing.T) {
	if out := formatExecLine("", []string{"update"}); !strings.HasPrefix(out, "liquibase ") {
		t.Fatalf("expected default exec path, got %q", out)
	}
}

func TestEnsureBaseDirNilRunnerForJob(t *testing.T) {
	mgr := newManagerWithDeps(t, &fakeStore{}, newQueueStore(t), &testDeps{})
	if _, errResp := mgr.ensureRuntime(context.Background(), "job-1", preparedRequest{}, &TaskInput{}, nil); errResp == nil {
		t.Fatalf("expected error for nil runner")
	}
}

func TestResetStateDirEnsureStateErr(t *testing.T) {
	fs := &fakeStateFS{ensureStateErr: errors.New("boom")}
	if err := resetStateDir(context.Background(), fs, t.TempDir()); err == nil {
		t.Fatalf("expected ensure state error")
	}
}

func TestWithInitLockSuccess(t *testing.T) {
	base := t.TempDir()
	called := false
	if err := withInitLock(context.Background(), base, func() error {
		called = true
		return nil
	}); err != nil {
		t.Fatalf("withInitLock: %v", err)
	}
	if !called {
		t.Fatalf("expected callback")
	}
}

func TestWithInitLockOpenFileError(t *testing.T) {
	baseFile := filepath.Join(t.TempDir(), "base-file")
	if err := os.WriteFile(baseFile, []byte("x"), 0o600); err != nil {
		t.Fatalf("write base file: %v", err)
	}
	if err := withInitLock(context.Background(), baseFile, func() error { return nil }); err == nil {
		t.Fatalf("expected open file error for invalid base dir")
	}
}

func TestWithStateBuildLockPathErrors(t *testing.T) {
	rootFile := filepath.Join(t.TempDir(), "root-file")
	if err := os.WriteFile(rootFile, []byte("x"), 0o600); err != nil {
		t.Fatalf("write root file: %v", err)
	}
	lockPathWithInvalidParent := filepath.Join(rootFile, "sub", "lock")
	if err := withStateBuildLock(context.Background(), t.TempDir(), lockPathWithInvalidParent, "", func() error { return nil }); err == nil {
		t.Fatalf("expected mkdir error when lock path parent is invalid")
	}

	lockDir := filepath.Join(t.TempDir(), "lock-dir")
	if err := os.MkdirAll(lockDir, 0o700); err != nil {
		t.Fatalf("mkdir lock dir: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := withStateBuildLock(ctx, t.TempDir(), lockDir, "", func() error { return nil })
	if err == nil {
		t.Fatalf("expected open file error when lock path points to directory")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("withStateBuildLock stuck in retry loop for directory lock path: %v", err)
	}
}

func TestWithInitLockPathErrors(t *testing.T) {
	baseDir := t.TempDir()
	lockDir := filepath.Join(baseDir, baseInitLockName)
	if err := os.MkdirAll(lockDir, 0o700); err != nil {
		t.Fatalf("mkdir lock dir: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	err := withInitLock(ctx, baseDir, func() error { return nil })
	if err == nil {
		t.Fatalf("expected open file error when init lock path points to directory")
	}
	if errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("withInitLock stuck in retry loop for directory lock path: %v", err)
	}
}

func TestShouldRetryLockAcquireOnTransientExistRace(t *testing.T) {
	lockPath := filepath.Join(t.TempDir(), baseInitLockName)
	err := &os.PathError{Op: "open", Path: lockPath, Err: os.ErrExist}
	if !shouldRetryLockAcquire(err, lockPath) {
		t.Fatalf("expected retry for transient exist error when lock file disappeared")
	}
}

func TestExecuteLiquibaseStepContextCancelAfterRun(t *testing.T) {
	liquibase := &fakeLiquibaseRunner{output: "ok"}
	mgr := newManagerWithDeps(t, &fakeStore{}, newQueueStore(t), &testDeps{liquibase: liquibase})
	prepared := preparedRequest{request: Request{PrepareKind: "lb"}, normalizedArgs: []string{"update"}}
	rt := &jobRuntime{instance: engineRuntime.Instance{ID: "container-1", Host: "127.0.0.1", Port: 5432}}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if errResp := mgr.executeLiquibaseStep(ctx, "job-1", prepared, rt, taskState{}); errResp == nil {
		t.Fatalf("expected cancelled error")
	}
}

func TestResetStateDirTimeout(t *testing.T) {
	fs := &fakeStateFS{}
	ctx, cancel := context.WithTimeout(context.Background(), time.Millisecond)
	defer cancel()
	if err := resetStateDir(ctx, fs, t.TempDir()); err != nil {
		t.Fatalf("resetStateDir: %v", err)
	}
}

func TestLiquibaseWorkDirDerivationExtraCoverage(t *testing.T) {
	if dir := deriveLiquibaseWorkDir([]string{"--changelog-file=C:\\work\\changelog.xml"}); dir != "C:\\work" {
		t.Fatalf("unexpected changelog workdir from equals form: %q", dir)
	}
	if dir := deriveLiquibaseWorkDir([]string{"--changelog-file"}); dir != "" {
		t.Fatalf("expected empty workdir for missing changelog value, got %q", dir)
	}
}

func TestResolveWSLPrimaryIPv4Coverage(t *testing.T) {
	prevList := listNetInterfaces
	prevAddrs := ifaceAddrs
	t.Cleanup(func() {
		listNetInterfaces = prevList
		ifaceAddrs = prevAddrs
	})

	t.Run("interfaces error", func(t *testing.T) {
		listNetInterfaces = func() ([]net.Interface, error) {
			return nil, errors.New("boom")
		}
		ifaceAddrs = func(iface net.Interface) ([]net.Addr, error) {
			return nil, nil
		}
		if _, err := resolveWSLPrimaryIPv4(); err == nil {
			t.Fatalf("expected interfaces error")
		}
	})

	t.Run("prefers eth0", func(t *testing.T) {
		listNetInterfaces = func() ([]net.Interface, error) {
			return []net.Interface{{Name: "eth1", Flags: net.FlagUp}, {Name: "eth0", Flags: net.FlagUp}}, nil
		}
		ifaceAddrs = func(iface net.Interface) ([]net.Addr, error) {
			switch iface.Name {
			case "eth1":
				return []net.Addr{&net.IPNet{IP: net.ParseIP("10.50.0.10"), Mask: net.CIDRMask(24, 32)}}, nil
			case "eth0":
				return []net.Addr{&net.IPNet{IP: net.ParseIP("172.20.10.2"), Mask: net.CIDRMask(24, 32)}}, nil
			default:
				return nil, nil
			}
		}
		got, err := resolveWSLPrimaryIPv4()
		if err != nil {
			t.Fatalf("resolveWSLPrimaryIPv4: %v", err)
		}
		if got != "172.20.10.2" {
			t.Fatalf("expected eth0 address, got %q", got)
		}
	})

	t.Run("fallback first global", func(t *testing.T) {
		listNetInterfaces = func() ([]net.Interface, error) {
			return []net.Interface{{Name: "wlan0", Flags: net.FlagUp}}, nil
		}
		ifaceAddrs = func(iface net.Interface) ([]net.Addr, error) {
			return []net.Addr{&net.IPNet{IP: net.ParseIP("192.168.10.4"), Mask: net.CIDRMask(24, 32)}}, nil
		}
		got, err := resolveWSLPrimaryIPv4()
		if err != nil {
			t.Fatalf("resolveWSLPrimaryIPv4: %v", err)
		}
		if got != "192.168.10.4" {
			t.Fatalf("expected first global address, got %q", got)
		}
	})

	t.Run("no global ipv4", func(t *testing.T) {
		listNetInterfaces = func() ([]net.Interface, error) {
			return []net.Interface{{Name: "lo", Flags: net.FlagUp | net.FlagLoopback}, {Name: "eth1", Flags: net.FlagUp}, {Name: "eth2", Flags: net.FlagUp}}, nil
		}
		ifaceAddrs = func(iface net.Interface) ([]net.Addr, error) {
			switch iface.Name {
			case "eth1":
				return nil, errors.New("addr error")
			case "eth2":
				return []net.Addr{&net.IPAddr{IP: net.ParseIP("10.0.0.3")}, &net.IPNet{IP: net.ParseIP("::1"), Mask: net.CIDRMask(128, 128)}, &net.IPNet{IP: net.ParseIP("0.0.0.0"), Mask: net.CIDRMask(32, 32)}}, nil
			default:
				return nil, nil
			}
		}
		if _, err := resolveWSLPrimaryIPv4(); err == nil || !strings.Contains(err.Error(), "no global ipv4") {
			t.Fatalf("expected no global ipv4 error, got %v", err)
		}
	})
}
