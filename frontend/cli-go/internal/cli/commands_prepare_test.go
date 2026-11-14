package cli

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sqlrs/cli/internal/client"
	"github.com/sqlrs/cli/internal/daemon"
)

func TestRunPrepareRemoteSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/prepare-jobs":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			io.WriteString(w, `{"job_id":"job-1","status_url":"/v1/prepare-jobs/job-1","events_url":"/v1/prepare-jobs/job-1/events"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/prepare-jobs/job-1/events":
			writeEventStream(w, []client.PrepareJobEvent{statusEvent("succeeded")})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/prepare-jobs/job-1":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"job_id":"job-1","status":"succeeded","result":{"dsn":"dsn","instance_id":"inst","state_id":"state","image_id":"image","prepare_kind":"psql","prepare_args_normalized":"-c select 1"}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	result, err := RunPrepare(context.Background(), PrepareOptions{
		Mode:     "remote",
		Endpoint: server.URL,
		ImageID:  "image",
		PsqlArgs: []string{"-c", "select 1"},
		Timeout:  time.Second,
	})
	if err != nil {
		t.Fatalf("RunPrepare: %v", err)
	}
	if result.DSN != "dsn" || result.InstanceID != "inst" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestRunPrepareRemoteRequiresEndpoint(t *testing.T) {
	_, err := RunPrepare(context.Background(), PrepareOptions{Mode: "remote"})
	if err == nil || !strings.Contains(err.Error(), "explicit endpoint") {
		t.Fatalf("expected endpoint error, got %v", err)
	}
}

func TestRunPrepareLocalAutoUsesEngineState(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/v1/health":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"ok":true,"instanceId":"inst"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/prepare-jobs":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			io.WriteString(w, `{"job_id":"job-1","status_url":"/v1/prepare-jobs/job-1","events_url":"/v1/prepare-jobs/job-1/events"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/prepare-jobs/job-1/events":
			writeEventStream(w, []client.PrepareJobEvent{statusEvent("succeeded")})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/prepare-jobs/job-1":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"job_id":"job-1","status":"succeeded","result":{"dsn":"dsn","instance_id":"inst","state_id":"state","image_id":"image","prepare_kind":"psql","prepare_args_normalized":"-c select 1"}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	stateDir := t.TempDir()
	if err := daemon.WriteEngineState(filepath.Join(stateDir, "engine.json"), daemon.EngineState{
		Endpoint:   server.URL,
		AuthToken:  "token",
		InstanceID: "inst",
	}); err != nil {
		t.Fatalf("WriteEngineState: %v", err)
	}

	result, err := RunPrepare(context.Background(), PrepareOptions{
		Mode:     "local",
		Endpoint: "",
		StateDir: stateDir,
		ImageID:  "image",
		PsqlArgs: []string{"-c", "select 1"},
		Timeout:  time.Second,
		Verbose:  true,
	})
	if err != nil {
		t.Fatalf("RunPrepare: %v", err)
	}
	if result.DSN != "dsn" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestRunPrepareFailedWithErrorDetails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/prepare-jobs":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			io.WriteString(w, `{"job_id":"job-1","status_url":"/v1/prepare-jobs/job-1","events_url":"/v1/prepare-jobs/job-1/events"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/prepare-jobs/job-1/events":
			writeEventStream(w, []client.PrepareJobEvent{statusEvent("failed")})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/prepare-jobs/job-1":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"job_id":"job-1","status":"failed","error":{"message":"boom","details":"details"}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	_, err := RunPrepare(context.Background(), PrepareOptions{
		Mode:     "remote",
		Endpoint: server.URL,
		ImageID:  "image",
		PsqlArgs: []string{"-c", "select 1"},
		Timeout:  time.Second,
	})
	if err == nil || !strings.Contains(err.Error(), "boom: details") {
		t.Fatalf("expected error details, got %v", err)
	}
}

// A03c: a durable failure may have only a final error event, without a
// separate status event. The CLI must validate the saved job status and stop.
func TestWaitForPrepareTerminalErrorEventWithoutStatusEvent(t *testing.T) {
	var eventCalls, statusCalls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/prepare-jobs/job-1/events":
			atomic.AddInt32(&eventCalls, 1)
			writeEventStream(w, []client.PrepareJobEvent{{
				Type: "error", Status: "failed",
				Error: &client.ErrorResponse{Message: "cannot bind container Liquibase execution"},
			}})
		case "/v1/prepare-jobs/job-1":
			atomic.AddInt32(&statusCalls, 1)
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"job_id":"job-1","status":"failed","error":{"message":"cannot bind container Liquibase execution"}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cli := client.New(server.URL, client.Options{Timeout: time.Second})
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := waitForPrepare(ctx, cli, "job-1", server.URL+"/v1/prepare-jobs/job-1/events", io.Discard, false)
	if err == nil || !strings.Contains(err.Error(), "cannot bind container Liquibase execution") {
		t.Fatalf("expected saved job failure, got %v", err)
	}
	if got := atomic.LoadInt32(&eventCalls); got != 1 {
		t.Fatalf("expected one events request, got %d", got)
	}
	if got := atomic.LoadInt32(&statusCalls); got != 1 {
		t.Fatalf("expected one job status request, got %d", got)
	}
}

func TestRunPrepareJobIDMissing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/prepare-jobs" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		io.WriteString(w, `{"job_id":"","status_url":"/v1/prepare-jobs/job-1"}`)
	}))
	defer server.Close()

	_, err := RunPrepare(context.Background(), PrepareOptions{
		Mode:     "remote",
		Endpoint: server.URL,
		ImageID:  "image",
		PsqlArgs: []string{"-c", "select 1"},
		Timeout:  time.Second,
	})
	if err == nil || !strings.Contains(err.Error(), "prepare job id missing") {
		t.Fatalf("expected job id error, got %v", err)
	}
}

func TestRunPrepareJobNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/prepare-jobs":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			io.WriteString(w, `{"job_id":"job-1","status_url":"/v1/prepare-jobs/job-1","events_url":"/v1/prepare-jobs/job-1/events"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/prepare-jobs/job-1/events":
			writeEventStream(w, []client.PrepareJobEvent{statusEvent("running")})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/prepare-jobs/job-1":
			w.WriteHeader(http.StatusNotFound)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	_, err := RunPrepare(context.Background(), PrepareOptions{
		Mode:     "remote",
		Endpoint: server.URL,
		ImageID:  "image",
		PsqlArgs: []string{"-c", "select 1"},
		Timeout:  time.Second,
	})
	if err == nil || !strings.Contains(err.Error(), "prepare job not found") {
		t.Fatalf("expected not found error, got %v", err)
	}
}

func TestRunPrepareSucceededWithoutResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/prepare-jobs":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			io.WriteString(w, `{"job_id":"job-1","status_url":"/v1/prepare-jobs/job-1","events_url":"/v1/prepare-jobs/job-1/events"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/prepare-jobs/job-1/events":
			writeEventStream(w, []client.PrepareJobEvent{statusEvent("succeeded")})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/prepare-jobs/job-1":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"job_id":"job-1","status":"succeeded"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	_, err := RunPrepare(context.Background(), PrepareOptions{
		Mode:     "remote",
		Endpoint: server.URL,
		ImageID:  "image",
		PsqlArgs: []string{"-c", "select 1"},
		Timeout:  time.Second,
	})
	if err == nil || !strings.Contains(err.Error(), "succeeded without result") {
		t.Fatalf("expected missing result error, got %v", err)
	}
}

