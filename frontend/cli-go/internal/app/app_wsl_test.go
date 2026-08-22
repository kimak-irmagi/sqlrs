//go:build windows

package app

import (
	"path/filepath"
	"testing"

	"github.com/sqlrs/cli/internal/config"
	"github.com/sqlrs/cli/internal/paths"
)

func TestWindowsToWSLPath(t *testing.T) {
	out, err := windowsToWSLPath(`C:\Users\Zlygo\bin\sqlrs-engine`)
	if err != nil {
		t.Fatalf("windowsToWSLPath: %v", err)
	}
	if out != "/mnt/c/Users/Zlygo/bin/sqlrs-engine" {
		t.Fatalf("unexpected path: %s", out)
	}
}

func TestWindowsToWSLPathRejectsRelative(t *testing.T) {
	if _, err := windowsToWSLPath(`bin\sqlrs-engine`); err == nil {
		t.Fatalf("expected error for relative path")
	}
}

func TestResolveWSLSettingsUsesConfig(t *testing.T) {
	cfg := config.Config{
		Engine: config.EngineConfig{
			WSL: config.EngineWSLConfig{
				Mode:       "auto",
				Distro:     "Ubuntu",
				StateDir:   "/var/lib/sqlrs/store",
				EnginePath: "/home/user/.local/lib/sqlrs/sqlrs-engine",
			},
		},
	}
	dirs := paths.Dirs{StateDir: filepath.Join("C:\\", "sqlrs", "state")}
	daemonPath, runDir, statePath, storeDir, distro, mountDevice, mountFSType, err := resolveWSLSettings(cfg, dirs, filepath.Join("C:\\", "sqlrs", "bin", "sqlrs-engine"))
	if err != nil {
		t.Fatalf("resolveWSLSettings: %v", err)
	}
	if distro != "Ubuntu" {
		t.Fatalf("expected distro Ubuntu, got %q", distro)
	}
	if runDir != "/var/lib/sqlrs/store/run" {
		t.Fatalf("unexpected runDir: %s", runDir)
	}
	if storeDir != "/var/lib/sqlrs/store" {
		t.Fatalf("unexpected storeDir: %s", storeDir)
	}
	if statePath != "/mnt/c/sqlrs/state/engine.json" {
		t.Fatalf("unexpected statePath: %s", statePath)
	}
	if daemonPath != "/home/user/.local/lib/sqlrs/sqlrs-engine" {
		t.Fatalf("unexpected daemonPath: %s", daemonPath)
	}
	if mountDevice != "" || mountFSType != "" {
		t.Fatalf("expected empty mount metadata, got device=%q fstype=%q", mountDevice, mountFSType)
	}
}

func TestResolveWSLSettingsNoMode(t *testing.T) {
	cfg := config.Config{
		Engine: config.EngineConfig{
			WSL: config.EngineWSLConfig{},
		},
	}
	dirs := paths.Dirs{StateDir: filepath.Join("C:\\", "sqlrs", "state")}
	daemonPath, runDir, statePath, storeDir, distro, mountDevice, mountFSType, err := resolveWSLSettings(cfg, dirs, filepath.Join("C:\\", "sqlrs", "bin", "sqlrs-engine"))
	if err != nil {
		t.Fatalf("resolveWSLSettings: %v", err)
	}
	if daemonPath == "" || runDir != "" || statePath != "" || storeDir != "" || distro != "" || mountDevice != "" || mountFSType != "" {
		t.Fatalf("expected empty WSL settings, got runDir=%q statePath=%q storeDir=%q distro=%q device=%q fstype=%q", runDir, statePath, storeDir, distro, mountDevice, mountFSType)
	}
}

func TestResolveWSLSettingsRequiredMissingMountUnit(t *testing.T) {
	cfg := config.Config{
		Engine: config.EngineConfig{
			WSL: config.EngineWSLConfig{
				Mode:     "required",
				Distro:   "Ubuntu",
				StateDir: "/var/lib/sqlrs",
			},
		},
	}
	dirs := paths.Dirs{StateDir: filepath.Join("C:\\", "sqlrs", "state")}
	if _, _, _, _, _, _, _, err := resolveWSLSettings(cfg, dirs, filepath.Join("C:\\", "sqlrs", "bin", "sqlrs-engine")); err == nil {
		t.Fatalf("expected error for missing mount unit")
	}
}

func TestResolveWSLSettingsRequiredMissingStateDir(t *testing.T) {
	cfg := config.Config{
		Engine: config.EngineConfig{
			WSL: config.EngineWSLConfig{
				Mode:   "required",
				Distro: "Ubuntu",
			},
		},
	}
	dirs := paths.Dirs{StateDir: filepath.Join("C:\\", "sqlrs", "state")}
	if _, _, _, _, _, _, _, err := resolveWSLSettings(cfg, dirs, filepath.Join("C:\\", "sqlrs", "bin", "sqlrs-engine")); err == nil {
		t.Fatalf("expected error for missing stateDir")
	}
}

func TestResolveWSLSettingsDefaultsMountFSType(t *testing.T) {
	cfg := config.Config{
		Engine: config.EngineConfig{
			WSL: config.EngineWSLConfig{
				Mode:     "required",
				Distro:   "Ubuntu",
				StateDir: "/var/lib/sqlrs",
				Mount: config.EngineWSLMountConfig{
					Unit: "sqlrs-state-store.mount",
				},
			},
		},
	}
	dirs := paths.Dirs{StateDir: filepath.Join("C:\\", "sqlrs", "state")}
	_, _, _, _, _, _, mountFSType, err := resolveWSLSettings(cfg, dirs, filepath.Join("C:\\", "sqlrs", "bin", "sqlrs-engine"))
	if err != nil {
		t.Fatalf("resolveWSLSettings: %v", err)
	}
	if mountFSType != "btrfs" {
		t.Fatalf("expected mount fstype btrfs, got %q", mountFSType)
	}
}

func TestWindowsToWSLPathEmpty(t *testing.T) {
	if _, err := windowsToWSLPath("  "); err == nil {
		t.Fatalf("expected error for empty path")
	}
}
