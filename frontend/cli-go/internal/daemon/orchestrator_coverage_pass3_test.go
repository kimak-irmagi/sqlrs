package daemon

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"
)

func TestCacheKeyReturnsEmptyForBlankStateDir(t *testing.T) {
	clearCachedEngineStates()
	t.Cleanup(clearCachedEngineStates)

	opts := ConnectOptions{StateDir: "  "}
	if got := cacheKey(opts); got != "" {
		t.Fatalf("expected empty cache key, got %q", got)
	}
	storeCachedEngineState(opts, EngineState{Endpoint: "http://cached"})
	if _, ok := loadCachedEngineState(opts); ok {
		t.Fatalf("expected blank state dir to bypass cache")
	}
}

func TestEnsureWSLStoreMountRetryAttachRecoversExpectedFSType(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only path")
	}
	prevWSL := runWSLCommandFn
	prevHost := runHostCommandFn
	defer func() {
		runWSLCommandFn = prevWSL
		runHostCommandFn = prevHost
	}()

	isActiveCalls := 0
	findmntCalls := 0
	runHostCommandFn = func(ctx context.Context, args ...string) (string, error) {
		return "", nil
	}
	runWSLCommandFn = func(ctx context.Context, distro string, args ...string) (string, error) {
		switch {
		case len(args) >= 2 && args[0] == "systemctl" && args[1] == "is-active":
			isActiveCalls++
			switch isActiveCalls {
			case 1:
				return "inactive\n", exitError(3)
			default:
				return "active\n", nil
			}
		case len(args) >= 2 && args[0] == "systemctl" && args[1] == "start":
			return "", errors.New("start failed")
		case len(args) >= 1 && args[0] == "journalctl":
			return "mount tail", nil
		case len(args) >= 1 && args[0] == "nsenter":
			findmntCalls++
			if findmntCalls == 1 {
				return "ext4\n", nil
			}
			return "btrfs\n", nil
		default:
			return "", nil
		}
	}

	err := ensureWSLStoreMount(nil, ConnectOptions{
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

func TestEnsureWSLStoreMountFindmntRetryFailureAfterAttach(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only path")
	}
	prevWSL := runWSLCommandFn
	prevHost := runHostCommandFn
	defer func() {
		runWSLCommandFn = prevWSL
		runHostCommandFn = prevHost
	}()

	findmntCalls := 0
	runHostCommandFn = func(ctx context.Context, args ...string) (string, error) {
		return "", nil
	}
	runWSLCommandFn = func(ctx context.Context, distro string, args ...string) (string, error) {
		switch {
		case len(args) >= 2 && args[0] == "systemctl" && args[1] == "is-active":
			return "active\n", nil
		case len(args) >= 1 && args[0] == "nsenter":
			findmntCalls++
			if findmntCalls == 1 {
				return "ext4\n", nil
			}
			return "", errors.New("findmnt retry failed")
		default:
			return "", nil
		}
	}

	err := ensureWSLStoreMount(context.Background(), ConnectOptions{
		WSLDistro:      "Ubuntu",
		EngineStoreDir: "/mnt/sqlrs/store",
		WSLMountUnit:   "sqlrs-state-store.mount",
		WSLMountFSType: "btrfs",
		WSLVHDXPath:    "C:\\temp\\store.vhdx",
	})
	if err == nil || !strings.Contains(err.Error(), "findmnt retry failed") {
		t.Fatalf("expected findmnt retry error, got %v", err)
	}
}

func TestEnsureWSLStoreMountEmptyFSTypeAfterRetryWithMountUnitError(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("windows-only path")
	}
	prevWSL := runWSLCommandFn
	prevHost := runHostCommandFn
	defer func() {
		runWSLCommandFn = prevWSL
		runHostCommandFn = prevHost
	}()

	findmntCalls := 0
	isActiveCalls := 0
	runHostCommandFn = func(ctx context.Context, args ...string) (string, error) {
		return "", nil
	}
	runWSLCommandFn = func(ctx context.Context, distro string, args ...string) (string, error) {
		switch {
		case len(args) >= 2 && args[0] == "systemctl" && args[1] == "is-active":
			isActiveCalls++
			if isActiveCalls == 1 {
				return "active\n", nil
			}
			return "inactive\n", exitError(3)
		case len(args) >= 2 && args[0] == "systemctl" && args[1] == "start":
			return "", errors.New("start failed")
		case len(args) >= 1 && args[0] == "journalctl":
			return "mount tail", nil
		case len(args) >= 1 && args[0] == "nsenter":
			findmntCalls++
			if findmntCalls == 1 {
				return "ext4\n", nil
			}
			return "\n", nil
		default:
			return "", nil
		}
	}

	err := ensureWSLStoreMount(context.Background(), ConnectOptions{
		WSLDistro:      "Ubuntu",
		EngineStoreDir: "/mnt/sqlrs/store",
		WSLMountUnit:   "sqlrs-state-store.mount",
		WSLMountFSType: "btrfs",
		WSLVHDXPath:    "C:\\temp\\store.vhdx",
	})
	if err == nil || !strings.Contains(err.Error(), "empty fstype") || !strings.Contains(err.Error(), "mount tail") {
		t.Fatalf("expected empty fstype error with mount tail, got %v", err)
	}
}

