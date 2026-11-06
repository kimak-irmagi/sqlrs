package discover

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/sqlrs/cli/internal/alias"
)

func TestAnalyzeAliasesRanksRootAndBuildsCopyPasteCommand(t *testing.T) {
	workspace := t.TempDir()
	mustWriteFile(t, filepath.Join(workspace, "schema.sql"), []byte("create table users(id int);\n\\i child.sql\n"))
	mustWriteFile(t, filepath.Join(workspace, "child.sql"), []byte("select 1;\n"))

	report, err := AnalyzeAliases(Options{WorkspaceRoot: workspace, CWD: workspace})
	if err != nil {
		t.Fatalf("AnalyzeAliases: %v", err)
	}
	if report.Suppressed != 1 {
		t.Fatalf("expected one suppressed child candidate, got %+v", report)
	}
	if len(report.Findings) != 1 {
		t.Fatalf("expected one root finding, got %+v", report)
	}

	finding := report.Findings[0]
	if finding.Type != alias.ClassPrepare || finding.Kind != "psql" {
		t.Fatalf("unexpected finding: %+v", finding)
	}
	if finding.Ref != "schema" {
		t.Fatalf("unexpected ref: %+v", finding)
	}
	if finding.AliasPath != "schema.prep.s9s.yaml" {
		t.Fatalf("unexpected alias path: %+v", finding)
	}
	if !strings.Contains(finding.CreateCommand, "sqlrs alias create schema prepare:psql -- -f schema.sql") {
		t.Fatalf("unexpected create command: %+v", finding)
	}
}

func TestAnalyzeAliasesReportsGenericMetadata(t *testing.T) {
	workspace := t.TempDir()
	mustWriteFile(t, filepath.Join(workspace, "schema.sql"), []byte("create table users(id int);\n"))

	report, err := AnalyzeAliases(Options{
		WorkspaceRoot: workspace,
		CWD:           workspace,
		ShellFamily:   ShellFamilyPowerShell,
	})
	if err != nil {
		t.Fatalf("AnalyzeAliases: %v", err)
	}
	if got := strings.Join(report.SelectedAnalyzers, ","); got != AnalyzerAliases {
		t.Fatalf("unexpected selected analyzers: %q", got)
	}
	if len(report.Summaries) != 1 || report.Summaries[0].Analyzer != AnalyzerAliases || report.Summaries[0].Findings != 1 {
		t.Fatalf("unexpected summaries: %+v", report.Summaries)
	}
	if len(report.Findings) != 1 {
		t.Fatalf("expected one finding, got %+v", report)
	}
	finding := report.Findings[0]
	if finding.Analyzer != AnalyzerAliases || finding.Target != "schema.sql" || finding.Action == "" {
		t.Fatalf("unexpected generic finding metadata: %+v", finding)
	}
	if finding.FollowUpCommand == nil || finding.FollowUpCommand.ShellFamily != ShellFamilyPowerShell {
		t.Fatalf("expected shell-aware follow-up command: %+v", finding)
	}
}

func TestAnalyzeAliasesUsesStableRelativePathsThroughSymlinkedWorkspace(t *testing.T) {
	root := t.TempDir()
	realRoot := filepath.Join(root, "real")
	linkRoot := filepath.Join(root, "link")
	workspace := filepath.Join(realRoot, "workspace")
	mustWriteFile(t, filepath.Join(workspace, "schema.sql"), []byte("create table users(id int);\n"))
	if err := os.Symlink(realRoot, linkRoot); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	record, ok := classifyDiscoverFile(filepath.Join(linkRoot, "workspace"), workspace, filepath.Join(workspace, "schema.sql"))
	if !ok {
		t.Fatalf("expected discover file record")
	}
	if record.WorkspaceRel != "schema.sql" {
		t.Fatalf("WorkspaceRel = %q, want %q", record.WorkspaceRel, "schema.sql")
	}
	if record.CwdRel != "schema.sql" {
		t.Fatalf("CwdRel = %q, want %q", record.CwdRel, "schema.sql")
	}

	proposal, ok := proposeCandidate(record)
	if !ok {
		t.Fatalf("expected candidate proposal")
	}
	if proposal.Ref != "schema" {
		t.Fatalf("Ref = %q, want %q", proposal.Ref, "schema")
	}
}

