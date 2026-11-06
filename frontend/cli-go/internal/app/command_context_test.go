package app

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sqlrs/cli/internal/cli"
	"github.com/sqlrs/cli/internal/paths"
	"github.com/sqlrs/cli/internal/pathutil"
)

func TestResolveCommandContextDefaultsFromProjectConfig(t *testing.T) {
	temp := t.TempDir()
	setTestDirs(t, temp)

	dirs, err := paths.Resolve()
	if err != nil {
		t.Fatalf("resolve dirs: %v", err)
	}
	if err := os.MkdirAll(dirs.ConfigDir, 0o700); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}

	workspace := filepath.Join(temp, "workspace")
	projectDir := filepath.Join(workspace, ".sqlrs")
	if err := os.MkdirAll(projectDir, 0o700); err != nil {
		t.Fatalf("mkdir project dir: %v", err)
	}
	projectConfig := []byte(
		"defaultProfile: remote\n" +
			"client:\n" +
			"  timeout: 45s\n" +
			"  output: json\n" +
			"orchestrator:\n" +
			"  startupTimeout: 7s\n" +
			"  idleTimeout: 9s\n" +
			"  daemonPath: ./bin/sqlrs-engine\n" +
			"profiles:\n" +
			"  remote:\n" +
			"    mode: remote\n" +
			"    endpoint: http://project.example\n" +
			"    auth:\n" +
			"      tokenEnv: SQLRS_AUTH_TOKEN\n",
	)
	if err := os.WriteFile(filepath.Join(projectDir, "config.yaml"), projectConfig, 0o600); err != nil {
		t.Fatalf("write project config: %v", err)
	}
	t.Setenv("SQLRS_AUTH_TOKEN", "env-token")

	ctx, err := resolveCommandContext(temp, cli.GlobalOptions{Workspace: "workspace"})
	if err != nil {
		t.Fatalf("resolveCommandContext: %v", err)
	}

	if !pathutil.SameLocalPath(workspace, ctx.workspaceRoot) {
		t.Fatalf("workspaceRoot = %q, want %q", ctx.workspaceRoot, workspace)
	}
	if ctx.profileName != "remote" {
		t.Fatalf("profileName = %q, want remote", ctx.profileName)
	}
	if ctx.mode != "remote" {
		t.Fatalf("mode = %q, want remote", ctx.mode)
	}
	if ctx.output != "json" {
		t.Fatalf("output = %q, want json", ctx.output)
	}
	if ctx.timeout != 45*time.Second {
		t.Fatalf("timeout = %s, want 45s", ctx.timeout)
	}
	if ctx.startupTimeout != 7*time.Second {
		t.Fatalf("startupTimeout = %s, want 7s", ctx.startupTimeout)
	}
	if ctx.idleTimeout != 9*time.Second {
		t.Fatalf("idleTimeout = %s, want 9s", ctx.idleTimeout)
	}
	if ctx.authToken != "env-token" {
		t.Fatalf("authToken = %q, want env-token", ctx.authToken)
	}
	wantDaemon := filepath.Join(projectDir, "bin", "sqlrs-engine")
	if !pathutil.SameLocalPath(wantDaemon, ctx.daemonPath) {
		t.Fatalf("daemonPath = %q, want %q", ctx.daemonPath, wantDaemon)
	}
	wantRunDir := filepath.Join(dirs.StateDir, "run")
	if !pathutil.SameLocalPath(wantRunDir, ctx.runDir) {
		t.Fatalf("runDir = %q, want %q", ctx.runDir, wantRunDir)
	}
}

