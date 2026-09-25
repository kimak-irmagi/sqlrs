//go:build linux

package snapshot

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

type btrfsFakeRunner struct {
	calls []runCall
	err   error
}

func (f *btrfsFakeRunner) Run(ctx context.Context, name string, args []string) error {
	f.calls = append(f.calls, runCall{name: name, args: append([]string{}, args...)})
	return f.err
}

type btrfsSequencedRunner struct {
	calls      []runCall
	failOnCall int
	err        error
}

func (r *btrfsSequencedRunner) Run(ctx context.Context, name string, args []string) error {
	r.calls = append(r.calls, runCall{name: name, args: append([]string{}, args...)})
	if r.failOnCall == len(r.calls) {
		return r.err
	}
	return nil
}

func TestBtrfsManagerCloneRequiresDirs(t *testing.T) {
	mgr := btrfsManager{runner: &btrfsFakeRunner{}}
	if _, err := mgr.Clone(context.Background(), "", "dest"); err == nil {
		t.Fatalf("expected error for missing src")
	}
	if _, err := mgr.Clone(context.Background(), "src", ""); err == nil {
		t.Fatalf("expected error for missing dest")
	}
}

func TestBtrfsManagerCloneCreatesSnapshot(t *testing.T) {
	runner := &btrfsFakeRunner{}
	mgr := btrfsManager{runner: runner}
	src := filepath.Join(t.TempDir(), "src")
	if err := os.MkdirAll(src, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	destParent := filepath.Join(t.TempDir(), "states")
	dest := filepath.Join(destParent, "state-1")

	res, err := mgr.Clone(context.Background(), src, dest)
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}
	if res.MountDir != dest {
		t.Fatalf("unexpected mount dir: %s", res.MountDir)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected one command call, got %+v", runner.calls)
	}
	if runner.calls[0].name != "btrfs" {
		t.Fatalf("expected btrfs command, got %+v", runner.calls[0])
	}
	if want := []string{"subvolume", "snapshot", src, dest}; !equalArgs(runner.calls[0].args, want) {
		t.Fatalf("unexpected args: %+v", runner.calls[0].args)
	}
	if _, err := os.Stat(destParent); err != nil {
		t.Fatalf("expected parent directory to exist: %v", err)
	}
	if err := res.Cleanup(); err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if len(runner.calls) != 2 || !equalArgs(runner.calls[1].args, []string{"subvolume", "delete", dest}) {
		t.Fatalf("expected delete call, got %+v", runner.calls)
	}
}

func TestBtrfsManagerCleanupReturnsDeleteError(t *testing.T) {
	runner := &btrfsSequencedRunner{failOnCall: 2, err: errors.New("boom")}
	mgr := btrfsManager{runner: runner}
	src := t.TempDir()
	dest := filepath.Join(t.TempDir(), "state-1")
	res, err := mgr.Clone(context.Background(), src, dest)
	if err != nil {
		t.Fatalf("Clone: %v", err)
	}
	if err := res.Cleanup(); err == nil {
		t.Fatalf("expected cleanup error")
	}
}

func TestBtrfsManagerCloneMkdirError(t *testing.T) {
	prevMkdir := osMkdirAllBtrfs
	osMkdirAllBtrfs = func(string, os.FileMode) error {
		return errors.New("mkdir failed")
	}
	t.Cleanup(func() { osMkdirAllBtrfs = prevMkdir })

	mgr := btrfsManager{runner: &btrfsFakeRunner{}}
	if _, err := mgr.Clone(context.Background(), "src", filepath.Join(t.TempDir(), "dest", "state")); err == nil {
		t.Fatalf("expected mkdir error")
	}
}

func TestBtrfsManagerCloneCommandError(t *testing.T) {
	mgr := btrfsManager{runner: &btrfsFakeRunner{err: errors.New("boom")}}
	if _, err := mgr.Clone(context.Background(), "src", filepath.Join(t.TempDir(), "dest")); err == nil {
		t.Fatalf("expected command error")
	}
}

func TestBtrfsManagerSnapshotRequiresDirs(t *testing.T) {
	mgr := btrfsManager{runner: &btrfsFakeRunner{}}
	if err := mgr.Snapshot(context.Background(), "", "dest"); err == nil {
		t.Fatalf("expected error for missing src")
	}
	if err := mgr.Snapshot(context.Background(), "src", ""); err == nil {
		t.Fatalf("expected error for missing dest")
	}
}

