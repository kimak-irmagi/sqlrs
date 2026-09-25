package snapshot

import (
	"context"
	"strings"
)

type CloneResult struct {
	MountDir string
	Cleanup  func() error
}

type Manager interface {
	Kind() string
	Capabilities() Capabilities
	Clone(ctx context.Context, srcDir string, destDir string) (CloneResult, error)
	Snapshot(ctx context.Context, srcDir string, destDir string) error
	Destroy(ctx context.Context, dir string) error
}

type Capabilities struct {
	RequiresDBStop        bool
	SupportsWritableClone bool
	SupportsSendReceive   bool
}

type Options struct {
	PreferOverlay  bool
	Backend        string
	StateStoreRoot string
}

func NewManager(opts Options) Manager {
	backend := strings.TrimSpace(opts.Backend)
	if backend == "" {
		if opts.PreferOverlay {
			backend = "overlay"
		} else {
			backend = "auto"
		}
	}
	switch backend {
	case "overlay":
		if overlaySupportedFn() {
			return newOverlayManagerFn()
		}
		return CopyManager{}
	case "btrfs":
		if btrfsSupportedFn(opts.StateStoreRoot) {
			return newBtrfsManagerFn()
		}
		return CopyManager{}
	case "zfs":
		if zfsSupportedFn(opts.StateStoreRoot) {
			return newZfsManagerFn()
		}
		return CopyManager{}
	case "copy":
		return CopyManager{}
	case "auto":
		if btrfsSupportedFn(opts.StateStoreRoot) {
			return newBtrfsManagerFn()
		}
		if zfsSupportedFn(opts.StateStoreRoot) {
			return newZfsManagerFn()
		}
		if overlaySupportedFn() {
			return newOverlayManagerFn()
		}
		return CopyManager{}
	default:
		return CopyManager{}
	}
}

var (
	overlaySupportedFn  = overlaySupported
	newOverlayManagerFn = newOverlayManager
	btrfsSupportedFn    = btrfsSupported
	newBtrfsManagerFn   = newBtrfsManager
	zfsSupportedFn      = zfsSupported
	newZfsManagerFn     = newZfsManager
)
