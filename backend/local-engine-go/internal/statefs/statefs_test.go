package statefs

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sqlrs/engine-local/internal/snapshot"
)

type fakeBackend struct {
	kind           string
	caps           snapshot.Capabilities
	ensureCalls    []string
	destroyCalls   []string
	destroyErr     error
	cloneResult    snapshot.CloneResult
	cloneErr       error
	isSubvolume    bool
	isSubvolumeErr error
}

func (f *fakeBackend) Kind() string {
	return f.kind
}

func (f *fakeBackend) Capabilities() snapshot.Capabilities {
	return f.caps
}

func (f *fakeBackend) Clone(ctx context.Context, srcDir string, destDir string) (snapshot.CloneResult, error) {
	return f.cloneResult, f.cloneErr
}

func (f *fakeBackend) Snapshot(ctx context.Context, srcDir string, destDir string) error {
	return nil
}

func (f *fakeBackend) Destroy(ctx context.Context, dir string) error {
	f.destroyCalls = append(f.destroyCalls, dir)
	return f.destroyErr
}

func (f *fakeBackend) EnsureSubvolume(ctx context.Context, path string) error {
	f.ensureCalls = append(f.ensureCalls, path)
	return nil
}

func (f *fakeBackend) IsSubvolume(ctx context.Context, path string) (bool, error) {
	return f.isSubvolume, f.isSubvolumeErr
}

type fakeZfsBackend struct {
	ensureCalls  []string
	destroyCalls []string
	destroyErr   error
	isDataset    bool
	isDatasetErr error
}

func (f *fakeZfsBackend) Kind() string                    { return "zfs" }
func (f *fakeZfsBackend) Capabilities() snapshot.Capabilities { return snapshot.Capabilities{} }
func (f *fakeZfsBackend) Clone(_ context.Context, _, _ string) (snapshot.CloneResult, error) {
	return snapshot.CloneResult{}, nil
}
func (f *fakeZfsBackend) Snapshot(_ context.Context, _, _ string) error { return nil }
func (f *fakeZfsBackend) Destroy(_ context.Context, dir string) error {
	f.destroyCalls = append(f.destroyCalls, dir)
	return f.destroyErr
}
func (f *fakeZfsBackend) EnsureDataset(_ context.Context, path string) error {
	f.ensureCalls = append(f.ensureCalls, path)
	return nil
}
func (f *fakeZfsBackend) IsDataset(_ context.Context, _ string) (bool, error) {
	return f.isDataset, f.isDatasetErr
}

type fakeBackendNoEnsure struct {
	kind string
}

func (f *fakeBackendNoEnsure) Kind() string {
	return f.kind
}

func (f *fakeBackendNoEnsure) Capabilities() snapshot.Capabilities {
	return snapshot.Capabilities{}
}

func (f *fakeBackendNoEnsure) Clone(ctx context.Context, srcDir string, destDir string) (snapshot.CloneResult, error) {
	return snapshot.CloneResult{}, nil
}

func (f *fakeBackendNoEnsure) Snapshot(ctx context.Context, srcDir string, destDir string) error {
	return nil
}

func (f *fakeBackendNoEnsure) Destroy(ctx context.Context, dir string) error {
	return nil
}

func TestParseImageIDVariants(t *testing.T) {
	engine, tag := parseImageID("")
	if engine != "unknown" || tag != "latest" {
		t.Fatalf("unexpected defaults: %s %s", engine, tag)
	}
	engine, tag = parseImageID("postgres")
	if engine != "postgres" || tag != "latest" {
		t.Fatalf("unexpected parse: %s %s", engine, tag)
	}
	engine, tag = parseImageID("repo/postgres:15")
	if engine != "postgres" || tag != "15" {
		t.Fatalf("unexpected parse: %s %s", engine, tag)
	}
	engine, tag = parseImageID("repo/postgres@sha256:abc")
	if engine != "postgres" || tag != "sha256_abc" {
		t.Fatalf("unexpected parse: %s %s", engine, tag)
	}
	engine, tag = parseImageID("repo/postgres:16@sha256:abc")
	if engine != "postgres_16" || tag != "sha256_abc" {
		t.Fatalf("unexpected parse: %s %s", engine, tag)
	}
	engine, tag = parseImageID("repo/po$stgres:1.0")
	if engine != "po_stgres" || tag != "1.0" {
		t.Fatalf("unexpected parse: %s %s", engine, tag)
	}
}