func TestBtrfsManagerSnapshotCreatesReadonlySubvolume(t *testing.T) {
	runner := &btrfsFakeRunner{}
	mgr := btrfsManager{runner: runner}
	src := filepath.Join(t.TempDir(), "src")
	if err := os.MkdirAll(src, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	dest := filepath.Join(t.TempDir(), "snap", "state-1")

	if err := mgr.Snapshot(context.Background(), src, dest); err != nil {
		t.Fatalf("Snapshot: %v", err)
	}
	if len(runner.calls) != 1 {
		t.Fatalf("expected one command call, got %+v", runner.calls)
	}
	if want := []string{"subvolume", "snapshot", "-r", src, dest}; !equalArgs(runner.calls[0].args, want) {
		t.Fatalf("unexpected args: %+v", runner.calls[0].args)
	}
}

func TestBtrfsManagerSnapshotMkdirError(t *testing.T) {
	prevMkdir := osMkdirAllBtrfs
	osMkdirAllBtrfs = func(string, os.FileMode) error {
		return errors.New("mkdir failed")
	}
	t.Cleanup(func() { osMkdirAllBtrfs = prevMkdir })

	mgr := btrfsManager{runner: &btrfsFakeRunner{}}
	if err := mgr.Snapshot(context.Background(), "src", filepath.Join(t.TempDir(), "dest", "state")); err == nil {
		t.Fatalf("expected mkdir error")
	}
}

func TestBtrfsManagerSnapshotCommandError(t *testing.T) {
	mgr := btrfsManager{runner: &btrfsFakeRunner{err: errors.New("boom")}}
	if err := mgr.Snapshot(context.Background(), "src", filepath.Join(t.TempDir(), "dest")); err == nil {
		t.Fatalf("expected command error")
	}
}

func TestBtrfsManagerSnapshotExistingDestinationBoundaries(t *testing.T) {
	previousStat := osStatBtrfs
	previousShow := runBtrfsShowFn
	t.Cleanup(func() {
		osStatBtrfs = previousStat
		runBtrfsShowFn = previousShow
	})
	osStatBtrfs = func(string) (os.FileInfo, error) { return &btrfsFakeFileInfo{}, nil }

	t.Run("subvolume", func(t *testing.T) {
		runBtrfsShowFn = func(context.Context, string) (string, error) { return "Name: state-1", nil }
		runner := &btrfsFakeRunner{}
		err := (btrfsManager{runner: runner}).Snapshot(context.Background(), "src", "dest")
		if err == nil || !strings.Contains(err.Error(), "existing subvolume") {
			t.Fatalf("expected existing subvolume error, got %v", err)
		}
		if len(runner.calls) != 0 {
			t.Fatalf("snapshot command should not run: %+v", runner.calls)
		}
	})

	t.Run("subvolume without diagnostic output", func(t *testing.T) {
		runBtrfsShowFn = func(context.Context, string) (string, error) { return "", nil }
		err := (btrfsManager{runner: &btrfsFakeRunner{}}).Snapshot(context.Background(), "src", "dest")
		if err == nil || !strings.Contains(err.Error(), "existing subvolume") {
			t.Fatalf("expected existing subvolume error, got %v", err)
		}
	})

	t.Run("ordinary path", func(t *testing.T) {
		runBtrfsShowFn = func(context.Context, string) (string, error) { return "not a subvolume", errors.New("show failed") }
		runner := &btrfsFakeRunner{}
		if err := (btrfsManager{runner: runner}).Snapshot(context.Background(), "src", "dest"); err != nil {
			t.Fatalf("Snapshot: %v", err)
		}
		if len(runner.calls) != 1 {
			t.Fatalf("expected snapshot command, got %+v", runner.calls)
		}
	})
}

func TestBtrfsManagerFilesystemAndDestroyBoundaries(t *testing.T) {
	t.Run("snapshot stat error", func(t *testing.T) {
		previous := osStatBtrfs
		osStatBtrfs = func(string) (os.FileInfo, error) { return nil, errors.New("stat failed") }
		t.Cleanup(func() { osStatBtrfs = previous })
		if err := (btrfsManager{runner: &btrfsFakeRunner{}}).Snapshot(context.Background(), "src", "dest"); err == nil || !strings.Contains(err.Error(), "stat failed") {
			t.Fatalf("expected stat error, got %v", err)
		}
	})

	t.Run("ensure stat error", func(t *testing.T) {
		previous := osStatBtrfs
		osStatBtrfs = func(string) (os.FileInfo, error) { return nil, errors.New("stat failed") }
		t.Cleanup(func() { osStatBtrfs = previous })
		if err := (btrfsManager{runner: &btrfsFakeRunner{}}).EnsureSubvolume(context.Background(), "state"); err == nil || !strings.Contains(err.Error(), "stat failed") {
			t.Fatalf("expected stat error, got %v", err)
		}
	})

	t.Run("is subvolume stat error", func(t *testing.T) {
		previous := osStatBtrfs
		osStatBtrfs = func(string) (os.FileInfo, error) { return nil, errors.New("stat failed") }
		t.Cleanup(func() { osStatBtrfs = previous })
		if ok, err := (btrfsManager{runner: &btrfsFakeRunner{}}).IsSubvolume(context.Background(), "state"); err == nil || ok {
			t.Fatalf("expected stat error, got ok=%v err=%v", ok, err)
		}
	})

	t.Run("destroy success", func(t *testing.T) {
		if err := (btrfsManager{runner: &btrfsFakeRunner{}}).Destroy(context.Background(), "state"); err != nil {
			t.Fatalf("Destroy: %v", err)
		}
	})

	t.Run("destroy list error", func(t *testing.T) {
		previous := btrfsListSubvolumesFn
		btrfsListSubvolumesFn = func(context.Context, string) (string, error) { return "", errors.New("list failed") }
		t.Cleanup(func() { btrfsListSubvolumesFn = previous })
		failure := errors.New("delete failed")
		err := (btrfsManager{runner: &btrfsFakeRunner{err: failure}}).Destroy(context.Background(), "state")
		if !errors.Is(err, failure) {
			t.Fatalf("expected delete error, got %v", err)
		}
	})
}

func TestBtrfsCommandFormattingQuotesUnsafeArguments(t *testing.T) {
	if got := quoteCommandArg(""); got != `""` {
		t.Fatalf("empty argument = %q", got)
	}
	if got := quoteCommandArg("two words"); got != `"two words"` {
		t.Fatalf("quoted argument = %q", got)
	}
	if got := formatCommand("btrfs", []string{"subvolume", "two words"}); got != `btrfs subvolume "two words"` {
		t.Fatalf("command = %q", got)
	}
}

func TestBtrfsListDiagnosticsDoNotChangeCloneOrSnapshotResults(t *testing.T) {
	previous := btrfsListSubvolumesFn
	t.Cleanup(func() { btrfsListSubvolumesFn = previous })

	for _, tc := range []struct {
		name   string
		output string
	}{
		{name: "listed subvolumes", output: "ID 12 gen 1 path nested"},
		{name: "empty list"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			btrfsListSubvolumesFn = func(context.Context, string) (string, error) { return tc.output, nil }
			cloneRunner := &btrfsFakeRunner{}
			if _, err := (btrfsManager{runner: cloneRunner}).Clone(context.Background(), "src", filepath.Join(t.TempDir(), "clone")); err != nil {
				t.Fatalf("Clone: %v", err)
			}

			snapshotRunner := &btrfsFakeRunner{}
			if err := (btrfsManager{runner: snapshotRunner}).Snapshot(context.Background(), "src", filepath.Join(t.TempDir(), "snapshot")); err != nil {
				t.Fatalf("Snapshot: %v", err)
			}
		})
	}
}

