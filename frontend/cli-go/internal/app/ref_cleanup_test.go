package app

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sqlrs/cli/internal/cli"
	"github.com/sqlrs/cli/internal/client"
	"github.com/sqlrs/cli/internal/config"
	"github.com/sqlrs/cli/internal/inputset"
	"github.com/sqlrs/cli/internal/refctx"
)

func TestBindPreparePsqlInputsCleansWorktreeOnError(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not in PATH")
	}

	repo, _ := initPrepareRefTestRepo(t)
	cwd := filepath.Join(repo, "examples")
	before := countGitWorktrees(t, repo)

	_, err := bindPreparePsqlInputs(
		cli.PrepareOptions{},
		repo,
		cwd,
		prepareArgs{Ref: "HEAD", RefMode: "worktree", PsqlArgs: []string{"-f", "../../outside.sql"}},
		nil,
		strings.NewReader(""),
	)
	if err == nil {
		t.Fatal("expected bindPreparePsqlInputs error")
	}

	after := countGitWorktrees(t, repo)
	if after != before {
		t.Fatalf("worktree count = %d, want %d after cleanup", after, before)
	}
}

func TestBindPrepareLiquibaseInputsCleansWorktreeOnError(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not in PATH")
	}

	repo, _ := initPrepareRefTestRepo(t)
	cwd := filepath.Join(repo, "examples")
	before := countGitWorktrees(t, repo)

	_, err := bindPrepareLiquibaseInputs(
		cli.PrepareOptions{},
		repo,
		cwd,
		prepareArgs{Ref: "HEAD", RefMode: "worktree", PsqlArgs: []string{"update", "--changelog-file", "../../outside.xml"}},
		nil,
		false,
	)
	if err == nil {
		t.Fatal("expected bindPrepareLiquibaseInputs error")
	}

	after := countGitWorktrees(t, repo)
	if after != before {
		t.Fatalf("worktree count = %d, want %d after cleanup", after, before)
	}
}

func TestBuildStageRuntimeLiquibaseConfigErrorCleansWorktreeOnError(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not in PATH")
	}

	repo, _ := initPrepareRefTestRepo(t)
	cwd := filepath.Join(repo, "examples")
	before := countGitWorktrees(t, repo)

	configPath := filepath.Join(t.TempDir(), "broken-config.yaml")
	if err := os.WriteFile(configPath, []byte("liquibase:\n  exec: [\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}

	_, err := buildStageRuntime(io.Discard, cli.PrepareOptions{}, config.LoadedConfig{
		ProjectConfigPath: configPath,
	}, stageRunRequest{
		mode:          stageModePlan,
		kind:          "lb",
		parsed:        prepareArgs{Ref: "HEAD", RefMode: "worktree", Image: "img", PsqlArgs: []string{"update"}},
		workspaceRoot: repo,
		cwd:           cwd,
		invocationCwd: cwd,
	})
	if err == nil {
		t.Fatal("expected buildStageRuntime error")
	}

	after := countGitWorktrees(t, repo)
	if after != before {
		t.Fatalf("worktree count = %d, want %d after cleanup", after, before)
	}
}

func TestBuildStageRuntimeLiquibaseRefUsesProjectedWorktreePaths(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not in PATH")
	}

	repo, parentRef := initPrepareRefTestRepo(t)
	cwd := filepath.Join(repo, "examples")
	before := countGitWorktrees(t, repo)

	runtime, err := buildStageRuntime(io.Discard, cli.PrepareOptions{}, config.LoadedConfig{}, stageRunRequest{
		mode:                    stageModePrepare,
		kind:                    "lb",
		parsed:                  prepareArgs{Ref: parentRef, RefMode: "worktree", Image: "img", PsqlArgs: []string{"update", "--changelog-file", "config/liquibase/master.xml"}},
		workspaceRoot:           repo,
		cwd:                     cwd,
		invocationCwd:           cwd,
		relativizeLiquibasePath: true,
	})
	if err != nil {
		t.Fatalf("buildStageRuntime: %v", err)
	}
	if runtime.cleanup == nil {
		t.Fatal("expected cleanup for projected worktree")
	}
	t.Cleanup(func() {
		if err := runtime.cleanup(); err != nil {
			t.Fatalf("cleanup: %v", err)
		}
		after := countGitWorktrees(t, repo)
		if after != before {
			t.Fatalf("worktree count = %d, want %d after cleanup", after, before)
		}
	})

	if runtime.actualRef == nil {
		t.Fatal("expected resolved ref context")
	}
	if runtime.opts.WorkDir != runtime.actualRef.BaseDir {
		t.Fatalf("WorkDir = %q, want projected base dir %q", runtime.opts.WorkDir, runtime.actualRef.BaseDir)
	}
	if len(runtime.opts.LiquibaseArgs) < 3 {
		t.Fatalf("unexpected liquibase args: %+v", runtime.opts.LiquibaseArgs)
	}
	if got, want := filepath.Clean(runtime.opts.LiquibaseArgs[2]), filepath.Join("config", "liquibase", "master.xml"); got != want {
		t.Fatalf("changelog arg = %q, want %q", runtime.opts.LiquibaseArgs[2], want)
	}
}

