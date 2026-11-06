package app

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sqlrs/cli/internal/cli"
	"github.com/sqlrs/cli/internal/client"
	"github.com/sqlrs/cli/internal/config"
	"github.com/sqlrs/cli/internal/paths"
	"github.com/sqlrs/cli/internal/refctx"
)

func TestStagePipelinePlanPsqlRendersHumanAndJSONOutputs(t *testing.T) {
	prevBind := bindPreparePsqlInputsFn
	bindPreparePsqlInputsFn = func(cli.PrepareOptions, string, string, prepareArgs, *refctx.Context, io.Reader) (prepareStageBinding, error) {
		return prepareStageBinding{PsqlArgs: []string{"-c", "select 1"}}, nil
	}
	t.Cleanup(func() { bindPreparePsqlInputsFn = prevBind })

	prevRunPlan := runPlanFn
	runPlanCalls := 0
	runPlanFn = func(context.Context, cli.PrepareOptions) (cli.PlanResult, error) {
		runPlanCalls++
		return cli.PlanResult{
			PrepareKind:           "psql",
			ImageID:               "img",
			PrepareArgsNormalized: "-c select 1",
			Tasks: []client.PlanTask{
				{TaskID: "plan", Type: "plan", PlannerKind: "psql"},
				{TaskID: "execute-0", Type: "state_execute", OutputStateID: "state-1"},
			},
		}, nil
	}
	t.Cleanup(func() { runPlanFn = prevRunPlan })

	var human bytes.Buffer
	if err := runPlanKindParsedWithPathMode(&human, io.Discard, cli.PrepareOptions{}, config.LoadedConfig{}, "", "", prepareArgs{
		Image:    "img",
		PsqlArgs: []string{"-c", "select 1"},
	}, nil, "human", "psql", true); err != nil {
		t.Fatalf("runPlanKindParsedWithPathMode(human): %v", err)
	}
	if !strings.Contains(human.String(), "Final state: state-1") {
		t.Fatalf("unexpected human output: %q", human.String())
	}

	var jsonOut bytes.Buffer
	if err := runPlanKindParsedWithPathMode(&jsonOut, io.Discard, cli.PrepareOptions{}, config.LoadedConfig{}, "", "", prepareArgs{
		Image:    "img",
		PsqlArgs: []string{"-c", "select 1"},
	}, nil, "json", "psql", true); err != nil {
		t.Fatalf("runPlanKindParsedWithPathMode(json): %v", err)
	}
	var got cli.PlanResult
	if err := json.Unmarshal(jsonOut.Bytes(), &got); err != nil {
		t.Fatalf("json.Unmarshal(plan output): %v", err)
	}
	if got.PrepareKind != "psql" || got.ImageID != "img" {
		t.Fatalf("unexpected json output: %+v", got)
	}
	if runPlanCalls != 2 {
		t.Fatalf("runPlan calls = %d, want 2", runPlanCalls)
	}
}

