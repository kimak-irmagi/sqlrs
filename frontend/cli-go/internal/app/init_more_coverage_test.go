package app

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestParseInitFlagsAdditionalValidationCoverage(t *testing.T) {
	t.Run("reject positional args", func(t *testing.T) {
		if _, _, err := parseInitFlags([]string{"local", "extra"}, ""); err == nil {
			t.Fatalf("expected positional args error")
		}
	})

	t.Run("reject invalid store size", func(t *testing.T) {
		if _, _, err := parseInitFlags([]string{"local", "--store-size", "badGB"}, ""); err == nil {
			t.Fatalf("expected store-size parse error")
		}
	})

	t.Run("reject remote url token in local mode", func(t *testing.T) {
		if _, _, err := parseInitFlags([]string{"local", "--url", "https://e", "--token", "t"}, ""); err == nil {
			t.Fatalf("expected local/remote flags mismatch error")
		}
	})

	t.Run("reject unknown snapshot", func(t *testing.T) {
		if _, _, err := parseInitFlags([]string{"local", "--snapshot", "unknown"}, ""); err == nil {
			t.Fatalf("expected unknown snapshot error")
		}
	})

	t.Run("reject unknown store type", func(t *testing.T) {
		if _, _, err := parseInitFlags([]string{"local", "--store-type", "unknown"}, ""); err == nil {
			t.Fatalf("expected unknown store type error")
		}
	})

	t.Run("reject reinit with overlay", func(t *testing.T) {
		if _, _, err := parseInitFlags([]string{"local", "--snapshot", "overlay", "--reinit"}, ""); err == nil {
			t.Fatalf("expected reinit overlay error")
		}
	})

	if runtime.GOOS == "windows" {
		t.Run("reject btrfs with dir store on windows", func(t *testing.T) {
			if _, _, err := parseInitFlags([]string{"local", "--snapshot", "btrfs", "--store-type", "dir"}, ""); err == nil {
				t.Fatalf("expected windows btrfs dir error")
			}
		})
	}
}

func TestRunInitUpdateDryRunWithValidConfigCoverage(t *testing.T) {
	workspace := t.TempDir()
	marker := filepath.Join(workspace, ".sqlrs")
	if err := os.MkdirAll(marker, 0o700); err != nil {
		t.Fatalf("mkdir marker: %v", err)
	}
	if err := os.WriteFile(filepath.Join(marker, "config.yaml"), []byte("client:\n  output: human\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var out bytes.Buffer
	if err := runInit(&out, workspace, "", []string{"local", "--update", "--dry-run"}, false); err != nil {
		t.Fatalf("runInit: %v", err)
	}
	if !strings.Contains(out.String(), "Workspace already initialized") || !strings.Contains(out.String(), "dry-run") {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestRunInitWSLAutoModeBranchCoverage(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only path")
	}

	withInitWSLStub(t, func(opts wslInitOptions) (wslInitResult, error) {
		return wslInitResult{
			UseWSL:      true,
			Distro:      "Ubuntu",
			StateDir:    "/mnt/sqlrs/store",
			EnginePath:  "/home/user/.local/lib/sqlrs/sqlrs-engine",
			StorePath:   "C:\\sqlrs\\store\\btrfs.vhdx",
			MountUnit:   "sqlrs.mount",
			MountFSType: "btrfs",
		}, nil
	})

	workspace := t.TempDir()
	var out bytes.Buffer
	if err := runInit(&out, workspace, "", []string{"local"}, false); err != nil {
		t.Fatalf("runInit: %v", err)
	}

	raw := loadConfigMap(t, filepath.Join(workspace, ".sqlrs", "config.yaml"))
	if got := nestedString(raw, "engine", "wsl", "mode"); got != "auto" {
		t.Fatalf("expected engine.wsl.mode=auto, got %q", got)
	}
}

func TestRunInitUpdateRejectsScalarConfigMap(t *testing.T) {
	workspace := t.TempDir()
	marker := filepath.Join(workspace, ".sqlrs")
	if err := os.MkdirAll(marker, 0o700); err != nil {
		t.Fatalf("mkdir marker: %v", err)
	}
	configPath := filepath.Join(marker, "config.yaml")
	if err := os.WriteFile(configPath, []byte("42\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	var out bytes.Buffer
	err := runInit(&out, workspace, "", []string{"local", "--update", "--engine", filepath.Join(workspace, "bin", "sqlrs-engine")}, false)
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 4 {
		t.Fatalf("expected ExitError code 4, got %v", err)
	}
	if !strings.Contains(err.Error(), "Cannot read config.yaml") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRunInitRejectsMarkerFile(t *testing.T) {
	workspace := t.TempDir()
	marker := filepath.Join(workspace, ".sqlrs")
	if err := os.WriteFile(marker, []byte("not a directory"), 0o600); err != nil {
		t.Fatalf("write marker file: %v", err)
	}

	var out bytes.Buffer
	err := runInit(&out, workspace, "", []string{"local"}, false)
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 4 {
		t.Fatalf("expected ExitError code 4, got %v", err)
	}
}

func TestResolveWorkspacePathRelativeDirectory(t *testing.T) {
	cwd := t.TempDir()
	child := filepath.Join(cwd, "child")
	if err := os.MkdirAll(child, 0o700); err != nil {
		t.Fatalf("mkdir child: %v", err)
	}

	got, err := resolveWorkspacePath("child", cwd)
	if err != nil {
		t.Fatalf("resolveWorkspacePath: %v", err)
	}
	if got != child {
		t.Fatalf("expected %q, got %q", child, got)
	}
}

func TestNormalizeEnginePathPreservesWorkspaceSpellingForCanonicalizedCWD(t *testing.T) {
	temp := t.TempDir()
	realRoot := filepath.Join(temp, "real")
	linkRoot := filepath.Join(temp, "link")
	if err := os.MkdirAll(realRoot, 0o700); err != nil {
		t.Fatalf("mkdir real root: %v", err)
	}
	if err := os.Symlink(realRoot, linkRoot); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	got := normalizeEnginePath(filepath.Join("bin", "sqlrs-engine"), realRoot, linkRoot)
	want, err := filepath.Rel(filepath.Join(linkRoot, ".sqlrs"), filepath.Join(linkRoot, "bin", "sqlrs-engine"))
	if err != nil {
		t.Fatalf("rel path: %v", err)
	}
	if got != want {
		t.Fatalf("expected %q, got %q", want, got)
	}
}

func TestBuildWorkspaceConfigWSLDefaultsAndTrailingNewline(t *testing.T) {
	data, err := buildWorkspaceConfig(initOptions{Mode: "local"}, &wslInitResult{
		Distro:      "Ubuntu",
		StateDir:    "/state/sqlrs",
		StorePath:   filepath.Join("C:\\", "sqlrs", "store", "btrfs.vhdx"),
		MountDevice: "/dev/sda1",
		MountFSType: "btrfs",
		MountUnit:   "sqlrs.mount",
	}, map[string]any{
		"client": map[string]any{
			"output": "human",
		},
	})
	if err != nil {
		t.Fatalf("buildWorkspaceConfig: %v", err)
	}
	if len(data) == 0 || data[len(data)-1] != '\n' {
		t.Fatalf("expected trailing newline, got %q", string(data))
	}

	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if got := nestedString(raw, "engine", "wsl", "mode"); got != "auto" {
		t.Fatalf("expected default engine.wsl.mode=auto, got %q", got)
	}
	if got := nestedString(raw, "engine", "wsl", "distro"); got != "Ubuntu" {
		t.Fatalf("expected engine.wsl.distro=Ubuntu, got %q", got)
	}
}
