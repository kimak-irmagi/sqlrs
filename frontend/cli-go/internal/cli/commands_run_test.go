package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sqlrs/cli/internal/client"
	"github.com/sqlrs/cli/internal/daemon"
)

func TestRunRunMissingInstance(t *testing.T) {
	_, err := RunRun(context.Background(), RunOptions{
		Kind: "psql",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "instance") {
		t.Fatalf("expected instance error, got %v", err)
	}
}

func TestRunRunUnknownKind(t *testing.T) {
	_, err := RunRun(context.Background(), RunOptions{
		Kind:        "unknown",
		InstanceRef: "inst",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "unknown run kind") {
		t.Fatalf("expected kind error, got %v", err)
	}
}

func TestRunRunRemoteRequiresEndpoint(t *testing.T) {
	_, err := RunRun(context.Background(), RunOptions{
		Mode:        "remote",
		Kind:        "psql",
		InstanceRef: "inst",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "requires explicit endpoint") {
		t.Fatalf("expected endpoint error, got %v", err)
	}
}

func TestRunRunRemoteStreamsOutput(t *testing.T) {
	var gotRequest client.RunRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/runs" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotRequest)
		w.Header().Set("Content-Type", "application/x-ndjson")
		io.WriteString(w, `{"type":"start","ts":"2026-01-22T00:00:00Z","instance_id":"inst"}`+"\n")
		io.WriteString(w, `{"type":"stdout","ts":"2026-01-22T00:00:01Z","data":"hello"}`+"\n")
		io.WriteString(w, `{"type":"stderr","ts":"2026-01-22T00:00:02Z","data":"warn"}`+"\n")
		io.WriteString(w, `{"type":"exit","ts":"2026-01-22T00:00:03Z","exit_code":0}`+"\n")
	}))
	defer server.Close()

	var out bytes.Buffer
	var errOut bytes.Buffer
	_, err := RunRun(context.Background(), RunOptions{
		Mode:        "remote",
		Endpoint:    server.URL,
		Kind:        "psql",
		InstanceRef: "inst",
		Args:        []string{"-c", "select 1"},
	}, &out, &errOut)
	if err != nil {
		t.Fatalf("RunRun: %v", err)
	}
	if gotRequest.InstanceRef != "inst" || gotRequest.Kind != "psql" {
		t.Fatalf("unexpected request: %+v", gotRequest)
	}
	if out.String() != "hello" {
		t.Fatalf("unexpected stdout: %q", out.String())
	}
	if errOut.String() != "warn" {
		t.Fatalf("unexpected stderr: %q", errOut.String())
	}
}

