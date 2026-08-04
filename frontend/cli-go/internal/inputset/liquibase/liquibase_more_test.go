package liquibase

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sqlrs/cli/internal/inputset"
	"github.com/sqlrs/cli/internal/pathutil"
	"gopkg.in/yaml.v3"
)

type hookFS struct {
	stat     func(string) (fs.FileInfo, error)
	readFile func(string) ([]byte, error)
	readDir  func(string) ([]fs.DirEntry, error)
}

func (h hookFS) Stat(path string) (fs.FileInfo, error) {
	if h.stat != nil {
		return h.stat(path)
	}
	return os.Stat(path)
}

func (h hookFS) ReadFile(path string) ([]byte, error) {
	if h.readFile != nil {
		return h.readFile(path)
	}
	return os.ReadFile(path)
}

func (h hookFS) ReadDir(path string) ([]fs.DirEntry, error) {
	if h.readDir != nil {
		return h.readDir(path)
	}
	return os.ReadDir(path)
}

func TestBoolishUnmarshalAndUseChangelogDir(t *testing.T) {
	var value boolish
	if err := yaml.Unmarshal([]byte("false\n"), &value); err != nil {
		t.Fatalf("yaml.Unmarshal: %v", err)
	}
	if value.useChangelogDir() {
		t.Fatalf("expected false YAML flag to disable changelog dir")
	}

	if err := json.Unmarshal([]byte("true"), &value); err != nil {
		t.Fatalf("json.Unmarshal(bool): %v", err)
	}
	if !value.useChangelogDir() {
		t.Fatalf("expected true JSON bool to keep changelog dir")
	}

	if err := json.Unmarshal([]byte(`"false"`), &value); err != nil {
		t.Fatalf("json.Unmarshal(string): %v", err)
	}
	if value.useChangelogDir() {
		t.Fatalf("expected false JSON string to disable changelog dir")
	}

	if err := json.Unmarshal([]byte("123"), &value); err == nil {
		t.Fatalf("expected invalid JSON type error")
	}
}

func TestNormalizeArgsKeepsSearchPathAliasAndRejectsEmptyEquals(t *testing.T) {
	root := t.TempDir()
	resolver := inputset.NewWorkspaceResolver(root, root, nil)

	args, err := NormalizeArgs([]string{"--search-path=dir"}, resolver, false)
	if err != nil {
		t.Fatalf("NormalizeArgs: %v", err)
	}
	if len(args) != 1 || args[0] != "--search-path="+filepath.Join(root, "dir") {
		t.Fatalf("unexpected args: %+v", args)
	}

	cases := []struct {
		name string
		args []string
		code string
	}{
		{name: "empty changelog", args: []string{"--changelog-file="}, code: "empty_path"},
		{name: "empty defaults", args: []string{"--defaults-file="}, code: "empty_path"},
		{name: "empty searchPath", args: []string{"--searchPath="}, code: "empty_search_path"},
		{name: "missing flag value", args: []string{"--search-path"}, code: "missing_path_arg"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NormalizeArgs(tc.args, resolver, true)
			var userErr *inputset.UserError
			if !errors.As(err, &userErr) || userErr.Code != tc.code {
				t.Fatalf("expected %s, got %v", tc.code, err)
			}
		})
	}

	args, err = NormalizeArgs([]string{
		"--changelog-file=master.xml",
		"--defaults-file=defaults.properties",
		"--searchPath=dir",
		"--search-path=dir",
	}, resolver, true)
	if err != nil {
		t.Fatalf("NormalizeArgs equals forms: %v", err)
	}
	if got := strings.Join(args, "|"); !strings.Contains(got, "--changelog-file="+filepath.Join(root, "master.xml")) || !strings.Contains(got, "--defaults-file="+filepath.Join(root, "defaults.properties")) || !strings.Contains(got, "--searchPath="+filepath.Join(root, "dir")) {
		t.Fatalf("unexpected equals-form args: %+v", args)
	}

	badResolver := inputset.NewWorkspaceResolver(root, root, func(string) (string, error) {
		return "", errors.New("boom")
	})
	if _, err := NormalizeArgs([]string{"--changelog-file", "master.xml"}, badResolver, true); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected positional resolver error, got %v", err)
	}
	if _, err := NormalizeArgs([]string{"--search-path=dir"}, badResolver, true); err == nil || !strings.Contains(err.Error(), "boom") {
		t.Fatalf("expected equals search-path resolver error, got %v", err)
	}
}

