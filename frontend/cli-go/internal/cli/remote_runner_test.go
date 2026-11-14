package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/sqlrs/cli/internal/remotesource"
)

// All three direct Liquibase commands expose the selector from the approved
// user contract in docs/user-guides/sqlrs-liquibase-container-cli-design.md.
func TestLiquibaseUsageShowsVersionSelector(t *testing.T) {
	for _, printUsage := range []func(io.Writer){PrintPrepareUsage, PrintPlanUsage, PrintCacheUsage} {
		var out bytes.Buffer
		printUsage(&out)
		if !strings.Contains(out.String(), "--liquibase-version") {
			t.Fatalf("Liquibase version selector missing from usage: %s", out.String())
		}
	}
}

// TestRemotePrepareRequestOmitsRunnerProperties checks the shared transport
// builder and actual create-job bytes from the remote-runner CLI design.
func TestRemotePrepareRequestOmitsRunnerProperties(t *testing.T) {
	for _, syncEnabled := range []bool{false, true} {
		for _, planOnly := range []bool{false, true} {
			t.Run(strings.Join([]string{boolLabel(syncEnabled), boolLabel(planOnly)}, "/"), func(t *testing.T) {
				var bodies [][]byte
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					body, _ := io.ReadAll(r.Body)
					bodies = append(bodies, body)
					w.WriteHeader(http.StatusCreated)
					_, _ = io.WriteString(w, `{"job_id":"job-1","status_url":"/v1/prepare-jobs/job-1"}`)
				}))
				t.Cleanup(server.Close)
				opts := PrepareOptions{
					Mode: "remote", Endpoint: server.URL, PrepareKind: "lb", ImageID: "image-1",
					LiquibaseArgs: []string{"update"},
				}
				if syncEnabled {
					opts.SourceSync = &remotesource.Options{Enabled: true, WorkspaceRoot: t.TempDir()}
				}
				built := BuildPrepareJobRequest(opts, planOnly)
				assertNoRunnerJSON(t, built)
				if _, _, err := createPrepareJob(context.Background(), opts, planOnly); err != nil {
					t.Fatal(err)
				}
				if len(bodies) != 1 {
					t.Fatalf("POST count = %d", len(bodies))
				}
				assertNoRunnerBytes(t, bodies[0])
			})
		}
	}
}

func assertNoRunnerJSON(t *testing.T, value any) {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	assertNoRunnerBytes(t, data)
}

func assertNoRunnerBytes(t *testing.T, data []byte) {
	t.Helper()
	for _, key := range []string{"liquibase_exec", "liquibase_exec_mode", "liquibase_env"} {
		if strings.Contains(string(data), `"`+key+`"`) {
			t.Fatalf("remote request contains %s: %s", key, data)
		}
	}
}

func boolLabel(v bool) string {
	if v {
		return "on"
	}
	return "off"
}
