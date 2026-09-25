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

func TestPrivateDirectoryRequiresExistingOwnerControlledRoot(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	if _, err := preparePrivateDirectory(missing); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("missing root error = %v", err)
	}

	root := t.TempDir()
	if output, err := exec.Command("icacls", root, "/grant", "*S-1-1-0:(OI)(CI)M").CombinedOutput(); err != nil {
		t.Skipf("cannot prepare broad test DACL: %v (%s)", err, output)
	}
	if _, err := preparePrivateDirectory(root); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("broad write DACL error = %v", err)
	}
}

func TestWindowsACLValidationBoundaries(t *testing.T) {
	if err := validateWindowsPrivateDirectory("bad\x00path"); err == nil {
		t.Fatal("invalid UTF-16 path accepted")
	}
	if err := validateWindowsPrivateDirectory(filepath.Join(t.TempDir(), "missing")); err == nil {
		t.Fatal("missing path ACL accepted")
	}
	if sameWindowsSID(nil, nil) {
		t.Fatal("nil SIDs compare equal")
	}
	regular := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(regular, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := validateWindowsPrivateDirectory(regular); err != nil {
		t.Fatalf("regular-file ACL setup is not owner-controlled: %v", err)
	}
	if _, err := preparePrivateDirectory(regular); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("regular file accepted as private directory: %v", err)
	}
}

func TestWindowsWriteACEClassification(t *testing.T) {
	if sid, reject := windowsWriteACE(nil); sid != nil || reject {
		t.Fatalf("nil ACE classified as write-capable: %v, %v", sid, reject)
	}
	readOnly := &windowsAllowedACE{Header: windowsACEHeader{Type: accessAllowedACEType}, Mask: syscall.GENERIC_READ}
	if sid, reject := windowsWriteACE(readOnly); sid != nil || reject {
		t.Fatalf("read-only ACE classified as write-capable: %v, %v", sid, reject)
	}
	unknown := &windowsAllowedACE{Header: windowsACEHeader{Type: 0xff}, Mask: writeLikeAccessMask}
	if sid, reject := windowsWriteACE(unknown); sid != nil || reject {
		t.Fatalf("unknown ACE classified as allowed write: %v, %v", sid, reject)
	}
	object := &windowsAllowedACE{Header: windowsACEHeader{Type: objectAllowedACEType}, Mask: writeLikeAccessMask}
	if sid, reject := windowsWriteACE(object); sid != nil || !reject {
		t.Fatalf("object ACE classification = %v, %v", sid, reject)
	}
	allowed := &windowsAllowedACE{Header: windowsACEHeader{Type: accessAllowedACEType}, Mask: writeLikeAccessMask}
	if sid, reject := windowsWriteACE(allowed); sid == nil || reject {
		t.Fatalf("allowed ACE classification = %v, %v", sid, reject)
	}
	broad := broadWindowsSIDs()
	if !windowsSIDInSet(broad[0], broad) {
		t.Fatal("well-known broad SID not found")
	}
	system, err := syscall.StringToSid("S-1-5-18")
	if err != nil {
		t.Fatal(err)
	}
	if windowsSIDInSet(system, broad) {
		t.Fatal("Local System classified as a broad SID")
	}
}

func TestWindowsDirectorySecurityPolicy(t *testing.T) {
	owner, err := syscall.StringToSid("S-1-5-18")
	if err != nil {
		t.Fatal(err)
	}
	other, err := syscall.StringToSid("S-1-5-19")
	if err != nil {
		t.Fatal(err)
	}
	empty := &windowsACL{}
	reader := func(*windowsACL, uint16) (*syscall.SID, bool, error) {
		t.Fatal("reader called for empty ACL")
		return nil, false, nil
	}
	for _, test := range []struct {
		name    string
		owner   *syscall.SID
		dacl    *windowsACL
		current *syscall.SID
	}{
		{"nil owner", nil, empty, owner},
		{"nil DACL", owner, nil, owner},
		{"wrong owner", owner, empty, other},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := validateWindowsDirectorySecurity(test.owner, test.dacl, test.current, reader); !errors.Is(err, ErrUnsafePath) {
				t.Fatalf("policy error = %v", err)
			}
		})
	}
	if err := validateWindowsDirectorySecurity(owner, empty, owner, reader); err != nil {
		t.Fatalf("owner-only empty DACL: %v", err)
	}

	oneACE := &windowsACL{Count: 1}
	boom := errors.New("boom")
	if err := validateWindowsDirectorySecurity(owner, oneACE, owner, func(*windowsACL, uint16) (*syscall.SID, bool, error) {
		return nil, false, boom
	}); !errors.Is(err, boom) {
		t.Fatalf("ACE read failure = %v", err)
	}
	if err := validateWindowsDirectorySecurity(owner, oneACE, owner, func(*windowsACL, uint16) (*syscall.SID, bool, error) {
		return nil, true, nil
	}); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("object ACE policy = %v", err)
	}
	if err := validateWindowsDirectorySecurity(owner, oneACE, owner, func(*windowsACL, uint16) (*syscall.SID, bool, error) {
		return broadWindowsSIDs()[0], false, nil
	}); !errors.Is(err, ErrUnsafePath) {
		t.Fatalf("broad ACE policy = %v", err)
	}
	if err := validateWindowsDirectorySecurity(owner, oneACE, owner, func(*windowsACL, uint16) (*syscall.SID, bool, error) {
		return nil, false, nil
	}); err != nil {
		t.Fatalf("non-write ACE policy = %v", err)
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