func TestCollectHandlesJSONIncludeAllAndErrors(t *testing.T) {
	t.Run("includeAll sorts and filters", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, filepath.Join(root, "defaults.properties"), "x=y\n")
		writeFile(t, filepath.Join(root, "master.json"), `{"databaseChangeLog":[{"includeAll":{"path":"changes"}}]}`)
		writeFile(t, filepath.Join(root, "changes", "b.sql"), "select 2;\n")
		writeFile(t, filepath.Join(root, "changes", "a.xml"), `<databaseChangeLog xmlns="http://www.liquibase.org/xml/ns/dbchangelog"/>`)
		writeFile(t, filepath.Join(root, "changes", "skip.txt"), "ignore\n")
		if err := os.MkdirAll(filepath.Join(root, "changes", "nested"), 0o700); err != nil {
			t.Fatalf("MkdirAll nested: %v", err)
		}

		set, err := Collect([]string{
			"--defaults-file=defaults.properties",
			"--changelog-file=master.json",
		}, inputset.NewDiffResolver(root), inputset.OSFileSystem{})
		if err != nil {
			t.Fatalf("Collect: %v", err)
		}
		want := []string{
			"defaults.properties",
			"master.json",
			filepath.ToSlash("changes/a.xml"),
			filepath.ToSlash("changes/b.sql"),
		}
		if len(set.Entries) != len(want) {
			t.Fatalf("entries = %+v", set.Entries)
		}
		for i, entry := range set.Entries {
			if entry.Path != want[i] {
				t.Fatalf("entry[%d] = %+v, want %q", i, entry, want[i])
			}
		}
	})

	t.Run("missing changelog", func(t *testing.T) {
		_, err := Collect([]string{"update"}, inputset.NewDiffResolver(t.TempDir()), inputset.OSFileSystem{})
		if err == nil || !strings.Contains(err.Error(), "no --changelog-file") {
			t.Fatalf("expected missing changelog error, got %v", err)
		}
	})

	t.Run("malformed formats", func(t *testing.T) {
		cases := []struct {
			name    string
			path    string
			content string
		}{
			{name: "xml", path: "master.xml", content: `<databaseChangeLog><include></databaseChangeLog>`},
			{name: "yaml", path: "master.yaml", content: "databaseChangeLog: ["},
			{name: "json", path: "master.json", content: `{"databaseChangeLog":[}`},
		}
		for _, tc := range cases {
			t.Run(tc.name, func(t *testing.T) {
				root := t.TempDir()
				writeFile(t, filepath.Join(root, tc.path), tc.content)
				_, err := Collect([]string{"--changelog-file", tc.path}, inputset.NewDiffResolver(root), inputset.OSFileSystem{})
				if err == nil || !strings.Contains(err.Error(), "parse liquibase changelog") {
					t.Fatalf("expected parse error, got %v", err)
				}
			})
		}
	})

	t.Run("read leaf error", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, filepath.Join(root, "defaults.properties"), "x=y\n")
		writeFile(t, filepath.Join(root, "master.xml"), `<databaseChangeLog xmlns="http://www.liquibase.org/xml/ns/dbchangelog"/>`)
		fs := hookFS{
			readFile: func(path string) ([]byte, error) {
				if strings.HasSuffix(filepath.ToSlash(path), "/defaults.properties") || filepath.Base(path) == "defaults.properties" {
					return nil, errors.New("boom")
				}
				return os.ReadFile(path)
			},
		}
		_, err := Collect([]string{
			"--defaults-file=defaults.properties",
			"--changelog-file=master.xml",
		}, inputset.NewDiffResolver(root), fs)
		if err == nil || !strings.Contains(err.Error(), "read") || !strings.Contains(err.Error(), "boom") {
			t.Fatalf("expected wrapped read error, got %v", err)
		}
	})
}

