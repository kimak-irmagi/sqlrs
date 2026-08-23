package main

import (
	"context"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/sqlrs/engine-local/internal/deletion"
	"github.com/sqlrs/engine-local/internal/httpapi"
	"github.com/sqlrs/engine-local/internal/prepare"
	"github.com/sqlrs/engine-local/internal/prepare/queue"
	runpkg "github.com/sqlrs/engine-local/internal/run"
	"github.com/sqlrs/engine-local/internal/store/sqlite"
)

func TestActivityTrackerIdleFor(t *testing.T) {
	tracker := newActivityTracker()
	tracker.last = time.Now().Add(-2 * time.Second).UnixNano()
	if tracker.IdleFor() < time.Second {
		t.Fatalf("expected idle >= 1s, got %v", tracker.IdleFor())
	}
	tracker.Touch()
	if tracker.IdleFor() < 0 {
		t.Fatalf("expected non-negative idle")
	}
}

func TestActivityTrackerInflightRequests(t *testing.T) {
	tracker := newActivityTracker()
	if tracker.HasInflightRequests() {
		t.Fatalf("expected no in-flight requests")
	}
	tracker.StartRequest()
	if !tracker.HasInflightRequests() {
		t.Fatalf("expected in-flight request")
	}
	tracker.FinishRequest()
	if tracker.HasInflightRequests() {
		t.Fatalf("expected in-flight requests to be drained")
	}
}

func TestActivityTrackerIdleStartsFromLastRequestCompletion(t *testing.T) {
	tracker := newActivityTracker()
	tracker.last = time.Now().Add(-2 * time.Second).UnixNano()

	tracker.StartRequest()
	if tracker.IdleFor() < time.Second {
		t.Fatalf("start request must not reset idle timer")
	}

	tracker.FinishRequest()
	if tracker.IdleFor() > 200*time.Millisecond {
		t.Fatalf("finish request should reset idle timer, got %v", tracker.IdleFor())
	}
}

func TestRandomHex(t *testing.T) {
	value, err := randomHex(8)
	if err != nil {
		t.Fatalf("randomHex: %v", err)
	}
	if len(value) != 16 {
		t.Fatalf("expected 16 chars, got %d", len(value))
	}
	if _, err := hex.DecodeString(value); err != nil {
		t.Fatalf("expected hex string, got %q", value)
	}
}

func TestRandomHexError(t *testing.T) {
	prevReader := randReader
	randReader = errorReader{}
	t.Cleanup(func() { randReader = prevReader })

	if _, err := randomHex(4); err == nil {
		t.Fatalf("expected error")
	}
}

func TestWriteAndRemoveEngineState(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "engine.json")
	state := EngineState{
		Endpoint:   "127.0.0.1:1234",
		PID:        42,
		StartedAt:  "2024-01-01T00:00:00Z",
		AuthToken:  "token",
		Version:    "dev",
		InstanceID: "instance",
	}
	if err := writeEngineState(path, state); err != nil {
		t.Fatalf("writeEngineState: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read state: %v", err)
	}
	var decoded EngineState
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("decode state: %v", err)
	}
	if decoded.Endpoint != state.Endpoint || decoded.AuthToken != state.AuthToken {
		t.Fatalf("unexpected state: %+v", decoded)
	}
	removeEngineState(path)
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected state file removed, got %v", err)
	}
}

