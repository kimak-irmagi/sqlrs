package daemon

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"github.com/sqlrs/cli/internal/client"
	"github.com/sqlrs/cli/internal/util"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/sqlrs/cli/internal/enginebin"
)

var isWindows = runtime.GOOS == "windows"

var resolveHostEngineForStartFn = func(candidate string) (enginebin.Resolved, error) {
	return (enginebin.Resolver{}).Resolve(enginebin.Request{
		Kind:       enginebin.KindHost,
		TargetOS:   runtime.GOOS,
		TargetArch: runtime.GOARCH,
		ConfigPath: candidate,
	})
}

var validateWSLEngineForStartFn = func(ctx context.Context, distro, enginePath string) error {
	_, err := runWSLCommandFn(ctx, distro, "test", "-x", enginePath)
	return err
}

var cachedEngineStates = struct {
	mu sync.Mutex
	m  map[string]EngineState
}{
	m: map[string]EngineState{},
}

type ConnectOptions struct {
	Endpoint        string
	Autostart       bool
	DaemonPath      string
	RunDir          string
	StateDir        string
	EngineRunDir    string
	EngineStatePath string
	WSLDistro       string
	EngineStoreDir  string
	WSLVHDXPath     string
	WSLMountUnit    string
	WSLMountFSType  string
	IdleTimeout     time.Duration
	StartupTimeout  time.Duration
	ClientTimeout   time.Duration
	Verbose         bool
}

type ConnectResult struct {
	Endpoint  string
	AuthToken string
	State     EngineState
}