func TestLiquibaseHelperBranches(t *testing.T) {
	root := t.TempDir()
	aliasPath := filepath.Join(root, "aliases", "demo.prep.s9s.yaml")
	aliasDir := filepath.Dir(aliasPath)
	if err := os.MkdirAll(filepath.Join(aliasDir, "dir"), 0o700); err != nil {
		t.Fatalf("MkdirAll dir: %v", err)
	}
	writeFile(t, filepath.Join(aliasDir, "shared", "child.xml"), `<databaseChangeLog xmlns="http://www.liquibase.org/xml/ns/dbchangelog"/>`)

	resolver := inputset.NewAliasResolver(root, aliasPath)
	if issues := validateSearchPath("", resolver, inputset.OSFileSystem{}); len(issues) != 1 || issues[0].Code != "empty_search_path" {
		t.Fatalf("unexpected empty searchPath issues: %+v", issues)
	}
	if issues := validateSearchPath("missing", resolver, inputset.OSFileSystem{}); len(issues) != 1 || issues[0].Code != "missing_path" {
		t.Fatalf("unexpected missing searchPath issues: %+v", issues)
	}

	if issue, ok := validateLocalArg("classpath:db/changelog.xml", resolver, inputset.OSFileSystem{}, true); ok {
		t.Fatalf("expected remote ref to be ignored, got %+v", issue)
	}
	if issue, ok := validateLocalArg("", resolver, inputset.OSFileSystem{}, true); !ok || issue.Code != "empty_path" {
		t.Fatalf("expected empty_path issue, got %+v ok=%v", issue, ok)
	}
	if issue, ok := validateLocalArg("dir", resolver, inputset.OSFileSystem{}, true); !ok || issue.Code != "expected_file" {
		t.Fatalf("expected expected_file issue, got %+v ok=%v", issue, ok)
	}

	badResolver := inputset.NewWorkspaceResolver(root, root, func(string) (string, error) {
		return "", errors.New("boom")
	})
	if issue, ok := validateLocalArg("file.xml", badResolver, inputset.OSFileSystem{}, false); !ok || issue.Code != "invalid_path" {
		t.Fatalf("expected invalid_path issue, got %+v ok=%v", issue, ok)
	}
	if issue, ok := validateLocalArg("../../outside.xml", resolver, inputset.OSFileSystem{}, false); !ok || issue.Code != "path_outside_workspace" {
		t.Fatalf("expected path_outside_workspace issue, got %+v ok=%v", issue, ok)
	}

	searchParts, err := resolveSearchPathParts("classpath:db,shared", resolver)
	if err != nil {
		t.Fatalf("resolveSearchPathParts: %v", err)
	}
	if len(searchParts) != 1 || !pathutil.SameLocalPath(searchParts[0], filepath.Join(aliasDir, "shared")) {
		t.Fatalf("unexpected search parts: %+v", searchParts)
	}

	tracker := &tracker{
		fs:          inputset.OSFileSystem{},
		root:        root,
		searchPaths: []string{filepath.Join(aliasDir, "shared")},
		seen:        make(map[string]struct{}),
	}
	if got := tracker.resolveIncludePath(filepath.Join(root, "db"), " ", true); got != "" {
		t.Fatalf("expected blank include to be ignored, got %q", got)
	}
	if got := tracker.resolveIncludePath(filepath.Join(root, "db"), "classpath:db", false); got != "" {
		t.Fatalf("expected remote include to be ignored, got %q", got)
	}
	abs := filepath.Join(root, "abs.xml")
	if got := tracker.resolveIncludePath(filepath.Join(root, "db"), abs, true); !pathutil.SameLocalPath(got, abs) {
		t.Fatalf("expected abs include to stay absolute, got %q", got)
	}
	if got := tracker.resolveIncludePath(filepath.Join(root, "db"), "child.xml", false); !pathutil.SameLocalPath(got, filepath.Join(aliasDir, "shared", "child.xml")) {
		t.Fatalf("expected search-path resolution, got %q", got)
	}
	if got := tracker.resolveIncludePath(filepath.Join(root, "db"), "missing.xml", false); !pathutil.SameLocalPath(got, filepath.Join(root, "missing.xml")) {
		t.Fatalf("expected root fallback, got %q", got)
	}
}

