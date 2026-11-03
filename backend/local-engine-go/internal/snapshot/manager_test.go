package snapshot

import (
	"context"
	"testing"
)

func TestNewManagerPreferOverlayUsesOverlayWhenAvailable(t *testing.T) {
	prevSupported := overlaySupportedFn
	prevNew := newOverlayManagerFn
	prevBtrfsSupported := btrfsSupportedFn
	prevBtrfsNew := newBtrfsManagerFn
	defer func() {
		overlaySupportedFn = prevSupported
		newOverlayManagerFn = prevNew
		btrfsSupportedFn = prevBtrfsSupported
		newBtrfsManagerFn = prevBtrfsNew
	}()

	overlaySupportedFn = func() bool { return true }
	newOverlayManagerFn = func() Manager { return fakeManager{kind: "overlay"} }
	btrfsSupportedFn = func(string) bool { return true }
	newBtrfsManagerFn = func() Manager { return fakeManager{kind: "btrfs"} }

	mgr := NewManager(Options{PreferOverlay: true})
	if mgr.Kind() != "overlay" {
		t.Fatalf("expected overlay manager, got %s", mgr.Kind())
	}
}

func TestNewManagerAutoPrefersBtrfsThenZfsThenOverlayThenCopy(t *testing.T) {
	prevSupported := overlaySupportedFn
	prevNew := newOverlayManagerFn
	prevBtrfsSupported := btrfsSupportedFn
	prevBtrfsNew := newBtrfsManagerFn
	prevZfsSupported := zfsSupportedFn
	prevZfsNew := newZfsManagerFn
	defer func() {
		overlaySupportedFn = prevSupported
		newOverlayManagerFn = prevNew
		btrfsSupportedFn = prevBtrfsSupported
		newBtrfsManagerFn = prevBtrfsNew
		zfsSupportedFn = prevZfsSupported
		newZfsManagerFn = prevZfsNew
	}()

	overlaySupportedFn = func() bool { return true }
	newOverlayManagerFn = func() Manager { return fakeManager{kind: "overlay"} }
	zfsSupportedFn = func(string) bool { return true }
	newZfsManagerFn = func() Manager { return fakeManager{kind: "zfs"} }
	var btrfsPath string
	btrfsSupportedFn = func(path string) bool { btrfsPath = path; return true }
	newBtrfsManagerFn = func() Manager { return fakeManager{kind: "btrfs"} }

	mgr := NewManager(Options{Backend: "auto"})
	if mgr.Kind() != "btrfs" {
		t.Fatalf("expected btrfs manager, got %s", mgr.Kind())
	}
	if btrfsPath != "" {
		t.Fatalf("expected empty btrfs probe path, got %s", btrfsPath)
	}

	btrfsSupportedFn = func(path string) bool { btrfsPath = path; return false }
	zfsSupportedFn = func(string) bool { return true }
	overlaySupportedFn = func() bool { return true }
	mgr = NewManager(Options{Backend: "auto", StateStoreRoot: "root"})
	if mgr.Kind() != "zfs" {
		t.Fatalf("expected zfs manager when btrfs unavailable, got %s", mgr.Kind())
	}

	btrfsSupportedFn = func(string) bool { return false }
	zfsSupportedFn = func(string) bool { return false }
	overlaySupportedFn = func() bool { return true }
	mgr = NewManager(Options{Backend: "auto", StateStoreRoot: "root"})
	if mgr.Kind() != "overlay" {
		t.Fatalf("expected overlay manager, got %s", mgr.Kind())
	}

	btrfsSupportedFn = func(string) bool { return false }
	zfsSupportedFn = func(string) bool { return false }
	overlaySupportedFn = func() bool { return false }
	mgr = NewManager(Options{Backend: "auto", StateStoreRoot: "root"})
	if mgr.Kind() != "copy" {
		t.Fatalf("expected copy manager, got %s", mgr.Kind())
	}
}

