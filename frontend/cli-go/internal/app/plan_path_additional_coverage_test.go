package app

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/sqlrs/cli/internal/cli"
	"github.com/sqlrs/cli/internal/config"
	"github.com/sqlrs/cli/internal/paths"
)

func TestRunPlanKindAdditionalBranches(t *testing.T) {
	t.Run("unsupported-kind", func(t *testing.T) {
		err := runPlanKind(&bytes.Buffer{}, &bytes.Buffer{}, cli.PrepareOptions{}, config.LoadedConfig{}, t.TempDir(), t.TempDir(), []string{"--image", "img", "--", "-c", "select 1"}, "json", "unknown")
		if err == nil || !strings.Contains(err.Error(), "unsupported plan kind") {
			t.Fatalf("expected unsupported kind error, got %v", err)
		}
	})

	t.Run("liquibase-exec-error", func(t *testing.T) {
		root := t.TempDir()
		projectConfig := filepath.Join(root, ".sqlrs", "config.yaml")
		if err := os.MkdirAll(filepath.Dir(projectConfig), 0o700); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(projectConfig, []byte("liquibase: ["), 0o600); err != nil {
			t.Fatalf("write project config: %v", err)
		}

		err := runPlanKind(
			&bytes.Buffer{},
			&bytes.Buffer{},
			cli.PrepareOptions{},
			config.LoadedConfig{
				ProjectConfigPath: projectConfig,
				Paths:             paths.Dirs{ConfigDir: t.TempDir()},
			},
			root,
			root,
			[]string{"--image", "img", "--", "update"},
			"json",
			"lb",
		)
		if err == nil {
			t.Fatalf("expected liquibase exec config error")
		}
	})

	t.Run("liquibase-global-config-error", func(t *testing.T) {
		root := t.TempDir()
		projectConfig := filepath.Join(root, ".sqlrs", "config.yaml")
		if err := os.MkdirAll(filepath.Dir(projectConfig), 0o700); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(projectConfig, []byte("liquibase: {}\n"), 0o600); err != nil {
			t.Fatalf("write project config: %v", err)
		}

		globalDir := t.TempDir()
		globalConfig := filepath.Join(globalDir, "config.yaml")
		if err := os.WriteFile(globalConfig, []byte("liquibase: ["), 0o600); err != nil {
			t.Fatalf("write global config: %v", err)
		}

		err := runPlanKind(
			&bytes.Buffer{},
			&bytes.Buffer{},
			cli.PrepareOptions{},
			config.LoadedConfig{
				ProjectConfigPath: projectConfig,
				Paths:             paths.Dirs{ConfigDir: globalDir},
			},
			root,
			root,
			[]string{"--image", "img", "--", "update"},
			"json",
			"lb",
		)
		if err == nil {
			t.Fatalf("expected liquibase exec mode error")
		}
	})

	t.Run("liquibase-normalize-args-error", func(t *testing.T) {
		root := t.TempDir()

		err := runPlanKind(
			&bytes.Buffer{},
			&bytes.Buffer{},
			cli.PrepareOptions{},
			config.LoadedConfig{
				Paths: paths.Dirs{ConfigDir: t.TempDir()},
			},
			root,
			root,
			[]string{"--image", "img", "--", "--search-path="},
			"json",
			"lb",
		)
		if err == nil || !strings.Contains(err.Error(), "Missing value for --search-path") {
			t.Fatalf("expected liquibase args normalization error, got %v", err)
		}
	})

	t.Run("remote-liquibase-rejects-removed-windows-executable", func(t *testing.T) {
		root := t.TempDir()
		var requests atomic.Int32
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			requests.Add(1)
			w.WriteHeader(http.StatusInternalServerError)
		}))
		t.Cleanup(server.Close)
		projectConfig := filepath.Join(root, ".sqlrs", "config.yaml")
		if err := os.MkdirAll(filepath.Dir(projectConfig), 0o700); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(projectConfig, []byte("liquibase:\n  exec: liquibase.bat\n"), 0o600); err != nil {
			t.Fatalf("write project config: %v", err)
		}

		err := runPlanKind(
			&bytes.Buffer{},
			&bytes.Buffer{},
			cli.PrepareOptions{Mode: "remote", Endpoint: server.URL},
			config.LoadedConfig{
				ProjectConfigPath: projectConfig,
				Paths:             paths.Dirs{ConfigDir: t.TempDir()},
			},
			root,
			root,
			[]string{"--image", "img", "--", "update"},
			"json",
			"lb",
		)
		if requests.Load() != 0 {
			t.Fatalf("removed runner setting caused %d remote requests", requests.Load())
		}
		if err == nil || !strings.Contains(err.Error(), "liquibase.exec") {
			t.Fatalf("expected removed runner setting error, got %v", err)
		}
	})

	t.Run("liquibase-normalize-workdir-error", func(t *testing.T) {
		root := t.TempDir()
		projectConfig := filepath.Join(root, ".sqlrs", "config.yaml")
		if err := os.MkdirAll(filepath.Dir(projectConfig), 0o700); err != nil {
			t.Fatalf("mkdir: %v", err)
		}
		if err := os.WriteFile(projectConfig, []byte("liquibase: {}\n"), 0o600); err != nil {
			t.Fatalf("write project config: %v", err)
		}

		err := runPlanKind(
			&bytes.Buffer{},
			&bytes.Buffer{},
			cli.PrepareOptions{WSLDistro: "Ubuntu", Mode: "remote"},
			config.LoadedConfig{
				ProjectConfigPath: projectConfig,
				Paths:             paths.Dirs{ConfigDir: t.TempDir()},
			},
			root,
			"relative-workdir",
			[]string{"--image", "img", "--", "update"},
			"json",
			"lb",
		)
		if err == nil || !strings.Contains(err.Error(), "path is not absolute") {
			t.Fatalf("expected normalize workdir error, got %v", err)
		}
	})
}

func TestIsWithinRelError(t *testing.T) {
	if isWithin("bad\x00base", "bad\x00target") {
		t.Fatalf("expected false when filepath.Rel fails")
	}
}
