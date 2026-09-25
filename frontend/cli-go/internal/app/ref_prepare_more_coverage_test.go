package app

import (
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/sqlrs/cli/internal/cli"
	"github.com/sqlrs/cli/internal/inputset"
	"github.com/sqlrs/cli/internal/pathutil"
	"github.com/sqlrs/cli/internal/refctx"
)

func TestEffectiveRefBindBaseDir(t *testing.T) {
	cwd := filepath.Join("a", "b", "..", "c")
	ctx := &refctx.Context{BaseDir: filepath.Join("x", "y")}

	if got := effectiveRefBindBaseDir(cwd, ctx, &refctx.Context{}); !pathutil.SameLocalPath(got, filepath.Clean(cwd)) {
		t.Fatalf("effectiveRefBindBaseDir(existing) = %q, want %q", got, filepath.Clean(cwd))
	}
	if got := effectiveRefBindBaseDir(cwd, ctx, nil); got != ctx.BaseDir {
		t.Fatalf("effectiveRefBindBaseDir(ctx) = %q, want %q", got, ctx.BaseDir)
	}
	if got := effectiveRefBindBaseDir(cwd, nil, nil); got != cwd {
		t.Fatalf("effectiveRefBindBaseDir(cwd) = %q, want %q", got, cwd)
	}
}

func TestCleanupAndPsqlHelperBranches(t *testing.T) {
	if err := joinCleanup(nil, func() error { return nil })(); err != nil {
		t.Fatalf("joinCleanup success: %v", err)
	}

	err := joinCleanup(
		func() error { return errors.New("first") },
		nil,
		func() error { return errors.New("second") },
	)()
	var exitErr *ExitError
	if !errors.As(err, &exitErr) || !strings.Contains(err.Error(), "first; second") {
		t.Fatalf("joinCleanup error = %v", err)
	}

	runErr := errors.New("run failed")
	cleanupErr := errors.New("cleanup failed")
	if got := combineBindingCleanupError(runErr, nil); !errors.Is(got, runErr) {
		t.Fatalf("combineBindingCleanupError(run,nil) = %v", got)
	}
	if got := combineBindingCleanupError(nil, cleanupErr); !errors.Is(got, cleanupErr) {
		t.Fatalf("combineBindingCleanupError(nil,cleanup) = %v", got)
	}
	if got := combineBindingCleanupError(runErr, cleanupErr); got == nil || !strings.Contains(got.Error(), "cleanup: cleanup failed") {
		t.Fatalf("combineBindingCleanupError(run,cleanup) = %v", got)
	}

	cases := []struct {
		args []string
		want bool
	}{
		{args: []string{"-f", "query.sql"}, want: true},
		{args: []string{"--file=query.sql"}, want: true},
		{args: []string{"-fquery.sql"}, want: true},
		{args: []string{"-f", "-"}, want: false},
		{args: []string{"--file="}, want: false},
		{args: []string{"-f"}, want: false},
		{args: []string{"-c", "select 1"}, want: false},
	}
	for _, tc := range cases {
		if got := psqlHasFileArgs(tc.args); got != tc.want {
			t.Fatalf("psqlHasFileArgs(%v) = %v, want %v", tc.args, got, tc.want)
		}
	}
}