func TestBtrfsEnsureSubvolumeRequiresPath(t *testing.T) {
	if err := (btrfsManager{runner: &btrfsFakeRunner{}}).EnsureSubvolume(context.Background(), " "); err == nil {
		t.Fatal("expected missing path error")
	}
}

func TestBtrfsManagerDestroyCommandError(t *testing.T) {
	prevList := btrfsListSubvolumesFn
	btrfsListSubvolumesFn = func(context.Context, string) (string, error) {
		return "", nil
	}
	t.Cleanup(func() { btrfsListSubvolumesFn = prevList })

	mgr := btrfsManager{runner: &btrfsFakeRunner{err: errors.New("boom")}}
	if err := mgr.Destroy(context.Background(), "state"); err == nil {
		t.Fatalf("expected destroy error")
	}
}

func TestBtrfsManagerDestroyIncludesNestedListOnError(t *testing.T) {
	prevList := btrfsListSubvolumesFn
	btrfsListSubvolumesFn = func(context.Context, string) (string, error) {
		return "ID 12 gen 1 path states/root/runtime", nil
	}
	t.Cleanup(func() { btrfsListSubvolumesFn = prevList })

	mgr := btrfsManager{runner: &btrfsFakeRunner{err: errors.New("boom")}}
	err := mgr.Destroy(context.Background(), "state")
	if err == nil {
		t.Fatalf("expected destroy error")
	}
	if !strings.Contains(err.Error(), "nested subvolumes") {
		t.Fatalf("expected nested subvolumes in error, got %v", err)
	}
	if !strings.Contains(err.Error(), "states/root/runtime") {
		t.Fatalf("expected list output in error, got %v", err)
	}
}

