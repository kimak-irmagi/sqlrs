package app

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/sqlrs/cli/internal/enginebin"
)

func TestParseInitFlagsSeparatesHostAndWSLEngines(t *testing.T) {
	withWindowsMode(t)
	opts, help, err := parseInitFlags([]string{"local", "--engine", "host.exe", "--wsl-engine", "linux-engine"}, "")
	if err != nil || help {
		t.Fatalf("parseInitFlags() help=%v err=%v", help, err)
	}
	if opts.EnginePath != "host.exe" || opts.WSLEnginePath != "linux-engine" {
		t.Fatalf("unexpected engine paths: host=%q wsl=%q", opts.EnginePath, opts.WSLEnginePath)
	}
}

func TestParseInitFlagsRejectsWSLEngineOutsideWindows(t *testing.T) {
	previous := isWindows
	isWindows = false
	t.Cleanup(func() { isWindows = previous })

	_, _, err := parseInitFlags([]string{"local", "--wsl-engine", "linux-engine"}, "")
	if err == nil || !strings.Contains(err.Error(), "only valid on Windows") {
		t.Fatalf("expected Windows-only flag error, got %v", err)
	}
}

func TestParseInitFlagsRejectsWSLEngineForRemote(t *testing.T) {
	_, _, err := parseInitFlags([]string{"remote", "--url", "https://example.test", "--token", "secret", "--wsl-engine", "linux-engine"}, "")
	if err == nil || !strings.Contains(err.Error(), "local-only") {
		t.Fatalf("expected local-only flag error, got %v", err)
	}
}

func TestInstallWSLEngineWritesAtomically(t *testing.T) {
	source := filepath.Join(t.TempDir(), "sqlrs-engine")
	payload := []byte("\x7fELF-linux-engine")
	if err := os.WriteFile(source, payload, 0o755); err != nil {
		t.Fatal(err)
	}

	previous := runWSLCommandWithInputFn
	t.Cleanup(func() { runWSLCommandWithInputFn = previous })
	var gotInput, gotCommand string
	var gotArgs []string
	runWSLCommandWithInputFn = func(_ context.Context, distro string, verbose bool, desc, input, command string, args ...string) (string, error) {
		if distro != "Ubuntu" || desc != "install WSL engine" {
			t.Fatalf("unexpected call distro=%q desc=%q", distro, desc)
		}
		gotInput, gotCommand, gotArgs = input, command, append([]string(nil), args...)
		return defaultInstalledWSLEnginePath + "\n", nil
	}

	got, err := installWSLEngine(context.Background(), "Ubuntu", source, "", false)
	if err != nil {
		t.Fatalf("installWSLEngine: %v", err)
	}
	if got != defaultInstalledWSLEnginePath {
		t.Fatalf("installed path=%q", got)
	}
	if !bytes.Equal([]byte(gotInput), payload) {
		t.Fatal("installer did not stream the source payload")
	}
	expectedMachine, err := expectedELFMachineHex(runtime.GOARCH)
	if err != nil {
		t.Fatal(err)
	}
	if gotArgs[len(gotArgs)-2] != expectedMachine {
		t.Fatalf("default install args=%q, want expected machine before destination", gotArgs)
	}
	if gotArgs[len(gotArgs)-1] != defaultWSLEngineDestination {
		t.Fatalf("default install args=%q, want a non-empty default destination sentinel", gotArgs)
	}
	for _, arg := range gotArgs {
		if arg == "" {
			t.Fatalf("default install must not pass an empty argument through wsl.exe: %q", gotArgs)
		}
	}
	joined := gotCommand + " " + strings.Join(gotArgs, " ")
	for _, required := range []string{"mktemp", "chmod 755", "mv -f", "$HOME/.local/lib/sqlrs/sqlrs-engine", defaultWSLEngineDestination} {
		if !strings.Contains(joined, required) {
			t.Fatalf("atomic install command %q does not contain %q", joined, required)
		}
	}
}