func TestMaterializeRefHelpers(t *testing.T) {
	root := t.TempDir()
	query := writeTestFile(t, root, filepath.Join("nested", "query.sql"), "select 1;\n")
	writeTestFile(t, root, filepath.Join("nested", "other.sql"), "select 2;\n")
	emptyDir := filepath.Join(root, "empty")
	if err := os.MkdirAll(emptyDir, 0o700); err != nil {
		t.Fatalf("mkdir empty dir: %v", err)
	}

	stageFiles, err := materializeRefFiles(root, []string{query, query}, []string{emptyDir}, inputset.OSFileSystem{})
	if err != nil {
		t.Fatalf("materializeRefFiles: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(stageFiles) })

	data, err := os.ReadFile(filepath.Join(stageFiles, "nested", "query.sql"))
	if err != nil {
		t.Fatalf("read staged query: %v", err)
	}
	if string(data) != "select 1;\n" {
		t.Fatalf("staged query = %q", string(data))
	}
	if stat, err := os.Stat(filepath.Join(stageFiles, "empty")); err != nil || !stat.IsDir() {
		t.Fatalf("staged dir stat = %v, %v", stat, err)
	}

	stageTree, err := materializeRefTree(root, inputset.OSFileSystem{})
	if err != nil {
		t.Fatalf("materializeRefTree: %v", err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(stageTree) })

	otherData, err := os.ReadFile(filepath.Join(stageTree, "nested", "other.sql"))
	if err != nil {
		t.Fatalf("read staged tree file: %v", err)
	}
	if string(otherData) != "select 2;\n" {
		t.Fatalf("staged tree file = %q", string(otherData))
	}
	if err := materializeRefTreeFromPath(root, filepath.Join(root, "missing.sql"), stageTree, inputset.OSFileSystem{}); err == nil {
		t.Fatalf("expected missing file error from materializeRefTreeFromPath")
	}
}

func TestPsqlPathRewriteHelpers(t *testing.T) {
	root := t.TempDir()
	stageRoot := t.TempDir()
	query := writeTestFile(t, root, filepath.Join("sql", "query.sql"), "select 1;\n")
	outside := filepath.Join(t.TempDir(), "outside.sql")

	if got := mapPathToStageRoot(root, stageRoot, root); !pathutil.SameLocalPath(got, stageRoot) {
		t.Fatalf("mapPathToStageRoot(root) = %q, want %q", got, stageRoot)
	}
	if got := mapPathToStageRoot(root, stageRoot, query); !pathutil.SameLocalPath(got, filepath.Join(stageRoot, "sql", "query.sql")) {
		t.Fatalf("mapPathToStageRoot(query) = %q", got)
	}
	if got := mapPathToStageRoot(root, stageRoot, outside); !pathutil.SameLocalPath(got, outside) {
		t.Fatalf("mapPathToStageRoot(outside) = %q, want %q", got, outside)
	}
	if got := mapPathToStageRoot(root, stageRoot, "query.sql"); got != "query.sql" {
		t.Fatalf("mapPathToStageRoot(relative) = %q", got)
	}

	args := []string{"-f", query, "--file=" + query, "-f" + query, "-f", "-", "-c", "select 1"}
	rewritten := rewritePsqlFileArgsToRoot(args, root, stageRoot)
	wantMapped := filepath.Join(stageRoot, "sql", "query.sql")
	if rewritten[1] != wantMapped || rewritten[2] != "--file="+wantMapped || rewritten[3] != "-f"+wantMapped || rewritten[5] != "-" {
		t.Fatalf("rewritePsqlFileArgsToRoot(%v) = %v", args, rewritten)
	}

	converted, err := convertPsqlFileArgs(args, func(value string) (string, error) {
		return "conv:" + filepath.Base(value), nil
	})
	if err != nil {
		t.Fatalf("convertPsqlFileArgs: %v", err)
	}
	if converted[1] != "conv:query.sql" || converted[2] != "--file=conv:query.sql" || converted[3] != "-fconv:query.sql" || converted[5] != "-" {
		t.Fatalf("convertPsqlFileArgs(%v) = %v", args, converted)
	}
	if _, err := convertPsqlFileArgs([]string{"--file=" + query}, func(string) (string, error) {
		return "", errors.New("boom")
	}); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected convertPsqlFileArgs error, got %v", err)
	}

	if got, err := convertDirectFileArg("-", func(string) (string, error) { return "bad", nil }); err != nil || got != "-" {
		t.Fatalf("convertDirectFileArg(stdin) = %q, %v", got, err)
	}
	if got, err := convertDirectFileArg("relative.sql", func(string) (string, error) { return "bad", nil }); err != nil || got != "relative.sql" {
		t.Fatalf("convertDirectFileArg(relative) = %q, %v", got, err)
	}
	if got, err := convertDirectFileArg(query, func(string) (string, error) { return "converted", nil }); err != nil || got != "converted" {
		t.Fatalf("convertDirectFileArg(abs) = %q, %v", got, err)
	}
}

func TestLiquibasePathHelpers(t *testing.T) {
	root := t.TempDir()
	stageRoot := t.TempDir()
	changelog := writeTestFile(
		t,
		root,
		filepath.Join("config", "liquibase", "master.xml"),
		`<databaseChangeLog xmlns="http://www.liquibase.org/xml/ns/dbchangelog"><include file="nested/child.xml" relativeToChangelogFile="true"/></databaseChangeLog>`+"\n",
	)
	child := writeTestFile(
		t,
		root,
		filepath.Join("config", "liquibase", "nested", "child.xml"),
		`<databaseChangeLog xmlns="http://www.liquibase.org/xml/ns/dbchangelog"></databaseChangeLog>`+"\n",
	)
	defaults := writeTestFile(t, root, filepath.Join("config", "liquibase.properties"), "url=jdbc:postgresql://localhost/db\n")
	searchDir := filepath.Join(root, "search")
	if err := os.MkdirAll(searchDir, 0o700); err != nil {
		t.Fatalf("mkdir search dir: %v", err)
	}

	args := []string{
		"update",
		"--changelog-file", changelog,
		"--defaults-file=" + defaults,
		"--searchPath", searchDir + ",classpath:db,https://example.com/db",
		"--search-path=" + searchDir,
	}

	files, dirs, hasLocal, err := liquibaseLocalArtifacts(args, inputset.NewWorkspaceResolver(root, root, nil), inputset.OSFileSystem{})
	if err != nil {
		t.Fatalf("liquibaseLocalArtifacts: %v", err)
	}
	if !hasLocal {
		t.Fatalf("expected local Liquibase artifacts")
	}
	if !containsPath(files, changelog) || !containsPath(files, defaults) || !containsPath(files, child) {
		t.Fatalf("unexpected files: %v", files)
	}
	if !containsPath(dirs, searchDir) {
		t.Fatalf("unexpected dirs: %v", dirs)
	}

	rewritten := rewriteLiquibaseArgsToRoot(args, root, stageRoot)
	wantMapped := filepath.Join(stageRoot, "config", "liquibase", "master.xml")
	if rewritten[2] != wantMapped {
		t.Fatalf("rewritten changelog = %q, want %q", rewritten[2], wantMapped)
	}
	if !strings.Contains(strings.Join(rewritten, "|"), "classpath:db") || !strings.Contains(strings.Join(rewritten, "|"), "https://example.com/db") {
		t.Fatalf("expected remote refs preserved in %v", rewritten)
	}

	converted, err := convertLiquibaseHostPaths(rewritten, func(value string) (string, error) {
		return "conv:" + filepath.ToSlash(value), nil
	})
	if err != nil {
		t.Fatalf("convertLiquibaseHostPaths: %v", err)
	}
	if converted[2] != "conv:"+filepath.ToSlash(wantMapped) {
		t.Fatalf("converted changelog = %q", converted[2])
	}
	if !strings.Contains(strings.Join(converted, "|"), "classpath:db") || !strings.Contains(strings.Join(converted, "|"), "https://example.com/db") {
		t.Fatalf("expected remote refs preserved after conversion: %v", converted)
	}
	if _, err := convertLiquibaseHostPaths([]string{"--searchPath", searchDir}, func(string) (string, error) {
		return "", errors.New("boom")
	}); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected convertLiquibaseHostPaths error, got %v", err)
	}

	if got := rewriteLiquibaseSearchPathToRoot(" , classpath:db , "+searchDir, root, stageRoot); !strings.Contains(got, "classpath:db") || !strings.Contains(got, filepath.Join(stageRoot, "search")) {
		t.Fatalf("rewriteLiquibaseSearchPathToRoot = %q", got)
	}
	if got, err := convertLiquibaseValue("classpath:db", func(string) (string, error) { return "bad", nil }); err != nil || got != "classpath:db" {
		t.Fatalf("convertLiquibaseValue(remote) = %q, %v", got, err)
	}
	if got, err := convertLiquibaseValue(changelog, func(string) (string, error) { return "converted", nil }); err != nil || got != "converted" {
		t.Fatalf("convertLiquibaseValue(local) = %q, %v", got, err)
	}
	searchPath, err := convertLiquibaseSearchPath(" , "+searchDir+",classpath:db,"+defaults, func(value string) (string, error) {
		return "conv:" + filepath.Base(value), nil
	})
	if err != nil {
		t.Fatalf("convertLiquibaseSearchPath: %v", err)
	}
	if !strings.Contains(searchPath, "conv:search") || !strings.Contains(searchPath, "classpath:db") || !strings.Contains(searchPath, "conv:liquibase.properties") {
		t.Fatalf("convertLiquibaseSearchPath = %q", searchPath)
	}
	if _, err := convertLiquibaseSearchPath(searchDir, func(string) (string, error) {
		return "", errors.New("boom")
	}); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected convertLiquibaseSearchPath error, got %v", err)
	}
}

