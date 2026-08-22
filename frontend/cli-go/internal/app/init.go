package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/sqlrs/cli/internal/cli"
	"github.com/sqlrs/cli/internal/config"
	"github.com/sqlrs/cli/internal/enginebin"
	"github.com/sqlrs/cli/internal/paths"
	"github.com/sqlrs/cli/internal/util"
)

type initOptions struct {
	Workspace     string
	Force         bool
	Update        bool
	EnginePath    string
	WSLEnginePath string
	SharedCache   bool
	DryRun        bool
	Snapshot      string
	StoreType     string
	StorePath     string
	StoreSizeGB   int
	Reinit        bool
	Distro        string
	NoStart       bool
	WSLMode       string
	RemoteURL     string
	RemoteToken   string
	Mode          string
	Verbose       bool
}

type localBtrfsInitOptions struct {
	StoreType   string
	StorePath   string
	StoreSizeGB int
	Reinit      bool
	Verbose     bool
}

type localBtrfsInitResult struct {
	StorePath string
}

const defaultBtrfsStoreSizeGB = 100

var initLocalBtrfsStoreFn = initLocalBtrfsStore

var resolveHostEngineFn = func(req enginebin.Request) (enginebin.Resolved, error) {
	return (enginebin.Resolver{}).Resolve(req)
}

var resolveWSLPayloadFn = func(explicit string) (enginebin.Resolved, error) {
	return (enginebin.Resolver{}).Resolve(enginebin.Request{
		Kind:            enginebin.KindWSLPayload,
		TargetOS:        "linux",
		TargetArch:      runtime.GOARCH,
		ExplicitPath:    explicit,
		EnvironmentPath: os.Getenv("SQLRS_WSL_ENGINE_PATH"),
	})
}

