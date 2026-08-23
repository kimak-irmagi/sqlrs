package daemon

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestConnectOrStartWithExplicitEndpoint(t *testing.T) {
	result, err := ConnectOrStart(context.Background(), ConnectOptions{
		Endpoint: "http://localhost:1234",
	})
	if err != nil {
		t.Fatalf("ConnectOrStart: %v", err)
	}
	if result.Endpoint != "http://localhost:1234" {
		t.Fatalf("unexpected endpoint: %q", result.Endpoint)
	}
}

func TestEnsureWSLStoreMountSkippedWithoutMetadata(t *testing.T) {
	prev := runWSLCommandFn
	runWSLCommandFn = func(context.Context, string, ...string) (string, error) {
		t.Fatalf("unexpected wsl command")
		return "", nil
	}
	t.Cleanup(func() { runWSLCommandFn = prev })

	if err := ensureWSLStoreMount(context.Background(), ConnectOptions{WSLDistro: "Ubuntu"}); err != nil {
		t.Fatalf("ensureWSLStoreMount: %v", err)
	}
}

func TestEnsureWSLStoreMountAlreadyMounted(t *testing.T) {
	prev := runWSLCommandFn
	calls := 0
	runWSLCommandFn = func(ctx context.Context, distro string, args ...string) (string, error) {
		calls++
		switch calls {
		case 1:
			if args[0] != "systemctl" || args[1] != "is-active" {
				t.Fatalf("expected systemctl is-active, got %+v", args)
			}
			return "active\n", nil
		case 2:
			if args[0] != "nsenter" {
				t.Fatalf("expected nsenter, got %+v", args)
			}
			return "btrfs\n", nil
		default:
			return "", nil
		}
	}
	t.Cleanup(func() { runWSLCommandFn = prev })

	err := ensureWSLStoreMount(context.Background(), ConnectOptions{
		WSLDistro:      "Ubuntu",
		EngineStoreDir: "/mnt/sqlrs/store",
		WSLMountUnit:   "sqlrs-state-store.mount",
		WSLMountFSType: "btrfs",
	})
	if err != nil {
		t.Fatalf("ensureWSLStoreMount: %v", err)
	}
}

func TestEnsureWSLStoreMountWrongFSType(t *testing.T) {
	prev := runWSLCommandFn
	calls := 0
	runWSLCommandFn = func(ctx context.Context, distro string, args ...string) (string, error) {
		calls++
		switch calls {
		case 1:
			if args[0] != "systemctl" || args[1] != "is-active" {
				t.Fatalf("expected systemctl is-active, got %+v", args)
			}
			return "active\n", nil
		case 2:
			if args[0] != "nsenter" {
				t.Fatalf("expected nsenter, got %+v", args)
			}
			return "ext4\n", nil
		default:
			return "", nil
		}
	}
	t.Cleanup(func() { runWSLCommandFn = prev })

	err := ensureWSLStoreMount(context.Background(), ConnectOptions{
		WSLDistro:      "Ubuntu",
		EngineStoreDir: "/mnt/sqlrs/store",
		WSLMountUnit:   "sqlrs-state-store.mount",
		WSLMountFSType: "btrfs",
	})
	if err == nil || !strings.Contains(err.Error(), "expected btrfs") {
		t.Fatalf("expected fstype error, got %v", err)
	}
}

func TestEnsureWSLStoreMountStartsWhenInactive(t *testing.T) {
	prev := runWSLCommandFn
	calls := 0
	runWSLCommandFn = func(ctx context.Context, distro string, args ...string) (string, error) {
		calls++
		switch calls {
		case 1:
			if args[0] != "systemctl" || args[1] != "is-active" {
				t.Fatalf("expected systemctl is-active, got %+v", args)
			}
			return "inactive\n", exitError(3)
		case 2:
			if args[0] != "systemctl" || args[1] != "start" || args[2] != "--no-block" {
				t.Fatalf("expected systemctl start --no-block, got %+v", args)
			}
			return "", nil
		case 3:
			if args[0] != "systemctl" || args[1] != "is-active" {
				t.Fatalf("expected systemctl is-active, got %+v", args)
			}
			return "active\n", nil
		case 4:
			if args[0] != "nsenter" {
				t.Fatalf("expected nsenter, got %+v", args)
			}
			return "btrfs\n", nil
		default:
			return "", nil
		}
	}
	t.Cleanup(func() { runWSLCommandFn = prev })

	err := ensureWSLStoreMount(context.Background(), ConnectOptions{
		WSLDistro:      "Ubuntu",
		EngineStoreDir: "/mnt/sqlrs/store",
		WSLMountUnit:   "sqlrs-state-store.mount",
		WSLMountFSType: "btrfs",
	})
	if err != nil {
		t.Fatalf("ensureWSLStoreMount: %v", err)
	}
}

