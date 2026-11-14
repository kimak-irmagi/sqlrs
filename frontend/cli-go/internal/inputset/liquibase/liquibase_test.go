package liquibase

import (
	"crypto/sha256"
	"encoding/hex"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sqlrs/cli/internal/inputset"
)

func TestNormalizeArgsRewritesAllPathFlags(t *testing.T) {
	root := t.TempDir()
	resolver := inputset.NewWorkspaceResolver(root, root, nil)

	args, err := NormalizeArgs([]string{
		"update",
		"--changelog-file", "db/changelog.xml",
		"--defaults-file=defaults.properties",
		"--search-path", "db, conf",
	}, resolver, true)
	if err != nil {
		t.Fatalf("NormalizeArgs: %v", err)
	}

	want := []string{
		"update",
		"--changelog-file", filepath.Join(root, "db", "changelog.xml"),
		"--defaults-file=" + filepath.Join(root, "defaults.properties"),
		"--searchPath", filepath.Join(root, "db") + "," + filepath.Join(root, "conf"),
	}
	if strings.Join(args, "|") != strings.Join(want, "|") {
		t.Fatalf("args = %q, want %q", strings.Join(args, "|"), strings.Join(want, "|"))
	}
}

func TestCollectIncludesDefaultsFileAndYamlChangelog(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "defaults.properties"), "x=y\n")
	writeFile(t, filepath.Join(root, "changelog", "root.yaml"), "databaseChangeLog:\n  - include:\n      file: child.sql\n")
	writeFile(t, filepath.Join(root, "changelog", "child.sql"), "--liquibase formatted sql\n--changeset dev:child\nselect 1;\n")

	resolver := inputset.NewDiffResolver(root)
	set, err := Collect([]string{
		"--defaults-file", "defaults.properties",
		"--changelog-file", "changelog/root.yaml",
	}, resolver, inputset.OSFileSystem{})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(set.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %+v", set.Entries)
	}
	if set.Entries[0].Path != "defaults.properties" {
		t.Fatalf("entry[0] = %+v", set.Entries[0])
	}
	if set.Entries[1].Path != filepath.ToSlash("changelog/root.yaml") {
		t.Fatalf("entry[1] = %+v", set.Entries[1])
	}
	if set.Entries[2].Path != filepath.ToSlash("changelog/child.sql") {
		t.Fatalf("entry[2] = %+v", set.Entries[2])
	}
}

func TestCollectResolvesRelativeFalseAgainstSearchPath(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "config", "liquibase", "master.xml"), `<databaseChangeLog xmlns="http://www.liquibase.org/xml/ns/dbchangelog"><include file="child.xml" relativeToChangelogFile="false"/></databaseChangeLog>`)
	writeFile(t, filepath.Join(root, "shared", "child.xml"), `<databaseChangeLog xmlns="http://www.liquibase.org/xml/ns/dbchangelog"/>`)

	resolver := inputset.NewDiffResolver(root)
	set, err := Collect([]string{
		"--searchPath", ".,shared",
		"--changelog-file", "config/liquibase/master.xml",
	}, resolver, inputset.OSFileSystem{})
	if err != nil {
		t.Fatalf("Collect: %v", err)
	}
	if len(set.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %+v", set.Entries)
	}
	if set.Entries[1].Path != filepath.ToSlash("shared/child.xml") {
		t.Fatalf("entry[1] = %+v", set.Entries[1])
	}
}

func TestCollectInvocationInputsIncludesDefaultsFileWithoutChangelog(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "defaults.properties"), "x=y\n")

	resolver := inputset.NewWorkspaceResolver(root, root, nil)
	set, err := CollectInvocationInputs([]string{
		"update",
		"--defaults-file", "defaults.properties",
	}, resolver, inputset.OSFileSystem{})
	if err != nil {
		t.Fatalf("CollectInvocationInputs: %v", err)
	}
	if len(set.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %+v", set.Entries)
	}
	if set.Entries[0].Path != "defaults.properties" {
		t.Fatalf("entry[0] = %+v", set.Entries[0])
	}
}

