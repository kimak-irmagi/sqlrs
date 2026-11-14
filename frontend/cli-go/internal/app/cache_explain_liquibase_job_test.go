package app

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/sqlrs/cli/internal/cli"
)

// A03: a Liquibase explanation is rendered only from the saved result of its
// durable job, not from the admission response or an in-memory cache lookup.
func TestExplainLiquibaseCacheWaitsForSavedJobResult(t *testing.T) {
	var posts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/cache/explain/prepare":
			posts.Add(1)
			w.Header().Set("Location", "/v1/prepare-jobs/job-lb")
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `{"job_id":"job-lb","status_url":"/v1/prepare-jobs/job-lb","status":"queued"}`)
		case "/v1/prepare-jobs/job-lb":
			_, _ = io.WriteString(w, `{"job_id":"job-lb","status":"succeeded","prepare_kind":"lb","purpose":"cache_explain","image_id":"postgres:17","plan_only":true,"cache_explain_result":{"decision":"miss","reason_code":"no_matching_state","signature":"sig-lb"}}`)
		default:
			t.Errorf("unexpected route: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	result, err := explainPrepareCache(context.Background(), cli.PrepareOptions{
		Mode: "remote", Endpoint: server.URL, PrepareKind: "lb", ImageID: "postgres:17", LiquibaseArgs: []string{"update"},
	})
	if err != nil || posts.Load() != 1 || result.Decision != "miss" || result.Signature != "sig-lb" {
		t.Fatalf("result=%+v posts=%d err=%v", result, posts.Load(), err)
	}
}

// A03: a paused planning job is not a cache miss and cannot advance the
// current pipeline as though a result had been saved.
func TestExplainLiquibaseCacheStopsAtRecoveryRequired(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/cache/explain/prepare":
			w.Header().Set("Location", "/v1/prepare-jobs/job-lb")
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `{"job_id":"job-lb","status_url":"/v1/prepare-jobs/job-lb","status":"queued"}`)
		case "/v1/prepare-jobs/job-lb":
			_, _ = io.WriteString(w, `{"job_id":"job-lb","status":"recovery_required","prepare_kind":"lb","purpose":"cache_explain","image_id":"postgres:17","error":{"code":"recovery_required","message":"planning proof unavailable"}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	result, err := explainPrepareCache(context.Background(), cli.PrepareOptions{
		Mode: "remote", Endpoint: server.URL, PrepareKind: "lb", ImageID: "postgres:17", LiquibaseArgs: []string{"update"},
	})
	if err == nil || !strings.Contains(err.Error(), "job-lb") || result.Decision != "" {
		t.Fatalf("recovery was rendered as explanation: result=%+v err=%v", result, err)
	}
}

// A03: after a lost admission response the CLI must give a discovery path,
// not silently submit the potentially accepted operation again.
func TestExplainLiquibaseCacheLostResponsePointsToJobList(t *testing.T) {
	var posts atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		posts.Add(1)
		panic(http.ErrAbortHandler)
	}))
	defer server.Close()

	_, err := explainPrepareCache(context.Background(), cli.PrepareOptions{
		Mode: "remote", Endpoint: server.URL, PrepareKind: "lb", ImageID: "postgres:17", LiquibaseArgs: []string{"update"},
	})
	if err == nil || !strings.Contains(err.Error(), "prepare-job list") || posts.Load() != 1 {
		t.Fatalf("ambiguous admission: posts=%d err=%v", posts.Load(), err)
	}
}