func TestEnsureWSLStoreMountInactiveVerificationFails(t *testing.T) {
	prev := runWSLCommandFn
	calls := 0
	runWSLCommandFn = func(ctx context.Context, distro string, args ...string) (string, error) {
		calls++
		switch calls {
		case 1:
			if args[0] != "systemctl" || args[1] != "is-active" {
				t.Fatalf("expected systemctl is-active, got %+v", args)
			}
			return "inactive\n", exitError(3)
		case 2:
			if args[0] != "systemctl" || args[1] != "start" || args[2] != "--no-block" {
				t.Fatalf("expected systemctl start --no-block, got %+v", args)
			}
			return "", nil
		case 3:
			if args[0] != "systemctl" || args[1] != "is-active" {
				t.Fatalf("expected systemctl is-active, got %+v", args)
			}
			return "inactive\n", nil
		default:
			return "", nil
		}
	}
	t.Cleanup(func() { runWSLCommandFn = prev })

	err := ensureWSLStoreMount(context.Background(), ConnectOptions{
		WSLDistro:      "Ubuntu",
		EngineStoreDir: "/mnt/sqlrs/store",
		WSLMountUnit:   "sqlrs-state-store.mount",
		WSLMountFSType: "btrfs",
	})
	if err == nil {
		t.Fatalf("expected mount unit error")
	}
}