func TestWriteEngineStateInvalidDir(t *testing.T) {
	dir := t.TempDir()
	blocker := filepath.Join(dir, "blocked")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	path := filepath.Join(blocker, "engine.json")
	if err := writeEngineState(path, EngineState{Endpoint: "127.0.0.1:1234"}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestWriteEngineStateRemoveError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "engine.json")
	if err := os.MkdirAll(path, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(path, "file.txt"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if err := writeEngineState(path, EngineState{Endpoint: "127.0.0.1:1234"}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestWriteEngineStateTempFileError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "engine.json")
	tmp := path + ".tmp"
	if err := os.MkdirAll(tmp, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := writeEngineState(path, EngineState{Endpoint: "127.0.0.1:1234"}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestWriteEngineStateWriteError(t *testing.T) {
	prevWrite := writeFileFn
	writeFileFn = func(string, []byte, os.FileMode) error {
		return errors.New("boom")
	}
	t.Cleanup(func() { writeFileFn = prevWrite })

	dir := t.TempDir()
	path := filepath.Join(dir, "engine.json")
	if err := writeEngineState(path, EngineState{Endpoint: "127.0.0.1:1234"}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestWriteEngineStateRenameError(t *testing.T) {
	prevRename := renameFn
	renameFn = func(string, string) error {
		return errors.New("boom")
	}
	t.Cleanup(func() { renameFn = prevRename })

	dir := t.TempDir()
	path := filepath.Join(dir, "engine.json")
	if err := writeEngineState(path, EngineState{Endpoint: "127.0.0.1:1234"}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestWriteEngineStateMarshalError(t *testing.T) {
	prevMarshal := jsonMarshalIndent
	jsonMarshalIndent = func(any, string, string) ([]byte, error) {
		return nil, errors.New("boom")
	}
	t.Cleanup(func() { jsonMarshalIndent = prevMarshal })

	if err := writeEngineState("engine.json", EngineState{Endpoint: "127.0.0.1:1234"}); err == nil {
		t.Fatalf("expected error")
	}
}

func TestOpenDBFnEmptyPath(t *testing.T) {
	if _, err := openDBFn(""); err == nil || !strings.Contains(err.Error(), "sqlite path is empty") {
		t.Fatalf("expected empty path error, got %v", err)
	}
}

func TestOpenDBFnMkdirError(t *testing.T) {
	dir := t.TempDir()
	filePath := filepath.Join(dir, "state-dir")
	if err := os.WriteFile(filePath, []byte("x"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	_, err := openDBFn(filepath.Join(filePath, "state.db"))
	if err == nil {
		t.Fatalf("expected mkdir error")
	}
}

func TestRemoveEngineStateError(t *testing.T) {
	dir := t.TempDir()
	nested := filepath.Join(dir, "nested")
	if err := os.MkdirAll(nested, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(nested, "file.txt"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	removeEngineState(nested)
	if _, err := os.Stat(nested); err != nil {
		t.Fatalf("expected directory to remain, got %v", err)
	}
}

type fakeConfigStore struct {
	value any
	err   error
}

func (f fakeConfigStore) Get(path string, effective bool) (any, error) {
	return f.value, f.err
}

func (f fakeConfigStore) Set(path string, value any) (any, error) {
	return nil, nil
}

func (f fakeConfigStore) Remove(path string) (any, error) {
	return nil, nil
}

func (f fakeConfigStore) Schema() any {
	return nil
}

func TestSnapshotBackendFromConfigDefaultsOnError(t *testing.T) {
	backend := snapshotBackendFromConfig(fakeConfigStore{err: errors.New("boom")})
	if backend != "auto" {
		t.Fatalf("expected auto fallback, got %s", backend)
	}
}

func TestSnapshotBackendFromConfigValidValues(t *testing.T) {
	cases := []string{"auto", "overlay", "btrfs", "copy"}
	for _, value := range cases {
		backend := snapshotBackendFromConfig(fakeConfigStore{value: value})
		if backend != value {
			t.Fatalf("expected %s, got %s", value, backend)
		}
	}
}

func TestSnapshotBackendFromConfigRejectsInvalidValues(t *testing.T) {
	backend := snapshotBackendFromConfig(fakeConfigStore{value: "bad"})
	if backend != "auto" {
		t.Fatalf("expected auto fallback, got %s", backend)
	}
	backend = snapshotBackendFromConfig(fakeConfigStore{value: 1})
	if backend != "auto" {
		t.Fatalf("expected auto fallback, got %s", backend)
	}
}

func TestLogLevelFromConfig(t *testing.T) {
	if logLevelFromConfig(nil) != "" {
		t.Fatalf("expected empty level for nil config")
	}
	if logLevelFromConfig(fakeConfigStore{err: errors.New("boom")}) != "" {
		t.Fatalf("expected empty level on error")
	}
	if logLevelFromConfig(fakeConfigStore{value: 123}) != "" {
		t.Fatalf("expected empty level for non-string")
	}
	if logLevelFromConfig(fakeConfigStore{value: "info"}) != "info" {
		t.Fatalf("expected info level")
	}
}

func TestContainerRuntimeFromConfig(t *testing.T) {
	if mode := containerRuntimeFromConfig(nil); mode != "auto" {
		t.Fatalf("expected auto for nil config, got %q", mode)
	}
	if mode := containerRuntimeFromConfig(fakeConfigStore{err: errors.New("boom")}); mode != "auto" {
		t.Fatalf("expected auto on config error, got %q", mode)
	}
	if mode := containerRuntimeFromConfig(fakeConfigStore{value: 1}); mode != "auto" {
		t.Fatalf("expected auto for non-string value, got %q", mode)
	}
	if mode := containerRuntimeFromConfig(fakeConfigStore{value: "docker"}); mode != "docker" {
		t.Fatalf("expected docker mode, got %q", mode)
	}
	if mode := containerRuntimeFromConfig(fakeConfigStore{value: "podman"}); mode != "podman" {
		t.Fatalf("expected podman mode, got %q", mode)
	}
	if mode := containerRuntimeFromConfig(fakeConfigStore{value: "bad"}); mode != "auto" {
		t.Fatalf("expected auto for invalid value, got %q", mode)
	}
}

func TestResolveContainerRuntimeBinaryDefaultsToDocker(t *testing.T) {
	prevLook := execLookPathFn
	prevCmd := execCommandContextFn
	execLookPathFn = func(string) (string, error) {
		return "", errors.New("missing")
	}
	execCommandContextFn = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		t.Fatalf("unexpected command call: %s %v", name, args)
		return testCommandExit(ctx, 1)
	}
	t.Cleanup(func() {
		execLookPathFn = prevLook
		execCommandContextFn = prevCmd
	})

	t.Setenv("SQLRS_CONTAINER_RUNTIME", "")
	t.Setenv("CONTAINER_HOST", "")
	if got := resolveContainerRuntimeBinary("auto"); got != "docker" {
		t.Fatalf("expected docker fallback, got %q", got)
	}
}

func TestResolveContainerRuntimeBinaryPrefersDocker(t *testing.T) {
	dockerPath := "/usr/bin/docker"
	podmanPath := "/usr/bin/podman"
	if runtime.GOOS == "windows" {
		dockerPath = `C:\Program Files\Docker\docker.exe`
		podmanPath = `C:\Program Files\RedHat\Podman\podman.exe`
	}

	prevLook := execLookPathFn
	prevCmd := execCommandContextFn
	execLookPathFn = func(name string) (string, error) {
		if name == "docker" {
			return dockerPath, nil
		}
		if name == dockerPath {
			return dockerPath, nil
		}
		if name == "podman" {
			return podmanPath, nil
		}
		if name == podmanPath {
			return podmanPath, nil
		}
		return "", errors.New("missing")
	}
	execCommandContextFn = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		t.Fatalf("unexpected command call: %s %v", name, args)
		return testCommandExit(ctx, 1)
	}
	t.Cleanup(func() {
		execLookPathFn = prevLook
		execCommandContextFn = prevCmd
	})

	t.Setenv("SQLRS_CONTAINER_RUNTIME", "")
	t.Setenv("CONTAINER_HOST", "")
	if got := resolveContainerRuntimeBinary("auto"); got != dockerPath {
		t.Fatalf("expected docker path, got %q", got)
	}
	if host := os.Getenv("CONTAINER_HOST"); host != "" {
		t.Fatalf("expected empty CONTAINER_HOST, got %q", host)
	}
}

func TestResolveContainerRuntimeBinaryFallsBackToPodmanAndSetsContainerHost(t *testing.T) {
	prevLook := execLookPathFn
	prevCmd := execCommandContextFn
	execLookPathFn = func(name string) (string, error) {
		if name == "docker" {
			return "", errors.New("missing")
		}
		if name == "podman" {
			return "/usr/local/bin/podman", nil
		}
		return "", errors.New("unexpected")
	}
	execCommandContextFn = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		if name != "/usr/local/bin/podman" {
			t.Fatalf("expected podman binary, got %s", name)
		}
		if len(args) < 5 || args[0] != "system" || args[1] != "connection" || args[2] != "list" || args[3] != "--format" {
			t.Fatalf("unexpected args: %v", args)
		}
		if args[4] != "{{if .Default}}{{.URI}}\t{{.Identity}}{{end}}" {
			t.Fatalf("expected default format, got %q", args[4])
		}
		return testCommandOutput(ctx, "\nunix:///tmp/podman.sock\t/tmp/podman.identity\n")
	}
	t.Cleanup(func() {
		execLookPathFn = prevLook
		execCommandContextFn = prevCmd
	})

	t.Setenv("SQLRS_CONTAINER_RUNTIME", "")
	t.Setenv("CONTAINER_HOST", "")
	t.Setenv("CONTAINER_SSHKEY", "")
	if got := resolveContainerRuntimeBinary("auto"); got != "/usr/local/bin/podman" {
		t.Fatalf("expected podman path, got %q", got)
	}
	if host := os.Getenv("CONTAINER_HOST"); host != "unix:///tmp/podman.sock" {
		t.Fatalf("expected podman CONTAINER_HOST, got %q", host)
	}
	if sshKey := os.Getenv("CONTAINER_SSHKEY"); sshKey != "" {
		t.Fatalf("expected empty CONTAINER_SSHKEY for unix socket, got %q", sshKey)
	}
}

func TestResolveContainerRuntimeBinaryEnvPodmanSetsContainerHost(t *testing.T) {
	prevLook := execLookPathFn
	prevCmd := execCommandContextFn
	execLookPathFn = func(name string) (string, error) {
		if name == "podman" {
			return "/opt/homebrew/bin/podman", nil
		}
		return "", errors.New("unexpected")
	}
	execCommandContextFn = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		if name != "/opt/homebrew/bin/podman" {
			t.Fatalf("expected podman binary, got %s", name)
		}
		return testCommandOutput(ctx, "unix:///Users/runner/.local/share/containers/podman.sock\t/Users/runner/.local/share/containers/podman/machine/machine\n")
	}
	t.Cleanup(func() {
		execLookPathFn = prevLook
		execCommandContextFn = prevCmd
	})

	t.Setenv("SQLRS_CONTAINER_RUNTIME", "podman")
	t.Setenv("CONTAINER_HOST", "")
	t.Setenv("CONTAINER_SSHKEY", "")
	if got := resolveContainerRuntimeBinary("docker"); got != "/opt/homebrew/bin/podman" {
		t.Fatalf("expected env override to win over config mode, got %q", got)
	}
	if host := os.Getenv("CONTAINER_HOST"); host != "unix:///Users/runner/.local/share/containers/podman.sock" {
		t.Fatalf("expected podman CONTAINER_HOST, got %q", host)
	}
	if sshKey := os.Getenv("CONTAINER_SSHKEY"); sshKey != "" {
		t.Fatalf("expected empty CONTAINER_SSHKEY for unix socket, got %q", sshKey)
	}
}

func TestResolveContainerRuntimeBinaryDockerModeDoesNotFallbackToPodman(t *testing.T) {
	prevLook := execLookPathFn
	prevCmd := execCommandContextFn
	execLookPathFn = func(name string) (string, error) {
		if name == "docker" {
			return "", errors.New("missing")
		}
		if name == "podman" {
			t.Fatalf("unexpected podman lookup in docker mode")
		}
		return "", errors.New("unexpected")
	}
	execCommandContextFn = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		t.Fatalf("unexpected command call: %s %v", name, args)
		return testCommandExit(ctx, 1)
	}
	t.Cleanup(func() {
		execLookPathFn = prevLook
		execCommandContextFn = prevCmd
	})

	t.Setenv("SQLRS_CONTAINER_RUNTIME", "")
	t.Setenv("CONTAINER_HOST", "")
	if got := resolveContainerRuntimeBinary("docker"); got != "docker" {
		t.Fatalf("expected docker binary in docker mode, got %q", got)
	}
}

func TestResolveContainerRuntimeBinaryPodmanModeUsesPodman(t *testing.T) {
	prevLook := execLookPathFn
	prevCmd := execCommandContextFn
	execLookPathFn = func(name string) (string, error) {
		if name == "podman" {
			return "/opt/homebrew/bin/podman", nil
		}
		if name == "docker" {
			t.Fatalf("unexpected docker lookup in podman mode")
		}
		return "", errors.New("unexpected")
	}
	execCommandContextFn = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		if name != "/opt/homebrew/bin/podman" {
			t.Fatalf("expected podman binary, got %q", name)
		}
		return testCommandOutput(ctx, "unix:///Users/runner/.local/share/containers/podman.sock\t/Users/runner/.local/share/containers/podman/machine/machine\n")
	}
	t.Cleanup(func() {
		execLookPathFn = prevLook
		execCommandContextFn = prevCmd
	})

	t.Setenv("SQLRS_CONTAINER_RUNTIME", "")
	t.Setenv("CONTAINER_HOST", "")
	t.Setenv("CONTAINER_SSHKEY", "")
	if got := resolveContainerRuntimeBinary("podman"); got != "/opt/homebrew/bin/podman" {
		t.Fatalf("expected resolved podman path, got %q", got)
	}
	if host := os.Getenv("CONTAINER_HOST"); host != "unix:///Users/runner/.local/share/containers/podman.sock" {
		t.Fatalf("expected podman CONTAINER_HOST, got %q", host)
	}
	if sshKey := os.Getenv("CONTAINER_SSHKEY"); sshKey != "" {
		t.Fatalf("expected empty CONTAINER_SSHKEY for unix socket, got %q", sshKey)
	}
}

func TestResolveContainerRuntimeBinaryEnvAbsolutePodmanPath(t *testing.T) {
	absPodman := "/opt/podman/bin/podman"
	if runtime.GOOS == "windows" {
		absPodman = `C:\podman\podman.exe`
	}

	prevLook := execLookPathFn
	prevCmd := execCommandContextFn
	execLookPathFn = func(name string) (string, error) {
		t.Fatalf("unexpected lookpath call for %q", name)
		return "", errors.New("unexpected")
	}
	execCommandContextFn = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		if name != absPodman {
			t.Fatalf("expected absolute podman path, got %s", name)
		}
		return testCommandOutput(ctx, "unix:///tmp/abs-podman.sock\t/tmp/abs-podman.identity\n")
	}
	t.Cleanup(func() {
		execLookPathFn = prevLook
		execCommandContextFn = prevCmd
	})

	t.Setenv("SQLRS_CONTAINER_RUNTIME", absPodman)
	t.Setenv("CONTAINER_HOST", "")
	t.Setenv("CONTAINER_SSHKEY", "")
	if got := resolveContainerRuntimeBinary("auto"); got != absPodman {
		t.Fatalf("expected absolute podman path, got %q", got)
	}
	if host := os.Getenv("CONTAINER_HOST"); host != "unix:///tmp/abs-podman.sock" {
		t.Fatalf("expected podman CONTAINER_HOST, got %q", host)
	}
	if sshKey := os.Getenv("CONTAINER_SSHKEY"); sshKey != "" {
		t.Fatalf("expected empty CONTAINER_SSHKEY for unix socket, got %q", sshKey)
	}
}

func TestResolveContainerRuntimeBinaryKeepsExistingContainerHost(t *testing.T) {
	prevLook := execLookPathFn
	prevCmd := execCommandContextFn
	calls := 0
	execLookPathFn = func(name string) (string, error) {
		if name == "docker" {
			return "", errors.New("missing")
		}
		return "/usr/local/bin/podman", nil
	}
	execCommandContextFn = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		calls++
		return testCommandOutput(ctx, "unix:///tmp/podman.sock\n")
	}
	t.Cleanup(func() {
		execLookPathFn = prevLook
		execCommandContextFn = prevCmd
	})

	t.Setenv("SQLRS_CONTAINER_RUNTIME", "")
	t.Setenv("CONTAINER_HOST", "unix:///already/set.sock")
	if got := resolveContainerRuntimeBinary("auto"); got != "/usr/local/bin/podman" {
		t.Fatalf("expected podman path, got %q", got)
	}
	if host := os.Getenv("CONTAINER_HOST"); host != "unix:///already/set.sock" {
		t.Fatalf("expected CONTAINER_HOST to remain unchanged, got %q", host)
	}
	if calls != 0 {
		t.Fatalf("expected no podman connection probe, got %d calls", calls)
	}
}

func TestPodmanDefaultConnectionURIFallsBackToURIListFormat(t *testing.T) {
	prevCmd := execCommandContextFn
	calls := 0
	execCommandContextFn = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		calls++
		if len(args) < 5 || args[3] != "--format" {
			t.Fatalf("unexpected args: %v", args)
		}
		switch args[4] {
		case "{{if .Default}}{{.URI}}\t{{.Identity}}{{end}}":
			return testCommandOutput(ctx, "\n")
		case "{{.URI}}\t{{.Identity}}":
			return testCommandOutput(ctx, "\n")
		case "{{if .Default}}{{.URI}}{{end}}":
			return testCommandOutput(ctx, "\n")
		case "{{.URI}}":
			return testCommandOutput(ctx, "unix:///fallback.sock\n")
		default:
			t.Fatalf("unexpected format: %q", args[4])
		}
		return testCommandExit(ctx, 1)
	}
	t.Cleanup(func() { execCommandContextFn = prevCmd })

	got := podmanDefaultConnectionURI("/usr/local/bin/podman")
	if got != "unix:///fallback.sock" {
		t.Fatalf("expected fallback URI, got %q", got)
	}
	if calls != 4 {
		t.Fatalf("expected four podman calls, got %d", calls)
	}
}

func TestPodmanDefaultConnectionURIReturnsEmptyOnCommandError(t *testing.T) {
	prevCmd := execCommandContextFn
	calls := 0
	execCommandContextFn = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		calls++
		return testCommandExit(ctx, 1)
	}
	t.Cleanup(func() { execCommandContextFn = prevCmd })

	if got := podmanDefaultConnectionURI("/usr/local/bin/podman"); got != "" {
		t.Fatalf("expected empty URI, got %q", got)
	}
	if calls != 4 {
		t.Fatalf("expected four failed attempts, got %d", calls)
	}
}

func TestResolveContainerRuntimeBinaryPodmanSSHConnectionSetsContainerSSHKey(t *testing.T) {
	prevLook := execLookPathFn
	prevCmd := execCommandContextFn
	execLookPathFn = func(name string) (string, error) {
		if name == "podman" {
			return "/opt/homebrew/bin/podman", nil
		}
		return "", errors.New("unexpected")
	}
	execCommandContextFn = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		if name != "/opt/homebrew/bin/podman" {
			t.Fatalf("expected podman binary, got %q", name)
		}
		return testCommandOutput(ctx, "ssh://core@127.0.0.1:52830/run/user/501/podman/podman.sock\t/Users/test/.local/share/containers/podman/machine/machine\n")
	}
	t.Cleanup(func() {
		execLookPathFn = prevLook
		execCommandContextFn = prevCmd
	})

	t.Setenv("SQLRS_CONTAINER_RUNTIME", "podman")
	t.Setenv("CONTAINER_HOST", "")
	t.Setenv("CONTAINER_SSHKEY", "")
	if got := resolveContainerRuntimeBinary("auto"); got != "/opt/homebrew/bin/podman" {
		t.Fatalf("expected resolved podman path, got %q", got)
	}
	if host := os.Getenv("CONTAINER_HOST"); host != "ssh://core@127.0.0.1:52830/run/user/501/podman/podman.sock" {
		t.Fatalf("expected podman CONTAINER_HOST, got %q", host)
	}
	if sshKey := os.Getenv("CONTAINER_SSHKEY"); sshKey != "/Users/test/.local/share/containers/podman/machine/machine" {
		t.Fatalf("expected CONTAINER_SSHKEY from podman identity, got %q", sshKey)
	}
}

func TestResolveContainerRuntimeBinaryPodmanSSHConnectionKeepsExistingContainerSSHKey(t *testing.T) {
	prevLook := execLookPathFn
	prevCmd := execCommandContextFn
	execLookPathFn = func(name string) (string, error) {
		if name == "podman" {
			return "/opt/homebrew/bin/podman", nil
		}
		return "", errors.New("unexpected")
	}
	execCommandContextFn = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		if name != "/opt/homebrew/bin/podman" {
			t.Fatalf("expected podman binary, got %q", name)
		}
		return testCommandOutput(ctx, "ssh://core@127.0.0.1:52830/run/user/501/podman/podman.sock\t/Users/test/.local/share/containers/podman/machine/machine\n")
	}
	t.Cleanup(func() {
		execLookPathFn = prevLook
		execCommandContextFn = prevCmd
	})

	t.Setenv("SQLRS_CONTAINER_RUNTIME", "podman")
	t.Setenv("CONTAINER_HOST", "")
	t.Setenv("CONTAINER_SSHKEY", "/Users/test/.ssh/existing")
	if got := resolveContainerRuntimeBinary("auto"); got != "/opt/homebrew/bin/podman" {
		t.Fatalf("expected resolved podman path, got %q", got)
	}
	if sshKey := os.Getenv("CONTAINER_SSHKEY"); sshKey != "/Users/test/.ssh/existing" {
		t.Fatalf("expected existing CONTAINER_SSHKEY to remain unchanged, got %q", sshKey)
	}
}

func TestResolveContainerRuntimeBinaryPodmanSSHConnectionWithoutIdentityDoesNotSetContainerSSHKey(t *testing.T) {
	prevLook := execLookPathFn
	prevCmd := execCommandContextFn
	execLookPathFn = func(name string) (string, error) {
		if name == "podman" {
			return "/opt/homebrew/bin/podman", nil
		}
		return "", errors.New("unexpected")
	}
	execCommandContextFn = func(ctx context.Context, name string, args ...string) *exec.Cmd {
		if name != "/opt/homebrew/bin/podman" {
			t.Fatalf("expected podman binary, got %q", name)
		}
		return testCommandOutput(ctx, "ssh://core@127.0.0.1:52830/run/user/501/podman/podman.sock\n")
	}
	t.Cleanup(func() {
		execLookPathFn = prevLook
		execCommandContextFn = prevCmd
	})

	t.Setenv("SQLRS_CONTAINER_RUNTIME", "podman")
	t.Setenv("CONTAINER_HOST", "")
	t.Setenv("CONTAINER_SSHKEY", "")
	if got := resolveContainerRuntimeBinary("auto"); got != "/opt/homebrew/bin/podman" {
		t.Fatalf("expected resolved podman path, got %q", got)
	}
	if sshKey := os.Getenv("CONTAINER_SSHKEY"); sshKey != "" {
		t.Fatalf("expected empty CONTAINER_SSHKEY when podman identity is missing, got %q", sshKey)
	}
}

func TestBuildSummaryNotEmpty(t *testing.T) {
	if summary := buildSummary(); summary == "" {
		t.Fatalf("expected non-empty summary")
	}
}

func TestSetupLoggingWritesToFile(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "engine.json")
	logPath := filepath.Join(dir, "logs", "engine.log")

	prevWriter := log.Writer()
	t.Cleanup(func() { log.SetOutput(prevWriter) })

	closeLog, err := setupLogging(statePath)
	if err != nil {
		t.Fatalf("setupLogging: %v", err)
	}
	log.Print("hello log")
	closeLog()

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(data), "hello log") {
		t.Fatalf("expected log output, got %q", string(data))
	}
}

