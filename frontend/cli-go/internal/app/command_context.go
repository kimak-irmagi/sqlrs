package app

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/sqlrs/cli/internal/cli"
	"github.com/sqlrs/cli/internal/config"
	"github.com/sqlrs/cli/internal/enginebin"
)

// commandContext centralizes shared CLI command wiring defined in
// docs/architecture/cli-maintainability-refactor.md.
type commandContext struct {
	cwd                  string
	workspaceRoot        string
	cfgResult            config.LoadedConfig
	profileName          string
	profile              config.ProfileConfig
	mode                 string
	output               string
	authToken            string
	daemonPath           string
	runDir               string
	engineRunDir         string
	engineStatePath      string
	engineStoreDir       string
	engineHostStorePath  string
	engineWSLMountUnit   string
	engineWSLMountFSType string
	wslDistro            string
	timeout              time.Duration
	idleTimeout          time.Duration
	startupTimeout       time.Duration
	verbose              bool
}

// resolveCommandContext resolves config, profile, runtime paths, and output mode
// once for the whole CLI invocation as described by the maintainability refactor design.
func resolveCommandContext(cwd string, opts cli.GlobalOptions) (commandContext, error) {
	result := commandContext{cwd: cwd, verbose: opts.Verbose}

	configWorkingDir := cwd
	if workspace := strings.TrimSpace(opts.Workspace); workspace != "" {
		if filepath.IsAbs(workspace) {
			configWorkingDir = filepath.Clean(workspace)
		} else {
			configWorkingDir = filepath.Clean(filepath.Join(cwd, workspace))
		}
	}

	cfgResult, err := config.Load(config.LoadOptions{WorkingDir: configWorkingDir})
	if err != nil {
		return result, err
	}
	result.cfgResult = cfgResult
	if strings.TrimSpace(opts.Workspace) != "" {
		result.workspaceRoot = configWorkingDir
	}
	if cfgResult.ProjectConfigPath != "" {
		result.workspaceRoot = filepath.Dir(filepath.Dir(cfgResult.ProjectConfigPath))
	}

	cfg := cfgResult.Config
	profileName := strings.TrimSpace(opts.Profile)
	if profileName == "" {
		profileName = cfg.DefaultProfile
	}
	if profileName == "" {
		profileName = "local"
	}
	profile, ok := cfg.Profiles[profileName]
	if !ok {
		return result, fmt.Errorf("profile not found: %s", profileName)
	}
	if strings.TrimSpace(opts.Endpoint) != "" {
		profile.Endpoint = opts.Endpoint
	}

	mode := strings.ToLower(strings.TrimSpace(opts.Mode))
	if mode == "" || mode == "auto" {
		mode = strings.ToLower(strings.TrimSpace(profile.Mode))
		if mode == "" {
			mode = "local"
		}
	}
	if mode != "local" && mode != "remote" {
		return result, fmt.Errorf("invalid mode: %s", mode)
	}

	output := strings.ToLower(strings.TrimSpace(cfg.Client.Output))
	if strings.TrimSpace(opts.Output) != "" {
		output = strings.ToLower(strings.TrimSpace(opts.Output))
	}
	if output == "" {
		output = "human"
	}
	if output != "human" && output != "json" {
		return result, fmt.Errorf("invalid output: %s", output)
	}

	timeout := opts.Timeout
	if timeout == 0 {
		parsed, err := config.ParseDuration(cfg.Client.Timeout, defaultTimeout)
		if err != nil {
			return result, err
		}
		timeout = parsed
	}
	startupTimeout, err := config.ParseDuration(cfg.Orchestrator.StartupTimeout, defaultStartupTimeout)
	if err != nil {
		return result, err
	}
	idleTimeout, err := config.ParseDuration(cfg.Orchestrator.IdleTimeout, defaultIdleTimeout)
	if err != nil {
		return result, err
	}

	runDir := cfg.Orchestrator.RunDir
	if runDir == "" {
		runDir = filepath.Join(cfgResult.Paths.StateDir, "run")
	}

	daemonPath := os.Getenv("SQLRS_DAEMON_PATH")
	if daemonPath == "" {
		daemonPath = cfg.Orchestrator.DaemonPath
	}
	if strings.TrimSpace(daemonPath) == "" && mode == "local" {
		if resolved, resolveErr := (enginebin.Resolver{}).Resolve(enginebin.Request{
			Kind:       enginebin.KindHost,
			TargetOS:   runtime.GOOS,
			TargetArch: runtime.GOARCH,
		}); resolveErr == nil {
			daemonPath = resolved.Path
		}
	}
	engineRunDir := ""
	engineStatePath := ""
	engineStoreDir := strings.TrimSpace(cfg.Engine.StorePath)
	engineHostStorePath := engineStoreDir
	engineWSLMountUnit := ""
	engineWSLMountFSType := ""
	wslDistro := ""
	if runtime.GOOS == "windows" {
		daemonPath, engineRunDir, engineStatePath, engineStoreDir, wslDistro, engineWSLMountUnit, engineWSLMountFSType, err = resolveWSLSettings(cfg, cfgResult.Paths, daemonPath)
		if err != nil {
			return result, err
		}
	}

	result.profileName = profileName
	result.profile = profile
	result.mode = mode
	result.output = output
	result.authToken = resolveAuthToken(profile.Auth)
	result.daemonPath = daemonPath
	result.runDir = runDir
	result.engineRunDir = engineRunDir
	result.engineStatePath = engineStatePath
	result.engineStoreDir = engineStoreDir
	result.engineHostStorePath = engineHostStorePath
	result.engineWSLMountUnit = engineWSLMountUnit
	result.engineWSLMountFSType = engineWSLMountFSType
	result.wslDistro = wslDistro
	result.timeout = timeout
	result.idleTimeout = idleTimeout
	result.startupTimeout = startupTimeout
	return result, nil
}