func TestParseIncludesAndFlattenHelpers(t *testing.T) {
	xmlIncludes, xmlIncludeAll, err := parseIncludes("master.xml", []byte(`<databaseChangeLog xmlns="http://www.liquibase.org/xml/ns/dbchangelog"><include file="child.xml"/><includeAll path="dir"/></databaseChangeLog>`))
	if err != nil {
		t.Fatalf("parseIncludes(xml): %v", err)
	}
	if len(xmlIncludes) != 1 || xmlIncludes[0].File != "child.xml" || len(xmlIncludeAll) != 1 || xmlIncludeAll[0].Path != "dir" {
		t.Fatalf("unexpected xml parse result: includes=%+v includeAll=%+v", xmlIncludes, xmlIncludeAll)
	}

	yamlIncludes, yamlIncludeAll, err := parseIncludes("master.yaml", []byte("databaseChangeLog:\n  - include:\n      file: child.yaml\n  - includeAll:\n      path: dir\n"))
	if err != nil {
		t.Fatalf("parseIncludes(yaml): %v", err)
	}
	if len(yamlIncludes) != 1 || yamlIncludes[0].File != "child.yaml" || len(yamlIncludeAll) != 1 || yamlIncludeAll[0].Path != "dir" {
		t.Fatalf("unexpected yaml parse result: includes=%+v includeAll=%+v", yamlIncludes, yamlIncludeAll)
	}

	jsonIncludes, jsonIncludeAll, err := parseIncludes("master.json", []byte(`{"databaseChangeLog":[{"include":{"file":"child.json"}},{"includeAll":{"path":"dir"}}]}`))
	if err != nil {
		t.Fatalf("parseIncludes(json): %v", err)
	}
	if len(jsonIncludes) != 1 || jsonIncludes[0].File != "child.json" || len(jsonIncludeAll) != 1 || jsonIncludeAll[0].Path != "dir" {
		t.Fatalf("unexpected json parse result: includes=%+v includeAll=%+v", jsonIncludes, jsonIncludeAll)
	}

	otherIncludes, otherIncludeAll, err := parseIncludes("leaf.sql", []byte("select 1;"))
	if err != nil {
		t.Fatalf("parseIncludes(other): %v", err)
	}
	if otherIncludes != nil || otherIncludeAll != nil {
		t.Fatalf("expected non-changelog leaf to have no includes, got %+v %+v", otherIncludes, otherIncludeAll)
	}

	items := []changeItem{
		{Include: &includeSpec{File: "one.xml"}},
		{},
		{IncludeAll: &includeAll{Path: "dir"}},
	}
	if got := flattenIncludes(items); len(got) != 1 || got[0].File != "one.xml" {
		t.Fatalf("unexpected flattenIncludes: %+v", got)
	}
	if got := flattenIncludeAll(items); len(got) != 1 || got[0].Path != "dir" {
		t.Fatalf("unexpected flattenIncludeAll: %+v", got)
	}
}

