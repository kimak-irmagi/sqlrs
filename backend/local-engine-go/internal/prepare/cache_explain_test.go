package prepare

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sqlrs/engine-local/internal/store"
)

func TestCacheExplainReturnsHitWithoutCreatingJobs(t *testing.T) {
	queueStore := newQueueStore(t)
	stateStore := &fakeStore{statesByID: map[string]store.StateEntry{}}
	mgr := newManagerWithQueue(t, stateStore, queueStore)

	scriptPath := filepath.Join(t.TempDir(), "prepare.sql")
	if err := os.WriteFile(scriptPath, []byte("select 1;\n"), 0o600); err != nil {
		t.Fatalf("write script: %v", err)
	}
	req := Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-f", scriptPath},
	}
	prepared, err := mgr.prepareRequest(req)
	if err != nil {
		t.Fatalf("prepareRequest: %v", err)
	}
	outputStateID := psqlOutputStateID(t, mgr, prepared, TaskInput{Kind: "image", ID: "image-1@sha256:resolved"})
	stateStore.statesByID[outputStateID] = store.StateEntry{
		StateID:     outputStateID,
		ImageID:     "image-1@sha256:resolved",
		PrepareKind: "psql",
	}

	result, err := mgr.CacheExplain(context.Background(), req)
	if err != nil {
		t.Fatalf("CacheExplain: %v", err)
	}
	if result.Decision != "hit" || result.ReasonCode != "exact_state_match" {
		t.Fatalf("unexpected explain result: %+v", result)
	}
	if result.MatchedStateID != outputStateID || result.Signature == "" {
		t.Fatalf("unexpected cache hit payload: %+v", result)
	}

	jobs, err := queueStore.ListJobs(context.Background(), "")
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("expected no queued jobs, got %+v", jobs)
	}
	if len(stateStore.states) != 0 || len(stateStore.instances) != 0 {
		t.Fatalf("expected no store mutations, got states=%+v instances=%+v", stateStore.states, stateStore.instances)
	}
}

func TestCacheExplainReturnsMiss(t *testing.T) {
	mgr := newManagerWithQueue(t, &fakeStore{}, newQueueStore(t))
	scriptPath := filepath.Join(t.TempDir(), "prepare.sql")
	if err := os.WriteFile(scriptPath, []byte("select 1;\n"), 0o600); err != nil {
		t.Fatalf("write script: %v", err)
	}

	result, err := mgr.CacheExplain(context.Background(), Request{
		PrepareKind: "psql",
		ImageID:     "image-1",
		PsqlArgs:    []string{"-f", scriptPath},
	})
	if err != nil {
		t.Fatalf("CacheExplain: %v", err)
	}
	if result.Decision != "miss" || result.ReasonCode != "no_matching_state" {
		t.Fatalf("unexpected explain result: %+v", result)
	}
	if result.MatchedStateID != "" || result.Signature == "" {
		t.Fatalf("unexpected miss payload: %+v", result)
	}
}

func TestCacheExplainLiquibaseUsesIsolatedTempStateRoot(t *testing.T) {
	queueStore := newQueueStore(t)
	runtime := &fakeRuntime{}
	liquibase := &fakeLiquibaseRunner{
		output: strings.Join([]string{
			"-- Changeset changelog.xml::1::dev",
			"CREATE TABLE test(id INT);",
		}, "\n"),
	}
	stateRoot := filepath.Join(t.TempDir(), "persistent-state-root")
	mgr := newManagerWithDeps(t, &fakeStore{}, queueStore, &testDeps{
		runtime:   runtime,
		liquibase: liquibase,
		stateRoot: stateRoot,
	})

	changelog := filepath.Join(t.TempDir(), "changelog.xml")
	if err := os.WriteFile(changelog, []byte("<databaseChangeLog/>"), 0o600); err != nil {
		t.Fatalf("write changelog: %v", err)
	}

	result, err := mgr.CacheExplain(context.Background(), Request{
		PrepareKind:   "lb",
		ImageID:       "image-1",
		LiquibaseArgs: []string{"update", "--changelog-file", changelog},
	})
	if err != nil {
		t.Fatalf("CacheExplain: %v", err)
	}
	if result.Decision != "miss" || result.Signature == "" {
		t.Fatalf("unexpected explain result: %+v", result)
	}
	if len(runtime.initCalls) != 1 {
		t.Fatalf("expected one base init, got %+v", runtime.initCalls)
	}
	if len(runtime.startCalls) != 1 {
		t.Fatalf("expected one runtime start, got %+v", runtime.startCalls)
	}
	if !strings.HasPrefix(runtime.initCalls[0].DataDir, stateRoot) {
		t.Fatalf("base state should stay under persistent root: %q", runtime.initCalls[0].DataDir)
	}
	if !strings.HasPrefix(runtime.startCalls[0].DataDir, stateRoot) {
		t.Fatalf("runtime dir should stay under persistent root: %q", runtime.startCalls[0].DataDir)
	}

	relInit, err := filepath.Rel(stateRoot, runtime.initCalls[0].DataDir)
	if err != nil {
		t.Fatalf("Rel init: %v", err)
	}
	if relInit == "." {
		t.Fatalf("expected nested init dir under temporary state root, got %q", runtime.initCalls[0].DataDir)
	}
	initTop := strings.Split(filepath.ToSlash(relInit), "/")[0]
	if !strings.HasPrefix(initTop, "cache-explain-") {
		t.Fatalf("expected temp root prefix for init dir, got %q", relInit)
	}

	relStart, err := filepath.Rel(stateRoot, runtime.startCalls[0].DataDir)
	if err != nil {
		t.Fatalf("Rel start: %v", err)
	}
	if relStart == "." {
		t.Fatalf("expected nested runtime dir under temporary state root, got %q", runtime.startCalls[0].DataDir)
	}
	startTop := strings.Split(filepath.ToSlash(relStart), "/")[0]
	if startTop != initTop {
		t.Fatalf("expected init and runtime dirs to share temp root, got %q and %q", relInit, relStart)
	}

	tempRoot := filepath.Join(stateRoot, initTop)
	if _, err := os.Stat(tempRoot); !os.IsNotExist(err) {
		t.Fatalf("expected temp root cleanup, stat err=%v", err)
	}
	entries, err := os.ReadDir(stateRoot)
	if err != nil {
		t.Fatalf("ReadDir state root: %v", err)
	}
	if len(entries) != 0 {
		t.Fatalf("expected persistent root to be empty after cleanup, got %+v", entries)
	}

	jobs, err := queueStore.ListJobs(context.Background(), "")
	if err != nil {
		t.Fatalf("ListJobs: %v", err)
	}
	if len(jobs) != 0 {
		t.Fatalf("expected no queued jobs, got %+v", jobs)
	}
}