func (ctx commandContext) lsOptions() cli.LsOptions {
	return cli.LsOptions{
		ProfileName:     ctx.profileName,
		Mode:            ctx.mode,
		AuthToken:       ctx.authToken,
		Endpoint:        ctx.profile.Endpoint,
		Autostart:       ctx.profile.Autostart,
		DaemonPath:      ctx.daemonPath,
		RunDir:          ctx.runDir,
		StateDir:        ctx.cfgResult.Paths.StateDir,
		EngineRunDir:    ctx.engineRunDir,
		EngineStatePath: ctx.engineStatePath,
		EngineStoreDir:  ctx.engineStoreDir,
		WSLVHDXPath:     ctx.engineHostStorePath,
		WSLMountUnit:    ctx.engineWSLMountUnit,
		WSLMountFSType:  ctx.engineWSLMountFSType,
		WSLDistro:       ctx.wslDistro,
		Timeout:         ctx.timeout,
		IdleTimeout:     ctx.idleTimeout,
		StartupTimeout:  ctx.startupTimeout,
		Verbose:         ctx.verbose,
	}
}

func (ctx commandContext) rmOptions() cli.RmOptions {
	return cli.RmOptions{
		ProfileName:     ctx.profileName,
		Mode:            ctx.mode,
		AuthToken:       ctx.authToken,
		Endpoint:        ctx.profile.Endpoint,
		Autostart:       ctx.profile.Autostart,
		DaemonPath:      ctx.daemonPath,
		RunDir:          ctx.runDir,
		StateDir:        ctx.cfgResult.Paths.StateDir,
		EngineRunDir:    ctx.engineRunDir,
		EngineStatePath: ctx.engineStatePath,
		EngineStoreDir:  ctx.engineStoreDir,
		WSLVHDXPath:     ctx.engineHostStorePath,
		WSLMountUnit:    ctx.engineWSLMountUnit,
		WSLMountFSType:  ctx.engineWSLMountFSType,
		WSLDistro:       ctx.wslDistro,
		Timeout:         ctx.timeout,
		IdleTimeout:     ctx.idleTimeout,
		StartupTimeout:  ctx.startupTimeout,
		Verbose:         ctx.verbose,
	}
}

