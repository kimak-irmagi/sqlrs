package app

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"

	"github.com/sqlrs/cli/internal/cli"
	"github.com/sqlrs/cli/internal/client"
	"github.com/sqlrs/cli/internal/config"
	"github.com/sqlrs/cli/internal/paths"
	"github.com/sqlrs/cli/internal/refctx"
)

func TestRunPrepareParsedPrintsDSN(t *testing.T) {
	prevBind := bindPreparePsqlInputsFn
	bindPreparePsqlInputsFn = func(cli.PrepareOptions, string, string, prepareArgs, *refctx.Context, io.Reader) (prepareStageBinding, error) {
		return prepareStageBinding{PsqlArgs: []string{"-c", "select 1"}}, nil
	}
	t.Cleanup(func() { bindPreparePsqlInputsFn = prevBind })

	prevRunPrepare := runPrepareFn
	runPrepareFn = func(_ context.Context, opts cli.PrepareOptions) (client.PrepareJobResult, error) {
		if opts.PrepareKind != "psql" {
			t.Fatalf("PrepareKind = %q, want %q", opts.PrepareKind, "psql")
		}
		return client.PrepareJobResult{DSN: "postgres://sqlrs@local/instance/psql"}, nil
	}
	t.Cleanup(func() { runPrepareFn = prevRunPrepare })

	root := t.TempDir()
	cfg := config.LoadedConfig{Paths: paths.Dirs{ConfigDir: t.TempDir()}}
	var stdout bytes.Buffer
	err := runPrepareParsed(&stdout, io.Discard, cli.PrepareOptions{}, cfg, root, root, prepareArgs{
		Image:    "image-1",
		PsqlArgs: []string{"-c", "select 1"},
		Watch:    true,
	}, nil)
	if err != nil {
		t.Fatalf("runPrepareParsed: %v", err)
	}
	if !strings.Contains(stdout.String(), "DSN=postgres://sqlrs@local/instance/psql") {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
}

func TestRunPrepareParsedHandlesSubmittedPrepare(t *testing.T) {
	prevBind := bindPreparePsqlInputsFn
	bindPreparePsqlInputsFn = func(cli.PrepareOptions, string, string, prepareArgs, *refctx.Context, io.Reader) (prepareStageBinding, error) {
		return prepareStageBinding{PsqlArgs: []string{"-c", "select 1"}}, nil
	}
	t.Cleanup(func() { bindPreparePsqlInputsFn = prevBind })

	prevRunPrepare := runPrepareFn
	runPrepareFn = func(context.Context, cli.PrepareOptions) (client.PrepareJobResult, error) {
		t.Fatal("runPrepareFn should not be called for handled prepare")
		return client.PrepareJobResult{}, nil
	}
	t.Cleanup(func() { runPrepareFn = prevRunPrepare })

	prevSubmit := submitPrepareFn
	submitPrepareFn = func(context.Context, cli.PrepareOptions) (client.PrepareJobAccepted, error) {
		return client.PrepareJobAccepted{
			JobID:     "job-psql",
			StatusURL: "/v1/prepare-jobs/job-psql",
			EventsURL: "/v1/prepare-jobs/job-psql/events",
		}, nil
	}
	t.Cleanup(func() { submitPrepareFn = prevSubmit })

	root := t.TempDir()
	cfg := config.LoadedConfig{Paths: paths.Dirs{ConfigDir: t.TempDir()}}
	var stdout bytes.Buffer
	err := runPrepareParsed(&stdout, io.Discard, cli.PrepareOptions{}, cfg, root, root, prepareArgs{
		Image:          "image-1",
		PsqlArgs:       []string{"-c", "select 1"},
		WatchSpecified: true,
	}, nil)
	if err != nil {
		t.Fatalf("runPrepareParsed: %v", err)
	}
	if !strings.Contains(stdout.String(), "JOB_ID=job-psql") || strings.Contains(stdout.String(), "DSN=") {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
}

func TestRunPrepareParsedReturnsError(t *testing.T) {
	root := t.TempDir()
	cfg := config.LoadedConfig{Paths: paths.Dirs{ConfigDir: t.TempDir()}}
	err := runPrepareParsed(&bytes.Buffer{}, io.Discard, cli.PrepareOptions{}, cfg, root, root, prepareArgs{
		PsqlArgs: []string{"-c", "select 1"},
		Watch:    true,
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "Missing base image id") {
		t.Fatalf("expected missing image error, got %v", err)
	}
}

func TestRunPrepareLiquibaseParsedWithPathModePrintsDSN(t *testing.T) {
	prevBind := bindPrepareLiquibaseInputsFn
	bindPrepareLiquibaseInputsFn = func(cli.PrepareOptions, string, string, prepareArgs, *refctx.Context, bool) (prepareStageBinding, error) {
		return prepareStageBinding{
			LiquibaseArgs: []string{"update", "--changelog-file", "master.xml"},
			WorkDir:       t.TempDir(),
		}, nil
	}
	t.Cleanup(func() { bindPrepareLiquibaseInputsFn = prevBind })

	prevRunPrepare := runPrepareFn
	runPrepareFn = func(_ context.Context, opts cli.PrepareOptions) (client.PrepareJobResult, error) {
		if opts.PrepareKind != "lb" {
			t.Fatalf("PrepareKind = %q, want %q", opts.PrepareKind, "lb")
		}
		return client.PrepareJobResult{DSN: "postgres://sqlrs@local/instance/lb"}, nil
	}
	t.Cleanup(func() { runPrepareFn = prevRunPrepare })

	root := t.TempDir()
	cfg := config.LoadedConfig{Paths: paths.Dirs{ConfigDir: t.TempDir()}}
	var stdout bytes.Buffer
	err := runPrepareLiquibaseParsedWithPathMode(&stdout, io.Discard, cli.PrepareOptions{}, cfg, root, root, prepareArgs{
		Image:    "image-1",
		PsqlArgs: []string{"update", "--changelog-file", "master.xml"},
		Watch:    true,
	}, nil, true)
	if err != nil {
		t.Fatalf("runPrepareLiquibaseParsedWithPathMode: %v", err)
	}
	if !strings.Contains(stdout.String(), "DSN=postgres://sqlrs@local/instance/lb") {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
}

func TestRunPrepareLiquibaseParsedWithPathModeHandlesSubmittedPrepare(t *testing.T) {
	prevBind := bindPrepareLiquibaseInputsFn
	bindPrepareLiquibaseInputsFn = func(cli.PrepareOptions, string, string, prepareArgs, *refctx.Context, bool) (prepareStageBinding, error) {
		return prepareStageBinding{
			LiquibaseArgs: []string{"update", "--changelog-file", "master.xml"},
			WorkDir:       t.TempDir(),
		}, nil
	}
	t.Cleanup(func() { bindPrepareLiquibaseInputsFn = prevBind })

	prevRunPrepare := runPrepareFn
	runPrepareFn = func(context.Context, cli.PrepareOptions) (client.PrepareJobResult, error) {
		t.Fatal("runPrepareFn should not be called for handled prepare")
		return client.PrepareJobResult{}, nil
	}
	t.Cleanup(func() { runPrepareFn = prevRunPrepare })

	prevSubmit := submitPrepareFn
	submitPrepareFn = func(context.Context, cli.PrepareOptions) (client.PrepareJobAccepted, error) {
		return client.PrepareJobAccepted{
			JobID:     "job-lb",
			StatusURL: "/v1/prepare-jobs/job-lb",
			EventsURL: "/v1/prepare-jobs/job-lb/events",
		}, nil
	}
	t.Cleanup(func() { submitPrepareFn = prevSubmit })

	root := t.TempDir()
	cfg := config.LoadedConfig{Paths: paths.Dirs{ConfigDir: t.TempDir()}}
	var stdout bytes.Buffer
	err := runPrepareLiquibaseParsedWithPathMode(&stdout, io.Discard, cli.PrepareOptions{}, cfg, root, root, prepareArgs{
		Image:          "image-1",
		PsqlArgs:       []string{"update", "--changelog-file", "master.xml"},
		WatchSpecified: true,
	}, nil, true)
	if err != nil {
		t.Fatalf("runPrepareLiquibaseParsedWithPathMode: %v", err)
	}
	if !strings.Contains(stdout.String(), "JOB_ID=job-lb") || strings.Contains(stdout.String(), "DSN=") {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
}

func TestRunPrepareLiquibaseParsedWithPathModeReturnsError(t *testing.T) {
	root := t.TempDir()
	cfg := config.LoadedConfig{Paths: paths.Dirs{ConfigDir: t.TempDir()}}
	err := runPrepareLiquibaseParsedWithPathMode(&bytes.Buffer{}, io.Discard, cli.PrepareOptions{}, cfg, root, root, prepareArgs{
		Image: "image-1",
	}, nil, true)
	if err == nil || !strings.Contains(err.Error(), "liquibase command is required") {
		t.Fatalf("expected missing liquibase command error, got %v", err)
	}
}