func TestRunMissingListen(t *testing.T) {
	code, err := run([]string{})
	if code != 2 || err == nil || err.Error() != "missing --listen" {
		t.Fatalf("expected missing --listen, got code=%d err=%v", code, err)
	}
}

func TestRunMissingStatePath(t *testing.T) {
	code, err := run([]string{"--listen=127.0.0.1:0"})
	if code != 2 || err == nil || err.Error() != "missing --write-engine-json" {
		t.Fatalf("expected missing --write-engine-json, got code=%d err=%v", code, err)
	}
}

func TestRunRunDirError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "run-dir")
	if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	statePath := filepath.Join(dir, "engine.json")
	code, err := run([]string{"--listen=127.0.0.1:0", "--write-engine-json=" + statePath, "--run-dir=" + path})
	if code != 1 || err == nil || !strings.Contains(err.Error(), "create run dir") {
		t.Fatalf("expected run dir error, got code=%d err=%v", code, err)
	}
}

func TestRunInvalidListen(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "engine.json")
	code, err := run([]string{"--listen=bad", "--write-engine-json=" + statePath})
	if code != 1 || err == nil || !strings.Contains(err.Error(), "listen:") {
		t.Fatalf("expected listen error, got code=%d err=%v", code, err)
	}
}

func TestRunUnknownFlag(t *testing.T) {
	code, err := run([]string{"--nope"})
	if code != 2 || err == nil {
		t.Fatalf("expected parse error, got code=%d err=%v", code, err)
	}
}

