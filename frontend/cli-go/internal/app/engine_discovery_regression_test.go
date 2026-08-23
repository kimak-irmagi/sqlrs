package app

import (
	"bytes"
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/sqlrs/cli/internal/cli"
	"github.com/sqlrs/cli/internal/enginebin"
)

var defaultTestHostResolver = func(req enginebin.Request) (enginebin.Resolved, error) {
	for _, candidate := range []struct {
		path   string
		origin enginebin.Origin
	}{
		{req.ExplicitPath, enginebin.OriginExplicit},
		{req.EnvironmentPath, enginebin.OriginEnvironment},
		{req.ConfigPath, enginebin.OriginConfig},
	} {
		if strings.TrimSpace(candidate.path) != "" {
			return enginebin.Resolved{Path: candidate.path, Kind: enginebin.KindHost, Origin: candidate.origin}, nil
		}
	}
	return enginebin.Resolved{Path: filepath.Join(os.TempDir(), "sqlrs-engine"), Kind: enginebin.KindHost, Origin: enginebin.OriginBundle}, nil
}

func TestMain(m *testing.M) {
	resolveHostEngineFn = defaultTestHostResolver
	resolveWSLPayloadFn = func(explicit string) (enginebin.Resolved, error) {
		if strings.TrimSpace(explicit) != "" {
			return (enginebin.Resolver{}).Resolve(enginebin.Request{
				Kind:         enginebin.KindWSLPayload,
				TargetOS:     "linux",
				TargetArch:   runtime.GOARCH,
				ExplicitPath: explicit,
			})
		}
		return enginebin.Resolved{Path: filepath.Join(os.TempDir(), "sqlrs-engine-linux"), Kind: enginebin.KindWSLPayload, Origin: enginebin.OriginBundle}, nil
	}
	os.Exit(m.Run())
}

func withHostResolver(t *testing.T, fn func(enginebin.Request) (enginebin.Resolved, error)) {
	t.Helper()
	previous := resolveHostEngineFn
	resolveHostEngineFn = fn
	t.Cleanup(func() { resolveHostEngineFn = previous })
}