func TestRunPrepareFailedWithoutError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/prepare-jobs":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			io.WriteString(w, `{"job_id":"job-1","status_url":"/v1/prepare-jobs/job-1","events_url":"/v1/prepare-jobs/job-1/events"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/prepare-jobs/job-1/events":
			writeEventStream(w, []client.PrepareJobEvent{statusEvent("failed")})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/prepare-jobs/job-1":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"job_id":"job-1","status":"failed"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	_, err := RunPrepare(context.Background(), PrepareOptions{
		Mode:     "remote",
		Endpoint: server.URL,
		ImageID:  "image",
		PsqlArgs: []string{"-c", "select 1"},
		Timeout:  time.Second,
	})
	if err == nil || !strings.Contains(err.Error(), "prepare job failed") {
		t.Fatalf("expected failed error, got %v", err)
	}
}

func TestRunPrepareRejectsPlanOnly(t *testing.T) {
	_, err := RunPrepare(context.Background(), PrepareOptions{PlanOnly: true})
	if err == nil || !strings.Contains(err.Error(), "plan-only is not supported") {
		t.Fatalf("expected plan-only error, got %v", err)
	}
}

func TestRunPrepareCreateJobError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	_, err := RunPrepare(context.Background(), PrepareOptions{
		Mode:     "remote",
		Endpoint: server.URL,
		ImageID:  "image",
		PsqlArgs: []string{"-c", "select 1"},
		Timeout:  time.Second,
	})
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestRunPrepareLocalAutostartDisabled(t *testing.T) {
	_, err := RunPrepare(context.Background(), PrepareOptions{
		Mode:      "local",
		Endpoint:  "",
		StateDir:  t.TempDir(),
		Autostart: false,
		ImageID:   "image",
		PsqlArgs:  []string{"-c", "select 1"},
		Timeout:   time.Second,
	})
	if err == nil || !strings.Contains(err.Error(), "local engine is not running") {
		t.Fatalf("expected autostart error, got %v", err)
	}
}

func TestRunPlanRemoteSuccess(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/prepare-jobs":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			io.WriteString(w, `{"job_id":"job-1","status_url":"/v1/prepare-jobs/job-1","events_url":"/v1/prepare-jobs/job-1/events"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/prepare-jobs/job-1/events":
			writeEventStream(w, []client.PrepareJobEvent{statusEvent("succeeded")})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/prepare-jobs/job-1":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"job_id":"job-1","status":"succeeded","plan_only":true,"prepare_kind":"psql","image_id":"image","prepare_args_normalized":"-c select 1","tasks":[{"task_id":"plan","type":"plan","planner_kind":"psql"}]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	result, err := RunPlan(context.Background(), PrepareOptions{
		Mode:     "remote",
		Endpoint: server.URL,
		ImageID:  "image",
		PsqlArgs: []string{"-c", "select 1"},
		Timeout:  time.Second,
		Verbose:  true,
	})
	if err != nil {
		t.Fatalf("RunPlan: %v", err)
	}
	if result.PrepareKind != "psql" || result.ImageID != "image" || len(result.Tasks) != 1 {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestRunPlanJobIDMissing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/prepare-jobs" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		io.WriteString(w, `{"job_id":"","status_url":"/v1/prepare-jobs/job-1"}`)
	}))
	defer server.Close()

	_, err := RunPlan(context.Background(), PrepareOptions{
		Mode:     "remote",
		Endpoint: server.URL,
		ImageID:  "image",
		PsqlArgs: []string{"-c", "select 1"},
		Timeout:  time.Second,
	})
	if err == nil || !strings.Contains(err.Error(), "prepare job id missing") {
		t.Fatalf("expected job id error, got %v", err)
	}
}

func TestRunPlanRemoteRequiresEndpoint(t *testing.T) {
	_, err := RunPlan(context.Background(), PrepareOptions{Mode: "remote"})
	if err == nil || !strings.Contains(err.Error(), "explicit endpoint") {
		t.Fatalf("expected endpoint error, got %v", err)
	}
}

func TestRunPlanNotPlanOnly(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/prepare-jobs":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			io.WriteString(w, `{"job_id":"job-1","status_url":"/v1/prepare-jobs/job-1","events_url":"/v1/prepare-jobs/job-1/events"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/prepare-jobs/job-1/events":
			writeEventStream(w, []client.PrepareJobEvent{statusEvent("succeeded")})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/prepare-jobs/job-1":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"job_id":"job-1","status":"succeeded","plan_only":false}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	_, err := RunPlan(context.Background(), PrepareOptions{
		Mode:     "remote",
		Endpoint: server.URL,
		ImageID:  "image",
		PsqlArgs: []string{"-c", "select 1"},
		Timeout:  time.Second,
	})
	if err == nil || !strings.Contains(err.Error(), "not plan-only") {
		t.Fatalf("expected not plan-only error, got %v", err)
	}
}

func TestRunPlanCreateJobError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	_, err := RunPlan(context.Background(), PrepareOptions{
		Mode:     "remote",
		Endpoint: server.URL,
		ImageID:  "image",
		PsqlArgs: []string{"-c", "select 1"},
		Timeout:  time.Second,
	})
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestRunPlanNoTasks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/prepare-jobs":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			io.WriteString(w, `{"job_id":"job-1","status_url":"/v1/prepare-jobs/job-1","events_url":"/v1/prepare-jobs/job-1/events"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/prepare-jobs/job-1/events":
			writeEventStream(w, []client.PrepareJobEvent{statusEvent("succeeded")})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/prepare-jobs/job-1":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"job_id":"job-1","status":"succeeded","plan_only":true,"prepare_kind":"psql","image_id":"image","prepare_args_normalized":"-c select 1","tasks":[]}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	_, err := RunPlan(context.Background(), PrepareOptions{
		Mode:     "remote",
		Endpoint: server.URL,
		ImageID:  "image",
		PsqlArgs: []string{"-c", "select 1"},
		Timeout:  time.Second,
	})
	if err == nil || !strings.Contains(err.Error(), "without tasks") {
		t.Fatalf("expected missing tasks error, got %v", err)
	}
}

func TestRunPlanFailedWithMessage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/prepare-jobs":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			io.WriteString(w, `{"job_id":"job-1","status_url":"/v1/prepare-jobs/job-1","events_url":"/v1/prepare-jobs/job-1/events"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/prepare-jobs/job-1/events":
			writeEventStream(w, []client.PrepareJobEvent{statusEvent("failed")})
		case r.Method == http.MethodGet && r.URL.Path == "/v1/prepare-jobs/job-1":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"job_id":"job-1","status":"failed","error":{"message":"boom"}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	_, err := RunPlan(context.Background(), PrepareOptions{
		Mode:     "remote",
		Endpoint: server.URL,
		ImageID:  "image",
		PsqlArgs: []string{"-c", "select 1"},
		Timeout:  time.Second,
	})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected error message, got %v", err)
	}
}