func TestRewriteHelpersAndCollectDeclaredBranches(t *testing.T) {
	root := t.TempDir()
	resolver := inputset.NewWorkspaceResolver(root, root, nil)
	writeFile(t, filepath.Join(root, "dir", "child.xml"), `<databaseChangeLog xmlns="http://www.liquibase.org/xml/ns/dbchangelog"/>`)

	if got := normalizeFlag("--search-path", true); got != "--searchPath" {
		t.Fatalf("unexpected normalized flag: %q", got)
	}
	if got := normalizeFlag("--search-path", false); got != "--search-path" {
		t.Fatalf("unexpected preserved flag: %q", got)
	}

	if value, err := rewriteValue("--defaults-file", "classpath:db/changelog.xml", resolver); err != nil || value != "classpath:db/changelog.xml" {
		t.Fatalf("expected remote defaults-file to pass through, got %q err=%v", value, err)
	}
	if _, err := rewriteSearchPath("dir,,child", resolver); err == nil {
		t.Fatalf("expected empty searchPath item error")
	}

	declared, searchPaths, changelogPath, err := collectDeclared([]string{
		"--defaults-file", "classpath:db/defaults.properties",
		"--search-path=classpath:db,dir",
		"--changelog-file", "dir/child.xml",
	}, resolver)
	if err != nil {
		t.Fatalf("collectDeclared: %v", err)
	}
	if len(declared) != 1 || declared[0].flag != "--changelog-file" || len(searchPaths) != 1 || !pathutil.SameLocalPath(changelogPath, filepath.Join(root, "dir", "child.xml")) {
		t.Fatalf("unexpected collectDeclared result: declared=%+v searchPaths=%+v changelog=%q", declared, searchPaths, changelogPath)
	}

	if _, _, _, err := collectDeclared([]string{"--searchPath"}, resolver); err == nil {
		t.Fatalf("expected missing searchPath value error")
	}
	if _, _, _, err := collectDeclared([]string{"--defaults-file= "}, resolver); err == nil {
		t.Fatalf("expected defaults-file equals error")
	}
	if _, _, _, err := collectDeclared([]string{"--search-path= "}, resolver); err == nil {
		t.Fatalf("expected search-path equals error")
	}
	if _, _, _, err := collectDeclared([]string{"--changelog-file", "dir/child.xml", "--searchPath= "}, resolver); err == nil {
		t.Fatalf("expected appendSearchPaths error")
	}

	tracker := &tracker{
		fs:   inputset.OSFileSystem{},
		root: root,
		seen: make(map[string]struct{}),
	}
	var order []string
	if err := tracker.addLeaf(filepath.Join(root, "dir", "child.xml"), &order); err != nil {
		t.Fatalf("addLeaf: %v", err)
	}
	if err := tracker.addLeaf(filepath.Join(root, "dir", "child.xml"), &order); err != nil {
		t.Fatalf("addLeaf duplicate: %v", err)
	}
	if len(order) != 1 {
		t.Fatalf("expected duplicate leaf to be ignored, got %+v", order)
	}

	if err := tracker.collectIncludeAll(filepath.Join(root, "missing"), &order); err == nil {
		t.Fatalf("expected missing includeAll dir error")
	}
}