func TestNewBtrfsManagerKindAndCapabilities(t *testing.T) {
	mgr := newBtrfsManager()
	if mgr.Kind() != "btrfs" {
		t.Fatalf("expected btrfs kind, got %s", mgr.Kind())
	}
	caps := mgr.Capabilities()
	if !caps.RequiresDBStop || !caps.SupportsWritableClone || caps.SupportsSendReceive {
		t.Fatalf("unexpected capabilities: %+v", caps)
	}
}

func TestBtrfsEnsureSubvolumeCreates(t *testing.T) {
	prevStat := osStatBtrfs
	osStatBtrfs = func(string) (os.FileInfo, error) {
		return nil, os.ErrNotExist
	}
	t.Cleanup(func() { osStatBtrfs = prevStat })

	runner := &btrfsFakeRunner{}
	mgr := btrfsManager{runner: runner}
	path := filepath.Join(t.TempDir(), "state-1")
	if err := mgr.EnsureSubvolume(context.Background(), path); err != nil {
		t.Fatalf("EnsureSubvolume: %v", err)
	}
	if len(runner.calls) != 1 || !equalArgs(runner.calls[0].args, []string{"subvolume", "create", path}) {
		t.Fatalf("unexpected calls: %+v", runner.calls)
	}
}

func TestBtrfsEnsureSubvolumeExistingSubvolume(t *testing.T) {
	prevStat := osStatBtrfs
	osStatBtrfs = func(string) (os.FileInfo, error) {
		return &btrfsFakeFileInfo{}, nil
	}
	t.Cleanup(func() { osStatBtrfs = prevStat })

	runner := &btrfsFakeRunner{}
	mgr := btrfsManager{runner: runner}
	path := filepath.Join(t.TempDir(), "state-1")
	if err := mgr.EnsureSubvolume(context.Background(), path); err != nil {
		t.Fatalf("EnsureSubvolume: %v", err)
	}
	if len(runner.calls) != 1 || !equalArgs(runner.calls[0].args, []string{"subvolume", "show", path}) {
		t.Fatalf("unexpected calls: %+v", runner.calls)
	}
}

func TestBtrfsEnsureSubvolumeRecreatesNonSubvolume(t *testing.T) {
	prevStat := osStatBtrfs
	osStatBtrfs = func(string) (os.FileInfo, error) {
		return &btrfsFakeFileInfo{}, nil
	}
	t.Cleanup(func() { osStatBtrfs = prevStat })

	prevRemove := osRemoveAllBtrfs
	removed := ""
	osRemoveAllBtrfs = func(path string) error {
		removed = path
		return nil
	}
	t.Cleanup(func() { osRemoveAllBtrfs = prevRemove })

	runner := &btrfsSequencedRunner{failOnCall: 1, err: errors.New("not subvolume")}
	mgr := btrfsManager{runner: runner}
	path := filepath.Join(t.TempDir(), "state-1")
	if err := mgr.EnsureSubvolume(context.Background(), path); err != nil {
		t.Fatalf("expected recreate, got %v", err)
	}
	if removed != path {
		t.Fatalf("expected remove of %s, got %s", path, removed)
	}
	if len(runner.calls) != 2 || !equalArgs(runner.calls[0].args, []string{"subvolume", "show", path}) ||
		!equalArgs(runner.calls[1].args, []string{"subvolume", "create", path}) {
		t.Fatalf("unexpected calls: %+v", runner.calls)
	}
}