func TestAnalyzeAliasesSuppressesInboundDependencyThroughSymlinkedWorkspace(t *testing.T) {
	root := t.TempDir()
	realRoot := filepath.Join(root, "real")
	linkRoot := filepath.Join(root, "link")
	workspace := filepath.Join(realRoot, "workspace")
	mustWriteFile(t, filepath.Join(workspace, "schema.sql"), []byte("create table users(id int);\n\\i child.sql\n"))
	mustWriteFile(t, filepath.Join(workspace, "child.sql"), []byte("select 1;\n"))
	if err := os.Symlink(realRoot, linkRoot); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}

	symlinkedWorkspace := filepath.Join(linkRoot, "workspace")
	report, err := AnalyzeAliases(Options{WorkspaceRoot: symlinkedWorkspace, CWD: symlinkedWorkspace})
	if err != nil {
		t.Fatalf("AnalyzeAliases: %v", err)
	}
	if report.Suppressed != 1 {
		t.Fatalf("expected child candidate to be suppressed, got %+v", report)
	}
	if len(report.Findings) != 1 {
		t.Fatalf("expected only the root finding to remain, got %+v", report)
	}

	finding := report.Findings[0]
	if finding.File != "schema.sql" {
		t.Fatalf("unexpected root finding: %+v", finding)
	}
}

func TestShellQuoteForGoOS(t *testing.T) {
	tests := []struct {
		name  string
		goos  string
		value string
		want  string
	}{
		{
			name:  "windows apostrophe",
			goos:  "windows",
			value: "O'Brien",
			want:  "'O''Brien'",
		},
		{
			name:  "windows comma",
			goos:  "windows",
			value: "liquibase/jhipster-sample-app,shared",
			want:  "'liquibase/jhipster-sample-app,shared'",
		},
		{
			name:  "posix apostrophe",
			goos:  "linux",
			value: "O'Brien",
			want:  `'O'"'"'Brien'`,
		},
		{
			name:  "posix comma",
			goos:  "linux",
			value: "liquibase/jhipster-sample-app,shared",
			want:  "liquibase/jhipster-sample-app,shared",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := shellQuoteForGoOS(tt.goos, tt.value); got != tt.want {
				t.Fatalf("unexpected shell quote: got %q want %q", got, tt.want)
			}
		})
	}
}

func TestShellJoinUsesCurrentShellQuoting(t *testing.T) {
	got := shellJoin([]string{"sqlrs", "alias", "create", "O'Brien", "prepare:psql", "--", "-f", "O'Brien.sql"})
	want := `sqlrs alias create 'O''Brien' prepare:psql -- -f 'O''Brien.sql'`
	if runtime.GOOS != "windows" {
		want = `sqlrs alias create 'O'"'"'Brien' prepare:psql -- -f 'O'"'"'Brien.sql'`
	}
	if got != want {
		t.Fatalf("unexpected shell join: got %q want %q", got, want)
	}
}

func TestShellJoinQuotesCommaSeparatedLiquibaseSearchPath(t *testing.T) {
	got := shellJoin([]string{
		"sqlrs", "alias", "create", "liquibase", "prepare:lb", "--",
		"update", "--searchPath", "liquibase/jhipster-sample-app,shared",
		"--changelog-file", "liquibase/jhipster-sample-app/config/liquibase/master.xml",
	})
	if runtime.GOOS == "windows" {
		if !strings.Contains(got, "'liquibase/jhipster-sample-app,shared'") {
			t.Fatalf("expected comma-separated searchPath to be quoted, got %q", got)
		}
		return
	}
	if strings.Contains(got, "'liquibase/jhipster-sample-app,shared'") {
		t.Fatalf("expected POSIX shell to keep comma-separated searchPath bare, got %q", got)
	}
	if !strings.Contains(got, "liquibase/jhipster-sample-app,shared") {
		t.Fatalf("expected comma-separated searchPath to be preserved, got %q", got)
	}
}