func TestWaitForPrepareCanceled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/prepare-jobs/job-1/events":
			w.Header().Set("Content-Type", "application/x-ndjson")
			w.WriteHeader(http.StatusOK)
			<-r.Context().Done()
		case "/v1/prepare-jobs/job-1":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"job_id":"job-1","status":"running"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cli := client.New(server.URL, client.Options{Timeout: time.Second})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := waitForPrepare(ctx, cli, "job-1", server.URL+"/v1/prepare-jobs/job-1/events", io.Discard, false)
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("expected canceled error, got %v", err)
	}
}

func TestWaitForPrepareEvents4xx(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/prepare-jobs/job-1/events" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	cli := client.New(server.URL, client.Options{Timeout: time.Second})
	_, err := waitForPrepare(context.Background(), cli, "job-1", server.URL+"/v1/prepare-jobs/job-1/events", io.Discard, false)
	if err == nil || !strings.Contains(err.Error(), "events stream") {
		t.Fatalf("expected events stream error, got %v", err)
	}
}

func TestWaitForPrepareStreamEndsWithoutTerminal(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/prepare-jobs/job-1/events":
			writeEventStream(w, []client.PrepareJobEvent{statusEvent("running")})
		case "/v1/prepare-jobs/job-1":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"job_id":"job-1","status":"running"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cli := client.New(server.URL, client.Options{Timeout: time.Second})
	_, err := waitForPrepare(context.Background(), cli, "job-1", server.URL+"/v1/prepare-jobs/job-1/events", io.Discard, false)
	if err == nil || !strings.Contains(err.Error(), "terminal") {
		t.Fatalf("expected terminal status error, got %v", err)
	}
}