func TestBtrfsEnsureSubvolumeMkdirError(t *testing.T) {
	prevMkdir := osMkdirAllBtrfs
	osMkdirAllBtrfs = func(string, os.FileMode) error {
		return errors.New("mkdir failed")
	}
	t.Cleanup(func() { osMkdirAllBtrfs = prevMkdir })

	mgr := btrfsManager{runner: &btrfsFakeRunner{}}
	if err := mgr.EnsureSubvolume(context.Background(), "state"); err == nil {
		t.Fatalf("expected mkdir error")
	}
}

func TestBtrfsEnsureSubvolumeCreateError(t *testing.T) {
	prevStat := osStatBtrfs
	osStatBtrfs = func(string) (os.FileInfo, error) {
		return nil, os.ErrNotExist
	}
	t.Cleanup(func() { osStatBtrfs = prevStat })

	mgr := btrfsManager{runner: &btrfsFakeRunner{err: errors.New("boom")}}
	if err := mgr.EnsureSubvolume(context.Background(), "state"); err == nil {
		t.Fatalf("expected create error")
	}
}

func TestBtrfsEnsureSubvolumeRemoveError(t *testing.T) {
	prevStat := osStatBtrfs
	osStatBtrfs = func(string) (os.FileInfo, error) {
		return &btrfsFakeFileInfo{}, nil
	}
	t.Cleanup(func() { osStatBtrfs = prevStat })

	prevRemove := osRemoveAllBtrfs
	osRemoveAllBtrfs = func(string) error {
		return errors.New("remove failed")
	}
	t.Cleanup(func() { osRemoveAllBtrfs = prevRemove })

	runner := &btrfsSequencedRunner{failOnCall: 1, err: errors.New("not subvolume")}
	mgr := btrfsManager{runner: runner}
	if err := mgr.EnsureSubvolume(context.Background(), "state"); err == nil {
		t.Fatalf("expected remove error")
	}
}

func TestBtrfsIsSubvolumeMissingPath(t *testing.T) {
	mgr := btrfsManager{runner: &btrfsFakeRunner{}}
	if _, err := mgr.IsSubvolume(context.Background(), " "); err == nil {
		t.Fatalf("expected error")
	}
}

func TestBtrfsIsSubvolumeNotExists(t *testing.T) {
	prevStat := osStatBtrfs
	osStatBtrfs = func(string) (os.FileInfo, error) {
		return nil, os.ErrNotExist
	}
	t.Cleanup(func() { osStatBtrfs = prevStat })

	mgr := btrfsManager{runner: &btrfsFakeRunner{}}
	ok, err := mgr.IsSubvolume(context.Background(), "/data/subvol")
	if err != nil || ok {
		t.Fatalf("expected missing to be false, got ok=%v err=%v", ok, err)
	}
}

func TestBtrfsIsSubvolumeShowErrorReturnsFalse(t *testing.T) {
	prevStat := osStatBtrfs
	osStatBtrfs = func(string) (os.FileInfo, error) {
		return &btrfsFakeFileInfo{}, nil
	}
	t.Cleanup(func() { osStatBtrfs = prevStat })

	mgr := btrfsManager{runner: &btrfsFakeRunner{err: errors.New("boom")}}
	ok, err := mgr.IsSubvolume(context.Background(), "/data/subvol")
	if err != nil || ok {
		t.Fatalf("expected false on show error, got ok=%v err=%v", ok, err)
	}
}

