package inputbundle

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// physicalInputTestRoot resolves only the platform temporary-directory alias;
// deliberately created input symlinks remain subject to production checks.
func physicalInputTestRoot(t *testing.T) string {
	t.Helper()
	root, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func TestPublishAndMaterializePreserveCapturedBytes(t *testing.T) {
	ctx := context.Background()
	root := physicalInputTestRoot(t)
	source := filepath.Join(root, "changes.xml")
	if err := os.WriteFile(source, []byte("first"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := Store{Root: filepath.Join(root, "store")}
	ref, err := store.Publish(ctx, []File{{Path: "db/changes.xml", Source: source}})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(ref.ManifestDigest, "sha256:") {
		t.Fatalf("manifest digest = %q", ref.ManifestDigest)
	}
	if err := os.WriteFile(source, []byte("second"), 0o600); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(root, "materialized")
	if err := store.Materialize(ctx, ref, dest); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(dest, "db", "changes.xml"))
	if err != nil || string(content) != "first" {
		t.Fatalf("captured content = %q, %v", content, err)
	}
	if _, err := store.Publish(ctx, []File{{Path: "db/changes.xml", Source: source}}); err != nil {
		t.Fatal(err)
	}
}

func TestPublishMixedCapturesInMemoryStdinWithFiles(t *testing.T) {
	ctx := context.Background()
	root := physicalInputTestRoot(t)
	source := filepath.Join(root, "script.sql")
	if err := os.WriteFile(source, []byte("select 1;"), 0o600); err != nil {
		t.Fatal(err)
	}
	stdin := []byte("select 2;")
	store := Store{Root: filepath.Join(root, "store")}
	ref, err := store.PublishMixed(ctx, []File{{Path: "script.sql", Source: source}}, []MemoryFile{{Path: "_sqlrs/stdin-1.sql", Data: stdin}})
	if err != nil {
		t.Fatal(err)
	}
	stdin[7] = '9'
	if err := os.WriteFile(source, []byte("select 3;"), 0o600); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(root, "materialized")
	if err := store.Materialize(ctx, ref, dest); err != nil {
		t.Fatal(err)
	}
	for path, want := range map[string]string{"script.sql": "select 1;", "_sqlrs/stdin-1.sql": "select 2;"} {
		data, err := os.ReadFile(filepath.Join(dest, filepath.FromSlash(path)))
		if err != nil || string(data) != want {
			t.Fatalf("%s=%q err=%v", path, data, err)
		}
	}
}

func TestPublishRejectsUnsafeLogicalPathsAndSymlinks(t *testing.T) {
	root := physicalInputTestRoot(t)
	source := filepath.Join(root, "changes.xml")
	if err := os.WriteFile(source, []byte("safe"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := Store{Root: filepath.Join(root, "store")}
	for _, path := range []string{"../outside", "/absolute", "C:/absolute", "a\\b", "", "a/../b", "a//b", "CON", "a/NUL.txt", "a/trailing.", "a/bad?name", "a/line\nbreak"} {
		if _, err := store.Publish(context.Background(), []File{{Path: path, Source: source}}); err == nil {
			t.Errorf("accepted unsafe logical path %q", path)
		}
	}
	link := filepath.Join(root, "link.xml")
	if err := os.Symlink(source, link); err == nil {
		if _, err := store.Publish(context.Background(), []File{{Path: "link.xml", Source: link}}); err == nil {
			t.Fatal("accepted symlink")
		}
	}
}

func TestMaterializeRejectsCorruptBlob(t *testing.T) {
	root := physicalInputTestRoot(t)
	source := filepath.Join(root, "changes.xml")
	if err := os.WriteFile(source, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := Store{Root: filepath.Join(root, "store")}
	ref, err := store.Publish(context.Background(), []File{{Path: "changes.xml", Source: source}})
	if err != nil {
		t.Fatal(err)
	}
	manifest, err := store.ReadManifest(ref)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(store.blobPath(manifest.Files[0].Digest), []byte("tampered"), 0o600); err != nil {
		t.Fatal(err)
	}
	dest := filepath.Join(root, "materialized")
	if err := store.Materialize(context.Background(), ref, dest); err == nil {
		t.Fatal("materialized corrupt blob")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("materialized directory left behind: %v", err)
	}
}

// B03/S02: Docker's unprivileged Liquibase user can traverse only the
// materialized mount, while the host-side parent remains private.
func TestMaterializedBundleIsReadableInsideUnprivilegedContainer(t *testing.T) {
	root := physicalInputTestRoot(t)
	source := filepath.Join(root, "changes.xml")
	if err := os.WriteFile(source, []byte("change"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := Store{Root: filepath.Join(root, "private-store")}
	ref, err := store.Publish(context.Background(), []File{{Path: "db/changes.xml", Source: source}})
	if err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(root, "mount")
	if err := store.Materialize(context.Background(), ref, destination); err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS == "windows" {
		return // Windows ACL-to-container mapping is verified by Docker E2E.
	}
	for _, path := range []string{destination, filepath.Join(destination, "db")} {
		info, err := os.Stat(path)
		if err != nil || info.Mode().Perm()&0o005 != 0o005 {
			t.Fatalf("container user cannot traverse %q: info=%v err=%v", path, info, err)
		}
	}
	file, err := os.Stat(filepath.Join(destination, "db", "changes.xml"))
	if err != nil || file.Mode().Perm()&0o004 == 0 || file.Mode().Perm()&0o222 != 0 {
		t.Fatalf("container file is not readable and read-only: info=%v err=%v", file, err)
	}
}

// B01/B03: admission preserves the complete, ordered directory view used by
// Liquibase includeAll, including an empty directory and captured file bytes.
func TestPublishRootsPreservesDirectoryViewAfterSourceMutation(t *testing.T) {
	root := physicalInputTestRoot(t)
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(filepath.Join(project, "nested", "empty"), 0o700); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(project, "nested", "change.xml")
	if err := os.WriteFile(source, []byte("before"), 0o600); err != nil {
		t.Fatal(err)
	}
	store := Store{Root: filepath.Join(root, "store")}
	ref, err := store.PublishRoots(context.Background(), []MountedRoot{{Path: "path1", Source: project}})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(source, []byte("after"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(project, "nested", "empty")); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(root, "materialized")
	if err := store.Materialize(context.Background(), ref, destination); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(destination, "path1", "nested", "change.xml"))
	if err != nil || string(got) != "before" {
		t.Fatalf("materialized file=%q err=%v", got, err)
	}
	info, err := os.Stat(filepath.Join(destination, "path1", "nested", "empty"))
	if err != nil || !info.IsDir() {
		t.Fatalf("empty directory was not preserved: info=%v err=%v", info, err)
	}
}

func TestPublishRootsRejectsSymlinkAndOutsideLogicalPath(t *testing.T) {
	root := physicalInputTestRoot(t)
	project := filepath.Join(root, "project")
	if err := os.MkdirAll(project, 0o700); err != nil {
		t.Fatal(err)
	}
	store := Store{Root: filepath.Join(root, "store")}
	if _, err := store.PublishRoots(context.Background(), []MountedRoot{{Path: "../escape", Source: project}}); err == nil {
		t.Fatal("unsafe logical mount was accepted")
	}
	link := filepath.Join(project, "link")
	if err := os.Symlink(root, link); err == nil {
		if _, err := store.PublishRoots(context.Background(), []MountedRoot{{Path: "path1", Source: project}}); err == nil {
			t.Fatal("symlinked input tree was accepted")
		}
	}
}