func TestLiquibaseCollectAndValidateAdditionalBranches(t *testing.T) {
	t.Run("collect declared and leaf stat errors", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, filepath.Join(root, "master.xml"), `<databaseChangeLog xmlns="http://www.liquibase.org/xml/ns/dbchangelog"/>`)
		badResolver := inputset.NewWorkspaceResolver(root, root, func(string) (string, error) {
			return "", errors.New("boom")
		})
		if _, err := Collect([]string{"--changelog-file", "master.xml"}, badResolver, inputset.OSFileSystem{}); err == nil || !strings.Contains(err.Error(), "boom") {
			t.Fatalf("expected collectDeclared error, got %v", err)
		}

		fs := hookFS{
			stat: func(path string) (fs.FileInfo, error) {
				if filepath.Base(path) == "defaults.properties" {
					return nil, errors.New("boom")
				}
				return os.Stat(path)
			},
		}
		writeFile(t, filepath.Join(root, "defaults.properties"), "x=y\n")
		if _, err := Collect([]string{"--defaults-file=defaults.properties", "--changelog-file=master.xml"}, inputset.NewDiffResolver(root), fs); err == nil || !strings.Contains(err.Error(), "boom") {
			t.Fatalf("expected addLeaf stat error, got %v", err)
		}
	})

	t.Run("validate long and equals forms", func(t *testing.T) {
		root := t.TempDir()
		resolver := inputset.NewAliasResolver(root, filepath.Join(root, "aliases", "demo.prep.s9s.yaml"))
		issues := ValidateArgs([]string{
			"--changelog-file",
			"--searchPath",
			"--changelog-file=missing.xml",
			"--defaults-file=missing.properties",
			"--searchPath=missing",
			"--search-path=missing",
		}, resolver, inputset.OSFileSystem{})
		var missingPathCount int
		for _, issue := range issues {
			if issue.Code == "missing_path" {
				missingPathCount++
			}
		}
		if missingPathCount < 5 {
			t.Fatalf("expected missing_path issues for long and equals forms, got %+v", issues)
		}
	})

	t.Run("tracker branch coverage", func(t *testing.T) {
		root := t.TempDir()
		writeFile(t, filepath.Join(root, "master.xml"), `<databaseChangeLog xmlns="http://www.liquibase.org/xml/ns/dbchangelog"><include file=" "/><include file="missing.xml"/><includeAll path=" "/></databaseChangeLog>`)
		trk := &tracker{
			fs:   inputset.OSFileSystem{},
			root: root,
			seen: make(map[string]struct{}),
		}
		var order []string
		if err := trk.collect(filepath.Join(root, "master.xml"), &order); err == nil || !strings.Contains(err.Error(), "missing.xml") {
			t.Fatalf("expected child collect error, got %v", err)
		}

		order = nil
		trk.seen[filepath.Join(root, "master.xml")] = struct{}{}
		if err := trk.collect(filepath.Join(root, "master.xml"), &order); err != nil {
			t.Fatalf("expected seen collect to noop, got %v", err)
		}

		writeFile(t, filepath.Join(root, "changes", "bad.xml"), `<databaseChangeLog>`)
		trk = &tracker{
			fs:   inputset.OSFileSystem{},
			root: root,
			seen: make(map[string]struct{}),
		}
		if err := trk.collectIncludeAll(filepath.Join(root, "changes"), &order); err == nil || !strings.Contains(err.Error(), "parse liquibase changelog") {
			t.Fatalf("expected includeAll child collect error, got %v", err)
		}
	})
}

func TestLiquibaseSecondPassCollectionErrorsAndBlobHashes(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "defaults.properties")
	writeFile(t, file, "x=y\n")
	resolver := inputset.NewWorkspaceResolver(root, root, nil)

	if _, err := CollectInvocationInputs([]string{"--defaults-file"}, resolver, inputset.OSFileSystem{}); err == nil {
		t.Fatal("expected missing invocation path error")
	}

	set, err := buildInputSet([]string{file}, root, hookBlobFS{
		hookFS:  hookFS{},
		blobOID: func(string) (string, error) { return "blob-oid", nil },
	})
	if err != nil || len(set.Entries) != 1 || set.Entries[0].Hash != "blob-oid" {
		t.Fatalf("blob input set = %#v, %v", set, err)
	}
	if _, err := buildInputSet([]string{file}, root, hookBlobFS{
		hookFS:  hookFS{},
		blobOID: func(string) (string, error) { return "", errors.New("oid failed") },
	}); err == nil || !strings.Contains(err.Error(), "hash") {
		t.Fatalf("expected blob hash error, got %v", err)
	}
	if _, err := buildInputSet([]string{file}, root, hookFS{
		readFile: func(string) ([]byte, error) { return nil, errors.New("read failed") },
	}); err == nil || !strings.Contains(err.Error(), "read") {
		t.Fatalf("expected input read error, got %v", err)
	}

	issues := append(
		ValidateArgs([]string{"--defaults-file"}, resolver, inputset.OSFileSystem{}),
		ValidateArgs([]string{"--search-path"}, resolver, inputset.OSFileSystem{})...,
	)
	if len(issues) != 2 || issues[0].Code != "missing_path_arg" || issues[1].Code != "missing_path_arg" {
		t.Fatalf("missing argument issues = %#v", issues)
	}
}