func runInit(w io.Writer, cwd, globalWorkspace string, args []string, verbose bool) error {
	opts, showHelp, err := parseInitFlags(args, globalWorkspace)
	if err != nil {
		return err
	}
	if showHelp {
		cli.PrintInitUsage(w)
		return nil
	}
	opts.Verbose = verbose
	if strings.TrimSpace(opts.Mode) == "" {
		opts.Mode = "local"
	}

	target, err := resolveWorkspacePath(opts.Workspace, cwd)
	if err != nil {
		return ExitErrorf(4, "Cannot create .sqlrs directory: %v", err)
	}
	explicitHostSource := ""
	if opts.EnginePath != "" {
		explicitHostSource = normalizeEngineSourcePath(opts.EnginePath, cwd)
		opts.EnginePath = normalizeEnginePath(opts.EnginePath, cwd, target)
	}
	if opts.WSLEnginePath != "" {
		opts.WSLEnginePath = normalizeEngineSourcePath(opts.WSLEnginePath, cwd)
	}

	localMarker := filepath.Join(target, ".sqlrs")
	localExists := dirExists(localMarker)

	parentMarker := findParentWorkspace(target)
	if parentMarker && !localExists && !opts.Force {
		return ExitErrorf(2, "Refusing to create nested workspace")
	}

	configPath := filepath.Join(localMarker, "config.yaml")
	hasUpdateFlags := opts.EnginePath != "" ||
		opts.WSLEnginePath != "" ||
		opts.SharedCache ||
		opts.Snapshot != "" ||
		opts.StoreType != "" ||
		opts.StorePath != "" ||
		opts.StoreSizeGB > 0 ||
		opts.Reinit ||
		opts.Distro != "" ||
		opts.RemoteURL != "" ||
		opts.RemoteToken != ""
	configExists := fileExists(configPath)
	configValid := false

	if localExists {
		if configExists {
			if err := validateConfig(configPath); err != nil {
				if !opts.Update {
					return ExitErrorf(3, "Workspace config is corrupted: %v", err)
				}
			} else {
				configValid = true
			}
		}
		if !opts.Update {
			if isWindows && configValid && !opts.DryRun && strings.EqualFold(opts.Mode, "local") {
				repaired, repairErr := repairExistingWSLEngine(configPath, opts.WSLEnginePath, opts.Verbose)
				if repairErr != nil {
					if errors.Is(repairErr, errLegacyWSLEngineUpdateRequired) {
						fmt.Fprintf(w, "Legacy WSL engine configuration requires an explicit update. Run: sqlrs init local --workspace %q --update\n", target)
					} else {
						return ExitErrorf(1, "WSL engine repair failed: %v", repairErr)
					}
				}
				if repaired {
					fmt.Fprintln(w, "Repaired the provisioned WSL engine")
				}
			}
			if opts.DryRun {
				fmt.Fprintf(w, "Workspace already initialized at %s (dry-run)\n", target)
			} else {
				fmt.Fprintf(w, "Workspace already initialized at %s\n", target)
			}
			return nil
		}
		if configExists && configValid && !hasUpdateFlags {
			if opts.DryRun {
				fmt.Fprintf(w, "Workspace already initialized at %s (dry-run)\n", target)
			} else {
				fmt.Fprintf(w, "Workspace already initialized at %s\n", target)
			}
			return nil
		}
	}

	var wslResult *wslInitResult
	usesWSLRuntime := false
	if strings.EqualFold(opts.Mode, "local") {
		snapshot := strings.ToLower(strings.TrimSpace(opts.Snapshot))
		if snapshot == "" {
			snapshot = "auto"
		}
		opts.Snapshot = snapshot

		resolvedStoreType, err := resolveStoreType(snapshot, opts.StoreType)
		if err != nil {
			return ExitErrorf(64, "Invalid arguments: %v", err)
		}
		resolvedStorePath, err := resolveStorePath(resolvedStoreType, opts.StorePath)
		if err != nil {
			return ExitErrorf(64, "Invalid arguments: %v", err)
		}

		storeExplicit := opts.StoreType != "" || opts.StorePath != ""
		useWSL, requireWSL := shouldUseWSL(snapshot, resolvedStoreType, storeExplicit)
		var wslEngineSource string
		if useWSL {
			resolved, resolveErr := resolveWSLPayloadFn(opts.WSLEnginePath)
			if resolveErr != nil {
				return ExitErrorf(64, "Invalid WSL engine: %v", resolveErr)
			}
			wslEngineSource = resolved.Path
		}
		usesWSLRuntime = useWSL
		if useWSL && !opts.DryRun {
			result, err := initWSLFn(wslInitOptions{
				Enable:           true,
				Distro:           opts.Distro,
				Require:          requireWSL,
				NoStart:          opts.NoStart,
				Workspace:        target,
				Verbose:          opts.Verbose,
				StoreSizeGB:      opts.StoreSizeGB,
				Reinit:           opts.Reinit,
				StorePath:        resolvedStorePath,
				EngineSourcePath: wslEngineSource,
			})
			if err != nil {
				return ExitErrorf(1, "WSL init failed: %v", err)
			}
			if result.Warning != "" {
				fmt.Fprintln(os.Stderr, strings.TrimSpace(result.Warning))
			}
			if requireWSL && !result.UseWSL {
				return ExitErrorf(1, "WSL init failed: %s", strings.TrimSpace(result.Warning))
			}
			if result.UseWSL {
				wslResult = &result
				if requireWSL {
					opts.WSLMode = "required"
				} else {
					opts.WSLMode = "auto"
				}
			} else if snapshot == "auto" && !requireWSL {
				usesWSLRuntime = false
				resolvedStoreType = "dir"
				resolvedStorePath, _ = resolveStorePath(resolvedStoreType, "")
			}
		}

		if !usesWSLRuntime {
			configuredHostPath, loadErr := configuredHostEngineForInit(target, configExists, configValid)
			if loadErr != nil {
				return ExitErrorf(64, "Invalid host engine configuration: %v", loadErr)
			}
			if _, resolveErr := resolveHostEngineFn(enginebin.Request{
				Kind:            enginebin.KindHost,
				TargetOS:        runtime.GOOS,
				TargetArch:      runtime.GOARCH,
				ExplicitPath:    explicitHostSource,
				EnvironmentPath: os.Getenv("SQLRS_DAEMON_PATH"),
				ConfigPath:      configuredHostPath,
			}); resolveErr != nil {
				return ExitErrorf(64, "Invalid host engine: %v", resolveErr)
			}
		}

		// Strict btrfs policy is shared across platforms:
		// when snapshot backend is btrfs, init must provision/verify real btrfs
		// or fail fast instead of silently degrading.
		if shouldRunStrictBtrfsInit(snapshot, opts.DryRun, wslResult) {
			localResult, err := initLocalBtrfsStoreFn(localBtrfsInitOptions{
				StoreType:   resolvedStoreType,
				StorePath:   resolvedStorePath,
				StoreSizeGB: opts.StoreSizeGB,
				Reinit:      opts.Reinit,
				Verbose:     opts.Verbose,
			})
			if err != nil {
				return ExitErrorf(1, "Linux btrfs init failed: %v", err)
			}
			if strings.TrimSpace(localResult.StorePath) != "" {
				resolvedStorePath = strings.TrimSpace(localResult.StorePath)
			}
		}

		if wslResult == nil && resolvedStorePath != "" {
			opts.StorePath = resolvedStorePath
		}
	}

	if opts.DryRun {
		if !localExists {
			fmt.Fprintf(w, "Would create %s\n", localMarker)
		}
		fmt.Fprintf(w, "Would write %s\n", configPath)
		return nil
	}

	if !localExists {
		if err := os.MkdirAll(localMarker, 0o700); err != nil {
			return ExitErrorf(4, "Cannot create .sqlrs directory: %v", err)
		}
	}

	baseConfig := map[string]any(nil)
	if opts.Update && configExists && configValid {
		loaded, err := readConfigMap(configPath)
		if err != nil {
			return ExitErrorf(4, "Cannot read config.yaml: %v", err)
		}
		baseConfig = loaded
		if wslResult != nil {
			removeLegacyWSLEngineDaemon(baseConfig, configPath)
		}
	}
	configData, err := buildWorkspaceConfig(opts, wslResult, baseConfig)
	if err != nil {
		return ExitErrorf(1, "Internal error: %v", err)
	}
	if err := util.AtomicWriteFile(configPath, configData, 0o600); err != nil {
		return ExitErrorf(4, "Cannot write config.yaml: %v", err)
	}

	if localExists {
		fmt.Fprintf(w, "Updated workspace at %s\n", target)
	} else {
		fmt.Fprintf(w, "Initialized workspace at %s\n", target)
	}
	return nil
}

