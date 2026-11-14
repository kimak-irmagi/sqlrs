package app

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/sqlrs/cli/internal/cli"
	"github.com/sqlrs/cli/internal/client"
	"github.com/sqlrs/cli/internal/config"
	"github.com/sqlrs/cli/internal/paths"
)

func TestParsePrepareArgsSplitsPsqlArgs(t *testing.T) {
	opts, showHelp, err := parsePrepareArgs([]string{"--image", "img", "--", "-f", "init.sql"})
	if err != nil {
		t.Fatalf("parse args: %v", err)
	}
	if showHelp {
		t.Fatalf("did not expect help")
	}
	if opts.Image != "img" {
		t.Fatalf("expected image img, got %q", opts.Image)
	}
	if len(opts.PsqlArgs) != 2 || opts.PsqlArgs[0] != "-f" || opts.PsqlArgs[1] != "init.sql" {
		t.Fatalf("unexpected psql args: %#v", opts.PsqlArgs)
	}
}

func TestParsePrepareArgsTreatsUnknownAsPsql(t *testing.T) {
	opts, showHelp, err := parsePrepareArgs([]string{"-f", "init.sql"})
	if err != nil {
		t.Fatalf("parse args: %v", err)
	}
	if showHelp {
		t.Fatalf("did not expect help")
	}
	if len(opts.PsqlArgs) != 2 || opts.PsqlArgs[0] != "-f" {
		t.Fatalf("unexpected psql args: %#v", opts.PsqlArgs)
	}
}

func TestParsePrepareArgsNoWatch(t *testing.T) {
	opts, showHelp, err := parsePrepareArgs([]string{"--no-watch", "--", "-c", "select 1"})
	if err != nil {
		t.Fatalf("parse args: %v", err)
	}
	if showHelp {
		t.Fatalf("did not expect help")
	}
	if opts.Watch {
		t.Fatalf("expected watch disabled")
	}
	if !opts.WatchSpecified {
		t.Fatalf("expected watch mode to be marked as explicitly set")
	}
}

func TestParsePrepareArgsWatch(t *testing.T) {
	opts, showHelp, err := parsePrepareArgs([]string{"--watch", "--", "-c", "select 1"})
	if err != nil {
		t.Fatalf("parse args: %v", err)
	}
	if showHelp {
		t.Fatalf("did not expect help")
	}
	if !opts.Watch {
		t.Fatalf("expected watch enabled")
	}
	if !opts.WatchSpecified {
		t.Fatalf("expected watch mode to be marked as explicitly set")
	}
}

func TestParsePrepareArgsImageValidation(t *testing.T) {
	t.Run("missing separate value", func(t *testing.T) {
		_, _, err := parsePrepareArgs([]string{"--image"})
		if err == nil || !strings.Contains(err.Error(), "Missing value for --image") {
			t.Fatalf("expected missing image value error, got %v", err)
		}
	})

	t.Run("blank equals value", func(t *testing.T) {
		_, _, err := parsePrepareArgs([]string{"--image=   "})
		if err == nil || !strings.Contains(err.Error(), "Missing value for --image") {
			t.Fatalf("expected blank image value error, got %v", err)
		}
	})
}

func TestResolvePrepareImagePrecedence(t *testing.T) {
	temp := t.TempDir()
	globalDir := filepath.Join(temp, "config")
	if err := os.MkdirAll(globalDir, 0o700); err != nil {
		t.Fatalf("mkdir global: %v", err)
	}
	globalPath := filepath.Join(globalDir, "config.yaml")
	if err := os.WriteFile(globalPath, []byte("dbms:\n  image: global\n"), 0o600); err != nil {
		t.Fatalf("write global config: %v", err)
	}

	projectDir := filepath.Join(temp, "project", ".sqlrs")
	if err := os.MkdirAll(projectDir, 0o700); err != nil {
		t.Fatalf("mkdir project: %v", err)
	}
	projectPath := filepath.Join(projectDir, "config.yaml")
	if err := os.WriteFile(projectPath, []byte("dbms:\n  image: workspace\n"), 0o600); err != nil {
		t.Fatalf("write project config: %v", err)
	}

	cfg := config.LoadedConfig{
		Paths:             paths.Dirs{ConfigDir: globalDir},
		ProjectConfigPath: projectPath,
	}

	image, source, err := resolvePrepareImage("", cfg)
	if err != nil {
		t.Fatalf("resolve image: %v", err)
	}
	if image != "workspace" || source != "workspace config" {
		t.Fatalf("expected workspace config, got %q (%s)", image, source)
	}

	image, source, err = resolvePrepareImage("cli", cfg)
	if err != nil {
		t.Fatalf("resolve image: %v", err)
	}
	if image != "cli" || source != "command line" {
		t.Fatalf("expected cli override, got %q (%s)", image, source)
	}
}

