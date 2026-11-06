package app

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/sqlrs/cli/internal/cli"
	"github.com/sqlrs/cli/internal/config"
	"github.com/sqlrs/cli/internal/paths"
)

func TestParsePrepareArgsSeparator(t *testing.T) {
	opts, showHelp, err := parsePrepareArgs([]string{"--image", "img", "--", "-c", "select 1"})
	if err != nil || showHelp {
		t.Fatalf("parsePrepareArgs: err=%v help=%v", err, showHelp)
	}
	if opts.Image != "img" {
		t.Fatalf("expected image, got %q", opts.Image)
	}
	if len(opts.PsqlArgs) != 2 || opts.PsqlArgs[0] != "-c" {
		t.Fatalf("unexpected args: %+v", opts.PsqlArgs)
	}
}

func TestRunPrepareLiquibaseHelpHandled(t *testing.T) {
	if err := runPrepareLiquibase(os.Stdout, os.Stderr, cli.PrepareOptions{}, config.LoadedConfig{}, "", t.TempDir(), []string{"--help"}); err != nil {
		t.Fatalf("expected help to be handled, got %v", err)
	}
}

func TestPrepareResultLiquibaseErrors(t *testing.T) {
	cfg := config.LoadedConfig{Paths: paths.Dirs{ConfigDir: t.TempDir()}}
	_, _, err := prepareResultLiquibase(stdoutAndErr{stdout: os.Stdout, stderr: os.Stderr}, cli.PrepareOptions{}, cfg, "", t.TempDir(), []string{})
	if err == nil || !strings.Contains(err.Error(), "liquibase command") {
		t.Fatalf("expected liquibase command error, got %v", err)
	}

	_, _, err = prepareResultLiquibase(stdoutAndErr{stdout: os.Stdout, stderr: os.Stderr}, cli.PrepareOptions{}, cfg, "", t.TempDir(), []string{"update"})
	if err == nil || !strings.Contains(err.Error(), "Missing base image id") {
		t.Fatalf("expected missing image error, got %v", err)
	}

	_, _, err = prepareResultLiquibase(stdoutAndErr{stdout: os.Stdout, stderr: os.Stderr}, cli.PrepareOptions{Mode: "remote"}, cfg, "", t.TempDir(), []string{"--image", "img", "--", "update"})
	if err == nil {
		t.Fatalf("expected RunPrepare error")
	}
}

func TestNormalizeLiquibaseArgsVariants(t *testing.T) {
	root := t.TempDir()
	cwd := root
	_, err := normalizeLiquibaseArgs([]string{"--changelog-file"}, root, cwd, nil)
	if err == nil {
		t.Fatalf("expected missing value error")
	}

	args, err := normalizeLiquibaseArgs([]string{"--search-path", "dir1,dir2"}, root, cwd, nil)
	if err != nil {
		t.Fatalf("normalizeLiquibaseArgs: %v", err)
	}
	if got := strings.Join(args, " "); !strings.Contains(got, "--searchPath") {
		t.Fatalf("expected searchPath rewrite, got %q", got)
	}

	_, err = normalizeLiquibaseArgs([]string{"--defaults-file="}, root, cwd, nil)
	if err == nil {
		t.Fatalf("expected missing defaults-file value error")
	}
}

func TestNormalizeWorkDirCoverage(t *testing.T) {
	if got, err := normalizeWorkDir("", nil); err != nil || got != "" {
		t.Fatalf("expected empty workdir, got %q err=%v", got, err)
	}
	workDir := "/root"
	if runtime.GOOS == "windows" {
		workDir = "C:\\root"
	}
	_, err := normalizeWorkDir(workDir, func(string) (string, error) {
		return "", errors.New("boom")
	})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected conversion error, got %v", err)
	}
}

func TestRewriteLiquibasePathArg(t *testing.T) {
	root := t.TempDir()
	cwd := root
	if _, err := rewriteLiquibasePathArg("--searchPath", "", root, cwd, nil); err == nil {
		t.Fatalf("expected empty searchPath error")
	}
}

