package daemon

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sqlrs/cli/internal/enginebin"
)

func TestMain(m *testing.M) {
	resolveHostEngineForStartFn = func(candidate string) (enginebin.Resolved, error) {
		if strings.TrimSpace(candidate) == "" {
			return enginebin.Resolved{}, errors.New("host engine was not found")
		}
		return enginebin.Resolved{Path: candidate, Kind: enginebin.KindHost, Origin: enginebin.OriginConfig}, nil
	}
	validateWSLEngineForStartFn = func(context.Context, string, string) error { return nil }
	os.Exit(m.Run())
}

func withStartHostResolver(t *testing.T, fn func(string) (enginebin.Resolved, error)) {
	t.Helper()
	previous := resolveHostEngineForStartFn
	resolveHostEngineForStartFn = fn
	t.Cleanup(func() { resolveHostEngineForStartFn = previous })
}

func withStartWSLValidator(t *testing.T, fn func(context.Context, string, string) error) {
	t.Helper()
	previous := validateWSLEngineForStartFn
	validateWSLEngineForStartFn = fn
	t.Cleanup(func() { validateWSLEngineForStartFn = previous })
}

func TestConnectOrStartDefersHostResolutionUntilAutostart(t *testing.T) {
	server, stateDir := healthyEngineFixture(t)
	defer server.Close()
	withStartHostResolver(t, func(string) (enginebin.Resolved, error) {
		t.Fatal("healthy engine must be used without resolving its executable")
		return enginebin.Resolved{}, nil
	})

	result, err := ConnectOrStart(context.Background(), ConnectOptions{
		Endpoint:      "auto",
		Autostart:     true,
		DaemonPath:    "missing-engine",
		RunDir:        stateDir,
		StateDir:      stateDir,
		ClientTimeout: time.Second,
	})
	if err != nil {
		t.Fatalf("ConnectOrStart: %v", err)
	}
	if result.Endpoint != server.URL {
		t.Fatalf("endpoint=%q, want %q", result.Endpoint, server.URL)
	}
}

func TestConnectOrStartDefersHostResolutionUntilLockedHealthRecheck(t *testing.T) {
	healthChecks := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/health" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		healthChecks++
		if healthChecks < 3 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true,"instanceId":"inst"}`)
	}))
	defer server.Close()

	stateDir := t.TempDir()
	if err := WriteEngineState(filepath.Join(stateDir, "engine.json"), EngineState{
		Endpoint:   server.URL,
		InstanceID: "inst",
	}); err != nil {
		t.Fatalf("WriteEngineState: %v", err)
	}
	withStartHostResolver(t, func(string) (enginebin.Resolved, error) {
		t.Fatal("engine that became healthy before the locked recheck must not be resolved")
		return enginebin.Resolved{}, nil
	})

	result, err := ConnectOrStart(context.Background(), ConnectOptions{
		Endpoint:       "auto",
		Autostart:      true,
		DaemonPath:     "missing-engine",
		RunDir:         stateDir,
		StateDir:       stateDir,
		StartupTimeout: time.Second,
		ClientTimeout:  time.Second,
	})
	if err != nil {
		t.Fatalf("ConnectOrStart: %v", err)
	}
	if result.Endpoint != server.URL {
		t.Fatalf("endpoint=%q, want %q", result.Endpoint, server.URL)
	}
}

func TestConnectOrStartMissingWSLEngineReturnsInitHint(t *testing.T) {
	withStartWSLValidator(t, func(context.Context, string, string) error {
		return errors.New("exit status 1")
	})
	stateDir := t.TempDir()
	_, err := ConnectOrStart(context.Background(), ConnectOptions{
		Endpoint:      "auto",
		Autostart:     true,
		DaemonPath:    "/home/user/.local/lib/sqlrs/sqlrs-engine",
		RunDir:        stateDir,
		StateDir:      stateDir,
		WSLDistro:     "Ubuntu",
		ClientTimeout: time.Second,
	})
	if err == nil || !strings.Contains(err.Error(), "rerun sqlrs init local") {
		t.Fatalf("expected init repair hint, got %v", err)
	}
}

func TestConnectOrStartMissingWSLEnginePathReturnsInitHint(t *testing.T) {
	stateDir := t.TempDir()
	_, err := ConnectOrStart(context.Background(), ConnectOptions{
		Endpoint:      "auto",
		Autostart:     true,
		RunDir:        stateDir,
		StateDir:      stateDir,
		WSLDistro:     "Ubuntu",
		ClientTimeout: time.Second,
	})
	if err == nil || !strings.Contains(err.Error(), "rerun sqlrs init local") {
		t.Fatalf("expected init repair hint, got %v", err)
	}
}

func healthyEngineFixture(t *testing.T) (*httptest.Server, string) {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/health" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"ok":true,"instanceId":"inst"}`)
	}))
	stateDir := t.TempDir()
	if err := WriteEngineState(filepath.Join(stateDir, "engine.json"), EngineState{
		Endpoint:   server.URL,
		InstanceID: "inst",
	}); err != nil {
		server.Close()
		t.Fatalf("WriteEngineState: %v", err)
	}
	return server, stateDir
}