func TestRunInitPassesResolvedWSLEngineToProvisioner(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("WSL init is only exercised by the Windows CI job")
	}
	withWindowsMode(t)
	workspace := t.TempDir()
	engine := filepath.Join(workspace, "sqlrs-engine")
	if err := os.WriteFile(engine, []byte("engine"), 0o755); err != nil {
		t.Fatal(err)
	}

	withInitWSLStub(t, func(opts wslInitOptions) (wslInitResult, error) {
		if opts.EngineSourcePath != engine {
			t.Fatalf("engine source=%q, want %q", opts.EngineSourcePath, engine)
		}
		return wslInitResult{UseWSL: true, EnginePath: defaultInstalledWSLEnginePath}, nil
	})
	previousResolver := resolveWSLPayloadFn
	resolveWSLPayloadFn = func(explicit string) (enginebin.Resolved, error) {
		if explicit != engine {
			t.Fatalf("explicit WSL engine=%q", explicit)
		}
		return enginebin.Resolved{Path: engine, Kind: enginebin.KindWSLPayload}, nil
	}
	t.Cleanup(func() { resolveWSLPayloadFn = previousResolver })

	var out bytes.Buffer
	if err := runInit(&out, workspace, "", []string{"local", "--snapshot", "btrfs", "--wsl-engine", "sqlrs-engine"}, false); err != nil {
		t.Fatalf("runInit: %v", err)
	}
}

func TestRunInitRepairsMissingDerivedWSLEngineWithoutUpdate(t *testing.T) {
	withWindowsMode(t)
	workspace := t.TempDir()
	marker := filepath.Join(workspace, ".sqlrs")
	if err := os.MkdirAll(marker, 0o700); err != nil {
		t.Fatal(err)
	}
	const installed = "/home/user/.local/lib/sqlrs/sqlrs-engine"
	configData := []byte("engine:\n  wsl:\n    mode: required\n    distro: Ubuntu\n    enginePath: " + installed + "\n")
	if err := os.WriteFile(filepath.Join(marker, "config.yaml"), configData, 0o600); err != nil {
		t.Fatal(err)
	}
	source := filepath.Join(t.TempDir(), "sqlrs-engine")
	if err := os.WriteFile(source, []byte("payload"), 0o755); err != nil {
		t.Fatal(err)
	}

	previousResolver := resolveWSLPayloadFn
	resolveWSLPayloadFn = func(string) (enginebin.Resolved, error) {
		return enginebin.Resolved{Path: source, Kind: enginebin.KindWSLPayload}, nil
	}
	t.Cleanup(func() { resolveWSLPayloadFn = previousResolver })
	previousCheck := runWSLCommandAllowFailureFn
	runWSLCommandAllowFailureFn = func(context.Context, string, bool, string, string, ...string) (string, error) {
		return "", os.ErrNotExist
	}
	t.Cleanup(func() { runWSLCommandAllowFailureFn = previousCheck })
	previousInstall := runWSLCommandWithInputFn
	installedCalls := 0
	runWSLCommandWithInputFn = func(_ context.Context, _ string, _ bool, _ string, _ string, _ string, args ...string) (string, error) {
		installedCalls++
		if args[len(args)-1] != installed {
			t.Fatalf("repair destination=%q", args[len(args)-1])
		}
		return installed + "\n", nil
	}
	t.Cleanup(func() { runWSLCommandWithInputFn = previousInstall })

	var out bytes.Buffer
	if err := runInit(&out, workspace, "", []string{"local"}, false); err != nil {
		t.Fatalf("runInit: %v", err)
	}
	if installedCalls != 1 {
		t.Fatalf("install calls=%d", installedCalls)
	}
	got, err := os.ReadFile(filepath.Join(marker, "config.yaml"))
	if err != nil || !bytes.Equal(got, configData) {
		t.Fatalf("repair changed workspace config: err=%v\n%s", err, got)
	}
}

func TestRunInitAutoRejectsUnavailableWSLPayload(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("automatic WSL selection is only available on Windows")
	}
	withWindowsMode(t)
	previousResolver := resolveWSLPayloadFn
	resolveWSLPayloadFn = func(string) (enginebin.Resolved, error) {
		return enginebin.Resolved{}, os.ErrNotExist
	}
	t.Cleanup(func() { resolveWSLPayloadFn = previousResolver })
	workspace := t.TempDir()
	var out bytes.Buffer
	err := runInit(&out, workspace, "", []string{"local"}, false)
	if err == nil || !strings.Contains(err.Error(), "Invalid WSL engine") {
		t.Fatalf("expected missing-payload error, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(workspace, ".sqlrs")); !os.IsNotExist(statErr) {
		t.Fatalf("workspace marker must not be written, stat err=%v", statErr)
	}
}