func TestRunRunResolvesUniqueInstancePrefix(t *testing.T) {
	const fullID = "abcdef1234567890abcdef1234567890"
	var exactCalls, listCalls, runCalls int
	var gotRequest client.RunRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/instances/ABCDEF12":
			exactCalls++
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/instances":
			listCalls++
			if got := r.URL.Query().Get("id_prefix"); got != "abcdef12" {
				t.Errorf("id_prefix = %q, want abcdef12", got)
			}
			_ = json.NewEncoder(w).Encode([]client.InstanceEntry{{InstanceID: fullID}})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/runs":
			runCalls++
			_ = json.NewDecoder(r.Body).Decode(&gotRequest)
			w.Header().Set("Content-Type", "application/x-ndjson")
			io.WriteString(w, `{"type":"exit","exit_code":0}`+"\n")
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	_, err := RunRun(context.Background(), RunOptions{
		Mode:        "remote",
		Endpoint:    server.URL,
		Kind:        "psql",
		InstanceRef: "ABCDEF12",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("RunRun: %v", err)
	}
	if exactCalls != 1 || listCalls != 1 || runCalls != 1 {
		t.Fatalf("calls: exact=%d list=%d run=%d", exactCalls, listCalls, runCalls)
	}
	if gotRequest.InstanceRef != fullID {
		t.Fatalf("instance_ref = %q, want %q", gotRequest.InstanceRef, fullID)
	}
}

func TestRunRunPrefixShapedExactNameWins(t *testing.T) {
	const fullID = "11111111111111111111111111111111"
	var listCalls int
	var gotRequest client.RunRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/instances/deadbeef":
			_ = json.NewEncoder(w).Encode(client.InstanceEntry{InstanceID: fullID})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/instances":
			listCalls++
			_ = json.NewEncoder(w).Encode([]client.InstanceEntry{})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/runs":
			_ = json.NewDecoder(r.Body).Decode(&gotRequest)
			io.WriteString(w, `{"type":"exit","exit_code":0}`+"\n")
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	_, err := RunRun(context.Background(), RunOptions{
		Mode:        "remote",
		Endpoint:    server.URL,
		Kind:        "psql",
		InstanceRef: "deadbeef",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("RunRun: %v", err)
	}
	if listCalls != 0 {
		t.Fatalf("prefix list called %d times after exact match", listCalls)
	}
	if gotRequest.InstanceRef != fullID {
		t.Fatalf("instance_ref = %q, want %q", gotRequest.InstanceRef, fullID)
	}
}

func TestRunRunRejectsAmbiguousInstancePrefix(t *testing.T) {
	var runCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/instances/deadbeef":
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/instances":
			_ = json.NewEncoder(w).Encode([]client.InstanceEntry{
				{InstanceID: "deadbeef111111111111111111111111"},
				{InstanceID: "deadbeef222222222222222222222222"},
			})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/runs":
			runCalls++
			io.WriteString(w, `{"type":"exit","exit_code":0}`+"\n")
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	_, err := RunRun(context.Background(), RunOptions{
		Mode:        "remote",
		Endpoint:    server.URL,
		Kind:        "psql",
		InstanceRef: "deadbeef",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	var ambiguous *AmbiguousPrefixError
	if !errors.As(err, &ambiguous) {
		t.Fatalf("expected AmbiguousPrefixError, got %v", err)
	}
	if runCalls != 0 {
		t.Fatalf("run called %d times for ambiguous prefix", runCalls)
	}
}

func TestRunRunRejectsMissingInstancePrefix(t *testing.T) {
	var runCalls int
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/instances/cafebabe":
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/instances":
			_ = json.NewEncoder(w).Encode([]client.InstanceEntry{})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/runs":
			runCalls++
			io.WriteString(w, `{"type":"exit","exit_code":0}`+"\n")
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	_, err := RunRun(context.Background(), RunOptions{
		Mode:        "remote",
		Endpoint:    server.URL,
		Kind:        "psql",
		InstanceRef: "cafebabe",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "instance not found") {
		t.Fatalf("expected instance not found, got %v", err)
	}
	if runCalls != 0 {
		t.Fatalf("run called %d times for missing prefix", runCalls)
	}
}

func TestRunRunPropagatesInstanceLookupErrors(t *testing.T) {
	tests := []struct {
		name      string
		exactCode int
		listCode  int
	}{
		{name: "exact lookup", exactCode: http.StatusInternalServerError},
		{name: "prefix list", exactCode: http.StatusNotFound, listCode: http.StatusInternalServerError},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			var runCalls int
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/v1/instances/deadbeef":
					w.WriteHeader(test.exactCode)
				case r.Method == http.MethodGet && r.URL.Path == "/v1/instances":
					w.WriteHeader(test.listCode)
				case r.Method == http.MethodPost && r.URL.Path == "/v1/runs":
					runCalls++
					io.WriteString(w, `{"type":"exit","exit_code":0}`+"\n")
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()

			_, err := RunRun(context.Background(), RunOptions{
				Mode:        "remote",
				Endpoint:    server.URL,
				Kind:        "psql",
				InstanceRef: "deadbeef",
			}, &bytes.Buffer{}, &bytes.Buffer{})
			if err == nil {
				t.Fatal("expected lookup error")
			}
			if runCalls != 0 {
				t.Fatalf("run called %d times after lookup error", runCalls)
			}
		})
	}
}

func TestRunRunLeavesNonPrefixNameUnchanged(t *testing.T) {
	var requestCount int
	var gotRequest client.RunRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestCount++
		if r.Method != http.MethodPost || r.URL.Path != "/v1/runs" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&gotRequest)
		io.WriteString(w, `{"type":"exit","exit_code":0}`+"\n")
	}))
	defer server.Close()

	_, err := RunRun(context.Background(), RunOptions{
		Mode:        "remote",
		Endpoint:    server.URL,
		Kind:        "psql",
		InstanceRef: "staging",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("RunRun: %v", err)
	}
	if requestCount != 1 || gotRequest.InstanceRef != "staging" {
		t.Fatalf("requests=%d instance_ref=%q", requestCount, gotRequest.InstanceRef)
	}
}