func TestRunInitRejectsMissingHostEngineBeforeWritingConfig(t *testing.T) {
	workspace := t.TempDir()
	withHostResolver(t, func(req enginebin.Request) (enginebin.Resolved, error) {
		if req.Kind != enginebin.KindHost {
			t.Fatalf("kind=%q, want host", req.Kind)
		}
		return enginebin.Resolved{}, errors.New("host engine was not found")
	})

	var out bytes.Buffer
	err := runInit(&out, workspace, "", []string{"local", "--snapshot", "copy"}, false)
	if err == nil || !strings.Contains(err.Error(), "Invalid host engine") {
		t.Fatalf("expected host discovery error, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(workspace, ".sqlrs")); !os.IsNotExist(statErr) {
		t.Fatalf("workspace marker must not be written, stat err=%v", statErr)
	}
}

func TestResolveCommandContextDefersConfiguredHostCandidateValidation(t *testing.T) {
	temp := t.TempDir()
	setTestDirs(t, temp)
	marker := filepath.Join(temp, ".sqlrs")
	if err := os.MkdirAll(marker, 0o700); err != nil {
		t.Fatal(err)
	}
	configured := filepath.Join(temp, "wrong-platform-engine")
	configData := []byte("profiles:\n  local:\n    mode: local\norchestrator:\n  daemonPath: " + configured + "\n")
	if err := os.WriteFile(filepath.Join(marker, "config.yaml"), configData, 0o600); err != nil {
		t.Fatal(err)
	}
	withHostResolver(t, func(req enginebin.Request) (enginebin.Resolved, error) {
		t.Fatal("command context must defer host discovery until daemon autostart")
		return enginebin.Resolved{}, nil
	})

	ctx, err := resolveCommandContext(temp, cli.GlobalOptions{})
	if err != nil {
		t.Fatalf("resolveCommandContext: %v", err)
	}
	if ctx.daemonPath != configured {
		t.Fatalf("daemonPath=%q, want deferred candidate %q", ctx.daemonPath, configured)
	}
}

func TestRunInitDoesNotAttemptWSLRepairOutsideWindows(t *testing.T) {
	previousWindows := isWindows
	isWindows = false
	t.Cleanup(func() { isWindows = previousWindows })

	workspace := t.TempDir()
	marker := filepath.Join(workspace, ".sqlrs")
	if err := os.MkdirAll(marker, 0o700); err != nil {
		t.Fatal(err)
	}
	configData := []byte("engine:\n  wsl:\n    mode: required\n    distro: Ubuntu\n    enginePath: /home/user/.local/lib/sqlrs/sqlrs-engine\n")
	configPath := filepath.Join(marker, "config.yaml")
	if err := os.WriteFile(configPath, configData, 0o600); err != nil {
		t.Fatal(err)
	}
	previousCheck := runWSLCommandAllowFailureFn
	runWSLCommandAllowFailureFn = func(context.Context, string, bool, string, string, ...string) (string, error) {
		t.Fatal("non-Windows init attempted WSL repair")
		return "", nil
	}
	t.Cleanup(func() { runWSLCommandAllowFailureFn = previousCheck })

	var out bytes.Buffer
	if err := runInit(&out, workspace, "", []string{"local"}, false); err != nil {
		t.Fatalf("runInit: %v", err)
	}
}

func TestRunInitRepairsExecutableButInvalidWSLEngine(t *testing.T) {
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
	previousValidate := validateInstalledWSLEngineFn
	validateInstalledWSLEngineFn = func(context.Context, string, string, bool) error {
		return errors.New("invalid ELF architecture")
	}
	t.Cleanup(func() { validateInstalledWSLEngineFn = previousValidate })
	previousResolver := resolveWSLPayloadFn
	source := filepath.Join(t.TempDir(), "sqlrs-engine")
	if err := os.WriteFile(source, []byte("payload"), 0o755); err != nil {
		t.Fatal(err)
	}
	resolveWSLPayloadFn = func(string) (enginebin.Resolved, error) {
		return enginebin.Resolved{Path: source, Kind: enginebin.KindWSLPayload}, nil
	}
	t.Cleanup(func() { resolveWSLPayloadFn = previousResolver })
	previousInstall := runWSLCommandWithInputFn
	installCalls := 0
	runWSLCommandWithInputFn = func(_ context.Context, _ string, _ bool, _ string, _ string, _ string, _ ...string) (string, error) {
		installCalls++
		return installed + "\n", nil
	}
	t.Cleanup(func() { runWSLCommandWithInputFn = previousInstall })

	var out bytes.Buffer
	if err := runInit(&out, workspace, "", []string{"local"}, false); err != nil {
		t.Fatalf("runInit: %v", err)
	}
	if installCalls != 1 {
		t.Fatalf("install calls=%d, want 1", installCalls)
	}
}

func TestRunInitLegacyWSLRequiresUpdateWithoutChangingConfig(t *testing.T) {
	withWindowsMode(t)
	workspace := t.TempDir()
	marker := filepath.Join(workspace, ".sqlrs")
	if err := os.MkdirAll(marker, 0o700); err != nil {
		t.Fatal(err)
	}
	legacy := filepath.Join(workspace, "legacy", "sqlrs-engine")
	if err := os.MkdirAll(filepath.Dir(legacy), 0o700); err != nil {
		t.Fatal(err)
	}
	writeTestELF(t, legacy)
	configData := []byte("orchestrator:\n  daemonPath: ../legacy/sqlrs-engine\nengine:\n  wsl:\n    mode: required\n    distro: Ubuntu\n")
	configPath := filepath.Join(marker, "config.yaml")
	if err := os.WriteFile(configPath, configData, 0o600); err != nil {
		t.Fatal(err)
	}
	previousResolver := resolveWSLPayloadFn
	resolveWSLPayloadFn = func(string) (enginebin.Resolved, error) {
		t.Fatal("valid relative legacy engine should be resolved from config")
		return enginebin.Resolved{}, nil
	}
	t.Cleanup(func() { resolveWSLPayloadFn = previousResolver })
	previousInstall := runWSLCommandWithInputFn
	runWSLCommandWithInputFn = func(context.Context, string, bool, string, string, string, ...string) (string, error) {
		t.Fatal("plain init must not install or migrate a legacy WSL engine")
		return "", nil
	}
	t.Cleanup(func() { runWSLCommandWithInputFn = previousInstall })

	var out bytes.Buffer
	if err := runInit(&out, workspace, "", []string{"local"}, false); err != nil {
		t.Fatalf("runInit: %v", err)
	}
	if !strings.Contains(out.String(), "sqlrs init local --workspace") || !strings.Contains(out.String(), "--update") {
		t.Fatalf("missing exact migration command: %q", out.String())
	}
	got, err := os.ReadFile(configPath)
	if err != nil || !bytes.Equal(got, configData) {
		t.Fatalf("plain init changed legacy config: err=%v\n%s", err, got)
	}
}

func TestRemoveLegacyWSLEngineDaemonDuringExplicitUpdate(t *testing.T) {
	marker := t.TempDir()
	legacy := filepath.Join(marker, "legacy-linux-engine")
	writeTestELF(t, legacy)
	raw := map[string]any{
		"orchestrator": map[string]any{"daemonPath": filepath.Base(legacy)},
		"engine":       map[string]any{"wsl": map[string]any{"mode": "required"}},
	}

	removeLegacyWSLEngineDaemon(raw, filepath.Join(marker, "config.yaml"))
	orchestrator := raw["orchestrator"].(map[string]any)
	if _, exists := orchestrator["daemonPath"]; exists {
		t.Fatal("validated legacy Linux daemonPath was not removed during explicit update")
	}
}

func writeTestELF(t *testing.T, path string) {
	t.Helper()
	header := make([]byte, 64)
	copy(header[:4], []byte("\x7fELF"))
	header[4] = 2
	header[5] = 1
	machine := uint16(62)
	if runtime.GOARCH == "arm64" {
		machine = 183
	}
	binary.LittleEndian.PutUint16(header[18:20], machine)
	if err := os.WriteFile(path, header, 0o755); err != nil {
		t.Fatal(err)
	}
}