func TestStagePipelinePreparePsqlWatchRunsPrepare(t *testing.T) {
	prevBind := bindPreparePsqlInputsFn
	bindPreparePsqlInputsFn = func(cli.PrepareOptions, string, string, prepareArgs, *refctx.Context, io.Reader) (prepareStageBinding, error) {
		return prepareStageBinding{PsqlArgs: []string{"-c", "select 1"}}, nil
	}
	t.Cleanup(func() { bindPreparePsqlInputsFn = prevBind })

	prevSubmit := submitPrepareFn
	submitPrepareFn = func(context.Context, cli.PrepareOptions) (client.PrepareJobAccepted, error) {
		t.Fatal("submitPrepareFn should not be called for watched prepare")
		return client.PrepareJobAccepted{}, nil
	}
	t.Cleanup(func() { submitPrepareFn = prevSubmit })

	prevRunPrepare := runPrepareFn
	runPrepareFn = func(_ context.Context, opts cli.PrepareOptions) (client.PrepareJobResult, error) {
		if opts.PrepareKind != "psql" {
			t.Fatalf("PrepareKind = %q, want %q", opts.PrepareKind, "psql")
		}
		if got := strings.Join(opts.PsqlArgs, " "); got != "-c select 1" {
			t.Fatalf("PsqlArgs = %q", got)
		}
		return client.PrepareJobResult{DSN: "dsn"}, nil
	}
	t.Cleanup(func() { runPrepareFn = prevRunPrepare })

	result, handled, err := prepareResultParsed(stdoutAndErr{stdout: &bytes.Buffer{}, stderr: io.Discard}, cli.PrepareOptions{}, config.LoadedConfig{}, "", "", prepareArgs{
		Image:    "img",
		PsqlArgs: []string{"-c", "select 1"},
		Watch:    true,
	}, nil)
	if err != nil {
		t.Fatalf("prepareResultParsed: %v", err)
	}
	if handled {
		t.Fatalf("expected handled=false")
	}
	if result.DSN != "dsn" {
		t.Fatalf("unexpected result: %+v", result)
	}
}

func TestStagePipelinePrepareNoWatchSubmitsAndPrintsRefs(t *testing.T) {
	prevBind := bindPreparePsqlInputsFn
	bindPreparePsqlInputsFn = func(cli.PrepareOptions, string, string, prepareArgs, *refctx.Context, io.Reader) (prepareStageBinding, error) {
		return prepareStageBinding{PsqlArgs: []string{"-c", "select 1"}}, nil
	}
	t.Cleanup(func() { bindPreparePsqlInputsFn = prevBind })

	prevRunPrepare := runPrepareFn
	runPrepareFn = func(context.Context, cli.PrepareOptions) (client.PrepareJobResult, error) {
		t.Fatal("runPrepareFn should not be called for --no-watch")
		return client.PrepareJobResult{}, nil
	}
	t.Cleanup(func() { runPrepareFn = prevRunPrepare })

	prevSubmit := submitPrepareFn
	submitPrepareFn = func(_ context.Context, opts cli.PrepareOptions) (client.PrepareJobAccepted, error) {
		if opts.PrepareKind != "psql" {
			t.Fatalf("PrepareKind = %q, want %q", opts.PrepareKind, "psql")
		}
		return client.PrepareJobAccepted{
			JobID:     "job-1",
			StatusURL: "/v1/prepare-jobs/job-1",
			EventsURL: "/v1/prepare-jobs/job-1/events",
		}, nil
	}
	t.Cleanup(func() { submitPrepareFn = prevSubmit })

	var stdout bytes.Buffer
	_, handled, err := prepareResultParsed(stdoutAndErr{stdout: &stdout, stderr: io.Discard}, cli.PrepareOptions{CompositeRun: true}, config.LoadedConfig{}, "", "", prepareArgs{
		Image:          "img",
		PsqlArgs:       []string{"-c", "select 1"},
		Watch:          false,
		WatchSpecified: true,
	}, nil)
	if err != nil {
		t.Fatalf("prepareResultParsed: %v", err)
	}
	if !handled {
		t.Fatalf("expected handled=true")
	}
	out := stdout.String()
	if !strings.Contains(out, "JOB_ID=job-1") || !strings.Contains(out, "RUN_SKIPPED=prepare_not_watched") {
		t.Fatalf("unexpected stdout: %q", out)
	}
}