func TestResolvePrepareBindingContext(t *testing.T) {
	existing := &refctx.Context{BaseDir: "existing"}
	ctx, cleanup, err := resolvePrepareBindingContext("", "", prepareArgs{}, existing)
	if err != nil {
		t.Fatalf("resolvePrepareBindingContext(existing): %v", err)
	}
	if ctx != existing {
		t.Fatalf("expected existing context, got %+v", ctx)
	}
	if cleanup == nil {
		t.Fatalf("expected cleanup for existing context")
	}
	if err := cleanup(); err != nil {
		t.Fatalf("existing cleanup: %v", err)
	}

	ctx, cleanup, err = resolvePrepareBindingContext("", "", prepareArgs{}, nil)
	if err != nil || ctx != nil || cleanup != nil {
		t.Fatalf("resolvePrepareBindingContext(no ref) = ctx=%+v cleanup_nil=%v err=%v", ctx, cleanup == nil, err)
	}

	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not in PATH")
	}

	repo, parentRef := initPrepareRefTestRepo(t)
	cwd := filepath.Join(repo, "examples")
	ctx, cleanup, err = resolvePrepareBindingContext(repo, cwd, prepareArgs{Ref: parentRef, RefMode: "blob"}, nil)
	if err != nil {
		t.Fatalf("resolvePrepareBindingContext(blob): %v", err)
	}
	if ctx == nil || ctx.RefMode != "blob" {
		t.Fatalf("expected blob ref context, got %+v", ctx)
	}
	if cleanup == nil {
		t.Fatalf("expected cleanup function")
	}
	if err := cleanup(); err != nil {
		t.Fatalf("blob cleanup: %v", err)
	}

	if _, _, err := resolvePrepareBindingContext(repo, cwd, prepareArgs{Ref: "missing-ref", RefMode: "blob"}, nil); err == nil {
		t.Fatalf("expected resolvePrepareBindingContext error for missing ref")
	}
}