func TestEnsureWSLStoreMountFindmntError(t *testing.T) {
	prev := runWSLCommandFn
	calls := 0
	runWSLCommandFn = func(ctx context.Context, distro string, args ...string) (string, error) {
		calls++
		switch calls {
		case 1:
			if args[0] != "systemctl" || args[1] != "is-active" {
				t.Fatalf("expected systemctl is-active, got %+v", args)
			}
			return "active\n", nil
		default:
			if args[0] != "nsenter" {
				t.Fatalf("expected nsenter, got %+v", args)
			}
			return "", errors.New("boom")
		}
	}
	t.Cleanup(func() { runWSLCommandFn = prev })

	err := ensureWSLStoreMount(context.Background(), ConnectOptions{
		WSLDistro:      "Ubuntu",
		EngineStoreDir: "/mnt/sqlrs/store",
		WSLMountUnit:   "sqlrs-state-store.mount",
		WSLMountFSType: "btrfs",
	})
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestShouldFallbackFromInitNamespace(t *testing.T) {
	cases := []error{
		errors.New("command not found"),
		errors.New("exit status 0xffffffff (C a t a s t r o p h i c   f a i l u r e Error code: W s l / S e r v i c e / E _ U N E X P E C T E D)"),
	}
	for _, err := range cases {
		if !shouldFallbackFromInitNamespace(err) {
			t.Fatalf("expected fallback for %v", err)
		}
	}
	if shouldFallbackFromInitNamespace(errors.New("permission denied")) {
		t.Fatalf("did not expect fallback for unrelated error")
	}
}

func TestEnsureWSLStoreMountFallsBackFromInitNamespaceServiceError(t *testing.T) {
	prev := runWSLCommandFn
	calls := 0
	runWSLCommandFn = func(ctx context.Context, distro string, args ...string) (string, error) {
		calls++
		switch calls {
		case 1:
			if args[0] != "systemctl" || args[1] != "is-active" {
				t.Fatalf("expected systemctl is-active, got %+v", args)
			}
			return "active\n", nil
		case 2:
			if args[0] != "nsenter" {
				t.Fatalf("expected nsenter, got %+v", args)
			}
			return "", errors.New("exit status 0xffffffff (C a t a s t r o p h i c   f a i l u r e Error code: W s l / S e r v i c e / E _ U N E X P E C T E D)")
		case 3:
			if args[0] != "findmnt" {
				t.Fatalf("expected direct findmnt fallback, got %+v", args)
			}
			return "btrfs\n", nil
		default:
			return "", nil
		}
	}
	t.Cleanup(func() { runWSLCommandFn = prev })

	err := ensureWSLStoreMount(context.Background(), ConnectOptions{
		WSLDistro:      "Ubuntu",
		EngineStoreDir: "/mnt/sqlrs/store",
		WSLMountUnit:   "sqlrs-state-store.mount",
		WSLMountFSType: "btrfs",
	})
	if err != nil {
		t.Fatalf("ensureWSLStoreMount: %v", err)
	}
}

func TestIsTransientWSLServiceError(t *testing.T) {
	if !isTransientWSLServiceError(errors.New("exit status 0xffffffff (C a t a s t r o p h i c   f a i l u r e Error code: W s l / S e r v i c e / E _ U N E X P E C T E D)")) {
		t.Fatalf("expected transient WSL service error")
	}
	if isTransientWSLServiceError(errors.New("permission denied")) {
		t.Fatalf("did not expect transient classification for unrelated error")
	}
}

func TestEnsureWSLStoreMountIgnoresTransientFindmntServiceErrorOnWindowsWithVHDX(t *testing.T) {
	prevWSL := runWSLCommandFn
	prevWindows := isWindows
	runWSLCommandFn = func(ctx context.Context, distro string, args ...string) (string, error) {
		switch {
		case len(args) >= 2 && args[0] == "systemctl" && args[1] == "is-active":
			return "active\n", nil
		case len(args) >= 1 && args[0] == "nsenter":
			return "", errors.New("exit status 0xffffffff (C a t a s t r o p h i c   f a i l u r e Error code: W s l / S e r v i c e / E _ U N E X P E C T E D)")
		case len(args) >= 1 && args[0] == "findmnt":
			return "", errors.New("exit status 0xffffffff (C a t a s t r o p h i c   f a i l u r e Error code: W s l / S e r v i c e / E _ U N E X P E C T E D)")
		default:
			return "", nil
		}
	}
	isWindows = true
	t.Cleanup(func() {
		runWSLCommandFn = prevWSL
		isWindows = prevWindows
	})

	err := ensureWSLStoreMount(context.Background(), ConnectOptions{
		WSLDistro:      "Ubuntu",
		EngineStoreDir: "/mnt/sqlrs/store",
		WSLMountUnit:   "sqlrs-state-store.mount",
		WSLMountFSType: "btrfs",
		WSLVHDXPath:    "C:\\temp\\store.vhdx",
	})
	if err != nil {
		t.Fatalf("ensureWSLStoreMount: %v", err)
	}
}
func TestConnectOrStartAutostartDisabled(t *testing.T) {
	temp := t.TempDir()
	_, err := ConnectOrStart(context.Background(), ConnectOptions{
		Endpoint:  "auto",
		Autostart: false,
		StateDir:  temp,
	})
	if err == nil || !strings.Contains(err.Error(), "not running") {
		t.Fatalf("expected not running error, got %v", err)
	}
}

func TestConnectOrStartMissingDaemonPath(t *testing.T) {
	temp := t.TempDir()
	_, err := ConnectOrStart(context.Background(), ConnectOptions{
		Endpoint:  "auto",
		Autostart: true,
		RunDir:    filepath.Join(temp, "run"),
		StateDir:  temp,
	})
	if err == nil || !strings.Contains(err.Error(), "resolve host engine") {
		t.Fatalf("expected host discovery error, got %v", err)
	}
}

func TestConnectOrStartMissingRunDir(t *testing.T) {
	temp := t.TempDir()
	_, err := ConnectOrStart(context.Background(), ConnectOptions{
		Endpoint:   "auto",
		Autostart:  true,
		DaemonPath: "sqlrs-engine",
		StateDir:   temp,
	})
	if err == nil || !strings.Contains(err.Error(), "runDir") {
		t.Fatalf("expected runDir error, got %v", err)
	}
}

func TestConnectOrStartReturnsHealthyState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/health" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"ok":true,"instanceId":"inst"}`)
	}))
	defer server.Close()

	temp := t.TempDir()
	statePath := filepath.Join(temp, "engine.json")
	state := EngineState{
		Endpoint:   server.URL,
		InstanceID: "inst",
		AuthToken:  "token",
	}
	if err := WriteEngineState(statePath, state); err != nil {
		t.Fatalf("WriteEngineState: %v", err)
	}

	result, err := ConnectOrStart(context.Background(), ConnectOptions{
		Endpoint:      "auto",
		Autostart:     false,
		StateDir:      temp,
		ClientTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("ConnectOrStart: %v", err)
	}
	if result.Endpoint != server.URL || result.AuthToken != "token" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestConnectOrStartRefreshesEngineStateOnAuthRejected(t *testing.T) {
	temp := t.TempDir()
	statePath := filepath.Join(temp, "engine.json")
	var mu sync.Mutex
	updated := false
	var server *httptest.Server
	server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/health":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"ok":true,"instanceId":"inst"}`)
		case "/v1/prepare-jobs":
			auth := strings.TrimSpace(r.Header.Get("Authorization"))
			if auth == "Bearer token-new" {
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, "[]")
				return
			}
			mu.Lock()
			if !updated {
				updated = true
				_ = WriteEngineState(statePath, EngineState{
					Endpoint:   server.URL,
					InstanceID: "inst",
					AuthToken:  "token-new",
				})
			}
			mu.Unlock()
			w.WriteHeader(http.StatusForbidden)
			return
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	if err := WriteEngineState(statePath, EngineState{
		Endpoint:   server.URL,
		InstanceID: "inst",
		AuthToken:  "token-old",
	}); err != nil {
		t.Fatalf("WriteEngineState: %v", err)
	}

	result, err := ConnectOrStart(context.Background(), ConnectOptions{
		Endpoint:      "auto",
		Autostart:     false,
		StateDir:      temp,
		ClientTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("ConnectOrStart: %v", err)
	}
	if result.AuthToken != "token-new" {
		t.Fatalf("expected refreshed token, got %+v", result)
	}
}

func TestCheckAuthWithTokenAcceptsEndpointWithoutScheme(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/prepare-jobs" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if strings.TrimSpace(r.Header.Get("Authorization")) != "Bearer token" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, "[]")
	}))
	defer server.Close()

	endpoint := strings.TrimPrefix(server.URL, "http://")
	ok, rejected, reason := checkAuthWithToken(context.Background(), endpoint, "token", time.Second)
	if !ok || rejected {
		t.Fatalf("expected auth probe success, got ok=%v rejected=%v reason=%q", ok, rejected, reason)
	}
}