func TestRunServeError(t *testing.T) {
	previousServe := serveHTTP
	serveHTTP = func(server *http.Server, listener net.Listener) error {
		_ = listener.Close()
		return errors.New("boom")
	}
	t.Cleanup(func() { serveHTTP = previousServe })

	dir := t.TempDir()
	statePath := filepath.Join(dir, "engine.json")
	code, err := run([]string{"--listen=127.0.0.1:0", "--write-engine-json=" + statePath})
	if err != nil || code != 1 {
		t.Fatalf("expected server error code, got code=%d err=%v", code, err)
	}
}

func TestServeHTTPDefault(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	server := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})}
	_ = listener.Close()
	if err := serveHTTP(server, listener); err == nil {
		t.Fatalf("expected serveHTTP error")
	}
}

func TestRunRandomHexError(t *testing.T) {
	prevReader := randReader
	randReader = errorReader{}
	t.Cleanup(func() { randReader = prevReader })

	dir := t.TempDir()
	statePath := filepath.Join(dir, "engine.json")
	code, err := run([]string{"--listen=127.0.0.1:0", "--write-engine-json=" + statePath})
	if code != 1 || err == nil || !strings.Contains(err.Error(), "instance id") {
		t.Fatalf("expected instance id error, got code=%d err=%v", code, err)
	}
}

func TestRunAuthTokenError(t *testing.T) {
	prevReader := randReader
	seq := &sequenceReader{failOn: 2}
	randReader = seq
	t.Cleanup(func() { randReader = prevReader })

	dir := t.TempDir()
	statePath := filepath.Join(dir, "engine.json")
	code, err := run([]string{"--listen=127.0.0.1:0", "--write-engine-json=" + statePath})
	if code != 1 || err == nil || !strings.Contains(err.Error(), "auth token") {
		t.Fatalf("expected auth token error, got code=%d err=%v", code, err)
	}
}