func TestDedupePaths(t *testing.T) {
	root := t.TempDir()
	paths := []string{"", " ", filepath.Join(root, "a"), filepath.Join(root, ".", "a"), filepath.Join(root, "b")}
	want := []string{".", filepath.Join(root, "a"), filepath.Join(root, "b")}
	if got := dedupePaths(paths); !reflect.DeepEqual(got, want) {
		t.Fatalf("dedupePaths(%v) = %v, want %v", paths, got, want)
	}
}

func TestBindPreparePsqlInputsAdditionalBranches(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not in PATH")
	}

	repo, parentRef := initPrepareRefTestRepo(t)
	cwd := filepath.Join(repo, "examples")
	root := t.TempDir()
	writeTestFile(t, root, "query.sql", "select 1;\n")

	t.Run("resolve error", func(t *testing.T) {
		_, err := bindPreparePsqlInputs(cli.PrepareOptions{}, repo, cwd, prepareArgs{Ref: "missing-ref", RefMode: "blob"}, nil, strings.NewReader(""))
		if err == nil {
			t.Fatalf("expected resolve error")
		}
	})

	t.Run("normalize error", func(t *testing.T) {
		_, err := bindPreparePsqlInputs(cli.PrepareOptions{}, root, root, prepareArgs{PsqlArgs: []string{"-f"}}, &refctx.Context{
			WorkspaceRoot: root,
			BaseDir:       root,
			RefMode:       "blob",
			FileSystem:    inputset.OSFileSystem{},
		}, strings.NewReader(""))
		if err == nil {
			t.Fatalf("expected normalize error")
		}
	})

	t.Run("blob no file args", func(t *testing.T) {
		binding, err := bindPreparePsqlInputs(cli.PrepareOptions{}, root, root, prepareArgs{PsqlArgs: []string{"-c", "select 1"}}, &refctx.Context{
			WorkspaceRoot: root,
			BaseDir:       root,
			RefMode:       "blob",
			FileSystem:    inputset.OSFileSystem{},
		}, strings.NewReader(""))
		if err != nil {
			t.Fatalf("bindPreparePsqlInputs(no file args): %v", err)
		}
		if !reflect.DeepEqual(binding.PsqlArgs, []string{"-c", "select 1"}) {
			t.Fatalf("PsqlArgs = %v", binding.PsqlArgs)
		}
	})

	t.Run("collect error", func(t *testing.T) {
		_, err := bindPreparePsqlInputs(cli.PrepareOptions{}, root, root, prepareArgs{PsqlArgs: []string{"-f", "missing.sql"}}, &refctx.Context{
			WorkspaceRoot: root,
			BaseDir:       root,
			RefMode:       "blob",
			FileSystem:    inputset.OSFileSystem{},
		}, strings.NewReader(""))
		if err == nil {
			t.Fatalf("expected collect error")
		}
	})

	t.Run("stage error", func(t *testing.T) {
		fs := newScriptedFS(map[string][]byte{
			filepath.Join(root, "query.sql"): []byte("select 1;\n"),
		})
		fs.readFileErrors[filepath.Join(root, "query.sql")] = map[int]error{
			2: errors.New("stage read failed"),
		}
		_, err := bindPreparePsqlInputs(cli.PrepareOptions{}, root, root, prepareArgs{PsqlArgs: []string{"-f", "query.sql"}}, &refctx.Context{
			WorkspaceRoot: root,
			BaseDir:       root,
			RefMode:       "blob",
			FileSystem:    fs,
		}, strings.NewReader(""))
		if err == nil || !strings.Contains(err.Error(), "stage read failed") {
			t.Fatalf("expected stage read error, got %v", err)
		}
	})

	_ = parentRef
}