// The prepare-events flow requires a clean stream end without a confirmed
// terminal job state to fail even when HTTP uses chunked transfer encoding.
func TestWaitForPrepareChunkedStreamEndsWithoutTerminal(t *testing.T) {
	var eventCalls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/prepare-jobs/job-1/events":
			if atomic.AddInt32(&eventCalls, 1) != 1 {
				w.WriteHeader(http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/x-ndjson")
			w.WriteHeader(http.StatusOK)
			w.(http.Flusher).Flush()
			io.WriteString(w, encodeEvents([]client.PrepareJobEvent{statusEvent("running")}))
		case "/v1/prepare-jobs/job-1":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"job_id":"job-1","status":"running"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cli := client.New(server.URL, client.Options{Timeout: time.Second})
	_, err := waitForPrepare(context.Background(), cli, "job-1", server.URL+"/v1/prepare-jobs/job-1/events", io.Discard, false)
	if err == nil || !strings.Contains(err.Error(), "ended without terminal status") {
		t.Fatalf("expected terminal-status error after complete chunked stream, got %v", err)
	}
	if got := atomic.LoadInt32(&eventCalls); got != 1 {
		t.Fatalf("complete stream should not reconnect, requests=%d", got)
	}
}

func TestWaitForPrepareReconnectRangeIgnored(t *testing.T) {
	var eventCalls int32
	var statusCalls int32
	eventsPayload := encodeEvents([]client.PrepareJobEvent{statusEvent("running"), statusEvent("succeeded")})
	firstChunk := eventsPayload[:strings.Index(eventsPayload, "\n")+1]
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/prepare-jobs/job-1/events":
			call := atomic.AddInt32(&eventCalls, 1)
			w.Header().Set("Content-Type", "application/x-ndjson")
			w.Header().Set("Content-Length", strconv.Itoa(len(eventsPayload)))
			if call == 1 {
				io.WriteString(w, firstChunk)
				return
			}
			if r.Header.Get("Range") == "" {
				t.Fatalf("expected Range header on reconnect")
			}
			io.WriteString(w, eventsPayload)
		case "/v1/prepare-jobs/job-1":
			call := atomic.AddInt32(&statusCalls, 1)
			w.Header().Set("Content-Type", "application/json")
			if call == 1 {
				io.WriteString(w, `{"job_id":"job-1","status":"running"}`)
				return
			}
			io.WriteString(w, `{"job_id":"job-1","status":"succeeded","result":{"dsn":"dsn","instance_id":"inst","state_id":"state","image_id":"image","prepare_kind":"psql","prepare_args_normalized":"-c select 1"}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cli := client.New(server.URL, client.Options{Timeout: time.Second})
	status, err := waitForPrepare(context.Background(), cli, "job-1", server.URL+"/v1/prepare-jobs/job-1/events", io.Discard, false)
	if err != nil {
		t.Fatalf("waitForPrepare: %v", err)
	}
	if status.Status != "succeeded" {
		t.Fatalf("expected succeeded status, got %q", status.Status)
	}
}

func TestWaitForPrepareReconnectRangePartial(t *testing.T) {
	var eventCalls int32
	var statusCalls int32
	eventsPayload := encodeEvents([]client.PrepareJobEvent{statusEvent("running"), statusEvent("succeeded")})
	firstChunk := eventsPayload[:strings.Index(eventsPayload, "\n")+1]
	lastChunk := eventsPayload[strings.Index(eventsPayload, "\n")+1:]
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/prepare-jobs/job-1/events":
			call := atomic.AddInt32(&eventCalls, 1)
			w.Header().Set("Content-Type", "application/x-ndjson")
			if call == 1 {
				w.Header().Set("Content-Length", strconv.Itoa(len(eventsPayload)))
				io.WriteString(w, firstChunk)
				return
			}
			if r.Header.Get("Range") == "" {
				t.Fatalf("expected Range header on reconnect")
			}
			w.Header().Set("Content-Range", "events 1-1/2")
			w.WriteHeader(http.StatusPartialContent)
			io.WriteString(w, lastChunk)
		case "/v1/prepare-jobs/job-1":
			call := atomic.AddInt32(&statusCalls, 1)
			w.Header().Set("Content-Type", "application/json")
			if call == 1 {
				io.WriteString(w, `{"job_id":"job-1","status":"running"}`)
				return
			}
			io.WriteString(w, `{"job_id":"job-1","status":"succeeded","result":{"dsn":"dsn","instance_id":"inst","state_id":"state","image_id":"image","prepare_kind":"psql","prepare_args_normalized":"-c select 1"}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cli := client.New(server.URL, client.Options{Timeout: time.Second})
	status, err := waitForPrepare(context.Background(), cli, "job-1", server.URL+"/v1/prepare-jobs/job-1/events", io.Discard, false)
	if err != nil {
		t.Fatalf("waitForPrepare: %v", err)
	}
	if status.Status != "succeeded" {
		t.Fatalf("expected succeeded status, got %q", status.Status)
	}
}

func TestWaitForPrepareSpinnerNoNewlines(t *testing.T) {
	var statusCalls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/prepare-jobs/job-1/events":
			events := []client.PrepareJobEvent{
				statusEvent("running"),
				statusEvent("running"),
				statusEvent("succeeded"),
			}
			writeEventStream(w, events)
		case "/v1/prepare-jobs/job-1":
			call := atomic.AddInt32(&statusCalls, 1)
			w.Header().Set("Content-Type", "application/json")
			if call == 1 {
				io.WriteString(w, `{"job_id":"job-1","status":"running"}`)
				return
			}
			io.WriteString(w, `{"job_id":"job-1","status":"succeeded","result":{"dsn":"dsn","instance_id":"inst","state_id":"state","image_id":"image","prepare_kind":"psql","prepare_args_normalized":"-c select 1"}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cli := client.New(server.URL, client.Options{Timeout: time.Second})
	var stderr bytes.Buffer
	_, err := waitForPrepare(context.Background(), cli, "job-1", server.URL+"/v1/prepare-jobs/job-1/events", &stderr, false)
	if err != nil {
		t.Fatalf("waitForPrepare: %v", err)
	}
	if strings.Count(stderr.String(), "\n") > 1 {
		t.Fatalf("expected no extra newlines, got %q", stderr.String())
	}
}

func TestPrepareProgressSpinnerOnDuplicateOnly(t *testing.T) {
	var buf bytes.Buffer
	progress := newPrepareProgress(&buf, false)
	event := client.PrepareJobEvent{
		Type:   "task",
		Status: "running",
		TaskID: "execute-0",
	}

	progress.Update(event)
	line := lastProgressLine(buf.String())
	if hasSpinnerSuffix(line) {
		t.Fatalf("expected no spinner on first event, got %q", line)
	}

	progress.Update(event)
	line = lastProgressLine(buf.String())
	if !hasSpinnerSuffix(line) {
		t.Fatalf("expected spinner on duplicate event, got %q", line)
	}

	progress.Update(client.PrepareJobEvent{
		Type:    "log",
		Message: "docker: pull",
	})
	line = lastProgressLine(buf.String())
	if hasSpinnerSuffix(line) {
		t.Fatalf("expected spinner cleared on new event, got %q", line)
	}
}

func TestPrepareProgressWriteSpinnerMinimal(t *testing.T) {
	var buf bytes.Buffer
	progress := newPrepareProgress(&buf, false)
	progress.minSpinner = true

	progress.writeSpinner("-")
	if buf.String() != " -" {
		t.Fatalf("expected initial spinner output, got %q", buf.String())
	}

	progress.writeSpinner("|")
	if buf.String() != " -\b|" {
		t.Fatalf("expected backspace spinner update, got %q", buf.String())
	}
}

func TestPrepareProgressWriteSpinnerDiscard(t *testing.T) {
	progress := newPrepareProgress(io.Discard, false)
	progress.minSpinner = true
	progress.writeSpinner("-")
}

func TestPrepareProgressTerminalHelpers(t *testing.T) {
	prevIsTerminal := isTerminal
	prevGetTermSize := getTermSize
	defer func() {
		isTerminal = prevIsTerminal
		getTermSize = prevGetTermSize
	}()

	isTerminal = func(fd int) bool { return true }
	getTermSize = func(fd int) (int, int, error) { return 20, 5, nil }
	t.Setenv("WT_SESSION", "1")

	if !supportsAnsiClear(os.Stdout) {
		t.Fatalf("expected supportsAnsiClear true with terminal")
	}
	if !supportsSpinnerBackspace(os.Stdout) {
		t.Fatalf("expected supportsSpinnerBackspace true with terminal")
	}
	if width, ok := terminalWidth(os.Stdout); !ok || width != 20 {
		t.Fatalf("expected terminalWidth 20 true, got %d %v", width, ok)
	}
	if got := truncateLine("abcdef", 4); got != "a..." {
		t.Fatalf("unexpected truncateLine: %q", got)
	}
	if got := truncateLine("abc", 2); got != "ab" {
		t.Fatalf("unexpected truncateLine narrow: %q", got)
	}
}

func TestPrepareProgressTerminalHelpersFalse(t *testing.T) {
	if supportsAnsiClear(&bytes.Buffer{}) {
		t.Fatalf("expected supportsAnsiClear false for non-file writer")
	}
	if supportsSpinnerBackspace(&bytes.Buffer{}) {
		t.Fatalf("expected supportsSpinnerBackspace false for non-file writer")
	}
	if width, ok := terminalWidth(&bytes.Buffer{}); ok || width != 0 {
		t.Fatalf("expected terminalWidth false for non-file writer, got %d %v", width, ok)
	}
}

func TestPrepareProgressTerminalHelpersEnvEmpty(t *testing.T) {
	prevIsTerminal := isTerminal
	defer func() { isTerminal = prevIsTerminal }()
	isTerminal = func(fd int) bool { return true }
	t.Setenv("WT_SESSION", "")
	t.Setenv("TERM", "")
	if runtime.GOOS == "windows" {
		if supportsAnsiClear(os.Stdout) {
			t.Fatalf("expected supportsAnsiClear false with empty env")
		}
		if supportsSpinnerBackspace(os.Stdout) {
			t.Fatalf("expected supportsSpinnerBackspace false with empty env")
		}
	} else {
		if !supportsAnsiClear(os.Stdout) {
			t.Fatalf("expected supportsAnsiClear true on non-windows")
		}
		if !supportsSpinnerBackspace(os.Stdout) {
			t.Fatalf("expected supportsSpinnerBackspace true on non-windows")
		}
	}
}

func TestPrepareProgressTerminalWidthErrors(t *testing.T) {
	prevIsTerminal := isTerminal
	prevGetTermSize := getTermSize
	defer func() {
		isTerminal = prevIsTerminal
		getTermSize = prevGetTermSize
	}()
	isTerminal = func(fd int) bool { return true }
	getTermSize = func(fd int) (int, int, error) { return 0, 0, errors.New("boom") }
	if width, ok := terminalWidth(os.Stdout); ok || width != 0 {
		t.Fatalf("expected terminalWidth error false, got %d %v", width, ok)
	}
	getTermSize = func(fd int) (int, int, error) { return 0, 0, nil }
	if width, ok := terminalWidth(os.Stdout); ok || width != 0 {
		t.Fatalf("expected terminalWidth false for zero width, got %d %v", width, ok)
	}
}

func TestPrepareProgressWriteLinePadding(t *testing.T) {
	var buf bytes.Buffer
	progress := newPrepareProgress(&buf, false)
	progress.clearLine = false
	progress.wroteLine = true
	progress.lastVisible = 10
	progress.writeLine("short")
	out := buf.String()
	if !strings.HasPrefix(out, "\rshort") {
		t.Fatalf("expected carriage return and content, got %q", out)
	}
	if len([]rune(out)) < len([]rune("\rshort"))+5 {
		t.Fatalf("expected padded spaces, got %q", out)
	}
}

func TestPrepareProgressWriteLineClear(t *testing.T) {
	var buf bytes.Buffer
	progress := newPrepareProgress(&buf, false)
	progress.clearLine = true
	progress.wroteLine = true
	progress.writeLine("next")
	if !strings.HasPrefix(buf.String(), "\r\033[2Knext") {
		t.Fatalf("expected clear line prefix, got %q", buf.String())
	}
}

func TestPrepareProgressWriteLineWidthTruncate(t *testing.T) {
	prevIsTerminal := isTerminal
	prevGetTermSize := getTermSize
	defer func() {
		isTerminal = prevIsTerminal
		getTermSize = prevGetTermSize
	}()
	isTerminal = func(fd int) bool { return true }
	getTermSize = func(fd int) (int, int, error) { return 6, 5, nil }

	tmp, err := os.CreateTemp(t.TempDir(), "line")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer tmp.Close()

	progress := newPrepareProgress(tmp, false)
	progress.writeLine("123456789")
	info, err := tmp.Stat()
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Size() == 0 {
		t.Fatalf("expected truncated output")
	}
}

func TestPrepareProgressWriteLinePaddingWidthClamp(t *testing.T) {
	prevIsTerminal := isTerminal
	prevGetTermSize := getTermSize
	defer func() {
		isTerminal = prevIsTerminal
		getTermSize = prevGetTermSize
	}()
	isTerminal = func(fd int) bool { return true }
	getTermSize = func(fd int) (int, int, error) { return 8, 5, nil }

	tmp, err := os.CreateTemp(t.TempDir(), "pad")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer tmp.Close()

	progress := newPrepareProgress(tmp, false)
	progress.clearLine = false
	progress.wroteLine = true
	progress.lastVisible = 10
	progress.writeLine("short")
	info, err := tmp.Stat()
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Size() == 0 {
		t.Fatalf("expected padded output")
	}
}

func TestPrepareProgressNewNilWriter(t *testing.T) {
	progress := newPrepareProgress(nil, false)
	if progress.writer != io.Discard {
		t.Fatalf("expected discard writer for nil")
	}
}

func TestPrepareProgressTruncateLineNoop(t *testing.T) {
	if got := truncateLine("abc", 0); got != "abc" {
		t.Fatalf("unexpected truncateLine width 0: %q", got)
	}
	if got := truncateLine("abc", 5); got != "abc" {
		t.Fatalf("unexpected truncateLine no truncation: %q", got)
	}
}

func TestPrepareProgressWriteSpinnerWidthSkip(t *testing.T) {
	prevIsTerminal := isTerminal
	prevGetTermSize := getTermSize
	defer func() {
		isTerminal = prevIsTerminal
		getTermSize = prevGetTermSize
	}()

	isTerminal = func(fd int) bool { return true }
	getTermSize = func(fd int) (int, int, error) { return 6, 5, nil }
	tmp, err := os.CreateTemp(t.TempDir(), "spinner")
	if err != nil {
		t.Fatalf("CreateTemp: %v", err)
	}
	defer tmp.Close()
	progress := newPrepareProgress(tmp, false)
	progress.minSpinner = true
	progress.lastVisible = 5
	progress.writeSpinner("-")
	info, err := tmp.Stat()
	if err != nil {
		t.Fatalf("Stat: %v", err)
	}
	if info.Size() != 0 {
		t.Fatalf("expected spinner suppressed on narrow width, got %d bytes", info.Size())
	}
}

func TestPrepareProgressUpdateVerbose(t *testing.T) {
	var buf bytes.Buffer
	progress := newPrepareProgress(&buf, true)
	progress.Update(client.PrepareJobEvent{Type: "status", Status: "running"})
	if !strings.Contains(buf.String(), "prepare status") || !strings.HasSuffix(buf.String(), "\n") {
		t.Fatalf("expected verbose line, got %q", buf.String())
	}
}

func TestPrepareProgressUpdateVerboseDiscard(t *testing.T) {
	progress := newPrepareProgress(io.Discard, true)
	progress.Update(client.PrepareJobEvent{Type: "status", Status: "running"})
	if progress.wroteLine {
		t.Fatalf("expected wroteLine false for discard")
	}
}

func TestPrepareProgressUpdateMinSpinner(t *testing.T) {
	var buf bytes.Buffer
	progress := newPrepareProgress(&buf, false)
	progress.minSpinner = true
	event := client.PrepareJobEvent{Type: "task", Status: "running", TaskID: "execute-0"}
	progress.Update(event)
	progress.Update(event)
	progress.Update(event)
	if !strings.Contains(buf.String(), "\b") {
		t.Fatalf("expected backspace spinner update, got %q", buf.String())
	}
}

func TestWaitForPrepareVerboseNewlines(t *testing.T) {
	var statusCalls int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/prepare-jobs/job-1/events":
			events := []client.PrepareJobEvent{
				{Type: "log", Ts: "2026-01-24T00:00:00Z", Message: "docker pull"},
				statusEvent("running"),
				{Type: "log", Ts: "2026-01-24T00:00:01Z", Message: "psql start"},
				statusEvent("succeeded"),
			}
			writeEventStream(w, events)
		case "/v1/prepare-jobs/job-1":
			call := atomic.AddInt32(&statusCalls, 1)
			w.Header().Set("Content-Type", "application/json")
			if call == 1 {
				io.WriteString(w, `{"job_id":"job-1","status":"running"}`)
				return
			}
			io.WriteString(w, `{"job_id":"job-1","status":"succeeded","result":{"dsn":"dsn","instance_id":"inst","state_id":"state","image_id":"image","prepare_kind":"psql","prepare_args_normalized":"-c select 1"}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cli := client.New(server.URL, client.Options{Timeout: time.Second})
	var stderr bytes.Buffer
	_, err := waitForPrepare(context.Background(), cli, "job-1", server.URL+"/v1/prepare-jobs/job-1/events", &stderr, true)
	if err != nil {
		t.Fatalf("waitForPrepare: %v", err)
	}
	if strings.Count(stderr.String(), "\n") < 3 {
		t.Fatalf("expected verbose output lines, got %q", stderr.String())
	}
}

func writeEventStream(w http.ResponseWriter, events []client.PrepareJobEvent) {
	payload := encodeEvents(events)
	w.Header().Set("Content-Type", "application/x-ndjson")
	w.Header().Set("Content-Length", strconv.Itoa(len(payload)))
	io.WriteString(w, payload)
}

func encodeEvents(events []client.PrepareJobEvent) string {
	var builder strings.Builder
	for _, event := range events {
		data, err := json.Marshal(event)
		if err != nil {
			continue
		}
		builder.Write(data)
		builder.WriteByte('\n')
	}
	return builder.String()
}

func statusEvent(status string) client.PrepareJobEvent {
	return client.PrepareJobEvent{
		Type:   "status",
		Ts:     "2026-01-24T00:00:00Z",
		Status: status,
	}
}

func lastProgressLine(output string) string {
	if idx := strings.LastIndex(output, "\r"); idx != -1 {
		return output[idx+1:]
	}
	return output
}

func hasSpinnerSuffix(line string) bool {
	if line == "" {
		return false
	}
	last := line[len(line)-1]
	return last == '-' || last == '\\' || last == '|' || last == '/'
}

func TestRunWatchSucceededImmediately(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/prepare-jobs/job-1":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"job_id":"job-1","status":"succeeded","result":{"dsn":"dsn","instance_id":"inst","state_id":"state","image_id":"image","prepare_kind":"psql","prepare_args_normalized":"-c select 1"}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	status, err := RunWatch(context.Background(), PrepareOptions{
		Mode:     "remote",
		Endpoint: server.URL,
		Timeout:  time.Second,
	}, "job-1")
	if err != nil {
		t.Fatalf("RunWatch: %v", err)
	}
	if status.Status != "succeeded" {
		t.Fatalf("expected succeeded status, got %q", status.Status)
	}
}

func TestRunWatchNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	_, err := RunWatch(context.Background(), PrepareOptions{
		Mode:     "remote",
		Endpoint: server.URL,
		Timeout:  time.Second,
	}, "job-missing")
	if err == nil || !strings.Contains(err.Error(), "prepare job not found") {
		t.Fatalf("expected not found error, got %v", err)
	}
}

func TestHandlePrepareControlActionDetach(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/prepare-jobs/job-1" {
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"job_id":"job-1","status":"running","prepare_kind":"psql","image_id":"image"}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	prevPrompt := promptPrepareControl
	promptPrepareControl = func(writer io.Writer, interrupts <-chan os.Signal) (prepareControlAction, error) {
		return prepareControlDetach, nil
	}
	defer func() { promptPrepareControl = prevPrompt }()

	cliClient := client.New(server.URL, client.Options{Timeout: time.Second})
	tracker := newPrepareProgress(io.Discard, false)
	defer tracker.Close()
	_, err := handlePrepareControlAction(context.Background(), cliClient, "job-1", tracker, make(chan os.Signal))
	var detached *PrepareDetachedError
	if !errors.As(err, &detached) {
		t.Fatalf("expected detached error, got %v", err)
	}
}

func TestHandlePrepareControlActionStopRequestsCancel(t *testing.T) {
	var cancelCalled bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/prepare-jobs/job-1":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"job_id":"job-1","status":"running","prepare_kind":"psql","image_id":"image"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/prepare-jobs/job-1/cancel":
			cancelCalled = true
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			io.WriteString(w, `{"job_id":"job-1","status":"running","prepare_kind":"psql","image_id":"image"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	prevPrompt := promptPrepareControl
	promptPrepareControl = func(writer io.Writer, interrupts <-chan os.Signal) (prepareControlAction, error) {
		return prepareControlStop, nil
	}
	defer func() { promptPrepareControl = prevPrompt }()

	cliClient := client.New(server.URL, client.Options{Timeout: time.Second})
	tracker := newPrepareProgress(io.Discard, false)
	defer tracker.Close()
	status, err := handlePrepareControlAction(context.Background(), cliClient, "job-1", tracker, make(chan os.Signal))
	if err != nil {
		t.Fatalf("handlePrepareControlAction: %v", err)
	}
	if status != nil {
		t.Fatalf("expected nil status for accepted cancel request, got %+v", status)
	}
	if !cancelCalled {
		t.Fatalf("expected cancel endpoint to be called")
	}
}

func TestPrepareDetachedErrorMessage(t *testing.T) {
	if got := (&PrepareDetachedError{}).Error(); got != "prepare detached" {
		t.Fatalf("unexpected empty detached message: %q", got)
	}
	if got := (&PrepareDetachedError{JobID: "job-1"}).Error(); got != "prepare detached from job job-1" {
		t.Fatalf("unexpected detached message: %q", got)
	}
}

func TestSubmitPrepare(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/prepare-jobs" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		io.WriteString(w, `{"job_id":"job-submit","status_url":"/v1/prepare-jobs/job-submit","events_url":"/v1/prepare-jobs/job-submit/events"}`)
	}))
	defer server.Close()

	accepted, err := SubmitPrepare(context.Background(), PrepareOptions{
		Mode:     "remote",
		Endpoint: server.URL,
		ImageID:  "image",
		PsqlArgs: []string{"-c", "select 1"},
		Timeout:  time.Second,
	})
	if err != nil {
		t.Fatalf("SubmitPrepare: %v", err)
	}
	if accepted.JobID != "job-submit" {
		t.Fatalf("unexpected accepted payload: %+v", accepted)
	}
}

func TestRunWatchRequiresJobID(t *testing.T) {
	_, err := RunWatch(context.Background(), PrepareOptions{}, "   ")
	if err == nil || !strings.Contains(err.Error(), "prepare job id is required") {
		t.Fatalf("expected job id error, got %v", err)
	}
}

func TestRunWatchFailedImmediately(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/prepare-jobs/job-1" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"job_id":"job-1","status":"failed","error":{"message":"boom","details":"failed fast"}}`)
	}))
	defer server.Close()

	_, err := RunWatch(context.Background(), PrepareOptions{
		Mode:     "remote",
		Endpoint: server.URL,
		Timeout:  time.Second,
	}, "job-1")
	if err == nil || !strings.Contains(err.Error(), "boom: failed fast") {
		t.Fatalf("expected failed status error, got %v", err)
	}
}