func TestRunOpenStateDBError(t *testing.T) {
	prevOpen := openDBFn
	openDBFn = func(string) (*sql.DB, error) {
		return nil, errors.New("boom")
	}
	t.Cleanup(func() { openDBFn = prevOpen })

	dir := t.TempDir()
	statePath := filepath.Join(dir, "engine.json")
	code, err := run([]string{"--listen=127.0.0.1:0", "--write-engine-json=" + statePath})
	if code != 1 || err == nil || !strings.Contains(err.Error(), "open state db") {
		t.Fatalf("expected state db error, got code=%d err=%v", code, err)
	}
}

func TestRunUsesStateStoreRootForDB(t *testing.T) {
	prevOpen := openDBFn
	var gotPath string
	openDBFn = func(path string) (*sql.DB, error) {
		gotPath = path
		return nil, errors.New("boom")
	}
	t.Cleanup(func() { openDBFn = prevOpen })

	stateDir := t.TempDir()
	storeRoot := filepath.Join(t.TempDir(), "store-root")
	t.Setenv("SQLRS_STATE_STORE", storeRoot)
	statePath := filepath.Join(stateDir, "engine.json")
	code, err := run([]string{"--listen=127.0.0.1:0", "--write-engine-json=" + statePath})
	if code != 1 || err == nil || !strings.Contains(err.Error(), "open state db") {
		t.Fatalf("expected state db error, got code=%d err=%v", code, err)
	}
	expected := filepath.Join(storeRoot, "state.db")
	if gotPath != expected {
		t.Fatalf("expected state db path %s, got %s", expected, gotPath)
	}
}

func TestRunUsesExplicitStateDBPath(t *testing.T) {
	prevOpen := openDBFn
	var gotPath string
	openDBFn = func(path string) (*sql.DB, error) {
		gotPath = path
		return nil, errors.New("boom")
	}
	t.Cleanup(func() { openDBFn = prevOpen })

	stateDir := t.TempDir()
	storeRoot := filepath.Join(t.TempDir(), "store-root")
	dbPath := filepath.Join(t.TempDir(), "metadata", "state.db")
	t.Setenv("SQLRS_STATE_STORE", storeRoot)
	t.Setenv("SQLRS_STATE_DB", dbPath)
	statePath := filepath.Join(stateDir, "engine.json")
	code, err := run([]string{"--listen=127.0.0.1:0", "--write-engine-json=" + statePath})
	if code != 1 || err == nil || !strings.Contains(err.Error(), "open state db") {
		t.Fatalf("expected state db error, got code=%d err=%v", code, err)
	}
	if gotPath != dbPath {
		t.Fatalf("expected state db path %s, got %s", dbPath, gotPath)
	}
	if info, statErr := os.Stat(filepath.Dir(dbPath)); statErr != nil || !info.IsDir() {
		t.Fatalf("expected state db directory to be created: %v", statErr)
	}
}

func TestRunOpenQueueDBError(t *testing.T) {
	prevNew := newQueueFn
	newQueueFn = func(*sql.DB) (*queue.SQLiteStore, error) {
		return nil, errors.New("boom")
	}
	t.Cleanup(func() { newQueueFn = prevNew })

	dir := t.TempDir()
	statePath := filepath.Join(dir, "engine.json")
	code, err := run([]string{"--listen=127.0.0.1:0", "--write-engine-json=" + statePath})
	if code != 1 || err == nil || !strings.Contains(err.Error(), "open queue db") {
		t.Fatalf("expected queue db error, got code=%d err=%v", code, err)
	}
}

func TestRunPrepareManagerError(t *testing.T) {
	prevNew := newPrepareServiceFn
	newPrepareServiceFn = func(prepare.Options) (*prepare.PrepareService, error) {
		return nil, errors.New("boom")
	}
	t.Cleanup(func() { newPrepareServiceFn = prevNew })

	dir := t.TempDir()
	statePath := filepath.Join(dir, "engine.json")
	code, err := run([]string{"--listen=127.0.0.1:0", "--write-engine-json=" + statePath})
	if code != 1 || err == nil || !strings.Contains(err.Error(), "prepare service") {
		t.Fatalf("expected prepare service error, got code=%d err=%v", code, err)
	}
}

func TestRunPrepareRecoverError(t *testing.T) {
	prevRecover := prepareRecoverFn
	prepareRecoverFn = func(*prepare.PrepareService) error {
		return errors.New("boom")
	}
	t.Cleanup(func() { prepareRecoverFn = prevRecover })

	dir := t.TempDir()
	statePath := filepath.Join(dir, "engine.json")
	code, err := run([]string{"--listen=127.0.0.1:0", "--write-engine-json=" + statePath})
	if code != 1 || err == nil || !strings.Contains(err.Error(), "prepare recovery") {
		t.Fatalf("expected prepare recovery error, got code=%d err=%v", code, err)
	}
}

func TestRunDeletionManagerError(t *testing.T) {
	prevNew := newDeletionManagerFn
	newDeletionManagerFn = func(deletion.Options) (*deletion.Manager, error) {
		return nil, errors.New("boom")
	}
	t.Cleanup(func() { newDeletionManagerFn = prevNew })

	dir := t.TempDir()
	statePath := filepath.Join(dir, "engine.json")
	code, err := run([]string{"--listen=127.0.0.1:0", "--write-engine-json=" + statePath})
	if code != 1 || err == nil || !strings.Contains(err.Error(), "delete manager") {
		t.Fatalf("expected delete manager error, got code=%d err=%v", code, err)
	}
}

func TestRunRunManagerError(t *testing.T) {
	prevNew := newRunManagerFn
	newRunManagerFn = func(runpkg.Options) (*runpkg.Manager, error) {
		return nil, errors.New("boom")
	}
	t.Cleanup(func() { newRunManagerFn = prevNew })

	dir := t.TempDir()
	statePath := filepath.Join(dir, "engine.json")
	code, err := run([]string{"--listen=127.0.0.1:0", "--write-engine-json=" + statePath})
	if code != 1 || err == nil || !strings.Contains(err.Error(), "run manager") {
		t.Fatalf("expected run manager error, got code=%d err=%v", code, err)
	}
}

func TestRunSetupLoggingError(t *testing.T) {
	previousServe := serveHTTP
	serveHTTP = func(server *http.Server, listener net.Listener) error {
		_ = listener.Close()
		return http.ErrServerClosed
	}
	t.Cleanup(func() { serveHTTP = previousServe })

	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "logs"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write logs file: %v", err)
	}
	statePath := filepath.Join(dir, "engine.json")
	code, err := run([]string{"--listen=127.0.0.1:0", "--write-engine-json=" + statePath, "--idle-timeout=0"})
	if code != 0 || err != nil {
		t.Fatalf("expected success, got code=%d err=%v", code, err)
	}
}

func TestRunStateStoreRootError(t *testing.T) {
	dir := t.TempDir()
	stateStorePath := filepath.Join(dir, "state-store")
	if err := os.WriteFile(stateStorePath, []byte("x"), 0o600); err != nil {
		t.Fatalf("write state-store file: %v", err)
	}
	statePath := filepath.Join(dir, "engine.json")
	code, err := run([]string{"--listen=127.0.0.1:0", "--write-engine-json=" + statePath})
	if code != 1 || err == nil || !strings.Contains(err.Error(), "create state store root") {
		t.Fatalf("expected state store root error, got code=%d err=%v", code, err)
	}
}

