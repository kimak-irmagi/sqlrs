package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sqlrs/cli/internal/cli"
	"github.com/sqlrs/cli/internal/config"
	"github.com/sqlrs/cli/internal/paths"
	"github.com/sqlrs/cli/internal/pathutil"
)

func TestParsePrepareArgsImageFlag(t *testing.T) {
	opts, showHelp, err := parsePrepareArgs([]string{"--image", "img", "-c", "select 1"})
	if err != nil || showHelp {
		t.Fatalf("parsePrepareArgs: err=%v help=%v", err, showHelp)
	}
	if opts.Image != "img" {
		t.Fatalf("expected image, got %q", opts.Image)
	}
	if len(opts.PsqlArgs) != 2 || opts.PsqlArgs[0] != "-c" {
		t.Fatalf("unexpected psql args: %+v", opts.PsqlArgs)
	}
}

func TestParsePrepareArgsHelp(t *testing.T) {
	_, showHelp, err := parsePrepareArgs([]string{"--help"})
	if err != nil || !showHelp {
		t.Fatalf("expected help, err=%v help=%v", err, showHelp)
	}
	_, showHelp, err = parsePrepareArgs([]string{"-h"})
	if err != nil || !showHelp {
		t.Fatalf("expected help for -h, err=%v help=%v", err, showHelp)
	}
}

func TestParsePrepareArgsUnicodeDashHint(t *testing.T) {
	_, _, err := parsePrepareArgs([]string{"—image", "img"})
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 2 {
		t.Fatalf("expected ExitError code 2, got %v", err)
	}
	if !strings.Contains(exitErr.Error(), "Unicode dash") {
		t.Fatalf("expected unicode dash hint, got %v", exitErr)
	}
	if !strings.Contains(exitErr.Error(), "--image") {
		t.Fatalf("expected normalized suggestion, got %v", exitErr)
	}
}

func TestParsePrepareArgsImageEquals(t *testing.T) {
	opts, showHelp, err := parsePrepareArgs([]string{"--image=img", "-c", "select 1"})
	if err != nil || showHelp {
		t.Fatalf("parsePrepareArgs: err=%v help=%v", err, showHelp)
	}
	if opts.Image != "img" {
		t.Fatalf("expected image, got %q", opts.Image)
	}
}

func TestParsePrepareArgsRefOptions(t *testing.T) {
	opts, showHelp, err := parsePrepareArgs([]string{"--ref", "HEAD~1", "--ref-mode", "blob", "--", "-f", "init.sql"})
	if err != nil || showHelp {
		t.Fatalf("parsePrepareArgs: err=%v help=%v", err, showHelp)
	}
	if opts.Ref != "HEAD~1" {
		t.Fatalf("expected ref, got %q", opts.Ref)
	}
	if opts.RefMode != "blob" {
		t.Fatalf("expected ref mode blob, got %q", opts.RefMode)
	}
	if opts.RefKeepWorktree {
		t.Fatalf("expected ref keep worktree false")
	}
	if len(opts.PsqlArgs) != 2 || opts.PsqlArgs[0] != "-f" || opts.PsqlArgs[1] != "init.sql" {
		t.Fatalf("unexpected psql args: %+v", opts.PsqlArgs)
	}
}

func TestParsePrepareArgsRefKeepWorktreeDefaultsMode(t *testing.T) {
	opts, showHelp, err := parsePrepareArgs([]string{"--ref", "origin/main", "--ref-keep-worktree", "--", "-c", "select 1"})
	if err != nil || showHelp {
		t.Fatalf("parsePrepareArgs: err=%v help=%v", err, showHelp)
	}
	if opts.Ref != "origin/main" {
		t.Fatalf("expected ref, got %q", opts.Ref)
	}
	if opts.RefMode != "worktree" {
		t.Fatalf("expected default ref mode worktree, got %q", opts.RefMode)
	}
	if !opts.RefKeepWorktree {
		t.Fatalf("expected ref keep worktree true")
	}
}