func (ctx commandContext) prepareOptions(composite bool) cli.PrepareOptions {
	return cli.PrepareOptions{
		ProfileName:         ctx.profileName,
		Mode:                ctx.mode,
		AuthToken:           ctx.authToken,
		Endpoint:            ctx.profile.Endpoint,
		Autostart:           ctx.profile.Autostart,
		DaemonPath:          ctx.daemonPath,
		RunDir:              ctx.runDir,
		StateDir:            ctx.cfgResult.Paths.StateDir,
		EngineRunDir:        ctx.engineRunDir,
		EngineStatePath:     ctx.engineStatePath,
		EngineStoreDir:      ctx.engineStoreDir,
		WSLVHDXPath:         ctx.engineHostStorePath,
		WSLMountUnit:        ctx.engineWSLMountUnit,
		WSLMountFSType:      ctx.engineWSLMountFSType,
		WSLDistro:           ctx.wslDistro,
		Timeout:             ctx.timeout,
		IdleTimeout:         ctx.idleTimeout,
		StartupTimeout:      ctx.startupTimeout,
		Verbose:             ctx.verbose,
		CompositeRun:        composite,
		SourceSyncMode:      ctx.profile.SourceSync.Mode,
		SourceSyncMaxRounds: ctx.profile.SourceSync.MaxRounds,
	}
}

func (ctx commandContext) runOptions() cli.RunOptions {
	return cli.RunOptions{
		ProfileName:     ctx.profileName,
		Mode:            ctx.mode,
		AuthToken:       ctx.authToken,
		Endpoint:        ctx.profile.Endpoint,
		Autostart:       ctx.profile.Autostart,
		DaemonPath:      ctx.daemonPath,
		RunDir:          ctx.runDir,
		StateDir:        ctx.cfgResult.Paths.StateDir,
		EngineRunDir:    ctx.engineRunDir,
		EngineStatePath: ctx.engineStatePath,
		EngineStoreDir:  ctx.engineStoreDir,
		WSLVHDXPath:     ctx.engineHostStorePath,
		WSLMountUnit:    ctx.engineWSLMountUnit,
		WSLMountFSType:  ctx.engineWSLMountFSType,
		WSLDistro:       ctx.wslDistro,
		Timeout:         ctx.timeout,
		IdleTimeout:     ctx.idleTimeout,
		StartupTimeout:  ctx.startupTimeout,
		Verbose:         ctx.verbose,
	}
}

func (ctx commandContext) statusOptions() cli.StatusOptions {
	return cli.StatusOptions{
		ProfileName:     ctx.profileName,
		Mode:            ctx.mode,
		AuthToken:       ctx.authToken,
		Endpoint:        ctx.profile.Endpoint,
		Autostart:       ctx.profile.Autostart,
		DaemonPath:      ctx.daemonPath,
		RunDir:          ctx.runDir,
		StateDir:        ctx.cfgResult.Paths.StateDir,
		EngineRunDir:    ctx.engineRunDir,
		EngineStatePath: ctx.engineStatePath,
		EngineStoreDir:  ctx.engineStoreDir,
		WSLVHDXPath:     ctx.engineHostStorePath,
		WSLMountUnit:    ctx.engineWSLMountUnit,
		WSLMountFSType:  ctx.engineWSLMountFSType,
		WSLDistro:       ctx.wslDistro,
		Timeout:         ctx.timeout,
		IdleTimeout:     ctx.idleTimeout,
		StartupTimeout:  ctx.startupTimeout,
		Verbose:         ctx.verbose,
	}
}

func (ctx commandContext) configOptions() cli.ConfigOptions {
	return cli.ConfigOptions{
		ProfileName:     ctx.profileName,
		Mode:            ctx.mode,
		AuthToken:       ctx.authToken,
		Endpoint:        ctx.profile.Endpoint,
		Autostart:       ctx.profile.Autostart,
		DaemonPath:      ctx.daemonPath,
		RunDir:          ctx.runDir,
		StateDir:        ctx.cfgResult.Paths.StateDir,
		EngineRunDir:    ctx.engineRunDir,
		EngineStatePath: ctx.engineStatePath,
		EngineStoreDir:  ctx.engineStoreDir,
		WSLVHDXPath:     ctx.engineHostStorePath,
		WSLMountUnit:    ctx.engineWSLMountUnit,
		WSLMountFSType:  ctx.engineWSLMountFSType,
		WSLDistro:       ctx.wslDistro,
		Timeout:         ctx.timeout,
		IdleTimeout:     ctx.idleTimeout,
		StartupTimeout:  ctx.startupTimeout,
		Verbose:         ctx.verbose,
	}
}

func (ctx commandContext) userOrgOptions() cli.UserOrgOptions {
	return cli.UserOrgOptions{
		ProfileName: ctx.profileName,
		Mode:        ctx.mode,
		AuthToken:   ctx.authToken,
		Endpoint:    ctx.profile.Endpoint,
		Timeout:     ctx.timeout,
		Verbose:     ctx.verbose,
	}
}
