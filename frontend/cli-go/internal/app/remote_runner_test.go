package app

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/sqlrs/cli/internal/cli"
	"github.com/sqlrs/cli/internal/client"
	"github.com/sqlrs/cli/internal/config"
	"github.com/sqlrs/cli/internal/paths"
	"github.com/sqlrs/cli/internal/refctx"
)

// TestRemoteStageRejectsRemovedRunnerSettings covers C02a in
// docs/architecture/liquibase-container-tests.md for all direct commands.
func TestRemoteStageRejectsRemovedRunnerSettings(t *testing.T) {
	prior := bindPrepareLiquibaseInputsFn
	bindPrepareLiquibaseInputsFn = func(_ cli.PrepareOptions, _, cwd string, _ prepareArgs, _ *refctx.Context, _ bool) (prepareStageBinding, error) {
		return prepareStageBinding{LiquibaseArgs: []string{"update", "--changelog-file", "changelog.xml"}, WorkDir: cwd}, nil
	}
	t.Cleanup(func() { bindPrepareLiquibaseInputsFn = prior })
	priorExplain := explainPrepareCacheFn
	explainCalls := 0
	explainPrepareCacheFn = func(context.Context, cli.PrepareOptions) (client.CacheExplainPrepareResponse, error) {
		explainCalls++
		return client.CacheExplainPrepareResponse{}, nil
	}
	t.Cleanup(func() { explainPrepareCacheFn = priorExplain })
	for _, setting := range []struct {
		name string
		yaml string
	}{
		{name: "exec", yaml: "liquibase:\n  exec: C:\\Program Files\\Liquibase\\liquibase.exe\n"},
		{name: "exec_mode", yaml: "liquibase:\n  exec_mode: windows-bat\n"},
		{name: "env", yaml: "liquibase:\n  env:\n    JAVA_HOME: client-jdk\n"},
	} {
		for _, command := range []string{"prepare:lb", "plan:lb", "cache explain prepare:lb"} {
			t.Run(setting.name+"/"+command, func(t *testing.T) {
				root := t.TempDir()
				projectConfig := filepath.Join(root, ".sqlrs", "config.yaml")
				if err := os.MkdirAll(filepath.Dir(projectConfig), 0o700); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(projectConfig, []byte(setting.yaml), 0o600); err != nil {
					t.Fatal(err)
				}
				cfg := config.LoadedConfig{ProjectConfigPath: projectConfig, Paths: paths.Dirs{ConfigDir: t.TempDir()}}
				opts := cli.PrepareOptions{Mode: "remote", Endpoint: "http://127.0.0.1:1", SourceSyncMode: "off"}
				var err error
				switch command {
				case "prepare:lb", "plan:lb":
					mode := stageModePrepare
					if command == "plan:lb" {
						mode = stageModePlan
					}
					_, err = buildStageRuntime(io.Discard, opts, cfg, stageRunRequest{
						mode: mode, kind: "lb", workspaceRoot: root, cwd: root,
						parsed: prepareArgs{Image: "image-1", PsqlArgs: []string{"update", "--changelog-file", "changelog.xml"}},
					})
				case "cache explain prepare:lb":
					explainCalls = 0
					err = runCache(io.Discard, io.Discard, opts, cfg, root, root, []string{"explain", "prepare:lb", "--image", "image-1", "--", "update"}, "json")
					if explainCalls != 0 {
						t.Fatalf("removed workspace setting reached cache explain %d times", explainCalls)
					}
				}
				if err == nil || !strings.Contains(err.Error(), "liquibase."+setting.name) {
					t.Fatalf("expected removed %s setting error, got %v", setting.name, err)
				}
			})
		}
	}
}

// TestRemoteCacheExplainOmitsRunnerBytes protects the separate provenance
// transport path, which must share prepare-job request construction.
func TestRemoteCacheExplainOmitsRunnerBytes(t *testing.T) {
	var body atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/v1/cache/explain/prepare":
			data, _ := io.ReadAll(r.Body)
			body.Store(string(data))
			w.Header().Set("Location", "/v1/prepare-jobs/job-lb")
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `{"job_id":"job-lb","status_url":"/v1/prepare-jobs/job-lb","status":"queued"}`)
		case "/v1/prepare-jobs/job-lb":
			_, _ = io.WriteString(w, `{"job_id":"job-lb","status":"succeeded","prepare_kind":"lb","purpose":"cache_explain","image_id":"image-1","plan_only":true,"cache_explain_result":{"decision":"miss","reason_code":"no_matching_state","signature":"sig-lb"}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)
	_, err := explainPrepareCache(context.Background(), cli.PrepareOptions{
		Mode: "remote", Endpoint: server.URL, PrepareKind: "lb", ImageID: "image-1",
		LiquibaseArgs: []string{"update"},
	})
	if err != nil {
		t.Fatal(err)
	}
	requestBody, _ := body.Load().(string)
	for _, key := range []string{"liquibase_exec", "liquibase_exec_mode", "liquibase_env"} {
		if strings.Contains(requestBody, `"`+key+`"`) {
			t.Fatalf("cache explain leaked %s: %s", key, requestBody)
		}
	}
}
