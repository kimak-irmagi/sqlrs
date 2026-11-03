package statefs

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/sqlrs/engine-local/internal/snapshot"
)

type Capabilities struct {
	RequiresDBStop        bool
	SupportsWritableClone bool
	SupportsSendReceive   bool
}

type CloneResult struct {
	MountDir string
	Cleanup  func() error
}

type StateFS interface {
	Kind() string
	Capabilities() Capabilities
	Validate(root string) error

	BaseDir(root, imageID string) (string, error)
	StatesDir(root, imageID string) (string, error)
	StateDir(root, imageID, stateID string) (string, error)
	JobRuntimeDir(root, jobID string) (string, error)

	EnsureBaseDir(ctx context.Context, baseDir string) error
	EnsureStateDir(ctx context.Context, stateDir string) error
	Clone(ctx context.Context, srcDir, destDir string) (CloneResult, error)
	Snapshot(ctx context.Context, srcDir, destDir string) error
	RemovePath(ctx context.Context, path string) error
}

type Options struct {
	PreferOverlay  bool
	Backend        string
	StateStoreRoot string
}

type Manager struct {
	backend snapshot.Manager
}

var removeAll = os.RemoveAll

func NewManager(opts Options) StateFS {
	return &Manager{
		backend: snapshot.NewManager(snapshot.Options{
			PreferOverlay:  opts.PreferOverlay,
			Backend:        opts.Backend,
			StateStoreRoot: opts.StateStoreRoot,
		}),
	}
}

func (m *Manager) Kind() string {
	return m.backend.Kind()
}

func (m *Manager) Capabilities() Capabilities {
	caps := m.backend.Capabilities()
	return Capabilities{
		RequiresDBStop:        caps.RequiresDBStop,
		SupportsWritableClone: caps.SupportsWritableClone,
		SupportsSendReceive:   caps.SupportsSendReceive,
	}
}

func (m *Manager) Validate(root string) error {
	return snapshot.ValidateStore(m.backend.Kind(), root)
}

func (m *Manager) BaseDir(root, imageID string) (string, error) {
	return baseDir(root, imageID)
}

func (m *Manager) StatesDir(root, imageID string) (string, error) {
	return statesDir(root, imageID)
}

func (m *Manager) StateDir(root, imageID, stateID string) (string, error) {
	return stateDir(root, imageID, stateID)
}

func (m *Manager) JobRuntimeDir(root, jobID string) (string, error) {
	return jobRuntimeDir(root, jobID)
}

func (m *Manager) EnsureBaseDir(ctx context.Context, baseDir string) error {
	if ensurer, ok := m.backend.(subvolumeEnsurer); ok {
		return ensurer.EnsureSubvolume(ctx, baseDir)
	}
	if ensurer, ok := m.backend.(datasetEnsurer); ok {
		return ensurer.EnsureDataset(ctx, baseDir)
	}
	return os.MkdirAll(baseDir, 0o700)
}

func (m *Manager) EnsureStateDir(ctx context.Context, stateDir string) error {
	if ensurer, ok := m.backend.(subvolumeEnsurer); ok {
		if m.backend.Kind() == "btrfs" {
			return os.MkdirAll(filepath.Dir(stateDir), 0o700)
		}
		return ensurer.EnsureSubvolume(ctx, stateDir)
	}
	if m.backend.Kind() == "zfs" {
		// For ZFS, Clone/Snapshot create each individual state as a child
		// dataset via "zfs clone". The statesDir parent is a plain directory,
		// exactly as for btrfs. Only the base dir (EnsureBaseDir) must be a
		// real ZFS dataset.
		return os.MkdirAll(filepath.Dir(stateDir), 0o700)
	}
	return os.MkdirAll(stateDir, 0o700)
}

func (m *Manager) Clone(ctx context.Context, srcDir, destDir string) (CloneResult, error) {
	res, err := m.backend.Clone(ctx, srcDir, destDir)
	if err != nil {
		return CloneResult{}, err
	}
	return CloneResult{MountDir: res.MountDir, Cleanup: res.Cleanup}, nil
}

func (m *Manager) Snapshot(ctx context.Context, srcDir, destDir string) error {
	return m.backend.Snapshot(ctx, srcDir, destDir)
}

func (m *Manager) RemovePath(ctx context.Context, path string) error {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil
	}
	if m.backend.Kind() == "btrfs" {
		if checker, ok := m.backend.(subvolumeChecker); ok {
			if isSub, err := checker.IsSubvolume(ctx, path); err == nil && isSub {
				if err := m.backend.Destroy(ctx, path); err != nil {
					return err
				}
				return removeAll(path)
			}
		}
	}
	if m.backend.Kind() == "zfs" {
		if checker, ok := m.backend.(datasetChecker); ok {
			if isDS, err := checker.IsDataset(ctx, path); err == nil && isDS {
				if err := m.backend.Destroy(ctx, path); err != nil {
					return err
				}
				return removeAll(path)
			}
		}
	}
	if err := removeAll(path); err != nil {
		if err := m.backend.Destroy(ctx, path); err != nil {
			return err
		}
		return removeAll(path)
	}
	return nil
}

type subvolumeEnsurer interface {
	EnsureSubvolume(ctx context.Context, path string) error
}

type subvolumeChecker interface {
	IsSubvolume(ctx context.Context, path string) (bool, error)
}

type datasetEnsurer interface {
	EnsureDataset(ctx context.Context, path string) error
}

type datasetChecker interface {
	IsDataset(ctx context.Context, path string) (bool, error)
}