func TestBindPrepareLiquibaseInputsAdditionalBranches(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not in PATH")
	}

	repo, _ := initPrepareRefTestRepo(t)
	cwd := filepath.Join(repo, "examples")
	root := t.TempDir()
	writeTestFile(t, root, "master.xml", "<databaseChangeLog/>\n")

	t.Run("resolve error", func(t *testing.T) {
		_, err := bindPrepareLiquibaseInputs(cli.PrepareOptions{}, repo, cwd, prepareArgs{Ref: "missing-ref", RefMode: "blob"}, nil, "", "", false)
		if err == nil {
			t.Fatalf("expected resolve error")
		}
	})

	t.Run("normalize error", func(t *testing.T) {
		_, err := bindPrepareLiquibaseInputs(cli.PrepareOptions{}, root, root, prepareArgs{PsqlArgs: []string{"update", "--changelog-file="}}, &refctx.Context{
			WorkspaceRoot: root,
			BaseDir:       root,
			RefMode:       "blob",
			FileSystem:    inputset.OSFileSystem{},
		}, "", "", false)
		if err == nil {
			t.Fatalf("expected normalize error")
		}
	})

	t.Run("relativize blob paths", func(t *testing.T) {
		binding, err := bindPrepareLiquibaseInputs(cli.PrepareOptions{}, root, root, prepareArgs{PsqlArgs: []string{"update", "--changelog-file", "master.xml"}}, &refctx.Context{
			WorkspaceRoot: root,
			BaseDir:       root,
			RefMode:       "blob",
			FileSystem:    inputset.OSFileSystem{},
		}, "", "", true)
		if err != nil {
			t.Fatalf("bindPrepareLiquibaseInputs(relativize): %v", err)
		}
		if len(binding.LiquibaseArgs) < 3 || filepath.IsAbs(binding.LiquibaseArgs[2]) {
			t.Fatalf("expected relativized changelog path, got %v", binding.LiquibaseArgs)
		}
	})

	t.Run("relative workdir rejected", func(t *testing.T) {
		_, err := bindPrepareLiquibaseInputs(cli.PrepareOptions{}, root, "relative", prepareArgs{PsqlArgs: []string{"update"}}, nil, "", "", false)
		if err == nil || !strings.Contains(err.Error(), "path is not absolute") {
			t.Fatalf("expected relative workdir error, got %v", err)
		}
	})
}

func TestRefPrepareFilesystemErrorBranches(t *testing.T) {
	root := t.TempDir()
	stageRootFile := filepath.Join(t.TempDir(), "stage-root-file")
	if err := os.WriteFile(stageRootFile, []byte("block"), 0o600); err != nil {
		t.Fatalf("write stage root file: %v", err)
	}

	t.Run("materializeMappedDirs", func(t *testing.T) {
		err := materializeMappedDirs(root, stageRootFile, []string{filepath.Join(root, "nested")})
		if err == nil {
			t.Fatalf("expected materializeMappedDirs error")
		}
	})

	t.Run("materializeRefFiles read error", func(t *testing.T) {
		fs := newScriptedFS(map[string][]byte{
			filepath.Join(root, "query.sql"): []byte("select 1;\n"),
		})
		fs.readFileErrors[filepath.Join(root, "query.sql")] = map[int]error{
			1: errors.New("boom"),
		}
		if _, err := materializeRefFiles(root, []string{filepath.Join(root, "query.sql")}, nil, fs); err == nil || !strings.Contains(err.Error(), "boom") {
			t.Fatalf("expected materializeRefFiles read error, got %v", err)
		}
	})

	t.Run("materializeRefTreeFromPath mkdir error", func(t *testing.T) {
		fs := newScriptedFS(nil)
		fs.dirs[root] = []string{}
		if err := materializeRefTreeFromPath(root, root, stageRootFile, fs); err == nil {
			t.Fatalf("expected materializeRefTreeFromPath mkdir error")
		}
	})

	t.Run("materializeRefTreeFromPath readdir error", func(t *testing.T) {
		fs := newScriptedFS(nil)
		fs.dirs[root] = []string{}
		fs.readDirErrors[root] = map[int]error{1: errors.New("read dir failed")}
		if err := materializeRefTreeFromPath(root, root, t.TempDir(), fs); err == nil || !strings.Contains(err.Error(), "read dir failed") {
			t.Fatalf("expected readdir error, got %v", err)
		}
	})

	t.Run("materializeRefTreeFromPath nested error", func(t *testing.T) {
		fs := newScriptedFS(nil)
		fs.dirs[root] = []string{"child.sql"}
		if err := materializeRefTreeFromPath(root, root, t.TempDir(), fs); err == nil {
			t.Fatalf("expected nested stat error")
		}
	})

	t.Run("materializeRefTreeFromPath read file error", func(t *testing.T) {
		file := filepath.Join(root, "query.sql")
		fs := newScriptedFS(map[string][]byte{file: []byte("select 1;\n")})
		fs.readFileErrors[file] = map[int]error{1: errors.New("file read failed")}
		if err := materializeRefTreeFromPath(root, file, t.TempDir(), fs); err == nil || !strings.Contains(err.Error(), "file read failed") {
			t.Fatalf("expected file read error, got %v", err)
		}
	})
}