func TestRunWatchRecoveryRequiredStopsWithoutStreaming(t *testing.T) {
	var streamCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/v1/prepare-jobs/job-1" {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"job_id":"job-1","status":"recovery_required","error":{"message":"database result is unclear","details":"database session could still finish"}}`)
			return
		}
		streamCalls.Add(1)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()

	_, err := RunWatch(context.Background(), PrepareOptions{Mode: "remote", Endpoint: server.URL, Timeout: time.Second}, "job-1")
	if err == nil || !strings.Contains(err.Error(), "job-1") || !strings.Contains(err.Error(), "database session could still finish") {
		t.Fatalf("expected actionable recovery error, got %v", err)
	}
	if got := streamCalls.Load(); got != 0 {
		t.Fatalf("recovery-required watch opened stream %d times", got)
	}
}

func TestPromptPrepareControlDefault(t *testing.T) {
	withTestStdin(t, "\n", func() {
		var out bytes.Buffer
		action, err := promptPrepareControlDefault(&out, make(chan os.Signal, 1))
		if err != nil {
			t.Fatalf("promptPrepareControlDefault: %v", err)
		}
		if action != prepareControlContinue {
			t.Fatalf("expected continue, got %v", action)
		}
	})

	withTestStdin(t, "d", func() {
		action, err := promptPrepareControlDefault(io.Discard, make(chan os.Signal, 1))
		if err != nil {
			t.Fatalf("promptPrepareControlDefault: %v", err)
		}
		if action != prepareControlDetach {
			t.Fatalf("expected detach, got %v", action)
		}
	})

	withTestStdin(t, "sy", func() {
		action, err := promptPrepareControlDefault(io.Discard, make(chan os.Signal, 1))
		if err != nil {
			t.Fatalf("promptPrepareControlDefault: %v", err)
		}
		if action != prepareControlStop {
			t.Fatalf("expected stop, got %v", action)
		}
	})

	withTestStdin(t, string([]byte{3}), func() {
		action, err := promptPrepareControlDefault(io.Discard, make(chan os.Signal, 1))
		if err != nil {
			t.Fatalf("promptPrepareControlDefault: %v", err)
		}
		if action != prepareControlContinue {
			t.Fatalf("expected continue for ctrl+c byte, got %v", action)
		}
	})

	interrupts := make(chan os.Signal, 1)
	interrupts <- os.Interrupt
	action, err := promptPrepareControlDefault(io.Discard, interrupts)
	if err != nil {
		t.Fatalf("promptPrepareControlDefault: %v", err)
	}
	if action != prepareControlContinue {
		t.Fatalf("expected continue on interrupt, got %v", action)
	}
}