func TestRelativizeLiquibaseArgsCoverage(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "project")
	if err := os.MkdirAll(cwd, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	abs := filepath.Join(root, "change.xml")
	args := []string{"--changelog-file", abs, "--defaults-file=" + abs}
	out := relativizeLiquibaseArgs(args, root, cwd)
	if out[1] != abs {
		t.Fatalf("expected changelog path to remain absolute when outside cwd, got %q", out[1])
	}
}

func TestToRelativeIfWithin(t *testing.T) {
	root := t.TempDir()
	if got := toRelativeIfWithin(root, ""); got != "" {
		t.Fatalf("expected empty, got %q", got)
	}
	if got := toRelativeIfWithin(root, "relative.sql"); got != "relative.sql" {
		t.Fatalf("expected relative unchanged, got %q", got)
	}
	abs := filepath.Join(root, "file.sql")
	if got := toRelativeIfWithin(root, abs); got == abs {
		t.Fatalf("expected relative path, got %q", got)
	}
	outer := filepath.Join(filepath.Dir(root), "outer.sql")
	if got := toRelativeIfWithin(root, outer); got != outer {
		t.Fatalf("expected outside to remain absolute, got %q", got)
	}
}

func TestResolvePrepareImageGlobalError(t *testing.T) {
	root := t.TempDir()
	cfg := config.LoadedConfig{Paths: paths.Dirs{ConfigDir: root}}
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, "config.yaml"), []byte("dbms: ["), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	if _, _, err := resolvePrepareImage("", cfg); err == nil {
		t.Fatalf("expected error from bad global config")
	}
}

func TestPrepareResultLiquibaseHelp(t *testing.T) {
	handledResult, handled, err := prepareResultLiquibase(stdoutAndErr{stdout: os.Stdout, stderr: os.Stderr}, cli.PrepareOptions{}, config.LoadedConfig{}, "", t.TempDir(), []string{"--help"})
	if err != nil || !handled || handledResult.DSN != "" {
		t.Fatalf("expected help handled, got result=%+v handled=%v err=%v", handledResult, handled, err)
	}
}

func TestPrepareResultLiquibaseWorkDirConversion(t *testing.T) {
	root := t.TempDir()
	cfg := config.LoadedConfig{Paths: paths.Dirs{ConfigDir: root}}
	runOpts := cli.PrepareOptions{Mode: "remote"}
	_, _, err := prepareResultLiquibase(stdoutAndErr{stdout: os.Stdout, stderr: os.Stderr}, runOpts, cfg, root, root, []string{"--image", "img", "--", "update"})
	if err == nil {
		t.Fatalf("expected RunPrepare error for missing endpoint")
	}
}

func TestNormalizeLiquibaseArgsMissingSearchPathValue(t *testing.T) {
	root := t.TempDir()
	_, err := normalizeLiquibaseArgs([]string{"--searchPath="}, root, root, nil)
	if err == nil {
		t.Fatalf("expected missing searchPath error")
	}
}

func TestNormalizeLiquibaseArgsMissingFlagValue(t *testing.T) {
	root := t.TempDir()
	_, err := normalizeLiquibaseArgs([]string{"--defaults-file"}, root, root, nil)
	if err == nil {
		t.Fatalf("expected missing defaults-file value")
	}
}

func TestNormalizeLiquibaseArgsWithConverterError(t *testing.T) {
	root := t.TempDir()
	_, err := normalizeLiquibaseArgs([]string{"--changelog-file", "file.xml"}, root, root, func(string) (string, error) {
		return "", errors.New("boom")
	})
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected converter error, got %v", err)
	}
}