func TestRunStateStoreRootFromEnv(t *testing.T) {
	previousServe := serveHTTP
	serveHTTP = func(server *http.Server, listener net.Listener) error {
		_ = listener.Close()
		return http.ErrServerClosed
	}
	t.Cleanup(func() { serveHTTP = previousServe })

	dir := t.TempDir()
	storeRoot := filepath.Join(dir, "custom-store")
	t.Setenv("SQLRS_STATE_STORE", storeRoot)

	statePath := filepath.Join(dir, "engine.json")
	code, err := run([]string{"--listen=127.0.0.1:0", "--write-engine-json=" + statePath, "--idle-timeout=0"})
	if code != 0 || err != nil {
		t.Fatalf("expected success, got code=%d err=%v", code, err)
	}
	if _, err := os.Stat(storeRoot); err != nil {
		t.Fatalf("expected state store root created, got %v", err)
	}
}

func TestRunConfigManagerError(t *testing.T) {
	dir := t.TempDir()
	stateStoreRoot := filepath.Join(dir, "state-store")
	if err := os.MkdirAll(stateStoreRoot, 0o700); err != nil {
		t.Fatalf("mkdir state-store: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateStoreRoot, "config.json"), []byte("{"), 0o600); err != nil {
		t.Fatalf("write config.json: %v", err)
	}
	statePath := filepath.Join(dir, "engine.json")
	code, err := run([]string{"--listen=127.0.0.1:0", "--write-engine-json=" + statePath})
	if code != 1 || err == nil || !strings.Contains(err.Error(), "config manager") {
		t.Fatalf("expected config manager error, got code=%d err=%v", code, err)
	}
}

func TestRunStoreError(t *testing.T) {
	prevStore := newStoreFn
	newStoreFn = func(*sql.DB) (*sqlite.Store, error) {
		return nil, errors.New("boom")
	}
	t.Cleanup(func() { newStoreFn = prevStore })

	dir := t.TempDir()
	statePath := filepath.Join(dir, "engine.json")
	code, err := run([]string{"--listen=127.0.0.1:0", "--write-engine-json=" + statePath})
	if code != 1 || err == nil || !strings.Contains(err.Error(), "open state db") {
		t.Fatalf("expected state store error, got code=%d err=%v", code, err)
	}
}

func TestRunWriteEngineStateError(t *testing.T) {
	previousServe := serveHTTP
	serveHTTP = func(server *http.Server, listener net.Listener) error {
		_ = listener.Close()
		return http.ErrServerClosed
	}
	t.Cleanup(func() { serveHTTP = previousServe })

	dir := t.TempDir()
	stateDir := filepath.Join(dir, "engine.json")
	if err := os.MkdirAll(stateDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(stateDir, "file.txt"), []byte("x"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	code, err := run([]string{"--listen=127.0.0.1:0", "--write-engine-json=" + stateDir})
	if code != 1 || err == nil || !strings.Contains(err.Error(), "write engine.json") {
		t.Fatalf("expected write engine.json error, got code=%d err=%v", code, err)
	}
}

func TestRunWithRunDir(t *testing.T) {
	previousServe := serveHTTP
	serveHTTP = func(server *http.Server, listener net.Listener) error {
		_ = listener.Close()
		return http.ErrServerClosed
	}
	t.Cleanup(func() { serveHTTP = previousServe })

	dir := t.TempDir()
	runDir := filepath.Join(dir, "run")
	statePath := filepath.Join(dir, "engine.json")
	code, err := run([]string{"--listen=127.0.0.1:0", "--write-engine-json=" + statePath, "--run-dir=" + runDir})
	if code != 0 || err != nil {
		t.Fatalf("expected success, got code=%d err=%v", code, err)
	}
	if _, err := os.Stat(runDir); err != nil {
		t.Fatalf("expected run dir created, got %v", err)
	}
}

func TestEnsureWSLMountSkippedWithoutEnv(t *testing.T) {
	prevRun := runMountCommandFn
	runMountCommandFn = func(string, ...string) (string, error) {
		t.Fatalf("unexpected mount command")
		return "", nil
	}
	t.Cleanup(func() { runMountCommandFn = prevRun })

	t.Setenv("SQLRS_WSL_MOUNT_UNIT", "")
	t.Setenv("SQLRS_WSL_MOUNT_FSTYPE", "")
	if err := ensureWSLMount("/tmp/sqlrs"); err != nil {
		t.Fatalf("ensureWSLMount: %v", err)
	}
}

func TestEnsureWSLMountRequiresStateStore(t *testing.T) {
	t.Setenv("SQLRS_WSL_MOUNT_UNIT", "sqlrs-state-store.mount")
	t.Setenv("SQLRS_WSL_MOUNT_FSTYPE", "btrfs")
	if err := ensureWSLMount(""); err == nil {
		t.Fatalf("expected error for empty state store")
	}
}

func TestEnsureWSLMountAlreadyMountedBtrfs(t *testing.T) {
	prevRun := runMountCommandFn
	calls := 0
	runMountCommandFn = func(name string, args ...string) (string, error) {
		calls++
		switch calls {
		case 1:
			if name != "systemctl" {
				t.Fatalf("expected systemctl, got %s", name)
			}
			return "active\n", nil
		case 2:
			if name != "findmnt" {
				t.Fatalf("expected findmnt, got %s", name)
			}
			return "btrfs\n", nil
		default:
			return "", nil
		}
	}
	t.Cleanup(func() { runMountCommandFn = prevRun })

	t.Setenv("SQLRS_WSL_MOUNT_UNIT", "sqlrs-state-store.mount")
	t.Setenv("SQLRS_WSL_MOUNT_FSTYPE", "btrfs")
	if err := ensureWSLMount(t.TempDir()); err != nil {
		t.Fatalf("ensureWSLMount: %v", err)
	}
	if calls != 2 {
		t.Fatalf("expected systemctl and findmnt calls, got %d", calls)
	}
}

func TestEnsureWSLMountAlreadyMountedWrongFS(t *testing.T) {
	prevRun := runMountCommandFn
	runMountCommandFn = func(name string, args ...string) (string, error) {
		switch name {
		case "systemctl":
			return "active\n", nil
		case "findmnt":
			return "ext4\n", nil
		default:
			return "", nil
		}
	}
	t.Cleanup(func() { runMountCommandFn = prevRun })

	t.Setenv("SQLRS_WSL_MOUNT_UNIT", "sqlrs-state-store.mount")
	t.Setenv("SQLRS_WSL_MOUNT_FSTYPE", "btrfs")
	if err := ensureWSLMount(t.TempDir()); err == nil {
		t.Fatalf("expected error for wrong fs")
	}
}

func TestEnsureWSLMountMountsAndVerifies(t *testing.T) {
	prevRun := runMountCommandFn
	var calls []string
	runMountCommandFn = func(name string, args ...string) (string, error) {
		calls = append(calls, name)
		switch len(calls) {
		case 1:
			if name != "systemctl" {
				t.Fatalf("expected systemctl, got %s", name)
			}
			return "inactive\n", exitError(3)
		case 2:
			if name != "systemctl" {
				t.Fatalf("expected systemctl start, got %s", name)
			}
			return "", nil
		case 3:
			if name != "systemctl" {
				t.Fatalf("expected systemctl is-active, got %s", name)
			}
			return "active\n", nil
		case 4:
			if name != "findmnt" {
				t.Fatalf("expected findmnt, got %s", name)
			}
			return "btrfs\n", nil
		default:
			return "", nil
		}
	}
	t.Cleanup(func() { runMountCommandFn = prevRun })

	t.Setenv("SQLRS_WSL_MOUNT_UNIT", "sqlrs-state-store.mount")
	t.Setenv("SQLRS_WSL_MOUNT_FSTYPE", "btrfs")
	if err := ensureWSLMount(t.TempDir()); err != nil {
		t.Fatalf("ensureWSLMount: %v", err)
	}
}

func TestEnsureWSLMountMountFails(t *testing.T) {
	prevRun := runMountCommandFn
	runMountCommandFn = func(name string, args ...string) (string, error) {
		if name == "systemctl" && len(args) > 0 && args[0] == "is-active" {
			return "inactive\n", exitError(3)
		}
		if name == "systemctl" && len(args) > 0 && args[0] == "start" {
			return "boom\n", errors.New("fail")
		}
		return "", nil
	}
	t.Cleanup(func() { runMountCommandFn = prevRun })

	t.Setenv("SQLRS_WSL_MOUNT_UNIT", "sqlrs-state-store.mount")
	t.Setenv("SQLRS_WSL_MOUNT_FSTYPE", "btrfs")
	if err := ensureWSLMount(t.TempDir()); err == nil {
		t.Fatalf("expected mount error")
	}
}

func TestEnsureWSLMountVerifyFails(t *testing.T) {
	prevRun := runMountCommandFn
	runMountCommandFn = func(name string, args ...string) (string, error) {
		switch name {
		case "systemctl":
			if len(args) > 0 && args[0] == "is-active" {
				return "active\n", nil
			}
			return "", nil
		case "findmnt":
			return "ext4\n", nil
		default:
			return "", nil
		}
	}
	t.Cleanup(func() { runMountCommandFn = prevRun })

	t.Setenv("SQLRS_WSL_MOUNT_UNIT", "sqlrs-state-store.mount")
	t.Setenv("SQLRS_WSL_MOUNT_FSTYPE", "btrfs")
	if err := ensureWSLMount(t.TempDir()); err == nil {
		t.Fatalf("expected verification error")
	}
}

func TestEnsureWSLMountFindmntUnexpectedError(t *testing.T) {
	prevRun := runMountCommandFn
	runMountCommandFn = func(name string, args ...string) (string, error) {
		switch name {
		case "systemctl":
			return "active\n", nil
		case "findmnt":
			return "", errors.New("boom")
		default:
			t.Fatalf("unexpected command: %s", name)
		}
		return "", nil
	}
	t.Cleanup(func() { runMountCommandFn = prevRun })

	t.Setenv("SQLRS_WSL_MOUNT_UNIT", "sqlrs-state-store.mount")
	t.Setenv("SQLRS_WSL_MOUNT_FSTYPE", "btrfs")
	if err := ensureWSLMount(t.TempDir()); err == nil {
		t.Fatalf("expected error")
	}
}

func TestRunIdleTimeoutShutdown(t *testing.T) {
	previousServe := serveHTTP
	serveHTTP = func(server *http.Server, listener net.Listener) error {
		return server.Serve(listener)
	}
	t.Cleanup(func() { serveHTTP = previousServe })
	prevTicker := idleTickerEvery
	idleTickerEvery = 10 * time.Millisecond
	t.Cleanup(func() { idleTickerEvery = prevTicker })

	dir := t.TempDir()
	statePath := filepath.Join(dir, "engine.json")
	code, err := run([]string{"--listen=127.0.0.1:0", "--write-engine-json=" + statePath, "--idle-timeout=100ms"})
	if err != nil || code != 0 {
		t.Fatalf("expected success, got code=%d err=%v", code, err)
	}
}

func TestRunIdleTimeoutWaitsForInflightRequest(t *testing.T) {
	prevHandler := newHandlerFn
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	newHandlerFn = func(opts httpapi.Options) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			select {
			case started <- struct{}{}:
			default:
			}
			<-release
			w.WriteHeader(http.StatusOK)
		})
	}
	t.Cleanup(func() { newHandlerFn = prevHandler })

	previousServe := serveHTTP
	serveHTTP = func(server *http.Server, listener net.Listener) error {
		defer func() { _ = server.Close() }()

		serverDone := make(chan error, 1)
		go func() {
			serverDone <- server.Serve(listener)
		}()

		reqDone := make(chan error, 1)
		go func() {
			resp, err := http.Get("http://" + listener.Addr().String() + "/hold")
			if err == nil {
				_ = resp.Body.Close()
				if resp.StatusCode != http.StatusOK {
					err = errors.New("unexpected status code")
				}
			}
			reqDone <- err
		}()

		select {
		case <-started:
		case <-time.After(2 * time.Second):
			return errors.New("request did not reach handler")
		}

		time.Sleep(300 * time.Millisecond)

		select {
		case <-serverDone:
			return errors.New("server shut down while request was still in flight")
		default:
		}

		close(release)

		select {
		case err := <-reqDone:
			if err != nil {
				return err
			}
		case <-time.After(2 * time.Second):
			return errors.New("request did not complete")
		}

		select {
		case err := <-serverDone:
			return err
		case <-time.After(2 * time.Second):
			return errors.New("server did not shut down after request completion")
		}
	}
	t.Cleanup(func() { serveHTTP = previousServe })

	prevTicker := idleTickerEvery
	idleTickerEvery = 10 * time.Millisecond
	t.Cleanup(func() { idleTickerEvery = prevTicker })

	dir := t.TempDir()
	statePath := filepath.Join(dir, "engine.json")
	code, err := run([]string{"--listen=127.0.0.1:0", "--write-engine-json=" + statePath, "--idle-timeout=100ms"})
	if err != nil || code != 0 {
		t.Fatalf("expected success, got code=%d err=%v", code, err)
	}
}

func exitError(code int) error {
	cmd := exec.Command("sh", "-c", "exit "+strconv.Itoa(code))
	if runtime.GOOS == "windows" {
		cmd = exec.Command("cmd", "/c", "exit "+strconv.Itoa(code))
	}
	err := cmd.Run()
	if err == nil {
		return errors.New("expected exit error")
	}
	return err
}

func TestRunShutdownError(t *testing.T) {
	previousServe := serveHTTP
	serveHTTP = func(server *http.Server, listener net.Listener) error {
		time.Sleep(50 * time.Millisecond)
		_ = listener.Close()
		return http.ErrServerClosed
	}
	t.Cleanup(func() { serveHTTP = previousServe })
	prevTicker := idleTickerEvery
	idleTickerEvery = 10 * time.Millisecond
	t.Cleanup(func() { idleTickerEvery = prevTicker })

	prevShutdown := serverShutdownFn
	serverShutdownFn = func(server *http.Server, ctx context.Context) error {
		return errors.New("boom")
	}
	t.Cleanup(func() { serverShutdownFn = prevShutdown })

	dir := t.TempDir()
	statePath := filepath.Join(dir, "engine.json")
	code, err := run([]string{"--listen=127.0.0.1:0", "--write-engine-json=" + statePath, "--idle-timeout=10ms"})
	if err != nil || code != 0 {
		t.Fatalf("expected success, got code=%d err=%v", code, err)
	}
}

func TestSetupLoggingSuccess(t *testing.T) {
	dir := t.TempDir()
	statePath := filepath.Join(dir, "engine.json")
	prev := log.Writer()
	cleanup, err := setupLogging(statePath)
	if err != nil {
		t.Fatalf("setupLogging: %v", err)
	}
	t.Cleanup(func() {
		cleanup()
		log.SetOutput(prev)
	})
	if _, err := os.Stat(filepath.Join(dir, "logs", "engine.log")); err != nil {
		t.Fatalf("expected log file, got %v", err)
	}
}

func TestSetupLoggingRejectsLogDirFile(t *testing.T) {
	dir := t.TempDir()
	logDir := filepath.Join(dir, "logs")
	if err := os.WriteFile(logDir, []byte("x"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, err := setupLogging(filepath.Join(dir, "engine.json")); err == nil {
		t.Fatalf("expected error")
	}
}

func TestSetupLoggingRejectsLogFileDir(t *testing.T) {
	dir := t.TempDir()
	logDir := filepath.Join(dir, "logs")
	if err := os.MkdirAll(logDir, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	logPath := filepath.Join(logDir, "engine.log")
	if err := os.MkdirAll(logPath, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if _, err := setupLogging(filepath.Join(dir, "engine.json")); err == nil {
		t.Fatalf("expected error")
	}
}

func TestSetupLoggingCharDevice(t *testing.T) {
	prevFn := isCharDeviceFn
	isCharDeviceFn = func(*os.File) bool { return true }
	t.Cleanup(func() { isCharDeviceFn = prevFn })

	dir := t.TempDir()
	statePath := filepath.Join(dir, "engine.json")
	cleanup, err := setupLogging(statePath)
	if err != nil {
		t.Fatalf("setupLogging: %v", err)
	}
	t.Cleanup(cleanup)
}

func TestIsCharDeviceBehaviors(t *testing.T) {
	if isCharDevice(nil) {
		t.Fatalf("expected false for nil file")
	}
	file, err := os.CreateTemp(t.TempDir(), "char-test-*")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	if isCharDevice(file) {
		t.Fatalf("expected false for regular file")
	}
	if err := file.Close(); err != nil {
		t.Fatalf("close: %v", err)
	}
	if isCharDevice(file) {
		t.Fatalf("expected false for closed file")
	}
}

func TestRunMountCommandOutput(t *testing.T) {
	var name string
	var args []string
	if runtime.GOOS == "windows" {
		name = "cmd"
		args = []string{"/c", "echo", "ok"}
	} else {
		name = "sh"
		args = []string{"-c", "echo ok"}
	}
	out, err := runMountCommand(name, args...)
	if err != nil {
		t.Fatalf("runMountCommand: %v", err)
	}
	if strings.TrimSpace(out) != "ok" {
		t.Fatalf("expected output ok, got %q", out)
	}
}

func TestRunSuccess(t *testing.T) {
	previousServe := serveHTTP
	called := false
	serveHTTP = func(server *http.Server, listener net.Listener) error {
		called = true
		_ = listener.Close()
		return http.ErrServerClosed
	}
	t.Cleanup(func() { serveHTTP = previousServe })

	dir := t.TempDir()
	statePath := filepath.Join(dir, "engine.json")
	code, err := run([]string{"--listen=127.0.0.1:0", "--write-engine-json=" + statePath, "--idle-timeout=0"})
	if err != nil || code != 0 {
		t.Fatalf("expected success, got code=%d err=%v", code, err)
	}
	if !called {
		t.Fatalf("expected serveHTTP to be called")
	}
	if _, err := os.Stat(statePath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected engine.json removed, got %v", err)
	}
}

func TestRunHandlerLogsRequest(t *testing.T) {
	previousServe := serveHTTP
	serveHTTP = func(server *http.Server, listener net.Listener) error {
		req := httptest.NewRequest(http.MethodGet, "http://example/v1/health", nil)
		writer := &flushWriter{}
		server.Handler.ServeHTTP(writer, req)
		_ = listener.Close()
		return http.ErrServerClosed
	}
	t.Cleanup(func() { serveHTTP = previousServe })

	dir := t.TempDir()
	statePath := filepath.Join(dir, "engine.json")
	code, err := run([]string{"--listen=127.0.0.1:0", "--write-engine-json=" + statePath, "--idle-timeout=0"})
	if err != nil || code != 0 {
		t.Fatalf("expected success, got code=%d err=%v", code, err)
	}
}

func TestRunHandlerDefaultsStatusWhenNoWrite(t *testing.T) {
	prevHandler := newHandlerFn
	newHandlerFn = func(opts httpapi.Options) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})
	}
	t.Cleanup(func() { newHandlerFn = prevHandler })

	previousServe := serveHTTP
	serveHTTP = func(server *http.Server, listener net.Listener) error {
		req := httptest.NewRequest(http.MethodGet, "http://example/noop", nil)
		writer := &flushWriter{}
		server.Handler.ServeHTTP(writer, req)
		_ = listener.Close()
		return http.ErrServerClosed
	}
	t.Cleanup(func() { serveHTTP = previousServe })

	dir := t.TempDir()
	statePath := filepath.Join(dir, "engine.json")
	code, err := run([]string{"--listen=127.0.0.1:0", "--write-engine-json=" + statePath, "--idle-timeout=0"})
	if err != nil || code != 0 {
		t.Fatalf("expected success, got code=%d err=%v", code, err)
	}
}

func TestMainUsesExitCode(t *testing.T) {
	prevExit := exitFn
	exitCode := 0
	exitFn = func(code int) {
		exitCode = code
	}
	t.Cleanup(func() { exitFn = prevExit })

	prevArgs := os.Args
	os.Args = []string{"sqlrs-engine"}
	t.Cleanup(func() { os.Args = prevArgs })

	main()
	if exitCode != 2 {
		t.Fatalf("expected exit code 2, got %d", exitCode)
	}
}

func TestStatusRecorderWriteDefaults(t *testing.T) {
	rec := &statusRecorder{ResponseWriter: httptest.NewRecorder()}
	if rec.status != 0 {
		t.Fatalf("expected zero status, got %d", rec.status)
	}
	n, err := rec.Write([]byte("ok"))
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if n != 2 || rec.bytes != 2 {
		t.Fatalf("expected 2 bytes, got n=%d bytes=%d", n, rec.bytes)
	}
	if rec.status != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.status)
	}
}

