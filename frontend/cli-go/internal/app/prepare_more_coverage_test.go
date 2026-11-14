package app

import (
	"bytes"
	"errors"
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
	"github.com/sqlrs/cli/internal/config"
	"github.com/sqlrs/cli/internal/paths"
)

func TestParsePrepareArgsImageWhitespaceCoverage(t *testing.T) {
	if _, _, err := parsePrepareArgs([]string{"--image", " "}); err == nil {
		t.Fatalf("expected empty --image value error")
	}
}

func TestRunPrepareLiquibaseRemoteSuccessCoverage(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/v1/prepare-jobs":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusAccepted)
			io.WriteString(w, `{"job_id":"job-1","status_url":"/v1/prepare-jobs/job-1","events_url":"/v1/prepare-jobs/job-1/events"}`)
		case r.Method == http.MethodGet && r.URL.Path == "/v1/prepare-jobs/job-1/events":
			w.Header().Set("Content-Type", "application/x-ndjson")
			io.WriteString(w, `{"type":"status","ts":"2026-01-24T00:00:00Z","status":"succeeded"}`+"\n")
		case r.Method == http.MethodGet && r.URL.Path == "/v1/prepare-jobs/job-1":
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"job_id":"job-1","status":"succeeded","result":{"dsn":"postgres://sqlrs@local/instance/lb","instance_id":"lb","state_id":"state","image_id":"img","prepare_kind":"lb","prepare_args_normalized":"update"}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(server.Close)

	var stdout bytes.Buffer
	err := runPrepareLiquibase(
		&stdout,
		io.Discard,
		cli.PrepareOptions{Mode: "remote", Endpoint: server.URL, Timeout: time.Second},
		config.LoadedConfig{},
		t.TempDir(),
		t.TempDir(),
		[]string{"--image", "img", "--", "update"},
	)
	if err != nil {
		t.Fatalf("runPrepareLiquibase: %v", err)
	}
	if !strings.Contains(stdout.String(), "DSN=postgres://sqlrs@local/instance/lb") {
		t.Fatalf("unexpected output: %q", stdout.String())
	}
}

