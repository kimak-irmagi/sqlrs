package resolver_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/resolver"
)

func TestDirectoryCacheRoundTripRestartAndCorruption(t *testing.T) {
	directory := t.TempDir()
	cache, err := resolver.NewDirectoryCache(directory)
	if err != nil {
		t.Fatal(err)
	}
	declaration := workspaceDeclaration(t)
	key, err := resolver.NewCacheKey(resolver.Workspace{Root: t.TempDir()}, descriptor("one"), resolver.NormalizedDeclaration{Declaration: declaration})
	if err != nil {
		t.Fatal(err)
	}
	want := resolution(t, declaration)
	if err := cache.Store(context.Background(), key, want); err != nil {
		t.Fatal(err)
	}
	restarted, _ := resolver.NewDirectoryCache(directory)
	got, err := restarted.Load(context.Background(), key)
	if err != nil || !got.Hit || got.Resolution.Identity.IdentitySchema() != want.Identity.IdentitySchema() {
		t.Fatalf("load = %+v, %v", got, err)
	}
	entries, _ := os.ReadDir(directory)
	if len(entries) != 1 || strings.Contains(entries[0].Name(), string(filepath.Separator)) {
		t.Fatalf("entries = %+v", entries)
	}
	path := filepath.Join(directory, entries[0].Name())
	if err := os.WriteFile(path, []byte(`{"schema_version":"sqlrs.resolution-cache.v1"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.Load(context.Background(), key); err == nil {
		t.Fatal("corrupt cache accepted")
	}
}

func TestDirectoryCacheConcurrentReplacementAndPrune(t *testing.T) {
	directory := t.TempDir()
	cache, _ := resolver.NewDirectoryCache(directory)
	workspace := resolver.Workspace{Root: t.TempDir()}
	declaration := resolver.NormalizedDeclaration{Declaration: workspaceDeclaration(t)}
	key, _ := resolver.NewCacheKey(workspace, descriptor("one"), declaration)
	value := resolution(t, declaration.Declaration)
	var wait sync.WaitGroup
	for index := 0; index < 8; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			for attempt := 0; attempt < 10; attempt++ {
				if err := cache.Store(context.Background(), key, value); err != nil {
					t.Errorf("store: %v", err)
					return
				}
				loaded, err := cache.Load(context.Background(), key)
				if err != nil || !loaded.Hit {
					t.Errorf("load: %+v %v", loaded, err)
					return
				}
			}
		}()
	}
	wait.Wait()
	result, err := cache.Prune(context.Background(), resolver.PrunePolicy{SemanticVersion: "1", MaximumEntries: 0})
	if err != nil || result.Removed != 1 {
		t.Fatalf("prune = %+v, %v", result, err)
	}
	if loaded, err := cache.Load(context.Background(), key); err != nil || loaded.Hit {
		t.Fatalf("post-prune = %+v, %v", loaded, err)
	}
}

func TestCacheKeyScopesPhysicalWorkspaceAndHidesPath(t *testing.T) {
	root := t.TempDir()
	alias := filepath.Join(root, ".")
	declaration := resolver.NormalizedDeclaration{Declaration: workspaceDeclaration(t)}
	a, _ := resolver.NewCacheKey(resolver.Workspace{Root: root}, descriptor("one"), declaration)
	b, _ := resolver.NewCacheKey(resolver.Workspace{Root: alias}, descriptor("one"), declaration)
	c, _ := resolver.NewCacheKey(resolver.Workspace{Root: t.TempDir()}, descriptor("one"), declaration)
	if a.String() != b.String() || a.String() == c.String() {
		t.Fatalf("keys = %s %s %s", a.String(), b.String(), c.String())
	}
	if strings.Contains(a.String(), root) {
		t.Fatal("key leaked workspace path")
	}
}
