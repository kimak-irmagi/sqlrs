package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/sqlrs/cli/internal/paths"
)

func TestParseDuration(t *testing.T) {
	fallback := 5 * time.Second
	out, err := ParseDuration("", fallback)
	if err != nil {
		t.Fatalf("ParseDuration: %v", err)
	}
	if out != fallback {
		t.Fatalf("expected fallback, got %v", out)
	}
	if _, err := ParseDuration("bad", fallback); err == nil {
		t.Fatalf("expected error for invalid duration")
	}
}

func TestAuthConfigLoadsOIDCSessionFields(t *testing.T) {
	dir := t.TempDir()
	dirs := paths.Dirs{
		ConfigDir: filepath.Join(dir, "config"),
		StateDir:  filepath.Join(dir, "state"),
		CacheDir:  filepath.Join(dir, "cache"),
	}
	if err := os.MkdirAll(dirs.ConfigDir, 0o700); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	raw := []byte(
		"defaultProfile: remote\n" +
			"profiles:\n" +
			"  remote:\n" +
			"    mode: remote\n" +
			"    endpoint: https://sqlrs.example.org\n" +
			"    auth:\n" +
			"      mode: oidcSession\n" +
			"      clientID: client-id\n" +
			"      clientSecret: client-secret\n" +
			"      issuer: https://accounts.google.com\n" +
			"      tokenEnv: SQLRS_TOKEN\n")
	if err := os.WriteFile(filepath.Join(dirs.ConfigDir, "config.yaml"), raw, 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	loaded, err := Load(LoadOptions{WorkingDir: dir, Dirs: &dirs})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	auth := loaded.Config.Profiles["remote"].Auth
	if auth.Mode != "oidcSession" || auth.ClientID != "client-id" || auth.ClientSecret != "client-secret" || auth.Issuer != "https://accounts.google.com" || auth.TokenEnv != "SQLRS_TOKEN" {
		t.Fatalf("auth config = %+v, want oidc session fields", auth)
	}
}

func TestLookupDBMSImage(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("dbms:\n  image: postgres:15\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	image, ok, err := LookupDBMSImage(path)
	if err != nil {
		t.Fatalf("LookupDBMSImage: %v", err)
	}
	if !ok || image != "postgres:15" {
		t.Fatalf("unexpected image: %q (ok=%v)", image, ok)
	}
}

func TestLookupDBMSImageMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("dbms:\n  image: \"\"\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_, ok, err := LookupDBMSImage(path)
	if err != nil {
		t.Fatalf("LookupDBMSImage: %v", err)
	}
	if ok {
		t.Fatalf("expected no image")
	}
}

func TestLookupDBMSImageInvalidType(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("dbms:\n  image: 123\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_, ok, err := LookupDBMSImage(path)
	if err != nil {
		t.Fatalf("LookupDBMSImage: %v", err)
	}
	if ok {
		t.Fatalf("expected invalid type to return ok=false")
	}
}

func TestLookupDBMSImageWhitespace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("dbms:\n  image: \"   \"\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_, ok, err := LookupDBMSImage(path)
	if err != nil {
		t.Fatalf("LookupDBMSImage: %v", err)
	}
	if ok {
		t.Fatalf("expected whitespace image to return ok=false")
	}
}

func TestLookupDBMSImageNoSectionOrField(t *testing.T) {
	dir := t.TempDir()

	noSection := filepath.Join(dir, "no-section.yaml")
	if err := os.WriteFile(noSection, []byte("client:\n  timeout: 10s\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, ok, err := LookupDBMSImage(noSection); err != nil || ok {
		t.Fatalf("expected missing dbms section to return ok=false, ok=%v err=%v", ok, err)
	}

	noField := filepath.Join(dir, "no-field.yaml")
	if err := os.WriteFile(noField, []byte("dbms:\n  mode: local\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, ok, err := LookupDBMSImage(noField); err != nil || ok {
		t.Fatalf("expected missing dbms.image to return ok=false, ok=%v err=%v", ok, err)
	}
}

func TestNormalizeMap(t *testing.T) {
	input := map[any]any{
		"key": map[any]any{
			"nested": []any{"a", map[any]any{"b": "c"}},
		},
		10: "ignore",
	}
	out := normalizeMap(input).(map[string]any)
	if _, ok := out["key"]; !ok {
		t.Fatalf("expected key to be preserved")
	}
	if _, ok := out["10"]; ok {
		t.Fatalf("expected non-string key to be dropped")
	}
	nested := out["key"].(map[string]any)
	list := nested["nested"].([]any)
	if list[0].(string) != "a" {
		t.Fatalf("unexpected list: %+v", list)
	}
	inner := list[1].(map[string]any)
	if inner["b"].(string) != "c" {
		t.Fatalf("unexpected inner map: %+v", inner)
	}
}

func TestLoadConfigMergesAndResolvesPaths(t *testing.T) {
	root := t.TempDir()
	dirs := paths.Dirs{
		ConfigDir: filepath.Join(root, "config"),
		StateDir:  filepath.Join(root, "state"),
		CacheDir:  filepath.Join(root, "cache"),
	}
	if err := os.MkdirAll(dirs.ConfigDir, 0o700); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}

	globalConfig := []byte("orchestrator:\n  daemonPath: bin/sqlrs-engine\n")
	if err := os.WriteFile(filepath.Join(dirs.ConfigDir, "config.yaml"), globalConfig, 0o600); err != nil {
		t.Fatalf("write global config: %v", err)
	}

	projectDir := filepath.Join(root, "project")
	projectConfigDir := filepath.Join(projectDir, ".sqlrs")
	if err := os.MkdirAll(projectConfigDir, 0o700); err != nil {
		t.Fatalf("mkdir project config: %v", err)
	}
	projectConfig := []byte("orchestrator:\n  runDir: ./run/..\nprofiles: null\n")
	if err := os.WriteFile(filepath.Join(projectConfigDir, "config.yaml"), projectConfig, 0o600); err != nil {
		t.Fatalf("write project config: %v", err)
	}

	loaded, err := Load(LoadOptions{WorkingDir: projectDir, Dirs: &dirs})
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.ProjectConfigPath == "" {
		t.Fatalf("expected project config path")
	}
	if loaded.Config.Orchestrator.RunDir == "" {
		t.Fatalf("expected runDir to be set")
	}
	if !filepath.IsAbs(loaded.Config.Orchestrator.DaemonPath) {
		t.Fatalf("expected daemonPath to be absolute, got %q", loaded.Config.Orchestrator.DaemonPath)
	}
	if loaded.Config.Profiles == nil {
		t.Fatalf("expected profiles map to be initialized")
	}
}
