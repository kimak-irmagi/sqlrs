package app

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sqlrs/cli/internal/cli"
	"github.com/sqlrs/cli/internal/client"
	"github.com/sqlrs/cli/internal/inputset"
	"github.com/sqlrs/cli/internal/refctx"
)

func TestProvenanceTraceHelperErrorAndFallbackBranches(t *testing.T) {
	root := t.TempDir()

	for _, tt := range []struct {
		name string
		kind string
		args []string
	}{
		{name: "psql normalize error", kind: "psql", args: []string{"-f"}},
		{name: "liquibase normalize error", kind: "lb", args: []string{"update", "--changelog-file"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			_, err := collectPrepareTrace(stageRunRequest{
				kind:          tt.kind,
				parsed:        prepareArgs{PsqlArgs: tt.args},
				workspaceRoot: root,
				cwd:           root,
			}, cli.PrepareOptions{}, nil)
			if err == nil {
				t.Fatal("expected normalization error")
			}
		})
	}

	trace, err := collectPrepareTrace(stageRunRequest{
		kind:          "custom",
		parsed:        prepareArgs{PsqlArgs: []string{"one", "two"}},
		workspaceRoot: root,
		cwd:           root,
	}, cli.PrepareOptions{}, nil)
	if err != nil || len(trace.NormalizedArgs) != 2 {
		t.Fatalf("custom trace = %#v, %v", trace, err)
	}

	_, err = collectPrepareTrace(stageRunRequest{
		class:         "alias",
		kind:          "custom",
		aliasPath:     filepath.Join(root, "missing.yaml"),
		workspaceRoot: root,
		cwd:           root,
	}, cli.PrepareOptions{}, nil)
	if err == nil {
		t.Fatal("expected missing alias trace input error")
	}

	actualRef := &refctx.Context{
		RepoRoot:   root,
		BaseDir:    filepath.Join(root, "base"),
		FileSystem: inputset.OSFileSystem{},
	}
	gotRoot, gotBase, _ := traceCollectorContext(stageRunRequest{}, actualRef)
	if gotRoot != root || gotBase != actualRef.BaseDir {
		t.Fatalf("traceCollectorContext = %q, %q", gotRoot, gotBase)
	}
	if got := traceBindBaseDir(stageRunRequest{cwd: root}, nil); got != root {
		t.Fatalf("nil ref base = %q", got)
	}
	if got := traceBindBaseDir(stageRunRequest{kind: "psql", cwd: root}, &refctx.Context{}); got != root {
		t.Fatalf("empty ref base = %q", got)
	}
	if got := traceBindBaseDir(stageRunRequest{kind: "lb", cwd: root}, actualRef); got != actualRef.BaseDir {
		t.Fatalf("liquibase ref base = %q", got)
	}
}

func TestProvenanceInputAndPathHelperBranches(t *testing.T) {
	root := t.TempDir()
	file := filepath.Join(root, "input.sql")
	if err := os.WriteFile(file, []byte("select 1;\n"), 0o600); err != nil {
		t.Fatalf("write input: %v", err)
	}

	oidFS := traceBlobOIDFS{FileSystem: inputset.OSFileSystem{}, oid: "blob-oid"}
	if got, err := traceInputHash(file, oidFS); err != nil || got != "blob-oid" {
		t.Fatalf("traceInputHash blob = %q, %v", got, err)
	}
	if _, err := traceInputHash(file, traceBlobOIDFS{FileSystem: inputset.OSFileSystem{}, err: errors.New("oid failed")}); err == nil {
		t.Fatal("expected BlobOID error")
	}
	if _, err := traceInputHash(filepath.Join(root, "missing.sql"), inputset.OSFileSystem{}); err == nil {
		t.Fatal("expected read error")
	}
	if _, err := traceInputFromPath(filepath.Join(root, "missing.sql"), root, inputset.OSFileSystem{}); err == nil {
		t.Fatal("expected trace input error")
	}

	inputs := traceInputsFromSet(inputset.InputSet{Entries: []inputset.InputEntry{{AbsPath: file, Hash: "hash"}}}, root)
	if len(inputs) != 1 || inputs[0].Path != "input.sql" {
		t.Fatalf("trace inputs = %#v", inputs)
	}
	if got := relativeTracePath(root, root); got != "." {
		t.Fatalf("relative root = %q", got)
	}
	if got := relativeTracePath(root, filepath.Join(t.TempDir(), "outside")); !strings.Contains(got, "outside") {
		t.Fatalf("outside path = %q", got)
	}

	result := cacheExplainResult(prepareTraceBase{}, client.CacheExplainPrepareResponse{})
	if result.Decision != "miss" {
		t.Fatalf("default decision = %q", result.Decision)
	}
}

func TestProvenanceArtifactAndOutcomeHelperBranches(t *testing.T) {
	root := t.TempDir()
	if got := resolveProvenancePath(root, " "); got != "" {
		t.Fatalf("blank path = %q", got)
	}
	abs := filepath.Join(root, "artifact.json")
	if got := resolveProvenancePath("ignored", abs); got != abs {
		t.Fatalf("absolute path = %q", got)
	}
	if got := resolveProvenancePath("", "artifact.json"); got != "artifact.json" {
		t.Fatalf("default cwd path = %q", got)
	}
	if err := writeProvenanceArtifact("", provenanceArtifact{}); err == nil {
		t.Fatal("expected blank provenance path error")
	}
	blocker := filepath.Join(root, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o600); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	if err := writeProvenanceArtifact(filepath.Join(blocker, "artifact.json"), provenanceArtifact{}); err == nil {
		t.Fatal("expected provenance mkdir error")
	}
	if err := writeProvenanceArtifact(root, provenanceArtifact{}); err == nil {
		t.Fatal("expected provenance write-to-directory error")
	}

	primary := errors.New("primary")
	secondary := errors.New("secondary")
	if got := joinStageErrors(nil, secondary); !errors.Is(got, secondary) {
		t.Fatalf("joinStageErrors nil primary = %v", got)
	}
	if got := joinStageErrors(primary, nil); !errors.Is(got, primary) {
		t.Fatalf("joinStageErrors nil secondary = %v", got)
	}
	if got := joinStageErrors(primary, secondary); got == nil || !strings.Contains(got.Error(), "primary; secondary") {
		t.Fatalf("joinStageErrors combined = %v", got)
	}

	tasks := []client.PlanTask{
		{Type: "prepare_instance", Input: &client.TaskInput{Kind: "image", ID: "ignored"}},
		{Type: "prepare_instance", Input: &client.TaskInput{Kind: "state", ID: "state-1"}},
		{Type: "state_execute", OutputStateID: "state-2"},
		{Type: "state_execute"},
	}
	if got := finalPlanStateID(tasks); got != "state-2" {
		t.Fatalf("final state = %q", got)
	}
	if got := finalPlanStateID([]client.PlanTask{{Type: "prepare_instance", Input: &client.TaskInput{Kind: "state", ID: "state-1"}}}); got != "state-1" {
		t.Fatalf("prepare state = %q", got)
	}
	if got := finalPlanStateID(nil); got != "" {
		t.Fatalf("empty final state = %q", got)
	}
}

type traceBlobOIDFS struct {
	inputset.FileSystem
	oid string
	err error
}

func (f traceBlobOIDFS) BlobOID(string) (string, error) {
	return f.oid, f.err
}