func TestNewManagerBackendSelectionFallbacks(t *testing.T) {
	prevSupported := overlaySupportedFn
	prevNew := newOverlayManagerFn
	prevBtrfsSupported := btrfsSupportedFn
	prevBtrfsNew := newBtrfsManagerFn
	prevZfsSupported := zfsSupportedFn
	prevZfsNew := newZfsManagerFn
	defer func() {
		overlaySupportedFn = prevSupported
		newOverlayManagerFn = prevNew
		btrfsSupportedFn = prevBtrfsSupported
		newBtrfsManagerFn = prevBtrfsNew
		zfsSupportedFn = prevZfsSupported
		newZfsManagerFn = prevZfsNew
	}()

	overlaySupportedFn = func() bool { return false }
	newOverlayManagerFn = func() Manager { return fakeManager{kind: "overlay"} }
	btrfsSupportedFn = func(string) bool { return false }
	newBtrfsManagerFn = func() Manager { return fakeManager{kind: "btrfs"} }
	zfsSupportedFn = func(string) bool { return false }
	newZfsManagerFn = func() Manager { return fakeManager{kind: "zfs"} }

	mgr := NewManager(Options{Backend: "overlay"})
	if mgr.Kind() != "copy" {
		t.Fatalf("expected copy fallback, got %s", mgr.Kind())
	}

	mgr = NewManager(Options{Backend: "btrfs", StateStoreRoot: "root"})
	if mgr.Kind() != "copy" {
		t.Fatalf("expected copy fallback, got %s", mgr.Kind())
	}

	mgr = NewManager(Options{Backend: "zfs", StateStoreRoot: "root"})
	if mgr.Kind() != "copy" {
		t.Fatalf("expected copy fallback, got %s", mgr.Kind())
	}

	mgr = NewManager(Options{Backend: "copy"})
	if mgr.Kind() != "copy" {
		t.Fatalf("expected copy manager, got %s", mgr.Kind())
	}

	mgr = NewManager(Options{Backend: "unknown"})
	if mgr.Kind() != "copy" {
		t.Fatalf("expected copy fallback, got %s", mgr.Kind())
	}
}

func TestNewManagerZfsBackendUsesZfsWhenSupported(t *testing.T) {
	prevZfsSupported := zfsSupportedFn
	prevZfsNew := newZfsManagerFn
	defer func() {
		zfsSupportedFn = prevZfsSupported
		newZfsManagerFn = prevZfsNew
	}()

	zfsSupportedFn = func(string) bool { return true }
	newZfsManagerFn = func() Manager { return fakeManager{kind: "zfs"} }

	mgr := NewManager(Options{Backend: "zfs", StateStoreRoot: "root"})
	if mgr.Kind() != "zfs" {
		t.Fatalf("expected zfs manager, got %s", mgr.Kind())
	}
}

func TestNewManagerAutoIncludesZfsBeforeOverlay(t *testing.T) {
	prevOverlaySupported := overlaySupportedFn
	prevOverlayNew := newOverlayManagerFn
	prevBtrfsSupported := btrfsSupportedFn
	prevBtrfsNew := newBtrfsManagerFn
	prevZfsSupported := zfsSupportedFn
	prevZfsNew := newZfsManagerFn
	defer func() {
		overlaySupportedFn = prevOverlaySupported
		newOverlayManagerFn = prevOverlayNew
		btrfsSupportedFn = prevBtrfsSupported
		newBtrfsManagerFn = prevBtrfsNew
		zfsSupportedFn = prevZfsSupported
		newZfsManagerFn = prevZfsNew
	}()

	btrfsSupportedFn = func(string) bool { return false }
	newBtrfsManagerFn = func() Manager { return fakeManager{kind: "btrfs"} }
	overlaySupportedFn = func() bool { return true }
	newOverlayManagerFn = func() Manager { return fakeManager{kind: "overlay"} }
	zfsSupportedFn = func(string) bool { return true }
	newZfsManagerFn = func() Manager { return fakeManager{kind: "zfs"} }

	mgr := NewManager(Options{Backend: "auto", StateStoreRoot: "root"})
	if mgr.Kind() != "zfs" {
		t.Fatalf("expected zfs before overlay in auto mode, got %s", mgr.Kind())
	}
}

type fakeManager struct {
	kind string
}

func (f fakeManager) Kind() string {
	return f.kind
}

func (f fakeManager) Capabilities() Capabilities {
	return Capabilities{}
}

func (f fakeManager) Clone(ctx context.Context, srcDir string, destDir string) (CloneResult, error) {
	return CloneResult{}, nil
}

func (f fakeManager) Snapshot(ctx context.Context, srcDir string, destDir string) error {
	return nil
}

func (f fakeManager) Destroy(ctx context.Context, dir string) error {
	return nil
}