func TestRunRunPreservesExactFullInstanceID(t *testing.T) {
	const fullID = "abcdef1234567890abcdef1234567890"
	var listCalls int
	var gotRequest client.RunRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/instances/"+fullID:
			_ = json.NewEncoder(w).Encode(client.InstanceEntry{InstanceID: fullID})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/instances":
			listCalls++
			_ = json.NewEncoder(w).Encode([]client.InstanceEntry{})
		case r.Method == http.MethodPost && r.URL.Path == "/v1/runs":
			_ = json.NewDecoder(r.Body).Decode(&gotRequest)
			io.WriteString(w, `{"type":"exit","exit_code":0}`+"\n")
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	_, err := RunRun(context.Background(), RunOptions{
		Mode:        "remote",
		Endpoint:    server.URL,
		Kind:        "psql",
		InstanceRef: fullID,
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("RunRun: %v", err)
	}
	if listCalls != 0 || gotRequest.InstanceRef != fullID {
		t.Fatalf("list calls=%d instance_ref=%q", listCalls, gotRequest.InstanceRef)
	}
}

func TestRunRunRemoteRunCommandError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodPost && r.URL.Path == "/v1/runs" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	_, err := RunRun(context.Background(), RunOptions{
		Mode:        "remote",
		Endpoint:    server.URL,
		Kind:        "psql",
		InstanceRef: "inst",
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatalf("expected run command error")
	}
}

func TestRunRunUsesSteps(t *testing.T) {
	var gotRequest client.RunRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotRequest)
		w.Header().Set("Content-Type", "application/x-ndjson")
		io.WriteString(w, `{"type":"exit","ts":"2026-01-22T00:00:03Z","exit_code":0}`+"\n")
	}))
	defer server.Close()

	_, err := RunRun(context.Background(), RunOptions{
		Mode:        "remote",
		Endpoint:    server.URL,
		Kind:        "psql",
		InstanceRef: "inst",
		Steps: []RunStep{
			{Args: []string{"-c", "select 1"}},
		},
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("RunRun: %v", err)
	}
	if len(gotRequest.Steps) != 1 || len(gotRequest.Args) != 0 {
		t.Fatalf("unexpected request: %+v", gotRequest)
	}
}

func TestRunRunCommandOverride(t *testing.T) {
	var gotRequest client.RunRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &gotRequest)
		w.Header().Set("Content-Type", "application/x-ndjson")
		io.WriteString(w, `{"type":"exit","ts":"2026-01-22T00:00:03Z","exit_code":0}`+"\n")
	}))
	defer server.Close()

	_, err := RunRun(context.Background(), RunOptions{
		Mode:        "remote",
		Endpoint:    server.URL,
		Kind:        "psql",
		InstanceRef: "inst",
		Command:     "custom-psql",
		Args:        []string{"-c", "select 1"},
	}, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("RunRun: %v", err)
	}
	if gotRequest.Command == nil || *gotRequest.Command != "custom-psql" {
		t.Fatalf("expected command override, got %+v", gotRequest.Command)
	}
}

func TestReadRunStreamErrorEvent(t *testing.T) {
	stream := strings.NewReader(`{"type":"error","ts":"2026-01-22T00:00:01Z","error":{"message":"boom","details":"info"}}` + "\n")
	_, err := readRunStream(stream, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "boom: info") {
		t.Fatalf("expected error, got %v", err)
	}
}