func TestLiquibaseHelperVariantBranches(t *testing.T) {
	root := t.TempDir()
	writeTestFile(
		t,
		root,
		"broken.xml",
		`<databaseChangeLog xmlns="http://www.liquibase.org/xml/ns/dbchangelog"><include file="missing.xml" relativeToChangelogFile="true"/></databaseChangeLog>`+"\n",
	)
	localDir := filepath.Join(root, "search")
	if err := os.MkdirAll(localDir, 0o700); err != nil {
		t.Fatalf("mkdir localDir: %v", err)
	}

	t.Run("liquibaseLocalArtifacts missing value variants", func(t *testing.T) {
		cases := [][]string{
			{"update", "--changelog-file"},
			{"update", "--defaults-file"},
			{"update", "--searchPath"},
			{"update", "--search-path"},
			{"update", "--searchPath=" + localDir, "--search-path=" + localDir},
		}
		for _, args := range cases {
			files, dirs, hasLocal, err := liquibaseLocalArtifacts(args, inputset.NewWorkspaceResolver(root, root, nil), inputset.OSFileSystem{})
			if err != nil {
				t.Fatalf("liquibaseLocalArtifacts(%v): %v", args, err)
			}
			if strings.Contains(strings.Join(args, "|"), "--searchPath=") {
				if !hasLocal || len(files) != 0 || len(dirs) != 1 || !pathutil.SameLocalPath(dirs[0], localDir) {
					t.Fatalf("unexpected search-path artifacts for %v: files=%v dirs=%v hasLocal=%v", args, files, dirs, hasLocal)
				}
				continue
			}
			if hasLocal || len(files) != 0 || len(dirs) != 0 {
				t.Fatalf("expected no local artifacts for %v, got files=%v dirs=%v hasLocal=%v", args, files, dirs, hasLocal)
			}
		}
	})

	t.Run("liquibaseLocalArtifacts collect error", func(t *testing.T) {
		_, _, _, err := liquibaseLocalArtifacts([]string{"update", "--changelog-file", "broken.xml"}, inputset.NewWorkspaceResolver(root, root, nil), inputset.OSFileSystem{})
		if err == nil {
			t.Fatalf("expected collect error")
		}
	})

	t.Run("rewriteLiquibaseArgsToRoot variants", func(t *testing.T) {
		stageRoot := t.TempDir()
		args := rewriteLiquibaseArgsToRoot([]string{
			"update",
			"--changelog-file",
			"--searchPath",
			"--changelog-file=classpath:db/master.xml",
			"--defaults-file=classpath:db/liquibase.properties",
			"--searchPath=" + localDir + ",classpath:db",
			"--search-path=" + localDir,
		}, root, stageRoot)
		if args[2] != "--searchPath" {
			t.Fatalf("expected missing-value pair preserved, got %v", args)
		}
		if !strings.Contains(strings.Join(args, "|"), "classpath:db/master.xml") || !strings.Contains(strings.Join(args, "|"), "classpath:db/liquibase.properties") {
			t.Fatalf("expected remote refs preserved, got %v", args)
		}
	})

	t.Run("convertLiquibaseHostPaths variants", func(t *testing.T) {
		args, err := convertLiquibaseHostPaths([]string{
			"update",
			"--changelog-file",
			"--searchPath",
			"--changelog-file=" + filepath.Join(root, "broken.xml"),
			"--defaults-file=" + filepath.Join(root, "broken.xml"),
			"--searchPath=" + localDir + ",classpath:db",
			"--search-path=" + localDir,
		}, func(value string) (string, error) {
			return "conv:" + filepath.Base(value), nil
		})
		if err != nil {
			t.Fatalf("convertLiquibaseHostPaths variants: %v", err)
		}
		if !strings.Contains(strings.Join(args, "|"), "--changelog-file=conv:broken.xml") || !strings.Contains(strings.Join(args, "|"), "--defaults-file=conv:broken.xml") || !strings.Contains(strings.Join(args, "|"), "classpath:db") {
			t.Fatalf("unexpected converted args: %v", args)
		}
	})
}