func TestLiquibaseSecondPassSearchAndTrackerBranches(t *testing.T) {
	root := t.TempDir()
	resolver := inputset.NewWorkspaceResolver(root, root, nil)
	for _, value := range []string{"", "one,,two"} {
		if _, err := resolveSearchPathParts(value, resolver); err == nil {
			t.Fatalf("resolveSearchPathParts(%q) should fail", value)
		}
	}
	parts, err := resolveSearchPathParts("classpath:db,https://example.org/db", resolver)
	if err != nil || len(parts) != 0 {
		t.Fatalf("remote search parts = %v, %v", parts, err)
	}
	badResolver := inputset.NewWorkspaceResolver(root, root, func(string) (string, error) {
		return "", errors.New("resolve failed")
	})
	if _, err := resolveSearchPathParts("local", badResolver); err == nil {
		t.Fatal("expected search path resolution error")
	}

	master := filepath.Join(root, "master.xml")
	writeFile(t, master, `<databaseChangeLog xmlns="http://www.liquibase.org/xml/ns/dbchangelog"/>`)
	trk := &tracker{
		fs: hookFS{
			readFile: func(string) ([]byte, error) { return nil, errors.New("changelog read failed") },
		},
		root: root,
		seen: make(map[string]struct{}),
	}
	var order []string
	if err := trk.collect(master, &order); err == nil || !strings.Contains(err.Error(), "read failed") {
		t.Fatalf("expected changelog read error, got %v", err)
	}

	writeFile(t, master, `<databaseChangeLog xmlns="http://www.liquibase.org/xml/ns/dbchangelog"><include file="classpath:db/child.xml"/><includeAll path="https://example.org/db"/><changeSet><sqlFile path="classpath:db/query.sql"/></changeSet></databaseChangeLog>`)
	trk = &tracker{fs: inputset.OSFileSystem{}, root: root, seen: make(map[string]struct{})}
	order = nil
	if err := trk.collect(master, &order); err != nil || len(order) != 1 {
		t.Fatalf("remote-only changelog order = %v, %v", order, err)
	}

	writeFile(t, master, `<databaseChangeLog xmlns="http://www.liquibase.org/xml/ns/dbchangelog"><includeAll path="missing" relativeToChangelogFile="true"/></databaseChangeLog>`)
	trk = &tracker{fs: inputset.OSFileSystem{}, root: root, seen: make(map[string]struct{})}
	if err := trk.collect(master, &order); err == nil || !strings.Contains(err.Error(), "includeAll") {
		t.Fatalf("expected includeAll error, got %v", err)
	}

	writeFile(t, master, `<databaseChangeLog xmlns="http://www.liquibase.org/xml/ns/dbchangelog"><changeSet><loadData file="missing.csv" relativeToChangelogFile="true"/></changeSet></databaseChangeLog>`)
	trk = &tracker{fs: inputset.OSFileSystem{}, root: root, seen: make(map[string]struct{})}
	if err := trk.collect(master, &order); err == nil {
		t.Fatal("expected local leaf error")
	}
}

func TestLiquibaseSecondPassParsingAndLocalPathFlattening(t *testing.T) {
	if _, _, err := parseIncludes("broken.json", []byte(`{"databaseChangeLog":`)); err == nil {
		t.Fatal("expected parseIncludes error")
	}

	sqlFile := &localPathRef{File: "query.sql"}
	loadData := &localPathRef{File: "data.csv"}
	loadUpdate := &localPathRef{File: "updates.csv"}
	refs := localPathRefsFromChangeSet(changeSetSpec{
		SQLFile:        sqlFile,
		LoadData:       loadData,
		LoadUpdateData: loadUpdate,
		Changes: []changeSetChange{{
			SQLFile:        sqlFile,
			LoadData:       loadData,
			LoadUpdateData: loadUpdate,
		}},
	})
	if len(refs) != 6 {
		t.Fatalf("local refs = %#v", refs)
	}
	items := []changeItem{{}, {ChangeSet: &changeSetSpec{SQLFile: sqlFile}}}
	if got := flattenLocalPathRefs(items); len(got) != 1 || got[0].File != "query.sql" {
		t.Fatalf("flattened local refs = %#v", got)
	}
}

type hookBlobFS struct {
	hookFS
	blobOID func(string) (string, error)
}

func (h hookBlobFS) BlobOID(path string) (string, error) {
	return h.blobOID(path)
}