func ConnectOrStart(ctx context.Context, opts ConnectOptions) (ConnectResult, error) {
	endpoint := opts.Endpoint
	if endpoint != "" && endpoint != "auto" {
		return ConnectResult{Endpoint: endpoint}, nil
	}

	enginePath := filepath.Join(opts.StateDir, "engine.json")
	if state, ok := loadCachedEngineState(opts); ok {
		if healthy, ok := tryHealthyState(ctx, state, opts.ClientTimeout, opts.Verbose); ok {
			return ConnectResult{Endpoint: healthy.Endpoint, AuthToken: healthy.AuthToken, State: healthy}, nil
		}
	}
	logVerbose(opts.Verbose, "checking engine.json at %s", enginePath)
	if state, ok, _ := loadHealthyStateWithReason(ctx, enginePath, opts.ClientTimeout); ok {
		if refreshed, authOK := refreshStateOnAuthRejected(ctx, enginePath, state, opts.ClientTimeout, opts.Verbose); authOK {
			state = refreshed
		} else {
			goto LONG_PATH
		}
		storeCachedEngineState(opts, state)
		logVerbose(opts.Verbose, "engine healthy at %s", state.Endpoint)
		return ConnectResult{Endpoint: state.Endpoint, AuthToken: state.AuthToken, State: state}, nil
	}

LONG_PATH:
	if err := ensureWSLStoreMount(ctx, opts); err != nil {
		return ConnectResult{}, err
	}

	mustStartDaemon := false
	// Recheck after WSL mount setup to avoid unnecessary daemon startup.
	if state, ok, _ := loadHealthyStateWithReason(ctx, enginePath, opts.ClientTimeout); ok {
		if refreshed, authOK := refreshStateOnAuthRejected(ctx, enginePath, state, opts.ClientTimeout, opts.Verbose); authOK {
			state = refreshed
			logVerbose(opts.Verbose, "engine healthy at %s", state.Endpoint)
			storeCachedEngineState(opts, state)
			return ConnectResult{Endpoint: state.Endpoint, AuthToken: state.AuthToken, State: state}, nil
		}
		mustStartDaemon = true
	}

	if !opts.Autostart {
		logVerbose(opts.Verbose, "engine not running and autostart disabled")
		return ConnectResult{}, fmt.Errorf("local engine is not running")
	}
	if opts.RunDir == "" {
		return ConnectResult{}, fmt.Errorf("runDir is not configured")
	}

	if err := util.EnsureDir(opts.RunDir); err != nil {
		return ConnectResult{}, err
	}

	lockPath := filepath.Join(opts.RunDir, "daemon.lock")
	lock, err := AcquireLock(lockPath, opts.StartupTimeout)
	if err != nil {
		return ConnectResult{}, err
	}
	defer lock.Release()

	logVerbose(opts.Verbose, "acquired engine lock")
	if !mustStartDaemon {
		if state, ok, _ := loadHealthyStateWithReason(ctx, enginePath, opts.ClientTimeout); ok {
			if refreshed, authOK := refreshStateOnAuthRejected(ctx, enginePath, state, opts.ClientTimeout, opts.Verbose); authOK {
				state = refreshed
				logVerbose(opts.Verbose, "engine became healthy while waiting for lock")
				storeCachedEngineState(opts, state)
				return ConnectResult{Endpoint: state.Endpoint, AuthToken: state.AuthToken, State: state}, nil
			}
			mustStartDaemon = true
		}
	}

	if mustStartDaemon {
		logVerbose(opts.Verbose, "healthy engine rejected auth after refresh; starting new daemon")
	}
	if strings.TrimSpace(opts.WSLDistro) != "" {
		if strings.TrimSpace(opts.DaemonPath) == "" {
			return ConnectResult{}, fmt.Errorf("configured WSL engine is missing; rerun sqlrs init local")
		}
		if err := validateWSLEngineForStartFn(ctx, opts.WSLDistro, opts.DaemonPath); err != nil {
			return ConnectResult{}, fmt.Errorf("configured WSL engine %q is unavailable: %v; rerun sqlrs init local", opts.DaemonPath, err)
		}
	} else {
		resolved, err := resolveHostEngineForStartFn(opts.DaemonPath)
		if err != nil {
			return ConnectResult{}, fmt.Errorf("resolve host engine: %w", err)
		}
		opts.DaemonPath = resolved.Path
	}
	logsDir := filepath.Join(opts.StateDir, "logs")
	if err := util.EnsureDir(logsDir); err != nil {
		return ConnectResult{}, err
	}

	logPath := filepath.Join(logsDir, "engine.log")
	isWSL := opts.WSLDistro != ""
	if isWSL {
		logPath = filepath.Join(logsDir, "engine-wsl.log")
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return ConnectResult{}, err
	}
	defer logFile.Close()

	daemonRunDir := opts.EngineRunDir
	if daemonRunDir == "" {
		daemonRunDir = opts.RunDir
	}
	daemonStatePath := opts.EngineStatePath
	if daemonStatePath == "" {
		daemonStatePath = enginePath
	}
	cmd := buildDaemonCommand(
		opts.DaemonPath,
		daemonRunDir,
		daemonStatePath,
		opts.WSLDistro,
		opts.EngineStoreDir,
		opts.WSLMountUnit,
		opts.WSLMountFSType,
		opts.IdleTimeout,
		logPath,
	)
	cmd.Stdout = logFile
	cmd.Stderr = logFile

	tailer, _ := startLogTail(logPath, os.Stderr)
	logVerbose(opts.Verbose, "starting engine: %s", opts.DaemonPath)
	if err := cmd.Start(); err != nil {
		if tailer != nil {
			tailer.Stop()
		}
		return ConnectResult{}, err
	}

	exitCh := make(chan error, 1)
	go func() {
		exitCh <- cmd.Wait()
	}()

	logVerbose(opts.Verbose, "waiting for engine to become healthy")
	deadline := time.Now().Add(opts.StartupTimeout)
	lastReason := ""
	lastReasonAt := time.Time{}
	for {
		select {
		case err := <-exitCh:
			if tailer != nil {
				tailer.Stop()
			}
			return ConnectResult{}, formatEngineExit(err)
		default:
		}
		if state, ok, reason := loadHealthyStateWithReason(ctx, enginePath, opts.ClientTimeout); ok {
			if tailer != nil {
				tailer.Stop()
			}
			logVerbose(opts.Verbose, "engine healthy at %s", state.Endpoint)
			storeCachedEngineState(opts, state)
			return ConnectResult{Endpoint: state.Endpoint, AuthToken: state.AuthToken, State: state}, nil
		} else if opts.Verbose && reason != "" {
			now := time.Now()
			if reason != lastReason || now.Sub(lastReasonAt) > time.Second {
				logVerbose(opts.Verbose, "engine not healthy yet: %s", reason)
				lastReason = reason
				lastReasonAt = now
			}
		}
		if time.Now().After(deadline) {
			if tailer != nil {
				tailer.Stop()
			}
			return ConnectResult{}, fmt.Errorf("engine did not become healthy within startup timeout")
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func logVerbose(enabled bool, format string, args ...any) {
	if !enabled {
		return
	}
	fmt.Fprintf(os.Stderr, format+"\n", args...)
}

func loadHealthyState(ctx context.Context, enginePath string, timeout time.Duration) (EngineState, bool) {
	state, ok, _ := loadHealthyStateWithReason(ctx, enginePath, timeout)
	return state, ok
}

func checkHealth(ctx context.Context, endpoint string, timeout time.Duration) (client.HealthResponse, error) {
	client := client.New(endpoint, client.Options{Timeout: timeout})
	return client.Health(ctx)
}

func loadHealthyStateWithReason(ctx context.Context, enginePath string, timeout time.Duration) (EngineState, bool, string) {
	state, err := ReadEngineState(enginePath)
	if err != nil {
		return EngineState{}, false, fmt.Sprintf("engine.json read failed: %v", err)
	}
	health, healthErr := checkHealth(ctx, state.Endpoint, timeout)
	if healthErr != nil {
		return EngineState{}, false, fmt.Sprintf("health check failed: %v", healthErr)
	}
	pidRunning := processExists(state.PID)
	if isWindows {
		pidRunning = true
	}
	if state.InstanceID != "" && health.InstanceID != "" && state.InstanceID != health.InstanceID {
		return EngineState{}, false, fmt.Sprintf("instanceId mismatch (state=%s health=%s)", state.InstanceID, health.InstanceID)
	}
	if state.PID > 0 && !pidRunning {
		return EngineState{}, false, fmt.Sprintf("engine pid not running (pid=%d)", state.PID)
	}
	return state, true, ""
}

func cacheKey(opts ConnectOptions) string {
	stateDir := strings.TrimSpace(opts.StateDir)
	if stateDir == "" {
		return ""
	}
	return filepath.Clean(stateDir)
}

func loadCachedEngineState(opts ConnectOptions) (EngineState, bool) {
	key := cacheKey(opts)
	if key == "" {
		return EngineState{}, false
	}
	cachedEngineStates.mu.Lock()
	defer cachedEngineStates.mu.Unlock()
	state, ok := cachedEngineStates.m[key]
	return state, ok
}

func storeCachedEngineState(opts ConnectOptions, state EngineState) {
	key := cacheKey(opts)
	if key == "" || strings.TrimSpace(state.Endpoint) == "" {
		return
	}
	cachedEngineStates.mu.Lock()
	cachedEngineStates.m[key] = state
	cachedEngineStates.mu.Unlock()
}

func tryHealthyState(ctx context.Context, state EngineState, timeout time.Duration, verbose bool) (EngineState, bool) {
	health, err := checkHealth(ctx, state.Endpoint, timeout)
	if err != nil {
		return EngineState{}, false
	}
	if state.InstanceID != "" && health.InstanceID != "" && state.InstanceID != health.InstanceID {
		return EngineState{}, false
	}
	if ok, _, reason := checkAuthWithToken(ctx, state.Endpoint, state.AuthToken, timeout); ok {
		logVerbose(verbose, "engine healthy at %s", state.Endpoint)
		return state, true
	} else if reason != "" {
		logVerbose(verbose, "auth probe failed: %s", reason)
	}
	return EngineState{}, false
}

func refreshStateOnAuthRejected(ctx context.Context, enginePath string, state EngineState, timeout time.Duration, verbose bool) (EngineState, bool) {
	if ok, authRejected, reason := checkAuthWithToken(ctx, state.Endpoint, state.AuthToken, timeout); ok {
		return state, true
	} else if !authRejected {
		logVerbose(verbose, "auth probe failed: %s", reason)
		return EngineState{}, false
	}
	logVerbose(verbose, "auth rejected, re-reading engine.json")
	refreshed, ok, _ := loadHealthyStateWithReason(ctx, enginePath, timeout)
	if !ok {
		return EngineState{}, false
	}
	if ok, _, _ := checkAuthWithToken(ctx, refreshed.Endpoint, refreshed.AuthToken, timeout); !ok {
		return EngineState{}, false
	}
	return refreshed, true
}

func checkAuthWithToken(ctx context.Context, endpoint, token string, timeout time.Duration) (ok bool, authRejected bool, reason string) {
	if strings.TrimSpace(endpoint) == "" {
		return false, false, "empty endpoint"
	}
	// If auth is not configured, health check is sufficient.
	if strings.TrimSpace(token) == "" {
		return true, false, ""
	}
	base := strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if !strings.HasPrefix(base, "http://") && !strings.HasPrefix(base, "https://") {
		base = "http://" + base
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/v1/prepare-jobs", nil)
	if err != nil {
		return false, false, err.Error()
	}
	req.Header.Set("Authorization", "Bearer "+strings.TrimSpace(token))
	cli := &http.Client{Timeout: timeout}
	resp, err := cli.Do(req)
	if err != nil {
		return false, false, err.Error()
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return false, true, resp.Status
	}
	return true, false, ""
}

func formatEngineExit(err error) error {
	if err == nil {
		return fmt.Errorf("engine exited before becoming healthy (exit code 0)")
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return fmt.Errorf("engine exited before becoming healthy (exit code %d)", exitErr.ExitCode())
	}
	return fmt.Errorf("engine exited before becoming healthy: %v", err)
}

var runWSLCommandFn = runWSLCommand
var runHostCommandFn = runHostCommand

func ensureWSLStoreMount(ctx context.Context, opts ConnectOptions) error {
	if strings.TrimSpace(opts.WSLDistro) == "" {
		return nil
	}
	unit := strings.TrimSpace(opts.WSLMountUnit)
	fstype := strings.TrimSpace(opts.WSLMountFSType)
	storeDir := strings.TrimSpace(opts.EngineStoreDir)
	if unit == "" || storeDir == "" {
		return nil
	}
	if fstype == "" {
		fstype = "btrfs"
	}
	if ctx == nil {
		ctx = context.Background()
	}
	logVerbose(opts.Verbose, "ensuring WSL store mount (%s) via %s", fstype, unit)

	var mountUnitErr error
	if err := ensureWSLMountUnitActive(ctx, opts.WSLDistro, unit); err != nil {
		mountUnitErr = err
		if runtime.GOOS == "windows" && strings.TrimSpace(opts.WSLVHDXPath) != "" {
			if attachErr := attachVHDXToWSL(ctx, opts.WSLVHDXPath, opts.Verbose); attachErr != nil {
				return appendWSLMountLogs(ctx, opts.WSLDistro, unit, attachErr)
			}
			if err := ensureWSLMountUnitActive(ctx, opts.WSLDistro, unit); err != nil {
				mountUnitErr = err
				logVerbose(opts.Verbose, "WSL mount unit not active after attach: %v", err)
			} else {
				mountUnitErr = nil
			}
		} else {
			return err
		}
	}
	out, err := runWSLCommandInInitNamespace(ctx, opts.WSLDistro, "findmnt", "-n", "-o", "FSTYPE", "-T", storeDir)
	if err != nil {
		if mountUnitErr == nil && shouldIgnoreWSLStoreMountVerificationError(opts, err) {
			logVerbose(opts.Verbose, "ignoring transient WSL store verification failure: %v", err)
			return nil
		}
		if mountUnitErr != nil {
			return fmt.Errorf("%v; findmnt failed: %w", mountUnitErr, err)
		}
		return err
	}
	fs := strings.TrimSpace(out)
	if fs == "" {
		if mountUnitErr != nil {
			return fmt.Errorf("%v; WSL store mount is not available (empty fstype)", mountUnitErr)
		}
		return fmt.Errorf("WSL store mount is not available (empty fstype)")
	}
	if fs != fstype {
		if runtime.GOOS == "windows" && strings.TrimSpace(opts.WSLVHDXPath) != "" {
			logVerbose(opts.Verbose, "WSL store mount is %s, retrying attach/start", fs)
			if attachErr := attachVHDXToWSL(ctx, opts.WSLVHDXPath, opts.Verbose); attachErr != nil {
				return appendWSLMountLogs(ctx, opts.WSLDistro, unit, attachErr)
			}
			if err := ensureWSLMountUnitActive(ctx, opts.WSLDistro, unit); err != nil {
				mountUnitErr = err
				logVerbose(opts.Verbose, "WSL mount unit still not active after retry attach: %v", err)
			} else {
				mountUnitErr = nil
			}
			out, err = runWSLCommandInInitNamespace(ctx, opts.WSLDistro, "findmnt", "-n", "-o", "FSTYPE", "-T", storeDir)
			if err != nil {
				if mountUnitErr == nil && shouldIgnoreWSLStoreMountVerificationError(opts, err) {
					logVerbose(opts.Verbose, "ignoring transient WSL store verification retry failure: %v", err)
					return nil
				}
				if mountUnitErr != nil {
					return fmt.Errorf("%v; findmnt retry failed: %w", mountUnitErr, err)
				}
				return err
			}
			fs = strings.TrimSpace(out)
			if fs == "" {
				if mountUnitErr != nil {
					return fmt.Errorf("%v; WSL store mount is not available (empty fstype)", mountUnitErr)
				}
				return fmt.Errorf("WSL store mount is not available (empty fstype)")
			}
			if fs != fstype {
				mismatchErr := fmt.Errorf("WSL store mount is %s, expected %s", fs, fstype)
				if mountUnitErr != nil {
					mismatchErr = fmt.Errorf("%v; %w", mountUnitErr, mismatchErr)
				}
				return appendWSLMountLogs(ctx, opts.WSLDistro, unit, mismatchErr)
			}
			return nil
		}
		return appendWSLMountLogs(ctx, opts.WSLDistro, unit, fmt.Errorf("WSL store mount is %s, expected %s", fs, fstype))
	}
	return nil
}

func runWSLCommand(ctx context.Context, distro string, args ...string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	cmdArgs := []string{"-d", distro, "-u", "root", "--"}
	cmdArgs = append(cmdArgs, args...)
	return runCommandWithSplitOutput(ctx, "wsl.exe", cmdArgs...)
}

func runHostCommand(ctx context.Context, args ...string) (string, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	return runCommandWithSplitOutput(ctx, args[0], args[1:]...)
}

func runCommandWithSplitOutput(ctx context.Context, name string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	out := stdout.String()
	if err == nil {
		return out, nil
	}
	detail := strings.TrimSpace(stderr.String())
	if detail == "" {
		detail = strings.TrimSpace(out)
	}
	if detail != "" {
		return out, fmt.Errorf("%v (%s)", err, detail)
	}
	return out, err
}

func runWSLCommandInInitNamespace(ctx context.Context, distro string, args ...string) (string, error) {
	cmdArgs := append([]string{"nsenter", "-t", "1", "-m", "--"}, args...)
	out, err := runWSLCommandFn(ctx, distro, cmdArgs...)
	if err == nil {
		return out, nil
	}
	if shouldFallbackFromInitNamespace(err) {
		return runWSLCommandFn(ctx, distro, args...)
	}
	return out, err
}

func shouldFallbackFromInitNamespace(err error) bool {
	if err == nil {
		return false
	}
	normalized := normalizeVHDXAttachErrorText(err.Error())
	return strings.Contains(normalized, "commandnotfound") || isTransientWSLServiceErrorNormalized(normalized)
}

func shouldIgnoreWSLStoreMountVerificationError(opts ConnectOptions, err error) bool {
	return isWindows && strings.TrimSpace(opts.WSLVHDXPath) != "" && isTransientWSLServiceError(err)
}

func isTransientWSLServiceError(err error) bool {
	if err == nil {
		return false
	}
	return isTransientWSLServiceErrorNormalized(normalizeVHDXAttachErrorText(err.Error()))
}

func isTransientWSLServiceErrorNormalized(normalized string) bool {
	return strings.Contains(normalized, "wsl_service_e_unexpected") ||
		strings.Contains(normalized, "wslserviceeunexpected") ||
		strings.Contains(normalized, "catastrophicfailure")
}

func ensureWSLMountUnitActive(ctx context.Context, distro, unit string) error {
	out, err := runWSLCommandFn(ctx, distro, "systemctl", "is-active", unit)
	if err == nil && strings.TrimSpace(out) == "active" {
		return nil
	}
	_, startErr := runWSLCommandFn(ctx, distro, "systemctl", "start", "--no-block", unit)
	if startErr != nil {
		return appendWSLMountLogs(ctx, distro, unit, startErr)
	}
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		out, err = runWSLCommandFn(ctx, distro, "systemctl", "is-active", unit)
		if err == nil && strings.TrimSpace(out) == "active" {
			return nil
		}
		time.Sleep(200 * time.Millisecond)
	}
	return appendWSLMountLogs(ctx, distro, unit, fmt.Errorf("WSL mount unit is not active"))
}

func appendWSLMountLogs(ctx context.Context, distro, unit string, err error) error {
	tail, tailErr := runWSLCommandFn(ctx, distro, "journalctl", "-u", unit, "-n", "20", "--no-pager")
	if tailErr == nil && strings.TrimSpace(tail) != "" {
		return fmt.Errorf("%v\n%s", err, strings.TrimSpace(tail))
	}
	return err
}

func attachVHDXToWSL(ctx context.Context, vhdxPath string, verbose bool) error {
	if strings.TrimSpace(vhdxPath) == "" {
		return fmt.Errorf("VHDX path is empty")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	logVerbose(verbose, "attaching VHDX: %s", vhdxPath)
	out, err := runHostCommandFn(ctx, "wsl.exe", "--mount", vhdxPath, "--vhd", "--bare")
	if err != nil {
		if isVHDXAlreadyAttachedError(err, out) {
			logVerbose(verbose, "VHDX already attached: %s", vhdxPath)
			return nil
		}
		if strings.TrimSpace(out) != "" {
			return fmt.Errorf("attach VHDX failed: %v (%s)", err, strings.TrimSpace(out))
		}
		return fmt.Errorf("attach VHDX failed: %v", err)
	}
	return nil
}

func isVHDXAlreadyAttachedError(err error, output string) bool {
	if err == nil {
		return false
	}
	if strings.Contains(normalizeVHDXAttachErrorText(err.Error()), "wsl_e_user_vhd_already_attached") {
		return true
	}
	normalizedOutput := normalizeVHDXAttachErrorText(output)
	return strings.Contains(normalizedOutput, "wsl_e_user_vhd_already_attached") ||
		strings.Contains(normalizedOutput, "alreadyattached")
}

func normalizeVHDXAttachErrorText(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	var builder strings.Builder
	builder.Grow(len(value))
	for _, symbol := range strings.ToLower(value) {
		if unicode.IsLetter(symbol) || unicode.IsDigit(symbol) || symbol == '_' {
			builder.WriteRune(symbol)
		}
	}
	return builder.String()
}
func isExitStatus(err error, code int) bool {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode() == code
	}
	return false
}

type logTail struct {
	stop chan struct{}
	done chan struct{}
	once sync.Once
}

func (t *logTail) Stop() {
	if t == nil {
		return
	}
	t.once.Do(func() {
		close(t.stop)
		<-t.done
	})
}

func startLogTail(path string, out io.Writer) (*logTail, error) {
	if out == nil {
		return nil, fmt.Errorf("log tail output is nil")
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	if _, err := file.Seek(0, io.SeekEnd); err != nil {
		_ = file.Close()
		return nil, err
	}

	tail := &logTail{
		stop: make(chan struct{}),
		done: make(chan struct{}),
	}

	go func() {
		defer close(tail.done)
		defer file.Close()

		buf := make([]byte, 4096)
		for {
			select {
			case <-tail.stop:
				return
			default:
			}

			n, err := file.Read(buf)
			if n > 0 {
				_, _ = out.Write(buf[:n])
			}
			if errors.Is(err, io.EOF) {
				time.Sleep(50 * time.Millisecond)
				continue
			}
			if err != nil {
				return
			}
		}
	}()

	return tail, nil
}
