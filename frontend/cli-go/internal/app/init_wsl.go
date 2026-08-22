package app

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"

	"github.com/sqlrs/cli/internal/wsl"
)

type wslInitOptions struct {
	Enable           bool
	Distro           string
	Require          bool
	NoStart          bool
	Workspace        string
	Verbose          bool
	StoreSizeGB      int
	Reinit           bool
	StorePath        string
	EngineSourcePath string
}

type wslInitResult struct {
	UseWSL          bool
	Distro          string
	StateDir        string
	EnginePath      string
	StorePath       string
	MountDevice     string
	MountFSType     string
	MountUnit       string
	MountDeviceUUID string
	Warning         string
}

type wslInitDeps struct {
	lookPath       func(string) (string, error)
	listWSLDistros func() ([]wsl.Distro, error)
	isElevated     func(bool) (bool, error)
}

type wslBootstrapPhase struct {
	Distro   string
	Warnings []string
}

type wslStoragePhase struct {
	Distro    string
	StateDir  string
	StorePath string
	MountUnit string
	Partition string
}

type wslMountPhase struct {
	DeviceUUID string
	Warnings   []string
}

var initWSLFn = initWSL
var listWSLDistrosFn = listWSLDistros
var runWSLCommandFn = runWSLCommand
var runWSLCommandAllowFailureFn = runWSLCommandAllowFailure
var runWSLCommandWithInputFn = runWSLCommandWithInput
var runHostCommandFn = runHostCommand
var isElevatedFn = isElevated
var isTerminalWriterFn = isTerminalWriter
var isWindows = runtime.GOOS == "windows"

const defaultVHDXName = "btrfs.vhdx"
const defaultInstalledWSLEnginePath = "/home/user/.local/lib/sqlrs/sqlrs-engine"

func defaultWSLInitDeps() wslInitDeps {
	return wslInitDeps{
		lookPath:       exec.LookPath,
		listWSLDistros: listWSLDistrosFn,
		isElevated:     isElevatedFn,
	}
}

func initWSL(opts wslInitOptions) (wslInitResult, error) {
	if !opts.Enable {
		return wslInitResult{}, nil
	}
	if !isWindows {
		return wslInitResult{}, fmt.Errorf("WSL init is only supported on Windows")
	}

	deps := defaultWSLInitDeps()

	bootstrap, err := bootstrapWSLInit(context.Background(), deps, opts)
	if err != nil {
		return wslUnavailable(opts, err.Error())
	}

	enginePath := ""
	if strings.TrimSpace(opts.EngineSourcePath) != "" {
		enginePath, err = installWSLEngine(context.Background(), bootstrap.Distro, opts.EngineSourcePath, "", opts.Verbose)
		if err != nil {
			return wslUnavailable(opts, err.Error())
		}
	}

	storage, err := prepareWSLStorage(context.Background(), deps, opts, bootstrap.Distro)
	if err != nil {
		return wslUnavailable(opts, err.Error())
	}

	mount, err := finalizeWSLMount(context.Background(), deps, opts, storage)
	if err != nil {
		return wslUnavailable(opts, err.Error())
	}

	warnings := append([]string{}, bootstrap.Warnings...)
	warnings = append(warnings, mount.Warnings...)
	warnings = append(warnings, "WSL restart required: wsl.exe --shutdown")

	return wslInitResult{
		UseWSL:          true,
		Distro:          bootstrap.Distro,
		EnginePath:      enginePath,
		StateDir:        storage.StateDir,
		StorePath:       storage.StorePath,
		MountDevice:     storage.Partition,
		MountFSType:     "btrfs",
		MountUnit:       storage.MountUnit,
		MountDeviceUUID: mount.DeviceUUID,
		Warning:         strings.Join(warnings, "\n"),
	}, nil
}

func installWSLEngine(ctx context.Context, distro, sourcePath, destination string, verbose bool) (string, error) {
	payload, err := os.ReadFile(sourcePath)
	if err != nil {
		return "", fmt.Errorf("read WSL engine payload: %w", err)
	}
	const script = `set -eu
dest="$1"
if [ -z "$dest" ]; then dest="$HOME/.local/lib/sqlrs/sqlrs-engine"; fi
dir="${dest%/*}"
mkdir -p "$dir"
tmp="$(mktemp "$dir/.sqlrs-engine.XXXXXX")"
trap 'rm -f "$tmp"' EXIT
cat >"$tmp"
chmod 755 "$tmp"
mv -f "$tmp" "$dest"
trap - EXIT
printf '%s\n' "$dest"`
	out, err := runWSLCommandWithInputFn(ctx, distro, verbose, "install WSL engine", string(payload), "sh", "-c", script, "sqlrs-engine-installer", strings.TrimSpace(destination))
	if err != nil {
		return "", fmt.Errorf("install WSL engine: %w", err)
	}
	installed := strings.TrimSpace(out)
	if installed == "" || !strings.HasPrefix(installed, "/") {
		return "", fmt.Errorf("install WSL engine: installer returned invalid path %q", installed)
	}
	return installed, nil
}

func wslUnavailable(opts wslInitOptions, warning string) (wslInitResult, error) {
	if opts.Require {
		return wslInitResult{}, errors.New(warning)
	}
	return wslInitResult{UseWSL: false, Warning: strings.TrimSpace(warning)}, nil
}

func sanitizeWSLOutput(data []byte) string {
	trimmed := strings.TrimSpace(string(data))
	if strings.Contains(trimmed, "\x00") {
		trimmed = strings.ReplaceAll(trimmed, "\x00", "")
		trimmed = strings.TrimSpace(trimmed)
	}
	return trimmed
}

func escapePowerShellString(value string) string {
	return strings.ReplaceAll(value, "'", "''")
}

func isVHDXInUseError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "objectinuse") ||
		strings.Contains(msg, "in use by another process") ||
		strings.Contains(msg, "0x80070020")
}

func isWSLNotMountedError(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "not mounted") || strings.Contains(msg, "exit status 32")
}