func TestParsePrepareArgsMissingImageValue(t *testing.T) {
	_, _, err := parsePrepareArgs([]string{"--image"})
	if err == nil {
		t.Fatalf("expected error")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 2 {
		t.Fatalf("expected ExitError code 2, got %v", err)
	}
}

func TestParsePrepareArgsImageEqualsMissing(t *testing.T) {
	_, _, err := parsePrepareArgs([]string{"--image="})
	if err == nil {
		t.Fatalf("expected error")
	}
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != 2 {
		t.Fatalf("expected ExitError code 2, got %v", err)
	}
}

func TestParsePrepareArgsRejectsInvalidRefCombinations(t *testing.T) {
	cases := []struct {
		name string
		args []string
		want string
	}{
		{
			name: "missing ref value",
			args: []string{"--ref"},
			want: "Missing value for --ref",
		},
		{
			name: "empty ref value",
			args: []string{"--ref", "   "},
			want: "Missing value for --ref",
		},
		{
			name: "ref mode without ref",
			args: []string{"--ref-mode", "blob"},
			want: "--ref-mode requires --ref",
		},
		{
			name: "missing ref mode value",
			args: []string{"--ref", "HEAD", "--ref-mode"},
			want: "Missing value for --ref-mode",
		},
		{
			name: "bad ref mode",
			args: []string{"--ref", "HEAD", "--ref-mode", "bad"},
			want: "--ref-mode \"bad\" is not supported",
		},
		{
			name: "keep worktree without ref",
			args: []string{"--ref-keep-worktree"},
			want: "--ref-keep-worktree requires --ref",
		},
		{
			name: "keep worktree with blob",
			args: []string{"--ref", "HEAD", "--ref-mode", "blob", "--ref-keep-worktree"},
			want: "--ref-keep-worktree is only valid with --ref-mode worktree",
		},
		{
			name: "no-watch with ref",
			args: []string{"--ref", "HEAD", "--no-watch", "--", "-c", "select 1"},
			want: "--no-watch is not supported with --ref",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, _, err := parsePrepareArgs(tc.args)
			if err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("expected %q, got %v", tc.want, err)
			}
		})
	}
}

func TestResolvePrepareImageSources(t *testing.T) {
	temp := t.TempDir()
	cfg := config.LoadedConfig{
		Paths: paths.Dirs{ConfigDir: filepath.Join(temp, "config")},
	}

	id, source, err := resolvePrepareImage("img", cfg)
	if err != nil || id != "img" || source != "command line" {
		t.Fatalf("unexpected command line source: id=%q source=%q err=%v", id, source, err)
	}

	projectDir := filepath.Join(temp, "project", ".sqlrs")
	if err := os.MkdirAll(projectDir, 0o700); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	projectPath := filepath.Join(projectDir, "config.yaml")
	if err := os.WriteFile(projectPath, []byte("dbms:\n  image: workspace\n"), 0o600); err != nil {
		t.Fatalf("write project config: %v", err)
	}
	cfg.ProjectConfigPath = projectPath
	id, source, err = resolvePrepareImage("", cfg)
	if err != nil || id != "workspace" || source != "workspace config" {
		t.Fatalf("unexpected workspace source: id=%q source=%q err=%v", id, source, err)
	}

	if err := os.MkdirAll(cfg.Paths.ConfigDir, 0o700); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	globalPath := filepath.Join(cfg.Paths.ConfigDir, "config.yaml")
	if err := os.WriteFile(globalPath, []byte("dbms:\n  image: global\n"), 0o600); err != nil {
		t.Fatalf("write global config: %v", err)
	}
	cfg.ProjectConfigPath = ""
	id, source, err = resolvePrepareImage("", cfg)
	if err != nil || id != "global" || source != "global config" {
		t.Fatalf("unexpected global source: id=%q source=%q err=%v", id, source, err)
	}

	id, source, err = resolvePrepareImage("", config.LoadedConfig{Paths: paths.Dirs{ConfigDir: filepath.Join(temp, "missing")}})
	if err != nil || id != "" || source != "" {
		t.Fatalf("expected empty result, got id=%q source=%q err=%v", id, source, err)
	}
}