func TestCollectInvocationInputsRejectsExternalResources(t *testing.T) {
	cases := []struct {
		name    string
		args    []string
		content string
	}{
		{name: "changelog", args: []string{"update", "--changelog-file", "https://example.com/changelog.xml"}},
		{name: "defaults", args: []string{"update", "--defaults-file", "https://example.com/liquibase.properties"}},
		{name: "search path", args: []string{"update", "--searchPath", "classpath:db"}},
		{name: "include", args: []string{"update", "--changelog-file", "master.xml"}, content: `<databaseChangeLog><include file="classpath:db/child.xml"/></databaseChangeLog>`},
		{name: "include all", args: []string{"update", "--changelog-file", "master.xml"}, content: `<databaseChangeLog><includeAll path="https://example.com/db"/></databaseChangeLog>`},
		{name: "sql file", args: []string{"update", "--changelog-file", "master.xml"}, content: `<databaseChangeLog><changeSet id="1" author="dev"><sqlFile path="https://example.com/seed.sql"/></changeSet></databaseChangeLog>`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			if tc.content != "" {
				writeFile(t, filepath.Join(root, "master.xml"), tc.content)
			}
			resolver := inputset.NewWorkspaceResolver(root, root, nil)
			if _, err := CollectInvocationInputs(tc.args, resolver, inputset.OSFileSystem{}); err == nil {
				t.Fatal("external resource must fail before cache lookup")
			}
		})
	}
}

// TestCollectInvocationInputsRejectsUnclassifiedChangelogs verifies B02c in
// docs/architecture/liquibase-container-tests.md before cache lookup.
func TestCollectInvocationInputsRejectsUnclassifiedChangelogs(t *testing.T) {
	for _, tc := range []struct {
		name    string
		file    string
		content string
	}{
		{name: "unsupported extension", file: "master.txt", content: "select 1;\n"},
		{name: "plain SQL", file: "master.sql", content: "select 1;\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, filepath.Join(root, tc.file), tc.content)
			resolver := inputset.NewWorkspaceResolver(root, root, nil)
			if _, err := CollectInvocationInputs([]string{"update", "--changelog-file", tc.file}, resolver, inputset.OSFileSystem{}); err == nil {
				t.Fatalf("unclassified changelog %q entered input set", tc.file)
			}
		})
	}
}

// IncludeAll must not silently omit an unclassified file from the input set.
func TestCollectInvocationInputsRejectsUnclassifiedIncludeAllFile(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "master.xml"), `<databaseChangeLog><includeAll path="changes"/></databaseChangeLog>`)
	writeFile(t, filepath.Join(root, "changes", "unclassified.txt"), "select 1;\n")
	resolver := inputset.NewWorkspaceResolver(root, root, nil)
	if _, err := CollectInvocationInputs([]string{"update", "--changelog-file", "master.xml"}, resolver, inputset.OSFileSystem{}); err == nil {
		t.Fatal("includeAll silently omitted an unclassified file")
	}
}

// TestCollectInvocationInputsPreservesMixedIncludeOrder verifies B01a in
// docs/architecture/liquibase-container-tests.md using independently computed
// paths and content hashes, including the order of includeAll and include.
func TestCollectInvocationInputsPreservesMixedIncludeOrder(t *testing.T) {
	root := t.TempDir()
	content := map[string]string{
		"master.xml":    `<databaseChangeLog><includeAll path="changes"/><include file="child.xml"/></databaseChangeLog>`,
		"changes/a.xml": `<databaseChangeLog><!-- a --></databaseChangeLog>`,
		"child.xml":     `<databaseChangeLog><!-- child --></databaseChangeLog>`,
	}
	for path, body := range content {
		writeFile(t, filepath.Join(root, filepath.FromSlash(path)), body)
	}
	resolver := inputset.NewWorkspaceResolver(root, root, nil)
	set, err := CollectInvocationInputs([]string{"update", "--changelog-file", "master.xml"}, resolver, inputset.OSFileSystem{})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"master.xml", "changes/a.xml", "child.xml"}
	if len(set.Entries) != len(want) {
		t.Fatalf("input count=%d, want %d: %+v", len(set.Entries), len(want), set.Entries)
	}
	for i, path := range want {
		entry := set.Entries[i]
		sum := sha256.Sum256([]byte(content[path]))
		if entry.Path != path || entry.Hash != hex.EncodeToString(sum[:]) {
			t.Fatalf("entry[%d]=%+v, want path=%q hash=%x", i, entry, path, sum)
		}
	}
}