func TestNormalizeWorkDirConversion(t *testing.T) {
	workDir := "/root"
	if runtime.GOOS == "windows" {
		workDir = "C:\\root"
	}
	want := "/mnt/root"
	if runtime.GOOS == "windows" {
		want = "/mnt/c/root"
	}
	out, err := normalizeWorkDir(workDir, func(string) (string, error) {
		return want, nil
	})
	if err != nil || out != want {
		t.Fatalf("unexpected conversion result: %q err=%v", out, err)
	}
}

func TestNormalizeLiquibaseArgsSearchPathEmptyItem(t *testing.T) {
	root := t.TempDir()
	_, err := normalizeLiquibaseArgs([]string{"--searchPath", "dir1, "}, root, root, nil)
	if err == nil {
		t.Fatalf("expected searchPath empty item error")
	}
}

func TestPrepareResultUsesImageSourceVerbose(t *testing.T) {
	root := t.TempDir()
	cfgPath := filepath.Join(root, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("dbms:\n  image: pg\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg := config.LoadedConfig{Paths: paths.Dirs{ConfigDir: root}}
	runOpts := cli.PrepareOptions{Mode: "remote", Verbose: true}
	_, _, err := prepareResult(stdoutAndErr{stdout: os.Stdout, stderr: os.Stderr}, runOpts, cfg, root, root, []string{"--", "-c", "select 1"})
	if err == nil {
		t.Fatalf("expected RunPrepare error for missing endpoint")
	}
}

func TestPrepareResultLiquibaseRejectsRemovedRunnerSettings(t *testing.T) {
	root := t.TempDir()
	var requests atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	t.Cleanup(server.Close)
	cfgPath := filepath.Join(root, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("liquibase:\n  exec: lb.cmd\n  execMode: windows-bat\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg := config.LoadedConfig{Paths: paths.Dirs{ConfigDir: root}}
	runOpts := cli.PrepareOptions{Mode: "remote", Endpoint: server.URL}
	_, _, err := prepareResultLiquibase(stdoutAndErr{stdout: os.Stdout, stderr: os.Stderr}, runOpts, cfg, root, root, []string{"--image", "img", "--", "update"})
	if requests.Load() != 0 {
		t.Fatalf("removed runner setting caused %d remote requests", requests.Load())
	}
	if err == nil || !strings.Contains(err.Error(), "liquibase.exec") {
		t.Fatalf("expected removed runner setting error, got %v", err)
	}
}

func TestNormalizeLiquibaseArgsSearchPathAlias(t *testing.T) {
	root := t.TempDir()
	args, err := normalizeLiquibaseArgs([]string{"--search-path", "dir1"}, root, root, nil)
	if err != nil {
		t.Fatalf("normalizeLiquibaseArgs: %v", err)
	}
	if args[0] != "--searchPath" {
		t.Fatalf("expected searchPath alias rewrite, got %+v", args)
	}
}

func TestNormalizeLiquibaseArgsDefaultsFileFlagValue(t *testing.T) {
	root := t.TempDir()
	args, err := normalizeLiquibaseArgs([]string{"--defaults-file", "file.properties"}, root, root, nil)
	if err != nil {
		t.Fatalf("normalizeLiquibaseArgs: %v", err)
	}
	if len(args) != 2 || args[0] != "--defaults-file" {
		t.Fatalf("unexpected args: %+v", args)
	}
}

func TestNormalizeLiquibaseArgsChangelogFlagValue(t *testing.T) {
	root := t.TempDir()
	args, err := normalizeLiquibaseArgs([]string{"--changelog-file", "change.xml"}, root, root, nil)
	if err != nil {
		t.Fatalf("normalizeLiquibaseArgs: %v", err)
	}
	if len(args) != 2 || args[0] != "--changelog-file" {
		t.Fatalf("unexpected args: %+v", args)
	}
}

func TestPrepareResultLiquibaseMissingCommand(t *testing.T) {
	cfg := config.LoadedConfig{Paths: paths.Dirs{ConfigDir: t.TempDir()}}
	_, _, err := prepareResultLiquibase(stdoutAndErr{stdout: os.Stdout, stderr: os.Stderr}, cli.PrepareOptions{}, cfg, "", t.TempDir(), []string{"--image", "img"})
	if err == nil {
		t.Fatalf("expected missing liquibase command error")
	}
}

func TestNormalizeLiquibaseArgsSearchPathEmpty(t *testing.T) {
	root := t.TempDir()
	_, err := normalizeLiquibaseArgs([]string{"--searchPath", ""}, root, root, nil)
	if err == nil {
		t.Fatalf("expected empty searchPath error")
	}
}

func TestNormalizeLiquibaseArgsSearchPathMultiple(t *testing.T) {
	root := t.TempDir()
	args, err := normalizeLiquibaseArgs([]string{"--searchPath", "dir1,dir2"}, root, root, nil)
	if err != nil {
		t.Fatalf("normalizeLiquibaseArgs: %v", err)
	}
	if len(args) != 2 || !strings.Contains(args[1], ",") {
		t.Fatalf("unexpected searchPath args: %+v", args)
	}
}

func TestPrepareResultUsesImageSourceVerboseLB(t *testing.T) {
	root := t.TempDir()
	cfgPath := filepath.Join(root, "config.yaml")
	if err := os.WriteFile(cfgPath, []byte("dbms:\n  image: pg\n"), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	cfg := config.LoadedConfig{Paths: paths.Dirs{ConfigDir: root}}
	runOpts := cli.PrepareOptions{Mode: "remote", Verbose: true}
	_, _, err := prepareResultLiquibase(stdoutAndErr{stdout: os.Stdout, stderr: os.Stderr}, runOpts, cfg, root, root, []string{"--", "update"})
	if err == nil {
		t.Fatalf("expected RunPrepare error for missing endpoint")
	}
}

func TestNormalizeLiquibaseArgsDefaultsFileAlias(t *testing.T) {
	root := t.TempDir()
	args, err := normalizeLiquibaseArgs([]string{"--defaults-file=file.properties"}, root, root, nil)
	if err != nil {
		t.Fatalf("normalizeLiquibaseArgs: %v", err)
	}
	if len(args) != 1 || !strings.Contains(args[0], "--defaults-file=") {
		t.Fatalf("unexpected args: %+v", args)
	}
}

func TestPrepareResultLiquibaseRunPrepareError(t *testing.T) {
	root := t.TempDir()
	cfg := config.LoadedConfig{Paths: paths.Dirs{ConfigDir: root}}
	runOpts := cli.PrepareOptions{Mode: "remote"}
	_, _, err := prepareResultLiquibase(stdoutAndErr{stdout: os.Stdout, stderr: os.Stderr}, runOpts, cfg, root, root, []string{"--image", "img", "--", "update"})
	if err == nil {
		t.Fatalf("expected RunPrepare error")
	}
}

func TestNormalizeLiquibaseArgsChangelogFileEquals(t *testing.T) {
	root := t.TempDir()
	args, err := normalizeLiquibaseArgs([]string{"--changelog-file=change.xml"}, root, root, nil)
	if err != nil {
		t.Fatalf("normalizeLiquibaseArgs: %v", err)
	}
	if len(args) != 1 || !strings.Contains(args[0], "--changelog-file=") {
		t.Fatalf("unexpected args: %+v", args)
	}
}

func TestNormalizeLiquibaseArgsSearchPathBadItem(t *testing.T) {
	root := t.TempDir()
	_, err := normalizeLiquibaseArgs([]string{"--searchPath", "dir1,,dir2"}, root, root, nil)
	if err == nil {
		t.Fatalf("expected searchPath empty item error")
	}
}

func TestNormalizeWorkDirWithConverterError(t *testing.T) {
	workDir := "/root"
	if runtime.GOOS == "windows" {
		workDir = "C:\\root"
	}
	_, err := normalizeWorkDir(workDir, func(string) (string, error) {
		return "", context.DeadlineExceeded
	})
	if err == nil {
		t.Fatalf("expected conversion error")
	}
}