func TestStatusRecorderWriteHeader(t *testing.T) {
	rec := &statusRecorder{ResponseWriter: httptest.NewRecorder()}
	rec.WriteHeader(http.StatusNotFound)
	_, err := rec.Write([]byte("x"))
	if err != nil {
		t.Fatalf("write: %v", err)
	}
	if rec.status != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.status)
	}
	if rec.bytes != 1 {
		t.Fatalf("expected 1 byte, got %d", rec.bytes)
	}
}

func TestStatusRecorderFlusher(t *testing.T) {
	writer := &flushWriter{}
	rec := &statusRecorder{ResponseWriter: writer}
	wrapped := &statusRecorderFlusher{statusRecorder: rec, flusher: writer}
	wrapped.Flush()
	if !writer.flushed {
		t.Fatalf("expected flush to be called")
	}
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) {
	return 0, errors.New("boom")
}

type sequenceReader struct {
	calls  int
	failOn int
}

func (r *sequenceReader) Read(p []byte) (int, error) {
	r.calls++
	if r.calls == r.failOn {
		return 0, errors.New("boom")
	}
	for i := range p {
		p[i] = 0x1
	}
	return len(p), nil
}

type flushWriter struct {
	header  http.Header
	flushed bool
}

func (w *flushWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *flushWriter) Write(data []byte) (int, error) {
	return len(data), nil
}

func (w *flushWriter) WriteHeader(status int) {
}

func (w *flushWriter) Flush() {
	w.flushed = true
}

func testCommandOutput(ctx context.Context, output string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		lines := strings.Split(strings.TrimRight(output, "\n"), "\n")
		parts := make([]string, 0, len(lines))
		for _, line := range lines {
			line = strings.TrimRight(line, "\r")
			if line == "" {
				parts = append(parts, "echo.")
				continue
			}
			parts = append(parts, "echo "+line)
		}
		return exec.CommandContext(ctx, "cmd", "/c", strings.Join(parts, " & "))
	}
	escaped := strings.ReplaceAll(output, `"`, `\"`)
	return exec.CommandContext(ctx, "sh", "-c", "printf \"%s\" \""+escaped+"\"")
}

func testCommandExit(ctx context.Context, code int) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.CommandContext(ctx, "cmd", "/c", "exit "+strconv.Itoa(code))
	}
	return exec.CommandContext(ctx, "sh", "-c", "exit "+strconv.Itoa(code))
}