func TestScorePrepareLiquibaseRecognizesRelativeToChangelogFile(t *testing.T) {
	workspace := t.TempDir()
	got := scorePrepareLiquibase(fileRecord{
		AbsPath:       filepath.Join(workspace, "db", "root.xml"),
		WorkspaceRoot: workspace,
		WorkspaceRel:  filepath.ToSlash(filepath.Join("db", "root.xml")),
		CwdRel:        filepath.ToSlash(filepath.Join("db", "root.xml")),
		Ext:           ".xml",
		LowerPath:     "db/root.xml",
		Content:       `relativetochangelogfile true`,
	})
	if got.Score != 30 {
		t.Fatalf("expected lowercase relativeToChangelogFile signal to score 30, got %+v", got)
	}
}

func TestAnalyzeAliasesUsesAncestorRefForParentDirectoryCandidate(t *testing.T) {
	workspace := t.TempDir()
	mustWriteFile(t, filepath.Join(workspace, "schema.sql"), []byte("create table users(id int);\n"))

	cwd := filepath.Join(workspace, "nested")
	report, err := AnalyzeAliases(Options{WorkspaceRoot: workspace, CWD: cwd})
	if err != nil {
		t.Fatalf("AnalyzeAliases: %v", err)
	}
	if len(report.Findings) != 1 {
		t.Fatalf("expected one root finding, got %+v", report)
	}

	finding := report.Findings[0]
	if finding.Ref != "../schema" {
		t.Fatalf("unexpected ref: %+v", finding)
	}
	if finding.AliasPath != "schema.prep.s9s.yaml" {
		t.Fatalf("unexpected alias path: %+v", finding)
	}
	if !strings.Contains(finding.CreateCommand, "sqlrs alias create ../schema prepare:psql -- -f ../schema.sql") {
		t.Fatalf("unexpected create command: %+v", finding)
	}
}

func TestAnalyzeAliasesKeepsAbsoluteSourcePathWhenCwdRelFails(t *testing.T) {
	if runtime.GOOS != "windows" {
		t.Skip("cross-drive fallback is Windows-specific")
	}

	workspace := t.TempDir()
	mustWriteFile(t, filepath.Join(workspace, "schema.sql"), []byte("create table users(id int);\n"))

	report, err := AnalyzeAliases(Options{
		WorkspaceRoot: workspace,
		CWD:           `D:\discover-cwd`,
	})
	if err != nil {
		t.Fatalf("AnalyzeAliases: %v", err)
	}
	if len(report.Findings) != 1 {
		t.Fatalf("expected one root finding, got %+v", report)
	}

	finding := report.Findings[0]
	if !strings.HasPrefix(finding.Ref, filepath.ToSlash(filepath.Clean(workspace))+string('/')) {
		t.Fatalf("unexpected absolute ref: %+v", finding)
	}
	if finding.AliasPath != "schema.prep.s9s.yaml" {
		t.Fatalf("unexpected alias path: %+v", finding)
	}
	if !strings.Contains(finding.CreateCommand, filepath.ToSlash(filepath.Join(workspace, "schema.sql"))) {
		t.Fatalf("expected absolute source path in create command, got %+v", finding)
	}
}

func TestAnalyzeAliasesSuppressesCoveredAlias(t *testing.T) {
	workspace := t.TempDir()
	mustWriteFile(t, filepath.Join(workspace, "schema.sql"), []byte("create table users(id int);\n"))
	mustWriteFile(t, filepath.Join(workspace, "schema.prep.s9s.yaml"), []byte("kind: psql\nargs:\n  - -f\n  - schema.sql\n"))

	report, err := AnalyzeAliases(Options{WorkspaceRoot: workspace, CWD: workspace})
	if err != nil {
		t.Fatalf("AnalyzeAliases: %v", err)
	}
	if report.Suppressed != 1 {
		t.Fatalf("expected covered candidate to be suppressed, got %+v", report)
	}
	if len(report.Findings) != 0 {
		t.Fatalf("expected no findings for covered alias, got %+v", report)
	}
}

