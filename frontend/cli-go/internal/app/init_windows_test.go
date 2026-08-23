//go:build windows

package app

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestResolveStoreTypeAutoWindows(t *testing.T) {
	if got, err := resolveStoreType("auto", ""); err != nil || got != "image" {
		t.Fatalf("expected auto -> image, got %q (err=%v)", got, err)
	}
	if got, err := resolveStoreType("btrfs", ""); err != nil || got != "image" {
		t.Fatalf("expected btrfs -> image, got %q (err=%v)", got, err)
	}
	if got, err := resolveStoreType("overlay", ""); err != nil || got != "dir" {
		t.Fatalf("expected overlay -> dir, got %q (err=%v)", got, err)
	}
}

func TestShouldUseWSLWindows(t *testing.T) {
	if use, require := shouldUseWSL("btrfs", "dir", false); use || !require {
		t.Fatalf("expected btrfs+dir -> use=false require=true, got use=%v require=%v", use, require)
	}
	if use, require := shouldUseWSL("btrfs", "image", false); !use || !require {
		t.Fatalf("expected btrfs+image -> use=true require=true, got use=%v require=%v", use, require)
	}
	if use, require := shouldUseWSL("auto", "dir", false); use || require {
		t.Fatalf("expected auto+dir -> use=false require=false, got use=%v require=%v", use, require)
	}
	if use, require := shouldUseWSL("auto", "image", false); !use || require {
		t.Fatalf("expected auto+image (implicit) -> use=true require=false, got use=%v require=%v", use, require)
	}
	if use, require := shouldUseWSL("auto", "image", true); !use || !require {
		t.Fatalf("expected auto+image (explicit) -> use=true require=true, got use=%v require=%v", use, require)
	}
	if use, require := shouldUseWSL("copy", "dir", false); use || require {
		t.Fatalf("expected copy+dir -> use=false require=false, got use=%v require=%v", use, require)
	}
}

func TestInitAutoWSLFallbackWritesStorePath(t *testing.T) {
	workspace := t.TempDir()
	storeRoot := filepath.Join(workspace, "store")
	t.Setenv("SQLRS_STATE_STORE", storeRoot)

	withInitWSLStub(t, func(opts wslInitOptions) (wslInitResult, error) {
		return wslInitResult{UseWSL: false, Warning: "WSL unavailable"}, nil
	})

	var out bytes.Buffer
	if err := runInit(&out, workspace, "", []string{"local", "--snapshot", "auto"}, false); err != nil {
		t.Fatalf("runInit: %v", err)
	}

	configPath := filepath.Join(workspace, ".sqlrs", "config.yaml")
	raw := loadConfigMap(t, configPath)
	if got := nestedString(raw, "engine", "storePath"); got != storeRoot {
		t.Fatalf("expected storePath %q, got %q", storeRoot, got)
	}
	if got := nestedString(raw, "snapshot", "backend"); got != "auto" {
		t.Fatalf("expected snapshot.backend auto, got %q", got)
	}
}

func TestInitRejectsWindowsEngineBinaryForWSL(t *testing.T) {
	workspace := t.TempDir()
	enginePath := filepath.Join(workspace, "sqlrs-engine.exe")
	if err := os.WriteFile(enginePath, []byte{'M', 'Z', 0x00, 0x00}, 0o600); err != nil {
		t.Fatalf("write engine: %v", err)
	}

	var out bytes.Buffer
	err := runInit(&out, workspace, "", []string{
		"local",
		"--snapshot", "btrfs",
		"--wsl-engine", enginePath,
	}, false)
	var exitErr *ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("expected ExitError, got %v", err)
	}
	if exitErr.Code != 64 {
		t.Fatalf("expected exit code 64, got %d", exitErr.Code)
	}
	if !strings.Contains(err.Error(), "Invalid WSL engine") {
		t.Fatalf("unexpected error message: %q", err.Error())
	}
}

func TestInitAcceptsLinuxEngineBinaryForWSL(t *testing.T) {
	workspace := t.TempDir()
	enginePath := filepath.Join(workspace, "sqlrs-engine")
	if err := os.WriteFile(enginePath, []byte{0x7f, 'E', 'L', 'F'}, 0o600); err != nil {
		t.Fatalf("write engine: %v", err)
	}

	withInitWSLStub(t, func(opts wslInitOptions) (wslInitResult, error) {
		return wslInitResult{UseWSL: true, Distro: "Ubuntu"}, nil
	})

	var out bytes.Buffer
	if err := runInit(&out, workspace, "", []string{
		"local",
		"--snapshot", "btrfs",
		"--wsl-engine", enginePath,
	}, false); err != nil {
		t.Fatalf("runInit: %v", err)
	}
}