func TestDirBuildersErrors(t *testing.T) {
	if _, err := baseDir("", "image"); err == nil {
		t.Fatalf("expected baseDir error for empty root")
	}
	if _, err := statesDir("", "image"); err == nil {
		t.Fatalf("expected statesDir error for empty root")
	}
	if _, err := stateDir("", "image", "state"); err == nil {
		t.Fatalf("expected stateDir error for empty root")
	}
	if _, err := stateDir(t.TempDir(), "image", ""); err == nil {
		t.Fatalf("expected stateDir error for empty state id")
	}
	if _, err := jobRuntimeDir("", "job"); err == nil {
		t.Fatalf("expected jobRuntimeDir error for empty root")
	}
	if _, err := jobRuntimeDir(t.TempDir(), ""); err == nil {
		t.Fatalf("expected jobRuntimeDir error for empty job id")
	}
}

func TestEnsureDirsUsesSubvolumeEnsurer(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "base")
	state := filepath.Join(root, "state")
	backend := &fakeBackend{kind: "btrfs"}
	mgr := &Manager{backend: backend}
	if err := mgr.EnsureBaseDir(context.Background(), base); err != nil {
		t.Fatalf("EnsureBaseDir: %v", err)
	}
	if err := mgr.EnsureStateDir(context.Background(), state); err != nil {
		t.Fatalf("EnsureStateDir: %v", err)
	}
	if len(backend.ensureCalls) != 1 {
		t.Fatalf("expected ensure calls, got %+v", backend.ensureCalls)
	}
	if _, err := os.Stat(filepath.Dir(state)); err != nil {
		t.Fatalf("expected state parent dir created: %v", err)
	}
}

func TestEnsureDirsFallbackToMkdirAll(t *testing.T) {
	root := t.TempDir()
	base := filepath.Join(root, "base")
	state := filepath.Join(root, "state")
	mgr := &Manager{backend: &fakeBackendNoEnsure{}}
	if err := mgr.EnsureBaseDir(context.Background(), base); err != nil {
		t.Fatalf("EnsureBaseDir: %v", err)
	}
	if err := mgr.EnsureStateDir(context.Background(), state); err != nil {
		t.Fatalf("EnsureStateDir: %v", err)
	}
	if _, err := os.Stat(base); err != nil {
		t.Fatalf("expected base dir created: %v", err)
	}
	if _, err := os.Stat(state); err != nil {
		t.Fatalf("expected state dir created: %v", err)
	}
}

func TestRemovePathBtrfsSubvolume(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	backend := &fakeBackend{kind: "btrfs", isSubvolume: true}
	mgr := &Manager{backend: backend}
	if err := mgr.RemovePath(context.Background(), dir); err != nil {
		t.Fatalf("RemovePath: %v", err)
	}
	if len(backend.destroyCalls) != 1 || backend.destroyCalls[0] != dir {
		t.Fatalf("expected destroy call, got %+v", backend.destroyCalls)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("expected path removed")
	}
}

func TestRemovePathFallbackToDestroy(t *testing.T) {
	backend := &fakeBackend{kind: "copy"}
	mgr := &Manager{backend: backend}
	calls := 0
	prevRemove := removeAll
	removeAll = func(path string) error {
		calls++
		if calls == 1 {
			return os.ErrInvalid
		}
		return nil
	}
	t.Cleanup(func() { removeAll = prevRemove })

	if err := mgr.RemovePath(context.Background(), "dir"); err != nil {
		t.Fatalf("RemovePath: %v", err)
	}
	if len(backend.destroyCalls) != 1 {
		t.Fatalf("expected destroy fallback, got %+v", backend.destroyCalls)
	}
	if calls < 2 {
		t.Fatalf("expected retry removeAll, got %d calls", calls)
	}
}