func configuredHostEngineForInit(target string, configExists, configValid bool) (string, error) {
	if configExists && !configValid {
		return "", nil
	}
	loaded, err := config.Load(config.LoadOptions{WorkingDir: target})
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(loaded.Config.Orchestrator.DaemonPath), nil
}

var errLegacyWSLEngineUpdateRequired = errors.New("legacy WSL engine configuration requires --update")

func repairExistingWSLEngine(configPath, explicitSource string, verbose bool) (bool, error) {
	data, err := os.ReadFile(configPath)
	if err != nil {
		return false, err
	}
	var cfg config.Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return false, err
	}
	mode := strings.ToLower(strings.TrimSpace(cfg.Engine.WSL.Mode))
	distro := strings.TrimSpace(cfg.Engine.WSL.Distro)
	installed := strings.TrimSpace(cfg.Engine.WSL.EnginePath)
	if (mode != "auto" && mode != "required") || distro == "" {
		return false, nil
	}
	if installed == "" {
		legacyDaemonPath := resolveConfigRelativeEnginePath(cfg.Orchestrator.DaemonPath, configPath)
		if _, resolveErr := resolveLegacyWSLEngineSource(legacyDaemonPath, explicitSource); resolveErr != nil {
			return false, resolveErr
		}
		return false, errLegacyWSLEngineUpdateRequired
	}
	if err := validateInstalledWSLEngineFn(context.Background(), distro, installed, verbose); err == nil {
		return false, nil
	}
	resolved, err := resolveWSLPayloadFn(explicitSource)
	if err != nil {
		return false, err
	}
	actual, err := installWSLEngine(context.Background(), distro, resolved.Path, installed, verbose)
	if err != nil {
		return false, err
	}
	if actual != installed {
		return false, fmt.Errorf("installer returned %q, expected configured path %q", actual, installed)
	}
	return true, nil
}

func resolveConfigRelativeEnginePath(value, configPath string) string {
	resolved := strings.TrimSpace(value)
	if resolved == "" || filepath.IsAbs(resolved) {
		return resolved
	}
	return filepath.Clean(filepath.Join(filepath.Dir(configPath), resolved))
}