func TestReadRunStreamErrorWithoutMessage(t *testing.T) {
	stream := strings.NewReader(`{"type":"error","ts":"2026-01-22T00:00:01Z","error":{"message":""}}` + "\n")
	_, err := readRunStream(stream, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil || !strings.Contains(err.Error(), "run failed") {
		t.Fatalf("expected run failed, got %v", err)
	}
}

func TestReadRunStreamInvalidJSON(t *testing.T) {
	stream := strings.NewReader(`not-json` + "\n")
	_, err := readRunStream(stream, &bytes.Buffer{}, &bytes.Buffer{})
	if err == nil {
		t.Fatalf("expected json error")
	}
}

func TestReadRunStreamExitWithoutCode(t *testing.T) {
	stream := strings.NewReader(`{"type":"exit","ts":"2026-01-22T00:00:01Z"}` + "\n")
	res, err := readRunStream(stream, &bytes.Buffer{}, &bytes.Buffer{})
	if err != nil {
		t.Fatalf("readRunStream: %v", err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", res.ExitCode)
	}
}

func TestRunClientRemoteRequiresEndpoint(t *testing.T) {
	_, err := runClient(context.Background(), RunOptions{Mode: "remote"})
	if err == nil || !strings.Contains(err.Error(), "requires explicit endpoint") {
		t.Fatalf("expected endpoint error, got %v", err)
	}
}

func TestRunClientInvalidMode(t *testing.T) {
	_, err := runClient(context.Background(), RunOptions{Mode: "bad"})
	if err == nil || !strings.Contains(err.Error(), "invalid mode") {
		t.Fatalf("expected invalid mode, got %v", err)
	}
}

func TestRunClientLocalAutoHealthyState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"ok":true,"version":"dev","instanceId":"id","pid":1}`)
	}))
	defer server.Close()

	stateDir := t.TempDir()
	enginePath := filepath.Join(stateDir, "engine.json")
	if err := daemon.WriteEngineState(enginePath, daemon.EngineState{
		Endpoint:   server.URL,
		PID:        os.Getpid(),
		StartedAt:  "2026-01-22T00:00:00Z",
		AuthToken:  "token",
		Version:    "dev",
		InstanceID: "id",
	}); err != nil {
		t.Fatalf("WriteEngineState: %v", err)
	}

	_, err := runClient(context.Background(), RunOptions{
		Mode:      "local",
		Endpoint:  "auto",
		Autostart: false,
		StateDir:  stateDir,
		Timeout:   time.Second,
	})
	if err != nil {
		t.Fatalf("runClient: %v", err)
	}
}

func TestRunClientLocalAutoAutostartDisabled(t *testing.T) {
	_, err := runClient(context.Background(), RunOptions{
		Mode:      "local",
		Endpoint:  "auto",
		Autostart: false,
		StateDir:  t.TempDir(),
		Timeout:   time.Second,
	})
	if err == nil || !strings.Contains(err.Error(), "local engine is not running") {
		t.Fatalf("expected autostart disabled error, got %v", err)
	}
}

func TestRunClientLocalAutoMissingDiscoverableEngine(t *testing.T) {
	t.Setenv("PATH", t.TempDir())
	_, err := runClient(context.Background(), RunOptions{
		Mode:      "local",
		Endpoint:  "auto",
		Autostart: true,
		RunDir:    t.TempDir(),
		StateDir:  t.TempDir(),
		Timeout:   time.Second,
	})
	if err == nil || !strings.Contains(err.Error(), "resolve host engine") {
		t.Fatalf("expected host discovery error, got %v", err)
	}
}

func TestRunClientLocalAutoMissingRunDir(t *testing.T) {
	_, err := runClient(context.Background(), RunOptions{
		Mode:       "local",
		Endpoint:   "auto",
		Autostart:  true,
		DaemonPath: "sqlrs-engine",
		StateDir:   t.TempDir(),
		Timeout:    time.Second,
	})
	if err == nil || !strings.Contains(err.Error(), "runDir is not configured") {
		t.Fatalf("expected runDir error, got %v", err)
	}
}

func TestRunClientLocalExplicitEndpoint(t *testing.T) {
	client, err := runClient(context.Background(), RunOptions{
		Mode:     "local",
		Endpoint: "http://127.0.0.1:1",
		Timeout:  time.Second,
	})
	if err != nil {
		t.Fatalf("runClient: %v", err)
	}
	if client == nil {
		t.Fatalf("expected client")
	}
}

func TestDeleteInstanceRemote(t *testing.T) {
	var gotPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"dry_run":false,"outcome":"deleted","root":{"kind":"instance","id":"inst","connections":0}}`)
	}))
	defer server.Close()

	err := DeleteInstance(context.Background(), RunOptions{
		Mode:     "remote",
		Endpoint: server.URL,
		Timeout:  time.Second,
	}, "inst")
	if err != nil {
		t.Fatalf("DeleteInstance: %v", err)
	}
	if gotPath != "/v1/instances/inst" {
		t.Fatalf("unexpected path: %s", gotPath)
	}
}

func TestDeleteInstanceEmptyIDNoop(t *testing.T) {
	err := DeleteInstance(context.Background(), RunOptions{}, "")
	if err != nil {
		t.Fatalf("expected nil, got %v", err)
	}
}