func TestNormalizePsqlArgsAbsoluteAndStdin(t *testing.T) {
	cwd := t.TempDir()
	args := []string{"-f", "init.sql", "-f", "-"}
	stdin := bytes.NewBufferString("select 1;")

	normalized, stdinValue, err := normalizePsqlArgs(args, cwd, cwd, stdin, nil)
	if err != nil {
		t.Fatalf("normalize args: %v", err)
	}
	if stdinValue == nil || *stdinValue != "select 1;" {
		t.Fatalf("expected stdin content, got %#v", stdinValue)
	}
	expected := filepath.Join(cwd, "init.sql")
	if normalized[0] != "-f" || normalized[1] != expected {
		t.Fatalf("expected absolute file path, got %#v", normalized)
	}
}

func TestFormatImageSource(t *testing.T) {
	out := formatImageSource("img", "global config")
	if !strings.Contains(out, "dbms.image=img") {
		t.Fatalf("unexpected output: %q", out)
	}
	if !strings.Contains(out, "source: global config") {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestNormalizeWorkDir(t *testing.T) {
	out, err := normalizeWorkDir("", nil)
	if err != nil {
		t.Fatalf("normalizeWorkDir: %v", err)
	}
	if out != "" {
		t.Fatalf("expected empty output, got %q", out)
	}

	workDir := "/work"
	if runtime.GOOS == "windows" {
		workDir = "C:\\work"
	}
	want := "wsl:" + workDir
	out, err = normalizeWorkDir(workDir, func(path string) (string, error) {
		return "wsl:" + path, nil
	})
	if err != nil {
		t.Fatalf("normalizeWorkDir: %v", err)
	}
	if out != want {
		t.Fatalf("unexpected output: %q", out)
	}
}

func TestNormalizeLiquibaseArgsRewritesPaths(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "examples")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	args := []string{"update", "--changelog-file", "db/changelog.xml", "--searchPath", "db,extras"}
	out, err := normalizeLiquibaseArgs(args, root, cwd, nil)
	if err != nil {
		t.Fatalf("normalizeLiquibaseArgs: %v", err)
	}

	wantChangelog := filepath.Join(cwd, "db", "changelog.xml")
	wantSearch := filepath.Join(cwd, "db") + "," + filepath.Join(cwd, "extras")

	if out[1] != "--changelog-file" || out[2] != wantChangelog {
		t.Fatalf("unexpected changelog rewrite: %v", out)
	}
	if out[3] != "--searchPath" || out[4] != wantSearch {
		t.Fatalf("unexpected searchPath rewrite: %v", out)
	}
}

func TestNormalizeLiquibaseArgsAcceptsSearchPathAlias(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "examples")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	args := []string{"update", "--search-path", "db"}
	out, err := normalizeLiquibaseArgs(args, root, cwd, nil)
	if err != nil {
		t.Fatalf("normalizeLiquibaseArgs: %v", err)
	}
	if out[1] != "--searchPath" {
		t.Fatalf("expected --searchPath, got %v", out)
	}
}

func TestNormalizeLiquibaseArgsRejectsEmptyValues(t *testing.T) {
	_, err := normalizeLiquibaseArgs([]string{"update", "--defaults-file="}, "", "", nil)
	if err == nil || !strings.Contains(err.Error(), "Missing value for --defaults-file") {
		t.Fatalf("expected defaults-file error, got %v", err)
	}

	_, err = normalizeLiquibaseArgs([]string{"update", "--searchPath", ""}, "", "", nil)
	if err == nil || !strings.Contains(err.Error(), "searchPath is empty") {
		t.Fatalf("expected searchPath error, got %v", err)
	}
}

func TestRelativizeLiquibaseArgs(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "examples")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	path := filepath.Join(cwd, "db", "changelog.xml")
	args := []string{"update", "--changelog-file", path}
	out := relativizeLiquibaseArgs(args, root, cwd)
	if out[1] != "--changelog-file" || out[2] == path {
		t.Fatalf("expected relative changelog, got %v", out)
	}
	if strings.HasPrefix(out[2], "..") {
		t.Fatalf("expected within root, got %v", out)
	}
}

func TestPrintPrepareJobRefs(t *testing.T) {
	var out bytes.Buffer
	printPrepareJobRefs(&out, client.PrepareJobAccepted{
		JobID:     "job-1",
		StatusURL: "/v1/prepare-jobs/job-1",
		EventsURL: "/v1/prepare-jobs/job-1/events",
	})
	text := out.String()
	if !strings.Contains(text, "JOB_ID=job-1") {
		t.Fatalf("expected job id in output, got %q", text)
	}
	if !strings.Contains(text, "STATUS_URL=/v1/prepare-jobs/job-1") {
		t.Fatalf("expected status url in output, got %q", text)
	}
	if !strings.Contains(text, "EVENTS_URL=/v1/prepare-jobs/job-1/events") {
		t.Fatalf("expected events url in output, got %q", text)
	}
}

func TestPrintRunSkipped(t *testing.T) {
	var out bytes.Buffer
	printRunSkipped(&out, "prepare_detached")
	if out.String() != "RUN_SKIPPED=prepare_detached\n" {
		t.Fatalf("unexpected output: %q", out.String())
	}
}