func TestAnalyzeAliasesSkipsUnsupportedExtensions(t *testing.T) {
	workspace := t.TempDir()
	mustWriteFile(t, filepath.Join(workspace, "README.md"), []byte("create table users(id int);\nselect 1;\n"))
	mustWriteFile(t, filepath.Join(workspace, "run.sh"), []byte("select 1;\n"))
	mustWriteFile(t, filepath.Join(workspace, "notes.txt"), []byte("create table users(id int);\n"))
	mustWriteFile(t, filepath.Join(workspace, "schema.sql"), []byte("create table users(id int);\n"))

	report, err := AnalyzeAliases(Options{WorkspaceRoot: workspace, CWD: workspace})
	if err != nil {
		t.Fatalf("AnalyzeAliases: %v", err)
	}
	if report.Prefiltered != 1 {
		t.Fatalf("expected one supported candidate, got %+v", report)
	}
	if report.Validated != 1 {
		t.Fatalf("expected one validated candidate, got %+v", report)
	}
	if len(report.Findings) != 1 {
		t.Fatalf("expected one finding, got %+v", report)
	}

	finding := report.Findings[0]
	if finding.File != "schema.sql" {
		t.Fatalf("unexpected finding file: %+v", finding)
	}
	if finding.Type != alias.ClassPrepare || finding.Kind != "psql" {
		t.Fatalf("unexpected finding kind: %+v", finding)
	}
}

func TestAnalyzeAliasesSuppressesLiquibaseClosureCoveredByAlias(t *testing.T) {
	workspace := t.TempDir()
	mustWriteFile(t, filepath.Join(workspace, "liquibase", "jhipster-sample-app.prep.s9s.yaml"), []byte(`kind: lb
args:
  - update
  - --searchPath
  - liquibase/jhipster-sample-app
  - --changelog-file
  - liquibase/jhipster-sample-app/config/liquibase/master.xml
`))
	mustWriteFile(t, filepath.Join(workspace, "liquibase", "jhipster-sample-app", "config", "liquibase", "master.xml"), []byte(`<?xml version="1.0" encoding="UTF-8"?>
<databaseChangeLog xmlns="http://www.liquibase.org/xml/ns/dbchangelog"
    xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
    <include file="config/liquibase/changelog/00000000000000_initial_schema.xml" relativeToChangelogFile="false"/>
</databaseChangeLog>
`))
	mustWriteFile(t, filepath.Join(workspace, "liquibase", "jhipster-sample-app", "config", "liquibase", "changelog", "00000000000000_initial_schema.xml"), []byte(`<?xml version="1.0" encoding="UTF-8"?>
<databaseChangeLog xmlns="http://www.liquibase.org/xml/ns/dbchangelog"></databaseChangeLog>
`))

	report, err := AnalyzeAliases(Options{WorkspaceRoot: workspace, CWD: workspace})
	if err != nil {
		t.Fatalf("AnalyzeAliases: %v", err)
	}
	if report.Suppressed != 2 {
		t.Fatalf("expected master and child candidates to be suppressed, got %+v", report)
	}
	if len(report.Findings) != 0 {
		t.Fatalf("expected no findings for covered liquibase tree, got %+v", report)
	}
}