func removeLegacyWSLEngineDaemon(raw map[string]any, configPath string) {
	if nestedMapString(raw, "engine", "wsl", "enginePath") != "" {
		return
	}
	orchestrator, ok := raw["orchestrator"].(map[string]any)
	if !ok {
		return
	}
	legacy, _ := orchestrator["daemonPath"].(string)
	legacy = strings.TrimSpace(legacy)
	if legacy == "" {
		return
	}
	legacy = resolveConfigRelativeEnginePath(legacy, configPath)
	if _, err := enginebin.Validate(legacy, "linux", runtime.GOARCH); err == nil {
		delete(orchestrator, "daemonPath")
	}
}

func nestedMapString(root map[string]any, keys ...string) string {
	current := any(root)
	for _, key := range keys {
		next, ok := current.(map[string]any)
		if !ok {
			return ""
		}
		current = next[key]
	}
	value, _ := current.(string)
	return strings.TrimSpace(value)
}

func resolveLegacyWSLEngineSource(legacyDaemonPath, explicitSource string) (enginebin.Resolved, error) {
	if strings.TrimSpace(explicitSource) != "" {
		return resolveWSLPayloadFn(explicitSource)
	}
	legacy := strings.TrimSpace(legacyDaemonPath)
	if legacy != "" {
		if validation, err := enginebin.Validate(legacy, "linux", runtime.GOARCH); err == nil {
			return enginebin.Resolved{Path: legacy, Kind: enginebin.KindWSLPayload, Origin: enginebin.OriginConfig, Format: validation.Format, Arch: validation.Arch}, nil
		}
	}
	return resolveWSLPayloadFn("")
}

func shouldRunStrictBtrfsInit(snapshot string, dryRun bool, wslResult *wslInitResult) bool {
	return strings.EqualFold(strings.TrimSpace(snapshot), "btrfs") && !dryRun && wslResult == nil
}

func validateLinuxEngineBinaryForWSL(enginePath string) error {
	pathValue := strings.TrimSpace(enginePath)
	if pathValue == "" {
		return nil
	}
	if strings.EqualFold(filepath.Ext(pathValue), ".exe") {
		return fmt.Errorf("--engine must point to a Linux sqlrs-engine binary when WSL runtime is required")
	}

	format, err := detectBinaryFormat(pathValue)
	if err != nil {
		if os.IsNotExist(err) {
			// Keep backward compatibility when the path is configured before the file is built.
			return nil
		}
		return fmt.Errorf("cannot inspect --engine binary %q: %v", pathValue, err)
	}

	switch format {
	case "elf":
		return nil
	case "pe":
		return fmt.Errorf("--engine must point to a Linux sqlrs-engine binary when WSL runtime is required")
	default:
		return fmt.Errorf("--engine binary %q is not recognized as Linux ELF", pathValue)
	}
}

func detectBinaryFormat(pathValue string) (string, error) {
	file, err := os.Open(pathValue)
	if err != nil {
		return "", err
	}
	defer file.Close()

	header := make([]byte, 4)
	if _, err := io.ReadFull(file, header); err != nil {
		return "", err
	}
	if header[0] == 0x7f && header[1] == 'E' && header[2] == 'L' && header[3] == 'F' {
		return "elf", nil
	}
	if header[0] == 'M' && header[1] == 'Z' {
		return "pe", nil
	}
	return "unknown", nil
}