func TestBtrfsIsSubvolumeSuccess(t *testing.T) {
	prevStat := osStatBtrfs
	osStatBtrfs = func(string) (os.FileInfo, error) {
		return &btrfsFakeFileInfo{}, nil
	}
	t.Cleanup(func() { osStatBtrfs = prevStat })

	runner := &btrfsFakeRunner{}
	mgr := btrfsManager{runner: runner}
	ok, err := mgr.IsSubvolume(context.Background(), "/data/subvol")
	if err != nil || !ok {
		t.Fatalf("expected true, got ok=%v err=%v", ok, err)
	}
	if len(runner.calls) == 0 {
		t.Fatalf("expected subvolume show call")
	}
	call := runner.calls[0]
	if call.name != "btrfs" || len(call.args) < 3 || call.args[0] != "subvolume" || call.args[1] != "show" || call.args[2] != "/data/subvol" {
		t.Fatalf("expected subvolume show call, got %+v", call)
	}
}

func TestBtrfsSupportedUsesStatfs(t *testing.T) {
	prevStatfs := statfsFn
	prevLookPath := execLookPathBtrfs
	t.Cleanup(func() { statfsFn = prevStatfs })
	t.Cleanup(func() { execLookPathBtrfs = prevLookPath })

	var gotPath string
	statfsFn = func(path string, stat *syscall.Statfs_t) error {
		gotPath = path
		stat.Type = btrfsSuperMagic
		return nil
	}
	execLookPathBtrfs = func(string) (string, error) {
		return "/usr/bin/btrfs", nil
	}
	if !btrfsSupported("/data") {
		t.Fatalf("expected btrfs supported")
	}
	if gotPath != "/data" {
		t.Fatalf("expected statfs path /data, got %s", gotPath)
	}
}

func TestBtrfsSupportedRejectsMissingPath(t *testing.T) {
	if btrfsSupported("") {
		t.Fatalf("expected unsupported for empty path")
	}
}

func TestBtrfsSupportedStatfsError(t *testing.T) {
	prevStatfs := statfsFn
	prevLookPath := execLookPathBtrfs
	statfsFn = func(string, *syscall.Statfs_t) error {
		return errors.New("boom")
	}
	execLookPathBtrfs = func(string) (string, error) {
		return "/usr/bin/btrfs", nil
	}
	t.Cleanup(func() { statfsFn = prevStatfs })
	t.Cleanup(func() { execLookPathBtrfs = prevLookPath })

	if btrfsSupported("/data") {
		t.Fatalf("expected unsupported on statfs error")
	}
}

func TestBtrfsSupportedNonBtrfs(t *testing.T) {
	prevStatfs := statfsFn
	prevLookPath := execLookPathBtrfs
	statfsFn = func(path string, stat *syscall.Statfs_t) error {
		stat.Type = 0
		return nil
	}
	execLookPathBtrfs = func(string) (string, error) {
		return "/usr/bin/btrfs", nil
	}
	t.Cleanup(func() { statfsFn = prevStatfs })
	t.Cleanup(func() { execLookPathBtrfs = prevLookPath })

	if btrfsSupported("/data") {
		t.Fatalf("expected unsupported on non-btrfs fs")
	}
}

func TestBtrfsSupportedMissingBinary(t *testing.T) {
	prevLookPath := execLookPathBtrfs
	prevStatfs := statfsFn
	execLookPathBtrfs = func(string) (string, error) {
		return "", errors.New("missing")
	}
	statfsFn = func(string, *syscall.Statfs_t) error {
		t.Fatalf("statfs should not be called when btrfs is missing")
		return nil
	}
	t.Cleanup(func() { execLookPathBtrfs = prevLookPath })
	t.Cleanup(func() { statfsFn = prevStatfs })

	if btrfsSupported("/data") {
		t.Fatalf("expected unsupported without btrfs binary")
	}
}

func equalArgs(got, want []string) bool {
	if len(got) != len(want) {
		return false
	}
	for i := range want {
		if got[i] != want[i] {
			return false
		}
	}
	return true
}

type btrfsFakeFileInfo struct{}

func (btrfsFakeFileInfo) Name() string       { return "fake" }
func (btrfsFakeFileInfo) Size() int64        { return 0 }
func (btrfsFakeFileInfo) Mode() os.FileMode  { return 0 }
func (btrfsFakeFileInfo) ModTime() time.Time { return time.Time{} }
func (btrfsFakeFileInfo) IsDir() bool        { return true }
func (btrfsFakeFileInfo) Sys() any           { return nil }