func TestFormatImageSourceEmpty(t *testing.T) {
	if formatImageSource("", "") != "" {
		t.Fatalf("expected empty output")
	}
	out := formatImageSource("img", "global config")
	if !strings.Contains(out, "dbms.image=img") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestNormalizePsqlArgs(t *testing.T) {
	cwd := t.TempDir()
	args := []string{"-f", "file.sql", "--file=other.sql", "-fmore.sql", "-c", "select 1"}
	normalized, stdin, err := normalizePsqlArgs(args, cwd, cwd, strings.NewReader("stdin"), nil)
	if err != nil {
		t.Fatalf("normalizePsqlArgs: %v", err)
	}
	if stdin != nil {
		t.Fatalf("expected no stdin")
	}
	if len(normalized) == 0 {
		t.Fatalf("expected normalized args")
	}

	normalized, stdin, err = normalizePsqlArgs([]string{"-f", "-"}, cwd, cwd, strings.NewReader("data"), nil)
	if err != nil {
		t.Fatalf("normalizePsqlArgs stdin: %v", err)
	}
	if stdin == nil || *stdin != "data" {
		t.Fatalf("expected stdin data, got %+v", stdin)
	}
	if len(normalized) != 2 || normalized[1] != "-" {
		t.Fatalf("unexpected normalized args: %+v", normalized)
	}
}

func TestNormalizePsqlArgsConvertsPaths(t *testing.T) {
	cwd := t.TempDir()
	path := filepath.Join(cwd, "query.sql")
	if err := os.WriteFile(path, []byte("select 1;"), 0o600); err != nil {
		t.Fatalf("write file: %v", err)
	}
	args := []string{"-f", "query.sql"}
	normalized, _, err := normalizePsqlArgs(args, cwd, cwd, strings.NewReader(""), func(value string) (string, error) {
		return "/mnt/converted" + filepath.ToSlash(value), nil
	})
	if err != nil {
		t.Fatalf("normalizePsqlArgs: %v", err)
	}
	if len(normalized) != 2 || !strings.HasPrefix(normalized[1], "/mnt/converted") {
		t.Fatalf("unexpected args: %+v", normalized)
	}
}

func TestNormalizePsqlArgsStdinReadError(t *testing.T) {
	_, _, err := normalizePsqlArgs([]string{"-f", "-"}, t.TempDir(), t.TempDir(), errorReader{}, nil)
	if err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected read error, got %v", err)
	}
}

func TestNormalizePsqlArgsMissingValue(t *testing.T) {
	_, _, err := normalizePsqlArgs([]string{"-f"}, t.TempDir(), t.TempDir(), strings.NewReader(""), nil)
	if err == nil {
		t.Fatalf("expected error")
	}
}

func TestNormalizeFilePath(t *testing.T) {
	cwd := t.TempDir()
	path, useStdin, err := normalizeFilePath("-", cwd, cwd, nil)
	if err != nil || !useStdin || path != "-" {
		t.Fatalf("expected stdin path, got %q useStdin=%v err=%v", path, useStdin, err)
	}

	rel, useStdin, err := normalizeFilePath("file.sql", cwd, cwd, nil)
	if err != nil || useStdin || !strings.HasPrefix(rel, cwd) {
		t.Fatalf("unexpected relative path: %q useStdin=%v err=%v", rel, useStdin, err)
	}

	abs := filepath.Join(cwd, "abs.sql")
	out, useStdin, err := normalizeFilePath(abs, cwd, cwd, nil)
	if err != nil || useStdin || out != abs {
		t.Fatalf("unexpected abs path: %q useStdin=%v err=%v", out, useStdin, err)
	}

	if _, _, err := normalizeFilePath(" ", cwd, cwd, nil); err == nil {
		t.Fatalf("expected empty path error")
	}
}

func TestNormalizeFilePathRejectsOutsideWorkspace(t *testing.T) {
	workspace := t.TempDir()
	outer := filepath.Dir(workspace)
	outside := filepath.Join(outer, "outside.sql")
	if _, _, err := normalizeFilePath(outside, workspace, workspace, nil); err == nil {
		t.Fatalf("expected workspace boundary error")
	}
}