func parseInitFlags(args []string, globalWorkspace string) (initOptions, bool, error) {
	var opts initOptions
	if err := validateNoUnicodeDashFlags(args, 64); err != nil {
		return opts, false, err
	}

	mode, rest, err := splitInitMode(args)
	if err != nil {
		return opts, false, ExitErrorf(64, "Invalid arguments: %v", err)
	}
	normalizedArgs, err := preprocessStoreArgs(rest)
	if err != nil {
		return opts, false, ExitErrorf(64, "Invalid arguments: %v", err)
	}

	fs := flag.NewFlagSet("sqlrs init", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	workspace := fs.String("workspace", "", "workspace root")
	force := fs.Bool("force", false, "allow nested workspace")
	engine := fs.String("engine", "", "engine binary path")
	wslEngine := fs.String("wsl-engine", "", "Linux engine binary to install into WSL")
	sharedCache := fs.Bool("shared-cache", false, "use shared cache")
	update := fs.Bool("update", false, "update existing workspace config")
	snapshot := fs.String("snapshot", "", "snapshot backend")
	storeType := fs.String("store-type", "", "store type")
	storePath := fs.String("store-path", "", "store path")
	storeSize := fs.String("store-size", "", "btrfs store size (example: 100GB)")
	reinit := fs.Bool("reinit", false, "recreate store")
	distro := fs.String("distro", "", "WSL distro name")
	noStart := fs.Bool("no-start", false, "skip WSL engine auto-start")
	url := fs.String("url", "", "remote engine url")
	token := fs.String("token", "", "remote engine token")
	dryRun := fs.Bool("dry-run", false, "dry run")
	help := fs.Bool("help", false, "show help")
	helpShort := fs.Bool("h", false, "show help")

	if err := fs.Parse(normalizedArgs); err != nil {
		return opts, false, ExitErrorf(64, "Invalid arguments: %v", err)
	}

	if *help || *helpShort {
		return opts, true, nil
	}

	if fs.NArg() > 0 {
		return opts, false, ExitErrorf(64, "Invalid arguments")
	}

	opts.Mode = mode
	opts.Workspace = strings.TrimSpace(*workspace)
	if opts.Workspace == "" {
		opts.Workspace = strings.TrimSpace(globalWorkspace)
	}
	opts.Force = *force
	opts.Update = *update
	opts.EnginePath = strings.TrimSpace(*engine)
	opts.WSLEnginePath = strings.TrimSpace(*wslEngine)
	opts.SharedCache = *sharedCache
	opts.DryRun = *dryRun
	opts.Snapshot = normalizeSnapshot(*snapshot)
	opts.StoreType = normalizeStoreType(*storeType)
	opts.StorePath = strings.TrimSpace(*storePath)
	opts.Reinit = *reinit
	opts.Distro = strings.TrimSpace(*distro)
	opts.NoStart = *noStart
	opts.RemoteURL = strings.TrimSpace(*url)
	opts.RemoteToken = strings.TrimSpace(*token)

	if size := strings.TrimSpace(*storeSize); size != "" {
		value, err := parseStoreSizeGB(size)
		if err != nil {
			return opts, false, ExitErrorf(64, "Invalid arguments: %v", err)
		}
		opts.StoreSizeGB = value
	}

	mode = strings.ToLower(strings.TrimSpace(opts.Mode))
	if mode == "" {
		mode = "local"
		opts.Mode = mode
	}
	if mode != "local" && mode != "remote" {
		return opts, false, ExitErrorf(64, "Invalid arguments: unknown init mode")
	}

	if mode == "remote" {
		if opts.RemoteURL == "" || opts.RemoteToken == "" {
			return opts, false, ExitErrorf(64, "Invalid arguments: --url and --token are required for remote init")
		}
		if opts.EnginePath != "" || opts.WSLEnginePath != "" || opts.SharedCache || opts.Snapshot != "" || opts.StoreType != "" || opts.StorePath != "" || opts.StoreSizeGB > 0 || opts.Reinit || opts.Distro != "" || opts.NoStart {
			return opts, false, ExitErrorf(64, "Invalid arguments: local-only flags are not valid for remote init")
		}
		return opts, false, nil
	}

	if opts.RemoteURL != "" || opts.RemoteToken != "" {
		return opts, false, ExitErrorf(64, "Invalid arguments: --url/--token require remote init")
	}

	if opts.Snapshot != "" && !isKnownSnapshot(opts.Snapshot) {
		return opts, false, ExitErrorf(64, "Invalid arguments: unknown snapshot backend")
	}
	if opts.StorePath != "" && opts.StoreType == "" {
		return opts, false, ExitErrorf(64, "Invalid arguments: --store path requires a store type")
	}
	if opts.StoreType != "" && !isKnownStoreType(opts.StoreType) {
		return opts, false, ExitErrorf(64, "Invalid arguments: unknown store type")
	}
	if opts.StoreType == "device" && opts.StorePath == "" {
		return opts, false, ExitErrorf(64, "Invalid arguments: --store device requires a path")
	}
	if opts.StoreSizeGB > 0 && opts.StoreType != "image" {
		return opts, false, ExitErrorf(64, "Invalid arguments: --store-size requires --store image")
	}
	if (opts.StoreType == "image" || opts.StoreType == "device") && (opts.Snapshot == "overlay" || opts.Snapshot == "copy") {
		return opts, false, ExitErrorf(64, "Invalid arguments: store type requires btrfs backend")
	}
	if opts.Reinit && (opts.Snapshot == "overlay" || opts.Snapshot == "copy") {
		return opts, false, ExitErrorf(64, "Invalid arguments: --reinit requires btrfs backend")
	}
	if opts.Snapshot == "overlay" && runtime.GOOS != "linux" {
		return opts, false, ExitErrorf(64, "Invalid arguments: overlay snapshots are only supported on Linux")
	}
	if opts.Snapshot == "btrfs" && runtime.GOOS == "darwin" {
		return opts, false, ExitErrorf(64, "Invalid arguments: btrfs snapshots are not supported on macOS")
	}
	if runtime.GOOS == "darwin" && (opts.StoreType == "image" || opts.StoreType == "device") && (opts.Snapshot == "" || opts.Snapshot == "auto") {
		return opts, false, ExitErrorf(64, "Invalid arguments: btrfs snapshots are not supported on macOS")
	}
	if opts.Snapshot == "btrfs" && runtime.GOOS == "windows" && opts.StoreType == "dir" {
		return opts, false, ExitErrorf(64, "Invalid arguments: btrfs on Windows requires an image or device store")
	}

	return opts, false, nil
}

func resolveWorkspacePath(workspace, cwd string) (string, error) {
	target := strings.TrimSpace(workspace)
	if target == "" {
		target = cwd
	}
	if target == "" {
		return "", errors.New("workspace path is empty")
	}
	if !filepath.IsAbs(target) {
		target = filepath.Join(cwd, target)
	}
	abs := filepath.Clean(target)

	info, err := os.Stat(abs)
	if err != nil {
		return "", err
	}
	if !info.IsDir() {
		return "", fmt.Errorf("workspace is not a directory: %s", abs)
	}
	return abs, nil
}

func findParentWorkspace(target string) bool {
	parent := filepath.Dir(target)
	for parent != "" && parent != filepath.Dir(parent) {
		if dirExists(filepath.Join(parent, ".sqlrs")) {
			return true
		}
		parent = filepath.Dir(parent)
	}
	if parent != "" && parent != target {
		if dirExists(filepath.Join(parent, ".sqlrs")) {
			return true
		}
	}
	return false
}

func buildWorkspaceConfig(opts initOptions, wslResult *wslInitResult, base map[string]any) ([]byte, error) {
	var cfg map[string]any
	if base != nil {
		cfg = base
	} else {
		cfg = config.DefaultConfigMap()
	}
	if opts.EnginePath != "" {
		setNested(cfg, []string{"orchestrator", "daemonPath"}, opts.EnginePath)
	}
	if opts.SharedCache {
		setNested(cfg, []string{"cache", "shared"}, true)
	}
	if strings.EqualFold(opts.Mode, "local") {
		if strings.TrimSpace(opts.Snapshot) != "" {
			setNested(cfg, []string{"snapshot", "backend"}, opts.Snapshot)
		}
		if opts.StorePath != "" {
			setNested(cfg, []string{"engine", "storePath"}, opts.StorePath)
		}
	}
	if strings.EqualFold(opts.Mode, "remote") {
		setNested(cfg, []string{"defaultProfile"}, "remote")
		setNested(cfg, []string{"profiles", "remote", "mode"}, "remote")
		if opts.RemoteURL != "" {
			setNested(cfg, []string{"profiles", "remote", "endpoint"}, opts.RemoteURL)
		}
		if opts.RemoteToken != "" {
			setNested(cfg, []string{"profiles", "remote", "auth", "token"}, opts.RemoteToken)
		}
	}
	if wslResult != nil {
		mode := strings.TrimSpace(opts.WSLMode)
		if mode == "" {
			mode = "auto"
		}
		setNested(cfg, []string{"engine", "wsl", "mode"}, mode)
		distro := opts.Distro
		if wslResult.Distro != "" {
			distro = wslResult.Distro
		}
		if distro != "" {
			setNested(cfg, []string{"engine", "wsl", "distro"}, distro)
		}
		if wslResult.StateDir != "" {
			setNested(cfg, []string{"engine", "wsl", "stateDir"}, wslResult.StateDir)
		}
		if wslResult.EnginePath != "" {
			setNested(cfg, []string{"engine", "wsl", "enginePath"}, wslResult.EnginePath)
		}
		if wslResult.MountDevice != "" {
			setNested(cfg, []string{"engine", "wsl", "mount", "device"}, wslResult.MountDevice)
		}
		if wslResult.MountFSType != "" {
			setNested(cfg, []string{"engine", "wsl", "mount", "fstype"}, wslResult.MountFSType)
		}
		if wslResult.MountDeviceUUID != "" {
			setNested(cfg, []string{"engine", "wsl", "mount", "deviceUUID"}, wslResult.MountDeviceUUID)
		}
		if wslResult.MountUnit != "" {
			setNested(cfg, []string{"engine", "wsl", "mount", "unit"}, wslResult.MountUnit)
		}
		if wslResult.StorePath != "" {
			setNested(cfg, []string{"engine", "storePath"}, wslResult.StorePath)
		}
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		return nil, err
	}
	if len(data) == 0 || data[len(data)-1] != '\n' {
		data = append(data, '\n')
	}
	return data, nil
}

func normalizeEnginePath(enginePath, cwd, workspace string) string {
	path := strings.TrimSpace(enginePath)
	if path == "" {
		return path
	}
	if filepath.IsAbs(path) {
		return filepath.Clean(path)
	}

	cwdAbs := resolveExistingPath(cwd)
	cwdAbs = rebasePathToWorkspaceRoot(cwdAbs, workspace)
	if isWithin(workspace, cwdAbs) {
		configDir := filepath.Join(workspace, ".sqlrs")
		absEngine := filepath.Clean(filepath.Join(cwdAbs, path))
		absEngine = rebasePathToWorkspaceRoot(absEngine, workspace)
		if rel, err := filepath.Rel(configDir, absEngine); err == nil {
			return rel
		}
		return absEngine
	}

	return filepath.Clean(filepath.Join(cwdAbs, path))
}

func normalizeEngineSourcePath(enginePath, cwd string) string {
	value := strings.TrimSpace(enginePath)
	if value == "" {
		return ""
	}
	if filepath.IsAbs(value) {
		return filepath.Clean(value)
	}
	base := strings.TrimSpace(cwd)
	if base == "" {
		base, _ = os.Getwd()
	}
	return filepath.Clean(filepath.Join(base, value))
}

func resolveExistingPath(value string) string {
	path := value
	if !filepath.IsAbs(path) {
		if abs, err := filepath.Abs(path); err == nil {
			path = abs
		}
	}
	path = filepath.Clean(path)
	if resolved, err := filepath.EvalSymlinks(path); err == nil {
		path = resolved
	}
	return path
}

func validateConfig(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var raw any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return err
	}
	return nil
}

