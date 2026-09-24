package resolver

import (
	"bytes"
	"context"
	"io"
	"os"
	"os/exec"
	"strings"
	"testing"

	runtimev2 "github.com/kimak-irmagi/sqlrs/backend/libs/runtime-go"
)

const subprocessMode = "SQLRS_RUNTIME_V2_SUBPROCESS"

func subprocessValues(t *testing.T, workspaceRoot string) (CacheKey, Resolution) {
	t.Helper()
	declaration, err := runtimev2.NewInputDeclaration(runtimev2.ExtensionSpecificationInput{
		SchemaVersion: runtimev2.SchemaVersion, Owner: "owner", Kind: "kind",
		SpecificationSchema: "owner.kind.v1", Fields: []runtimev2.DeclarationField{},
	})
	if err != nil {
		t.Fatal(err)
	}
	descriptor := Descriptor{Role: "input", Owner: "owner", Kind: "kind", SpecificationSchema: "owner.kind.v1", SemanticVersion: "1"}
	key, err := NewCacheKey(Workspace{Root: workspaceRoot}, descriptor, NormalizedDeclaration{Declaration: declaration})
	if err != nil {
		t.Fatal(err)
	}
	identity, err := runtimev2.NewResolvedExtensionIdentity(runtimev2.ResolvedExtensionIdentityInput{SchemaVersion: runtimev2.SchemaVersion, Owner: "owner", Kind: "kind", IdentitySchema: "owner.kind.v1", Fields: []runtimev2.ResolvedField{}})
	if err != nil {
		t.Fatal(err)
	}
	return key, Resolution{Identity: identity, Evidence: []byte(`{}`)}
}

func runTestSubprocess(t *testing.T, mode string, values ...string) *exec.Cmd {
	t.Helper()
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command(executable, "-test.run=^TestRuntimeV2SubprocessHelper$")
	command.Env = append(os.Environ(), append([]string{subprocessMode + "=" + mode}, values...)...)
	return command
}

func TestRuntimeV2SubprocessHelper(t *testing.T) {
	switch os.Getenv(subprocessMode) {
	case "cache":
		cache, err := NewDirectoryCache(os.Getenv("SQLRS_CACHE_ROOT"))
		if err != nil {
			t.Fatal(err)
		}
		key, value := subprocessValues(t, os.Getenv("SQLRS_WORKSPACE_ROOT"))
		for index := 0; index < 20; index++ {
			if err := cache.Store(context.Background(), key, value); err != nil {
				t.Fatal(err)
			}
		}
	case "artifact":
		store, err := NewDirectoryArtifactStore(os.Getenv("SQLRS_ARTIFACT_ROOT"))
		if err != nil {
			t.Fatal(err)
		}
		raw := []byte("shared artifact")
		artifact, err := store.PublishVerified(context.Background(), strings.NewReader(string(raw)), digestOf(raw))
		if err != nil {
			t.Fatal(err)
		}
		artifact.Close()
	default:
		t.Skip("subprocess helper")
	}
}

func TestDirectoryCacheAndArtifactSubprocessPublication(t *testing.T) {
	cacheRoot, workspaceRoot := t.TempDir(), t.TempDir()
	artifactRoot := t.TempDir()
	type process struct {
		command *exec.Cmd
		output  bytes.Buffer
	}
	var processes []*process
	for index := 0; index < 4; index++ {
		processes = append(processes, &process{command: runTestSubprocess(t, "cache", "SQLRS_CACHE_ROOT="+cacheRoot, "SQLRS_WORKSPACE_ROOT="+workspaceRoot)})
		processes = append(processes, &process{command: runTestSubprocess(t, "artifact", "SQLRS_ARTIFACT_ROOT="+artifactRoot)})
	}
	for _, process := range processes {
		process.command.Stdout = &process.output
		process.command.Stderr = &process.output
		if err := process.command.Start(); err != nil {
			t.Fatal(err)
		}
	}
	for _, process := range processes {
		if err := process.command.Wait(); err != nil {
			t.Fatalf("subprocess: %v\n%s", err, process.output.String())
		}
	}
	cache, err := NewDirectoryCache(cacheRoot)
	if err != nil {
		t.Fatal(err)
	}
	key, _ := subprocessValues(t, workspaceRoot)
	loaded, err := cache.Load(context.Background(), key)
	if err != nil || !loaded.Hit {
		t.Fatalf("cache=%+v err=%v", loaded, err)
	}
	store, err := NewDirectoryArtifactStore(artifactRoot)
	if err != nil {
		t.Fatal(err)
	}
	raw := []byte("shared artifact")
	artifact, err := store.PublishVerified(context.Background(), strings.NewReader(string(raw)), digestOf(raw))
	if err != nil {
		t.Fatal(err)
	}
	got, err := io.ReadAll(artifact)
	artifact.Close()
	if err != nil || string(got) != string(raw) {
		t.Fatalf("artifact=%q err=%v", got, err)
	}
}