func TestRemovePathIgnoresEmpty(t *testing.T) {
	backend := &fakeBackend{kind: "btrfs", isSubvolume: true}
	mgr := &Manager{backend: backend}
	if err := mgr.RemovePath(context.Background(), "   "); err != nil {
		t.Fatalf("RemovePath: %v", err)
	}
	if len(backend.destroyCalls) != 0 {
		t.Fatalf("expected no destroy calls, got %+v", backend.destroyCalls)
	}
}

func TestNewManagerKindAndJobRuntimeDir(t *testing.T) {
	root := t.TempDir()
	mgr := NewManager(Options{Backend: "copy", StateStoreRoot: root})
	if mgr.Kind() != "copy" {
		t.Fatalf("expected copy kind, got %s", mgr.Kind())
	}
	dir, err := mgr.JobRuntimeDir(root, "job-1")
	if err != nil {
		t.Fatalf("JobRuntimeDir: %v", err)
	}
	expected := filepath.Join(root, "jobs", "job-1", "runtime")
	if dir != expected {
		t.Fatalf("expected job runtime dir %s, got %s", expected, dir)
	}
}

func TestManagerCapabilitiesMapping(t *testing.T) {
	backend := &fakeBackend{
		kind: "copy",
		caps: snapshot.Capabilities{
			RequiresDBStop:        true,
			SupportsWritableClone: true,
			SupportsSendReceive:   true,
		},
	}
	mgr := &Manager{backend: backend}
	if caps := mgr.Capabilities(); caps != (Capabilities{
		RequiresDBStop:        true,
		SupportsWritableClone: true,
		SupportsSendReceive:   true,
	}) {
		t.Fatalf("unexpected capabilities: %+v", caps)
	}
}

func TestManagerBaseStatesStateDirs(t *testing.T) {
	root := t.TempDir()
	mgr := &Manager{backend: &fakeBackend{kind: "copy"}}

	base, err := mgr.BaseDir(root, "postgres:15")
	if err != nil {
		t.Fatalf("BaseDir: %v", err)
	}
	if !strings.HasSuffix(base, filepath.Join("engines", "postgres", "15", "base")) {
		t.Fatalf("unexpected base dir: %s", base)
	}

	states, err := mgr.StatesDir(root, "postgres:15")
	if err != nil {
		t.Fatalf("StatesDir: %v", err)
	}
	if !strings.HasSuffix(states, filepath.Join("engines", "postgres", "15", "states")) {
		t.Fatalf("unexpected states dir: %s", states)
	}

	state, err := mgr.StateDir(root, "postgres:15", "state-1")
	if err != nil {
		t.Fatalf("StateDir: %v", err)
	}
	if !strings.HasSuffix(state, filepath.Join("engines", "postgres", "15", "states", "state-1")) {
		t.Fatalf("unexpected state dir: %s", state)
	}
}

func TestEnsureStateDirUsesSubvolumeWhenNonBtrfs(t *testing.T) {
	root := t.TempDir()
	state := filepath.Join(root, "state")
	backend := &fakeBackend{kind: "overlay"}
	mgr := &Manager{backend: backend}

	if err := mgr.EnsureStateDir(context.Background(), state); err != nil {
		t.Fatalf("EnsureStateDir: %v", err)
	}
	if len(backend.ensureCalls) != 1 || backend.ensureCalls[0] != state {
		t.Fatalf("expected ensure subvolume call, got %+v", backend.ensureCalls)
	}
}

func TestManagerCloneResult(t *testing.T) {
	backend := &fakeBackend{
		kind:        "copy",
		cloneResult: snapshot.CloneResult{MountDir: "/mnt", Cleanup: func() error { return nil }},
	}
	mgr := &Manager{backend: backend}

	res, err := mgr.Clone(context.Background(), "src", "dest")
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}
	if res.MountDir != "/mnt" || res.Cleanup == nil {
		t.Fatalf("unexpected clone result: %+v", res)
	}
}