func TestAnalyzeAliasesRecognizesLiquibaseProjectRoot(t *testing.T) {
	workspace := t.TempDir()
	mustWriteFile(t, filepath.Join(workspace, "liquibase", "jhipster-sample-app", "config", "liquibase", "master.xml"), []byte(`<?xml version="1.0" encoding="UTF-8"?>
<databaseChangeLog xmlns="http://www.liquibase.org/xml/ns/dbchangelog"
    xmlns:xsi="http://www.w3.org/2001/XMLSchema-instance">
    <include file="config/liquibase/changelog/00000000000000_initial_schema.xml" relativeToChangelogFile="false"/>
</databaseChangeLog>
`))
	mustWriteFile(t, filepath.Join(workspace, "liquibase", "jhipster-sample-app", "config", "liquibase", "changelog", "00000000000000_initial_schema.xml"), []byte(`<?xml version="1.0" encoding="UTF-8"?>
<databaseChangeLog xmlns="http://www.liquibase.org/xml/ns/dbchangelog"></databaseChangeLog>
`))

	report, err := AnalyzeAliases(Options{WorkspaceRoot: workspace, CWD: workspace})
	if err != nil {
		t.Fatalf("AnalyzeAliases: %v", err)
	}
	if report.Suppressed != 1 {
		t.Fatalf("expected child changelog to be suppressed by topology, got %+v", report)
	}
	if len(report.Findings) != 1 {
		t.Fatalf("expected one liquibase root finding, got %+v", report)
	}

	finding := report.Findings[0]
	if finding.Type != alias.ClassPrepare || finding.Kind != "lb" {
		t.Fatalf("unexpected liquibase finding: %+v", finding)
	}
	if finding.Ref != "liquibase/jhipster-sample-app" {
		t.Fatalf("unexpected liquibase ref: %+v", finding)
	}
	if finding.AliasPath != filepath.ToSlash(filepath.Join("liquibase", "jhipster-sample-app.prep.s9s.yaml")) {
		t.Fatalf("unexpected liquibase alias path: %+v", finding)
	}
	if !finding.Valid {
		t.Fatalf("expected liquibase finding to be valid: %+v", finding)
	}
	if !strings.Contains(finding.CreateCommand, "--searchPath liquibase/jhipster-sample-app") {
		t.Fatalf("expected liquibase create command to include searchPath, got %+v", finding)
	}
}

func TestAnalyzeAliasesSkipsUnrelatedBinaryArtifactsAndValidatesLiquibaseJar(t *testing.T) {
	workspace := t.TempDir()
	mustWriteFile(t, filepath.Join(workspace, "build", "app.jar"), []byte("binary payload"))
	mustWriteFile(t, filepath.Join(workspace, "build", "Worker.class"), []byte("binary payload"))
	mustWriteFile(t, filepath.Join(workspace, "liquibase", "master.jar"), []byte("binary payload"))

	report, err := AnalyzeAliases(Options{WorkspaceRoot: workspace, CWD: workspace})
	if err != nil {
		t.Fatalf("AnalyzeAliases: %v", err)
	}
	if report.Prefiltered != 1 {
		t.Fatalf("expected only the Liquibase-named jar to be promoted, got %+v", report)
	}
	if report.Validated != 1 {
		t.Fatalf("expected one validated binary candidate, got %+v", report)
	}
	if len(report.Findings) != 1 {
		t.Fatalf("expected one finding, got %+v", report)
	}

	finding := report.Findings[0]
	if finding.Type != alias.ClassPrepare || finding.Kind != "lb" {
		t.Fatalf("unexpected binary Liquibase finding: %+v", finding)
	}
	if finding.Valid || !strings.Contains(finding.Error, "unsupported Liquibase changelog extension") {
		t.Fatalf("expected unsupported binary changelog diagnostic: %+v", finding)
	}
	if finding.File != filepath.ToSlash(filepath.Join("liquibase", "master.jar")) {
		t.Fatalf("unexpected binary Liquibase file: %+v", finding)
	}
	if finding.AliasPath != "liquibase.prep.s9s.yaml" {
		t.Fatalf("unexpected binary Liquibase alias path: %+v", finding)
	}
	if !strings.Contains(finding.CreateCommand, "sqlrs alias create liquibase prepare:lb -- update --changelog-file liquibase/master.jar") {
		t.Fatalf("unexpected binary Liquibase create command: %+v", finding)
	}
}