func TestRefPrepareRemainingArgumentPolicies(t *testing.T) {
	root := t.TempDir()
	absolute := filepath.Join(root, "input.sql")
	convertErr := errors.New("conversion failed")
	failConvert := func(string) (string, error) { return "", convertErr }

	if got := effectiveLiquibaseBindBaseDir(root, nil, nil, prepareArgs{Ref: "HEAD"}); got != root {
		t.Fatalf("effectiveLiquibaseBindBaseDir without context = %q, want %q", got, root)
	}

	fs := newScriptedFS(nil)
	fs.statErrors[root] = map[int]error{1: errors.New("stat failed")}
	if _, err := materializeRefTree(root, fs); err == nil || !strings.Contains(err.Error(), "stat failed") {
		t.Fatalf("materializeRefTree error = %v", err)
	}

	if got, err := convertPsqlFileArgs([]string{"--file=" + absolute}, failConvert); !errors.Is(err, convertErr) || got != nil {
		t.Fatalf("convertPsqlFileArgs --file error = %v, %v", got, err)
	}
	if got, err := convertPsqlFileArgs([]string{"-f" + absolute}, failConvert); !errors.Is(err, convertErr) || got != nil {
		t.Fatalf("convertPsqlFileArgs -f error = %v, %v", got, err)
	}
	for _, args := range [][]string{{"--file"}, {"-f"}} {
		if got, err := convertPsqlFileArgs(args, failConvert); err != nil || !reflect.DeepEqual(got, args) {
			t.Fatalf("convertPsqlFileArgs(%v) = %v, %v", args, got, err)
		}
	}
	if got := rewritePsqlFileArgsToRoot([]string{"--file"}, root, t.TempDir()); !reflect.DeepEqual(got, []string{"--file"}) {
		t.Fatalf("rewritePsqlFileArgsToRoot missing value = %v", got)
	}
	if got, err := convertPsqlFileArgs([]string{"--file", absolute}, failConvert); !errors.Is(err, convertErr) || got != nil {
		t.Fatalf("convertPsqlFileArgs positional error = %v, %v", got, err)
	}
	args := []string{"--file", absolute}
	if got, err := convertPsqlFileArgs(args, nil); err != nil || !reflect.DeepEqual(got, args) {
		t.Fatalf("convertPsqlFileArgs nil converter = %v, %v", got, err)
	}

	for _, args := range [][]string{{"--changelog-file"}, {"--defaults-file"}, {"--searchPath"}, {"--search-path"}} {
		if got := rewriteLiquibaseArgsToRoot(args, root, t.TempDir()); !reflect.DeepEqual(got, args) {
			t.Fatalf("rewriteLiquibaseArgsToRoot(%v) = %v", args, got)
		}
		if got, err := convertLiquibaseHostPaths(args, failConvert); err != nil || !reflect.DeepEqual(got, args) {
			t.Fatalf("convertLiquibaseHostPaths(%v) = %v, %v", args, got, err)
		}
	}
	for _, args := range [][]string{
		{"--changelog-file=" + absolute},
		{"--defaults-file=" + absolute},
		{"--searchPath=" + absolute},
		{"--search-path=" + absolute},
	} {
		if got, err := convertLiquibaseHostPaths(args, failConvert); !errors.Is(err, convertErr) || got != nil {
			t.Fatalf("convertLiquibaseHostPaths(%v) = %v, %v", args, got, err)
		}
	}
	if _, _, _, err := liquibaseLocalArtifacts([]string{"update", "--changelog-file=" + absolute}, inputset.NewWorkspaceResolver(root, root, nil), inputset.OSFileSystem{}); err == nil {
		t.Fatal("expected missing equals-form changelog to fail collection")
	}
	rewritten := rewriteLiquibaseArgsToRoot([]string{
		"--changelog-file", "classpath:db/master.xml",
		"--changelog-file=" + absolute,
	}, root, t.TempDir())
	if rewritten[1] != "classpath:db/master.xml" || rewritten[2] == "--changelog-file="+absolute {
		t.Fatalf("rewriteLiquibaseArgsToRoot remote/local values = %v", rewritten)
	}
	if got, err := convertLiquibaseHostPaths([]string{"--changelog-file", absolute}, failConvert); !errors.Is(err, convertErr) || got != nil {
		t.Fatalf("convertLiquibaseHostPaths positional error = %v, %v", got, err)
	}
	args = []string{"--changelog-file", absolute}
	if got, err := convertLiquibaseHostPaths(args, nil); err != nil || !reflect.DeepEqual(got, args) {
		t.Fatalf("convertLiquibaseHostPaths nil converter = %v, %v", got, err)
	}
}