func TestBindPrepareLiquibaseInputsBlobStagesAdditionalLocalAssets(t *testing.T) {
	root := t.TempDir()
	writeTestFile(
		t,
		root,
		"master.xml",
		`<databaseChangeLog xmlns="http://www.liquibase.org/xml/ns/dbchangelog"><loadData file="seed.csv" tableName="seed" relativeToChangelogFile="true"/></databaseChangeLog>`+"\n",
	)
	writeTestFile(t, root, "seed.csv", "id,name\n1,example\n")

	bound, err := bindPrepareLiquibaseInputs(
		cli.PrepareOptions{},
		root,
		root,
		prepareArgs{PsqlArgs: []string{"update", "--changelog-file", "master.xml"}},
		&refctx.Context{
			WorkspaceRoot: root,
			BaseDir:       root,
			RefMode:       "blob",
			FileSystem:    inputset.OSFileSystem{},
		},
		false,
	)
	if err != nil {
		t.Fatalf("bindPrepareLiquibaseInputs: %v", err)
	}
	if bound.cleanup != nil {
		t.Cleanup(func() {
			if err := bound.cleanup(); err != nil {
				t.Fatalf("cleanup: %v", err)
			}
		})
	}

	if len(bound.LiquibaseArgs) < 3 {
		t.Fatalf("unexpected liquibase args: %+v", bound.LiquibaseArgs)
	}
	changelogPath := strings.TrimSpace(bound.LiquibaseArgs[2])
	if changelogPath == "" {
		t.Fatalf("expected staged changelog path, got %+v", bound.LiquibaseArgs)
	}
	assetPath := filepath.Join(filepath.Dir(changelogPath), "seed.csv")
	data, err := os.ReadFile(assetPath)
	if err != nil {
		t.Fatalf("expected staged asset %q: %v", assetPath, err)
	}
	if string(data) != "id,name\n1,example\n" {
		t.Fatalf("unexpected staged asset contents: %q", string(data))
	}
}

func TestPrepareResultParsedPropagatesCleanupError(t *testing.T) {
	prevBind := bindPreparePsqlInputsFn
	bindPreparePsqlInputsFn = func(cli.PrepareOptions, string, string, prepareArgs, *refctx.Context, io.Reader) (prepareStageBinding, error) {
		return prepareStageBinding{
			PsqlArgs: []string{"-c", "select 1"},
			cleanup: func() error {
				return errors.New("cleanup failed")
			},
		}, nil
	}
	t.Cleanup(func() { bindPreparePsqlInputsFn = prevBind })

	prevRunPrepare := runPrepareFn
	runPrepareFn = func(context.Context, cli.PrepareOptions) (client.PrepareJobResult, error) {
		return client.PrepareJobResult{DSN: "dsn"}, nil
	}
	t.Cleanup(func() { runPrepareFn = prevRunPrepare })

	_, handled, err := prepareResultParsed(
		stdoutAndErr{stdout: &bytes.Buffer{}, stderr: io.Discard},
		cli.PrepareOptions{},
		config.LoadedConfig{},
		"",
		"",
		prepareArgs{Image: "image", PsqlArgs: []string{"-c", "select 1"}, Watch: true},
		nil,
	)
	if err == nil || !strings.Contains(err.Error(), "cleanup failed") {
		t.Fatalf("expected cleanup error, got %v", err)
	}
	if handled {
		t.Fatalf("expected handled=false on cleanup error")
	}
}