func TestConfirmPrepareStop(t *testing.T) {
	confirmed, err := confirmPrepareStop(bufio.NewReader(strings.NewReader("y")), io.Discard, make(chan os.Signal, 1))
	if err != nil {
		t.Fatalf("confirmPrepareStop: %v", err)
	}
	if !confirmed {
		t.Fatalf("expected confirm=true for y")
	}

	confirmed, err = confirmPrepareStop(bufio.NewReader(strings.NewReader("N")), io.Discard, make(chan os.Signal, 1))
	if err != nil {
		t.Fatalf("confirmPrepareStop: %v", err)
	}
	if confirmed {
		t.Fatalf("expected confirm=false for N")
	}

	interrupts := make(chan os.Signal, 1)
	interrupts <- os.Interrupt
	confirmed, err = confirmPrepareStop(bufio.NewReader(strings.NewReader("")), io.Discard, interrupts)
	if err != nil {
		t.Fatalf("confirmPrepareStop: %v", err)
	}
	if confirmed {
		t.Fatalf("expected confirm=false on interrupt")
	}

	confirmed, err = confirmPrepareStop(bufio.NewReader(strings.NewReader(string([]byte{3}))), io.Discard, make(chan os.Signal, 1))
	if err != nil {
		t.Fatalf("confirmPrepareStop: %v", err)
	}
	if confirmed {
		t.Fatalf("expected confirm=false for ctrl+c byte")
	}
}

