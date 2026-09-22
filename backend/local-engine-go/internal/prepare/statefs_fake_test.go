package prepare

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/sqlrs/engine-local/internal/statefs"
)

var testLayoutFS = statefs.NewManager(statefs.Options{Backend: "copy"})

type fakeStateFS struct {
	kind           string
	caps           statefs.Capabilities
	cloneCalls     []string
	snapshotCalls  []string
	removeCalls    []string
	cloneErr       error
	snapshotErr    error
	removeErr      error
	ensureBaseErr  error
	ensureStateErr error
	validateErr    error
	mountDir       string
}

func (f *fakeStateFS) Kind() string {
	if f == nil {
		return ""
	}
	if f.kind != "" {
		return f.kind
	}
	return "fake"
}

func (f *fakeStateFS) Capabilities() statefs.Capabilities {
	if f == nil {
		return statefs.Capabilities{}
	}
	return f.caps
}

func (f *fakeStateFS) Validate(root string) error {
	if f == nil {
		return nil
	}
	return f.validateErr
}

func (f *fakeStateFS) BaseDir(root, imageID string) (string, error) {
	return testLayoutFS.BaseDir(root, imageID)
}

func (f *fakeStateFS) StatesDir(root, imageID string) (string, error) {
	return testLayoutFS.StatesDir(root, imageID)
}

func (f *fakeStateFS) StateDir(root, imageID, stateID string) (string, error) {
	return testLayoutFS.StateDir(root, imageID, stateID)
}

func (f *fakeStateFS) JobRuntimeDir(root, jobID string) (string, error) {
	return testLayoutFS.JobRuntimeDir(root, jobID)
}

func (f *fakeStateFS) EnsureBaseDir(ctx context.Context, baseDir string) error {
	if f != nil && f.ensureBaseErr != nil {
		return f.ensureBaseErr
	}
	return os.MkdirAll(baseDir, 0o700)
}

func (f *fakeStateFS) EnsureStateDir(ctx context.Context, stateDir string) error {
	if f != nil && f.ensureStateErr != nil {
		return f.ensureStateErr
	}
	return os.MkdirAll(stateDir, 0o700)
}

func (f *fakeStateFS) Clone(ctx context.Context, srcDir, destDir string) (statefs.CloneResult, error) {
	if f != nil {
		f.cloneCalls = append(f.cloneCalls, srcDir)
		if f.cloneErr != nil {
			return statefs.CloneResult{}, f.cloneErr
		}
	}
	if err := os.MkdirAll(destDir, 0o700); err != nil {
		return statefs.CloneResult{}, err
	}
	if data, err := os.ReadFile(filepath.Join(srcDir, "PG_VERSION")); err == nil {
		if writeErr := os.WriteFile(filepath.Join(destDir, "PG_VERSION"), data, 0o600); writeErr != nil {
			return statefs.CloneResult{}, writeErr
		}
	} else if !os.IsNotExist(err) {
		return statefs.CloneResult{}, err
	}
	mountDir := destDir
	if f != nil && f.mountDir != "" {
		mountDir = f.mountDir
	}
	return statefs.CloneResult{
		MountDir: mountDir,
		Cleanup:  func() error { return nil },
	}, nil
}

func (f *fakeStateFS) Snapshot(ctx context.Context, srcDir, destDir string) error {
	if f != nil {
		f.snapshotCalls = append(f.snapshotCalls, srcDir)
		if f.snapshotErr != nil {
			return f.snapshotErr
		}
	}
	return os.MkdirAll(destDir, 0o700)
}

func (f *fakeStateFS) RemovePath(ctx context.Context, path string) error {
	if f != nil {
		f.removeCalls = append(f.removeCalls, path)
		if f.removeErr != nil {
			return f.removeErr
		}
	}
	if strings.TrimSpace(path) == "" {
		return nil
	}
	return os.RemoveAll(path)
}