func readConfigMap(path string) (map[string]any, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		return nil, err
	}
	if raw == nil {
		return map[string]any{}, nil
	}
	return raw, nil
}

func setNested(root map[string]any, keys []string, value any) {
	current := root
	for i, key := range keys {
		if i == len(keys)-1 {
			current[key] = value
			return
		}
		next, ok := current[key].(map[string]any)
		if !ok {
			next = map[string]any{}
			current[key] = next
		}
		current = next
	}
}

func parseStoreSizeGB(value string) (int, error) {
	raw := strings.TrimSpace(value)
	if raw == "" {
		return 0, fmt.Errorf("store size is empty")
	}
	upper := strings.ToUpper(raw)
	if !strings.HasSuffix(upper, "GB") {
		return 0, fmt.Errorf("store size must use GB suffix")
	}
	num := strings.TrimSpace(raw[:len(raw)-2])
	if num == "" {
		return 0, fmt.Errorf("store size is empty")
	}
	size, err := strconv.Atoi(num)
	if err != nil || size <= 0 {
		return 0, fmt.Errorf("store size must be a positive integer")
	}
	return size, nil
}

func splitInitMode(args []string) (string, []string, error) {
	if len(args) == 0 {
		return "local", args, nil
	}
	first := strings.TrimSpace(args[0])
	if first == "" || strings.HasPrefix(first, "-") {
		return "local", args, nil
	}
	switch strings.ToLower(first) {
	case "local", "remote":
		return strings.ToLower(first), args[1:], nil
	default:
		return "", args, fmt.Errorf("unknown init mode: %s", first)
	}
}