func TestPrepareResultLiquibaseErrorBranchesCoverage(t *testing.T) {
	_, _, err := prepareResult(
		stdoutAndErr{stdout: io.Discard, stderr: io.Discard},
		cli.PrepareOptions{},
		config.LoadedConfig{},
		"",
		t.TempDir(),
		[]string{"--image"},
	)
	if err == nil {
		t.Fatalf("expected parse error for prepareResult")
	}

	_, _, err = prepareResultLiquibase(
		stdoutAndErr{stdout: io.Discard, stderr: io.Discard},
		cli.PrepareOptions{},
		config.LoadedConfig{},
		"",
		t.TempDir(),
		[]string{"--image"},
	)
	if err == nil {
		t.Fatalf("expected parse error for prepareResultLiquibase")
	}

	projectRoot := t.TempDir()
	projectPath := filepath.Join(projectRoot, ".sqlrs", "config.yaml")
	if err := os.MkdirAll(filepath.Dir(projectPath), 0o700); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(projectPath, []byte("dbms: ["), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_, _, err = prepareResultLiquibase(
		stdoutAndErr{stdout: io.Discard, stderr: io.Discard},
		cli.PrepareOptions{},
		config.LoadedConfig{ProjectConfigPath: projectPath, Paths: paths.Dirs{ConfigDir: t.TempDir()}},
		"",
		t.TempDir(),
		[]string{"--", "update"},
	)
	if err == nil {
		t.Fatalf("expected resolve image error")
	}

	globalRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(globalRoot, "config.yaml"), []byte("liquibase: ["), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	_, _, err = prepareResultLiquibase(
		stdoutAndErr{stdout: io.Discard, stderr: io.Discard},
		cli.PrepareOptions{},
		config.LoadedConfig{Paths: paths.Dirs{ConfigDir: globalRoot}},
		"",
		t.TempDir(),
		[]string{"--image", "img", "--", "update"},
	)
	if err == nil {
		t.Fatalf("expected invalid Liquibase configuration error")
	}

	_, _, err = prepareResultLiquibase(
		stdoutAndErr{stdout: io.Discard, stderr: io.Discard},
		cli.PrepareOptions{},
		config.LoadedConfig{},
		"",
		t.TempDir(),
		[]string{"--image", "img", "--", "--searchPath", ""},
	)
	if err == nil {
		t.Fatalf("expected normalizeLiquibaseArgs error")
	}

	if runtime.GOOS == "windows" {
		_, _, err = prepareResultLiquibase(
			stdoutAndErr{stdout: io.Discard, stderr: io.Discard},
			cli.PrepareOptions{WSLDistro: "Ubuntu"},
			config.LoadedConfig{},
			"",
			"relative",
			[]string{"--image", "img", "--", "update"},
		)
		if err == nil {
			t.Fatalf("expected normalizeWorkDir conversion error")
		}
	}
}

func TestNormalizePsqlArgsConverterBranchesCoverage(t *testing.T) {
	cwd := t.TempDir()
	convertErr := func(string) (string, error) {
		return "", errors.New("boom")
	}
	if _, _, err := normalizePsqlArgs([]string{"-f", "file.sql"}, "", cwd, strings.NewReader(""), convertErr); err == nil {
		t.Fatalf("expected -f convert error")
	}
	if _, _, err := normalizePsqlArgs([]string{"--file=file.sql"}, "", cwd, strings.NewReader(""), convertErr); err == nil {
		t.Fatalf("expected --file= convert error")
	}
	if _, _, err := normalizePsqlArgs([]string{"-ffile.sql"}, "", cwd, strings.NewReader(""), convertErr); err == nil {
		t.Fatalf("expected -ffile convert error")
	}

	pathValue, _, err := normalizeFilePath("file.sql", "", cwd, nil)
	if err != nil || !strings.HasPrefix(pathValue, cwd) {
		t.Fatalf("expected fallback root to cwd, got %q err=%v", pathValue, err)
	}
}

func TestNormalizeLiquibaseArgsAdditionalBranchesCoverage(t *testing.T) {
	root := t.TempDir()
	cwd := root

	if _, err := normalizeLiquibaseArgs([]string{"--changelog-file= "}, root, cwd, nil); err == nil {
		t.Fatalf("expected empty changelog value error")
	}
	if _, err := normalizeLiquibaseArgs([]string{"--changelog-file=../outside.xml"}, root, cwd, nil); err == nil {
		t.Fatalf("expected changelog path validation error")
	}
	if _, err := normalizeLiquibaseArgs([]string{"--defaults-file=../outside.properties"}, root, cwd, nil); err == nil {
		t.Fatalf("expected defaults path validation error")
	}
	if _, err := normalizeLiquibaseArgs([]string{"--searchPath=../outside"}, root, cwd, nil); err == nil {
		t.Fatalf("expected searchPath path validation error")
	}
	if _, err := normalizeLiquibaseArgs([]string{"--search-path= "}, root, cwd, nil); err == nil {
		t.Fatalf("expected empty search-path value error")
	}
	args, err := normalizeLiquibaseArgs([]string{"--search-path=dir1"}, root, cwd, nil)
	if err != nil {
		t.Fatalf("normalizeLiquibaseArgs: %v", err)
	}
	if len(args) != 1 || args[0] == "--search-path=dir1" {
		t.Fatalf("expected --searchPath rewrite, got %+v", args)
	}
	if _, err := rewriteLiquibasePathArg("--searchPath", "../outside", root, cwd, nil); err == nil {
		t.Fatalf("expected searchPath path validation error")
	}
	if _, err := rewriteLiquibasePathArg("--defaults-file", " ", root, cwd, nil); err == nil {
		t.Fatalf("expected empty defaults path error")
	}
}

func TestRelativizeAndRelativeCoverage(t *testing.T) {
	abs := "/root/changelog.xml"
	if runtime.GOOS == "windows" {
		abs = `C:\root\changelog.xml`
	}
	args := []string{"--changelog-file", "--changelog-file=" + abs}
	out := relativizeLiquibaseArgs(args, "", "")
	if strings.Join(out, " ") != strings.Join(args, " ") {
		t.Fatalf("expected args unchanged for empty base, got %+v", out)
	}

	base := t.TempDir()
	out = relativizeLiquibaseArgs([]string{"--changelog-file", "--defaults-file", "--changelog-file=" + filepath.Join(base, "a.xml")}, base, base)
	if len(out) != 3 || !strings.HasPrefix(out[2], "--changelog-file=") {
		t.Fatalf("unexpected relativize result: %+v", out)
	}

	out = relativizeLiquibaseArgs([]string{"--changelog-file"}, base, base)
	if len(out) != 1 || out[0] != "--changelog-file" {
		t.Fatalf("expected missing-value branch to preserve arg, got %+v", out)
	}

	if got := toRelativeIfWithin(base, base); got != "." {
	}

	if runtime.GOOS == "windows" {
		if got := toRelativeIfWithin(`C:\root`, `D:\file.sql`); got != `D:\file.sql` {
			t.Fatalf("expected unchanged value on rel error, got %q", got)
		}
	}
}
