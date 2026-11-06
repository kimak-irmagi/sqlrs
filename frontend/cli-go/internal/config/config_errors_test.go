package config

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/sqlrs/cli/internal/paths"
)

func TestLookupDBMSImageReadError(t *testing.T) {
	dir := t.TempDir()
	if _, _, err := LookupDBMSImage(dir); err == nil {
		t.Fatalf("expected read error")
	}
}

func TestReadConfigMapInvalidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("dbms: ["), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, err := readConfigMap(path); err == nil {
		t.Fatalf("expected unmarshal error")
	}
}

func TestReadConfigMapNonMapExtra(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	if err := os.WriteFile(path, []byte("- item\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	parsed, err := readConfigMap(path)
	if err != nil {
		t.Fatalf("readConfigMap: %v", err)
	}
	if len(parsed) != 0 {
		t.Fatalf("expected empty map, got %+v", parsed)
	}
}

func TestLoadResolvePathsError(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only paths resolution")
	}
	t.Setenv("APPDATA", "")
	t.Setenv("LOCALAPPDATA", "")
	t.Setenv("USERPROFILE", "")
	t.Setenv("HOMEDRIVE", "")
	t.Setenv("HOMEPATH", "")

	if _, err := Load(LoadOptions{WorkingDir: t.TempDir()}); err == nil {
		t.Fatalf("expected resolve paths error")
	}
}

func TestLoadGlobalConfigReadError(t *testing.T) {
	root := t.TempDir()
	dirs := paths.Dirs{
		ConfigDir: filepath.Join(root, "config"),
		StateDir:  filepath.Join(root, "state"),
		CacheDir:  filepath.Join(root, "cache"),
	}
	if err := os.MkdirAll(dirs.ConfigDir, 0o700); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dirs.ConfigDir, "config.yaml"), []byte("dbms: ["), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	if _, err := Load(LoadOptions{WorkingDir: t.TempDir(), Dirs: &dirs}); err == nil || !strings.Contains(err.Error(), "read config") {
		t.Fatalf("expected read config error, got %v", err)
	}
}

func TestLoadProjectConfigReadError(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	projectConfigDir := filepath.Join(project, ".sqlrs")
	if err := os.MkdirAll(projectConfigDir, 0o700); err != nil {
		t.Fatalf("mkdir project config: %v", err)
	}
	if err := os.WriteFile(filepath.Join(projectConfigDir, "config.yaml"), []byte("dbms: ["), 0o600); err != nil {
		t.Fatalf("write project config: %v", err)
	}

	dirs := paths.Dirs{
		ConfigDir: filepath.Join(root, "config"),
		StateDir:  filepath.Join(root, "state"),
		CacheDir:  filepath.Join(root, "cache"),
	}
	if _, err := Load(LoadOptions{WorkingDir: project, Dirs: &dirs}); err == nil || !strings.Contains(err.Error(), "read project config") {
		t.Fatalf("expected project config error, got %v", err)
	}
}

func TestLoadFindProjectConfigError(t *testing.T) {
	prev := findProjectConfigFn
	findProjectConfigFn = func(string) (string, error) {
		return "", errors.New("boom")
	}
	t.Cleanup(func() { findProjectConfigFn = prev })

	dirs := &paths.Dirs{
		ConfigDir: t.TempDir(),
		StateDir:  t.TempDir(),
		CacheDir:  t.TempDir(),
	}
	if _, err := Load(LoadOptions{WorkingDir: t.TempDir(), Dirs: dirs}); err == nil {
		t.Fatalf("expected working dir error")
	}
}