func TestAnalyzeAliasesValidatesNestedPsqlAndPgbenchCandidates(t *testing.T) {
	workspace := t.TempDir()
	mustWriteFile(t, filepath.Join(workspace, "team", "workloads", "schema.sql"), []byte("create table users(id int);\n"))
	mustWriteFile(t, filepath.Join(workspace, "team", "perf", "bench.sql"), []byte("select 1;\n"))

	report, err := AnalyzeAliases(Options{WorkspaceRoot: workspace, CWD: workspace})
	if err != nil {
		t.Fatalf("AnalyzeAliases: %v", err)
	}
	if len(report.Findings) != 2 {
		t.Fatalf("expected two findings, got %+v", report)
	}

	findings := make(map[string]Finding, len(report.Findings))
	for _, finding := range report.Findings {
		findings[finding.AliasPath] = finding
	}

	psqlFinding, ok := findings[filepath.ToSlash(filepath.Join("team", "workloads.prep.s9s.yaml"))]
	if !ok {
		t.Fatalf("missing nested psql finding: %+v", report.Findings)
	}
	if !psqlFinding.Valid {
		t.Fatalf("expected nested psql finding to be valid: %+v", psqlFinding)
	}
	if !strings.Contains(psqlFinding.CreateCommand, "sqlrs alias create team/workloads prepare:psql -- -f team/workloads/schema.sql") {
		t.Fatalf("unexpected nested psql create command: %+v", psqlFinding)
	}

	pgbenchFinding, ok := findings[filepath.ToSlash(filepath.Join("team", "perf.run.s9s.yaml"))]
	if !ok {
		t.Fatalf("missing nested pgbench finding: %+v", report.Findings)
	}
	if !pgbenchFinding.Valid {
		t.Fatalf("expected nested pgbench finding to be valid: %+v", pgbenchFinding)
	}
	if !strings.Contains(pgbenchFinding.CreateCommand, "sqlrs alias create team/perf run:pgbench -- -f team/perf/bench.sql") {
		t.Fatalf("unexpected nested pgbench create command: %+v", pgbenchFinding)
	}
}

func TestAnalyzeAliasesDeduplicatesAliasPathFindings(t *testing.T) {
	workspace := t.TempDir()
	mustWriteFile(t, filepath.Join(workspace, "batch", "001_init.sql"), []byte("create table users(id int);\n"))
	mustWriteFile(t, filepath.Join(workspace, "batch", "002_users.sql"), []byte("create table orders(id int);\n"))

	report, err := AnalyzeAliases(Options{WorkspaceRoot: workspace, CWD: workspace})
	if err != nil {
		t.Fatalf("AnalyzeAliases: %v", err)
	}
	if report.Suppressed != 1 {
		t.Fatalf("expected one duplicate suppression, got %+v", report)
	}
	if len(report.Findings) != 1 {
		t.Fatalf("expected one deduplicated finding, got %+v", report)
	}

	finding := report.Findings[0]
	if finding.AliasPath != filepath.ToSlash("batch.prep.s9s.yaml") {
		t.Fatalf("unexpected alias path: %+v", finding)
	}
	if finding.File != filepath.ToSlash(filepath.Join("batch", "001_init.sql")) {
		t.Fatalf("unexpected winning finding: %+v", finding)
	}
}

func TestAnalyzeAliasesCountsAllScannedFiles(t *testing.T) {
	workspace := t.TempDir()
	mustWriteFile(t, filepath.Join(workspace, "README.md"), []byte("create table ignored(id int);\n"))
	mustWriteFile(t, filepath.Join(workspace, "notes.txt"), []byte("select 1;\n"))
	mustWriteFile(t, filepath.Join(workspace, "schema.sql"), []byte("create table users(id int);\n"))

	report, err := AnalyzeAliases(Options{WorkspaceRoot: workspace, CWD: workspace})
	if err != nil {
		t.Fatalf("AnalyzeAliases: %v", err)
	}
	if report.Scanned != 3 {
		t.Fatalf("expected all visited files to be counted, got %+v", report)
	}
	if report.Prefiltered != 1 || report.Validated != 1 || len(report.Findings) != 1 {
		t.Fatalf("unexpected report counts: %+v", report)
	}
}

func mustWriteFile(t *testing.T, path string, contents []byte) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("mkdir %s: %v", filepath.Dir(path), err)
	}
	if err := os.WriteFile(path, contents, 0o600); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
}
