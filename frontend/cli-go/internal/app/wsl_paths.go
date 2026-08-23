package app

import (
	"fmt"
	"path"
	"path/filepath"
	"strings"

	"github.com/sqlrs/cli/internal/config"
	"github.com/sqlrs/cli/internal/paths"
	"github.com/sqlrs/cli/internal/wsl"
)

func resolveWSLSettings(cfg config.Config, dirs paths.Dirs, daemonPath string) (string, string, string, string, string, string, string, error) {
	mode := strings.ToLower(strings.TrimSpace(cfg.Engine.WSL.Mode))
	if mode == "" {
		return daemonPath, "", "", "", "", "", "", nil
	}
	if mode != "auto" && mode != "required" {
		return daemonPath, "", "", "", "", "", "", nil
	}

	stateDir := strings.TrimSpace(cfg.Engine.WSL.StateDir)
	distro := strings.TrimSpace(cfg.Engine.WSL.Distro)
	mountUnit := strings.TrimSpace(cfg.Engine.WSL.Mount.Unit)
	mountFSType := strings.TrimSpace(cfg.Engine.WSL.Mount.FSType)
	if distro == "" {
		distros, err := listWSLDistrosFn()
		if err != nil {
			if mode == "required" {
				return "", "", "", "", "", "", "", fmt.Errorf("WSL unavailable: %v", err)
			}
			return daemonPath, "", "", "", "", "", "", nil
		}
		distro, err = wsl.SelectDistro(distros, "")
		if err != nil {
			if mode == "required" {
				return "", "", "", "", "", "", "", fmt.Errorf("WSL distro resolution failed: %v", err)
			}
			return daemonPath, "", "", "", "", "", "", nil
		}
	}
	if distro == "" || stateDir == "" {
		if mode == "required" {
			return "", "", "", "", "", "", "", fmt.Errorf("WSL configuration is missing distro or stateDir")
		}
		return daemonPath, "", "", "", "", "", "", nil
	}
	if mountUnit == "" && mode == "required" {
		return "", "", "", "", "", "", "", fmt.Errorf("WSL configuration is missing mount unit (run sqlrs init local --snapshot btrfs)")
	}
	if mountFSType == "" && mountUnit != "" {
		mountFSType = "btrfs"
	}

	engineBinary := daemonPath
	if cfg.Engine.WSL.EnginePath != "" {
		engineBinary = strings.TrimSpace(cfg.Engine.WSL.EnginePath)
	}
	wslDaemonPath := engineBinary
	if !strings.HasPrefix(wslDaemonPath, "/") {
		var err error
		wslDaemonPath, err = windowsToWSLPath(engineBinary)
		if err != nil {
			if mode == "required" {
				return "", "", "", "", "", "", "", err
			}
			return daemonPath, "", "", "", "", "", "", nil
		}
	}

	statePath := filepath.Join(dirs.StateDir, "engine.json")
	wslStatePath, err := windowsToWSLPath(statePath)
	if err != nil {
		if mode == "required" {
			return "", "", "", "", "", "", "", err
		}
		return daemonPath, "", "", "", "", "", "", nil
	}

	runDir := path.Join(stateDir, "run")
	return wslDaemonPath, runDir, wslStatePath, stateDir, distro, mountUnit, mountFSType, nil
}

func windowsToWSLPath(value string) (string, error) {
	cleaned := strings.TrimSpace(value)
	if cleaned == "" {
		return "", fmt.Errorf("path is empty")
	}
	if strings.HasPrefix(cleaned, "/") {
		return cleaned, nil
	}
	drive := ""
	rest := ""
	if len(cleaned) >= 2 && cleaned[1] == ':' {
		letter := cleaned[0]
		if (letter >= 'A' && letter <= 'Z') || (letter >= 'a' && letter <= 'z') {
			drive = strings.ToLower(cleaned[:1])
			rest = cleaned[2:]
		}
	}
	if drive == "" {
		vol := filepath.VolumeName(cleaned)
		if vol == "" {
			return "", fmt.Errorf("path is not absolute: %s", cleaned)
		}
		drive = strings.TrimSuffix(strings.ToLower(vol), ":")
		rest = cleaned[len(vol):]
	}
	rest = strings.TrimLeft(rest, `\\/`)
	rest = strings.ReplaceAll(rest, "\\", "/")
	if rest == "" {
		return fmt.Sprintf("/mnt/%s", drive), nil
	}
	return fmt.Sprintf("/mnt/%s/%s", drive, rest), nil
}
