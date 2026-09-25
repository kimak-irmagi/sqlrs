//go:build windows

package resolver

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
)

func windowsFileDeclaration(t *testing.T, name string) runtimev2.InputDeclaration {
	t.Helper()
	value, err := runtimev2.NewInputDeclaration(runtimev2.ExtensionSpecificationInput{SchemaVersion: runtimev2.SchemaVersion, Owner: workspaceOwner, Kind: workspaceKind, SpecificationSchema: workspaceSpecification, Fields: []runtimev2.DeclarationField{{Name: "path", Value: name}}})
	if err != nil {
		t.Fatal(err)
	}
	return value
}

func TestWindowsUSNEvidenceCurrentAndMutation(t *testing.T) {
	workspace := Workspace{Root: t.TempDir()}
	path := filepath.Join(workspace.Root, "input.sql")
	original := []byte("select 1")
	if err := os.WriteFile(path, original, 0o600); err != nil {
		t.Fatal(err)
	}
	providerValue, _ := NewWorkspaceFileResolver(nil)
	provider := providerValue.(*workspaceFileResolver)
	normalized, err := provider.Normalize(context.Background(), workspace, windowsFileDeclaration(t, "input.sql"))
	if err != nil {
		t.Fatal(err)
	}
	resolution, err := provider.Resolve(context.Background(), workspace, normalized)
	if err != nil {
		t.Fatal(err)
	}
	var evidence fileEvidence
	if err := json.Unmarshal(resolution.Evidence, &evidence); err != nil {
		t.Fatal(err)
	}
	if evidence.FilesystemClass != "ntfs-usn" || !evidence.Strong {
		t.Fatalf("native NTFS/USN evidence unavailable: %+v", evidence)
	}
	current, err := provider.Revalidate(context.Background(), workspace, resolution)
	if err != nil || current.Status != Current {
		t.Fatalf("current=%+v err=%v", current, err)
	}

	modified := []byte("select 2")
	if len(modified) != len(original) {
		t.Fatal("fixture size changed")
	}
	mtime := time.Unix(0, evidence.ModifiedNanos)
	if err := os.WriteFile(path, modified, 0o600); err != nil {
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
		opened, openErr := os.Open(path)
		if openErr != nil {
			t.Fatal(openErr)
		}
		afterInfo, _ := opened.Stat()
		afterEvidence := nativeContinuityEvidence(opened, afterInfo)
		opened.Close()
		t.Fatalf("same-size restored-mtime overwrite accepted: before=%+v after=%+v result=%+v", evidence, afterEvidence, changed)
	}
}

func TestWindowsContinuityEvidenceFallbacks(t *testing.T) {
	closed, err := os.CreateTemp(t.TempDir(), "closed-")
	if err != nil {
		t.Fatal(err)
	}
	info, err := closed.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if err := closed.Close(); err != nil {
		t.Fatal(err)
	}
	if got := nativeContinuityEvidence(closed, info); got.Class != "windows-unknown" {
		t.Fatalf("closed handle=%+v", got)
	}
	if got := windowsFilesystemName(syscall.Handle(closed.Fd())); got != "" {
		t.Fatalf("closed filesystem=%q", got)
	}
	if token, ok := windowsFileUSN(syscall.Handle(closed.Fd())); ok {
		t.Fatalf("closed USN=%q", token)
	}
	nul, err := os.Open(os.DevNull)
	if err != nil {
		t.Fatal(err)
	}
	defer nul.Close()
	nulInfo, err := nul.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if got := nativeContinuityEvidence(nul, nulInfo); got.Class != "windows-unknown" {
		t.Fatalf("device evidence=%+v", got)
	}
	reader, writer, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	defer writer.Close()
	pipeInfo, err := reader.Stat()
	if err != nil {
		t.Fatal(err)
	}
	if got := nativeContinuityEvidence(reader, pipeInfo); got.Class != "windows-unknown" {
		t.Fatalf("pipe evidence=%+v", got)
	}
	nativePath := filepath.Join(t.TempDir(), "native")
	if err := os.WriteFile(nativePath, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	native, err := os.Open(nativePath)
	if err != nil {
		t.Fatal(err)
	}
	defer native.Close()
	nativeInfo, err := native.Stat()
	if err != nil {
		t.Fatal(err)
	}
	originalRead := readWindowsFileUSN
	readWindowsFileUSN = func(syscall.Handle) (string, bool) { return "", false }
	t.Cleanup(func() { readWindowsFileUSN = originalRead })
	if got := nativeContinuityEvidence(native, nativeInfo); got.Class != "windows-unknown" {
		t.Fatalf("USN failure=%+v", got)
	}
}