func TestResolveCommandContextAppliesCLIOverrides(t *testing.T) {
	temp := t.TempDir()
	setTestDirs(t, temp)

	dirs, err := paths.Resolve()
	if err != nil {
		t.Fatalf("resolve dirs: %v", err)
	}
	if err := os.MkdirAll(dirs.ConfigDir, 0o700); err != nil {
		t.Fatalf("mkdir config dir: %v", err)
	}
	globalConfig := []byte(
		"defaultProfile: remote\n" +
			"client:\n" +
			"  timeout: 1m\n" +
			"  output: human\n" +
			"orchestrator:\n" +
			"  startupTimeout: 11s\n" +
			"  idleTimeout: 13s\n" +
			"  daemonPath: /opt/sqlrs-engine\n" +
			"  runDir: /var/sqlrs/run\n" +
			"engine:\n" +
			"  storePath: /var/sqlrs/store\n" +
			"profiles:\n" +
			"  remote:\n" +
			"    mode: remote\n" +
			"    endpoint: http://config.example\n" +
			"    autostart: true\n" +
			"    auth:\n" +
			"      token: config-token\n",
	)
	if err := os.WriteFile(filepath.Join(dirs.ConfigDir, "config.yaml"), globalConfig, 0o600); err != nil {
		t.Fatalf("write global config: %v", err)
	}
	t.Setenv("SQLRS_DAEMON_PATH", "/env/sqlrs-engine")

	ctx, err := resolveCommandContext(temp, cli.GlobalOptions{
		Profile:  "remote",
		Mode:     "local",
		Endpoint: "http://cli.example",
		Output:   "json",
		Timeout:  5 * time.Second,
		Verbose:  true,
	})
	if err != nil {
		t.Fatalf("resolveCommandContext: %v", err)
	}

	if ctx.profile.Endpoint != "http://cli.example" {
		t.Fatalf("endpoint = %q, want http://cli.example", ctx.profile.Endpoint)
	}
	if ctx.mode != "local" {
		t.Fatalf("mode = %q, want local", ctx.mode)
	}
	if ctx.output != "json" {
		t.Fatalf("output = %q, want json", ctx.output)
	}
	if ctx.timeout != 5*time.Second {
		t.Fatalf("timeout = %s, want 5s", ctx.timeout)
	}
	if ctx.daemonPath != "/env/sqlrs-engine" {
		t.Fatalf("daemonPath = %q, want /env/sqlrs-engine", ctx.daemonPath)
	}
	// Config strings keep YAML slashes; host OS normalizes separators (and may change drive case).
	if !pathutil.SameLocalPath("/var/sqlrs/run", ctx.runDir) {
		t.Fatalf("runDir = %q, want equivalent of /var/sqlrs/run", ctx.runDir)
	}
	wantEngineStoreDir := "/var/sqlrs/store"
	if ctx.engineStoreDir != wantEngineStoreDir {
		t.Fatalf("engineStoreDir = %q, want %q", ctx.engineStoreDir, wantEngineStoreDir)
	}
	if !pathutil.SameLocalPath("/var/sqlrs/store", ctx.engineHostStorePath) {
		t.Fatalf("engineHostStorePath = %q, want equivalent of /var/sqlrs/store", ctx.engineHostStorePath)
	}
	if ctx.authToken != "config-token" {
		t.Fatalf("authToken = %q, want config-token", ctx.authToken)
	}
	if !ctx.verbose {
		t.Fatalf("expected verbose=true")
	}
}

func TestResolveCommandContextUsesWorkspaceFlagAsFallbackRoot(t *testing.T) {
	temp := t.TempDir()
	setTestDirs(t, temp)

	workspace := filepath.Join(temp, "workspace")
	if err := os.MkdirAll(workspace, 0o700); err != nil {
		t.Fatalf("mkdir workspace: %v", err)
	}

	ctx, err := resolveCommandContext(temp, cli.GlobalOptions{Workspace: "workspace"})
	if err != nil {
		t.Fatalf("resolveCommandContext: %v", err)
	}

	if !pathutil.SameLocalPath(workspace, ctx.workspaceRoot) {
		t.Fatalf("workspaceRoot = %q, want %q", ctx.workspaceRoot, workspace)
	}
}