func TestConnectOrStartUsesCachedStateBeforeEngineJSON(t *testing.T) {
	temp := t.TempDir()
	statePath := filepath.Join(temp, "engine.json")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/health":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"ok":true,"instanceId":"inst"}`)
		case "/v1/prepare-jobs":
			if strings.TrimSpace(r.Header.Get("Authorization")) != "Bearer token" {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, "[]")
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	if err := WriteEngineState(statePath, EngineState{
		Endpoint:   strings.TrimPrefix(server.URL, "http://"),
		InstanceID: "inst",
		AuthToken:  "token",
	}); err != nil {
		t.Fatalf("WriteEngineState: %v", err)
	}

	first, err := ConnectOrStart(context.Background(), ConnectOptions{
		Endpoint:      "auto",
		Autostart:     false,
		StateDir:      temp,
		ClientTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("first ConnectOrStart: %v", err)
	}
	if strings.TrimSpace(first.Endpoint) == "" {
		t.Fatalf("expected non-empty endpoint: %+v", first)
	}

	if err := os.Remove(statePath); err != nil {
		t.Fatalf("remove engine.json: %v", err)
	}

	second, err := ConnectOrStart(context.Background(), ConnectOptions{
		Endpoint:      "auto",
		Autostart:     false,
		StateDir:      temp,
		ClientTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("second ConnectOrStart: %v", err)
	}
	if second.Endpoint != first.Endpoint {
		t.Fatalf("expected cached endpoint %q, got %q", first.Endpoint, second.Endpoint)
	}
}

func TestConnectOrStartEnsureDirError(t *testing.T) {
	temp := t.TempDir()
	runDir := filepath.Join(temp, "run")
	if err := os.WriteFile(runDir, []byte("x"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	_, err := ConnectOrStart(context.Background(), ConnectOptions{
		Endpoint:   "auto",
		Autostart:  true,
		DaemonPath: "sqlrs-engine",
		RunDir:     runDir,
		StateDir:   temp,
	})
	if err == nil {
		t.Fatalf("expected ensure dir error")
	}
}

func TestConnectOrStartStartError(t *testing.T) {
	temp := t.TempDir()
	runDir := filepath.Join(temp, "run")
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		t.Fatalf("mkdir runDir: %v", err)
	}

	_, err := ConnectOrStart(context.Background(), ConnectOptions{
		Endpoint:       "auto",
		Autostart:      true,
		DaemonPath:     filepath.Join(temp, "missing.exe"),
		RunDir:         runDir,
		StateDir:       temp,
		StartupTimeout: 50 * time.Millisecond,
	})
	if err == nil {
		t.Fatalf("expected start error")
	}
}

func TestLoadHealthyState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/health" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"ok":true,"instanceId":"inst"}`)
	}))
	defer server.Close()

	temp := t.TempDir()
	statePath := filepath.Join(temp, "engine.json")
	state := EngineState{
		Endpoint:   server.URL,
		InstanceID: "inst",
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}
	if err := os.WriteFile(statePath, data, 0o600); err != nil {
		t.Fatalf("write state: %v", err)
	}

	got, ok := loadHealthyState(context.Background(), statePath, 2*time.Second)
	if !ok || got.Endpoint != server.URL {
		t.Fatalf("expected healthy state, got %+v (ok=%v)", got, ok)
	}
}