func TestHandlePrepareControlActionTerminalStatus(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet || r.URL.Path != "/v1/prepare-jobs/job-1" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"job_id":"job-1","status":"succeeded","result":{"dsn":"dsn","instance_id":"inst","state_id":"state","image_id":"image","prepare_kind":"psql","prepare_args_normalized":"-c select 1"}}`)
	}))
	defer server.Close()

	prevPrompt := promptPrepareControl
	promptPrepareControl = func(writer io.Writer, interrupts <-chan os.Signal) (prepareControlAction, error) {
		t.Fatalf("prompt should not be called for terminal status")
		return prepareControlContinue, nil
	}
	defer func() { promptPrepareControl = prevPrompt }()

	cliClient := client.New(server.URL, client.Options{Timeout: time.Second})
	tracker := newPrepareProgress(io.Discard, false)
	defer tracker.Close()

	status, err := handlePrepareControlAction(context.Background(), cliClient, "job-1", tracker, make(chan os.Signal))
	if err != nil {
		t.Fatalf("handlePrepareControlAction: %v", err)
	}
	if status == nil || status.Status != "succeeded" {
		t.Fatalf("expected succeeded status, got %+v", status)
	}
}

func TestHandlePrepareControlActionCancelReturnsFailed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/prepare-jobs/job-1":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"job_id":"job-1","status":"running","prepare_kind":"psql","image_id":"image"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/prepare-jobs/job-1/cancel":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			io.WriteString(w, `{"job_id":"job-1","status":"failed","error":{"message":"boom","details":"cancel failed"}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	prevPrompt := promptPrepareControl
	promptPrepareControl = func(writer io.Writer, interrupts <-chan os.Signal) (prepareControlAction, error) {
		return prepareControlStop, nil
	}
	defer func() { promptPrepareControl = prevPrompt }()

	cliClient := client.New(server.URL, client.Options{Timeout: time.Second})
	tracker := newPrepareProgress(io.Discard, false)
	defer tracker.Close()

	status, err := handlePrepareControlAction(context.Background(), cliClient, "job-1", tracker, make(chan os.Signal))
	if err == nil || !strings.Contains(err.Error(), "boom: cancel failed") {
		t.Fatalf("expected failed cancel error, got status=%+v err=%v", status, err)
	}
}

func TestSubmitPrepareError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	_, err := SubmitPrepare(context.Background(), PrepareOptions{
		Mode:     "remote",
		Endpoint: server.URL,
		Timeout:  time.Second,
	})
	if err == nil {
		t.Fatalf("expected submit error")
	}
}

func TestRunWatchPrepareClientError(t *testing.T) {
	_, err := RunWatch(context.Background(), PrepareOptions{Mode: "remote"}, "job-1")
	if err == nil || !strings.Contains(err.Error(), "explicit endpoint") {
		t.Fatalf("expected endpoint error, got %v", err)
	}
}

func TestRunWatchGetJobError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	_, err := RunWatch(context.Background(), PrepareOptions{
		Mode:     "remote",
		Endpoint: server.URL,
		Timeout:  time.Second,
	}, "job-1")
	if err == nil {
		t.Fatalf("expected get job error")
	}
}

func TestHandlePrepareControlActionErrors(t *testing.T) {
	t.Run("get status error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer server.Close()
		cliClient := client.New(server.URL, client.Options{Timeout: time.Second})
		tracker := newPrepareProgress(io.Discard, false)
		defer tracker.Close()
		_, err := handlePrepareControlAction(context.Background(), cliClient, "job-1", tracker, make(chan os.Signal))
		if err == nil {
			t.Fatalf("expected status error")
		}
	})

	t.Run("not found", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()
		cliClient := client.New(server.URL, client.Options{Timeout: time.Second})
		tracker := newPrepareProgress(io.Discard, false)
		defer tracker.Close()
		_, err := handlePrepareControlAction(context.Background(), cliClient, "job-1", tracker, make(chan os.Signal))
		if err == nil || !strings.Contains(err.Error(), "prepare job not found") {
			t.Fatalf("expected not found error, got %v", err)
		}
	})

	t.Run("prompt error", func(t *testing.T) {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodGet && r.URL.Path == "/v1/prepare-jobs/job-1" {
				w.Header().Set("Content-Type", "application/json")
				io.WriteString(w, `{"job_id":"job-1","status":"running","prepare_kind":"psql","image_id":"image"}`)
				return
			}
			w.WriteHeader(http.StatusNotFound)
		}))
		defer server.Close()

		prevPrompt := promptPrepareControl
		promptPrepareControl = func(writer io.Writer, interrupts <-chan os.Signal) (prepareControlAction, error) {
			return prepareControlContinue, errors.New("prompt failed")
		}
		defer func() { promptPrepareControl = prevPrompt }()

		cliClient := client.New(server.URL, client.Options{Timeout: time.Second})
		tracker := newPrepareProgress(io.Discard, false)
		defer tracker.Close()
		_, err := handlePrepareControlAction(context.Background(), cliClient, "job-1", tracker, make(chan os.Signal))
		if err == nil || !strings.Contains(err.Error(), "prompt failed") {
			t.Fatalf("expected prompt error, got %v", err)
		}
	})
}

