package prepare

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestTypedClientRunnerSettingsAreRejected(t *testing.T) {
	fields := []struct {
		name string
		set  func(*Request)
	}{
		{"exec", func(r *Request) { r.LiquibaseExec = "client-liquibase" }},
		{"mode", func(r *Request) { r.LiquibaseExecMode = "native" }},
		{"env", func(r *Request) { r.LiquibaseEnv = map[string]string{"JAVA_HOME": "client-jdk"} }},
	}
	for _, kind := range []string{"lb", "psql"} {
		for _, field := range fields {
			t.Run(kind+"/"+field.name, func(t *testing.T) {
				q := newQueueStore(t)
				lb := &fakeLiquibaseRunner{}
				mgr := newManagerWithDeps(t, &fakeStore{}, q, &testDeps{liquibase: lb})
				req := Request{PrepareKind: kind, ImageID: "image-1", PsqlArgs: []string{"-c", "select 1"}, LiquibaseArgs: []string{"update"}}
				field.set(&req)
				for _, call := range []struct {
					name string
					fn   func() error
				}{
					{"submit", func() error { _, err := mgr.Submit(context.Background(), req); return err }},
					{"explain", func() error { _, err := mgr.CacheExplain(context.Background(), req); return err }},
				} {
					if err := call.fn(); err == nil || ToErrorResponse(err).Code != "invalid_argument" {
						t.Fatalf("%s: expected invalid_argument, got %v", call.name, err)
					}
				}
				jobs, err := q.ListJobs(context.Background(), "")
				if err != nil || len(jobs) != 0 || len(lb.runs) != 0 {
					t.Fatalf("rejected request had side effects: jobs=%v runs=%v err=%v", jobs, lb.runs, err)
				}
			})
		}
	}
}

func TestSavedLiquibaseRequestRejectsClientRunnerOverrides(t *testing.T) {
	for _, field := range []struct {
		name string
		set  func(*Request)
	}{
		{"exec", func(r *Request) { r.LiquibaseExec = "client-liquibase" }},
		{"mode", func(r *Request) { r.LiquibaseExecMode = "native" }},
		{"env", func(r *Request) { r.LiquibaseEnv = map[string]string{"JAVA_HOME": "client-jdk"} }},
	} {
		t.Run(field.name, func(t *testing.T) {
			ctx := context.Background()
			q := newQueueStore(t)
			lb, rt := &fakeLiquibaseRunner{}, &fakeRuntime{}
			req := Request{PrepareKind: "lb", ImageID: "image-1", LiquibaseArgs: []string{"update"}}
			field.set(&req)
			createJobRecord(t, q, "saved-request", req, StatusQueued)
			mgr := newManagerWithDeps(t, &fakeStore{}, q, &testDeps{liquibase: lb, runtime: rt})
			before, found, err := q.GetJob(ctx, "saved-request")
			if err != nil || !found {
				t.Fatalf("saved job found=%t err=%v", found, err)
			}
			for i := 0; i < 2; i++ {
				if _, err := mgr.prepareFromJob(before); err == nil || !strings.Contains(err.Error(), "client Liquibase runner settings are not allowed") {
					t.Fatalf("client override was not rejected: %v", err)
				}
				after, found, err := q.GetJob(ctx, "saved-request")
				if err != nil || !found || !reflect.DeepEqual(before, after) || len(lb.runs) != 0 || len(rt.startCalls) != 0 {
					t.Fatalf("validation changed saved data or launched resources: found=%t err=%v runs=%v starts=%v", found, err, lb.runs, rt.startCalls)
				}
			}
		})
	}
}

func TestTypedLiquibaseVersionSelection(t *testing.T) {
	mgr := newManagerWithDeps(t, &fakeStore{}, newQueueStore(t), nil)
	for _, tc := range []struct {
		kind    string
		version string
		want    string
		valid   bool
	}{
		{kind: "lb", version: "", want: "latest", valid: true},
		{kind: "lb", version: "5.0.3", want: "5.0.3", valid: true},
		{kind: "lb", version: "liquibase/liquibase:latest"},
		{kind: "lb", version: "sha256:abc"},
		{kind: "psql", version: "5.0.3"},
	} {
		t.Run(tc.kind+"/"+tc.version, func(t *testing.T) {
			req := Request{PrepareKind: tc.kind, ImageID: "image-1", LiquibaseArgs: []string{"update"}, PsqlArgs: []string{"-c", "select 1"}, LiquibaseVersion: tc.version}
			prepared, err := mgr.prepareRequest(req)
			if !tc.valid {
				if err == nil || !strings.Contains(err.Error(), "liquibase_version") {
					t.Fatalf("invalid selector accepted: %+v, %v", prepared.request, err)
				}
				return
			}
			if err != nil || prepared.request.LiquibaseVersion != tc.want {
				t.Fatalf("selector=%q, want %q, error=%v", prepared.request.LiquibaseVersion, tc.want, err)
			}
		})
	}
}