func TestPrepareResultHelp(t *testing.T) {
	root := t.TempDir()
	var out bytes.Buffer

	_, handled, err := prepareResult(
		stdoutAndErr{stdout: &out, stderr: io.Discard},
		cli.PrepareOptions{},
		config.LoadedConfig{},
		root,
		root,
		[]string{"--help"},
	)
	if err != nil {
		t.Fatalf("prepareResult: %v", err)
	}
	if !handled {
		t.Fatalf("expected handled=true for help")
	}
	if !strings.Contains(out.String(), "sqlrs prepare") {
		t.Fatalf("expected usage output, got %q", out.String())
	}
}

func TestPrepareResultNoWatchCompositeRun(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/prepare-jobs" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		io.WriteString(w, `{"job_id":"job-1","status_url":"/v1/prepare-jobs/job-1","events_url":"/v1/prepare-jobs/job-1/events"}`)
	}))
	defer server.Close()

	root := t.TempDir()
	var out bytes.Buffer
	var errOut bytes.Buffer
	_, handled, err := prepareResult(
		stdoutAndErr{stdout: &out, stderr: &errOut},
		cli.PrepareOptions{
			Mode:         "remote",
			Endpoint:     server.URL,
			Timeout:      time.Second,
			CompositeRun: true,
		},
		config.LoadedConfig{},
		root,
		root,
		[]string{"--no-watch", "--image", "image", "--", "-c", "select 1"},
	)
	if err != nil {
		t.Fatalf("prepareResult: %v", err)
	}
	if !handled {
		t.Fatalf("expected handled=true")
	}
	text := out.String()
	if !strings.Contains(text, "JOB_ID=job-1") {
		t.Fatalf("expected job refs output, got %q", text)
	}
	if !strings.Contains(text, "RUN_SKIPPED=prepare_not_watched") {
		t.Fatalf("expected run skipped output, got %q", text)
	}
}

func TestPrepareResultNoWatchSubmitError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	root := t.TempDir()
	_, handled, err := prepareResult(
		stdoutAndErr{stdout: io.Discard, stderr: io.Discard},
		cli.PrepareOptions{
			Mode:     "remote",
			Endpoint: server.URL,
			Timeout:  time.Second,
		},
		config.LoadedConfig{},
		root,
		root,
		[]string{"--no-watch", "--image", "image", "--", "-c", "select 1"},
	)
	if err == nil {
		t.Fatalf("expected submit error")
	}
	if handled {
		t.Fatalf("expected handled=false on submit error")
	}
}

func TestPrepareResultLiquibaseNoWatchCompositeRun(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v1/prepare-jobs" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusAccepted)
		io.WriteString(w, `{"job_id":"job-lb","status_url":"/v1/prepare-jobs/job-lb","events_url":"/v1/prepare-jobs/job-lb/events"}`)
	}))
	defer server.Close()

	root := t.TempDir()
	var out bytes.Buffer
	var errOut bytes.Buffer
	_, handled, err := prepareResultLiquibase(
		stdoutAndErr{stdout: &out, stderr: &errOut},
		cli.PrepareOptions{
			Mode:         "remote",
			Endpoint:     server.URL,
			Timeout:      time.Second,
			CompositeRun: true,
		},
		config.LoadedConfig{},
		root,
		root,
		[]string{"--no-watch", "--image", "image", "--", "update", "--changelog-file", "changelog.xml"},
	)
	if err != nil {
		t.Fatalf("prepareResultLiquibase: %v", err)
	}
	if !handled {
		t.Fatalf("expected handled=true")
	}
	text := out.String()
	if !strings.Contains(text, "JOB_ID=job-lb") {
		t.Fatalf("expected job refs output, got %q", text)
	}
	if !strings.Contains(text, "RUN_SKIPPED=prepare_not_watched") {
		t.Fatalf("expected run skipped output, got %q", text)
	}
}

func TestPrepareResultLiquibaseNoWatchSubmitError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	root := t.TempDir()
	_, handled, err := prepareResultLiquibase(
		stdoutAndErr{stdout: io.Discard, stderr: io.Discard},
		cli.PrepareOptions{
			Mode:     "remote",
			Endpoint: server.URL,
			Timeout:  time.Second,
		},
		config.LoadedConfig{},
		root,
		root,
		[]string{"--no-watch", "--image", "image", "--", "update", "--changelog-file", "changelog.xml"},
	)
	if err == nil {
		t.Fatalf("expected submit error")
	}
	if handled {
		t.Fatalf("expected handled=false on submit error")
	}
}

func TestPrepareResultLiquibaseHelpPrintsUsage(t *testing.T) {
	root := t.TempDir()
	var out bytes.Buffer

	_, handled, err := prepareResultLiquibase(
		stdoutAndErr{stdout: &out, stderr: io.Discard},
		cli.PrepareOptions{},
		config.LoadedConfig{},
		root,
		root,
		[]string{"--help"},
	)
	if err != nil {
		t.Fatalf("prepareResultLiquibase: %v", err)
	}
	if !handled {
		t.Fatalf("expected handled=true for help")
	}
	if !strings.Contains(out.String(), "sqlrs prepare") {
		t.Fatalf("expected usage output, got %q", out.String())
	}
}
