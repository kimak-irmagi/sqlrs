//go:build !linux

package snapshot

import "testing"

func TestZfsSupportedAlwaysFalse(t *testing.T) {
	if zfsSupported("/data") {
		t.Fatalf("expected zfs unsupported on non-linux")
	}
}

func TestNewZfsManagerReturnsCopy(t *testing.T) {
	mgr := newZfsManager()
	if mgr.Kind() != "copy" {
		t.Fatalf("expected copy manager, got %s", mgr.Kind())
	}
}