func containsPath(paths []string, want string) bool {
	for _, path := range paths {
		if pathutil.SameLocalPath(path, want) {
			return true
		}
	}
	return false
}

type scriptedFS struct {
	files          map[string][]byte
	dirs           map[string][]string
	readCounts     map[string]int
	readDirCounts  map[string]int
	statCounts     map[string]int
	readFileErrors map[string]map[int]error
	readDirErrors  map[string]map[int]error
	statErrors     map[string]map[int]error
}

func newScriptedFS(files map[string][]byte) *scriptedFS {
	fs := &scriptedFS{
		files:          make(map[string][]byte),
		dirs:           make(map[string][]string),
		readCounts:     make(map[string]int),
		readDirCounts:  make(map[string]int),
		statCounts:     make(map[string]int),
		readFileErrors: make(map[string]map[int]error),
		readDirErrors:  make(map[string]map[int]error),
		statErrors:     make(map[string]map[int]error),
	}
	for path, data := range files {
		cleaned := filepath.Clean(path)
		fs.files[cleaned] = data
		dir := filepath.Dir(cleaned)
		base := filepath.Base(cleaned)
		if _, ok := fs.dirs[dir]; !ok {
			fs.dirs[dir] = []string{}
		}
		if !containsTestString(fs.dirs[dir], base) {
			fs.dirs[dir] = append(fs.dirs[dir], base)
		}
	}
	return fs
}

func (s *scriptedFS) Stat(path string) (fs.FileInfo, error) {
	cleaned := filepath.Clean(path)
	s.statCounts[cleaned]++
	if err := scriptedErr(s.statErrors[cleaned], s.statCounts[cleaned]); err != nil {
		return nil, err
	}
	if _, ok := s.dirs[cleaned]; ok {
		return stubFileInfo{name: filepath.Base(cleaned), dir: true}, nil
	}
	if data, ok := s.files[cleaned]; ok {
		return stubFileInfo{name: filepath.Base(cleaned), size: int64(len(data))}, nil
	}
	return nil, fs.ErrNotExist
}

func (s *scriptedFS) ReadFile(path string) ([]byte, error) {
	cleaned := filepath.Clean(path)
	s.readCounts[cleaned]++
	if err := scriptedErr(s.readFileErrors[cleaned], s.readCounts[cleaned]); err != nil {
		return nil, err
	}
	data, ok := s.files[cleaned]
	if !ok {
		return nil, fs.ErrNotExist
	}
	return append([]byte(nil), data...), nil
}

func (s *scriptedFS) ReadDir(path string) ([]fs.DirEntry, error) {
	cleaned := filepath.Clean(path)
	s.readDirCounts[cleaned]++
	if err := scriptedErr(s.readDirErrors[cleaned], s.readDirCounts[cleaned]); err != nil {
		return nil, err
	}
	names, ok := s.dirs[cleaned]
	if !ok {
		return nil, fs.ErrNotExist
	}
	entries := make([]fs.DirEntry, 0, len(names))
	for _, name := range names {
		child := filepath.Join(cleaned, name)
		_, isDir := s.dirs[child]
		entries = append(entries, stubDirEntry{info: stubFileInfo{name: name, dir: isDir}})
	}
	return entries, nil
}

func scriptedErr(errs map[int]error, count int) error {
	if errs == nil {
		return nil
	}
	return errs[count]
}

type stubFileInfo struct {
	name string
	dir  bool
	size int64
}

func (s stubFileInfo) Name() string { return s.name }
func (s stubFileInfo) Size() int64  { return s.size }
func (s stubFileInfo) Mode() fs.FileMode {
	if s.dir {
		return fs.ModeDir | 0o755
	}
	return 0o644
}
func (s stubFileInfo) ModTime() time.Time { return time.Time{} }
func (s stubFileInfo) IsDir() bool        { return s.dir }
func (s stubFileInfo) Sys() any           { return nil }

type stubDirEntry struct {
	info stubFileInfo
}

func (s stubDirEntry) Name() string               { return s.info.Name() }
func (s stubDirEntry) IsDir() bool                { return s.info.IsDir() }
func (s stubDirEntry) Type() fs.FileMode          { return s.info.Mode().Type() }
func (s stubDirEntry) Info() (fs.FileInfo, error) { return s.info, nil }

func containsTestString(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