func TestRebasePathToWorkspaceRootCoverage(t *testing.T) {
	root := t.TempDir()
	child := filepath.Join(root, "nested", "query.sql")
	outside := filepath.Join(filepath.Dir(root), "outside.sql")

	if got := rebasePathToWorkspaceRoot("", root); got != "" {
		t.Fatalf("expected empty path to remain empty, got %q", got)
	}
	if got := rebasePathToWorkspaceRoot(child, ""); got != child {
		t.Fatalf("expected empty root to keep path, got %q", got)
	}
	if got := rebasePathToWorkspaceRoot(root, root); !pathutil.SameLocalPath(got, root) {
		t.Fatalf("expected root path to stay rooted, got %q", got)
	}
	if got := rebasePathToWorkspaceRoot(child, root); !pathutil.SameLocalPath(got, child) {
		t.Fatalf("expected child path to stay within root, got %q", got)
	}
	if got := rebasePathToWorkspaceRoot(outside, root); !pathutil.SameLocalPath(got, outside) {
		t.Fatalf("expected outside path unchanged, got %q", got)
	}
}

func TestCanonicalizeBoundaryPathCoverage(t *testing.T) {
	if got := canonicalizeBoundaryPath(" "); got != "" {
		t.Fatalf("expected empty canonical path, got %q", got)
	}

	root := t.TempDir()
	want, err := filepath.EvalSymlinks(root)
	if err != nil {
		want = filepath.Clean(root)
	}
	if got := canonicalizeBoundaryPath(root); !pathutil.SameLocalPath(got, want) {
		t.Fatalf("canonicalizeBoundaryPath(existing) = %q, want %q", got, want)
	}

	missing := filepath.Join(root, "nested", "missing.sql")
	gotMissing := canonicalizeBoundaryPath(missing)
	probe := filepath.Clean(gotMissing)
	for {
		if _, err := os.Stat(probe); err == nil {
			break
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			t.Fatalf("expected existing ancestor for %q", gotMissing)
		}
		probe = parent
	}
	wantRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		wantRoot = filepath.Clean(root)
	}
	gotRoot, err := filepath.EvalSymlinks(probe)
	if err != nil {
		gotRoot = filepath.Clean(probe)
	}
	if !pathutil.SameLocalPath(gotRoot, wantRoot) {
		t.Fatalf("canonicalizeBoundaryPath(missing) root = %q, want %q", gotRoot, wantRoot)
	}
	rel, err := filepath.Rel(probe, gotMissing)
	if err != nil {
		t.Fatalf("filepath.Rel(%q, %q): %v", probe, gotMissing, err)
	}
	if rel != filepath.Join("nested", "missing.sql") {
		t.Fatalf("canonicalizeBoundaryPath(missing) relative path = %q, want %q", rel, filepath.Join("nested", "missing.sql"))
	}
}

func TestBuildPathConverterNoWSLDistro(t *testing.T) {
	if conv := buildPathConverter(cli.PrepareOptions{}); conv != nil {
		t.Fatalf("expected nil converter without WSL distro")
	}
}

func TestDeriveLiquibaseWorkDirFromArgsCoverage(t *testing.T) {
	fallback := filepath.Join("fallback", "dir")
	if got := deriveLiquibaseWorkDirFromArgs([]string{"update"}, fallback); got != fallback {
		t.Fatalf("expected fallback without search path, got %q", got)
	}
	if got := deriveLiquibaseWorkDirFromArgs([]string{"update", "--searchPath"}, fallback); got != fallback {
		t.Fatalf("expected fallback for missing searchPath value, got %q", got)
	}
	if got := deriveLiquibaseWorkDirFromArgs([]string{"update", "--search-path", "dir1,dir2"}, fallback); got != filepath.Clean("dir1") {
		t.Fatalf("expected dir1 workdir, got %q", got)
	}
	if got := deriveLiquibaseWorkDirFromArgs([]string{"update", "--searchPath=dir2,dir3"}, fallback); got != filepath.Clean("dir2") {
		t.Fatalf("expected dir2 workdir, got %q", got)
	}
}

func TestFirstLiquibaseSearchPathDirCoverage(t *testing.T) {
	if got := firstLiquibaseSearchPathDir(" "); got != "" {
		t.Fatalf("expected empty dir for blank searchPath, got %q", got)
	}
	if got := firstLiquibaseSearchPathDir(" , dir/sub "); got != filepath.Clean("dir/sub") {
		t.Fatalf("expected first local dir, got %q", got)
	}
}

type errorReader struct{}

func (errorReader) Read([]byte) (int, error) {
	return 0, errors.New("boom")
}