func TestHandlePrepareControlActionStopSucceeded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/prepare-jobs/job-1":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"job_id":"job-1","status":"running","prepare_kind":"psql","image_id":"image"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/prepare-jobs/job-1/cancel":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			io.WriteString(w, `{"job_id":"job-1","status":"succeeded","result":{"dsn":"dsn","instance_id":"inst","state_id":"state","image_id":"image","prepare_kind":"psql","prepare_args_normalized":"-c select 1"}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	prevPrompt := promptPrepareControl
	promptPrepareControl = func(writer io.Writer, interrupts <-chan os.Signal) (prepareControlAction, error) {
		return prepareControlStop, nil
	}
	defer func() { promptPrepareControl = prevPrompt }()

	cliClient := client.New(server.URL, client.Options{Timeout: time.Second})
	tracker := newPrepareProgress(io.Discard, false)
	defer tracker.Close()
	status, err := handlePrepareControlAction(context.Background(), cliClient, "job-1", tracker, make(chan os.Signal))
	if err != nil {
		t.Fatalf("handlePrepareControlAction: %v", err)
	}
	if status == nil || status.Status != "succeeded" {
		t.Fatalf("expected succeeded status, got %+v", status)
	}
}

func TestDefaultCanUsePrepareControlPrompt(t *testing.T) {
	prevIsTerminal := isTerminal
	defer func() { isTerminal = prevIsTerminal }()

	isTerminal = func(fd int) bool { return false }
	if defaultCanUsePrepareControlPrompt(os.Stdout) {
		t.Fatalf("expected false when output fd is not terminal")
	}

	isTerminal = func(fd int) bool { return true }
	if !defaultCanUsePrepareControlPrompt(os.Stdout) {
		t.Fatalf("expected true when output and stdin are terminal")
	}
}

func TestPromptPrepareControlDefaultNilWriter(t *testing.T) {
	withTestStdin(t, "\n", func() {
		action, err := promptPrepareControlDefault(nil, make(chan os.Signal, 1))
		if err != nil {
			t.Fatalf("promptPrepareControlDefault: %v", err)
		}
		if action != prepareControlContinue {
			t.Fatalf("expected continue, got %v", action)
		}
	})
}

func TestPromptPrepareControlDefaultStopWithLineInput(t *testing.T) {
	withTestStdin(t, "s\ny\n", func() {
		action, err := promptPrepareControlDefault(io.Discard, make(chan os.Signal, 1))
		if err != nil {
			t.Fatalf("promptPrepareControlDefault: %v", err)
		}
		if action != prepareControlStop {
			t.Fatalf("expected stop action, got %v", action)
		}
	})
}

func TestConfirmPrepareStopReaderError(t *testing.T) {
	reader := bufio.NewReader(&failingReader{err: errors.New("read failed")})
	confirmed, err := confirmPrepareStop(reader, io.Discard, make(chan os.Signal, 1))
	if err == nil || !strings.Contains(err.Error(), "read failed") {
		t.Fatalf("expected read error, got confirmed=%v err=%v", confirmed, err)
	}
}

func TestHandlePrepareControlActionDrainsStaleInterruptBeforePrompt(t *testing.T) {
	var cancelCalled bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/prepare-jobs/job-1":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"job_id":"job-1","status":"running","prepare_kind":"psql","image_id":"image"}`)
		case r.Method == http.MethodPost && r.URL.Path == "/v1/prepare-jobs/job-1/cancel":
			cancelCalled = true
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			io.WriteString(w, `{"job_id":"job-1","status":"running","prepare_kind":"psql","image_id":"image"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	prevPrompt := promptPrepareControl
	promptPrepareControl = promptPrepareControlDefault
	defer func() { promptPrepareControl = prevPrompt }()

	cliClient := client.New(server.URL, client.Options{Timeout: time.Second})
	tracker := newPrepareProgress(io.Discard, false)
	defer tracker.Close()
	interrupts := make(chan os.Signal, 2)
	interrupts <- os.Interrupt

	withTestStdin(t, "sy", func() {
		status, err := handlePrepareControlAction(context.Background(), cliClient, "job-1", tracker, interrupts)
		if err != nil {
			t.Fatalf("handlePrepareControlAction: %v", err)
		}
		if status != nil {
			t.Fatalf("expected nil status for accepted cancel, got %+v", status)
		}
	})

	if !cancelCalled {
		t.Fatalf("expected cancel endpoint to be called after confirmation")
	}
}

func TestRunWatchResolvesUniquePrefix(t *testing.T) {
	const prefix = "deadbeef"
	const fullID = "deadbeefcafebabe"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/prepare-jobs/"+prefix:
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/prepare-jobs" && r.URL.Query().Get("job") == prefix:
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `[{"job_id":"`+fullID+`","status":"running","prepare_kind":"psql","image_id":"image"}]`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/prepare-jobs/"+fullID:
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"job_id":"`+fullID+`","status":"succeeded","result":{"dsn":"dsn","instance_id":"inst","state_id":"state","image_id":"image","prepare_kind":"psql","prepare_args_normalized":"-c select 1"}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	status, err := RunWatch(context.Background(), PrepareOptions{
		Mode:     "remote",
		Endpoint: server.URL,
		Timeout:  time.Second,
	}, prefix)
	if err != nil {
		t.Fatalf("RunWatch: %v", err)
	}
	if status.JobID != fullID || status.Status != "succeeded" {
		t.Fatalf("unexpected status: %+v", status)
	}
}

func TestRunWatchRejectsAmbiguousPrefix(t *testing.T) {
	const prefix = "deadbeef"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/prepare-jobs/"+prefix:
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/prepare-jobs" && r.URL.Query().Get("job") == prefix:
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `[{"job_id":"deadbeef00000001","status":"running","prepare_kind":"psql","image_id":"image"},{"job_id":"deadbeef00000002","status":"running","prepare_kind":"psql","image_id":"image"}]`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	_, err := RunWatch(context.Background(), PrepareOptions{
		Mode:     "remote",
		Endpoint: server.URL,
		Timeout:  time.Second,
	}, prefix)
	if err == nil || !strings.Contains(err.Error(), "ambiguous job id prefix") {
		t.Fatalf("expected ambiguous prefix error, got %v", err)
	}
}

func TestRunWatchResolvesNonHexPrefix(t *testing.T) {
	const prefix = "job-a"
	const fullID = "job-alpha-0001"

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v1/prepare-jobs/"+prefix:
			w.WriteHeader(http.StatusNotFound)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/prepare-jobs" && r.URL.Query().Get("job") == prefix:
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `[{"job_id":"`+fullID+`","status":"running","prepare_kind":"psql","image_id":"image"}]`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/prepare-jobs/"+fullID:
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"job_id":"`+fullID+`","status":"succeeded","result":{"dsn":"dsn","instance_id":"inst","state_id":"state","image_id":"image","prepare_kind":"psql","prepare_args_normalized":"-c select 1"}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	status, err := RunWatch(context.Background(), PrepareOptions{
		Mode:     "remote",
		Endpoint: server.URL,
		Timeout:  time.Second,
	}, prefix)
	if err != nil {
		t.Fatalf("RunWatch: %v", err)
	}
	if status.JobID != fullID || status.Status != "succeeded" {
		t.Fatalf("unexpected status: %+v", status)
	}
}

type failingReader struct {
	err error
}

func (r *failingReader) Read(_ []byte) (int, error) {
	return 0, r.err
}

func withTestStdin(t *testing.T, input string, fn func()) {
	t.Helper()
	old := os.Stdin
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	if _, err := io.WriteString(w, input); err != nil {
		t.Fatalf("write stdin: %v", err)
	}
	if err := w.Close(); err != nil {
		t.Fatalf("close stdin writer: %v", err)
	}
	os.Stdin = r
	defer func() {
		os.Stdin = old
		_ = r.Close()
	}()
	fn()
}