func TestIsVHDXAlreadyAttachedErrorNilError(t *testing.T) {
	if isVHDXAlreadyAttachedError(nil, "WSL_E_USER_VHD_ALREADY_ATTACHED") {
		t.Fatalf("expected nil error to be treated as not attached")
	}
}

func TestConnectOrStartWaitsForHealthyDaemon(t *testing.T) {
	clearCachedEngineStates()
	defer clearCachedEngineStates()

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/health":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"ok":true,"instanceId":"inst-1"}`))
		case "/v1/prepare-jobs":
			_, _ = w.Write([]byte("[]"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	stateDir, err := os.MkdirTemp("", "sqlrs-daemon-healthy-*")
	if err != nil {
		t.Fatalf("MkdirTemp: %v", err)
	}
	t.Cleanup(func() {
		_ = waitUntilFileUnlockedAndRemove(filepath.Join(stateDir, "logs", "engine.log"), 15*time.Second)
		_ = os.RemoveAll(stateDir)
	})
	runDir := filepath.Join(stateDir, "run")
	if err := os.MkdirAll(runDir, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}

	daemonPath := writeHealthyDaemonScript(t, stateDir, server.URL, "inst-1", "token-1")
	result, err := ConnectOrStart(context.Background(), ConnectOptions{
		Endpoint:       "auto",
		Autostart:      true,
		DaemonPath:     daemonPath,
		RunDir:         runDir,
		StateDir:       stateDir,
		ClientTimeout:  100 * time.Millisecond,
		StartupTimeout: 10 * time.Second,
		Verbose:        true,
	})
	if err != nil {
		t.Fatalf("ConnectOrStart: %v", err)
	}
	if result.Endpoint != server.URL || result.AuthToken != "token-1" {
		t.Fatalf("unexpected connect result: %+v", result)
	}
}

func writeHealthyDaemonScript(t *testing.T, dir, endpoint, instanceID, token string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		path := filepath.Join(dir, "daemon-healthy.cmd")
		content := "@echo off\r\n" +
			"set state=\r\n" +
			":parse\r\n" +
			"if \"%~1\"==\"\" goto parsed\r\n" +
			"if \"%~1\"==\"--write-engine-json\" (\r\n" +
			"  set state=%~2\r\n" +
			"  shift\r\n" +
			")\r\n" +
			"shift\r\n" +
			"goto parse\r\n" +
			":parsed\r\n" +
			"ping -n 2 127.0.0.1 >nul\r\n" +
			fmt.Sprintf("> \"%%state%%\" echo {\"endpoint\":\"%s\",\"instanceId\":\"%s\",\"authToken\":\"%s\"}\r\n", endpoint, instanceID, token) +
			"timeout /t 2 /nobreak >nul\r\n"
		if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
		return path
	}

	path := filepath.Join(dir, "daemon-healthy.sh")
	content := "#!/bin/sh\n" +
		"state=\"\"\n" +
		"while [ $# -gt 0 ]; do\n" +
		"  if [ \"$1\" = \"--write-engine-json\" ]; then\n" +
		"    state=$2\n" +
		"    shift\n" +
		"  fi\n" +
		"  shift\n" +
		"done\n" +
		"sleep 0.1\n" +
		fmt.Sprintf("printf '%%s' '%s' > \"$state\"\n", fmt.Sprintf(`{"endpoint":"%s","instanceId":"%s","authToken":"%s"}`, endpoint, instanceID, token)) +
		"sleep 1\n"
	if err := os.WriteFile(path, []byte(content), 0o700); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	return path
}
