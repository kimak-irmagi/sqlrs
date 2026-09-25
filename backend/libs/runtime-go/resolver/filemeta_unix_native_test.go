//go:build linux || darwin

package resolver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
)

func TestNativeUnixContinuityRejectsSameSizeRestoredMtimeMutation(t *testing.T) {
	workspace := Workspace{Root: t.TempDir()}
	path := filepath.Join(workspace.Root, "input")
	if err := os.WriteFile(path, []byte("before!"), 0o600); err != nil {
		t.Fatal(err)
	}
	providerValue, err := NewWorkspaceFileResolver(nil)
	if err != nil {
		t.Fatal(err)
	}
	provider := providerValue.(*workspaceFileResolver)
	declaration := nativeFileDeclaration(t, "input")
	resolution, err := provider.Resolve(context.Background(), workspace, NormalizedDeclaration{Declaration: declaration})
	if err != nil {
		t.Fatal(err)
	}
	var evidence fileEvidence
	if err := json.Unmarshal(resolution.Evidence, &evidence); err != nil {
		t.Fatal(err)
	}
	if !evidence.Strong {
		if os.Getenv("SQLRS_REQUIRE_NATIVE_CONTINUITY") == "1" {
			t.Fatalf("CI filesystem %q has no approved cheap-revalidation revision", evidence.FilesystemClass)
		}
		t.Skipf("native filesystem %q has no approved cheap-revalidation revision", evidence.FilesystemClass)
	}
	current, err := provider.Revalidate(context.Background(), workspace, resolution)
	if err != nil || current.Status != Current {
		t.Fatalf("unchanged revalidation = %+v, %v", current, err)
	}
	mtime := time.Unix(0, evidence.ModifiedNanos)
	if err := os.WriteFile(path, []byte("after!!"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, mtime, mtime); err != nil {
		t.Fatal(err)
	}
	changed, err := provider.Revalidate(context.Background(), workspace, resolution)
	if err != nil {
		t.Fatal(err)
	}
	if changed.Status == Current {
		t.Fatalf("same-size restored-mtime mutation returned CURRENT: %+v", changed)
	}
}

func nativeFileDeclaration(t *testing.T, path string) runtimev2.InputDeclaration {
	t.Helper()
	value, err := runtimev2.NewInputDeclaration(runtimev2.ExtensionSpecificationInput{
		SchemaVersion: runtimev2.SchemaVersion, Owner: workspaceOwner, Kind: workspaceKind,
		SpecificationSchema: workspaceSpecification,
		Fields:              []runtimev2.DeclarationField{{Name: "path", Value: path}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return value
}
