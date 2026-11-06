package diff

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestBuildLbFileList_SingleChangelog(t *testing.T) {
	dir := t.TempDir()
	changelog := filepath.Join(dir, "changelog.xml")
	if err := os.WriteFile(changelog, []byte(`<databaseChangeLog xmlns="http://www.liquibase.org/xml/ns/dbchangelog"/>`), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx := Context{Root: dir}
	list, err := BuildLbFileList(ctx, []string{"--changelog-file", "changelog.xml", "update"})
	if err != nil {
		t.Fatalf("BuildLbFileList: %v", err)
	}
	if len(list.Entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(list.Entries))
	}
	if list.Entries[0].Path != "changelog.xml" {
		t.Fatalf("expected path changelog.xml, got %q", list.Entries[0].Path)
	}
}

func TestBuildLbFileList_WithInclude(t *testing.T) {
	dir := t.TempDir()
	sub := filepath.Join(dir, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	inc := filepath.Join(sub, "001.xml")
	if err := os.WriteFile(inc, []byte(`<databaseChangeLog xmlns="http://www.liquibase.org/xml/ns/dbchangelog"/>`), 0o600); err != nil {
		t.Fatal(err)
	}
	changelog := filepath.Join(dir, "master.xml")
	if err := os.WriteFile(changelog, []byte(`<databaseChangeLog xmlns="http://www.liquibase.org/xml/ns/dbchangelog">
  <include file="sub/001.xml"/>
</databaseChangeLog>`), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx := Context{Root: dir}
	list, err := BuildLbFileList(ctx, []string{"--changelog-file", "master.xml"})
	if err != nil {
		t.Fatalf("BuildLbFileList: %v", err)
	}
	if len(list.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(list.Entries))
	}
}

func TestBuildLbFileList_NoChangelogFile(t *testing.T) {
	ctx := Context{Root: t.TempDir()}
	_, err := BuildLbFileList(ctx, []string{"update"})
	if err == nil || err.Error() != "liquibase command has no --changelog-file (required for diff)" {
		t.Fatalf("expected error about --changelog-file, got %v", err)
	}
}

// JHipster-style: master under config/liquibase/ includes paths from context root
// with relativeToChangelogFile="false" (not relative to master’s directory).
func TestBuildLbFileList_MalformedMasterXML(t *testing.T) {
	dir := t.TempDir()
	master := filepath.Join(dir, "master.xml")
	if err := os.WriteFile(master, []byte(`<databaseChangeLog xmlns="http://www.liquibase.org/xml/ns/dbchangelog">
  <include file="other.xml"></include>
  not valid xml`), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx := Context{Root: dir}
	_, err := BuildLbFileList(ctx, []string{"--changelog-file", "master.xml"})
	if err == nil {
		t.Fatal("expected error for malformed XML changelog")
	}
	if !strings.Contains(err.Error(), "parse liquibase changelog") {
		t.Fatalf("expected parse error, got %v", err)
	}
}

func TestBuildLbFileList_SQLLeafIncludedWithoutXMLParse(t *testing.T) {
	// includeAll picks .sql files; they must not require XML parse.
	dir := t.TempDir()
	changelogDir := filepath.Join(dir, "ch")
	if err := os.MkdirAll(changelogDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(changelogDir, "a.sql"), []byte("--liquibase formatted sql\n--changeset test:1\nSELECT 1;\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	master := filepath.Join(dir, "master.xml")
	if err := os.WriteFile(master, []byte(`<databaseChangeLog xmlns="http://www.liquibase.org/xml/ns/dbchangelog">
  <includeAll path="ch"/>
</databaseChangeLog>`), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx := Context{Root: dir}
	list, err := BuildLbFileList(ctx, []string{"--changelog-file", "master.xml"})
	if err != nil {
		t.Fatalf("BuildLbFileList: %v", err)
	}
	if len(list.Entries) != 2 {
		t.Fatalf("expected master + a.sql, got %d entries", len(list.Entries))
	}
}

func TestBuildLbFileList_IncludeRelativeToChangelogFileFalse(t *testing.T) {
	dir := t.TempDir()
	lbDir := filepath.Join(dir, "config", "liquibase")
	chDir := filepath.Join(lbDir, "changelog")
	if err := os.MkdirAll(chDir, 0o755); err != nil {
		t.Fatal(err)
	}
	inc := filepath.Join(chDir, "00000000000001.xml")
	if err := os.WriteFile(inc, []byte(`<databaseChangeLog xmlns="http://www.liquibase.org/xml/ns/dbchangelog"/>`), 0o600); err != nil {
		t.Fatal(err)
	}
	master := filepath.Join(lbDir, "master.xml")
	if err := os.WriteFile(master, []byte(`<databaseChangeLog xmlns="http://www.liquibase.org/xml/ns/dbchangelog">
  <include file="config/liquibase/changelog/00000000000001.xml" relativeToChangelogFile="false"/>
</databaseChangeLog>`), 0o600); err != nil {
		t.Fatal(err)
	}
	ctx := Context{Root: dir}
	list, err := BuildLbFileList(ctx, []string{"--changelog-file", "config/liquibase/master.xml"})
	if err != nil {
		t.Fatalf("BuildLbFileList: %v", err)
	}
	if len(list.Entries) != 2 {
		t.Fatalf("expected 2 entries (master + included), got %d: %+v", len(list.Entries), list.Entries)
	}
	if list.Entries[0].Path != filepath.ToSlash("config/liquibase/master.xml") {
		t.Fatalf("unexpected first path: %q", list.Entries[0].Path)
	}
	wantChild := filepath.ToSlash("config/liquibase/changelog/00000000000001.xml")
	if list.Entries[1].Path != wantChild {
		t.Fatalf("expected second path %q, got %q", wantChild, list.Entries[1].Path)
	}
}

func TestBuildLbFileList_BlobRefMode(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not in PATH")
	}
	emptyTemplate := t.TempDir()
	repo := t.TempDir()
	initCmd := exec.Command("git", "-C", repo, "init", "--template", emptyTemplate)
	if out, err := initCmd.CombinedOutput(); err != nil {
		t.Skipf("git init: %v\n%s", err, out)
	}
	runGit := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", repo}, args...)...)
		if out, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("git %v: %v\n%s", args, err, out)
		}
	}
	runGit("config", "user.email", "t@e.st")
	runGit("config", "user.name", "t")
	if err := os.MkdirAll(filepath.Join(repo, "ch"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(repo, "ch", "001.sql"), []byte("--liquibase formatted sql\n--changeset test:1\nselect 1;\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	master := filepath.Join(repo, "master.xml")
	if err := os.WriteFile(master, []byte(`<databaseChangeLog xmlns="http://www.liquibase.org/xml/ns/dbchangelog">
  <includeAll path="ch"/>
</databaseChangeLog>`), 0o600); err != nil {
		t.Fatal(err)
	}
	runGit("add", "master.xml", "ch/001.sql")
	runGit("commit", "-m", "first")

	ctx := Context{Root: repo, BaseDir: repo, GitRef: "HEAD"}
	list, err := BuildLbFileList(ctx, []string{"--changelog-file", "master.xml"})
	if err != nil {
		t.Fatalf("BuildLbFileList: %v", err)
	}
	if len(list.Entries) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(list.Entries))
	}
	if list.Entries[0].Path != "master.xml" || list.Entries[1].Path != "ch/001.sql" {
		t.Fatalf("unexpected entries: %+v", list.Entries)
	}
}
