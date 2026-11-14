package client

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestExplainPrepareCache(t *testing.T) {
	var gotAuth string
	var gotReq PrepareJobRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/cache/explain/prepare" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		gotAuth = r.Header.Get("Authorization")
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"decision":"hit","reason_code":"exact_state_match","signature":"sig-1","matched_state_id":"state-1","resolved_image_id":"image@sha256:resolved"}`))
	}))
	defer server.Close()

	cli := New(server.URL, Options{Timeout: time.Second, AuthToken: "secret"})
	result, err := cli.ExplainPrepareCache(context.Background(), PrepareJobRequest{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-f", "prepare.sql"},
	})
	if err != nil {
		t.Fatalf("ExplainPrepareCache: %v", err)
	}
	if gotAuth != "Bearer secret" {
		t.Fatalf("expected auth header, got %q", gotAuth)
	}
	if gotReq.PrepareKind != "psql" || gotReq.ImageID != "image-1" {
		t.Fatalf("unexpected request: %+v", gotReq)
	}
	if result.Decision != "hit" || result.ReasonCode != "exact_state_match" {
		t.Fatalf("unexpected explain result: %+v", result)
	}
	if result.Signature != "sig-1" || result.MatchedStateID != "state-1" {
		t.Fatalf("unexpected explain cache payload: %+v", result)
	}
	if result.ResolvedImageID != "image@sha256:resolved" {
		t.Fatalf("unexpected resolved image id: %+v", result)
	}
}

// A03: Liquibase explanation admission returns a durable job reference. The
// client must not interpret a 202 body as a synchronous cache decision.
func TestAdmitLiquibaseCacheExplainReturnsJob(t *testing.T) {
	var gotReq PrepareJobRequest
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		if r.Method != http.MethodPost || r.URL.Path != "/v1/cache/explain/prepare" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if err := json.NewDecoder(r.Body).Decode(&gotReq); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Location", "/v1/prepare-jobs/job-lb")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"job_id":"job-lb","status_url":"/v1/prepare-jobs/job-lb","events_url":"/v1/prepare-jobs/job-lb/events","status":"queued"}`))
	}))
	defer server.Close()

	cli := New(server.URL, Options{Timeout: time.Second})
	accepted, err := cli.AdmitLiquibaseCacheExplain(context.Background(), PrepareJobRequest{
		PrepareKind:   "lb",
		ImageID:       "image-1",
		LiquibaseArgs: []string{"update", "--changelog-file", "db/changelog.xml"},
		WorkDir:       "/workspace",
	})
	if err != nil {
		t.Fatalf("AdmitLiquibaseCacheExplain: %v", err)
	}
	if gotReq.PrepareKind != "lb" {
		t.Fatalf("expected lb request, got %+v", gotReq)
	}
	if len(gotReq.LiquibaseArgs) != 3 || gotReq.WorkDir != "/workspace" {
		t.Fatalf("unexpected liquibase request: %+v", gotReq)
	}
	if calls.Load() != 1 || accepted.JobID != "job-lb" || accepted.StatusURL != "/v1/prepare-jobs/job-lb" || accepted.Status != "queued" {
		t.Fatalf("unexpected admission: calls=%d accepted=%+v", calls.Load(), accepted)
	}
}

// A03: a lost response is ambiguous; the transport may not submit a second
// request merely because it did not receive the job ID.
func TestAdmitLiquibaseCacheExplainDoesNotRetryLostResponse(t *testing.T) {
	var calls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		panic(http.ErrAbortHandler)
	}))
	defer server.Close()

	cli := New(server.URL, Options{Timeout: time.Second})
	if _, err := cli.AdmitLiquibaseCacheExplain(context.Background(), PrepareJobRequest{PrepareKind: "lb", ImageID: "image-1"}); err == nil {
		t.Fatal("expected ambiguous transport error")
	}
	if calls.Load() != 1 {
		t.Fatalf("ambiguous POST was automatically resubmitted: %d calls", calls.Load())
	}
}

func TestAdmitLiquibaseCacheExplainRejectsMissingJobReference(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/v1/prepare-jobs/job-lb")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"decision":"miss"}`))
	}))
	defer server.Close()

	cli := New(server.URL, Options{Timeout: time.Second})
	if _, err := cli.AdmitLiquibaseCacheExplain(context.Background(), PrepareJobRequest{PrepareKind: "lb", ImageID: "image-1"}); err == nil {
		t.Fatal("accepted a 202 body without a durable job reference")
	}
}

func TestExplainPrepareCacheDoesNotDecodeAcceptedJobAsDecision(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", "/v1/prepare-jobs/job-lb")
		w.WriteHeader(http.StatusAccepted)
		_, _ = w.Write([]byte(`{"job_id":"job-lb","status_url":"/v1/prepare-jobs/job-lb"}`))
	}))
	defer server.Close()

	cli := New(server.URL, Options{Timeout: time.Second})
	if _, err := cli.ExplainPrepareCache(context.Background(), PrepareJobRequest{PrepareKind: "lb", ImageID: "image-1"}); err == nil {
		t.Fatal("decoded a durable job reference as a synchronous cache decision")
	}
}

// A02: status and list views must preserve the purpose and only expose a
// saved explanation on success; ordinary prepare results remain separate.
func TestCacheExplainJobStatusAndListDecodePurposeAndSavedResult(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v1/prepare-jobs/job-lb":
			_, _ = w.Write([]byte(`{"job_id":"job-lb","status":"succeeded","prepare_kind":"lb","purpose":"cache_explain","image_id":"postgres:17","plan_only":true,"liquibase_image_digest":"sha256:bound","cache_explain_result":{"decision":"miss","reason_code":"no_matching_state","signature":"sig-lb","liquibase_image_digest":"sha256:bound"}}`))
		case "/v1/prepare-jobs":
			_, _ = w.Write([]byte(`[{"job_id":"job-lb","status":"succeeded","prepare_kind":"lb","purpose":"cache_explain","image_id":"postgres:17","plan_only":true,"cache_explain_result":{"decision":"miss","signature":"sig-lb"}}]`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()

	cli := New(server.URL, Options{Timeout: time.Second})
	status, found, err := cli.GetPrepareJob(context.Background(), "job-lb")
	if err != nil || !found || status.Purpose != "cache_explain" || status.Result != nil || status.LiquibaseImageDigest != "sha256:bound" || status.CacheExplainResult == nil || status.CacheExplainResult.Signature != "sig-lb" || status.CacheExplainResult.LiquibaseImageDigest != "sha256:bound" {
		t.Fatalf("status lost saved explanation: found=%t status=%+v err=%v", found, status, err)
	}
	jobs, err := cli.ListPrepareJobs(context.Background(), "")
	if err != nil || len(jobs) != 1 || jobs[0].Purpose != "cache_explain" || jobs[0].CacheExplainResult == nil || jobs[0].CacheExplainResult.Signature != "sig-lb" {
		t.Fatalf("list lost saved explanation: jobs=%+v err=%v", jobs, err)
	}
}

func TestExplainPrepareCacheError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"invalid prepare request"}`))
	}))
	defer server.Close()

	cli := New(server.URL, Options{Timeout: time.Second})
	_, err := cli.ExplainPrepareCache(context.Background(), PrepareJobRequest{
		PrepareKind: "psql",
		ImageID:     "image-1",
	})
	if err == nil || err.Error() != "invalid prepare request" {
		t.Fatalf("expected invalid request error, got %v", err)
	}
}