// TestCollectInvocationInputsFollowsDefaultsPaths verifies B01: a defaults
// file is an input itself, and its changelog and search path participate in
// the same dependency closure as command-line paths.
func TestCollectInvocationInputsFollowsDefaultsPaths(t *testing.T) {
	for _, tc := range []struct {
		name     string
		args     []string
		defaults string
	}{
		{name: "explicit", args: []string{"update", "--defaults-file", "settings/liquibase.properties"}, defaults: "liquibase.changelogFile=master.xml\nliquibase.searchPath=changes\n"},
		{name: "automatic", args: []string{"update"}, defaults: "liquibase.changelogFile=master.xml\nliquibase.searchPath=changes\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			defaultsPath := "liquibase.properties"
			if tc.name == "explicit" {
				defaultsPath = "settings/liquibase.properties"
			}
			writeFile(t, filepath.Join(root, filepath.FromSlash(defaultsPath)), tc.defaults)
			writeFile(t, filepath.Join(root, "changes", "master.xml"), `<databaseChangeLog><include file="child.xml" relativeToChangelogFile="true"/></databaseChangeLog>`)
			writeFile(t, filepath.Join(root, "changes", "child.xml"), `<databaseChangeLog/>`)
			resolver := inputset.NewWorkspaceResolver(root, root, nil)
			set, err := CollectInvocationInputs(tc.args, resolver, inputset.OSFileSystem{})
			if err != nil {
				t.Fatal(err)
			}
			want := []string{defaultsPath, "changes/master.xml", "changes/child.xml"}
			if len(set.Entries) != len(want) {
				t.Fatalf("entries=%+v, want %v", set.Entries, want)
			}
			for i, path := range want {
				if set.Entries[i].Path != path {
					t.Fatalf("entry[%d]=%q, want %q", i, set.Entries[i].Path, path)
				}
			}
		})
	}
}