func TestLoadHealthyStateWithReasonHealthError(t *testing.T) {
	temp := t.TempDir()
	statePath := filepath.Join(temp, "engine.json")
	state := EngineState{
		Endpoint:   "http://127.0.0.1:1",
		InstanceID: "inst",
	}
	data, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("marshal state: %v", err)
	}
	if err := os.WriteFile(statePath, data, 0o600); err != nil {
		t.Fatalf("write state: %v", err)
	}

	_, ok, reason := loadHealthyStateWithReason(context.Background(), statePath, 10*time.Millisecond)
	if ok {
		t.Fatalf("expected not ok")
	}
	if !strings.Contains(reason, "health check failed") {
		t.Fatalf("unexpected reason: %q", reason)
	}
}

func TestLogVerboseWrites(t *testing.T) {
	var buf bytes.Buffer
	old := os.Stderr
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stderr = w
	defer func() {
		_ = w.Close()
		os.Stderr = old
	}()

	logVerbose(true, "hello %s", "world")
	_ = w.Close()
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("read: %v", err)
	}
	if !strings.Contains(buf.String(), "hello world") {
		t.Fatalf("unexpected output: %q", buf.String())
	}
}

func TestFormatEngineExitNil(t *testing.T) {
	err := formatEngineExit(nil)
	if err == nil || !strings.Contains(err.Error(), "exit code 0") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFormatEngineExitCode(t *testing.T) {
	cmd := exitCommand(7)
	execErr := cmd.Run()
	if execErr == nil {
		t.Fatalf("expected exit error")
	}
	err := formatEngineExit(execErr)
	if err == nil || !strings.Contains(err.Error(), "exit code 7") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestFormatEngineExitMessage(t *testing.T) {
	err := formatEngineExit(errors.New("boom"))
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStartLogTailWritesOutput(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "engine.log")
	if err := os.WriteFile(logPath, []byte(""), 0o600); err != nil {
		t.Fatalf("create log: %v", err)
	}

	var out lockedBuffer
	tailer, err := startLogTail(logPath, &out)
	if err != nil {
		t.Fatalf("startLogTail: %v", err)
	}
	defer tailer.Stop()

	if err := appendLog(logPath, "hello\n"); err != nil {
		t.Fatalf("append log: %v", err)
	}

	if err := waitForSubstring(&out, "hello", time.Second); err != nil {
		t.Fatalf("expected tail output: %v", err)
	}
}

func TestStartLogTailStopAllowsWrites(t *testing.T) {
	dir := t.TempDir()
	logPath := filepath.Join(dir, "engine.log")
	if err := os.WriteFile(logPath, []byte(""), 0o600); err != nil {
		t.Fatalf("create log: %v", err)
	}

	var out lockedBuffer
	tailer, err := startLogTail(logPath, &out)
	if err != nil {
		t.Fatalf("startLogTail: %v", err)
	}
	tailer.Stop()

	if err := appendLog(logPath, "after\n"); err != nil {
		t.Fatalf("append log after stop: %v", err)
	}

	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read log: %v", err)
	}
	if !strings.Contains(string(data), "after") {
		t.Fatalf("expected log to contain appended output")
	}
}

func exitCommand(code int) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.Command("cmd.exe", "/c", "exit", "/b", fmt.Sprintf("%d", code))
	}
	return exec.Command("sh", "-c", fmt.Sprintf("exit %d", code))
}

func exitError(code int) error {
	cmd := exitCommand(code)
	err := cmd.Run()
	if err == nil {
		return errors.New("expected exit error")
	}
	return err
}

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func appendLog(path, line string) error {
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.WriteString(line)
	return err
}

func waitForSubstring(buf *lockedBuffer, substr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if strings.Contains(buf.String(), substr) {
			return nil
		}
		time.Sleep(10 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for %q", substr)
}
