package resolver_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
	"github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go/resolver"
)

func TestWorkspaceFileResolveRevalidateAndAcquire(t *testing.T) {
	workspace := t.TempDir()
	if err := os.WriteFile(filepath.Join(workspace, "input.sql"), []byte("select 1"), 0o600); err != nil {
		t.Fatal(err)
	}
	store, err := resolver.NewDirectoryArtifactStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	provider, err := resolver.NewWorkspaceFileResolver(store)
	if err != nil {
		t.Fatal(err)
	}
	declaration := fileDeclaration(t, "input.sql")
	normalized, err := provider.Normalize(context.Background(), resolver.Workspace{Root: workspace}, declaration)
	if err != nil {
		t.Fatal(err)
	}
	got, err := provider.Resolve(context.Background(), resolver.Workspace{Root: workspace}, normalized)
	if err != nil {
		t.Fatal(err)
	}
	wantRaw := sha256.Sum256([]byte("select 1"))
	want := "sha256:" + hex.EncodeToString(wantRaw[:])
	if fields := got.Identity.Fields(); len(fields) != 1 || fields[0].Name != "content.digest" || fields[0].Value != want {
		t.Fatalf("identity = %+v", fields)
	}
	if err := provider.ValidateResolution(got); err != nil {
		t.Fatal(err)
	}
	revalidation, err := provider.Revalidate(context.Background(), resolver.Workspace{Root: workspace}, got)
	if err != nil || (revalidation.Status != resolver.Unknown && revalidation.Status != resolver.Current) {
		t.Fatalf("revalidate = %+v, %v", revalidation, err)
	}
	artifact, err := provider.Acquire(context.Background(), resolver.Workspace{Root: workspace}, got)
	if err != nil {
		t.Fatal(err)
	}
	defer artifact.Close()
	raw, err := io.ReadAll(artifact)
	if err != nil || string(raw) != "select 1" {
		t.Fatalf("artifact = %q, %v", raw, err)
	}
	if artifact.Kind() != "file" {
		t.Fatalf("kind = %q", artifact.Kind())
	}
}

func TestWorkspaceFileRejectsUnsafePathsAndLinks(t *testing.T) {
	workspace := t.TempDir()
	outside := filepath.Join(t.TempDir(), "outside.sql")
	_ = os.WriteFile(outside, []byte("secret"), 0o600)
	provider, _ := resolver.NewWorkspaceFileResolver(nil)
	unsafe := []string{"", "../outside.sql", "/absolute", `C:\absolute`, "name:stream", "NUL", "trailing. "}
	for _, path := range unsafe {
		if _, err := provider.Normalize(context.Background(), resolver.Workspace{Root: workspace}, fileDeclaration(t, path)); !errors.Is(err, resolver.ErrInvalidDeclaration) {
			t.Errorf("path %q error = %v", path, err)
		}
	}
	if err := os.Symlink(outside, filepath.Join(workspace, "link.sql")); err == nil {
		normalized, _ := provider.Normalize(context.Background(), resolver.Workspace{Root: workspace}, fileDeclaration(t, "link.sql"))
		if _, err := provider.Resolve(context.Background(), resolver.Workspace{Root: workspace}, normalized); !errors.Is(err, resolver.ErrUnsafePath) {
			t.Fatalf("symlink error = %v", err)
		}
	} else if runtime.GOOS != "windows" {
		t.Fatal(err)
	}
}

func TestWorkspaceFileDetectsChangeAndDigestMismatch(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "input.sql")
	_ = os.WriteFile(path, []byte("one"), 0o600)
	store, _ := resolver.NewDirectoryArtifactStore(t.TempDir())
	provider, _ := resolver.NewWorkspaceFileResolver(store)
	normalized, _ := provider.Normalize(context.Background(), resolver.Workspace{Root: workspace}, fileDeclaration(t, "input.sql"))
	resolution, _ := provider.Resolve(context.Background(), resolver.Workspace{Root: workspace}, normalized)
	_ = os.WriteFile(path, []byte("different-size"), 0o600)
	status, err := provider.Revalidate(context.Background(), resolver.Workspace{Root: workspace}, resolution)
	if err != nil || status.Status != resolver.Stale {
		t.Fatalf("status = %+v, %v", status, err)
	}
	if _, err := provider.Acquire(context.Background(), resolver.Workspace{Root: workspace}, resolution); !errors.Is(err, resolver.ErrChanged) {
		t.Fatalf("acquire mismatch = %v", err)
	}
}

func TestWorkspaceFileRevalidateTreatsDirectoryReplacementAsStale(t *testing.T) {
	workspace := t.TempDir()
	path := filepath.Join(workspace, "input.sql")
	if err := os.WriteFile(path, []byte("select 1"), 0o600); err != nil {
		t.Fatal(err)
	}
	provider, err := resolver.NewWorkspaceFileResolver(nil)
	if err != nil {
		t.Fatal(err)
	}
	normalized, err := provider.Normalize(context.Background(), resolver.Workspace{Root: workspace}, fileDeclaration(t, "input.sql"))
	if err != nil {
		t.Fatal(err)
	}
	resolution, err := provider.Resolve(context.Background(), resolver.Workspace{Root: workspace}, normalized)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	revalidation, err := provider.Revalidate(context.Background(), resolver.Workspace{Root: workspace}, resolution)
	if err != nil {
		t.Fatal(err)
	}
	if revalidation.Status != resolver.Stale || revalidation.Reason != "source_missing_or_replaced" {
		t.Fatalf("revalidation = %+v", revalidation)
	}
}

func fileDeclaration(t *testing.T, path string) runtimev2.InputDeclaration {
	t.Helper()
	value, err := runtimev2.NewInputDeclaration(runtimev2.ExtensionSpecificationInput{SchemaVersion: runtimev2.SchemaVersion, Owner: "sqlrs.workspace", Kind: "file", SpecificationSchema: "sqlrs.workspace-file.declaration.v1", Fields: []runtimev2.DeclarationField{{Name: "path", Value: path}}})
	if err != nil && path != "" {
		t.Fatal(err)
	}
	return value
}
