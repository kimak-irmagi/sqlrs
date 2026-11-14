package app

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"github.com/sqlrs/cli/internal/cli"
	"github.com/sqlrs/cli/internal/client"
	"github.com/sqlrs/cli/internal/config"
)

// TestDirectLiquibaseCommandsBindVersion verifies C01a in
// docs/architecture/liquibase-container-tests.md at each direct command
// boundary, before the engine resolves the selector to an image digest.
func TestDirectLiquibaseCommandsBindVersion(t *testing.T) {
	t.Setenv("JAVA_HOME", "")
	var captured cli.PrepareOptions
	var calls int
	prevSubmit, prevPlan, prevExplain := submitPrepareFn, runPlanFn, explainPrepareCacheFn
	submitPrepareFn = func(_ context.Context, opts cli.PrepareOptions) (client.PrepareJobAccepted, error) {
		captured, calls = opts, calls+1
		return client.PrepareJobAccepted{JobID: "job-1"}, nil
	}
	runPlanFn = func(_ context.Context, opts cli.PrepareOptions) (cli.PlanResult, error) {
		captured, calls = opts, calls+1
		return cli.PlanResult{PrepareKind: "lb", ImageID: opts.ImageID}, nil
	}
	explainPrepareCacheFn = func(_ context.Context, opts cli.PrepareOptions) (client.CacheExplainPrepareResponse, error) {
		captured, calls = opts, calls+1
		return client.CacheExplainPrepareResponse{Decision: "miss", ReasonCode: "no_matching_state"}, nil
	}
	t.Cleanup(func() {
		submitPrepareFn, runPlanFn, explainPrepareCacheFn = prevSubmit, prevPlan, prevExplain
	})

	for _, tc := range []struct {
		name    string
		version string
		args    []string
	}{
		{name: "explicit", version: "5.0.3", args: []string{"--image", "postgres:17", "--liquibase-version", "5.0.3", "--", "update"}},
		{name: "explicit equals", version: "5.0.3", args: []string{"--image", "postgres:17", "--liquibase-version=5.0.3", "--", "update"}},
		{name: "default", version: "latest", args: []string{"--image", "postgres:17", "--", "update"}},
	} {
		for _, command := range []string{"prepare:lb", "plan:lb", "cache explain prepare:lb"} {
			t.Run(command+"/"+tc.name, func(t *testing.T) {
				root := t.TempDir()
				calls = 0
				var err error
				switch command {
				case "prepare:lb":
					args := append([]string{"--no-watch"}, tc.args...)
					_, _, err = prepareResultLiquibase(stdoutAndErr{stdout: io.Discard, stderr: io.Discard}, cli.PrepareOptions{}, config.LoadedConfig{}, root, root, args)
				case "plan:lb":
					err = runPlanKind(io.Discard, io.Discard, cli.PrepareOptions{}, config.LoadedConfig{}, root, root, tc.args, "json", "lb")
				case "cache explain prepare:lb":
					args := append([]string{"explain", "prepare:lb"}, tc.args...)
					err = runCache(io.Discard, io.Discard, cli.PrepareOptions{}, config.LoadedConfig{}, root, root, args, "json")
				}
				if err != nil {
					t.Fatalf("%s rejected version %q: %v", command, tc.version, err)
				}
				if calls != 1 {
					t.Fatalf("%s made %d engine calls, want one", command, calls)
				}

				requestBytes, err := json.Marshal(cli.BuildPrepareJobRequest(captured, captured.PlanOnly))
				if err != nil {
					t.Fatal(err)
				}
				var request map[string]any
				if err := json.Unmarshal(requestBytes, &request); err != nil {
					t.Fatal(err)
				}
				if request["prepare_kind"] != "lb" || request["image_id"] != "postgres:17" {
					t.Fatalf("command selected wrong kind or base DB image: %s", requestBytes)
				}
				if tc.version != "latest" && request["liquibase_version"] != tc.version {
					t.Fatalf("version was not bound as a separate selector: %s", requestBytes)
				}
				if tc.name == "default" && request["liquibase_version"] != nil && request["liquibase_version"] != "latest" {
					t.Fatalf("omitted version must select latest: %s", requestBytes)
				}
				for _, arg := range captured.LiquibaseArgs {
					if strings.HasPrefix(arg, "--liquibase-version") || arg == "5.0.3" {
						t.Fatalf("sqlrs version option reached Liquibase args: %s", requestBytes)
					}
				}
			})
		}
	}
}