func TestErrorFromExplainResponseKeepsInternalErrorsOutOfValidationPath(t *testing.T) {
	err := errorFromExplainResponse(&ErrorResponse{
		Code:    "internal_error",
		Message: "cannot resolve image",
		Details: "boom",
	})
	if err == nil {
		t.Fatal("expected error")
	}
	if _, ok := err.(ValidationError); ok {
		t.Fatalf("expected non-validation error, got %T", err)
	}
	if got := err.Error(); got != "cannot resolve image: boom" {
		t.Fatalf("error = %q, want %q", got, "cannot resolve image: boom")
	}
}

func TestCacheExplainReturnsPreparationResolutionAndLookupErrors(t *testing.T) {
	t.Run("preparation", func(t *testing.T) {
		mgr := newManagerWithQueue(t, &fakeStore{}, newQueueStore(t))
		if _, err := mgr.CacheExplain(context.Background(), Request{}); err == nil {
			t.Fatal("expected invalid request error")
		}
	})

	t.Run("resolution", func(t *testing.T) {
		mgr := newManagerWithDeps(t, &fakeStore{}, newQueueStore(t), &testDeps{
			runtime: &fakeRuntime{resolveErr: errors.New("resolve failed")},
		})
		scriptPath := filepath.Join(t.TempDir(), "prepare.sql")
		if err := os.WriteFile(scriptPath, []byte("select 1;\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := mgr.CacheExplain(context.Background(), Request{
			PrepareKind: "psql",
			ImageID:     "image-1",
			PsqlArgs:    []string{"-f", scriptPath},
		})
		if err == nil || !strings.Contains(err.Error(), "cannot resolve image") {
			t.Fatalf("expected resolution error, got %v", err)
		}
	})

	t.Run("cache lookup", func(t *testing.T) {
		mgr := newManagerWithQueue(t, &fakeStore{getStateErr: errors.New("lookup failed")}, newQueueStore(t))
		scriptPath := filepath.Join(t.TempDir(), "prepare.sql")
		if err := os.WriteFile(scriptPath, []byte("select 1;\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		_, err := mgr.CacheExplain(context.Background(), Request{
			PrepareKind: "psql",
			ImageID:     "image-1@sha256:resolved",
			PsqlArgs:    []string{"-f", scriptPath},
		})
		if err == nil || !strings.Contains(err.Error(), "lookup failed") {
			t.Fatalf("expected cache lookup error, got %v", err)
		}
	})
}

func TestNewCacheExplainPlannerReturnsStateRootError(t *testing.T) {
	blocker := filepath.Join(t.TempDir(), "state-root")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	mgr := newManagerWithDeps(t, &fakeStore{}, newQueueStore(t), &testDeps{stateRoot: blocker})
	if _, _, _, err := mgr.newCacheExplainPlanner(); err == nil {
		t.Fatal("expected state root error")
	}
}

func TestErrorFromExplainResponseVariants(t *testing.T) {
	tests := []struct {
		name       string
		response   *ErrorResponse
		want       string
		validation bool
	}{
		{name: "nil", want: "internal error"},
		{name: "validation", response: &ErrorResponse{Code: "invalid_argument", Message: "bad input", Details: "field"}, want: "bad input", validation: true},
		{name: "message", response: &ErrorResponse{Code: "internal_error", Message: "failed"}, want: "failed"},
		{name: "empty", response: &ErrorResponse{Code: "internal_error"}, want: "internal error"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := errorFromExplainResponse(tt.response)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v, want %q", err, tt.want)
			}
			_, isValidation := err.(ValidationError)
			if isValidation != tt.validation {
				t.Fatalf("validation = %v, want %v", isValidation, tt.validation)
			}
		})
	}
}
