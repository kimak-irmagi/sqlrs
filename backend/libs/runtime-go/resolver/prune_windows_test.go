//go:build windows

package resolver

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"testing"
	"time"
)

func openWithoutDeleteSharing(t *testing.T, path string, shareMode uint32) syscall.Handle {
	t.Helper()
	name, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		t.Fatal(err)
	}
	handle, err := syscall.CreateFile(name, syscall.GENERIC_READ, shareMode, nil, syscall.OPEN_EXISTING, syscall.FILE_ATTRIBUTE_NORMAL, 0)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = syscall.CloseHandle(handle) })
	return handle
}

func createJunction(t *testing.T, link, target string) {
	t.Helper()
	if err := exec.Command("cmd", "/c", "mklink", "/J", link, target).Run(); err != nil {
		t.Skipf("junction unavailable: %v", err)
	}
}

type symlinkFileInfo struct{ os.FileInfo }

func (symlinkFileInfo) Mode() os.FileMode { return os.ModeSymlink }

func TestPruneReportsWindowsSharingFailures(t *testing.T) {
	makeEntry := func(t *testing.T) (*DirectoryCache, CacheKey) {
		t.Helper()
		cache, err := NewDirectoryCache(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		key, value := coverageKey(t, "1")
		if err := cache.Store(context.Background(), key, value); err != nil {
			t.Fatal(err)
		}
		return cache, key
	}

	t.Run("read", func(t *testing.T) {
		cache, key := makeEntry(t)
		_ = openWithoutDeleteSharing(t, cache.path(key), 0)
		if _, err := cache.Prune(context.Background(), PrunePolicy{}); err == nil {
			t.Fatal("sharing violation ignored")
		}
	})
	t.Run("aged removal", func(t *testing.T) {
		cache, key := makeEntry(t)
		old := time.Now().Add(-2 * time.Hour)
		if err := os.Chtimes(cache.path(key), old, old); err != nil {
			t.Fatal(err)
		}
		_ = openWithoutDeleteSharing(t, cache.path(key), syscall.FILE_SHARE_READ)
		if _, err := cache.Prune(context.Background(), PrunePolicy{MaximumAge: time.Hour}); err == nil {
			t.Fatal("sharing violation ignored")
		}
	})
	t.Run("count removal", func(t *testing.T) {
		cache, key := makeEntry(t)
		_ = openWithoutDeleteSharing(t, cache.path(key), syscall.FILE_SHARE_READ)
		if _, err := cache.Prune(context.Background(), PrunePolicy{MaximumEntries: 0}); err == nil {
			t.Fatal("sharing violation ignored")
		}
	})
}

func TestPrivateDirectoryRejectsWindowsJunction(t *testing.T) {
	base := t.TempDir()
	target := filepath.Join(base, "target")
	link := filepath.Join(base, "junction")
	if err := os.Mkdir(target, 0o700); err != nil {
		t.Fatal(err)
	}
	createJunction(t, link, target)
	if _, err := preparePrivateDirectory(link); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("junction error = %v", err)
	}
}

func TestWindowsLinkAndSharingBoundaries(t *testing.T) {
	if !isLinkLike(symlinkFileInfo{}) {
		t.Fatal("symbolic-link mode not detected")
	}
	t.Run("cache load sharing", func(t *testing.T) {
		cache, err := NewDirectoryCache(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		key, value := coverageKey(t, "1")
		if err := cache.Store(context.Background(), key, value); err != nil {
			t.Fatal(err)
		}
		_ = openWithoutDeleteSharing(t, cache.path(key), 0)
		loaded, err := cache.Load(context.Background(), key)
		if err == nil && !loaded.Hit {
			t.Fatal("permitted shared read did not return the cache record")
		}
	})
	t.Run("artifact sharing", func(t *testing.T) {
		store, err := NewDirectoryArtifactStore(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		digest := digestOf([]byte("content"))
		artifact, err := store.PublishVerified(context.Background(), strings.NewReader("content"), digest)
		if err != nil {
			t.Fatal(err)
		}
		artifact.Close()
		_ = openWithoutDeleteSharing(t, filepath.Join(store.root, digest[7:]), 0)
		shared, err := store.PublishVerified(context.Background(), strings.NewReader("content"), digest)
		if err == nil {
			shared.Close()
		}
	})
	t.Run("junction entries", func(t *testing.T) {
		cache, err := NewDirectoryCache(t.TempDir())
		if err != nil {
			t.Fatal(err)
		}
		target := t.TempDir()
		link := filepath.Join(cache.root, "linked.json")
		createJunction(t, link, target)
		if _, err := cache.Prune(context.Background(), PrunePolicy{}); !errors.Is(err, ErrUnsafePath) {
			t.Fatalf("prune junction=%v", err)
		}
		workspace := t.TempDir()
		nested := filepath.Join(t.TempDir(), "nested")
		if err := os.Mkdir(nested, 0o700); err != nil {
			t.Fatal(err)
		}
		workspaceLink := filepath.Join(workspace, "junction")
		createJunction(t, workspaceLink, filepath.Dir(nested))
		if _, _, err := openSafeFile(workspace, "junction/nested"); !errors.Is(err, ErrUnsafePath) {
			t.Fatalf("workspace junction=%v", err)
		}
	})
}