// TestDirectLiquibaseCommandsRejectInvalidVersion verifies C02a in
// docs/architecture/liquibase-container-tests.md: invalid selectors fail
// before a prepare, plan, or cache-explain request reaches the engine.
func TestDirectLiquibaseCommandsRejectInvalidVersion(t *testing.T) {
	t.Setenv("JAVA_HOME", "")
	var calls int
	prevSubmit, prevPlan, prevExplain := submitPrepareFn, runPlanFn, explainPrepareCacheFn
	submitPrepareFn = func(context.Context, cli.PrepareOptions) (client.PrepareJobAccepted, error) {
		calls++
		return client.PrepareJobAccepted{JobID: "job-1"}, nil
	}
	runPlanFn = func(context.Context, cli.PrepareOptions) (cli.PlanResult, error) {
		calls++
		return cli.PlanResult{}, nil
	}
	explainPrepareCacheFn = func(context.Context, cli.PrepareOptions) (client.CacheExplainPrepareResponse, error) {
		calls++
		return client.CacheExplainPrepareResponse{}, nil
	}
	t.Cleanup(func() {
		submitPrepareFn, runPlanFn, explainPrepareCacheFn = prevSubmit, prevPlan, prevExplain
	})
	for _, version := range []string{"", "liquibase/liquibase:latest", "sha256:abc", "../latest"} {
		for _, command := range []string{"prepare:lb", "plan:lb", "cache explain prepare:lb"} {
			t.Run(command+"/"+version, func(t *testing.T) {
				root := t.TempDir()
				calls = 0
				args := []string{"--image", "postgres:17", "--liquibase-version", version, "--", "update"}
				var err error
				switch command {
				case "prepare:lb":
					_, _, err = prepareResultLiquibase(stdoutAndErr{stdout: io.Discard, stderr: io.Discard}, cli.PrepareOptions{}, config.LoadedConfig{}, root, root, append([]string{"--no-watch"}, args...))
				case "plan:lb":
					err = runPlanKind(io.Discard, io.Discard, cli.PrepareOptions{}, config.LoadedConfig{}, root, root, args, "json", "lb")
				case "cache explain prepare:lb":
					err = runCache(io.Discard, io.Discard, cli.PrepareOptions{}, config.LoadedConfig{}, root, root, append([]string{"explain", "prepare:lb"}, args...), "json")
				}
				if err == nil || !strings.Contains(err.Error(), "liquibase-version") {
					t.Fatalf("invalid version %q should fail before engine call: %v", version, err)
				}
				if calls != 0 {
					t.Fatalf("invalid version %q caused %d engine calls", version, calls)
				}
			})
		}
	}
}

// The selector belongs to Liquibase transforms, not to the shared SQL
// preparation parser or the database base-image selection.
func TestPreparePsqlRejectsLiquibaseVersion(t *testing.T) {
	root := t.TempDir()
	var calls int
	previous := runPrepareFn
	runPrepareFn = func(context.Context, cli.PrepareOptions) (client.PrepareJobResult, error) {
		calls++
		return client.PrepareJobResult{}, nil
	}
	t.Cleanup(func() { runPrepareFn = previous })
	_, _, err := prepareResult(stdoutAndErr{stdout: io.Discard, stderr: io.Discard}, cli.PrepareOptions{}, config.LoadedConfig{}, root, root, []string{"--image", "postgres:17", "--liquibase-version", "5.0.3", "--", "-c", "select 1"})
	if err == nil || !strings.Contains(err.Error(), "--liquibase-version") {
		t.Fatalf("psql accepted Liquibase selector: %v", err)
	}
	if calls != 0 {
		t.Fatalf("psql reached engine %d times", calls)
	}
}