func preprocessStoreArgs(args []string) ([]string, error) {
	out := make([]string, 0, len(args))
	for i := 0; i < len(args); i++ {
		arg := args[i]
		if arg == "--store" {
			if i+1 >= len(args) {
				return nil, fmt.Errorf("--store requires a type")
			}
			storeType := args[i+1]
			i++
			out = append(out, "--store-type", storeType)
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				out = append(out, "--store-path", args[i+1])
				i++
			}
			continue
		}
		if strings.HasPrefix(arg, "--store=") {
			storeType := strings.TrimPrefix(arg, "--store=")
			if storeType == "" {
				return nil, fmt.Errorf("--store requires a type")
			}
			out = append(out, "--store-type", storeType)
			if i+1 < len(args) && !strings.HasPrefix(args[i+1], "-") {
				out = append(out, "--store-path", args[i+1])
				i++
			}
			continue
		}
		out = append(out, arg)
	}
	return out, nil
}

func normalizeSnapshot(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func normalizeStoreType(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}

func isKnownSnapshot(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "auto", "btrfs", "overlay", "copy":
		return true
	default:
		return false
	}
}

func isKnownStoreType(value string) bool {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "dir", "device", "image":
		return true
	default:
		return false
	}
}

func resolveStoreType(snapshot string, storeType string) (string, error) {
	return resolveStoreTypeForPlatform(snapshot, storeType, runtime.GOOS, defaultStoreRoot, isBtrfsPath)
}