func TestManagerValidateAndSnapshot(t *testing.T) {
	root := t.TempDir()
	mgr := NewManager(Options{Backend: "copy", StateStoreRoot: root})
	if err := mgr.Validate(root); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	srcDir := filepath.Join(root, "src")
	destDir := filepath.Join(root, "dest")
	if err := os.MkdirAll(srcDir, 0o700); err != nil {
		t.Fatalf("mkdir src: %v", err)
	}
	if err := mgr.Snapshot(context.Background(), srcDir, destDir); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
}

func TestRemovePathBtrfsDestroyError(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	backend := &fakeBackend{kind: "btrfs", isSubvolume: true, destroyErr: os.ErrInvalid}
	mgr := &Manager{backend: backend}
	if err := mgr.RemovePath(context.Background(), dir); err == nil {
		t.Fatalf("expected destroy error")
	}
}

func TestRemovePathDestroyErrorOnFallback(t *testing.T) {
	backend := &fakeBackend{kind: "copy", destroyErr: os.ErrInvalid}
	mgr := &Manager{backend: backend}
	prevRemove := removeAll
	removeAll = func(path string) error { return os.ErrInvalid }
	t.Cleanup(func() { removeAll = prevRemove })

	if err := mgr.RemovePath(context.Background(), "dir"); err == nil {
		t.Fatalf("expected destroy error")
	}
}

func TestEnsureBaseDirUsesDatasetEnsurer(t *testing.T) {
	root := t.TempDir()
	baseDir := filepath.Join(root, "base")
	backend := &fakeZfsBackend{}
	mgr := &Manager{backend: backend}

	if err := mgr.EnsureBaseDir(context.Background(), baseDir); err != nil {
		t.Fatalf("EnsureBaseDir: %v", err)
	}
	if len(backend.ensureCalls) != 1 || backend.ensureCalls[0] != baseDir {
		t.Fatalf("expected EnsureDataset(%s), got %+v", baseDir, backend.ensureCalls)
	}
}

func TestEnsureStateDirZfsCreatesMkdirNotDataset(t *testing.T) {
	root := t.TempDir()
	stateDir := filepath.Join(root, "states", "state-1")
	backend := &fakeZfsBackend{}
	mgr := &Manager{backend: backend}

	if err := mgr.EnsureStateDir(context.Background(), stateDir); err != nil {
		t.Fatalf("EnsureStateDir: %v", err)
	}
	// ZFS states are created by "zfs clone", not EnsureDataset.
	// EnsureStateDir must only create the parent as a plain directory.
	if len(backend.ensureCalls) != 0 {
		t.Fatalf("expected no EnsureDataset calls for ZFS state dir, got %+v", backend.ensureCalls)
	}
	wantParent := filepath.Dir(stateDir)
	if _, err := os.Stat(wantParent); err != nil {
		t.Fatalf("expected parent dir %s to be created: %v", wantParent, err)
	}
}

func TestRemovePathZfsDataset(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state-1")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	backend := &fakeZfsBackend{isDataset: true}
	mgr := &Manager{backend: backend}

	if err := mgr.RemovePath(context.Background(), dir); err != nil {
		t.Fatalf("RemovePath: %v", err)
	}
	if len(backend.destroyCalls) != 1 || backend.destroyCalls[0] != dir {
		t.Fatalf("expected Destroy(%s), got %+v", dir, backend.destroyCalls)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("expected path removed after destroy")
	}
}

func TestRemovePathZfsNonDatasetFallsBackToRemoveAll(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "plain-dir")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	backend := &fakeZfsBackend{isDataset: false}
	mgr := &Manager{backend: backend}

	if err := mgr.RemovePath(context.Background(), dir); err != nil {
		t.Fatalf("RemovePath: %v", err)
	}
	if len(backend.destroyCalls) != 0 {
		t.Fatalf("expected no Destroy calls, got %+v", backend.destroyCalls)
	}
	if _, err := os.Stat(dir); !os.IsNotExist(err) {
		t.Fatalf("expected plain dir removed by removeAll")
	}
}

func TestRemovePathZfsDestroyError(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "state-1")
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	backend := &fakeZfsBackend{isDataset: true, destroyErr: os.ErrInvalid}
	mgr := &Manager{backend: backend}

	if err := mgr.RemovePath(context.Background(), dir); err == nil {
		t.Fatalf("expected destroy error")
	}
}
