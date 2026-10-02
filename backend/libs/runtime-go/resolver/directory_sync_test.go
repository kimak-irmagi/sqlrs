package resolver

import (
	"context"
	"errors"
	"testing"
)

// CR10: a directory-sync failure is observable even after an atomic rename;
// this tests a durability error, not a simulated power loss.
func TestDirectoryCacheReportsDirectorySyncFailure(t *testing.T) {
	cache, err := NewDirectoryCache(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	key, value := coverageKey(t, "1")
	original := syncCacheDirectory
	t.Cleanup(func() { syncCacheDirectory = original })
	syncFailure := errors.New("directory sync failed")
	calls := 0
	syncCacheDirectory = func(string) error { calls++; return syncFailure }
	if err := cache.Store(context.Background(), key, coverageSchema(), value); !errors.Is(err, syncFailure) || calls != 1 {
		t.Fatalf("sync failure = %v, calls=%d", err, calls)
	}
	loaded, err := cache.Load(context.Background(), key, coverageSchema())
	if err != nil || !loaded.Hit {
		t.Fatalf("complete renamed record not visible: %+v, %v", loaded, err)
	}
	syncCacheDirectory = func(string) error { calls++; return nil }
	if err := cache.Store(context.Background(), key, coverageSchema(), value); err != nil || calls != 2 {
		t.Fatalf("successful sync = %v, calls=%d", err, calls)
	}
}