func TestStagePipelineLiquibaseBindsInputsWithoutHostRunner(t *testing.T) {
	t.Setenv("JAVA_HOME", "client-jdk-must-not-be-forwarded")
	cfgDir := t.TempDir()
	projectConfig := filepath.Join(cfgDir, ".sqlrs", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(projectConfig), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(projectConfig, []byte("liquibase: {}\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	prevBind := bindPrepareLiquibaseInputsFn
	bindCalls := 0
	bindPrepareLiquibaseInputsFn = func(_ cli.PrepareOptions, workspaceRoot string, cwd string, parsed prepareArgs, _ *refctx.Context, relativizePaths bool) (prepareStageBinding, error) {
		bindCalls++
		if !relativizePaths {
			t.Fatal("expected relativizePaths=true")
		}
		if workspaceRoot != cfgDir || cwd != cfgDir {
			t.Fatalf("unexpected workspace/cwd: %q %q", workspaceRoot, cwd)
		}
		if got := strings.Join(parsed.PsqlArgs, " "); got != "update --changelog-file master.xml" {
			t.Fatalf("unexpected parsed args: %q", got)
		}
		return prepareStageBinding{
			LiquibaseArgs: []string{"update", "--changelog-file", "master.xml"},
			WorkDir:       cwd,
		}, nil
	}
	t.Cleanup(func() { bindPrepareLiquibaseInputsFn = prevBind })

	prevRunPlan := runPlanFn
	runPlanFn = func(_ context.Context, opts cli.PrepareOptions) (cli.PlanResult, error) {
		if opts.WorkDir != cfgDir {
			t.Fatalf("WorkDir = %q, want %q", opts.WorkDir, cfgDir)
		}
		return cli.PlanResult{
			PrepareKind:           "lb",
			ImageID:               "img",
			PrepareArgsNormalized: "update --changelog-file master.xml",
			Tasks: []client.PlanTask{
				{TaskID: "plan", Type: "plan", PlannerKind: "lb"},
				{TaskID: "execute-0", Type: "state_execute", OutputStateID: "state-1"},
			},
		}, nil
	}
	t.Cleanup(func() { runPlanFn = prevRunPlan })

	err := runPlanKindParsedWithPathMode(&bytes.Buffer{}, io.Discard, cli.PrepareOptions{}, config.LoadedConfig{
		ProjectConfigPath: projectConfig,
		Paths:             paths.Dirs{ConfigDir: t.TempDir()},
	}, cfgDir, cfgDir, prepareArgs{
		Image:    "img",
		PsqlArgs: []string{"update", "--changelog-file", "master.xml"},
	}, nil, "json", "lb", true)
	if err != nil {
		t.Fatalf("runPlanKindParsedWithPathMode: %v", err)
	}
	if bindCalls != 1 {
		t.Fatalf("bindPrepareLiquibaseInputs calls = %d, want 1", bindCalls)
	}
}

func TestStagePipelineRejectsPlanWatchFlagsBeforeInvocation(t *testing.T) {
	prevBind := bindPreparePsqlInputsFn
	bindPreparePsqlInputsFn = func(cli.PrepareOptions, string, string, prepareArgs, *refctx.Context, io.Reader) (prepareStageBinding, error) {
		t.Fatal("bindPreparePsqlInputsFn should not be called")
		return prepareStageBinding{}, nil
	}
	t.Cleanup(func() { bindPreparePsqlInputsFn = prevBind })

	prevRunPlan := runPlanFn
	runPlanFn = func(context.Context, cli.PrepareOptions) (cli.PlanResult, error) {
		t.Fatal("runPlanFn should not be called")
		return cli.PlanResult{}, nil
	}
	t.Cleanup(func() { runPlanFn = prevRunPlan })

	err := runPlanKindParsedWithPathMode(&bytes.Buffer{}, io.Discard, cli.PrepareOptions{}, config.LoadedConfig{}, "", "", prepareArgs{
		Image:          "img",
		PsqlArgs:       []string{"-c", "select 1"},
		Watch:          false,
		WatchSpecified: true,
	}, nil, "json", "psql", true)
	if err == nil || !strings.Contains(err.Error(), "plan does not support --watch/--no-watch") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestStagePipelineRejectsLiquibaseWithoutCommand(t *testing.T) {
	prevBind := bindPrepareLiquibaseInputsFn
	bindPrepareLiquibaseInputsFn = func(cli.PrepareOptions, string, string, prepareArgs, *refctx.Context, bool) (prepareStageBinding, error) {
		t.Fatal("bindPrepareLiquibaseInputsFn should not be called")
		return prepareStageBinding{}, nil
	}
	t.Cleanup(func() { bindPrepareLiquibaseInputsFn = prevBind })

	_, _, err := prepareResultLiquibaseParsedWithPathMode(stdoutAndErr{stdout: &bytes.Buffer{}, stderr: io.Discard}, cli.PrepareOptions{}, config.LoadedConfig{}, "", "", prepareArgs{
		Image: "img",
		Watch: true,
	}, nil, true)
	if err == nil || !strings.Contains(err.Error(), "liquibase command is required") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAliasBackedPlanAndPrepareReuseSharedStagePipeline(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "demo.prep.s9s.yaml"), []byte("kind: psql\nimage: img\nargs:\n  - -c\n  - select 1\n"), 0o600); err != nil {
		t.Fatalf("write alias: %v", err)
	}

	prevBind := bindPreparePsqlInputsFn
	var seen []string
	bindPreparePsqlInputsFn = func(_ cli.PrepareOptions, workspaceRoot string, cwd string, parsed prepareArgs, _ *refctx.Context, _ io.Reader) (prepareStageBinding, error) {
		if workspaceRoot != root || cwd != root {
			t.Fatalf("unexpected workspace/cwd: %q %q", workspaceRoot, cwd)
		}
		seen = append(seen, parsed.Image+"|"+strings.Join(parsed.PsqlArgs, " "))
		return prepareStageBinding{PsqlArgs: parsed.PsqlArgs}, nil
	}
	t.Cleanup(func() { bindPreparePsqlInputsFn = prevBind })

	prevRunPrepare := runPrepareFn
	runPrepareFn = func(_ context.Context, opts cli.PrepareOptions) (client.PrepareJobResult, error) {
		if opts.PrepareKind != "psql" {
			t.Fatalf("unexpected prepare kind: %q", opts.PrepareKind)
		}
		return client.PrepareJobResult{DSN: "dsn"}, nil
	}
	t.Cleanup(func() { runPrepareFn = prevRunPrepare })

	prevRunPlan := runPlanFn
	runPlanFn = func(_ context.Context, opts cli.PrepareOptions) (cli.PlanResult, error) {
		if opts.PrepareKind != "psql" || !opts.PlanOnly {
			t.Fatalf("unexpected plan opts: %+v", opts)
		}
		return cli.PlanResult{
			PrepareKind:           "psql",
			ImageID:               "img",
			PrepareArgsNormalized: "-c select 1",
			Tasks: []client.PlanTask{
				{TaskID: "plan", Type: "plan", PlannerKind: "psql"},
				{TaskID: "execute-0", Type: "state_execute", OutputStateID: "state-1"},
			},
		}, nil
	}
	t.Cleanup(func() { runPlanFn = prevRunPlan })

	opts := cli.GlobalOptions{}
	configure := func(deps *runnerDeps) {
		deps.getwd = func() (string, error) { return root, nil }
		deps.resolveCommandContext = func(string, cli.GlobalOptions) (commandContext, error) {
			return testCommandContext(root, "json", false), nil
		}
	}

	if err := runWithParsedCommands(t, opts, []cli.Command{{Name: "prepare", Args: []string{"demo"}}}, configure); err != nil {
		t.Fatalf("prepare alias run: %v", err)
	}
	if err := runWithParsedCommands(t, opts, []cli.Command{{Name: "plan", Args: []string{"demo"}}}, configure); err != nil {
		t.Fatalf("plan alias run: %v", err)
	}

	if len(seen) != 2 {
		t.Fatalf("bindPreparePsqlInputs calls = %d, want 2", len(seen))
	}
	for _, got := range seen {
		if got != "img|-c select 1" {
			t.Fatalf("unexpected bound alias args: %q", got)
		}
	}
}