func TestPrepareResultParsedDisablesControlPromptForRef(t *testing.T) {
	prevBind := bindPreparePsqlInputsFn
	bindPreparePsqlInputsFn = func(cli.PrepareOptions, string, string, prepareArgs, *refctx.Context, io.Reader) (prepareStageBinding, error) {
		return prepareStageBinding{PsqlArgs: []string{"-c", "select 1"}}, nil
	}
	t.Cleanup(func() { bindPreparePsqlInputsFn = prevBind })

	prevRunPrepare := runPrepareFn
	runPrepareFn = func(_ context.Context, opts cli.PrepareOptions) (client.PrepareJobResult, error) {
		if !opts.DisableControlPrompt {
			t.Fatalf("expected DisableControlPrompt for ref-backed prepare")
		}
		return client.PrepareJobResult{DSN: "dsn"}, nil
	}
	t.Cleanup(func() { runPrepareFn = prevRunPrepare })

	_, handled, err := prepareResultParsed(
		stdoutAndErr{stdout: &bytes.Buffer{}, stderr: io.Discard},
		cli.PrepareOptions{},
		config.LoadedConfig{},
		"",
		"",
		prepareArgs{Image: "image", PsqlArgs: []string{"-c", "select 1"}, Watch: true},
		&refctx.Context{},
	)
	if err != nil {
		t.Fatalf("prepareResultParsed: %v", err)
	}
	if handled {
		t.Fatalf("expected handled=false")
	}
}

func TestRunPlanKindParsedWithPathModePropagatesCleanupError(t *testing.T) {
	prevBind := bindPreparePsqlInputsFn
	bindPreparePsqlInputsFn = func(cli.PrepareOptions, string, string, prepareArgs, *refctx.Context, io.Reader) (prepareStageBinding, error) {
		return prepareStageBinding{
			PsqlArgs: []string{"-c", "select 1"},
			cleanup: func() error {
				return errors.New("cleanup failed")
			},
		}, nil
	}
	t.Cleanup(func() { bindPreparePsqlInputsFn = prevBind })

	prevRunPlan := runPlanFn
	runPlanFn = func(context.Context, cli.PrepareOptions) (cli.PlanResult, error) {
		return cli.PlanResult{
			PrepareKind:           "psql",
			ImageID:               "image",
			PrepareArgsNormalized: "-c select 1",
			Tasks:                 []client.PlanTask{{TaskID: "plan", Type: "plan", PlannerKind: "psql"}},
		}, nil
	}
	t.Cleanup(func() { runPlanFn = prevRunPlan })

	err := runPlanKindParsedWithPathMode(
		&bytes.Buffer{},
		io.Discard,
		cli.PrepareOptions{},
		config.LoadedConfig{},
		"",
		"",
		prepareArgs{Image: "image", PsqlArgs: []string{"-c", "select 1"}},
		nil,
		"json",
		"psql",
		true,
	)
	if err == nil || !strings.Contains(err.Error(), "cleanup failed") {
		t.Fatalf("expected cleanup error, got %v", err)
	}
}

func countGitWorktrees(t *testing.T, repo string) int {
	t.Helper()

	out, err := exec.Command("git", "-C", repo, "worktree", "list", "--porcelain").CombinedOutput()
	if err != nil {
		t.Fatalf("git worktree list: %v\n%s", err, out)
	}

	count := 0
	for _, line := range strings.Split(string(out), "\n") {
		if strings.HasPrefix(line, "worktree ") {
			count++
		}
	}
	return count
}