func resolveStoreTypeForPlatform(snapshot string, storeType string, goos string, resolveDefaultStoreRoot func() (string, error), isBtrfsStoreRoot func(string) (bool, error)) (string, error) {
	normalized := normalizeStoreType(storeType)
	if normalized != "" {
		return normalized, nil
	}
	switch strings.ToLower(strings.TrimSpace(snapshot)) {
	case "copy", "overlay":
		return "dir", nil
	case "btrfs":
		switch goos {
		case "windows":
			return "image", nil
		case "linux":
			root, err := resolveDefaultStoreRoot()
			if err != nil {
				return "image", nil
			}
			ok, err := isBtrfsStoreRoot(root)
			if err == nil && ok {
				return "dir", nil
			}
			return "image", nil
		default:
			return "dir", nil
		}
	case "auto":
		if goos == "windows" {
			return "image", nil
		}
		return "dir", nil
	default:
		return "dir", nil
	}
}

func resolveStorePath(storeType string, storePath string) (string, error) {
	pathValue := strings.TrimSpace(storePath)
	if pathValue != "" {
		return pathValue, nil
	}
	normalized := normalizeStoreType(storeType)
	if normalized == "" || normalized == "device" {
		return "", nil
	}
	root, err := defaultStoreRoot()
	if err != nil {
		return "", err
	}
	switch normalized {
	case "dir":
		return root, nil
	case "image":
		name := "btrfs.img"
		if runtime.GOOS == "windows" {
			name = "btrfs.vhdx"
		}
		return filepath.Join(root, name), nil
	default:
		return "", nil
	}
}

func defaultStoreRoot() (string, error) {
	if value := strings.TrimSpace(os.Getenv("SQLRS_STATE_STORE")); value != "" {
		return value, nil
	}
	dirs, err := paths.Resolve()
	if err != nil {
		return "", err
	}
	return filepath.Join(dirs.StateDir, "store"), nil
}

func shouldUseWSL(snapshot string, storeType string, storeExplicit bool) (bool, bool) {
	if runtime.GOOS != "windows" {
		return false, false
	}
	normalizedSnapshot := strings.ToLower(strings.TrimSpace(snapshot))
	normalizedStore := normalizeStoreType(storeType)
	if normalizedSnapshot == "btrfs" {
		if normalizedStore == "dir" {
			return false, true
		}
		return true, true
	}
	if normalizedSnapshot == "auto" {
		if normalizedStore == "dir" {
			return false, false
		}
		return true, storeExplicit
	}
	return false, false
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

func dirExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}