// TestCollectInvocationInputsRejectsUnresolvedAndEscapingPaths verifies B02:
// unsupported placeholders and paths outside the admitted root are errors.
func TestCollectInvocationInputsRejectsUnresolvedAndEscapingPaths(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
	}{
		{name: "unresolved variable", content: `<databaseChangeLog><include file="${migrationFile}"/></databaseChangeLog>`},
		{name: "parent traversal", content: `<databaseChangeLog><include file="../outside.xml"/></databaseChangeLog>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			parent := t.TempDir()
			root := filepath.Join(parent, "workspace")
			writeFile(t, filepath.Join(root, "master.xml"), tc.content)
			if tc.name == "unresolved variable" {
				// A literal file with this name must not disguise an unresolved
				// Liquibase property substitution as a complete closure.
				writeFile(t, filepath.Join(root, "${migrationFile}"), `<databaseChangeLog/>`)
			}
			if tc.name == "parent traversal" {
				writeFile(t, filepath.Join(parent, "outside.xml"), `<databaseChangeLog/>`)
			}
			resolver := inputset.NewWorkspaceResolver(root, root, nil)
			if _, err := CollectInvocationInputs([]string{"update", "--changelog-file", "master.xml"}, resolver, inputset.OSFileSystem{}); err == nil {
				t.Fatalf("%s must be rejected before cache lookup", tc.name)
			}
		})
	}
}

// An XML document that parses successfully is not necessarily a supported
// Liquibase changelog. Unknown resource loaders also make the input closure
// incomplete even when the referenced class can be found at runtime.
func TestCollectInvocationInputsRejectsUnknownChangelogSemantics(t *testing.T) {
	for _, tc := range []struct {
		name    string
		content string
	}{
		{name: "wrong root", content: `<notAChangeLog/>`},
		{name: "custom resource loader", content: `<databaseChangeLog><changeSet id="one" author="dev"><customChange class="example.CustomChange"/></changeSet></databaseChangeLog>`},
	} {
		t.Run(tc.name, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, filepath.Join(root, "master.xml"), tc.content)
			resolver := inputset.NewWorkspaceResolver(root, root, nil)
			if _, err := CollectInvocationInputs([]string{"update", "--changelog-file", "master.xml"}, resolver, inputset.OSFileSystem{}); err == nil {
				t.Fatalf("%s was accepted as a complete changelog closure", tc.name)
			}
		})
	}
}

// Community Liquibase does not support include directives inside formatted
// SQL root changelogs. They must fail closed rather than disappear from the
// manifest while the engine reports a cache hit.
func TestCollectInvocationInputsRejectsFormattedSQLIncludes(t *testing.T) {
	for _, directive := range []string{"--include file:child.xml", "--includeAll path:changes"} {
		t.Run(directive, func(t *testing.T) {
			root := t.TempDir()
			writeFile(t, filepath.Join(root, "master.sql"), "--liquibase formatted sql\n"+directive+"\n")
			writeFile(t, filepath.Join(root, "child.xml"), `<databaseChangeLog/>`)
			writeFile(t, filepath.Join(root, "changes", "one.xml"), `<databaseChangeLog/>`)
			resolver := inputset.NewWorkspaceResolver(root, root, nil)
			if _, err := CollectInvocationInputs([]string{"update", "--changelog-file", "master.sql"}, resolver, inputset.OSFileSystem{}); err == nil {
				t.Fatalf("unsupported formatted SQL directive %q was silently accepted", directive)
			}
		})
	}
}

type changingChangelogFS struct {
	root  string
	reads int
}

func (f *changingChangelogFS) Stat(path string) (fs.FileInfo, error) {
	return os.Stat(path)
}

func (f *changingChangelogFS) ReadDir(path string) ([]fs.DirEntry, error) {
	return os.ReadDir(path)
}

func (f *changingChangelogFS) ReadFile(path string) ([]byte, error) {
	if filepath.Clean(path) == filepath.Join(f.root, "master.xml") {
		f.reads++
		if f.reads == 1 {
			return []byte(`<databaseChangeLog><include file="child.xml"/></databaseChangeLog>`), nil
		}
		return []byte(`<databaseChangeLog/>`), nil
	}
	return os.ReadFile(path)
}

// B02: the hash must describe the bytes actually parsed to discover child
// resources; a changed file cannot publish a self-inconsistent input list.
func TestCollectInvocationInputsRejectsChangelogChangeDuringCapture(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "master.xml"), `<databaseChangeLog/>`)
	writeFile(t, filepath.Join(root, "child.xml"), `<databaseChangeLog/>`)
	resolver := inputset.NewWorkspaceResolver(root, root, nil)
	fs := &changingChangelogFS{root: root}
	if _, err := CollectInvocationInputs([]string{"update", "--changelog-file", "master.xml"}, resolver, fs); err == nil {
		t.Fatal("changed changelog produced an inconsistent manifest")
	}
	if fs.reads < 2 {
		t.Fatalf("changelog was not independently checked after parsing: reads=%d", fs.reads)
	}
}

type changingDefaultsFS struct {
	reads int
}

func (f *changingDefaultsFS) Stat(path string) (fs.FileInfo, error)      { return os.Stat(path) }
func (f *changingDefaultsFS) ReadDir(path string) ([]fs.DirEntry, error) { return os.ReadDir(path) }
func (f *changingDefaultsFS) ReadFile(path string) ([]byte, error) {
	if filepath.Base(path) == "liquibase.properties" {
		f.reads++
		if f.reads == 1 {
			return []byte("liquibase.changelogFile=master.xml\n"), nil
		}
		return []byte("x=y\n"), nil
	}
	return os.ReadFile(path)
}

func TestCollectInvocationInputsRejectsDefaultsChangeDuringCapture(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "liquibase.properties"), "x=y\n")
	writeFile(t, filepath.Join(root, "master.xml"), `<databaseChangeLog/>`)
	fs := &changingDefaultsFS{}
	if _, err := CollectInvocationInputs([]string{"update"}, inputset.NewWorkspaceResolver(root, root, nil), fs); err == nil {
		t.Fatal("changed defaults file produced an inconsistent manifest")
	}
	if fs.reads < 2 {
		t.Fatalf("settings file was not checked after parsing: reads=%d", fs.reads)
	}
}

func TestCollectInvocationInputsIncludesSecondaryLocalAssets(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "changelog", "master.xml"), strings.Join([]string{
		`<databaseChangeLog xmlns="http://www.liquibase.org/xml/ns/dbchangelog">`,
		`  <changeSet id="1" author="dev">`,
		`    <sqlFile path="seed.sql" relativeToChangelogFile="true"/>`,
		`    <loadData file="seed.csv" relativeToChangelogFile="true"/>`,
		`  </changeSet>`,
		`</databaseChangeLog>`,
	}, "\n"))
	writeFile(t, filepath.Join(root, "changelog", "seed.sql"), "select 1;\n")
	writeFile(t, filepath.Join(root, "changelog", "seed.csv"), "id,name\n1,test\n")

	resolver := inputset.NewWorkspaceResolver(root, root, nil)
	set, err := CollectInvocationInputs([]string{
		"update",
		"--changelog-file", "changelog/master.xml",
	}, resolver, inputset.OSFileSystem{})
	if err != nil {
		t.Fatalf("CollectInvocationInputs: %v", err)
	}

	if len(set.Entries) != 3 {
		t.Fatalf("expected 3 entries, got %+v", set.Entries)
	}
	if set.Entries[0].Path != filepath.ToSlash("changelog/master.xml") {
		t.Fatalf("entry[0] = %+v", set.Entries[0])
	}
	if set.Entries[1].Path != filepath.ToSlash("changelog/seed.sql") {
		t.Fatalf("entry[1] = %+v", set.Entries[1])
	}
	if set.Entries[2].Path != filepath.ToSlash("changelog/seed.csv") {
		t.Fatalf("entry[2] = %+v", set.Entries[2])
	}
}

func TestValidateArgsAccumulatesLiquibaseIssues(t *testing.T) {
	root := t.TempDir()
	aliasPath := filepath.Join(root, "aliases", "demo.prep.s9s.yaml")
	if err := os.MkdirAll(filepath.Join(filepath.Dir(aliasPath), "migrations"), 0o700); err != nil {
		t.Fatalf("MkdirAll migrations: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(filepath.Dir(aliasPath), "shared"), 0o700); err != nil {
		t.Fatalf("MkdirAll shared: %v", err)
	}
	resolver := inputset.NewAliasResolver(root, aliasPath)

	issues := ValidateArgs([]string{
		"--defaults-file=missing.properties",
		"--searchPath=",
		"--searchPath", "migrations,,shared",
	}, resolver, inputset.OSFileSystem{})
	if len(issues) != 3 {
		t.Fatalf("expected 3 issues, got %+v", issues)
	}
	if issues[0].Code != "missing_path" || issues[1].Code != "empty_search_path" || issues[2].Code != "empty_search_path_item" {
		t.Fatalf("unexpected issues: %+v", issues)
	}
}

func writeFile(t *testing.T, path string, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
}
